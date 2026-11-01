// Package plugins loads Claude plugin bundles and manages immutable local installations.
// It never executes code contained in a plugin.
package plugins

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"
)

type Author struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	URL   string `json:"url,omitempty"`
}

// Source is either a relative path or a repository/package descriptor in a marketplace.
type Source struct {
	Source   string             `json:"source,omitempty"`
	Repo     string             `json:"repo,omitempty"`
	URL      string             `json:"url,omitempty"`
	Path     string             `json:"path,omitempty"`
	Ref      string             `json:"ref,omitempty"`
	SHA      string             `json:"sha,omitempty"`
	Package  string             `json:"package,omitempty"`
	Version  string             `json:"version,omitempty"`
	Registry string             `json:"registry,omitempty"`
	Relative string             `json:"-"`
	Name     string             `json:"name,omitempty"`
	Owner    *Author            `json:"owner,omitempty"`
	Plugins  []MarketplaceEntry `json:"plugins,omitempty"`
}

func (s *Source) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		return json.Unmarshal(data, &s.Relative)
	}
	type plain Source
	return json.Unmarshal(data, (*plain)(s))
}

func (s Source) MarshalJSON() ([]byte, error) {
	if s.Relative != "" {
		return json.Marshal(s.Relative)
	}
	type plain Source
	return json.Marshal(plain(s))
}

type Dependency struct {
	Name        string `json:"name"`
	Marketplace string `json:"marketplace,omitempty"`
	Version     string `json:"version,omitempty"`
}

func (d *Dependency) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		return json.Unmarshal(data, &d.Name)
	}
	type plain Dependency
	return json.Unmarshal(data, (*plain)(d))
}

// Component declarations retain their JSON union shape until resolved by Load.
type Manifest struct {
	Name           string                      `json:"name"`
	DisplayName    string                      `json:"displayName,omitempty"`
	Version        string                      `json:"version,omitempty"`
	Description    string                      `json:"description,omitempty"`
	Author         *Author                     `json:"author,omitempty"`
	Homepage       string                      `json:"homepage,omitempty"`
	Repository     string                      `json:"repository,omitempty"`
	License        string                      `json:"license,omitempty"`
	Keywords       []string                    `json:"keywords,omitempty"`
	DefaultEnabled *bool                       `json:"defaultEnabled,omitempty"`
	Dependencies   []Dependency                `json:"dependencies,omitempty"`
	Commands       json.RawMessage             `json:"commands,omitempty"`
	Skills         json.RawMessage             `json:"skills,omitempty"`
	Agents         json.RawMessage             `json:"agents,omitempty"`
	Hooks          json.RawMessage             `json:"hooks,omitempty"`
	MCPServers     json.RawMessage             `json:"mcpServers,omitempty"`
	LSPServers     json.RawMessage             `json:"lspServers,omitempty"`
	OutputStyles   json.RawMessage             `json:"outputStyles,omitempty"`
	Settings       map[string]any              `json:"settings,omitempty"`
	UserConfig     map[string]UserConfigOption `json:"userConfig,omitempty"`
	Channels       []Channel                   `json:"channels,omitempty"`
	Metadata       map[string]any              `json:"metadata,omitempty"`
	Experimental   map[string]json.RawMessage  `json:"experimental,omitempty"`
	Unknown        map[string]json.RawMessage  `json:"-"`
}

type UserConfigOption struct {
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Required    bool     `json:"required,omitempty"`
	Sensitive   bool     `json:"sensitive,omitempty"`
	Multiple    bool     `json:"multiple,omitempty"`
	Default     any      `json:"default,omitempty"`
	Min         *float64 `json:"min,omitempty"`
	Max         *float64 `json:"max,omitempty"`
	Options     []string `json:"options,omitempty"`
}

type Channel struct {
	Server      string                      `json:"server"`
	DisplayName string                      `json:"displayName,omitempty"`
	UserConfig  map[string]UserConfigOption `json:"userConfig,omitempty"`
}

