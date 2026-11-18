package engine

import (
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"
	_ "time/tzdata"

	"gcode/internal/contracts"
	"github.com/robfig/cron/v3"
)

var automationCronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

func nextAutomation(a contracts.Automation, now time.Time) (time.Time, error) {
	if !a.Enabled || a.Mode == "manual" {
		return time.Time{}, nil
	}
	if a.Mode == "interval" {
		if a.IntervalSeconds < 60 || a.IntervalSeconds > 366*24*3600 {
			return time.Time{}, errors.New("interval must be between 60 seconds and 366 days")
		}
		return now.Add(time.Duration(a.IntervalSeconds) * time.Second), nil
	}
	if a.Mode != "cron" {
		return time.Time{}, errors.New("unknown schedule mode")
	}
	loc, err := time.LoadLocation(a.Timezone)
	if err != nil {
		return time.Time{}, fmt.Errorf("timezone: %w", err)
	}
	spec, err := automationCronParser.Parse(a.Cron)
	if err != nil {
		return time.Time{}, err
	}
	next := spec.Next(now.In(loc))
	if next.IsZero() {
		return next, errors.New("cron has no future occurrence")
	}
	return next, nil
}

func (e *Engine) automationIndex(id string) int {
	return slices.IndexFunc(e.state.Automations, func(a contracts.Automation) bool { return a.ID == id })
}

func (e *Engine) automationBusy(id string) bool {
	return slices.ContainsFunc(e.state.AutomationRuns, func(r contracts.AutomationRun) bool { return r.AutomationID == id && r.Status == "running" })
}

func (e *Engine) automationCommandLocked(command contracts.Command) error {
	i := e.automationIndex(command.ID)
	switch command.Kind {
	case "automation.save":
		a := *command.Automation
		a.Name, a.Prompt = strings.TrimSpace(a.Name), strings.TrimSpace(a.Prompt)
		if a.Name == "" || a.Prompt == "" {
			return errors.New("name and prompt are required")
		}
		if _, _, err := e.resolveModel(a.ModelID); err != nil {
			return err
		}
		if a.TargetSessionID != "" {
			target := e.sessionIndex(a.TargetSessionID)
			if target < 0 || e.state.Sessions[target].Archived || e.state.Sessions[target].SubagentType != "" {
				return errors.New("target session is unavailable")
			}
			a.ProjectID, a.Continuous = e.state.Sessions[target].ProjectID, true
		}
		if a.ProjectID != "" && !slices.ContainsFunc(e.state.Projects, func(p contracts.Project) bool { return p.ID == a.ProjectID }) {
			return errors.New("project not found")
		}
		if a.Timezone == "" {
			a.Timezone = "Asia/Shanghai"
		}
		// Validate disabled definitions too, before they can be resumed.
		valid := a
		valid.Enabled = true
		if _, err := nextAutomation(valid, time.Now()); err != nil {
			return err
		}
		i = e.automationIndex(a.ID)
		if i >= 0 && e.automationBusy(a.ID) {
			return errors.New("stop the automation before editing it")
		}
		if i < 0 {
			if a.ID != "" {
				return errors.New("automation not found")
			}
			a.ID, a.CreatedAt = id("automation"), time.Now()
			a.SessionID = ""
		} else {
			previous := e.state.Automations[i]
			a.CreatedAt, a.SessionID = previous.CreatedAt, previous.SessionID
			if previous.TargetSessionID != a.TargetSessionID || previous.ProjectID != a.ProjectID {
				a.SessionID = ""
			}
		}
		if a.TargetSessionID != "" {
			a.SessionID = a.TargetSessionID
		}
		a.NextAt, _ = nextAutomation(a, time.Now())
		if i < 0 {
			e.state.Automations = append(e.state.Automations, a)
		} else {
			e.state.Automations[i] = a
		}
	case "automation.toggle":
		if i < 0 {
			return errors.New("automation not found")
		}
		a := &e.state.Automations[i]
		a.Enabled = command.Enabled
		next, err := nextAutomation(*a, time.Now())
		if err != nil {
			return err
		}
		a.NextAt = next
	case "automation.delete":
		if i < 0 {
			return errors.New("automation not found")
		}
		if e.automationBusy(command.ID) {
			return errors.New("pause the automation and wait for it to stop before deleting it")
		}
		e.state.Automations = slices.Delete(e.state.Automations, i, i+1)
		e.state.AutomationRuns = slices.DeleteFunc(e.state.AutomationRuns, func(r contracts.AutomationRun) bool { return r.AutomationID == command.ID })
	case "automation.run", "automation.fire":
		if i < 0 {
			return errors.New("automation not found")
		}
		if command.Kind == "automation.fire" {
			a := e.state.Automations[i]
			if !a.Enabled || a.NextAt.IsZero() || a.NextAt.After(time.Now()) || e.automationBusy(a.ID) {
				return nil
			}
		}
		return e.fireAutomationLocked(i, time.Now())
	default:
		return errors.New("unknown automation command")
	}
	if err := e.persistLocked(); err != nil {
		return err
	}
	// Cancellation is a side effect, performed only after the pause is durable.
	if command.Kind == "automation.toggle" && !command.Enabled {
		for _, r := range e.state.AutomationRuns {
			if r.AutomationID == command.ID && r.Status == "running" {
				if run := e.runs[r.SessionID]; run != nil && slices.ContainsFunc(e.state.Runs, func(record contracts.Run) bool {
					return record.ID == run.id && record.RequestID == r.ID
				}) {
					run.cancel()
				}
			}
		}
	}
	e.publishLocked(contracts.Event{Kind: "state.changed"})
	return nil
}

