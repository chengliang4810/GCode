package desktop

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func (a *App) editHook(hook *contracts.HookRule) {
	s := &a.capabilityUI
	s.hookOpen = true
	s.hook = contracts.HookRule{ID: "hook-" + rand.Text(), Event: contracts.HookPreToolUse, Type: "command", Enabled: true, TimeoutSeconds: 30}
	if hook != nil {
		s.hook = contracts.CloneHookRules([]contracts.HookRule{*hook})[0]
	}
	s.hookExec = s.hook.Args != nil
	s.hookEvent = string(s.hook.Event)
	args, _ := json.Marshal(s.hook.Args)
	s.hookArgs = string(args)
	if s.hook.Args == nil {
		s.hookArgs = "[]"
	}
	s.hookTimeout = strconv.FormatFloat(s.hook.TimeoutSeconds, 'g', -1, 64)
	headers, _ := json.Marshal(s.hook.Headers)
	s.hookHeaders = string(headers)
	if s.hook.Headers == nil {
		s.hookHeaders = "{}"
	}
	allowed, _ := json.Marshal(s.hook.AllowedEnvVars)
	s.hookAllowedEnv = string(allowed)
	if s.hook.AllowedEnvVars == nil {
		s.hookAllowedEnv = "[]"
	}
	s.hookInput = string(s.hook.ToolInput)
}

func (a *App) hookSettings(c *ui.Context) {
	if a.capabilityUI.hookOpen {
		a.hookSettingsForm(c)
		return
	}
	query := a.capabilityToolbar(c, "Hooks", "Add hook", func() { a.editHook(nil) })
	found := 0
	for _, hook := range a.state.Settings.Hooks {
		title := hook.Name
		if title == "" {
			title = string(hook.Event)
		}
		if !matchesResource(query, title, string(hook.Event), hook.Command, hook.Matcher) {
			continue
		}
		found++
		detail := string(hook.Event) + " · " + hook.Type
		a.resourceRow(c, "hook-"+hook.ID, title, detail, hook.Enabled, func() { a.editHook(&hook) }, func() {
			a.state.Settings.Hooks = contracts.CloneHookRules(a.state.Settings.Hooks)
			for i := range a.state.Settings.Hooks {
				if a.state.Settings.Hooks[i].ID == hook.ID {
					a.state.Settings.Hooks[i].Enabled = !hook.Enabled
				}
			}
			a.saveSettings()
		}, func() {
			a.confirmSettingsRemoval("hook", hook.ID, title)
		}, nil)
	}
	if found == 0 {
		ui.Text(c, tr(c, "No hooks")).TextColor(a.palette.Muted)
	}
}

