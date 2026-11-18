package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

// newChangeCore builds the smallest engine that owns one session with a working
// directory and an active run, which is all change tracking needs.
func newChangeCore(t *testing.T, dir string) (*Engine, *string) {
	t.Helper()
	core := &Engine{
		state: contracts.Snapshot{
			Messages: map[string][]contracts.Message{},
			Sessions: []contracts.Session{{ID: "session", ProjectID: "project", CWD: dir}},
		},
		runs: map[string]*activeRun{"session": {id: "run", edits: map[string]editBefore{}}},
		subs: map[uint64]chan contracts.Event{}, done: make(chan struct{}),
	}
	current := ""
	if err := core.applyAgentEventLocked("session", "run", &current, agent.Event{Kind: "message.start"}); err != nil {
		t.Fatal(err)
	}
	return core, &current
}

func applyChangeEvent(t *testing.T, core *Engine, current *string, event agent.Event) {
	t.Helper()
	if err := core.applyAgentEventLocked("session", "run", current, event); err != nil {
		t.Fatal(err)
	}
}

// runExecTool opens a shell-tool call, applies the write the command would have
// done, and closes the call, mirroring the event order the agent emits.
func runExecTool(t *testing.T, core *Engine, current *string, callID string, write func()) {
	t.Helper()
	call := model.Call{ID: callID, Name: "Bash", Arguments: `{"command":"write"}`}
	applyChangeEvent(t, core, current, agent.Event{Kind: "tool.start", Call: call})
	write()
	applyChangeEvent(t, core, current, agent.Event{Kind: "tool.end", Call: call, Output: "ok"})
}

