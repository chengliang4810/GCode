package plugins

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

const maxDefinitionSize = 8 << 20

var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// Load discovers a plugin without running shell commands, hooks, or servers.
func Load(root string) (Bundle, error) {
	return LoadWithManifest(root, Manifest{}, Source{})
}

// LoadWithManifest uses marketplace fields when plugin.json is absent, and supplements
// component declarations when it is present. The marketplace's strict=false conflict
// rule is checked by Manager.Install.
func LoadWithManifest(root string, fallback Manifest, source Source) (Bundle, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Bundle{}, err
	}
	if err := validatePluginLinks(root); err != nil {
		return Bundle{}, err
	}
	manifest := fallback
	path := filepath.Join(root, ".claude-plugin", "plugin.json")
	data, err := readPluginDefinition(root, path)
	hasManifest := err == nil
	if err == nil {
		manifest = Manifest{}
		if err := json.Unmarshal(data, &manifest); err != nil {
			return Bundle{}, fmt.Errorf("plugin manifest: %w", err)
		}
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(data, &fields)
		known := map[string]bool{}
		for _, k := range strings.Fields("$schema name displayName version description author homepage repository license keywords defaultEnabled dependencies commands skills agents hooks mcpServers lspServers outputStyles settings userConfig channels metadata experimental") {
			known[k] = true
		}
		manifest.Unknown = map[string]json.RawMessage{}
		for k, v := range fields {
			if !known[k] {
				manifest.Unknown[k] = v
			}
		}
		manifest = supplementManifest(manifest, fallback)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Bundle{}, err
	}
	if manifest.Name == "" {
		manifest.Name = filepath.Base(root)
	}
	if !validName.MatchString(manifest.Name) {
		return Bundle{}, fmt.Errorf("invalid plugin name %q", manifest.Name)
	}
	b := Bundle{
		ID: manifest.Name, Root: root, Manifest: manifest,
		Hooks: map[string][]HookMatcher{}, MCPServers: map[string]MCPServer{},
		LSPServers: map[string]LSPServer{}, Settings: map[string]any{},
	}
	for key := range manifest.Unknown {
		b.Unsupported = append(b.Unsupported, Issue{Component: key, Message: "unrecognized manifest field"})
	}
	// Skills add to the default scan; commands, agents, and output styles replace it.
	for _, spec := range []struct {
		kind string
		raw  json.RawMessage
		add  bool
		dst  *[]MarkdownComponent
	}{
		{"skills", manifest.Skills, !(source.Relative != "" && filepath.Clean(filepath.FromSlash(source.Relative)) == "." && len(fallback.Skills) > 0), &b.Skills},
		{"commands", manifest.Commands, false, &b.Commands},
		{"agents", manifest.Agents, false, &b.Agents},
		{"output-styles", manifest.OutputStyles, false, &b.OutputStyles},
	} {
		if err := b.loadMarkdown(spec.kind, spec.raw, spec.add, spec.dst); err != nil {
			return Bundle{}, err
		}
	}
	hookSpec := manifest.Hooks
	if !hasManifest && len(hookSpec) > 0 && bytes.TrimSpace(hookSpec)[0] != '{' {
		b.Unsupported = append(b.Unsupported, Issue{Component: "hooks", Message: "marketplace entry hook paths and arrays are not supported"})
		hookSpec = nil
	}
	if err := b.loadConfig("hooks", hookSpec, "hooks/hooks.json"); err != nil {
		return Bundle{}, err
	}
	if hasManifest && len(fallback.Hooks) > 0 {
		if bytes.TrimSpace(fallback.Hooks)[0] != '{' {
			b.Unsupported = append(b.Unsupported, Issue{Component: "hooks", Message: "marketplace entry hook paths and arrays are not supported"})
		} else {
			additional := Bundle{Root: b.Root, Hooks: map[string][]HookMatcher{}}
			if err := additional.loadConfig("hooks", fallback.Hooks, ""); err != nil {
				return Bundle{}, err
			}
			for event, matchers := range additional.Hooks {
				b.Hooks[event] = matchers
			}
			b.Unsupported = append(b.Unsupported, additional.Unsupported...)
		}
	}
	if err := b.loadConfig("mcpServers", manifest.MCPServers, ".mcp.json"); err != nil {
		return Bundle{}, err
	}
	if err := b.loadConfig("lspServers", manifest.LSPServers, ".lsp.json"); err != nil {
		return Bundle{}, err
	}
	settings := manifest.Settings
	if data, err := readPluginDefinition(root, filepath.Join(root, "settings.json")); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return Bundle{}, fmt.Errorf("plugin settings: %w", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Bundle{}, err
	}
	for k, v := range settings {
		if k == "agent" || k == "subagentStatusLine" {
			b.Settings[k] = v
		} else {
			b.Unsupported = append(b.Unsupported, Issue{Component: "settings", Path: k, Message: "setting is not allowed for plugins"})
		}
	}
	for key := range manifest.Experimental {
		b.Unsupported = append(b.Unsupported, Issue{Component: "experimental." + key, Message: "component retained but not executed"})
	}
	if len(manifest.Channels) != 0 {
		b.Unsupported = append(b.Unsupported, Issue{Component: "channels", Message: "channel notifications require host support"})
	}
	slices.SortFunc(b.Unsupported, func(a, c Issue) int { return strings.Compare(a.Component+a.Path, c.Component+c.Path) })
	return b, nil
}

