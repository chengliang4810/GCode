package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/model"
)

type probeTool struct {
	name       string
	concurrent bool
	execute    func(context.Context) error
}

type failingImageTool struct{}

func (failingImageTool) Definition() model.Tool { return model.Tool{Name: "broken-image"} }
func (failingImageTool) Concurrent() bool       { return true }
func (failingImageTool) Execute(context.Context, string, json.RawMessage) (model.ToolOutput, error) {
	return model.ToolOutput{
		Text: "read failed", Images: []model.Image{{MIME: "image/png", Data: "private-image-payload"}},
	}, errors.New("decode failure")
}

func TestToolErrorsDropImagesAndKeepDisplayText(t *testing.T) {
	var recorded model.Message
	var ended Event
	err := executeTools(
		context.Background(),
		Options{Emit: func(event Event) error {
			if event.Kind == "tool.end" {
				ended = event
			}
			return nil
		}},
		map[string]Tool{"broken-image": failingImageTool{}},
		[]model.Call{{ID: "failed-call", Name: "broken-image", Arguments: "{}"}},
		func(message model.Message) error { recorded = message; return nil },
	)
	if err != nil || !recorded.IsError || len(recorded.Images) != 0 ||
		!strings.Contains(recorded.Text, "decode failure") || !ended.Error ||
		strings.Contains(ended.Output, "private-image-payload") {
		t.Fatalf("error output published image bytes: message=%+v event=%+v err=%v", recorded, ended, err)
	}
}

func (t probeTool) Definition() model.Tool { return model.Tool{Name: t.name} }
func (t probeTool) Concurrent() bool       { return t.concurrent }
func (t probeTool) Execute(ctx context.Context, _ string, _ json.RawMessage) (model.ToolOutput, error) {
	return model.ToolOutput{Text: t.name}, t.execute(ctx)
}

func TestReadToolsOverlapAndMutationIsBarrier(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var running, started atomic.Int32
	both := make(chan struct{})
	read := probeTool{name: "Read", concurrent: true, execute: func(ctx context.Context) error {
		running.Add(1)
		defer running.Add(-1)
		if started.Add(1) == 2 {
			close(both)
		}
		select {
		case <-both:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	write := probeTool{name: "Write", execute: func(context.Context) error {
		if running.Load() != 0 || started.Load() != 2 {
			t.Error("mutation crossed the read barrier")
		}
		return nil
	}}
	var recorded []string
	err := executeTools(ctx, Options{Emit: func(Event) error { return nil }}, map[string]Tool{"Read": read, "Write": write},
		[]model.Call{{ID: "a", Name: "Read"}, {ID: "b", Name: "Read"}, {ID: "c", Name: "Write"}},
		func(m model.Message) error { recorded = append(recorded, m.CallID); return nil })
	if err != nil || len(recorded) != 3 || recorded[0] != "a" || recorded[1] != "b" || recorded[2] != "c" {
		t.Fatalf("%v %v", recorded, err)
	}
}
