package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/plugins"
	"gcode/internal/storage"
)

func readPluginConfiguration(root string) (map[string]map[string]string, error) {
	path := filepath.Join(root, "plugin-configuration.json")
	if err := rejectResourceLinks(path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) > 4<<20 {
		return nil, errors.New("plugin configuration exceeds 4 MiB")
	}
	var config map[string]map[string]string
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("plugin configuration: %w", err)
	}
	if config == nil {
		config = map[string]map[string]string{}
	}
	return config, nil
}

func publicPluginSource(source plugins.Source) string {
	address := source.URL
	if source.Repo != "" {
		address = source.Repo
	}
	if source.Relative != "" {
		address = source.Relative
	}
	if source.Path != "" && (source.Source == "directory" || source.Source == "file") {
		address = source.Path
	}
	if u, err := url.Parse(address); err == nil && u.IsAbs() && u.Host != "" {
		u.User = nil
		u.RawQuery, u.Fragment = "", ""
		address = u.String()
	}
	return address
}

func bundleComponents(b plugins.Bundle) []string {
	var result []string
	for _, c := range []struct {
		name string
		n    int
	}{{"skills", len(b.Skills)}, {"commands", len(b.Commands)}, {"agents", len(b.Agents)}, {"hooks", len(b.Hooks)}, {"mcp", len(b.MCPServers)}, {"lsp", len(b.LSPServers)}, {"output-styles", len(b.OutputStyles)}} {
		if c.n != 0 {
			result = append(result, c.name)
		}
	}
	return result
}

func bundleComponentWarnings(b plugins.Bundle) []string {
	var result []string
	for _, components := range [][]plugins.MarkdownComponent{b.Skills, b.Commands, b.Agents, b.OutputStyles} {
		for _, component := range components {
			for _, warning := range component.Warnings {
				result = append(result, component.Name+": "+warning)
			}
		}
	}
	return result
}

// pluginProjection is built by the host worker, never by the native view.
func (e *Engine) pluginProjection() contracts.PluginState {
	result := contracts.PluginState{}
	catalogs := e.plugins.Catalogs()
	for _, source := range e.plugins.Sources() {
		row := contracts.PluginMarketplace{ID: source.ID, Name: source.Name, Source: publicPluginSource(source.Source), Kind: source.Source.Source, Ref: source.Source.Ref, Builtin: source.Builtin, State: "idle"}
		for _, catalog := range catalogs {
			if catalog.SourceID != source.ID {
				continue
			}
			row.Name, row.State, row.PluginCount, row.UpdatedAt = catalog.Marketplace.Name, "ready", len(catalog.Marketplace.Plugins), catalog.RefreshedAt
			for _, entry := range catalog.Marketplace.Plugins {
				item := contracts.PluginCatalogItem{
					ID: entry.Name + "@" + catalog.Marketplace.Name, Name: entry.Name,
					Marketplace: catalog.Marketplace.Name, Description: entry.Description,
					Version: entry.Version, Category: entry.Category, Homepage: entry.Homepage,
				}
				if entry.Author != nil {
					item.Author = entry.Author.Name
				}
				result.Catalog = append(result.Catalog, item)
			}
		}
		result.Marketplaces = append(result.Marketplaces, row)
	}
	for _, install := range e.plugins.Installed() {
		item := contracts.InstalledPlugin{
			ID: install.ID, CatalogID: install.ID, Name: install.Name, Marketplace: install.Marketplace,
			Version: install.Version, Commit: install.Commit, Enabled: install.Enabled, Path: install.Path,
		}
		bundle, err := plugins.LoadInstallation(install)
		if err != nil {
			item.Issues = append(item.Issues, err.Error())
		} else {
			item.Name = bundle.Manifest.Name
			item.Components = bundleComponents(bundle)
			item.Issues = append(item.Issues, bundleComponentWarnings(bundle)...)
			for _, issue := range bundle.Unsupported {
				item.Issues = append(item.Issues, issue.Component+": "+issue.Message)
			}
			if len(bundle.LSPServers) > 0 {
				item.Issues = append(item.Issues, "LSP runtime is not connected.")
			}
			if len(bundle.OutputStyles) > 0 {
				item.Issues = append(item.Issues, "Output style selection requires host support.")
			}
			keys := slices.Sorted(maps.Keys(bundle.Manifest.UserConfig))
			for _, key := range keys {
				option := bundle.Manifest.UserConfig[key]
				field, configErr := publicPluginConfigField(key, option)
				if configErr != nil {
					item.Issues = append(item.Issues, "userConfig: "+configErr.Error())
					continue
				}
				item.Config = append(item.Config, field)
			}
			redactPluginConfigProjection(&item, pluginConfigSecrets(bundle.Manifest.UserConfig, nil))
		}
		result.Installed = append(result.Installed, item)
	}
	return result
}

func (e *Engine) installedPluginLocked(target string) (plugins.Installation, error) {
	for _, i := range e.plugins.Installed() {
		if i.ID == target {
			return i, nil
		}
	}
	return plugins.Installation{}, errors.New("installed plugin not found")
}

