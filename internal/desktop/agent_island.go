package desktop

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"time"

	"gcode/internal/assets"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
	localize "gcode/internal/i18n/view"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type agentIslandState struct {
	Window          *mygo.Window
	Windows         map[int64]*mygo.Window
	Displays        []mygo.Display
	Expanded        bool
	HoverExpanded   bool
	Hovered         map[int64]bool
	HoverTimer      *time.Timer
	HoverGeneration uint64
	HoverCooldown   time.Time
	Dismissed       map[string]bool
	Observed        map[string]string
	CollapseAt      time.Time
	Height          int
	Width           int
}

type islandEntry struct {
	SessionID, Title, Phase, Detail, Token string
	StartedAt, UpdatedAt                   time.Time
}

func islandRank(phase string) int {
	switch phase {
	case "needs-interaction":
		return 0
	case "error":
		return 1
	case "running":
		return 2
	case "interrupted":
		return 3
	default:
		return 4
	}
}

// islandEntries uses the core's lightweight run index, including sessions whose
// transcript is not subscribed. It never scans or loads historical message bodies.
func islandEntries(state contracts.Snapshot, now time.Time, dismissed map[string]bool) []islandEntry {
	latest := make(map[string]contracts.Run)
	for _, run := range state.Runs {
		if previous, ok := latest[run.SessionID]; !ok || !run.StartedAt.Before(previous.StartedAt) {
			latest[run.SessionID] = run
		}
	}
	entries := []islandEntry{}
	for _, session := range state.Sessions {
		if session.Archived || session.SubagentType != "" || session.SideChat {
			continue
		}
		run, ok := latest[session.ID]
		if !ok {
			continue
		}
		entry := islandEntry{SessionID: session.ID, Title: session.Title, StartedAt: run.StartedAt, UpdatedAt: session.UpdatedAt, Token: run.ID}
		switch {
		case session.Status == contracts.AwaitingInput:
			entry.Phase, entry.Detail = "needs-interaction", "Waiting for input"
		case session.Status == contracts.Working || run.Status == "running":
			entry.Phase, entry.Detail = "running", "Working"
		default:
			if !session.Unread || dismissed[run.ID] || now.Sub(session.UpdatedAt) > 24*time.Hour {
				continue
			}
			entry.Phase, entry.Detail = "completed", "Completed"
			// A turn that never finished is not a completion: it keeps its own
			// phase so both the colour and the reminder stay neutral.
			if run.Status == "failed" || session.Status == contracts.Errored {
				entry.Phase, entry.Detail = "error", "Failed"
			} else if run.Status == "interrupted" {
				entry.Phase, entry.Detail = "interrupted", "Interrupted"
			}
		}
		entries = append(entries, entry)
	}
	slices.SortFunc(entries, func(a, b islandEntry) int {
		if priority := cmp.Compare(islandRank(a.Phase), islandRank(b.Phase)); priority != 0 {
			return priority
		}
		if n := b.UpdatedAt.Compare(a.UpdatedAt); n != 0 {
			return n
		}
		return cmp.Compare(a.SessionID, b.SessionID)
	})
	return entries
}

