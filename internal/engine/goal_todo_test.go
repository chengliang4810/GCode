package engine

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestGoalUsesAuthoritativeTodosAndPausesOnResolution(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Chat, model.Responses, model.Messages} {
		for _, resolution := range []string{"", "blocked", "cancelled", "scope_changed"} {
			t.Run(fmt.Sprintf("%s/resolution=%s", protocol, resolution), func(t *testing.T) {
				var work, checks atomic.Int32
				core, _ := goalTestCore(t, protocol, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body := subagentRequest(t, r)
					if isGoalVerification(body) {
						n := checks.Add(1)
						raw, _ := json.Marshal(body)
						if !strings.Contains(string(raw), "currentTodos") || !strings.Contains(string(raw), "Verify the files") {
							t.Error("verifier did not receive the current structured todo state")
						}
						if work.Load() < 6 {
							t.Error("requested a completion check with executable todos")
							goalResponse(w, protocol, "Here is my progress report.", nil, true)
							return
						}
						if n > 1 {
							http.Error(w, "completion did not settle", http.StatusBadRequest)
							return
						}
						goalVerdictResponse(w, protocol, `{"passed":true,"reason":"All done."}`, true)
						return
					}
					n := work.Add(1)
					switch n {
					case 1:
						args := `{"todos":[{"text":"Verify the files","status":"in_progress"}]}`
						if resolution != "" {
							args = fmt.Sprintf(`{"todos":[{"text":"Verify the files","status":"pending"}],"resolution":{"kind":%q,"reason":"Waiting for required access."}}`, resolution)
						}
						goalResponse(w, protocol, "", &model.Call{ID: "pending", Name: "TodoWrite", Arguments: args}, true)
					case 2, 4, 6:
						goalResponse(w, protocol, "Ready for review.", nil, true)
					case 3:
						goalResponse(w, protocol, "", &model.Call{ID: "partial", Name: "TodoWrite",
							Arguments: `{"todos":[{"text":"Verify the files","status":"in_progress"},{"text":"Verify the last file","status":"pending"}]}`}, true)
					case 5:
						goalResponse(w, protocol, "", &model.Call{ID: "done", Name: "TodoWrite",
							Arguments: `{"todos":[{"text":"Verify the files","status":"completed"},{"text":"Verify the last file","status":"completed"}]}`}, true)
					default:
						http.Error(w, "ordinary closure restarted a settled goal", http.StatusBadRequest)
					}
				}))
				sendGoal(t, core, "/goal Verify and deliver")
				summary, err := core.Snapshot(t.Context())
				if err != nil || len(summary.Sessions) != 1 {
					t.Fatalf("goal session was not created: %+v, %v", summary.Sessions, err)
				}
				sessionID := summary.Sessions[0].ID
				state := waitSubagentState(t, core, runsFinished, sessionID)
				goal := state.Sessions[0].Goal
				if resolution != "" {
					if goal.Status != contracts.GoalPaused || goal.Reason != "Waiting for required access." ||
						work.Load() != 2 || checks.Load() != 0 || len(goal.Verifications) != 0 {
						t.Fatalf("resolution was ignored or marked complete: goal=%+v work=%d checks=%d", goal, work.Load(), checks.Load())
					}
				} else if goal.Status != contracts.GoalComplete || work.Load() != 6 || checks.Load() != 1 ||
					goal.Iteration != 3 || len(goal.Verifications) != 1 || !goal.Verifications[0].Passed {
					t.Fatalf("unfinished list passed verification: goal=%+v work=%d checks=%d", goal, work.Load(), checks.Load())
				}
			})
		}
	}
}

