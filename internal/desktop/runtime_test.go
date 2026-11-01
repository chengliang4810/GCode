package desktop

import (
	"context"
	"fmt"
	"gcode/internal/assets"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/appconfig"
	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/i18n"
	"gcode/internal/model"
	"gcode/internal/storage"
	"github.com/egoist/mygo/ui"
)

func TestLiveChatAfterDesktopRestart(t *testing.T) {
	baseURL := os.Getenv("GCODE_LIVE_BASE_URL")
	if baseURL == "" {
		t.Skip("set GCODE_LIVE_BASE_URL, GCODE_LIVE_API_KEY and GCODE_LIVE_MODEL to run")
	}
	key, modelID := os.Getenv("GCODE_LIVE_API_KEY"), os.Getenv("GCODE_LIVE_MODEL")
	if key == "" || modelID == "" {
		t.Fatal("live API key and model ID are required")
	}
	if err := assets.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	var previousRequestID string
	for attempt := 1; attempt <= 2; attempt++ {
		func() {
			store, err := storage.Open(storage.Options{Home: home})
			if err != nil {
				t.Fatal(err)
			}
			core, err := engine.Open(engine.Options{Storage: store})
			if err != nil {
				t.Fatal(err)
			}
			defer core.Close()
			ctx := context.Background()
			if attempt == 1 {
				config := model.Provider{ID: "live", Name: "Live chat verification", Protocol: model.Chat, BaseURL: baseURL, APIKey: key, Models: []model.Model{{ID: modelID, MaxOutput: 512}}}
				if err = core.Execute(ctx, contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: config}}); err != nil {
					t.Fatal(err)
				}
			}
			app, err := New(core)
			if err != nil {
				t.Fatal(err)
			}
			defer app.Close()
			tt := ui.NewTester(app.View, 1320, 880)
			tt.SetLocale(i18n.Native(app.state.Settings.General.Language))
			if err = tt.Click(app.t("Message")); err != nil {
				t.Fatal(err)
			}
			tt.Type("这是重启后对话验证。不要调用工具，只回复 GCODE_CHAT_RESTART_OK")
			tt.Key(0, ui.KeyEnter)
			snapshot, err := core.Snapshot(ctx)
			if err != nil || len(snapshot.Runs) != attempt {
				t.Fatalf("UI did not start a new model run after launch %d: %v", attempt, err)
			}
			current := snapshot.Runs[attempt-1]
			if current.RequestID == "" || current.RequestID == previousRequestID {
				t.Fatal("desktop reused a previous send identity")
			}
			previousRequestID = current.RequestID
			deadline := time.Now().Add(90 * time.Second)
			for current.Status == "running" && time.Now().Before(deadline) {
				time.Sleep(50 * time.Millisecond)
				snapshot, err = core.Snapshot(ctx)
				if err != nil {
					t.Fatal(err)
				}
				current = snapshot.Runs[attempt-1]
			}
			if current.Status != "completed" {
				t.Fatalf("live chat failed: %s: %s", current.Status, current.Error)
			}
			app.apply(snapshot)
			tt.Frame()
			var reply strings.Builder
			for _, message := range snapshot.Messages[current.SessionID] {
				if message.Role == "assistant" {
					for _, block := range message.Blocks {
						if block.Kind == "text" {
							reply.WriteString(block.Text)
						}
					}
				}
			}
			if app.pendingNew || app.sessionID != current.SessionID || !strings.Contains(reply.String(), "GCODE_CHAT_RESTART_OK") || !strings.Contains(strings.Join(tt.Texts(), "\n"), "GCODE_CHAT_RESTART_OK") {
				t.Fatalf("live reply was not shown in the new conversation: %q", reply.String())
			}
			t.Logf("launch=%d request=%s session=%s model=%s reply=%s", attempt, current.RequestID, current.SessionID, modelID, reply.String())
		}()
	}
}

