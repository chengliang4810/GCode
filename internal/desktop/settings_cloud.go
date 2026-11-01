package desktop

import (
	"fmt"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func (a *App) cloudSettings(c *ui.Context) {
	a.integrationSubtitle(c, "Run your agents in cloud sandboxes.")
	if a.cloudPage.error != "" {
		ui.Text(c, a.cloudPage.error).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Danger)
	}
	v := a.state.Integrations.Cloud
	if a.cloudPage.busy == "cloud.enable" {
		v.State = "provisioning"
	}
	if v.LoadState == contracts.LoadLoading {
		a.integrationLoading(c, "Checking Cloud")
		return
	}
	if v.LoadState == contracts.LoadError {
		a.settingsCard(c, func() { a.cloudSettingsError(c, tr(c, "Couldn't check Cloud: ")+v.Error, "cloud.refresh") })
		return
	}
	if !v.Available || !a.state.User.SignedIn {
		a.settingsCard(c, func() {
			a.settingsRow(c, "Cloud", "Cloud needs a signed-in account with sync turned on.", func() {})
		})
		return
	}
	switch v.State {
	case "error":
		a.settingsCard(c, func() { a.integrationError(c, v.Error, "cloud.retry", nil) })
	case "provisioning", "deleting":
		label := "Turning on Cloud…"
		if v.State == "deleting" {
			label = "Turning off Cloud…"
		}
		a.settingsCard(c, func() { a.settingsRow(c, "Cloud", label, func() {}) })
	}
	a.cloudUsage(c, v)
	sectionTitle(c, tr(c, "GitHub"))
	a.settingsCard(c, func() {
		g := v.Github
		message, label := "Not connected", "Connect"
		disabled := v.State != "ready"
		if disabled {
			message = "Turn on Cloud to connect GitHub."
		}
		switch g.State {
		case "waiting":
			message, label = "Waiting for you to approve on GitHub…", "Cancel"
		case "loading":
			message, label = "Checking GitHub…", "Connect"
		case "failed":
			message, label = g.Error, "Try again"
		case "needsReconnect":
			message, label = "Needs reconnect", "Reconnect"
		case "connected":
			message, label = g.Account, "Disconnect"
		case "notAuthorized":
			message, label = g.Account, "Connect"
		}
		a.settingsRow(c, "GitHub", message, func() {
			textButton(c, tr(c, label)).Disabled(disabled).OnClick(func() {
				kind := "cloud.github.start"
				switch g.State {
				case "waiting":
					kind = "cloud.github.cancel"
				case "connected":
					kind = "cloud.github.disconnect"
				}
				if g.State == "notAuthorized" {
					a.dispatch(contracts.Command{Kind: "cloud.authorize", Enabled: true, Integration: &contracts.IntegrationRequest{ProviderID: "github"}})
				} else {
					a.dispatch(contracts.Command{Kind: kind})
				}
			})
		}, func() {
			ui.Box(c).Size(36, 36).Radius(10).Background(a.palette.Text.Alpha(.06)).Center().Children(func() { icon(c, "github-mark", 16) })
		})
		if g.State == "waiting" {
			line(c)
			ui.Column(c).FillWidth().Padding(12, 16).Gap(8).Children(func() {
				ui.Text(c, tr(c, "Enter this code on GitHub")).FontSize(design.TextSize(c, design.TextBase)).FontWeight(500)
				ui.Text(c, tr(c, "Approve GCode there; this page moves on by itself.")).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
				services := c.Services()
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					controlCodeButton(c, g.UserCode, a.state.Settings.Appearance.CodeFont).Label(tr(c, "Copy GitHub code ") + g.UserCode).OnClick(func() { services.WriteClipboard(g.UserCode); a.integrationUI.GithubCopied = true })
					if a.integrationUI.GithubCopied {
						ui.Text(c, tr(c, "Copied")).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
					}
					ui.Spacer(c)
					textButton(c, tr(c, "Open GitHub")).OnClick(func() { a.dispatch(contracts.Command{Kind: "cloud.github.poll"}) })
				})
			})
		}
		if g.State == "connected" {
			a.cloudRepositories(c, v)
		}
	})
}

