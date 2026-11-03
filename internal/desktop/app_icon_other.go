//go:build !windows

package desktop

import (
	"gcode/internal/assets"
	"runtime"

	"github.com/egoist/mygo"
)

func setApplicationIcon(win *mygo.Window) error {
	if runtime.GOOS == "darwin" {
		return mygo.App.Dock.SetIcon(assets.MacAppIcon())
	}
	return win.SetIcon(assets.AppIcon())
}
