//go:build !windows

package desktop

import (
	"gcode/internal/assets"

	"github.com/egoist/mygo"
)

func setApplicationIcon(win *mygo.Window) error {
	return win.SetIcon(assets.AppIcon())
}
