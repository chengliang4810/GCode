package mock

import (
	"context"
	"path"
	"time"

	"gcode/internal/contracts"
)

func (s *Server) runtimeActivityLocked(sessionID, runID, prompt string) ([]contracts.Block, string) {
	i := s.sessionIndexLocked(sessionID)
	session := s.state.Sessions[i]
	task := taskDefinition(taskIndex(prompt))
	before := ""
	exists := false
	for _, file := range s.state.Files {
		if file.ProjectID == session.ProjectID && file.Path == task.path {
			before = file.Content
			exists = true
			break
		}
	}
	if task.review && !exists {
		return []contracts.Block{{Kind: "reasoning", Text: task.thought}, toolBlock(contracts.ToolGlob, "Glob", "Locate the review target", task.path, "", "No matching files")}, "I checked the selected project. `" + task.path + "` is not present, so I have not changed any files. Select the project containing that source to continue the review."
	}
	if exists && before == task.after && !task.review {
		read := toolBlock(contracts.ToolReadFile, "Read", "Verify the current implementation", "", task.path, before)
		read.Tool.Language = task.language
		check := toolBlock(contracts.ToolExec, "Bash", "Verify the affected checks", "go test ./"+path.Dir(task.path), "", "ok  focused checks  0.012s")
		if task.path == "README.md" {
			check.Tool.Command = "git diff --check"
			check.Tool.Args = check.Tool.Command
			check.Tool.Output = ""
		}
		zero := 0
		check.Tool.ExitCode = &zero
		return []contracts.Block{{Kind: "reasoning", Text: task.thought}, read, check}, "Checked `" + task.path + "`. The requested behavior is already present, and the focused checks passed. No additional file changes were needed."
	}
	task.failure = false
	after := task.after
	if task.index == 0 && exists {
		after = before + "\n// Keep the minimum conversation width when panels change.\n"
	}
	blocks := taskBlocks(task, session.ProjectID, before, after)
	return blocks, task.final
}

// Each step materializes an ordered activity item, then settles that same ID.
// It never runs a process or reads a project from disk.
func (s *Server) launchActivityLocked(ctx context.Context, sessionID, runID, prompt string) {
	blocks, text := s.runtimeActivityLocked(sessionID, runID, prompt)
	started := s.opts.Now().UTC()
	origin := started
	for _, run := range s.state.Runs {
		if run.ID == runID {
			origin = run.StartedAt
			if logical := origin.Add(time.Duration(run.DurationMS) * time.Millisecond); logical.After(started) {
				started = logical
			}
			break
		}
	}
	ms := s.state.Messages[sessionID]
	last := &ms[len(ms)-1]
	stampBlocks(blocks, runID+"-"+last.ID, started)
	if len(blocks) > 0 && blocks[0].Kind == "reasoning" {
		last.Blocks = append([]contracts.Block{blocks[0]}, last.Blocks...)
		blocks = blocks[1:]
	}
	s.state.Messages[sessionID] = ms
	s.publishLocked("reasoning.updated", sessionID, runID, "", "")
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		elapsed := started.Sub(origin).Milliseconds() + 650
		for _, block := range blocks {
			if !s.waitActivity(ctx) {
				return
			}
			if block.Tool != nil {
				working := cloneBlocks([]contracts.Block{block})[0]
				working.Tool.Status = "working"
				working.Tool.Output = ""
				working.Tool.ExitCode = nil
				working.Tool.IsError = false
				working.Tool.Diff = nil
				working.Tool.FinishedAt = nil
				if !s.applyActivity(sessionID, runID, working, false, working.Tool.StartedAt.Sub(origin).Milliseconds()) {
					return
				}
				if !s.waitActivity(ctx) {
					return
				}
			}
			if block.Tool != nil && block.Tool.FinishedAt != nil {
				elapsed = block.Tool.FinishedAt.Sub(origin).Milliseconds()
			} else {
				elapsed += 650
			}
			if !s.applyActivity(sessionID, runID, block, true, elapsed) {
				return
			}
			if issue := s.activityError(sessionID, runID); issue != "" {
				text = "The last edit could not be applied because the file changed or is read-only. Review the recorded output before retrying."
				break
			}
		}
		if !s.waitActivity(ctx) {
			return
		}
		// Stream the trailing reply separately from all intermediate narration.
		runes := []rune(text)
		if s.opts.Instant {
			s.advanceRun(sessionID, runID, text, true)
			return
		}
		for offset := 0; offset < len(runes); {
			if !s.waitActivity(ctx) {
				return
			}
			offset = min(len(runes), offset+12)
			if !s.advanceRun(sessionID, runID, string(runes[:offset]), offset == len(runes)) {
				return
			}
		}
	}()
}

