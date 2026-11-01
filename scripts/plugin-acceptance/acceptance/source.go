package acceptance

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"gcode/internal/plugins"
)

var commitPattern = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

func loadCache(cfg Config) ([]CachedSource, error) {
	var sources []CachedSource
	if cfg.SourceRoot != "" {
		root, err := filepath.Abs(cfg.SourceRoot)
		if err != nil {
			return nil, err
		}
		sources = append(sources, CachedSource{Repository: cfg.MarketplaceRepository, Commit: cfg.MarketplaceCommit, Root: root})
	}
	if cfg.Cache == "" {
		return sources, nil
	}
	path := cfg.Cache
	if info, err := os.Stat(path); err != nil {
		return nil, err
	} else if info.IsDir() {
		path = filepath.Join(path, "index.json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cache CacheIndex
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	for _, source := range cache.Sources {
		if source.Repository == "" || !commitPattern.MatchString(source.Commit) || source.Root == "" {
			return nil, errors.New("cache source requires repository, full commit and root")
		}
		if !filepath.IsAbs(source.Root) {
			source.Root = filepath.Join(filepath.Dir(path), source.Root)
		}
		absolute, err := filepath.Abs(source.Root)
		if err != nil {
			return nil, err
		}
		source.Root = absolute
		for _, old := range sources {
			if old.Repository == source.Repository && old.Commit == source.Commit && old.Root != source.Root {
				return nil, errors.New("cache contains ambiguous repository/commit roots")
			}
		}
		sources = append(sources, source)
	}
	return sources, nil
}

type sourceExport struct {
	done    chan struct{}
	root    string
	archive string
	links   map[string]string
	err     error
}

type sourceExporter struct {
	mu        sync.Mutex
	exports   map[string]*sourceExport
	directory string
	cache     []CachedSource
}

func (e *sourceExporter) prepare(ctx context.Context, row *Row, target string) error {
	if !commitPattern.MatchString(row.FixedSource.Commit) {
		return errors.New("source has no verified full commit; resolve and freeze it before acceptance")
	}
	key := strings.ToLower(row.FixedSource.Repository + "@" + row.FixedSource.Commit)
	e.mu.Lock()
	result, exists := e.exports[key]
	if !exists {
		result = &sourceExport{done: make(chan struct{}), root: filepath.Join(e.directory, hash([]byte(key))[:24]), links: map[string]string{}}
		e.exports[key] = result
	}
	e.mu.Unlock()
	if exists {
		select {
		case <-result.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	} else {
		result.archive, result.err = exportRepository(ctx, row.FixedSource, e.cache, result.root, result.links)
		close(result.done)
	}
	if result.err != nil {
		return result.err
	}
	relative := filepath.ToSlash(row.FixedSource.Path)
	if relative == "" {
		relative = "."
	}
	if err := plugins.MaterializePluginSource(ctx, plugins.SourceTree{Root: result.root, Links: result.links}, relative, target); err != nil {
		return err
	}
	row.Evidence = append(row.Evidence, result.archive, result.root+".links.json")
	if row.FixedSource.ManifestTextSHA256 != "" {
		manifest, err := os.ReadFile(filepath.Join(target, ".claude-plugin", "plugin.json"))
		if err != nil {
			return fmt.Errorf("source manifest missing despite snapshot: %w", err)
		}
		if manifestTextHash(manifest) != row.FixedSource.ManifestTextSHA256 {
			return errors.New("source manifest text differs from frozen static snapshot")
		}
	}
	return nil
}

func exportRepository(ctx context.Context, fixed FixedSource, cache []CachedSource, target string, links map[string]string) (string, error) {
	var root string
	for _, source := range cache {
		if strings.EqualFold(source.Repository, fixed.Repository) && strings.EqualFold(source.Commit, fixed.Commit) {
			root = source.Root
			break
		}
	}
	if root == "" {
		return "", fmt.Errorf("no offline source cache for %s@%s", fixed.Repository, fixed.Commit)
	}
	var stdout bytes.Buffer
	if err := localGit(ctx, root, &stdout, "rev-parse", "--verify", fixed.Commit+"^{commit}"); err != nil {
		return "", err
	}
	if !strings.EqualFold(strings.TrimSpace(stdout.String()), fixed.Commit) {
		return "", errors.New("cached Git object does not match frozen source commit")
	}
	if err := os.MkdirAll(target, 0700); err != nil {
		return "", err
	}
	archivePath := filepath.Join(filepath.Dir(target), filepath.Base(target)+".tar")
	file, err := os.OpenFile(archivePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	bounded := &limitedWriter{target: file, limit: 512 << 20}
	runErr := localGit(ctx, root, bounded, "archive", "--format=tar", fixed.Commit)
	closeErr := file.Close()
	if err := errors.Join(runErr, closeErr); err != nil {
		return "", err
	}
	archive, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer archive.Close()
	if err := extractRepository(ctx, archive, target, links); err != nil {
		return "", err
	}
	if err := writeJSON(target+".links.json", links); err != nil {
		return "", err
	}
	return archivePath, nil
}

// The historical investigator decoded Git bytes as text, normalizing CRLF to LF.
// The Git commit and archive/artifact digests still bind the original file bytes.
func manifestTextHash(data []byte) string {
	return hash(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")))
}

type limitedWriter struct {
	target         io.Writer
	written, limit int64
}

func (w *limitedWriter) Write(data []byte) (int, error) {
	if int64(len(data))+w.written > w.limit {
		return 0, errors.New("source archive exceeds 512 MiB acceptance limit")
	}
	n, err := w.target.Write(data)
	w.written += int64(n)
	return n, err
}

func localGit(ctx context.Context, root string, stdout io.Writer, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	global := []string{"--no-replace-objects", "-c", "core.hooksPath=", "-c", "core.attributesFile=", "-c", "protocol.ext.allow=never", "-C", root}
	cmd := exec.CommandContext(ctx, "git", append(global, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "GIT_NO_REPLACE_OBJECTS=1", "GIT_NO_LAZY_FETCH=1", "GIT_ATTR_NOSYSTEM=1")
	cmd.WaitDelay = 2 * time.Second
	var stderr bytes.Buffer
	cmd.Stdout = stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		text := stderr.String()
		if len(text) > 4096 {
			text = text[:4096]
		}
		return fmt.Errorf("local Git snapshot: %w: %s", err, strings.TrimSpace(text))
	}
	return nil
}

func extract(ctx context.Context, reader io.Reader, root string) error {
	return extractTar(ctx, reader, root, nil)
}

func extractRepository(ctx context.Context, reader io.Reader, root string, links map[string]string) error {
	return extractTar(ctx, reader, root, links)
}

func extractTar(ctx context.Context, reader io.Reader, root string, links map[string]string) error {
	archive := tar.NewReader(reader)
	var count int
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeXGlobalHeader {
			for key, value := range header.PAXRecords {
				if key != "comment" || !commitPattern.MatchString(value) {
					return errors.New("source archive has unsupported global PAX metadata")
				}
			}
			continue
		}
		path, err := safePath(root, header.Name)
		if err != nil {
			return err
		}
		count++
		total += header.Size
		if count > 100000 || header.Size > 128<<20 || total > 512<<20 {
			return errors.New("source archive exceeds file/byte acceptance limits")
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0700); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return err
			}
			file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(header.Mode)&0777)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, archive)
			closeErr := file.Close()
			if err := errors.Join(copyErr, closeErr); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if links == nil {
				return fmt.Errorf("source archive contains a symbolic link: %s", header.Name)
			}
			if strings.ContainsRune(header.Linkname, '\x00') {
				return errors.New("source archive has an invalid symbolic link target")
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			key := filepath.ToSlash(relative)
			if _, exists := links[key]; exists {
				return errors.New("duplicate symbolic link in source archive")
			}
			links[key] = header.Linkname
		default:
			return fmt.Errorf("source archive contains a link or nonregular entry: %s", header.Name)
		}
	}
	// Source links remain typed metadata. Only the final candidate materializer
	// creates required internal links. Unrelated repository links require no
	// operating-system privilege and can never redirect an archive write.
	for key := range links {
		path := filepath.Join(root, filepath.FromSlash(key))
		if _, err := os.Lstat(path); err == nil {
			return errors.New("source archive path is both a symbolic link and a regular entry")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		for parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(key))); parent != "."; parent = filepath.ToSlash(filepath.Dir(filepath.FromSlash(parent))) {
			if _, exists := links[parent]; exists {
				return errors.New("source archive entry has a symbolic link as its parent")
			}
		}
	}
	return nil
}

