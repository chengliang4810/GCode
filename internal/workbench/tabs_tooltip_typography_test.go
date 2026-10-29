package workbench

import (
	"fmt"
	"math"
	"testing"

	"gcode/internal/assets"

	"github.com/egoist/mygo/ui"
)

func TestTabTooltipUsesDetachedSystemFontAndSnappedLineBox(t *testing.T) {
	if err := assets.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	for _, close := range []bool{false, true} {
		for _, scale := range []float32{1, 1.25, 2} {
			for _, rootFont := range []string{"Geist", "monospace"} {
				t.Run(fmt.Sprintf("close%t-scale%g-root%s", close, scale, rootFont), func(t *testing.T) {
					s, _, _ := testWorkspace(t)
					text := "Tab02.txt"
					owner := &tabTooltipOwner{phase: tabTooltipVisible, text: text, x: 20, y: 20, close: close}
					s.tabTooltip.owners = []*tabTooltipOwner{owner}
					size, wantHeight := float32(10.5), float32(17)
					if close {
						size, wantHeight = 11, 18
					}
					if scale == 1.25 {
						wantHeight = 16.8
						if close {
							wantHeight = 17.6
						}
					}
					tt := newTabTooltipTestWindow(s, func(c *ui.Context) {
						c.SetTheme(s.palette().UI())
						c.Root().Font(rootFont).FontSize(25).FontWeight(700).LineHeight(3)
						ui.Text(c, text).Label("independent typography control").Absolute().Left(20).Top(100).
							Font(tabTooltipFontFamily()).FontSize(size).FontWeight(400).FixedLineHeight(wantHeight)
						s.renderTabTooltip(c, tabTooltipPass{visible: owner})
					}, 500, 200)
					tt.SetScale(scale)
					card, ok := tt.card(close)
					if !ok {
						t.Fatal("production tooltip card missing")
					}
					control, ok := tt.Find("independent typography control")
					if !ok {
						t.Fatal("independent text control missing")
					}
					// The production card's measured natural content must equal a
					// separately styled control, despite arbitrary owner typography.
					if math.Abs(float64(card.W-float32(math.Ceil(float64(control.W)))-20)) > .0001 || math.Abs(float64(card.H-control.H-14)) > .0001 {
						t.Fatalf("tooltip inherited owner font or unsnapped line box: card%+v control%+v; wanted content H%g", card, control, wantHeight)
					}
					if math.Abs(float64(control.H-wantHeight)) > .0001 {
						t.Fatalf("independent line control H%g, want%g", control.H, wantHeight)
					}
				})
			}
		}
	}
}
