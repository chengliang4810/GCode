package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestGrepFiltersModesAndPagination(t *testing.T) {
	root := t.TempDir()
	writeSearchFixture(t, root, map[string]string{
		"a.go": "Alpha\nhit hit\nhit\nlast\n", "b.go": "HIT\nnone\n",
		"src/c.ts": "hit\n", "src/d.tsx": "hit\n",
		"node_modules/generated.go": "hit\n", ".git/secret.go": "hit\n",
	})
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{name: "default file paths", args: map[string]any{"pattern": "hit", "type": "go"}, want: "a.go"},
		{name: "case insensitive", args: map[string]any{
			"pattern": "hit", "type": "go", "-i": true,
		}, want: "a.go\nb.go"},
		{name: "glob filter", args: map[string]any{"pattern": "hit", "glob": "src/*.{ts,tsx}"}, want: "src/c.ts\nsrc/d.tsx"},
		{name: "glob overrides type", args: map[string]any{
			"pattern": "hit", "glob": "src/*", "type": "go",
		}, want: "src/c.ts\nsrc/d.tsx"},
		{name: "unique matched line count", args: map[string]any{
			"pattern": "hit", "type": "go", "output_mode": "count",
		}, want: "a.go:2\nTotal: 2"},
		{name: "single file", args: map[string]any{
			"pattern": "hit", "path": "a.go", "output_mode": "content",
		}, want: "a.go:2:hit hit\na.go:3:hit"},
		{name: "content without line numbers", args: map[string]any{
			"pattern": "hit", "path": "a.go", "output_mode": "content", "-n": false,
		}, want: "a.go:hit hit\na.go:hit"},
		{name: "file pagination", args: map[string]any{
			"pattern": "hit", "offset": 1, "head_limit": 1,
		}, want: "src/c.ts\n[results truncated; continue with offset: 2]"},
		{name: "content pagination", args: map[string]any{
			"pattern": "hit", "path": "a.go", "output_mode": "content", "offset": 1, "head_limit": 1,
		}, want: "a.go:3:hit"},
		{name: "count pagination", args: map[string]any{
			"pattern": "hit", "output_mode": "count", "offset": 1, "head_limit": 1,
		}, want: "src/c.ts:1\n[results truncated; continue with offset: 2]\nTotal: 4"},
		{name: "offset beyond matches", args: map[string]any{
			"pattern": "hit", "offset": 10,
		}, want: "No entries at this offset."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(tc.args)
			output, err := (grepTool{}).Execute(context.Background(), root, raw)
			if err != nil || output.Text != tc.want {
				t.Fatalf("grep output %q, want %q, err=%v", output.Text, tc.want, err)
			}
		})
	}
}

func TestGrepContextMergesWindowsWithoutDuplicates(t *testing.T) {
	root := t.TempDir()
	writeSearchFixture(t, root, map[string]string{"one.txt": "one\nhit\nthree\nhit\nfive\nsix\n"})
	for _, tc := range []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "merged context",
			args: map[string]any{"pattern": "hit", "output_mode": "content", "context": 1},
			want: "one.txt-1-one\none.txt:2:hit\none.txt-3-three\none.txt:4:hit\none.txt-5-five",
		},
		{
			name: "asymmetric override",
			args: map[string]any{
				"pattern": "hit", "output_mode": "content", "context": 1, "-B": 0, "-A": 0,
			},
			want: "one.txt:2:hit\none.txt:4:hit",
		},
		{
			name: "context pagination",
			args: map[string]any{
				"pattern": "hit", "output_mode": "content", "context": 1, "offset": 2, "head_limit": 2,
			},
			want: "one.txt-3-three\none.txt:4:hit\n[results truncated; continue with offset: 4]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(tc.args)
			output, err := (grepTool{}).Execute(context.Background(), root, raw)
			if err != nil || output.Text != tc.want {
				t.Fatalf("context output %q, want %q, err=%v", output.Text, tc.want, err)
			}
		})
	}
}

func TestGrepMultilineAndCRLF(t *testing.T) {
	root := t.TempDir()
	writeSearchFixture(t, root, map[string]string{"one.go": "前言\r\nfunc hello() {\r\n  return\r\n}\r\n", "empty.go": ""})
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{name: "single line cannot cross", args: map[string]any{"pattern": "hello.*return"}, want: "No matches."},
		{name: "multiline file mode", args: map[string]any{"pattern": "hello.*return", "multiline": true}, want: "one.go"},
		{name: "multiline content", args: map[string]any{
			"pattern": "hello.*return", "multiline": true, "output_mode": "content",
		}, want: "one.go:2:func hello() {\none.go:3:  return"},
		{name: "multiline unique lines", args: map[string]any{
			"pattern": "hello.*return", "multiline": true, "output_mode": "count",
		}, want: "one.go:2\nTotal: 2"},
		{name: "empty file does not contain a line", args: map[string]any{
			"pattern": "^", "path": "empty.go", "multiline": true,
		}, want: "No matches."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(tc.args)
			output, err := (grepTool{}).Execute(context.Background(), root, raw)
			if err != nil || output.Text != tc.want {
				t.Fatalf("multiline output %q, want %q, err=%v", output.Text, tc.want, err)
			}
		})
	}
}

