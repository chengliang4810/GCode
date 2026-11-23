package engine

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func fileSaveFixture(t *testing.T) (*os.Root, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "document.txt"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root, dir
}

func assertFileSaveContent(t *testing.T, root *os.Root, name, want string) {
	t.Helper()
	data, err := root.ReadFile(name)
	if err != nil || string(data) != want {
		t.Fatalf("%s = %q, %v; want %q", name, data, err, want)
	}
}

func assertNoFileSaveStage(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".file-save-") {
			t.Errorf("staging file leaked: %s", entry.Name())
		}
	}
}

func TestFileSaveCommandPreservesEditorSemantics(t *testing.T) {
	t.Parallel()
	root, dir := fileSaveFixture(t)
	file := contracts.File{ID: "file", Path: "document.txt", Name: "document.txt", Kind: "file"}
	project := contracts.Project{Path: dir}
	opened, err := executeFileCommand(contracts.Command{Kind: "file.open"}, project, file, true)
	if err != nil || !opened.updated || !opened.file.Loaded || opened.file.Content != "original" {
		t.Fatalf("open = %+v, %v", opened, err)
	}
	command := contracts.Command{Kind: "file.save", Text: "保存完成\n", Version: opened.file.Version}
	saved, err := executeFileCommand(command, project, opened.file, true)
	if err != nil || !saved.updated || saved.refresh || !saved.file.Loaded ||
		saved.file.Content != command.Text || saved.file.Version != fileVersion([]byte(command.Text)) {
		t.Fatalf("save = %+v, %v", saved, err)
	}
	assertFileSaveContent(t, root, file.Path, command.Text)
	result, err := executeFileCommand(command, project, opened.file, true)
	if !errors.Is(err, errFileSaveConflict) || result.updated {
		t.Fatalf("stale save = %+v, %v", result, err)
	}
	if _, err := executeFileCommand(command, project, file, false); err == nil {
		t.Fatal("saved missing editor entry")
	}
	assertNoFileSaveStage(t, dir)
}

func TestFileSavePreservesPermissions(t *testing.T) {
	for _, mode := range []os.FileMode{0600, 0640, 0755, 0711} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			root, dir := fileSaveFixture(t)
			if err := root.Chmod("document.txt", mode); err != nil {
				t.Fatal(err)
			}
			before, err := root.Stat("document.txt")
			if err != nil {
				t.Fatal(err)
			}
			if err := saveEditorFile(root, "document.txt", []byte("new data"), fileVersion([]byte("original")), fileSaveIO{}); err != nil {
				t.Fatal(err)
			}
			after, err := root.Stat("document.txt")
			if err != nil || after.Mode().Perm() != before.Mode().Perm() || os.SameFile(before, after) {
				t.Fatalf("before = %v, after = %v, error = %v", before, after, err)
			}
			assertFileSaveContent(t, root, "document.txt", "new data")
			assertNoFileSaveStage(t, dir)
		})
	}
}

