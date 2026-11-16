package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestSubagentLargeResultDeliveryAndRestart(t *testing.T) {
	for _, background := range []bool{false, true} {
		t.Run(fmt.Sprintf("background=%t", background), func(t *testing.T) {
			full := "RESULT HEAD\n" + strings.Repeat("完整结果\n", 8000) + "\nRESULT TAIL"
			release := make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			var calls atomic.Int32
			parent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := subagentRequest(t, r)
				w.Header().Set("Content-Type", "text/event-stream")
				switch calls.Add(1) {
				case 1:
					subagentCall(w, fmt.Sprintf(`{"description":"Large result","prompt":"Return findings","subagent_type":"worker","run_in_background":%t}`, background))
				case 2:
					if background {
						subagentText(w, model.Chat, "PARENT WAITING")
						return
					}
					fallthrough
				default:
					encoded, _ := json.Marshal(body)
					text := string(encoded)
					for _, marker := range []string{"RESULT HEAD", "RESULT TAIL", "incomplete preview", "Complete output saved to:", "Read with offset/limit"} {
						if !strings.Contains(text, marker) {
							t.Errorf("parent request missing %q", marker)
						}
					}
					if strings.Contains(text, strings.Repeat("完整结果\\n", 1000)) {
						t.Error("large child result flooded the parent request")
					}
					subagentText(w, model.Chat, "PARENT RECEIVED PREVIEW")
				}
			}))
			defer parent.Close()
			child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if background {
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
				}
				subagentText(w, model.Chat, full)
			}))
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
				child.Close()
			}()
			core, _ := subagentCore(t, parent.URL, child.URL, model.Chat)
			state, _ := core.Snapshot(t.Context())
			state.Settings.Subagents = []contracts.SubagentProfile{{
				ID: "worker", Name: "Worker", Description: "Return findings",
				Model: &contracts.ModelSelection{ModelID: "child/child-model", Reasoning: "Low"},
			}}
			if err := core.Execute(t.Context(), contracts.Command{Kind: "settings.update", Settings: &state.Settings}); err != nil {
				t.Fatal(err)
			}
			startSubagentParent(t, core)
			if background {
				waitSubagentState(t, core, func(s contracts.Snapshot) bool {
					return len(s.Runs) == 2 && s.Runs[0].Status == "completed"
				})
				close(release)
			}
			wantRuns := 2
			if background {
				wantRuns = 3
			}
			state = waitSubagentState(t, core, func(s contracts.Snapshot) bool {
				return len(s.Runs) == wantRuns && runsFinished(s)
			})
			if calls.Load() != int32(wantRuns) {
				t.Fatal("result delivery made extra parent model requests", calls.Load())
			}
			var parentSession, childSession contracts.Session
			for _, session := range state.Sessions {
				if session.SubagentType != "" {
					childSession = session
				} else {
					parentSession = session
				}
			}
			var outputFile string
			for _, run := range state.Runs {
				if run.Status != "completed" {
					t.Fatalf("large result failed the task: %+v", run)
				}
				if run.SessionID == childSession.ID {
					outputFile = run.OutputFile
				}
			}
			if outputFile == "" || !strings.HasPrefix(outputFile, filepath.Join(core.storage.Directory(), "sessions", childSession.ID, "tool-results")) {
				t.Fatal("result was not preserved in the child's own storage root", outputFile)
			}
			check := func(core *Engine) {
				t.Helper()
				tools, err := core.sessionRuntimeTools(parentSession, "")
				if err != nil {
					t.Fatal(err)
				}
				result := callSessionTestTool(t, tools, "TaskOutput", map[string]any{"task_id": childSession.ID, "block": false})
				data := result.Data.(map[string]any)
				if data["output_file"] != outputFile || data["output_truncated"] != true ||
					!strings.Contains(result.Text, "RESULT TAIL") || utf8.RuneCountInString(result.Text) > 5000 {
					t.Fatal("TaskOutput lost the original bounded result reference")
				}
				content, err := os.ReadFile(outputFile)
				if err != nil || string(content) != full {
					t.Fatal("preserved final reply is incomplete", err)
				}
				callSessionTestTool(t, tools, "Read", map[string]any{"file_path": outputFile, "limit": 1})
			}
			check(core)
			anchor := core.storage.ConfigDirectory()
			if err := core.Execute(t.Context(), contracts.Command{Kind: "storage.change", Path: filepath.Join(t.TempDir(), "new-root")}); err != nil {
				t.Fatal(err)
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			store, err := storage.Open(storage.Options{Directory: anchor})
			if err != nil {
				t.Fatal(err)
			}
			reopened, err := openExecutionFixture(t, Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			check(reopened)
		})
	}
}