func (a *App) syncAgentIsland() {
	if a.win == nil || a.win.ID() == 0 || a.closed {
		return
	}
	s := &a.island
	s.Displays = mygo.Screen.Displays()
	if !a.state.Settings.AgentIsland {
		a.closeIslandWindows()
		return
	}
	displays := islandDisplays(s.Displays, a.state.Settings.Island.DisplayTarget)
	if s.Windows == nil {
		s.Windows = map[int64]*mygo.Window{}
	}
	wanted := map[int64]bool{}
	for _, display := range displays {
		wanted[display.ID] = true
	}
	for id, window := range s.Windows {
		if !wanted[id] {
			delete(s.Windows, id)
			a.observeIslandPointer(id, false)
			window.Close()
		}
	}
	for _, display := range displays {
		if s.Windows[display.ID] != nil {
			continue
		}
		area := display.Bounds
		window := mygo.NewWindow(mygo.WindowOptions{
			Title: "GCode: " + a.t("Dynamic Island"), Width: 210, Height: 28,
			X: area.X + (area.Width-210)/2, Y: area.Y,
			Frameless: true, Panel: true, AlwaysOnTop: true, SkipTaskbar: true, DisableResize: true,
			DisableMinimize: true, DisableMaximize: true, UseContentSize: true,
			Transparent: true, BackgroundColor: "#00000000", DisableShadow: true,
			Hidden: true, Content: ui.View(func(c *ui.Context) { a.renderAgentIsland(c, display.ID, true) }),
		})
		s.Windows[display.ID] = window
		window.OnClose(func(event *mygo.CloseEvent) {
			if !a.closed && a.state.Settings.AgentIsland && s.Windows[display.ID] == window {
				event.PreventDefault()
			}
		})
		configureIslandWindow(window)
		window.ShowInactive()
	}
	s.Window = nil
	if len(displays) > 0 {
		s.Window = s.Windows[displays[0].ID]
	}

	now := time.Now()
	entries := islandEntries(a.state, now, s.Dismissed)
	observed := make(map[string]string, len(entries))
	for _, entry := range entries {
		observed[entry.Token] = entry.Phase
		if s.Observed == nil || s.Observed[entry.Token] == entry.Phase {
			continue
		}
		if !a.state.Settings.Island.SubtleReminders && (entry.Phase == "needs-interaction" || entry.Phase == "completed" || entry.Phase == "error") {
			s.Expanded = true
			s.CollapseAt = time.Time{}
			if entry.Phase == "completed" {
				s.CollapseAt = now.Add(5 * time.Second)
			}
			if entry.Phase == "error" {
				s.CollapseAt = now.Add(12 * time.Second)
			}
		}
	}
	s.Observed = observed
	if !s.CollapseAt.IsZero() && !now.Before(s.CollapseAt) {
		s.Expanded = false
		s.CollapseAt = time.Time{}
	}
	a.resizeAgentIsland(len(entries))
	for _, window := range s.Windows {
		window.Invalidate()
	}
}

func (a *App) setIslandEnabled(enabled bool) {
	a.state.Settings.AgentIsland = enabled
	a.saveSettings()
	a.syncAgentIsland()
}

func (a *App) toggleIslandExpanded() {
	a.playIslandClick()
	a.island.Expanded = !a.island.Expanded
	a.island.CollapseAt = time.Time{}
	a.resizeAgentIsland(len(islandEntries(a.state, time.Now(), a.island.Dismissed)))
}

func (a *App) openIslandSession(sessionID string) {
	a.playIslandClick()
	if a.win == nil {
		a.selectSession(sessionID)
		a.navigate("chat/" + sessionID)
		return
	}
	a.win.Update(func() {
		if a.closed {
			return
		}
		a.selectSession(sessionID)
		a.navigate("chat/" + sessionID)
		a.win.Show()
		a.win.Focus()
	})
}

func islandDisplays(displays []mygo.Display, target string) []mygo.Display {
	if target == "all" {
		return displays
	}
	if target != "" && target != "primary" {
		for _, display := range displays {
			if strconv.FormatInt(display.ID, 10) == target {
				return []mygo.Display{display}
			}
		}
	}
	for _, display := range displays {
		if display.Primary {
			return []mygo.Display{display}
		}
	}
	if len(displays) > 0 {
		return displays[:1]
	}
	return nil
}

func (a *App) closeIslandWindows() {
	s := &a.island
	windows := s.Windows
	s.Windows = nil
	s.Hovered = nil
	s.HoverExpanded = false
	s.HoverCooldown = time.Time{}
	s.cancelHoverTimer()
	s.Window = nil
	s.Width = 0
	s.Height = 0
	for _, window := range windows {
		window.Close()
	}
}

