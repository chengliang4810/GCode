package linkui

import (
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gcode/internal/design"
	i18nview "gcode/internal/i18n/view"
	"gcode/internal/linktarget"
	"github.com/egoist/mygo/ui"
)

func linkTester(d Destination, cfg *Config, dark bool) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		c.SetTheme(design.Palette("rcode", dark).UI())
		ui.Column(c).Fill().Padding(24).Background(c.Theme().Background).Selectable().Children(func() {
			ui.RichText(c).Children(func() {
				ui.Text(c, "Read ")
				Link(c.Key("guide-link"), []ui.Span{{Text: "the guide"}}, d, *cfg)
				ui.Text(c, " before starting.")
			})
		})
	}, 420, 320)
}

func TestLinkNativeActivationAndPreference(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			var internal, external []string
			cfg := Config{
				Internal:     true,
				Open:         func(raw string) { internal = append(internal, raw) },
				OpenExternal: func(raw string) { external = append(external, raw) },
			}
			d := Destination{Raw: "https://example.com/guide", Kind: linktarget.Web}
			tt := linkTester(d, &cfg, dark)
			if err := tt.Click("the guide"); err != nil {
				t.Fatal(err)
			}
			tt.Key(0, ui.KeyEnter)
			tt.Key(0, ui.KeySpace)
			if len(internal) != 3 || len(external) != 0 {
				t.Fatalf("native click/Enter/Space: internal=%v external=%v", internal, external)
			}
			cfg.Internal = false
			tt.Frame()
			tt.Key(0, ui.KeyEnter)
			if !slices.Equal(external, []string{d.Raw}) {
				t.Fatalf("external preference: %v", external)
			}
		})
	}
}

func TestLinkMenuCopiesFullTargetAndRespectsHost(t *testing.T) {
	for _, local := range []bool{false, true} {
		t.Run(map[bool]string{false: "remote", true: "local"}[local], func(t *testing.T) {
			var opened, external, revealed string
			d := Destination{Raw: "/workspace/docs/guide.md:18", Tooltip: "/workspace/docs/guide.md:18", Kind: linktarget.File, KnownFile: true, Local: local}
			cfg := Config{
				Open: func(raw string) { opened = raw }, OpenExternal: func(raw string) { external = raw },
				Reveal: func(raw string) { revealed = raw },
			}
			tt := linkTester(d, &cfg, false)
			if err := tt.RightClick("the guide"); err != nil {
				t.Fatal(err)
			}
			want := []string{"Copy path", "Open in GCode"}
			if local {
				want = append(want, "Open in default app", "Show in folder")
			}
			for _, label := range want {
				if !slices.Contains(tt.Menu(), label) {
					t.Fatalf("missing %q in %v", label, tt.Menu())
				}
			}
			if !local && (slices.Contains(tt.Menu(), "Open in default app") || slices.Contains(tt.Menu(), "Show in folder")) {
				t.Fatalf("remote file exposes local actions: %v", tt.Menu())
			}
			if err := tt.ChooseMenuItem("Copy path"); err != nil {
				t.Fatal(err)
			}
			if tt.Clipboard() != d.Tooltip {
				t.Fatalf("clipboard=%q", tt.Clipboard())
			}
			if err := tt.Click("the guide"); err != nil {
				t.Fatal(err)
			}
			if opened != d.Raw {
				t.Fatalf("file route=%q", opened)
			}
			tt.Key(ui.Shift, ui.KeyF10)
			if err := tt.ChooseMenuItem("Open in GCode"); err != nil {
				t.Fatal(err)
			}
			if local {
				tt.Key(ui.Shift, ui.KeyF10)
				if err := tt.ChooseMenuItem("Open in default app"); err != nil {
					t.Fatal(err)
				}
				tt.Key(ui.Shift, ui.KeyF10)
				if err := tt.ChooseMenuItem("Show in folder"); err != nil {
					t.Fatal(err)
				}
				if external != d.Raw || revealed != d.Raw {
					t.Fatalf("local actions external=%q reveal=%q", external, revealed)
				}
			}
		})
	}
}

