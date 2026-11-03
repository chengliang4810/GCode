package engine

import (
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/plugins"
)

func TestPluginDynamicContextRedactsConfiguredAndDefaultSecrets(t *testing.T) {
	for _, value := range []string{"configured-secret$123", "default-secret$456"} {
		t.Run(value, func(t *testing.T) {
			bundle := plugins.Bundle{Manifest: plugins.Manifest{UserConfig: map[string]plugins.UserConfigOption{
				"key": {Sensitive: true, Default: value},
			}}}
			skill := runtimeSkill{bundle: &bundle, config: map[string]string{"key": value}, component: plugins.MarkdownComponent{
				Path:    filepath.Join(t.TempDir(), "SKILL.md"),
				Content: "!`printf '%s' \"${user_config.key}\"`",
			}}
			var observed string
			invoke := func(_ context.Context, input agent.HookInput) (agent.HookResult, error) {
				data, _ := json.Marshal(input)
				observed += string(data)
				return agent.HookResult{}, nil
			}
			output, err := (runtimeResources{}).expand(context.Background(), skill, "", t.TempDir(), "fixture", invoke)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(output+observed, value) || !strings.Contains(output, "[REDACTED]") {
				t.Fatalf("sensitive process output crossed the model/hook boundary: %q", output)
			}
		})
	}
}

func TestPluginMCPInterpolationKeepsConfigurationOpaque(t *testing.T) {
	t.Setenv("PLUGIN_TEST_ENV", "env$value")
	bundle := plugins.Bundle{
		ID: "sample@fixture",
		Manifest: plugins.Manifest{Name: "sample", UserConfig: map[string]plugins.UserConfigOption{
			"key": {Type: "string", Default: "abc$UNSET"},
		}},
		MCPServers: map[string]plugins.MCPServer{"echo": {
			Command: "echo",
			Args:    []string{"${user_config.key}", "${PLUGIN_TEST_ENV}", "${MISSING_PLUGIN_TEST_ENV:-fallback}"},
		}},
	}
	resources, err := buildRuntimeResources(t.TempDir(), t.TempDir(), t.TempDir(), "fixture", []plugins.Bundle{bundle}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := resources.mcp[0].Args
	if len(got) != 3 || got[0] != "abc$UNSET" || got[1] != "env$value" || got[2] != "fallback" {
		t.Fatalf("opaque values were reinterpreted: %q", got)
	}
}

func TestPluginSensitiveMultipleRedactsIndividualProcessOutput(t *testing.T) {
	bundle := plugins.Bundle{Manifest: plugins.Manifest{UserConfig: map[string]plugins.UserConfigOption{
		"keys": {Type: "string", Multiple: true, Sensitive: true, Default: []string{"array-first-secret", "array-second-secret"}},
	}}}
	skill := runtimeSkill{bundle: &bundle, config: map[string]string{"keys": `["array-first-secret","array-second-secret"]`}, component: plugins.MarkdownComponent{
		Path:    filepath.Join(t.TempDir(), "SKILL.md"),
		Content: "!`value=${user_config.keys}; value=${value#\\[\\\"}; printf '%s' \"${value%%\\\"*}\"`",
	}}
	var observed string
	invoke := func(_ context.Context, input agent.HookInput) (agent.HookResult, error) {
		data, _ := json.Marshal(input)
		observed += string(data)
		return agent.HookResult{}, nil
	}
	output, err := (runtimeResources{}).expand(context.Background(), skill, "", t.TempDir(), "fixture", invoke)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output+observed, "array-first-secret") || output != "[REDACTED]" {
		t.Fatalf("sensitive array member crossed the output boundary: %q", output)
	}
}

func TestPluginPowerShellConfigurationIsAnOpaqueEnvironmentValue(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell runtime")
	}
	const value = "literal$variable'\";Write-Output injected"
	bundle := plugins.Bundle{Manifest: plugins.Manifest{UserConfig: map[string]plugins.UserConfigOption{
		"key": {Type: "string"},
	}}}
	skill := runtimeSkill{bundle: &bundle, config: map[string]string{"key": value}, component: plugins.MarkdownComponent{
		Path:     filepath.Join(t.TempDir(), "SKILL.md"),
		Content:  "!`[Console]::Write(\"${user_config.key}\")`",
		Metadata: plugins.SkillMetadata{Shell: "powershell"},
	}}
	output, err := (runtimeResources{}).expand(context.Background(), skill, "", t.TempDir(), "fixture", nil)
	if err != nil || output != value {
		t.Fatalf("PowerShell reinterpreted a configuration value: %q %v", output, err)
	}
}

func TestPluginProjectionSeparatesCatalogIdentityAndRuntimeName(t *testing.T) {
	core, _ := resourceCore(t)
	market := t.TempDir()
	writePluginFile(t, market, ".claude-plugin/marketplace.json", `{"name":"fixture","owner":{"name":"fixture"},"plugins":[{"name":"sample","source":"./sample"}]}`)
	writePluginFile(t, market, "sample/.claude-plugin/plugin.json", `{"name":"runtime-name","version":"1.0.0"}`)
	installRuntimeFixture(t, core, market)
	state, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Plugins.Installed) != 1 {
		t.Fatal("installation missing")
	}
	item := state.Plugins.Installed[0]
	if item.ID != "sample@fixture" || item.CatalogID != "sample@fixture" || item.Name != "runtime-name" {
		t.Fatalf("catalog and runtime identities were conflated: %+v", item)
	}
}

func TestPluginOptionalMetadataWarningReachesHostDiagnostics(t *testing.T) {
	core, _ := resourceCore(t)
	market, cwd := t.TempDir(), t.TempDir()
	writePluginFile(t, market, ".claude-plugin/marketplace.json", `{"name":"fixture","owner":{"name":"fixture"},"plugins":[{"name":"sample","source":"./sample"}]}`)
	writePluginFile(t, market, "sample/.claude-plugin/plugin.json", `{"name":"sample","version":"1.0.0"}`)
	writePluginFile(t, market, "sample/skills/review/SKILL.md", "---\ndescription: [\n---\nReview the fixture.\n")
	installRuntimeFixture(t, core, market)
	state, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Plugins.Installed) != 1 || len(state.Plugins.Installed[0].Issues) == 0 {
		t.Fatal("optional metadata warning disappeared from the public installation")
	}
	bundles, err := core.plugins.Bundles()
	if err != nil {
		t.Fatal(err)
	}
	resources, err := core.prepareRuntimeResources(core.storage.ConfigDirectory(), contracts.Session{CWD: cwd}, bundles, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resources.instructions(), "invalid YAML frontmatter") ||
		!strings.Contains(resources.skills["sample:review"].component.Content, "Review the fixture.") {
		t.Fatal("warning or preserved skill body was lost at the host boundary")
	}
}
