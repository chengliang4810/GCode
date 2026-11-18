package desktop

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"gcode/internal/appusage"
	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
	localize "gcode/internal/i18n/view"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type menuBarState struct {
	Tray       *mygo.Tray
	Window     *mygo.Window
	Tab        string
	Error      string
	Title      string
	UsageDay   string
	UsageDirty bool
}

func (a *App) syncMenuBar() {
	if a.win == nil || a.closed {
		return
	}
	s := &a.menuBar
	if !a.state.Settings.MenuBar {
		a.closeMenuBar()
		return
	}
	if s.Tray == nil {
		png, err := assets.MenuBarIcon()
		if err == nil {
			s.Tray, err = mygo.NewTray(mygo.TrayOptions{ID: "gcode-task-usage", Icon: png, IconIsTemplate: true, ToolTip: a.t("Tasks and usage")})
		}
		if err != nil {
			s.Error = err.Error()
			return
		}
		s.Error = ""
		s.Tray.OnClick(a.openMenuPanel)
		s.Tray.OnRightClick(a.openMenuPanel)
		a.syncMenuBarTitle()
		a.refreshMenuBarUsage()
	}
	a.syncMenuBarTitle()
	if s.Window != nil {
		s.Window.Invalidate()
	}
}

func (a *App) closeMenuBar() {
	s := &a.menuBar
	if s.Window != nil {
		window := s.Window
		s.Window = nil
		window.Close()
	}
	if s.Tray != nil {
		s.Tray.Destroy()
		s.Tray = nil
	}
	s.Title = ""
	s.UsageDay = ""
}

func (a *App) openMenuPanel() {
	s := &a.menuBar
	if s.Tray == nil {
		return
	}
	if s.Window == nil {
		spec := design.MenuPanel()
		window := mygo.NewFlyout(mygo.FlyoutOptions{
			Tray: s.Tray, Placement: mygo.PlacementBottomEnd, Gap: 6, Width: spec.Width, Height: spec.Height,
			Hidden: true, Focusable: true, Shadow: true, CornerRadius: spec.Radius,
			Vibrancy: mygo.VibrancyPopover, Content: ui.View(a.menuPanelView),
		})
		s.Window = window
		window.SetTitle("GCode — " + a.t("Tasks and usage"))
		window.OnClose(func(event *mygo.CloseEvent) {
			if !a.closed && s.Window == window {
				event.PreventDefault()
				window.Hide()
			}
		})
	}
	if s.Window.IsVisible() {
		s.Window.Hide()
		return
	}
	a.refreshAppUsage()
	s.Window.Show()
}

func (a *App) menuPanelNavigate(route string) {
	if a.menuBar.Window != nil {
		a.menuBar.Window.Hide()
	}
	if a.win == nil {
		if route != "" {
			a.navigate(route)
		}
		return
	}
	a.win.Update(func() {
		if a.closed {
			return
		}
		if route != "" {
			a.navigate(route)
		}
		a.win.Show()
		a.win.Focus()
	})
}

