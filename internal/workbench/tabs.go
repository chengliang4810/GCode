package workbench

import (
	"time"

	"gcode/internal/assets"
	"gcode/internal/design"

	"github.com/egoist/mygo/ui"
)

type tabDrag struct{ Kind, ID string }

func (s *State) syncAppearance(c *ui.Context) {
	s.systemDark = c.Theme().Dark
	for _, terminal := range s.terminals {
		s.configureTerminal(terminal)
	}
}

// TabStrip uses the official TabsBase focus and keyboard model inside a native
// horizontal scroller. Chips keep the reference's 112×24 geometry at overflow.
func (s *State) TabStrip(c *ui.Context, expanded bool) {
	// Consume only the request this pass can render. A synchronous menu
	// choice below may select a new tab after the chips were already built;
	// that request belongs to the next build pass.
	reveal := s.revealTab
	s.revealTab = false
	s.syncAppearance(c)
	p := s.palette()
	tips := s.tabTooltip.begin(c, s.scopeID, len(s.tabs))
	// shell/tabs.rs supplies one 4px top inset over the transparent titlebar;
	// its fixed expand button follows the scrolling chips and add-tab button.
	ui.Row(c).Key("workspace-tabs:"+s.scopeID).Label(tr(c, "Workspace tabs")).FillWidth().Height(38).Shrink(0).Gap(4).Padding(4, 4, 0, 8).AlignItems(ui.Center).Clip().Children(func() {
		scroller := ui.ScrollHorizontal(c).Label(tr(c, "Scroll workspace tabs")).Grow(1).MinWidth(0).FillHeight().Gap(4).TrackScroll(&s.tabScroll).Scrollbars(false)
		if design.Glass(p, s.snapshot.Settings.Appearance.Surface, c.Vibrancy()).A == 255 {
			// TrackScroll commits these plain values during layout. Read them
			// at paint time so a MaxX-only change cannot leave a stale fade
			// while the window is idle; no additional frame is needed.
			scroll := &s.tabScroll
			background := p.Shell
			scroller.DrawOver(func(painter *ui.Painter, r ui.Rect) {
				paintTabStripFades(painter, r, background, scroll.X > 1, scroll.X < scroll.MaxX-1)
			})
		}
		scroller.Children(func() {
			parts := ui.TabsBase(c, &s.active, len(s.tabs))
			var moveFrom, moveTo tabDrag
			move := false
			parts.List.Gap(4).Children(func() {
				for i, tab := range s.tabs {
					// Keep close beside the semantic tab: MyGo treats tabs as
					// accessibility leaves and would hide a nested button.
					slot := ui.Box(c.Key("workspace-tab-slot:"+tab.kind+":"+tab.id)).Size(112, 24).Shrink(0)
					slot.Children(func() {
						c.Key(tab.kind + ":" + tab.id)
						detail := s.tabDetail(tab)
						chip := parts.Tab(i).Label(s.tabTitle(tab)).Description(detail).Width(112).Height(24).Padding(0, 4).Gap(3).Radius(6).Drag(tabDrag{tab.kind, tab.id}).OnClick(func() { s.selectTab(i) })
						s.tabObserver.bind("chip", tab.id, chip)
						s.tabTooltip.observe(c, &tips, i, false, detail, chip, slot.Hovered())
						selected := i == s.active
						if selected || slot.Hovered() {
							dark := p.Dark
							chip.Draw(func(painter *ui.Painter, r ui.Rect) {
								paintTabChipWash(painter, r, dark, selected)
							})
						}
						if selected && reveal {
							chip.ScrollIntoView()
						}
						if dragged, ok := ui.Drop[tabDrag](chip); ok {
							moveFrom, moveTo, move = dragged, tabDrag{tab.kind, tab.id}, true
						}
						chip.ContextMenu(func(menu *ui.Menu) {
							if menu.Item(tr(c, "Close tab")).Chosen() {
								s.closeTab(i)
							}
							if menu.Item(tr(c, "Close other tabs")).Disabled(len(s.tabs) < 2).Chosen() {
								s.closeOtherTabs(tab)
							}
						})
						chip.Children(func() {
							ui.Box(c).Size(18, 18).Shrink(0).Center().Children(func() {
								if tab.kind == "file" && s.documents[tab.id] != nil {
									icon := ui.Image(c, sourceFileIcon(s.documents[tab.id].file, false, p.Dark)).Size(14, 14)
									if !selected {
										icon.Opacity(.78)
									}
								} else {
									icon := ui.Icon(c, assets.Icon(tabIcon(tab.kind))).Size(12, 12).TextColor(p.Muted)
									if !selected {
										icon.Opacity(.7)
									}
								}
							})
							color := p.Muted
							if selected {
								color = p.Text
							}
							ui.Text(c, s.tabTitle(tab)).SingleLine().Grow(1).MinWidth(0).FontSize(design.TextSize(c, design.TextBase)).TextColor(color)
							ui.Spacer(c).Width(18).Shrink(0)
						})
						close := ui.ButtonBase(c.Key("workspace-close:"+tab.kind+":"+tab.id)).Label(tr(c, "Close tab ")+s.tabTitle(tab)).Description(tr(c, "Close tab")).Absolute().Right(4).Top(3).Size(18, 18).Radius(4).OnClick(func() { s.closeTab(i) })
						s.tabTooltip.observe(c, &tips, i, true, tr(c, "Close tab"), close, close.Hovered())
						close.Children(func() {
							if chip.Hovered() || close.Hovered() || close.FocusVisible() {
								ui.Icon(c, assets.Icon("close")).Size(12, 12).TextColor(p.Muted)
							} else if d := s.documents[tab.id]; tab.kind == "file" && d != nil && d.editor.Buffer.Modified() {
								ui.Box(c).Size(6, 6).Radius(3).Background(p.Muted)
							}
						})
					})
				}
			})
			// Keep the menu button outside the TabsBase focus group, while
			// including it in the same scrolling content as the chips.
			if len(s.tabs) > 0 {
				plus := tabStripButton(c, "add-surface", "New surface", "New tab", "plus", 24, 13, p)
				s.tabObserver.bind("plus", "", plus)
				plus.Menu(func(menu *ui.Menu) {
					if s.OnNewSideChat != nil && s.scopeID != "" {
						if menu.Item(tr(c, "Side chat")).Disabled(!s.canNewSideChat()).Chosen() {
							s.OpenSurface("sidechat")
						}
					}
					for _, item := range []struct{ kind, title string }{{"files", "Files"}, {"browser", "Browser"}, {"terminal", "Terminal"}, {"changes", "Changes"}, {"history", "History"}} {
						if (item.kind == "changes" || item.kind == "history") && !s.hasGit() {
							continue
						}
						if menu.Item(tr(c, item.title)).Chosen() {
							if item.kind == "terminal" {
								s.NewTerminal()
							} else {
								s.OpenSurface(item.kind)
							}
						}
					}
				})
			}
			if move {
				s.moveTab(moveFrom, moveTo)
			}
			parts.List.OnChange(func() { s.selectTab(s.active) })
		})
		if s.OnToggleExpand != nil {
			icon, tooltip := "expand-arrows", "Expand panel"
			if expanded {
				icon, tooltip = "collapse-arrows", "Collapse panel"
			}
			expand := tabStripButton(c, "expand-workspace", "Expand workspace", tooltip, icon, 28, 16, p).OnClick(s.OnToggleExpand)
			s.tabObserver.bind("expand", "", expand)
		}
	})
	s.renderTabTooltip(c, tips)
}