func TestLinkURLMenuAndChineseLabels(t *testing.T) {
	var opened, external string
	cfg := Config{Internal: false, Open: func(raw string) { opened = raw }, OpenExternal: func(raw string) { external = raw }}
	cfg.SetInternal = func(value bool) { cfg.Internal = value }
	d := Destination{Raw: "https://example.com/" + strings.Repeat("long/", 80), Kind: linktarget.Web}
	tt := linkTester(d, &cfg, false)
	if err := tt.RightClick("the guide"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Copy link address"); err != nil {
		t.Fatal(err)
	}
	if tt.Clipboard() != d.Raw {
		t.Fatalf("URL copy truncated: %q", tt.Clipboard())
	}
	if err := tt.RightClick("the guide"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Open in GCode"); err != nil {
		t.Fatal(err)
	}
	if opened != d.Raw {
		t.Fatalf("explicit internal route=%q", opened)
	}
	if err := tt.RightClick("the guide"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Open in external browser"); err != nil {
		t.Fatal(err)
	}
	if external != d.Raw {
		t.Fatalf("explicit external route=%q", external)
	}
	if err := tt.RightClick("the guide"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Open web links in GCode"); err != nil {
		t.Fatal(err)
	}
	if !cfg.Internal {
		t.Fatal("URL preference did not update")
	}
	zh := ui.NewTester(func(c *ui.Context) {
		i18nview.Set(c, "zh-CN")
		Link(c, []ui.Span{{Text: "指南"}}, d, cfg)
	}, 400, 300)
	if err := zh.RightClick("指南"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(zh.Menu(), "在 GCode 中打开") {
		t.Fatalf("menu is not localized: %v", zh.Menu())
	}
}

func TestLinkSelectionDoesNotNavigate(t *testing.T) {
	opened := 0
	cfg := Config{Internal: true, Open: func(string) { opened++ }}
	d := Destination{Raw: "https://example.com", Kind: linktarget.Web}
	tt := linkTester(d, &cfg, false)
	r, ok := tt.Find("the guide")
	if !ok {
		t.Fatal("link missing")
	}
	tt.Press(r.X+1, r.Y+r.H/2)
	tt.Move(r.X+r.W-1, r.Y+r.H/2)
	tt.Release(r.X+r.W-1, r.Y+r.H/2)
	if opened != 0 {
		t.Fatalf("dragging a link navigated: opens=%d", opened)
	}
	tt.Key(ui.Cmd, ui.KeyC)
	if tt.Clipboard() != "the guide" {
		t.Fatalf("link drag did not copy the original label: %q", tt.Clipboard())
	}
	read, _ := tt.Find("Read ")
	tt.Press(read.X+1, read.Y+read.H/2)
	tt.Move(r.X+r.W-1, r.Y+r.H/2)
	tt.Release(r.X+r.W-1, r.Y+r.H/2)
	tt.Key(ui.Cmd, ui.KeyC)
	if opened != 0 || !strings.Contains(tt.Clipboard(), "the guid") {
		t.Fatalf("selection navigated or lost original label: opens=%d copy=%q", opened, tt.Clipboard())
	}
}

func TestStandaloneLinkPreservesNativeSelectionAndKeys(t *testing.T) {
	opened := 0
	cfg := Config{Internal: true, Open: func(string) { opened++ }}
	d := Destination{Raw: "https://example.com", Kind: linktarget.Web}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Padding(24).Children(func() {
			Link(c, []ui.Span{{Text: "standalone label"}}, d, cfg)
		})
	}, 400, 200)
	r, _ := tt.Find("standalone label")
	tt.Press(r.X+1, r.Y+r.H/2)
	tt.Move(r.X+r.W-1, r.Y+r.H/2)
	tt.Release(r.X+r.W-1, r.Y+r.H/2)
	tt.Key(ui.Cmd, ui.KeyC)
	if opened != 0 || tt.Clipboard() != "standalone label" {
		t.Fatalf("opens=%d clipboard=%q", opened, tt.Clipboard())
	}
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeySpace)
	if opened != 2 {
		t.Fatalf("standalone native key activation=%d", opened)
	}
}

