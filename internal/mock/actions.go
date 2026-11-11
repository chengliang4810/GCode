package mock

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/i18n"
)

func (s *Server) sessionIndexLocked(id string) int {
	for i, v := range s.state.Sessions {
		if v.ID == id {
			return i
		}
	}
	return -1
}

func (s *Server) CreateSession(ctx context.Context, projectID string) (contracts.Session, error) {
	if err := ctx.Err(); err != nil {
		return contracts.Session{}, err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return contracts.Session{}, ErrClosed
	}
	var project contracts.Project
	if projectID != "" {
		found := false
		for _, p := range s.state.Projects {
			if p.ID == projectID {
				project = p
				found = true
				break
			}
		}
		if !found {
			s.mu.Unlock()
			return contracts.Session{}, ErrNotFound
		}
	}
	now := s.opts.Now().UTC()
	v := contracts.Session{ID: s.idLocked("session"), Title: "New session", ProjectID: projectID, DeviceID: project.DeviceID, CWD: project.Path, ModelID: "claude-opus-4-6", Branch: project.Branch, Status: contracts.Idle, CreatedAt: now, UpdatedAt: now, ReasoningLevel: "High"}
	if v.DeviceID == "" {
		v.DeviceID = "local"
	}
	s.state.Sessions = append([]contracts.Session{v}, s.state.Sessions...)
	s.state.Messages[v.ID] = []contracts.Message{}
	s.publishLocked("session.created", v.ID, "", "", "")
	s.mu.Unlock()
	return v, s.persist()
}

func (s *Server) CreateProject(ctx context.Context, name, path string) (contracts.Project, error) {
	return s.CreateProjectOnDevice(ctx, contracts.ProjectCreate{DeviceID: "local", Name: name, Path: path})
}

func (s *Server) CreateProjectOnDevice(ctx context.Context, request contracts.ProjectCreate) (contracts.Project, error) {
	if err := ctx.Err(); err != nil {
		return contracts.Project{}, err
	}
	name := strings.TrimSpace(request.Name)
	path := strings.TrimSpace(request.Path)
	if name == "" || path == "" {
		return contracts.Project{}, errors.New("project name and path are required")
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return contracts.Project{}, ErrClosed
	}
	deviceFound := false
	for _, device := range s.state.Devices {
		if device.ID != request.DeviceID {
			continue
		}
		deviceFound = true
		if !device.Current && device.Status != "online" && device.Status != "connected" {
			s.mu.Unlock()
			return contracts.Project{}, errors.New("device is offline")
		}
	}
	if !deviceFound {
		s.mu.Unlock()
		return contracts.Project{}, errors.New("device not found")
	}
	for _, p := range s.state.Projects {
		if p.DeviceID == request.DeviceID && p.Path == path {
			s.mu.Unlock()
			return contracts.Project{}, errors.New("project is already added")
		}
	}
	v := contracts.Project{ID: s.idLocked("project"), Name: name, Path: path, DeviceID: request.DeviceID, Branch: "main", Branches: []string{"main"}}
	s.state.Projects = append(s.state.Projects, v)
	s.publishLocked("project.created", "", "", "", "")
	s.mu.Unlock()
	return clone(v), s.persist()
}

