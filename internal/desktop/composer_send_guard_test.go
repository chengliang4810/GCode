package desktop

import (
	"runtime"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

// A blocked send names its reason in the order send() refuses the draft.
func TestSendGuardBlockReasons(t *testing.T) {
	blocked := func(t *testing.T, mutate func(a *App)) string {
		t.Helper()
		a := testApp(t, "default")
		a.selectSession("chat-0")
		a.draft = "Fix the failing test"
		mutate(a)
		return a.sendBlockReason()
	}
	for _, test := range []struct {
		name   string
		mutate func(a *App)
		want   string
	}{
		{
			name:   "nothing blocks a ready composer",
			mutate: func(a *App) {},
			want:   "",
		},
		{
			name:   "a new session is still being minted",
			mutate: func(a *App) { a.pendingNew = true },
			want:   "Entering a name for the new session.",
		},
		{
			name: "an attachment is still importing",
			mutate: func(a *App) {
				a.attachmentImports = map[string]int{a.attachmentScope(): 1}
			},
			want: "Finish importing the attachment before sending.",
		},
		{
			name: "the footer git save is running",
			mutate: func(a *App) {
				a.sessionID = ""
				a.footerGit.PendingRef = "main"
			},
			want: "Wait for the running save to finish.",
		},
		{
			name:   "no model is chosen",
			mutate: func(a *App) { a.modelID = "" },
			want:   "Choose a model before sending.",
		},
		{
			name: "no conversation and no agents",
			mutate: func(a *App) {
				a.sessionID = ""
				a.state.Providers = nil
			},
			want: "Open a conversation before sending.",
		},
		{
			name:   "an empty draft names no reason by itself",
			mutate: func(a *App) { a.draft = "" },
			want:   "",
		},
		{
			name: "an empty draft still reports the block ahead of it",
			mutate: func(a *App) {
				a.draft = ""
				a.modelID = ""
			},
			want: "Choose a model before sending.",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := blocked(t, test.mutate); got != test.want {
				t.Fatalf("sendBlockReason = %q, want %q", got, test.want)
			}
		})
	}
}

// Enter on a refused send keeps the draft, says why under the pill, and sends
// once the reason is gone.
func TestSendGuardEnterKeepsDraftAndShowsHint(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	model := a.modelID
	if model == "" {
		t.Fatal("the fixture has no model to restore")
	}
	a.modelID = ""
	recorder := &sendRecordingService{Service: a.service}
	a.service = recorder
	tt := ui.NewTester(a.View, 1320, 880)
	tt.Frame()
	if err := tt.Click("Message"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Keep this draft")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if len(recorder.commands) != 0 {
		t.Fatalf("a blocked Enter dispatched %d sends", len(recorder.commands))
	}
	if a.draft != "Keep this draft" {
		t.Fatalf("a blocked Enter changed the draft to %q", a.draft)
	}
	if !tt.HasText("Choose a model before sending.") {
		t.Fatalf("a blocked Enter explained nothing: %v", tt.Texts())
	}
	a.modelID = model
	tt.Frame()
	if tt.HasText("Choose a model before sending.") {
		t.Fatal("the hint outlived the condition it reported")
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if len(recorder.commands) != 1 {
		t.Fatalf("the corrected Enter dispatched %d sends", len(recorder.commands))
	}
	if tt.HasText("Choose a model before sending.") {
		t.Fatal("a successful send kept the blocked hint")
	}
}

// Escape stops a running response and does nothing while the session is idle.
func TestSendGuardEscapeOnlyStopsWhileRunning(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.state.Settings.General.StopWithEscape = true
	recorder := &recordingService{Service: a.service}
	a.service = recorder
	tt := ui.NewTester(a.View, 1320, 880)
	tt.Frame()
	if err := tt.Click("Message"); err != nil {
		t.Fatal(err)
	}
	stops := func() int {
		count := 0
		for _, command := range recorder.executed {
			if command.Kind == "message.stop" {
				count++
			}
		}
		return count
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if stops() != 0 {
		t.Fatal("Escape stopped an idle session")
	}
	setSessionStatus(a, contracts.Working)
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if stops() != 1 {
		t.Fatalf("Escape dispatched %d stops for a running response", stops())
	}
}

// modEnter mode answers the platform's own modifier: Command on macOS and
// Control elsewhere, never the other platform's key.
func TestSendGuardModEnterAcceptedPerPlatform(t *testing.T) {
	for _, test := range []struct {
		name string
		mods ui.Modifiers
		goos string
		want bool
	}{
		{"command on macOS", ui.Super, "darwin", true},
		{"control on macOS", ui.Ctrl, "darwin", false},
		{"control on Windows", ui.Ctrl, "windows", true},
		{"super on Windows", ui.Super, "windows", false},
		{"control on Linux", ui.Ctrl, "linux", true},
		{"shift alone", ui.Shift, "linux", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := modEnterAccepted(test.mods, test.goos); got != test.want {
				t.Fatalf("modEnterAccepted(%v, %q) = %v, want %v", test.mods, test.goos, got, test.want)
			}
		})
	}
}

// The send key follows the same rule on the platform it runs on.
func TestSendGuardModEnterSendsOnThisPlatform(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.state.Settings.General.SendBehavior = "modEnter"
	recorder := &sendRecordingService{Service: a.service}
	a.service = recorder
	tt := ui.NewTester(a.View, 1320, 880)
	tt.Frame()
	if err := tt.Click("Message"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Ship the guard")
	// The modifier of the other platform must not send here.
	foreign := ui.Ctrl
	if runtime.GOOS == "darwin" {
		foreign = ui.Ctrl
	} else {
		foreign = ui.Super
	}
	tt.Key(foreign, ui.KeyEnter)
	tt.Frame()
	if len(recorder.commands) != 0 {
		t.Fatalf("the foreign modifier sent %d times", len(recorder.commands))
	}
	tt.Key(ui.Cmd, ui.KeyEnter)
	tt.Frame()
	if len(recorder.commands) != 1 {
		t.Fatalf("the platform modifier dispatched %d sends", len(recorder.commands))
	}
}