func TestLinkTooltipFocusHoverAndScroll(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			d := Destination{Raw: "https://example.com/" + strings.Repeat("nested-path/", 400), Kind: linktarget.Web}
			cfg := Config{Internal: true}
			tt := linkTester(d, &cfg, dark)
			r, _ := tt.Find("the guide")
			tt.Move(r.X+r.W/2, r.Y+r.H/2)
			tt.Advance(599 * time.Millisecond)
			if _, shown := tt.Find(d.Raw); shown {
				t.Fatal("hover tooltip appeared before delay")
			}
			tt.Advance(time.Millisecond)
			tip, ok := tt.Find(d.Raw)
			if !ok || tip.W > 360 || tip.H > 24*18+16 || tip.Y+tip.H > 320 {
				t.Fatalf("tooltip geometry=%v shown=%v", tip, ok)
			}
			x, y := tip.X+tip.W/2, tip.Y+tip.H/2
			tt.Move(x, y)
			tt.Advance(300 * time.Millisecond)
			if _, shown := tt.Find(d.Raw); !shown {
				t.Fatal("tooltip closed while moving to scrollable content")
			}
			tt.Scroll(x, y, 0, 300)
			if _, shown := tt.Find(d.Raw); !shown {
				t.Fatal("scroll closed tooltip")
			}
			writeEvidence(t, tt, map[bool]string{false: "light", true: "dark"}[dark]+"-tooltip")
			tt.Move(419, 319)
			tt.Advance(200 * time.Millisecond)
			tt.Key(0, ui.KeyTab)
			if _, shown := tt.Find(d.Raw); !shown {
				t.Fatal("keyboard focus did not show full destination")
			}
			tt.Key(0, ui.KeyEscape)
			if _, shown := tt.Find(d.Raw); shown {
				t.Fatal("Escape did not dismiss tooltip")
			}
		})
	}
}

func TestLinkTooltipThemeFontAndNarrowWindow(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, width := range []int{260, 420} {
			var previousHeight float32
			for _, fontSize := range []float32{14, 20} {
				theme := design.Palette("rcode", dark).UI()
				theme.FontSize = fontSize
				d := Destination{Raw: "https://example.com/guide", Kind: linktarget.Web}
				tt := ui.NewTester(func(c *ui.Context) {
					c.SetTheme(theme)
					c.Root().FontSize(fontSize).Background(theme.Background)
					ui.Column(c).Fill().Padding(24).Children(func() {
						Link(c.Key("guide-link"), []ui.Span{{Text: "Guide"}}, d, Config{Internal: true})
					})
				}, width, 320)
				r, ok := tt.Find("Guide")
				if !ok {
					t.Fatal("link missing")
				}
				tt.Move(r.X+r.W/2, r.Y+r.H/2)
				tt.Advance(600 * time.Millisecond)
				tip, ok := tt.Find(d.Raw)
				if !ok || tip.W > float32(width)-40 || tip.X < 0 || tip.X+tip.W > float32(width) || tip.Y+tip.H > 320 {
					t.Fatalf("dark=%v font=%v width=%v tooltip=%+v shown=%v", dark, fontSize, width, tip, ok)
				}
				if previousHeight > 0 && tip.H <= previousHeight {
					t.Fatalf("tooltip did not follow larger interface text: old=%v new=%v", previousHeight, tip.H)
				}
				previousHeight = tip.H
				pixel := tt.Image().RGBAAt(int(tip.X+12), int(tip.Y+3))
				expected := theme.Surface.Over(theme.Background)
				if pixel.R != expected.R || pixel.G != expected.G || pixel.B != expected.B {
					t.Fatalf("tooltip background does not follow theme (dark=%v): got=%v want=%v", dark, pixel, expected)
				}
				tt.Key(0, ui.KeyEscape)
				if _, shown := tt.Find(d.Raw); shown {
					t.Fatal("Escape did not dismiss themed tooltip")
				}
			}
		}
	}
}

func writeEvidence(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	directory := os.Getenv("RCODE_LINK_EVIDENCE")
	if directory == "" {
		return
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(directory, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
