package workbench

import (
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
func (s *State) TabStrip(c *ui.Context) {
	s.syncAppearance(c)
	p := s.palette()
	ui.Row(c).Key("workspace-tabs:"+s.scopeID).Label("Workspace tabs").FillWidth().Height(38).Shrink(0).Gap(4).Padding(4, 8, 0, 8).AlignItems(ui.Center).Background(p.Background).BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Children(func() {
		ui.ScrollHorizontal(c).Label("Scroll workspace tabs").Grow(1).MinWidth(0).Height(24).TrackScroll(&s.tabScroll).Children(func() {
			parts := ui.TabsBase(c, &s.active, len(s.tabs))
			var moveFrom, moveTo tabDrag
			move := false
			parts.List.Gap(4).Children(func() {
				for i, tab := range s.tabs {
					c.Key(tab.kind + ":" + tab.id)
					chip := parts.Tab(i).Label(tab.title).Tooltip(s.tabDetail(tab)).Width(112).Height(24).Padding(0, 4).Gap(3).Radius(6).Drag(tabDrag{tab.kind, tab.id}).OnClick(func() { s.selectTab(i) })
					selected := i == s.active
					if selected {
						chip.Background(sourceInk(p.Dark, .10))
					} else if chip.Hovered() {
						chip.Background(sourceInk(p.Dark, .06))
					}
					if selected && s.revealTab {
						chip.ScrollIntoView()
					}
					if dragged, ok := ui.Drop[tabDrag](chip); ok {
						moveFrom, moveTo, move = dragged, tabDrag{tab.kind, tab.id}, true
					}
					chip.ContextMenu(func(menu *ui.Menu) {
						if menu.Item("Close tab").Chosen() {
							s.closeTab(i)
						}
						if menu.Item("Close other tabs").Disabled(len(s.tabs) < 2).Chosen() {
							s.closeOtherTabs(tab)
						}
					})
					chip.Children(func() {
						ui.Box(c).Size(18, 18).Shrink(0).Center().Children(func() {
							if tab.kind == "file" && s.documents[tab.id] != nil {
								ui.Image(c, sourceFileIcon(s.documents[tab.id].file, false, p.Dark)).Size(14, 14)
							} else {
								ui.Icon(c, assets.Icon(tabIcon(tab.kind))).Size(12, 12).TextColor(p.Muted)
							}
						})
						color := p.Muted
						if selected {
							color = p.Text
						}
						ui.Text(c, tab.title).SingleLine().Grow(1).MinWidth(0).FontSize(design.UISize(c, 11.5)).TextColor(color)
						close := ui.ButtonBase(c).Label("Close tab "+tab.title).Tooltip("Close tab").Size(18, 18).Shrink(0).Radius(4).OnClick(func() { s.closeTab(i) })
						close.Children(func() {
							if chip.Hovered() || close.Hovered() || close.FocusVisible() {
								ui.Icon(c, assets.Icon("close")).Size(12, 12).TextColor(p.Muted)
							} else if d := s.documents[tab.id]; tab.kind == "file" && d != nil && d.editor.Buffer.Modified() {
								ui.Box(c).Size(5, 5).Radius(3).Background(p.Muted)
							}
						})
					})
				}
			})
			if move {
				s.moveTab(moveFrom, moveTo)
			}
			parts.List.OnChange(func() { s.selectTab(s.active) })
		})
		if s.OnToggleExpand != nil {
			s.iconButton(c, "expand-arrows", "Expand workspace", s.OnToggleExpand)
		}
		ui.ButtonBase(c).Label("New surface").Tooltip("New tab").Size(24, 24).Shrink(0).Radius(6).Menu(func(menu *ui.Menu) {
			for _, item := range []struct{ kind, title string }{{"files", "Files"}, {"browser", "Browser"}, {"terminal", "Terminal"}, {"changes", "Diffs"}, {"history", "History"}} {
				if menu.Item(item.title).Disabled((item.kind == "changes" || item.kind == "history") && !s.hasGit()).Chosen() {
					s.OpenSurface(item.kind)
				}
			}
		}).Children(func() { ui.Icon(c, assets.Icon("plus")).Size(13, 13).TextColor(p.Muted) })
	})
	s.revealTab = false
}

func tabIcon(kind string) string {
	switch kind {
	case "browser":
		return "globe"
	case "terminal":
		return "terminal"
	case "session":
		return "chat-round-line"
	case "history":
		return "git-branch"
	case "diff", "changes":
		return "list"
	case "pr", "pullrequests":
		return "pull-request"
	case "files":
		return "folder"
	default:
		return "plus"
	}
}

func (s *State) tabDetail(tab surface) string {
	if d := s.documents[tab.id]; tab.kind == "file" && d != nil {
		return d.file.Path
	}
	return tab.title
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
