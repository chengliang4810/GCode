package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

// editBefore is one tool call's baseline. Write and Edit record the target file
// directly; tools that hide their writes behind a shell command or a patch
// payload record a bounded working-tree snapshot instead.
type editBefore struct {
	path, content string
	existed       bool
	tree          *treeSnapshot
	// changes maps a path this turn already reported to the change row that
	// holds it, so a later write replaces that row instead of adding one.
	changes map[string]string
}

// turnChangesKey reserves one edits entry for the paths a turn already
// reported. contracts.Change carries no turn field, so the turn's rows have to
// be tracked here to tell them apart from earlier turns' rows.
const turnChangesKey = "\x00turn-changes"

// Working-tree tracking caps. A redirect or a patch can touch any path, so the
// engine compares bounded snapshots instead of trusting the arguments. The caps
// keep one tool call from turning into an unbounded repository scan, and what
// they exclude is reported instead of being dropped silently.
const (
	changeScanDepth = 8       // directory levels below the session working directory
	changeScanFiles = 512     // files recorded per snapshot
	changeScanFile  = 1 << 20 // largest file whose bytes feed a change diff
	changeScanBytes = 4 << 20 // total bytes cached as before content per snapshot
	changeScanNotes = 4       // distinct caps reported per snapshot
)

// changeScanSkip names directories whose contents are not a tool's own edits:
// history, dependencies and build output.
var changeScanSkip = map[string]bool{
	".git": true, ".cache": true, ".venv": true, "__pycache__": true,
	"node_modules": true, "output": true, "dist": true, "build": true, "target": true,
}

// treeFile is one file's bounded snapshot entry.
type treeFile struct {
	size    int64
	modTime time.Time
	content string
	known   bool
}

// treeSnapshot records the working tree as it was around one instant. start is
// the moment the comparison begins, so a file that is absent from a truncated
// baseline is still recognisable as this call's write by its modification time.
type treeSnapshot struct {
	start  time.Time
	files  map[string]treeFile
	bytes  int
	cutOff bool
	notes  []string
}

func (tree *treeSnapshot) note(text string) {
	if slices.Contains(tree.notes, text) || len(tree.notes) >= changeScanNotes {
		return
	}
	tree.notes = append(tree.notes, text)
}

// cut records a limit that stopped part of the scan: absence from the snapshot
// no longer proves a path was not there.
func (tree *treeSnapshot) cut(format string, args ...any) {
	tree.cutOff = true
	tree.note(fmt.Sprintf(format, args...))
}

func toolKind(name string) contracts.ToolKind {
	switch name {
	case "Read":
		return contracts.ToolReadFile
	case "Write":
		return contracts.ToolWriteFile
	case "Edit":
		return contracts.ToolEditFile
	case "ApplyPatch", "apply_patch", "applyPatch", "patch":
		return contracts.ToolApplyPatch
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
	case "CreateWorkflow", "AmendWorkflow", "SaveWorkflow", "ListSavedWorkflows", "EvalWorkflowSnippet",
		"ListWorkflowRuns", "GetWorkflowRun", "ResumeWorkflowRun", "ResolveWorkflowQuestion", "FillWorkflowHole":
		return contracts.ToolWorkflow
	default:
		if strings.HasPrefix(name, "mcp__") {
			return contracts.ToolMCP
		}
		return contracts.ToolUnknown
	}
}

// tracksWorkingTree reports whether a call's writes can only be found by
// comparing the working tree around the execution.
func tracksWorkingTree(kind contracts.ToolKind) bool {
	return kind == contracts.ToolExec || kind == contracts.ToolApplyPatch
}

