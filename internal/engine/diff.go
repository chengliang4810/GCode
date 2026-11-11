package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

type editBefore struct {
	path, content string
	existed       bool
}

func toolKind(name string) contracts.ToolKind {
	switch name {
	case "Read":
		return contracts.ToolReadFile
	case "Write":
		return contracts.ToolWriteFile
	case "Edit":
		return contracts.ToolEditFile
	case "Grep":
		return contracts.ToolSearch
	case "Glob":
		return contracts.ToolGlob
	case "ListDirectory":
		return contracts.ToolDirectory
	case "Bash":
		return contracts.ToolExec
	case "TodoWrite":
		return contracts.ToolTodo
	case "TaskOutput":
		return contracts.ToolTaskOutput
	case "TaskStop":
		return contracts.ToolTaskStop
	case "Skill":
		return contracts.ToolSkill
	case "goal_read":
		return contracts.ToolGoalRead
	case "Agent":
		return contracts.ToolAgent
	case "WebFetch":
		return contracts.ToolWebFetch
	case "WebSearch":
		return contracts.ToolWebSearch
	default:
		if strings.HasPrefix(name, "mcp__") {
			return contracts.ToolMCP
		}
		return contracts.ToolUnknown
	}
}
func (e *Engine) beginEditLocked(sessionID string, call model.Call) string {
	var args struct {
		Path     string `json:"path"`
		FilePath string `json:"file_path"`
	}
	if json.Unmarshal([]byte(call.Arguments), &args) != nil {
		return ""
	}
	if call.Name == "Read" || call.Name == "Write" || call.Name == "Edit" {
		args.Path = args.FilePath
	}
	if args.Path == "" {
		return ""
	}
	index := e.sessionIndex(sessionID)
	if index < 0 {
		return args.Path
	}
	path := args.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(e.state.Sessions[index].CWD, path)
	}
	if call.Name != "Write" && call.Name != "Edit" {
		return path
	}
	run := e.runs[sessionID]
	if run == nil {
		return path
	}
	content, err := readDiffText(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return path
	}
	if run.edits == nil {
		run.edits = map[string]editBefore{}
	}
	run.edits[call.ID] = editBefore{path: path, content: content, existed: err == nil}
	return path
}
func readDiffText(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 2<<20+1))
	if err != nil {
		return "", err
	}
	if len(data) > 2<<20 || !utf8.Valid(data) {
		return "", errors.New("diff text exceeds the size or encoding limit")
	}
	return string(data), nil
}
func (e *Engine) finishEditLocked(sessionID string, call model.Call) *contracts.Change {
	run := e.runs[sessionID]
	if run == nil {
		return nil
	}
	before, ok := run.edits[call.ID]
	if !ok {
		return nil
	}
	delete(run.edits, call.ID)
	content, err := readDiffText(before.path)
	if err != nil || content == before.content {
		return nil
	}
	index := e.sessionIndex(sessionID)
	if index < 0 {
		return nil
	}
	session := e.state.Sessions[index]
	path, err := filepath.Rel(session.CWD, before.path)
	if err != nil || !filepath.IsLocal(path) {
		path = before.path
	}
	path = filepath.ToSlash(path)
	old := splitDiffLines(before.content)
	next := splitDiffLines(content)
	prefix := 0
	for prefix < len(old) && prefix < len(next) && old[prefix] == next[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(old)-prefix && suffix < len(next)-prefix && old[len(old)-1-suffix] == next[len(next)-1-suffix] {
		suffix++
	}
	deletions, additions := len(old)-prefix-suffix, len(next)-prefix-suffix
	lines := []contracts.DiffLine{}
	for i := prefix; i < len(old)-suffix; i++ {
		lines = append(lines, contracts.DiffLine{Kind: "deletion", Text: old[i], OldLine: i + 1})
	}
	for i := prefix; i < len(next)-suffix; i++ {
		lines = append(lines, contracts.DiffLine{Kind: "addition", Text: next[i], NewLine: i + 1})
	}
	change := contracts.Change{ID: call.ID, SessionID: sessionID, ProjectID: session.ProjectID, Path: path, Status: "modified", OldContent: before.content, NewContent: content, Additions: additions, Deletions: deletions, Hunks: []contracts.DiffHunk{{Header: fmt.Sprintf("@@ -%d,%d +%d,%d @@", prefix+1, deletions, prefix+1, additions), OldStart: prefix + 1, NewStart: prefix + 1, Lines: lines}}}
	if !before.existed {
		change.Status = "added"
	}
	e.state.Changes = append(e.state.Changes, change)
	for i := range e.state.Files {
		file := &e.state.Files[i]
		if file.ProjectID == session.ProjectID && file.Path == path && file.Loaded {
			file.Content = content
			file.Version = fileVersion([]byte(content))
		}
	}
	return &change
}
func splitDiffLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

func (e *Engine) refreshProjectFilesLocked(projectID string) {
	for _, project := range e.state.Projects {
		if project.ID == projectID {
			e.startProjectFilesLocked(project)
			return
		}
	}
}
