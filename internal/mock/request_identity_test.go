package mock

import (
	"context"
	"path/filepath"
	"testing"

	"gcode/internal/contracts"
)

func TestSendRequestIdentitySurvivesCoalescingPersistenceAndOwnedSnapshots(t *testing.T) {
	options := Options{Scenario: "default", Instant: true, StatePath: filepath.Join(t.TempDir(), "state.json")}
	server := newTestServer(t, options)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, err := server.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	run, err := server.Send(ctx, contracts.SendRequest{Text: "Durable creation identity", RequestID: "owned-request", ProjectID: "fieldnotes", DeviceID: "local", ModelID: "claude-opus-4-6"})
	if err != nil {
		t.Fatal(err)
	}
	event := <-events
	if event.RequestID == "owned-request" || event.Snapshot == nil {
		t.Fatal("fixture did not coalesce the transient ACK into a full later snapshot")
	}
	check := func(snapshot contracts.Snapshot) {
		t.Helper()
		for _, candidate := range snapshot.Runs {
			if candidate.ID == run.ID {
				if candidate.RequestID != "owned-request" || candidate.SessionID != run.SessionID {
					t.Fatalf("durable creation identity changed: %+v", candidate)
				}
				return
			}
		}
		t.Fatal("created run missing from snapshot")
	}
	check(*event.Snapshot)
	for i := range event.Snapshot.Runs {
		event.Snapshot.Runs[i].RequestID = "caller mutation"
	}
	snapshot, err := server.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	check(snapshot)
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := newTestServer(t, options)
	snapshot, err = reopened.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	check(snapshot)
}
