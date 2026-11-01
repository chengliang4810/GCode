package engine

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"

	"gcode/internal/plugins"
	"slices"
	"strings"

	"gcode/internal/agent"
	"gcode/internal/contracts"
)

// redactHookText is the shared boundary for persisted hook diagnostics,
// notification queues, verifier errors and model-facing hook context.
func (e *Engine) redactHookText(rule contracts.HookRule, text string) string {
	return e.hookTextRedactor(rule)(text)
}

func (e *Engine) hookTextRedactor(rule contracts.HookRule) func(string) string {
	e.mu.Lock()
	secrets := make([]string, 0)
	for _, provider := range e.providers {
		if provider.APIKey != "" {
			secrets = append(secrets, provider.APIKey)
		}
	}
	saved := clonePluginConfig(e.pluginConfig)
	projection := e.state.Plugins.Clone()
	e.mu.Unlock()
	ownerSchema := map[string]plugins.UserConfigOption{}
	matchesOwner := func(id, path string) bool {
		base, _, _ := strings.Cut(id, "#")
		return rule.PluginID != "" && (rule.PluginID == id || rule.PluginID == base) && (rule.PluginRoot == "" || filepath.Clean(rule.PluginRoot) == filepath.Clean(path))
	}
	// Public field metadata supplies saved-value types, including list members.
	// It deliberately contains no default values, so defaults come from the
	// private installation manifest below instead of entering the snapshot.
	for _, installed := range projection.Installed {
		schema := map[string]plugins.UserConfigOption{}
		for _, field := range installed.Config {
			schema[field.Key] = pluginConfigOptionFromField(field)
		}
		secrets = append(secrets, pluginConfigSecrets(schema, saved[installed.ID])...)
		if matchesOwner(installed.ID, installed.Path) {
			for key, option := range schema {
				ownerSchema[key] = option
			}
		}
	}
	if e.plugins != nil {
		for _, installed := range e.plugins.Installed() {
			schema := installed.Manifest.UserConfig
			secrets = append(secrets, pluginConfigSecrets(schema, saved[installationKey(installed)])...)
			secrets = append(secrets, pluginConfigSecrets(schema, nil)...)
			if matchesOwner(installed.ID, installed.Path) {
				for key, option := range schema {
					ownerSchema[key] = option
				}
			}
		}
	}
	for _, key := range rule.SensitiveConfigKeys {
		option := ownerSchema[key]
		option.Type = "string"
		option.Sensitive = true
		option.Multiple = strings.HasPrefix(strings.TrimSpace(rule.UserConfig[key]), "[")
		ownerSchema[key] = option
	}
	secrets = append(secrets, pluginConfigSecrets(ownerSchema, rule.UserConfig)...)
	// Non-plugin/user rules have no owning manifest. Keep the explicit
	// credential-name fallback, including arrays when the value is a JSON list.
	for key, value := range rule.UserConfig {
		if value == "" || !hookSensitiveKey(key) {
			continue
		}
		schema := map[string]plugins.UserConfigOption{key: {Type: "string", Sensitive: true, Multiple: strings.HasPrefix(strings.TrimSpace(value), "[")}}
		secrets = append(secrets, pluginConfigSecrets(schema, rule.UserConfig)...)
	}
	for _, name := range rule.AllowedEnvVars {
		if value := os.Getenv(name); value != "" {
			secrets = append(secrets, value)
		}
	}
	for key, value := range rule.Headers {
		if hookSensitiveKey(key) && !strings.Contains(value, "$") && value != "" {
			secrets = append(secrets, value)
		}
	}
	variants := make([]string, 0, len(secrets)*3)
	for _, secret := range secrets {
		variants = append(variants, secret, url.QueryEscape(secret), url.PathEscape(secret))
		if encoded, err := json.Marshal(secret); err == nil && len(encoded) > 2 {
			variants = append(variants, string(encoded[1:len(encoded)-1]))
		}
	}
	slices.SortFunc(variants, func(a, b string) int { return len(b) - len(a) })
	variants = slices.Compact(variants)
	return func(text string) string { return redactPluginConfigText(text, variants) }
}

func hookSensitiveKey(key string) bool {
	key = strings.ToLower(key)
	for _, part := range []string{"token", "secret", "password", "credential", "api_key", "apikey", "authorization", "private_key"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}

func (e *Engine) redactHookResult(rule contracts.HookRule, result agent.HookResult) agent.HookResult {
	return redactHookResultWith(result, e.hookTextRedactor(rule))
}

func redactHookResultWith(result agent.HookResult, redact func(string) string) agent.HookResult {
	result.Context = redact(result.Context)
	result.Reason = redact(result.Reason)
	result.SystemMessage = redact(result.SystemMessage)
	result.StopReason = redact(result.StopReason)
	result.InitialUserMessage = redact(result.InitialUserMessage)
	result.SessionTitle = redact(result.SessionTitle)
	if result.DisplayContent != nil {
		value := redact(*result.DisplayContent)
		result.DisplayContent = &value
	}
	return result
}
