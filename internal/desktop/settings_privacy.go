package desktop

import (
	"fmt"
	"slices"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/third_party/magpie/pii"
	"gcode/third_party/magpie/redact"
	"github.com/egoist/mygo/ui"
)

func (a *App) privacySettings(c *ui.Context) {
	p := &a.state.Settings.Privacy
	a.settingsCard(c, func() {
		a.settingsRow(c, "Mask sensitive information", "Replace sensitive text before sending it to a model and restore it in replies and tool calls. Off by default.", func() {
			a.settingsToggle(c, "Mask sensitive information", &p.Enabled, func() {
				if p.Enabled && !p.Secrets && !p.Personal && len(p.Words) == 0 {
					p.Secrets = true
				}
			})
		})
	})
	if !p.Enabled {
		return
	}
	ui.Text(c, tr(c, "Masking covers request text. Images, attachments, encoded data and signed reasoning are not scanned. Local conversation history keeps original values.")).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
	sectionTitle(c, tr(c, "Detection"))
	a.settingsCard(c, func() {
		a.settingsRow(c, "Mask secrets", "API keys, private keys, tokens, passwords and custom rules.", func() {
			a.settingsToggle(c, "Mask secrets", &p.Secrets)
		})
		line(c)
		a.settingsRow(c, "Keep key format", "Replace supported keys with made-up keys of the same form instead of placeholders.", func() {
			a.settingsToggle(c, "Keep key format", &p.Shape)
		})
		line(c)
		a.settingsRow(c, "Mask personal data", "Email addresses, phone numbers, identity documents and bank cards.", func() {
			a.settingsToggle(c, "Mask personal data", &p.Personal)
		})
		line(c)
		a.settingsRow(c, "Notify when values are masked", "Show counts only. Warning actions always notify.", func() {
			a.settingsToggle(c, "Notify when values are masked", &p.Notify)
		})
	})
	if p.Personal {
		sectionTitle(c, tr(c, "Personal data categories"))
		a.settingsCard(c, func() {
			for index, category := range redact.Categories {
				if index > 0 {
					line(c)
				}
				category := category
				on, exists := p.Kinds[category.ID]
				if !exists {
					on = category.On
				}
				label := privacyCategoryLabel(category.ID)
				a.settingsRow(c, label, "", func() {
					a.settingsToggle(c, label, &on, func() {
						if p.Kinds == nil {
							p.Kinds = map[string]bool{}
						}
						p.Kinds[category.ID] = on
					})
				})
			}
		})
	}
	sectionTitle(c, tr(c, "Custom sensitive words"))
	a.privacyListInput(c, "privacy-words", "One sensitive word per line", p.Words, func(values []string) { p.Words = values })
	a.settingsCard(c, func() {
		a.settingsRow(c, "Ignore word case", "", func() {
			a.settingsToggle(c, "Ignore word case", &p.WordsAnyCase)
		})
		line(c)
		a.settingsRow(c, "Match whole words", "", func() {
			a.settingsToggle(c, "Match whole words", &p.WordsWhole)
		})
	})
	a.privacyRules(c)
	a.privacyLocalModels(c)
	sectionTitle(c, tr(c, "Allowlist"))
	ui.Text(c, tr(c, "These whole values are sent unchanged, regardless of detection rules. Matching ignores case.")).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
	a.privacyListInput(c, "privacy-allow", "One allowed value per line", p.Allow, func(values []string) { p.Allow = values })
	a.privacyActions(c)
}

func privacyCategoryLabel(id string) string {
	return map[string]string{
		"ssn": "Social security numbers", "passport": "Passport numbers", "iban": "International bank accounts",
		"birthday": "Birthdays", "mac": "MAC addresses", "serial": "Serial numbers",
		"home": "Home directory usernames", "userhost": "User and host names", "ip": "Public IP addresses", "bucket": "Cloud storage buckets",
	}[id]
}

func privacyLines(text string) []string {
	var values []string
	for _, line := range strings.Split(text, "\n") {
		if value := strings.TrimSpace(line); value != "" && !slices.Contains(values, value) {
			values = append(values, value)
		}
	}
	return values
}

func (a *App) privacyListInput(c *ui.Context, key, label string, values []string, set func([]string)) {
	owner := ui.Column(c.Key(key)).FillWidth()
	state := ui.Local(owner, "draft", func() string { return strings.Join(values, "\n") })
	owner.Children(func() {
		input := controlTextArea(c.Key(key+"-input"), state).FillWidth().Height(120).Label(tr(c, label)).Placeholder(tr(c, label))
		settingsInputCommit(input, state, func() {
			before := a.state.Settings.Privacy.Clone()
			set(privacyLines(*state))
			if err := a.state.Settings.Privacy.Validate(); err != nil {
				a.state.Settings.Privacy = before
				a.notification(toastError, err.Error())
				return
			}
			a.saveSettings()
		})
	})
}