func (s *Server) waitActivity(ctx context.Context) bool {
	if s.opts.Instant {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(s.opts.TokenInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-s.done:
		return false
	case <-timer.C:
		return true
	}
}

func (s *Server) applyActivity(sessionID, runID string, block contracts.Block, settled bool, duration int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	active, ok := s.runs[sessionID]
	if s.closed || !ok || active.id != runID {
		return false
	}
	ms := s.state.Messages[sessionID]
	messageIndex := -1
	for i := len(ms) - 1; i >= 0; i-- {
		if ms[i].Role == "assistant" && ms[i].TurnID == runID {
			messageIndex = i
			break
		}
	}
	if messageIndex < 0 {
		return false
	}
	if settled && block.Tool != nil && block.Tool.Diff != nil {
		change := block.Tool.Diff
		conflict := false
		for i, file := range s.state.Files {
			if file.ProjectID == change.ProjectID && file.Path == change.Path {
				if file.ReadOnly || file.Content != change.OldContent {
					conflict = true
					break
				}
				s.state.Files[i].Version++
				break
			}
		}
		if conflict {
			one := 1
			block.Tool.Status = "failed"
			block.Tool.IsError = true
			block.Tool.ExitCode = &one
			block.Tool.Output = "The file is read-only or changed before this edit completed."
			for ri := range s.state.Runs {
				if s.state.Runs[ri].ID == runID {
					s.state.Runs[ri].Error = block.Tool.Output
				}
			}
		} else {
			ensureAgentFile(&s.state, change.ProjectID, change.Path, change.NewContent)
			mergeAgentChange(&s.state, *change)
		}
	}
	message := &ms[messageIndex]
	found := false
	for i := range message.Blocks {
		if message.Blocks[i].ID == block.ID {
			message.Blocks[i] = block
			found = true
			break
		}
	}
	if !found {
		at := max(0, len(message.Blocks)-1)
		message.Blocks = append(message.Blocks, contracts.Block{})
		copy(message.Blocks[at+1:], message.Blocks[at:])
		message.Blocks[at] = block
	}
	s.state.Messages[sessionID] = ms
	for i := range s.state.Runs {
		if s.state.Runs[i].ID == runID && duration > s.state.Runs[i].DurationMS {
			s.state.Runs[i].DurationMS = duration
		}
	}
	kind := "activity.appended"
	if block.Tool != nil {
		kind = "tool.started"
		if settled {
			kind = "tool.completed"
			if block.Tool.Failed() {
				kind = "tool.failed"
			}
		}
	}
	s.publishLocked(kind, sessionID, runID, "", "")
	return true
}

func (s *Server) activityError(sessionID, runID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, run := range s.state.Runs {
		if run.ID == runID && run.SessionID == sessionID {
			return run.Error
		}
	}
	return ""
}

func (s *Server) finishRunTimingLocked(sessionID, runID string) time.Time {
	finished := s.opts.Now().UTC()
	for i := range s.state.Runs {
		run := &s.state.Runs[i]
		if run.ID != runID {
			continue
		}
		logical := run.StartedAt.Add(time.Duration(run.DurationMS) * time.Millisecond)
		if logical.After(finished) {
			finished = logical
		}
		run.DurationMS = finished.Sub(run.StartedAt).Milliseconds()
		for mi := range s.state.Messages[sessionID] {
			if s.state.Messages[sessionID][mi].TurnID == runID && s.state.Messages[sessionID][mi].Role == "assistant" {
				s.state.Messages[sessionID][mi].DurationMS = run.DurationMS
			}
		}
		return finished
	}
	return finished
}

func (s *Server) liveScenarioLocked() {
	ctx, cancel := context.WithCancel(context.Background())
	s.runs["chat-0"] = activeRun{id: "live-turn", cancel: cancel}
	s.launchActivityLocked(ctx, "chat-0", "live-turn", "Verify the workspace panel constraints")
}
