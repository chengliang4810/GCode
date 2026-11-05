package desktop

import (
	"fmt"
	"slices"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

type pluginSettingsState struct {
	query, marketplace          string
	catalogQuery, catalogSource string
	catalogPage                 int
	sourcesOpen, sourceOpen     bool
	source                      contracts.PluginMarketplaceInput
	sourcePending, sourceError  string
	sourceEpoch                 uint64
	configID                    string
	config                      map[string]*string
	removeConfig                []string
}

func (a *App) pluginSettings(c *ui.Context) {
	ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
		textButton(c.Key("plugin-browse-marketplace"), tr(c, "Browse plugin marketplace")).OnClick(func() {
			a.selectSettingsSection("Marketplace")
		})
	})
	a.pluginOperations(c)
	a.pluginInstalled(c)
}

func (a *App) pluginMarketplaceActions(c *ui.Context) {
	ui.Row(c.Key("plugin-marketplace-actions")).Gap(8).AlignItems(ui.Center).Children(func() {
		iconButton(c.Key("plugin-source-manage"), "list", tr(c, "Manage marketplace sources")).OnClick(func() {
			a.capabilityUI.plugins.sourcesOpen = true
			a.capabilityUI.plugins.sourceOpen = false
		})
		primaryButton(c.Key("plugin-source-add"), tr(c, "Add marketplace source")).OnClick(a.openPluginSourceAdd)
	})
}

func (a *App) pluginMarketplaceSettings(c *ui.Context) {
	a.pluginOperations(c)
	a.pluginCatalog(c)
}

func (a *App) pluginOperations(c *ui.Context) {
	for _, operation := range a.state.Plugins.Operations {
		if operation.State != "running" && operation.Error == "" {
			continue
		}
		labels := map[string]string{
			"plugins.source.save": "Save marketplace", "plugins.source.refresh": "Refresh marketplace",
			"plugins.source.remove": "Remove marketplace", "plugins.install": "Install",
			"plugins.update": "Update", "plugins.uninstall": "Uninstall",
			"plugins.enable": "Enable", "plugins.disable": "Disable", "plugins.configure": "Configure",
		}
		stateLabel := "Running"
		if operation.Error != "" {
			stateLabel = "Failed"
		}
		text := tr(c, labels[operation.Kind]) + " · " + operation.TargetID + " · " + tr(c, stateLabel)
		if operation.Error != "" {
			text += " · " + operation.Error
		}
		color := a.palette.Muted
		if operation.Error != "" {
			color = a.palette.Danger
		}
		ui.Text(c, text).FontSize(design.TextSize(c, design.TextSmall)).TextColor(color).Selectable()
	}
}

func pluginBusy(state contracts.PluginState, id string) bool {
	for _, operation := range state.Operations {
		if operation.State == "running" && operation.TargetID == id {
			return true
		}
	}
	return false
}

func pluginSummary(parts ...string) string {
	return strings.Join(slices.DeleteFunc(parts, func(part string) bool { return strings.TrimSpace(part) == "" }), " · ")
}

func (a *App) pluginAction(c *ui.Context, key, label, kind, id string) {
	textButton(c.Key("plugin-"+key+"-"+id), tr(c, label)).Label(tr(c, label) + " " + id).
		Disabled(pluginBusy(a.state.Plugins, id)).
		OnClick(func() { a.dispatch(contracts.Command{Kind: kind, ID: id}) })
}