type privacyRuleDraft struct {
	Kind, Mode, Pattern string
	Editing             int
}

func (a *App) privacyRules(c *ui.Context) {
	sectionTitle(c, tr(c, "Custom masking rules"))
	ui.Text(c, tr(c, "Rules run with Mask secrets. Choose a prefix or an RE2 expression; capture group 1 is the sensitive value.")).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
	a.settingsCard(c, func() {
		owner := ui.Column(c.Key("privacy-rules")).FillWidth()
		draft := ui.Local(owner, "draft", func() privacyRuleDraft {
			return privacyRuleDraft{Kind: "CUSTOM", Mode: "prefix", Editing: -1}
		})
		owner.Children(func() {
			for index, rule := range a.state.Settings.Privacy.Rules {
				index, rule := index, rule
				pattern := rule.Prefix
				if pattern == "" {
					pattern = rule.Regex
				}
				a.settingsDataRow(c, rule.Kind, pattern, func() {
					iconButton(c.Key(fmt.Sprintf("privacy-rule-edit-%d", index)), "pen-new-square", tr(c, "Edit masking rule")).OnClick(func() {
						draft.Kind, draft.Pattern, draft.Editing = rule.Kind, pattern, index
						draft.Mode = "prefix"
						if rule.Regex != "" {
							draft.Mode = "regex"
						}
					})
					iconButton(c.Key(fmt.Sprintf("privacy-rule-delete-%d", index)), "trash-bin-minimalistic", tr(c, "Delete masking rule")).OnClick(func() {
						a.state.Settings.Privacy.Rules = slices.Delete(slices.Clone(a.state.Settings.Privacy.Rules), index, index+1)
						draft.Editing, draft.Pattern = -1, ""
						a.saveSettings()
					})
				})
			}
			ui.Column(c.Key("privacy-rule-new")).FillWidth().Gap(8).Padding(12).Children(func() {
				controlTextInput(c.Key("privacy-rule-kind"), &draft.Kind).FillWidth().Label(tr(c, "Rule kind")).Placeholder("CUSTOM")
				sel := controlSelect(c.Key("privacy-rule-mode"), &draft.Mode)
				sel.Trigger.Label(tr(c, "Rule matching")).Children(func() {
					ui.Text(c, tr(c, map[string]string{"prefix": "Prefix", "regex": "Regular expression"}[draft.Mode]))
				})
				sel.Popup(func(panel ui.Element) {
					controlSelectPopup(c, panel).Children(func() {
						for _, option := range []settingOption{{"prefix", "Prefix"}, {"regex", "Regular expression"}} {
							controlSelectItem(c, sel.Item(option.value)).Children(func() { ui.Text(c, tr(c, option.label)) })
						}
					})
				})
				controlTextInput(c.Key("privacy-rule-pattern"), &draft.Pattern).FillWidth().Label(tr(c, "Rule pattern")).Placeholder(tr(c, "Prefix or regular expression"))
				label := "Add masking rule"
				if draft.Editing >= 0 {
					label = "Update masking rule"
					textButton(c, tr(c, "Cancel")).OnClick(func() { draft.Editing, draft.Pattern = -1, "" })
				}
				textButton(c.Key("privacy-rule-add"), tr(c, label)).Disabled(strings.TrimSpace(draft.Pattern) == "").OnClick(func() {
					rule := redact.Rule{Kind: draft.Kind}
					if draft.Mode == "prefix" {
						rule.Prefix = draft.Pattern
					} else {
						rule.Regex = draft.Pattern
					}
					next := slices.Clone(a.state.Settings.Privacy.Rules)
					if draft.Editing >= 0 && draft.Editing < len(next) {
						next[draft.Editing] = rule
					} else {
						next = append(next, rule)
					}
					rules, err := redact.CheckRules(next)
					if err != nil {
						a.notification(toastError, err.Error())
						return
					}
					a.state.Settings.Privacy.Rules = rules
					draft.Pattern, draft.Editing = "", -1
					a.saveSettings()
				})
			})
		})
	})
}