func TestGoalPausesOnRepeatedIdleAndResumesTheAuthoritativeTodo(t *testing.T) {
	var work, checks, resumed atomic.Int32
	const active = "Finish the running check"
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		if isGoalVerification(body) {
			checks.Add(1)
			goalVerdictResponse(w, model.Chat, `{"passed":true,"reason":"Evidence verified"}`, true)
			return
		}
		if work.Add(1) == 1 {
			goalResponse(w, model.Chat, "", &model.Call{ID: "todo", Name: "TodoWrite",
				Arguments: `{"todos":[{"text":"Later task","status":"pending"},{"text":"Finish the running check","status":"in_progress"}]}`}, true)
			return
		}
		if resumed.CompareAndSwap(1, 2) {
			messages := body["messages"].([]any)
			last := messages[len(messages)-1].(map[string]any)
			if text, _ := last["content"].(string); !strings.Contains(text, `"nextAction":"`+active+`"`) {
				t.Error("resume omitted the saved in-progress item from the worker's first request")
			}
			goalResponse(w, model.Chat, "", &model.Call{ID: "finished", Name: "TodoWrite", Arguments: `{"todos":[]}`}, true)
			return
		}
		goalResponse(w, model.Chat, "Stage report.", nil, true)
	}))
	sendGoal(t, core, "/goal Finish the checklist")
	summary, _ := core.Snapshot(t.Context())
	id := summary.Sessions[0].ID
	state := waitSubagentState(t, core, runsFinished, id)
	goal := state.Sessions[0].Goal
	if work.Load() != agent.TodoClosureLimit+2 || checks.Load() != 0 || goal.Status != contracts.GoalPaused || goal.NextAction != active {
		t.Fatalf("unbounded or premature stop: work=%d checks=%d goal=%+v", work.Load(), checks.Load(), goal)
	}
	// The incident record has a checklist and an empty saved next action.
	func() {
		core.mu.Lock()
		defer core.mu.Unlock()
		if err := core.ensureSessionDataLocked(id); err != nil {
			t.Fatal(err)
		}
		core.state.Sessions[core.sessionIndex(id)].Goal.NextAction = ""
		if err := core.persistLocked(id); err != nil {
			t.Fatal(err)
		}
	}()
	resumed.Store(1)
	if err := core.Execute(t.Context(), contracts.Command{Kind: "goal.resume", SessionID: id, ID: goal.ID}); err != nil {
		t.Fatal(err)
	}
	state = waitSubagentState(t, core, runsFinished, id)
	if state.Sessions[0].Goal.Status != contracts.GoalComplete || checks.Load() != 1 {
		t.Fatalf("resume did not finish: checks=%d goal=%+v", checks.Load(), state.Sessions[0].Goal)
	}
}

func TestGoalRevisionRejectsEditsAndVerdictsAfterObjectiveChangesBack(t *testing.T) {
	checking, release := make(chan struct{}, 1), make(chan struct{})
	var checks, work atomic.Int32
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isGoalVerification(subagentRequest(t, r)) {
			if checks.Add(1) == 1 {
				checking <- struct{}{}
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
			}
			goalVerdictResponse(w, model.Chat, `{"passed":true,"reason":"Verified."}`, true)
			return
		}
		work.Add(1)
		goalResponse(w, model.Chat, "Ready.", nil, true)
	}))
	sendGoal(t, core, "/goal Original")
	select {
	case <-checking:
	case <-time.After(10 * time.Second):
		t.Fatal("goal never reached verification")
	}
	summary, err := core.Snapshot(t.Context())
	if err != nil || len(summary.Sessions) != 1 {
		t.Fatalf("goal session disappeared: %+v, %v", summary.Sessions, err)
	}
	state, err := core.Snapshot(t.Context(), summary.Sessions[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	session := state.Sessions[0]
	edit := func(objective string, revision uint64) error {
		return core.Execute(t.Context(), contracts.Command{Kind: "goal.edit", SessionID: session.ID, ID: session.Goal.ID,
			GoalEdit: &contracts.GoalEditInput{Objective: objective, ExpectedRevision: revision}})
	}
	if err := edit("Changed", 0); err != nil {
		t.Fatal(err)
	}
	if err := edit("Original", 1); err != nil {
		t.Fatal(err)
	}
	if err := edit("Stale draft", 0); err == nil {
		t.Fatal("changing back to identical text allowed a stale editor to overwrite")
	}
	close(release)
	state = waitSubagentState(t, core, runsFinished, session.ID)
	goal := state.Sessions[0].Goal
	if goal.Revision != 2 || goal.Objective != "Original" || goal.Status != contracts.GoalComplete ||
		checks.Load() != 2 || work.Load() != 2 || len(goal.Verifications) != 1 {
		t.Fatalf("stale verdict survived an objective revision: %+v checks=%d work=%d", goal, checks.Load(), work.Load())
	}
}

func TestGoalCommandsReadAndExplicitReplacePreserveUserIntent(t *testing.T) {
	var work atomic.Int32
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isGoalVerification(subagentRequest(t, r)) {
			goalVerdictResponse(w, model.Chat, `{"passed":true,"reason":"Verified."}`, true)
			return
		}
		n := work.Add(1)
		switch n {
		case 1:
			goalResponse(w, model.Chat, "", &model.Call{ID: "blocked", Name: "TodoWrite", Arguments: `{"todos":[{"text":"Need access","status":"pending"}],"resolution":{"kind":"blocked","reason":"Need access."}}`}, true)
		case 3:
			goalResponse(w, model.Chat, "", &model.Call{ID: "clear", Name: "TodoWrite", Arguments: `{"todos":[]}`}, true)
		default:
			goalResponse(w, model.Chat, "Ready.", nil, true)
		}
	}))
	sendGoal(t, core, "/goal First goal")
	summary, err := core.Snapshot(t.Context())
	if err != nil || len(summary.Sessions) != 1 {
		t.Fatalf("goal session was not created: %+v, %v", summary.Sessions, err)
	}
	sessionID := summary.Sessions[0].ID
	state := waitSubagentState(t, core, runsFinished, sessionID)
	session := state.Sessions[0]
	send := func(text string) error {
		return core.Execute(t.Context(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
			SessionID: session.ID, Text: text, ModelID: "goal/model",
		}})
	}
	if err := send("/goal"); err != nil {
		t.Fatal("bare /goal did not read the current goal:", err)
	}
	if work.Load() != 2 {
		t.Fatal("reading a goal consumed model calls")
	}
	if err := send("/goal Unintended overwrite"); err == nil {
		t.Fatal("a new /goal silently replaced unfinished work")
	}
	if err := send("/goal replace"); err == nil {
		t.Fatal("empty replacement objective was accepted")
	}
	unchanged, _ := core.Snapshot(t.Context(), session.ID)
	if unchanged.Sessions[0].Goal.ID != session.Goal.ID ||
		len(unchanged.Messages[session.ID]) != len(state.Messages[session.ID]) {
		t.Fatal("reading or rejecting a replacement changed the goal or transcript")
	}
	if err := send("/goal replace Replacement goal"); err != nil {
		t.Fatal(err)
	}
	state = waitSubagentState(t, core, runsFinished, session.ID)
	goal := state.Sessions[0].Goal
	if goal.ID == session.Goal.ID || goal.Objective != "Replacement goal" || goal.Status != contracts.GoalComplete {
		t.Fatalf("explicit replacement did not start a new goal: %+v", goal)
	}
}