func (a *App) cloudRepositories(c *ui.Context, v contracts.CloudSettingsState) {
	line(c)
	message := "None yet: choose the repositories Cloud sessions can open."
	if len(v.Repositories) == 1 {
		message = "Cloud sessions can open 1 repository."
	} else if len(v.Repositories) > 1 {
		message = fmt.Sprintf(tr(c, "Cloud sessions can open %d repositories."), len(v.Repositories))
	}
	if v.RepositoriesState == contracts.LoadLoading {
		message = "Checking which repositories Cloud can open…"
	}
	a.settingsRow(c, "Repositories", message, func() {
		iconButton(c, "refresh", tr(c, "Check repositories again")).OnClick(func() { a.dispatch(contracts.Command{Kind: "cloud.repositories.refresh"}) })
		textButton(c, tr(c, "Choose on GitHub")).OnClick(func() { a.dispatch(contracts.Command{Kind: "cloud.repositories.choose"}) })
	})
	for _, repo := range v.Repositories {
		ui.Row(c).FillWidth().Padding(8, 16).Gap(6).AlignItems(ui.Center).Children(func() {
			icon(c, "folder", 14)
			ui.Text(c, repo.Name).FontSize(design.TextSize(c, design.TextBase)).Grow(1)
			if repo.Private {
				ui.Text(c, tr(c, "Private")).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted).Padding(2, 5).Radius(4).Background(a.palette.Text.Alpha(.06))
			}
		})
	}
}

func (a *App) cloudUsage(c *ui.Context, v contracts.CloudSettingsState) {
	if len(v.Usage) == 0 {
		return
	}
	sectionTitle(c, tr(c, "Usage"))
	total, peak := float64(0), float64(1)
	for _, day := range v.Usage {
		total += day.Credits
		peak = max(peak, day.Credits)
	}
	a.settingsCard(c, func() {
		a.settingsRow(c, integrationCredits(total)+tr(c, " credits used"), integrationCredits(a.state.Integrations.Billing.Credits.Total)+tr(c, " credits left"), func() {
			label := "Last 7 days"
			options := []settingOption{{"7d", "Last 7 days"}, {"30d", "Last 30 days"}, {"thisMonth", "This month"}, {"lastMonth", "Last month"}, {"90d", "Last 90 days"}}
			for _, o := range options {
				if o.value == v.UsageRange {
					label = o.label
				}
			}
			controlMenuButton(c, tr(c, label), func(menu *ui.Menu) {
				for _, o := range options {
					if menu.Item(tr(c, o.label)).Checked(v.UsageRange == o.value).Chosen() {
						a.dispatch(contracts.Command{Kind: "cloud.usage", Integration: &contracts.IntegrationRequest{Value: o.value}})
					}
				}
			}).Label(tr(c, "Usage range")).Width(128)
		})
		ui.Column(c).FillWidth().Padding(0, 16, 12, 16).Gap(6).Children(func() {
			gap := float32(3)
			if len(v.Usage) > 45 {
				gap = 1
			}
			ui.Row(c).FillWidth().Height(88).Gap(gap).AlignItems(ui.End).Label(tr(c, "Cloud usage chart")).Children(func() {
				for _, day := range v.Usage {
					height := float32(2)
					color := a.palette.Text.Alpha(.06)
					if day.Credits > 0 {
						height = 88 * float32(max(.04, day.Credits/peak))
						color = a.palette.Accent.Alpha(.85)
					}
					ui.Box(c).Grow(1).MinWidth(1).Height(height).Radius(2).Background(color).Tooltip(tr(c, date(c, day.Day, "Jan 2")) + " · " + tr(c, integrationCredits(day.Credits)) + tr(c, " credits"))
				}
			})
			ui.Row(c).FillWidth().Children(func() {
				ui.Text(c, date(c, v.Usage[0].Day, "Jan 2")).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
				ui.Spacer(c)
				ui.Text(c, date(c, v.Usage[len(v.Usage)-1].Day, "Jan 2")).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
			})
		})
	})
}
