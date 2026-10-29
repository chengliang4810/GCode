package workbench

import (
	"fmt"
	"image/color"
	"testing"

	"gcode/internal/assets"
	"github.com/egoist/mygo/ui"
)

func TestTabStripChipWashKeepsSourceFloatAndBackdrop(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, glass := range []bool{false, true} {
			for _, scale := range []float32{1, 2} {
				for _, base := range []ui.Color{ui.RGB(13, 13, 13), ui.RGB(70, 80, 90), ui.Transparent} {
					t.Run(fmt.Sprintf("dark%t-glass%t-scale%g-base%v", dark, glass, scale, base), func(t *testing.T) {
						s, _, snapshot := testWorkspace(t)
						snapshot.Settings.Appearance.Mode, snapshot.Settings.Appearance.ThemeID = "light", "zeron-light"
						if dark {
							snapshot.Settings.Appearance.Mode, snapshot.Settings.Appearance.ThemeID = "dark", "zeron-dark"
						}
						snapshot.Settings.Appearance.Surface = "opaque"
						if glass {
							snapshot.Settings.Appearance.Surface = "frosted"
						}
						s.Update(snapshot)
						s.choose("history", "first", "Selected")
						s.choose("history", "second", "Hovered")
						s.selectTab(0)
						tt := ui.NewTester(func(c *ui.Context) {
							c.SetTheme(s.palette().UI())
							c.Root().Background(base)
							ui.Column(c).Fill().Children(func() { s.TabStrip(c, false) })
						}, 360, 80)
						tt.SetScale(scale)
						tt.SetVibrancy(glass)
						backdrop := tt.Image().RGBAAt(int(350*scale), int(60*scale))
						for _, test := range []struct {
							label string
							alpha float32
						}{{"Selected", .10}, {"Hovered", .06}} {
							chip, ok := tt.Find(test.label)
							if !ok {
								t.Fatalf("missing chip %q", test.label)
							}
							if test.label == "Hovered" {
								tt.Move(chip.X+3, chip.Y+chip.H/2)
							}
							got := tt.Image().RGBAAt(int((chip.X+3)*scale), int((chip.Y+chip.H/2)*scale))
							want := sourceTabWashOver(backdrop, dark, test.alpha)
							if got != want {
								t.Errorf("%s interior wash: got%v want%v from source float over actual backdrop%v", test.label, got, want, backdrop)
							}
							if got := tt.Image().RGBAAt(int(chip.X*scale), int(chip.Y*scale)); got != backdrop {
								t.Errorf("rounded corner painted outside radius6: got%v want%v", got, backdrop)
							}
						}
					})
				}
			}
		}
	}
}

