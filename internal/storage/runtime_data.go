package storage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	// One active 8 MiB log plus archived logs fit the total count/byte bounds.
	RuntimeLogMaxFileBytes     = 8 << 20
	RuntimeLogMaxFiles         = 8
	RuntimeLogMaxBytes         = 64 << 20
	RuntimeLogMaxAge           = 14 * 24 * time.Hour
	RuntimeMaintenanceInterval = 15 * time.Minute

	runtimeCacheDirectory = "cache/gcode"
	runtimeTempDirectory  = "tmp/gcode"
)

type runtimeRetention struct {
	files int
	bytes int64
	age   time.Duration
}

var (
	// Windows may report a delete-pending archive as AccessDenied to a second
	// scanner. Serialize the bounded archive catalogs, rather than treating real
	// permission errors as a successful cleanup.
	runtimeLogPruneMu   sync.Mutex
	runtimeLogRetention = runtimeRetention{
		files: RuntimeLogMaxFiles - 1,
		bytes: RuntimeLogMaxBytes - RuntimeLogMaxFileBytes,
		age:   RuntimeLogMaxAge,
	}
	runtimeCacheRetention = runtimeRetention{files: 1024, bytes: 512 << 20, age: 30 * 24 * time.Hour}
	runtimeTempRetention  = runtimeRetention{files: 256, bytes: 128 << 20, age: 24 * time.Hour}
)

// PrepareRuntime creates application-owned runtime directories only after the
// active registered root has been resolved. It never recreates a missing root.
func PrepareRuntime(store *Store) error {
	root, _, err := openRuntimeRoot(store)
	if err != nil {
		return err
	}
	defer root.Close()
	return prepareRuntime(root)
}

func prepareRuntime(root *os.Root) error {
	for _, name := range []string{"logs", runtimeCacheDirectory, runtimeTempDirectory, "browser", "downloads"} {
		if err := ensureRuntimeDirectory(root, name); err != nil {
			return fmt.Errorf("prepare runtime %s: %w", name, err)
		}
	}
	return nil
}

func openRuntimeRoot(store *Store) (*os.Root, string, error) {
	if store == nil {
		return nil, "", errors.New("runtime storage is not configured")
	}
	id := store.ActiveRootID()
	directory, err := store.Resolve(id)
	if err != nil {
		return nil, "", err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, "", err
	}
	data, err := root.ReadFile(rootMarkerFile)
	var marker rootMarker
	if err == nil {
		err = decodeJSON(data, &marker)
	}
	if err == nil && (marker.Version != rootVersion || marker.ID != id) {
		err = errors.New("runtime root identity changed")
	}
	if err != nil {
		_ = root.Close()
		return nil, "", err
	}
	return root, id, nil
}

func ensureRuntimeDirectory(root *os.Root, name string) error {
	current := ""
	for _, part := range strings.Split(name, "/") {
		current = path.Join(current, part)
		info, err := root.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			if err = root.Mkdir(current, 0700); err != nil && !errors.Is(err, fs.ErrExist) {
				return err
			}
			info, err = root.Lstat(current)
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("runtime directory must be a real directory: %s", current)
		}
	}
	return nil
}

func openRuntimeDirectory(root *os.Root, name string) (*os.Root, error) {
	if err := ensureRuntimeDirectory(root, name); err != nil {
		return nil, err
	}
	before, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	owned, err := root.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	directory, err := owned.Open(".")
	if err == nil {
		var after fs.FileInfo
		after, err = directory.Stat()
		_ = directory.Close()
		if err == nil && (!before.IsDir() || !os.SameFile(before, after)) {
			err = errors.New("runtime directory changed while opening it")
		}
	}
	if err != nil {
		_ = owned.Close()
		return nil, err
	}
	return owned, nil
}

