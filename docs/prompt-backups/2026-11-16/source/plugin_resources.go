package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/mcp"
	"gcode/internal/model"
	"gcode/internal/plugins"
)

type runtimeSkill struct {
	name      string
	component plugins.MarkdownComponent
	bundle    *plugins.Bundle
	data      string
	config    map[string]string
}

type runtimeResources struct {
	guard    ToolGuard
	skills   map[string]runtimeSkill
	profiles []contracts.SubagentProfile
	hooks    []contracts.HookRule
	mcp      []mcp.Config
	warnings []string
	envFile  string
}

type forkSkillScope struct {
	hooks      []contracts.HookRule
	disallowed []string
}

type forkSkillScopeKey struct{}

func pluginDataDirectory(root string, b plugins.Bundle) string {
	key := b.ID
	if b.Installation != nil {
		key = b.Installation.ID
	}
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(root, "plugins", "data", hex.EncodeToString(sum[:16]))
}

func clonePluginConfig(config map[string]map[string]string) map[string]map[string]string {
	result := make(map[string]map[string]string, len(config))
	for k, values := range config {
		result[k] = maps.Clone(values)
	}
	return result
}

func (e *Engine) prepareRuntimeResources(configRoot string, session contracts.Session, bundles []plugins.Bundle, config map[string]map[string]string, profiles []contracts.SubagentProfile) (runtimeResources, error) {
	rootID := session.StorageRootID
	if session.ID == "" {
		rootID = e.storage.ActiveRootID()
	}
	runtimeRoot, err := e.storage.Resolve(rootID)
	if err != nil {
		return runtimeResources{}, fmt.Errorf("plugin runtime storage: %w", err)
	}
	resources, err := buildRuntimeResources(configRoot, runtimeRoot, session.CWD, session.ID, bundles, config, profiles)
	resources.guard = e.toolGuard
	return resources, err
}

