package plugins

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SourceTree is a complete, read-only source checkout or archive export.
// Links overlays archived symbolic-link metadata without creating OS links.
// Keys are canonical slash-separated relative paths; values are original
// link targets. Only selected plugin links need OS creation during installation.
type SourceTree struct {
	Root  string
	Links map[string]string
}

type sourceTreeReader struct {
	root     string
	handle   *os.Root
	links    map[string]string
	children map[string]map[string]bool
}

func openSourceTree(source SourceTree) (*sourceTreeReader, error) {
	root, err := pluginSourceRoot(source.Root)
	if err != nil {
		return nil, err
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	tree := &sourceTreeReader{
		root: root, handle: handle, links: maps.Clone(source.Links),
		children: map[string]map[string]bool{},
	}
	fail := func(err error) (*sourceTreeReader, error) {
		_ = handle.Close()
		return nil, err
	}
	if len(tree.links) > maxPluginEntries {
		return fail(errors.New("source symbolic-link metadata exceeds its entry limit"))
	}
	nodes := map[string]bool{}
	metadataBytes := 0
	for key, target := range tree.links {
		metadataBytes += len(key) + len(target)
		if metadataBytes > 16<<20 {
			return fail(errors.New("source symbolic-link metadata exceeds its byte limit"))
		}
		if key == "." || path.Clean(key) != key || !filepath.IsLocal(filepath.FromSlash(key)) ||
			strings.ContainsAny(key, "\\:\x00") || target == "" || strings.ContainsRune(target, '\x00') {
			return fail(fmt.Errorf("invalid source symbolic-link metadata path: %q", key))
		}
		parts := strings.Split(key, "/")
		if len(parts) > maxPluginDepth {
			return fail(errors.New("source symbolic-link metadata exceeds its depth limit"))
		}
		for i := range parts {
			name := strings.Join(parts[:i+1], "/")
			if i < len(parts)-1 {
				if _, linked := tree.links[name]; linked {
					return fail(fmt.Errorf("source link metadata has a symbolic-link parent: %s", key))
				}
			}
			if !nodes[name] {
				nodes[name] = true
				metadataBytes += len(name)
				if metadataBytes > 16<<20 {
					return fail(errors.New("source symbolic-link metadata exceeds its byte limit"))
				}
			}
			if len(nodes) > maxPluginEntries {
				return fail(errors.New("source symbolic-link metadata exceeds its node limit"))
			}
			parent := path.Dir(name)
			if tree.children[parent] == nil {
				tree.children[parent] = map[string]bool{}
			}
			tree.children[parent][path.Base(name)] = true
		}
		if _, err := handle.Lstat(filepath.FromSlash(key)); err == nil {
			return fail(fmt.Errorf("source path has both file and symbolic-link metadata: %s", key))
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fail(err)
		}
	}
	return tree, nil
}

func (s *sourceTreeReader) close() error { return s.handle.Close() }

func (s *sourceTreeReader) key(path string) (string, error) {
	relative, err := relativeWithin(s.root, path)
	return filepath.ToSlash(relative), err
}

func (s *sourceTreeReader) lstat(path string) (fs.FileInfo, error) {
	key, err := s.key(path)
	if err != nil {
		return nil, err
	}
	if target, linked := s.links[key]; linked {
		return sourceEntryInfo{name: filepath.Base(path), mode: fs.ModeSymlink | 0777, size: int64(len(target))}, nil
	}
	info, err := s.handle.Lstat(filepath.FromSlash(key))
	if errors.Is(err, fs.ErrNotExist) && len(s.children[key]) != 0 {
		return sourceEntryInfo{name: filepath.Base(path), mode: fs.ModeDir | 0700}, nil
	}
	return info, err
}

func (s *sourceTreeReader) readlink(path string) (string, error) {
	key, err := s.key(path)
	if err != nil {
		return "", err
	}
	if target, linked := s.links[key]; linked {
		return filepath.FromSlash(target), nil
	}
	return s.handle.Readlink(filepath.FromSlash(key))
}

func (s *sourceTreeReader) readDir(path string, visit func(string) error) error {
	key, err := s.key(path)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	directory, err := s.handle.Open(filepath.FromSlash(key))
	if err != nil && !(errors.Is(err, fs.ErrNotExist) && len(s.children[key]) != 0) {
		return err
	}
	if directory != nil {
		defer directory.Close()
		for {
			batch, readErr := directory.ReadDir(1024)
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				return readErr
			}
			for _, entry := range batch {
				seen[entry.Name()] = true
				if err := visit(entry.Name()); err != nil {
					return err
				}
			}
			if errors.Is(readErr, io.EOF) {
				break
			}
		}
	}
	var virtual []string
	for name := range s.children[key] {
		if !seen[name] {
			virtual = append(virtual, name)
		}
	}
	sort.Strings(virtual)
	for _, name := range virtual {
		if err := visit(name); err != nil {
			return err
		}
	}
	return nil
}

type sourceEntryInfo struct {
	name string
	mode fs.FileMode
	size int64
}

func (i sourceEntryInfo) Name() string       { return i.name }
func (i sourceEntryInfo) Size() int64        { return i.size }
func (i sourceEntryInfo) Mode() fs.FileMode  { return i.mode }
func (i sourceEntryInfo) ModTime() time.Time { return time.Time{} }
func (i sourceEntryInfo) IsDir() bool        { return i.mode.IsDir() }
func (i sourceEntryInfo) Sys() any           { return nil }
