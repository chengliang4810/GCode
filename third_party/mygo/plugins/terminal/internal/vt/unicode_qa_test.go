package vt

import (
	"os"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestUnicodeQA(t *testing.T) {
	path := os.Getenv("MYGO_GHOSTTY_VT")
	if path == "" {
		t.Fatal("MYGO_GHOSTTY_VT is required")
	}
	if err := Load(path); err != nil {
		t.Fatal(err)
	}
	term, err := NewTerminal(80, 10, Effects{})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Free()
	term.SetModeDefault(ModeGraphemes, true)

	input := "ASCII|中文|世界|emoji|😀|👍🏽|👨‍👩‍👧|é́"
	term.Write([]byte(input + "\r\n"))

	text := term.Text()
	if text != input || !utf8.ValidString(text) || !strings.Contains(text, "中文") || !strings.Contains(text, "😀") {
		t.Fatalf("text=%q bytes=% x", text, []byte(text))
	}
	rows, cols := term.RowText(0)
	if got := strings.TrimRight(string(rows), " "); got != input {
		t.Fatalf("row text=%q cols=%v", got, cols)
	}

	render, err := NewRenderState()
	if err != nil {
		t.Fatal(err)
	}
	defer render.Free()
	if err := render.Update(term); err != nil {
		t.Fatal(err)
	}
	render.Rows()
	if !render.NextRow() {
		t.Fatal("missing first row")
	}
	cells := render.RowCells()
	want := []struct {
		codepoint rune
		wide      Wide
		graphemes []rune
	}{
		{'A', Narrow, []rune{'A'}}, {'S', Narrow, []rune{'S'}}, {'C', Narrow, []rune{'C'}}, {'I', Narrow, []rune{'I'}}, {'I', Narrow, []rune{'I'}}, {'|', Narrow, []rune{'|'}},
		{'中', WideChar, []rune{'中'}}, {'文', WideChar, []rune{'文'}}, {'|', Narrow, []rune{'|'}},
		{'世', WideChar, []rune{'世'}}, {'界', WideChar, []rune{'界'}}, {'|', Narrow, []rune{'|'}},
		{'e', Narrow, []rune{'e'}}, {'m', Narrow, []rune{'m'}}, {'o', Narrow, []rune{'o'}}, {'j', Narrow, []rune{'j'}}, {'i', Narrow, []rune{'i'}}, {'|', Narrow, []rune{'|'}},
		{'😀', WideChar, []rune{'😀'}}, {'|', Narrow, []rune{'|'}},
		{'👍', WideChar, []rune{'👍', '🏽'}}, {'|', Narrow, []rune{'|'}},
		{'👨', WideChar, []rune{'👨', '\u200d', '👩', '\u200d', '👧'}}, {'|', Narrow, []rune{'|'}},
		{'é', Narrow, []rune{'é', '\u0301'}},
	}
	type visibleCell struct {
		x    int
		cell Cell
	}
	var visible []visibleCell
	for x, cell := range cells {
		if cell.Wide != SpacerTail {
			visible = append(visible, visibleCell{x: x, cell: cell})
		}
	}
	if len(visible) < len(want) {
		t.Fatalf("visible cells=%d want at least %d", len(visible), len(want))
	}
	for i, w := range want {
		got := visible[i].cell
		graphemes := render.CellGraphemes(visible[i].x, nil)
		if got.Codepoint != w.codepoint || got.Wide != w.wide || got.Grapheme != (len(w.graphemes) > 1) || !slices.Equal(graphemes, w.graphemes) {
			t.Errorf("cell[%d]=%+v graphemes=%U want codepoint=U+%04X wide=%d graphemes=%U", visible[i].x, got, graphemes, w.codepoint, w.wide, w.graphemes)
		}
	}
	selection, ok := term.SelectAll()
	if !ok {
		t.Fatal("SelectAll failed")
	}
	term.SetSelection(&selection)
	selected, ok := term.SelectionText()
	if !ok || selected != input {
		t.Fatalf("selection_ok=%t selection=%q bytes=% x", ok, selected, []byte(selected))
	}
}
