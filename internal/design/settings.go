package design

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
