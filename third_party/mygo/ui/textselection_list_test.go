package ui

import "testing"

// virtualizedSelection builds a list of rows tall enough to scroll, each row
// one selectable paragraph, inside one selectable scope.
func virtualizedSelection(t *testing.T, list *ListState, rows int) *Tester {
	t.Helper()
	list.Key = func(i int) any { return i }
	return coreNewTester(func(c *context) {
		coreColumn(c).Fill().Selectable().Children(func() {
			coreList(c, list, rows, func(i int) {
				coreTextf(c, "Row number %02d text", i).Height(24)
			}).Fill()
		})
	}, 320, 240)
}

func selectionRowBuilt(tt *Tester, row int) bool {
	for _, s := range tt.rt.texts {
		if s.editor != nil && s.editor.source == rowText(row) {
			return true
		}
	}
	return false
}

func rowText(row int) string {
	return "Row number " + twoDigits(row) + " text"
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

// A selection that reaches across rows a list later stops building keeps its
// text: copying joins the spans it resolved while they were built, so the
// paragraphs a virtualized list scrolled away still copy.
func TestListTextSelectionSurvivesRowsLeavingTheBuild(t *testing.T) {
	var list ListState
	tt := virtualizedSelection(t, &list, 60)
	x0, y0 := selectableAt(t, tt, rowText(2), 0)
	x1, y1 := selectableAt(t, tt, rowText(5), 13)
	tt.Press(x0, y0)
	tt.Move(x1, y1)
	tt.Release(x1, y1)
	if tt.rt.selection.scope == 0 {
		t.Fatal("the drag selected nothing")
	}
	list.ScrollToEnd()
	for range 3 {
		tt.Frame()
	}
	if selectionRowBuilt(tt, 3) || selectionRowBuilt(tt, 4) || selectionRowBuilt(tt, 5) {
		t.Fatal("the selected rows are still built, so nothing was tested")
	}
	if tt.rt.selection.scope == 0 {
		t.Fatal("the selection was dropped when its rows left the build")
	}
	tt.Key(Cmd, KeyC)
	want := rowText(2) + "\n" + rowText(3) + "\n" + rowText(4) + "\nRow number 05"
	if got := tt.Clipboard(); got != want {
		t.Fatalf("copied %q, want %q", got, want)
	}
}

// Scrolling back to a selection paints it again from the spans the selection
// kept, without the drag that made it.
func TestListTextSelectionPaintsAgainWhenItsRowsReturn(t *testing.T) {
	var list ListState
	tt := virtualizedSelection(t, &list, 60)
	x0, y0 := selectableAt(t, tt, rowText(2), 0)
	x1, y1 := selectableAt(t, tt, rowText(4), len("Row number 04"))
	tt.Press(x0, y0)
	tt.Move(x1, y1)
	tt.Release(x1, y1)
	list.ScrollToEnd()
	for range 3 {
		tt.Frame()
	}
	list.ScrollTo(0, Start)
	for range 3 {
		tt.Frame()
	}
	for _, row := range []int{3, 4} {
		var painted bool
		for _, s := range tt.rt.texts {
			if s.editor != nil && s.editor.source == rowText(row) && s.editor.anchor != s.editor.caret {
				painted = true
			}
		}
		if !painted {
			t.Fatalf("row %d came back without its part of the selection", row)
		}
	}
}