func TestTabStripGenericGlyphOpacityAndForeground(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, scale := range []float32{1, 2} {
			t.Run(fmt.Sprintf("dark%t-scale%g", dark, scale), func(t *testing.T) {
				s, _, snapshot := testWorkspace(t)
				snapshot.Settings.Appearance.Mode, snapshot.Settings.Appearance.ThemeID = "light", "zeron-light"
				if dark {
					snapshot.Settings.Appearance.Mode, snapshot.Settings.Appearance.ThemeID = "dark", "zeron-dark"
				}
				s.Update(snapshot)
				s.choose("history", "first", "Selected")
				s.choose("history", "second", "Inactive")
				s.selectTab(0)
				base := ui.RGB(13, 13, 13)
				tt := ui.NewTester(func(c *ui.Context) {
					c.SetTheme(s.palette().UI())
					c.Root().Background(base)
					ui.Column(c).Fill().Children(func() { s.TabStrip(c, false) })
				}, 360, 80)
				tt.SetScale(scale)
				for _, test := range []struct {
					label   string
					opacity float32
					plate   color.RGBA
				}{{"Inactive", .7, color.RGBA{13, 13, 13, 255}}, {"Selected", 1, sourceTabWashOver(color.RGBA{13, 13, 13, 255}, dark, .10)}} {
					chip, _ := tt.Find(test.label)
					reference := ui.NewTester(func(c *ui.Context) {
						c.Root().Background(ui.RGB(test.plate.R, test.plate.G, test.plate.B))
						ui.Icon(c, assets.Icon("git-branch")).Size(12, 12).TextColor(s.palette().Muted).Opacity(test.opacity)
					}, 12, 12)
					reference.SetScale(scale)
					actual := tt.Image()
					for y := 0; y < int(12*scale); y++ {
						for x := 0; x < int(12*scale); x++ {
							got := actual.RGBAAt(int((chip.X+7)*scale)+x, int((chip.Y+6)*scale)+y)
							if want := reference.Image().RGBAAt(x, y); got != want {
								t.Fatalf("%s generic glyph or background affected foreground: pixel%d,%d got%v want%v", test.label, x, y, got, want)
							}
						}
					}
				}
				before, _ := tt.Find("Inactive")
				if err := tt.Click("Inactive"); err != nil {
					t.Fatal(err)
				}
				if s.active != 1 {
					t.Fatal("chip paint intercepted selection")
				}
				if after, _ := tt.Find("Inactive"); after != before {
					t.Fatal("chip paint changed geometry")
				}
				if err := tt.Click("Close tab Inactive"); err != nil {
					t.Fatal(err)
				}
				if len(s.tabs) != 1 || s.tabs[0].id != "first" {
					t.Fatal("chip paint intercepted close action")
				}
			})
		}
	}
}

func TestTabStripDirtyDotIsSixDIPAndHidesOnHover(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	var id string
	for _, file := range snapshot.Files {
		if !file.ReadOnly && file.Kind != "directory" && file.Kind != "image" {
			id = file.ID
			break
		}
	}
	s.OpenFile(id)
	document := s.documents[id]
	if document == nil {
		t.Fatal("no editable file fixture")
	}
	document.editor.Buffer.Set(document.editor.Buffer.String() + "\nunsaved R15\n")
	tt := tabStripTester(s, 360, false)
	tt.SetScale(2)
	close, ok := tt.Find("Close tab " + s.tabs[0].title)
	if !ok {
		t.Fatal("missing dirty file close slot")
	}
	centerX, centerY := int((close.X+close.W/2)*2), int((close.Y+close.H/2)*2)
	plate := tt.Image().RGBAAt(int((close.X-2)*2), centerY)
	muted := s.palette().Muted
	cut := (int(plate.R) + int(muted.R)) / 2
	var pixels []int
	for x := int(close.X * 2); x < int((close.X+close.W)*2); x++ {
		value := int(tt.Image().RGBAAt(x, centerY).R)
		if muted.R > plate.R && value > cut || muted.R < plate.R && value < cut {
			pixels = append(pixels, x)
		}
	}
	if len(pixels) != 12 || pixels[0] != centerX-6 || pixels[len(pixels)-1] != centerX+5 {
		t.Fatalf("dirty dot center row is not6DIP at2x: %v, center%d", pixels, centerX)
	}
	before := tt.Image().RGBAAt(centerX-5, centerY)
	tt.Move(close.X+close.W/2, close.Y+close.H/2)
	if before == tt.Image().RGBAAt(centerX-5, centerY) {
		t.Fatal("dirty dot did not yield its close slot on hover")
	}
	if !document.editor.Buffer.Modified() {
		t.Fatal("hover modified the unsaved buffer")
	}
}

// Source Hsla wash has float luminance and alpha. The base is an actual
// premultiplied rendered pixel, so glass is not flattened onto a Shell guess.
func sourceTabWashOver(base color.RGBA, dark bool, alpha float32) color.RGBA {
	luminance := float32(.10)
	if dark {
		luminance = .92
	}
	channel := func(c uint8) uint8 { return uint8((float32(c)/255*(1-alpha)+luminance*alpha)*255 + .5) }
	return color.RGBA{channel(base.R), channel(base.G), channel(base.B), uint8((float32(base.A)/255*(1-alpha)+alpha)*255 + .5)}
}
