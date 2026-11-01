package richtext

import (
	"math"
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func TestEditorKeyHookSeesCurrentDocumentAndConsumesEnter(t *testing.T) {
	doc := NewDocument(Text{})
	var sent string
	var state EditorState
	tt := ui.NewTester(func(c *ui.Context) {
		EditorBase(c, doc, EditorOptions{
			State: &state, NoMarkdown: true,
			OnKey: func(ev ui.InputEvent, composing bool) bool {
				if ev.Key != ui.KeyEnter {
					return false
				}
				if composing {
					return true
				}
				sent = doc.Text().String()
				doc.SetText(Text{})
				return true
			},
		}).Fill().AutoFocus()
	}, 300, 100)
	tt.Type("最新文字")
	tt.Key(0, ui.KeyEnter)
	if sent != "最新文字" || doc.Text().String() != "" || state.DisplayText != "" {
		t.Fatalf("sent %q, document %q, display %q", sent, doc.Text().String(), state.DisplayText)
	}
	tt.Type("ab")
	tt.Key(0, ui.KeyLeft)
	tt.Type("x")
	if got := doc.Text().String(); got != "axb" {
		t.Fatalf("unconsumed key did not retain native editing: %q", got)
	}
}

func TestEditorKeyHookLeavesCompositionUncommitted(t *testing.T) {
	doc := NewDocument(NewText("A"))
	var state EditorState
	seen := false
	v := view{doc: doc, caret: 1, selAnchor: 1, opts: EditorOptions{
		State: &state,
		OnKey: func(ev ui.InputEvent, composing bool) bool {
			seen = composing && ev.Key == ui.KeyEnter
			return seen
		},
	}}
	v.sync()
	v.SetMarkedText(nil, "你好", ui.TextInputRange{Start: 2, End: 2})
	if !v.input(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyEnter}) {
		t.Fatal("composition Enter was not consumed")
	}
	if !seen || doc.Text().String() != "A" || state.DisplayText != "A你好" || !state.Composing {
		t.Fatalf("seen %v, text %q, state %+v", seen, doc.Text().String(), state)
	}
	if v.compose != "你好" {
		t.Fatalf("marked text changed to %q", v.compose)
	}
}

