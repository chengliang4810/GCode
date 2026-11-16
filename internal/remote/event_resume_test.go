package remote

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gcode/internal/contracts"
)

type eventResumeProbe struct {
	contracts.Service
	cursors chan uint64
}

func (p eventResumeProbe) Subscribe(context.Context, ...string) (<-chan contracts.Event, error) {
	events := make(chan contracts.Event, 1)
	events <- contracts.Event{Kind: "snapshot", Snapshot: &contracts.Snapshot{}}
	close(events)
	return events, nil
}

func (p eventResumeProbe) SubscribeFrom(_ context.Context, after uint64, _ ...string) (<-chan contracts.Event, error) {
	p.cursors <- after
	events := make(chan contracts.Event, 1)
	events <- contracts.Event{Kind: "resumed", Sequence: after + 1}
	close(events)
	return events, nil
}

func TestEventResumeServerAcceptsZeroAndRejectsPriorEpoch(t *testing.T) {
	probe := eventResumeProbe{cursors: make(chan uint64, 2)}
	token := strings.Repeat("x", 24)
	handler, err := (Server{Core: probe, Token: token}).Handler()
	if err != nil {
		t.Fatal(err)
	}
	host := httptest.NewServer(handler)
	defer host.Close()
	client, err := NewClient(host.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	read := func(cursor string) (string, contracts.Event) {
		t.Helper()
		response, err := client.requestWithCursor(t.Context(), http.MethodGet, "/v1/events", nil, cursor)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var id string
		var event contracts.Event
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "id: ") {
				id = strings.TrimPrefix(line, "id: ")
			}
			if strings.HasPrefix(line, "data: ") {
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
					t.Fatal(err)
				}
				return id, event
			}
		}
		t.Fatalf("missing stream event: %v", scanner.Err())
		return "", event
	}
	id, event := read("")
	if event.Kind != "snapshot" || !strings.HasSuffix(id, ":0") {
		t.Fatalf("missing initial zero cursor: %s %+v", id, event)
	}
	_, event = read(id)
	if event.Kind != "resumed" || <-probe.cursors != 0 {
		t.Fatal("server discarded a valid zero cursor")
	}
	_, event = read("prior-host:7")
	if event.Kind != "snapshot" || len(probe.cursors) != 0 {
		t.Fatal("prior epoch incorrectly replayed events from a new host")
	}
}
