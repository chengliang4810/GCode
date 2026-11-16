package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"gcode/internal/model"
)

func TestContextFallbackAccountsForMultibyteText(t *testing.T) {
	ascii := contextTokens(model.Request{System: "abcd"})
	chinese := contextTokens(model.Request{System: "中文中文"})
	if chinese <= ascii || chinese != 6 {
		t.Fatalf("UTF-8 fallback lost multibyte cost: ascii=%d chinese=%d", ascii, chinese)
	}
}

func TestAutomaticCompactionDoesNotSendAnOversizedProtectedTail(t *testing.T) {
	history := []model.Message{{Role: "user", Text: "first task", UserInput: true}}
	for range 12 {
		history = append(history, model.Message{Role: "assistant", Text: "previous work"})
	}
	history = append(history, model.Message{Role: "user", Text: strings.Repeat("中文", 5000), UserInput: true})
	summaries := 0
	err := Run(t.Context(), Options{
		Request: model.Request{History: history},
		Model: testModel{stream: func(request model.Request) (model.Result, error) {
			if request.ContextChange != "summary" {
				t.Fatal("sent a normal model request with a known oversized tail")
			}
			summaries++
			return model.Result{Message: model.Message{Role: "assistant", Text: "short summary"}}, nil
		}},
		ContextLimits:  func() ContextLimits { return ContextLimits{Window: 2000, MaxOutput: 100} },
		ReplaceContext: func([]model.Message) error { t.Fatal("failed compaction replaced context"); return nil },
		Record:         func(model.Message) error { return nil },
		Emit:           func(Event) error { return nil },
		Pending:        func(bool) []model.Message { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "model budget") || summaries != 1 {
		t.Fatalf("oversized protected tail: summaries=%d err=%v", summaries, err)
	}
}

func TestContextPressureUsesReportedInputIncludingCacheOnlyOnce(t *testing.T) {
	request := model.Request{System: "system", Effort: "low", History: []model.Message{
		{Role: "user", Text: "short"},
		{Role: "assistant", Text: "answer"},
		{Role: "tool", Text: "new result"},
	}}
	request.History[1].ContextUsage = &model.ContextUsage{
		Known: true, InputTokens: 80_000, OutputTokens: 500, HistoryLength: 1, ModelKey: "model", PromptHash: promptHash(request),
	}
	data, err := json.Marshal(request.History)
	if err != nil {
		t.Fatal(err)
	}
	var restored []model.Message
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	request.History = restored
	limits := ContextLimits{Window: 100_000, MaxOutput: 16_000, ModelKey: "model"}
	want := 80_500 + estimateMessages(request.History[2:])
	if got := contextPressure(request, limits); got != want || got <= compactThreshold(limits) {
		t.Fatalf("reported usage did not govern pressure: got=%d want=%d threshold=%d", got, want, compactThreshold(limits))
	}
	for _, name := range []string{"unknown usage", "model changed", "prompt changed", "position changed"} {
		t.Run(name, func(t *testing.T) {
			copy := request
			copy.History = model.CloneMessages(request.History)
			switch name {
			case "unknown usage":
				copy.History[1].ContextUsage.Known = false
			case "model changed":
				copy.History[1].ContextUsage.ModelKey = "other"
			case "prompt changed":
				copy.System += "changed"
			case "position changed":
				copy.History[1].ContextUsage.HistoryLength++
			}
			if got := contextPressure(copy, limits); got != contextTokens(copy) {
				t.Fatalf("invalid usage anchor reused: %d", got)
			}
		})
	}
}

func TestCompactionBaselineResetsOnlyUntilNewResponse(t *testing.T) {
	history := []model.Message{{Role: "user", Summary: true}, {Role: "assistant", ContextUsage: &model.ContextUsage{HistoryLength: 9}}}
	if !needsCompactionBaseline(history) {
		t.Fatal("retained pre-compact usage hid the expected reset")
	}
	history = append(history, model.Message{Role: "assistant", ContextUsage: &model.ContextUsage{HistoryLength: 2}})
	if needsCompactionBaseline(history) {
		t.Fatal("baseline kept resetting after a new response")
	}
}
