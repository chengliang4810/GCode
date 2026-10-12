package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/agent"
	"gcode/internal/appmodel"
	"gcode/internal/sessions"
	"gcode/internal/streamjson"
)

func TestRuntimeChildMonitorUsesOneIndependentRunID(t *testing.T) {
	dataDir := t.TempDir()
	workspace := t.TempDir()
	writeRuntimeFile(t, filepath.Join(dataDir, "specialists", "run-identity-child.md"), `---
name: run-identity-child
description: Run identity child fixture
tools:
  - "read-only"
---

Complete the assigned child work.
`)
	provider := newRuntimeScriptedProvider()
	engine := newRuntimeTestEngine(t, dataDir, provider)
	const parentSessionID = "run-identity-parent"
	if _, err := engine.SessionStore().Create(sessions.CreateInput{
		SessionID: parentSessionID,
		Cwd:       workspace,
		ModelID:   "scripted",
		Provider:  "scripted",
	}); err != nil {
		t.Fatal(err)
	}
	promptPath := filepath.Join(t.TempDir(), "child-prompt.md")
	if err := os.WriteFile(promptPath, []byte("# Specialist Invocation\n\nComplete the assigned child work.\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	parent := Input{
		RunID:     "run_parent_identity",
		SessionID: parentSessionID,
		Cwd:       workspace,
		Provider:  appmodel.Provider{ID: "scripted", Model: "scripted"},
	}
	progressEvents := make([]streamjson.Event, 0, 8)
	runChild := engine.childRunner(parent, Callbacks{}, agent.Provider(nil), "")
	result, err := runChild(context.Background(), "", []string{
		"--init-session-id", "run-identity-child",
		"--model", "scripted",
		"--cwd", workspace,
		"--auto", "low",
		"--depth", "1",
		"--session-title", "Run identity child",
		"--file", promptPath,
	}, func(event streamjson.Event) {
		progressEvents = append(progressEvents, event)
	})
	if err != nil {
		t.Fatalf("child run: %v", err)
	}
	if !result.Started || len(result.Events) == 0 || len(progressEvents) != len(result.Events) {
		t.Fatalf("child result = %#v, progress events = %#v", result, progressEvents)
	}

	childRunID := progressEvents[0].RunID
	if childRunID == "" || childRunID == parent.RunID {
		t.Fatalf("child run id = %q, parent run id = %q", childRunID, parent.RunID)
	}
	for index, event := range progressEvents {
		if event.RunID != childRunID {
			t.Fatalf("progress event %d has run id %q, want child id %q: %#v", index, event.RunID, childRunID, event)
		}
	}
	for index, event := range result.Events {
		if event.RunID != childRunID {
			t.Fatalf("returned event %d has run id %q, want child id %q: %#v", index, event.RunID, childRunID, event)
		}
	}

	events, err := engine.SessionStore().ReadRehydratedEvents("run-identity-child")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("child session has no durable events")
	}
	for _, event := range events {
		var payload struct {
			RunID string `json:"runId"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode child %s payload: %v", event.Type, err)
		}
		if payload.RunID != childRunID {
			t.Fatalf("child session %s event run id = %q, want %q", event.Type, payload.RunID, childRunID)
		}
	}
	t.Logf("child run identity evidence: run=%s progressEvents=%d journalEvents=%d", childRunID, len(progressEvents), len(events))
}
