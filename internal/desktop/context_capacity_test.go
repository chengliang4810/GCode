package desktop

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	localize "gcode/internal/i18n/view"
	"github.com/egoist/mygo/ui"
)

func TestContextCapacityReferenceLayout(t *testing.T) {
	usage := contracts.ContextUsage{Tokens: footerNumber(329000), Window: footerNumber(1000000)}
	if got := footerContextSummary(usage, "zh-CN"); got != "32.9万/100万 (32.9%)" {
		t.Fatal(got)
	}
	for _, dark := range []bool{false, true} {
		a := testApp(t, "default")
		a.state.Settings.General.Language = "zh-CN"
		tt := ui.NewTester(func(c *ui.Context) {
			a.palette = design.Palette("rcode", dark)
			c.SetTheme(a.palette.UI())
			localize.Set(c, "zh-CN")
			c.Root().Font("Geist").FontSize(13).Background(a.palette.Background)
			ui.Column(c).Width(360).Children(func() { a.footerContextCard(c, usage) })
		}, 360, 100)
		tt.Advance(time.Second)
		bar, ok := tt.Find("context-capacity-progress")
		if !ok || bar.W != 344 || bar.H != 8 {
			t.Fatalf("progress geometry: %+v", bar)
		}
		if dir := os.Getenv("RCODE_CONTEXT_EVIDENCE"); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			name := "light.png"
			if dark {
				name = "dark.png"
			}
			f, err := os.Create(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, tt.Image())
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}