func menuPanelTasks(snapshot contracts.Snapshot, active bool) []contracts.Session {
	tasks := []contracts.Session{}
	for _, session := range snapshot.Sessions {
		if session.Archived || session.SubagentType != "" {
			continue
		}
		running := session.Status == contracts.Working || session.Status == contracts.AwaitingInput
		if active && running || !active && session.Unread && !running {
			tasks = append(tasks, session)
		}
	}
	slices.SortFunc(tasks, func(a, b contracts.Session) int {
		if n := b.UpdatedAt.Compare(a.UpdatedAt); n != 0 {
			return n
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return tasks
}

func (a *App) menuPanelView(c *ui.Context) {
	localize.Set(c, a.state.Settings.General.Language)
	c.SetTheme(a.palette.UI())
	spec := design.MenuPanel()
	theme := c.Theme()
	background := theme.Background
	if c.Vibrancy() {
		background = design.Glass(a.palette, "frosted", true)
	}
	c.Root().Background(ui.Transparent)
	now := time.Now()
	unread := menuPanelTasks(a.state, false)
	active := menuPanelTasks(a.state, true)
	today := a.appUsage.report.Days[now.Format("2006-01-02")]
	var trend [30]int64
	var total int64
	for i := range trend {
		day := now.AddDate(0, 0, i-29)
		trend[i] = a.appUsage.report.Days[day.Format("2006-01-02")].Tokens
		total += trend[i]
	}
	ui.Column(c.Key("menubar-panel")).Fill().Padding(spec.Padding).Gap(spec.Gap).Background(background).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "GCode").Bold().FontSize(18)
			ui.Text(c, a.menuPanelUpdated(c)).Grow(1).TextColor(theme.TextMuted)
			iconButton(c.Key("menubar-refresh"), "refresh", tr(c, "Refresh")).Disabled(a.appUsage.loading).OnClick(a.refreshAppUsage)
		})
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			ui.Text(c, tr(c, "Your work, at a glance")).FontSize(design.TextSize(c, spec.Heading)).Bold()
			ui.Text(c, fmt.Sprintf(tr(c, "%d active, %d unread"), len(active), len(unread))).TextColor(theme.TextMuted).FontFeatures("tnum")
		})
		ui.Row(c).FillWidth().Gap(4).Children(func() {
			for _, tab := range []struct{ key, label string }{{"", "Unread"}, {"active", "In progress"}, {"today", "Usage today"}} {
				tab := tab
				controlNavButton(c.Key("menubar-tab-"+tab.key), tr(c, tab.label), a.menuBar.Tab == tab.key).Grow(1).Children(func() { ui.Text(c, tr(c, tab.label)) }).OnClick(func() { a.menuBar.Tab = tab.key })
			}
		})
		ui.Scroll(c.Key("menubar-content")).Grow(1).FillWidth().MinHeight(0).Gap(8).Children(func() {
			if a.menuBar.Tab == "today" {
				if a.appUsage.errorText != "" {
					ui.Text(c, a.appUsage.errorText).Wrap().TextColor(theme.Danger)
					return
				}
				if a.appUsage.generated.IsZero() {
					ui.Text(c, tr(c, "Loading usage…")).TextColor(theme.TextMuted)
					return
				}
				ui.Text(c, compactUsage(today.Tokens)).FontSize(design.TextSize(c, spec.Metric)).Bold().FontFeatures("tnum")
				ui.Text(c, tr(c, "Token today")).TextColor(theme.TextMuted)
				ui.Text(c, fmt.Sprintf(tr(c, "%d turns, %d tools"), today.Turns, today.Tools)).TextColor(theme.TextMuted).FontFeatures("tnum")
				for _, model := range appusage.Models([]appusage.Day{today}) {
					ui.Row(c).FillWidth().Gap(8).Children(func() {
						ui.Text(c, a.usageModelName(model.ID)).Grow(1).SingleLine()
						ui.Text(c, compactUsage(model.Tokens)).FontFeatures("tnum")
					})
				}
				return
			}
			tasks := unread
			empty := "No unread results"
			if a.menuBar.Tab == "active" {
				tasks = active
				empty = "No tasks in progress"
			}
			if len(tasks) == 0 {
				ui.Text(c, tr(c, empty)).TextColor(theme.TextMuted).Margin(0, 8).Wrap()
			}
			for _, value := range tasks {
				session := value
				controlTileButton(c.Key("menubar-session-"+session.ID), false).FillWidth().Label(session.Title).OnClick(func() { a.menuPanelNavigate("chat/" + session.ID) }).Children(func() {
					ui.Column(c).FillWidth().Gap(5).Children(func() {
						ui.Text(c, session.Title).Bold().SingleLine()
						label := "Completed"
						if session.Status == contracts.Working {
							label = "Working"
						}
						if session.Status == contracts.AwaitingInput {
							label = "Waiting for input"
						}
						if session.Status == contracts.Errored {
							label = "Failed"
						}
						ui.Text(c, tr(c, label)).TextColor(theme.TextMuted)
					})
				})
			}
		})
		controlTileButton(c.Key("menubar-usage-summary"), false).FillWidth().Label(tr(c, "Open usage statistics")).OnClick(func() { a.menuPanelNavigate("settings/Usage statistics") }).Children(func() {
			ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).Gap(5).Children(func() {
					ui.Text(c, tr(c, "Last 30 days on this host")).TextColor(theme.TextMuted)
					value := "—"
					if !a.appUsage.generated.IsZero() && a.appUsage.errorText == "" {
						value = compactUsage(total)
					}
					ui.Text(c, value+" "+tr(c, "Tokens")).Bold().FontFeatures("tnum")
				})
				menuUsageTrend(c, trend, theme.Accent)
				icon(c, "alt-arrow-right", 14)
			})
		})
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			buttonWithIcon(c.Key("menubar-open-app"), tr(c, "Open GCode"), "action-play", buttonSecondary).Grow(1).OnClick(func() { a.menuPanelNavigate("") })
			iconButton(c.Key("menubar-settings"), "settings", tr(c, "Menu bar settings")).OnClick(func() { a.menuPanelNavigate("settings/Usage statistics") })
		})
	})
}

