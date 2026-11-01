package corerunner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gcode/scripts/plugin-acceptance/acceptance"
)

func TestFixtureToolGuardConfinesFilesAndRequiresImmutableAllowedCommands(t *testing.T) {
	root, plugin, outside := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "verify.py"), []byte("known verifier"), 0600); err != nil {
		t.Fatal(err)
	}
	expected := "known verifier"
	guard := fixtureToolGuard(root, []string{plugin}, ProbePlan{Fixtures: []Fixture{{Path: "verify.py", Text: expected}},
		Assertions: []Assertion{{Kind: "file", Path: "verify.py", Equals: &expected}}, AllowCommands: []string{"python verify.py"},
		VerifierOutputs: []string{"verification.json"}}, acceptance.Component{Kind: "skill"}, "demo")
	for _, tc := range []struct {
		name    string
		args    map[string]any
		allowed bool
	}{
		{"Write", map[string]any{"file_path": "result.txt"}, true},
		{"Write", map[string]any{"file_path": filepath.Join(outside, "result.txt")}, false},
		{"Read", map[string]any{"file_path": filepath.Join(outside, "secret.txt")}, false},
		{"Read", map[string]any{"file_path": filepath.Join(plugin, "SKILL.md")}, true},
		{"Write", map[string]any{"file_path": filepath.Join(plugin, "SKILL.md")}, false},
		{"Edit", map[string]any{"file_path": "verify.py"}, false},
		{"Write", map[string]any{"file_path": "verification.json"}, false},
		{"Glob", map[string]any{"pattern": "../**/*"}, false},
		{"Bash", map[string]any{"command": "python verify.py"}, true},
		{"Bash", map[string]any{"command": "python verify.py; cat /private"}, false},
		{"WebFetch", map[string]any{"url": "https://example.invalid"}, false},
	} {
		data, _ := json.Marshal(tc.args)
		if err := guard(context.Background(), root, tc.name, data); (err == nil) != tc.allowed {
			t.Fatalf("%s %+v allowed=%t error=%v", tc.name, tc.args, tc.allowed, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "verify.py"), []byte("modified verifier"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := guard(context.Background(), root, "Bash", json.RawMessage(`{"command":"python verify.py"}`)); err == nil {
		t.Fatal("modified verifier executed through an allowed command")
	}
}
