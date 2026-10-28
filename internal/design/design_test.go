package design

import (
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
			if p.UI().Font != "Geist" || p.UI().FontSize != 13 || p.UI().Radius != 6 {
				t.Fatal("source UI metrics changed")
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
	if p := Palette("not-a-theme", false); p.ID != "zeron-light" || p.Background != ui.Hex("#ffffff") {
		t.Fatal("bad light fallback")
	}
	if p := Palette("zeron", true); p.ID != "zeron-dark" || p.Background != ui.Hex("#060606") {
		t.Fatal("bad dark family resolution")
	}
	themes := Themes()
	themes[0].Colors["background"] = "#ff0000"
	if Palette(themes[0].ID, false).Background == ui.Hex("#ff0000") {
		t.Fatal("caller changed embedded registry")
	}
	first := Palette("zeron-dark", true)
	first.Syntax["keyword"] = ui.Hex("#ff0000")
	if Palette("zeron-dark", true).Syntax["keyword"] == ui.Hex("#ff0000") {
		t.Fatal("palette maps are shared")
	}
}
