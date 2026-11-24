package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
)

// createTestBrowser browses and creates folders on the real filesystem, but
// only inside the folders a test owns.
func createTestBrowser() *directoryBrowser {
	return &directoryBrowser{
		permits: make(chan struct{}, 1), timeout: 5 * time.Second,
		home: os.UserHomeDir, abs: filepath.Abs,
		open:  func(path string) (directoryReader, error) { return os.Open(path) },
		stat:  os.Stat,
		mkdir: func(path string) error { return os.Mkdir(path, 0o755) },
		roots: func() []string { return []string{"/"} },
	}
}

func TestDirectoryCreateMakesOneFolderAndListsIt(t *testing.T) {
	b := createTestBrowser()
	parent := t.TempDir()
	listing, err := b.createDirectory(context.Background(), contracts.DirectoryCreateRequest{DeviceID: "local", Parent: parent, Name: "  new-project  "})
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parent, "new-project")
	if listing.Path != filepath.ToSlash(target) || listing.DeviceID != "local" || len(listing.Entries) != 0 {
		t.Fatalf("created listing: %+v", listing)
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		t.Fatalf("folder was not created: %v", err)
	}
	if _, err := b.createDirectory(context.Background(), contracts.DirectoryCreateRequest{DeviceID: "local", Parent: parent, Name: "new-project"}); err == nil {
		t.Fatal("the same folder name was created twice")
	}
	if _, err := b.createDirectory(context.Background(), contracts.DirectoryCreateRequest{DeviceID: "remote", Parent: parent, Name: "other"}); err == nil {
		t.Fatal("a folder was created for an unknown device")
	}
	waitBrowsePermit(t, b)
}

func TestDirectoryCreateRejectsUnusableNames(t *testing.T) {
	for _, name := range []string{"", "   ", ".", "..", "nested/folder", `nested\folder`, "null\x00byte", strings.Repeat("a", 256)} {
		t.Run(strings.ReplaceAll(name, "\x00", "null"), func(t *testing.T) {
			b := createTestBrowser()
			parent := t.TempDir()
			if _, err := b.createDirectory(context.Background(), contracts.DirectoryCreateRequest{DeviceID: "local", Parent: parent, Name: name}); err == nil {
				t.Fatalf("name %q was accepted", name)
			}
			entries, err := os.ReadDir(parent)
			if err != nil || len(entries) != 0 {
				t.Fatalf("rejected name left folders behind: %v %v", entries, err)
			}
			waitBrowsePermit(t, b)
		})
	}
}

func TestDirectoryCreateRequiresAnExistingParentFolder(t *testing.T) {
	b := createTestBrowser()
	parent := t.TempDir()
	file := filepath.Join(parent, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := b.createDirectory(context.Background(), contracts.DirectoryCreateRequest{DeviceID: "local", Parent: filepath.Join(parent, "missing"), Name: "folder"}); err == nil {
		t.Fatal("a folder was created inside a missing parent")
	}
	if _, err := b.createDirectory(context.Background(), contracts.DirectoryCreateRequest{DeviceID: "local", Parent: file, Name: "folder"}); err == nil {
		t.Fatal("a folder was created inside a file")
	}
	waitBrowsePermit(t, b)
}

func TestDirectoryCreateDefaultsToTheHomeFolder(t *testing.T) {
	b := createTestBrowser()
	home := t.TempDir()
	b.home = func() (string, error) { return home, nil }
	listing, err := b.createDirectory(context.Background(), contracts.DirectoryCreateRequest{DeviceID: "local", Name: "from-home"})
	if err != nil {
		t.Fatal(err)
	}
	if listing.Path != filepath.ToSlash(filepath.Join(home, "from-home")) {
		t.Fatalf("home folder creation: %+v", listing)
	}
	waitBrowsePermit(t, b)
}

func TestEngineCreateDirectoryUsesTheHostFilesystem(t *testing.T) {
	core := &Engine{}
	parent := t.TempDir()
	listing, err := core.CreateDirectory(context.Background(), contracts.DirectoryCreateRequest{DeviceID: "local", Parent: parent, Name: "engine-folder"})
	if err != nil {
		t.Fatal(err)
	}
	if listing.Path != filepath.ToSlash(filepath.Join(parent, "engine-folder")) {
		t.Fatalf("engine listing: %+v", listing)
	}
	if _, err := os.Stat(filepath.Join(parent, "engine-folder")); err != nil {
		t.Fatal(err)
	}
}
