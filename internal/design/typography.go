package design

import (
	"runtime"

	"github.com/egoist/mygo/ui"
)

// ApplyReferenceTypography preserves the frozen renderer's platform defaults.
// GPUI's Windows shaper always installs a Typography object, even when empty;
// This changes kerning and common ligatures compared with MyGo's skipped
// Typography call. The public kern=0 feature matches those explicit defaults
// without changing font files, font sizes, or other platforms' shapers.
func ApplyReferenceTypography(c *ui.Context) {
	if runtime.GOOS == "windows" {
		c.Root().FontFeatures("kern=0")
	}
}
