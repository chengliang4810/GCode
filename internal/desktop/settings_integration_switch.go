package desktop

import (
	"github.com/egoist/mygo/ui"
	"strings"
	"time"
)

// Uses the same source-derived drawing as the shared settings switch.
func (a *App) styleIntegrationSwitch(c *ui.Context, b ui.Element, value bool) {
	on := value
	target := float32(0)
	if on {
		target = 1
	}
	position := b.AnimateWith("switch-travel", target, 180*time.Millisecond, func(v float32) float32 { return 1 - (1-v)*(1-v)*(1-v) })
	if a.state.Settings.Appearance.ReduceMotion || strings.EqualFold(a.state.Settings.Appearance.MotionPreference, "system") && c.Preferences().ReduceMotion {
		position = target
	}
	frost := a.palette.Frosted || strings.EqualFold(a.state.Settings.Appearance.Surface, "Frosted")
	if strings.EqualFold(a.state.Settings.Appearance.Surface, "Opaque") {
		frost = false
	}
	b.Draw(func(p *ui.Painter, r ui.Rect) {
		track := ui.Rect{X: r.X, Y: r.Y + 4, W: 44.8, H: 20.8}
		ink := ui.Hex("#000000")
		if a.palette.Dark {
			ink = ui.Hex("#ffffff")
		}
		coverage := float32(.1)
		if a.palette.Dark {
			coverage = .18
		}
		if frost {
			if a.palette.Dark {
				coverage = .22
			} else {
				coverage = .12
			}
		}
		color := ink.Alpha(coverage).Over(a.palette.Shell)
		if on {
			color = a.palette.AccentStrong
			if a.palette.Dark {
				color = ui.Hex("#000000").Alpha(.14).Over(color)
			}
		}
		light, shade := color, color
		if frost {
			light = ui.Hex("#ffffff").Alpha(.07).Over(color)
			shade = ui.Hex("#000000").Alpha(.09).Over(color)
		}
		p.FillGradient(track, ui.LinearGradient{From: light, To: shade, Angle: 180}, 10.4)
		rim := a.palette.Border.Over(color)
		if on {
			rim = ui.Hex("#ffffff").Alpha(.16).Over(color)
		}
		p.Stroke(track, rim, 10.4, 1)
		p.Fill(ui.Rect{X: r.X + 9, Y: r.Y + 10.8, W: 1.2, H: 7.2}, ui.Hex("#ffffff").Alpha(.96*position), .6)
		p.Stroke(ui.Rect{X: r.X + 31.6, Y: r.Y + 10.8, W: 7.2, H: 7.2}, ui.Hex("#ffffff").Alpha(.92*(1-position)), 3.6, 1)
		x := r.X + 1.6 + 17.6*position
		thumb := ui.Hex("#ffffff").Alpha(.94).Over(a.palette.Shell)
		thumbLight, thumbShade := thumb, thumb
		if frost {
			thumbLight = ui.Hex("#ffffff").Alpha(.12).Over(thumb)
			thumbShade = ui.Hex("#000000").Alpha(.07).Over(thumb)
		}
		thumbRect := ui.Rect{X: x, Y: r.Y + 5.6, W: 24, H: 17.6}
		p.FillGradient(thumbRect, ui.LinearGradient{From: thumbLight, To: thumbShade, Angle: 180}, 8.8)
		p.Stroke(thumbRect, ui.Hex("#000000").Alpha(.1).Over(thumb), 8.8, 1)
		if frost && position > 0 {
			p.Fill(ui.Rect{X: x + 7.2, Y: r.Y + 7.2, W: 9.6, H: 1}, ui.Hex("#ffffff").Alpha(.45*position).Over(thumbLight), .5)
		}
	})
}
