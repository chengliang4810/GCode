package engine

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gcode/internal/model"
)

func TestManualCompactionPersistenceFailureRestoresContextInBothModes(t *testing.T) {
	for _, hierarchical := range []bool{false, true} {
		t.Run(map[bool]string{false: "standard", true: "hierarchical"}[hierarchical], func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := subagentRequest(t, r)
				encoded, _ := json.Marshal(body)
				raw := string(encoded)
				if strings.Contains(raw, "Summarize this coding conversation") || strings.Contains(raw, "Host context-fold scope ends here") {
					close(started)
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
					goalResponse(w, model.Chat, "Stored evidence summary.", nil, true)
					return
				}
				goalResponse(w, model.Chat, "done", nil, true)
			}))
			sendGoal(t, core, "Initial task")
			state := waitSubagentState(t, core, runsFinished)
			sessionID := state.Sessions[0].ID
			unpin := pinSessionView(t, core, sessionID)
			defer unpin()
			core.mu.Lock()
			core.state.Settings.General.HierarchicalContext = hierarchical
			for range 12 {
				message := model.Message{Role: "assistant", Text: "Original full context and evidence."}
				core.history[sessionID] = append(core.history[sessionID], message)
				core.context[sessionID] = append(core.context[sessionID], message)
			}
			before := model.CloneMessages(core.context[sessionID])
			err := core.compactLocked(sessionID, "")
			core.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(10 * time.Second):
				t.Fatal("summary did not start")
			}
			indexPath := filepath.Join(core.storage.ConfigDirectory(), "engine.json")
			// Hold the transaction gate while replacing the index: background
			// project loading must not recreate it between Remove and Mkdir.
			func() {
				core.mu.Lock()
				defer core.mu.Unlock()
				if err := os.Remove(indexPath); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(indexPath, 0700); err != nil {
					t.Fatal(err)
				}
			}()
			defer os.Remove(indexPath)
			close(release)
			state = waitSubagentState(t, core, runsFinished, sessionID)
			core.mu.Lock()
			after := model.CloneMessages(core.context[sessionID])
			core.mu.Unlock()
			if !reflect.DeepEqual(before, after) {
				t.Fatal("failed durable commit replaced the execution context")
			}
			last := state.Runs[len(state.Runs)-1]
			if last.Status != "failed" {
				t.Fatalf("failed persistence reported success: %+v", last)
			}
			for _, message := range state.Messages[sessionID] {
				if message.TurnID == last.ID && strings.Contains(message.Blocks[0].Text, "Context compacted.") {
					t.Fatal("failed compaction retained a success message")
				}
			}
		})
	}
}