func buildRuntimeResources(configRoot, runtimeRoot, cwd, sessionID string, bundles []plugins.Bundle, config map[string]map[string]string, profiles []contracts.SubagentProfile) (runtimeResources, error) {
	r := runtimeResources{skills: map[string]runtimeSkill{}}
	// Runtime storage belongs to the conversation root, independently of the
	// fixed configuration and immutable plugin installation directories.
	root, err := os.OpenRoot(runtimeRoot)
	if err != nil {
		return r, err
	}
	defer root.Close()
	envDir := filepath.Join(runtimeRoot, "plugins", "sessions")
	if err := rejectResourceLinks(envDir); err != nil {
		return r, err
	}
	if err := root.MkdirAll(filepath.Join("plugins", "sessions"), 0700); err != nil {
		return r, err
	}
	digest := sha256.Sum256([]byte(sessionID))
	r.envFile = filepath.Join(envDir, hex.EncodeToString(digest[:16])+".env")
	if err := rejectResourceLinks(r.envFile); err != nil {
		return r, err
	}
	for _, b := range bundles {
		b := b.Clone()
		data := pluginDataDirectory(runtimeRoot, b)
		if err := rejectResourceLinks(data); err != nil {
			return r, err
		}
		relative, err := filepath.Rel(runtimeRoot, data)
		if err != nil {
			return r, err
		}
		if err := root.MkdirAll(relative, 0700); err != nil {
			return r, err
		}
		var saved map[string]string
		if b.Installation != nil {
			saved = config[b.Installation.ID]
		}
		values, configErr := effectivePluginConfig(b.Manifest.UserConfig, saved)
		if configErr != nil {
			return r, fmt.Errorf("%s: %w", b.ID, configErr)
		}
		configSecrets := pluginConfigSecrets(b.Manifest.UserConfig, values)
		for _, warning := range bundleComponentWarnings(b) {
			r.warnings = append(r.warnings, redactPluginConfigText(b.ID+": "+warning, configSecrets))
		}
		for key, option := range b.Manifest.UserConfig {
			if option.Required && values[key] == "" {
				r.warnings = append(r.warnings, b.ID+": configuration required: "+key)
			}
		}
		for _, c := range append(slices.Clone(b.Commands), b.Skills...) {
			name := c.Name
			r.skills[name] = runtimeSkill{name: name, component: c, bundle: &b, data: data, config: values}
		}
		for _, c := range b.Agents {
			name := c.Name
			text, err := expandPluginText(c.Content, b, data, c.Path, sessionID, values, false)
			if err != nil {
				r.warnings = append(r.warnings, name+": "+err.Error())
				continue
			}
			r.profiles = append(r.profiles, contracts.SubagentProfile{
				ID: name, Name: name, Description: redactPluginConfigText(c.Metadata.Description, configSecrets), Prompt: redactPluginConfigText(text, configSecrets),
				Tools:           frontmatterTools(c.Frontmatter["tools"]),
				DisallowedTools: frontmatterTools(c.Frontmatter["disallowedTools"]),
			})
		}
		r.hooks = append(r.hooks, componentHooks(b.ID, b.Hooks, &b, data, values, "plugin")...)
		for name, server := range b.MCPServers {
			expand := func(text string) string {
				// Expand the original template once. Configuration and environment
				// values are opaque, including any dollar signs they contain.
				return os.Expand(text, func(key string) string {
					switch key {
					case "CLAUDE_PLUGIN_ROOT":
						return b.Root
					case "CLAUDE_PLUGIN_DATA":
						return data
					case "CLAUDE_SESSION_ID":
						return sessionID
					}
					if v, ok := strings.CutPrefix(key, "user_config."); ok {
						return values[v]
					}
					parts := strings.SplitN(key, ":-", 2)
					value := os.Getenv(parts[0])
					if value == "" && len(parts) > 1 {
						return parts[1]
					}
					return value
				})
			}
			c := mcp.Config{SensitiveValues: pluginConfigSecrets(b.Manifest.UserConfig, values), ID: b.ID + ":" + name, Name: name, Plugin: b.Manifest.Name, Transport: server.Type, Command: expand(server.Command), URL: expand(server.URL), Args: slices.Clone(server.Args), Env: map[string]string{}, Headers: map[string]string{}}
			for i := range c.Args {
				c.Args[i] = expand(c.Args[i])
			}
			for k, v := range server.Env {
				c.Env[k] = expand(v)
			}
			for k, v := range server.Headers {
				c.Headers[k] = expand(v)
			}
			r.mcp = append(r.mcp, c)
			if server.HeadersHelper != "" {
				r.warnings = append(r.warnings, b.ID+"/"+name+": headersHelper is not executed")
			}
		}
		for _, issue := range b.Unsupported {
			r.warnings = append(r.warnings, b.ID+": "+issue.Component+": "+issue.Message)
		}
	}
	user, err := readResourceCatalog(configRoot)
	if err != nil {
		return r, err
	}
	for _, s := range user.Skills {
		if !s.Enabled {
			continue
		}
		path := filepath.Join(configRoot, "skills", s.ID, "SKILL.md")
		c, err := plugins.ParseMarkdownText(s.Content, path)
		if err != nil {
			return r, err
		}
		if c.Metadata.Description == "" {
			c.Metadata.Description = s.Description
		}
		r.skills[s.ID] = runtimeSkill{name: s.ID, component: c}
		// The app's explicit skill invocation keeps a separate resource kind.
		r.skills["skill:"+s.ID] = runtimeSkill{name: "skill:" + s.ID, component: c}
	}
	for _, c := range user.Commands {
		if !c.Enabled {
			continue
		}
		component, err := plugins.ParseMarkdownText(c.Content, filepath.Join(configRoot, "commands", c.Name+".md"))
		if err != nil {
			return r, err
		}
		if component.Metadata.Description == "" {
			component.Metadata.Description = c.Description
		}
		r.skills[c.Name] = runtimeSkill{name: c.Name, component: component}
	}
	// Project resources override personal and plugin resources by their observable name.
	for _, kind := range []string{"skills", "commands", "agents"} {
		directory := filepath.Join(cwd, ".claude", kind)
		err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return errors.New("project resource symbolic links are not allowed")
			}
			if entry.IsDir() {
				return nil
			}
			if kind == "skills" && entry.Name() != "SKILL.md" || kind != "skills" && !strings.HasSuffix(entry.Name(), ".md") {
				return nil
			}
			c, err := plugins.ParseMarkdown(path)
			if err != nil {
				return err
			}
			relative, _ := filepath.Rel(directory, path)
			pathIdentity := strings.TrimSuffix(filepath.ToSlash(relative), ".md")
			if kind == "skills" {
				pathIdentity = filepath.Base(filepath.Dir(relative))
			}
			c.Name = plugins.ComponentName(kind, "", pathIdentity, c.Metadata.Name)
			if kind == "agents" {
				r.profiles = append(r.profiles, contracts.SubagentProfile{ID: c.Name, Name: c.Name, Description: c.Metadata.Description, Prompt: c.Content, Tools: frontmatterTools(c.Frontmatter["tools"]), DisallowedTools: frontmatterTools(c.Frontmatter["disallowedTools"])})
			} else {
				r.skills[c.Name] = runtimeSkill{name: c.Name, component: c}
			}
			return nil
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return r, err
		}
	}
	// Built-ins, plugin/project definitions, then explicit user profiles.
	all := contracts.DefaultSubagentProfiles()
	for _, p := range append(r.profiles, profiles...) {
		index := slices.IndexFunc(all, func(v contracts.SubagentProfile) bool { return v.ID == p.ID })
		if index < 0 {
			all = append(all, p)
		} else {
			all[index] = p
		}
	}
	r.profiles = all
	return r, nil
}

