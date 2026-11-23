package engine

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const editorFileLimit = 16 << 20

var errFileSaveConflict = errors.New("file changed on disk; reopen it before saving")

// fileSaveIO is per-operation fault injection, never shared mutable state.
type fileSaveIO struct {
	createStage  func(*os.Root) (string, fileSaveStage, error)
	beforeCommit func() error
}

type fileSaveStage interface {
	io.Writer
	Chmod(os.FileMode) error
	Sync() error
	Close() error
}

// saveEditorFile never opens the destination with truncation. Parent symlinks
// are allowed only within root; the opened parent pins staging and replacement
// to the same directory even if an ancestor is renamed. Final symlinks and
// multiple hard links are rejected, rather than silently breaking link semantics.
// The final check and Rename are separate operations, NOT cross-process CAS.
// Permission bits are preserved, not owner, ACLs or extended attributes.
// File data is synced before commit; directory crash durability is not promised.
// Rename atomicity is guaranteed by Unix, not by Go on non-Unix platforms.
func saveEditorFile(root *os.Root, path string, data []byte, version uint64, operations fileSaveIO) (err error) {
	if len(data) > editorFileLimit {
		return errors.New("file exceeds the 16 MiB editor limit")
	}
	if !filepath.IsLocal(path) || (runtime.GOOS == "windows" && strings.Contains(path, ":")) {
		return errors.New("file path must stay within the project")
	}
	for _, component := range strings.Split(path, string(filepath.Separator)) {
		if component == ".." {
			return errors.New("file path must not contain parent traversal")
		}
	}
	parentPath, name := filepath.Dir(path), filepath.Base(path)
	if name == "." || name == ".." || strings.HasSuffix(path, string(filepath.Separator)) {
		return errors.New("file path must name a regular file")
	}
	parent, err := root.OpenRoot(parentPath)
	if err != nil {
		return err
	}
	defer parent.Close()
	parentInfo, err := parent.Stat(".")
	if err != nil {
		return err
	}
	original, current, err := readFileSaveSource(parent, name)
	if err != nil {
		return err
	}
	if fileVersion(current) != version {
		return errFileSaveConflict
	}
	create := operations.createStage
	if create == nil {
		create = createFileSaveStage
	}
	temp, stage, err := create(parent)
	if err != nil {
		return err
	}
	defer func() {
		if temp != "" {
			err = errors.Join(err, parent.Remove(temp))
		}
	}()
	mode := original.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	if err := writeFileSaveStage(stage, data, mode); err != nil {
		return err
	}
	if operations.beforeCommit != nil {
		if err := operations.beforeCommit(); err != nil {
			return err
		}
	}
	// Detect ancestor replacement/retargeting as well as leaf changes. All
	// subsequent leaf operations still use the pinned parent, never its name.
	visibleParent, err := root.Stat(parentPath)
	if err != nil {
		return err
	}
	if !os.SameFile(parentInfo, visibleParent) {
		return errFileSaveConflict
	}
	latest, latestData, err := readFileSaveSource(parent, name)
	if err != nil {
		return err
	}
	if !sameFileSaveState(original, latest) || !bytes.Equal(current, latestData) {
		return errFileSaveConflict
	}
	if err := parent.Rename(temp, name); err != nil {
		return err
	}
	temp = ""
	return nil
}

func createFileSaveStage(parent *os.Root) (string, fileSaveStage, error) {
	for {
		name := ".file-save-" + rand.Text() + ".tmp"
		file, err := parent.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", nil, err
		}
		return name, file, nil
	}
}

func writeFileSaveStage(stage fileSaveStage, data []byte, mode os.FileMode) error {
	n, err := stage.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err == nil {
		// Set permissions after writing, which can clear set-ID bits on Unix.
		err = stage.Chmod(mode)
	}
	if err == nil {
		err = stage.Sync()
	}
	return errors.Join(err, stage.Close())
}

func sameFileSaveState(a, b os.FileInfo) bool {
	return os.SameFile(a, b) && a.Mode() == b.Mode() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

func readFileSaveSource(parent *os.Root, name string) (os.FileInfo, []byte, error) {
	before, err := parent.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if before.Mode()&os.ModeSymlink != 0 {
		return nil, nil, errors.New("saving symbolic links is not supported")
	}
	if !before.Mode().IsRegular() {
		return nil, nil, errors.New("file is not a regular file")
	}
	if before.Size() > editorFileLimit {
		return nil, nil, errors.New("file exceeds the 16 MiB editor limit")
	}
	file, err := openFileSaveSource(parent, name)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !sameFileSaveState(before, opened) {
		return nil, nil, errFileSaveConflict
	}
	if err := checkFileSaveLinks(file, opened); err != nil {
		return nil, nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, editorFileLimit+1))
	if err != nil {
		return nil, nil, err
	}
	if len(data) > editorFileLimit {
		return nil, nil, errors.New("file exceeds the 16 MiB editor limit")
	}
	after, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	if err := checkFileSaveLinks(file, after); err != nil {
		return nil, nil, err
	}
	visible, err := parent.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if !sameFileSaveState(opened, after) || !sameFileSaveState(after, visible) {
		return nil, nil, errFileSaveConflict
	}
	return after, data, nil
}
