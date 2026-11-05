package contracts

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"time"
)

// PluginState is the public host projection. Configuration values and Git
// credentials belong to the host and are never broadcast in snapshots.
type PluginState struct {
	Marketplaces []PluginMarketplace `json:"marketplaces"`
	Catalog      []PluginCatalogItem `json:"catalog"`
	Installed    []InstalledPlugin   `json:"installed"`
	Operations   []PluginOperation   `json:"operations"`
}

type PluginMarketplace struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Source      string    `json:"source"`
	Kind        string    `json:"kind"`
	Ref         string    `json:"ref,omitempty"`
	Builtin     bool      `json:"builtin"`
	State       string    `json:"state"`
	Error       string    `json:"error,omitempty"`
	PluginCount int       `json:"pluginCount"`
	UpdatedAt   time.Time `json:"updatedAt,omitempty"`
}

type PluginCatalogItem struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Marketplace string   `json:"marketplace"`
	Description string   `json:"description"`
	Version     string   `json:"version,omitempty"`
	Author      string   `json:"author,omitempty"`
	Category    string   `json:"category,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
	Components  []string `json:"components,omitempty"`
	Issues      []string `json:"issues,omitempty"`
}

type InstalledPlugin struct {
	ID          string              `json:"id"`
	CatalogID   string              `json:"catalogId"`
	Name        string              `json:"name"`
	Marketplace string              `json:"marketplace"`
	Version     string              `json:"version,omitempty"`
	Commit      string              `json:"commit,omitempty"`
	Enabled     bool                `json:"enabled"`
	Path        string              `json:"path"`
	Components  []string            `json:"components"`
	Issues      []string            `json:"issues"`
	Config      []PluginConfigField `json:"config"`
}

type PluginConfigField struct {
	Key         string   `json:"key"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Required    bool     `json:"required"`
	Sensitive   bool     `json:"sensitive"`
	Multiple    bool     `json:"multiple"`
	Options     []string `json:"options,omitempty"`
	Min         *float64 `json:"min,omitempty"`
	Max         *float64 `json:"max,omitempty"`
	// DefaultConfigured describes a usable default without exposing its value.
	DefaultConfigured bool `json:"defaultConfigured"`
	Saved             bool `json:"saved"`
	Configured        bool `json:"configured"`
}

type PluginOperation struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	TargetID   string    `json:"targetId"`
	State      string    `json:"state"`
	Error      string    `json:"error,omitempty"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
}

type PluginMarketplaceInput struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Source string `json:"source"`
	Kind   string `json:"kind"`
	Ref    string `json:"ref,omitempty"`
}

type PluginInstallInput struct {
	Marketplace string `json:"marketplace"`
	Name        string `json:"name"`
}

type PluginInput struct {
	Marketplace      *PluginMarketplaceInput `json:"marketplace,omitempty"`
	Install          *PluginInstallInput     `json:"install,omitempty"`
	Configuration    map[string]string       `json:"configuration,omitempty"`
	RemoveConfigKeys []string                `json:"removeConfigKeys,omitempty"`
}

func (s PluginState) Clone() PluginState {
	s.Marketplaces = slices.Clone(s.Marketplaces)
	s.Catalog = slices.Clone(s.Catalog)
	for i := range s.Catalog {
		s.Catalog[i].Components = slices.Clone(s.Catalog[i].Components)
		s.Catalog[i].Issues = slices.Clone(s.Catalog[i].Issues)
	}
	s.Installed = slices.Clone(s.Installed)
	for i := range s.Installed {
		s.Installed[i].Components = slices.Clone(s.Installed[i].Components)
		s.Installed[i].Issues = slices.Clone(s.Installed[i].Issues)
		s.Installed[i].Config = slices.Clone(s.Installed[i].Config)
		for j := range s.Installed[i].Config {
			field := &s.Installed[i].Config[j]
			field.Options = slices.Clone(field.Options)
			field.Min = copyPointer(field.Min)
			field.Max = copyPointer(field.Max)
		}
	}
	s.Operations = slices.Clone(s.Operations)
	return s
}

func (p *PluginInput) Clone() *PluginInput {
	if p == nil {
		return nil
	}
	c := *p
	c.Marketplace = copyPointer(p.Marketplace)
	c.Install = copyPointer(p.Install)
	c.Configuration = maps.Clone(p.Configuration)
	c.RemoveConfigKeys = slices.Clone(p.RemoveConfigKeys)
	return &c
}

func (c Command) validatePlugins() error {
	if c.Plugin != nil && !strings.HasPrefix(c.Kind, "plugins.") {
		return errors.New("plugin payload does not match command")
	}
	if !strings.HasPrefix(c.Kind, "plugins.") {
		return nil
	}
	if c.Resource != nil || c.Goal != nil || c.GoalEdit != nil || c.Provider != nil || c.Reorder != nil ||
		c.ProjectCreate != nil || c.Integration != nil || c.Git != nil ||
		c.Adjustment != nil || c.Settings != nil || c.Send != nil ||
		c.Session != nil || c.Attachment != nil || c.TerminalSize != nil {
		return errors.New("plugin operations accept only their typed plugin payload")
	}
	switch c.Kind {
	case "plugins.source.save":
		if c.Plugin == nil || c.Plugin.Marketplace == nil || c.Plugin.Install != nil ||
			c.Plugin.Configuration != nil || strings.TrimSpace(c.Plugin.Marketplace.Source) == "" {
			return errors.New("a marketplace source is required")
		}
	case "plugins.install":
		if c.Plugin == nil || c.Plugin.Install == nil || c.Plugin.Marketplace != nil || c.Plugin.Configuration != nil {
			return errors.New("plugin installation details are required")
		}
		i := c.Plugin.Install
		if strings.TrimSpace(i.Name) == "" || strings.TrimSpace(i.Marketplace) == "" {
			return errors.New("an installation requires plugin and marketplace")
		}
	case "plugins.configure":
		if c.ID == "" || c.Plugin == nil || c.Plugin.Configuration == nil ||
			c.Plugin.Install != nil || c.Plugin.Marketplace != nil {
			return errors.New("a plugin ID and configuration values are required")
		}
	case "plugins.source.remove", "plugins.source.refresh", "plugins.enable", "plugins.disable", "plugins.update", "plugins.uninstall":
		if c.ID == "" || c.Plugin != nil {
			return errors.New("a plugin or source ID without a payload is required")
		}
	default:
		return errors.New("unsupported plugin operation")
	}
	return nil
}