func TestFileSaveRejectsCommitChanges(t *testing.T) {
	tests := []struct {
		name   string
		change func(*testing.T, *os.Root, string) error
		check  func(*testing.T, *os.Root)
	}{
		{
			name: "content with same size and timestamp",
			change: func(t *testing.T, root *os.Root, _ string) error {
				info, err := root.Stat("document.txt")
				if err != nil {
					return err
				}
				if err := root.WriteFile("document.txt", []byte("external"), 0600); err != nil {
					return err
				}
				return root.Chtimes("document.txt", info.ModTime(), info.ModTime())
			},
			check: func(t *testing.T, root *os.Root) { assertFileSaveContent(t, root, "document.txt", "external") },
		},
		{
			name: "replacement with identical content",
			change: func(t *testing.T, root *os.Root, _ string) error {
				info, err := root.Stat("document.txt")
				if err != nil {
					return err
				}
				if err := root.Rename("document.txt", "old.txt"); err != nil {
					return err
				}
				if err := root.WriteFile("document.txt", []byte("original"), 0600); err != nil {
					return err
				}
				return root.Chtimes("document.txt", info.ModTime(), info.ModTime())
			},
			check: func(t *testing.T, root *os.Root) { assertFileSaveContent(t, root, "document.txt", "original") },
		},
		{
			name: "permissions changed",
			change: func(_ *testing.T, root *os.Root, _ string) error {
				// A read-only change is also observable on Windows.
				return root.Chmod("document.txt", 0400)
			},
			check: func(t *testing.T, root *os.Root) {
				t.Cleanup(func() { _ = root.Chmod("document.txt", 0600) })
				info, err := root.Stat("document.txt")
				if err != nil || info.Mode().Perm()&0200 != 0 {
					t.Fatalf("external permissions overwritten: %v, %v", info, err)
				}
				assertFileSaveContent(t, root, "document.txt", "original")
			},
		},
		{
			name: "timestamp changed",
			change: func(_ *testing.T, root *os.Root, _ string) error {
				stamp := time.Date(2001, time.January, 1, 0, 0, 0, 0, time.UTC)
				return root.Chtimes("document.txt", stamp, stamp)
			},
			check: func(t *testing.T, root *os.Root) { assertFileSaveContent(t, root, "document.txt", "original") },
		},
		{
			name:   "deletion",
			change: func(_ *testing.T, root *os.Root, _ string) error { return root.Remove("document.txt") },
			check: func(t *testing.T, root *os.Root) {
				if _, err := root.Lstat("document.txt"); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("deleted target recreated: %v", err)
				}
			},
		},
		{
			name: "symlink replacement",
			change: func(t *testing.T, root *os.Root, _ string) error {
				if err := root.Rename("document.txt", "old.txt"); err != nil {
					return err
				}
				if err := root.Symlink("old.txt", "document.txt"); err != nil {
					t.Skipf("cannot create symlink: %v", err)
				}
				return nil
			},
			check: func(t *testing.T, root *os.Root) {
				assertFileSaveContent(t, root, "old.txt", "original")
				info, err := root.Lstat("document.txt")
				if err != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Fatalf("link overwritten: %v, %v", info, err)
				}
			},
		},
		{
			name: "hardlink added",
			change: func(t *testing.T, root *os.Root, _ string) error {
				if err := root.Link("document.txt", "alias.txt"); err != nil {
					t.Skipf("cannot create hard link: %v", err)
				}
				return nil
			},
			check: func(t *testing.T, root *os.Root) {
				assertFileSaveContent(t, root, "document.txt", "original")
				assertFileSaveContent(t, root, "alias.txt", "original")
			},
		},
		{
			name: "growth beyond limit",
			change: func(_ *testing.T, root *os.Root, _ string) error {
				file, err := root.OpenFile("document.txt", os.O_WRONLY, 0)
				if err != nil {
					return err
				}
				return errors.Join(file.Truncate(editorFileLimit+1), file.Close())
			},
			check: func(t *testing.T, root *os.Root) {
				info, err := root.Stat("document.txt")
				if err != nil || info.Size() != editorFileLimit+1 {
					t.Fatalf("external growth overwritten: %v, %v", info, err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root, dir := fileSaveFixture(t)
			err := saveEditorFile(root, "document.txt", []byte("must not win"), fileVersion([]byte("original")), fileSaveIO{
				beforeCommit: func() error { return test.change(t, root, dir) },
			})
			if err == nil {
				t.Fatal("overwrote externally changed target")
			}
			test.check(t, root)
			assertNoFileSaveStage(t, dir)
		})
	}
}

// Wrap a real stage so failures still exercise descriptor close and cleanup.
type failingFileSaveStage struct {
	*os.File
	failure string
	events  *[]string
}

var errFileSaveInjected = errors.New("injected save failure")

func (stage failingFileSaveStage) Write(data []byte) (int, error) {
	*stage.events = append(*stage.events, "write")
	if stage.failure == "write" {
		n, _ := stage.File.Write(data[:1])
		return n, errFileSaveInjected
	}
	if stage.failure == "short write" {
		return stage.File.Write(data[:1])
	}
	return stage.File.Write(data)
}

