package desktop

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

type capabilitySettingsState struct {
	plugins                                pluginSettingsState
	queries                                map[string]*string
	mcpOpen, mcpJSONMode                   bool
	mcp                                    contracts.MCPServerInput
	mcpArgs, mcpEnv, mcpHeaders, mcpJSON   string
	mcpEnvKeys, mcpHeaderKeys              []string
	mcpRemoveEnv, mcpRemoveHeaders         []string
	skillOpen                              bool
	skillDetailsOpen, skillImportOpen      bool
	skill                                  contracts.UserSkill
	skillImport                            string
	commandOpen                            bool
	command                                contracts.UserCommand
	hookOpen                               bool
	hook                                   contracts.HookRule
	hookArgs, hookTimeout, hookEvent       string
	hookExec                               bool
	hookHeaders, hookAllowedEnv, hookInput string
	pendingHookSave                        *contracts.HookRule
	pendingSubagentSave                    *contracts.SubagentProfile
}

func (a *App) capabilityToolbar(c *ui.Context, page, add string, action func(), importAction ...func()) string {
	state := &a.capabilityUI
	if state.queries == nil {
		state.queries = make(map[string]*string)
	}
	query := state.queries[page]
	if query == nil {
		query = new(string)
		state.queries[page] = query
	}
	ui.Row(c.Key("resource-toolbar-content-" + page)).FillWidth().Gap(8).Children(func() {
		controlTextInput(c.Key("resource-search-"+page), query).Grow(1).Label(tr(c, "Search "+page)).Placeholder(tr(c, "Search"))
		textButton(c, tr(c, "Refresh")).OnClick(func() { a.dispatch(contracts.Command{Kind: "resource.refresh"}) })
		if len(importAction) > 0 {
			textButton(c, tr(c, "Import")).OnClick(importAction[0])
		}
		primaryButton(c, tr(c, add)).OnClick(action)
	})
	return strings.ToLower(strings.TrimSpace(*query))
}

func matchesResource(query string, parts ...string) bool {
	return query == "" || strings.Contains(strings.ToLower(strings.Join(parts, " ")), query)
}

func (a *App) resourceRow(c *ui.Context, id, title, detail string, enabled bool, edit, toggle, remove func(), extra func()) {
	ui.Row(c.Key(id)).FillWidth().AlignItems(ui.Center).Gap(8).Padding(10, 4).BorderWidth(0, 0, 1, 0).BorderColor(a.palette.Border).Children(func() {
		controlRowButton(c.Key("open")).Grow(1).MinWidth(0).Label(tr(c, "Edit") + " " + title).Children(func() {
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Text(c, title).FontWeight(500)
				if detail != "" {
					ui.Text(c, detail).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
				}
			})
		}).OnClick(edit)
		if extra != nil {
			extra()
		}
		a.controlSwitch(c.Key("enabled"), &enabled, tr(c, "Enable")+" "+title, false, func(bool) {
			toggle()
		})
		textButton(c, tr(c, "Remove")).Label(tr(c, "Remove") + " " + title).OnClick(remove)
	})
}

func (a *App) resourceEditor(c *ui.Context, key string, draw func(), save func()) {
	ui.Column(c.Key(key)).FillWidth().Padding(16).Gap(10).Radius(10).Border(1, a.palette.Border).Children(func() {
		draw()
		ui.Row(c).FillWidth().Justify(ui.End).Gap(8).Children(func() {
			textButton(c, tr(c, "Cancel")).OnClick(func() {
				a.capabilityUI.mcpOpen, a.capabilityUI.skillOpen, a.capabilityUI.commandOpen, a.capabilityUI.hookOpen = false, false, false, false
			})
			primaryButton(c, tr(c, "Save")).OnClick(save)
		})
	})
}

func resourceText(c *ui.Context, key, label string, value *string) {
	ui.Column(c.Key(key + "-field")).FillWidth().Gap(4).Children(func() {
		ui.Text(c, tr(c, label)).FontSize(design.TextSize(c, design.TextBase)).TextColor(c.Theme().TextMuted)
		controlTextInput(c.Key(key), value).FillWidth().Label(tr(c, label))
	})
}

func parseStringMap(value string) (map[string]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var parsed map[string]string
	err := json.Unmarshal([]byte(value), &parsed)
	if err != nil {
		return nil, fmt.Errorf("expected a JSON object containing string values: %w", err)
	}
	if parsed == nil {
		return nil, fmt.Errorf("expected a JSON object containing string values")
	}
	return parsed, nil
}

func parseStringArgs(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var args []string
	if err := json.Unmarshal([]byte(value), &args); err != nil {
		return nil, fmt.Errorf("arguments must be a JSON string array: %w", err)
	}
	if args == nil {
		return nil, fmt.Errorf("arguments must be a JSON string array")
	}
	return args, nil
}

