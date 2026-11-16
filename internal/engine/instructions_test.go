package engine

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gcode/internal/contracts"
)

func TestGlobalInstructionsBelongToConfigurationHost(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	ctx := t.Context()
	if text, err := core.ReadGlobalInstructions(ctx); err != nil || text != "" {
		t.Fatalf("missing instructions: %q %v", text, err)
	}
	if err := core.WriteGlobalInstructions(ctx, "用户全局指令"); err != nil {
		t.Fatal(err)
	}
	if text, err := core.ReadGlobalInstructions(ctx); err != nil || text != "用户全局指令" {
		t.Fatalf("instructions: %q %v", text, err)
	}
	if err := core.WriteGlobalInstructions(ctx, strings.Repeat("x", contracts.MaxGlobalInstructionsBytes+1)); err == nil {
		t.Fatal("oversized instructions accepted")
	}
	if err := core.WriteGlobalInstructions(ctx, string([]byte{0xff})); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := core.WriteGlobalInstructions(cancelled, "must not write"); err == nil {
		t.Fatal("cancelled write succeeded")
	}
	if data, err := os.ReadFile(filepath.Join(core.storage.ConfigDirectory(), "AGENTS.md")); err != nil || string(data) != "用户全局指令" {
		t.Fatalf("rejected write replaced saved instructions: %q %v", data, err)
	}
}

func TestGlobalInstructionsCannotRevealSymlinkedCredentialStore(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires local symlink privilege")
	}
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	path := filepath.Join(core.storage.ConfigDirectory(), "AGENTS.md")
	if err := os.Symlink("engine.json", path); err != nil {
		t.Fatal(err)
	}
	if _, err := core.ReadGlobalInstructions(t.Context()); err == nil {
		t.Fatal("instruction read revealed the symlinked engine configuration")
	}
	if err := core.WriteGlobalInstructions(t.Context(), "replacement"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("instruction save did not replace the link atomically: %v", err)
	}
}
