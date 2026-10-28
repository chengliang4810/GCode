package desktop

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// Passive indicator from settings/widgets.rs. The native ToggleBase row owns
// focus, keyboard activation and its binding; this only paints its 44.8px mark.
func (a *composerController) composerPlanSwitch(c *ui.Context, on bool) {
	box := ui.Box(c).Size(44.8, 28.8).Shrink(0)
	target := float32(0)
	if on {
		target = 1
	}
	position := box.AnimateWith("travel", target, 180*time.Millisecond, ui.EaseOut)
	if a.reduceMotion(c) {
		position = target
	}
	frost := a.palette.Frosted
	if strings.EqualFold(a.state.Settings.Appearance.Surface, "opaque") {
		frost = false
	}
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		ink := ui.Hex("#000000")
		coverage := float32(.10)
		if a.palette.Dark {
			ink = ui.Hex("#ffffff")
			coverage = .18
		}
		if frost {
			coverage = .12
			if a.palette.Dark {
				coverage = .22
			}
		}
		track := ink.Alpha(coverage).Over(a.palette.Shell)
		if on {
			track = a.palette.AccentStrong
			if a.palette.Dark {
				track = ui.Hex("#000000").Alpha(.14).Over(track)
			}
		}
		light, shade := track, track
		if frost {
			light = ui.Hex("#ffffff").Alpha(.07).Over(track)
			shade = ui.Hex("#000000").Alpha(.09).Over(track)
		}
		trackBounds := ui.Rect{X: r.X, Y: r.Y + 4, W: 44.8, H: 20.8}
		p.FillGradient(trackBounds, ui.LinearGradient{From: light, To: shade, Angle: 180}, 10.4)
		rim := a.palette.Border.Over(track)
		if on {
			alpha := float32(.12)
			if frost {
				alpha = .16
			}
			rim = ui.Hex("#ffffff").Alpha(alpha).Over(track)
		}
		p.Stroke(trackBounds, rim, 10.4, 1)
		p.Fill(ui.Rect{X: r.X + 9, Y: r.Y + 10.8, W: 1.2, H: 7.2}, ui.Hex("#ffffff").Alpha(.96*position), .6)
		p.Stroke(ui.Rect{X: r.X + 31.6, Y: r.Y + 11.2, W: 6.4, H: 6.4}, ui.Hex("#ffffff").Alpha(.92*(1-position)), 3.2, 1)
		alpha := float32(1)
		if frost {
			alpha = .96
			if a.palette.Dark {
				alpha = .94
			}
		} else if a.palette.Dark {
			alpha = .96
		}
		thumb := ui.Hex("#ffffff").Alpha(alpha).Over(a.palette.Shell)
		thumbLight, thumbShade := thumb, thumb
		if frost {
			thumbLight = ui.Hex("#ffffff").Alpha(.12).Over(thumb)
			thumbShade = ui.Hex("#000000").Alpha(.07).Over(thumb)
		}
		x := r.X + 1.6 + 17.6*position
		thumbBounds := ui.Rect{X: x, Y: r.Y + 5.6, W: 24, H: 17.6}
		p.FillGradient(thumbBounds, ui.LinearGradient{From: thumbLight, To: thumbShade, Angle: 180}, 8.8)
		p.Stroke(thumbBounds, ui.Hex("#000000").Alpha(.10).Over(thumb), 8.8, 1)
		if frost && position > 0 {
			p.Fill(ui.Rect{X: x + 7.2, Y: r.Y + 7.2, W: 9.6, H: 1}, ui.Hex("#ffffff").Alpha(.45*position).Over(thumbLight), .5)
		}
	})
}