func TestGoalEditedDuringWorkReachesExecutorBeforeVerification(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Chat, model.Responses, model.Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			started, release := make(chan struct{}, 1), make(chan struct{})
			var work, checks atomic.Int32
			core, _ := goalTestCore(t, protocol, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := subagentRequest(t, r)
				if isGoalVerification(body) {
					checks.Add(1)
					if work.Load() < 2 {
						t.Error("the old work answer reached verification before executing the edited objective")
					}
					goalVerdictResponse(w, protocol, `{"passed":true,"reason":"New requirements verified."}`, true)
					return
				}
				if work.Add(1) == 1 {
					started <- struct{}{}
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
				} else {
					raw, _ := json.Marshal(body)
					if !strings.Contains(string(raw), "New requirements") {
						t.Error("the executor did not receive the edited objective")
					}
				}
				goalResponse(w, protocol, "Ready.", nil, true)
			}))
			sendGoal(t, core, "/goal Original requirements")
			select {
			case <-started:
			case <-time.After(10 * time.Second):
				t.Fatal("initial work request did not start")
			}
			summary, err := core.Snapshot(t.Context())
			if err != nil || len(summary.Sessions) != 1 {
				t.Fatalf("goal session was not created: %+v, %v", summary.Sessions, err)
			}
			state, err := core.Snapshot(t.Context(), summary.Sessions[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			session := state.Sessions[0]
			if err := core.Execute(t.Context(), contracts.Command{Kind: "goal.edit", SessionID: session.ID, ID: session.Goal.ID,
				GoalEdit: &contracts.GoalEditInput{Objective: "New requirements", ExpectedRevision: 0}}); err != nil {
				t.Fatal(err)
			}
			close(release)
			state = waitSubagentState(t, core, runsFinished, session.ID)
			if state.Sessions[0].Goal.Status != contracts.GoalComplete || work.Load() != 2 || checks.Load() != 1 {
				t.Fatalf("edit bypassed the work boundary: goal=%+v work=%d checks=%d", state.Sessions[0].Goal, work.Load(), checks.Load())
			}
		})
	}
}
