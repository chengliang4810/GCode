package mock

import (
	"errors"
	"time"

	"gcode/internal/contracts"
)

func (s *Server) branchSessionLocked(c contracts.Command) error {
	id := c.SessionID
	if id == "" {
		id = c.ID
	}
	i := s.sessionIndexLocked(id)
	if i < 0 {
		return ErrNotFound
	}
	side := c.Kind == "session.sidechat"
	if c.RequestID != "" {
		for _, session := range s.state.Sessions {
			if session.CreationRequestID == c.RequestID {
				if session.SideChat != side || side && session.ParentSessionID != id || !side && session.ParentSessionID != "" {
					return errors.New("conversation request ID is already in use")
				}
				return nil
			}
		}
	}
	v := s.state.Sessions[i]
	parent := v.ID
	v.ID = s.idLocked("session")
	// A side chat stays a child of its parent; a branch is the user's own
	// conversation and takes a place in the conversation list.
	v.ParentSessionID = ""
	if side {
		v.ParentSessionID = parent
	}
	v.CreationRequestID = c.RequestID
	v.SideChat = side
	v.Status = contracts.Idle
	v.Archived = false
	v.Pinned = false
	v.PinOrder = 0
	v.Unread = false
	v.LastOutcome = ""
	v.RunningSubagents = 0
	v.SubagentType = ""
	v.Goal = nil
	v.CreatedAt = s.opts.Now().UTC()
	v.UpdatedAt = v.CreatedAt
	if c.Name != "" {
		v.Title = c.Name
	} else if v.SideChat {
		v.Title = "Side chat"
	} else {
		v.Title = "Branch of " + v.Title
	}
	if c.Checkout != nil {
		project, _ := s.gitProjectLocked(v.ProjectID)
		resolved, err := s.resolveNewCheckoutLocked(project, v.ID, contracts.SendRequest{DeviceID: v.DeviceID, Checkout: c.Checkout})
		if err != nil {
			return err
		}
		v.ProjectID, v.DeviceID = resolved.ProjectID, resolved.DeviceID
		v.CWD, v.Branch, v.WorktreeID = resolved.CWD, resolved.Branch, resolved.WorktreeID
		v.SourceContext = resolved.SourceContext
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
					if !b.Question.Resolved {
						b.Question.Resolved = true
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
