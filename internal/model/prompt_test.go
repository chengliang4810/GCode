package model

import (
	"testing"

	"gcode/internal/privacy"
	"gcode/third_party/magpie/redact"
)

func TestPromptCloneOwnsNestedToolDefinitions(t *testing.T) {
	original := Prompt{Tools: []Tool{{Name: "Read", Parameters: map[string]any{
		"nested": map[string]any{"enum": []string{"one"}, "anyOf": []any{map[string]any{"type": "string"}}},
	}}}}
	cloned := ClonePrompt(original)
	nested := cloned.Tools[0].Parameters["nested"].(map[string]any)
	nested["enum"].([]string)[0] = "changed"
	nested["anyOf"].([]any)[0].(map[string]any)["type"] = "changed"
	cloned.Tools[0].Name = "changed"
	prior := original.Tools[0].Parameters["nested"].(map[string]any)
	if original.Tools[0].Name != "Read" || prior["enum"].([]string)[0] != "one" ||
		prior["anyOf"].([]any)[0].(map[string]any)["type"] != "string" {
		t.Fatal("saved request prefix shares mutable tool definitions")
	}
}

func TestOutputLimitMatchesProtocolDefaults(t *testing.T) {
	for _, protocol := range []Protocol{Chat, Responses, Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			want := 0
			if protocol == Messages {
				want = 8192
			}
			if got := (Model{}).OutputLimit(protocol); got != want {
				t.Fatalf("default output reservation differs from wire: %d, want %d", got, want)
			}
			if got := (Model{MaxOutput: 4096}).OutputLimit(protocol); got != 4096 {
				t.Fatal("configured output limit was not reserved")
			}
		})
	}
}

func TestRequestOutputLimit(t *testing.T) {
	for _, protocol := range []Protocol{Chat, Responses, Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			large := Model{MaxOutput: 384_000}
			small := Model{MaxOutput: 4_096}
			if large.RequestOutputLimit(protocol, false) != 64_000 ||
				large.RequestOutputLimit(protocol, true) != 16_000 ||
				small.RequestOutputLimit(protocol, false) != 4_096 ||
				small.RequestOutputLimit(protocol, true) != 4_096 {
				t.Fatal("request budgets do not respect the mode and configured ceiling")
			}
		})
	}
}

func TestContextKeyTracksEffectivePrivacyPolicy(t *testing.T) {
	provider := Provider{ID: "provider", Protocol: Chat, BaseURL: "https://example.test", APIKey: "key"}
	selected := Model{ID: "model"}

	base := ContextKey(provider, selected, "low", privacy.Settings{})
	disabled := ContextKey(provider, selected, "low", privacy.Settings{Words: []string{"private"}})
	if disabled != base {
		t.Fatal("disabled privacy settings changed the context key")
	}

	settings := privacy.Settings{Enabled: true, Secrets: true, Personal: true, Words: []string{"private"}}
	if got := ContextKey(provider, selected, "low", settings); got == base {
		t.Fatal("enabling privacy did not invalidate the context key")
	}
	if got := ContextKey(provider, selected, "low", settings.Clone()); got != ContextKey(provider, selected, "low", settings) {
		t.Fatal("equivalent privacy settings produced different context keys")
	}
	withNotify := settings.Clone()
	withNotify.Notify = true
	if ContextKey(provider, selected, "low", withNotify) != ContextKey(provider, selected, "low", settings) {
		t.Fatal("notification-only change invalidated the context key")
	}
	gateway := &Gateway{provider: provider, target: adapterTarget{model: selected}}
	gateway.SetPrivacy(settings)
	replayScope := gateway.target.scope
	gateway.SetPrivacy(withNotify)
	if gateway.target.scope != replayScope || !gateway.Privacy.Notify {
		t.Fatal("notification-only change invalidated replay or lost the notification setting")
	}

	variants := []struct {
		name   string
		mutate func(*privacy.Settings)
	}{
		{name: "secrets", mutate: func(value *privacy.Settings) { value.Secrets = false }},
		{name: "personal", mutate: func(value *privacy.Settings) { value.Personal = false }},
		{name: "shape", mutate: func(value *privacy.Settings) { value.Shape = true }},
		{name: "words", mutate: func(value *privacy.Settings) { value.Words[0] = "other" }},
		{name: "word case", mutate: func(value *privacy.Settings) { value.WordsAnyCase = true }},
		{name: "word boundary", mutate: func(value *privacy.Settings) { value.WordsWhole = true }},
		{name: "kinds", mutate: func(value *privacy.Settings) { value.Kinds = map[string]bool{"ip": true} }},
		{name: "rules", mutate: func(value *privacy.Settings) { value.Rules = []redact.Rule{{Kind: "CUSTOM", Prefix: "private-"}} }},
		{name: "actions", mutate: func(value *privacy.Settings) { value.Actions = map[string]string{"API_KEY": "block"} }},
		{name: "allow", mutate: func(value *privacy.Settings) { value.Allow = []string{"public"} }},
		{name: "local model", mutate: func(value *privacy.Settings) { value.LocalModel = true }},
		{name: "model name", mutate: func(value *privacy.Settings) { value.ModelName = "astrlink-guard" }},
		{name: "model score", mutate: func(value *privacy.Settings) { value.ModelScores = map[string]float64{"astrlink-guard": .8} }},
	}
	for _, test := range variants {
		t.Run(test.name, func(t *testing.T) {
			variant := settings.Clone()
			test.mutate(&variant)
			if got := ContextKey(provider, selected, "low", variant); got == ContextKey(provider, selected, "low", settings) {
				t.Fatal("effective privacy policy change did not invalidate the context key")
			}
		})
	}

	if got := ContextKey(provider, selected, "high", settings); got == ContextKey(provider, selected, "low", settings) {
		t.Fatal("reasoning effort change did not invalidate the context key")
	}
	if got := ContextKey(provider, Model{ID: "other"}, "low", settings); got == ContextKey(provider, selected, "low", settings) {
		t.Fatal("model change did not invalidate the context key")
	}
}