func TestGrepReportsSkippedFilesAndBoundsOutput(t *testing.T) {
	root := t.TempDir()
	writeSearchFixture(t, root, map[string]string{
		"binary.bin": "\x00hit", "invalid.txt": "\xffhit",
		"large.txt": strings.Repeat("x", maxSearchFileBytes+1),
		"long.txt":  "hit" + strings.Repeat("中文", 2000),
	})
	output, err := (grepTool{}).Execute(context.Background(), root, json.RawMessage(`{
		"pattern":"hit","output_mode":"content","head_limit":0
	}`))
	if err != nil || !strings.Contains(output.Text, "skipped 1 ") ||
		!strings.Contains(output.Text, "[line truncated]") || !utf8.ValidString(output.Text) {
		t.Fatalf("bounded file handling failed: %q %v", output.Text, err)
	}
	writeSearchFixture(t, root, map[string]string{
		"output.txt": strings.Repeat("hit"+strings.Repeat("x", 500)+"\n", 150),
	})
	output, err = (grepTool{}).Execute(context.Background(), root, json.RawMessage(`{
		"pattern":"hit","path":"output.txt","output_mode":"content","head_limit":0
	}`))
	if err != nil || len(output.Text) > maxSearchOutput+150 || !strings.Contains(output.Text, "[results truncated;") {
		t.Fatalf("output budget was not enforced: bytes=%d %v", len(output.Text), err)
	}
}

func TestGrepMultilineDoesNotInventTrailingEmptyLines(t *testing.T) {
	root := t.TempDir()
	writeSearchFixture(t, root, map[string]string{"one.txt": "nonempty\n", "two.txt": "nonempty\n\n"})
	for _, mode := range []string{"files_with_matches", "content", "count"} {
		t.Run(mode, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{
				"pattern": "^$", "multiline": true, "output_mode": mode, "path": "one.txt",
			})
			output, err := (grepTool{}).Execute(context.Background(), root, raw)
			if err != nil || output.Text != "No matches." {
				t.Fatalf("trailing newline invented a matched line: %q %v", output.Text, err)
			}
			raw, _ = json.Marshal(map[string]any{
				"pattern": "^$", "multiline": true, "output_mode": mode, "path": "two.txt",
			})
			output, err = (grepTool{}).Execute(context.Background(), root, raw)
			want := "two.txt"
			if mode == "content" {
				want = "two.txt:2:"
			} else if mode == "count" {
				want = "two.txt:1\nTotal: 1"
			}
			if err != nil || output.Text != want {
				t.Fatalf("real blank line was lost: %q want %q %v", output.Text, want, err)
			}
		})
	}
}

func TestGrepMultilineCountsLinesInsteadOfRegexOccurrences(t *testing.T) {
	root := t.TempDir()
	writeSearchFixture(t, root, map[string]string{
		"one.txt": strings.Repeat("x", 100001) + "\n" + strings.Repeat("x", 100001) + "\n",
	})
	for _, mode := range []string{"files_with_matches", "content", "count"} {
		t.Run(mode, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{
				"pattern": ".", "multiline": true, "output_mode": mode,
			})
			output, err := (grepTool{}).Execute(context.Background(), root, raw)
			if err != nil {
				t.Fatalf("dense valid file was rejected: %v", err)
			}
			if mode == "count" && output.Text != "one.txt:2\nTotal: 2" {
				t.Fatalf("count is not unique matched lines: %q", output.Text)
			}
			if mode == "content" && (!strings.Contains(output.Text, "one.txt:1:") ||
				!strings.Contains(output.Text, "one.txt:2:")) {
				t.Fatalf("dense matching lines were lost: %q", output.Text)
			}
		})
	}
}

func TestGrepRejectsInvalidOptionsAndCancels(t *testing.T) {
	root := t.TempDir()
	for _, raw := range []string{
		`{}`, `{"pattern":"["}`, `{"pattern":"hit","type":"made-up"}`,
		`{"pattern":"hit","glob":"["}`, `{"pattern":"hit","output_mode":"other"}`,
		`{"pattern":"hit","offset":-1}`, `{"pattern":"hit","head_limit":-1}`,
		`{"pattern":"hit","context":101}`, `{"pattern":"hit","-B":-1}`,
		`{"pattern":"hit","-A":101}`, `{"pattern":"hit","case_insensitive":true}`,
		`{"pattern":"hit","path":"missing"}`, `{"pattern":"hit"} {}`,
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := (grepTool{}).Execute(context.Background(), root, json.RawMessage(raw)); err == nil {
				t.Fatal("invalid grep options accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (grepTool{}).Execute(ctx, root, json.RawMessage(`{"pattern":"hit"}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestGrepUsesIgnoreFilesAndPublicFlags(t *testing.T) {
	root := t.TempDir()
	writeSearchFixture(t, root, map[string]string{
		".gitignore":  "ignored.txt\n",
		"ignored.txt": "MATCH\n",
		"visible.txt": "before\nMatch\nafter\n",
	})
	out, err := (grepTool{}).Execute(context.Background(), root, json.RawMessage(`{"pattern":"match","-i":true,"-n":false,"-C":1,"output_mode":"content"}`))
	if err != nil || out.Text != "visible.txt-before\nvisible.txt:Match\nvisible.txt-after" {
		t.Fatalf("public flags or .gitignore semantics: %q %v", out.Text, err)
	}
}