// Opaque strips use shell-color ramps; glass requires a subtree alpha mask.
func paintTabStripFades(painter *ui.Painter, r ui.Rect, background ui.Color, left, right bool) {
	const band float32 = 36
	painter.Clip(r, 0, func() {
		if left {
			painter.FillGradient(ui.Rect{X: r.X, Y: r.Y, W: band, H: r.H}, ui.LinearGradient{From: background, To: background.Alpha(0), Angle: 90}, 0)
		}
		if right {
			painter.FillGradient(ui.Rect{X: r.X + r.W - band, Y: r.Y, W: band, H: r.H}, ui.LinearGradient{From: background, To: background.Alpha(0), Angle: 270}, 0)
		}
	})
}

func tabStripButton(c *ui.Context, key, label, tooltip, icon string, size, glyphSize float32, p design.Tokens) ui.Element {
	c.Key(key)
	button := ui.ButtonBase(c).Label(tr(c, label)).Tooltip(tr(c, tooltip)).Size(size, size).Shrink(0).Radius(6).Center()
	alpha := float32(0)
	if button.Hovered() {
		alpha = .11
	}
	// Button hover keeps its existing byte-color animation path.
	wash := ui.RGBA(26, 26, 26, alpha)
	if p.Dark {
		wash = ui.RGBA(235, 235, 235, alpha)
	}
	if c.Preferences().ReduceMotion {
		button.Background(wash)
	} else {
		button.Background(button.AnimateColor("hover", wash, ui.Animation{Duration: 150 * time.Millisecond, Ease: ui.CubicBezier(.4, 0, .2, 1)}))
	}
	button.Children(func() { ui.Icon(c, assets.Icon(icon)).Size(glyphSize, glyphSize).TextColor(p.Muted) })
	return button
}

func tabIcon(kind string) string {
	switch kind {
	case "browser":
		return "globe"
	case "terminal":
		return "terminal"
	case "session":
		return "chat-round-line"
	case "trajectory":
		return "list"
	case "goal":
		return "checklist"
	case "history":
		return "git-branch"
	case "diff", "changes":
		return "list"
	case "files":
		return "folder"
	default:
		return "plus"
	}
}

func (s *State) tabDetail(tab surface) string {
	if tab.kind == "trajectory" {
		return tab.title + "\n" + tab.id
	}
	if d := s.documents[tab.id]; tab.kind == "file" && d != nil {
		return d.file.Path
	}
	if b := s.browsers[tab.id]; tab.kind == "browser" && b != nil {
		return b.data.URL
	}
	return ""
}

func (s *State) moveTab(from, to tabDrag) {
	first, last := -1, -1
	for i, tab := range s.tabs {
		if tab.kind == from.Kind && tab.id == from.ID {
			first = i
		}
		if tab.kind == to.Kind && tab.id == to.ID {
			last = i
		}
	}
	if first < 0 || last < 0 || first == last {
		return
	}
	active := s.tabs[s.active]
	moved := s.tabs[first]
	if first < last {
		copy(s.tabs[first:last], s.tabs[first+1:last+1])
	} else {
		copy(s.tabs[last+1:first+1], s.tabs[last:first])
	}
	s.tabs[last] = moved
	for i, tab := range s.tabs {
		if tab == active {
			s.selectTab(i)
			break
		}
	}
}

func (s *State) closeOtherTabs(keep surface) {
	for i := len(s.tabs) - 1; i >= 0; i-- {
		if s.tabs[i] != keep {
			s.closeTab(i)
			if s.pendingClose != "" {
				return
			}
		}
	}
}
