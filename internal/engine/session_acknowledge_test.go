package engine

import (
	"context"
	"net/http"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

// A run that fails marks the session until somebody sees it. Seeing it returns
// the session to idle, and the outcome keeps naming the failure so the
// transcript, the menu bar and the failed turn's edit anchor still show what
// happened.
func TestSeenFailureReturnsTheSessionToIdle(t *testing.T) {
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fixture failure", http.StatusBadRequest)
	}))
	ctx := context.Background()
	if err := core.Execute(ctx, contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{Text: "first", RequestID: "failure"}}); err != nil {
		t.Fatal(err)
	}
	state, err := core.Snapshot(ctx)
	if err != nil || len(state.Sessions) != 1 {
		t.Fatalf("snapshot: %v sessions=%d", err, len(state.Sessions))
	}
	sid := state.Sessions[0].ID
	state = waitIdle(t, core, sid)
	if session := state.Sessions[0]; session.Status != contracts.Errored || session.LastOutcome != "failed" {
		t.Fatalf("unseen failure = %s/%s", session.Status, session.LastOutcome)
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "session.acknowledge", SessionID: sid}); err != nil {
		t.Fatal(err)
	}
	state, err = core.Snapshot(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	seen := state.Sessions[0]
	if seen.Status != contracts.Idle || seen.LastOutcome != "failed" {
		t.Fatalf("seen failure = %s/%s", seen.Status, seen.LastOutcome)
	}
	anchor := contracts.EditableTurnMessage(seen, state.Messages[sid], state.Runs)
	if anchor < 0 || state.Messages[sid][anchor].Role != "user" {
		t.Fatalf("a seen failure lost its edit anchor: runs=%+v", state.Runs)
	}
	// Reading a failure twice is harmless, an unknown session is not.
	if err := core.Execute(ctx, contracts.Command{Kind: "session.acknowledge", SessionID: sid}); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(ctx, contracts.Command{Kind: "session.acknowledge", SessionID: "missing"}); err == nil {
		t.Fatal("acknowledged an unknown session")
	}
}
