package storage

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"sync"
	"time"
)

const runtimeLogFile = "gcode.log"

// runtimeLog owns the active log and its lock. Rotation and writes are serialized;
// the periodic pruner only visits closed gcode-*.log archives.
type runtimeLog struct {
	mu        sync.Mutex
	store     *Store
	rootID    string
	directory *os.Root
	lock      *os.File
	file      *os.File
	size      int64
	maxBytes  int64
	closed    bool
}

// OpenRuntimeLog opens logs/gcode.log in the active registered root. It rotates
// before a write would exceed 8 MiB. One active file plus at most seven archives
// fit the 8-file/64-MiB budget; archives expire after 14 days. Writes require store
// to remain open. The caller must Close to release the log lock before starting
// a replacement process. Concurrent writers for the same root are rejected
// rather than interleaving rotations.
func OpenRuntimeLog(store *Store) (io.WriteCloser, error) {
	root, id, err := openRuntimeRoot(store)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	directory, err := openRuntimeDirectory(root, "logs")
	if err != nil {
		return nil, err
	}
	writer := &runtimeLog{
		store: store, rootID: id, directory: directory, maxBytes: RuntimeLogMaxFileBytes,
	}
	writer.lock, err = openRuntimeRegular(directory, ".gcode-log.lock", os.O_CREATE|os.O_RDWR)
	if err == nil {
		err = lockFile(writer.lock)
	}
	if err == nil {
		err = writer.open()
	}
	if err == nil {
		err = pruneRuntimeDirectory(directory, ".", runtimeLogRetention, true, time.Now(), nil)
	}
	if err != nil {
		_ = writer.Close()
		return nil, err
	}
	return writer, nil
}

func openRuntimeRegular(root *os.Root, name string, flags int) (*os.File, error) {
	info, err := root.Lstat(name)
	if err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("runtime file must be regular: %s", name)
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	file, err := root.OpenFile(name, flags, 0600)
	if err != nil {
		return nil, err
	}
	current, err := file.Stat()
	if err == nil && !current.Mode().IsRegular() {
		err = fmt.Errorf("runtime file must be regular: %s", name)
	}
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func (w *runtimeLog) open() error {
	file, err := openRuntimeRegular(w.directory, runtimeLogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	w.file, w.size = file, info.Size()
	return nil
}

func (w *runtimeLog) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, fs.ErrClosed
	}
	if _, err := w.store.Resolve(w.rootID); err != nil {
		return 0, err
	}
	written := 0
	for len(data) > 0 {
		if w.file == nil {
			if err := w.open(); err != nil {
				return written, err
			}
		}
		if w.size >= w.maxBytes {
			if err := w.rotate(); err != nil {
				return written, err
			}
		}
		count := min(int64(len(data)), w.maxBytes-w.size)
		n, err := w.file.Write(data[:int(count)])
		w.size += int64(n)
		written += n
		data = data[n:]
		if err != nil {
			return written, err
		}
		if int64(n) != count {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}

func (w *runtimeLog) rotate() error {
	if err := w.file.Close(); err != nil {
		w.file = nil
		return err
	}
	w.file = nil
	archive := "gcode-" + time.Now().UTC().Format("20060102T150405.000000000Z") +
		"-" + strings.ToLower(rand.Text()) + ".log"
	if err := w.directory.Rename(runtimeLogFile, archive); err != nil {
		return err
	}
	if err := w.open(); err != nil {
		return err
	}
	return pruneRuntimeDirectory(w.directory, ".", runtimeLogRetention, true, time.Now(), nil)
}

func (w *runtimeLog) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	var result error
	for _, file := range []*os.File{w.file, w.lock} {
		if file != nil {
			result = errors.Join(result, file.Close())
		}
	}
	if w.directory != nil {
		result = errors.Join(result, w.directory.Close())
	}
	return result
}