func (s *Server) UpdateSession(ctx context.Context, u contracts.SessionUpdate) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	i := s.sessionIndexLocked(u.ID)
	if i < 0 {
		s.mu.Unlock()
		return ErrNotFound
	}
	if u.Title != nil && strings.TrimSpace(*u.Title) == "" {
		s.mu.Unlock()
		return errors.New("title is empty")
	}
	if u.ModelID != nil {
		found := false
		for _, m := range s.state.Models {
			if m.ID == *u.ModelID {
				found = true
				break
			}
		}
		if !found {
			s.mu.Unlock()
			return errors.New("unknown model")
		}
	}
	if u.DeviceID != nil {
		found := false
		for _, d := range s.state.Devices {
			if d.ID == *u.DeviceID {
				found = true
				break
			}
		}
		if !found {
			s.mu.Unlock()
			return errors.New("unknown device")
		}
	}
	v := &s.state.Sessions[i]
	if u.Title != nil {
		v.Title = strings.TrimSpace(*u.Title)
	}
	if u.Pinned != nil {
		v.Pinned = *u.Pinned
		if *u.Pinned && u.PinOrder == nil {
			maxOrder := -1
			for _, p := range s.state.Sessions {
				if p.Pinned && p.ID != v.ID && p.PinOrder > maxOrder {
					maxOrder = p.PinOrder
				}
			}
			v.PinOrder = maxOrder + 1
		}
	}
	if u.Archived != nil {
		v.Archived = *u.Archived
		if *u.Archived {
			if active, ok := s.runs[v.ID]; ok {
				active.cancel()
				delete(s.runs, v.ID)
				now := s.finishRunTimingLocked(v.ID, active.id)
				for ri := range s.state.Runs {
					if s.state.Runs[ri].ID == active.id {
						s.state.Runs[ri].Status = "interrupted"
						s.state.Runs[ri].FinishReason = "archived"
						s.state.Runs[ri].FinishedAt = &now
					}
				}
				ms := s.state.Messages[v.ID]
				for mi := range ms {
					ms[mi].Streaming = false
				}
				s.state.Messages[v.ID] = ms
				v.Status = contracts.Idle
				v.LastOutcome = "interrupted"
			}
		}
	}
	if u.Unread != nil {
		v.Unread = *u.Unread
	}
	if u.Archived != nil && *u.Archived {
		s.settleRunToolsLocked(v.ID)
		for ri := range s.state.Runs {
			if s.state.Runs[ri].SessionID == v.ID && (s.state.Runs[ri].Status == "running" || s.state.Runs[ri].Status == "awaitingInput") {
				s.state.Runs[ri].Status = "interrupted"
				s.state.Runs[ri].FinishReason = "archived"
				end := s.finishRunTimingLocked(v.ID, s.state.Runs[ri].ID)
				s.state.Runs[ri].FinishedAt = &end
			}
		}
		ms := s.state.Messages[v.ID]
		for mi := range ms {
			ms[mi].Streaming = false
			for bi := range ms[mi].Blocks {
				b := &ms[mi].Blocks[bi]
				if b.Question != nil && !b.Question.Resolved {
					b.Question.Resolved = true
				}
				if b.Permission != nil && b.Permission.Answer == "" {
					b.Permission.Answer = "Cancelled"
				}
			}
		}
		s.state.Messages[v.ID] = ms
		if v.Status == contracts.Working || v.Status == contracts.AwaitingInput {
			v.Status = contracts.Idle
			v.LastOutcome = "interrupted"
		}
	}
	if u.PinOrder != nil {
		order := []contracts.Session{}
		for _, p := range s.state.Sessions {
			if p.Pinned && p.ID != v.ID {
				order = append(order, p)
			}
		}
		slices.SortStableFunc(order, func(a, b contracts.Session) int { return cmp.Compare(a.PinOrder, b.PinOrder) })
		position := max(0, min(len(order), *u.PinOrder))
		order = append(order, contracts.Session{})
		copy(order[position+1:], order[position:])
		order[position] = *v
		for rank, p := range order {
			for j := range s.state.Sessions {
				if s.state.Sessions[j].ID == p.ID {
					s.state.Sessions[j].PinOrder = rank
					break
				}
			}
		}
	}
	if u.ModelID != nil {
		v.ModelID = *u.ModelID
	}
	if u.ReasoningLevel != nil {
		v.ReasoningLevel = *u.ReasoningLevel
	}
	if u.Branch != nil {
		v.Branch = *u.Branch
	}
	if u.WorktreeID != nil {
		v.WorktreeID = *u.WorktreeID
	}
	if u.DeviceID != nil {
		v.DeviceID = *u.DeviceID
	}
	if u.GroupID != nil {
		if *u.GroupID != "" && s.sessionGroupIndexLocked(*u.GroupID) < 0 {
			s.mu.Unlock()
			return errors.New("unknown session group")
		}
		v.GroupID = *u.GroupID
	}
	if u.GroupOrder != nil {
		v.GroupOrder = *u.GroupOrder
	}
	v.UpdatedAt = s.opts.Now().UTC()
	s.publishLocked("session.updated", u.ID, "", "", "")
	s.mu.Unlock()
	return s.persist()
}

func (s *Server) sessionGroupIndexLocked(id string) int {
	for i := range s.state.SessionGroups {
		if s.state.SessionGroups[i].ID == id {
			return i
		}
	}
	return -1
}

// GroupCreate adds an empty conversation group ordered after existing groups.
func (s *Server) GroupCreate(ctx context.Context, title string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("title is empty")
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	order := 0
	for _, group := range s.state.SessionGroups {
		order = max(order, group.Order+1)
	}
	s.state.SessionGroups = append(s.state.SessionGroups, contracts.SessionGroup{ID: s.idLocked("group"), Title: title, Order: order})
	s.publishLocked("session.grouped", "", "", "", "")
	s.mu.Unlock()
	return s.persist()
}

