package engine

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestGoalResumeAfterOrdinaryTask(t *testing.T) {
	for _, action := range []string{
		"resume", "queued input", "awaiting answer", "cancel", "clear", "archive",
		"stop ordinary task", "ordinary failure", "admission failure", "host restart",
	} {
		t.Run(action, func(t *testing.T) {
			var workers, ordinary, checks atomic.Int32
			entered, release := make(chan struct{}), make(chan struct{})
			finish := sync.OnceFunc(func() { close(release) })
			defer finish()
			core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := subagentRequest(t, r)
				if isGoalVerification(body) {
					checks.Add(1)
					goalVerdictResponse(w, model.Chat, `{"passed":true,"reason":"Device verified."}`, true)
					return
				}
				messages := body["messages"].([]any)
				system, _ := messages[0].(map[string]any)["content"].(string)
				if !strings.Contains(system, "Current authoritative session goal") {
					if ordinary.Add(1) == 1 {
						close(entered)
						if action == "awaiting answer" {
							goalResponse(w, model.Chat, "", &model.Call{ID: "ask", Name: "AskUserQuestion", Arguments: `{"questions":[{"question":"Choose","header":"Choice","options":[{"label":"A","description":"First option"},{"label":"B","description":"Second option"}]}]}`}, true)
							return
						}
						select {
						case <-release:
						case <-r.Context().Done():
							return
						}
						if action == "ordinary failure" {
							http.Error(w, "ordinary task failed", http.StatusBadRequest)
							return
						}
					}
					goalResponse(w, model.Chat, "Ordinary task answered.", nil, true)
					return
				}
				switch workers.Add(1) {
				case 1:
					goalResponse(w, model.Chat, "", &model.Call{ID: "device", Name: "TodoWrite", Arguments: `{"todos":[{"text":"Verify device","status":"pending"}],"resolution":{"kind":"blocked","reason":"Device disconnected."}}`}, true)
				case 3:
					last, _ := messages[len(messages)-1].(map[string]any)["content"].(string)
					if !strings.Contains(last, `"nextAction":"Verify device"`) {
						t.Error("resumed goal lost its own checklist")
					}
					goalResponse(w, model.Chat, "", &model.Call{ID: "done", Name: "TodoWrite", Arguments: `{"todos":[]}`}, true)
				default:
					goalResponse(w, model.Chat, "Goal progress.", nil, true)
				}
			}))
			sendGoal(t, core, "/goal Verify the device")
			summary, _ := core.Snapshot(t.Context())
			id := summary.Sessions[0].ID
			before := waitSubagentState(t, core, runsFinished, id).Sessions[0].Goal
			send := func(text string) {
				t.Helper()
				if err := core.Execute(t.Context(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{SessionID: id, Text: text}}); err != nil {
					t.Fatal(err)
				}
			}
			send("Answer an independent question")
			select {
			case <-entered:
			case <-time.After(10 * time.Second):
				t.Fatal("ordinary task did not reach its model")
			}
			var questionID string
			if action == "awaiting answer" {
				waitSubagentState(t, core, func(s contracts.Snapshot) bool {
					if s.Sessions[0].Status == contracts.AwaitingInput {
						for _, msg := range s.Messages[id] {
							for _, block := range msg.Blocks {
								if q := block.Question; q != nil && !q.Resolved {
									questionID = q.ID
									return true
								}
							}
						}
					}
					return false
				}, id)
			}
			if err := core.Execute(t.Context(), contracts.Command{Kind: "goal.resume", SessionID: id, ID: before.ID, RequestID: "resume-after-task"}); err != nil {
				t.Fatal("busy session rejected the resume:", err)
			}
			send("/goal resume")
			waiting, _ := core.Snapshot(t.Context(), id)
			g := waiting.Sessions[0].Goal
			if g.Status != contracts.GoalResumePending || g.ID != before.ID || g.ActiveSince != nil || g.TokensUsed != before.TokensUsed ||
				g.TimeUsedMS != before.TimeUsedMS || len(waiting.Runs) != 2 || workers.Load() != 2 || checks.Load() != 0 {
				t.Fatalf("resume started too early or lost its ownership: goal=%+v runs=%d", g, len(waiting.Runs))
			}
			switch action {
			case "cancel":
				send("/goal pause")
			case "clear":
				send("/goal clear")
			case "queued input":
				send("Another independent question")
			case "awaiting answer":
				if waiting.Sessions[0].Status != contracts.AwaitingInput {
					t.Fatal("scheduling the resume dismissed the ordinary task's question")
				}
				if err := core.Execute(t.Context(), contracts.Command{Kind: "request.answer", ID: questionID, QuestionAnswers: []contracts.QuestionAnswer{{Options: []string{"A"}}}}); err != nil {
					t.Fatal(err)
				}
			case "archive":
				if err := core.Execute(t.Context(), contracts.Command{Kind: "session.archive", ID: id, Enabled: true}); err != nil {
					t.Fatal(err)
				}
			case "stop ordinary task":
				if err := core.Execute(t.Context(), contracts.Command{Kind: "message.stop", SessionID: id}); err != nil {
					t.Fatal(err)
				}
			case "admission failure":
				provider := waiting.CustomProviders[0].Provider
				disableModel := func(disabled bool) {
					t.Helper()
					provider.Models[0].Disabled = disabled
					if err := core.Execute(t.Context(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}}); err != nil {
						t.Fatal(err)
					}
				}
				disableModel(true)
				finish()
				failed := waitSubagentState(t, core, runsFinished, id)
				goal := failed.Sessions[0].Goal
				if len(failed.Runs) != 2 || goal.Status != contracts.GoalPaused || goal.ID != before.ID || goal.ResumeRequestID != "" ||
					goal.NextAction != before.NextAction || !strings.Contains(goal.Reason, "select a model") {
					t.Fatalf("failed admission left a phantom execution: goal=%+v runs=%d", goal, len(failed.Runs))
				}
				disableModel(false)
				if err := core.Execute(t.Context(), contracts.Command{Kind: "goal.resume", SessionID: id, ID: before.ID, RequestID: "retry"}); err != nil {
					t.Fatal(err)
				}
			case "host restart":
				dir := core.storage.ConfigDirectory()
				if err := core.Close(); err != nil {
					t.Fatal(err)
				}
				store, err := storage.Open(storage.Options{Directory: dir})
				if err != nil {
					t.Fatal(err)
				}
				reopened, err := openExecutionFixture(t, Options{Storage: store})
				if err != nil {
					t.Fatal(err)
				}
				defer reopened.Close()
				recovered, err := reopened.Snapshot(t.Context(), id)
				if err != nil {
					t.Fatal(err)
				}
				goal := recovered.Sessions[0].Goal
				if goal.Status != contracts.GoalPaused || goal.ResumeRequestID != "" || goal.ID != before.ID ||
					workers.Load() != 2 || len(recovered.Runs) != 2 || recovered.Runs[1].Status != contracts.RunStatusInterrupted {
					t.Fatalf("host restart replayed interrupted work: goal=%+v runs=%v", goal, recovered.Runs)
				}
				return
			}
			finish()
			state := waitSubagentState(t, core, runsFinished, id)
			goal := state.Sessions[0].Goal
			wantOrdinary := int32(1)
			if action == "queued input" {
				wantOrdinary++
			}
			wantStatus := contracts.RunStatusCompleted
			if action == "ordinary failure" {
				wantStatus = contracts.RunStatusFailed
			} else if action == "archive" || action == "stop ordinary task" {
				wantStatus = contracts.RunStatusInterrupted
			}
			wantCalls := wantOrdinary
			if action == "awaiting answer" {
				wantCalls++
			}
			if ordinary.Load() != wantCalls || state.Runs[1].Status != wantStatus {
				t.Fatalf("resume interfered with ordinary work: calls=%d run=%+v", ordinary.Load(), state.Runs[1])
			}
			if action == "cancel" || action == "clear" || action == "archive" {
				if workers.Load() != 2 || len(state.Runs) != 2 || checks.Load() != 0 ||
					(action != "clear" && (goal == nil || goal.Status != contracts.GoalPaused || goal.Reason != before.Reason)) ||
					(action == "clear" && goal != nil) {
					t.Fatalf("withdrawn resume launched work or lost the paused state: %+v", goal)
				}
				return
			}
			if goal == nil || goal.Status != contracts.GoalComplete || goal.ID == before.ID || goal.InitialInputID != before.InitialInputID ||
				workers.Load() != 4 || checks.Load() != 1 || len(state.Runs) != int(wantOrdinary)+2 {
				t.Fatalf("goal did not resume exactly once: goal=%+v workers=%d checks=%d runs=%d", goal, workers.Load(), checks.Load(), len(state.Runs))
			}
			last := state.Runs[len(state.Runs)-1]
			prior := state.Runs[len(state.Runs)-2]
			requestID := "resume-after-task"
			if action == "admission failure" {
				requestID = "retry"
			}
			if last.RequestID != requestID || prior.FinishedAt == nil || last.StartedAt.Before(*prior.FinishedAt) {
				t.Fatalf("goal ran before ordinary work settled or lost its request: prior=%+v next=%+v", prior, last)
			}
		})
	}
}

