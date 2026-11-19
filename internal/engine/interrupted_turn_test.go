package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestInterruptedMessageReplacement(t *testing.T) {
	t.Run("stopped", func(t *testing.T) { testMessageReplacement(t, false) })
	t.Run("failed", func(t *testing.T) { testMessageReplacement(t, true) })
}

func testMessageReplacement(t *testing.T, fail bool) {
	started := make(chan struct{})
	release := make(chan struct{})
	replacement := make(chan string, 1)
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if strings.Contains(string(body), "ORIGINAL_INPUT") {
			select {
			case <-started:
				replacement <- string(body)
			default:
				close(started)
				select {
				case <-r.Context().Done():
				case <-release:
					http.Error(w, "fixture failure", http.StatusBadRequest)
				}
				return
			}
		} else {
			replacement <- string(body)
		}
		goalResponse(w, model.Chat, "New reply.", nil, true)
	}))
	ctx := context.Background()
	execute := func(command contracts.Command) {
		t.Helper()
		if err := core.Execute(ctx, command); err != nil {
			t.Fatal(err)
		}
	}
	execute(contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{Text: "ORIGINAL_INPUT", RequestID: "first"}})
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("original request did not start")
	}
	state, _ := core.Snapshot(ctx)
	sid := state.Sessions[0].ID
	state, _ = core.Snapshot(ctx, sid)
	old := state.Messages[sid][0].ID
	edit := contracts.Command{Kind: "message.send", Send: &contracts.SendRequest{
		SessionID: sid, ReplaceMessageID: old, Text: "EDITED_INPUT", RequestID: "replacement",
	}}
	if err := core.Execute(ctx, edit); err == nil {
		t.Fatal("accepted replacement while running")
	}
	if fail {
		close(release)
		state = waitIdle(t, core, sid)
		if state.Sessions[0].Status != contracts.Errored {
			t.Fatalf("expected failed turn, got %s", state.Sessions[0].Status)
		}
	} else {
		execute(contracts.Command{Kind: "message.stop", SessionID: sid})
		waitSideChatSession(t, core, sid, contracts.Idle)
	}
	state, _ = core.Snapshot(ctx, sid)
	if i := contracts.EditableTurnMessage(state.Sessions[0], state.Messages[sid], state.Runs); i < 0 || state.Messages[sid][i].ID != old {
		t.Fatalf("missing stopped anchor: session=%+v runs=%+v messages=%+v", state.Sessions[0], state.Runs, state.Messages[sid])
	}
	empty := edit.Clone()
	empty.Send.Text = ""
	if err := core.Execute(ctx, empty); err == nil {
		t.Fatal("accepted empty replacement")
	}
	state, _ = core.Snapshot(ctx, sid)
	if state.Messages[sid][0].ID != old {
		t.Fatal("invalid replacement removed the original")
	}
	execute(edit)
	execute(edit)
	state = waitIdle(t, core, sid)
	users := 0
	for _, msg := range state.Messages[sid] {
		if msg.TurnID == state.Runs[0].ID {
			t.Fatal("stopped reply is still in the transcript")
		}
		if msg.Role == "user" {
			users++
			if msg.ID == old || msg.Blocks[0].Text != "EDITED_INPUT" {
				t.Fatalf("wrong user message: %+v", msg)
			}
		}
	}
	if users != 1 || len(state.Runs) != 2 {
		t.Fatalf("replacement duplicated the turn: users=%d runs=%d", users, len(state.Runs))
	}
	select {
	case body := <-replacement:
		if strings.Contains(body, "ORIGINAL_INPUT") || !strings.Contains(body, "EDITED_INPUT") {
			t.Fatal("provider context did not replace the original input")
		}
	default:
		t.Fatal("replacement never reached the model")
	}
}
