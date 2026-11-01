package mock

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func TestLegacyProfileHistoryAndEditsStayIsolatedFromFreshReviewScenario(t *testing.T) {
	ctx := context.Background()
	clock := func() time.Time { return time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC) }
	legacy := seed(Options{Scenario: "default", Platform: "windows", Architecture: "amd64", Now: clock})
	legacyReply := "I'll keep the implementation consistent with the existing workspace.\n\nThe layout is organized around three independent regions: sidebar, conversation, and workspace."
	legacy.Messages["chat-0"] = []contracts.Message{
		{ID: "chat-0-user", SessionID: "chat-0", TurnID: "chat-0-turn", Role: "user", CreatedAt: clock().Add(-time.Hour), Blocks: []contracts.Block{{ID: "chat-0-prompt", Kind: "text", Text: "Build a calm workspace for Fieldnotes."}}},
		{ID: "chat-0-assistant", SessionID: "chat-0", TurnID: "chat-0-turn", Role: "assistant", CreatedAt: clock().Add(-time.Hour + time.Second), Blocks: []contracts.Block{{ID: "block-0-text", Kind: "text", Text: legacyReply}}},
		{ID: "user-added-prompt", SessionID: "chat-0", TurnID: "user-added-turn", Role: "user", CreatedAt: clock(), Blocks: []contracts.Block{{ID: "user-added-text", Kind: "text", Text: "Keep my later request and its answer."}}},
		{ID: "user-added-answer", SessionID: "chat-0", TurnID: "user-added-turn", Role: "assistant", CreatedAt: clock().Add(time.Second), Blocks: []contracts.Block{{ID: "user-added-response", Kind: "text", Text: "This is stored user history."}}},
	}
	legacy.Runs = nil
	legacy.Sessions[0].Title = "My renamed conversation"
	legacy.Settings.Appearance.Mode = "light"
	legacy.Settings.Appearance.FontSize = 18
	legacy.Settings.SidebarGrouping = "timeline"
	legacy.Settings.ModelPicker.Selections = map[string]contracts.ModelSelection{"openai": {ModelID: "gpt-5.4", Reasoning: "Extra high"}}
	for i := range legacy.Files {
		if legacy.Files[i].ID == "file-layout" {
			legacy.Files[i].Content = "user's saved file edit\n"
			legacy.Files[i].Version = 7
			legacy.Files[i].Modified = true
		}
	}
	statePath := filepath.Join(t.TempDir(), "normal-profile", "state.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(persisted{Snapshot: legacy, Next: 4000})
	if err != nil {
		t.Fatal(err)
	}
	// Old schema-1 files have no catalog load-state field.
	var envelope map[string]any
	if err = json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	delete(envelope["snapshot"].(map[string]any), "modelCatalog")
	data, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(statePath, data, 0600); err != nil {
		t.Fatal(err)
	}
	normal := newTestServer(t, Options{StatePath: statePath, Now: clock})
	loaded, _ := normal.Snapshot(ctx)
	if loaded.Sessions[0].Title != "My renamed conversation" || len(loaded.Messages["chat-0"]) != 4 || loaded.Messages["chat-0"][1].Blocks[0].Text != legacyReply || loaded.Messages["chat-0"][3].Blocks[0].Text != "This is stored user history." {
		t.Fatal("old fixture identification rewrote or removed user history")
	}
	if loaded.Settings.Appearance.Mode != "light" || loaded.Settings.Appearance.FontSize != 18 || loaded.Settings.SidebarGrouping != "timeline" || loaded.Settings.ModelPicker.Selections["openai"].ModelID != "gpt-5.4" {
		t.Fatal("opening old profile replaced saved preferences")
	}
	for _, file := range loaded.Files {
		if file.ID == "file-layout" && (file.Content != "user's saved file edit\n" || file.Version != 7) {
			t.Fatal("opening old profile replaced a saved file edit")
		}
	}
	if err = normal.Close(); err != nil {
		t.Fatal(err)
	}
	beforeReview, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	review := newTestServer(t, Options{Scenario: "default", Now: clock})
	fresh, _ := review.Snapshot(ctx)
	if fresh.Messages["chat-0"][1].Blocks[0].Kind != "reasoning" || fresh.Messages["chat-0"][len(fresh.Messages["chat-0"])-1].Blocks[0].Text == legacyReply {
		t.Fatal("fresh review scenario inherited stale persisted fixture text")
	}
	if err = review.Close(); err != nil {
		t.Fatal(err)
	}
	afterReview, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeReview, afterReview) {
		t.Fatal("memory-only review scenario modified the normal profile")
	}
}
