package desktop

import (
	"fmt"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func (a *App) providerSettings(c *ui.Context) {
	if a.state.Runtime != nil {
		a.customProviderSettings(c)
		return
	}
	state := a.state.Integrations
	ui.Row(c).FillWidth().Padding(0, 8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, tr(c, "Device")).TextColor(a.palette.Muted)
		ui.Spacer(c)
		label := tr(c, "This device")
		for _, d := range a.state.Devices {
			if d.ID == a.integrationTarget() {
				label = a.deviceDisplayName(d)
			}
		}
		if a.integrationTarget() == "cloud-1" {
			label = tr(c, "Cloud")
		}
		controlMenuButton(c, label, func(menu *ui.Menu) {
			for _, d := range a.state.Devices {
				if d.Platform == "cloud" {
					continue
				}
				if menu.Item(a.deviceDisplayName(d)).Checked(d.ID == a.integrationTarget()).Chosen() {
					a.integrationUI.ProviderTarget = d.ID
					a.integrationUI.ExpandedProvider = ""
				}
			}
			if a.state.User.SignedIn && a.state.Integrations.Cloud.State == "ready" {
				menu.Separator()
				if menu.Item(tr(c, "Cloud")).Checked(a.integrationTarget() == "cloud-1").Chosen() {
					a.integrationUI.ProviderTarget = "cloud-1"
					a.integrationUI.ExpandedProvider = ""
				}
			}
		}).Label(tr(c, "Provider device"))
		iconButton(c, "refresh", tr(c, "Check for agents and updates")).OnClick(func() { a.dispatch(contracts.Command{Kind: "providers.refresh"}) })
	})
	if state.ProvidersState == contracts.LoadLoading {
		a.integrationLoading(c, "Loading providers")
		return
	}
	if state.ProvidersState == contracts.LoadError {
		a.settingsCard(c, func() { a.integrationError(c, state.ProvidersError, "providers.refresh", nil) })
		return
	}
	a.settingsCard(c, func() {
		first := true
		for _, p := range a.state.Providers {
			d, ok := a.providerDetails(p.ID)
			if !ok {
				continue
			}
			if !first {
				line(c)
			}
			first = false
			a.providerRow(c, p, d)
			if d.Enabled && a.integrationUI.ExpandedProvider == p.ID {
				a.providerDetailsView(c, p, d)
			}
		}
	})
}

func (a *App) providerDetails(id string) (contracts.ProviderDetails, bool) {
	for _, d := range a.state.Integrations.Providers {
		if d.TargetID == a.integrationTarget() && d.ProviderID == id {
			if d.TargetID == "local" {
				for _, p := range a.state.Providers {
					if p.ID == id {
						d.Enabled = p.Enabled
					}
				}
			}
			return d, true
		}
	}
	return contracts.ProviderDetails{}, false
}