func (stage failingFileSaveStage) Chmod(mode os.FileMode) error {
	*stage.events = append(*stage.events, "chmod")
	if stage.failure == "chmod" {
		return errFileSaveInjected
	}
	return stage.File.Chmod(mode)
}

func (stage failingFileSaveStage) Sync() error {
	*stage.events = append(*stage.events, "sync")
	if stage.failure == "sync" {
		return errFileSaveInjected
	}
	return stage.File.Sync()
}

func (stage failingFileSaveStage) Close() error {
	*stage.events = append(*stage.events, "close")
	err := stage.File.Close()
	if stage.failure == "close" {
		return errors.Join(err, errFileSaveInjected)
	}
	return err
}

func TestFileSaveStageFailuresAndOrder(t *testing.T) {
	for _, failure := range []string{"create", "write", "short write", "chmod", "sync", "close", "before commit", "rename", "success"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			root, dir := fileSaveFixture(t)
			var events []string
			var stageName string
			operations := fileSaveIO{
				createStage: func(parent *os.Root) (string, fileSaveStage, error) {
					if failure == "create" {
						return "", nil, errFileSaveInjected
					}
					name, stage, err := createFileSaveStage(parent)
					if err != nil {
						return "", nil, err
					}
					stageName = name
					return name, failingFileSaveStage{stage.(*os.File), failure, &events}, nil
				},
				beforeCommit: func() error {
					if strings.Join(events, ",") != "write,chmod,sync,close" {
						t.Fatalf("commit before stage sync/close: %v", events)
					}
					assertFileSaveContent(t, root, "document.txt", "original")
					if failure == "before commit" {
						return errFileSaveInjected
					}
					if failure == "rename" {
						return root.Remove(stageName)
					}
					return nil
				},
			}
			err := saveEditorFile(root, "document.txt", []byte("replacement"), fileVersion([]byte("original")), operations)
			if failure == "success" {
				if err != nil {
					t.Fatal(err)
				}
				assertFileSaveContent(t, root, "document.txt", "replacement")
			} else {
				if err == nil {
					t.Fatal("injected failure succeeded")
				}
				if failure != "rename" && failure != "short write" && !errors.Is(err, errFileSaveInjected) {
					t.Fatalf("lost injected error: %v", err)
				}
				if failure == "short write" && !errors.Is(err, io.ErrShortWrite) {
					t.Fatalf("lost short write: %v", err)
				}
				assertFileSaveContent(t, root, "document.txt", "original")
				if failure != "create" && (len(events) == 0 || events[len(events)-1] != "close") {
					t.Fatalf("stage not closed: %v", events)
				}
			}
			assertNoFileSaveStage(t, dir)
		})
	}
}

