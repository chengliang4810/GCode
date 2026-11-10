package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestHookLifecyclePreparesOriginalInputAfterStartupContext(t *testing.T) {
	prepared, committed := 0, 0
	var prompt, display string
	var displayedInput HookInput
	err := Run(context.Background(), Options{
		Model: testModel{stream: func(request model.Request) (model.Result, error) {
			foundPrepared, foundStartup := false, false
			for _, message := range request.History {
				foundPrepared = foundPrepared || message.Text == "expanded skill"
				foundStartup = foundStartup || strings.Contains(message.Text, "startup facts")
			}
			if !foundPrepared || !foundStartup {
				t.Errorf("prepared=%v startup=%v history=%+v", foundPrepared, foundStartup, request.History)
			}
			return model.Result{Message: model.Message{Role: "assistant", Text: "original display"}}, nil
		}},
		Request: model.Request{History: []model.Message{{Role: "user", Text: "/example:skill", SourceText: "/example:skill", UserInput: true}}},
		Record:  func(model.Message) error { return nil }, Emit: func(event Event) error {
			if event.Kind == "message.display" {
				display = event.Text
			}
			return nil
		}, Pending: func(bool) []model.Message { return nil },
		PrepareInput: func(_ context.Context, message model.Message) (model.Message, error) {
			prepared++
			if message.SourceText != "/example:skill" {
				t.Errorf("prepared wrong input: %+v", message)
			}
			message.Text = "expanded skill"
			return message, nil
		},
		CommitPreparedInput: func(message model.Message) error {
			committed++
			if message.Text != "expanded skill" {
				t.Errorf("%+v", message)
			}
			return nil
		},
		Hook: func(_ context.Context, input HookInput) (HookResult, error) {
			switch input.Event {
			case contracts.HookSessionStart:
				return HookResult{Context: "startup facts"}, nil
			case contracts.HookUserPromptSubmit:
				prompt = input.Prompt
			case contracts.HookMessageDisplay:
				displayedInput = input
				replacement := "display replacement"
				return HookResult{DisplayContent: &replacement}, nil
			}
			return HookResult{}, nil
		}})
	if err != nil || prepared != 1 || committed != 1 || prompt != "/example:skill" || display != "display replacement" {
		t.Fatalf("prepared=%d committed=%d prompt=%q display=%q err=%v", prepared, committed, prompt, display, err)
	}
	if displayedInput.Index != 0 || !displayedInput.Final || displayedInput.Delta != "original display" || displayedInput.TurnID == "" || displayedInput.MessageID == "" {
		t.Fatalf("invalid public display event: %+v", displayedInput)
	}
}

func TestHookLifecyclePostBatchUsesModelFacingToolResults(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	var batches atomic.Int32
	var postTools atomic.Int32
	err := Run(context.Background(), Options{CWD: directory, Tools: CoreTools(),
		Model: testModel{stream: func(model.Request) (model.Result, error) {
			calls++
			if calls == 1 {
				return model.Result{Message: model.Message{Role: "assistant", Calls: []model.Call{
					{ID: "a", Name: "Read", Arguments: `{"file_path":"a.txt"}`}, {ID: "b", Name: "Read", Arguments: `{"file_path":"b.txt"}`}}}}, nil
			}
			return model.Result{Message: model.Message{Role: "assistant", Text: "done"}}, nil
		}},
		Record: func(model.Message) error { return nil }, Emit: func(Event) error { return nil }, Pending: func(bool) []model.Message { return nil },
		Hook: func(_ context.Context, input HookInput) (HookResult, error) {
			if input.Event == contracts.HookPostToolUse {
				postTools.Add(1)
				if _, ok := input.ToolOutput.(map[string]any); !ok {
					t.Errorf("PostToolUse lost structured output: %#v", input.ToolOutput)
				}
			}
			if input.Event == contracts.HookPostToolBatch {
				batches.Add(1)
				if len(input.ToolCalls) != 2 {
					t.Errorf("partial batch: %+v", input.ToolCalls)
				}
				for _, call := range input.ToolCalls {
					if text, ok := call.ToolResponse.(string); !ok || !strings.HasPrefix(text, "1: ") {
						t.Errorf("batch received structured data instead of model content: %+v", call)
					}
				}
			}
			return HookResult{}, nil
		}})
	if err != nil || batches.Load() != 1 || postTools.Load() != 2 || calls != 2 {
		t.Fatalf("batches=%d post=%d calls=%d err=%v", batches.Load(), postTools.Load(), calls, err)
	}
}

func TestHookLifecycleContinueFalseStopsToolAndModel(t *testing.T) {
	directory := t.TempDir()
	calls := 0
	stop := false
	err := Run(context.Background(), Options{CWD: directory, Tools: CoreTools(),
		Model: testModel{stream: func(model.Request) (model.Result, error) {
			calls++
			return model.Result{Message: model.Message{Role: "assistant", Calls: []model.Call{{ID: "write", Name: "Write", Arguments: `{"file_path":"never.txt","content":"no"}`}}}}, nil
		}},
		Record: func(model.Message) error { return nil }, Emit: func(Event) error { return nil }, Pending: func(bool) []model.Message { return nil },
		Hook: func(_ context.Context, input HookInput) (HookResult, error) {
			if input.Event == contracts.HookPreToolUse {
				return HookResult{Continue: &stop, StopReason: "finished"}, nil
			}
			return HookResult{}, nil
		}})
	if err != nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	if _, err := os.Stat(filepath.Join(directory, "never.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stopped hook still wrote file: %v", err)
	}
}

