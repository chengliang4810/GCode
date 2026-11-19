package design

import (
	"math"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestFrozenResolvedThemeCatalog(t *testing.T) {
	themes := Themes()
	if len(themes) != 30 {
		t.Fatalf("got %d variants, want 30", len(themes))
	}
	families := map[string]bool{}
	ids := map[string]bool{}
	for _, variant := range themes {
		t.Run(variant.ID, func(t *testing.T) {
			if ids[variant.ID] {
				t.Fatal("duplicate variant")
			}
			ids[variant.ID] = true
			families[variant.FamilyID] = true
			if variant.Source.URL == "" || variant.Source.Revision == "" || variant.Source.License == "" || !strings.HasPrefix(variant.Source.AssetHash, "sha256:") {
				t.Fatal("incomplete source provenance")
			}
			p := Palette(variant.ID, variant.Appearance == "dark")
			if p.ID != variant.ID || p.Background.A != 255 || p.Text.A != 255 || len(p.Syntax) < 12 {
				t.Fatal("incomplete semantic palette")
			}
			if p.UI().Font != "Geist" || p.UI().FontSize != UIFontSize || p.UI().Radius != 6 {
				t.Fatal("shared UI metrics changed")
			}
			for _, c := range p.ANSI {
				if c.A != 255 {
					t.Fatal("invalid terminal ANSI color")
				}
			}
		})
	}
	if len(families) != 19 {
		t.Fatalf("got %d families, want 19", len(families))
	}
}

func TestPaletteResolutionAndCopyIsolation(t *testing.T) {
	if p := Palette("not-a-theme", false); p.ID != "rcode-light" || p.Background != ui.Hex("#ffffff") {
		t.Fatal("bad light fallback")
	}
	if p := Palette("rcode", true); p.ID != "rcode-dark" || p.Background != ui.Hex("#060606") {
		t.Fatal("bad dark family resolution")
	}
	themes := Themes()
	themes[0].Colors["background"] = "#ff0000"
	if Palette(themes[0].ID, false).Background == ui.Hex("#ff0000") {
		t.Fatal("caller changed embedded registry")
	}
	first := Palette("rcode-dark", true)
	first.Syntax["keyword"] = ui.Hex("#ff0000")
	if Palette("rcode-dark", true).Syntax["keyword"] == ui.Hex("#ff0000") {
		t.Fatal("palette maps are shared")
	}
}

// The inner corners of a surface stay concentric with the outer corners they
// sit in: the inner radius is the outer radius minus the padding between
// them, never a second, unrelated corner size.
func TestSharedRadiiStayConcentric(t *testing.T) {
	controls := Controls()
	if want := controls.PopupRadius - controls.PopupPadding; controls.OptionRadius != want {
		t.Errorf("option radius %g is not concentric with the popup's %g: want %g",
			controls.OptionRadius, controls.PopupRadius, want)
	}
	if want := IslandRadius(true) - IslandCardPadding; IslandCardRadius != want {
		t.Errorf("island card radius %g is not concentric with the island's %g: want %g",
			IslandCardRadius, IslandRadius(true), want)
	}
}

// The button height is the one text-bearing control height, so it has to sit
// on the 4px geometry grid without falling below the line it must hold: a
// 14px label needs 1.5 line boxes plus the two paddings.
func TestButtonHeightStaysOnGridAndHoldsItsLabel(t *testing.T) {
	controls := Controls()
	if math.Mod(float64(controls.ButtonHeight), 4) != 0 {
		t.Errorf("button height %g is off the 4px grid", controls.ButtonHeight)
	}
	if need := TextBase*1.5 + controls.ButtonPaddingY*2; controls.ButtonHeight < need {
		t.Errorf("button height %g cannot hold a %gpx label box (%g)", controls.ButtonHeight, TextBase, need)
	}
	if math.Mod(float64(controls.TileRadius), 4) != 0 {
		t.Errorf("tile radius %g is off the 4px grid", controls.TileRadius)
	}
}

// The shared settings heading takes its size and leading from the page tokens
// at the frame's wide breakpoint. Balance only decides where a wrapped title
// breaks, so the heading keeps the height the frame's fixed leading gives it.
func TestSettingsPageHeadingFollowsTheFrameTokens(t *testing.T) {
	s := SettingsPage()
	if s.WideBreakpoint <= 0 || s.HeadingSize <= 0 || s.WideHeadingSize <= 0 {
		t.Fatalf("incomplete heading tokens: %+v", s)
	}
	for _, test := range []struct {
		name          string
		width         int
		size, leading float32
	}{
		{name: "narrow", width: 900, size: s.HeadingSize, leading: s.HeadingLineHeight},
		{name: "below-breakpoint", width: int(s.WideBreakpoint) - 1, size: s.HeadingSize, leading: s.HeadingLineHeight},
		{name: "at-breakpoint", width: int(s.WideBreakpoint), size: s.WideHeadingSize, leading: s.WideHeadingLineHeight},
		{name: "wider", width: 2048, size: s.WideHeadingSize, leading: s.WideHeadingLineHeight},
	} {
		t.Run(test.name, func(t *testing.T) {
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(Palette("rcode-light", false).UI())
				ui.Column(c).Fill().Children(func() {
					s.Heading(c, "Usage statistics", "Settings page heading")
					ui.Text(c, "Usage statistics").Label("Native font minimum").
						FontSize(TextSize(c, test.size)).FontWeight(s.HeadingWeight).FixedLineHeight(1)
				})
			}, test.width, 400)
			bounds, ok := tt.Find("Settings page heading")
			if !ok {
				t.Fatal("the shared heading did not render")
			}
			minimum, ok := tt.Find("Native font minimum")
			if !ok {
				t.Fatal("the native font sample did not render")
			}
			want := max(test.leading, minimum.H)
			if math.Abs(float64(bounds.H-want)) > 1 {
				t.Fatalf("heading height = %g, want %g (token %g, native minimum %g)", bounds.H, want, test.leading, minimum.H)
			}
			if bounds.X != s.HeadingInset {
				t.Fatalf("heading inset = %g, want %g", bounds.X, s.HeadingInset)
			}
		})
	}
}
