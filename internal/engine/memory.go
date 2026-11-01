package engine

import (
	"os"
	"path/filepath"
	"strings"

	"gcode/internal/model"
)

const (
	memoryIndexFile     = "MEMORY.md"
	memoryIndexMarker   = "gcode:memory-index"
	memoryIndexMaxLines = 200
	memoryIndexMaxBytes = 25_000
)

// memoryDirectory mirrors the reference implementation's per-project layout:
// <config>/projects/<sanitized-cwd>/memory/. The config directory is the
// engine anchor (~/.gcode by default), so memory follows a custom data
// directory the same way the global AGENTS.md file does.
func memoryDirectory(configDir, cwd string) string {
	return filepath.Join(configDir, "projects", memoryProjectSlug(cwd), "memory")
}

func memoryProjectSlug(cwd string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			return r
		default:
			return '-'
		}
	}, cwd)
}

// loadMemoryIndex returns the MEMORY.md index trimmed to the reference caps
// (200 lines and 25 KiB; the line cap applies first, the byte cap cuts at the
// last newline before the limit) with a truncation note appended when a cap
// fired. A missing or empty index returns "".
func loadMemoryIndex(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, memoryIndexFile))
	if err != nil {
		return ""
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return ""
	}
	truncated := false
	if lines := strings.Split(trimmed, "\n"); len(lines) > memoryIndexMaxLines {
		trimmed = strings.Join(lines[:memoryIndexMaxLines], "\n")
		truncated = true
	}
	if len(trimmed) > memoryIndexMaxBytes {
		cut := trimmed[:memoryIndexMaxBytes]
		if i := strings.LastIndex(cut, "\n"); i >= 0 {
			cut = cut[:i]
		}
		trimmed = cut
		truncated = true
	}
	if truncated {
		trimmed += "\n\n[memory index truncated: exceeded the 200-line / 25 KB cap for " + memoryIndexFile + "; trim the index]"
	}
	return trimmed
}

func hasMemoryIndex(messages []model.Message) bool {
	for _, message := range messages {
		if strings.Contains(message.Text, memoryIndexMarker) {
			return true
		}
	}
	return false
}

func memoryContextMessage(index string) model.Message {
	return model.Message{Role: "user", Text: "<system-reminder>\n" + memoryIndexMarker + "\n\n" + index + "\n</system-reminder>"}
}

// memorySection renders the persistent-memory instructions for the system
// prompt (the official English memory text; AGENTS.md replaces CLAUDE.md in
// the do-not-save list, and recalled-memory trust lives in basePrompt's
// Harness section). Only the directory is filled in at runtime.
func memorySection(dir string) string {
	frontmatter := strings.Join([]string{
		"```markdown",
		"---",
		"name: <short-kebab-case-slug>",
		"description: <one-line summary, used to decide relevance during recall>",
		"metadata:",
		"  type: user | feedback | project | reference",
		"---",
		"",
		"<the fact; for feedback/project, follow with **Why:** and **How to apply:** lines. Link related memories with [[their-name]].>",
		"```",
	}, "\n")
	return strings.Join([]string{
		"# Memory",
		"",
		"You have a persistent file-based memory at `" + dir + "`. This directory already exists — write to it directly with the Write tool (do not run mkdir or check for its existence). Each memory is one file holding one fact, with frontmatter:",
		"",
		frontmatter,
		"",
		"In the body, link to related memories with `[[name]]`, where `name` is the other memory's `name:` slug. Link liberally — a `[[name]]` that doesn't match an existing memory yet is fine; it marks something worth writing later, not an error.",
		"",
		"`user`: who the user is (role, expertise, preferences). `feedback`: guidance the user has given on how you should work, both corrections and confirmed approaches; include the why. `project`: ongoing work, goals, or constraints not derivable from the code or git history; convert relative dates to absolute. `reference`: pointers to external resources (URLs, dashboards, tickets).",
		"",
		"After writing the file, add a one-line pointer in `MEMORY.md` (`- [Title](file.md) — hook`). `MEMORY.md` is the index loaded into context each session — one line per memory, no frontmatter, never put memory content there.",
		"",
		"Before saving, check for an existing file that already covers it. Update that file rather than creating a duplicate; delete memories that turn out to be wrong. Don't save what the repo already records (code structure, past fixes, git history, AGENTS.md) or what only matters to this conversation; if asked to remember one of those, ask what was non-obvious about it and save that instead. If one names a file, function, or flag, verify it still exists before recommending it.",
	}, "\n\n")
}
