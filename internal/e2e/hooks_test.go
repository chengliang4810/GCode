//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

// Real command hooks run in a private local core, then the native app reopens
// their persisted records. Only the model endpoint is a local HTTP fixture.
func TestDesktopLocalHookRecords(t *testing.T) {
	for _, appearance := range []string{"light", "dark"} {
		t.Run(appearance, func(t *testing.T) {
			binary, artifacts := testPaths(t)
			data, workspace := filepath.Join(artifacts, "data"), filepath.Join(artifacts, "workspace")
			if err := os.MkdirAll(workspace, 0700); err != nil {
				t.Fatal(err)
			}
			proofPath := filepath.Join(workspace, "hook-proof.txt")
			if err := os.WriteFile(proofPath, []byte("NATIVE_READ_PROOF"), 0600); err != nil {
				t.Fatal(err)
			}
			var contextText strings.Builder
			contextText.WriteString("NATIVE_HOOK_CONTEXT\n")
			for i := range 80 {
				fmt.Fprintf(&contextText, "HOOK OUTPUT %03d\n", i)
			}
			contextText.WriteString("NATIVE_HOOK_TAIL")
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				var request struct {
					Messages []struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"messages"`
				}
				if err := json.Unmarshal(body, &request); err != nil {
					t.Error(err)
				}
				injected := false
				for _, message := range request.Messages {
					if strings.Contains(message.Content, "NATIVE_HOOK_CONTEXT") {
						injected = message.Role == "user" && strings.Contains(message.Content, "NATIVE_HOOK_TAIL")
					}
					if strings.Contains(message.Content, "Hook Native startup") {
						t.Error("presentation log entered model history")
					}
				}
				if !injected {
					t.Error("startup context no longer reaches the model")
				}
				number := requests.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				if number == 1 {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"read-proof\",\"function\":{\"name\":\"Read\",\"arguments\":\"{\\\"file_path\\\":\\\"hook-proof.txt\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
				} else {
					if number != 2 || !strings.Contains(string(body), "NATIVE_READ_PROOF") {
						t.Error("provider did not receive the real read result")
					}
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Native hook reply.\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				}
			}))
			defer server.Close()
			store, err := storage.Open(storage.Options{Directory: data})
			if err != nil {
				t.Fatal(err)
			}
			core, err := openExecutionFixture(t, engine.Options{Storage: store})
			if err != nil {
				_ = store.Close()
				t.Fatal(err)
			}
			defer core.Close()
			ctx := context.Background()
			snapshot, err := core.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			settings := snapshot.Settings
			settings.General.CompactMode = appearance == "dark"
			settings.Appearance.ReduceMotion = true
			settings.Appearance.FontSize, settings.Appearance.CodeFontSize = 14, 12
			settings.Hooks = []contracts.HookRule{
				{ID: "native-start", Name: "Native startup", Event: contracts.HookSessionStart, Type: "command",
					Command: "printf '%s\\n' '" + contextText.String() + "'", Shell: "bash", Enabled: true},
				{ID: "native-stop", Name: "Native stop", Event: contracts.HookStop, Type: "command",
					Command: `printf '%s\n' '{"systemMessage":"NATIVE_STOP_NOTICE"}'`, Shell: "bash", Enabled: true},
			}
			provider := model.Provider{ID: "hooks", Name: "Local hook fixture", Protocol: model.Chat,
				BaseURL: server.URL, Models: []model.Model{{ID: "test"}}}
			for _, command := range []contracts.Command{
				{Kind: "settings.update", Settings: &settings},
				{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: provider}},
				{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace, Name: "Hook fixture"}},
			} {
				if err := core.Execute(ctx, command); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err = core.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
				ProjectID: snapshot.Projects[0].ID, ModelID: "hooks/test", Text: "Hello hook fixture.",
			}}); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(stateTimeout)
			for {
				snapshot, err = core.Snapshot(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if len(snapshot.Sessions) == 1 && snapshot.Sessions[0].Status == contracts.Idle &&
					snapshot.Sessions[0].LastOutcome == "completed" {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("real hook fixture did not complete")
				}
				time.Sleep(20 * time.Millisecond)
			}
			sessionID := snapshot.Sessions[0].ID
			snapshot, err = core.Snapshot(ctx, sessionID)
			if err != nil {
				t.Fatal(err)
			}
			var entryID, startID, stopID string
			for _, message := range snapshot.Messages[sessionID] {
				if entryID == "" && message.Role == "assistant" {
					entryID = message.ID
				}
				for _, block := range message.Blocks {
					if hook := block.Hook; hook != nil {
						if hook.Status != "completed" || hook.ExitCode == nil || *hook.ExitCode != 0 || block.Text != "" {
							t.Fatalf("invalid persisted hook: %+v", block)
						}
						switch hook.Event {
						case contracts.HookSessionStart:
							startID = block.ID
						case contracts.HookStop:
							stopID = block.ID
						}
					}
				}
			}
			if startID == "" || stopID == "" || requests.Load() != 2 {
				t.Fatalf("missing real hooks or unexpected requests: start=%q stop=%q requests=%d", startID, stopID, requests.Load())
			}
			if err := core.Close(); err != nil {
				t.Fatal(err)
			}
			app := startApp(t, binary, artifacts, "hooks", data, "--local", "--appearance", appearance,
				"--language", "en", "--width", "900", "--height", "880")
			app.act("back-to-conversations", "press", "")
			app.act("sidebar-session:"+sessionID, "press", "")
			groupID := entryID + ":activity:" + startID
			stopGroupID := entryID + ":activity:" + stopID
			if settings.General.CompactMode {
				stopGroupID = groupID
			}
			groupToggle := "activity-toggle:" + groupID
			startToggle := "activity-item-toggle:" + groupID + ":" + startID
			stopToggle := "activity-item-toggle:" + stopGroupID + ":" + stopID
			app.waitKey(groupToggle)
			app.waitKey("activity-toggle:" + stopGroupID)
			app.await("hook output starts collapsed after reopening", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				for _, text := range []string{"Native hook reply."} {
					if !activityTreeContains(tree, text) {
						return fmt.Errorf("missing %q", text)
					}
				}
				for _, text := range []string{"Native startup", "Native stop", "NATIVE_HOOK_CONTEXT", "NATIVE_STOP_NOTICE"} {
					if activityTreeContains(tree, text) {
						return fmt.Errorf("hook details expanded without user action: %q", text)
					}
				}
				for _, node := range tree.Nodes {
					var key string
					if json.Unmarshal(node.Key, &key) == nil && strings.HasPrefix(key, "activity-item-toggle:hook:") {
						return fmt.Errorf("hook still rendered outside its activity group")
					}
				}
				return nil
			})
			app.capture("hook-records-collapsed")
			app.act(groupToggle, "press", "")
			app.waitKey(startToggle)
			app.await("hook and real Read share the activity group", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if !activityTreeContains(tree, "SessionStart") || !activityTreeContains(tree, "Open file: "+proofPath) ||
					activityTreeContains(tree, "NATIVE_HOOK_CONTEXT") {
					return fmt.Errorf("group does not contain the hook and Read with closed hook details")
				}
				return nil
			})
			app.capture("hook-and-read-group-expanded")
			app.act(startToggle, "press", "")
			app.assertActivityScroll("activity-detail-viewport:"+groupID+":"+startID,
				startToggle, "NATIVE_HOOK_TAIL", 24*14*1.5)
			app.capture("startup-output-expanded-tail")
			app.act(startToggle, "press", "")
			if stopGroupID != groupID {
				app.act("activity-toggle:"+stopGroupID, "press", "")
			}
			app.waitKey(stopToggle)
			app.act(stopToggle, "press", "")
			app.await("Stop hook notice is available in details", func() error {
				tree, err := app.client.tree()
				if err != nil {
					return err
				}
				if !activityTreeContains(tree, "NATIVE_STOP_NOTICE") {
					return fmt.Errorf("Stop hook notice missing")
				}
				return nil
			})
			app.capture("stop-output-expanded")
		})
	}
}