func (a *App) privacyActions(c *ui.Context) {
	sectionTitle(c, tr(c, "Detection actions"))
	ui.Text(c, tr(c, "Mask restores values in replies. Warn sends original values and shows a warning. Block stops the entire request.")).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
	kinds := []string{"TERM", "CUSTOM"}
	for _, kind := range redact.BuiltInKinds() {
		if !slices.Contains(kinds, kind) {
			kinds = append(kinds, kind)
		}
	}
	for _, rule := range a.state.Settings.Privacy.Rules {
		if !slices.Contains(kinds, rule.Kind) {
			kinds = append(kinds, rule.Kind)
		}
	}
	a.settingsCard(c, func() {
		for _, kind := range kinds {
			a.settingsDataRow(c, kind, "", func() {
				owner := ui.Box(c.Key("privacy-action-state-" + kind))
				action := ui.Local(owner, "value", func() string {
					if value := a.state.Settings.Privacy.Actions[kind]; value != "" {
						return value
					}
					return "mask"
				})
				owner.Children(func() {
					a.settingsChoice(c, kind, action, []settingOption{{"mask", "Mask"}, {"warn", "Warn"}, {"block", "Block"}}, func() {
						if a.state.Settings.Privacy.Actions == nil {
							a.state.Settings.Privacy.Actions = map[string]string{}
						}
						a.state.Settings.Privacy.Actions[kind] = *action
					})
				})
			})
		}
	})
}

func (a *App) privacyLocalModels(c *ui.Context) {
	p := &a.state.Settings.Privacy
	sectionTitle(c, tr(c, "Local model detection"))
	ui.Text(c, tr(c, "Optional local models detect names, addresses and secrets beyond rules. Download a model first; inference stays on this host.")).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
	a.settingsCard(c, func() {
		a.settingsRow(c, "Find with a local model", "If enabled detection fails, the request is blocked.", func() {
			a.settingsToggle(c, "Find with a local model", &p.LocalModel)
		})
		line(c)
		a.settingsRow(c, "Local privacy model", "", func() {
			const local = "local-privacy"
			a.renderSettingsModelPicker(c, p.ModelName, 200, settingsModelPicker{
				label: tr(c, "Choose ") + tr(c, "Local privacy model"), optionLabel: tr(c, "Local privacy model"),
				providers: []contracts.Provider{{ID: local, Name: tr(c, "Local models")}},
				models: []contracts.Model{
					{ID: "", ProviderID: local, Label: tr(c, "AstrLink Guard (default)")},
					{ID: pii.AstrLinkGuard, ProviderID: local, Label: "AstrLink Guard"},
					{ID: pii.PrivacyFilter, ProviderID: local, Label: "Privacy Filter"},
				},
				selectID: func(id string) { p.ModelName = id; a.saveSettings() },
			})
		})
		line(c)
		a.settingsRow(c, "Detection confidence", "Only findings at or above this score are masked.", func() {
			owner := ui.Box(c.Key("privacy-confidence-" + p.ModelName))
			value := ui.Local(owner, "value", func() string {
				return fmt.Sprintf("%.2f", pii.Threshold(p.ModelScores, pii.Of(p.ModelName)))
			})
			owner.Children(func() {
				a.settingsChoice(c, "Detection confidence", value, []settingOption{{"0.50", "50%"}, {"0.60", "60%"}, {"0.70", "70%"}, {"0.80", "80%"}, {"0.90", "90%"}, {"0.95", "95%"}, {"0.99", "99%"}}, func() {
					if p.ModelScores == nil {
						p.ModelScores = map[string]float64{}
					}
					var score float64
					_, _ = fmt.Sscan(*value, &score)
					p.ModelScores[pii.Of(p.ModelName).ID()] = score
				})
			})
		})
		for _, status := range a.state.PrivacyModels {
			status := status
			name := "AstrLink Guard"
			if status.Model == pii.PrivacyFilter {
				name = "Privacy Filter"
			}
			state := tr(c, "Not downloaded")
			if status.Installed {
				state = tr(c, "Downloaded")
			}
			if !status.Supported {
				state = tr(c, "Unavailable on this host")
			}
			if status.Downloading {
				state = fmt.Sprintf("%s %.1f / %.1f MB", tr(c, "Downloading"), float64(status.Done)/1e6, float64(status.Total)/1e6)
			}
			if status.Error != "" {
				state += ", " + status.Error
			}
			line(c)
			a.settingsDataRow(c, name, fmt.Sprintf("%.1f MB, %s", float64(status.Size)/1e6, state), func() {
				if status.Downloading {
					textButton(c.Key("privacy-model-cancel-"+status.Model), tr(c, "Cancel download")).OnClick(func() {
						a.dispatch(contracts.Command{Kind: "privacy.model.cancel", ID: status.Model})
					})
				} else if status.Installed {
					textButton(c.Key("privacy-model-remove-"+status.Model), tr(c, "Remove")).Disabled(p.Enabled && p.LocalModel && pii.Of(p.ModelName).ID() == status.Model).OnClick(func() {
						a.dispatch(contracts.Command{Kind: "privacy.model.remove", ID: status.Model})
					})
				} else {
					textButton(c.Key("privacy-model-download-"+status.Model), tr(c, "Download model")).Disabled(!status.Supported).OnClick(func() {
						a.dispatch(contracts.Command{Kind: "privacy.model.download", ID: status.Model})
					})
				}
			})
		}
	})
}
