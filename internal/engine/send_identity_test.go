package engine

import (
	"net/http"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestSendRequestIdentitySurvivesQueueHandoff(t *testing.T) {
	release := make(chan struct{})
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
			goalResponse(w, model.Chat, "Done.", nil, true)
		case <-r.Context().Done():
		}
	}))
	ctx := t.Context()
	first := contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
		RequestID: "first-request", Text: "Identical text", ModelID: "goal/model",
	}}
	if err := core.Execute(ctx, first); err != nil {
		t.Fatal(err)
	}
	state, err := core.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sessionID := state.Sessions[0].ID
	state, err = core.Snapshot(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Messages[sessionID][0].RequestID != first.Send.RequestID {
		t.Fatal("accepted user message lost its request identity")
	}
	queued := first.Clone()
	queued.Send.SessionID, queued.Send.RequestID = sessionID, "queued-request"
	for range 2 {
		if err := core.Execute(ctx, queued); err != nil {
			t.Fatal(err)
		}
	}
	state, _ = core.Snapshot(ctx, sessionID)
	if len(state.Queue) != 1 || state.Queue[0].RequestID != queued.Send.RequestID {
		t.Fatalf("queue lost request identity or duplicated a retry: %+v", state.Queue)
	}
	close(release)
	state = waitQueueDone(t, core, sessionID)
	requestIDs := []string{}
	for _, message := range state.Messages[sessionID] {
		if message.Role == "user" {
			requestIDs = append(requestIDs, message.RequestID)
		}
	}
	if len(requestIDs) != 2 || requestIDs[0] != first.Send.RequestID || requestIDs[1] != queued.Send.RequestID {
		t.Fatalf("handoff lost the submitted request identities: %v", requestIDs)
	}
	// Once the queued run has ended, its original request must still be
	// recognized through the persisted user message rather than sent again.
	if err := core.Execute(ctx, queued); err != nil {
		t.Fatal(err)
	}
	after, _ := core.Snapshot(ctx, sessionID)
	if len(after.Messages[sessionID]) != len(state.Messages[sessionID]) || len(after.Queue) != 0 {
		t.Fatal("retry after handoff duplicated an accepted input")
	}
}
