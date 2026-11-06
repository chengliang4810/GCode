package mock

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func assertTurnReferences(t *testing.T, snapshot contracts.Snapshot) {
	t.Helper()
	runs := make(map[string]contracts.Run)
	for _, run := range snapshot.Runs {
		if _, duplicate := runs[run.ID]; duplicate {
			t.Fatalf("duplicate run %s", run.ID)
		}
		runs[run.ID] = run
	}
	messageIDs := make(map[string]bool)
	toolIDs := make(map[string]bool)
	for sid, messages := range snapshot.Messages {
		prior := make(map[string]contracts.Message)
		for _, message := range messages {
			if messageIDs[message.ID] {
				t.Fatalf("duplicate message %s", message.ID)
			}
			messageIDs[message.ID] = true
			run, ok := runs[message.TurnID]
			if !ok || run.SessionID != sid || message.SessionID != sid {
				t.Fatalf("message %s has foreign/missing turn %s", message.ID, message.TurnID)
			}
			if parent := message.ContinuationOf; parent != "" {
				previous, ok := prior[parent]
				if !ok || previous.Role != "assistant" || previous.TurnID != message.TurnID {
					t.Fatalf("invalid continuation %s -> %s", message.ID, parent)
				}
			}
			if message.CreatedAt.Before(run.StartedAt) {
				t.Fatalf("message precedes turn: %s", message.ID)
			}
			for _, block := range message.Blocks {
				if block.Tool == nil {
					continue
				}
				tool := block.Tool
				if toolIDs[tool.ID] {
					t.Fatalf("duplicate tool invocation %s", tool.ID)
				}
				toolIDs[tool.ID] = true
				if tool.StartedAt.Before(run.StartedAt) || tool.FinishedAt != nil && tool.FinishedAt.Before(tool.StartedAt) {
					t.Fatalf("invalid tool timing: %s", tool.ID)
				}
				if run.FinishedAt != nil && tool.FinishedAt != nil && tool.FinishedAt.After(*run.FinishedAt) {
					t.Fatalf("tool finishes after run: %s", tool.ID)
				}
				if tool.Status == "completed" && tool.Failed() {
					t.Fatalf("successful tool has error metadata: %s", tool.ID)
				}
				if (tool.Status == "completed" || tool.Status == "failed" || tool.Status == "interrupted") && tool.FinishedAt == nil {
					t.Fatalf("settled tool has no completion time: %s", tool.ID)
				}
			}
			prior[message.ID] = message
		}
	}
}

func TestDefaultAgentTasksHaveDistinctTracesAndTruthfulSummaries(t *testing.T) {
	s := newTestServer(t, Options{})
	snapshot := snapshotWithAllSessions(t, s)
	assertTurnReferences(t, snapshot)
	finals := make(map[string]bool)
	traces := make(map[string]bool)
	for _, session := range snapshot.Sessions {
		if session.ParentSessionID != "" {
			continue
		}
		messages := snapshot.Messages[session.ID]
		final := messages[len(messages)-1].Blocks[0].Text
		if finals[final] {
			t.Fatalf("session %s repeats another final response", session.ID)
		}
		finals[final] = true
		var signature []string
		for _, block := range contracts.TurnActivity(messages, session.ID+"-turn") {
			entry := block.Kind
			if block.Tool != nil {
				entry += ":" + string(block.Tool.Category())
				if block.Tool.Failed() {
					entry += ":failed"
				}
			}
			signature = append(signature, entry)
		}
		traces[strings.Join(signature, ",")] = true
	}
	if len(finals) != 10 || len(traces) < 8 {
		t.Fatalf("insufficient task/trajectory variety: %d/%d", len(finals), len(traces))
	}
	for _, run := range snapshot.Runs {
		if run.DurationMS <= 0 || run.FinishedAt == nil || run.DurationMS != run.FinishedAt.Sub(run.StartedAt).Milliseconds() {
			t.Fatalf("run duration is not recorded timing: %s", run.ID)
		}
		if run.ID == "chat-2-turn" && (run.Summary.Commands != 2 || run.Summary.Failed != 1 || run.Summary.EditedFiles != 1) {
			t.Fatalf("auth recovery counts incorrect: %+v", run.Summary)
		}
		if run.ID == "chat-1-turn" && run.Summary.EditedFiles != 1 {
			t.Fatal("two palette edits counted the same file twice")
		}
		if run.ID == "chat-3-turn" && (run.Summary.Fetches != 1 || run.Summary.Reads != 0 || run.Summary.EditedFiles != 1) {
			t.Fatal("deployment trace categories disagree with summary")
		}
	}
}

