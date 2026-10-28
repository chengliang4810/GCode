// Package design maps Zeron's resolved, versioned semantic palette onto native MyGo widgets.
package design

import (
	_ "embed"
	"encoding/json"
	"maps"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

// SourceCommit is the sole visual source baseline for this implementation.
const SourceCommit = "ab189159f10cf8e63c0502be75a59bbc487e0eaf"

// Dimensions are logical pixels, extracted from crates/ui/src/{settings,theme,lib}.rs.
const (
	WindowWidth         float32 = 1320
	WindowHeight        float32 = 880
	WindowMinWidth      float32 = 900
	WindowMinHeight     float32 = 600
	TitlebarHeight      float32 = 38
	TitlebarTopPadding  float32 = 4
	StatusHeight        float32 = 24
	SidebarWidth        float32 = 256
	SidebarMin          float32 = 242
	SidebarMax          float32 = 400
	FilesWidth          float32 = 286
	FilesMin            float32 = 220
	FilesMax            float32 = 440
	RightWidth          float32 = 520
	RightMin            float32 = 360
	ChatMin             float32 = 300
	TranscriptWidth     float32 = 736
	TranscriptMin       float32 = 560
	TranscriptMax       float32 = 1200
	TerminalHeight      float32 = 280
	TerminalMin         float32 = 160
	TerminalMaxFraction float32 = .55
	BubbleRadius        float32 = 16
	PanelRadius         float32 = 10
	ControlRadius       float32 = 6
	SpaceXS             float32 = 4
	SpaceSM             float32 = 8
	SpaceMD             float32 = 12
	SpaceLG             float32 = 16
	UIFontSize          float32 = 16
	CodeFontSize        float32 = 12.5
	TerminalFontSize    float32 = 13
)

//go:embed themes.json
var registryJSON []byte

// Source records the provenance that accompanies each curated Zeron theme.
type Source struct {
	Format    string `json:"format"`
	URL       string `json:"url"`
	Revision  string `json:"revision"`
	License   string `json:"license"`
	AssetHash string `json:"assetHash"`
}

type accent struct {
	Primary   string    `json:"primary"`
	Strong    string    `json:"strong"`
	Wash      string    `json:"wash"`
	On        string    `json:"on"`
	Selection string    `json:"selection"`
	Caret     string    `json:"caret"`
	Activity  string    `json:"activity"`
	Glyph     [3]string `json:"glyph"`
}

type terminal struct {
	Background string     `json:"background"`
	Foreground string     `json:"foreground"`
	Selection  string     `json:"selection"`
	ANSI       [16]string `json:"ansi"`
}

// Variant contains an unmodified resolved source palette and its provenance.
type Variant struct {
	ID                          string            `json:"id"`
	FamilyID                    string            `json:"familyId"`
	Name                        string            `json:"name"`
	Appearance                  string            `json:"appearance"`
	RecommendedSurfaceTreatment string            `json:"recommendedSurfaceTreatment"`
	Colors                      map[string]string `json:"colors"`
	Accent                      accent            `json:"accent"`
	Syntax                      map[string]string `json:"syntax"`
	Terminal                    terminal          `json:"terminal"`
	Source                      Source            `json:"source"`
}

var once sync.Once
var variants []Variant

func load() {
	once.Do(func() {
		var registry struct {
			Families []struct {
				Variants []Variant `json:"variants"`
			} `json:"families"`
		}
		if err := json.Unmarshal(registryJSON, &registry); err != nil {
			panic("invalid embedded Zeron theme registry: " + err.Error())
		}
		for _, family := range registry.Families {
			variants = append(variants, family.Variants...)
		}
	})
}

// Themes returns independent copies of all thirty source variants.
func Themes() []Variant {
	load()
	result := append([]Variant(nil), variants...)
	for i := range result {
		result[i].Colors = maps.Clone(result[i].Colors)
		result[i].Syntax = maps.Clone(result[i].Syntax)
	}
	return result
}

// Tokens names the semantic surfaces used by the replica, without losing original roles.
type Tokens struct {
	ID, Name                                                  string
	Dark                                                      bool
	Frosted                                                   bool
	Background, Shell, Raised, Card, Dialog, Overlay          ui.Color
	Text, Muted, Faint, Solid, OnSolid                        ui.Color
	Accent, AccentStrong, AccentOn, Selection, Caret          ui.Color
	Border, BorderStrong, Hover, Active, Input                ui.Color
	Danger, Warning, Success, DiffAdd, DiffDelete, DiffHunk   ui.Color
	TerminalBackground, TerminalForeground, TerminalSelection ui.Color
	Glyph                                                     [3]ui.Color
	ANSI                                                      [16]ui.Color
	Syntax                                                    map[string]ui.Color
}

// Palette resolves a variant id or family id, falling back to Zeron in the requested appearance.
func Palette(themeID string, dark bool) Tokens {
	load()
	appearance := "light"
	if dark {
		appearance = "dark"
	}
	var chosen *Variant
	for i := range variants {
		v := &variants[i]
		if v.ID == themeID {
			chosen = v
			break
		}
		if v.FamilyID == themeID && v.Appearance == appearance {
			chosen = v
		}
	}
	if chosen == nil {
		for i := range variants {
			if variants[i].ID == "zeron-"+appearance {
				chosen = &variants[i]
				break
			}
		}
	}
	if chosen == nil {
		panic("Zeron palette is missing from the embedded registry")
	}
	v := chosen
	c := func(role string) ui.Color { return ui.Hex(v.Colors[role]) }
	t := Tokens{ID: v.ID, Name: v.Name, Dark: v.Appearance == "dark", Frosted: v.RecommendedSurfaceTreatment == "frosted",
		Background: c("background"), Shell: c("shell"), Raised: c("raised"), Card: c("card"), Dialog: c("dialog"), Overlay: c("overlay"),
		Text: c("text"), Muted: c("textMuted"), Faint: c("textFaint"), Solid: c("solid"), OnSolid: c("onSolid"),
		Accent: ui.Hex(v.Accent.Primary), AccentStrong: ui.Hex(v.Accent.Strong), AccentOn: ui.Hex(v.Accent.On), Selection: ui.Hex(v.Accent.Selection), Caret: ui.Hex(v.Accent.Caret),
		Border: c("border"), BorderStrong: c("borderStrong"), Hover: c("hover"), Active: c("active"), Input: c("input"),
		Danger: c("danger"), Warning: c("warning"), Success: c("success"), DiffAdd: c("diffAdd"), DiffDelete: c("diffDelete"), DiffHunk: c("diffHunk"),
		TerminalBackground: ui.Hex(v.Terminal.Background), TerminalForeground: ui.Hex(v.Terminal.Foreground), TerminalSelection: ui.Hex(v.Terminal.Selection), Syntax: make(map[string]ui.Color)}
	for i, color := range v.Terminal.ANSI {
		t.ANSI[i] = ui.Hex(color)
	}
	for i, color := range v.Accent.Glyph {
		t.Glyph[i] = ui.Hex(color)
	}
	for name, color := range v.Syntax {
		t.Syntax[name] = ui.Hex(color)
	}
	return t
}

// UI builds a fresh theme for native widgets, using source roles and source metrics.
func (t Tokens) UI() *ui.Theme {
	return &ui.Theme{Dark: t.Dark, Background: t.Background, Surface: t.Input, SurfaceHover: t.Hover.Over(t.Input), SurfacePressed: t.Active.Over(t.Input),
		Border: t.Border, Text: t.Text, TextMuted: t.Muted, Accent: t.Accent, AccentHover: t.AccentStrong, AccentPressed: t.AccentStrong, AccentText: t.AccentOn,
		Danger: t.Danger, Warning: t.Warning, Success: t.Success, Selection: t.Selection, Focus: t.Accent, Inverse: t.Solid, InverseText: t.OnSolid,
		Scrollbar: t.Muted.Alpha(.35), ScrollbarWidth: 6, Radius: ControlRadius, Spacing: SpaceXS, FontSize: 13, Font: "Geist"}
}

// Font normalizes persisted source font identifiers into native family names.
func Font(name string) string {
	switch strings.ToLower(name) {
	case "", "geist":
		return "Geist"
	case "geistmono", "geist mono":
		return "Geist Mono"
	case "system":
		return ""
	default:
		return strings.TrimPrefix(name, "installed:")
	}
}

func MonoFont(name string) string {
	if name == "" {
		return "Geist Mono"
	}
	if strings.EqualFold(name, "system") {
		return "monospace"
	}
	return Font(name)
}

// UISize maps an authored ui_rems pixel size to the selected UI rem base.
// Editor and terminal font settings use their own absolute pixel sizes.
func UISize(c *ui.Context, authored float32) float32 {
	if c.Theme().FontSize <= 0 {
		return authored
	}
	return authored * c.Theme().FontSize / 13
}
