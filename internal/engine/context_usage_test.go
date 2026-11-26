package engine

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// The composer ring reads this snapshot after every model round. Each row is
// one turn with its response for both protocols and the input count the ring
// must show afterwards. A compatible Messages endpoint reports a placeholder
// zero at message_start and the real counters in message_delta.
func TestContextUsageFollowsReportedInput(t *testing.T) {
	reported, explicitZero := uint64(120), uint64(0)
	steps := []struct {
		name     string
		chat     string
		messages string
		tokens   *uint64
	}{
		{
			name: "reported input",
			chat: "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":120,\"completion_tokens\":7}}\n\ndata: [DONE]\n\n",
			messages: `data: {"type":"message_start","message":{"usage":{"input_tokens":0,"output_tokens":0}}}` + "\n\n" +
				`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}` + "\n\n" +
				`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":100,"output_tokens":7,"cache_read_input_tokens":20}}` + "\n\n" +
				"data: {\"type\":\"message_stop\"}\n\n",
			tokens: &reported,
		},
		{
			// Output alone cannot turn an unreported input into a real zero.
			name:     "unreported input",
			chat:     "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}],\"usage\":{\"completion_tokens\":7}}\n\ndata: [DONE]\n\n",
			messages: `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}` + "\n\ndata: {\"type\":\"message_stop\"}\n\n",
		},
		{
			name: "explicit zero",
			chat: "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":0,\"completion_tokens\":0}}\n\ndata: [DONE]\n\n",
			messages: `data: {"type":"message_start","message":{"usage":{"input_tokens":0}}}` + "\n\n" +
				`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":0}}` + "\n\n" +
				"data: {\"type\":\"message_stop\"}\n\n",
			tokens: &explicitZero,
		},
	}
	for _, protocol := range []model.Protocol{model.Chat, model.Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				step := int(calls.Add(1)) - 1
				if step >= len(steps) {
					http.Error(w, "unexpected model request", http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if protocol == model.Chat {
					fmt.Fprint(w, steps[step].chat)
					return
				}
				fmt.Fprint(w, steps[step].messages)
			}))
			defer server.Close()
			store, err := storage.Open(storage.Options{Directory: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			defer core.Close()
			ctx := t.Context()
			if err := core.Execute(ctx, contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
				ID: "P", Name: "Reported", Protocol: protocol, BaseURL: server.URL,
				Models: []model.Model{{ID: "family/model", ContextWindow: 1000000}},
			}}}); err != nil {
				t.Fatal(err)
			}
			if err := core.Execute(ctx, contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: t.TempDir(), Name: "Context"}}); err != nil {
				t.Fatal(err)
			}
			state, _ := core.Snapshot(ctx)
			var sessionID string
			for i, step := range steps {
				if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
					ProjectID: state.Projects[0].ID, SessionID: sessionID, ModelID: "P/family/model", Text: step.name,
				}}); err != nil {
					t.Fatal(err)
				}
				if sessionID == "" {
					summary, err := core.Snapshot(ctx)
					if err != nil || len(summary.Sessions) != 1 {
						t.Fatalf("conversation was not created: %+v, %v", summary.Sessions, err)
					}
					sessionID = summary.Sessions[0].ID
				}
				state = waitIdle(t, core, sessionID)
				if last := state.Runs[len(state.Runs)-1]; last.Status != contracts.RunStatusCompleted {
					t.Fatalf("%s: actual request failed: status %q, error %q", step.name, last.Status, last.Error)
				}
				usage, ok := state.ContextUsage[sessionID]
				if !ok || !usage.HasWindow() || *usage.Window != 1000000 {
					t.Fatalf("%s: context capacity = %+v", step.name, usage)
				}
				if got, want := contextInput(usage.Tokens), contextInput(step.tokens); got != want {
					t.Fatalf("%s: context input = %s; want %s (after %d turns)", step.name, got, want, i+1)
				}
			}
		})
	}
}

func contextInput(value *uint64) string {
	if value == nil {
		return "unknown"
	}
	return fmt.Sprint(*value)
}
