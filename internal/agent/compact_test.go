package agent

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gcode/internal/model"
)

func TestCompactionReusesOriginalRequestPrefixAndRejectsToolExecution(t *testing.T) {
	history := []model.Message{
		{Role: "user", Text: "inspect", UserInput: true},
		{Role: "assistant", Calls: []model.Call{{ID: "read", Name: "Read", Arguments: "{}"}}},
		{Role: "tool", CallID: "read", Text: "original output"},
		{Role: "user", Text: "continue"}, {Role: "assistant", Text: "working"}, {Role: "user", Text: "finish"},
	}
	for range 5 {
		history = append(history, model.Message{Role: "user", Text: "follow-up"})
	}
	request := model.Request{System: "stable system", Effort: "high", Tools: []model.Tool{{Name: "Read"}}, History: history}
	for _, calls := range []bool{false, true} {
		name := "summary"
		if calls {
			name = "tool request rejected"
		}
		t.Run(name, func(t *testing.T) {
			backend := testModel{stream: func(got model.Request) (model.Result, error) {
				if got.System != request.System || got.Effort != request.Effort || !reflect.DeepEqual(got.Tools, request.Tools) ||
					!reflect.DeepEqual(got.History[:3], request.History[:3]) || got.ContextChange != "summary" {
					t.Fatalf("summary rewrote cached prefix: %+v", got)
				}
				last := got.History[len(got.History)-1]
				if !strings.Contains(last.Text, "do not call tools") {
					t.Fatal("summary has no no-tools instruction")
				}
				result := model.Result{Message: model.Message{Role: "assistant", Text: "summary"}}
				if calls {
					result.Message.Calls = []model.Call{{ID: "unsafe", Name: "Write"}}
				}
				return result, nil
			}}
			got, err := Compact(t.Context(), backend, request)
			if calls {
				if err == nil || got != nil {
					t.Fatal("summary accepted tool execution")
				}
			} else if err != nil || !got[0].Summary || !reflect.DeepEqual(got[1:], history[3:]) {
				t.Fatalf("summary failed or changed retained tail: %v, %+v", err, got)
			}
		})
	}
}

func TestRunKeepsOldToolResultsUnchangedBelowContextPressure(t *testing.T) {
	history := []model.Message{
		{Role: "user", Text: "first", UserInput: true},
		{Role: "assistant", Calls: []model.Call{{ID: "old", Name: "Read"}}},
		{Role: "tool", CallID: "old", Text: strings.Repeat("old output\n", 1000)},
		{Role: "user", Text: "second", UserInput: true},
		{Role: "assistant", Text: "done"},
		{Role: "user", Text: "third", UserInput: true},
	}
	err := Run(t.Context(), Options{
		Model: testModel{stream: func(request model.Request) (model.Result, error) {
			if !reflect.DeepEqual(request.History, history) {
				t.Fatal("historical prefix was rewritten without context pressure")
			}
			return model.Result{Message: model.Message{Role: "assistant", Text: "done"}}, nil
		}},
		ContextLimits:  func() ContextLimits { return ContextLimits{Window: 1_000_000} },
		Request:        model.Request{History: history},
		ReplaceContext: func([]model.Message) error { t.Fatal("unexpected compaction"); return nil },
		Record:         func(model.Message) error { return nil }, Emit: func(Event) error { return nil }, Pending: func(bool) []model.Message { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
}
func TestCompactionUsesUpdatedContextWindowBetweenSteps(t *testing.T) {
	window, normal, summaries, replacements := 0, 0, 0, 0
	var history []model.Message
	for range 12 {
		history = append(history, model.Message{Role: "user", Text: strings.Repeat("history ", 100), UserInput: true})
	}
	backend := testModel{stream: func(request model.Request) (model.Result, error) {
		if request.ContextChange == "summary" {
			summaries++
			return model.Result{Message: model.Message{Role: "assistant", Text: "Earlier task context."}, Stop: model.StopComplete}, nil
		}
		normal++
		if normal == 1 {
			window = 100
			return model.Result{Message: model.Message{Role: "assistant", Calls: []model.Call{{
				ID: "write", Name: "Write", Arguments: `{"file_path":"proof.txt","content":"done"}`,
			}}}, Stop: model.StopTools}, nil
		}
		return model.Result{Message: model.Message{Role: "assistant", Text: "Done."}, Stop: model.StopComplete}, nil
	}}
	err := Run(t.Context(), Options{
		Model: backend, ContextLimits: func() ContextLimits { return ContextLimits{Window: window} },
		CWD: t.TempDir(), Tools: CoreTools(), Request: model.Request{History: history},
		ReplaceContext: func([]model.Message) error { replacements++; return nil },
		Record:         func(model.Message) error { return nil },
		Emit:           func(Event) error { return nil },
		Pending:        func(bool) []model.Message { return nil },
	})
	if err != nil || normal != 2 || summaries != 1 || replacements != 1 {
		t.Fatalf("updated capacity was not applied: normal=%d summaries=%d replacements=%d err=%v", normal, summaries, replacements, err)
	}
}

func TestCompactionPreservesQueuedInputOwnership(t *testing.T) {
	history := make([]model.Message, 20)
	for i := range history {
		history[i] = model.Message{Role: "user", Text: "requirement", InputIDs: []string{fmt.Sprint(i)}}
	}
	result, err := Compact(context.Background(), &imagePressureModel{}, model.Request{History: history})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, message := range result {
		ids = append(ids, message.InputIDs...)
	}
	var want []string
	for _, message := range history {
		want = append(want, message.InputIDs...)
	}
	if !slices.Equal(ids, want) {
		t.Fatalf("compaction lost input ownership: %v, want %v", ids, want)
	}
	cloned := model.CloneMessages(result)
	cloned[0].InputIDs[0] = "changed"
	if result[0].InputIDs[0] != "0" {
		t.Fatal("worker history shares input ownership metadata")
	}
}

type imagePressureModel struct {
	called bool
}

func (m *imagePressureModel) Stream(_ context.Context, request model.Request, _ func(model.Delta) error) (model.Result, error) {
	m.called = true
	return model.Result{
		Message: model.Message{Role: "assistant", Text: "image received"}, Stop: model.StopComplete,
	}, nil
}

func TestFirstImageRequestDoesNotFailTryingToCompactTooLittleHistory(t *testing.T) {
	bound := &imagePressureModel{}
	err := Run(context.Background(), Options{
		Model: bound, ContextLimits: func() ContextLimits { return ContextLimits{Window: 8192} },
		Request: model.Request{History: []model.Message{{
			Role: "user", Text: "inspect", Images: []model.Image{{MIME: "image/png", Width: 2048, Height: 2048}},
		}}},
		ReplaceContext: func([]model.Message) error {
			t.Fatal("an initial image cannot produce a complete prefix for compaction")
			return nil
		},
		Record:  func(model.Message) error { return nil },
		Emit:    func(Event) error { return nil },
		Pending: func(bool) []model.Message { return nil },
	})
	if err != nil || !bound.called {
		t.Fatalf("image request failed before reaching the model: %v", err)
	}
}
