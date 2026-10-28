package desktop

import (
	"fmt"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

type integrationUIState struct {
	Error                            string
	ProviderTarget, ExpandedProvider string
	RenameOpen                       bool
	RenameID, RenameText             string
	CopiedDevice                     string
	CopiedUntil                      time.Time
	LoginID, LoginCode               string
	APIProvider, APIKey              string
	APISaving                        bool
	Packs                            int
	GithubCopied                     bool
}

func (a *App) integrationTarget() string {
	if a.integrationUI.ProviderTarget != "" {
		return a.integrationUI.ProviderTarget
	}
	for _, d := range a.state.Devices {
		if d.Current {
			return d.ID
		}
	}
	return "local"
}

func (a *App) integrationCommand(kind, provider string) contracts.Command {
	return contracts.Command{Kind: kind, Integration: &contracts.IntegrationRequest{TargetID: a.integrationTarget(), ProviderID: provider}}
}

func (a *App) integrationSubtitle(c *ui.Context, text string) {
	ui.Text(c, text).FontSize(design.UISize(c, 13)).FixedLineHeight(19).TextColor(a.palette.Muted).Margin(-12, 8, 8, 8)
}
func (a *App) integrationLabel(c *ui.Context, label string) {
	ui.Text(c, label).FontSize(design.UISize(c, 11)).FontWeight(500).TextColor(a.palette.Muted).Margin(0, 0, 4, 0)
}

func (a *App) integrationToggle(c *ui.Context, key, label string, value, disabled bool, change func(bool)) {
	current := value
	b := ui.SwitchBase(c.Key(key), &current).Size(44.8, 28.8).Label(label).Disabled(disabled).OnClick(func() { change(current) })
	a.styleIntegrationSwitch(c, b, value)
}

func (a *App) integrationError(c *ui.Context, message, kind string, request *contracts.IntegrationRequest) {
	ui.Row(c).FillWidth().Padding(12, 16).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Text(c, message).Grow(1).FontSize(design.UISize(c, 12)).TextColor(a.palette.Danger)
		if kind != "" {
			textButton(c, "Retry").OnClick(func() { a.dispatch(contracts.Command{Kind: kind, Integration: request}) })
		}
	})
}

func (a *App) integrationLoading(c *ui.Context, label string) {
	ui.Column(c).FillWidth().Padding(16).Gap(8).Label(label).Children(func() {
		for _, width := range []float32{44, 66, 52} {
			ui.Box(c).Height(13).WidthPercent(width).Radius(6).Background(a.palette.Text.Alpha(.07))
		}
	})
}

