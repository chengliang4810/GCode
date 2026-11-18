package design

// MenuPanelTokens defines the native companion panel's reading hierarchy.
type MenuPanelTokens struct {
	Width, Height, Radius         int
	Padding, Gap, Heading, Metric float32
}

func MenuPanel() MenuPanelTokens {
	return MenuPanelTokens{Width: 360, Height: 520, Radius: 20, Padding: 20, Gap: 16, Heading: 23, Metric: 32}
}
