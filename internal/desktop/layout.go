package desktop

import (
	"strconv"
	"strings"

	"gcode/internal/appmodel"
	"github.com/egoist/mygo/ui"
)

const desktopHeaderMinHeight float32 = 48

// Messages and the composer share one reading column, including when the
// resource pane is hidden and the conversation has the whole window width.
const conversationColumnMaxWidth float32 = 748

func desktopHeaderHeight(c *ui.Context) float32 {
	height := desktopHeaderMinHeight
	if titleBarHeight := c.TitleBar().Height; titleBarHeight > height {
		height = titleBarHeight
	}
	return height
}

func (a *App) sidebar(c *ui.Context, p palette, data appmodel.Snapshot, activeID string, visualSample bool) {
	active, _ := activeSession(data.Sessions, activeID)
	activeProjectID := active.ProjectID
	if activeProjectID == "" && len(data.Projects) > 0 {
		activeProjectID = data.Projects[0].ID
	}
	ui.Column(c).Fill().Background(p.sidebar).Children(func() {
		a.sidebarHeader(c, p, activeProjectID)
		ui.Row(c).Height(36).Padding(0, 16).AlignItems(ui.Center).Children(func() {
			sectionLabel(c, p, "项目")
			ui.Spacer(c)
			ui.Text(c, stringCount(len(data.Projects))).FontSize(10.5).TextColor(p.muted)
		})
		ui.Scroll(c.Key("project-tree")).Grow(1).Padding(0, 8, 12, 8).Gap(12).Label("项目和对话").Children(func() {
			if len(data.Projects) == 0 {
				a.projectEmpty(c, p)
			} else {
				sessions := make(map[string][]appmodel.Session, len(data.Projects))
				for _, session := range data.Sessions {
					sessions[session.ProjectID] = append(sessions[session.ProjectID], session)
				}
				for _, project := range data.Projects {
					a.projectGroup(c, p, project, sessions[project.ID], activeID)
				}
			}
		})
		ui.Divider(c).Background(p.border)
		ui.Row(c).Height(44).Padding(0, 12).AlignItems(ui.Center).Children(func() {
			if visualSample {
				ui.Text(c, "示例数据").FontSize(10).TextColor(p.warning).SingleLine()
				ui.Spacer(c)
			}
			settings := compactAction(c, p, "settings", "设置")
			if settings.Clicked() {
				a.openSettings()
			}
		})
	})
}

func (a *App) sidebarHeader(c *ui.Context, p palette, activeProjectID string) {
	bar := c.TitleBar()
	left := bar.Left + 12
	if left < 16 {
		left = 16
	}
	right := bar.Right + 12
	if right < 12 {
		right = 12
	}
	height := desktopHeaderHeight(c)
	windowWidth, _ := c.Size()
	iconSize, buttonSize, gap, brandWidth := float32(25), float32(30), float32(7), float32(46)
	showBrand := windowWidth >= 1100
	if windowWidth < 1100 {
		iconSize, buttonSize, gap = 22, 28, 5
	}
	ui.Row(c).Height(height).Padding(0, right, 0, left).Gap(gap).AlignItems(ui.Center).DragWindow().Children(func() {
		ui.Row(c).Size(iconSize, iconSize).Radius(7).Center().Background(p.accentSoft).Label("GCode").Children(func() {
			ui.Text(c, "G").FontSize(15).FontWeight(700).TextColor(p.accent)
		})
		if showBrand {
			ui.Text(c, "GCode").FontSize(14).FontWeight(600).SingleLine().Width(brandWidth).Shrink(0)
		}
		if iconButton(c, p, "plus", "新会话").Size(buttonSize, buttonSize).Radius(8).Clicked() {
			a.newSession(activeProjectID)
		}
	})
}

func (a *App) projectEmpty(c *ui.Context, p palette) {
	ui.Column(c).Padding(8, 4).Gap(8).Background(p.surface).Radius(8).Border(1, p.border).Children(func() {
		ui.Text(c, "打开本地项目").FontSize(13.5).FontWeight(600)
		ui.Text(c, "打开本地目录后，可以在这里创建会话开始编码。").FontSize(11.5).LineHeight(1.4).TextColor(p.muted)
		path := a.projectPathValue()
		field := ui.TextInput(c.Key("project-path"), &path).Placeholder("/Users/.../project").Label("项目目录")
		if field.Changed() {
			a.setProjectPath(path)
		}
		if primaryButton(c, p, "folder", "打开项目").Clicked() {
			a.addProject(path)
		}
	})
}

