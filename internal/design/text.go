package design

import "github.com/egoist/mygo/ui"

// Interface sizes follow ZCode's text-ui hierarchy at its 14 px default.
// Preferences shift every level by the same pixel delta and never push it
// below MinTextSize: the 10 px level is gone, and the levels that end up on
// the floor at a small interface font keep their order through weight and
// gray level rather than through a smaller size. Code and terminal text render
// user content, not interface copy, and keep their own ranges.
const (
	TextSmall      float32 = 12
	TextCaption    float32 = 13
	TextBase       float32 = 14
	TextLarge      float32 = 16
	TextXL         float32 = 18
	TextTitle      float32 = 24
	TextTitleLarge float32 = 30
)

// MinTextSize is the smallest pixel size an interface text level may render
// at. TestInterfaceTextLevelsMeetPixelFloor keeps every level above it.
const MinTextSize float32 = 12

// TextSize maps an interface text level onto the current interface font size.
// The level keeps its pixel distance from the 14 px default and never drops
// below MinTextSize, so a small interface font makes the lower levels share
// the floor instead of shrinking past it.
func TextSize(c *ui.Context, size float32) float32 {
	return textSize(c.Theme().FontSize, size)
}

// textSize is the pure form of TextSize, so the floor is checkable without a
// window.
func textSize(base, size float32) float32 {
	if base <= 0 {
		base = UIFontSize
	}
	return max(size+base-UIFontSize, MinTextSize)
}