func (a *App) editMCP(server *contracts.MCPServer) {
	s := &a.capabilityUI
	s.mcpOpen, s.mcpJSONMode = true, false
	s.mcp = contracts.MCPServerInput{ID: "mcp-" + rand.Text(), Transport: "stdio", Enabled: true}
	s.mcpArgs, s.mcpEnv, s.mcpHeaders, s.mcpJSON = "[]", "", "", ""
	s.mcpEnvKeys, s.mcpHeaderKeys, s.mcpRemoveEnv, s.mcpRemoveHeaders = nil, nil, nil, nil
	if server != nil {
		s.mcp = contracts.MCPServerInput{ID: server.ID, Name: server.Name, Transport: server.Transport, Command: server.Command, Args: slices.Clone(server.Args), URL: server.URL, Enabled: server.Enabled}
		args, _ := json.Marshal(server.Args)
		if server.Args != nil {
			s.mcpArgs = string(args)
		}
		s.mcpEnvKeys, s.mcpHeaderKeys = slices.Clone(server.EnvKeys), slices.Clone(server.HeaderKeys)
	}
}

func (a *App) mcpDraft() (contracts.MCPServerInput, error) {
	s := &a.capabilityUI
	value := s.mcp
	var err error
	if s.mcpJSONMode {
		decoder := json.NewDecoder(strings.NewReader(s.mcpJSON))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&value); err != nil {
			return value, err
		}
		if err = decoder.Decode(new(any)); err != io.EOF {
			return value, fmt.Errorf("MCP JSON must contain one object")
		}
		if value.ID != s.mcp.ID {
			return value, fmt.Errorf("the server ID cannot be changed")
		}
	} else {
		value.Args, err = parseStringArgs(s.mcpArgs)
		if err != nil {
			return value, err
		}
		value.Env, err = parseStringMap(s.mcpEnv)
		if err != nil {
			return value, err
		}
		value.Headers, err = parseStringMap(s.mcpHeaders)
		if err != nil {
			return value, err
		}
		value.RemoveEnvKeys, value.RemoveHeaderKeys = slices.Clone(s.mcpRemoveEnv), slices.Clone(s.mcpRemoveHeaders)
	}
	value.Name = strings.TrimSpace(value.Name)
	if value.Transport == "http" {
		value.Command, value.Args = "", nil
	} else {
		value.URL = ""
	}
	return value, nil
}

func (a *App) mcpSettings(c *ui.Context) {
	if a.capabilityUI.mcpOpen {
		a.mcpSettingsForm(c)
		return
	}
	query := a.capabilityToolbar(c, "MCP servers", "Add MCP server", func() { a.editMCP(nil) }, func() { a.openSettingsImport("mcp") })
	found := 0
	for _, server := range a.state.UserResources.MCPServers {
		if !matchesResource(query, server.Name, server.URL, server.Command) {
			continue
		}
		found++
		detail := server.Command
		if server.Transport == "http" {
			detail = server.URL
		}
		if server.Test.State != "" {
			detail += " · " + a.t(server.Test.State)
		}
		a.resourceRow(c, "mcp-"+server.ID, server.Name, detail, server.Enabled,
			func() { a.editMCP(&server) },
			func() {
				a.dispatch(contracts.Command{Kind: "resource.mcp.toggle", ID: server.ID, Enabled: !server.Enabled})
			},
			func() { a.confirmSettingsRemoval("mcp", server.ID, server.Name) },
			func() {
				textButton(c, tr(c, "Test")).Label(tr(c, "Test") + " " + server.Name).Disabled(server.Test.State == "testing").OnClick(func() { a.dispatch(contracts.Command{Kind: "resource.mcp.test", ID: server.ID}) })
			})
		if server.Test.Error != "" {
			ui.Text(c, server.Test.Error).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Danger)
		}
		if len(server.Test.Tools) > 0 {
			key := "mcp-tools-" + server.ID
			textButton(c, fmt.Sprintf(a.t("%d tools"), len(server.Test.Tools))).Label(tr(c, "Show tools") + " " + server.Name).OnClick(func() { a.expanded[key] = !a.expanded[key] })
			if a.expanded[key] {
				ui.Column(c.Key(key)).FillWidth().Padding(0, 12, 8, 12).Gap(8).Children(func() {
					for _, tool := range server.Test.Tools {
						ui.Column(c.Key(tool.Name)).FillWidth().Gap(3).Children(func() {
							ui.Text(c, tool.Name).FontSize(design.TextSize(c, design.TextBase)).FontWeight(500)
							if tool.Description != "" {
								ui.Text(c, tool.Description).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
							}
						})
					}
				})
			}
		}
	}
	if found == 0 {
		ui.Text(c, tr(c, "No MCP servers")).TextColor(a.palette.Muted)
	}
}

