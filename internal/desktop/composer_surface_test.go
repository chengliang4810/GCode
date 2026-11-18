package desktop

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSendButtonCarriesTheAccentAndStopsWithoutIt(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.state.Settings.General.Language = "en"
	tt := ui.NewTester(a.View, 1320, 880)
	tt.Frame()
	if _, ok := tt.Find("Send message"); !ok {
		t.Fatal("send button missing")
	}
	// The stop control is not a primary action: it keeps the plain ink, so the
	// accent marks exactly one action.
	if got := composerActionInk(a, composerStop); got.tint == a.palette.Accent {
		t.Fatal("stop control took the accent")
	}
	if got := composerActionInk(a, composerSend); got.tint != a.palette.Accent || got.ink != a.palette.AccentOn {
		t.Fatalf("send control is not the accent pair: %+v", got)
	}
}

func TestPendingSendSpinsTheActionControl(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	if a.pendingSendInFlight() {
		t.Fatal("an idle composer reported a send in flight")
	}
	a.pendingSends = map[string]primaryPendingSend{a.sessionID: {RequestID: "primary:test"}}
	if !a.pendingSendInFlight() {
		t.Fatal("an accepted send did not report itself in flight")
	}
	a.pendingSends[a.sessionID] = primaryPendingSend{RequestID: "primary:test", Queued: true}
	if a.pendingSendInFlight() {
		t.Fatal("a queued send spun the primary action")
	}
}
