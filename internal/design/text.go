package design

import "github.com/egoist/mygo/ui"

// Interface sizes follow ZCode's text-ui hierarchy at its 14 px default.
// Preferences change each size by the same pixel delta, leaving geometry alone.
const (
	TextXS         float32 = 10
	TextSmall      float32 = 12
	TextCaption    float32 = 13
	TextBase       float32 = 14
	TextLarge      float32 = 16
	TextXL         float32 = 18
	TextTitle      float32 = 24
	TextTitleLarge float32 = 30
)

func TextSize(c *ui.Context, size float32) float32 {
	base := c.Theme().FontSize
	if base <= 0 {
		base = UIFontSize
	}
	return size + base - UIFontSize
}
