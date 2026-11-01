package plugins

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxPluginCopyBytes = int64(512 << 20)
	maxPluginEntries   = 100_000
	maxPluginDepth     = 128
	maxPluginLinkHops  = 64
)

var errPluginLinkOutside = errors.New("plugin symbolic link escapes its source root")

func relativeWithin(root, path string) (string, error) {
	relative, err := filepath.Rel(root, path)
	if err != nil || !filepath.IsLocal(relative) {
		return "", errPluginLinkOutside
	}
	return relative, nil
}

// resolvePluginPath resolves links one hop at a time without inspecting a path
// outside root. Missing optional component paths may be checked lexically.
func resolvePluginPath(root, path string, allowMissing bool) (string, error) {
	directory, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer directory.Close()
	return resolveSourcePath(&sourceTreeReader{root: root, handle: directory}, path, allowMissing)
}

func resolveSourcePath(source *sourceTreeReader, path string, allowMissing bool) (string, error) {
	root := source.root
	relative, err := relativeWithin(root, path)
	if err != nil {
		return "", err
	}
	remaining := strings.Split(relative, string(filepath.Separator))
	current, hops := root, 0
	for len(remaining) > 0 {
		part := remaining[0]
		remaining = remaining[1:]
		if part == "." || part == "" {
			continue
		}
		next := filepath.Join(current, part)
		info, statErr := source.lstat(next)
		if errors.Is(statErr, fs.ErrNotExist) && allowMissing {
			return filepath.Join(append([]string{next}, remaining...)...), nil
		}
		if statErr != nil {
			return "", statErr
		}
		if info.Mode()&os.ModeSymlink == 0 {
			current = next
			continue
		}
		hops++
		if hops > maxPluginLinkHops {
			return "", fmt.Errorf("plugin symbolic link cycle or chain exceeds %d hops: %s", maxPluginLinkHops, path)
		}
		link, err := source.readlink(next)
		if err != nil {
			return "", err
		}
		destination := link
		if strings.HasPrefix(destination, string(filepath.Separator)) && !filepath.IsAbs(destination) {
			return "", errPluginLinkOutside
		}
		if !filepath.IsAbs(destination) {
			destination = filepath.Join(filepath.Dir(next), link)
		}
		destination = filepath.Clean(destination)
		rel, err := relativeWithin(root, destination)
		if err != nil {
			return "", err
		}
		remaining = append(strings.Split(rel, string(filepath.Separator)), remaining...)
		current = root
	}
	return current, nil
}

func pluginSourceRoot(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("plugin source root must be a real directory")
	}
	// Normalize caller-owned parent links once, then confine all descendant IO.
	return filepath.EvalSymlinks(path)
}

func containsGitDirectory(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return true
	}
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if strings.EqualFold(part, ".git") {
			return true
		}
	}
	return false
}

// MaterializePluginSource copies a plugin from an already obtained marketplace
// tree. Plugin-internal links stay relative links. Links into another part of
// that marketplace are dereferenced; links outside the marketplace are skipped.
// It never executes source code and requires OS permission to create symlinks.
func MaterializePluginSource(ctx context.Context, tree SourceTree, pluginRelative, target string) error {
	sourceTree, err := openSourceTree(tree)
	if err != nil {
		return err
	}
	defer sourceTree.close()
	root := sourceTree.root
	if !filepath.IsLocal(filepath.FromSlash(pluginRelative)) || strings.ContainsAny(pluginRelative, "\\:") {
		return errors.New("plugin source path must remain inside its marketplace")
	}
	source, err := resolveSourcePath(sourceTree, filepath.Join(root, filepath.FromSlash(pluginRelative)), false)
	if err != nil {
		return err
	}
	return copyPluginTree(ctx, sourceTree, source, target)
}

func copyTree(ctx context.Context, source, target string) error {
	tree, err := openSourceTree(SourceTree{Root: source})
	if err != nil {
		return err
	}
	defer tree.close()
	return copyPluginTree(ctx, tree, tree.root, target)
}

type pluginTreeCopier struct {
	ctx        context.Context
	source     *sourceTreeReader
	target     *os.Root
	sourceRoot string
	pluginRoot string
	total      int64
	entries    int
	activeDirs map[string]bool
	links      []struct{ link, path string }
}

func copyPluginTree(ctx context.Context, source *sourceTreeReader, pluginRoot, target string) error {
	sourceRoot := source.root
	target, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	target, err = pluginCopyDestination(target)
	if err != nil {
		return err
	}
	if inside(sourceRoot, target) {
		return errors.New("copy destination must not be inside the source directory")
	}
	info, err := source.lstat(pluginRoot)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("plugin source must be a directory")
	}
	if err := os.MkdirAll(target, 0700); err != nil {
		return err
	}
	if _, err := pluginSourceRoot(target); err != nil {
		return err
	}
	destination, err := os.OpenRoot(target)
	if err != nil {
		return err
	}
	defer destination.Close()
	copy := pluginTreeCopier{
		ctx: ctx, source: source, target: destination, sourceRoot: sourceRoot,
		pluginRoot: pluginRoot, activeDirs: map[string]bool{},
	}
	if err := copy.entry(pluginRoot, ".", pluginRoot, ".", 0); err != nil {
		return err
	}
	// Link targets exist before creation, which also lets Windows distinguish
	// file links from directory links without guessing.
	for _, link := range copy.links {
		if err := destination.Symlink(link.link, link.path); err != nil {
			return fmt.Errorf("preserve plugin symbolic link %s: %w", link.path, err)
		}
	}
	return validatePluginLinks(target)
}

