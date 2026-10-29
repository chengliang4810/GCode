package workbench

import (
	"math"
	"runtime"
	"strings"

	"github.com/egoist/mygo/ui"
)

func tabTooltipLeafFeatures() string {
	if runtime.GOOS == "windows" {
		// The app's frozen Windows typography uses this explicit default.
		return "kern=0"
	}
	return ""
}

func tabTooltipLeafMinWidth(c *ui.Context, text string, size, maxWidth float32) float32 {
	// MeasureText has no element RTL/line-height parameters. Limit this
	// candidate to an unwrapped paragraph in the window's LTR context;
	// multiline and constrained wrapping keep the existing native path.
	if c.Direction() != ui.LeftToRight || strings.ContainsAny(text, "\r\n") {
		return 0
	}
	w, _ := c.MeasureText(0, ui.Span{
		Text: text, Font: tabTooltipFontFamily(), Size: size, Weight: 400,
		Features: tabTooltipLeafFeatures(), Lang: c.Locale(),
	})
	if w > 0 && w <= maxWidth {
		// Source TextState rounds the shaped leaf width, before card padding
		// and border. It does not ceil the already laid-out card border box.
		return float32(math.Ceil(float64(w)))
	}
	return 0
}