func frontmatterTools(value any) []string {
	var result []string
	switch value := value.(type) {
	case string:
		start, depth := 0, 0
		for i, ch := range value {
			if ch == '(' {
				depth++
			}
			if ch == ')' {
				depth--
			}
			if depth == 0 && (ch == ',' || unicode.IsSpace(ch)) {
				if tool := strings.TrimSpace(value[start:i]); tool != "" {
					result = append(result, tool)
				}
				start = i + 1
			}
		}
		if tool := strings.TrimSpace(value[start:]); tool != "" {
			result = append(result, tool)
		}
	case []any:
		for _, tool := range value {
			if text, ok := tool.(string); ok {
				result = append(result, text)
			}
		}
	case []string:
		result = slices.Clone(value)
	}
	return result
}

func componentHooks(prefix string, hooks map[string][]plugins.HookMatcher, bundle *plugins.Bundle, data string, config map[string]string, source string) []contracts.HookRule {
	var rules []contracts.HookRule
	for _, event := range slices.Sorted(maps.Keys(hooks)) {
		for group, match := range hooks[event] {
			for index, handler := range match.Hooks {
				input, _ := json.Marshal(handler.Input)
				if handler.Input == nil {
					input = nil
				}
				rule := contracts.HookRule{
					ID: fmt.Sprintf("%s/%s/%d/%d", prefix, event, group, index), Name: prefix,
					Event: contracts.HookEvent(event), Matcher: match.Matcher, Type: handler.Type,
					Command: handler.Command, Args: slices.Clone(handler.Args), Shell: handler.Shell,
					Async: handler.Async, AsyncRewake: handler.AsyncRewake, TimeoutSeconds: handler.Timeout,
					StatusMessage: handler.StatusMessage, Once: handler.Once, If: handler.If, OnFailure: handler.OnFailure,
					URL: handler.URL, Headers: maps.Clone(handler.Headers), AllowedEnvVars: slices.Clone(handler.AllowedEnvVars),
					Prompt: handler.Prompt, Model: handler.Model, MCPServer: handler.Server,
					ToolName: handler.Tool, ToolInput: input, Source: source, UserConfig: maps.Clone(config), Enabled: true,
				}
				if bundle != nil {
					rule.PluginRoot, rule.PluginData, rule.PluginID = bundle.Root, data, bundle.ID
					for key, option := range bundle.Manifest.UserConfig {
						if option.Sensitive {
							rule.SensitiveConfigKeys = append(rule.SensitiveConfigKeys, key)
						}
					}
					slices.Sort(rule.SensitiveConfigKeys)
				}
				if bundle != nil {
					if _, exists := bundle.MCPServers[handler.Server]; exists {
						rule.MCPServer = "plugin_" + bundle.Manifest.Name + "_" + handler.Server
					}
				}
				rules = append(rules, rule)
			}
		}
	}
	return rules
}

