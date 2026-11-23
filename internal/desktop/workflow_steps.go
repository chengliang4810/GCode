package desktop

import (
	"context"
	"fmt"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
	"gcode/internal/workflow"
	"github.com/egoist/mygo/ui"
)

// workflowStepsState is the script-steps panel of one run: one read of the
// run's own journal, kept until the user asks for it again. It is never
// re-read per frame.
type workflowStepsState struct {
	loading   bool
	loaded    bool
	records   []contracts.WorkflowJournalRecord
	damaged   int
	errorText string
	// collapsed remembers which groups the user folded away, per run.
	collapsed map[string]bool
	// job identifies the read in flight, so a stale answer cannot land on a
	// newer request.
	job uint64
}

// workflowSteps returns one run's steps panel state, creating it on first use.
func (a *App) workflowSteps(runID string) *workflowStepsState {
	if a.workflowUI.steps == nil {
		a.workflowUI.steps = map[string]*workflowStepsState{}
	}
	state := a.workflowUI.steps[runID]
	if state == nil {
		state = &workflowStepsState{collapsed: map[string]bool{}}
		a.workflowUI.steps[runID] = state
	}
	return state
}

// loadWorkflowSteps reads a run's journal once. A second call while a read is
// in flight joins it; force starts a new one.
func (a *App) loadWorkflowSteps(runID string, force bool) {
	state := a.workflowSteps(runID)
	if state.loading || state.loaded && !force {
		return
	}
	reader, ok := a.service.(contracts.WorkflowJournalService)
	if !ok {
		state.errorText = a.t("This host does not provide run steps.")
		state.loaded = true
		return
	}
	state.loading, state.errorText = true, ""
	state.job++
	job := state.job
	a.backgroundTask(func() func() {
		ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
		defer cancel()
		journal, err := reader.WorkflowJournal(ctx, runID)
		return func() {
			if job != state.job {
				// 这次读取已经被更新的一次取代了。
				return
			}
			state.loading, state.loaded = false, true
			if err != nil {
				state.errorText = err.Error()
				return
			}
			state.records, state.damaged = journal.Records, journal.Damaged
		}
	})
}

// workflowStepsSection draws the run's script steps: one group per phase, in
// the order the run entered them, then the run's own facts. Groups fold
// independently and the whole section is a single read.
func (a *App) workflowStepsSection(c *ui.Context, runID string) {
	state := a.workflowSteps(runID)
	if !state.loaded && !state.loading {
		a.loadWorkflowSteps(runID, false)
	}
	ui.Column(c.Key("workflow-steps:" + runID)).FillWidth().Gap(6).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, tr(c, "Steps")).FontWeight(600).SingleLine()
			ui.Spacer(c).Grow(1)
			iconButton(c.Key("workflow-steps-refresh:"+runID), "refresh", tr(c, "Read the steps again")).
				Disabled(state.loading).OnClick(func() { a.loadWorkflowSteps(runID, true) })
		})
		switch {
		case state.loading:
			ui.Text(c, tr(c, "Reading the steps…")).TextColor(a.palette.Muted)
		case state.errorText != "":
			ui.Text(c, state.errorText).FillWidth().Wrap().TextColor(a.palette.Readable(a.palette.Danger))
		default:
			a.workflowStepGroups(c, runID, state)
		}
		if state.damaged > 0 {
			ui.Text(c, fmt.Sprintf(tr(c, "%d lines could not be read."), state.damaged)).
				FillWidth().Wrap().FontSize(design.TextSize(c, design.TextSmall)).
				TextColor(a.palette.Readable(a.palette.Warning))
		}
	})
}

// workflowStepGroups draws the groups of one run's steps.
func (a *App) workflowStepGroups(c *ui.Context, runID string, state *workflowStepsState) {
	groups := workflow.BuildSteps(journalRecords(state.records))
	if len(groups) == 0 {
		ui.Text(c, tr(c, "This run has no steps yet")).TextColor(a.palette.Muted)
		return
	}
	for _, group := range groups {
		group := group
		key := group.PhaseID
		if key == "" {
			key = "run"
		}
		collapsed := a.workflowStepGroupCollapsed(state, key)
		ui.Column(c.Key("workflow-steps-group:" + runID + ":" + key)).FillWidth().Gap(2).Children(func() {
			controlRowButton(c.Key("workflow-steps-group-head:" + runID + ":" + key)).
				FillWidth().Label(a.workflowStepGroupName(c, group)).
				OnClick(func() { state.collapsed[key] = !collapsed }).Children(func() {
				glyph := "alt-arrow-down"
				if collapsed {
					glyph = "alt-arrow-right"
				}
				icon(c, glyph, design.Controls().IconSize)
				ui.Text(c, a.workflowStepGroupName(c, group)).Grow(1).MinWidth(0).SingleLine()
				ui.Text(c, fmt.Sprintf("%d", len(group.Steps))).FontSize(design.TextSize(c, design.TextSmall)).
					TextColor(a.palette.Muted).FontFeatures("tnum")
			})
			if collapsed {
				return
			}
			ui.Column(c).FillWidth().Padding(20, 0, 0, 0).Gap(2).Children(func() {
				for _, step := range group.Steps {
					a.workflowStepRow(c, runID, step)
				}
			})
		})
	}
}

