package corerunner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupRemovesOnlyExactOwnedHome(t *testing.T) {
	probe, err := os.MkdirTemp(t.TempDir(), "probe-")
	if err != nil {
		t.Fatal(err)
	}
	probe, err = filepath.EvalSymlinks(probe)
	if err != nil {
		t.Fatal(err)
	}
	home, cwd := filepath.Join(probe, "home"), filepath.Join(probe, "cwd")
	for path, text := range map[string]string{
		filepath.Join(home, ".gcode", "engine.json"): "COPIED_PRIVATE_KEY",
		filepath.Join(cwd, "business-result.txt"):    "evidence",
		filepath.Join(probe, "snapshot.json"):        "redacted evidence",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := removeOwnedProbeHome(probe, cwd); err == nil {
		t.Fatal("cleanup accepted a path other than the exact home")
	}
	if err := removeOwnedProbeHome(probe, home); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatal("private copied credential home remains")
	}
	for _, path := range []string{filepath.Join(cwd, "business-result.txt"), filepath.Join(probe, "snapshot.json")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal("cleanup removed business or redacted audit evidence")
		}
	}
}
