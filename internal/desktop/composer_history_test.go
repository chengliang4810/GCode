package desktop

import (
	"strings"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func composerHistoryMessage(role, text string) contracts.Message {
	return contracts.Message{Role: role, Blocks: []contracts.Block{{Kind: "text", Text: text}}}
}

func TestComposerPromptHistoryNavigation(t *testing.T) {
	a := testApp(t, "default")
	a.sessionID = "history"
	ref := (contracts.PromptReference{Kind: "skill", ID: "docs", Label: "文档"}).Markdown()
	const oldest = "最早的输入🙂"
	latest := "检查 " + ref + "\n/go"
	draft := "未发送 " + ref
	a.state.Messages[a.sessionID] = []contracts.Message{
		composerHistoryMessage("user", oldest),
		composerHistoryMessage("assistant", "not a prompt"),
		{Role: "user", Blocks: []contracts.Block{{Kind: "text", Text: "检查 " + ref}, {Kind: "attachment", Text: "do not recall"}, {Kind: "text", Text: "/go"}}},
		composerHistoryMessage("user", " \n "),
		{Role: "user", Blocks: []contracts.Block{{Kind: "attachment", Text: "only attachment"}}},
	}
	a.draft = draft
	sends, stops := 0, 0
	tt := promptTester(t, a, 600, &sends, &stops)
	if err := tt.Click("Draft"); err != nil {
		t.Fatal(err)
	}
	a.prompt.doc.SetSelection(2, 2)
	tt.Frame()
	for _, step := range []struct {
		key  ui.Key
		want string
	}{
		{ui.KeyDown, draft},
		{ui.KeyUp, latest},
		{ui.KeyUp, oldest},
		{ui.KeyUp, oldest},
		{ui.KeyDown, latest},
		{ui.KeyDown, draft},
		{ui.KeyDown, draft},
	} {
		tt.Key(0, step.key)
		if got := a.draft; got != step.want {
			t.Fatalf("key %v: draft=%q, want %q", step.key, got, step.want)
		}
		if step.want == latest && (a.prompt.open || !strings.ContainsRune(a.prompt.doc.Text().String(), '\ufffc')) {
			t.Fatal("recalled prompt lost its atomic reference or opened the picker")
		}
	}
	// The first Down is native and can move a single-line caret to the end.
	// Check restoration separately without a native Down before or after it.
	a.prompt.doc.SetSelection(2, 2)
	tt.Frame()
	tt.Key(0, ui.KeyUp)
	tt.Key(0, ui.KeyDown)
	if start, end := a.prompt.doc.Selection(); start != 2 || end != 2 || a.draft != draft {
		t.Fatalf("draft or caret was not restored: %q [%d,%d]", a.draft, start, end)
	}
	if sends != 0 || stops != 0 {
		t.Fatalf("arrows triggered send or stop: %d, %d", sends, stops)
	}
}

func TestComposerPromptHistoryEditingAndScope(t *testing.T) {
	a := testApp(t, "default")
	a.sessionID = "first"
	a.state.Messages["first"] = []contracts.Message{composerHistoryMessage("user", "first input")}
	a.state.Messages["second"] = []contracts.Message{composerHistoryMessage("user", "second input")}
	sends, stops := 0, 0
	tt := promptTester(t, a, 360, &sends, &stops)
	if err := tt.Click("Draft"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyUp)
	tt.Type(" edited")
	// An edit ends the browse; the next Up takes a fresh snapshot and
	// saves the edited text as the new draft.
	a.state.Messages["first"] = append(a.state.Messages["first"], composerHistoryMessage("user", "new input"))
	tt.Key(0, ui.KeyDown)
	if a.draft != "first input edited" {
		t.Fatalf("Down replaced edited draft: %q", a.draft)
	}
	tt.Key(0, ui.KeyUp)
	if a.draft != "new input" {
		t.Fatalf("editing did not restart history: %q", a.draft)
	}
	tt.Key(0, ui.KeyDown)
	if a.draft != "first input edited" {
		t.Fatalf("edited draft was not restored: %q", a.draft)
	}
	tt.Key(0, ui.KeyUp)
	a.sessionID, a.draft = "second", ""
	tt.Frame()
	tt.Key(0, ui.KeyDown)
	if a.draft != "" {
		t.Fatalf("previous session draft leaked: %q", a.draft)
	}
	tt.Key(0, ui.KeyUp)
	if a.draft != "second input" {
		t.Fatalf("previous session history leaked: %q", a.draft)
	}
	a.sessionID, a.draft = "", ""
	tt.Frame()
	tt.Key(0, ui.KeyUp)
	if a.draft != "" {
		t.Fatalf("new conversation recalled an existing prompt: %q", a.draft)
	}
}

func TestComposerPromptHistoryUsesSideConversation(t *testing.T) {
	a, tt, child := sideViewTester(t)
	a.state.Messages[a.sessionID] = []contracts.Message{composerHistoryMessage("user", "central input")}
	a.state.Messages[child.ID] = []contracts.Message{composerHistoryMessage("user", "child input")}
	tt.Frame()
	if err := tt.Click("Message to " + child.Title); err != nil {
		t.Fatal(err)
	}
	tt.Type("Side draft")
	tt.Key(0, ui.KeyUp)
	state := a.sideComposer(child.ID)
	if state.Draft != "child input" || a.draft != "Preserve the central draft" {
		t.Fatalf("side recall crossed conversation scopes: side=%q central=%q", state.Draft, a.draft)
	}
	tt.Key(0, ui.KeyDown)
	if state.Draft != "Side draft" {
		t.Fatalf("side draft was not restored: %q", state.Draft)
	}
}

func TestComposerPromptHistoryKeepsNativeEditingAndPicker(t *testing.T) {
	a := testApp(t, "default")
	a.sessionID = "history"
	a.state.Messages[a.sessionID] = []contracts.Message{composerHistoryMessage("user", "saved input")}
	sends, stops := 0, 0
	tt := promptTester(t, a, 360, &sends, &stops)
	if err := tt.Click("Draft"); err != nil {
		t.Fatal(err)
	}
	for _, draft := range []string{"line one\nline two", strings.Repeat("wrapped text ", 10)} {
		a.draft = draft
		tt.Frame()
		end := a.prompt.doc.Text().Len()
		a.prompt.doc.SetSelection(end, end)
		tt.Frame()
		tt.Key(0, ui.KeyUp)
		if start, caret := a.prompt.doc.Selection(); caret >= end || a.draft != draft || start != caret {
			t.Fatalf("Up did not move within multiline draft: %q [%d,%d]", a.draft, start, caret)
		}
		tt.Key(ui.Shift, ui.KeyDown)
		if start, caret := a.prompt.doc.Selection(); start == caret || a.draft != draft {
			t.Fatalf("Shift+Down did not select within draft: %q [%d,%d]", a.draft, start, caret)
		}
		tt.Key(0, ui.KeyUp)
		if a.draft != draft {
			t.Fatalf("Up replaced a selection: %q", a.draft)
		}
	}
	a.draft = ""
	tt.Frame()
	tt.Type("/")
	if !a.prompt.open || len(a.prompt.choices) < 2 {
		t.Fatal("prompt choices did not open")
	}
	tt.Key(0, ui.KeyUp)
	if a.draft != "/" || !a.prompt.open || a.prompt.selected != len(a.prompt.choices)-1 {
		t.Fatalf("picker did not receive Up: %q, selected %d", a.draft, a.prompt.selected)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Compose("你好", 2)
	tt.Key(0, ui.KeyUp)
	if a.draft != "/" || !a.prompt.state.Composing {
		t.Fatalf("Up disturbed IME composition: %q, composing=%v", a.draft, a.prompt.state.Composing)
	}
}