func TestAgentScenesAndForkKeepTurnOwnership(t *testing.T) {
	for _, scenario := range []string{"default", "transcript", "showcase", "loading", "questions", "permissions", "error", "stress"} {
		t.Run(scenario, func(t *testing.T) {
			s := newTestServer(t, Options{Scenario: scenario})
			snapshot := snapshotWithAllSessions(t, s)
			assertTurnReferences(t, snapshot)
		})
	}
	s := newTestServer(t, Options{})
	ctx := context.Background()
	if err := s.Execute(ctx, contracts.Command{Kind: "session.fork", SessionID: "chat-0"}); err != nil {
		t.Fatal(err)
	}
	snapshot := snapshotWithAllSessions(t, s)
	assertTurnReferences(t, snapshot)
}

func TestNewSendStreamsTypedActivityAndFinalReply(t *testing.T) {
	s := newTestServer(t, Options{Instant: true})
	ctx := context.Background()
	project, err := s.CreateProject(ctx, "Fresh API", "/projects/fresh-api")
	if err != nil {
		t.Fatal(err)
	}
	events, _ := s.Subscribe(ctx)
	<-events
	run, err := s.Send(ctx, contracts.SendRequest{ProjectID: project.ID, Text: "Add deployment status with separate ready and failed states"})
	if err != nil {
		t.Fatal(err)
	}
	_ = await(t, events, func(v contracts.Snapshot) bool {
		for _, r := range v.Runs {
			if r.ID == run.ID {
				return r.Status == "completed"
			}
		}
		return false
	})
	snapshot := snapshotWithAllSessions(t, s)
	assertTurnReferences(t, snapshot)
	messages := snapshot.Messages[run.SessionID]
	reply := messages[len(messages)-1]
	if reply.Blocks[0].Kind != "reasoning" || reply.Blocks[len(reply.Blocks)-1].Kind != "text" || reply.Blocks[len(reply.Blocks)-1].Text == "" {
		t.Fatal("activity and final reply order was lost")
	}
	found := false
	for _, file := range snapshot.Files {
		if file.ProjectID == project.ID && file.Path == "internal/deploy/status.go" {
			found = strings.Contains(file.Content, "type Status struct")
		}
	}
	if !found {
		t.Fatal("write event did not change virtual file state")
	}
}

func TestPendingAnswerContinuesSameTurn(t *testing.T) {
	s := newTestServer(t, Options{Scenario: "questions", Instant: true})
	ctx := context.Background()
	before := snapshotForSessions(t, s, "chat-0")
	pending := before.Messages["chat-0"][len(before.Messages["chat-0"])-1]
	events, _ := s.Subscribe(ctx)
	<-events
	if err := s.Execute(ctx, contracts.Command{Kind: "request.answer", ID: "question-layout", QuestionAnswers: []contracts.QuestionAnswer{{Options: []string{"Keep the current layout"}}}}); err != nil {
		t.Fatal(err)
	}
	_ = await(t, events, func(v contracts.Snapshot) bool { return v.Sessions[0].Status == contracts.Idle })
	after := snapshotForSessions(t, s, "chat-0")
	if len(after.Runs) != len(before.Runs) {
		t.Fatal("answer split one turn into a second run")
	}
	last := after.Messages["chat-0"][len(after.Messages["chat-0"])-1]
	if last.TurnID != pending.TurnID || last.ContinuationOf != pending.ID {
		t.Fatal("answer continuation lost its original turn")
	}
	assertTurnReferences(t, after)
}

