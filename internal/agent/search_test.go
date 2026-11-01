package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchDirectorySortsAndCancels(t *testing.T) {
	root := t.TempDir()
	writeSearchFixture(t, root, map[string]string{"c.txt": "", "a.txt": "", "b.txt": ""})
	entries, err := readSearchDirectory(context.Background(), root)
	if err != nil || len(entries) != 3 ||
		entries[0].Name() != "a.txt" || entries[2].Name() != "c.txt" {
		t.Fatalf("directory order: %+v %v", entries, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readSearchDirectory(ctx, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("directory cancellation was lost: %v", err)
	}
}

func TestSearchPaginationResumesAtByteLimit(t *testing.T) {
	page, err := newSearchPage(searchOptions{HeadLimit: new(int)}, 100)
	if err != nil {
		t.Fatal(err)
	}
	for {
		err = page.add(strings.Repeat("x", 1000))
		if err != nil {
			break
		}
	}
	if !errors.Is(err, errSearchPageFull) || !page.truncated || len(page.lines) != 63 ||
		!strings.Contains(page.text(), "offset: 63") {
		t.Fatalf("byte-limit continuation lost records: %d %v %s", len(page.lines), err, page.text())
	}
}

func TestSearchResultPathsAreDirectlyReadable(t *testing.T) {
	cwd, outside := t.TempDir(), t.TempDir()
	writeSearchFixture(t, outside, map[string]string{"outside.go": "hit"})
	for _, tool := range []Tool{globTool{}, grepTool{}} {
		pattern := "*.go"
		if tool.Definition().Name == "Grep" {
			pattern = "hit"
		}
		raw, _ := json.Marshal(map[string]any{"pattern": pattern, "path": outside})
		output, err := tool.Execute(context.Background(), cwd, raw)
		path := filepath.FromSlash(output.Text)
		if err != nil || !filepath.IsAbs(path) {
			t.Fatalf("outside path became ambiguous: %q %v", output.Text, err)
		}
		if _, err := os.ReadFile(path); err != nil {
			t.Fatalf("result cannot be read: %q %v", path, err)
		}
	}
}

// The search tools work on a machine with no search program: the tree is
// walked here and the patterns are Go's own.
func TestSearchToolsNeedNoExternalBinary(t *testing.T) {
	root := t.TempDir()
	writeSearchFixture(t, root, map[string]string{"one.go": "package main\n"})
	t.Setenv("PATH", "")
	out, err := (globTool{}).Execute(context.Background(), root, json.RawMessage(`{"pattern":"*.go"}`))
	if err != nil || out.Text != "one.go" {
		t.Fatalf("glob without a search binary: %q %v", out.Text, err)
	}
	out, err = (grepTool{}).Execute(context.Background(), root, json.RawMessage(`{"pattern":"package"}`))
	if err != nil || out.Text != "one.go" {
		t.Fatalf("grep without a search binary: %q %v", out.Text, err)
	}
}

// An ignore file of a subdirectory keeps its rules to that subdirectory.
func TestSearchToolsHonorNestedIgnoreFiles(t *testing.T) {
	root := t.TempDir()
	writeSearchFixture(t, root, map[string]string{
		"src/.gitignore":    "local.go\n",
		"src/local.go":      "hit\n",
		"src/kept.go":       "hit\n",
		"other/local.go":    "hit\n",
		"src/deep/local.go": "hit\n",
		"src/deep/kept.go":  "hit\n",
	})
	out, err := (grepTool{}).Execute(context.Background(), root, json.RawMessage(`{"pattern":"hit"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.Text, "src/local.go") || strings.Contains(out.Text, "src/deep/local.go") {
		t.Fatalf("nested ignore file did not apply: %q", out.Text)
	}
	if !strings.Contains(out.Text, "src/kept.go") || !strings.Contains(out.Text, "other/local.go") {
		t.Fatalf("nested ignore file reached too far: %q", out.Text)
	}
}

// Searching a subdirectory keeps the ignore rules of the directories above
// it, including the repository's own exclude file.
func TestSearchToolsHonorParentIgnoreFiles(t *testing.T) {
	repo := t.TempDir()
	writeSearchFixture(t, repo, map[string]string{
		".gitignore":        "ignored.txt\n",
		".git/info/exclude": "excluded.txt\n",
		"sub/ignored.txt":   "hit\n",
		"sub/excluded.txt":  "hit\n",
		"sub/kept.txt":      "hit\n",
	})
	sub := filepath.Join(repo, "sub")
	out, err := (grepTool{}).Execute(context.Background(), sub, json.RawMessage(`{"pattern":"hit"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.Text, "ignored.txt") || strings.Contains(out.Text, "excluded.txt") {
		t.Fatalf("parent ignore rules did not apply: %q", out.Text)
	}
	if !strings.Contains(out.Text, "kept.txt") {
		t.Fatalf("parent ignore rules reached too far: %q", out.Text)
	}
}