func TestSubagentOutputBudgetUsesSharedUnicodeBoundary(t *testing.T) {
	for _, size := range []int{32000, 32001} {
		text := strings.Repeat("中", size)
		stored := false
		got, err := agent.PrepareOutput(t.Context(), func(_ context.Context, full string) (string, error) {
			stored = true
			if full != text {
				t.Fatal("storage did not receive the complete Unicode text")
			}
			return "/session/full.txt", nil
		}, model.Message{Text: text})
		if err != nil || stored != (size > 32000) {
			t.Fatal("shared Unicode boundary changed", size, err)
		}
		if !stored && got.Text != text {
			t.Fatal("short result was rewritten")
		}
	}
}

func TestSubagentResultStorageFailureKeepsOriginalReply(t *testing.T) {
	full := strings.Repeat("preserve original findings\n", 2000)
	var calls atomic.Int32
	parent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := subagentRequest(t, r)
		if calls.Add(1) == 1 {
			subagentCall(w, `{"description":"Storage failure","prompt":"Return findings","subagent_type":"worker"}`)
			return
		}
		text, _ := json.Marshal(body)
		if !strings.Contains(string(text), "cannot preserve subagent result") ||
			strings.Contains(string(text), "Complete output saved to:") {
			t.Error("parent received a fabricated output reference or lost the storage error")
		}
		subagentText(w, model.Chat, "PARENT SAW STORAGE ERROR")
	}))
	defer parent.Close()
	var core *Engine
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		core.mu.Lock()
		var childID string
		for _, session := range core.state.Sessions {
			if session.SubagentType != "" {
				childID = session.ID
			}
		}
		root := core.storage.Directory()
		core.mu.Unlock()
		// A regular file blocks the output directory without changing the
		// saved child transcript or touching anything outside the test profile.
		if err := os.WriteFile(filepath.Join(root, "sessions", childID, "tool-results"), []byte("blocked"), 0600); err != nil {
			t.Error(err)
		}
		subagentText(w, model.Chat, full)
	}))
	defer child.Close()
	core, _ = subagentCore(t, parent.URL, child.URL, model.Chat)
	state, _ := core.Snapshot(t.Context())
	state.Settings.Subagents = []contracts.SubagentProfile{{
		ID: "worker", Name: "Worker", Description: "Return findings",
		Model: &contracts.ModelSelection{ModelID: "child/child-model", Reasoning: "Low"},
	}}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "settings.update", Settings: &state.Settings}); err != nil {
		t.Fatal(err)
	}
	startSubagentParent(t, core)
	state = waitSubagentState(t, core, func(s contracts.Snapshot) bool {
		return len(s.Runs) == 2 && runsFinished(s)
	})
	core.mu.Lock()
	defer core.mu.Unlock()
	for _, session := range state.Sessions {
		if session.SubagentType == "" {
			continue
		}
		if err := core.ensureSessionDataLocked(session.ID); err != nil {
			t.Fatal(err)
		}
		if finalReply(core.history[session.ID]) != full {
			t.Fatal("storage failure discarded the original child result")
		}
		if session.LastOutcome != "failed" {
			t.Fatal("storage failure was not reported")
		}
	}
	for _, run := range state.Runs {
		if run.OutputFile != "" {
			t.Fatal("storage failure invented a preserved output file")
		}
	}
	if calls.Load() != 2 {
		t.Fatal("storage failure replayed the child task")
	}
}
