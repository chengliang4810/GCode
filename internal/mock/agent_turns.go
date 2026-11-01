package mock

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"gcode/internal/contracts"
)

func extendTranscriptScene(snapshot *contracts.Snapshot, opts Options) {
	ms := snapshot.Messages["chat-0"]
	last := &ms[len(ms)-1]
	fail := toolBlock(contracts.ToolExec, "Bash", "Check the schema fixture", "go test ./internal/schema -run TestFixture", "", "--- FAIL: TestFixture\nopen testdata/schema.json: file does not exist\nFAIL\nexit status 1")
	code := 1
	fail.Tool.ExitCode = &code
	fail.Tool.IsError = true
	fail.Tool.Status = "failed"
	write := toolBlock(contracts.ToolWriteFile, "Write", "Add the missing schema fixture", "", "testdata/schema.json", "Created testdata/schema.json")
	content := "{\n  \"schemaVersion\": 1,\n  \"entries\": []\n}\n"
	added, removed, hunks := contiguousDiff("", content)
	write.Tool.Diff = &contracts.Change{ID: "showcase-fixture-change", ProjectID: "fieldnotes", Path: "testdata/schema.json", Status: "added", NewContent: content, Additions: added, Deletions: removed, Hunks: hunks}
	write.Tool.Language = "json"
	check := toolBlock(contracts.ToolExec, "Bash", "Recheck the fixture", "go test ./internal/schema -run TestFixture", "", "ok  fieldnotes/internal/schema  0.011s")
	zero := 0
	check.Tool.ExitCode = &zero
	activity := []contracts.Block{{Kind: "reasoning", Text: "The schema check needs its fixture. I’ll add the missing file and rerun the same command."}, fail, write, check, {Kind: "todo", Todos: []contracts.Todo{{ID: "showcase-todo", Text: "Add and verify the schema fixture", Status: "completed"}}}, {Kind: "subagent", Subagent: &contracts.Subagent{SessionID: "showcase-agent", Name: "Schema reviewer", Task: "Check the fixture fields independently", Status: "completed", Summary: "schemaVersion and entries match the contract."}}}
	stampBlocks(activity, "showcase-trace", opts.Now().UTC())
	last.Blocks = append(activity, last.Blocks...)
	last.Blocks = append(last.Blocks, contracts.Block{ID: "showcase-final", Kind: "text", Text: "Added `testdata/schema.json` and reran the fixture check successfully. The schema reviewer confirmed the fields. The selected panel is Files and the recorded test permission was granted."})
	snapshot.Messages["chat-0"] = ms
	ensureAgentFile(snapshot, "fieldnotes", "testdata/schema.json", content)
	mergeAgentChange(snapshot, *write.Tool.Diff)
	started := opts.Now().UTC().Add(-time.Minute)
	snapshot.Sessions = append(snapshot.Sessions, contracts.Session{ID: "showcase-agent", ParentSessionID: "chat-0", SideChat: true, Title: "Review the schema fixture", ProjectID: "fieldnotes", DeviceID: "local", ModelID: "claude-sonnet-4-6", ReasoningLevel: "High", Status: contracts.Idle, CreatedAt: started, UpdatedAt: started.Add(time.Second), LastOutcome: "completed"})
	snapshot.Messages["showcase-agent"] = []contracts.Message{{ID: "showcase-agent-user", SessionID: "showcase-agent", TurnID: "showcase-agent-turn", Role: "user", CreatedAt: started, Blocks: []contracts.Block{{ID: "showcase-agent-prompt", Kind: "text", Text: "Check schemaVersion and entries in the fixture."}}}, {ID: "showcase-agent-reply", SessionID: "showcase-agent", TurnID: "showcase-agent-turn", Role: "assistant", CreatedAt: started.Add(time.Second), DurationMS: 1000, Blocks: []contracts.Block{{ID: "showcase-agent-result", Kind: "text", Text: "The fixture uses schemaVersion 1 and an empty entries array. The required fields are present."}}}}
}