// GroupRename renames a conversation group in place.
func (s *Server) GroupRename(ctx context.Context, id, title string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("title is empty")
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	i := s.sessionGroupIndexLocked(id)
	if i < 0 {
		s.mu.Unlock()
		return ErrNotFound
	}
	s.state.SessionGroups[i].Title = title
	s.publishLocked("session.grouped", "", "", "", "")
	s.mu.Unlock()
	return s.persist()
}

// GroupDelete removes a group; its conversations return to the ungrouped list.
func (s *Server) GroupDelete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	i := s.sessionGroupIndexLocked(id)
	if i < 0 {
		s.mu.Unlock()
		return ErrNotFound
	}
	removed := s.state.SessionGroups[i].ID
	s.state.SessionGroups = slices.Delete(s.state.SessionGroups, i, i+1)
	for j := range s.state.Sessions {
		if s.state.Sessions[j].GroupID == removed {
			s.state.Sessions[j].GroupID = ""
			s.state.Sessions[j].GroupOrder = 0
		}
	}
	s.publishLocked("session.grouped", "", "", "", "")
	s.mu.Unlock()
	return s.persist()
}

func (s *Server) Send(ctx context.Context, req contracts.SendRequest) (contracts.Run, error) {
	if err := ctx.Err(); err != nil {
		return contracts.Run{}, err
	}
	req = clone(req)
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" && len(req.Attachments) == 0 && len(req.Annotations) == 0 {
		return contracts.Run{}, errors.New("message is empty")
	}
	for i := range req.Annotations {
		req.Annotations[i] = req.Annotations[i].Bounded()
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return contracts.Run{}, ErrClosed
	}
	if existing := s.sessionIndexLocked(req.SessionID); req.SessionID != "" && existing >= 0 {
		// Draft selections cannot migrate an established conversation, even
		// when a stale caller still includes its former checkout payload.
		current := s.state.Sessions[existing]
		req.Checkout = nil
		req.DeviceID, req.Branch, req.WorktreeID = current.DeviceID, current.Branch, current.WorktreeID
	} else if req.SessionID == "" && req.Checkout != nil {
		if project, ok := s.gitProjectLocked(req.ProjectID); ok {
			req.DeviceID = project.DeviceID
			if req.Checkout.Kind == contracts.CheckoutCloud {
				req.DeviceID = ""
			}
		}
	}
	if req.ModelID != "" {
		found := false
		for _, m := range s.state.Models {
			if m.ID == req.ModelID {
				found = true
				break
			}
		}
		if !found {
			s.mu.Unlock()
			return contracts.Run{}, errors.New("unknown model")
		}
	}
	if req.DeviceID != "" {
		found := false
		for _, d := range s.state.Devices {
			if d.ID == req.DeviceID {
				found = true
				break
			}
		}
		if !found {
			s.mu.Unlock()
			return contracts.Run{}, errors.New("unknown device")
		}
	}
	if req.SessionID == "" {
		var project contracts.Project
		if req.ProjectID != "" {
			found := false
			for _, p := range s.state.Projects {
				if p.ID == req.ProjectID {
					project = p
					found = true
					break
				}
			}
			if !found {
				s.mu.Unlock()
				return contracts.Run{}, ErrNotFound
			}
		}
		now := s.opts.Now().UTC()
		resolved, err := s.resolveNewCheckoutLocked(project, s.idLocked("session"), req)
		if err != nil {
			s.mu.Unlock()
			return contracts.Run{}, err
		}
		v := resolved
		v.Title, v.ModelID, v.Status = "New session", "claude-opus-4-6", contracts.Idle
		v.CreatedAt, v.UpdatedAt, v.ReasoningLevel = now, now, "High"
		s.state.Sessions = append([]contracts.Session{v}, s.state.Sessions...)
		s.state.Messages[v.ID] = []contracts.Message{}
		req.SessionID = v.ID
	}
	i := s.sessionIndexLocked(req.SessionID)
	if i < 0 {
		s.mu.Unlock()
		return contracts.Run{}, ErrNotFound
	}
	if s.state.Sessions[i].Archived {
		s.mu.Unlock()
		return contracts.Run{}, errors.New("restore the archived session before sending")
	}
	if s.state.Sessions[i].Status == contracts.Working || s.state.Sessions[i].Status == contracts.AwaitingInput {
		current := s.state.Sessions[i]
		if req.ModelID == "" {
			req.ModelID = current.ModelID
		}
		if req.ReasoningLevel == "" {
			req.ReasoningLevel = current.ReasoningLevel
		}
		if req.DeviceID == "" {
			req.DeviceID = current.DeviceID
		}
		if req.Branch == "" {
			req.Branch = current.Branch
		}
		if req.WorktreeID == "" {
			req.WorktreeID = current.WorktreeID
		}
		q := contracts.QueuedMessage{ID: s.idLocked("queue"), RequestID: req.RequestID, SessionID: req.SessionID, Text: req.Text, Attachments: req.Attachments, CreatedAt: s.opts.Now().UTC(), ModelID: req.ModelID, ReasoningLevel: req.ReasoningLevel, DeviceID: req.DeviceID, Branch: req.Branch, WorktreeID: req.WorktreeID, Annotations: req.Annotations}
		s.state.Queue = append(s.state.Queue, q)
		s.publishLocked("queue.changed", req.SessionID, "", req.RequestID, "")
		s.mu.Unlock()
		return contracts.Run{ID: q.ID, SessionID: req.SessionID, Status: "queued"}, s.persist()
	}
	run := s.startLocked(i, req, true)
	s.mu.Unlock()
	return run, s.persist()
}