func (a *App) pluginCatalog(c *ui.Context) {
	s := &a.capabilityUI.plugins
	controlTextInput(c.Key("plugin-search"), &s.query).FillWidth().Label(tr(c, "Search plugins")).Placeholder(tr(c, "Search plugins"))
	if s.marketplace != "" && !slices.ContainsFunc(a.state.Plugins.Marketplaces, func(source contracts.PluginMarketplace) bool {
		return source.ID == s.marketplace
	}) {
		s.marketplace = ""
	}
	ui.Row(c.Key("plugin-marketplace-tabs")).FillWidth().Wrap().Gap(8).Children(func() {
		controlNavButton(c.Key("plugin-marketplace-all"), tr(c, "All marketplaces"), s.marketplace == "").
			Children(func() { ui.Text(c, tr(c, "All marketplaces")) }).OnClick(func() { s.marketplace = "" })
		for _, source := range a.state.Plugins.Marketplaces {
			controlNavButton(c.Key("plugin-marketplace-"+source.ID), source.Name, s.marketplace == source.ID).
				Children(func() { ui.Text(c, source.Name).SingleLine() }).OnClick(func() { s.marketplace = source.ID })
		}
	})
	query := strings.ToLower(strings.TrimSpace(s.query))
	if query != s.catalogQuery || s.marketplace != s.catalogSource {
		s.catalogPage = 0
		s.catalogQuery, s.catalogSource = query, s.marketplace
	}
	filterName := s.marketplace
	for _, source := range a.state.Plugins.Marketplaces {
		if source.ID == s.marketplace {
			filterName = source.Name
		}
	}
	var items []contracts.PluginCatalogItem
	for _, item := range a.state.Plugins.Catalog {
		if s.marketplace != "" && item.Marketplace != s.marketplace && item.Marketplace != filterName {
			continue
		}
		if matchesResource(query, item.Name, item.Description, item.Author, item.Category) {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		s.catalogPage = 0
		ui.Text(c, tr(c, "No plugins. Refresh a marketplace source to load its catalog.")).TextColor(a.palette.Muted)
		return
	}
	// Bound the native tree even when a marketplace contains thousands of plugins.
	const pageSize = 30
	s.catalogPage = min(s.catalogPage, (len(items)-1)/pageSize)
	start := s.catalogPage * pageSize
	end := min(start+pageSize, len(items))
	if len(items) > pageSize {
		ui.Row(c.Key("plugin-catalog-pagination")).FillWidth().Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
			textButton(c.Key("plugin-catalog-previous"), tr(c, "Previous page")).Disabled(start == 0).
				OnClick(func() { s.catalogPage-- })
			ui.Text(c, fmt.Sprintf(tr(c, "%d–%d of %d plugins"), start+1, end, len(items)))
			textButton(c.Key("plugin-catalog-next"), tr(c, "Next page")).Disabled(end == len(items)).
				OnClick(func() { s.catalogPage++ })
		})
	}
	for _, item := range items[start:end] {
		ui.Column(c.Key("plugin-catalog-"+item.ID)).FillWidth().Gap(8).Padding(12, 0).
			BorderWidth(0, 0, 1, 0).BorderColor(a.palette.Border).Children(func() {
			ui.Text(c, item.Name).FontWeight(600)
			ui.Text(c, item.Description).TextColor(a.palette.Muted)
			ui.Text(c, pluginSummary(item.Marketplace, item.Version)).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
			a.pluginDetails(c, item.Components, item.Issues)
			already := false
			for _, installed := range a.state.Plugins.Installed {
				if installed.CatalogID == item.ID {
					already = true
				}
			}
			primaryButton(c.Key("plugin-install-"+item.ID), tr(c, "Install")).
				Label(tr(c, "Install") + " " + item.Name).
				Disabled(already || pluginBusy(a.state.Plugins, item.ID)).
				OnClick(func() {
					input := contracts.PluginInstallInput{Marketplace: item.Marketplace, Name: item.Name}
					a.dispatch(contracts.Command{Kind: "plugins.install", Plugin: &contracts.PluginInput{Install: &input}})
				})
		})
	}
}

func (a *App) pluginDetails(c *ui.Context, components, issues []string) {
	if len(components) > 0 {
		ui.Text(c, strings.Join(components, " · ")).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
	}
	for _, issue := range issues {
		ui.Text(c, issue).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Warning).Selectable()
	}
}

func (a *App) pluginSourcesDialog(c *ui.Context) {
	s := &a.capabilityUI.plugins
	title := "Manage marketplace sources"
	if s.sourceOpen {
		title = "Add plugin marketplace"
		if s.source.ID != "" {
			title = "Edit marketplace source"
		}
	}
	a.settingsDialog(c, "plugin-sources-dialog", title, &s.sourcesOpen, 680, func() {
		if s.sourceOpen && s.source.ID == "" {
			a.pluginSourceAddForm(c)
		} else {
			a.pluginOperations(c)
			a.pluginSources(c)
		}
	})
	if !s.sourcesOpen {
		s.sourceOpen = false
		s.source = contracts.PluginMarketplaceInput{}
		s.sourcePending, s.sourceError = "", ""
	}
}

