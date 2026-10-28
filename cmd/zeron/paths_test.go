package main

import (
	"path/filepath"
	"testing"
)

func TestNativeDataNeverUsesZeronDisplayName(t *testing.T) {
	config := t.TempDir()
	state := filepath.Join(config, "gcode-zeron-ui", "state.json")
	if got := nativeDataDirectory(config, state, ""); got != filepath.Join(config, "gcode-zeron-ui", "native") {
		t.Fatalf("native store differs from independent application root: %s", got)
	}
	if got := nativeDataDirectory(config, "", "default"); got != filepath.Join(config, "gcode-zeron-ui", "acceptance", "default") {
		t.Fatalf("scenario profile is not isolated: %s", got)
	}
	custom := filepath.Join(config, "custom", "state.json")
	if got := nativeDataDirectory(config, custom, ""); got != filepath.Join(config, "custom", "native") {
		t.Fatal("explicit data directory was ignored")
	}
}

func TestUnknownScenarioCannotBecomeAProfilePath(t *testing.T) {
	for _, scenario := range []string{"../Zeron", "../../Zeron", "C:\\Zeron", "unknown"} {
		if validScenario(scenario) {
			t.Fatalf("invalid profile scenario accepted: %s", scenario)
		}
	}
	if !validScenario("showcase") || !validScenario("live") || !validScenario("transcript") || !validScenario("") {
		t.Fatal("normal scenarios rejected")
	}
}
