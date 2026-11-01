package desktop

import (
	"context"
	"image/color"
	"strings"

	"gcode/internal/design"
	"gcode/internal/wallpaper"
)

type wallpaperState struct {
	cancel context.CancelFunc
	epoch  uint64
	dark   bool
	color  color.NRGBA
	err    string
}

// refreshWallpaperColors runs as a controller action after attachment or a
// settings update. Only the worker reads the desktop and image files.
func (a *App) refreshWallpaperColors() {
	settings := a.state.Settings.Appearance
	enabled := settings.WallpaperColors && a.win != nil
	dark := strings.EqualFold(settings.Mode, "dark") || strings.EqualFold(settings.Mode, "system") && a.systemDark
	if a.wallpaper.cancel != nil && enabled && a.wallpaper.dark == dark {
		return
	}
	if a.wallpaper.cancel != nil {
		a.wallpaper.cancel()
		a.wallpaper.cancel = nil
	}
	a.wallpaper.epoch++
	if !enabled {
		a.wallpaper.color = color.NRGBA{}
		a.wallpaper.err = ""
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.wallpaper.cancel, a.wallpaper.dark = cancel, dark
	epoch, window := a.wallpaper.epoch, a.win
	go wallpaper.Watch(ctx, dark, func(result wallpaper.Result) {
		window.Update(func() {
			if a.closed || a.wallpaper.epoch != epoch {
				return
			}
			a.wallpaper.color = result.Color
			a.wallpaper.err = ""
			if result.Err != nil {
				a.wallpaper.err = result.Err.Error()
			}
		})
	})
}

func (a *App) wallpaperPalette(palette design.Tokens) design.Tokens {
	if !a.state.Settings.Appearance.WallpaperColors {
		return palette
	}
	return design.ApplyWallpaperColors(palette, a.wallpaper.color)
}
