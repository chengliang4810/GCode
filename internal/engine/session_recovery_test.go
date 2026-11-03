package engine

import (
	"reflect"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestReconcileLoadedSessionInterruptsOnlyItsHistoricalWork(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	target := newTestSession(t, core, "", "late-loaded")
	other := newTestSession(t, core, "", "still-owned")
	index := core.sessionIndex(target.ID)
	began := time.Unix(100, 0)
	lastSeen := began.Add(3 * time.Second)
	core.state.Sessions[index].Status = contracts.AwaitingInput
	core.state.Sessions[index].RunningSubagents = 2
	core.state.Sessions[index].Goal = &contracts.SessionGoal{
		ID: "goal", Status: contracts.GoalVerifying, ActiveSince: &began,
		LastSeenAt: lastSeen, TimeUsedMS: 100,
	}
	otherIndex := core.sessionIndex(other.ID)
	core.state.Sessions[otherIndex].Status = contracts.Working
	core.state.Sessions[otherIndex].RunningSubagents = 1
	core.state.Runs = []contracts.Run{
		{ID: "target-running", SessionID: target.ID, Status: "running"},
		{ID: "target-done", SessionID: target.ID, Status: "completed"},
		{ID: "other-running", SessionID: other.ID, Status: "running"},
	}
	core.state.Messages[target.ID] = []contracts.Message{{
		ID: "target", SessionID: target.ID, Streaming: true,
		Blocks: []contracts.Block{
			{Kind: "tool", Tool: &contracts.ToolCall{ID: "pending", Status: "running"}},
			{Kind: "tool", Tool: &contracts.ToolCall{ID: "finished", Status: "completed", Output: "keep"}},
			{Kind: "subagent", Subagent: &contracts.Subagent{Status: "running"}},
			{Kind: "question", Question: &contracts.Question{ID: "pending-question", Pages: []contracts.QuestionPage{{Question: "Mode"}}}},
		},
	}}
	core.state.Messages[other.ID] = []contracts.Message{{
		ID: "other", SessionID: other.ID, Streaming: true,
		Blocks: []contracts.Block{{Kind: "tool", Tool: &contracts.ToolCall{Status: "running"}}},
	}}
	history := []model.Message{
		{Role: "assistant", Calls: []model.Call{{ID: "completed", Name: "Read"}, {ID: "pending", Name: "Bash"}}},
		{Role: "tool", CallID: "completed", Text: "saved result"},
	}
	core.history[target.ID] = model.CloneMessages(history)
	core.context[target.ID] = model.CloneMessages(history)
	core.history[other.ID] = model.CloneMessages(history)
	core.context[other.ID] = model.CloneMessages(history)
	core.reconcileSessionLocked(target.ID)
	session := core.state.Sessions[index]
	if session.Status != contracts.Idle || session.LastOutcome != "interrupted" || session.RunningSubagents != 0 {
		t.Fatalf("late-loaded session was not interrupted: %+v", session)
	}
	if session.Goal.Status != contracts.GoalPaused || session.Goal.ActiveSince != nil || session.Goal.TimeUsedMS != 3100 {
		t.Fatalf("goal time or state was not reconciled: %+v", session.Goal)
	}
	if core.state.Runs[0].Status != "interrupted" || core.state.Runs[0].FinishReason != "engineRestarted" ||
		core.state.Runs[1].Status != "completed" || core.state.Runs[2].Status != "running" {
		t.Fatal("reconciliation changed the wrong run")
	}
	message := core.state.Messages[target.ID][0]
	if message.Streaming || message.Blocks[0].Tool.Status != "interrupted" ||
		message.Blocks[1].Tool.Output != "keep" || message.Blocks[2].Subagent.Status != "interrupted" || !message.Blocks[3].Question.Resolved {
		t.Fatal("historical transcript was not reconciled")
	}
	for _, messages := range [][]model.Message{core.history[target.ID], core.context[target.ID]} {
		if len(messages) != 3 || messages[1].Text != "saved result" || messages[2].Role != "tool" ||
			messages[2].CallID != "pending" || !messages[2].IsError {
			t.Fatalf("interrupted tool history was not closed exactly once: %+v", messages)
		}
	}
	if core.state.Sessions[otherIndex].Status != contracts.Working ||
		!core.state.Messages[other.ID][0].Streaming || len(core.history[other.ID]) != 2 {
		t.Fatal("reconciliation changed another session")
	}
	before := contracts.CloneSnapshot(core.state)
	beforeHistory := model.CloneMessages(core.history[target.ID])
	core.reconcileSessionLocked(target.ID)
	if !reflect.DeepEqual(before, core.state) || !reflect.DeepEqual(beforeHistory, core.history[target.ID]) {
		t.Fatal("repeated reconciliation changed already settled historical state")
	}
}