func TestPendingGoalResumeWaitsForOrdinaryChildResult(t *testing.T) {
	var goalCalls, ordinaryCalls atomic.Int32
	firstGoal, releaseChild := make(chan struct{}), make(chan struct{})
	finishChild := sync.OnceFunc(func() { close(releaseChild) })
	defer finishChild()
	parent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		if isGoalVerification(body) {
			goalVerdictResponse(w, model.Chat, `{"passed":true,"reason":"Checked."}`, true)
			return
		}
		messages := body["messages"].([]any)
		system, _ := messages[0].(map[string]any)["content"].(string)
		if strings.Contains(system, "Current authoritative session goal") {
			if goalCalls.Add(1) == 1 {
				close(firstGoal)
				<-r.Context().Done()
				return
			}
			if ordinaryCalls.Load() != 3 {
				t.Error("goal started before the ordinary parent processed its child result")
			}
			goalResponse(w, model.Chat, "Goal finished.", nil, true)
			return
		}
		switch ordinaryCalls.Add(1) {
		case 1:
			subagentCall(w, `{"description":"Background lookup","prompt":"Independent lookup","subagent_type":"worker","run_in_background":true}`)
		case 2:
			subagentText(w, model.Chat, "Ordinary parent finished its initial turn.")
		default:
			last, _ := json.Marshal(messages[len(messages)-1])
			if !strings.Contains(string(last), "ORDINARY CHILD RESULT") {
				t.Error("ordinary parent turn did not receive its child result")
			}
			subagentText(w, model.Chat, "Ordinary result consumed.")
		}
	}))
	t.Cleanup(parent.Close)
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-releaseChild:
			subagentText(w, model.Chat, "ORDINARY CHILD RESULT")
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(child.Close)
	core, _ := subagentCore(t, parent.URL, child.URL, model.Chat)
	state, _ := core.Snapshot(t.Context())
	state.Settings.Subagents = []contracts.SubagentProfile{{
		ID: "worker", Name: "Worker", Description: "Lookup",
		Model: &contracts.ModelSelection{ModelID: "child/child-model", Reasoning: "Low"},
	}}
	execute := func(cmd contracts.Command) {
		t.Helper()
		if err := core.Execute(t.Context(), cmd); err != nil {
			t.Fatal(err)
		}
	}
	execute(contracts.Command{Kind: "settings.update", Settings: &state.Settings})
	execute(contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
		ProjectID: state.Projects[0].ID, ModelID: "parent/parent-model", Text: "/goal Check the goal",
	}})
	select {
	case <-firstGoal:
	case <-time.After(10 * time.Second):
		t.Fatal("first goal did not reach its model")
	}
	state, _ = core.Snapshot(t.Context())
	parentID := state.Sessions[0].ID
	send := func(text string) {
		t.Helper()
		execute(contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{SessionID: parentID, Text: text}})
	}
	send("/goal pause")
	waitSubagentState(t, core, runsFinished, parentID)
	send("An independent lookup")
	waitSubagentState(t, core, func(s contracts.Snapshot) bool {
		for _, session := range s.Sessions {
			if session.ID == parentID {
				return session.RunningSubagents == 1 && session.LastOutcome == "completed"
			}
		}
		return false
	}, parentID)
	send("/goal resume")
	state, _ = core.Snapshot(t.Context(), parentID)
	for _, session := range state.Sessions {
		if session.ID == parentID && session.Goal.Status != contracts.GoalResumePending {
			t.Fatalf("resume bypassed the ordinary background child: %+v", session.Goal)
		}
	}
	finishChild()
	state = waitSubagentState(t, core, runsFinished, parentID)
	for _, session := range state.Sessions {
		if session.ID == parentID && (session.Goal.Status != contracts.GoalComplete || session.RunningSubagents != 0) {
			t.Fatalf("parent did not finish: %+v", session)
		}
	}
	if goalCalls.Load() != 2 || ordinaryCalls.Load() != 3 || len(state.Queue) != 0 {
		t.Fatalf("duplicated or missing execution: goal=%d ordinary=%d queue=%+v", goalCalls.Load(), ordinaryCalls.Load(), state.Queue)
	}
}