// carriesToolDiff reports whether a tool call shows its own change directly,
// which is the engine-owned Write/Edit path and patch tools that edit files in
// place. An exec block keeps its changes in the snapshot's change list only,
// because a command is not a file edit even when it redirects output.
func carriesToolDiff(kind contracts.ToolKind) bool {
	return kind == contracts.ToolWriteFile || kind == contracts.ToolEditFile || kind == contracts.ToolApplyPatch
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
	index := e.sessionIndex(sessionID)
	path := args.Path
	if index >= 0 && path != "" && !filepath.IsAbs(path) {
		path = filepath.Join(e.state.Sessions[index].CWD, path)
	}
	if tracksWorkingTree(toolKind(call.Name)) {
		// The arguments of these calls do not name their targets, so record the
		// working tree before the tool runs and compare it afterwards.
		e.captureTreeLocked(sessionID, call)
		return path
	}
	if path == "" || index < 0 || !carriesToolDiff(toolKind(call.Name)) {
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

// captureTreeLocked stores the baseline for a call whose writes are only
// visible in the working tree. Only the first snapshot counts: a streamed or
// hook-updated argument still arrives before execution, but a later one could
// land after the tool already wrote.
func (e *Engine) captureTreeLocked(sessionID string, call model.Call) {
	run := e.runs[sessionID]
	index := e.sessionIndex(sessionID)
	if run == nil || index < 0 {
		return
	}
	if run.edits == nil {
		run.edits = map[string]editBefore{}
	}
	if _, ok := run.edits[call.ID]; ok {
		return
	}
	tree := captureTree(e.state.Sessions[index].CWD, nil)
	run.edits[call.ID] = editBefore{tree: &tree}
}

// captureTree walks the working directory with hard caps. A baseline supplies
// earlier entries so unchanged files reuse their bytes instead of being read
// twice.
func captureTree(root string, baseline map[string]treeFile) treeSnapshot {
	tree := treeSnapshot{start: time.Now(), files: map[string]treeFile{}}
	if root == "" {
		tree.cut("the session has no working directory")
		return tree
	}
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				tree.cut("%q cannot be read: %v", root, err)
			}
			return nil
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		key := filepath.ToSlash(relative)
		depth := strings.Count(key, "/") + 1
		if depth > changeScanDepth {
			tree.cut("the depth limit of %d levels was reached", changeScanDepth)
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if changeScanSkip[entry.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		if len(tree.files) >= changeScanFiles {
			tree.cut("the limit of %d files per snapshot was reached", changeScanFiles)
			return fs.SkipAll
		}
		info, err := entry.Info()
		if err != nil {
			tree.cut("%q cannot be inspected: %v", key, err)
			return nil
		}
		file := treeFile{size: info.Size(), modTime: info.ModTime()}
		previous, ok := baseline[key]
		if ok && previous.size == file.size && previous.modTime.Equal(file.modTime) {
			file.content, file.known = previous.content, previous.known
		} else if file.size <= changeScanFile && tree.bytes+int(file.size) <= changeScanBytes {
			content, err := readDiffText(path)
			if err != nil {
				tree.cut("%q cannot be read: %v", key, err)
			} else {
				file.content, file.known = content, true
				tree.bytes += len(content)
			}
		}
		tree.files[key] = file
		return nil
	})
	if walkErr != nil {
		tree.cut("%q cannot be walked: %v", root, walkErr)
	}
	return tree
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

// finishToolChangesLocked stores the changes the finished call produced and
// returns them. Write/Edit keep their single-path record; exec and patch calls
// report every file that moved inside their bounded before/after snapshot.
func (e *Engine) finishToolChangesLocked(sessionID string, call model.Call) []contracts.Change {
	run := e.runs[sessionID]
	index := e.sessionIndex(sessionID)
	if run == nil || index < 0 {
		return nil
	}
	before, ok := run.edits[call.ID]
	if !ok {
		return nil
	}
	delete(run.edits, call.ID)
	session := e.state.Sessions[index]
	if before.tree != nil {
		return e.treeChangesLocked(run, session, call, *before.tree)
	}
	content, err := readDiffText(before.path)
	if err != nil || content == before.content {
		return nil
	}
	change := contracts.Change{ID: call.ID, SessionID: session.ID, ProjectID: session.ProjectID, Path: changePath(session.CWD, before.path), Status: "modified", OldContent: before.content, NewContent: content}
	if !before.existed {
		change.Status = "added"
	}
	change.Additions, change.Deletions, change.Hunks = lineDiff(before.content, content)
	e.recordChangeLocked(run, session, change)
	return []contracts.Change{change}
}

// treeChangesLocked compares the post-execution tree with the baseline. Files
// whose bytes a cap kept out of the snapshot are still reported by status, the
// missing diff is explained in the engine log instead of being dropped silently.
func (e *Engine) treeChangesLocked(run *activeRun, session contracts.Session, call model.Call, before treeSnapshot) []contracts.Change {
	after := captureTree(session.CWD, before.files)
	for _, note := range append(slices.Clone(before.notes), after.notes...) {
		log.Printf("change tracking for %s in %q is incomplete: %s", call.Name, session.CWD, note)
	}
	paths := make([]string, 0, len(after.files))
	for path := range after.files {
		paths = append(paths, path)
	}
	for path := range before.files {
		if _, ok := after.files[path]; !ok {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	changes := []contracts.Change{}
	untracked := []string{}
	for _, path := range paths {
		previous, existedBefore := before.files[path]
		next, existsAfter := after.files[path]
		status := "modified"
		switch {
		case existedBefore && existsAfter:
			if previous.size == next.size && previous.modTime.Equal(next.modTime) {
				continue
			}
		case existsAfter:
			// A truncated baseline cannot prove the file was absent, but a file
			// written after the call started still names itself as its write.
			if next.modTime.Before(before.start) {
				continue
			}
			if !before.cutOff {
				status = "added"
			}
		case after.cutOff:
			// The scan stopped early, so a missing path may only mean it was
			// never recorded.
			continue
		default:
			status = "deleted"
		}
		// Content is reported only when both sides are known: one side alone
		// would read as a whole-file rewrite in the file views.
		tracked := (!existedBefore || previous.known) && (!existsAfter || next.known)
		oldContent, newContent := "", ""
		if tracked {
			if existedBefore {
				oldContent = previous.content
			}
			if existsAfter {
				newContent = next.content
			}
		} else {
			untracked = append(untracked, fmt.Sprintf("%q (%s, content over the tracking limits)", path, status))
		}
		change := contracts.Change{ID: call.ID + ":" + path, SessionID: session.ID, ProjectID: session.ProjectID, Path: path, Status: status, OldContent: oldContent, NewContent: newContent}
		if tracked {
			change.Additions, change.Deletions, change.Hunks = lineDiff(oldContent, newContent)
			if change.Additions == 0 && change.Deletions == 0 && status == "modified" {
				// The timestamp moved but the bytes did not: nothing to report.
				continue
			}
		}
		if e.recordChangeLocked(run, session, change) {
			changes = append(changes, change)
		}
	}
	if len(untracked) > 0 {
		log.Printf("change tracking for %s in %q reports no content for %d changed files: %s", call.Name, session.CWD, len(untracked), strings.Join(untracked, "; "))
	}
	return changes
}

// recordChangeLocked stores one change for the current turn: a later write to a
// path this turn already reported replaces that row, so one file keeps one row
// per turn however many tools touch it.
func (e *Engine) recordChangeLocked(run *activeRun, session contracts.Session, change contracts.Change) bool {
	if run.edits == nil {
		run.edits = map[string]editBefore{}
	}
	turn := run.edits[turnChangesKey]
	previous, seen := turn.changes[change.Path]
	if !seen {
		e.state.Changes = append(e.state.Changes, change)
	} else if index := changeIndex(e.state.Changes, session.ID, previous); index >= 0 {
		e.state.Changes[index] = change
	} else {
		// The row is gone, so the path is reported again rather than lost.
		e.state.Changes = append(e.state.Changes, change)
	}
	if turn.changes == nil {
		turn.changes = map[string]string{}
	}
	turn.changes[change.Path] = change.ID
	run.edits[turnChangesKey] = turn
	e.syncChangeContentLocked(session, change)
	return true
}

func changeIndex(changes []contracts.Change, sessionID, changeID string) int {
	for i := range changes {
		if changes[i].SessionID == sessionID && changes[i].ID == changeID {
			return i
		}
	}
	return -1
}

func (e *Engine) syncChangeContentLocked(session contracts.Session, change contracts.Change) {
	if change.Status == "deleted" {
		return
	}
	for i := range e.state.Files {
		file := &e.state.Files[i]
		if file.ProjectID == session.ProjectID && file.Path == change.Path && file.Loaded {
			file.Content = change.NewContent
			file.Version = fileVersion([]byte(change.NewContent))
		}
	}
}

// lineDiff describes the changed middle of two texts as one hunk, matching the
// record Write and Edit have always produced.
func lineDiff(oldText, newText string) (int, int, []contracts.DiffHunk) {
	old := splitDiffLines(oldText)
	next := splitDiffLines(newText)
	prefix := 0
	for prefix < len(old) && prefix < len(next) && old[prefix] == next[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(old)-prefix && suffix < len(next)-prefix && old[len(old)-1-suffix] == next[len(next)-1-suffix] {
		suffix++
	}
	deletions, additions := len(old)-prefix-suffix, len(next)-prefix-suffix
	if deletions == 0 && additions == 0 {
		return 0, 0, nil
	}
	lines := []contracts.DiffLine{}
	for i := prefix; i < len(old)-suffix; i++ {
		lines = append(lines, contracts.DiffLine{Kind: "deletion", Text: old[i], OldLine: i + 1})
	}
	for i := prefix; i < len(next)-suffix; i++ {
		lines = append(lines, contracts.DiffLine{Kind: "addition", Text: next[i], NewLine: i + 1})
	}
	header := fmt.Sprintf("@@ -%d,%d +%d,%d @@", prefix+1, deletions, prefix+1, additions)
	return additions, deletions, []contracts.DiffHunk{{Header: header, OldStart: prefix + 1, NewStart: prefix + 1, Lines: lines}}
}

// changePath names a changed file relative to the session so the UI shows the
// same path the project files use.
func changePath(cwd, path string) string {
	if relative, err := filepath.Rel(cwd, path); err == nil && filepath.IsLocal(relative) {
		return filepath.ToSlash(relative)
	}
	return path
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
