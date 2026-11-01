package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/model"
)

func TestMemoryProjectSlug(t *testing.T) {
	got := memoryProjectSlug(`C:\Users\jerry\Documents\game`)
	if got != "C--Users-jerry-Documents-game" {
		t.Fatalf("memoryProjectSlug() = %q", got)
	}
	if got := memoryProjectSlug("/home/user/repo"); got != "-home-user-repo" {
		t.Fatalf("memoryProjectSlug() = %q", got)
	}
}

func TestLoadMemoryIndex(t *testing.T) {
	dir := t.TempDir()
	if got := loadMemoryIndex(dir); got != "" {
		t.Fatalf("missing index = %q", got)
	}

	if err := os.WriteFile(filepath.Join(dir, memoryIndexFile), []byte("- [Title](file.md) — hook\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := loadMemoryIndex(dir); got != "- [Title](file.md) — hook" {
		t.Fatalf("index = %q", got)
	}

	var long strings.Builder
	for i := 0; i < memoryIndexMaxLines+10; i++ {
		long.WriteString("line\n")
	}
	if err := os.WriteFile(filepath.Join(dir, memoryIndexFile), []byte(long.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	got := loadMemoryIndex(dir)
	note := strings.Index(got, "\n\n[memory index truncated")
	if note < 0 {
		t.Fatalf("truncation note missing: %q", got)
	}
	if lines := strings.Split(got[:note], "\n"); len(lines) != memoryIndexMaxLines {
		t.Fatalf("line cap not applied: %d lines", len(lines))
	}
}

func TestLoadMemoryIndexByteCap(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, memoryIndexFile), []byte(strings.Repeat("x", memoryIndexMaxBytes+100)), 0o600); err != nil {
		t.Fatal(err)
	}
	got := loadMemoryIndex(dir)
	if len(got) > memoryIndexMaxBytes+200 {
		t.Fatalf("byte cap not applied: %d bytes", len(got))
	}
	if !strings.Contains(got, "memory index truncated") {
		t.Fatalf("truncation note missing")
	}
}

func TestMemoryContextMessageMarker(t *testing.T) {
	message := memoryContextMessage("- [Title](file.md) — hook")
	if !hasMemoryIndex([]model.Message{message}) {
		t.Fatal("marker not found in context message")
	}
	if hasMemoryIndex([]model.Message{{Role: "user", Text: "plain"}}) {
		t.Fatal("marker found in unrelated message")
	}
}

func TestMemorySectionContainsDirectory(t *testing.T) {
	section := memorySection(filepath.Join("~", ".gcode", "projects", "demo", "memory"))
	for _, want := range []string{"# Memory", "MEMORY.md", "type: user | feedback | project | reference"} {
		if !strings.Contains(section, want) {
			t.Fatalf("memory section missing %q", want)
		}
	}
}

func TestBasePromptSections(t *testing.T) {
	for _, want := range []string{"# Harness", "# Simplicity & Surgical Changes", "# Change Discipline", "# Session-specific guidance", "# Context management", "Every changed line should trace directly to the user's request."} {
		if !strings.Contains(basePrompt, want) {
			t.Fatalf("basePrompt missing %q", want)
		}
	}
}
