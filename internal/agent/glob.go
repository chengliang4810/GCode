package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gcode/internal/model"
)

type globTool struct{}

func (globTool) Concurrent() bool { return true }

func (globTool) Definition() model.Tool {
	return model.Tool{
		Name: "Glob",
		Description: "Find files by path pattern. Use this for file-name searches and grep for file contents. " +
			"Supports *, ?, character classes, ** directory segments and brace alternatives such as src/**/*.{go,ts}. " +
			"Bare names such as *.go match at any depth; src/*.go matches only immediate files in src. " +
			"Path defaults to the working directory. Results are sorted by modification time, newest first. " +
			"Pagination skips offset entries before taking head_limit. Nested .git/node_modules and symlinks are skipped; " +
			"Respects .gitignore and .ignore. Searches have a 15-second/100000-file scan budget and bounded output.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string", "description": "Relative path pattern, e.g. internal/**/*.go"},
				"path":    map[string]any{"type": "string", "description": "Search directory; defaults to the working directory"},
				"head_limit": map[string]any{
					"type": "integer", "minimum": 0, "maximum": 2000,
					"description": "Page size, default 100. Zero removes the page limit; scan/output budgets still apply",
				},
				"offset": map[string]any{"type": "integer", "minimum": 0, "description": "Number of matching files to skip"},
			},
			"required": []string{"pattern"}, "additionalProperties": false,
		},
	}
}

func (globTool) Execute(ctx context.Context, cwd string, raw json.RawMessage) (model.ToolOutput, error) {
	var args struct {
		searchOptions
		Pattern string `json:"pattern"`
	}
	if err := decodeSearchInput(raw, &args); err != nil {
		return model.ToolOutput{}, err
	}
	patternText := args.Pattern
	if filepath.IsAbs(patternText) {
		normalized := filepath.ToSlash(patternText)
		prefix := normalized
		if wildcard := strings.IndexAny(prefix, "*?[{"); wildcard >= 0 {
			prefix = prefix[:wildcard]
		}
		at := strings.LastIndex(prefix, "/")
		if at >= 0 {
			args.Path = filepath.FromSlash(normalized[:at+1])
			patternText = normalized[at+1:]
		}
	}
	pattern, err := compilePathPattern(patternText)
	if err != nil {
		return model.ToolOutput{}, err
	}
	page, err := newSearchPage(args.searchOptions, 100)
	if err != nil {
		return model.ToolOutput{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return model.ToolOutput{}, err
	}
	root, err := searchRoot(cwd, args.Path, false)
	if err != nil {
		return model.ToolOutput{}, err
	}
	type match struct {
		name     string
		modified time.Time
	}
	// A pattern that names a directory chain begins its walk below the
	// search root: nothing outside that chain can match.
	matches := []match{}
	err = walkSearchFiles(ctx, globWalkRoot(root, patternText), searchWalkOptions{hidden: true}, func(name string, info os.FileInfo) error {
		if pattern.matches(searchPatternPath(root, name)) {
			matches = append(matches, match{name, info.ModTime()})
		}
		return nil
	})
	if err != nil {
		return model.ToolOutput{}, err
	}
	slices.SortFunc(matches, func(a, b match) int {
		if a.modified.Before(b.modified) {
			return 1
		}
		if a.modified.After(b.modified) {
			return -1
		}
		return strings.Compare(a.name, b.name)
	})
	for _, item := range matches {
		if err := ctx.Err(); err != nil {
			return model.ToolOutput{}, err
		}
		if err := page.add(displaySearchPath(cwd, item.name)); err != nil {
			break
		}
	}
	return model.ToolOutput{Text: page.text()}, err
}

// globWalkRoot returns the directory a pattern's walk begins at: the
// literal directory chain before its first wildcard, below root. A pattern
// that starts with a wildcard, or whose literal part is not a whole chain
// of directories, begins at root, which is always right and only slower.
func globWalkRoot(root, pattern string) string {
	pattern = strings.ReplaceAll(pattern, `\`, "/")
	if pattern == "" || strings.HasPrefix(pattern, "/") || strings.Contains(pattern, ":") {
		return root
	}
	literal := pattern
	if meta := strings.IndexAny(pattern, "*?[{"); meta >= 0 {
		literal = pattern[:meta]
	}
	at := strings.LastIndex(literal, "/")
	if at <= 0 {
		return root
	}
	directory := filepath.Join(root, filepath.FromSlash(literal[:at]))
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return root
	}
	// An ignored directory must stay ignored: the walk from root leaves it
	// out, and this one must not find what it holds.
	if ignoredBy(readIgnoreRules(root), directory, true) {
		return root
	}
	return directory
}
