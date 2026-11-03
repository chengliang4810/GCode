package design

// ControlTokens is the single source of dimensions for shared desktop controls.
// Width belongs to the page layout; single-line height and internal spacing do
// not. Geometry stays in logical pixels; text uses the interface font tokens.
// Text-bearing controls grow vertically only when their content needs it.
type ControlTokens struct {
	FontSize, BorderWidth                                      float32
	ButtonHeight, ButtonRadius, ButtonPaddingX, ButtonPaddingY float32
	ButtonGap, IconButtonSize, IconSize                        float32
	InlineIconButtonSize                                       float32
	RoundIconButtonRadius                                      float32
	InputHeight, InputRadius, InputPaddingX, InputPaddingY     float32
	SelectMinWidth, PopupPadding, PopupRadius                  float32
	OptionPaddingX, OptionPaddingY, OptionRadius               float32
	SwitchWidth, SwitchHeight                                  float32
	CheckboxSize, CheckboxRadius                               float32
	SliderHeight                                               float32
	ScrollbarHeight, ScrollbarMinThumb                         float32
	NavigationHeight, NavigationPaddingX, NavigationGap        float32
	TileRadius, TilePadding, SwatchSize                        float32
	GradientSwatchWidth, GradientSwatchHeight                  float32
	ReorderIndicatorWidth                                      float32
	ResourceIconSize, BadgePaddingX, BadgePaddingY             float32
}

// Controls defines shared component geometry. Change it here rather than in a
// page. Theme colors continue to come from Tokens.UI and the selected palette.
func Controls() ControlTokens {
	return ControlTokens{
		FontSize: TextBase, BorderWidth: 1,
		ButtonHeight: 28.8, ButtonRadius: 6, ButtonPaddingX: 8, ButtonPaddingY: 4,
		ButtonGap: 6, IconButtonSize: 28, IconSize: 16,
		InlineIconButtonSize:  24,
		RoundIconButtonRadius: 14,
		InputHeight:           32, InputRadius: 6, InputPaddingX: 10, InputPaddingY: 6,
		SelectMinWidth: 124, PopupPadding: 5, PopupRadius: 10,
		OptionPaddingX: 10, OptionPaddingY: 7, OptionRadius: 6,
		SwitchWidth: 44.8, SwitchHeight: 28.8,
		CheckboxSize: 16, CheckboxRadius: 4,
		SliderHeight:    28,
		ScrollbarHeight: 16, ScrollbarMinThumb: 24,
		NavigationHeight: 36, NavigationPaddingX: 10, NavigationGap: 10,
		TileRadius: 9, TilePadding: 10, SwatchSize: 20,
		GradientSwatchWidth: 24, GradientSwatchHeight: 12,
		ReorderIndicatorWidth: 2,
		ResourceIconSize:      36, BadgePaddingX: 6, BadgePaddingY: 2,
	}
}

// MessageRailTokens keeps the conversation outline's compact navigation and
// preview geometry together with the other shared controls.
type MessageRailTokens struct {
	MinContainerWidth, Left, Width, VerticalMargin          float32
	SlotHeight, Gap, MarkWidth, HoverWidth, MarkHeight      float32
	PreviewWidth, PreviewGap, PreviewPadding, PreviewRadius float32
	MaxMarks                                                int
}

func MessageRail() MessageRailTokens {
	return MessageRailTokens{
		MinContainerWidth: 768, Left: 16, Width: 26, VerticalMargin: 24,
		SlotHeight: 10, Gap: 3, MarkWidth: 12, HoverWidth: 20, MarkHeight: 2,
		PreviewWidth: 280, PreviewGap: 6, PreviewPadding: 12, PreviewRadius: 12,
		MaxMarks: 12,
	}
}

// SidebarTokens follows the reference sidebar's navigation/list hierarchy.
// Geometry stays independent of the interface font; rows grow for large text.
type SidebarTokens struct {
	PaddingX, NavigationPaddingY         float32
	RowHeight, HeaderHeight, RowPaddingX float32
	RowRadius, RowGap, IconSize          float32
	ConversationLeadingWidth             float32
	ConversationTrailingWidth            float32
	ChildInset, ConversationTextInset    float32
	PageSize                             int
}

// The conversation row's two fixed slots. The leading slot holds the status
// glyph and stays one icon box wide so titles share one column; it is not the
// width of the harness glyph that used to sit there. The trailing slot holds
// the relative time and, while hovered, the archive control, so its minimum is
// that control rather than an arbitrary wider number.
func Sidebar() SidebarTokens {
	// A conversation row pads 8 either side, then the leading status slot, then
	// a 4 gap before the title. Everything that has to line up with that title
	// (Show more, the empty state) is padded by the same sum, so it is derived
	// here instead of being restated as a second magic number.
	const conversationRowPaddingX = 8
	const conversationGap = 4
	controls := Controls()
	leading := controls.IconSize
	return SidebarTokens{
		PaddingX: 8, NavigationPaddingY: 12,
		RowHeight: 32, HeaderHeight: 28, RowPaddingX: 10,
		RowRadius: 8, RowGap: 2, IconSize: 16, ConversationLeadingWidth: leading,
		ConversationTrailingWidth: controls.InlineIconButtonSize,
		ChildInset:                8, ConversationTextInset: conversationRowPaddingX + leading + conversationGap,
		PageSize: 5,
	}
}