func (a *App) pluginSources(c *ui.Context) {
	s := &a.capabilityUI.plugins
	if s.sourceOpen {
		resourceText(c, "plugin-source-name", "Marketplace name", &s.source.Name)
		resourceText(c, "plugin-source-location", "Marketplace source", &s.source.Source)
		a.resourceSelect(c, "plugin-source-kind", "Source type", &s.source.Kind,
			[]settingOption{{"github", "GitHub"}, {"git", "Git"}, {"directory", "Directory"}, {"file", "File"}, {"url", "URL"}})
		resourceText(c, "plugin-source-ref", "Git reference", &s.source.Ref)
		ui.Row(c).Wrap().Gap(8).Children(func() {
			textButton(c.Key("plugin-source-cancel"), tr(c, "Cancel")).OnClick(func() { s.sourceOpen = false })
			primaryButton(c.Key("plugin-source-save"), tr(c, "Save")).Disabled(strings.TrimSpace(s.source.Source) == "").
				OnClick(func() {
					source := s.source
					a.dispatch(contracts.Command{Kind: "plugins.source.save", Plugin: &contracts.PluginInput{Marketplace: &source}})
					s.sourceOpen = false
				})
		})
		return
	}
	if len(a.state.Plugins.Marketplaces) == 0 {
		ui.Text(c, tr(c, "No marketplace sources")).TextColor(a.palette.Muted)
	}

	for _, source := range a.state.Plugins.Marketplaces {
		ui.Column(c.Key("plugin-source-"+source.ID)).FillWidth().Gap(8).Padding(12, 0).
			BorderWidth(0, 0, 1, 0).BorderColor(a.palette.Border).Children(func() {
			ui.Text(c, source.Name).FontWeight(600)
			ui.Text(c, source.Source).TextColor(a.palette.Muted).Selectable()
			ui.Text(c, fmt.Sprintf("%s · %d", source.State, source.PluginCount)).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted)
			if source.Error != "" {
				ui.Text(c, source.Error).TextColor(a.palette.Danger).Selectable()
			}
			ui.Row(c).Wrap().Gap(8).Children(func() {
				a.pluginAction(c, "refresh", "Refresh", "plugins.source.refresh", source.ID)
				textButton(c.Key("plugin-source-edit-"+source.ID), tr(c, "Edit")).Label(tr(c, "Edit") + " " + source.ID).
					Disabled(pluginBusy(a.state.Plugins, source.ID)).OnClick(func() {
					s.source = contracts.PluginMarketplaceInput{ID: source.ID, Name: source.Name, Source: source.Source, Kind: source.Kind, Ref: source.Ref}
					s.sourceOpen = true
				})
				a.pluginAction(c, "remove", "Remove", "plugins.source.remove", source.ID)
			})
		})
	}
}

func (a *App) pluginInstalled(c *ui.Context) {
	s := &a.capabilityUI.plugins
	if len(a.state.Plugins.Installed) == 0 {
		ui.Text(c, tr(c, "No installed plugins")).TextColor(a.palette.Muted)
	}
	for _, installed := range a.state.Plugins.Installed {
		ui.Column(c.Key("plugin-installed-"+installed.ID)).FillWidth().Gap(8).Padding(12, 0).
			BorderWidth(0, 0, 1, 0).BorderColor(a.palette.Border).Children(func() {
			ui.Text(c, installed.Name).FontWeight(600)
			detail := pluginSummary(installed.Marketplace, installed.Version)
			ui.Text(c, detail).FontSize(design.TextSize(c, design.TextSmall)).TextColor(a.palette.Muted).Selectable()
			a.pluginDetails(c, installed.Components, installed.Issues)
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				enabled := installed.Enabled
				a.controlSwitch(c.Key("plugin-enabled-"+installed.ID), &enabled, tr(c, "Enable")+" "+installed.ID, pluginBusy(a.state.Plugins, installed.ID), func(value bool) {
					kind := "plugins.disable"
					if value {
						kind = "plugins.enable"
					}
					a.dispatch(contracts.Command{Kind: kind, ID: installed.ID})
				})
				a.pluginAction(c, "update", "Update", "plugins.update", installed.ID)
				a.pluginAction(c, "uninstall", "Uninstall", "plugins.uninstall", installed.ID)
				if len(installed.Config) > 0 {
					textButton(c.Key("plugin-configure-"+installed.ID), tr(c, "Configure")).Label(tr(c, "Configure") + " " + installed.ID).OnClick(func() {
						s.configID, s.config, s.removeConfig = installed.ID, make(map[string]*string), nil
					})
				}
			})
			if s.configID == installed.ID {
				a.pluginConfig(c, installed)
			}
		})
	}
}

