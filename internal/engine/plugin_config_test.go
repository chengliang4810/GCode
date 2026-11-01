package engine

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/plugins"
)

func configBound(value float64) *float64 { return &value }
func TestPluginConfigurationValuesFollowDeclaredTypesAndConstraints(t *testing.T) {
	for _, test := range []struct {
		name      string
		option    plugins.UserConfigOption
		raw, want string
		reject    bool
	}{
		{name: "opaque string", option: plugins.UserConfigOption{Type: "string"}, raw: "${HOME} $ARGUMENTS", want: "${HOME} $ARGUMENTS"},
		{name: "opaque path", option: plugins.UserConfigOption{Type: "directory"}, raw: `D:\not-created\${RAW}`, want: `D:\not-created\${RAW}`},
		{name: "false is configured", option: plugins.UserConfigOption{Type: "boolean"}, raw: " false ", want: "false"},
		{name: "boolean string rejected", option: plugins.UserConfigOption{Type: "boolean"}, raw: `"true"`, reject: true},
		{name: "number zero", option: plugins.UserConfigOption{Type: "number", Min: configBound(0), Max: configBound(10)}, raw: "0", want: "0"},
		{name: "number below minimum", option: plugins.UserConfigOption{Type: "number", Min: configBound(0)}, raw: "-1", reject: true},
		{name: "number above maximum", option: plugins.UserConfigOption{Type: "number", Max: configBound(10)}, raw: "11", reject: true},
		{name: "numeric string rejected", option: plugins.UserConfigOption{Type: "number"}, raw: `"2"`, reject: true},
		{name: "nonfinite number rejected", option: plugins.UserConfigOption{Type: "number"}, raw: "1e400", reject: true},
		{name: "trailing JSON rejected", option: plugins.UserConfigOption{Type: "number"}, raw: "1 2", reject: true},
		{name: "multiple string list", option: plugins.UserConfigOption{Type: "string", Multiple: true}, raw: `["one", "two", "${OPAQUE}"]`, want: `["one","two","${OPAQUE}"]`},
		{name: "mixed list rejected", option: plugins.UserConfigOption{Type: "string", Multiple: true}, raw: `["one",2]`, reject: true},
		{name: "null list rejected", option: plugins.UserConfigOption{Type: "string", Multiple: true}, raw: `null`, reject: true},
		{name: "options exact match", option: plugins.UserConfigOption{Type: "string", Options: []string{"auto", "off"}}, raw: "auto", want: "auto"},
		{name: "unknown choice rejected", option: plugins.UserConfigOption{Type: "string", Options: []string{"auto", "off"}}, raw: "sensitive-invalid-input", reject: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, err := normalizePluginConfigValue("field", test.option, test.raw)
			if test.reject {
				if err == nil {
					t.Fatalf("accepted %q", test.raw)
				}
				if strings.Contains(err.Error(), "sensitive-invalid-input") {
					t.Fatal("validation error exposed a configuration value")
				}
				return
			}
			if err != nil || value != test.want {
				t.Fatalf("got %q, %v; want %q", value, err, test.want)
			}
		})
	}
}
func TestPluginConfigurationRejectsUnsupportedSchemaAndInvalidDefaults(t *testing.T) {
	for _, test := range []struct {
		name   string
		option plugins.UserConfigOption
	}{
		{"unknown type", plugins.UserConfigOption{Type: "object"}},
		{"multiple number", plugins.UserConfigOption{Type: "number", Multiple: true}},
		{"bounds on string", plugins.UserConfigOption{Type: "string", Min: configBound(1)}},
		{"inverted bounds", plugins.UserConfigOption{Type: "number", Min: configBound(10), Max: configBound(1)}},
		{"sensitive options", plugins.UserConfigOption{Type: "string", Sensitive: true, Required: true, Options: []string{"one"}}},
		{"multiple options", plugins.UserConfigOption{Type: "string", Multiple: true, Required: true, Options: []string{"one"}}},
		{"duplicate options", plugins.UserConfigOption{Type: "string", Required: true, Options: []string{"one", "one"}}},
		{"choice without default or required", plugins.UserConfigOption{Type: "string", Options: []string{"one"}}},
		{"invalid default choice", plugins.UserConfigOption{Type: "string", Options: []string{"one"}, Default: "sensitive-invalid-default"}},
		{"empty default choice", plugins.UserConfigOption{Type: "string", Options: []string{"one"}, Default: ""}},
		{"control in label", plugins.UserConfigOption{Type: "string", Required: true, Options: []string{"two\nlines"}}},
		{"wrong default type", plugins.UserConfigOption{Type: "number", Default: "sensitive-invalid-default"}},
		{"object default", plugins.UserConfigOption{Type: "string", Multiple: true, Default: map[string]string{"secret": "sensitive-invalid-default"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validatePluginOption("field", test.option)
			if err == nil {
				t.Fatal("invalid definition accepted")
			}
			if strings.Contains(err.Error(), "sensitive-invalid-default") {
				t.Fatal("default value leaked in error")
			}
		})
	}
}
func TestPluginConfigurationDefaultsUseTypedJSONAndEffectiveOverrides(t *testing.T) {
	schema := map[string]plugins.UserConfigOption{
		"off": {Type: "boolean", Default: false}, "zero": {Type: "number", Default: float64(0)},
		"list": {Type: "string", Multiple: true, Default: []any{"one", "two words", "${OPAQUE}"}},
		"one":  {Type: "string", Multiple: true, Default: "single"}, "empty": {Type: "string", Default: ""},
	}
	values, err := effectivePluginConfig(schema, map[string]string{"off": "true", "list": `["saved", "${UNCHANGED}"]`})
	if err != nil {
		t.Fatal(err)
	}
	if values["off"] != "true" || values["zero"] != "0" || values["list"] != `["saved","${UNCHANGED}"]` || values["one"] != `["single"]` {
		t.Fatalf("bad effective values: %#v", values)
	}
	for key, option := range schema {
		field, err := publicPluginConfigField(key, option)
		if err != nil {
			t.Fatal(err)
		}
		if field.Configured != (key != "empty") || field.DefaultConfigured != (key != "empty") {
			t.Fatalf("default presence wrong for %s: %+v", key, field)
		}
	}
	if schema["off"].Default != false || len(schema["list"].Default.([]any)) != 3 {
		t.Fatal("default metadata mutated")
	}
}
func TestPluginConfigurationUpdatesValidateAtomicallyAndAllowSecretRevocation(t *testing.T) {
	schema := map[string]plugins.UserConfigOption{"key": {Type: "string", Required: true, Sensitive: true}, "count": {Type: "number", Min: configBound(0), Max: configBound(3)}}
	original := map[string]string{"key": "private-saved-value", "count": "2"}
	if _, err := updatePluginConfig(schema, original, map[string]string{"key": "private-new-value", "count": "4"}, nil); err == nil {
		t.Fatal("out of range update accepted")
	}
	if !maps.Equal(original, map[string]string{"key": "private-saved-value", "count": "2"}) {
		t.Fatal("failed update changed stored values")
	}
	if _, err := updatePluginConfig(schema, original, nil, []string{"undeclared"}); err == nil {
		t.Fatal("undeclared removal accepted")
	}
	if _, err := updatePluginConfig(schema, original, map[string]string{"key": "new"}, []string{"key"}); err == nil {
		t.Fatal("ambiguous remove-and-set accepted")
	}
	blank, err := updatePluginConfig(schema, original, map[string]string{"key": ""}, nil)
	if err != nil || blank["key"] != original["key"] {
		t.Fatalf("blank overwrote a saved secret: %#v %v", blank, err)
	}
	revoked, err := updatePluginConfig(schema, original, nil, []string{"key"})
	if err != nil || revoked["key"] != "" {
		t.Fatalf("explicit credential revocation refused: %#v %v", revoked, err)
	}
	if _, err = updatePluginConfig(schema, nil, map[string]string{"count": "1"}, nil); err == nil {
		t.Fatal("missing required configuration accepted")
	}
}

func TestPluginConfigurationPublicProjectionHidesDefaultsAndSavedValues(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	market := t.TempDir()
	manifest := map[string]any{"name": "sample", "userConfig": map[string]any{
		"key":   map[string]any{"type": "string", "title": "Token default-sensitive-value", "description": "Hidden default-sensitive-value", "sensitive": true, "default": "default-sensitive-value"},
		"off":   map[string]any{"type": "boolean", "title": "Off", "default": false},
		"count": map[string]any{"type": "number", "title": "Count", "default": 0, "min": 0, "max": 3},
		"mode":  map[string]any{"type": "string", "title": "Mode", "default": "auto", "options": []string{"auto", "off"}},
	}}
	raw, _ := json.Marshal(manifest)
	writePluginFile(t, market, ".claude-plugin/marketplace.json", `{"name":"fixture","owner":{"name":"fixture"},"plugins":[{"name":"sample","source":"./sample"}]}`)
	writePluginFile(t, market, "sample/.claude-plugin/plugin.json", string(raw))
	installRuntimeFixture(t, core, market)
	state, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	public, _ := json.Marshal(state.Plugins)
	if strings.Contains(string(public), "default-sensitive-value") {
		t.Fatal("sensitive default reached a public snapshot")
	}
	for _, field := range state.Plugins.Installed[0].Config {
		if !field.Configured || !field.DefaultConfigured || field.Saved {
			t.Fatalf("default not reflected: %+v", field)
		}
	}
	save := func(values map[string]string, remove []string) error {
		return core.Execute(context.Background(), contracts.Command{Kind: "plugins.configure", ID: "sample@fixture", Plugin: &contracts.PluginInput{Configuration: values, RemoveConfigKeys: remove}})
	}
	if err = save(map[string]string{"key": "saved-sensitive-value", "count": "2"}, nil); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(core.storage.ConfigDirectory(), "plugin-configuration.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = save(map[string]string{"key": "invalid-sensitive-value", "count": "4"}, nil); err == nil {
		t.Fatal("invalid edit reached disk")
	}
	after, _ := os.ReadFile(filepath.Join(core.storage.ConfigDirectory(), "plugin-configuration.json"))
	if string(before) != string(after) {
		t.Fatal("invalid edit rewrote configuration")
	}
	if err = save(map[string]string{}, []string{"unknown"}); err == nil {
		t.Fatal("unknown removal reached disk")
	}
	if err = save(map[string]string{}, []string{"key"}); err != nil {
		t.Fatal(err)
	}
	state, _ = core.Snapshot(context.Background())
	for _, field := range state.Plugins.Installed[0].Config {
		if field.Key == "key" && (!field.Configured || field.Saved) {
			t.Fatalf("removing override did not reactivate default: %+v", field)
		}
	}
	public, _ = json.Marshal(state.Plugins)
	for _, secret := range []string{"default-sensitive-value", "saved-sensitive-value", "invalid-sensitive-value"} {
		if strings.Contains(string(public), secret) {
			t.Fatal("configuration value reached a public snapshot")
		}
	}
	clone := state.Plugins.Clone()
	for i := range clone.Installed[0].Config {
		field := &clone.Installed[0].Config[i]
		if field.Min != nil {
			*field.Min = 99
		}
		if len(field.Options) > 0 {
			field.Options[0] = "changed"
		}
	}
	for _, field := range state.Plugins.Installed[0].Config {
		if field.Min != nil && *field.Min == 99 || slices.Contains(field.Options, "changed") {
			t.Fatal("configuration metadata shared snapshot ownership")
		}
	}
}

func TestPluginHookSecretsIncludeDefaultsAndFrozenMultipleMembers(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	market := t.TempDir()
	writePluginFile(t, market, ".claude-plugin/marketplace.json", `{"name":"fixture","owner":{"name":"fixture"},"plugins":[{"name":"sample","source":"./sample"}]}`)
	writePluginFile(t, market, "sample/.claude-plugin/plugin.json", `{"name":"sample","userConfig":{"arbitrary":{"type":"string","title":"Private","sensitive":true,"default":"private-default-token"},"list":{"type":"string","title":"List","multiple":true,"sensitive":true,"default":["member-default-one","member-default-two"]}}}`)
	installRuntimeFixture(t, core, market)
	text := core.redactHookText(contracts.HookRule{}, "private-default-token member-default-one member-default-two")
	if strings.Contains(text, "private-default-token") || strings.Contains(text, "member-default-") {
		t.Fatalf("global default secret leaked: %s", text)
	}
	// No current owner/registry record matches this immutable rule any longer.
	rule := contracts.HookRule{PluginID: "old@removed", PluginRoot: "retired-version", SensitiveConfigKeys: []string{"arbitrary"}, UserConfig: map[string]string{"arbitrary": `["frozen-one", "frozen-two"]`}}
	redacted := core.redactHookText(rule, "frozen-one frozen-two")
	if strings.Contains(redacted, "frozen-") {
		t.Fatalf("frozen rule lost its sensitive member types: %s", redacted)
	}
	rules := contracts.CloneHookRules([]contracts.HookRule{rule})
	rules[0].SensitiveConfigKeys[0] = "changed"
	if rule.SensitiveConfigKeys[0] != "arbitrary" {
		t.Fatal("frozen key slice shared ownership")
	}
	encoded, _ := json.Marshal(rule)
	if strings.Contains(string(encoded), "sensitiveConfigKeys") {
		t.Fatal("internal sensitivity metadata became a transport field")
	}
}