func TestHookLifecyclePostBatchCanStopBeforeNextModelRequest(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "a.txt"), []byte("evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	var recorded []model.Message
	err := Run(context.Background(), Options{CWD: directory, Tools: CoreTools(),
		Model: testModel{stream: func(model.Request) (model.Result, error) {
			calls++
			return model.Result{Message: model.Message{Role: "assistant", Calls: []model.Call{{ID: "read", Name: "Read", Arguments: `{"file_path":"a.txt"}`}}}}, nil
		}},
		Record: func(message model.Message) error { recorded = append(recorded, message); return nil }, Emit: func(Event) error { return nil }, Pending: func(bool) []model.Message { return nil },
		Hook: func(_ context.Context, input HookInput) (HookResult, error) {
			if input.Event == contracts.HookPostToolBatch {
				return HookResult{Deny: true, Reason: "batch gate"}, nil
			}
			return HookResult{}, nil
		}})
	if err != nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	if len(recorded) == 0 || !strings.Contains(recorded[len(recorded)-1].Text, "batch gate") {
		t.Fatalf("blocking feedback not retained: %+v", recorded)
	}
}

func TestHookLifecycleStopFailureUsesPublicErrorType(t *testing.T) {
	expected := errors.New("model request failed (HTTP 429)")
	var failure HookInput
	stopped := false
	err := Run(context.Background(), Options{Model: testModel{stream: func(model.Request) (model.Result, error) { return model.Result{}, expected }},
		Record: func(model.Message) error { return nil }, Emit: func(Event) error { return nil }, Pending: func(bool) []model.Message { return nil },
		Hook: func(_ context.Context, input HookInput) (HookResult, error) {
			if input.Event == contracts.HookStopFailure {
				failure = input
			}
			if input.Event == contracts.HookStop {
				stopped = true
			}
			return HookResult{}, nil
		}})
	if !errors.Is(err, expected) || failure.ToolError != "rate_limit" || failure.ErrorDetails != expected.Error() || stopped {
		t.Fatalf("failure=%+v stop=%v err=%v", failure, stopped, err)
	}
}

func TestHookLifecycleCompactionEventsSurroundActualSummary(t *testing.T) {
	history := make([]model.Message, 6)
	for i := range history {
		history[i] = model.Message{Role: "user", Text: strings.Repeat("history", 200)}
	}
	calls := 0
	var events []HookInput
	err := Run(context.Background(), Options{ContextLimits: func() ContextLimits { return ContextLimits{Window: 100} }, Request: model.Request{History: history},
		Model: testModel{stream: func(request model.Request) (model.Result, error) {
			calls++
			if request.ContextChange == "summary" {
				return model.Result{Message: model.Message{Role: "assistant", Text: "actual compacted summary"}}, nil
			}
			return model.Result{Message: model.Message{Role: "assistant", Text: "done"}}, nil
		}},
		Record: func(model.Message) error { return nil }, ReplaceContext: func(messages []model.Message) error {
			if !strings.Contains(messages[0].Text, "actual compacted summary") {
				t.Errorf("wrong summary: %+v", messages)
			}
			return nil
		},
		Emit: func(Event) error { return nil }, Pending: func(bool) []model.Message { return nil },
		Hook: func(_ context.Context, input HookInput) (HookResult, error) {
			if input.Event == contracts.HookPreCompact || input.Event == contracts.HookPostCompact {
				events = append(events, input)
			}
			return HookResult{}, nil
		}})
	if err != nil || calls != 2 || len(events) != 2 || events[0].Event != contracts.HookPreCompact || events[1].Event != contracts.HookPostCompact || !strings.Contains(events[1].CompactSummary, "actual compacted summary") {
		t.Fatalf("calls=%d events=%+v err=%v", calls, events, err)
	}
}

type hookScopeActivationTool struct{ activate func() }

func (t hookScopeActivationTool) Definition() model.Tool { return model.Tool{Name: "Skill"} }
func (t hookScopeActivationTool) Concurrent() bool       { return false }
func (t hookScopeActivationTool) Execute(context.Context, string, json.RawMessage) (model.ToolOutput, error) {
	t.activate()
	return model.ToolOutput{Text: "skill activated"}, nil
}

func TestHookLifecycleSkillDenialAppliesWithinSameToolBatch(t *testing.T) {
	directory := t.TempDir()
	var denied atomic.Bool
	calls := 0
	tools := append(CoreTools(), hookScopeActivationTool{activate: func() { denied.Store(true) }})
	err := Run(context.Background(), Options{CWD: directory, Tools: tools, ToolAllowed: func(name string) bool { return !denied.Load() || name != "Write" },
		Model: testModel{stream: func(request model.Request) (model.Result, error) {
			calls++
			if calls == 1 {
				return model.Result{Message: model.Message{Role: "assistant", Calls: []model.Call{
					{ID: "activate", Name: "Skill", Arguments: `{}`}, {ID: "write", Name: "Write", Arguments: `{"file_path":"blocked.txt","content":"must not execute"}`}}}}, nil
			}
			for _, tool := range request.Tools {
				if tool.Name == "Write" {
					t.Error("disabled Write remained in next model request")
				}
			}
			last := request.History[len(request.History)-1]
			if !last.IsError || !strings.Contains(last.Text, "disabled") {
				t.Errorf("missing hard denial: %+v", last)
			}
			return model.Result{Message: model.Message{Role: "assistant", Text: "done"}}, nil
		}},
		Record: func(model.Message) error { return nil }, Emit: func(Event) error { return nil }, Pending: func(bool) []model.Message { return nil }})
	if err != nil || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	if _, err := os.Stat(filepath.Join(directory, "blocked.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("same-turn write bypassed skill restriction: %v", err)
	}
}
