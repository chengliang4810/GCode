package agent

import (
	"encoding/json"
	"testing"

	"gcode/internal/model"
)

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