func (a *App) mcpSettingsForm(c *ui.Context) {
	s := &a.capabilityUI
	title := "Add MCP server"
	if slices.ContainsFunc(a.state.UserResources.MCPServers, func(server contracts.MCPServer) bool { return server.ID == s.mcp.ID }) {
		title = "Edit MCP server"
	}
	a.settingsFormHeader(c, "MCP servers", title, func() { s.mcpOpen = false })
	a.resourceEditor(c, "mcp-editor", func() {
		textButton(c, tr(c, "Edit JSON")).OnClick(func() {
			if !s.mcpJSONMode {
				value, err := a.mcpDraft()
				if err != nil {
					a.errorText = err.Error()
					return
				}
				encoded, _ := json.MarshalIndent(value, "", "  ")
				s.mcpJSON = string(encoded)
			} else {
				value, err := a.mcpDraft()
				if err != nil {
					a.errorText = err.Error()
					return
				}
				s.mcp = value
				args, _ := json.Marshal(value.Args)
				s.mcpArgs = string(args)
				if value.Args == nil {
					s.mcpArgs = "[]"
				}
				env, _ := json.Marshal(value.Env)
				s.mcpEnv = string(env)
				if value.Env == nil {
					s.mcpEnv = ""
				}
				headers, _ := json.Marshal(value.Headers)
				s.mcpHeaders = string(headers)
				if value.Headers == nil {
					s.mcpHeaders = ""
				}
				s.mcpRemoveEnv, s.mcpRemoveHeaders = value.RemoveEnvKeys, value.RemoveHeaderKeys
			}
			s.mcpJSONMode = !s.mcpJSONMode
		})
		if s.mcpJSONMode {
			controlTextArea(c.Key("mcp-json"), &s.mcpJSON).FillWidth().Height(320).Label(tr(c, "MCP JSON"))
			return
		}
		resourceText(c, "mcp-name", "Server name", &s.mcp.Name)
		a.resourceSelect(c, "mcp-transport", "Transport", &s.mcp.Transport, []settingOption{{"stdio", "stdio"}, {"http", "HTTP"}})
		if s.mcp.Transport == "stdio" {
			resourceText(c, "mcp-command", "Executable", &s.mcp.Command)
			resourceText(c, "mcp-args", "Arguments (JSON array)", &s.mcpArgs)
			resourceText(c, "mcp-env", "Environment (JSON object)", &s.mcpEnv)
			a.credentialKeys(c, "env", s.mcpEnvKeys, &s.mcpRemoveEnv)
		} else {
			resourceText(c, "mcp-url", "Server URL", &s.mcp.URL)
			resourceText(c, "mcp-headers", "Headers (JSON object)", &s.mcpHeaders)
			a.credentialKeys(c, "header", s.mcpHeaderKeys, &s.mcpRemoveHeaders)
		}
		ui.Text(c, tr(c, "Saved secrets are not displayed. Empty fields keep saved values; remove keys explicitly.")).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
	}, func() {
		value, err := a.mcpDraft()
		if err != nil {
			a.errorText = err.Error()
			return
		}
		s.mcpOpen = false
		s.mcpEnv, s.mcpHeaders, s.mcpJSON = "", "", ""
		a.dispatch(contracts.Command{Kind: "resource.mcp.save", Resource: &contracts.ResourceInput{MCP: &value}})
	})
}

func (a *App) resourceSelect(c *ui.Context, key, label string, value *string, options []settingOption) {
	sel := controlSelect(c.Key(key), value)
	display := *value
	for _, option := range options {
		if option.value == display {
			display = option.label
			break
		}
	}
	sel.Trigger.FillWidth().Label(tr(c, label)).Children(func() { ui.Text(c, tr(c, display)).Grow(1); icon(c, "alt-arrow-down", 13) })
	sel.Popup(func(panel ui.Element) {
		controlSelectPopup(c, panel).Children(func() {
			for _, option := range options {
				controlSelectItem(c, sel.Item(option.value)).Label(tr(c, label) + " " + option.value).Children(func() { ui.Text(c, tr(c, option.label)) })
			}
		})
	})
}

func (a *App) credentialKeys(c *ui.Context, key string, keys []string, removed *[]string) {
	for _, name := range keys {
		value := slices.Contains(*removed, name)
		controlCheckbox(c.Key(key+"-"+name), &value, tr(c, "Remove saved key")+" "+name).OnClick(func() {
			if value {
				if !slices.Contains(*removed, name) {
					*removed = append(*removed, name)
				}
			} else {
				*removed = slices.DeleteFunc(*removed, func(key string) bool { return key == name })
			}
		})
	}
}

