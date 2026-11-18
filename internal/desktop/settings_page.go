package desktop

import (
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func settingsPageViewport(c *ui.Context) ui.Element {
	return ui.Scroll(c).Grow(1).AlignItems(ui.Stretch).Label(tr(c, "Settings page viewport"))
}

func settingsPageContent(c *ui.Context) ui.Element {
	s := design.SettingsPage()
	return ui.Column(c).FillWidth().MaxWidth(s.MaxWidth).AlignSelf(ui.Center).
		Padding(s.PaddingTop, s.PaddingX, s.PaddingBottom, s.PaddingX).
		Gap(s.Gap).Justify(ui.Start).Label(tr(c, "Settings page content"))
}

func settingsPageHeading(c *ui.Context, section string) ui.Element {
	return design.SettingsPage().Heading(c, tr(c, section), tr(c, "Settings page heading")+": "+tr(c, section))
}