// workflowStepRow draws one step: its sequence, what happened, and the one
// fact worth reading.
func (a *App) workflowStepRow(c *ui.Context, runID string, step workflow.Step) {
	ui.Row(c.Key(fmt.Sprintf("workflow-step:%s:%d", runID, step.Sequence))).FillWidth().
		Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, fmt.Sprintf("%d", step.Sequence)).FontSize(design.TextSize(c, design.TextSmall)).
			TextColor(a.palette.Muted).FontFeatures("tnum")
		ui.Text(c, a.workflowStepKindWord(c, step.Kind)).SingleLine().
			FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
		if step.Detail != "" {
			ui.Text(c, step.Detail).Grow(1).MinWidth(0).SingleLine()
		} else {
			ui.Spacer(c).Grow(1)
		}
		ui.Text(c, a.workflowStepTime(step.At)).FontSize(design.TextSize(c, design.TextSmall)).
			TextColor(a.palette.Muted).FontFeatures("tnum").SingleLine()
	})
}

// workflowStepGroupName names one group: its phase, or the run itself.
func (a *App) workflowStepGroupName(c *ui.Context, group workflow.StepGroup) string {
	if group.PhaseID == "" {
		return tr(c, "The run")
	}
	return group.PhaseID
}

// workflowStepGroupCollapsed remembers one group's fold state.
func (a *App) workflowStepGroupCollapsed(state *workflowStepsState, key string) bool {
	if state.collapsed == nil {
		state.collapsed = map[string]bool{}
	}
	return state.collapsed[key]
}

// workflowStepKindWord names one journal kind in the interface language.
func (a *App) workflowStepKindWord(c *ui.Context, kind string) string {
	switch kind {
	case workflow.JournalRunStarted:
		return tr(c, "Run started")
	case workflow.JournalPhaseEntered:
		return tr(c, "Phase entered")
	case workflow.JournalPhaseSettled:
		return tr(c, "Phase settled")
	case workflow.JournalPhaseReset:
		return tr(c, "Phase reset")
	case workflow.JournalActorStarted:
		return tr(c, "Instance started")
	case workflow.JournalActorSettled:
		return tr(c, "Instance settled")
	case workflow.JournalQuestionAsked:
		return tr(c, "Question asked")
	case workflow.JournalQuestionAnswer:
		return tr(c, "Question answered")
	case workflow.JournalHoleWaiting:
		return tr(c, "Hole waiting")
	case workflow.JournalHoleFilled:
		return tr(c, "Hole filled")
	case workflow.JournalArtifact:
		return tr(c, "Delivered")
	case workflow.JournalRunRetuned:
		return tr(c, "Settings changed")
	case workflow.JournalRunStopped:
		return tr(c, "Run stopped")
	case workflow.JournalRunResumed:
		return tr(c, "Run resumed")
	case workflow.JournalRunSettled:
		return tr(c, "Run settled")
	}
	return kind
}

// workflowStepTime is when a step happened, to the second, in the interface
// language.
func (a *App) workflowStepTime(at time.Time) string {
	if at.IsZero() {
		return "—"
	}
	return i18n.Date(a.state.Settings.General.Language, at, "3:04 PM")
}

// journalRecords converts the service's records into the rules package's own
// type: the panel groups them there, not in the interface.
func journalRecords(records []contracts.WorkflowJournalRecord) []workflow.JournalRecord {
	out := make([]workflow.JournalRecord, 0, len(records))
	for _, record := range records {
		out = append(out, workflow.JournalRecord{
			Sequence: record.Sequence, Kind: record.Kind, At: record.At, RunID: record.RunID, Data: record.Data,
		})
	}
	return out
}
