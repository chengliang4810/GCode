package remote

import (
	"context"
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

func TestDisconnectDoesNotCancelHostTaskAndReconnectRestoresState(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"host task completed\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer provider.Close()
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.Open(engine.Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	config := model.Provider{ID: "test", Name: "Test", Protocol: model.Chat, BaseURL: provider.URL, Models: []model.Model{{ID: "model"}}}
	if err := core.Execute(context.Background(), contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: config}}); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("test", 8)
	handler, err := (Server{Core: core, Token: token}).Handler()
	if err != nil {
		t.Fatal(err)
	}
	host := httptest.NewServer(handler)
	defer host.Close()
	unauthorized, err := http.Get(host.URL + "/v1/snapshot")
	if err != nil {
		t.Fatal(err)
	}
	_ = unauthorized.Body.Close()
	if unauthorized.StatusCode != 401 {
		t.Fatal("unauthenticated request accepted")
	}
	client, err := NewClient(host.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	events, err := client.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first := <-events
	if first.Snapshot == nil || first.Snapshot.Runtime == nil || !first.Snapshot.Runtime.Remote {
		t.Fatal("missing initial authoritative snapshot")
	}
	err = client.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{Text: "Work on the host", ModelID: "test/model", RequestID: "remote-request"}})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("host task did not start")
	}
	running, _ := core.Snapshot(context.Background())
	pending, err := client.ReadModelTrajectory(context.Background(), running.Sessions[0].ID)
	if err != nil || len(pending.Records) != 1 || pending.Records[0].CompletedAt != nil || pending.Records[0].Error != "" {
		t.Fatalf("live remote call was unavailable or incorrectly marked abandoned: %v", err)
	}
	cancel()
	_ = client.Close()
	close(release)
	deadline := time.After(5 * time.Second)
	for {
		state, _ := core.Snapshot(context.Background())
		if len(state.Runs) > 0 && state.Runs[0].Status == "completed" {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("host run ended incorrectly: %+v", state.Runs)
		case <-time.After(10 * time.Millisecond):
		}
	}
	reconnected, err := NewClient(host.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	defer reconnected.Close()
	state, err := reconnected.Bootstrap(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Sessions) != 1 || state.Runs[0].Status != "completed" {
		t.Fatal("reconnect lost host result")
	}
	trajectory, err := reconnected.ReadModelTrajectory(context.Background(), state.Sessions[0].ID)
	if err != nil || len(trajectory.Records) != 1 || trajectory.Records[0].Response.Message.Text != "host task completed" {
		t.Fatalf("remote trajectory did not read the host-owned model call: %v", err)
	}
	if _, err := reconnected.ReadModelTrajectory(context.Background(), "../foreign"); err == nil {
		t.Fatal("remote trajectory accepted an invalid conversation")
	}
	target := t.TempDir()
	if err := reconnected.Execute(context.Background(), contracts.Command{Kind: "storage.change", Path: target}); err != nil {
		t.Fatal(err)
	}
	state, _ = reconnected.Snapshot(context.Background())
	if state.Runtime.PendingDirectory != target {
		t.Fatal("remote settings did not update the host's pending data directory")
	}
}
