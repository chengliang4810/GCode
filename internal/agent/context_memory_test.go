package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gcode/internal/model"
)

func contextMemoryHistory() []model.Message {
	history := []model.Message{
		{Role: "user", Text: "Keep the user's constraints.", UserInput: true},
		{Role: "assistant", Calls: []model.Call{{ID: "read", Name: "Read", Arguments: `{"path":"原文.go"}`}}},
		{Role: "tool", CallID: "read", Text: "精确错误：TOKEN_EXPIRED\n" + strings.Repeat("原始日志\n", 300)},
	}
	for range 22 {
		history = append(history, model.Message{Role: "assistant", Text: strings.Repeat("completed work ", 100)})
	}
	history = append(history, model.Message{Role: "user", Text: "Latest requirement.", UserInput: true})
	AssignContextRefs(history)
	return history
}

func TestContextMemoryFoldProtectsInputsAndToolPairs(t *testing.T) {
	history := contextMemoryHistory()
	for _, test := range []struct {
		name       string
		start, end int
	}{
		{"first input", 0, 3}, {"tool call split", 1, 2}, {"orphan tool result", 2, 4},
		{"recent tail", 1, len(history) - 1}, {"reversed", 3, 1}, {"unknown", -1, 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, state, err := foldContext(history, ContextMemory{}, test.start, test.end, "summary"); err == nil || len(state.Blocks) != 0 {
				t.Fatal("unsafe fold accepted", err)
			}
		})
	}
	before := model.CloneMessages(history)
	view, state, err := foldContext(history, ContextMemory{}, 1, 3, "Read found TOKEN_EXPIRED in 原文.go; no repair yet.")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(history, before) || !reflect.DeepEqual(view[2:], history[3:]) || !reflect.DeepEqual(view[0], history[0]) {
		t.Fatal("fold rewrote protected content")
	}
	if !reflect.DeepEqual(state.Blocks[0].Originals, history[1:3]) || !view[1].Summary {
		t.Fatal("original tool exchange was not retained")
	}
	protected := model.CloneMessages(history)
	protected[4].UserInput = true
	protected[len(protected)-1].UserInput = false
	if _, _, err := foldContext(protected, ContextMemory{}, 1, 5, "summary"); err == nil {
		t.Fatal("latest user input was folded")
	}
}

func TestContextMemoryArchiveRetrievalAndHierarchy(t *testing.T) {
	history := contextMemoryHistory()
	state := ContextMemory{}
	for i := range 4 {
		start, end := i+1, i+3
		var err error
		history, state, err = foldContext(history, state, start, end, "Completed stage with preserved evidence and pending work.")
		if err != nil {
			t.Fatal(err)
		}
	}
	originalRef := state.Blocks[0].Originals[1].ContextRef
	backend := testModel{stream: func(request model.Request) (model.Result, error) {
		return model.Result{Message: model.Message{Role: "assistant", Text: "Four completed stages; TOKEN_EXPIRED remains relevant."}}, nil
	}}
	merged, next, err := CompactContext(t.Context(), backend, model.Request{History: history}, state)
	if err != nil {
		t.Fatal(err)
	}
	if next.Blocks[len(next.Blocks)-1].Tier != 2 || len(next.Blocks) != 5 {
		t.Fatal("summary hierarchy was not retained")
	}
	request := model.Request{History: merged}
	tools := contextMemoryTools(&request, &next, func(view []model.Message, state ContextMemory) error {
		request.History, next = view, state
		return nil
	})
	search := tools[2]
	out, err := search.Execute(t.Context(), "", json.RawMessage(`{"query":"TOKEN_EXPIRED"}`))
	if err != nil || !strings.Contains(out.Text, originalRef) {
		t.Fatal("original not searchable", out, err)
	}
	raw, _ := json.Marshal(map[string]any{"ref": originalRef, "limit": 18})
	out, err = tools[3].Execute(t.Context(), "", raw)
	if err != nil || !strings.Contains(out.Text, "精确错误：TOKEN_EXPIRED") || strings.Contains(out.Text, strings.Repeat("原始日志", 10)) {
		t.Fatal("exact Unicode page retrieval failed", out, err)
	}
	raw, _ = json.Marshal(map[string]any{"ref": merged[1].ContextRef})
	out, err = tools[3].Execute(t.Context(), "", raw)
	if err != nil || !strings.Contains(out.Text, state.Blocks[0].ID) {
		t.Fatal("parent lost child references", out, err)
	}
	cloned := CloneContextMemory(next)
	cloned.Blocks[0].Originals[0].Calls[0].Arguments = "changed"
	if next.Blocks[0].Originals[0].Calls[0].Arguments == "changed" {
		t.Fatal("worker archive aliases original")
	}
	thirdView, third, err := foldContext(merged, next, 1, 3, "Condensed stages and subsequent completed work.")
	if err != nil || third.Blocks[len(third.Blocks)-1].Tier != 3 || thirdView[1].ContextRef == merged[1].ContextRef {
		t.Fatal("third-level merge did not retain a new reference", err)
	}
	side := ContextMemoryForView(third, []model.Message{merged[1]})
	if len(side.Blocks) != 5 {
		t.Fatal("side chat did not retain exactly its reachable block tree", len(side.Blocks))
	}
}

