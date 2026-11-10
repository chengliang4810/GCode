package engine

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestToolOutputAndPromptSurviveRestartAndRootSwitch(t *testing.T) {
	home := t.TempDir()
	core := openStorageTestCore(t, home)
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	session := core.state.Sessions[0]
	root := core.storage.Directory()
	full := strings.Repeat("完整输出\n", 10000)
	path, err := core.storeToolOutput(t.Context(), session, full)
	if err != nil || !strings.HasPrefix(path, filepath.Join(root, "sessions", session.ID)+string(os.PathSeparator)) {
		t.Fatalf("tool output escaped session ownership: %s %v", path, err)
	}
	prompt := model.Prompt{System: "stable system", Effort: "high", Tools: []model.Tool{{Name: "Read", Parameters: map[string]any{"nested": map[string]any{"type": "object"}}}}}
	core.mu.Lock()
	if err := core.ensureSessionDataLocked(session.ID); err != nil {
		core.mu.Unlock()
		t.Fatal(err)
	}
	core.prompts[session.ID] = model.ClonePrompt(prompt)
	core.history[session.ID] = []model.Message{{Role: "tool", CallID: "saved", Text: "stable preview", OutputFile: path, ContentHash: "original",
		ContextUsage: &model.ContextUsage{Known: true, InputTokens: 500, HistoryLength: 0}}}
	core.context[session.ID] = model.CloneMessages(core.history[session.ID])
	err = core.persistLocked(session.ID)
	core.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "storage.change", Path: filepath.Join(home, "next-root")}); err != nil {
		t.Fatal(err)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	core = openStorageTestCore(t, home)
	defer core.Close()
	core.mu.Lock()
	err = core.ensureSessionDataLocked(session.ID)
	restored := model.CloneMessages(core.context[session.ID])
	prefix := model.ClonePrompt(core.prompts[session.ID])
	core.mu.Unlock()
	if err != nil || len(restored) != 1 || restored[0].OutputFile != path || prefix.System != prompt.System || prefix.Effort != prompt.Effort || len(prefix.Tools) != 1 {
		t.Fatalf("restart lost stable context: %v history=%+v prompt=%+v", err, restored, prefix)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != full {
		t.Fatal("full output was moved, removed or changed")
	}
	second, err := core.storeToolOutput(t.Context(), session, full)
	if err != nil || second != path {
		t.Fatalf("old session switched to current root: %s %v", second, err)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("private tool output permissions changed")
		}
	}
}

func TestToolOutputRejectsSymlinkDirectoryAndCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unprivileged symlink creation")
	}
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	session := core.state.Sessions[0]
	outside := t.TempDir()
	directory := filepath.Join(core.storage.Directory(), "sessions", session.ID, "tool-results")
	if err := os.Symlink(outside, directory); err != nil {
		t.Fatal(err)
	}
	if _, err := core.storeToolOutput(t.Context(), session, "private"); err == nil {
		t.Fatal("tool output followed a symlink directory")
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatal("outside directory was modified")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := core.storeToolOutput(ctx, session, "private"); err == nil {
		t.Fatal("canceled output storage continued")
	}
}
