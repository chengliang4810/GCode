package mock

import (
	"context"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func TestAnnotationsPersistAcrossQueue(t *testing.T) {
	s := newTestServer(t, Options{TokenInterval: time.Hour})
	ctx := context.Background()
	first, err := s.Send(ctx, contracts.SendRequest{SessionID: "chat-0", Text: "first"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Send(ctx, contracts.SendRequest{SessionID: "chat-0", Annotations: []contracts.Annotation{{Index: 1, URL: "https://example.com", Element: "button", HTML: "<button>Save</button>", Path: []string{"body", "button"}}}})
	if err != nil {
		t.Fatal(err)
	}
	s.advanceRun("chat-0", first.ID, "done", true)
	v, _ := s.Snapshot(ctx)
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

func TestMotionDefaults(t *testing.T) {
	d := DefaultSettings().Appearance
	if d.MotionPreference != "system" || d.PauseInBackground {
		t.Fatal("motion defaults differ from source .128")
	}
}
