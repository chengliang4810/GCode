package contracts

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/dlclark/regexp2"
)

type HookEvent string

const (
	HookSessionStart        HookEvent = "SessionStart"
	HookSetup               HookEvent = "Setup"
	HookInstructionsLoaded  HookEvent = "InstructionsLoaded"
	HookUserPromptSubmit    HookEvent = "UserPromptSubmit"
	HookUserPromptExpansion HookEvent = "UserPromptExpansion"
	HookMessageDisplay      HookEvent = "MessageDisplay"
	HookPreToolUse          HookEvent = "PreToolUse"
	HookPermissionRequest   HookEvent = "PermissionRequest"
	HookPermissionDenied    HookEvent = "PermissionDenied"
	HookPostToolUse         HookEvent = "PostToolUse"
	HookPostToolUseFailure  HookEvent = "PostToolUseFailure"
	HookPostToolBatch       HookEvent = "PostToolBatch"
	HookNotification        HookEvent = "Notification"
	HookSubagentStart       HookEvent = "SubagentStart"
	HookSubagentStop        HookEvent = "SubagentStop"
	HookTaskCreated         HookEvent = "TaskCreated"
	HookTaskCompleted       HookEvent = "TaskCompleted"
	HookStop                HookEvent = "Stop"
	HookStopFailure         HookEvent = "StopFailure"
	HookTeammateIdle        HookEvent = "TeammateIdle"
	HookConfigChange        HookEvent = "ConfigChange"
	HookCwdChanged          HookEvent = "CwdChanged"
	HookDirectoryAdded      HookEvent = "DirectoryAdded"
	HookFileChanged         HookEvent = "FileChanged"
	HookWorktreeCreate      HookEvent = "WorktreeCreate"
	HookWorktreeRemove      HookEvent = "WorktreeRemove"
	HookPreCompact          HookEvent = "PreCompact"
	HookPostCompact         HookEvent = "PostCompact"
	HookPreModelSwitch      HookEvent = "PreModelSwitch"
	HookPostModelSwitch     HookEvent = "PostModelSwitch"
	HookElicitation         HookEvent = "Elicitation"
	HookElicitationResult   HookEvent = "ElicitationResult"
	HookSessionEnd          HookEvent = "SessionEnd"
)

// HookRule is a flattened Claude handler plus its containing matcher group.
// Args nil means shell form; an empty nonnil slice means exec form.
type HookRule struct {
	SensitiveConfigKeys []string          `json:"-"`
	PluginID            string            `json:"-"`
	ID                  string            `json:"id"`
	Name                string            `json:"name"`
	Event               HookEvent         `json:"event"`
	Matcher             string            `json:"matcher,omitempty"`
	Type                string            `json:"type"`
	Command             string            `json:"command,omitempty"`
	Args                []string          `json:"args"`
	Shell               string            `json:"shell,omitempty"`
	Async               bool              `json:"async,omitempty"`
	AsyncRewake         bool              `json:"asyncRewake,omitempty"`
	TimeoutSeconds      float64           `json:"timeout,omitempty"`
	StatusMessage       string            `json:"statusMessage,omitempty"`
	Once                bool              `json:"once,omitempty"`
	If                  string            `json:"if,omitempty"`
	OnFailure           string            `json:"onFailure,omitempty"`
	URL                 string            `json:"url,omitempty"`
	Headers             map[string]string `json:"headers,omitempty"`
	AllowedEnvVars      []string          `json:"allowedEnvVars,omitempty"`
	Prompt              string            `json:"prompt,omitempty"`
	Model               string            `json:"model,omitempty"`
	ContinueOnBlock     bool              `json:"continueOnBlock,omitempty"`
	MCPServer           string            `json:"server,omitempty"`
	ToolName            string            `json:"tool,omitempty"`
	ToolInput           json.RawMessage   `json:"input,omitempty"`
	// Source "skill" enables once; other sources ignore it.
	Source     string            `json:"source,omitempty"`
	PluginRoot string            `json:"pluginRoot,omitempty"`
	PluginData string            `json:"pluginData,omitempty"`
	UserConfig map[string]string `json:"userConfig,omitempty"`
	Enabled    bool              `json:"enabled"`
}

func HookEvents() []HookEvent {
	return []HookEvent{HookSessionStart, HookSetup, HookInstructionsLoaded, HookUserPromptSubmit,
		HookUserPromptExpansion, HookMessageDisplay, HookPreToolUse, HookPermissionRequest,
		HookPermissionDenied, HookPostToolUse, HookPostToolUseFailure, HookPostToolBatch,
		HookNotification, HookSubagentStart, HookSubagentStop, HookTaskCreated, HookTaskCompleted,
		HookStop, HookStopFailure, HookTeammateIdle, HookConfigChange, HookCwdChanged,
		HookDirectoryAdded, HookFileChanged, HookWorktreeCreate, HookWorktreeRemove,
		HookPreCompact, HookPostCompact, HookPreModelSwitch, HookPostModelSwitch,
		HookElicitation, HookElicitationResult, HookSessionEnd}
}