func (a *App) providerRow(c *ui.Context, p contracts.Provider, d contracts.ProviderDetails) {
	cloud := a.integrationTarget() == "cloud-1"
	expanded := d.Enabled && a.integrationUI.ExpandedProvider == p.ID
	ui.Row(c.Key("provider-"+p.ID)).FillWidth().Padding(10, 16).Gap(12).AlignItems(ui.Center).Children(func() {
		trigger := controlRowButton(c).Label(p.Name + tr(c, " preferences")).Grow(1).MinWidth(0).Disabled(!d.Enabled).OnClick(func() {
			if expanded {
				a.integrationUI.ExpandedProvider = ""
			} else {
				a.integrationUI.ExpandedProvider = p.ID
			}
		})
		trigger.Children(func() {
			ui.Box(c).Size(36, 36).Radius(10).Background(a.palette.Text.Alpha(.06)).Center().Children(func() { icon(c, p.Icon, 16) })
			ui.Column(c).Grow(1).AlignItems(ui.Start).Gap(3).Children(func() {
				ui.Text(c, p.Name).FontSize(design.TextSize(c, design.TextBase)).FontWeight(500).LineHeight(1.5)
				meta := providerVersion(d)
				if cloud {
					count := 0
					for _, account := range a.state.Integrations.Accounts {
						if account.ProviderID == p.ID && account.TargetID == d.TargetID {
							count++
						}
					}
					meta = "No account"
					if count == 1 {
						meta = "1 account"
					} else if count > 1 {
						meta = fmt.Sprintf(tr(c, "%d accounts"), count)
					}
				}
				if !d.Installed {
					meta = tr(c, "Install the ") + providerCLI(p.ID) + tr(c, " CLI to enable")
				}
				if d.InstallState == "installing" {
					meta = tr(c, "Installing ") + p.Name + "…"
				}
				if d.Error != "" {
					meta = d.Error
				}
				if meta != "" {
					ui.Text(c, tr(c, meta)).TextColor(a.palette.Muted)
				}
			})
			if d.Enabled {
				glyph := "alt-arrow-down"
				if expanded {
					glyph = "alt-arrow-up"
				}
				icon(c, glyph, 12)
			}
		})
		if !cloud {
			if d.InstallState == "installing" {
				textButton(c, tr(c, "Cancel")).Label(tr(c, "Cancel installing ") + p.Name).OnClick(func() { a.dispatch(a.integrationCommand("provider.install.cancel", p.ID)) })
			} else if !d.Installed && d.CanInstall {
				textButton(c, tr(c, "Install")).Label(tr(c, "Install ") + p.Name).OnClick(func() { a.dispatch(a.integrationCommand("provider.install", p.ID)) })
			} else if d.UpdateState == "available" || d.UpdateState == "failed" {
				label := "Update"
				if d.UpdateState == "failed" {
					label = "Retry update"
				}
				textButton(c, tr(c, label)).Label(tr(c, label) + " " + p.Name).OnClick(func() { a.dispatch(a.integrationCommand("provider.update", p.ID)) })
			} else if d.UpdateState == "downloading" {
				textButton(c, tr(c, "Cancel")).Label(tr(c, "Cancel updating ") + p.Name).OnClick(func() { a.dispatch(a.integrationCommand("provider.update.cancel", p.ID)) })
			}
			enabled := 0
			for _, other := range a.state.Providers {
				if x, ok := a.providerDetails(other.ID); ok && x.Enabled && x.Installed {
					enabled++
				}
			}
			disabled := d.Enabled && d.Installed && enabled == 1 || !d.Enabled && !d.Installed
			a.integrationToggle(c, "provider-toggle-"+p.ID, tr(c, "Enable ")+p.Name, d.Enabled, disabled, func(on bool) { cmd := a.integrationCommand("provider.enable", p.ID); cmd.Enabled = on; a.dispatch(cmd) })
		}
	})
}

func providerCLI(id string) string {
	switch id {
	case "claude":
		return "claude"
	case "openai":
		return "codex"
	case "cursor":
		return "cursor-agent"
	}
	return id
}
func providerVersion(d contracts.ProviderDetails) string {
	switch d.UpdateState {
	case "downloading":
		return "Downloading…"
	case "updated":
		return "Updated"
	case "failed":
		return "Update failed"
	case "available":
		return "v" + d.Version + " → v" + d.LatestVersion
	}
	if d.Version != "" {
		return "v" + d.Version
	}
	return ""
}

func (a *App) providerDetailsView(c *ui.Context, p contracts.Provider, d contracts.ProviderDetails) {
	ui.Column(c.Key("provider-details-"+p.ID)).FillWidth().Padding(0, 16, 16, 64).Gap(20).Children(func() {
		ui.Column(c).FillWidth().Children(func() {
			a.integrationLabel(c, "Completion")
			for i, row := range []struct {
				key, title, description string
				value                   bool
			}{{"dollar", "Use $ for skills", "Type $ in the composer to pick a skill.", d.DollarSkills}, {"slash", "Separate / commands", "Keep skills out of the / menu.", d.SeparateSlash}} {
				if i > 0 {
					line(c)
				}
				a.providerPreferenceRow(c, row.title, row.description, func() {
					a.integrationToggle(c, p.ID+row.key, p.Name+": "+row.title, row.value, false, func(on bool) {
						cmd := a.integrationCommand("provider.preference", p.ID)
						cmd.Name = row.key
						cmd.Enabled = on
						a.dispatch(cmd)
					})
				})
			}
		})
		if d.CanSwitchSubagents && d.TargetID != "cloud-1" {
			ui.Column(c).FillWidth().Children(func() {
				a.integrationLabel(c, "Delegation")
				a.providerPreferenceRow(c, "Native subagents", "Off: delegate through side chats, from each chat's next turn.", func() {
					a.integrationToggle(c, p.ID+"native-subagents", p.Name+tr(c, ": Native subagents"), d.NativeSubagents, false, func(on bool) {
						cmd := a.integrationCommand("provider.preference", p.ID)
						cmd.Name = "subagents"
						cmd.Enabled = on
						a.dispatch(cmd)
					})
				})
			})
		}
		if d.TargetID != "cloud-1" && d.Installed {
			ui.Column(c).FillWidth().Children(func() {
				a.integrationLabel(c, "Updates")
				detail, label := "Install only when you choose Update.", "Notify"
				switch d.UpdatePolicy {
				case "autoWhenIdle":
					detail, label = "Install automatically after active runs finish.", "Auto when idle"
				case "off":
					detail, label = "Don't check for new versions.", "Off"
				}
				a.providerPreferenceRow(c, "Update policy", detail, func() {
					controlMenuButton(c, label, func(menu *ui.Menu) {
						for _, option := range []settingOption{{"notify", "Notify"}, {"autoWhenIdle", "Auto when idle"}, {"off", "Off"}} {
							if menu.Item(tr(c, option.label)).Checked(d.UpdatePolicy == option.value).Chosen() {
								cmd := a.integrationCommand("provider.preference", p.ID)
								cmd.Name = "updatePolicy"
								cmd.Integration.Value = option.value
								a.dispatch(cmd)
							}
						}
					}).Label(p.Name + tr(c, " update policy")).Width(136)
				})
			})
		}
		a.providerAccounts(c, p, d)
	})
}

