package mock

import (
	"errors"
	"strings"
	"time"

	"gcode/internal/contracts"
)

func (s *Server) projectActionLocked(c contracts.Command) error {
	pi := -1
	for i, p := range s.state.Projects {
		if p.ID == c.ProjectID {
			pi = i
			break
		}
	}
	if pi < 0 {
		return ErrNotFound
	}
	project := &s.state.Projects[pi]
	if c.Kind == "project.action.create" || c.Kind == "project.action.update" {
		if c.Action == nil || strings.TrimSpace(c.Action.Name) == "" || strings.TrimSpace(c.Action.Command) == "" {
			return errors.New("action name and command are required")
		}
		action := *c.Action
		action.Name = strings.TrimSpace(action.Name)
		if c.Kind == "project.action.create" {
			action.Enabled = true
			action.ID = s.idLocked("action")
			action.LastRunStatus = ""
			project.Actions = append(project.Actions, action)
			return nil
		}
		for i, a := range project.Actions {
			if a.ID == action.ID {
				project.Actions[i] = action
				return nil
			}
		}
		return ErrNotFound
	}
	for i, a := range project.Actions {
		if a.ID != c.ID {
			continue
		}
		if c.Kind == "project.action.delete" {
			project.Actions = append(project.Actions[:i], project.Actions[i+1:]...)
			return nil
		}
		if !a.Enabled {
			return errors.New("this project action is disabled")
		}
		output := ""
		status := "completed"
		switch strings.TrimSpace(a.Command) {
		case "go test ./...":
			output = "\x1b[32mok\x1b[0m  fieldnotes/internal/layout  0.012s\r\n\x1b[32mok\x1b[0m  fieldnotes/internal/session  0.008s\r\n"
		case "npm test", "pnpm test", "make test":
			output = "\x1b[32mAll checks passed.\x1b[0m\r\n"
		case "git status":
			output = "On branch " + project.Branch + "\r\nChanges not staged for commit\r\n"
		default:
			if strings.HasPrefix(a.Command, "echo ") {
				output = strings.TrimPrefix(a.Command, "echo ") + "\r\n"
			} else {
				output = "Command not found\r\n"
				status = "failed"
			}
		}
		project.Actions[i].LastRunStatus = status
		s.state.Terminals = append(s.state.Terminals, contracts.Terminal{ID: s.idLocked("terminal"), ProjectID: project.ID, Title: a.Name, CWD: project.Path, Columns: 80, Rows: 24, Status: status, Output: "❯ " + a.Command + "\r\n" + output + "\r\n❯ "})
		return nil
	}
	return ErrNotFound
}

func (s *Server) branchSessionLocked(c contracts.Command) error {
	id := c.SessionID
	if id == "" {
		id = c.ID
	}
	i := s.sessionIndexLocked(id)
	if i < 0 {
		return ErrNotFound
	}
	if c.RequestID != "" {
		for _, session := range s.state.Sessions {
			if session.CreationRequestID == c.RequestID {
				if session.ParentSessionID != id || session.SideChat != (c.Kind == "session.sidechat") {
					return errors.New("conversation request ID is already in use")
				}
				return nil
			}
		}
	}
	v := s.state.Sessions[i]
	parent := v.ID
	v.ID = s.idLocked("session")
	v.ParentSessionID = parent
	v.CreationRequestID = c.RequestID
	v.SideChat = c.Kind == "session.sidechat"
	v.Status = contracts.Idle
	v.Archived = false
	v.Pinned = false
	v.PinOrder = 0
	v.Unread = false
	v.LastOutcome = ""
	v.RunningSubagents = 0
	v.CreatedAt = s.opts.Now().UTC()
	v.UpdatedAt = v.CreatedAt
	if c.Name != "" {
		v.Title = c.Name
	} else if v.SideChat {
		v.Title = "Side chat"
	} else {
		v.Title = "Fork of " + v.Title
	}
	ms := []contracts.Message{}
	if !v.SideChat {
		ms = cloneSnapshot(s.state).Messages[parent]
		messageIDs := make(map[string]string, len(ms))
		turnIDs := make(map[string]string)
		for _, message := range ms {
			messageIDs[message.ID] = s.idLocked("message")
			if _, ok := turnIDs[message.TurnID]; !ok && message.TurnID != "" {
				turnIDs[message.TurnID] = s.idLocked("run")
			}
		}
		for j := range ms {
			ms[j].ID = messageIDs[ms[j].ID]
			ms[j].TurnID = turnIDs[ms[j].TurnID]
			ms[j].ContinuationOf = messageIDs[ms[j].ContinuationOf]
			ms[j].SessionID = v.ID
			ms[j].Streaming = false
			for bi := range ms[j].Blocks {
				b := &ms[j].Blocks[bi]
				b.ID = s.idLocked("block")
				if b.Tool != nil {
					b.Tool.ID = s.idLocked("tool")
				}
				if b.Question != nil {
					b.Question.ID = s.idLocked("question")
					if b.Question.Answer == "" {
						b.Question.Answer = "Cancelled"
					}
				}
				if b.Permission != nil {
					b.Permission.ID = s.idLocked("permission")
					if b.Permission.Answer == "" {
						b.Permission.Answer = "Cancelled"
					}
				}
				if b.Tool != nil && b.Tool.Status == "working" {
					b.Tool.Status = "interrupted"
					end := s.opts.Now().UTC()
					if end.Before(b.Tool.StartedAt) {
						end = b.Tool.StartedAt
					}
					b.Tool.FinishedAt = &end
				}
				if b.Subagent != nil && b.Subagent.Status == "working" {
					b.Subagent.Status = "interrupted"
				}
			}
		}
		for _, run := range append([]contracts.Run(nil), s.state.Runs...) {
			newID, ok := turnIDs[run.ID]
			if !ok || run.SessionID != parent {
				continue
			}
			run.ID = newID
			run.SessionID = v.ID
			if run.Status == "running" || run.Status == "awaitingInput" {
				run.Status = "interrupted"
				run.FinishReason = "forked"
				end := s.opts.Now().UTC()
				if logical := run.StartedAt.Add(time.Duration(run.DurationMS) * time.Millisecond); end.Before(logical) {
					end = logical
				}
				run.FinishedAt = &end
				run.DurationMS = end.Sub(run.StartedAt).Milliseconds()
			}
			run.Summary = contracts.SummarizeActivity(contracts.TurnActivity(ms, newID))
			s.state.Runs = append(s.state.Runs, run)
		}
	}
	s.state.Sessions = append([]contracts.Session{v}, s.state.Sessions...)
	s.state.Messages[v.ID] = ms
	return nil
}