func expandPluginText(text string, b plugins.Bundle, data, skillPath, sessionID string, config map[string]string, secrets bool) (string, error) {
	replacements := []string{"${CLAUDE_PLUGIN_ROOT}", b.Root, "${CLAUDE_PLUGIN_DATA}", data, "${CLAUDE_SESSION_ID}", sessionID}
	if skillPath != "" {
		replacements = append(replacements, "${CLAUDE_SKILL_DIR}", filepath.Dir(skillPath))
	}
	for key, value := range config {
		token := "${user_config." + key + "}"
		if b.Manifest.UserConfig[key].Sensitive && !secrets && strings.Contains(text, token) {
			return "", fmt.Errorf("sensitive plugin configuration %q cannot enter model content", key)
		}
		replacements = append(replacements, token, value)
	}
	return strings.NewReplacer(replacements...).Replace(text), nil
}

var invocationArgument = regexp.MustCompile(`\\?\$(?:ARGUMENTS(?:\[[0-9]+\])?|[0-9]+|[A-Za-z_][A-Za-z0-9_]*)`)

func parseInvocationArguments(text string) ([]string, error) {
	var result []string
	var current strings.Builder
	var quote rune
	escaped, started := false, false
	for _, ch := range text {
		if escaped {
			current.WriteRune(ch)
			escaped = false
			started = true
			continue
		}
		if ch == '\\' && quote != '\'' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
			} else {
				current.WriteRune(ch)
			}
			started = true
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			started = true
			continue
		}
		if unicode.IsSpace(ch) {
			if started {
				result = append(result, current.String())
				current.Reset()
				started = false
			}
		} else {
			current.WriteRune(ch)
			started = true
		}
	}
	if quote != 0 {
		return nil, errors.New("unterminated argument quote")
	}
	if escaped {
		current.WriteRune('\\')
	}
	if started {
		result = append(result, current.String())
	}
	return result, nil
}