func (a *App) resizeAgentIsland(count int) {
	s := &a.island
	width, height := design.IslandGeometry(s.Expanded || s.HoverExpanded, count)
	s.Width, s.Height = width, height
	for _, display := range s.Displays {
		window := s.Windows[display.ID]
		if window == nil {
			continue
		}
		area := display.Bounds
		bounds := mygo.Rectangle{X: area.X + (area.Width-width)/2, Y: area.Y, Width: width, Height: height}
		if window.Bounds() != bounds {
			window.SetBounds(bounds)
		}
	}
}

func (a *App) islandMascot(c *ui.Context, size float32) {
	name := a.state.Settings.Island.Mascot
	if name == "" {
		name = "pululu"
	}
	if image := assets.IslandMascot(name); image != nil {
		ui.Image(c, image).Size(size, size).Fit(ui.Contain)
		return
	}
	icon(c, "widget", size)
}

func islandTheme() *ui.Theme {
	theme := *ui.DarkTheme()
	theme.Background = ui.Transparent
	theme.Surface = ui.Hex("#101010")
	theme.SurfaceHover = ui.Hex("#202020")
	theme.SurfacePressed = ui.Hex("#282828")
	theme.Border = ui.Hex("#242424")
	theme.Text = ui.Hex("#e8e8e8")
	theme.TextMuted = ui.Hex("#858585")
	return &theme
}

func (a *App) islandNewConversation() {
	a.playIslandClick()
	if a.win == nil {
		a.newConversation()
		return
	}
	a.win.Update(func() {
		if a.closed {
			return
		}
		a.newConversation()
		a.win.Show()
		a.win.Focus()
	})
}

func (s *agentIslandState) cancelHoverTimer() {
	s.HoverGeneration++
	if s.HoverTimer != nil {
		s.HoverTimer.Stop()
		s.HoverTimer = nil
	}
}

func (a *App) observeIslandPointer(displayID int64, inside bool) {
	s := &a.island
	if s.Hovered[displayID] == inside {
		return
	}
	wasInside := len(s.Hovered) > 0
	if inside {
		if s.Hovered == nil {
			s.Hovered = make(map[int64]bool)
		}
		s.Hovered[displayID] = true
	} else {
		delete(s.Hovered, displayID)
	}
	anyInside := len(s.Hovered) > 0
	if wasInside == anyInside {
		return
	}
	s.cancelHoverTimer()
	if anyInside == s.HoverExpanded {
		return
	}
	delay := 150 * time.Millisecond
	if anyInside {
		delay = 500*time.Millisecond + max(0, time.Until(s.HoverCooldown))
	}
	generation := s.HoverGeneration
	win := a.win
	s.HoverTimer = time.AfterFunc(delay, func() {
		win.Update(func() {
			if a.closed || s.HoverGeneration != generation {
				return
			}
			s.HoverTimer = nil
			s.HoverExpanded = anyInside
			if !anyInside {
				s.HoverCooldown = time.Now().Add(150 * time.Millisecond)
			}
			a.syncAgentIsland()
		})
	})
}

func (a *App) agentIslandView(c *ui.Context) {
	a.renderAgentIsland(c, 0, false)
}

