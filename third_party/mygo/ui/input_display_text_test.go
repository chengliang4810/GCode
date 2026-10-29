package ui

import (
	"strings"
	"testing"
)

func TestInputDisplayTextIncludesUncommittedComposition(t *testing.T) {
	draft := "before after"
	shown, composing, selectCaret := "", false, false
	tt := NewTester(func(c *Context) {
		input := TextAreaBase(c, &draft).Label("Draft").Width(300)
		if selectCaret {
			input.SetTextSelection(6, 6)
			selectCaret = false
		}
		shown, composing = input.InputDisplayText(), input.Composing()
		if again := input.InputDisplayText(); again != shown {
			t.Fatalf("a second read changed display text: %q, %q", shown, again)
		}
	}, 400, 300)
	if shown != draft {
		t.Fatalf("initial display %q, want %q", shown, draft)
	}
	if err := tt.Click("Draft"); err != nil {
		t.Fatal(err)
	}
	selectCaret = true
	tt.Frame()
	tt.Compose("中文🙂", 3)
	snapshot := shown
	if shown != "before中文🙂 after" || draft != "before after" || !composing {
		t.Fatalf("read committed or changed composition: display %q, draft %q, composing %v", shown, draft, composing)
	}
	tt.Frame()
	if shown != snapshot || draft != "before after" || !composing {
		t.Fatal("a subsequent frame changed preedit while only reading it")
	}
	tt.Type("中文🙂")
	if shown != snapshot || draft != snapshot || composing {
		t.Fatalf("native commit changed by read API: display %q, draft %q, composing %v", shown, draft, composing)
	}
	tt.Command("undo")
	if draft != "before after" || shown != draft || snapshot != "before中文🙂 after" {
		t.Fatalf("read changed undo or retained snapshot: draft %q, display %q, snapshot %q", draft, shown, snapshot)
	}
	tt.Compose("cancel", 6)
	tt.Compose("", 0)
	if shown != draft || composing {
		t.Fatalf("cancelled composition still displayed: %q", shown)
	}
}

func TestInputDisplayTextMasksPasswordsAndIgnoresOtherElements(t *testing.T) {
	secret, shown := "sëcret", ""
	tt := NewTester(func(c *Context) {
		input := TextInputBase(c, &secret).Password().Label("Password").Width(200)
		shown = input.InputDisplayText()
		if Text(c, "Visible label").InputDisplayText() != "" || (Element{}).InputDisplayText() != "" {
			t.Fatal("non-input returned editable display text")
		}
	}, 400, 200)
	if shown != strings.Repeat("•", 6) || secret != "sëcret" {
		t.Fatalf("password display %q exposed or changed %q", shown, secret)
	}
	if err := tt.Click("Password"); err != nil {
		t.Fatal(err)
	}
	tt.Compose("中文", 2)
	if shown != strings.Repeat("•", 8) || secret != "sëcret" {
		t.Fatalf("password composition exposed or changed text: %q", shown)
	}
}

func TestInputDisplayTextRejectsReusedNonInputState(t *testing.T) {
	for _, selectable := range []bool{false, true} {
		name := "ordinary text"
		if selectable {
			name = "selectable text"
		}
		t.Run(name, func(t *testing.T) {
			draft, shown, editable := "old draft", "", true
			tt := NewTester(func(c *Context) {
				c.Key("reused")
				if editable {
					shown = TextInputBase(c, &draft).Width(200).InputDisplayText()
					return
				}
				label := Text(c, "Replacement label")
				if selectable {
					label.Selectable()
				}
				shown = label.InputDisplayText()
			}, 400, 200)
			if shown != draft {
				t.Fatalf("input snapshot %q, want %q", shown, draft)
			}
			editable = false
			tt.Frame()
			if shown != "" {
				t.Fatalf("non-input reused an editor snapshot: %q", shown)
			}
		})
	}
}
