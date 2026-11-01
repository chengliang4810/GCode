package agent

import (
	"context"
	"encoding/json"
	"errors"

	"gcode/internal/model"
)

type grepTool struct{}

type grepInput struct {
	searchOptions
	Pattern         string `json:"pattern"`
	Glob            string `json:"glob"`
	Type            string `json:"type"`
	OutputMode      string `json:"output_mode"`
	CaseInsensitive bool   `json:"-i"`
	Multiline       bool   `json:"multiline"`
	Context         int    `json:"context"`
	ShortContext    *int   `json:"-C"`
	BeforeContext   *int   `json:"-B"`
	AfterContext    *int   `json:"-A"`
	LineNumbers     *bool  `json:"-n"`
}

func (grepTool) Concurrent() bool { return true }

func (grepTool) Definition() model.Tool {
	contextLines := func(description string) any {
		return map[string]any{"type": "integer", "minimum": 0, "maximum": 100, "description": description}
	}
	return model.Tool{
		Name: "Grep",
		Description: "Search file contents with Go's regular expression syntax, the same engine ripgrep uses by default. Use Glob for file-name searches. " +
			"Defaults to files_with_matches; choose content for matching lines or count for matched-line counts per file. " +
			"Filter by a path glob and/or language type. Case-insensitive, multiline and context controls are explicit. " +
			"Pagination skips offset entries before head_limit; content counts output lines including context. " +
			"Paths default to the working directory; a single file is allowed. Traversal is deterministic. " +
			"Respects .gitignore and .ignore; symlinks are not followed. " +
			"Binary and files over 2 MiB are skipped. Non-UTF-8 matching records are skipped with a notice. " +
			"Searches have a 15-second timeout and bounded output.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string", "description": "Nonempty Go regular expression"},
				"path":    map[string]any{"type": "string", "description": "File or directory; defaults to the working directory"},
				"glob":    map[string]any{"type": "string", "description": "Path filter relative to search root, e.g. src/**/*.{go,ts}"},
				"type":    map[string]any{"type": "string", "description": "Language filter: go, ts, js, rust, py, java, c, cpp, css, html, json, yaml, md, sh, toml, sql or xml"},
				"output_mode": map[string]any{
					"type": "string", "enum": []string{"files_with_matches", "content", "count"},
					"description": "Default files_with_matches; count reports unique matched lines per file",
				},
				"-i":        map[string]any{"type": "boolean", "description": "Ignore case; default false"},
				"multiline": map[string]any{"type": "boolean", "description": "Match across lines; dot matches newlines and anchors match line boundaries"},
				"-C":        contextLines("Alias for context"),
				"context":   contextLines("Lines before and after each match; default 0"),
				"-B":        contextLines("Overrides context for preceding lines"),
				"-A":        contextLines("Overrides context for following lines"),
				"-n":        map[string]any{"type": "boolean", "description": "Include line numbers in content mode; default true"},
				"head_limit": map[string]any{
					"type": "integer", "minimum": 0, "maximum": 2000,
					"description": "Page size, default 250. Zero removes the page limit; scan/output budgets still apply",
				},
				"offset": map[string]any{"type": "integer", "minimum": 0, "description": "Entries to skip before taking the page"},
			},
			"required": []string{"pattern"}, "additionalProperties": false,
		},
	}
}

func (grepTool) Execute(ctx context.Context, cwd string, raw json.RawMessage) (model.ToolOutput, error) {
	var args grepInput
	if err := decodeSearchInput(raw, &args); err != nil {
		return model.ToolOutput{}, err
	}
	if args.Pattern == "" || len(args.Pattern) > 4096 {
		return model.ToolOutput{}, errors.New("pattern must contain 1..4096 bytes")
	}
	if args.OutputMode == "" {
		args.OutputMode = "files_with_matches"
	}
	switch args.OutputMode {
	case "content", "files_with_matches", "count":
	default:
		return model.ToolOutput{}, errors.New("output_mode must be files_with_matches, content or count")
	}
	if args.ShortContext != nil {
		args.Context = *args.ShortContext
	}
	before, after := args.Context, args.Context
	if args.BeforeContext != nil {
		before = *args.BeforeContext
	}
	if args.AfterContext != nil {
		after = *args.AfterContext
	}
	if args.Context < 0 || args.Context > 100 || before < 0 || before > 100 || after < 0 || after > 100 {
		return model.ToolOutput{}, errors.New("context values must be 0..100")
	}
	page, err := newSearchPage(args.searchOptions, 250)
	if err != nil {
		return model.ToolOutput{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return model.ToolOutput{}, err
	}
	root, err := searchRoot(cwd, args.Path, true)
	if err != nil {
		return model.ToolOutput{}, err
	}
	err = runGrep(ctx, cwd, root, args, before, after, page)
	return model.ToolOutput{Text: page.text()}, err
}