func (a *App) renderAgentIsland(c *ui.Context, displayID int64, trackPointer bool) {
	localize.Set(c, a.state.Settings.General.Language)
	c.SetTheme(islandTheme())
	c.Root().Background(ui.Transparent)
	entries := islandEntries(a.state, time.Now(), a.island.Dismissed)
	detail, count := tr(c, "No active tasks"), ""
	if len(entries) > 0 {
		detail, count = tr(c, entries[0].Detail), fmt.Sprint(len(entries))
	}
	// The compact row draws the detail and the count as separate texts; the
	// accessible name joins them because a label is one string.
	label := detail
	if count != "" {
		label = detail + ", " + count
	}
	expanded := a.island.Expanded || a.island.HoverExpanded
	radius := design.IslandRadius(expanded)
	panel := ui.Column(c.Key("agent-island")).Fill().Gap(0).Background(ui.Hex("#000000")).Radius(0, 0, radius, radius).Clip()
	if trackPointer {
		_, _, inside := panel.PointerPosition()
		a.observeIslandPointer(displayID, inside)
	}
	panel.Children(func() {
		if !expanded {
			controlIslandCompact(c.Key("island-expand"), label).OnClick(a.toggleIslandExpanded).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					a.islandMascot(c, 16)
					if len(entries) > 0 {
						color := c.Theme().Success
						switch entries[0].Phase {
						case "needs-interaction":
							color = c.Theme().Warning
						case "error":
							color = c.Theme().Danger
						case "interrupted":
							color = c.Theme().TextMuted
						}
						ui.Box(c).Size(5, 5).Radius(3).Background(color)
					}
					ui.Text(c, detail).FontSize(design.TextSize(c, design.TextSmall)).SingleLine()
					if count != "" {
						ui.Text(c, count).FontSize(design.TextSize(c, design.TextSmall)).SingleLine()
					}
				})
			})
			return
		}
		ui.Row(c).FillWidth().Padding(6, 14).Gap(6).AlignItems(ui.Center).Children(func() {
			a.islandMascot(c, 18)
			textButton(c.Key("island-expand"), label).Grow(1).OnClick(a.toggleIslandExpanded)
			iconButton(c.Key("island-new"), "plus", tr(c, "New conversation")).OnClick(a.islandNewConversation)
		})
		ui.Scroll(c.Key("island-sessions")).Grow(1).FillWidth().Padding(0, 16, 16, 16).Gap(8).Children(func() {
			if len(entries) == 0 {
				controlIslandCard(c.Key("island-new-task"), tr(c, "New conversation")).FillWidth().OnClick(a.islandNewConversation).Children(func() {
					ui.Row(c).FillWidth().Gap(14).AlignItems(ui.Center).Children(func() {
						icon(c, "plus", 22)
						ui.Column(c).Grow(1).Gap(4).Children(func() {
							ui.Text(c, tr(c, "New conversation")).Bold()
							ui.Text(c, tr(c, "Start a new task")).TextColor(c.Theme().TextMuted)
						})
					})
				})
			}
			for _, value := range entries {
				entry := value
				ui.Column(c.Key("island-session-" + entry.SessionID)).FillWidth().Gap(4).Children(func() {
					controlIslandCard(c.Key("island-open-"+entry.SessionID), entry.Title).FillWidth().OnClick(func() { a.openIslandSession(entry.SessionID) }).Children(func() {
						ui.Column(c).FillWidth().Gap(6).Children(func() {
							ui.Text(c, entry.Title).Bold().SingleLine()
							ui.Row(c).FillWidth().Gap(8).Children(func() {
								ui.Text(c, tr(c, entry.Detail)).TextColor(c.Theme().TextMuted)
								ui.Spacer(c).Grow(1)
								if entry.Phase == "running" || entry.Phase == "needs-interaction" {
									ui.Text(c, i18n.Duration(a.state.Settings.General.Language, max(0, time.Since(entry.StartedAt).Milliseconds()))).FontFeatures("tnum").TextColor(c.Theme().TextMuted)
								}
							})
						})
					})
					ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
						if entry.Phase == "running" || entry.Phase == "needs-interaction" {
							textButton(c.Key("island-stop-"+entry.SessionID), tr(c, "Stop")).OnClick(func() { a.dispatch(contracts.Command{Kind: "message.stop", SessionID: entry.SessionID}) })
						} else {
							textButton(c.Key("island-dismiss-"+entry.SessionID), tr(c, "Dismiss")).OnClick(func() {
								if a.island.Dismissed == nil {
									a.island.Dismissed = map[string]bool{}
								}
								a.island.Dismissed[entry.Token] = true
								a.markSessionRead(entry.SessionID)
							})
						}
					})
				})
			}
		})
	})
}

func (a *App) markSessionRead(sessionID string) {
	for _, session := range a.state.Sessions {
		if session.ID == sessionID && session.Unread {
			unread := false
			a.dispatch(contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: sessionID, Unread: &unread}})
			return
		}
	}
}

func islandHeight(expanded bool, count int) int {
	_, height := design.IslandGeometry(expanded, count)
	return height
}
