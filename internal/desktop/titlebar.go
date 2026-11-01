package desktop

import (
	"runtime"

	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

const (
	titlebarHeight     float32 = 38
	titlebarEdgeInset  float32 = 6
	titlebarControlGap float32 = 4
)

// HideNativeWindowButtons reports whether the app window hides its native
// caption controls so the title bar draws the shared SVG buttons. Windows
// draws them in the title bar; macOS keeps its traffic lights and Linux
// keeps the platform's controls. cmd/rcode passes it as
// mygo.WindowOptions.HideWindowButtons, so both sides agree on the layout.
const HideNativeWindowButtons = runtime.GOOS == "windows"

// titlebarWidths follows shell/tabs.rs panel_titlebar_widths. The caption
// clearance is paid once across the surface and Files columns; the two
// global toggles retain their own fixed slots outside the tabs' clip.
func titlebarWidths(right, files, available, caption float32) (surface, controls float32) {
	controls = max(files-caption, design.Controls().IconButtonSize*2+4)
	surface = max(0, min(right+files-caption-controls, available-controls))
	return surface, controls
}

// titlebarCaptionWidth is the room the window's own caption buttons take:
// three 28px controls and the two gaps between them.
func titlebarCaptionWidth() float32 {
	return design.Controls().IconButtonSize*3 + titlebarControlGap*2
}

// captionButtonsDrawn reports whether the title bar draws the window's
// caption controls: only where the window hides the platform's own, never in
// full screen where the platform hides its controls too, and never over a
// caption area the platform still reports.
func captionButtonsDrawn(hidesNative, fullscreen bool, nativeRight float32) bool {
	return hidesNative && !fullscreen && nativeRight == 0
}

// titlebarBandHeight is the band the title bar reserves at the top of the
// window. The chat route draws it as an overlay over the full-height pane
// surfaces, so the panes pad their own content down by the same value.
func titlebarBandHeight() float32 {
	return max(titlebarHeight, design.Controls().IconButtonSize+8)
}

// captionMaximizeGlyph swaps the maximize glyph for the restore glyph while
// the window is maximized, as the platform's own caption button does.
func captionMaximizeGlyph(maximized bool) string {
	if maximized {
		return "caption-restore"
	}
	return "caption-maximize"
}

func (a *App) titlebar(c *ui.Context) { a.titlebarRow(c, false) }

// titlebarOverlay draws the title bar over the full-height content row: the
// panes reach the window's top edge (as the source's right pane does) and
// keep their content clear of this band.
func (a *App) titlebarOverlay(c *ui.Context) { a.titlebarRow(c, true) }

func (a *App) titlebarRow(c *ui.Context, overlay bool) {
	bar := c.TitleBar()
	session := a.currentSession()
	onCanvas := a.route == "chat" && session.ID == ""
	viewport, _ := c.Size()
	columns := a.frameColumns
	// A window that hides its native caption buttons leaves the right edge to
	// the title bar, which draws the same controls from the shared icons. A
	// full-screen window shows none of them, as the platform's own controls
	// disappear there, so the title bar reclaims the whole width.
	drawn := captionButtonsDrawn(a.captionButtons, a.win != nil && a.win.IsFullScreen(), bar.Right)
	caption := bar.Right + titlebarEdgeInset
	if drawn {
		caption = titlebarCaptionWidth() + titlebarControlGap + titlebarEdgeInset
	}
	height := titlebarBandHeight()
	navigation := float32(0)
	if !a.fit.Sidebar {
		navigation = 10 + design.Controls().IconButtonSize*2 + 5
		if bar.Left > 0 {
			navigation += bar.Left + 5
		}
	}
	takeover := !onCanvas && a.workspaceTakeover()
	gapBudget := float32(24)
	if takeover {
		// In the source, row_left is sidebar-8 and the 8px row gap
		// cancels it. A collapsed sidebar instead clears the navigation.
		gapBudget = 0
		if !a.fit.Sidebar {
			gapBudget = 8
		}
	}
	surface, controls := titlebarWidths(columns.Right, columns.Files, viewport-columns.Sidebar-navigation-caption-gapBudget, caption)
	row := ui.Row(c).Key("main-titlebar").Height(height).Shrink(0).FillWidth().Padding(0, caption, 0, 0).AlignItems(ui.Center).DragWindow()
	if overlay {
		row.Absolute().Left(0).Right(0).Top(0)
	}
	row.Children(func() {
		if !a.fit.Sidebar {
			ui.Row(c).Width(navigation).Height(height).Shrink(0).Padding(4, 0, 0, 10).Gap(5).AlignItems(ui.Center).Children(func() {
				if bar.Left > 0 {
					ui.Spacer(c).Width(bar.Left).Shrink(0)
				}
				c.Key("titlebar-sidebar")
				iconButton(c, "sidebar-minimalistic", tr(c, "Toggle sidebar")).Shrink(0).OnClick(func() { a.setPanel(0, true) })
				c.Key("titlebar-back")
				iconButton(c, "alt-arrow-left", tr(c, "Back")).Shrink(0).OnClick(func() { a.back(-1) })
			})
		}
		identity := ui.Row(c).Grow(1).MinWidth(0).Height(height).AlignItems(ui.Center)
		if !takeover {
			identity.Padding(4, 8, 0, 10).Gap(8)
		}
		identity.Children(func() {
			if a.route == "settings" {
				ui.Text(c, tr(c, a.section)).FontWeight(600).SingleLine()
				return
			}
			if takeover {
				return
			}
			ui.Row(c).Grow(1).MinWidth(0).Clip().Gap(5).AlignItems(ui.Center).Children(func() {
				if session.ID != "" {
					ui.Text(c, session.Title).FontWeight(600).SingleLine().MinWidth(0)
					ui.Text(c, "~ @ "+a.deviceLabel()).TextColor(a.palette.Muted).SingleLine().Shrink(0)
				}
			})
			if session.ID != "" {
				c.Key("titlebar-session-info")
				iconButton(c, "info-circle", tr(c, "Session information")).Shrink(0).OnClick(a.toggleInfo)
			}
		})
		if a.route != "settings" && !onCanvas {
			if a.fit.Right && surface >= 44 {
				ui.Box(c).Width(surface).Height(height).Shrink(0).Clip().Children(func() { a.workspace.TabStrip(c, a.rightExpanded) })
			}
			ui.Row(c).Width(controls).Height(height).Shrink(0).Padding(4, 0, 0, 0).Gap(4).Justify(ui.End).AlignItems(ui.Center).Children(func() {
				c.Key("titlebar-files")
				iconButton(c, "folder-tree", tr(c, "Files")).Shrink(0).OnClick(func() { a.togglePanel(2) })
				c.Key("titlebar-right-pane")
				iconButton(c, "split-columns", tr(c, "Toggle right pane")).Shrink(0).OnClick(func() { a.togglePanel(1) }).ContextMenu(func(m *ui.Menu) {
					if m.Item(tr(c, "Expand pane")).Checked(a.rightExpanded).Chosen() {
						a.toggleWorkspaceExpanded()
					}
					if m.Item(tr(c, "Terminal panel")).Checked(a.terminal).Chosen() {
						a.toggleTerminal()
					}
				})
			})
		}
		if drawn {
			a.captionControls(c, height)
		}
	})
}

// captionControls draws the window's caption buttons where the platform's
// own are hidden. Each button keeps the title bar's shared 28px target, 16px
// glyph, 4px rhythm and 4px top padding, so the cluster sits on the same
// center line as the Files and pane toggles beside it.
func (a *App) captionControls(c *ui.Context, height float32) {
	maximized := a.win != nil && a.win.IsMaximized()
	maximizeIcon, maximizeLabel := captionMaximizeGlyph(maximized), tr(c, "Maximize window")
	if maximized {
		maximizeLabel = tr(c, "Restore window")
	}
	ui.Row(c).Key("window-caption-controls").Absolute().Top(0).Right(titlebarEdgeInset).Height(height).Padding(4, 0, 0, 0).Gap(titlebarControlGap).AlignItems(ui.Center).Children(func() {
		c.Key("window-minimize")
		iconButton(c, "caption-minimize", tr(c, "Minimize window")).Shrink(0).OnClick(func() {
			if a.win != nil {
				a.win.Minimize()
			}
		})
		c.Key("window-maximize")
		iconButton(c, maximizeIcon, maximizeLabel).Shrink(0).MaximizeButton().OnClick(func() {
			if a.win == nil {
				return
			}
			if a.win.IsMaximized() {
				a.win.Unmaximize()
				return
			}
			a.win.Maximize()
		})
		c.Key("window-close")
		windowCloseButton(c, tr(c, "Close window")).Shrink(0).OnClick(func() {
			if a.win != nil {
				a.win.Close()
			}
		})
	})
}
