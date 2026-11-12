package desktop

import (
	"fmt"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
	"time"
)

type sessionInfoProjection struct {
	revision             uint64
	session              string
	ready                bool
	agents               []contracts.Session
	additions, deletions int
}

const sessionCardWidth float32 = 272

func sessionCardReserve(main, transcript float32) (float32, bool) {
	left := main - sessionCardWidth - 12
	reserve := max(main+transcript-2*(left-24), 0)
	return reserve, main-reserve-96 >= transcript-.5
}
func (a *App) infoVisibility() (float32, bool) {
	if a.sessionID == "" || a.route != "chat" || a.rightExpanded {
		return 0, false
	}
	width := float32(a.state.Settings.Appearance.TranscriptWidth)
	if width <= 0 {
		width = 736
	}
	reserve, docked := sessionCardReserve(a.chatWidth, width)
	visible := docked
	if a.expanded["session-info-user"] {
		visible = a.expanded["session-info"]
	}
	if !visible {
		return 0, false
	}
	if !docked {
		reserve = 0
	}
	return reserve, true
}
func (a *App) toggleInfo() {
	_, visible := a.infoVisibility()
	a.expanded["session-info-user"] = true
	a.expanded["session-info"] = !visible
}
func (a *App) sessionInfo(c *ui.Context) {
	_, visible := a.infoVisibility()
	if !visible {
		return
	}
	left := a.chatWidth - sessionCardWidth - 12
	if a.fit.Sidebar {
		left += a.visibleSidebarWidth
	}
	s := a.currentSession()
	ui.Overlay(c, func() {
		ui.Column(c).Absolute().Left(max(12, left)).Top(50).Width(sessionCardWidth).Gap(8).Children(func() {
			ui.Column(c).FillWidth().Radius(10).Background(a.palette.Overlay.Alpha(.85)).Border(1, a.palette.Border).Children(func() {
				a.infoRow(c, "monitor", a.deviceNameFor(s.DeviceID))
				a.infoRow(c, "project-default", a.projectNameFor(s.ProjectID))
				a.sessionAgents(s.ID)
				adds, dels := a.sessionInfoProjection.additions, a.sessionInfoProjection.deletions
				ui.ButtonBase(c).FillWidth().Padding(6, 9).Gap(8).Justify(ui.Start).Label(tr(c, "Branch changes")).OnClick(func() { a.setPanel(1, true); a.workspace.OpenSurface("changes") }).Children(func() {
					icon(c, "git-branch", 16)
					ui.Text(c, s.Branch).FontSize(design.TextSize(c, design.TextBase)).SingleLine().Grow(1)
					ui.Text(c, fmt.Sprintf(tr(c, "+%d"), adds)).FontSize(design.TextSize(c, design.TextXS)).TextColor(a.palette.Success)
					ui.Text(c, fmt.Sprintf(tr(c, "−%d"), dels)).FontSize(design.TextSize(c, design.TextXS)).TextColor(a.palette.Danger)
				})
			})
			agents := a.sessionAgents(s.ID)
			if len(agents) == 0 {
				return
			}
			ui.Column(c).FillWidth().Radius(10).Background(a.palette.Overlay.Alpha(.85)).Border(1, a.palette.Border).Padding(10, 0).Children(func() {
				ui.Text(c, tr(c, "Agents")).FontSize(design.TextSize(c, design.TextXS)).LetterSpacing(.8).TextColor(a.palette.Muted).Padding(0, 12, 4, 12)
				ended := 0
				for _, child := range agents {
					if child.Status == contracts.Working {
						a.sessionAgentRow(c, child)
					} else {
						ended++
					}
				}
				if ended > 0 {
					key := "session-agents-ended:" + s.ID
					open := a.expanded[key]
					a.controlSessionAgentGroup(c.Key(key), ended, open).OnClick(func() { a.expanded[key] = !open })
					if open {
						for _, child := range agents {
							if child.Status != contracts.Working {
								a.sessionAgentRow(c, child)
							}
						}
					}
				}
			})
		})
	})
}
func (a *App) infoRow(c *ui.Context, glyph, label string) {
	ui.Row(c).Padding(6, 9).Gap(8).AlignItems(ui.Center).Children(func() {
		icon(c, glyph, 16)
		ui.Text(c, tr(c, label)).FontSize(design.TextSize(c, design.TextBase)).SingleLine()
	})
}

func (a *App) sessionAgentRow(c *ui.Context, child contracts.Session) {
	status := sessionAgentStatus(child)
	duration, known := a.sessionAgentDuration(child.ID)
	a.controlSessionAgent(c.Key("session-agent:"+child.ID), child.Title, status, metricDuration(duration, known)).
		OnClick(func() { a.workspace.OpenSession(child.ID) })
	if child.Status == contracts.Working || child.Status == contracts.AwaitingInput {
		c.After(time.Second)
	}
}

func sessionAgentStatus(session contracts.Session) string {
	switch session.Status {
	case contracts.Working:
		return "Running"
	case contracts.AwaitingInput:
		return "Awaiting input"
	case contracts.Errored:
		return "Failed"
	}
	switch session.LastOutcome {
	case "completed":
		return "Completed"
	case "failed":
		return "Failed"
	case "interrupted", "cancelled":
		return "Stopped"
	}
	if session.Archived {
		return "Ended"
	}
	return "Pending"
}

// Use this agent's run intervals, excluding idle gaps and delegated agents.
// A missing run is unknown, rather than elapsed time since session creation.
func (a *App) sessionAgentDuration(sessionID string) (time.Duration, bool) {
	metrics := a.state.Metrics[sessionID].Own
	now := time.Time{}
	if !metrics.RunningSince.IsZero() {
		now = a.metricsNow()
	}
	return metrics.Elapsed(now)
}

// sessionAgents aggregates the delegation tree, including finished and archived
// agents. Ordinary side chats and copied conversations are separate work.
func (a *App) sessionAgents(parentID string) []contracts.Session {
	cached := &a.sessionInfoProjection
	if cached.ready && cached.revision == a.state.Revision && cached.session == parentID {
		return cached.agents
	}

	scope := map[string]bool{parentID: true}
	for changed := true; changed; {
		changed = false
		for _, session := range a.state.Sessions {
			if session.SubagentType != "" && scope[session.ParentSessionID] && !scope[session.ID] {
				scope[session.ID] = true
				changed = true
			}
		}
	}
	var agents []contracts.Session
	for _, session := range a.state.Sessions {
		if session.ID != parentID && scope[session.ID] {
			agents = append(agents, session)
		}
	}
	*cached = sessionInfoProjection{revision: a.state.Revision, session: parentID, ready: true, agents: agents}
	for _, session := range a.state.Sessions {
		if session.ID != parentID {
			continue
		}
		for _, change := range a.state.Changes {
			if change.ProjectID == session.ProjectID {
				cached.additions += change.Additions
				cached.deletions += change.Deletions
			}
		}
		break
	}
	return agents
}