func (a *App) editCommand(command *contracts.UserCommand) {
	s := &a.capabilityUI
	s.commandOpen = true
	s.command = contracts.UserCommand{ID: "command-" + rand.Text(), Enabled: true}
	if command != nil {
		s.command = *command
	}
}

// Reopen only the draft that failed. A late response must not replace a newer
// editor, and credential values are recovered only from the submitted command,
// never from the public snapshot.
func (a *App) capabilityCommandFinished(command contracts.Command, err error) {
	a.customSubagentCommandFinished(command, err)
	s := &a.capabilityUI
	if command.Kind == "settings.update" && command.Settings != nil {
		if pending := s.pendingHookSave; pending != nil {
			for _, rule := range command.Settings.Hooks {
				if reflect.DeepEqual(rule, *pending) {
					s.pendingHookSave = nil
					if err != nil && s.hook.ID == pending.ID {
						s.hookOpen = true
					}
					break
				}
			}
		}
		if pending := s.pendingSubagentSave; pending != nil {
			for _, profile := range command.Settings.Subagents {
				if reflect.DeepEqual(profile, *pending) {
					s.pendingSubagentSave = nil
					if err != nil && a.runtimeUI.subagent.ID == pending.ID {
						a.runtimeUI.subagentOpen = true
					}
					break
				}
			}
		}
	}
	if err == nil || command.Resource == nil {
		return
	}
	switch command.Kind {
	case "resource.mcp.save":
		if command.Resource.MCP != nil && command.Resource.MCP.ID == s.mcp.ID {
			value := *command.Resource.MCP
			s.mcp = value
			s.mcpOpen, s.mcpJSONMode = true, false
			args, _ := json.Marshal(value.Args)
			s.mcpArgs = string(args)
			if value.Args == nil {
				s.mcpArgs = "[]"
			}
			env, _ := json.Marshal(value.Env)
			s.mcpEnv = string(env)
			if value.Env == nil {
				s.mcpEnv = ""
			}
			headers, _ := json.Marshal(value.Headers)
			s.mcpHeaders = string(headers)
			if value.Headers == nil {
				s.mcpHeaders = ""
			}
		}
	case "resource.skill.save":
		if command.Resource.Skill != nil && command.Resource.Skill.ID == s.skill.ID {
			s.skillOpen = true
		}
	case "resource.skill.import":
		if command.Resource.Skill != nil && command.Resource.Skill.ID == s.skill.ID {
			s.skillImportOpen = true
		}
	case "resource.command.save":
		if command.Resource.Command != nil && command.Resource.Command.ID == s.command.ID {
			s.commandOpen = true
		}
	}
}

func (a *App) commandSettings(c *ui.Context) {
	if a.capabilityUI.commandOpen {
		a.commandSettingsForm(c)
		return
	}
	query := a.capabilityToolbar(c, "Commands", "Add command", func() { a.editCommand(nil) }, func() { a.openSettingsImport("command") })
	found := 0
	for _, command := range a.state.UserResources.Commands {
		if !matchesResource(query, command.Name, command.Description) {
			continue
		}
		found++
		a.resourceRow(c, "command-"+command.ID, "/"+command.Name, command.Description, command.Enabled, func() { a.editCommand(&command) }, func() {
			a.dispatch(contracts.Command{Kind: "resource.command.toggle", ID: command.ID, Enabled: !command.Enabled})
		}, func() { a.confirmSettingsRemoval("command", command.ID, "/"+command.Name) }, nil)
	}
	if found == 0 {
		ui.Text(c, tr(c, "No commands")).TextColor(a.palette.Muted)
	}
}

func (a *App) commandSettingsForm(c *ui.Context) {
	s := &a.capabilityUI
	title := "Add command"
	if slices.ContainsFunc(a.state.UserResources.Commands, func(command contracts.UserCommand) bool { return command.ID == s.command.ID }) {
		title = "Edit command"
	}
	a.settingsFormHeader(c, "Commands", title, func() { s.commandOpen = false })
	a.resourceEditor(c, "command-editor", func() {
		resourceText(c, "command-name", "Command name", &s.command.Name)
		resourceText(c, "command-description", "Description", &s.command.Description)
		resourceText(c, "command-argument-hint", "Argument hint", &s.command.ArgumentHint)
		controlTextArea(c.Key("command-content"), &s.command.Content).FillWidth().Height(260).Label(tr(c, "Command prompt")).Placeholder(tr(c, "Command prompt"))
	}, func() {
		value := s.command
		s.commandOpen = false
		a.dispatch(contracts.Command{Kind: "resource.command.save", Resource: &contracts.ResourceInput{Command: &value}})
	})
}
