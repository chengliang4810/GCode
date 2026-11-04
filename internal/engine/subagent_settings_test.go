package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestBuiltinSubagentExecutesAfterReopeningWithNoSavedProfiles(t *testing.T) {
	var parentCalls, childCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		isParent := false
		for _, raw := range body["tools"].([]any) {
			function := raw.(map[string]any)["function"].(map[string]any)
			if function["name"] == "Agent" {
				isParent = true
			}
		}
		if isParent {
			if parentCalls.Add(1) == 1 {
				schema, _ := json.Marshal(delegationSchema(t, body))
				for _, id := range []string{"general-purpose", "Explore", "Plan", "claude"} {
					if !strings.Contains(string(schema), `"`+id+`"`) {
						t.Errorf("unsaved built-in %s is missing from the actual tool schema", id)
					}
				}
				subagentCall(w, `{"description":"Investigate files","prompt":"Report the evidence.","subagent_type":"Explore"}`)
				return
			}
			subagentText(w, model.Chat, "Parent received evidence.")
			return
		}
		childCalls.Add(1)
		if body["model"] != "parent-model" {
			t.Error("built-in did not inherit its parent model")
		}
		for _, raw := range body["tools"].([]any) {
			name := raw.(map[string]any)["function"].(map[string]any)["name"].(string)
			if !slices.Contains([]string{"Read", "ListDirectory", "Glob", "Grep"}, name) {
				t.Errorf("Explore exposes unavailable tool %s", name)
			}
		}
		subagentText(w, model.Chat, "Built-in investigation complete.")
	}))
	defer server.Close()
	original, _ := subagentCore(t, server.URL, server.URL, model.Chat)
	state, _ := original.Snapshot(context.Background())
	state.Settings.Subagents = nil
	if err := original.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &state.Settings}); err != nil {
		t.Fatal(err)
	}
	directory := original.storage.Directory()
	if err := original.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(storage.Options{Directory: directory})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	startSubagentParent(t, core)
	state = waitSubagentState(t, core, func(snapshot contracts.Snapshot) bool {
		return len(snapshot.Runs) == 2 && runsFinished(snapshot)
	})
	if parentCalls.Load() != 2 || childCalls.Load() != 1 {
		t.Fatal("reopened host did not execute the built-in subagent and return its result")
	}
	for _, session := range state.Sessions {
		if session.SubagentType == "Explore" && session.ModelID != "parent/parent-model" {
			t.Fatal("built-in child session did not retain its actual inherited model")
		}
	}
}

func TestSubagentSettingsControlActualInstructionsToolsAndAvailability(t *testing.T) {
	var parentCalls, childCalls atomic.Int32
	parent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		encoded, _ := json.Marshal(body)
		if !strings.Contains(string(encoded), "PROJECT_ONLY_INSTRUCTIONS") {
			t.Error("ordinary parent lost its project instructions")
		}
		if parentCalls.Add(1) == 1 {
			schema := delegationSchema(t, body)
			types := schema["properties"].(map[string]any)["subagent_type"].(map[string]any)["enum"].([]any)
			for _, value := range types {
				if value == "disabled-worker" {
					t.Error("disabled subagent was advertised")
				}
			}
			subagentCall(w, `{"description":"Read evidence","prompt":"Read evidence.txt","subagent_type":"worker"}`)
		} else {
			subagentText(w, model.Chat, "Reviewed child evidence.")
		}
	}))
	defer parent.Close()
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		encoded, _ := json.Marshal(body)
		if strings.Contains(string(encoded), "PROJECT_ONLY_INSTRUCTIONS") {
			t.Error("subagent ignored the project-instruction setting")
		}
		tools, _ := body["tools"].([]any)
		var names []string
		for _, raw := range tools {
			function := raw.(map[string]any)["function"].(map[string]any)
			names = append(names, function["name"].(string))
		}
		if !slices.Equal(names, []string{"Read"}) {
			t.Errorf("subagent ignored its tool selection: %v", names)
		}
		if childCalls.Add(1) == 1 {
			subagentWireCall(w, model.Chat, "Read", `{"file_path":"evidence.txt"}`)
		} else {
			if !strings.Contains(string(encoded), "ACTUAL FILE EVIDENCE") {
				t.Error("selected read tool did not execute")
			}
			subagentText(w, model.Chat, "Child verified evidence.")
		}
	}))
	defer child.Close()
	core, workspace := subagentCore(t, parent.URL, child.URL, model.Chat)
	if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte("PROJECT_ONLY_INSTRUCTIONS"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "evidence.txt"), []byte("ACTUAL FILE EVIDENCE"), 0600); err != nil {
		t.Fatal(err)
	}
	state, _ := core.Snapshot(context.Background())
	state.Settings.Subagents = []contracts.SubagentProfile{
		{ID: "disabled-worker", Name: "Disabled", Description: "Unavailable", Disabled: true},
		{
			ID: "worker", Name: "Worker", Description: "Read evidence", Tools: []string{"Read"}, SkipProjectInstructions: true,
			Model: &contracts.ModelSelection{ModelID: "child/child-model", Reasoning: "Low"},
		},
	}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "settings.update", Settings: &state.Settings}); err != nil {
		t.Fatal(err)
	}
	startSubagentParent(t, core)
	waitSubagentState(t, core, runsFinished)
	if parentCalls.Load() != 2 || childCalls.Load() != 2 {
		t.Fatalf("unexpected execution: parent=%d child=%d", parentCalls.Load(), childCalls.Load())
	}
}