func TestChatSendSurvivesDesktopAndEngineRestart(t *testing.T) {
	for _, surface := range []string{"canvas", "primary", "side"} {
		t.Run(surface, func(t *testing.T) {
			var calls atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"RESTART_REPLY_OK\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer provider.Close()
			home := t.TempDir()
			open := func() (*App, *engine.Engine) {
				t.Helper()
				store, err := storage.Open(storage.Options{Home: home})
				if err != nil {
					t.Fatal(err)
				}
				core, err := engine.Open(engine.Options{Storage: store})
				if err != nil {
					t.Fatal(err)
				}
				app, err := New(core)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(app.Close)
				return app, core
			}
			first, core := open()
			ctx := context.Background()
			config := model.Provider{ID: "restart", Name: "Restart fixture", Protocol: model.Chat, BaseURL: provider.URL, Models: []model.Model{{ID: "model"}}}
			if err := core.Execute(ctx, contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: config}}); err != nil {
				t.Fatal(err)
			}
			var sessionID string
			if surface != "canvas" {
				if err := core.Execute(ctx, contracts.Command{Kind: "session.create", Name: "Restart conversation"}); err != nil {
					t.Fatal(err)
				}
				snapshot, _ := core.Snapshot(ctx)
				sessionID = snapshot.Sessions[0].ID
				if surface == "side" {
					if err := core.Execute(ctx, contracts.Command{Kind: "session.sidechat", SessionID: sessionID}); err != nil {
						t.Fatal(err)
					}
					snapshot, _ = core.Snapshot(ctx)
					sessionID = snapshot.Sessions[0].ID
				}
			}
			send := func(app *App, current *engine.Engine, text string) {
				t.Helper()
				snapshot, err := current.Snapshot(ctx)
				if err != nil {
					t.Fatal(err)
				}
				app.apply(snapshot)
				app.modelID = "restart/model"
				if surface == "primary" {
					app.selectSession(sessionID)
				}
				if surface == "side" {
					var session contracts.Session
					for _, candidate := range snapshot.Sessions {
						if candidate.ID == sessionID {
							session = candidate
						}
					}
					state := app.sideComposer(sessionID)
					state.modelID = "restart/model"
					if !app.sendSideMessage(session, state, text) || state.Failure != "" {
						t.Fatalf("side send rejected: %s", state.Failure)
					}
				} else if !app.send(text) || app.errorText != "" {
					t.Fatalf("primary send rejected: %s", app.errorText)
				}
			}
			wait := func(current *engine.Engine, count int) contracts.Snapshot {
				t.Helper()
				deadline := time.Now().Add(5 * time.Second)
				for time.Now().Before(deadline) {
					snapshot, err := current.Snapshot(ctx)
					if err != nil {
						t.Fatal(err)
					}
					if len(snapshot.Runs) != count {
						t.Fatalf("fresh %s send was ignored after restart: runs=%d, want %d", surface, len(snapshot.Runs), count)
					}
					run := snapshot.Runs[count-1]
					if run.Status != "running" {
						if run.Status != "completed" {
							t.Fatalf("send failed: %s", run.Error)
						}
						return snapshot
					}
					time.Sleep(10 * time.Millisecond)
				}
				t.Fatal("chat did not complete")
				return contracts.Snapshot{}
			}
			send(first, core, "Before restart")
			before := wait(core, 1)
			first.Close()
			second, restarted := open()
			send(second, restarted, "After restart")
			after := wait(restarted, 2)
			if before.Runs[0].RequestID == after.Runs[1].RequestID || calls.Load() != 2 {
				t.Fatal("fresh desktop sends did not reach the model independently")
			}
			second.apply(after)
			if surface != "side" && (second.pendingNew || second.sessionID != after.Runs[1].SessionID) {
				t.Fatal("accepted reply did not reach the selected conversation")
			}
		})
	}
}

func TestRealCoreSettingsAndTypedMessageProjection(t *testing.T) {
	if err := assets.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.Open(engine.Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	connections, err := appconfig.OpenConnections(store)
	if err != nil {
		t.Fatal(err)
	}
	app, err := NewWithOptions(core, Options{Connections: connections})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	if app.state.Settings.General.Language != "zh-CN" {
		t.Fatal("Chinese must be the default")
	}
	app.state.Settings.General.Language = "en"
	app.saveSettings()
	app.OpenRoute("settings/Model settings")
	tt := ui.NewTester(app.View, 1320, 880)
	if err = tt.Click("Add provider"); err != nil {
		t.Fatal(err)
	}
	if len(app.state.CustomProviders) != 1 || app.state.CustomProviders[0].Provider.BaseURL != "" ||
		len(app.state.CustomProviders[0].Provider.Models) != 0 || len(app.state.Models) != 0 {
		t.Fatal("adding a provider must create an editable empty draft outside the execution catalog")
	}
	app.runtimeUI.provider.Name = "Local fixture"
	app.runtimeUI.provider.BaseURL = "http://127.0.0.1:1/v1"
	app.runtimeUI.provider.Models = []model.Model{{ID: "model-a"}, {ID: "model-b"}}
	app.runtimeUI.key = "dummy-test-key"
	app.markInlineProviderDirty()
	app.saveProviderSettings(app.runtimeUI.provider)
	if app.modelsUI.inlineError != "" {
		t.Fatal(app.modelsUI.inlineError)
	}
	tt.Frame()
	if app.errorText != "" || len(app.state.Models) != 2 || app.state.CustomProviders[0].Provider.APIKey != "" {
		t.Fatalf("provider form: %s", app.errorText)
	}
	app.OpenRoute("settings/General")
	tt.Frame()
	target := filepath.Join(filepath.Dir(store.Directory()), "chosen-data")
	app.runtimeUI.directory = target
	tt.Frame()
	if err = tt.Click("Apply"); err != nil {
		t.Fatal(err)
	}
	if app.state.Runtime.PendingDirectory != target {
		t.Fatalf("directory setting did not reach core: %s", app.errorText)
	}
	app.OpenRoute("settings/Devices")
	tt.Frame()
	if _, ok := tt.Find("Remote host"); !ok {
		t.Fatal("remote connection form missing")
	}
	app.applyEvent(contracts.Event{Kind: "message.changed", Revision: app.state.Revision + 1, SessionID: "stream-test", Message: &contracts.Message{ID: "m", Role: "assistant", Streaming: true}})
	app.applyEvent(contracts.Event{Kind: "message.delta", Revision: app.state.Revision + 1, SessionID: "stream-test", MessageID: "m", BlockKind: "text", Text: "原生 Go 增量"})
	if app.state.Messages["stream-test"][0].Blocks[0].Text != "原生 Go 增量" {
		t.Fatal("typed streaming projection failed")
	}
	snapshot, _ := core.Snapshot(context.Background())
	if snapshot.CustomProviders[0].Provider.Protocol != model.Chat {
		t.Fatal("protocol was not saved")
	}
}