func pluginCopyDestination(path string) (string, error) {
	current := path
	var suffix []string
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if current == path && info.Mode()&os.ModeSymlink != 0 {
				return "", errors.New("plugin copy destination must not be a symbolic link")
			}
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", err
			}
			return filepath.Join(append([]string{resolved}, suffix...)...), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		suffix = append([]string{filepath.Base(current)}, suffix...)
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		current = parent
	}
}

func (c *pluginTreeCopier) entry(path, out, mappedRoot, mappedOut string, depth int) error {
	if err := c.ctx.Err(); err != nil {
		return err
	}
	c.entries++
	if c.entries > maxPluginEntries || depth > maxPluginDepth {
		return errors.New("plugin source exceeds its entry/depth copy limit")
	}
	if containsGitDirectory(c.sourceRoot, path) {
		return nil
	}
	info, err := c.source.lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err := resolveSourcePath(c.source, path, false)
		if errors.Is(err, errPluginLinkOutside) {
			return nil
		}
		if err != nil {
			return err
		}
		if containsGitDirectory(c.sourceRoot, resolved) {
			return nil
		}
		for _, mapping := range []struct{ source, target string }{{c.pluginRoot, "."}, {mappedRoot, mappedOut}} {
			if rel, err := relativeWithin(mapping.source, resolved); err == nil {
				dest := filepath.Join(mapping.target, rel)
				link, err := filepath.Rel(filepath.Dir(out), dest)
				if err != nil {
					return err
				}
				c.links = append(c.links, struct{ link, path string }{link: link, path: out})
				return nil
			}
		}
		// Cross-plugin marketplace resources become ordinary cached files/dirs.
		return c.entry(resolved, out, resolved, out, depth+1)
	}
	relative, err := relativeWithin(c.sourceRoot, path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if c.activeDirs[path] {
			return fmt.Errorf("plugin directory symbolic link cycle: %s", path)
		}
		c.activeDirs[path] = true
		defer delete(c.activeDirs, path)
		if err := c.target.MkdirAll(out, 0700); err != nil {
			return err
		}
		return c.source.readDir(path, func(name string) error {
			return c.entry(filepath.Join(path, name), filepath.Join(out, name), mappedRoot, mappedOut, depth+1)
		})
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("nonregular file in plugin: %s", path)
	}
	remaining := maxPluginCopyBytes - c.total
	if info.Size() > remaining {
		return errors.New("plugin source exceeds 512 MiB copy limit")
	}
	in, err := c.source.handle.Open(relative)
	if err != nil {
		return err
	}
	outFile, err := c.target.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm()&0777)
	if err != nil {
		_ = in.Close()
		return err
	}
	n, copyErr := io.Copy(outFile, io.LimitReader(in, remaining+1))
	c.total += n
	closeErr := errors.Join(outFile.Close(), in.Close())
	if c.total > maxPluginCopyBytes {
		return errors.Join(errors.New("plugin source exceeds 512 MiB copy limit"), closeErr)
	}
	return errors.Join(copyErr, closeErr)
}

// validateStoredLinks checks physical managed entries without traversing linked
// directories. Individual plugin loads impose their own narrower root boundary.
func validateStoredLinks(root string) error {
	normalized, err := pluginSourceRoot(root)
	if err != nil {
		return err
	}
	return filepath.WalkDir(normalized, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && strings.EqualFold(entry.Name(), ".git") {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			_, err := resolvePluginPath(normalized, path, false)
			return err
		}
		return nil
	})
}

func walkPluginFiles(root, start string, visit func(string, fs.FileInfo) error) error {
	directory, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer directory.Close()
	active := map[string]bool{}
	entries := 0
	var walk func(string, int) error
	walk = func(path string, depth int) error {
		entries++
		if entries > maxPluginEntries || depth > maxPluginDepth {
			return errors.New("plugin exceeds its entry/depth traversal limit")
		}
		resolved, err := resolvePluginPath(root, path, false)
		if err != nil {
			return err
		}
		if containsGitDirectory(root, resolved) {
			return errors.New("plugin symbolic link cannot reference .git")
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("nonregular file in plugin: %s", path)
			}
			return visit(path, info)
		}
		if active[resolved] {
			return fmt.Errorf("plugin directory symbolic link cycle: %s", path)
		}
		active[resolved] = true
		defer delete(active, resolved)
		relative, err := relativeWithin(root, resolved)
		if err != nil {
			return err
		}
		children, err := directory.Open(relative)
		if err != nil {
			return err
		}
		defer children.Close()
		for {
			batch, readErr := children.ReadDir(1024)
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				return readErr
			}
			for _, child := range batch {
				if strings.EqualFold(child.Name(), ".git") {
					continue
				}
				if err := walk(filepath.Join(path, child.Name()), depth+1); err != nil {
					return err
				}
			}
			if errors.Is(readErr, io.EOF) {
				return nil
			}
		}
	}
	return walk(start, 0)
}

func validatePluginLinks(root string) error {
	normalized, err := pluginSourceRoot(root)
	if err != nil {
		return err
	}
	return walkPluginFiles(normalized, normalized, func(string, fs.FileInfo) error { return nil })
}

func readPluginDefinition(root, path string) ([]byte, error) {
	if _, err := resolvePluginPath(root, path, false); err != nil {
		return nil, err
	}
	relative, err := relativeWithin(root, path)
	if err != nil {
		return nil, err
	}
	directory, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	file, err := directory.Open(relative)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxDefinitionSize {
		return nil, fmt.Errorf("invalid or oversized definition: %s", path)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxDefinitionSize+1))
	if len(data) > maxDefinitionSize {
		return nil, fmt.Errorf("oversized definition: %s", path)
	}
	return data, err
}