func (a *App) integrationDialogs(c *ui.Context) {
	if !a.integrationHasSurface() {
		return
	}
	s := &a.integrationUI
	ui.DialogBase(c, &s.RenameOpen, func(back, panel ui.Element) {
		back.Background(ui.RGBA(0, 0, 0, .35))
		panel.Width(360).Padding(20).Radius(16).Background(a.palette.Dialog).Border(1, a.palette.Border)
		ui.Column(c).Gap(12).Children(func() {
			ui.Text(c, "Rename device").FontSize(design.UISize(c, 15)).FontWeight(600)
			ui.TextInput(c, &s.RenameText).Label("Device name").Placeholder("Device name").AutoFocus().OnSubmit(func() { a.renameDevice() })
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				textButton(c, "Cancel").OnClick(func() { s.RenameOpen = false })
				textButton(c, "Rename").Disabled(strings.TrimSpace(s.RenameText) == "").OnClick(a.renameDevice)
			})
		})
	})
	login := a.state.Integrations.Login
	if login == nil || login.State == "completed" {
		return
	}
	if s.LoginID != login.ID {
		s.LoginID = login.ID
		s.LoginCode = ""
	}
	name := login.ProviderID
	for _, p := range a.state.Providers {
		if p.ID == login.ProviderID {
			name = p.Name
		}
	}
	title := "Sign in to " + name
	if login.LoginProvider != "" {
		for _, d := range a.state.Integrations.Providers {
			if d.ProviderID == login.ProviderID && d.TargetID == login.TargetID {
				for _, option := range d.LoginOptions {
					if option.ID == login.LoginProvider {
						title = "Sign in to " + option.Label + " for " + name
					}
				}
			}
		}
	}
	open := true
	ui.DialogBase(c, &open, func(back, panel ui.Element) {
		back.Background(ui.RGBA(0, 0, 0, .35))
		panel.Width(360).Padding(20).Radius(16).Background(a.palette.Dialog).Border(1, a.palette.Border).Label(title)
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			ui.Text(c, title).FontSize(design.UISize(c, 15)).FontWeight(600)
			copy := "Finish signing in in your browser."
			if login.ProviderID == "claude" {
				copy = "Finish signing in to Claude in your browser. The new login is saved next to your current one — nothing changes until you switch."
			}
			if login.ProviderID == "openai" {
				copy = "Finish signing in to ChatGPT in your browser. The new login is saved next to your current one — nothing changes until you switch."
			}
			if login.Kind == "code" {
				copy = "Your browser opened Claude's sign-in page. Approve access, then paste the code Anthropic shows you below. Your current login is untouched until you switch."
			}
			if login.TargetID == "cloud-1" {
				provider := "Claude"
				if login.ProviderID == "openai" {
					provider = "ChatGPT"
				}
				copy = "Finish signing in to " + provider + " in your browser. The login is stored for Cloud and becomes the account every Cloud machine uses; this computer's login is untouched."
			}
			ui.Text(c, copy).FontSize(design.UISize(c, 13)).FixedLineHeight(19).TextColor(a.palette.Muted)
			req := &contracts.IntegrationRequest{TargetID: login.TargetID, ProviderID: login.ProviderID}
			if login.State == "failed" {
				ui.Text(c, login.Error).FontSize(design.UISize(c, 12)).TextColor(a.palette.Danger)
			} else {
				textButton(c, "Reopen the sign-in page").FontSize(design.UISize(c, 12)).OnClick(func() { a.dispatch(contracts.Command{Kind: "provider.login.poll", ID: login.ID, Integration: req}) })
				if login.Kind == "code" {
					ui.TextInput(c, &s.LoginCode).Label("Authorization code").Placeholder("Paste the authorization code").Font(a.state.Settings.Appearance.CodeFont).AutoFocus().OnSubmit(func() {
						a.dispatch(contracts.Command{Kind: "provider.login.submit", ID: login.ID, Text: s.LoginCode, Integration: req})
					})
				} else {
					ui.Text(c, "Waiting for you to finish in the browser…").Margin(8, 0, 0, 0).FontSize(design.UISize(c, 12.5)).TextColor(a.palette.Muted)
					if login.UserCode != "" {
						ui.Text(c, login.UserCode).Selectable().Font(a.state.Settings.Appearance.CodeFont).FontSize(18)
					}
				}
			}
			ui.Row(c).FillWidth().Gap(8).Margin(8, 0, 0, 0).Justify(ui.End).Children(func() {
				label := "Cancel"
				if login.State == "failed" {
					label = "Close"
				}
				textButton(c, label).OnClick(func() { a.dispatch(contracts.Command{Kind: "provider.login.cancel", ID: login.ID, Integration: req}) })
				if login.State == "failed" {
					textButton(c, "Retry").OnClick(func() { a.dispatch(contracts.Command{Kind: "provider.login.retry", Integration: req}) })
				} else if login.Kind == "code" {
					textButton(c, "Add account").Disabled(strings.TrimSpace(s.LoginCode) == "").OnClick(func() {
						a.dispatch(contracts.Command{Kind: "provider.login.submit", ID: login.ID, Text: s.LoginCode, Integration: req})
					})
				}
			})
		})
	})
	if !open {
		a.dispatch(contracts.Command{Kind: "provider.login.cancel", ID: login.ID, Integration: &contracts.IntegrationRequest{TargetID: login.TargetID, ProviderID: login.ProviderID}})
	}
}

func (a *App) renameDevice() {
	s := &a.integrationUI
	if strings.TrimSpace(s.RenameText) == "" {
		return
	}
	a.dispatch(contracts.Command{Kind: "device.rename", ID: s.RenameID, Name: s.RenameText})
}

func (a *App) integrationCommandFinished(cmd contracts.Command, err error) bool {
	if !contracts.IsIntegrationCommand(cmd.Kind) {
		return false
	}
	state := &a.integrationUI
	if err != nil {
		state.Error = err.Error()
		state.APISaving = false
		return true
	}
	state.Error = ""
	if cmd.Kind == "device.rename" {
		state.RenameOpen = false
	}
	if cmd.Kind == "provider.apiKey.save" {
		state.APISaving = false
		state.APIProvider = ""
		state.APIKey = ""
	}
	return true
}

func (a *App) integrationHasSurface() bool {
	return a.integrationUI.RenameOpen || a.state.Integrations.Login != nil && a.state.Integrations.Login.State != "completed"
}

func integrationCredits(value float64) string {
	raw := fmt.Sprintf("%.0f", value)
	for i := len(raw) - 3; i > 0; i -= 3 {
		raw = raw[:i] + "," + raw[i:]
	}
	return raw
}
func integrationPrice(cents int) string {
	if cents%100 == 0 {
		return fmt.Sprintf("$%d", cents/100)
	}
	return fmt.Sprintf("$%.2f", float64(cents)/100)
}
func integrationHours(credits float64) string {
	return fmt.Sprintf("about %s hours", integrationCredits(credits/60))
}