func (a *App) providerAccounts(c *ui.Context, p contracts.Provider, d contracts.ProviderDetails) {
	ui.Column(c).FillWidth().Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			a.integrationLabel(c, "Accounts")
			ui.Spacer(c)
			iconButton(c, "refresh", tr(c, "Refresh accounts")).OnClick(func() { a.dispatch(a.integrationCommand("provider.accounts.refresh", p.ID)) })
		})
		if d.Warning != "" {
			ui.Text(c, d.Warning).TextColor(a.palette.Danger)
		}
		if d.AccountNote != "" {
			ui.Text(c, d.AccountNote).TextColor(a.palette.Muted).Margin(4, 0, 0, 0)
		}
		if d.AccountsState == contracts.LoadLoading {
			a.integrationLoading(c, "Loading accounts")
			return
		}
		if d.AccountsState == contracts.LoadError {
			a.integrationError(c, d.AccountsError, "provider.accounts.refresh", &contracts.IntegrationRequest{TargetID: d.TargetID, ProviderID: p.ID})
			return
		}
		count := 0
		for _, account := range a.state.Integrations.Accounts {
			if account.ProviderID != p.ID || account.TargetID != d.TargetID {
				continue
			}
			if count > 0 {
				line(c)
			}
			count++
			a.providerAccountRow(c, account)
		}
		if count == 0 || !d.SingleLogin {
			if count > 0 {
				line(c)
			}
			ui.Row(c).FillWidth().Padding(8, 0).Gap(4).Children(func() {
				options := d.LoginOptions
				if len(options) == 0 {
					options = []contracts.ProviderLoginOption{{}}
				}
				for _, option := range options {
					label := "Add account"
					if count == 0 {
						label = tr(c, "Connect a ") + p.Name + tr(c, " account")
					}
					if option.ID != "" {
						label = tr(c, "Add ") + option.Label + tr(c, " account")
						if count == 0 {
							label = tr(c, "Connect ") + option.Label
						}
					}
					textButton(c, "+ "+tr(c, label)).Label(tr(c, label)).OnClick(func() {
						cmd := a.integrationCommand("provider.login.start", p.ID)
						cmd.Integration.Value = option.ID
						a.dispatch(cmd)
					})
				}
				if d.TargetID == "cloud-1" && (p.ID == "claude" || p.ID == "openai") && a.integrationUI.APIProvider != p.ID {
					key := "Anthropic API key"
					if p.ID == "openai" {
						key = "OpenAI API key"
					}
					textButton(c, tr(c, "+ Add ")+tr(c, key)).Label(tr(c, "Add ") + tr(c, key)).OnClick(func() { a.integrationUI.APIProvider = p.ID; a.integrationUI.APIKey = "" })
				}
			})
		}
		if a.integrationUI.APIProvider == p.ID {
			ui.Column(c).FillWidth().Gap(8).Padding(8, 0).Children(func() {
				controlTextInput(c, &a.integrationUI.APIKey).Password().Label(tr(c, "API key")).Placeholder(tr(c, "Paste an API key")).AutoFocus()
				ui.Text(c, tr(c, "Stored encrypted in your account for Cloud. It becomes the account used by your Cloud sessions.")).TextColor(a.palette.Muted)
				ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
					textButton(c, tr(c, "Cancel")).OnClick(func() { a.integrationUI.APIProvider = ""; a.integrationUI.APIKey = "" })
					textButton(c, tr(c, "Save key")).Disabled(strings.TrimSpace(a.integrationUI.APIKey) == "" || a.integrationUI.APISaving).OnClick(func() {
						cmd := a.integrationCommand("provider.apiKey.save", p.ID)
						cmd.Text = a.integrationUI.APIKey
						a.integrationUI.APISaving = true
						a.dispatch(cmd)
					})
				})
			})
		}
	})
}