func (a *App) projectGroup(c *ui.Context, p palette, project appmodel.Project, sessions []appmodel.Session, activeID string) {
	name := strings.TrimSpace(project.Name)
	if name == "" {
		name = "未命名项目"
	}
	a.mu.RLock()
	open, remembered := a.projectExpanded[project.ID]
	a.mu.RUnlock()
	if !remembered {
		open = true
	}
	ui.Column(c.Key("project-group-" + project.ID)).Gap(4).MinWidth(0).Children(func() {
		var parts ui.CollapsibleParts
		ui.Row(c).Height(32).Gap(2).AlignItems(ui.Center).Children(func() {
			parts = ui.CollapsibleBase(c.Key("project-"+project.ID), &open)
			trigger := parts.Trigger.Grow(1).MinWidth(0).Height(32).Padding(0, 6).Gap(7).Radius(6).
				AlignItems(ui.Center).Label("项目 " + name).Tooltip(project.Path).TextColor(p.text)
			if trigger.Hovered() {
				trigger = trigger.Background(p.surfaceAlt)
			}
			trigger.Children(func() {
				chevron := "chevron-right"
				if open {
					chevron = "chevron-down"
				}
				ui.Icon(c, icon(chevron)).FontSize(12).TextColor(p.muted)
				ui.Icon(c, icon("folder")).FontSize(14).TextColor(p.muted)
				ui.Text(c, name).FontSize(12.5).FontWeight(550).MinWidth(0).Grow(1).SingleLine()
				ui.Text(c, stringCount(len(sessions))).FontSize(10.5).TextColor(p.muted)
			})
			if trigger.Changed() {
				a.mu.Lock()
				a.projectExpanded[project.ID] = open
				a.mu.Unlock()
			}
			if iconButton(c.Key("new-session-"+project.ID), p, "plus", "新会话 · "+name).Size(28, 28).Radius(6).Clicked() {
				a.newSession(project.ID)
			}
		})
		// Build the panel beside its header, so closing the project hides only
		// its own children. The native disclosure keeps keyboard/AX semantics.
		if panel := parts.Panel(func() {
			ui.Column(c).Margin(0, 0, 0, 16).Padding(0, 0, 0, 8).Gap(2).
				BorderWidth(0, 0, 0, 1).BorderColor(p.border).Children(func() {
				if len(sessions) == 0 {
					ui.Text(c, "暂无对话").FontSize(11.5).TextColor(p.muted).Padding(8)
				}
				for _, session := range sessions {
					a.sessionRow(c, p, session, session.ID == activeID)
				}
			})
		}); panel.Valid() {
			panel.MinWidth(0)
		}
	})
}

func (a *App) sessionRow(c *ui.Context, p palette, session appmodel.Session, selected bool) {
	b := ui.ButtonBase(c.Key("session-"+session.ID)).Height(48).Padding(6, 8).Radius(7).Gap(7).
		AlignItems(ui.Center).Label("会话 " + session.Title).Tooltip(session.Cwd).TextColor(p.text)
	if selected {
		b = b.Background(p.selection)
	} else if b.Hovered() {
		b = b.Background(p.surfaceAlt)
	}
	b.Children(func() {
		tone := p.muted
		if session.State != appmodel.Idle {
			tone = p.statusColor(stateTone(session.State))
		}
		ui.Box(c).Size(6, 6).Radius(3).Background(tone).Tooltip(stateLabel(session.State))
		ui.Column(c).Gap(2).MinWidth(0).Grow(1).Children(func() {
			title := session.Title
			if title == "" {
				title = "未命名会话"
			}
			ui.Text(c, title).FontSize(12.5).FontWeight(550).SingleLine()
			ui.Row(c).Gap(6).MinWidth(0).AlignItems(ui.Center).Children(func() {
				branch := strings.TrimSpace(session.Branch)
				if branch == "" {
					branch = "本地目录"
				}
				ui.Text(c, branch).FontSize(10.5).TextColor(p.muted).Grow(1).MinWidth(0).SingleLine()
				ui.Text(c, relativeTime(session.UpdatedAt)).FontSize(10).TextColor(p.muted).SingleLine().Shrink(0)
			})
		})
		if count := queuedCount(session); count != "" {
			ui.Text(c, count).FontSize(10.5).TextColor(p.muted).SingleLine()
		}
	})
	if b.Clicked() {
		a.selectSession(session.ID)
	}
}

func projectName(projects []appmodel.Project, id string) string {
	for _, project := range projects {
		if project.ID == id {
			return project.Name
		}
	}
	return ""
}

func (a *App) toggleRightPanel(visible, narrow bool) {
	a.mu.Lock()
	a.rightOpen = !visible
	if narrow {
		a.rightResponsiveOverride = true
	} else {
		a.rightResponsiveOverride = false
	}
	a.mu.Unlock()
}

func (a *App) projectPathValue() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.projectPath
}

func (a *App) setProjectPath(path string) {
	a.mu.Lock()
	a.projectPath = path
	a.mu.Unlock()
}

func stringCount(value int) string {
	return strconv.Itoa(value)
}

const rightPanelMinimumWidth float32 = 290

func clampSidebarWidth(windowWidth, requested float32) float32 {
	minimum := float32(176)
	maximum := windowWidth * 0.22
	if maximum < minimum {
		maximum = minimum
	}
	if maximum > 220 {
		maximum = 220
	}
	if requested < minimum {
		return minimum
	}
	if requested > maximum {
		return maximum
	}
	return requested
}

func clampCenterWidth(windowWidth, leftWidth, requested float32) float32 {
	maximum := windowWidth - leftWidth - rightPanelMinimumWidth - 2
	if maximum < 0 {
		maximum = 0
	}
	if requested < 0 {
		requested = 0
	}
	if requested > maximum {
		return maximum
	}
	return requested
}
