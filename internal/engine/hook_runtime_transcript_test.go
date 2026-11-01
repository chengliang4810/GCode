package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestHookTranscriptUsesExistingBoundDirectory(t *testing.T) {
	home := t.TempDir()
	core := openStorageTestCore(t, home)
	if err := core.Execute(context.Background(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	session := core.state.Sessions[0]
	firstRoot := core.storage.Directory()
	storeTestMessage(t, core, session.ID, "original transcript")
	secondRoot := filepath.Join(home, "next-runtime")
	if err := core.storage.RequestDirectory(secondRoot); err != nil {
		t.Fatal(err)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	core = openStorageTestCore(t, home)
	defer core.Close()
	directory := filepath.Join(firstRoot, "sessions", session.ID)
	for _, text := range []string{"first export", "replaced export"} {
		core.mu.Lock()
		core.history[session.ID] = []model.Message{{Role: "user", Text: text}}
		core.mu.Unlock()
		path, err := core.writeHookTranscript(session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if path != filepath.Join(directory, "hook-transcript.jsonl") {
			t.Fatalf("historical transcript followed the active root: %q", path)
		}
		content, err := os.ReadFile(path)
		if err != nil || !bytes.Contains(content, []byte(text)) {
			t.Fatalf("transcript was not atomically replaced: %s, %v", content, err)
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".transcript-") {
			t.Fatal("transcript export left a temporary file")
		}
	}
	retained := directory + ".retained"
	if err := os.Rename(directory, retained); err != nil {
		t.Fatal(err)
	}
	defer os.Rename(retained, directory)
	if _, err := core.writeHookTranscript(session.ID); err == nil {
		t.Fatal("missing historical conversation directory was accepted")
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("transcript export recreated a missing historical directory")
	}
	if _, err := os.Stat(filepath.Join(secondRoot, "sessions", session.ID)); !os.IsNotExist(err) {
		t.Fatal("transcript export created a replacement under the active root")
	}
}

func TestHookTranscriptRejectsDirectorySymlinks(t *testing.T) {
	for _, name := range []string{"session directory", "sessions parent"} {
		t.Run(name, func(t *testing.T) {
			core := openStorageTestCore(t, t.TempDir())
			defer core.Close()
			if err := core.Execute(context.Background(), contracts.Command{Kind: "session.create"}); err != nil {
				t.Fatal(err)
			}
			session := core.state.Sessions[0]
			directory := filepath.Join(core.storage.Directory(), "sessions", session.ID)
			external := t.TempDir()
			protected := filepath.Join(external, "hook-transcript.jsonl")
			if name == "sessions parent" {
				directory = filepath.Dir(directory)
				protected = filepath.Join(external, session.ID, "hook-transcript.jsonl")
			}
			if err := os.MkdirAll(filepath.Dir(protected), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(protected, []byte("user-owned external transcript"), 0600); err != nil {
				t.Fatal(err)
			}
			retained := directory + ".retained"
			if err := os.Rename(directory, retained); err != nil {
				t.Fatal(err)
			}
			defer os.Rename(retained, directory)
			if err := os.Symlink(external, directory); err != nil {
				t.Skipf("directory symlink creation unavailable: %v", err)
			}
			defer os.Remove(directory)
			if _, err := core.writeHookTranscript(session.ID); err == nil {
				t.Fatal("transcript export followed a directory symlink")
			}
			content, err := os.ReadFile(protected)
			if err != nil || string(content) != "user-owned external transcript" {
				t.Fatalf("transcript export changed external data: %q, %v", content, err)
			}
		})
	}
}