func (a *App) providerAccountRow(c *ui.Context, account contracts.ProviderAccount) {
	ui.Row(c.Key("account-"+account.ID)).FillWidth().MinHeight(64).Padding(10, 0).Gap(10).AlignItems(ui.Center).Children(func() {
		color := ui.Transparent
		if account.InUse {
			color = a.palette.Accent
		}
		initial := "?"
		if chars := []rune(account.Email); len(chars) > 0 {
			initial = strings.ToUpper(string(chars[0]))
		}
		ui.Box(c).Size(32, 32).Radius(16).Border(1.5, color).Center().Children(func() {
			ui.Box(c).Size(25, 25).Radius(12.5).Background(a.palette.Text.Alpha(.09)).Center().Children(func() { ui.Text(c, initial).FontSize(design.TextSize(c, design.TextBase)).FontWeight(500) })
		})
		name := account.Email
		if name == "" {
			name = "Unknown account"
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, name).SingleLine()
			ui.Row(c).Gap(6).Children(func() {
				ui.Text(c, account.Plan).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
				if account.InUse {
					ui.Text(c, tr(c, "In use")).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Accent)
				}
				if len(account.Usage) == 0 && account.ReportsUsage {
					reason := account.UsageError
					if reason == "" {
						reason = "Usage unavailable"
					}
					if account.UsageState == contracts.LoadLoading {
						reason = "Checking usage…"
					}
					ui.Text(c, tr(c, reason)).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
				}
			})
		})
		ui.Column(c).Width(190).Gap(2).Children(func() {
			for i, window := range account.Usage {
				if i >= 2 {
					break
				}
				fraction := max(0, min(1, window.UsedPercent/100))
				color := a.palette.Muted
				if fraction >= .95 {
					color = a.palette.Danger
				} else if fraction >= .80 {
					color = a.palette.Warning
				}
				ui.Row(c).Height(16).Gap(8).AlignItems(ui.Center).Tooltip(tr(c, window.Label) + tr(c, ": resets ") + tr(c, date(c, window.ResetsAt.Local(), "Mon 3:04 PM"))).Children(func() {
					ui.Text(c, window.Label).Width(52).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted).SingleLine()
					ui.Box(c).Width(88).Height(4).Radius(2).Background(a.palette.Text.Alpha(.08)).Children(func() {
						if fraction > 0 {
							ui.Box(c).Width(88 * float32(max(.015, fraction))).Height(4).Radius(2).Background(color.Alpha(.8))
						}
					})
					ui.Text(c, fmt.Sprintf(tr(c, "%.0f%%"), window.UsedPercent)).Width(34).FontSize(design.TextSize(c, design.TextBase)).TextColor(color).TextAlign(ui.End)
				})
			}
		})
		iconButton(c, "menu-dots", tr(c, "Actions for ")+name).Menu(func(menu *ui.Menu) {
			req := &contracts.IntegrationRequest{TargetID: account.TargetID, ProviderID: account.ProviderID, AccountID: account.ID}
			if menu.Item(tr(c, "Switch to this account")).Disabled(account.InUse || !account.Switchable).Chosen() {
				a.dispatch(contracts.Command{Kind: "provider.account.switch", Integration: req})
			}
			if menu.Item(tr(c, "Remove account")).Disabled(!account.CanRemove).Chosen() {
				a.dispatch(contracts.Command{Kind: "provider.account.remove", Integration: req})
			}
		})
	})
}

func (a *App) providerPreferenceRow(c *ui.Context, title, description string, control func()) {
	ui.Row(c.Key("provider-preference-"+title)).FillWidth().MinHeight(52).Padding(10, 0).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, tr(c, title)).FontSize(design.TextSize(c, design.TextBase)).FontWeight(500).LineHeight(1.5)
			ui.Text(c, tr(c, description)).TextColor(a.palette.Muted)
		})
		control()
	})
}