func (a *App) hookSettingsForm(c *ui.Context) {
	s := &a.capabilityUI
	title := "Add hook"
	if slices.ContainsFunc(a.state.Settings.Hooks, func(hook contracts.HookRule) bool { return hook.ID == s.hook.ID }) {
		title = "Edit hook"
	}
	a.settingsFormHeader(c, "Hooks", title, func() { s.hookOpen = false })
	a.resourceEditor(c, "hook-editor", func() {
		resourceText(c, "hook-name", "Hook name", &s.hook.Name)
		options := make([]settingOption, 0, len(contracts.HookEvents()))
		for _, candidate := range contracts.HookEvents() {
			options = append(options, settingOption{string(candidate), string(candidate)})
		}
		// Event is copied back by the select's deferred action, not during the
		// build, because input updates after construction.
		sel := controlSelect(c.Key("hook-event"), &s.hookEvent)
		sel.Trigger.FillWidth().Label(tr(c, "Hook event")).OnChange(func() { s.hook.Event = contracts.HookEvent(s.hookEvent) }).Children(func() { ui.Text(c, s.hookEvent).Grow(1); icon(c, "alt-arrow-down", 13) })
		sel.Popup(func(panel ui.Element) {
			controlSelectPopup(c, panel).Children(func() {
				for _, option := range options {
					controlSelectItem(c, sel.Item(option.value)).Label(tr(c, "Hook event") + " " + option.value).Children(func() { ui.Text(c, option.label) })
				}
			})
		})
		resourceText(c, "hook-matcher", "Matcher (regular expression)", &s.hook.Matcher)
		a.resourceSelect(c, "hook-type", "Execution type", &s.hook.Type, []settingOption{
			{"command", "Command"}, {"http", "HTTP"}, {"prompt", "Prompt"}, {"agent", "Agent"}, {"mcp_tool", "MCP tool"},
		})
		switch s.hook.Type {
		case "command":
			controlCheckbox(c.Key("hook-exec"), &s.hookExec, tr(c, "Execute program directly"))
			label := "Shell command"
			if s.hookExec {
				label = "Executable"
			}
			resourceText(c, "hook-command", label, &s.hook.Command)
			if s.hookExec {
				resourceText(c, "hook-args", "Arguments (JSON array)", &s.hookArgs)
			} else {
				a.resourceSelect(c, "hook-shell", "Shell", &s.hook.Shell, []settingOption{{"", "Default"}, {"bash", "Bash"}, {"powershell", "PowerShell"}})
			}
			controlCheckbox(c.Key("hook-async"), &s.hook.Async, tr(c, "Run asynchronously"))
			controlCheckbox(c.Key("hook-async-rewake"), &s.hook.AsyncRewake, tr(c, "Wake agent after asynchronous hook"))
			ui.Text(c, tr(c, "Hooks execute local commands with your user permissions.")).FontSize(design.TextSize(c, design.TextBase)).TextColor(a.palette.Muted)
		case "http":
			resourceText(c, "hook-url", "Hook URL", &s.hook.URL)
			resourceText(c, "hook-headers", "Headers (JSON object)", &s.hookHeaders)
			resourceText(c, "hook-allowed-env", "Allowed environment variables (JSON array)", &s.hookAllowedEnv)
		case "prompt", "agent":
			controlTextArea(c.Key("hook-prompt"), &s.hook.Prompt).FillWidth().Height(120).Label(tr(c, "Hook prompt"))
			resourceText(c, "hook-model", "Model alias", &s.hook.Model)
			controlCheckbox(c.Key("hook-continue-block"), &s.hook.ContinueOnBlock, tr(c, "Continue after block"))
		case "mcp_tool":
			resourceText(c, "hook-mcp-server", "MCP server", &s.hook.MCPServer)
			resourceText(c, "hook-mcp-tool", "MCP tool name", &s.hook.ToolName)
			controlTextArea(c.Key("hook-mcp-input"), &s.hookInput).FillWidth().Height(100).Label(tr(c, "MCP input (JSON)"))
		}
		resourceText(c, "hook-timeout", "Timeout seconds (0 = default)", &s.hookTimeout)
		resourceText(c, "hook-status", "Status message", &s.hook.StatusMessage)
		resourceText(c, "hook-condition", "Condition", &s.hook.If)
		a.resourceSelect(c, "hook-on-failure", "On failure", &s.hook.OnFailure, []settingOption{{"", "Default"}, {"continue", "Continue"}, {"block", "Block"}})
		if s.hook.Source == "skill" {
			controlCheckbox(c.Key("hook-once"), &s.hook.Once, tr(c, "Run once"))
		}
		if !contracts.HookTypeSupported(s.hook.Event, s.hook.Type) {
			ui.Text(c, tr(c, "This event does not support the selected hook type; execution will skip it.")).TextColor(a.palette.Warning)
		}
	}, func() {
		hook, err := a.hookDraft()
		if err != nil {
			a.errorText = err.Error()
			return
		}
		hooks := contracts.CloneHookRules(a.state.Settings.Hooks)
		index := slices.IndexFunc(hooks, func(rule contracts.HookRule) bool { return rule.ID == hook.ID })
		if index < 0 {
			hooks = append(hooks, hook)
		} else {
			hooks[index] = hook
		}
		if err := contracts.ValidateHookRules(hooks); err != nil {
			a.errorText = err.Error()
			return
		}
		a.state.Settings.Hooks = hooks
		s.pendingHookSave = &contracts.CloneHookRules([]contracts.HookRule{hook})[0]
		s.hookOpen = false
		a.saveSettings()
	})
}

func (a *App) hookDraft() (contracts.HookRule, error) {
	s := &a.capabilityUI
	hook := s.hook
	timeout, err := strconv.ParseFloat(strings.TrimSpace(s.hookTimeout), 64)
	if err != nil || timeout < 0 || math.IsNaN(timeout) || math.IsInf(timeout, 0) {
		return hook, errors.New(a.t("Timeout must be a finite nonnegative number."))
	}
	hook.TimeoutSeconds = timeout
	hook.Args = nil
	switch hook.Type {
	case "command":
		if s.hookExec {
			hook.Args, err = parseStringArgs(s.hookArgs)
			if err != nil {
				return hook, err
			}
			if hook.Args == nil {
				hook.Args = []string{}
			}
			hook.Shell = ""
		}
	case "http":
		hook.Headers, err = parseStringMap(s.hookHeaders)
		if err == nil {
			hook.AllowedEnvVars, err = parseStringArgs(s.hookAllowedEnv)
		}
	case "mcp_tool":
		hook.ToolInput = json.RawMessage(strings.TrimSpace(s.hookInput))
	}
	if hook.Type != "command" {
		hook.Command, hook.Shell, hook.Async, hook.AsyncRewake = "", "", false, false
	}
	if hook.Type != "http" {
		hook.URL, hook.Headers, hook.AllowedEnvVars = "", nil, nil
	}
	if hook.Type != "prompt" && hook.Type != "agent" {
		hook.Prompt, hook.Model, hook.ContinueOnBlock = "", "", false
	}
	if hook.Type != "mcp_tool" {
		hook.MCPServer, hook.ToolName, hook.ToolInput = "", "", nil
	}
	return hook, err
}