func TestContextMemoryToolFailedCommitDoesNotApply(t *testing.T) {
	request := model.Request{History: contextMemoryHistory()}
	state := ContextMemory{}
	before := model.CloneMessages(request.History)
	tools := contextMemoryTools(&request, &state, func([]model.Message, ContextMemory) error { return errors.New("disk full") })
	raw, _ := json.Marshal(map[string]string{"start_ref": request.History[1].ContextRef, "end_ref": request.History[2].ContextRef, "summary": "Read failed with TOKEN_EXPIRED."})
	if _, err := tools[1].Execute(t.Context(), "", raw); err == nil {
		t.Fatal("commit failure hidden")
	}
	if !reflect.DeepEqual(before, request.History) || len(state.Blocks) != 0 {
		t.Fatal("failed compression applied")
	}
	for _, args := range []string{`null`, `{} {}`, `{"unknown":true}`} {
		if _, err := tools[0].Execute(t.Context(), "", json.RawMessage(args)); err == nil {
			t.Fatal("invalid arguments accepted", args)
		}
	}
	raw, _ = json.Marshal(map[string]string{"ref": request.History[0].ContextRef})
	if out, err := tools[3].Execute(t.Context(), "", raw); err != nil || !strings.Contains(out.Text, request.History[0].Text) {
		t.Fatal("visible search references cannot be read", out, err)
	}
}

func TestRunContextMemoryIsOptInAndDurable(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "off", true: "on"}[enabled], func(t *testing.T) {
			var state *ContextMemory
			if enabled {
				state = &ContextMemory{}
			}
			commits, steps := 0, 0
			history := contextMemoryHistory()
			backend := testModel{stream: func(request model.Request) (model.Result, error) {
				steps++
				hasStatus := false
				for _, tool := range request.Tools {
					hasStatus = hasStatus || tool.Name == "ContextStatus"
				}
				if hasStatus != enabled || strings.Contains(request.System, "Session context management") != enabled {
					t.Fatal("opt-in tools/instructions incorrect")
				}
				if enabled && steps == 1 {
					raw, _ := json.Marshal(map[string]string{"start_ref": request.History[1].ContextRef, "end_ref": request.History[2].ContextRef, "summary": "Read reported TOKEN_EXPIRED; still unresolved."})
					return model.Result{Message: model.Message{Role: "assistant", Calls: []model.Call{{ID: "fold", Name: "CompressContext", Arguments: string(raw)}}}}, nil
				}
				if enabled && (!request.History[1].Summary || len(state.Blocks) != 1 || request.History[len(request.History)-1].CallID != "fold") {
					t.Fatal("folded view or tool result missing")
				}
				return model.Result{Message: model.Message{Role: "assistant", Text: "done"}}, nil
			}}
			err := Run(t.Context(), Options{Model: backend, Request: model.Request{History: history}, Memory: state,
				ReplaceMemory:  func([]model.Message, ContextMemory) error { commits++; return nil },
				ReplaceContext: func([]model.Message) error { t.Fatal("unexpected ordinary compression"); return nil },
				Record:         func(model.Message) error { return nil }, Emit: func(Event) error { return nil }, Pending: func(bool) []model.Message { return nil },
			})
			if err != nil {
				t.Fatal(err)
			}
			if enabled && commits != 2 || !enabled && commits != 0 {
				t.Fatal("unexpected archive commits", commits)
			}
		})
	}
}