type MarketplaceEntry struct {
	Manifest
	Source Source `json:"source"`
	// Origin retains the original source when an isolated host adapter materializes
	// that source as a local directory. It is installation provenance.
	Origin   *Source  `json:"origin,omitempty"`
	Strict   *bool    `json:"strict,omitempty"`
	Category string   `json:"category,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

type Marketplace struct {
	Name                                string             `json:"name"`
	Owner                               Author             `json:"owner"`
	Plugins                             []MarketplaceEntry `json:"plugins"`
	AllowCrossMarketplaceDependenciesOn []string           `json:"allowCrossMarketplaceDependenciesOn,omitempty"`
	Metadata                            struct {
		PluginRoot  string `json:"pluginRoot,omitempty"`
		Description string `json:"description,omitempty"`
		Version     string `json:"version,omitempty"`
	} `json:"metadata,omitempty"`
}

type MarketplaceSource struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Source     Source            `json:"source"`
	Builtin    bool              `json:"builtin,omitempty"`
	AutoUpdate bool              `json:"autoUpdate,omitempty"`
	Enabled    bool              `json:"enabled"`
	Headers    map[string]string `json:"headers,omitempty"`
}

type Catalog struct {
	SourceID    string            `json:"sourceId"`
	Marketplace Marketplace       `json:"marketplace"`
	Root        string            `json:"root"`
	Links       map[string]string `json:"links,omitempty"`
	Commit      string            `json:"commit,omitempty"`
	RefreshedAt time.Time         `json:"refreshedAt"`
}

type InstallOptions struct {
	Scope       string
	ProjectPath string
}

type Installation struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Marketplace   string    `json:"marketplace"`
	SourceID      string    `json:"sourceId"`
	Version       string    `json:"version,omitempty"`
	Commit        string    `json:"commit,omitempty"`
	Path          string    `json:"path"`
	Enabled       bool      `json:"enabled"`
	Scope         string    `json:"scope"`
	ProjectPath   string    `json:"projectPath,omitempty"`
	InstalledAt   time.Time `json:"installedAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	Manifest      Manifest  `json:"manifest"`
	EntryManifest Manifest  `json:"entryManifest,omitempty"`
	Source        Source    `json:"source"`
	Issues        []Issue   `json:"issues,omitempty"`
}

type Issue struct {
	Component string `json:"component"`
	Path      string `json:"path,omitempty"`
	Message   string `json:"message"`
}

type SkillMetadata struct {
	Name                   string                   `yaml:"name" json:"name,omitempty"`
	Description            string                   `yaml:"description" json:"description,omitempty"`
	ArgumentHint           string                   `yaml:"-" json:"argumentHint,omitempty"`
	AllowedTools           []string                 `yaml:"-" json:"allowedTools,omitempty"`
	DisallowedTools        []string                 `yaml:"-" json:"disallowedTools,omitempty"`
	Model                  string                   `yaml:"model" json:"model,omitempty"`
	DisableModelInvocation bool                     `yaml:"disable-model-invocation" json:"disableModelInvocation,omitempty"`
	UserInvocable          bool                     `yaml:"user-invocable" json:"userInvocable"`
	Context                string                   `yaml:"context" json:"context,omitempty"`
	Agent                  string                   `yaml:"agent" json:"agent,omitempty"`
	Effort                 any                      `yaml:"effort" json:"effort,omitempty"`
	Hooks                  map[string][]HookMatcher `yaml:"hooks" json:"hooks,omitempty"`
	Arguments              []string                 `yaml:"-" json:"arguments,omitempty"`
	Shell                  string                   `yaml:"shell" json:"shell,omitempty"`
}

type MarkdownComponent struct {
	Name        string         `json:"name"`
	Path        string         `json:"path"`
	Content     string         `json:"content"`
	Metadata    SkillMetadata  `json:"metadata"`
	Frontmatter map[string]any `json:"frontmatter,omitempty"`
	Warnings    []string       `json:"warnings,omitempty"`
}

type Hook struct {
	Type           string            `json:"type" yaml:"type"`
	Command        string            `json:"command,omitempty" yaml:"command,omitempty"`
	Prompt         string            `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	URL            string            `json:"url,omitempty" yaml:"url,omitempty"`
	Headers        map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	AllowedEnvVars []string          `json:"allowedEnvVars,omitempty" yaml:"allowedEnvVars,omitempty"`
	Timeout        float64           `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Model          string            `json:"model,omitempty" yaml:"model,omitempty"`
	StatusMessage  string            `json:"statusMessage,omitempty" yaml:"statusMessage,omitempty"`
	Async          bool              `json:"async,omitempty" yaml:"async,omitempty"`
	AsyncRewake    bool              `json:"asyncRewake,omitempty" yaml:"asyncRewake,omitempty"`
	Once           bool              `json:"once,omitempty" yaml:"once,omitempty"`
	If             string            `json:"if,omitempty" yaml:"if,omitempty"`
	// A non-nil empty Args selects exec form. Do not collapse [] into nil.
	Args      []string       `json:"args,omitempty" yaml:"args,omitempty"`
	Shell     string         `json:"shell,omitempty" yaml:"shell,omitempty"`
	OnFailure string         `json:"onFailure,omitempty" yaml:"onFailure,omitempty"`
	Server    string         `json:"server,omitempty" yaml:"server,omitempty"`
	Tool      string         `json:"tool,omitempty" yaml:"tool,omitempty"`
	Input     map[string]any `json:"input,omitempty" yaml:"input,omitempty"`
	Unknown   map[string]any `json:"-" yaml:",inline"`
}