func TestFileSaveLinksAndPaths(t *testing.T) {
	for _, name := range []string{"final symlink", "dangling symlink", "hardlink", "directory", "parent traversal", "absolute", "parent symlink escape", "relative parent symlink escape", "parent symlink inside"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root, dir := fileSaveFixture(t)
			outside := t.TempDir()
			outsideFile := filepath.Join(outside, "outside.txt")
			if err := os.WriteFile(outsideFile, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			path := "document.txt"
			wantSuccess := false
			switch name {
			case "final symlink", "dangling symlink":
				target := "document.txt"
				if name == "dangling symlink" {
					target = "missing.txt"
				}
				if err := root.Symlink(target, "link.txt"); err != nil {
					t.Skipf("cannot create symlink: %v", err)
				}
				path = "link.txt"
			case "hardlink":
				if err := root.Link(path, "alias.txt"); err != nil {
					t.Skipf("cannot create hard link: %v", err)
				}
			case "directory":
				path = "."
			case "parent traversal":
				path = filepath.Join("..", filepath.Base(outside), "outside.txt")
			case "absolute":
				path = outsideFile
			case "parent symlink escape", "relative parent symlink escape":
				target := outside
				if name == "relative parent symlink escape" {
					var err error
					target, err = filepath.Rel(dir, outside)
					if err != nil {
						t.Fatal(err)
					}
				}
				if err := root.Symlink(target, "escape"); err != nil {
					t.Skipf("cannot create symlink: %v", err)
				}
				path = filepath.Join("escape", "outside.txt")
			case "parent symlink inside":
				if err := root.Mkdir("sub", 0700); err != nil {
					t.Fatal(err)
				}
				if err := root.Rename("document.txt", filepath.Join("sub", "document.txt")); err != nil {
					t.Fatal(err)
				}
				if err := root.Symlink("sub", "alias"); err != nil {
					t.Skipf("cannot create symlink: %v", err)
				}
				path, wantSuccess = filepath.Join("alias", "document.txt"), true
			}
			err := saveEditorFile(root, path, []byte("saved"), fileVersion([]byte("original")), fileSaveIO{})
			if (err == nil) != wantSuccess {
				t.Fatalf("save = %v; success wanted: %v", err, wantSuccess)
			}
			if wantSuccess {
				assertFileSaveContent(t, root, filepath.Join("sub", "document.txt"), "saved")
				assertNoFileSaveStage(t, filepath.Join(dir, "sub"))
			} else {
				assertFileSaveContent(t, root, "document.txt", "original")
			}
			data, readErr := os.ReadFile(outsideFile)
			if readErr != nil || string(data) != "original" {
				t.Fatalf("escaped project: %q, %v", data, readErr)
			}
			if name == "hardlink" {
				assertFileSaveContent(t, root, "alias.txt", "original")
				original, err := root.Stat("document.txt")
				if err != nil {
					t.Fatal(err)
				}
				alias, err := root.Stat("alias.txt")
				if err != nil || !os.SameFile(original, alias) {
					t.Fatalf("hard link identity changed: %v", err)
				}
			}
			assertNoFileSaveStage(t, outside)
			assertNoFileSaveStage(t, dir)
		})
	}
}

func TestFileSaveParentReplacement(t *testing.T) {
	t.Parallel()
	root, dir := fileSaveFixture(t)
	if err := root.Mkdir("sub", 0700); err != nil {
		t.Fatal(err)
	}
	if err := root.Rename("document.txt", filepath.Join("sub", "document.txt")); err != nil {
		t.Fatal(err)
	}
	err := saveEditorFile(root, filepath.Join("sub", "document.txt"), []byte("must not win"), fileVersion([]byte("original")), fileSaveIO{
		beforeCommit: func() error {
			if err := root.Rename("sub", "moved"); err != nil {
				return err
			}
			if err := root.Mkdir("sub", 0700); err != nil {
				return err
			}
			return root.WriteFile(filepath.Join("sub", "document.txt"), []byte("external"), 0600)
		},
	})
	if !errors.Is(err, errFileSaveConflict) {
		t.Fatalf("parent replacement = %v", err)
	}
	assertFileSaveContent(t, root, filepath.Join("moved", "document.txt"), "original")
	assertFileSaveContent(t, root, filepath.Join("sub", "document.txt"), "external")
	assertNoFileSaveStage(t, filepath.Join(dir, "moved"))
	assertNoFileSaveStage(t, filepath.Join(dir, "sub"))
}

