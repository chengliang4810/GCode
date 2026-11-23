package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/agent"
	"gcode/internal/model"
)

func TestProjectMemoryWriteAndEdit(t *testing.T) {
	cwd, config := t.TempDir(), t.TempDir()
	root, err := openMemoryRoot(config, cwd)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	base := agent.CoreTools()
	tools := agent.ScopeMemoryWrites(base, root)
	call := func(tools []agent.Tool, name string, args map[string]string) error {
		t.Helper()
		raw, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}
		for _, tool := range tools {
			if tool.Definition().Name == name {
				_, err := tool.Execute(t.Context(), cwd, raw)
				return err
			}
		}
		t.Fatalf("missing tool %s", name)
		return nil
	}
	index := filepath.Join(memoryDirectory(config, cwd), memoryIndexFile)
	write := map[string]string{"file_path": index, "content": "- original"}
	if err := call(base, "Write", write); err == nil {
		t.Fatal("unscoped tool could write the memory index")
	}
	if err := call(tools, "Write", write); err != nil {
		t.Fatal(err)
	}
	if err := call(tools, "Edit", map[string]string{"file_path": index, "old_string": "original", "new_string": "updated"}); err != nil {
		t.Fatal(err)
	}
	if got := loadMemoryIndex(root.Name()); got != "- updated" {
		t.Fatalf("memory index = %q", got)
	}
	other := filepath.Join(memoryDirectory(config, cwd+"-other"), memoryIndexFile)
	if err := call(tools, "Write", map[string]string{"file_path": other, "content": "blocked"}); err == nil {
		t.Fatal("scoped tool could write another project's index")
	}
	if err := root.Symlink(t.TempDir(), "outside"); err == nil {
		path := filepath.Join(root.Name(), "outside", "note.md")
		if err := call(tools, "Write", map[string]string{"file_path": path, "content": "blocked"}); err == nil {
			t.Fatal("scoped tool followed a symlink out of the memory root")
		}
	}
}

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
	for _, want := range []string{"You are an agent working with the user toward their goals, using your own judgment along the way.", "<tools>", "<rules>", "Use Read to examine files", "Use Edit for targeted changes", "Use Write only for new files or complete rewrites", "unless replace_all is true"} {
		if !strings.Contains(basePrompt, want) {
			t.Fatalf("basePrompt missing %q", want)
		}
	}
}
