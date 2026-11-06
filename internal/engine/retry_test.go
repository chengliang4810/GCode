package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestModelRetryProjectionPreservesCompletedToolAndClearsOnSettlement(t *testing.T) {
	for _, cancelRun := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%v", cancelRun), func(t *testing.T) {
			var calls, writes, waits atomic.Int32
			retrying, release := make(chan struct{}), make(chan struct{})
			client := model.NewClient()
			policy := model.DefaultRetryPolicy()
			policy.Wait = func(ctx context.Context, _ time.Duration) error {
				if waits.Add(1) == 3 {
					close(retrying)
					select {
					case <-release:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				return ctx.Err()
			}
			client.RetryPolicy = &policy
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch calls.Add(1) {
				case 1:
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"write-once\",\"function\":{\"name\":\"Write\",\"arguments\":\"{\\\"file_path\\\":\\\"once.txt\\\",\\\"content\\\":\\\"written once\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
				case 2:
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"discarded partial\"}}],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":2}}\n\n")
				case 3, 4:
					w.WriteHeader(http.StatusServiceUnavailable)
					fmt.Fprint(w, `{"error":{"type":"server_error"}}`)
				default:
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"recovered\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				}
			}))
			defer server.Close()
			directory, workspace := t.TempDir(), t.TempDir()
			store, err := storage.Open(storage.Options{Directory: directory})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, Options{Storage: store, Client: client,
				ToolGuard: func(_ context.Context, _, name string, _ json.RawMessage) error {
					if name == "Write" {
						writes.Add(1)
					}
					return nil
				}})
			if err != nil {
				t.Fatal(err)
			}
			defer core.Close()
			ctx := t.Context()
			for _, command := range []contracts.Command{
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{
					ID: "p", Name: "Fixture", Protocol: model.Chat, BaseURL: server.URL, Models: []model.Model{{ID: "m"}},
				}}},
				{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace, Name: "Retry"}},
			} {
				if err := core.Execute(ctx, command); err != nil {
					t.Fatal(err)
				}
			}
			state, _ := core.Snapshot(ctx)
			if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
				ProjectID: state.Projects[0].ID, ModelID: "p/m", Text: "write once then finish",
			}}); err != nil {
				t.Fatal(err)
			}
			select {
			case <-retrying:
			case <-time.After(15 * time.Second):
				t.Fatal("did not reach third retry")
			}
			state, _ = core.Snapshot(ctx)
			session := state.Sessions[0]
			state, _ = core.Snapshot(ctx, session.ID)
			if session.Status != contracts.Working || session.ModelRetry == nil || session.ModelRetry.Attempt != 3 ||
				session.ModelRetry.MaxRetries != 10 || session.ModelRetry.HTTPStatus != 503 || calls.Load() != 4 {
				t.Fatalf("wrong retry projection: %+v calls=%d", session, calls.Load())
			}
			session.ModelRetry.Attempt = 999
			fresh, _ := core.Snapshot(ctx, session.ID)
			if fresh.Sessions[0].ModelRetry.Attempt != 3 {
				t.Fatal("snapshot consumer mutated active retry")
			}
			// Retry is live state for desktop/remote clients, never saved progress.
			index, err := os.ReadFile(filepath.Join(directory, "engine.json"))
			if err != nil || bytes.Contains(index, []byte(`"modelRetry"`)) {
				t.Fatalf("retry leaked into persistent index: %v", err)
			}
			if cancelRun {
				if err := core.Execute(ctx, contracts.Command{Kind: "message.stop", SessionID: session.ID}); err != nil {
					t.Fatal(err)
				}
			} else {
				close(release)
			}
			state = waitIdle(t, core, session.ID)
			wantCalls, wantOutcome := int32(5), "completed"
			if cancelRun {
				wantCalls, wantOutcome = 4, "interrupted"
			}
			if calls.Load() != wantCalls || writes.Load() != 1 || state.Sessions[0].ModelRetry != nil ||
				len(state.ModelRequests) != int(wantCalls) || state.Runs[0].Status != wantOutcome {
				t.Fatalf("settlement replayed work or lost attempts: calls=%d writes=%d session=%+v requests=%d runs=%+v",
					calls.Load(), writes.Load(), state.Sessions[0], len(state.ModelRequests), state.Runs)
			}
			if state.ModelRequests[1].Usage.Input != 7 || state.ModelRequests[1].Usage.Output != 2 {
				t.Fatal("discarded partial response lost reported usage")
			}
			toolCount := 0
			for _, message := range state.Messages[session.ID] {
				for _, block := range message.Blocks {
					if block.Text == "discarded partial" {
						t.Fatal("incomplete response survived retry")
					}
					if block.Tool != nil && block.Tool.Name == "Write" {
						toolCount++
					}
				}
			}
			if content, err := os.ReadFile(filepath.Join(workspace, "once.txt")); err != nil || string(content) != "written once" || toolCount != 1 {
				t.Fatalf("completed tool was lost: %q count=%d err=%v", content, toolCount, err)
			}
		})
	}
}
