package privacy

import (
	"math"
	"testing"

	"gcode/third_party/magpie/redact"
)

func TestPrivacyDefaultsAndOwnedConfiguration(t *testing.T) {
	if defaults := DefaultSettings(); defaults.Enabled || !defaults.Secrets || defaults.LocalModel {
		t.Fatalf("unexpected defaults: %+v", defaults)
	}
	settings := Settings{
		Words: []string{"project"}, Allow: []string{"public"}, Rules: []redact.Rule{{Kind: "KEY", Prefix: "key-"}},
		Kinds: map[string]bool{"ip": true}, Actions: map[string]string{"API_KEY": "block"},
		ModelScores: map[string]float64{"astrlink-guard": .8},
	}
	copy := settings.Clone()
	copy.Words[0], copy.Allow[0], copy.Rules[0].Prefix = "changed", "changed", "changed"
	copy.Kinds["ip"], copy.Actions["API_KEY"], copy.ModelScores["astrlink-guard"] = false, "warn", .9
	if settings.Words[0] != "project" || settings.Allow[0] != "public" || settings.Rules[0].Prefix != "key-" ||
		!settings.Kinds["ip"] || settings.Actions["API_KEY"] != "block" || settings.ModelScores["astrlink-guard"] != .8 {
		t.Fatal("settings clone shares mutable state")
	}
}

func TestPrivacyRejectsInvalidRulesCategoriesAndPolicies(t *testing.T) {
	for _, settings := range []Settings{
		{Rules: []redact.Rule{{Regex: "["}}},
		{Rules: []redact.Rule{{Regex: ".*"}}},
		{Kinds: map[string]bool{"typo": true}},
		{Actions: map[string]string{"API_KEY": "ignore"}},
		{Actions: map[string]string{"api key": "block"}},
		{ModelName: "unknown"},
		{ModelScores: map[string]float64{"astrlink-guard": math.NaN()}},
		{ModelScores: map[string]float64{"privacy-filter": .1}},
		{Allow: []string{""}},
	} {
		if err := settings.Validate(); err == nil {
			t.Fatalf("accepted invalid configuration: %+v", settings)
		}
	}
}
