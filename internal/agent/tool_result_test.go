package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/model"
)

func TestToolResultOffloadIsDurableAndDedupUsesOnlyCurrentContext(t *testing.T) {
	text := "HEAD\n" + strings.Repeat("完整日志\n", 8000) + "\nTAIL"
	path := filepath.Join(t.TempDir(), "full.txt")
	stores := 0
	store := func(_ context.Context, content string) (string, error) {
		stores++
		return path, os.WriteFile(path, []byte(content), 0600)
	}
	call := model.Call{ID: "first", Name: "Read"}
	first, err := prepareToolResult(t.Context(), store, nil, call, model.Message{Role: "tool", CallID: call.ID, Text: text})
	if err != nil || first.OutputFile != path || len(first.Text) >= len(text) || !strings.Contains(first.Text, "HEAD") || !strings.Contains(first.Text, "TAIL") {
		t.Fatalf("output was lost or not bounded: err=%v message=%+v", err, first)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != text {
		t.Fatal("full output is not readable")
	}
	data, _ := json.Marshal([]model.Message{first})
	var restored []model.Message
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	second, err := prepareToolResult(t.Context(), store, restored, model.Call{ID: "second", Name: "Read"}, model.Message{Role: "tool", CallID: "second", Text: text})
	if err != nil || stores != 1 || second.ContentHash != "" || !strings.Contains(second.Text, "first") {
		t.Fatalf("resumed context was not deduplicated: %v %+v stores=%d", err, second, stores)
	}
	// A stub alone does not prove the original output is still visible.
	third, err := prepareToolResult(t.Context(), store, []model.Message{second}, model.Call{Name: "Read"}, model.Message{Role: "tool", Text: text})
	if err != nil || stores != 2 || third.OutputFile != path {
		t.Fatal("compaction caused a dangling content reference")
	}
}

func TestToolImagesDeduplicateWithoutChangingOldMessages(t *testing.T) {
	image := model.Image{MIME: "image/png", Data: "identical bytes", Width: 10, Height: 20}
	history := []model.Message{{Role: "tool", CallID: "original", Images: []model.Image{image}}}
	message := model.Message{Role: "tool", CallID: "new", Images: []model.Image{image}}
	got, err := prepareToolResult(t.Context(), nil, history, model.Call{Name: "Read"}, message)
	if err != nil || len(got.Images) != 0 || !strings.Contains(got.Text, "original") || len(message.Images) != 1 || history[0].Images[0] != image {
		t.Fatalf("image dedup modified a cached prefix: %+v %v", got, err)
	}
	again, err := prepareToolResult(t.Context(), nil, nil, model.Call{Name: "Read"}, message)
	if err != nil || len(again.Images) != 1 {
		t.Fatal("image stayed suppressed after leaving context")
	}
}

func TestToolOutputStorageFailurePreservesCompletedEffect(t *testing.T) {
	text := strings.Repeat("result\n", 6000)
	storageError := errors.New("disk full")
	var messages []model.Message
	steps := 0
	err := Run(t.Context(), Options{
		Model: testModel{stream: func(model.Request) (model.Result, error) {
			steps++
			return model.Result{Message: model.Message{Role: "assistant", Calls: []model.Call{{ID: "effect", Name: "Large"}}}}, nil
		}},
		Tools:           []Tool{largeOutputTool{text: text}},
		StoreToolOutput: func(context.Context, string) (string, error) { return "", storageError },
		Record:          func(message model.Message) error { messages = append(messages, message); return nil },
		Emit:            func(Event) error { return nil }, Pending: func(bool) []model.Message { return nil },
	})
	if !errors.Is(err, storageError) || steps != 1 || len(messages) != 2 || messages[1].Text != text || messages[1].OutputFile != "" {
		t.Fatalf("storage failure lost the effect or replayed it: %v steps=%d messages=%d", err, steps, len(messages))
	}
}

func TestHookReplacementDoesNotExposeOriginalOutputFile(t *testing.T) {
	output := model.ToolOutput{Text: "original", OutputFile: "/private/raw.txt",
		Data: map[string]any{"stdout": "original", "stderr": "", "interrupted": false}}
	got := applyHookToolOutput("Bash", output, json.RawMessage(`{"stdout":"redacted","stderr":"","interrupted":false}`))
	if got.OutputFile != "" || got.Text != "redacted" {
		t.Fatal("post-tool hook replacement exposed the original stream")
	}
}

func TestToolOutputPreviewPreservesFullHookContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output.txt")
	contextText := "\n<system-reminder>\n" + strings.Repeat("preserve this instruction\n", 100) + "</system-reminder>"
	steps := 0
	err := Run(t.Context(), Options{
		Model: testModel{stream: func(request model.Request) (model.Result, error) {
			steps++
			if steps == 1 {
				return model.Result{Message: model.Message{Role: "assistant", Calls: []model.Call{{ID: "large", Name: "Large"}}}}, nil
			}
			last := request.History[len(request.History)-1]
			if !strings.HasSuffix(last.Text, contextText) || strings.Count(last.Text, contextText) != 1 || last.OutputFile != path {
				t.Fatal("tool output budget shortened or duplicated lifecycle context")
			}
			return model.Result{Message: model.Message{Role: "assistant", Text: "done"}}, nil
		}},
		Tools: []Tool{largeOutputTool{text: strings.Repeat("log line\n", 5000), context: contextText}},
		StoreToolOutput: func(_ context.Context, text string) (string, error) {
			return path, os.WriteFile(path, []byte(text), 0600)
		},
		Record: func(model.Message) error { return nil }, Emit: func(Event) error { return nil }, Pending: func(bool) []model.Message { return nil },
	})
	if err != nil || steps != 2 {
		t.Fatalf("tool context round trip failed: %v steps=%d", err, steps)
	}
}

type largeOutputTool struct{ text, context string }

func (largeOutputTool) Definition() model.Tool { return model.Tool{Name: "Large"} }
func (largeOutputTool) Concurrent() bool       { return false }
func (t largeOutputTool) Execute(context.Context, string, json.RawMessage) (model.ToolOutput, error) {
	return model.ToolOutput{Text: t.text + t.context, HookContext: t.context}, nil
}
