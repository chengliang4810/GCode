// Package storage owns fixed application configuration and registered data roots.
package storage

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
)

const (
	locationVersion = 2
	rootVersion     = 1
	locationFile    = "location.json"
	rootMarkerFile  = ".gcode-root.json"
	configLockFile  = ".gcode-config.lock"
	rootLockFile    = ".gcode-root.lock"
)

// Root identifies a data directory independently of its current availability.
type Root struct {
	ID        string `json:"id"`
	Directory string `json:"directory"`
}

type location struct {
	Version       int    `json:"version"`
	ActiveRootID  string `json:"activeRootId"`
	PendingRootID string `json:"pendingRootId,omitempty"`
	Roots         []Root `json:"roots"`
}

type rootMarker struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
}

type Options struct {
	Home      string
	Directory string
}

// Store keeps configuration fixed and resolved data roots exclusively locked.
// Selecting another root never copies or removes existing data.
type Store struct {
	mu          sync.Mutex
	anchor      string
	home        string
	location    location
	changeError string
	configLock  *os.File
	rootLocks   map[string]*os.File
}

func Open(options Options) (_ *Store, resultErr error) {
	home := options.Home
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return nil, err
		}
	}
	anchor, err := filepath.Abs(filepath.Join(home, ".gcode"))
	if err != nil {
		return nil, err
	}
	if options.Directory != "" {
		anchor, err = expand(options.Directory, home)
		if err != nil {
			return nil, err
		}
	}
	if err = noSymlinkParents(anchor); err != nil {
		return nil, err
	}
	if err = os.MkdirAll(anchor, 0700); err != nil {
		return nil, err
	}
	configLock, err := lockAt(anchor, configLockFile)
	if err != nil {
		return nil, fmt.Errorf("configuration directory: %w", err)
	}
	s := &Store{anchor: anchor, home: home, configLock: configLock, rootLocks: map[string]*os.File{}}
	defer func() {
		if resultErr != nil {
			_ = s.Close()
		}
	}()
	data, err := readRegular(filepath.Join(anchor, locationFile))
	switch {
	case err == nil:
		// 旧 directory/pending 文件不推断为新注册表，避免误用已有数据。
		var version struct {
			Version int `json:"version"`
		}
		if err = json.Unmarshal(data, &version); err != nil {
			return nil, fmt.Errorf("read data locations: %w", err)
		}
		if version.Version != locationVersion {
			return nil, errors.New("unsupported storage location format; existing data was not changed")
		}
		if err = decodeJSON(data, &s.location); err != nil {
			return nil, fmt.Errorf("read data locations: %w", err)
		}
		if err = validateLocation(s.location, anchor); err != nil {
			return nil, err
		}
	case errors.Is(err, os.ErrNotExist):
		root, lock, initErr := initializeRoot(anchor, true)
		if initErr != nil {
			return nil, initErr
		}
		s.rootLocks[root.ID] = lock
		s.location = location{Version: locationVersion, ActiveRootID: root.ID, Roots: []Root{root}}
		if err = s.saveLocation(s.location); err != nil {
			_ = os.Remove(filepath.Join(root.Directory, rootMarkerFile))
			return nil, err
		}
	default:
		return nil, fmt.Errorf("read data locations: %w", err)
	}
	// 先验证待启用根，旧盘离线不应阻止切换到已登记的新盘。
	if s.location.PendingRootID != "" {
		if _, err = s.resolveLocked(s.location.PendingRootID); err != nil {
			s.changeError = "Cannot activate selected data root: " + err.Error()
		} else {
			next := s.location
			next.ActiveRootID, next.PendingRootID = next.PendingRootID, ""
			if err = s.saveLocation(next); err != nil {
				s.changeError = "Cannot save selected data root: " + err.Error()
			} else {
				previous := s.location.ActiveRootID
				s.location = next
				// 启动切换后旧根变成历史根，后续首次读取时再按需加锁。
				if lock := s.rootLocks[previous]; lock != nil {
					_ = lock.Close()
					delete(s.rootLocks, previous)
				}
			}
		}
	}
	// 已登记目录丢失时不创建空目录，也不回退到另一个默认位置。
	if _, err = s.resolveLocked(s.location.ActiveRootID); err != nil {
		if s.changeError != "" {
			err = errors.Join(err, errors.New(s.changeError))
		}
		return nil, fmt.Errorf("active data root unavailable: %w", err)
	}
	return s, nil
}

func (s *Store) Directory() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, _ := s.rootLocked(s.location.ActiveRootID)
	return root.Directory
}

func (s *Store) ConfigDirectory() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.anchor
}

func (s *Store) ActiveRootID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.location.ActiveRootID
}