func (h *Hook) UnmarshalJSON(data []byte) error {
	type plain Hook
	var parsed plain
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range strings.Fields("type command prompt url headers allowedEnvVars timeout model statusMessage async asyncRewake once if args shell onFailure server tool input") {
		delete(fields, key)
	}
	parsed.Unknown = fields
	*h = Hook(parsed)
	return nil
}

func (h Hook) MarshalJSON() ([]byte, error) {
	type plain Hook
	data, err := json.Marshal(plain(h))
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if h.Args != nil {
		fields["args"], err = json.Marshal(h.Args)
		if err != nil {
			return nil, err
		}
	}
	for key, value := range h.Unknown {
		if _, exists := fields[key]; exists {
			continue
		}
		fields[key], err = json.Marshal(value)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(fields)
}

type HookMatcher struct {
	Matcher string `json:"matcher,omitempty" yaml:"matcher,omitempty"`
	Hooks   []Hook `json:"hooks" yaml:"hooks"`
}

type MCPServer struct {
	Type          string            `json:"type,omitempty"`
	Command       string            `json:"command,omitempty"`
	Args          []string          `json:"args,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	URL           string            `json:"url,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	HeadersHelper string            `json:"headersHelper,omitempty"`
	OAuth         json.RawMessage   `json:"oauth,omitempty"`
}

type LSPServer struct {
	Command               string            `json:"command"`
	Args                  []string          `json:"args,omitempty"`
	ExtensionToLanguage   map[string]string `json:"extensionToLanguage"`
	Transport             string            `json:"transport,omitempty"`
	Env                   map[string]string `json:"env,omitempty"`
	InitializationOptions any               `json:"initializationOptions,omitempty"`
	Settings              any               `json:"settings,omitempty"`
	WorkspaceFolder       string            `json:"workspaceFolder,omitempty"`
	StartupTimeout        int               `json:"startupTimeout,omitempty"`
	ShutdownTimeout       int               `json:"shutdownTimeout,omitempty"`
	RestartOnCrash        bool              `json:"restartOnCrash,omitempty"`
	MaxRestarts           int               `json:"maxRestarts,omitempty"`
}

type Bundle struct {
	ID           string                   `json:"id"`
	Origin       string                   `json:"origin,omitempty"`
	Root         string                   `json:"root"`
	Installation *Installation            `json:"installation,omitempty"`
	Manifest     Manifest                 `json:"manifest"`
	Skills       []MarkdownComponent      `json:"skills"`
	Commands     []MarkdownComponent      `json:"commands"`
	Agents       []MarkdownComponent      `json:"agents"`
	OutputStyles []MarkdownComponent      `json:"outputStyles"`
	Hooks        map[string][]HookMatcher `json:"hooks"`
	MCPServers   map[string]MCPServer     `json:"mcpServers"`
	LSPServers   map[string]LSPServer     `json:"lspServers"`
	Settings     map[string]any           `json:"settings,omitempty"`
	Unsupported  []Issue                  `json:"unsupported,omitempty"`
}

// Clone returns an independent snapshot; consumers must not share mutable bundle maps.
func (b Bundle) Clone() Bundle {
	return copyValue(b)
}

func copyValue[T any](value T) T {
	return cloneValue(reflect.ValueOf(value)).Interface().(T)
}

func cloneValue(value reflect.Value) reflect.Value {
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.New(value.Type()).Elem()
		out.Set(cloneValue(value.Elem()))
		return out
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.New(value.Type().Elem())
		out.Elem().Set(cloneValue(value.Elem()))
		return out
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.MakeMapWithSize(value.Type(), value.Len())
		iter := value.MapRange()
		for iter.Next() {
			out.SetMapIndex(iter.Key(), cloneValue(iter.Value()))
		}
		return out
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := 0; i < value.Len(); i++ {
			out.Index(i).Set(cloneValue(value.Index(i)))
		}
		return out
	case reflect.Struct:
		out := reflect.New(value.Type()).Elem()
		out.Set(value)
		for i := 0; i < value.NumField(); i++ {
			if out.Field(i).CanSet() && value.Type().Field(i).IsExported() {
				out.Field(i).Set(cloneValue(value.Field(i)))
			}
		}
		return out
	default:
		return value
	}
}
