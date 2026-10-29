package desktop

import (
	"fmt"
	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

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
				var adds, dels int
				for _, change := range a.state.Changes {
					if change.ProjectID == s.ProjectID {
						adds += change.Additions
						dels += change.Deletions
					}
				}
				ui.ButtonBase(c).FillWidth().Padding(6, 9).Gap(8).Justify(ui.Start).Label(tr(c, "Branch changes")).OnClick(func() { a.setPanel(1, true); a.workspace.OpenSurface("changes") }).Children(func() {
					icon(c, "git-branch", 16)
					ui.Text(c, s.Branch).FontSize(design.UISize(c, 12)).SingleLine().Grow(1)
					ui.Text(c, fmt.Sprintf(tr(c, "+%d"), adds)).FontSize(design.UISize(c, 10)).TextColor(a.palette.Success)
					ui.Text(c, fmt.Sprintf(tr(c, "−%d"), dels)).FontSize(design.UISize(c, 10)).TextColor(a.palette.Danger)
				})
				line(c)
				ui.Text(c, tr(c, "ACTIONS")).FontSize(design.UISize(c, 10)).LetterSpacing(.8).TextColor(a.palette.Muted).Padding(12, 12, 4, 12)
				for _, project := range a.state.Projects {
					if project.ID == s.ProjectID {
						for _, action := range project.Actions {
							action := action
							a.infoAction(c, "action-play", action.Name, func() {
								a.dispatch(contracts.Command{Kind: "project.action.run", ProjectID: s.ProjectID, ID: action.ID})
								if !a.terminal {
									a.toggleTerminal()
								}
							})
						}
					}
				}
				a.infoAction(c, "plus", "Add action", func() { a.actionOpen = true; a.actionName = ""; a.actionCommand = "" })
			})
			ui.Column(c).FillWidth().Radius(10).Background(a.palette.Overlay.Alpha(.85)).Border(1, a.palette.Border).Padding(10, 0).Children(func() {
				ui.Text(c, tr(c, "SIDE CHATS")).FontSize(design.UISize(c, 10)).LetterSpacing(.8).TextColor(a.palette.Muted).Padding(0, 12, 4, 12)
				for _, child := range a.state.Sessions {
					if child.ParentSessionID == s.ID && !child.Archived {
						child := child
						menuChoice(c, child.Title, false, func() { a.openRelatedSession(child) })
					}
				}
				a.infoAction(c, "plus", "New side chat", func() { a.createRelatedSession(s.ID, true) }).Disabled(a.branchCreation != nil)
				a.infoAction(c, "fork", "Fork chat", func() { a.createRelatedSession(s.ID, false) }).Disabled(a.branchCreation != nil)
			})
		})
	})
	ui.DialogBase(c, &a.actionOpen, func(back, panel ui.Element) {
		back.Background(ui.RGBA(0, 0, 0, .35))
		panel.Width(460).Padding(24).Radius(12).Background(a.palette.Overlay)
		ui.Column(c).Gap(16).Children(func() {
			ui.Text(c, tr(c, "Add action")).FontSize(design.UISize(c, 18)).FontWeight(600)
			ui.TextInput(c, &a.actionName).Label(tr(c, "Action name")).Placeholder(tr(c, "Name")).AutoFocus()
			ui.TextInput(c, &a.actionCommand).Label(tr(c, "Action command")).Placeholder(tr(c, "Command"))
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				textButton(c, tr(c, "Cancel")).OnClick(func() { a.actionOpen = false })
				textButton(c, tr(c, "Add action")).Disabled(a.actionName == "" || a.actionCommand == "").OnClick(func() {
					action := contracts.ProjectAction{Name: a.actionName, Command: a.actionCommand, Icon: "action-play", Enabled: true}
					a.dispatch(contracts.Command{Kind: "project.action.create", ProjectID: s.ProjectID, Action: &action})
					a.actionOpen = false
				})
			})
		})
	})
}
func (a *App) infoRow(c *ui.Context, glyph, label string) {
	ui.Row(c).Padding(6, 9).Gap(8).AlignItems(ui.Center).Children(func() { icon(c, glyph, 16); ui.Text(c, tr(c, label)).FontSize(design.UISize(c, 13)).SingleLine() })
}
func (a *App) infoAction(c *ui.Context, glyph, label string, action func()) ui.Element {
	return ui.ButtonBase(c).FillWidth().Padding(7, 12).Gap(8).Justify(ui.Start).Label(tr(c, label)).OnClick(action).Children(func() { icon(c, glyph, 14); ui.Text(c, tr(c, label)).FontSize(design.UISize(c, 12)) })
}
