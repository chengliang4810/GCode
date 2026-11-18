package assets

import (
	"bytes"
	_ "embed"
	"image/color"
	"image/png"
	"sync"

	"github.com/egoist/mygo/ui"
)

// Original GCode monogram with the enclosing square removed, for system tinting.
//
//go:embed branding/menu-bar.svg
var menuBarSVG []byte
var menuIconOnce sync.Once
var menuIconPNG []byte
var menuIconErr error

func MenuBarIcon() ([]byte, error) {
	menuIconOnce.Do(func() {
		svg, err := ui.ParseSVG(menuBarSVG)
		if err != nil {
			menuIconErr = err
			return
		}
		var buffer bytes.Buffer
		menuIconErr = png.Encode(&buffer, svg.Image(32, 32, color.Black))
		menuIconPNG = buffer.Bytes()
	})
	return menuIconPNG, menuIconErr
}