func supplementManifest(m, entry Manifest) Manifest {
	if len(m.Commands) != 0 && len(entry.Commands) != 0 {
		var original, extra map[string]json.RawMessage
		if json.Unmarshal(m.Commands, &original) == nil && json.Unmarshal(entry.Commands, &extra) == nil {
			for key, value := range extra {
				original[key] = value
			}
			m.Commands, _ = json.Marshal(original)
			entry.Commands = nil
		}
	}
	// Metadata from plugin.json is authoritative. Additional component declarations merge.
	for _, field := range []struct {
		dst   *json.RawMessage
		extra json.RawMessage
	}{
		{&m.Commands, entry.Commands}, {&m.Skills, entry.Skills}, {&m.Agents, entry.Agents},
		{&m.MCPServers, entry.MCPServers},
		{&m.LSPServers, entry.LSPServers}, {&m.OutputStyles, entry.OutputStyles},
	} {
		if len(field.extra) == 0 {
			continue
		}
		if len(*field.dst) == 0 {
			*field.dst = field.extra
			continue
		}
		var a, c any
		_ = json.Unmarshal(*field.dst, &a)
		_ = json.Unmarshal(field.extra, &c)
		as, ok := a.([]any)
		if !ok {
			as = []any{a}
		}
		cs, ok := c.([]any)
		if !ok {
			cs = []any{c}
		}
		*field.dst, _ = json.Marshal(append(as, cs...))
	}
	return m
}

// LoadInstallation restores source-dependent marketplace component semantics.
func LoadInstallation(installation Installation) (Bundle, error) {
	bundle, err := LoadWithManifest(installation.Path, installation.EntryManifest, installation.Source)
	if err != nil {
		return Bundle{}, err
	}
	bundle.ID, bundle.Origin = installation.ID, installation.Marketplace
	copy := copyValue(installation)
	bundle.Installation = &copy
	return bundle, nil
}

func decodePaths(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	var path string
	if json.Unmarshal(raw, &path) == nil {
		return []string{path}, nil
	}
	var paths []string
	if err := json.Unmarshal(raw, &paths); err != nil {
		return nil, fmt.Errorf("expected a path or array of paths: %w", err)
	}
	return paths, nil
}

