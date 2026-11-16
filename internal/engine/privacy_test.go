package engine

import (
	"reflect"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/privacy"
	"gcode/third_party/magpie/redact"
)

func TestPrivacySettingsPersistAndBindAtHostBoundary(t *testing.T) {
	home := t.TempDir()
	core := openStorageTestCore(t, home)
	if core.state.Settings.Privacy.Enabled {
		t.Fatal("privacy must default off")
	}
	settings := core.state.Settings
	settings.Privacy = privacy.Settings{
		Enabled: true, Secrets: true, Personal: true, Words: []string{"内部项目"}, WordsWhole: true,
		Kinds: map[string]bool{"ip": true}, Rules: []redact.Rule{{Kind: "KEY", Prefix: "private-"}},
		Actions: map[string]string{"API_KEY": "block"}, Allow: []string{"public@example.com"},
		ModelName: "astrlink-guard", ModelScores: map[string]float64{"astrlink-guard": .9},
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	core = openStorageTestCore(t, home)
	defer core.Close()
	if !reflect.DeepEqual(core.state.Settings.Privacy, settings.Privacy) {
		t.Fatalf("privacy configuration did not survive reopen: %+v", core.state.Settings.Privacy)
	}
	if len(core.state.PrivacyModels) != 2 {
		t.Fatal("local models must be projected by the host")
	}
	provider := model.Provider{ID: "p", Name: "P", Protocol: model.Chat, BaseURL: "http://127.0.0.1", Models: []model.Model{{ID: "m"}}}
	core.mu.Lock()
	gateway, err := core.bindModelLocked(provider, provider.Models[0])
	core.mu.Unlock()
	if err != nil || !reflect.DeepEqual(gateway.Privacy, settings.Privacy) {
		t.Fatalf("host did not freeze privacy policy: %+v %v", gateway, err)
	}
	settings.Privacy.Words[0] = "new-word"
	if gateway.Privacy.Words[0] != "内部项目" {
		t.Fatal("gateway aliases UI-owned settings")
	}
}

func TestPrivacyCommandSnapshotsDoNotAlias(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	settings := core.state.Settings
	settings.Privacy.Words = []string{"private"}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	settings.Privacy.Words[0] = "mutated"
	snapshot, err := core.Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Settings.Privacy.Words[0] != "private" {
		t.Fatal("command retained caller-owned masking words")
	}
	snapshot.Settings.Privacy.Words[0] = "mutated snapshot"
	next, _ := core.Snapshot(t.Context())
	if next.Settings.Privacy.Words[0] != "private" {
		t.Fatal("snapshot exposes mutable masking policy")
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "privacy.model.download", ID: "unknown"}); err == nil {
		t.Fatal("unknown model ID accepted")
	}
}