func CloneHookRules(rules []HookRule) []HookRule {
	result := slices.Clone(rules)
	for i := range result {
		result[i].Args = slices.Clone(result[i].Args)
		result[i].AllowedEnvVars = slices.Clone(result[i].AllowedEnvVars)
		result[i].SensitiveConfigKeys = slices.Clone(result[i].SensitiveConfigKeys)
		result[i].Headers = maps.Clone(result[i].Headers)
		result[i].UserConfig = maps.Clone(result[i].UserConfig)
		result[i].ToolInput = slices.Clone(result[i].ToolInput)
	}
	return result
}

// HookTypeSupported follows the public event/type matrix. Unsupported handlers
// can be retained in plugin manifests; execution skips them.
func HookTypeSupported(event HookEvent, kind string) bool {
	if kind == "command" || kind == "mcp_tool" {
		return true
	}
	if event == HookSessionStart || event == HookSetup {
		return false
	}
	if kind == "http" {
		return true
	}
	if kind != "prompt" && kind != "agent" {
		return false
	}
	if event == HookPermissionRequest {
		return kind == "prompt"
	}
	return slices.Contains([]HookEvent{HookPermissionDenied, HookPostToolBatch, HookPostToolUse,
		HookPostToolUseFailure, HookPreToolUse, HookStop, HookSubagentStop, HookTaskCompleted,
		HookTaskCreated, HookTeammateIdle, HookUserPromptExpansion, HookUserPromptSubmit}, event)
}

func ValidateHookRules(rules []HookRule) error {
	seen := map[string]bool{}
	for _, rule := range rules {
		if strings.TrimSpace(rule.ID) == "" || seen[rule.ID] {
			return fmt.Errorf("hook IDs must be nonempty and unique")
		}
		seen[rule.ID] = true
		if !slices.Contains(HookEvents(), rule.Event) {
			return fmt.Errorf("hook %q has an unsupported event", rule.ID)
		}
		switch rule.Type {
		case "command":
			if strings.TrimSpace(rule.Command) == "" || strings.ContainsRune(rule.Command, 0) {
				return fmt.Errorf("hook %q requires a valid command", rule.ID)
			}
		case "http":
			u, err := url.Parse(rule.URL)
			if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
				return fmt.Errorf("hook %q requires an HTTP(S) URL", rule.ID)
			}
		case "prompt", "agent":
			if strings.TrimSpace(rule.Prompt) == "" {
				return fmt.Errorf("hook %q requires a prompt", rule.ID)
			}
		case "mcp_tool":
			if rule.MCPServer == "" || rule.ToolName == "" {
				return fmt.Errorf("hook %q requires server and tool", rule.ID)
			}
			if len(rule.ToolInput) > 0 && !json.Valid(rule.ToolInput) {
				return fmt.Errorf("hook %q has invalid MCP input", rule.ID)
			}
		default:
			return fmt.Errorf("hook %q has unsupported type %q", rule.ID, rule.Type)
		}
		if rule.Shell != "" && rule.Shell != "bash" && rule.Shell != "powershell" {
			return fmt.Errorf("hook %q shell must be bash or powershell", rule.ID)
		}
		if rule.OnFailure != "" && rule.OnFailure != "continue" && rule.OnFailure != "block" {
			return fmt.Errorf("hook %q onFailure must be continue or block", rule.ID)
		}
		if (rule.Async || rule.AsyncRewake) && rule.Type != "command" {
			return fmt.Errorf("hook %q async is only supported for command hooks", rule.ID)
		}
		for _, arg := range rule.Args {
			if strings.ContainsRune(arg, 0) {
				return fmt.Errorf("hook %q contains an invalid argument", rule.ID)
			}
		}
		if math.IsNaN(rule.TimeoutSeconds) || math.IsInf(rule.TimeoutSeconds, 0) ||
			rule.TimeoutSeconds < 0 || rule.TimeoutSeconds >= float64(1<<63-1)/float64(time.Second) {
			return fmt.Errorf("hook %q timeout must be finite, nonnegative and fit a duration", rule.ID)
		}
	}
	return nil
}

func hookLiteralMatcher(event HookEvent, matcher string) bool {
	narrow := event == HookFileChanged || event == HookStopFailure
	for _, c := range matcher {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '|' {
			continue
		}
		if !narrow && (c == '-' || c == ' ' || c == ',') {
			continue
		}
		return false
	}
	return true
}

// HookMatches distinguishes literal alternatives from unanchored regexes.
// Events without matchers ignore a configured matcher.
func HookMatches(event HookEvent, matcher, value string) bool {
	if slices.Contains([]HookEvent{HookUserPromptSubmit, HookPostToolBatch, HookStop,
		HookTeammateIdle, HookTaskCreated, HookTaskCompleted, HookWorktreeCreate,
		HookWorktreeRemove, HookMessageDisplay, HookCwdChanged}, event) || matcher == "" || matcher == "*" {
		return true
	}
	if hookLiteralMatcher(event, matcher) {
		for _, item := range strings.FieldsFunc(matcher, func(c rune) bool { return c == '|' || c == ',' }) {
			if strings.TrimSpace(item) == value {
				return true
			}
		}
		return false
	}
	re, err := regexp2.Compile(matcher, regexp2.ECMAScript)
	if err != nil {
		return strings.Contains(value, matcher)
	}
	re.MatchTimeout = 100 * time.Millisecond
	matched, err := re.MatchString(value)
	return err == nil && matched
}