func TestChangeCaptureRecordsShellWritesAndRemovals(t *testing.T) {
	dir := t.TempDir()
	core, current := newChangeCore(t, dir)
	overwritten := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(overwritten, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	created := filepath.Join(dir, "nested", "fresh.txt")
	if err := os.MkdirAll(filepath.Dir(created), 0o700); err != nil {
		t.Fatal(err)
	}
	removed := filepath.Join(dir, "doomed.txt")
	if err := os.WriteFile(removed, []byte("gone\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runExecTool(t, core, current, "call-shell", func() {
		if err := os.WriteFile(overwritten, []byte("hi\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(created, []byte("new\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(removed); err != nil {
			t.Fatal(err)
		}
	})
	if len(core.state.Changes) != 3 {
		t.Fatalf("shell writes produced %d changes: %+v", len(core.state.Changes), core.state.Changes)
	}
	changed := map[string]contracts.Change{}
	for _, change := range core.state.Changes {
		if change.SessionID != "session" || change.ProjectID != "project" {
			t.Fatalf("change lost its owner: %+v", change)
		}
		changed[change.Path] = change
	}
	modified, ok := changed["file.txt"]
	if !ok || modified.Status != "modified" || modified.OldContent != "before\n" || modified.NewContent != "hi\n" {
		t.Fatalf("overwritten file was not captured: %+v", modified)
	}
	if modified.Additions != 1 || modified.Deletions != 1 || len(modified.Hunks) != 1 ||
		modified.Hunks[0].Lines[0].Kind != "deletion" || modified.Hunks[0].Lines[1].Text != "hi" {
		t.Fatalf("overwritten file has no content diff: %+v", modified)
	}
	added, ok := changed["nested/fresh.txt"]
	if !ok || added.Status != "added" || added.NewContent != "new\n" || added.Deletions != 0 || added.Additions != 1 {
		t.Fatalf("created file was not captured: %+v", added)
	}
	deleted, ok := changed["doomed.txt"]
	if !ok || deleted.Status != "deleted" || deleted.OldContent != "gone\n" || deleted.NewContent != "" || deleted.Deletions != 1 || deleted.Additions != 0 {
		t.Fatalf("deleted file was not captured: %+v", deleted)
	}
	// A command keeps its writes in the change list instead of pretending to be
	// a file edit on the tool block.
	if tool := core.state.Messages["session"][0].Blocks[0].Tool; tool == nil || tool.Diff != nil {
		t.Fatalf("shell call was reported as a file edit: %+v", tool)
	}
}

func TestChangeCaptureKeepsWriteAndEditBehaviour(t *testing.T) {
	for _, test := range []struct {
		name, tool, arguments, before, after, status string
	}{
		{"write creates", "Write", `{"file_path":"note.md","content":"after\n"}`, "", "after\n", "added"},
		{"edit modifies", "Edit", `{"file_path":"note.md","old_string":"before","new_string":"after"}`, "before\n", "after\n", "modified"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			core, current := newChangeCore(t, dir)
			path := filepath.Join(dir, "note.md")
			if test.before != "" {
				if err := os.WriteFile(path, []byte(test.before), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			call := model.Call{ID: "call-edit", Name: test.tool, Arguments: test.arguments}
			applyChangeEvent(t, core, current, agent.Event{Kind: "tool.start", Call: call})
			if err := os.WriteFile(path, []byte(test.after), 0o600); err != nil {
				t.Fatal(err)
			}
			applyChangeEvent(t, core, current, agent.Event{Kind: "tool.end", Call: call, Output: "ok"})
			if len(core.state.Changes) != 1 {
				t.Fatalf("%s produced %d changes: %+v", test.tool, len(core.state.Changes), core.state.Changes)
			}
			change := core.state.Changes[0]
			if change.ID != call.ID || change.Path != "note.md" || change.Status != test.status || change.NewContent != test.after {
				t.Fatalf("%s change differs from the previous record: %+v", test.tool, change)
			}
			if change.Additions != 1 || change.Deletions != len(splitDiffLines(test.before)) || len(change.Hunks) != 1 {
				t.Fatalf("%s lost its content diff: %+v", test.tool, change)
			}
			tool := core.state.Messages["session"][0].Blocks[0].Tool
			if tool == nil || tool.Diff == nil || tool.Diff.ID != call.ID || tool.Kind != toolKind(test.tool) {
				t.Fatalf("%s block lost its change: %+v", test.tool, tool)
			}
		})
	}
}

func TestChangeCaptureReplacesRepeatedWritesInOneTurn(t *testing.T) {
	dir := t.TempDir()
	core, current := newChangeCore(t, dir)
	path := filepath.Join(dir, "out.txt")
	runExecTool(t, core, current, "call-first", func() {
		if err := os.WriteFile(path, []byte("one\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	})
	applyChangeEvent(t, core, current, agent.Event{Kind: "tool.start", Call: model.Call{ID: "call-write", Name: "Write", Arguments: `{"file_path":"out.txt","content":"two\n"}`}})
	if err := os.WriteFile(path, []byte("two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	applyChangeEvent(t, core, current, agent.Event{Kind: "tool.end", Call: model.Call{ID: "call-write", Name: "Write"}, Output: "ok"})
	runExecTool(t, core, current, "call-last", func() {
		if err := os.WriteFile(path, []byte("three\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	})
	if len(core.state.Changes) != 1 {
		t.Fatalf("one path kept %d change rows: %+v", len(core.state.Changes), core.state.Changes)
	}
	change := core.state.Changes[0]
	if change.Path != "out.txt" || change.NewContent != "three\n" || change.OldContent != "two\n" ||
		change.ID != "call-last:out.txt" || change.Additions != 1 || change.Deletions != 1 {
		t.Fatalf("the latest write did not replace the earlier row: %+v", change)
	}
}

func TestChangeCaptureStaysInsideItsScanLimits(t *testing.T) {
	dir := t.TempDir()
	// A file beyond the depth limit and a file beyond the size limit stay
	// unreported instead of stalling or crashing the engine.
	deep := filepath.Join(dir, strings.Repeat("level/", changeScanDepth+1))
	if err := os.MkdirAll(deep, 0o700); err != nil {
		t.Fatal(err)
	}
	deepFile := filepath.Join(deep, "deep.txt")
	large := filepath.Join(dir, "large.bin")
	if err := os.WriteFile(large, make([]byte, changeScanFile+1), 0o600); err != nil {
		t.Fatal(err)
	}
	core, current := newChangeCore(t, dir)
	runExecTool(t, core, current, "call-limits", func() {
		if err := os.WriteFile(deepFile, []byte("deep\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(large, []byte("small now\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	})
	for _, change := range core.state.Changes {
		if change.Path == filepath.ToSlash(filepath.Join(strings.Repeat("level/", changeScanDepth+1), "deep.txt")) {
			t.Fatalf("the depth limit did not hold: %+v", change)
		}
	}
	if len(core.state.Changes) != 1 {
		t.Fatalf("limit case reported %d changes: %+v", len(core.state.Changes), core.state.Changes)
	}
	// The large file is still a known write, but its bytes stayed over the
	// tracking limits, so the change carries the status without a content diff.
	change := core.state.Changes[0]
	if change.Path != "large.bin" || change.Status != "modified" || change.Hunks != nil ||
		change.OldContent != "" || change.NewContent != "" || change.Additions != 0 || change.Deletions != 0 {
		t.Fatalf("oversized file claimed a content diff: %+v", change)
	}
}

func TestChangeCaptureDoesNotGuessBeyondATruncatedBaseline(t *testing.T) {
	dir := t.TempDir()
	for i := range changeScanFiles + 8 {
		name := filepath.Join(dir, fmt.Sprintf("file-%03d.txt", i))
		if err := os.WriteFile(name, []byte("stale\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	tree := captureTree(dir, nil)
	if !tree.cutOff || len(tree.files) != changeScanFiles {
		t.Fatalf("the file limit did not cap the snapshot: %d files, cut off %v", len(tree.files), tree.cutOff)
	}
	core, current := newChangeCore(t, dir)
	// Nothing is written, so a truncated baseline must not turn the files it
	// never recorded into reported changes.
	runExecTool(t, core, current, "call-idle", func() {})
	if len(core.state.Changes) != 0 {
		t.Fatalf("a truncated baseline invented %d changes: %+v", len(core.state.Changes), core.state.Changes)
	}
}

func TestChangeCaptureFollowsRealShellRedirection(t *testing.T) {
	dir := t.TempDir()
	core, current := newChangeCore(t, dir)
	tools, closeTools := agent.RunToolSet(t.Context(), dir)
	defer closeTools()
	var shell agent.Tool
	for _, tool := range tools {
		if tool.Definition().Name == "Bash" {
			shell = tool
			break
		}
	}
	if shell == nil {
		t.Fatal("the shell tool is unavailable")
	}
	call := model.Call{ID: "call-real-shell", Name: "Bash", Arguments: `{"command":"echo hi > file.txt"}`}
	applyChangeEvent(t, core, current, agent.Event{Kind: "tool.start", Call: call})
	output, err := shell.Execute(t.Context(), dir, json.RawMessage(call.Arguments))
	applyChangeEvent(t, core, current, agent.Event{Kind: "tool.end", Call: call, Output: output.Text, Data: output.Data, Error: err != nil})
	if err != nil {
		t.Fatalf("shell command failed: %v", err)
	}
	if len(core.state.Changes) != 1 {
		t.Fatalf("shell redirection produced %d changes: %+v", len(core.state.Changes), core.state.Changes)
	}
	change := core.state.Changes[0]
	if change.Path != "file.txt" || change.Status != "added" || change.NewContent != "hi\n" || change.Additions != 1 || len(change.Hunks) != 1 {
		t.Fatalf("shell redirection was not tracked: %+v", change)
	}
}

func TestChangeCaptureCoversPatchToolCalls(t *testing.T) {
	dir := t.TempDir()
	core, current := newChangeCore(t, dir)
	call := model.Call{ID: "call-patch", Name: "apply_patch", Arguments: `{"patch":"*** Begin Patch"}`}
	applyChangeEvent(t, core, current, agent.Event{Kind: "tool.start", Call: call})
	path := filepath.Join(dir, "patched.txt")
	if err := os.WriteFile(path, []byte("patched\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	applyChangeEvent(t, core, current, agent.Event{Kind: "tool.end", Call: call, Output: "ok"})
	if len(core.state.Changes) != 1 {
		t.Fatalf("patch call produced %d changes: %+v", len(core.state.Changes), core.state.Changes)
	}
	change := core.state.Changes[0]
	if change.Path != "patched.txt" || change.Status != "added" || change.NewContent != "patched\n" {
		t.Fatalf("patch call was not tracked: %+v", change)
	}
	tool := core.state.Messages["session"][0].Blocks[0].Tool
	if tool == nil || tool.Kind != contracts.ToolApplyPatch || tool.Diff == nil || tool.Diff.ID != change.ID {
		t.Fatalf("patch block lost its change: %+v", tool)
	}
}