func reconcileAgentTurns(snapshot *contracts.Snapshot, opts Options) {
	if len(snapshot.Sessions) == 0 {
		snapshot.Runs = nil
		return
	}
	indices := make(map[string]int, len(snapshot.Runs))
	for i, run := range snapshot.Runs {
		indices[run.ID] = i
	}
	ids := make([]string, 0, len(snapshot.Messages))
	for id := range snapshot.Messages {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, sid := range ids {
		messages := snapshot.Messages[sid]
		groups := make(map[string][]contracts.Message)
		order := []string{}
		previous := contracts.Message{}
		for mi := range messages {
			message := &messages[mi]
			if message.TurnID == "" {
				if message.Role == "assistant" && previous.TurnID != "" {
					message.TurnID = previous.TurnID
				} else {
					message.TurnID = fmt.Sprintf("%s-turn-%d", sid, mi)
				}
			}
			if message.Role == "assistant" && previous.Role == "assistant" && previous.TurnID == message.TurnID && message.ContinuationOf == "" {
				message.ContinuationOf = previous.ID
			}
			if _, ok := groups[message.TurnID]; !ok {
				order = append(order, message.TurnID)
			}
			groups[message.TurnID] = append(groups[message.TurnID], *message)
			previous = *message
		}
		snapshot.Messages[sid] = messages
		for _, turnID := range order {
			group := groups[turnID]
			index, exists := indices[turnID]
			run := contracts.Run{ID: turnID, SessionID: sid, Status: "completed", FinishReason: "stop", StartedAt: group[0].CreatedAt}
			if exists {
				run = snapshot.Runs[index]
			}
			end := run.StartedAt
			pending, waiting, failed := false, false, false
			for _, message := range group {
				if message.CreatedAt.After(end) {
					end = message.CreatedAt
				}
				if message.DurationMS > run.DurationMS {
					run.DurationMS = message.DurationMS
				}
				pending = pending || message.Streaming
				for _, block := range message.Blocks {
					if block.Tool != nil && block.Tool.FinishedAt != nil && block.Tool.FinishedAt.After(end) {
						end = *block.Tool.FinishedAt
					}
					waiting = waiting || (block.Question != nil && block.Question.Answer == "") || (block.Permission != nil && block.Permission.Answer == "")
					if block.Kind == "error" {
						failed = true
						run.Error = block.Text
					}
				}
			}
			if span := end.Sub(run.StartedAt).Milliseconds(); span > run.DurationMS {
				run.DurationMS = span
			}
			if logical := run.StartedAt.Add(time.Duration(run.DurationMS) * time.Millisecond); logical.After(end) {
				end = logical
			}
			run.FinishedAt = &end
			if pending {
				run.Status = "running"
				run.FinishReason = ""
				run.FinishedAt = nil
			}
			if waiting {
				run.Status = "awaitingInput"
				run.FinishReason = ""
				run.FinishedAt = nil
			}
			if failed {
				run.Status = "failed"
				run.FinishReason = "error"
			}
			run.Summary = contracts.SummarizeActivity(contracts.TurnActivity(group, turnID))
			if exists {
				snapshot.Runs[index] = run
			} else {
				indices[turnID] = len(snapshot.Runs)
				snapshot.Runs = append(snapshot.Runs, run)
			}
		}
	}
	valid := snapshot.Runs[:0]
	for _, run := range snapshot.Runs {
		for _, message := range snapshot.Messages[run.SessionID] {
			if message.TurnID == run.ID {
				valid = append(valid, run)
				break
			}
		}
	}
	snapshot.Runs = valid
	_ = opts
}

func (s *Server) updateRunSummaryLocked(sessionID, runID string) {
	for i := range s.state.Runs {
		run := &s.state.Runs[i]
		if run.SessionID != sessionID || runID != "" && run.ID != runID {
			continue
		}
		run.Summary = contracts.SummarizeActivity(contracts.TurnActivity(s.state.Messages[sessionID], run.ID))
	}
}

func (s *Server) settleRunToolsLocked(sessionID string) {
	for mi := range s.state.Messages[sessionID] {
		message := &s.state.Messages[sessionID][mi]
		message.Streaming = false
		for bi := range message.Blocks {
			block := &message.Blocks[bi]
			if block.Tool != nil && (block.Tool.Status == "working" || block.Tool.Status == "running") {
				block.Tool.Status = "interrupted"
				end := s.opts.Now().UTC()
				if end.Before(block.Tool.StartedAt) {
					end = block.Tool.StartedAt
				}
				block.Tool.FinishedAt = &end
			}
			if block.Subagent != nil && strings.EqualFold(block.Subagent.Status, "working") {
				block.Subagent.Status = "interrupted"
			}
		}
	}
}
