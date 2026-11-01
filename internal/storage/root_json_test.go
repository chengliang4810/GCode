package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteJSONRootReplacesInsideOpenDirectory(t *testing.T) {
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, value := range []string{"old", "new"} {
		if err := WriteJSONRoot(root, "state.json", value); err != nil {
			t.Fatal(err)
		}
	}
	data, err := root.ReadFile("state.json")
	if err != nil || strings.TrimSpace(string(data)) != `"new"` {
		t.Fatalf("replacement result: %q, %v", data, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 || entries[0].Name() != "state.json" {
		t.Fatalf("unexpected files after replacement: %v %v", entries, err)
	}
	before := bytes.Clone(data)
	if err := WriteJSONRoot(root, "state.json", make(chan struct{})); err == nil {
		t.Fatal("unsupported JSON value was accepted")
	}
	after, err := root.ReadFile("state.json")
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("serialization error changed existing data")
	}
}

func TestWriteJSONRootRejectsNonlocalNames(t *testing.T) {
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, name := range []string{"", ".", "..", "../escaped.json", `..\escaped.json`, "child/state.json",
		`child\state.json`, filepath.Join(directory, "absolute.json"), "C:outside", "state.json:stream"} {
		if err := WriteJSONRoot(root, name, "no"); err == nil {
			t.Fatalf("accepted nonlocal filename %q", name)
		}
	}
	if err := WriteJSONRoot(nil, "state.json", "no"); err == nil {
		t.Fatal("nil root was accepted")
	}
}

func TestWriteJSONRootDoesNotFollowDestinationSymlink(t *testing.T) {
	directory, outside := t.TempDir(), t.TempDir()
	path := filepath.Join(outside, "keep.json")
	if err := os.WriteFile(path, []byte("outside data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, filepath.Join(directory, "state.json")); err != nil {
		t.Skipf("symlink creation is not available: %v", err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	// 实现可替换链接本身，也可拒绝链接；不能改写它指向的外部文件。
	writeErr := WriteJSONRoot(root, "state.json", "inside")
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "outside data" {
		t.Fatalf("write followed a destination link: %q %v", data, err)
	}
	if writeErr == nil {
		info, err := root.Lstat("state.json")
		if err != nil || !info.Mode().IsRegular() {
			t.Fatal("successful write did not replace the symlink itself")
		}
	}
}

func TestWriteJSONRootCleansStageOnRenameFailure(t *testing.T) {
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := root.Mkdir("occupied", 0700); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSONRoot(root, "occupied", "no"); err == nil {
		t.Fatal("replacement unexpectedly overwrote a directory")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 || entries[0].Name() != "occupied" {
		t.Fatalf("failed replacement left temporary files: %v %v", entries, err)
	}
}
