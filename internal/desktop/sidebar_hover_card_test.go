package desktop

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The sidebar hover card must appear beside the row it describes, at that
// row's height: the tooltip content lives inside the tip and attaches to
// the row's end, never floating elsewhere in the window.
func TestSidebarHoverCardBesideItsRow(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	hover := func(rowLabel, cardLine string) {
		t.Helper()
		row, ok := tt.Find(rowLabel)
		if !ok {
			t.Fatalf("%s row not found", rowLabel)
		}
		tt.Move(row.X+row.W/2, row.Y+row.H/2)
		tt.Advance(700_000_000)
		line, ok := tt.Find(cardLine)
		if !ok {
			t.Fatalf("%s hover card missing for %s", cardLine, rowLabel)
		}
		if line.X < row.X+row.W {
			t.Errorf("%s card at x=%g starts before the row ends at x=%g", rowLabel, line.X, row.X+row.W)
		}
		// The card top aligns with the row; its lines sit within roughly
		// one card title below it, not wherever the window corner is.
		if line.Y < row.Y-1 || line.Y > row.Y+row.H+48 {
			t.Errorf("%s card line at y=%g is not at the row's height y=%g", rowLabel, line.Y, row.Y)
		}
	}
	hover("Add deployment status", "fieldnotes/add-deployment-status")
	// Project rows share the card; hover one away from the first hover.
	tt.Move(660, 60)
	tt.Advance(500_000_000)
	hover("Fieldnotes", "~/projects/fieldnotes")
}

// The row's own content reads on the row's vertical center line, not low
// under the top padding.
func TestSidebarRowContentVerticallyCentered(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	row, ok := tt.Find("Fix authentication redirects")
	if !ok {
		t.Fatal("row not found")
	}
	age, ok := tt.Find("35m")
	if !ok {
		t.Fatal("row age text not found")
	}
	if offset := age.Y + age.H/2 - (row.Y + row.H/2); offset != 0 {
		t.Errorf("row content sits %g DIPs off the row's vertical center", offset)
	}
}