func (s *Server) startLocked(i int, req contracts.SendRequest, addUser bool) contracts.Run {
	return s.startContinuationLocked(i, req, addUser, "", "")
}

func (s *Server) startContinuationLocked(i int, req contracts.SendRequest, addUser bool, turnID, continuation string) contracts.Run {
	if turnID != "" {
		for _, message := range s.state.Messages[s.state.Sessions[i].ID] {
			if message.Role == "user" && message.TurnID == turnID {
				for _, block := range message.Blocks {
					if block.Kind == "text" {
						req.Text = block.Text + "\n\nUser answer: " + req.Text
						break
					}
				}
				break
			}
		}
	}
	now := s.opts.Now().UTC()
	session := &s.state.Sessions[i]
	if req.ModelID != "" {
		session.ModelID = req.ModelID
	}
	if req.ReasoningLevel != "" {
		session.ReasoningLevel = req.ReasoningLevel
	}
	s.captureSessionSourceLocked(session)
	run := contracts.Run{ID: s.idLocked("run"), RequestID: req.RequestID, SessionID: session.ID, Status: "running", StartedAt: now}
	runIndex := -1
	if turnID != "" {
		for ri, previous := range s.state.Runs {
			if previous.ID == turnID && previous.SessionID == session.ID {
				run = previous
				run.Status = "running"
				run.FinishReason = ""
				run.FinishedAt = nil
				run.Error = ""
				runIndex = ri
				break
			}
		}
	}
	if session.Title == "New session" && req.Text != "" {
		title := []rune(req.Text)
		if len(title) > 56 {
			title = append(title[:55], '…')
		}
		session.Title = string(title)
	}
	session.Status = contracts.Working
	session.UpdatedAt = now
	if addUser {
		blocks := []contracts.Block{}
		if req.Text != "" {
			blocks = append(blocks, contracts.Block{ID: s.idLocked("block"), Kind: "text", Text: req.Text})
		}
		for _, a := range req.Attachments {
			a := a
			blocks = append(blocks, contracts.Block{ID: s.idLocked("block"), Kind: "attachment", Attachment: &a})
		}
		for _, annotation := range req.Annotations {
			a := annotation.Bounded()
			blocks = append(blocks, contracts.Block{ID: s.idLocked("block"), Kind: "annotation", Annotation: &a})
		}
		s.state.Messages[session.ID] = append(s.state.Messages[session.ID], contracts.Message{ID: s.idLocked("message"), RequestID: req.RequestID, SessionID: session.ID, TurnID: run.ID, Role: "user", CreatedAt: now, Blocks: blocks})
	}
	msg := contracts.Message{ID: s.idLocked("message"), SessionID: session.ID, TurnID: run.ID, Role: "assistant", CreatedAt: now, Streaming: true, Blocks: []contracts.Block{{ID: s.idLocked("block"), Kind: "text"}}, Usage: contracts.Usage{InputTokens: 1240, ContextTokens: 18420, ContextLimit: 200000}}
	msg.ContinuationOf = continuation
	if logical := run.StartedAt.Add(time.Duration(run.DurationMS) * time.Millisecond); logical.After(msg.CreatedAt) {
		msg.CreatedAt = logical
	}
	msg.Usage.InputTokens = max(1, len([]rune(req.Text))/4) + 128
	for _, model := range s.state.Models {
		if model.ID == session.ModelID {
			msg.Usage.ContextLimit = model.ContextLimit
			break
		}
	}
	for mi := len(s.state.Messages[session.ID]) - 1; mi >= 0; mi-- {
		if usage := s.state.Messages[session.ID][mi].Usage; usage.ContextLimit > 0 {
			msg.Usage.ContextTokens = min(msg.Usage.ContextLimit, usage.ContextTokens+msg.Usage.InputTokens)
			break
		}
	}
	s.state.Messages[session.ID] = append(s.state.Messages[session.ID], msg)
	if runIndex < 0 {
		s.state.Runs = append(s.state.Runs, run)
	} else {
		s.state.Runs[runIndex] = run
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.runs[session.ID] = activeRun{id: run.ID, cancel: cancel}
	s.publishLocked("run.started", session.ID, run.ID, req.RequestID, "")
	if turnID == "" && strings.Contains(strings.ToLower(req.Text), "permission") {
		ms := s.state.Messages[session.ID]
		last := &ms[len(ms)-1]
		last.Streaming = false
		last.Blocks = []contracts.Block{{ID: s.idLocked("block"), Kind: "permission", Permission: &contracts.Permission{ID: s.idLocked("permission"), Title: "Run project checks", Description: "Allow the agent to run the project's test suite?", Command: "go test ./..."}}}
		s.state.Messages[session.ID] = ms
		session.Status = contracts.AwaitingInput
		s.state.Runs[len(s.state.Runs)-1].Status = "awaitingInput"
		s.publishLocked("request.created", session.ID, run.ID, req.RequestID, "")
		return run
	}
	if turnID == "" && strings.Contains(strings.ToLower(req.Text), "question") {
		ms := s.state.Messages[session.ID]
		last := &ms[len(ms)-1]
		last.Streaming = false
		last.Blocks = []contracts.Block{{ID: s.idLocked("block"), Kind: "question", Question: fixtureQuestion(s.idLocked("question"), "Which implementation should we use?", "Keep the current structure", "Use the new layout", "Compare both approaches")}}
		s.state.Messages[session.ID] = ms
		session.Status = contracts.AwaitingInput
		s.state.Runs[len(s.state.Runs)-1].Status = "awaitingInput"
		s.publishLocked("request.created", session.ID, run.ID, req.RequestID, "")
		return run
	}
	s.launchActivityLocked(ctx, session.ID, run.ID, req.Text)
	return run
}

func (s *Server) advanceRun(sessionID, runID, text string, done bool) bool {
	s.mu.Lock()
	active, ok := s.runs[sessionID]
	if s.closed || !ok || active.id != runID {
		s.mu.Unlock()
		return false
	}
	ms := s.state.Messages[sessionID]
	found := false
	for i := len(ms) - 1; i >= 0; i-- {
		if ms[i].TurnID == runID && ms[i].Role == "assistant" {
			for b := len(ms[i].Blocks) - 1; b >= 0; b-- {
				if ms[i].Blocks[b].Kind == "text" {
					ms[i].Blocks[b].Text = text
					found = true
					break
				}
			}
			ms[i].Streaming = !done
			ms[i].Usage.OutputTokens = len([]rune(text)) / 4
			break
		}
	}
	if !found {
		s.mu.Unlock()
		return false
	}
	s.state.Messages[sessionID] = ms
	if done {
		delete(s.runs, sessionID)
		active.cancel()
		for ri := range s.state.Runs {
			if s.state.Runs[ri].ID == runID {
				s.state.Runs[ri].DurationMS += 900
			}
		}
		now := s.finishRunTimingLocked(sessionID, runID)
		for i := range s.state.Runs {
			if s.state.Runs[i].ID == runID {
				s.state.Runs[i].Status = "completed"
				s.state.Runs[i].FinishReason = "stop"
				s.state.Runs[i].FinishedAt = &now
				if s.state.Runs[i].Error != "" {
					s.state.Runs[i].Status = "failed"
					s.state.Runs[i].FinishReason = "toolFailed"
				}
			}
		}
		si := s.sessionIndexLocked(sessionID)
		if si >= 0 {
			s.state.Sessions[si].Status = contracts.Idle
			s.state.Sessions[si].LastOutcome = "completed"
			s.state.Sessions[si].UpdatedAt = now
			for _, run := range s.state.Runs {
				if run.ID == runID && run.Status == "failed" {
					s.state.Sessions[si].Status = contracts.Errored
					s.state.Sessions[si].LastOutcome = "failed"
				}
			}
		}
		s.publishLocked("run.completed", sessionID, runID, "", text)
		s.finishHostContextLocked(sessionID, runID)
		for qi, q := range s.state.Queue {
			if q.SessionID == sessionID && si >= 0 {
				s.state.Queue = append(s.state.Queue[:qi], s.state.Queue[qi+1:]...)
				s.startLocked(si, contracts.SendRequest{RequestID: q.RequestID, SessionID: sessionID, Text: q.Text, Attachments: q.Attachments, ModelID: q.ModelID, ReasoningLevel: q.ReasoningLevel, DeviceID: q.DeviceID, Branch: q.Branch, WorktreeID: q.WorktreeID, Annotations: q.Annotations}, true)
				break
			}
		}
	} else {
		s.publishLocked("message.delta", sessionID, runID, "", text)
	}
	s.mu.Unlock()
	if done {
		_ = s.persist()
	}
	return !done
}

func (s *Server) Stop(ctx context.Context, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	i := s.sessionIndexLocked(sessionID)
	if i < 0 {
		s.mu.Unlock()
		return ErrNotFound
	}
	if r, ok := s.runs[sessionID]; ok {
		r.cancel()
		delete(s.runs, sessionID)
		now := s.finishRunTimingLocked(sessionID, r.id)
		for ri := range s.state.Runs {
			if s.state.Runs[ri].ID == r.id {
				s.state.Runs[ri].Status = "interrupted"
				s.state.Runs[ri].FinishReason = "userStopped"
				s.state.Runs[ri].FinishedAt = &now
			}
		}
	}
	s.settleRunToolsLocked(sessionID)
	for ri := range s.state.Runs {
		if s.state.Runs[ri].SessionID == sessionID && (s.state.Runs[ri].Status == "running" || s.state.Runs[ri].Status == "awaitingInput") {
			s.state.Runs[ri].Status = "interrupted"
			s.state.Runs[ri].FinishReason = "userStopped"
			end := s.finishRunTimingLocked(sessionID, s.state.Runs[ri].ID)
			s.state.Runs[ri].FinishedAt = &end
		}
	}
	ms := s.state.Messages[sessionID]
	for mi := range ms {
		ms[mi].Streaming = false
		for bi := range ms[mi].Blocks {
			b := &ms[mi].Blocks[bi]
			if b.Question != nil && !b.Question.Resolved {
				b.Question.Resolved = true
			}
			if b.Permission != nil && b.Permission.Answer == "" {
				b.Permission.Answer = "Cancelled"
			}
		}
	}
	s.state.Messages[sessionID] = ms
	s.state.Sessions[i].Status = contracts.Idle
	s.state.Sessions[i].LastOutcome = "interrupted"
	s.publishLocked("run.stopped", sessionID, "", "", "")
	s.mu.Unlock()
	return s.persist()
}

func (s *Server) Retry(ctx context.Context, sessionID string) (contracts.Run, error) {
	if err := ctx.Err(); err != nil {
		return contracts.Run{}, err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return contracts.Run{}, ErrClosed
	}
	i := s.sessionIndexLocked(sessionID)
	if i < 0 {
		s.mu.Unlock()
		return contracts.Run{}, ErrNotFound
	}
	if s.state.Sessions[i].Archived {
		s.mu.Unlock()
		return contracts.Run{}, errors.New("restore the archived session before retrying")
	}
	if s.state.Sessions[i].Status == contracts.Working || s.state.Sessions[i].Status == contracts.AwaitingInput {
		s.mu.Unlock()
		return contracts.Run{}, errors.New("a run is already active")
	}
	req := contracts.SendRequest{SessionID: sessionID}
	ms := s.state.Messages[sessionID]
	for m := len(ms) - 1; m >= 0; m-- {
		if ms[m].Role == "user" {
			for _, b := range ms[m].Blocks {
				if b.Kind == "text" {
					if req.Text != "" {
						req.Text += "\n"
					}
					req.Text += b.Text
				}
				if b.Attachment != nil {
					req.Attachments = append(req.Attachments, *b.Attachment)
				}
				if b.Annotation != nil {
					req.Annotations = append(req.Annotations, b.Annotation.Bounded())
				}
			}
			break
		}
	}
	if req.Text == "" && len(req.Attachments) == 0 && len(req.Annotations) == 0 {
		s.mu.Unlock()
		return contracts.Run{}, errors.New("no message to retry")
	}
	run := s.startLocked(i, req, false)
	s.mu.Unlock()
	return run, s.persist()
}

func (s *Server) Answer(ctx context.Context, id, answer string) error {
	return s.answer(ctx, id, answer, nil)
}

func (s *Server) answer(ctx context.Context, id, answer string, answers []contracts.QuestionAnswer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(answer) == "" && len(answers) == 0 {
		return errors.New("answer is empty")
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	for sid, ms := range s.state.Messages {
		for mi := range ms {
			for bi := range ms[mi].Blocks {
				b := &ms[mi].Blocks[bi]
				if (b.Question != nil && b.Question.ID == id) || (b.Permission != nil && b.Permission.ID == id) {
					if owner := s.sessionIndexLocked(sid); owner >= 0 && s.state.Sessions[owner].Archived {
						s.mu.Unlock()
						return errors.New("restore the archived session before answering")
					}
				}
				match := false
				denied := false
				if b.Question != nil && b.Question.ID == id {
					if b.Question.Resolved {
						s.mu.Unlock()
						return errors.New("question has already been answered")
					}
					if err := contracts.ValidateQuestionAnswers(b.Question.Pages, answers); err != nil {
						s.mu.Unlock()
						return err
					}
					b.Question.Answers = contracts.CloneQuestionAnswers(answers)
					b.Question.Resolved = true
					match = true
				}
				if b.Permission != nil && b.Permission.ID == id {
					if b.Permission.Answer != "" {
						s.mu.Unlock()
						return errors.New("permission has already been answered")
					}
					if answer != "Allow once" && answer != "Always allow" && answer != "Deny" {
						s.mu.Unlock()
						return errors.New("invalid permission answer")
					}
					denied = answer == "Deny"
					b.Permission.Answer = answer
					match = true
				}
				if match {
					s.state.Messages[sid] = ms
					si := s.sessionIndexLocked(sid)
					if si >= 0 {
						s.state.Sessions[si].Status = contracts.Idle
					}
					if active, ok := s.runs[sid]; ok {
						active.cancel()
						delete(s.runs, sid)
						now := s.opts.Now().UTC()
						for ri := range s.state.Runs {
							if s.state.Runs[ri].ID == active.id {
								s.state.Runs[ri].Status = "completed"
								s.state.Runs[ri].FinishedAt = &now
								s.state.Runs[ri].FinishReason = "answered"
							}
						}
					}
					s.publishLocked("request.answered", sid, "", id, answer)
					if denied {
						for ri := range s.state.Runs {
							if s.state.Runs[ri].ID == ms[mi].TurnID {
								s.state.Runs[ri].Status = "interrupted"
								s.state.Runs[ri].FinishReason = "permissionDenied"
								end := s.finishRunTimingLocked(sid, s.state.Runs[ri].ID)
								s.state.Runs[ri].FinishedAt = &end
							}
						}
						if si >= 0 {
							s.state.Sessions[si].LastOutcome = "interrupted"
						}
						s.state.Messages[sid] = append(s.state.Messages[sid], contracts.Message{ID: s.idLocked("message"), SessionID: sid, TurnID: ms[mi].TurnID, ContinuationOf: ms[mi].ID, Role: "assistant", CreatedAt: s.opts.Now().UTC(), Blocks: []contracts.Block{{ID: s.idLocked("block"), Kind: "text", Text: "I won’t run that command. You can continue with another approach."}}})
						s.publishLocked("permission.denied", sid, "", id, answer)
					} else if si >= 0 {
						s.startContinuationLocked(si, contracts.SendRequest{SessionID: sid, Text: "Continue with " + answer}, false, ms[mi].TurnID, ms[mi].ID)
					}
					s.mu.Unlock()
					return s.persist()
				}
			}
		}
	}
	s.mu.Unlock()
	return ErrNotFound
}

func (s *Server) SaveFile(ctx context.Context, id, content string, version uint64) (contracts.File, error) {
	if err := ctx.Err(); err != nil {
		return contracts.File{}, err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return contracts.File{}, ErrClosed
	}
	for i, v := range s.state.Files {
		if v.ID == id {
			if v.ReadOnly || v.Kind == "directory" {
				s.mu.Unlock()
				return contracts.File{}, ErrReadOnly
			}
			if version != 0 && v.Version != version {
				s.mu.Unlock()
				return contracts.File{}, ErrConflict
			}
			s.state.Files[i].Content = content
			s.state.Files[i].Version++
			s.state.Files[i].Modified = true
			s.syncFileChangeLocked(&s.state.Files[i], v.Content)
			file := s.state.Files[i]
			s.publishLocked("file.saved", "", "", "", id)
			s.mu.Unlock()
			return file, s.persist()
		}
	}
	s.mu.Unlock()
	return contracts.File{}, ErrNotFound
}

func (s *Server) UpdateSettings(ctx context.Context, settings contracts.Settings) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	settings = clone(settings)
	normalizeSettings(&settings)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	// Image identity and crop are changed only by background.* commands. A
	// settings draft opened before Replace image must not restore retired data.
	settings.Appearance.Background = s.state.Settings.Appearance.Background
	settings.Appearance.BackgroundName = s.state.Settings.Appearance.BackgroundName
	settings.Appearance.BackgroundAssetRef = s.state.Settings.Appearance.BackgroundAssetRef
	settings.Appearance.BackgroundAdjustment = s.state.Settings.Appearance.BackgroundAdjustment
	s.state.Settings = settings
	s.rethemeArtifactsLocked()
	s.publishLocked("settings.updated", "", "", "", "")
	s.mu.Unlock()
	return s.persist()
}

func normalizeSettings(v *contracts.Settings) {
	v.General.Language = i18n.Normalize(v.General.Language)
	v.SidebarWidth = clamp(v.SidebarWidth, 242, 400)
	v.FilesWidth = clamp(v.FilesWidth, 220, 440)
	v.RightPaneWidth = clamp(v.RightPaneWidth, 360, 2000)
	v.TerminalHeight = clamp(v.TerminalHeight, 160, 2000)
	a := &v.Appearance
	a.EffortTheme = contracts.NormalizeEffortTheme(a.EffortTheme)
	a.BackgroundAdjustment = normalizeAdjustment(a.BackgroundAdjustment)
	a.FontSize = clamp(a.FontSize, 12, 20)
	a.CodeFontSize = clamp(a.CodeFontSize, 8, 32)
	a.TerminalFontSize = clamp(a.TerminalFontSize, 8, 32)
	a.TranscriptWidth = clamp(a.TranscriptWidth, 560, 1200)
	v.Files.AutosaveDelay = max(100, min(10000, v.Files.AutosaveDelay))
}
func clamp(v, lo, hi float64) float64 {
	if v != v {
		return lo
	}
	return max(lo, min(hi, v))
}

func (s *Server) TerminalInput(ctx context.Context, id, input string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return "", ErrClosed
	}
	for i, v := range s.state.Terminals {
		if v.ID == id {
			cmd := strings.TrimSpace(input)
			output := ""
			switch cmd {
			case "clear":
				s.state.Terminals[i].Output = "\x1b[2J\x1b[H"
			case "pwd":
				output = v.CWD + "\r\n"
			case "ls", "dir":
				output = "\x1b[34mcmd  internal  resources\x1b[0m  go.mod  README.md\r\n"
			case "git status":
				output = "On branch main\r\nChanges not staged for commit:\r\n  \x1b[31mmodified: internal/layout.go\x1b[0m\r\n"
			case "go test ./...":
				output = "\x1b[32mok\x1b[0m  rcode/internal/layout  0.012s\r\n\x1b[32mok\x1b[0m  rcode/internal/session  0.008s\r\n"
			case "help":
				output = "Available commands: pwd, ls, git status, go test ./..., echo, clear\r\n"
			case "":
			default:
				if strings.HasPrefix(cmd, "echo ") {
					output = strings.TrimPrefix(cmd, "echo ") + "\r\n"
				} else {
					output = fmt.Sprintf("%s: command not found\r\n", strings.Fields(cmd)[0])
				}
			}
			if cmd != "clear" {
				s.state.Terminals[i].Output += input + "\r\n" + output
			}
			s.state.Terminals[i].Output += "\x1b[38;2;139;132;255m❯\x1b[0m "
			result := s.state.Terminals[i].Output
			s.publishLocked("terminal.output", "", "", id, output)
			s.mu.Unlock()
			return result, s.persist()
		}
	}
	s.mu.Unlock()
	return "", ErrNotFound
}

func (s *Server) removeQueueLocked(sessionID string) {
	out := s.state.Queue[:0]
	for _, q := range s.state.Queue {
		if q.SessionID != sessionID {
			out = append(out, q)
		}
	}
	s.state.Queue = out
}