func marketplaceSource(input contracts.PluginMarketplaceInput) (plugins.MarketplaceSource, error) {
	kind := input.Kind
	if kind == "" {
		kind = "github"
	}
	source := plugins.Source{Source: kind, Ref: input.Ref}
	switch kind {
	case "github":
		source.Repo = strings.TrimSpace(input.Source)
	case "git", "url":
		source.URL = strings.TrimSpace(input.Source)
		if kind == "git" {
			source.Source = "url"
			// Marketplace Git repositories are distinguished from a direct
			// marketplace.json URL by their explicit source representation.
			source.Source = "git"
		}
	case "directory", "file":
		if !filepath.IsAbs(input.Source) {
			return plugins.MarketplaceSource{}, errors.New("local marketplace paths must be absolute on the host")
		}
		source.Path = filepath.Clean(input.Source)
	default:
		return plugins.MarketplaceSource{}, errors.New("unsupported marketplace source kind")
	}
	id, name := strings.TrimSpace(input.ID), strings.TrimSpace(input.Name)
	if name == "" {
		name = id
	}
	if id == "" {
		id = name
	}
	if id == "" {
		return plugins.MarketplaceSource{}, errors.New("marketplace name is required")
	}
	return plugins.MarketplaceSource{ID: id, Name: name, Source: source, Enabled: true}, nil
}

func (e *Engine) startPluginOperationLocked(command contracts.Command) error {
	if len(e.pluginOperations) >= 8 {
		return errors.New("at most eight plugin operations can be pending")
	}
	var install plugins.Installation
	if strings.HasPrefix(command.Kind, "plugins.") && !strings.HasPrefix(command.Kind, "plugins.source.") && command.Kind != "plugins.install" {
		var err error
		install, err = e.installedPluginLocked(command.ID)
		if err != nil {
			return err
		}
	}
	var source plugins.MarketplaceSource
	if command.Kind == "plugins.source.save" {
		var err error
		source, err = marketplaceSource(*command.Plugin.Marketplace)
		if err != nil {
			return err
		}
	}
	for _, op := range e.state.Plugins.Operations {
		if op.State == "running" && op.TargetID == command.ID && command.ID != "" {
			return errors.New("a plugin operation for this target is already running")
		}
	}
	operationID := command.RequestID
	if operationID == "" {
		operationID = id("plugin-operation")
	}
	target := command.ID
	if command.Plugin != nil && command.Plugin.Install != nil {
		target = command.Plugin.Install.Name + "@" + command.Plugin.Install.Marketplace
	}
	if command.Kind == "plugins.configure" {
		next := map[string]map[string]string{}
		for k, values := range e.pluginConfig {
			next[k] = maps.Clone(values)
		}
		values, err := updatePluginConfig(install.Manifest.UserConfig, next[command.ID], command.Plugin.Configuration, command.Plugin.RemoveConfigKeys)
		if err != nil {
			return err
		}
		next[command.ID] = values
		if err := storage.WriteJSON(filepath.Join(e.storage.ConfigDirectory(), "plugin-configuration.json"), next); err != nil {
			return err
		}
		e.pluginConfig = next
		e.applyPluginConfiguredLocked()
		e.publishLocked(contracts.Event{Kind: "plugins.changed"})
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	e.pluginOperations[operationID] = cancel
	e.state.Plugins.Operations = append(e.state.Plugins.Operations, contracts.PluginOperation{ID: operationID, Kind: command.Kind, TargetID: target, State: "running", StartedAt: time.Now()})
	if len(e.state.Plugins.Operations) > 64 {
		e.state.Plugins.Operations = slices.Clone(e.state.Plugins.Operations[len(e.state.Plugins.Operations)-64:])
	}
	e.publishLocked(contracts.Event{Kind: "plugins.changed", RequestID: operationID})
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer cancel()
		var err error
		switch command.Kind {
		case "plugins.source.save":
			err = e.plugins.PutSource(ctx, source)
		case "plugins.source.remove":
			err = e.plugins.RemoveSource(ctx, command.ID)
		case "plugins.source.refresh":
			_, err = e.plugins.Refresh(ctx, command.ID)
		case "plugins.install":
			i := command.Plugin.Install
			_, err = e.plugins.Install(ctx, i.Marketplace, i.Name)
		case "plugins.enable", "plugins.disable":
			err = e.plugins.SetEnabled(ctx, install.ID, command.Kind == "plugins.enable")
		case "plugins.update":
			if _, err = e.plugins.Refresh(ctx, install.SourceID); err == nil {
				_, err = e.plugins.Update(ctx, install.ID)
			}
		case "plugins.uninstall":
			err = e.plugins.Uninstall(ctx, install.ID)
		}
		projection := e.pluginProjection()
		e.mu.Lock()
		defer e.mu.Unlock()
		projection.Operations = e.state.Plugins.Operations
		for i := range projection.Operations {
			op := &projection.Operations[i]
			if op.ID == operationID {
				op.State, op.FinishedAt = "completed", time.Now()
				if err != nil {
					op.State, op.Error = "failed", err.Error()
				}
			}
		}
		e.state.Plugins = projection
		delete(e.pluginOperations, operationID)
		e.applyPluginConfiguredLocked()
		e.publishLocked(contracts.Event{Kind: "plugins.changed", RequestID: operationID})
	}()
	return nil
}

func (e *Engine) applyPluginConfiguredLocked() {
	for i := range e.state.Plugins.Installed {
		item := &e.state.Plugins.Installed[i]
		schema := map[string]plugins.UserConfigOption{}
		for j := range item.Config {
			field := &item.Config[j]
			option := pluginConfigOptionFromField(*field)
			schema[field.Key] = option
			value, exists := e.pluginConfig[item.ID][field.Key]
			field.Saved = exists && value != ""
			field.Configured = field.DefaultConfigured
			if field.Saved {
				_, err := normalizePluginConfigValue(field.Key, option, value)
				field.Configured = err == nil
			}
		}
		redactPluginConfigProjection(item, pluginConfigSecrets(schema, e.pluginConfig[item.ID]))
	}
}
