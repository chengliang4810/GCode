package design

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

// activeAlpha is the theme-owned alpha of `colors.active`: 1a in light
// variants, 2e in dark ones. An accent may change its hue, never its alpha.
func activeAlpha(dark bool) uint8 {
	if dark {
		return 0x2e
	}
	return 0x1a
}

func mutedRoles(t Tokens) [3]ui.Color {
	return [3]ui.Color{t.DangerMuted, t.WarningMuted, t.SuccessMuted}
}

var mutedRoleKeys = [3]string{"dangerMuted", "warningMuted", "successMuted"}

func TestAccentMatrixDerivesActiveHueAndKeepsThemeRoles(t *testing.T) {
	presets := slices.Sorted(maps.Keys(accentPresets))
	if len(presets) != 7 {
		t.Fatalf("accent preset table has %d entries, want 7", len(presets))
	}
	themes := Themes()
	if len(themes) != 30 {
		t.Fatalf("Themes() returned %d variants, want 30", len(themes))
	}
	corrected := 0
	for _, variant := range themes {
		dark := variant.Appearance == "dark"
		original := Palette(variant.ID, dark)
		if got := original.Active.A; got != activeAlpha(dark) {
			t.Errorf("%s: theme active alpha is %#x, want %#x", variant.ID, got, activeAlpha(dark))
		}
		for _, preset := range presets {
			accented := ApplyAccent(original, preset)
			index := 1
			if dark {
				index = 0
			}
			if accented.Accent != ui.Hex(accentPresets[preset][index]) {
				corrected++
			}
			t.Run(variant.ID+"/"+preset, func(t *testing.T) {
				// 9.3: the pressed surface keeps the theme alpha and takes the
				// corrected accent's hue, so all 30 themes follow all 7 accents.
				if accented.Active.A != original.Active.A {
					t.Fatalf("active alpha drifted: %#x -> %#x", original.Active.A, accented.Active.A)
				}
				if accented.Active.R != accented.Accent.R || accented.Active.G != accented.Accent.G || accented.Active.B != accented.Accent.B {
					t.Fatalf("active hue (%d,%d,%d) does not follow the corrected accent (%d,%d,%d)",
						accented.Active.R, accented.Active.G, accented.Active.B, accented.Accent.R, accented.Accent.G, accented.Accent.B)
				}
				if got := contrast(accented.Accent, accented.Background); got < 3 {
					t.Fatalf("accent contrast against the background is %.2f, want >= 3", got)
				}
				// The muted semantic roles stay owned by the theme.
				for i, got := range mutedRoles(accented) {
					want := ui.Hex(variant.Colors[mutedRoleKeys[i]])
					if got == (ui.Color{}) {
						t.Fatalf("%s is unpainted", mutedRoleKeys[i])
					}
					if got != want || got != mutedRoles(original)[i] {
						t.Fatalf("%s is %v, want the unchanged theme role %v", mutedRoleKeys[i], got, want)
					}
				}
			})
		}
	}
	if corrected == 0 {
		t.Log("no preset needed contrast correction on the current backgrounds")
	}
	t.Logf("checked %d theme/accent combinations, %d of them contrast-corrected", len(themes)*len(presets), corrected)
}

// TestAccentHueFollowsCorrectedAccent covers the path where ensureContrast
// rewrites the preset before it reaches the tokens.
func TestAccentHueFollowsCorrectedAccent(t *testing.T) {
	background := ui.Hex("#ffffff")
	corrected := ensureContrast(ui.Hex("#fbbf24"), background, 3)
	if corrected == ui.Hex("#fbbf24") {
		t.Fatal("the sample color was not contrast-corrected")
	}
	base := ui.Color{R: 11, G: 22, B: 33, A: 0x1a}
	hue := accentedHue(corrected, base)
	if hue.R != corrected.R || hue.G != corrected.G || hue.B != corrected.B {
		t.Fatalf("hue %v does not follow the corrected accent %v", hue, corrected)
	}
	if hue.A != base.A {
		t.Fatalf("alpha drifted to %#x, want %#x", hue.A, base.A)
	}
}

func TestPalettePaintsMutedSemanticRoles(t *testing.T) {
	for _, variant := range Themes() {
		plain := Palette(variant.ID, variant.Appearance == "dark")
		for i, got := range mutedRoles(plain) {
			want := ui.Hex(variant.Colors[mutedRoleKeys[i]])
			if got == (ui.Color{}) || got != want {
				t.Fatalf("%s: %s is %v, want the theme role %v", variant.ID, mutedRoleKeys[i], got, want)
			}
		}
	}
}

func TestThemeRegistryDropsDeadColorKeys(t *testing.T) {
	var probe struct {
		Families []struct {
			Variants []struct {
				ID     string                     `json:"id"`
				Colors map[string]json.RawMessage `json:"colors"`
				Accent map[string]json.RawMessage `json:"accent"`
			} `json:"variants"`
		} `json:"families"`
	}
	if err := json.Unmarshal(registryJSON, &probe); err != nil {
		t.Fatal(err)
	}
	families, count := 0, 0
	for _, family := range probe.Families {
		families++
		for _, variant := range family.Variants {
			count++
			if _, ok := variant.Colors["cursor"]; ok {
				t.Fatalf("%s still declares the dead colors.cursor key", variant.ID)
			}
			for _, dead := range []string{"wash", "activity"} {
				if _, ok := variant.Accent[dead]; ok {
					t.Fatalf("%s still declares the dead accent.%s key", variant.ID, dead)
				}
			}
			if len(variant.Colors) != 26 {
				t.Fatalf("%s has %d colors keys, want 26", variant.ID, len(variant.Colors))
			}
			if len(variant.Accent) != 6 {
				t.Fatalf("%s has %d accent keys, want 6", variant.ID, len(variant.Accent))
			}
		}
	}
	if families != 19 || count != 30 {
		t.Fatalf("registry has %d families and %d variants, want 19 and 30", families, count)
	}
}

// The native theme carries the base colors only, so the muted tones a shared
// control needs are resolved from the catalog by the semantic roles the
// theme exposes. A theme outside the catalog keeps the base colors.
func TestSemanticTonesResolveTheThemesMutedRoles(t *testing.T) {
	for _, variant := range Themes() {
		plain := Palette(variant.ID, variant.Appearance == "dark")
		tones := SemanticTonesFor(plain.UI())
		if tones.Danger != plain.Danger || tones.Warning != plain.Warning || tones.Success != plain.Success {
			t.Fatalf("%s: base colors drifted: %+v", variant.ID, tones)
		}
		if tones.DangerMuted != plain.DangerMuted || tones.WarningMuted != plain.WarningMuted ||
			tones.SuccessMuted != plain.SuccessMuted {
			t.Fatalf("%s: muted roles %v/%v/%v, want the theme roles %v/%v/%v", variant.ID,
				tones.DangerMuted, tones.WarningMuted, tones.SuccessMuted,
				plain.DangerMuted, plain.WarningMuted, plain.SuccessMuted)
		}
	}
	outside := ui.DarkTheme()
	tones := SemanticTonesFor(outside)
	if tones.DangerMuted != outside.Danger || tones.WarningMuted != outside.Warning || tones.SuccessMuted != outside.Success {
		t.Fatalf("a theme outside the catalog invented a muted tone: %+v", tones)
	}
}
