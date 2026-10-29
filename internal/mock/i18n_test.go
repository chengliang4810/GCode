package mock

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/i18n"
)

func TestLanguagePersistsAndOldSettingsDefaultToChinese(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.json")
	server, err := New(Options{StatePath: path, Instant: true})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := server.Snapshot(ctx)
	snapshot.Settings.General.Language = i18n.English
	if err := server.UpdateSettings(ctx, snapshot.Settings); err != nil {
		t.Fatal(err)
	}
	server.Close()
	server, err = New(Options{StatePath: path, Instant: true})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ = server.Snapshot(ctx)
	if snapshot.Settings.General.Language != i18n.English {
		t.Fatal("English selection was not restored")
	}
	server.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	// Remove just the new field, as in state written before i18n was added.
	savedSnapshot := saved["snapshot"].(map[string]any)
	general := savedSnapshot["settings"].(map[string]any)["general"].(map[string]any)
	delete(general, "language")
	data, _ = json.Marshal(saved)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	server, err = New(Options{StatePath: path, Instant: true})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	snapshot, _ = server.Snapshot(ctx)
	if snapshot.Settings.General.Language != i18n.Chinese {
		t.Fatal("older state must default to Chinese")
	}
}