func (e *Engine) fireAutomationLocked(i int, now time.Time) error {
	a := e.state.Automations[i]
	if e.automationBusy(a.ID) {
		return errors.New("automation is already running")
	}
	active := 0
	for _, r := range e.state.AutomationRuns {
		if r.Status == "running" {
			active++
		}
	}
	if active >= 3 {
		return errors.New("automation concurrency limit reached")
	}
	var preparationErr error
	if a.Continuous && a.SessionID != "" {
		si := e.sessionIndex(a.SessionID)
		if si < 0 || e.state.Sessions[si].Archived {
			preparationErr = errors.New("automation session is unavailable")
		}
		if e.runs[a.SessionID] != nil {
			return errors.New("automation session is busy")
		}
	}
	r := contracts.AutomationRun{ID: id("automation-run"), AutomationID: a.ID, Status: "running", StartedAt: now}
	if !a.Continuous || a.SessionID == "" {
		session, err := e.newSessionLocked(a.ProjectID, a.Name, "")
		preparationErr = err
		if err == nil {
			a.SessionID = session.ID
		}
	}
	r.SessionID = a.SessionID
	if a.Once {
		a.Enabled = false
	}
	a.NextAt, _ = nextAutomation(a, now)
	if a.Mode == "interval" {
		a.NextAt = time.Time{}
	}
	e.state.Automations[i] = a
	e.state.AutomationRuns = append(e.state.AutomationRuns, r)
	// startLocked persists the accepted run and schedule together before launching
	// the model worker. A restart never replays an accepted prompt.
	err := preparationErr
	if err == nil {
		err = e.sendLocked(contracts.SendRequest{SessionID: r.SessionID, ProjectID: a.ProjectID, Text: a.Prompt, ModelID: a.ModelID, ReasoningLevel: a.ReasoningLevel, RequestID: r.ID})
	}
	if err != nil {
		j := len(e.state.AutomationRuns) - 1
		e.state.AutomationRuns[j].Status, e.state.AutomationRuns[j].Error, e.state.AutomationRuns[j].FinishedAt = "failed", err.Error(), now
		if a.Mode == "interval" {
			e.state.Automations[i].NextAt, _ = nextAutomation(a, now)
		}
		if saveErr := e.persistLocked(r.SessionID); saveErr != nil {
			return saveErr
		}
	}
	e.publishLocked(contracts.Event{Kind: "state.changed"})
	// Execution failures are stored in history rather than erasing the attempt.
	return nil
}

// recoverAutomationsLocked skips slots missed while the host was offline.
func (e *Engine) recoverAutomationsLocked(now time.Time) error {
	for i := range e.state.AutomationRuns {
		r := &e.state.AutomationRuns[i]
		if r.Status == "running" {
			r.Status, r.Error, r.FinishedAt = "interrupted", "Host stopped during execution", now
			for _, run := range e.state.Runs {
				if run.RequestID == r.ID && run.Status != "running" {
					r.Status, r.Error = run.Status, run.Error
					if run.FinishedAt != nil {
						r.FinishedAt = *run.FinishedAt
					}
					break
				}
			}
		}
	}
	for i := range e.state.Automations {
		a := &e.state.Automations[i]
		if a.NextAt.IsZero() || !a.NextAt.After(now) {
			next, err := nextAutomation(*a, now)
			if err != nil {
				return fmt.Errorf("automation %s: %w", a.Name, err)
			}
			a.NextAt = next
		}
	}
	return nil
}

func (e *Engine) automationTick(now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return
	}
	before := slices.Clone(e.state.AutomationRuns)
	definitions := slices.Clone(e.state.Automations)
	changed := false
	for i := range e.state.AutomationRuns {
		r := &e.state.AutomationRuns[i]
		if r.Status != "running" {
			continue
		}
		for _, run := range e.state.Runs {
			if run.RequestID != r.ID || run.Status == "running" {
				continue
			}
			r.Status, r.Error, r.FinishedAt = run.Status, run.Error, now
			if run.FinishedAt != nil {
				r.FinishedAt = *run.FinishedAt
			}
			if j := e.automationIndex(r.AutomationID); j >= 0 && e.state.Automations[j].Mode == "interval" {
				e.state.Automations[j].NextAt, _ = nextAutomation(e.state.Automations[j], r.FinishedAt)
			}
			changed = true
			break
		}
	}
	if changed {
		if err := e.persistLocked(); err != nil {
			e.state.AutomationRuns, e.state.Automations = before, definitions
			log.Printf("automation settlement: %v", err)
			return
		}
		e.publishLocked(contracts.Event{Kind: "state.changed"})
	}
}

func (e *Engine) fireDueAutomations(now time.Time) {
	e.mu.Lock()
	due := []contracts.Automation{}
	if !e.closed {
		for _, a := range e.state.Automations {
			if a.Enabled && !a.NextAt.IsZero() && !a.NextAt.After(now) && !e.automationBusy(a.ID) {
				due = append(due, a)
			}
		}
	}
	e.mu.Unlock()
	slices.SortFunc(due, func(a, b contracts.Automation) int { return a.NextAt.Compare(b.NextAt) })
	for _, job := range due {
		id := job.ID
		// The normal command transaction provides rollback on storage failure.
		if err := e.executeCommand(e.background, contracts.Command{Kind: "automation.fire", ID: id}, nil); err != nil {
			if strings.Contains(err.Error(), "concurrency limit") || strings.Contains(err.Error(), "session is busy") {
				continue
			}
			log.Printf("automation fire %s: %v", id, err)
		}
	}
}

func (e *Engine) runAutomationScheduler() {
	defer close(e.automationDone)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-e.background.Done():
			return
		case now := <-ticker.C:
			e.automationTick(now)
			e.fireDueAutomations(now)
		}
	}
}
