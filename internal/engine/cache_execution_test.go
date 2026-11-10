package engine

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestCacheStableToolOutputAndManualSummaryAcrossProtocols(t *testing.T) {
	for _, protocol := range []model.Protocol{model.Chat, model.Responses, model.Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			var mu sync.Mutex
			var bodies []map[string]any
			core, workspace := goalTestCore(t, protocol, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := subagentRequest(t, r)
				mu.Lock()
				bodies = append(bodies, body)
				step := len(bodies)
				mu.Unlock()
				raw, _ := json.Marshal(body)
				if strings.Contains(string(raw), "Summarize this coding conversation") {
					goalResponse(w, protocol, "Preserved task summary.", nil, true)
					return
				}
				if step <= 2 {
					id := "first-read"
					if step == 2 {
						id = "second-read"
					}
					goalResponse(w, protocol, "", &model.Call{ID: id, Name: "Read", Arguments: `{"file_path":"large.txt","limit":2000}`}, true)
					return
				}
				goalResponse(w, protocol, "Done.", nil, true)
			}))
			if err := os.WriteFile(filepath.Join(workspace, "large.txt"), []byte(strings.Repeat("important source data retained in full\n", 1800)), 0600); err != nil {
				t.Fatal(err)
			}
			sendGoal(t, core, "Read the same large file twice")
			state := waitSubagentState(t, core, runsFinished)
			session := state.Sessions[0]
			release := pinSessionView(t, core, session.ID)
			defer release()
			core.mu.Lock()
			history := model.CloneMessages(core.context[session.ID])
			prompt := model.ClonePrompt(core.prompts[session.ID])
			core.mu.Unlock()
			var outputs []model.Message
			for _, message := range history {
				if message.Role == "tool" {
					outputs = append(outputs, message)
				}
			}
			if len(outputs) != 2 || outputs[0].OutputFile == "" || outputs[1].ContentHash != "" ||
				!strings.Contains(outputs[1].Text, "first-read") {
				t.Fatalf("tool results were not frozen/deduplicated: %+v", outputs)
			}
			if content, err := os.ReadFile(outputs[0].OutputFile); err != nil || !strings.Contains(string(content), "important source data") || len(content) <= len(outputs[0].Text) {
				t.Fatal("complete tool result is not readable")
			}
			if prompt.System == "" || len(prompt.Tools) == 0 {
				t.Fatal("manual compaction has no durable request prefix")
			}
			if err := core.Execute(t.Context(), contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{SessionID: session.ID, Text: "/compact"}}); err != nil {
				t.Fatal(err)
			}
			waitSubagentState(t, core, runsFinished)
			mu.Lock()
			defer mu.Unlock()
			if len(bodies) != 4 {
				t.Fatalf("unexpected model calls: %d", len(bodies))
			}
			before, summary := bodies[2], bodies[3]
			for _, field := range []string{"model", "tools", "system", "instructions", "reasoning", "reasoning_effort", "thinking", "output_config"} {
				if !reflect.DeepEqual(before[field], summary[field]) {
					t.Fatalf("summary changed cache-critical %s", field)
				}
			}
			trajectory, err := core.ReadModelTrajectory(t.Context(), session.ID)
			if err != nil || len(trajectory.Records) != 4 {
				t.Fatalf("wire diagnostics missing: %v records=%d", err, len(trajectory.Records))
			}
			for _, record := range trajectory.Records {
				if record.Request.Bytes == 0 || record.Request.SHA256 == "" || record.Transport.ResponseHeadersMS == nil {
					t.Fatal("actual request lacked fingerprint or transport timing")
				}
			}
		})
	}
}