func TestEditorPasteHookSupportsShortcutMenuAndFallthrough(t *testing.T) {
	doc := NewDocument(NewText("A"))
	taken, calls := true, 0
	var state EditorState
	tt := ui.NewTester(func(c *ui.Context) {
		EditorBase(c, doc, EditorOptions{
			State:   &state,
			OnPaste: func() bool { calls++; return taken },
		}).Fill().AutoFocus()
	}, 300, 100)
	tt.SetClipboard("paste")
	tt.Key(ui.Cmd, ui.KeyV)
	tt.Command("paste")
	if calls != 2 || doc.Text().String() != "A" || doc.CanUndo() {
		t.Fatalf("consumed paste calls %d, text %q, undo %v", calls, doc.Text().String(), doc.CanUndo())
	}
	taken = false
	tt.Command("pasteAndMatchStyle")
	if calls != 3 || doc.Text().String() != "Apaste" {
		t.Fatalf("fallthrough calls %d, text %q", calls, doc.Text().String())
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if got := doc.Text().String(); got != "A" {
		t.Fatalf("paste undo %q", got)
	}
}

func TestEditorStateMeasuresWrapsContentAndFixedLineHeight(t *testing.T) {
	doc := NewDocument(NewText("a\nb\nc"))
	var state EditorState
	width := float32(300)
	var heightSeenByView float32
	theme := &Theme{FixedLineHeight: 22.75}
	tt := ui.NewTester(func(c *ui.Context) {
		heightSeenByView = state.ContentHeight
		EditorBase(c, doc, EditorOptions{State: &state, Theme: theme, Font: ui.Font{Size: 14}}).
			Width(width).Height(35).AutoFocus()
	}, 400, 100)
	if got := state.ContentHeight; math.Abs(float64(got-3*22.75)) > 0.01 {
		t.Fatalf("three fixed-height lines measure %v", got)
	}
	if heightSeenByView != state.ContentHeight {
		t.Fatalf("measurement did not request a frame: view %v, state %v", heightSeenByView, state.ContentHeight)
	}
	doc.SetText(NewText(strings.Repeat("word ", 18)))
	tt.Frame()
	wide := state.ContentHeight
	width = 80
	tt.Frame()
	narrow := state.ContentHeight
	if narrow <= wide {
		t.Fatalf("width change did not update height: wide %v, narrow %v", wide, narrow)
	}
	if heightSeenByView != narrow {
		t.Fatalf("width measurement did not request a frame: view %v, state %v", heightSeenByView, narrow)
	}
	doc.SetText(NewText("short"))
	tt.Frame()
	if math.Abs(float64(state.ContentHeight-22.75)) > 0.01 || state.DisplayText != "short" {
		t.Fatalf("content change did not update state: %+v", state)
	}
	doc.InsertAttachment(&Attachment{Name: "Skill", Width: 30, Height: 14})
	tt.Frame()
	if !strings.ContainsRune(state.DisplayText, ObjectReplacement) || strings.Contains(state.DisplayText, "Skill") {
		t.Fatalf("display must keep the object replacement character: %q", state.DisplayText)
	}
}

func TestEditorContentPaddingSetsAsymmetricTextBounds(t *testing.T) {
	doc := NewDocument(NewText("one two three four five six"))
	var state EditorState
	var editor *view
	padding := [4]float32{16, 23, 4, 11}
	tt := ui.NewTester(func(c *ui.Context) {
		element := EditorBase(c, doc, EditorOptions{
			State: &state, ContentPadding: padding, Theme: &Theme{FixedLineHeight: 22.75},
		}).Width(160).Height(90).AutoFocus()
		editor = ui.Local(element, "richtext", func() view { return view{} })
	}, 220, 120)
	if editor.width != 126 || editor.viewH != 70 {
		t.Fatalf("content box %v x %v, want 126 x 70", editor.width, editor.viewH)
	}
	if editor.padX != 11 || editor.padY != 16 || editor.padRight != 23 || editor.padBottom != 4 {
		t.Fatalf("padding %+v", editor.opts.ContentPadding)
	}
	paddedHeight := state.ContentHeight
	padding = [4]float32{}
	tt.Frame()
	if editor.width != 160 || editor.viewH != 90 || editor.padX != 0 || editor.padY != 0 {
		t.Fatalf("default EditorBase bounds: %v x %v, padding %v,%v", editor.width, editor.viewH, editor.padX, editor.padY)
	}
	if state.ContentHeight > paddedHeight {
		t.Fatalf("removing padding increased wrap height: %v > %v", state.ContentHeight, paddedHeight)
	}
}

func TestAttachmentNamesSurvivePlainTextClipboard(t *testing.T) {
	named := Attributes{Attachment: &Attachment{Name: "Review"}}
	unnamed := Attributes{Attachment: &Attachment{}}
	text := TextOf(
		Paragraph{Runs: []Run{
			{Text: "Use "}, {Text: string(ObjectReplacement), Attributes: named},
			{Text: " and "}, {Text: string(ObjectReplacement), Attributes: unnamed},
		}},
		Paragraph{Runs: []Run{{Text: "Second "}, {Text: string(ObjectReplacement), Attributes: named}}},
	)
	want := "Use Review and \nSecond Review"
	if got := text.PlainText(); got != want {
		t.Fatalf("plain text %q, want %q", got, want)
	}
	data := clipboardData(text)
	plain, err := data.Read(transfer.Text)
	if err != nil || string(plain) != want {
		t.Fatalf("clipboard plain text %q, %v", plain, err)
	}
}