func (a *App) menuPanelUpdated(c *ui.Context) string {
	if a.appUsage.loading {
		return tr(c, "Refreshing…")
	}
	if a.appUsage.errorText != "" {
		return tr(c, "Refresh failed")
	}
	if a.appUsage.generated.IsZero() {
		return tr(c, "Not refreshed")
	}
	return fmt.Sprintf(tr(c, "Updated at %s"), i18n.Date(a.state.Settings.General.Language, a.appUsage.generated.Local(), "15:04"))
}

func menuUsageTrend(c *ui.Context, values [30]int64, color ui.Color) {
	ui.Box(c.Key("menubar-usage-trend")).Size(80, 32).Label(tr(c, "30-day Token trend")).Draw(func(p *ui.Painter, r ui.Rect) {
		var peak int64
		for _, value := range values {
			peak = max(peak, value)
		}
		points := make([][2]float32, len(values))
		for i, value := range values {
			points[i] = [2]float32{r.X + float32(i)*r.W/29, r.Y + r.H - 2 - float32(value)/float32(max(1, peak))*(r.H-4)}
		}
		p.StrokePolyline(points, 1.5, color)
	})
}

// The menu bar is useful without opening its panel. Only committed usage events
// and a calendar rollover refresh this reading; streamed text does not poll it.
func (a *App) menuBarUsageTitle(now time.Time) string {
	if a.appUsage.generated.IsZero() || a.appUsage.errorText != "" {
		return "—"
	}
	return compactUsage(a.appUsage.report.Days[now.Format("2006-01-02")].Tokens) + " " + a.t("Tokens")
}

func (a *App) syncMenuBarTitle() {
	s := &a.menuBar
	if s.Tray == nil {
		return
	}
	title := a.menuBarUsageTitle(time.Now())
	if title != s.Title {
		s.Title = title
		s.Tray.SetTitle(title)
	}
}

func (a *App) refreshMenuBarDay() {
	s := &a.menuBar
	if s.Tray != nil && !a.appUsage.loading && s.UsageDay != time.Now().Format("2006-01-02") {
		a.refreshAppUsage()
	}
}

func (a *App) refreshMenuBarUsage() {
	if a.menuBar.Tray == nil {
		return
	}
	if a.appUsage.loading {
		a.menuBar.UsageDirty = true
		return
	}
	a.menuBar.UsageDirty = false
	a.refreshAppUsage()
}

func (a *App) menuBarSettings(c *ui.Context) {
	a.settingsCard(c, func() {
		a.settingsRow(c, "Show tasks and usage in menu bar", "Unread results, active tasks and Token usage.", func() {
			ui.Row(c).Gap(8).Children(func() {
				iconButton(c.Key("menubar-preview"), "widget", tr(c, "Open menu bar panel")).Disabled(!a.state.Settings.MenuBar).OnClick(a.openMenuPanel)
				a.controlSwitch(c.Key("menubar-enabled"), &a.state.Settings.MenuBar, tr(c, "Toggle ")+tr(c, "Show tasks and usage in menu bar"), false, func(bool) { a.syncMenuBar(); a.saveSettings() })
			})
		})
		if a.menuBar.Error != "" {
			ui.Text(c, a.menuBar.Error).Wrap().TextColor(a.palette.Danger)
		}
	})
}