func TestLiveTraceStopsWithoutLateActivity(t *testing.T) {
	s := newTestServer(t, Options{Scenario: "live", TokenInterval: time.Hour})
	ctx := context.Background()
	if err := s.Stop(ctx, "chat-0"); err != nil {
		t.Fatal(err)
	}
	before := snapshotForSessions(t, s, "chat-0")
	if s.applyActivity("chat-0", "live-turn", contracts.Block{ID: "late", Kind: "reasoning", Text: "late"}, true, 9000) {
		t.Fatal("stopped live run accepted activity")
	}
	after := snapshotForSessions(t, s, "chat-0")
	if !reflect.DeepEqual(before, after) {
		t.Fatal("stale step mutated stopped state")
	}
	for _, run := range after.Runs {
		if run.ID == "live-turn" && run.Status != "interrupted" {
			t.Fatalf("live run status %s", run.Status)
		}
	}
}

func TestActivityMetadataIsOwnedAndPersists(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	s := newTestServer(t, Options{StatePath: statePath})
	copy1 := snapshotWithAllSessions(t, s)
	changedDiff, changedExit := false, false
	for _, messages := range copy1.Messages {
		for _, message := range messages {
			for _, block := range message.Blocks {
				if block.Tool == nil {
					continue
				}
				if block.Tool.ExitCode != nil {
					*block.Tool.ExitCode = 99
					changedExit = true
				}
				if block.Tool.Diff != nil && len(block.Tool.Diff.Hunks) > 0 && len(block.Tool.Diff.Hunks[0].Lines) > 0 {
					block.Tool.Diff.Hunks[0].Lines[0].Text = "mutated snapshot"
					changedDiff = true
				}
			}
		}
	}
	if !changedDiff || !changedExit {
		t.Fatal("ownership fixture omitted diff or exit code")
	}
	original := snapshotWithAllSessions(t, s)
	for _, messages := range original.Messages {
		for _, message := range messages {
			for _, block := range message.Blocks {
				if block.Tool != nil && block.Tool.ExitCode != nil && *block.Tool.ExitCode == 99 {
					t.Fatal("exit code aliases a returned snapshot")
				}
				if block.Tool != nil && block.Tool.Diff != nil {
					for _, hunk := range block.Tool.Diff.Hunks {
						for _, line := range hunk.Lines {
							if line.Text == "mutated snapshot" {
								t.Fatal("diff hunk aliases a returned snapshot")
							}
						}
					}
				}
			}
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	restored := newTestServer(t, Options{StatePath: statePath})
	after := snapshotWithAllSessions(t, restored)
	assertTurnReferences(t, after)
	if !reflect.DeepEqual(original.Messages, after.Messages) || !reflect.DeepEqual(original.Runs, after.Runs) {
		t.Fatal("recorded activities changed during persistence round trip")
	}
}

func TestActiveToolCancellationAndForkHaveSettledTiming(t *testing.T) {
	for _, operation := range []string{"stop", "archive", "fork"} {
		t.Run(operation, func(t *testing.T) {
			s := newTestServer(t, Options{TokenInterval: time.Hour})
			ctx := context.Background()
			run, err := s.Send(ctx, contracts.SendRequest{SessionID: "chat-0", Text: "Check current panel state"})
			if err != nil {
				t.Fatal(err)
			}
			started := run.StartedAt.Add(10 * time.Millisecond)
			block := contracts.Block{ID: "pending-tool", Kind: "tool", Tool: &contracts.ToolCall{ID: "pending-invocation", Kind: contracts.ToolExec, Name: "Bash", Command: "go test ./internal/layout", Status: "working", StartedAt: started}}
			if !s.applyActivity("chat-0", run.ID, block, false, 10) {
				t.Fatal("active step rejected")
			}
			switch operation {
			case "stop":
				err = s.Stop(ctx, "chat-0")
			case "archive":
				value := true
				err = s.UpdateSession(ctx, contracts.SessionUpdate{ID: "chat-0", Archived: &value})
			case "fork":
				err = s.Execute(ctx, contracts.Command{Kind: "session.fork", SessionID: "chat-0"})
			}
			if err != nil {
				t.Fatal(err)
			}
			snapshot := snapshotWithAllSessions(t, s)
			assertTurnReferences(t, snapshot)
		})
	}
}