// Roots returns registration metadata without opening offline directories.
func (s *Store) Roots() []Root {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.location.Roots)
}

// Root looks up registration metadata without accessing the directory.
func (s *Store) Root(id string) (Root, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rootLocked(id)
}

func (s *Store) rootLocked(id string) (Root, bool) {
	for _, root := range s.location.Roots {
		if root.ID == id {
			return root, true
		}
	}
	return Root{}, false
}

func (s *Store) Pending() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, _ := s.rootLocked(s.location.PendingRootID)
	return root.Directory
}

func (s *Store) ChangeError() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.changeError
}

// Resolve checks registered identity on every access and locks on first use.
// Missing registered directories are never recreated.
func (s *Store) Resolve(rootID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resolveLocked(rootID)
}

func (s *Store) resolveLocked(rootID string) (string, error) {
	if s.configLock == nil {
		return "", errors.New("data store is closed")
	}
	root, ok := s.rootLocked(rootID)
	if !ok {
		return "", errors.New("data root is not registered")
	}
	if err := verifyRoot(root); err != nil {
		return "", err
	}
	if s.rootLocks[root.ID] == nil {
		lock, err := lockAt(root.Directory, rootLockFile)
		if err != nil {
			return "", err
		}
		if err = verifyRoot(root); err != nil {
			_ = lock.Close()
			return "", err
		}
		s.rootLocks[root.ID] = lock
	}
	return root.Directory, nil
}

// RequestDirectory registers a new empty root or selects an existing one.
// Only the next Open activates it; existing data retains its original root ID.
func (s *Store) RequestDirectory(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.configLock == nil {
		return errors.New("data store is closed")
	}
	target, err := expand(path, s.home)
	if err != nil {
		return err
	}
	next := s.location
	next.Roots = slices.Clone(next.Roots)
	for _, root := range next.Roots {
		if !samePath(root.Directory, target) {
			continue
		}
		if root.ID == next.ActiveRootID {
			next.PendingRootID = ""
		} else {
			if _, err = s.resolveLocked(root.ID); err != nil {
				return err
			}
			next.PendingRootID = root.ID
		}
		if err = s.saveLocation(next); err != nil {
			return err
		}
		s.location, s.changeError = next, ""
		return nil
	}
	for _, root := range next.Roots {
		if overlaps(root.Directory, target) {
			return errors.New("data directories must not contain each other")
		}
	}
	if overlaps(s.anchor, target) {
		return errors.New("data directory and configuration directory must not contain each other")
	}
	root, lock, err := initializeRoot(target, false)
	if err != nil {
		return err
	}
	next.Roots = append(next.Roots, root)
	next.PendingRootID = root.ID
	if err = s.saveLocation(next); err != nil {
		// 注册失败仅清理本次创建的身份标记，不碰目录内业务文件。
		_ = os.Remove(filepath.Join(root.Directory, rootMarkerFile))
		_ = lock.Close()
		return err
	}
	s.rootLocks[root.ID] = lock
	s.location, s.changeError = next, ""
	return nil
}

func (s *Store) saveLocation(loc location) error {
	if err := noSymlinkParents(s.anchor); err != nil {
		return err
	}
	return writeJSON(filepath.Join(s.anchor, locationFile), loc)
}

// Save writes small application configuration at the fixed entrance.
// Data owners use Resolve and WriteJSON for root-bound session and artifact data.
func (s *Store) Save(name string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.configLock == nil {
		return errors.New("data store is closed")
	}
	if filepath.Base(name) != name || filepath.VolumeName(name) != "" ||
		name == "." || name == "" || strings.ContainsAny(name, `\/:`) ||
		slices.Contains([]string{locationFile, rootMarkerFile, configLockFile, rootLockFile, "runtime.lock"}, name) {
		return errors.New("invalid application configuration filename")
	}
	if err := noSymlinkParents(s.anchor); err != nil {
		return err
	}
	return writeJSON(filepath.Join(s.anchor, name), value)
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.configLock == nil {
		return nil
	}
	var result error
	for id, lock := range s.rootLocks {
		result = errors.Join(result, lock.Close())
		delete(s.rootLocks, id)
	}
	result = errors.Join(result, s.configLock.Close())
	s.configLock = nil
	return result
}

