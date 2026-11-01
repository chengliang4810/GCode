package remote

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func TestSubscriptionReportsDisconnectAndReconnectWithoutReplayingTasks(t *testing.T) {
	var requests atomic.Int64
	disconnect := make(chan struct{})
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		attempt := requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"kind\":\"snapshot\",\"revision\":%d,\"snapshot\":{\"revision\":%d}}\n\n", attempt, attempt)
		w.(http.Flusher).Flush()
		if attempt == 1 {
			select {
			case <-disconnect:
			case <-r.Context().Done():
			}
		} else {
			<-r.Context().Done()
		}
	}))
	defer host.Close()
	client, err := NewClient(host.URL, strings.Repeat("x", 24))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, err := client.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	read := func() contracts.Event {
		t.Helper()
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatal("subscription closed")
			}
			return event
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for connection state")
			return contracts.Event{}
		}
	}
	if first := read(); first.Snapshot == nil || first.Snapshot.Revision != 1 {
		t.Fatalf("missing first authoritative snapshot: %+v", first)
	}
	close(disconnect)
	if event := read(); event.Kind != "connection.disconnected" {
		t.Fatalf("lost connection: %+v", event)
	}
	if event := read(); event.Kind != "connection.connected" {
		t.Fatalf("reconnection: %+v", event)
	}
	if event := read(); event.Snapshot == nil || event.Snapshot.Revision != 2 {
		t.Fatalf("missing reconstructed authoritative snapshot: %+v", event)
	}
	cancel()
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			if event.Kind == "connection.disconnected" {
				t.Fatal("intentional subscription cancellation reported a disconnect")
			}
		case <-time.After(time.Second):
			t.Fatal("subscription did not exit after cancellation")
		}
	}
}
