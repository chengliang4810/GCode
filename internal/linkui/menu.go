package linkui

import (
	i18nview "gcode/internal/i18n/view"
	"gcode/internal/linktarget"
	"github.com/egoist/mygo/ui"
)

// MarkdownMenu supplies the same host-aware actions to links rendered by
// MyGo itself, including links mixed with HTML, images or footnotes.
func MarkdownMenu(c *ui.Context, cfg Config) func(*ui.Menu, string) {
	build := linkMenu(c, cfg)
	return func(menu *ui.Menu, raw string) {
		if cfg.Resolve != nil {
			if destination, ok := cfg.Resolve(raw); ok {
				if destination.Raw == "" {
					destination.Raw = raw
				}
				build(menu, destination)
				return
			}
		}
		menu.EditItems()
	}
}

func linkMenu(c *ui.Context, cfg Config) func(*ui.Menu, Destination) {
	services := c.Services()
	copyLink, copyPath := i18nview.T(c, "Copy link address"), i18nview.T(c, "Copy path")
	openLabel, external := i18nview.T(c, "Open in GCode"), i18nview.T(c, "Open in external browser")
	folder, app := i18nview.T(c, "Show in folder"), i18nview.T(c, "Open in default app")
	preference := i18nview.T(c, "Open web links in GCode")
	return func(menu *ui.Menu, destination Destination) {
		label := copyLink
		if destination.Kind == linktarget.File {
			label = copyPath
		}
		if menu.Item(label).Chosen() {
			services.WriteClipboard(targetText(destination))
		}
		if menu.Item(openLabel).Disabled(cfg.Open == nil).Chosen() {
			open(destination, cfg, true)
		}
		switch destination.Kind {
		case linktarget.Web, linktarget.Mail:
			if menu.Item(external).Disabled(cfg.OpenExternal == nil).Chosen() {
				cfg.OpenExternal(destination.Raw)
			}
			if destination.Kind == linktarget.Web && cfg.SetInternal != nil {
				menu.Separator()
				if menu.Item(preference).Checked(cfg.Internal).Chosen() {
					cfg.SetInternal(!cfg.Internal)
				}
			}
		case linktarget.File:
			if destination.Local {
				if menu.Item(app).Disabled(cfg.OpenExternal == nil).Chosen() {
					cfg.OpenExternal(destination.Raw)
				}
				if menu.Item(folder).Disabled(cfg.Reveal == nil).Chosen() {
					cfg.Reveal(destination.Raw)
				}
			}
		}
		menu.Separator()
		menu.EditItems()
	}
}
