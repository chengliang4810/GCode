package agent

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"gcode/internal/model"
)

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
		Model: bound, ContextWindow: 8192,
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