func TestFileSaveParentSymlinkRetarget(t *testing.T) {
	for _, outside := range []bool{false, true} {
		name := "inside"
		if outside {
			name = "outside"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root, dir := fileSaveFixture(t)
			if err := root.Mkdir("sub", 0700); err != nil {
				t.Fatal(err)
			}
			if err := root.Rename("document.txt", filepath.Join("sub", "document.txt")); err != nil {
				t.Fatal(err)
			}
			if err := root.Symlink("sub", "alias"); err != nil {
				t.Skipf("cannot create symlink: %v", err)
			}
			next := filepath.Join(dir, "next")
			target := "next"
			if outside {
				next = t.TempDir()
				target = next
			} else if err := root.Mkdir("next", 0700); err != nil {
				t.Fatal(err)
			}
			externalPath := filepath.Join(next, "document.txt")
			if err := os.WriteFile(externalPath, []byte("external"), 0600); err != nil {
				t.Fatal(err)
			}
			err := saveEditorFile(root, filepath.Join("alias", "document.txt"), []byte("must not win"), fileVersion([]byte("original")), fileSaveIO{
				beforeCommit: func() error {
					if err := root.Remove("alias"); err != nil {
						return err
					}
					return root.Symlink(target, "alias")
				},
			})
			if err == nil || (!outside && !errors.Is(err, errFileSaveConflict)) {
				t.Fatalf("parent retarget = %v", err)
			}
			assertFileSaveContent(t, root, filepath.Join("sub", "document.txt"), "original")
			data, err := os.ReadFile(externalPath)
			if err != nil || string(data) != "external" {
				t.Fatalf("retarget destination changed: %q, %v", data, err)
			}
			assertNoFileSaveStage(t, filepath.Join(dir, "sub"))
			assertNoFileSaveStage(t, next)
			assertNoFileSaveStage(t, dir)
		})
	}
}

func TestFileSaveLimits(t *testing.T) {
	for _, test := range []struct {
		name                string
		sourceSize, newSize int
		wantSuccess         bool
	}{
		{name: "at limit", sourceSize: editorFileLimit, newSize: editorFileLimit, wantSuccess: true},
		{name: "source too large", sourceSize: editorFileLimit + 1, newSize: 1},
		{name: "new data too large", sourceSize: 1, newSize: editorFileLimit + 1},
		{name: "empty", sourceSize: 0, newSize: 0, wantSuccess: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, dir := fileSaveFixture(t)
			original := bytes.Repeat([]byte("a"), test.sourceSize)
			data := bytes.Repeat([]byte("b"), test.newSize)
			if err := root.WriteFile("document.txt", original, 0600); err != nil {
				t.Fatal(err)
			}
			err := saveEditorFile(root, "document.txt", data, fileVersion(original), fileSaveIO{})
			if (err == nil) != test.wantSuccess {
				t.Fatalf("save = %v; success wanted: %v", err, test.wantSuccess)
			}
			want := original
			if test.wantSuccess {
				want = data
			}
			got, err := root.ReadFile("document.txt")
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("content changed unexpectedly: %v", err)
			}
			assertNoFileSaveStage(t, dir)
		})
	}
}

func TestFileSaveConcurrentReadersSeeWholeFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows open reader handles can deny replacement; no retry-after-validation is used")
	}
	root, dir := fileSaveFixture(t)
	oldData := bytes.Repeat([]byte("a"), 256<<10)
	newData := bytes.Repeat([]byte("b"), 256<<10)
	if err := root.WriteFile("document.txt", oldData, 0600); err != nil {
		t.Fatal(err)
	}
	stop, done := make(chan struct{}), make(chan error, 1)
	observed := make(chan byte, 1)
	go func() {
		for {
			data, err := os.ReadFile(filepath.Join(dir, "document.txt"))
			if err != nil {
				done <- err
				return
			}
			if !bytes.Equal(data, oldData) && !bytes.Equal(data, newData) {
				done <- errors.New("reader observed a partial or mixed file")
				return
			}
			select {
			case observed <- data[0]:
			default:
			}
			select {
			case <-stop:
				done <- nil
				return
			default:
			}
		}
	}()
	t.Cleanup(func() {
		close(stop)
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	waitObserved := func(want byte) {
		t.Helper()
		deadline := time.NewTimer(5 * time.Second)
		defer deadline.Stop()
		for {
			select {
			case got := <-observed:
				if got == want {
					return
				}
			case <-deadline.C:
				t.Fatal("reader did not observe committed state")
			}
		}
	}
	waitObserved('a')
	current := oldData
	for i := 0; i < 12; i++ {
		next := newData
		if i%2 == 1 {
			next = oldData
		}
		if err := saveEditorFile(root, "document.txt", next, fileVersion(current), fileSaveIO{}); err != nil {
			t.Fatal(err)
		}
		waitObserved(next[0])
		current = next
	}
	assertNoFileSaveStage(t, dir)
}
