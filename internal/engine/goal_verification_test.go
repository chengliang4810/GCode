package engine

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestGoalVerifierUsesBoundedAttributedEvidence(t *testing.T) {
	long := strings.Repeat("检查工具输出", 1800)
	history := []model.Message{
		{Role: "user", Summary: true, Text: "Checkpoint", RetainedInputs: []string{"Original constraint"}, RetainedEvidence: []string{"Earlier inspection"}},
		{Role: "user", UserInput: true, Text: "First human requirement"},
	}
	for range 708 {
		history = append(history, model.Message{Role: "assistant", Text: "Stage summary", Reasoning: "private reasoning", Calls: []model.Call{{ID: "old", Name: "Read", Arguments: long}}},
			model.Message{Role: "tool", CallID: "old", Text: long})
	}
	history = append(history,
		model.Message{Role: "user", UserInput: true, Text: "Recheck the failure"},
		model.Message{Role: "assistant", Calls: []model.Call{{ID: "a", Name: "Bash", Arguments: "test A"}, {ID: "b", Name: "Read", Arguments: "file B"}}},
		model.Message{Role: "tool", CallID: "b", Text: "file B was missing", IsError: true},
		model.Message{Role: "tool", CallID: "a", Text: "test A:\n" + long + "\nPASS: test A",
			Images: []model.Image{{MIME: "image/png", Data: strings.Repeat("x", 256<<10)}, {MIME: "image/png", Data: "cHJvb2Y="}}},
		model.Message{Role: "user", Text: "Background subagent result: inspected logs"},
		model.Message{Role: "assistant", Text: "Worker finished"},
	)
	todos := &agent.TodoUpdate{Todos: []agent.TodoItem{{Text: long, Status: agent.TodoCompleted}}}
	request := goalVerificationRequest(&contracts.SessionGoal{Objective: "Check both files", NextAction: "Get the missing evidence"}, todos, history)
	if len(request.History) != 2 || request.History[1].Images[0].Data != "cHJvb2Y=" || !strings.Contains(request.History[1].Text, "image 2") {
		t.Fatalf("did not keep the bounded, attributed image: %d messages", len(request.History))
	}
	text := request.History[0].Text
	if len(text) > 64<<10 || !utf8.ValidString(text) || strings.Contains(text, "private reasoning") {
		t.Fatalf("unbounded or invalid evidence: %d bytes", len(text))
	}
	for _, want := range []string{"Original constraint", "Earlier inspection", "First human requirement", "Recheck the failure",
		"Worker finished", "inspected logs", "Check both files", "Get the missing evidence"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing evidence %q", want)
		}
	}
	var data struct {
		HistoryMessages int
		Evidence        []goalEvidenceItem
	}
	if err := json.Unmarshal([]byte(text), &data); err != nil || data.HistoryMessages != len(history) {
		t.Fatalf("invalid evidence record: %v, history=%d", err, data.HistoryMessages)
	}
	hasResult := func(tool, args, suffix string, failed bool) bool {
		return slices.ContainsFunc(data.Evidence, func(item goalEvidenceItem) bool {
			return item.Tool == tool && item.Arguments == args && strings.HasSuffix(item.Text, suffix) && item.Failed == failed
		})
	}
	if !hasResult("Bash", "test A", "PASS: test A", false) || !hasResult("Read", "file B", "file B was missing", true) {
		t.Error("parallel tool results lost their call, failure status or output tail")
	}
	if todos.Todos[0].Text != long || history[len(history)-3].Text != "test A:\n"+long+"\nPASS: test A" {
		t.Error("verification modified the worker's history or checklist")
	}
}

func TestGoalVerifierRejectedChoiceStillRequiresValidCall(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Chat, model.Responses, model.Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			var checks atomic.Int32
			core, _ := goalTestCore(t, protocol, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := subagentRequest(t, r)
				if !isGoalVerification(body) {
					goalResponse(w, protocol, "Work result", nil, true)
					return
				}
				switch n := checks.Add(1); n {
				case 1:
					if body["tool_choice"] == nil {
						t.Error("missing initial forced choice")
					}
					http.Error(w, `{"error":{"type":"invalid_request_error","message":"tool_choice is not supported in thinking mode"}}`, http.StatusBadRequest)
				case 2, 3, 4:
					if tools, _ := body["tools"].([]any); body["tool_choice"] != nil || len(tools) != 1 {
						t.Error("provider rejection must relax only the forced choice")
					}
					switch n {
					case 2:
						goalResponse(w, protocol, `{"passed":true,"reason":"Only prose"}`, nil, true)
					case 3:
						goalVerdictResponse(w, protocol, `{"passed":false,"reason":"Need another inspection","nextAction":"Check the file"}`, true)
					case 4:
						goalVerdictResponse(w, protocol, `{"passed":true,"reason":"Checked the result"}`, true)
					}
				default:
					http.Error(w, "extra request", http.StatusBadRequest)
				}
			}))
			sendGoal(t, core, "/goal Check the result")
			summary, _ := core.Snapshot(t.Context())
			state := waitSubagentState(t, core, runsFinished, summary.Sessions[0].ID)
			goal := state.Sessions[0].Goal
			if checks.Load() != 4 || goal.Status != contracts.GoalComplete || len(goal.Verifications) != 2 ||
				goal.Verifications[1].Reason != "Checked the result" {
				t.Fatalf("did not recover a strictly validated verdict: %+v, checks=%d", goal, checks.Load())
			}
		})
	}
}
