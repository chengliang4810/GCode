package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGlobScopesPatternsAndPages(t *testing.T) {
	root := t.TempDir()
	writeSearchFixture(t, root, map[string]string{
		"root.go": "", "src/a.go": "", "src/b.ts": "", "src/deep/c.go": "",
		"other/d.go": "", ".hidden.go": "", ".git/secret.go": "", "node_modules/generated.go": "",
	})
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{name: "immediate children", args: map[string]any{"pattern": "src/*.go"}, want: "src/a.go"},
		{name: "recursive", args: map[string]any{"pattern": "src/**/*.go"}, want: "src/a.go\nsrc/deep/c.go"},
		{name: "brace alternatives", args: map[string]any{"pattern": "src/*.{go,ts}"}, want: "src/a.go\nsrc/b.ts"},
		{name: "explicit root", args: map[string]any{"pattern": "*.go", "path": "src"}, want: "src/a.go\nsrc/deep/c.go"},
		{name: "offset then limit", args: map[string]any{
			"pattern": "src/**/*.{go,ts}", "offset": 1, "head_limit": 1,
		}, want: "src/b.ts\n[results truncated; continue with offset: 2]"},
		{name: "exact last page", args: map[string]any{
			"pattern": "src/**/*.{go,ts}", "offset": 2, "head_limit": 1,
		}, want: "src/deep/c.go"},
		{name: "offset exhausted", args: map[string]any{
			"pattern": "src/**/*.{go,ts}", "offset": 9,
		}, want: "No entries at this offset."},
		{name: "no match", args: map[string]any{"pattern": "*.missing"}, want: "No matches."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(tc.args)
			output, err := (globTool{}).Execute(context.Background(), root, raw)
			if err != nil || output.Text != tc.want {
				t.Fatalf("glob output %q, want %q, err=%v", output.Text, tc.want, err)
			}
		})
	}
	raw := json.RawMessage(`{"pattern":"*.go","head_limit":0}`)
	output, err := (globTool{}).Execute(context.Background(), root, raw)
	if err != nil || !strings.Contains(output.Text, ".hidden.go") ||
		strings.Contains(output.Text, "secret.go") || strings.Contains(output.Text, "generated.go") {
		t.Fatalf("wrong traversal/hidden exclusions: %q %v", output.Text, err)
	}
}

func TestGlobValidatesInputAndCancellation(t *testing.T) {
	root := t.TempDir()
	writeSearchFixture(t, root, map[string]string{"one.go": ""})
	for _, raw := range []string{
		`{}`, `{"pattern":"["}`, `{"pattern":"*.go","head_limit":-1}`,
		`{"pattern":"*.go","offset":-1}`, `{"pattern":"*.go","head_limit":2001}`,
		`{"pattern":"*.go","path":"missing"}`, `{"pattern":"*.go","path":"one.go"}`,
		`{"pattern":"*.go","old_field":true}`, `{"pattern":"*.go"} {}`,
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := (globTool{}).Execute(context.Background(), root, json.RawMessage(raw)); err == nil {
				t.Fatal("invalid glob request accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (globTool{}).Execute(ctx, root, json.RawMessage(`{"pattern":"*.go"}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestGlobSortsNewestFirstAndAcceptsAbsolutePattern(t *testing.T) {
	root := t.TempDir()
	writeSearchFixture(t, root, map[string]string{"old.go": "", "new.go": ""})
	when := time.Unix(1800000000, 0)
	if err := os.Chtimes(filepath.Join(root, "new.go"), when, when); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"pattern": filepath.Join(root, "*.go")})
	out, err := (globTool{}).Execute(context.Background(), root, raw)
	if err != nil || out.Text != "new.go\nold.go" {
		t.Fatalf("mtime/absolute pattern: %q %v", out.Text, err)
	}
}

func TestGlobHonorsIgnoreFiles(t *testing.T) {
	root := t.TempDir()
	writeSearchFixture(t, root, map[string]string{
		".gitignore":         "ignored/\n",
		".ignore":            "local.go\n",
		"ignored/private.go": "",
		"local.go":           "",
		"visible.go":         "",
	})
	out, err := (globTool{}).Execute(context.Background(), root, json.RawMessage(`{"pattern":"*.go"}`))
	if err != nil || out.Text != "visible.go" {
		t.Fatalf("ignored files leaked: %q %v", out.Text, err)
	}
}

func TestSearchToolsDoNotFollowSymlinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	writeSearchFixture(t, outside, map[string]string{"secret.go": "hidden-match"})
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	for _, tool := range []Tool{globTool{}, grepTool{}} {
		args := `{"pattern":"*.go"}`
		if tool.Definition().Name == "Grep" {
			args = `{"pattern":"hidden-match"}`
		}
		output, err := tool.Execute(context.Background(), root, json.RawMessage(args))
		if err != nil || output.Text != "No matches." {
			t.Fatalf("followed linked directory: %q %v", output.Text, err)
		}
	}
}

func writeSearchFixture(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		when := time.Unix(1700000000, 0)
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatal(err)
		}
	}
}

// A pattern that names a directory begins its walk there; one that starts
// with a wildcard begins at the search root.
func TestGlobWalkRootPrunesToLiteralDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ pattern, want string }{
		{"src/**/*.go", filepath.Join(root, "src")},
		{"src/*.go", filepath.Join(root, "src")},
		{"src/deep/a.go", filepath.Join(root, "src", "deep")},
		{"*.go", root},
		{"**/*.go", root},
		{"missing/*.go", root},
	}
	for _, tc := range cases {
		if got := globWalkRoot(root, tc.pattern); got != tc.want {
			t.Errorf("globWalkRoot(%q) = %q, want %q", tc.pattern, got, tc.want)
		}
	}
}
