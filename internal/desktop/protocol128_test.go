package desktop

import (
	"strings"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestAnnotationSendKeepsStructuredContent(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.attachAnnotation(contracts.Annotation{Index: 1, Title: "Example", URL: "https://example.com", Element: "button", Text: "Save"})
	a.draft = "Change this button"
	a.send(a.draft)
	var user contracts.Message
	for _, message := range a.state.Messages["chat-0"] {
		if message.Role == "user" {
			user = message
		}
	}
	if len(user.Blocks) != 2 || user.Blocks[0].Text != "Change this button" || user.Blocks[1].Annotation == nil {
		t.Fatalf("message lost native annotation or duplicated its encoded token: %#v", user.Blocks)
	}
	if len(a.annotations) != 0 {
		t.Fatal("sent annotation remained in draft")
	}
}

func TestFailedAnnotatedSendRetainsEditableDraft(t *testing.T) {
	a := testApp(t, "default")
	a.service = failingSendService{a.service}
	a.draft = "Change this button"
	a.attachAnnotation(contracts.Annotation{Index: 2, URL: "https://example.com", Element: "button"})
	a.send(a.draft)
	if a.draft != "Change this button" || len(a.annotations) != 1 || strings.Contains(a.draft, "rcode-annotation:") {
		t.Fatal("failed send lost structured annotation or exposed the protocol token")
	}
	a.removeAnnotation(0)
	if len(a.annotations) != 0 {
		t.Fatal("annotation chip was not removed")
	}
}

func TestUserMarkdownCollapsesLongPrompt(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.state.Messages[a.sessionID] = []contracts.Message{{ID: "long", Role: "user", Blocks: []contracts.Block{{ID: "text", Kind: "text", Text: strings.Repeat("A long **formatted** prompt with context.\n", 12)}}}}
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Expand message"); err != nil {
		t.Fatal(err)
	}
	if !a.expanded["user:long"] {
		t.Fatal("long prompt did not expand")
	}
	if err := tt.Click("Collapse message"); err != nil {
		t.Fatal(err)
	}
	if a.expanded["user:long"] {
		t.Fatal("long prompt did not collapse")
	}
}

func TestExplicitMotionPreferences(t *testing.T) {
	a := testApp(t, "default")
	var reduced bool
	tt := ui.NewTester(func(c *ui.Context) { reduced = a.reduceMotion(c); a.View(c) }, 1320, 880)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Advance(1)
	if !reduced {
		t.Fatal("system reduce motion was ignored")
	}
	a.state.Settings.Appearance.MotionPreference = "off"
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Advance(1)
	if reduced {
		t.Fatal("explicit off did not override system preference")
	}
	a.state.Settings.Appearance.MotionPreference = "on"
	tt.SetPreferences(ui.Preferences{})
	tt.Advance(1)
	if !reduced {
		t.Fatal("explicit on did not reduce motion")
	}
}