func (a *App) pluginConfig(c *ui.Context, installed contracts.InstalledPlugin) {
	s := &a.capabilityUI.plugins
	for _, field := range installed.Config {
		if s.config[field.Key] == nil {
			s.config[field.Key] = new(string)
		}
		value := s.config[field.Key]
		label := field.Title
		if label == "" {
			label = field.Key
		}
		format := ""
		switch {
		case field.Multiple:
			format = "JSON string[]"
		case field.Type == "number":
			format = tr(c, "Number")
			if field.Min != nil {
				format += fmt.Sprintf(" ≥ %g", *field.Min)
			}
			if field.Max != nil {
				format += fmt.Sprintf(" ≤ %g", *field.Max)
			}
		case field.Type == "boolean":
			format = "true / false"
		}
		title := label
		if format != "" {
			title += " (" + format + ")"
		}
		ui.Text(c, title).FontWeight(500)
		if field.Description != "" {
			ui.Text(c, field.Description).TextColor(a.palette.Muted).FontSize(design.TextSize(c, design.TextSmall))
		}
		placeholder := ""
		if field.Configured {
			placeholder = tr(c, "Configured; leave blank to keep")
		}
		if field.Saved {
			remove := slices.Contains(s.removeConfig, field.Key)
			controlCheckbox(c.Key("plugin-config-remove-"+field.Key), &remove, tr(c, "Remove saved value")+" "+label).
				OnClick(func() {
					if remove {
						s.removeConfig = append(s.removeConfig, field.Key)
					} else {
						s.removeConfig = slices.DeleteFunc(s.removeConfig, func(key string) bool { return key == field.Key })
					}
				})
		}
		if format != "" {
			placeholder = pluginSummary(placeholder, format)
		}
		inputLabel := tr(c, "Plugin configuration") + " " + label
		if field.Sensitive {
			controlSecretInput(c.Key("plugin-config-"+field.Key), value, false, false, func(input ui.Element) {
				input.FillWidth().Label(inputLabel).Placeholder(placeholder)
			}, nil)
		} else if !field.Multiple && (field.Type == "boolean" || len(field.Options) > 0) {
			options := []settingOption{{"", "Leave unchanged"}}
			values := field.Options
			if field.Type == "boolean" {
				values = []string{"true", "false"}
			}
			for _, option := range values {
				options = append(options, settingOption{option, option})
			}
			a.resourceSelect(c, "plugin-config-"+field.Key, inputLabel, value, options)
		} else {
			controlTextInput(c.Key("plugin-config-"+field.Key), value).FillWidth().Label(inputLabel).Placeholder(placeholder)
		}
	}
	ui.Row(c).Gap(8).Children(func() {
		textButton(c.Key("plugin-config-cancel"), tr(c, "Cancel")).OnClick(func() { s.configID, s.config = "", nil })
		primaryButton(c.Key("plugin-config-save"), tr(c, "Save")).Label(tr(c, "Save plugin configuration")).
			Disabled(pluginBusy(a.state.Plugins, installed.ID)).OnClick(func() {
			values := make(map[string]string)
			for key, value := range s.config {
				if *value != "" && !slices.Contains(s.removeConfig, key) {
					values[key] = *value
				}
			}
			a.dispatch(contracts.Command{Kind: "plugins.configure", ID: installed.ID, Plugin: &contracts.PluginInput{Configuration: values, RemoveConfigKeys: s.removeConfig}})
			s.configID, s.config, s.removeConfig = "", nil, nil
		})
	})
}