// PruneRuntime retains only bounded application-owned logs, cache and temporary
// files. Browser profiles, sessions, workspaces and worktrees are never visited.
// Cache and temporary files must live in their dedicated gcode namespaces.
func PruneRuntime(store *Store) error {
	return pruneRuntime(store, time.Now(), nil)
}

func pruneRuntime(store *Store, now time.Time, done <-chan struct{}) error {
	root, _, err := openRuntimeRoot(store)
	if err != nil {
		return err
	}
	defer root.Close()
	if err = prepareRuntime(root); err != nil {
		return err
	}
	var result error
	for _, target := range []struct {
		directory string
		retention runtimeRetention
		logs      bool
	}{
		{directory: "logs", retention: runtimeLogRetention, logs: true},
		{directory: runtimeCacheDirectory, retention: runtimeCacheRetention},
		{directory: runtimeTempDirectory, retention: runtimeTempRetention},
	} {
		if err := pruneRuntimeDirectory(root, target.directory, target.retention, target.logs, now, done); err != nil {
			result = errors.Join(result, fmt.Errorf("prune runtime %s: %w", target.directory, err))
		}
	}
	return result
}

type runtimeFile struct {
	name string
	info fs.FileInfo
}

func isRuntimeLog(name string) bool {
	return strings.HasPrefix(name, "gcode-") && strings.HasSuffix(name, ".log")
}

func pruneRuntimeDirectory(
	root *os.Root,
	directory string,
	retention runtimeRetention,
	logs bool,
	now time.Time,
	done <-chan struct{},
) error {
	if logs {
		runtimeLogPruneMu.Lock()
		defer runtimeLogPruneMu.Unlock()
	}
	owned, err := openRuntimeDirectory(root, directory)
	if err != nil {
		return err
	}
	defer owned.Close()
	files := []runtimeFile{}
	var total int64
	err = fs.WalkDir(owned.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		select {
		case <-done:
			return context.Canceled
		default:
		}
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			if logs && name != "." {
				return fs.SkipDir
			}
			return nil
		}
		if logs && !isRuntimeLog(name) {
			return nil
		}
		// WalkDir does not follow links. Do not remove links or special files,
		// even when their target/name looks like an application cache or log.
		info, err := entry.Info()
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		files = append(files, runtimeFile{name: name, info: info})
		total += info.Size()
		return nil
	})
	if err != nil {
		return err
	}
	slices.SortFunc(files, func(a, b runtimeFile) int {
		if cmp := a.info.ModTime().Compare(b.info.ModTime()); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.name, b.name)
	})
	count := len(files)
	var result error
	for _, file := range files {
		select {
		case <-done:
			return context.Canceled
		default:
		}
		if now.Sub(file.info.ModTime()) <= retention.age &&
			count <= retention.files && total <= retention.bytes {
			continue
		}
		// Recheck the entry without following links. Root.Remove stays confined
		// to the opened directory even if a parent is replaced concurrently.
		current, err := owned.Lstat(file.name)
		if err == nil && (!current.Mode().IsRegular() || !os.SameFile(file.info, current) ||
			current.Size() != file.info.Size() || !current.ModTime().Equal(file.info.ModTime())) {
			continue
		}
		if err == nil {
			err = owned.Remove(file.name)
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			result = errors.Join(result, err)
			continue
		}
		count--
		total -= file.info.Size()
	}
	return result
}

// StartRuntimeMaintenance prunes once and every 15 minutes. The caller owns ctx
// and must consume errors until the channel closes; cancellation stops both a
// pending error delivery and directory traversal. No user data is logged here.
func StartRuntimeMaintenance(ctx context.Context, store *Store) <-chan error {
	failures := make(chan error)
	go func() {
		defer close(failures)
		ticker := time.NewTicker(RuntimeMaintenanceInterval)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			if err := pruneRuntime(store, time.Now(), ctx.Done()); err != nil {
				if ctx.Err() != nil {
					return
				}
				select {
				case failures <- err:
				case <-ctx.Done():
					return
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return failures
}