func substituteInvocation(text, arguments string, names []string) (string, error) {
	var values []string
	var parseErr error
	parsed := false
	parseValues := func() bool {
		if !parsed {
			values, parseErr = parseInvocationArguments(arguments)
			parsed = true
		}
		return parseErr == nil
	}
	received := false
	text = invocationArgument.ReplaceAllStringFunc(text, func(token string) string {
		if strings.HasPrefix(token, `\`) {
			return token[1:]
		}
		name := strings.TrimPrefix(token, "$")
		if name == "ARGUMENTS" {
			received = true
			return arguments
		}
		if strings.HasPrefix(name, "ARGUMENTS[") {
			name = strings.TrimSuffix(strings.TrimPrefix(name, "ARGUMENTS["), "]")
		}
		if index, err := strconv.Atoi(name); err == nil {
			if !parseValues() {
				return token
			}
			if index < len(values) {
				received = true
				return values[index]
			}
			return token
		}
		for index, key := range names {
			if key == name {
				if !parseValues() {
					return token
				}
				received = true
				if index < len(values) {
					return values[index]
				}
				return ""
			}
		}
		return token
	})
	if parseErr != nil {
		return "", parseErr
	}
	if !received && strings.TrimSpace(arguments) != "" {
		text += "\n\nARGUMENTS: " + arguments
	}
	return text, nil
}

var inlineDynamic = regexp.MustCompile("(?m)(^|[\\t ])!`([^`\\n]+)`")
var fencedDynamic = regexp.MustCompile("(?ms)^```!\\s*\\n(.*?)\\n```")

func (s runtimeSkill) redactor() func(string) string {
	var variants []string
	if s.bundle != nil {
		variants = pluginConfigSecrets(s.bundle.Manifest.UserConfig, s.config)
	}
	return func(text string) string {
		return redactPluginConfigText(text, variants)
	}
}

func (r runtimeResources) expand(ctx context.Context, skill runtimeSkill, arguments, cwd, sessionID string, invoke func(context.Context, agent.HookInput) (agent.HookResult, error), policies ...func(string) bool) (string, error) {
	text, err := substituteInvocation(skill.component.Content, arguments, skill.component.Metadata.Arguments)
	if err != nil {
		return "", err
	}
	env := map[string]string{"CLAUDE_SESSION_ID": sessionID, "CLAUDE_SKILL_DIR": filepath.Dir(skill.component.Path), "CLAUDE_PROJECT_DIR": cwd, "RCODE_SESSION_ENV_FILE": r.envFile}
	if skill.bundle != nil {
		env["CLAUDE_PLUGIN_ROOT"], env["CLAUDE_PLUGIN_DATA"] = skill.bundle.Root, skill.data
	}
	execute := func(script string) (string, error) {
		for _, allowed := range policies {
			if allowed != nil && !allowed("Bash") {
				return "", errors.New("Bash is disabled by the active execution policy")
			}
		}
		if deniedSubagentTool(skill.component.Metadata.DisallowedTools, "Bash") {
			return "", errors.New("Bash is disabled by the active skill")
		}
		shell := skill.component.Metadata.Shell
		if shell == "" {
			shell = "bash"
		}
		for key, value := range skill.config {
			sum := sha256.Sum256([]byte(key))
			envKey := "RCODE_PLUGIN_OPTION_" + strings.ToUpper(hex.EncodeToString(sum[:8]))
			env[envKey] = value
			reference := "${" + envKey + "}"
			if shell == "powershell" {
				reference = "${env:" + envKey + "}"
			}
			script = strings.ReplaceAll(script, "${user_config."+key+"}", reference)
		}
		if shell == "powershell" {
			delete(env, "RCODE_SESSION_ENV_FILE")
			for _, name := range []string{"CLAUDE_SESSION_ID", "CLAUDE_SKILL_DIR", "CLAUDE_PROJECT_DIR", "CLAUDE_PLUGIN_ROOT", "CLAUDE_PLUGIN_DATA"} {
				script = strings.ReplaceAll(script, "${"+name+"}", "${env:"+name+"}")
			}
		}
		tool := dynamicContextTool{shell: shell, env: env, redact: skill.redactor(), guard: r.guard}
		raw, _ := json.Marshal(map[string]any{"command": script})
		output, err := agent.ExecuteHookedTool(ctx, tool, cwd, raw, invoke, id("dynamic-context"))
		return output.Text, err
	}
	for _, pattern := range []*regexp.Regexp{fencedDynamic, inlineDynamic} {
		var expandErr error
		text = pattern.ReplaceAllStringFunc(text, func(match string) string {
			if expandErr != nil {
				return match
			}
			parts := pattern.FindStringSubmatch(match)
			script, prefix := parts[1], ""
			if pattern == inlineDynamic {
				script, prefix = parts[2], parts[1]
			}
			value, err := execute(script)
			if err != nil {
				expandErr = err
				return match
			}
			return prefix + strings.TrimSpace(value)
		})
		if expandErr != nil {
			return "", expandErr
		}
	}
	if skill.bundle != nil {
		expanded, err := expandPluginText(text, *skill.bundle, skill.data, skill.component.Path, sessionID, skill.config, false)
		return skill.redactor()(expanded), err
	}
	return strings.NewReplacer("${CLAUDE_SKILL_DIR}", filepath.Dir(skill.component.Path), "${CLAUDE_SESSION_ID}", sessionID).Replace(text), nil
}

type dynamicContextTool struct {
	guard  ToolGuard
	shell  string
	env    map[string]string
	redact func(string) string
}

func (t dynamicContextTool) Definition() model.Tool { return model.Tool{Name: "Bash"} }
func (t dynamicContextTool) Concurrent() bool       { return false }
func (t dynamicContextTool) Execute(ctx context.Context, cwd string, raw json.RawMessage) (model.ToolOutput, error) {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return model.ToolOutput{}, err
	}
	if t.guard != nil {
		if err := t.guard(ctx, cwd, "Bash", slices.Clone(raw)); err != nil {
			return model.ToolOutput{}, err
		}
	}
	output, err := agent.RunShell(ctx, cwd, t.shell, args.Command, t.env, 120*time.Second, nil)
	if t.redact != nil {
		output.Text = t.redact(output.Text)
		if data, ok := output.Data.(map[string]any); ok {
			cloned := maps.Clone(data)
			for key, value := range cloned {
				if text, ok := value.(string); ok {
					cloned[key] = t.redact(text)
				}
			}
			output.Data = cloned
		}
		if err != nil {
			err = errors.New(t.redact(err.Error()))
		}
	}
	return output, err
}

func (r runtimeResources) instructions() string {
	var out strings.Builder
	out.WriteString("\n\nSkills are invoked through the Skill tool using their exact names. Read supporting files relative to each skill directory. Model role aliases inherit the effective execution model; user-selected subagent models take precedence.")
	for _, name := range slices.Sorted(maps.Keys(r.skills)) {
		s := r.skills[name]
		if s.component.Metadata.DisableModelInvocation || strings.HasPrefix(name, "skill:") {
			continue
		}
		fmt.Fprintf(&out, "\n- %s: %s", name, s.redactor()(s.component.Metadata.Description))
		if out.Len() > 32000 {
			break
		}
	}
	for _, warning := range r.warnings {
		out.WriteString("\nPlugin diagnostic: " + warning)
	}
	return out.String()
}

func (e *Engine) runtimeSkillTool(session contracts.Session, run *activeRun, r runtimeResources, invoke func(context.Context, agent.HookInput) (agent.HookResult, error), activate func(runtimeSkill) (func(), error)) agent.Tool {
	return delegationTool{
		definition: model.Tool{
			Name: "Skill", Description: "Invoke an available skill by its exact name, including plugin:name. Skills provide instructions and may run dynamic context or a separate subagent.",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{
				"skill": map[string]any{"type": "string"}, "args": map[string]any{"type": "string"},
			}, "required": []string{"skill"}, "additionalProperties": false},
		},
		execute: func(ctx context.Context, raw json.RawMessage) (model.ToolOutput, error) {
			var input struct {
				Skill string `json:"skill"`
				Args  string `json:"args"`
			}
			decoder := json.NewDecoder(strings.NewReader(string(raw)))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil {
				return model.ToolOutput{}, err
			}
			s, exists := r.skills[input.Skill]
			if !exists {
				return model.ToolOutput{}, errors.New("skill not found: " + input.Skill)
			}
			if s.component.Metadata.DisableModelInvocation {
				return model.ToolOutput{}, errors.New("skill is only available through an explicit user invocation")
			}
			return e.executeRuntimeSkill(ctx, session, run, r, s, input.Args, invoke, activate)
		},
	}
}

func (e *Engine) executeRuntimeSkill(ctx context.Context, session contracts.Session, run *activeRun, r runtimeResources, skill runtimeSkill, arguments string, invoke func(context.Context, agent.HookInput) (agent.HookResult, error), activate func(runtimeSkill) (func(), error)) (model.ToolOutput, error) {
	if activate != nil {
		cleanup, err := activate(skill)
		if err != nil {
			return model.ToolOutput{}, err
		}
		if cleanup != nil {
			defer cleanup()
		}
	}
	var allowed func(string) bool
	if run != nil {
		allowed = run.toolAllowed
	}
	text, err := r.expand(ctx, skill, arguments, session.CWD, session.ID, invoke, allowed)
	if err != nil {
		return model.ToolOutput{}, err
	}
	if skill.component.Metadata.Context == "fork" {
		scope := forkSkillScope{
			hooks:      componentHooks(skill.name, skill.component.Metadata.Hooks, skill.bundle, skill.data, skill.config, "skill"),
			disallowed: slices.Clone(skill.component.Metadata.DisallowedTools),
		}
		// A fork's Stop hooks govern the child completion boundary.
		for i := range scope.hooks {
			if scope.hooks[i].Event == contracts.HookStop {
				scope.hooks[i].Event = contracts.HookSubagentStop
			}
		}
		ctx = context.WithValue(ctx, forkSkillScopeKey{}, scope)
		agentType := skill.component.Metadata.Agent
		if agentType == "" {
			agentType = "general-purpose"
		} else if skill.bundle != nil && !strings.Contains(agentType, ":") {
			qualified := skill.bundle.Manifest.Name + ":" + agentType
			for _, profile := range r.profiles {
				if profile.ID == qualified {
					agentType = qualified
					break
				}
			}
		}
		tool := e.guardTool(e.delegationTool(session.ID, run, contracts.EnabledSubagentProfiles(r.profiles)))
		input, _ := json.Marshal(map[string]any{"description": "Skill " + skill.name, "prompt": text, "subagent_type": agentType})
		output, err := tool.Execute(ctx, session.CWD, input)
		if err == nil {
			e.skillInvoked(session.ID, run.id, skill)
		}
		return output, err
	}
	e.skillInvoked(session.ID, run.id, skill)
	return model.ToolOutput{Text: "Skill " + skill.name + "\nBase directory: " + filepath.Dir(skill.component.Path) + "\n\n" + text}, nil
}

func (e *Engine) skillInvoked(sessionID, runID string, skill runtimeSkill) {
	event := contracts.Event{Kind: "skill.invoked", SessionID: sessionID, RunID: runID, ResourceID: skill.name, ComponentID: skill.name}
	if skill.bundle != nil {
		event.PluginID = skill.bundle.ID
	}
	e.mu.Lock()
	e.publishLocked(event)
	e.mu.Unlock()
}

func (e *Engine) prepareRuntimeInput(ctx context.Context, message model.Message, session contracts.Session, run *activeRun, r runtimeResources, invoke func(context.Context, agent.HookInput) (agent.HookResult, error), activate func(runtimeSkill) (func(), error)) (model.Message, error) {
	raw := message.SourceText
	if raw == "" {
		raw = message.Text
	}
	raw = strings.TrimSpace(contracts.PlainPrompt(raw))
	fields := strings.Fields(raw)
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return message, nil
	}
	name := strings.TrimPrefix(fields[0], "/")
	s, exists := r.skills[name]
	if !exists {
		if strings.Contains(name, ":") {
			return message, errors.New("skill or command not found: " + name)
		}
		return message, nil
	}
	if !s.component.Metadata.UserInvocable {
		return message, errors.New("skill cannot be invoked as a user command: " + name)
	}
	arguments := strings.TrimSpace(strings.TrimPrefix(raw, fields[0]))
	if invoke != nil {
		result, err := invoke(ctx, agent.HookInput{Event: contracts.HookUserPromptExpansion, Prompt: raw, Source: name})
		if err != nil {
			return message, err
		}
		if result.Deny || result.Continue != nil && !*result.Continue {
			return message, errors.New(firstRuntimeText(result.Reason, "command expansion blocked"))
		}
	}
	if s.component.Metadata.Context == "fork" {
		// An explicit command forks before the first model step. Record an
		// actual Skill invocation so the child has a stable parent activity.
		e.mu.Lock()
		toolID := id("skill")
		args, _ := json.Marshal(map[string]any{"skill": name, "args": arguments})
		e.state.Messages[session.ID] = append(e.state.Messages[session.ID], contracts.Message{
			ID: id("message"), SessionID: session.ID, TurnID: run.id, Role: "assistant", CreatedAt: time.Now(),
			Blocks: []contracts.Block{{ID: toolID, Kind: "tool", Tool: &contracts.ToolCall{
				ID: toolID, Name: "Skill", Kind: contracts.ToolSkill, Summary: name, Command: arguments,
				Args: string(args), Status: "running",
			}}},
		})
		e.publishLocked(contracts.Event{Kind: "message.changed", SessionID: session.ID, RunID: run.id})
		e.mu.Unlock()
	}
	output, err := e.executeRuntimeSkill(ctx, session, run, r, s, arguments, invoke, activate)
	if err != nil {
		return message, err
	}
	message.Text = output.Text
	return message, nil
}

func firstRuntimeText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
