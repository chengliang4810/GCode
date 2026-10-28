package mock

import (
	"context"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func TestAccessPlanAndAnnotationsPersistAcrossQueue(t *testing.T) {
	s := newTestServer(t, Options{TokenInterval: time.Hour})
	ctx := context.Background()
	first, err := s.Send(ctx, contracts.SendRequest{SessionID: "chat-0", Text: "first"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Send(ctx, contracts.SendRequest{SessionID: "chat-0", Plan: true, AccessMode: contracts.AccessSupervised, Annotations: []contracts.Annotation{{Index: 1, URL: "https://example.com", Element: "button", HTML: "<button>Save</button>", Path: []string{"body", "button"}}}})
	if err != nil {
		t.Fatal(err)
	}
	s.advanceRun("chat-0", first.ID, "done", true)
	v, _ := s.Snapshot(ctx)
	if v.Sessions[0].AccessMode != contracts.AccessSupervised || !v.Sessions[0].Plan {
		t.Fatal("queued mode configuration was lost")
	}
	messages := v.Messages["chat-0"]
	user := messages[len(messages)-2]
	if len(user.Blocks) != 1 || user.Blocks[0].Kind != "annotation" || user.Blocks[0].Annotation == nil {
		t.Fatal("annotation-only message missing structured content")
	}
	user.Blocks[0].Annotation.Path[0] = "changed"
	again, _ := s.Snapshot(ctx)
	if again.Messages["chat-0"][len(messages)-2].Blocks[0].Annotation.Path[0] == "changed" {
		t.Fatal("annotation snapshot payload shares backend state")
	}
	if err = s.Stop(ctx, "chat-0"); err != nil {
		t.Fatal(err)
	}
}

func TestPlanReplyAndMotionDefaults(t *testing.T) {
	s := newTestServer(t, Options{Instant: true})
	ctx := context.Background()
	events, _ := s.Subscribe(ctx)
	<-events
	if _, err := s.Send(ctx, contracts.SendRequest{SessionID: "chat-0", Text: "Plan the changes", Plan: true, AccessMode: contracts.AccessAutoAcceptEdits}); err != nil {
		t.Fatal(err)
	}
	v := await(t, events, func(v contracts.Snapshot) bool {
		return v.Sessions[0].Status == contracts.Idle && len(v.Runs) > 0 && v.Runs[len(v.Runs)-1].Status == "completed"
	})
	last := v.Messages["chat-0"][len(v.Messages["chat-0"])-1]
	if !strings.Contains(last.Blocks[0].Text, "Proposed plan") {
		t.Fatal("plan mode returned an execution response")
	}
	if !v.Runs[len(v.Runs)-1].Plan || v.Runs[len(v.Runs)-1].AccessMode != contracts.AccessAutoAcceptEdits {
		t.Fatal("run did not record selected access/plan mode")
	}
	d := DefaultSettings().Appearance
	if !d.SmoothScrolling || d.MotionPreference != "system" || d.PauseInBackground {
		t.Fatal("motion defaults differ from source .128")
	}
}