func (b *Bundle) loadMarkdown(kind string, raw json.RawMessage, add bool, dst *[]MarkdownComponent) error {
	seen := map[string]bool{}
	loadPath := func(relative string, required bool) error {
		path, err := contained(b.Root, relative)
		if err != nil {
			return err
		}
		info, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) && !required {
			return nil
		}
		if err != nil {
			return err
		}
		if kind == "agents" && required && info.IsDir() {
			return fmt.Errorf("agents declaration must name a Markdown file: %s", relative)
		}
		var files []string
		if !info.IsDir() {
			files = []string{path}
		} else if kind == "skills" {
			if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err == nil {
				files = []string{filepath.Join(path, "SKILL.md")}
			} else {
				entries, err := os.ReadDir(path)
				if err != nil {
					return err
				}
				for _, entry := range entries {
					entryPath := filepath.Join(path, entry.Name())
					resolved, err := resolvePluginPath(b.Root, entryPath, false)
					if err != nil {
						return err
					}
					entryInfo, err := os.Stat(resolved)
					if err != nil {
						return err
					}
					if entryInfo.IsDir() {
						candidate := filepath.Join(path, entry.Name(), "SKILL.md")
						if _, err := os.Stat(candidate); err == nil {
							files = append(files, candidate)
						}
					}
				}
			}
		} else {
			err := walkPluginFiles(b.Root, path, func(p string, info fs.FileInfo) error {
				if strings.EqualFold(filepath.Ext(p), ".md") {
					files = append(files, p)
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		for _, file := range files {
			if seen[file] {
				continue
			}
			seen[file] = true
			if !strings.EqualFold(filepath.Ext(file), ".md") {
				return fmt.Errorf("component is not Markdown: %s", file)
			}
			data, err := readPluginDefinition(b.Root, file)
			if err != nil {
				return err
			}
			component, err := parseMarkdown(data, file)
			if err != nil {
				return err
			}
			name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
			if filepath.Base(file) == "SKILL.md" {
				name = filepath.Base(filepath.Dir(file))
			} else if info.IsDir() {
				relative, err := filepath.Rel(path, filepath.Dir(file))
				if err != nil {
					return err
				}
				if relative != "." {
					name = strings.ReplaceAll(filepath.ToSlash(relative), "/", ":") + ":" + name
				}
			}
			component.Name = ComponentName(kind, b.Manifest.Name, name, component.Metadata.Name)
			*dst = append(*dst, component)
		}
		return nil
	}
	if len(raw) == 0 || add {
		if err := loadPath("./"+kind, false); err != nil {
			return err
		}
	}
	if len(raw) == 0 {
		return nil
	}
	if kind == "commands" && bytes.TrimSpace(raw)[0] == '[' {
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return err
		}
		hasObjects := false
		for _, item := range items {
			if len(bytes.TrimSpace(item)) > 0 && bytes.TrimSpace(item)[0] == '{' {
				hasObjects = true
				break
			}
		}
		if hasObjects {
			for _, item := range items {
				if err := b.loadMarkdown(kind, item, false, dst); err != nil {
					return err
				}
			}
			return nil
		}
	}
	if kind == "commands" && len(raw) > 0 && bytes.TrimSpace(raw)[0] == '{' {
		var commands map[string]struct {
			Source       string   `json:"source"`
			Content      *string  `json:"content"`
			Description  string   `json:"description"`
			ArgumentHint string   `json:"argumentHint"`
			Model        string   `json:"model"`
			AllowedTools []string `json:"allowedTools"`
		}
		if err := json.Unmarshal(raw, &commands); err != nil {
			return err
		}
		names := make([]string, 0, len(commands))
		for name := range commands {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			c := commands[name]
			if (c.Source == "") == (c.Content == nil) {
				return fmt.Errorf("command %q must have exactly one source or content", name)
			}
			var component MarkdownComponent
			var err error
			if c.Source != "" {
				path, e := contained(b.Root, c.Source)
				if e != nil {
					return e
				}
				var data []byte
				data, err = readPluginDefinition(b.Root, path)
				if err == nil {
					component, err = parseMarkdown(data, path)
				}
			} else {
				component, err = parseMarkdown([]byte(*c.Content), "")
			}
			if err != nil {
				return err
			}
			component.Name = ComponentName(kind, b.Manifest.Name, name, component.Metadata.Name)
			if c.Description != "" {
				component.Metadata.Description = c.Description
			}
			if c.ArgumentHint != "" {
				component.Metadata.ArgumentHint = c.ArgumentHint
			}
			if c.Model != "" {
				component.Metadata.Model = c.Model
			}
			if c.AllowedTools != nil {
				component.Metadata.AllowedTools = c.AllowedTools
			}
			*dst = append(*dst, component)
		}
		return nil
	}
	paths, err := decodePaths(raw)
	if err != nil {
		return fmt.Errorf("%s: %w", kind, err)
	}
	for _, path := range paths {
		if path != "." && !strings.HasPrefix(path, "./") {
			return fmt.Errorf("%s path must start with ./: %q", kind, path)
		}
		if err := loadPath(path, true); err != nil {
			return fmt.Errorf("%s: %w", kind, err)
		}
	}
	return nil
}

func ParseMarkdown(path string) (MarkdownComponent, error) {
	data, err := readDefinition(path)
	if err != nil {
		return MarkdownComponent{}, err
	}
	return parseMarkdown(data, path)
}

// ParseMarkdownText parses an in-memory skill/command using the same YAML parser
// as disk-loaded bundles. Path is provenance and does not cause filesystem access.
func ParseMarkdownText(content, path string) (MarkdownComponent, error) {
	return parseMarkdown([]byte(content), path)
}

func parseMarkdown(data []byte, path string) (MarkdownComponent, error) {
	c := MarkdownComponent{Path: path, Metadata: SkillMetadata{UserInvocable: true}, Frontmatter: map[string]any{}}
	text := strings.ReplaceAll(strings.TrimPrefix(string(data), "\ufeff"), "\r\n", "\n")
	c.Content = text
	if !strings.HasPrefix(text, "---\n") {
		return c, nil
	}
	lines := strings.Split(text, "\n")
	end := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" || lines[i] == "..." {
			end = i
			break
		}
	}
	if end < 0 {
		c.Warnings = append(c.Warnings, "unterminated YAML frontmatter retained as skill body")
		return c, nil
	}
	header := []byte(strings.Join(lines[1:end], "\n"))
	if err := yaml.Unmarshal(header, &c.Frontmatter); err != nil {
		c.Frontmatter = map[string]any{}
		c.Warnings = append(c.Warnings, "invalid YAML frontmatter retained as skill body: "+err.Error())
		return c, nil
	}
	// Normalize scalar description before decoding the strongly typed known fields.
	if value, ok := c.Frontmatter["description"]; ok {
		if _, ok := value.(string); !ok {
			return c, fmt.Errorf("%s: description must be text", path)
		}
	}
	if err := yaml.Unmarshal(header, &c.Metadata); err != nil {
		return c, fmt.Errorf("%s frontmatter: %w", path, err)
	}
	switch hint := c.Frontmatter["argument-hint"].(type) {
	case string:
		c.Metadata.ArgumentHint = hint
	case []any:
		values := make([]string, 0, len(hint))
		for _, value := range hint {
			values = append(values, fmt.Sprint(value))
		}
		c.Metadata.ArgumentHint = strings.Join(values, ",")
		c.Warnings = append(c.Warnings, "argument-hint list rendered as text")
	case nil:
	default:
		c.Warnings = append(c.Warnings, "non-text argument-hint ignored")
	}
	switch arguments := c.Frontmatter["arguments"].(type) {
	case string:
		c.Metadata.Arguments = splitTools(arguments)
	case []any:
		for _, argument := range arguments {
			value, ok := argument.(string)
			if !ok {
				c.Metadata.Arguments = nil
				c.Warnings = append(c.Warnings, "non-string argument declarations retained only in frontmatter")
				break
			}
			c.Metadata.Arguments = append(c.Metadata.Arguments, value)
		}
	case nil:
	default:
		c.Warnings = append(c.Warnings, "non-text argument declarations retained only in frontmatter")
	}
	switch allowed := c.Frontmatter["allowed-tools"].(type) {
	case string:
		for _, tool := range splitTools(allowed) {
			if tool = strings.TrimSpace(tool); tool != "" {
				c.Metadata.AllowedTools = append(c.Metadata.AllowedTools, tool)
			}
		}
	case []any:
		for _, tool := range allowed {
			value, ok := tool.(string)
			if !ok {
				return c, fmt.Errorf("%s: allowed-tools must contain strings", path)
			}
			c.Metadata.AllowedTools = append(c.Metadata.AllowedTools, value)
		}
	case nil:
	default:
		return c, fmt.Errorf("%s: invalid allowed-tools", path)
	}
	switch disallowed := c.Frontmatter["disallowed-tools"].(type) {
	case string:
		c.Metadata.DisallowedTools = splitTools(disallowed)
	case []any:
		for _, tool := range disallowed {
			value, ok := tool.(string)
			if !ok {
				return c, fmt.Errorf("%s: disallowed-tools must contain strings", path)
			}
			c.Metadata.DisallowedTools = append(c.Metadata.DisallowedTools, value)
		}
	case nil:
	default:
		return c, fmt.Errorf("%s: invalid disallowed-tools", path)
	}
	c.Content = strings.Join(lines[end+1:], "\n")
	return c, nil
}

