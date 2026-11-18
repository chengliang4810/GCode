//go:build !windows

package desktop

import (
	"gcode/internal/assets"
	"runtime"

	"github.com/egoist/mygo"
)

func setApplicationIcon(win *mygo.Window) error {
	if runtime.GOOS == "darwin" {
		// Packaged apps already provide the same artwork as a multi-size
		// ICNS. Let AppKit choose its representation instead of replacing
		// it with a second, single-size NSImage at every launch.
		if mygo.App.IsPackaged() {
			return nil
		}
		return mygo.App.Dock.SetIcon(assets.MacAppIcon())
	}
	return win.SetIcon(assets.AppIcon())
}