func underSourceRoot(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func safePath(root, name string) (string, error) {
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:") || strings.ContainsRune(name, '\x00') {
		return "", fmt.Errorf("unsafe source path %q", name)
	}
	path := filepath.Join(root, filepath.FromSlash(name))
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("source path escapes root: %q", name)
	}
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if strings.EqualFold(part, ".git") {
			return "", errors.New("source archive must not expose .git")
		}
	}
	return path, nil
}

func treeDigest(root string) (string, error) {
	var paths []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		return "", err
	}
	slices.Sort(paths)
	digest := sha256.New()
	for _, path := range paths {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil || !underSourceRoot(root, resolved) {
				return "", errors.New("artifact symbolic link is broken or escapes the plugin")
			}
			target, err := os.Readlink(path)
			if err != nil {
				return "", err
			}
			_, _ = fmt.Fprintf(digest, "link\x00%s\x00%s\x00", filepath.ToSlash(relative), filepath.ToSlash(target))
			continue
		}
		if !info.Mode().IsRegular() {
			return "", errors.New("artifact contains nonregular file")
		}
		_, _ = fmt.Fprintf(digest, "file\x00%s\x00%d\x00%d\x00", filepath.ToSlash(relative), info.Mode().Perm(), info.Size())
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(digest, file)
		closeErr := file.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return "", err
		}
		_, _ = digest.Write([]byte{0})
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// ArtifactDigest binds a probe's newly installed core-owned bundle to the candidate.
func ArtifactDigest(root string) (string, error) { return treeDigest(root) }
