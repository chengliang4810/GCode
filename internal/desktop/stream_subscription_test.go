package desktop

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/engine"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestRealCoreSubscriptionAppliesStreamingDeltas(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(release) }) }
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"first \"}}]}\n\n")
		w.(http.Flusher).Flush()
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"second\"}}]}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
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
	defer finish()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	config := model.Provider{ID: "delta", Name: "Streaming fixture", Protocol: model.Chat, BaseURL: provider.URL, Models: []model.Model{{ID: "model"}}}
	if err := core.Execute(ctx, contracts.Command{Kind: "provider.save", Provider: &contracts.ProviderInput{Provider: config}}); err != nil {
		t.Fatal(err)
	}
	state, err := core.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings := state.Settings
	settings.General.ThreadNamingModel = "first-message"
	if err := core.Execute(ctx, contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "session.create", Name: "delta fixture"}); err != nil {
		t.Fatal(err)
	}
	state, err = core.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sessionID := state.Sessions[0].ID
	app, err := New(core)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	defer finish()
	app.sessionID = sessionID
	app.sessionSubscriptionScope = []string{sessionID}
	events, err := core.Subscribe(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{SessionID: sessionID, Text: "stream", ModelID: "delta/model"}}); err != nil {
		t.Fatal(err)
	}
	var deltas strings.Builder
	for {
		select {
		case <-ctx.Done():
			t.Fatal("stream subscription did not settle: ", ctx.Err())
		case event, ok := <-events:
			if !ok {
				t.Fatal("subscription closed before completion")
			}
			app.applyEvent(event)
			if event.Kind == "message.delta" && event.BlockKind == "text" {
				if event.Snapshot != nil {
					t.Fatal("real delta unexpectedly carried a full snapshot")
				}
				deltas.WriteString(event.Text)
				var projected strings.Builder
				for _, message := range app.state.Messages[sessionID] {
					if message.ID == event.MessageID {
						for _, block := range message.Blocks {
							if block.Kind == "text" {
								projected.WriteString(block.Text)
							}
						}
					}
				}
				if projected.String() != deltas.String() {
					t.Fatalf("incremental projection = %q, deltas = %q", projected.String(), deltas.String())
				}
				if deltas.String() == "first second" {
					finish()
				}
			}
			if deltas.String() == "first second" && event.Snapshot != nil {
				for _, run := range event.Snapshot.Runs {
					if run.SessionID == sessionID && run.Status == "completed" {
						return
					}
				}
			}
		}
	}
}
