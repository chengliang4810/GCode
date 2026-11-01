package desktop

import (
	"fmt"
	"math"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

// These are 1x visible integer-DIP layout checks. The original native
// inspector's 13px boxes are recorded in reference/validation/text-bounds.json;
// fractional transforms, offscreen text origins and other DPI are not asserted.
func TestModelOptionTextTracksVisibleIntegerRowsAtOneX(t *testing.T) {
	{
		for _, size := range []struct {
			font, line, standardTop float32
		}{
			{12, 19, 5}, {13, 21, 4}, {14, 23, 3},
			{15, 24, 3}, {16, 26, 2}, {17, 28, 1},
			{18, 29, 0}, {19, 31, -1}, {20, 32, -1},
		} {
			t.Run(fmt.Sprintf("font=%g", size.font), func(t *testing.T) {
				a := modelSurfaceTestApp(t, "dark")
				for i := range a.state.Models {
					if a.state.Models[i].ID == a.modelID {
						// Keep the measured first row away from the Scroll's final
						// clip edge when a large font exceeds the compact row.
						a.state.Models[i].Options = append(a.state.Models[i].Options, contracts.ModelOption{ID: "second", Label: "Second option", Choices: []string{"Sentinel"}, DefaultChoice: "Sentinel"})
					}
				}
				origin := ui.Point{X: 13, Y: 40}
				var advance float32
				tt := ui.NewTester(func(c *ui.Context) {
					theme := a.palette.UI()
					theme.Font, theme.FontSize = "Geist", size.font
					c.SetTheme(theme)
					c.Root().Font(theme.Font).FontSize(size.font).FontWeight(500)
					design.ApplyReferenceTypography(c)
					advance, _ = c.MeasureText(0, ui.Span{Text: "Standard", Font: theme.Font, Size: size.font, Weight: 500, Features: "kern=0"})
					ui.Column(c).Absolute().Left(origin.X).Top(origin.Y).Width(246).Children(func() {
						a.centralControls().modelOptionRows(c, a.centralControls().selectedModel(), "serviceTier")
					})
				}, 840, 700)
				for _, next := range []ui.Point{{X: 13, Y: 40}, {X: 27, Y: 103}, {X: 8, Y: 451}, {X: 103, Y: 502}, {X: 13, Y: 40}} {
					origin = next
					tt.Frame()
					row, rowOK := tt.Find("Context window")
					value, valueOK := tt.Find("Standard")
					top, height := size.standardTop, float32(30)
					width := float32(math.Ceil(float64(advance)))
					want := ui.Rect{X: row.X + row.W - 8 - 12 - 10 - width, Y: row.Y + top, W: width, H: size.line}
					if !rowOK || !valueOK || row.H != height || value != want {
						t.Fatalf("integer translation/font must use source centered, snapped text boxes: origin=%+v row=%+v value=%+v want=%+v", origin, row, value, want)
					}
					if tt.FramePending() {
						t.Fatal("option geometry should settle without geometry-feedback redraws")
					}
				}
			})
		}
	}
}

func TestSideModelOptionNativeTextKeepsSourceFlexAndChoiceIDs(t *testing.T) {
	a := testApp(t, "transcript")
	child := sideSession(t, a, "showcase-agent")
	option := contracts.ModelOption{
		ID: "contextWindow", Label: "Context window", Choices: []string{"standard", "1m"}, DefaultChoice: "standard",
		ChoiceLabels: map[string]string{"standard": "Standard", "1m": "1M tokens"},
	}
	for i := range a.state.Models {
		if a.state.Models[i].ID == child.ModelID {
			a.state.Models[i].Options = []contracts.ModelOption{option}
		}
	}
	a = modelSnapshotApp(t, a.state)
	state := a.sideComposer(child.ID)
	state.modelID = child.ModelID
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.palette.UI())
		c.Root().Font("Geist").FontSize(13).FontWeight(500)
		design.ApplyReferenceTypography(c)
		ui.Column(c).Absolute().Left(13).Top(451).Width(246).Children(func() {
			controller := a.sideControls(child.ID, state)
			controller.modelOptionRows(c, controller.selectedModel())
		})
	}, 840, 560)
	label, labelOK := tt.Find("Context window")
	value, valueOK := tt.Find("Standard")
	if !labelOK || !valueOK || label != (ui.Rect{X: 21, Y: 458, W: 136, H: 23}) || value != (ui.Rect{X: 167, Y: 458, W: 62, H: 23}) {
		t.Fatalf("native label must grow into the source remainder beside the ceil-width value: label=%+v value=%+v", label, value)
	}
	mainModel := a.modelID
	if err := tt.Click("Context window in " + child.Title); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("1M tokens"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("1M tokens") || a.state.Settings.ModelPicker.Options[child.ModelID+"/contextWindow"] != "1m" || a.modelID != mainModel {
		t.Fatal("text geometry must retain the native choice menu, business ID and side ownership")
	}
	// A value longer than the source's 100px cap still gives the label the
	// remaining space; it does not enlarge the popup or replace native text.
	for i := range a.state.Models {
		if a.state.Models[i].ID == child.ModelID {
			a.state.Models[i].Options[0].ChoiceLabels["1m"] = "A context window longer than one hundred pixels"
		}
	}
	tt.Frame()
	value, ok := tt.Find("A context window longer than one hundred pixels")
	label, _ = tt.Find("Context window")
	if !ok || value.W != 100 || label.W != 98 || value.X != 129 || label.X != 21 {
		t.Fatalf("long option value lost its source cap/flex remainder: label=%+v value=%+v", label, value)
	}
}
