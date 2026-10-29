package design

import (
	"math"
	"testing"

	"gcode/internal/assets"
	"github.com/egoist/mygo/ui"
)

func TestReferenceTypographyMatchesFrozenWindowsAdvances(t *testing.T) {
	if err := assets.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(func(c *ui.Context) {
		c.Root().Font("Geist").FontSize(13).FontWeight(500)
		ApplyReferenceTypography(c)
		ui.Column(c).AlignItems(ui.Start).Children(func() {
			for _, label := range []string{"Context window", "Standard"} {
				ui.Text(c, label).Label(label + " inherited")
				ui.Text(c, label).FontFeatures("kern=1").Label(label + " kerned")
			}
		})
	}, 400, 200)
	// Independently measured with the frozen GPUI DirectWrite Typography path
	// and the original static Geist Medium face, at 13 DIPs / weight 500.
	for _, tc := range []struct {
		label string
		width float32
	}{{"Context window", 101.907}, {"Standard", 57.135}} {
		inherited, ok := tt.Find(tc.label + " inherited")
		if !ok || math.Abs(float64(inherited.W-tc.width)) > .02 {
			t.Fatalf("%s inherited advance = %.4f, frozen Windows advance %.4f", tc.label, inherited.W, tc.width)
		}
		kerned, ok := tt.Find(tc.label + " kerned")
		if !ok || kerned.W >= inherited.W-1 {
			t.Fatalf("%s explicit kerning did not override the inherited default: inherited=%+v kerned=%+v", tc.label, inherited, kerned)
		}
	}
}
