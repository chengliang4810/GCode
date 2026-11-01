package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/mcp"
	"gcode/internal/storage"
)

func TestProcessEnvironmentStaysAtOwningRoot(t *testing.T) {
	core, home := terminalStorageCore(t)
	oldID := core.storage.ActiveRootID()
	oldDirectory := core.storage.Directory()
	next := filepath.Join(t.TempDir(), "next")
	if err := core.storage.RequestDirectory(next); err != nil {
		t.Fatal(err)
	}
	if err := core.storage.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core.storage = store
	for rootID, directory := range map[string]string{oldID: oldDirectory, store.ActiveRootID(): next} {
		env, err := core.processEnvironment(rootID, "session-owner")
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.ToSlash(filepath.Join(directory, "tmp", "processes", "session-owner"))
		if env["TEMP"] != want || env["TMP"] != want || env["TMPDIR"] != want {
			t.Fatalf("wrong root environment: %v want %s", env, want)
		}
		if info, err := os.Stat(filepath.FromSlash(want)); err != nil || !info.IsDir() {
			t.Fatal("process directory not created", err)
		}
	}
	if _, err := core.processEnvironment(oldID, "../outside"); err == nil {
		t.Fatal("unsafe process owner accepted")
	}
	if _, err := core.processEnvironment("unavailable-root", "owner"); err == nil {
		t.Fatal("unavailable owner silently fell back to active root")
	}
}

func TestMCPProcessEnvironmentKeepsExplicitUserPriority(t *testing.T) {
	defaults := map[string]string{"TEMP": "host-scratch", "TMP": "host-scratch", "TMPDIR": "host-scratch"}
	original := mcp.Config{
		Env:        map[string]string{"TEMP": "user-directory", "CUSTOM": "user"},
		ProcessEnv: map[string]string{"HOST_SETTING": "host-setting"},
	}
	merged := mcpProcessEnvironment(original, defaults)
	if merged.Env["TEMP"] != "user-directory" || merged.Env["TMP"] != "" || merged.ProcessEnv["TMP"] != "host-scratch" || merged.ProcessEnv["HOST_SETTING"] != "host-setting" {
		t.Fatalf("wrong environment layers: %+v", merged)
	}
	merged.Env["CUSTOM"] = "modified"
	merged.ProcessEnv["HOST_SETTING"] = "modified"
	merged.ProcessEnv["TMP"] = "modified"
	if original.Env["CUSTOM"] != "user" || len(original.Env) != 2 {
		t.Fatal("runtime defaults mutated persistent MCP configuration")
	}
	if original.ProcessEnv["HOST_SETTING"] != "host-setting" || defaults["TMP"] != "host-scratch" {
		t.Fatal("runtime defaults retain mutable map aliases")
	}
	if got := mcp.SanitizeText("host-scratch/artifact.json user-directory", merged); got != "host-scratch/artifact.json [redacted]" {
		t.Fatalf("host scratch and explicit credentials were conflated: %s", got)
	}
	wire, err := json.Marshal(merged)
	if err != nil || strings.Contains(string(wire), "host-scratch") || strings.Contains(string(wire), "host-setting") || strings.Contains(string(wire), "ProcessEnv") {
		t.Fatalf("host runtime metadata escaped configuration JSON: %s %v", wire, err)
	}
}