func TestCompactContextRejectsInvalidSummaryWithoutLosingArchive(t *testing.T) {
	for _, test := range []struct {
		name   string
		result model.Message
		err    error
	}{
		{"empty", model.Message{Text: "  "}, nil},
		{"too large", model.Message{Text: strings.Repeat("x", 8001)}, nil},
		{"tool call", model.Message{Text: "summary", Calls: []model.Call{{Name: "Write"}}}, nil},
		{"upstream failure", model.Message{}, errors.New("upstream failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			history := contextMemoryHistory()
			before := model.CloneMessages(history)
			_, state, err := CompactContext(context.Background(), testModel{stream: func(model.Request) (model.Result, error) { return model.Result{Message: test.result}, test.err }}, model.Request{History: history}, ContextMemory{})
			if err == nil || len(state.Blocks) != 0 || !reflect.DeepEqual(history, before) {
				t.Fatal("invalid summary changed context", err)
			}
		})
	}
}

func TestRunContextMemoryAutomaticallyFoldsBeforeBudgetOverflow(t *testing.T) {
	history := []model.Message{{Role: "user", Text: "Keep the first task verbatim.", UserInput: true}}
	for i := range 60 {
		history = append(history, model.Message{Role: "assistant", Text: fmt.Sprintf("stage %d\n", i) + strings.Repeat("work ", 800)})
	}
	history = append(history, model.Message{Role: "user", Text: "Keep the latest requirement.", UserInput: true})
	state := ContextMemory{}
	summaries, normal := 0, 0
	limits := ContextLimits{Window: 20000, MaxOutput: 1000}
	err := Run(t.Context(), Options{
		Memory: &state, Request: model.Request{History: history}, ContextLimits: func() ContextLimits { return limits },
		ReplaceMemory: func([]model.Message, ContextMemory) error { return nil },
		Record:        func(model.Message) error { return nil }, Emit: func(Event) error { return nil }, Pending: func(bool) []model.Message { return nil },
		Model: testModel{stream: func(request model.Request) (model.Result, error) {
			if request.ContextChange == "summary" {
				summaries++
				return model.Result{Message: model.Message{Role: "assistant", Text: "Completed stages with original evidence available."}}, nil
			}
			normal++
			if contextTokens(request) >= compactThreshold(limits) {
				t.Fatal("normal request exceeded reserved budget")
			}
			if request.History[0].Text != history[0].Text || request.History[len(request.History)-1].Text != history[len(history)-1].Text {
				t.Fatal("protected instructions changed")
			}
			return model.Result{Message: model.Message{Role: "assistant", Text: "done"}}, nil
		}},
	})
	if err != nil || summaries < 2 || normal != 1 || len(state.Blocks) != summaries {
		t.Fatal("budget folding failed", summaries, normal, len(state.Blocks), err)
	}
}

func TestRunContextMemoryGrowthNudge(t *testing.T) {
	state := ContextMemory{}
	history := []model.Message{{Role: "user", Text: "task", UserInput: true}}
	for range 12 {
		history = append(history, model.Message{Role: "assistant", Text: strings.Repeat("old work ", 1200)})
	}
	err := Run(t.Context(), Options{
		Memory: &state, Request: model.Request{History: history},
		ReplaceMemory: func([]model.Message, ContextMemory) error { return nil },
		Record:        func(model.Message) error { return nil }, Emit: func(Event) error { return nil }, Pending: func(bool) []model.Message { return nil },
		Model: testModel{stream: func(request model.Request) (model.Result, error) {
			if !strings.Contains(request.History[len(request.History)-1].Text, "about 50K tokens") {
				t.Fatal("growth reminder missing")
			}
			return model.Result{Message: model.Message{Role: "assistant", Text: "done"}}, nil
		}},
	})
	if err != nil || state.NextNudge < 100000 {
		t.Fatal("growth watermark not persisted", state.NextNudge, err)
	}
}

func TestWithoutContextMemoryRemovesSavedModePrompt(t *testing.T) {
	request := model.Request{System: "original" + contextMemoryInstructions, Tools: []model.Tool{
		{Name: "Read"}, {Name: "ContextStatus"}, {Name: "CompressContext"}, {Name: "SearchContext"}, {Name: "DecompressContext"},
	}}
	got := WithoutContextMemory(request)
	if got.System != "original" || len(got.Tools) != 1 || got.Tools[0].Name != "Read" || len(request.Tools) != 5 {
		t.Fatal("disabled manual compaction retained mode tools or changed the saved prompt", got)
	}
}
