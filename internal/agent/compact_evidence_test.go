package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gcode/internal/model"
)

func TestLightCompactOnlyChangesRecoverableOldOutput(t *testing.T) {
	old := model.Message{Role: "tool", CallID: "old", OutputFile: "/saved/old.txt",
		Text: "Complete output saved to: /saved/old.txt\n" + strings.Repeat("old output ", 400)}
	h := []model.Message{
		{Role: "user", Text: "previous task", UserInput: true},
		{Role: "assistant", Calls: []model.Call{{ID: "old", Name: "Bash"}}},
		old,
		{Role: "user", Text: "current task", UserInput: true},
	}
	for range 8 {
		h = append(h, model.Message{Role: "assistant", Text: "current evidence"})
	}
	req := model.Request{History: h}
	if got := planLightCompact(req, func(int) bool { return false }); got != nil || h[2].Text != old.Text {
		t.Fatal("insufficient plan modified the original")
	}
	got := planLightCompact(req, func(tokens int) bool { return tokens < contextTokens(req)-1500 })
	if len(got) != len(h) || got[2].CallID != old.CallID || !strings.Contains(got[2].Text, old.OutputFile) ||
		h[2].Text != old.Text || !reflect.DeepEqual(got[3:], h[3:]) {
		t.Fatal("light compaction broke the tool pair, output reference or current task")
	}
}

func TestRunLightCompactSkipsSummaryInBothModes(t *testing.T) {
	for _, mode := range []string{"ordinary", "layered"} {
		t.Run(mode, func(t *testing.T) {
			old := model.Message{Role: "tool", CallID: "old", OutputFile: "/saved/result",
				Text: "Complete output saved to: /saved/result\n" + strings.Repeat("archived output ", 1_000)}
			history := []model.Message{
				{Role: "user", Text: "old task", UserInput: true},
				{Role: "assistant", Calls: []model.Call{{ID: "old", Name: "Bash"}}},
				old, {Role: "user", Text: "new task", UserInput: true},
			}
			for range 8 {
				history = append(history, model.Message{Role: "assistant", Text: "current task evidence"})
			}
			writes, requests := 0, 0
			save := func(view []model.Message) error {
				if mode == "layered" && len(view) == len(history) && view[2].Text == old.Text {
					return nil // Initial reference assignment is not a compaction.
				}
				writes++
				if len(view) != len(history) || view[2].Text == old.Text || !strings.Contains(view[2].Text, old.OutputFile) {
					t.Fatal("saved an unexpected context projection")
				}
				return nil
			}
			opts := Options{
				Request: model.Request{History: history}, ContextLimits: func() ContextLimits { return ContextLimits{Window: 7_000} },
				Record: func(model.Message) error { return nil }, Emit: func(Event) error { return nil },
				Pending: func(bool) []model.Message { return nil }, ReplaceContext: save,
				ReplaceMemory: func(h []model.Message, _ ContextMemory) error { return save(h) },
				Model: testModel{stream: func(req model.Request) (model.Result, error) {
					requests++
					if req.ContextChange != "compacted" || writes != 1 || req.History[2].Text == old.Text {
						t.Fatal("expected one saved light projection before the normal request")
					}
					return model.Result{Message: model.Message{Role: "assistant", Text: "Done."}}, nil
				}},
			}
			if mode == "layered" {
				opts.Memory = &ContextMemory{}
			}
			if err := Run(t.Context(), opts); err != nil || writes != 1 || requests != 1 {
				t.Fatalf("light compaction: writes=%d requests=%d err=%v", writes, requests, err)
			}
		})
	}
}

// The opt-in replay reads archived originals but never runs their tools.
func TestCompactionRetainsDecisionsFromRecordedConversation(t *testing.T) {
	dir := os.Getenv("GCODE_TEST_CONTEXT_REPLAY_DIR")
	if dir == "" {
		t.Skip("no local replay directory")
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("expected one session snapshot: %v", err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct{ ContextMemory ContextMemory }
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	const decision = "发版本等我用过以后再决定"
	var source []model.Message
	for _, block := range snapshot.ContextMemory.Blocks {
		for _, msg := range block.Originals {
			if msg.UserInput && strings.Contains(msg.Text, decision) {
				source = block.Originals
				break
			}
		}
	}
	if len(source) == 0 {
		t.Fatal("release decision missing from the original record")
	}
	for _, mode := range []string{"ordinary", "layered"} {
		t.Run(mode, func(t *testing.T) {
			view := []model.Message{{Role: "user", Text: "Initial task."}}
			view = append(view, model.CloneMessages(source)...)
			// Preserve an actual safe historical tool boundary before the new task.
			for len(view) > 1 && len(view[len(view)-1].Calls) > 0 {
				view = view[:len(view)-1]
			}
			backend := testModel{stream: func(req model.Request) (model.Result, error) {
				if len(req.Tools) != 0 || req.ContextChange != "summary" {
					t.Fatal("invalid summary request")
				}
				return model.Result{Message: model.Message{Role: "assistant", Text: "Continuation summary omitting details."}}, nil
			}}
			var memory ContextMemory
			for round := range 2 {
				for range 9 {
					view = append(view, model.Message{Role: "assistant", Text: strings.Repeat("work ", 1_000)})
				}
				view = append(view, model.Message{Role: "user", Text: "功能已经写完了，提交吧", UserInput: true})
				req := model.Request{History: view}
				if mode == "layered" {
					view, memory, err = compactContextSegment(t.Context(), backend, req, memory, 1_000_000, true)
				} else {
					view, err = Compact(t.Context(), backend, req)
				}
				if err != nil {
					t.Fatalf("round %d: %v", round+1, err)
				}
				var summarized bool
				for _, msg := range view {
					summarized = summarized || msg.Summary && strings.Contains(msg.Text, decision) && len(msg.RetainedEvidence) > 0
				}
				if !summarized || view[len(view)-1].Text != "功能已经写完了，提交吧" {
					t.Fatalf("round %d lost release boundary, evidence or current input", round+1)
				}
			}
		})
	}
}
