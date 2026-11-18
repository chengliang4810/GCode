package remote

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestAutomationBelongsToHostAfterClientDisconnect(t *testing.T) {
	started, release := make(chan struct{}, 1), make(chan struct{}, 1)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Scheduled host proof\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer provider.Close()
	defer close(release)
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.Open(engine.Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	state, _ := core.Snapshot(t.Context())
	settings := state.Settings
	settings.General.ThreadNamingModel = "first-message"
	if err = core.Execute(t.Context(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	if err = core.Execute(t.Context(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: model.Provider{ID: "schedule", Name: "Fixture", Protocol: model.Chat, BaseURL: provider.URL, Models: []model.Model{{ID: "model"}}}}}); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("host", 8)
	handler, err := (Server{Core: core, Token: token}).Handler()
	if err != nil {
		t.Fatal(err)
	}
	host := httptest.NewServer(handler)
	defer host.Close()
	client, err := NewClient(host.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	job := contracts.Automation{Name: "Remote automation", Prompt: "Reply with host proof", ModelID: "schedule/model", Mode: "manual", Enabled: true, Timezone: "UTC"}
	if err = client.Execute(t.Context(), contracts.Command{Kind: "automation.save", Automation: &job}); err != nil {
		t.Fatal(err)
	}
	state, err = client.Snapshot(t.Context())
	if err != nil || len(state.Automations) != 1 {
		t.Fatalf("remote projection: %v %+v", err, state.Automations)
	}
	if err = client.Execute(t.Context(), contracts.Command{Kind: "automation.run", ID: state.Automations[0].ID}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("host did not start automation")
	}
	_ = client.Close()
	// Releasing the response proves closing the client did not cancel accepted work.
	release <- struct{}{}
	deadline := time.Now().Add(10 * time.Second)
	for {
		state, err = core.Snapshot(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if len(state.AutomationRuns) == 1 && state.AutomationRuns[0].Status == "completed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("host did not settle: %+v", state.AutomationRuns)
		}
		time.Sleep(10 * time.Millisecond)
	}
	reconnected, err := NewClient(host.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	defer reconnected.Close()
	recovered, err := reconnected.Snapshot(t.Context())
	if err != nil || recovered.AutomationRuns[0].Status != "completed" {
		t.Fatalf("reconnect lost history: %v %+v", err, recovered.AutomationRuns)
	}
}