func splitTools(text string) []string {
	var values []string
	start, depth := 0, 0
	for i, r := range text {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',', ' ', '\t', '\n':
			if depth == 0 {
				if part := strings.TrimSpace(text[start:i]); part != "" {
					values = append(values, part)
				}
				start = i + 1
			}
		}
	}
	if tail := strings.TrimSpace(text[start:]); tail != "" {
		values = append(values, tail)
	}
	return values
}

func (b *Bundle) loadConfig(kind string, raw json.RawMessage, defaultPath string) error {
	seen := map[string]bool{}
	var consume func(json.RawMessage) error
	consume = func(spec json.RawMessage) error {
		spec = bytes.TrimSpace(spec)
		if len(spec) == 0 || bytes.Equal(spec, []byte("null")) {
			return nil
		}
		switch spec[0] {
		case '"':
			var relative string
			if err := json.Unmarshal(spec, &relative); err != nil {
				return err
			}
			if kind == "mcpServers" && (strings.HasSuffix(relative, ".mcpb") || strings.HasSuffix(relative, ".dxt")) {
				b.Unsupported = append(b.Unsupported, Issue{Component: kind, Path: relative, Message: "MCP binary bundle installation is unsupported"})
				return nil
			}
			if !strings.HasPrefix(relative, "./") {
				return fmt.Errorf("%s path must start with ./", kind)
			}
			path, err := contained(b.Root, relative)
			if err != nil {
				return err
			}
			if seen[path] {
				return nil
			}
			seen[path] = true
			data, err := readPluginDefinition(b.Root, path)
			if err != nil {
				return err
			}
			return consume(data)
		case '[':
			var specs []json.RawMessage
			if err := json.Unmarshal(spec, &specs); err != nil {
				return err
			}
			for _, item := range specs {
				if err := consume(item); err != nil {
					return err
				}
			}
			return nil
		case '{':
			var object map[string]json.RawMessage
			if err := json.Unmarshal(spec, &object); err != nil {
				return err
			}
			if wrapper, ok := object[kind]; ok {
				return consume(wrapper)
			}
			switch kind {
			case "hooks":
				var hooks map[string][]HookMatcher
				if err := json.Unmarshal(spec, &hooks); err != nil {
					return err
				}
				for event, matchers := range hooks {
					for _, matcher := range matchers {
						if matcher.Matcher != "" && matcher.Matcher != "*" {
							if _, err := regexp.Compile(matcher.Matcher); err != nil {
								return fmt.Errorf("hook matcher: %w", err)
							}
						}
						for _, hook := range matcher.Hooks {
							switch hook.Type {
							case "command", "prompt", "agent", "http", "mcp_tool":
							default:
								b.Unsupported = append(b.Unsupported, Issue{Component: "hooks", Path: event, Message: "unsupported hook type: " + hook.Type})
							}
							for key := range hook.Unknown {
								b.Unsupported = append(b.Unsupported, Issue{Component: "hooks", Path: event + "." + key, Message: "unrecognized hook field retained"})
							}
						}
					}
					b.Hooks[event] = append(b.Hooks[event], matchers...)
				}
			case "mcpServers":
				var servers map[string]MCPServer
				if err := json.Unmarshal(spec, &servers); err != nil {
					return err
				}
				for name, server := range servers {
					if server.Type == "" {
						server.Type = "stdio"
					}
					switch server.Type {
					case "stdio":
						if server.Command == "" {
							return fmt.Errorf("MCP server %s has no command", name)
						}
					case "http", "sse":
						if server.URL == "" {
							return fmt.Errorf("MCP server %s has no URL", name)
						}
					default:
						b.Unsupported = append(b.Unsupported, Issue{Component: kind, Path: name, Message: "unsupported MCP transport: " + server.Type})
					}
					b.MCPServers[name] = server
				}
			case "lspServers":
				var servers map[string]LSPServer
				if err := json.Unmarshal(spec, &servers); err != nil {
					return err
				}
				for name, server := range servers {
					if server.Command == "" || len(server.ExtensionToLanguage) == 0 {
						return fmt.Errorf("invalid LSP server %s", name)
					}
					b.LSPServers[name] = server
				}
			}
			return nil
		default:
			return fmt.Errorf("invalid %s component declaration", kind)
		}
	}
	if defaultPath != "" {
		defaultFile, err := contained(b.Root, "./"+defaultPath)
		if err != nil {
			return err
		}
		if data, err := readPluginDefinition(b.Root, defaultFile); err == nil {
			seen[defaultFile] = true
			if err := consume(data); err != nil {
				return fmt.Errorf("%s: %w", kind, err)
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return consume(raw)
}

func readDefinition(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxDefinitionSize {
		return nil, fmt.Errorf("invalid or oversized definition: %s", path)
	}
	return os.ReadFile(path)
}

func contained(root, relative string) (string, error) {
	if filepath.IsAbs(relative) || strings.Contains(relative, "\\") || strings.Contains(relative, ":") {
		return "", fmt.Errorf("unsafe plugin path: %q", relative)
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("plugin path escapes root: %q", relative)
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if strings.EqualFold(part, ".git") {
			return "", fmt.Errorf("plugin component cannot reference .git: %q", relative)
		}
	}
	if _, err := resolvePluginPath(root, path, true); err != nil {
		return "", err
	}
	return path, nil
}
