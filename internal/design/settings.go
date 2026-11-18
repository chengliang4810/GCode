package design

import "github.com/egoist/mygo/ui"

// SettingsPageTokens defines the outer frame shared by every settings section.
// Section-specific cards, forms and columns live inside this frame.
type SettingsPageTokens struct {
	MaxWidth, PaddingTop, PaddingX, PaddingBottom, Gap              float32
	HeadingSize, HeadingLineHeight, WideHeadingSize, WideBreakpoint float32
	WideHeadingLineHeight                                           float32
	HeadingWeight                                                   int
	HeadingInset, HeadingBottomMargin                               float32
}

func SettingsPage() SettingsPageTokens {
	return SettingsPageTokens{
		MaxWidth: 1200, PaddingTop: 54, PaddingX: 40, PaddingBottom: 48, Gap: 16,
		HeadingSize: TextTitle, HeadingLineHeight: 32, HeadingWeight: 600,
		WideHeadingSize: TextTitleLarge, WideHeadingLineHeight: 36, WideBreakpoint: 1024,
		HeadingInset: 8, HeadingBottomMargin: 8,
	}
}

// Heading builds the one settings page title. It picks the narrow or wide
// size from the window width, keeps the fixed leading that the frame sizes
// around, and balances the lines so a long section name never leaves its last
// line holding a single word.
func (s SettingsPageTokens) Heading(c *ui.Context, text, label string) ui.Element {
	size, lineHeight := s.HeadingSize, s.HeadingLineHeight
	if width, _ := c.Size(); width >= s.WideBreakpoint {
		size = s.WideHeadingSize
		lineHeight = s.WideHeadingLineHeight
	}
	return ui.Text(c, text).Label(label).
		FontSize(TextSize(c, size)).FontWeight(s.HeadingWeight).
		LineHeight(lineHeight/size).
		Margin(0, s.HeadingInset, s.HeadingBottomMargin, s.HeadingInset).
		Balance()
}