func initializeRoot(directory string, configRoot bool) (Root, *os.File, error) {
	if err := noSymlinkParents(directory); err != nil {
		return Root{}, nil, err
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return Root{}, nil, err
	}
	if err := emptyRoot(directory, configRoot); err != nil {
		return Root{}, nil, err
	}
	lock, err := lockAt(directory, rootLockFile)
	if err != nil {
		return Root{}, nil, err
	}
	fail := func(err error) (Root, *os.File, error) {
		_ = lock.Close()
		return Root{}, nil, err
	}
	// 加锁后再次检查，避免初次检查与锁定之间出现不相干文件。
	if err = emptyRoot(directory, configRoot); err != nil {
		return fail(err)
	}
	root := Root{ID: "root-" + strings.ToLower(rand.Text()), Directory: directory}
	if err = writeJSON(filepath.Join(directory, rootMarkerFile), rootMarker{Version: rootVersion, ID: root.ID}); err != nil {
		return fail(err)
	}
	return root, lock, nil
}

func emptyRoot(directory string, configRoot bool) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == rootLockFile || configRoot && entry.Name() == configLockFile {
			continue
		}
		return errors.New("choose an empty data directory; unregistered existing data is not imported")
	}
	return nil
}

func verifyRoot(root Root) error {
	if err := noSymlinkParents(root.Directory); err != nil {
		return fmt.Errorf("data root %s: %w", root.ID, err)
	}
	info, err := os.Stat(root.Directory)
	if err != nil {
		return fmt.Errorf("data root %s at %s: %w", root.ID, root.Directory, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("data root %s is not a directory", root.ID)
	}
	data, err := readRegular(filepath.Join(root.Directory, rootMarkerFile))
	if err != nil {
		return fmt.Errorf("data root %s identity unavailable: %w", root.ID, err)
	}
	var marker rootMarker
	if err = decodeJSON(data, &marker); err != nil {
		return fmt.Errorf("data root %s identity invalid: %w", root.ID, err)
	}
	if marker.Version != rootVersion || marker.ID != root.ID {
		return fmt.Errorf("data root %s identity does not match its registration", root.ID)
	}
	return nil
}

func validateLocation(loc location, anchor string) error {
	if len(loc.Roots) == 0 || loc.ActiveRootID == "" {
		return errors.New("invalid storage root registry")
	}
	ids := map[string]bool{}
	for i, root := range loc.Roots {
		if !validRootID(root.ID) || ids[root.ID] || !filepath.IsAbs(root.Directory) ||
			filepath.Clean(root.Directory) != root.Directory {
			return errors.New("invalid storage root registration")
		}
		if !samePath(root.Directory, anchor) && overlaps(root.Directory, anchor) {
			return errors.New("registered data root overlaps the configuration directory")
		}
		ids[root.ID] = true
		for _, prior := range loc.Roots[:i] {
			if overlaps(prior.Directory, root.Directory) {
				return errors.New("registered data roots must not overlap")
			}
		}
	}
	if !ids[loc.ActiveRootID] || loc.PendingRootID != "" && (!ids[loc.PendingRootID] || loc.PendingRootID == loc.ActiveRootID) {
		return errors.New("active or pending data root is not registered")
	}
	return nil
}

func validRootID(id string) bool {
	if len(id) == 0 || len(id) > 80 {
		return false
	}
	for _, r := range id {
		if r != '-' && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func expand(path, home string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("data directory is required")
	}
	if path == "~" {
		path = home
	} else if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		path = filepath.Join(home, path[2:])
	}
	if !filepath.IsAbs(path) {
		return "", errors.New("data directory must be an absolute path")
	}
	return filepath.Clean(path), nil
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func overlaps(a, b string) bool {
	if samePath(a, b) {
		return true
	}
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err == nil && filepath.IsLocal(rel) {
			return true
		}
	}
	return false
}

func noSymlinkParents(path string) error {
	for {
		info, err := os.Lstat(path)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("storage path must not use symlinks: %s", path)
			}
			if !info.IsDir() {
				return fmt.Errorf("storage parent is not a directory: %s", path)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil
		}
		path = parent
	}
}

func readRegular(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("storage metadata must be a regular file")
	}
	return os.ReadFile(path)
}

func decodeJSON(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("storage metadata must contain exactly one JSON value")
	}
	return nil
}

func lockAt(directory, name string) (*os.File, error) {
	path := filepath.Join(directory, name)
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return nil, errors.New("storage lock must be a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = lockFile(f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("storage directory is already in use: %w", err)
	}
	return f, nil
}

// WriteJSON replaces one application-owned file after flushing its contents.
func WriteJSON(path string, value any) error { return writeJSON(path, value) }

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return WriteJSONBytes(path, data)
}

// WriteJSONBytes atomically replaces an application-owned file with already
// encoded JSON. It validates the bytes without encoding the value again.
func WriteJSONBytes(path string, data []byte) error {
	if !json.Valid(data) {
		return errors.New("invalid encoded JSON")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".write-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	err = errors.Join(f.Sync(), f.Close())
	if err != nil {
		return err
	}
	return replaceFile(temp, path)
}
