package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/model"
	"gcode/internal/privacy"
	"gcode/third_party/magpie/redact"
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
	if err == nil || !strings.Contains(err.Error(), "model budget") || summaries != 0 {
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
	request.History[1].ContextUsage.HistoryHash = historyHash(request.History[:2])
	request.History[1].ContextUsage.PromptTokens = contextTokens(model.Request{System: request.System})
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
	for _, name := range []string{"unknown usage", "model changed", "prompt changed", "position changed", "same length edit"} {
		t.Run(name, func(t *testing.T) {
			copy := request
			copy.History = model.CloneMessages(request.History)
			switch name {
			case "same length edit":
				copy.History[0].Text = "other"
			case "unknown usage":
				copy.History[1].ContextUsage.Known = false
			case "model changed":
				copy.History[1].ContextUsage.ModelKey = "other"
			case "prompt changed":
				copy.System += "changed"
				if got := contextPressure(copy, limits); got != want+contextTokens(model.Request{System: copy.System})-copy.History[1].ContextUsage.PromptTokens {
					t.Fatalf("changed prompt lost baseline: %d", got)
				}
				return
			case "position changed":
				copy.History[1].ContextUsage.HistoryLength++
			}
			if got := contextPressure(copy, limits); got != contextTokens(copy) {
				t.Fatalf("invalid usage anchor reused: %d", got)
			}
		})
	}
}

// Opt-in replay uses only local files, never a provider or the active store.
func TestContextBudgetOfflineReplay(t *testing.T) {
	dir := os.Getenv("GCODE_TEST_CONTEXT_REPLAY_DIR")
	if dir == "" {
		t.Skip("no local replay directory")
	}
	read := func(path string, v any) {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(b, v); err != nil {
			t.Fatal(err)
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	if len(files) != 1 {
		t.Fatal("expected one session snapshot")
	}
	var session struct {
		Context       []model.Message
		Prompt        model.Prompt
		ContextMemory ContextMemory
	}
	read(files[0], &session)
	root := filepath.Dir(filepath.Dir(dir))
	var cfg struct {
		Snapshot struct {
			Settings struct{ Privacy privacy.Settings }
		}
	}
	read(filepath.Join(root, "engine.json"), &cfg)
	redact.SetKeyPath(filepath.Join(root, "privacy-placeholder.key"))
	h := session.Context
	if os.Getenv("GCODE_TEST_CONTEXT_REPLAY_BEFORE_FOLD") != "" {
		const submission = "功能已经写完了，提交吧"
		for j := len(session.ContextMemory.Blocks); j >= 0; j-- {
			recovered := false
			for k := 1; k < len(h); k++ {
				anchor := h[k-1].ContextUsage
				if h[k].Text == submission && anchor != nil && anchor.HistoryLength == k-1 {
					h = h[:k+1]
					recovered = true
					break
				}
			}
			if recovered {
				break
			}
			if j == 0 {
				t.Fatal("could not reconstruct the pre-compaction prefix")
			}
			block := session.ContextMemory.Blocks[j-1]
			for k, msg := range h {
				if msg.ContextRef == block.ID {
					restored := append(model.CloneMessages(h[:k]), block.Originals...)
					h = append(restored, h[k+1:]...)
					break
				}
			}
		}
	}
	i := len(h) - 1
	for i >= 0 && h[i].ContextUsage == nil {
		i--
	}
	if i < 0 {
		t.Fatal("no usage")
	}
	a := h[i].ContextUsage
	files, _ = filepath.Glob(filepath.Join(dir, "trajectory", "call-*.json"))
	matched := false
	for j := len(files) - 1; j >= 0; j-- {
		var rec model.Invocation
		read(files[j], &rec)
		if rec.ContextChange == "summary" || rec.Response == nil || rec.Response.Usage.Input != a.InputTokens {
			continue
		}
		if rec.Request.MatchesChatHistory(t.Context(), h[:i], cfg.Snapshot.Settings.Privacy) {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatal("history did not match recorded wire request")
	}
	r := model.Request{History: h, System: session.Prompt.System + "\nchanged", Tools: session.Prompt.Tools}
	limits := ContextLimits{ModelKey: "verified-new-usage-scope", Window: 1_000_000, MaxOutput: 64_000}
	var memory ContextMemory
	ordinary := 0
	err := Run(t.Context(), Options{Request: r, Memory: &memory, ContextLimits: func() ContextLimits { return limits },
		VerifyPriorUsage: func([]model.Message, model.ContextUsage) bool { return matched },
		ReplaceMemory:    func([]model.Message, ContextMemory) error { return nil }, Record: func(model.Message) error { return nil },
		Emit: func(Event) error { return nil }, Pending: func(bool) []model.Message { return nil },
		Model: testModel{stream: func(req model.Request) (model.Result, error) {
			n, source := measureContext(req, limits)
			t.Logf("usage=%d raw=%d trigger=%d source=%s", n, contextTokens(req), compactThreshold(limits), source)
			if req.ContextChange == "summary" || n >= compactThreshold(limits) || source != "provider usage plus changes" {
				t.Fatal("unexpected compaction")
			}
			ordinary++
			return model.Result{Message: model.Message{Role: "assistant", Text: "done"}}, nil
		}}})
	if err != nil || ordinary != 1 {
		t.Fatalf("ordinary=%d err=%v", ordinary, err)
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
