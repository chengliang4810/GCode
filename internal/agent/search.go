package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	searchTimeout      = 15 * time.Second
	maxSearchEntries   = 100000
	maxSearchFileBytes = 2 << 20
	maxSearchFileLines = 100000
	maxSearchOutput    = 64000
	maxSearchLineBytes = 4000
)

var errSearchPageFull = errors.New("search page is full")

type searchOptions struct {
	Path      string `json:"path"`
	HeadLimit *int   `json:"head_limit"`
	Offset    int    `json:"offset"`
}

type searchPage struct {
	limit     int
	offset    int
	seen      int
	bytes     int
	lines     []string
	truncated bool
	skipped   int
	footer    string
}

func newSearchPage(options searchOptions, defaultLimit int) (*searchPage, error) {
	limit := defaultLimit
	if options.HeadLimit != nil {
		limit = *options.HeadLimit
	}
	if limit < 0 || limit > 2000 || options.Offset < 0 {
		return nil, errors.New("head_limit must be 0..2000 and offset must be nonnegative")
	}
	return &searchPage{limit: limit, offset: options.Offset, lines: []string{}}, nil
}

func (p *searchPage) add(line string) error {
	p.seen++
	if p.seen <= p.offset {
		return nil
	}
	if len(line)+1 > maxSearchOutput {
		return errors.New("one search result exceeds the output budget; narrow the search path")
	}
	if (p.limit > 0 && len(p.lines) >= p.limit) || p.bytes+len(line)+1 > maxSearchOutput {
		p.truncated = true
		return errSearchPageFull
	}
	p.lines = append(p.lines, line)
	p.bytes += len(line) + 1
	return nil
}

func (p *searchPage) text() string {
	text := strings.Join(p.lines, "\n")
	if len(p.lines) == 0 {
		text = "No matches."
		if p.seen > 0 && p.offset > 0 {
			text = "No entries at this offset."
		}
	}
	if p.truncated {
		text += fmt.Sprintf("\n[results truncated; continue with offset: %d]", p.offset+len(p.lines))
	}
	if p.skipped > 0 {
		text += fmt.Sprintf("\n[skipped %d unreadable, binary, non-UTF-8 or oversized files]", p.skipped)
	}
	if p.footer != "" {
		text += "\n" + p.footer
	}
	return text
}

func clipSearchLine(line string) string {
	if len(line) <= maxSearchLineBytes {
		return line
	}
	end := maxSearchLineBytes
	for end > 0 && !utf8.RuneStart(line[end]) {
		end--
	}
	return line[:end] + " … [line truncated]"
}

func decodeSearchInput(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("tool arguments must contain exactly one JSON object")
	}
	return nil
}

func searchRoot(cwd, requested string, allowFile bool) (string, error) {
	if requested == "" {
		requested = "."
	}
	if !filepath.IsAbs(requested) {
		requested = filepath.Join(cwd, requested)
	}
	root, err := filepath.Abs(requested)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && (!allowFile || !info.Mode().IsRegular())) {
		return "", errors.New("search path must be a directory or, for grep, a regular file; symlinks are not followed")
	}
	return root, nil
}

// searchWalkWorkers is how many directories a search reads at a time.
// Directory reads wait for the disk, so a few more than the cores of a
// small machine keep it busy without a burst of them.
const searchWalkWorkers = 8

// searchWalkOptions says which files a search visits.
type searchWalkOptions struct {
	// hidden visits the files and directories whose names start with a
	// dot, as a file-name search does; a content search leaves them out.
	hidden bool
}

// walkSearchFiles visits the files under root in path order, the ones the
// ignore files of the tree and of its parents exclude left out, and the
// hidden ones only when options.hidden says so. Symlinks are not followed.
// It stops when visit returns errSearchPageFull. The file information a
// directory read carries goes with each file, so callers need not stat it
// again.
//
// Directories are read in parallel, up to searchWalkWorkers at a time, and
// their files are sorted before they are visited, so that the order does
// not depend on which directory was read first.
func walkSearchFiles(ctx context.Context, root string, options searchWalkOptions, visit func(string, os.FileInfo) error) error {
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			err = visit(root, info)
		}
		if errors.Is(err, errSearchPageFull) || err == nil {
			return ctx.Err()
		}
		return err
	}
	type walkFile struct {
		path string
		info os.FileInfo
	}
	var (
		mu        sync.Mutex
		files     []walkFile
		failed    error
		remaining = maxSearchEntries - 1
	)
	fail := func(err error) {
		mu.Lock()
		if failed == nil {
			failed = err
		}
		mu.Unlock()
	}
	stopped := func() bool {
		mu.Lock()
		defer mu.Unlock()
		return failed != nil
	}
	sem := make(chan struct{}, searchWalkWorkers)
	var group sync.WaitGroup
	var walk func(directory string, rules []ignoreRule)
	walk = func(directory string, rules []ignoreRule) {
		if err := ctx.Err(); err != nil {
			fail(err)
			return
		}
		if stopped() {
			return
		}
		rules = append(slices.Clone(rules), readIgnoreRules(directory)...)
		entries, err := readSearchDirectory(ctx, directory)
		if err != nil {
			fail(err)
			return
		}
		mu.Lock()
		if len(entries) > remaining {
			mu.Unlock()
			fail(errors.New("search scan limit reached; narrow the search path"))
			return
		}
		remaining -= len(entries)
		mu.Unlock()
		for _, entry := range entries {
			if stopped() {
				return
			}
			if err := ctx.Err(); err != nil {
				fail(err)
				return
			}
			name := entry.Name()
			full := filepath.Join(directory, name)
			if entry.IsDir() {
				if name == ".git" || name == "node_modules" {
					continue
				}
				if !options.hidden && strings.HasPrefix(name, ".") {
					continue
				}
				if ignoredBy(rules, full, true) {
					continue
				}
				// Read the subtree on a worker, or here when they are all
				// busy, so that the goroutines stay bounded.
				select {
				case sem <- struct{}{}:
					group.Add(1)
					go func(directory string, rules []ignoreRule) {
						defer group.Done()
						defer func() { <-sem }()
						walk(directory, rules)
					}(full, rules)
				default:
					walk(full, rules)
				}
				continue
			}
			if !entry.Type().IsRegular() {
				continue
			}
			if !options.hidden && strings.HasPrefix(name, ".") {
				continue
			}
			if ignoredBy(rules, full, false) {
				continue
			}
			file, err := entry.Info()
			if err != nil {
				continue
			}
			mu.Lock()
			files = append(files, walkFile{full, file})
			mu.Unlock()
		}
	}
	walk(root, readParentIgnoreRules(root))
	group.Wait()
	mu.Lock()
	err = failed
	mu.Unlock()
	if err != nil {
		return err
	}
	slices.SortFunc(files, func(a, b walkFile) int {
		return strings.Compare(a.path, b.path)
	})
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := visit(file.path, file.info); err != nil {
			if errors.Is(err, errSearchPageFull) {
				return ctx.Err()
			}
			return err
		}
	}
	return nil
}

func readSearchDirectory(ctx context.Context, name string) ([]os.DirEntry, error) {
	directory, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	entries := []os.DirEntry{}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		batch, err := directory.ReadDir(256)
		entries = append(entries, batch...)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	})
	return entries, nil
}
func displaySearchPath(cwd, name string) string {
	relative, err := filepath.Rel(cwd, name)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(relative)
	}
	return filepath.ToSlash(name)
}

func searchPatternPath(root, name string) string {
	if root == name {
		return filepath.Base(name)
	}
	relative, _ := filepath.Rel(root, name)
	return filepath.ToSlash(relative)
}
