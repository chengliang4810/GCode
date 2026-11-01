package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"
)

// The content search walks the tree itself and matches with Go's own
// regular expressions, so it needs no search program on the machine: a
// host without ripgrep loses nothing. The expressions are the same
// engine ripgrep uses by default, minus the POSIX character classes.

// searchTypeExtensions maps the language filters to the file extensions
// they cover.
var searchTypeExtensions = map[string]map[string]bool{
	"go":   {".go": true},
	"ts":   {".ts": true, ".tsx": true, ".mts": true, ".cts": true},
	"js":   {".js": true, ".jsx": true, ".mjs": true, ".cjs": true},
	"rust": {".rs": true},
	"py":   {".py": true, ".pyi": true},
	"java": {".java": true},
	"c":    {".c": true, ".h": true},
	"cpp":  {".cc": true, ".cpp": true, ".cxx": true, ".hpp": true, ".hh": true, ".hxx": true, ".c++": true, ".h++": true},
	"css":  {".css": true, ".scss": true, ".sass": true, ".less": true},
	"html": {".html": true, ".htm": true, ".xhtml": true},
	"json": {".json": true, ".jsonc": true, ".json5": true, ".webmanifest": true},
	"yaml": {".yaml": true, ".yml": true},
	"md":   {".md": true, ".markdown": true, ".mdx": true},
	"sh":   {".sh": true, ".bash": true, ".zsh": true, ".fish": true, ".ksh": true},
	"toml": {".toml": true},
	"sql":  {".sql": true},
	"xml":  {".xml": true},
}

// runGrep searches the files under root for the pattern, walking the tree
// and matching with Go's regular expressions.
func runGrep(ctx context.Context, cwd, root string, args grepInput, before, after int, page *searchPage) error {
	expression := args.Pattern
	flags := ""
	if args.CaseInsensitive {
		flags += "i"
	}
	if args.Multiline {
		// Line anchors, and a dot that reaches the next line.
		flags += "ms"
	}
	if flags != "" {
		expression = "(?" + flags + ")" + expression
	}
	compiled, err := regexp.Compile(expression)
	if err != nil {
		return fmt.Errorf("pattern is not a valid regular expression: %w", err)
	}
	var filter *pathPattern
	if args.Glob != "" {
		filter, err = compilePathPattern(args.Glob)
		if err != nil {
			return err
		}
	}
	var types map[string]bool
	if args.Type != "" {
		set, ok := searchTypeExtensions[args.Type]
		if !ok {
			return fmt.Errorf("type %q is not one of the known language filters", args.Type)
		}
		// A path filter of its own replaces the language filter, as the
		// search program this replaces did.
		if filter == nil {
			types = set
		}
	}
	search := &grepSearch{
		ctx: ctx, cwd: cwd, args: args, before: before, after: after,
		page: page, expression: compiled,
		counts: map[string]int{}, seen: map[string]bool{},
	}
	// A pattern of plain text is matched by the string search, which is
	// what most searches ask for.
	if !args.CaseInsensitive {
		search.literal = literalPattern(args.Pattern)
	}
	// The files to search, in path order.
	var files []searchFile
	err = walkSearchFiles(ctx, root, searchWalkOptions{}, func(name string, info os.FileInfo) error {
		if filter != nil && !filter.matches(searchPatternPath(root, name)) {
			return nil
		}
		if types != nil && !types[strings.ToLower(filepath.Ext(name))] {
			return nil
		}
		if info.Size() > maxSearchFileBytes {
			return nil
		}
		files = append(files, searchFile{name: name, size: info.Size()})
		return nil
	})
	if err != nil {
		return err
	}
	// The files are read and matched in parallel, in chunks, so that the
	// results keep the order of the paths. Matching is CPU bound, so every
	// core takes a file.
	results := make([]grepFile, len(files))
	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}
	for start := 0; start < len(files); start += workers {
		end := min(start+workers, len(files))
		var group sync.WaitGroup
		for i := start; i < end; i++ {
			group.Add(1)
			go func(i int) {
				defer group.Done()
				results[i] = search.file(files[i])
			}(i)
		}
		group.Wait()
		if err := ctx.Err(); err != nil {
			return err
		}
		for i := start; i < end; i++ {
			search.emit(results[i])
		}
		// A full page of matches needs no more files: a count still adds
		// up every one of them, for its total.
		if search.outputFull && args.OutputMode != "count" {
			break
		}
	}
	if args.OutputMode == "count" {
		total := 0
		for _, name := range search.order {
			total += search.counts[name]
			if search.counts[name] > 0 && !search.outputFull {
				if err := page.add(fmt.Sprintf("%s:%d", name, search.counts[name])); err != nil {
					search.outputFull = true
				}
			}
		}
		if total > 0 {
			page.footer = fmt.Sprintf("Total: %d", total)
		}
	}
	// An explicitly requested file over the size budget is not searched,
	// and says so.
	if info, err := os.Stat(root); err == nil && !info.IsDir() && info.Size() > maxSearchFileBytes {
		page.skipped++
	}
	return nil
}

// literalPattern returns the text of a pattern that has no special
// meaning, and whether it is one.
func literalPattern(pattern string) string {
	if pattern == "" {
		return ""
	}
	for _, r := range pattern {
		switch r {
		case '\\', '.', '[', ']', '{', '}', '(', ')', '*', '+', '?', '|', '^', '$':
			return ""
		}
	}
	return pattern
}

// grepSearch holds what a search keeps while it walks the tree.
type grepSearch struct {
	ctx        context.Context
	cwd        string
	args       grepInput
	before     int
	after      int
	page       *searchPage
	expression *regexp.Regexp
	literal    string
	counts     map[string]int
	order      []string
	seen       map[string]bool
	outputFull bool
}

// searchFile is a file to search and its size, as the walk found it.
type searchFile struct {
	name string
	size int64
}

// grepFile is what one file holds: the entries of a content search, the
// matched lines of a count, and whether it matched at all.
type grepFile struct {
	display string
	entries []string
	count   int
	matched bool
	skipped int
}

// file searches one file.
func (g *grepSearch) file(file searchFile) grepFile {
	result := grepFile{}
	if err := g.ctx.Err(); err != nil {
		return result
	}
	if file.size > maxSearchFileBytes {
		// Over the size budget: not searched.
		return result
	}
	data, skipped := readSearchFile(file.name, file.size)
	if skipped {
		result.skipped++
		return result
	}
	if len(data) == 0 {
		return result
	}
	if bytes.IndexByte(data, 0) >= 0 {
		// Binary: not text to search.
		return result
	}
	text := string(data)
	if !utf8.Valid(data) {
		// Text the encoding does not hold is reported as skipped when it
		// matches, as the search program this replaces did.
		if g.matches(text) {
			result.skipped++
		}
		return result
	}
	// A plain pattern in a file-name listing needs one pass over the
	// text, not its lines.
	if g.literal != "" && !g.args.Multiline && g.args.OutputMode == "files_with_matches" {
		if strings.Contains(text, g.literal) {
			result.matched = true
			result.display = displaySearchPath(g.cwd, file.name)
		}
		return result
	}
	lines, matched := g.lines(text)
	hits := 0
	for _, hit := range matched {
		if hit {
			hits++
		}
	}
	if hits == 0 {
		return result
	}
	result.matched = true
	result.count = hits
	display := displaySearchPath(g.cwd, file.name)
	result.display = display
	if g.args.OutputMode == "content" {
		result.entries = g.content(display, lines, matched)
	}
	return result
}

// readSearchFile reads a file, at most the size budget, and reports
// whether it could not be read.
func readSearchFile(name string, size int64) ([]byte, bool) {
	if size > maxSearchFileBytes {
		// Over the size budget: not searched.
		return nil, false
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, true
	}
	defer file.Close()
	// The head of a file tells a binary from text, so that images and
	// archives are not read to their end only to be skipped.
	head := make([]byte, min(int(size), 8192))
	n, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, true
	}
	head = head[:n]
	if bytes.IndexByte(head, 0) >= 0 {
		return nil, false
	}
	// The rest fills a buffer of the file's size in one read, rather than
	// growing one as it goes.
	data := make([]byte, int(size))
	copy(data, head)
	m, err := io.ReadFull(file, data[n:])
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, true
	}
	return data[:n+m], false
}

// emit adds what a file holds to the page, in the order of the paths.
func (g *grepSearch) emit(result grepFile) {
	g.page.skipped += result.skipped
	if !result.matched {
		return
	}
	switch g.args.OutputMode {
	case "files_with_matches":
		g.addFile(result.display)
	case "count":
		g.record(result.display)
		g.counts[result.display] = result.count
	case "content":
		for _, entry := range result.entries {
			if g.outputFull {
				return
			}
			if err := g.page.add(entry); err != nil {
				g.outputFull = true
				return
			}
		}
	}
}

// addFile adds a file to the page, once.
func (g *grepSearch) addFile(display string) {
	if g.seen[display] {
		return
	}
	g.seen[display] = true
	g.order = append(g.order, display)
	if g.outputFull {
		return
	}
	if err := g.page.add(display); err != nil {
		g.outputFull = true
	}
}

// record keeps a file in the order the search found it.
func (g *grepSearch) record(display string) {
	if g.seen[display] {
		return
	}
	g.seen[display] = true
	g.order = append(g.order, display)
}

// content makes the entries of a file: its matching lines, with the lines
// around them.
func (g *grepSearch) content(display string, lines []string, matched []bool) []string {
	entries := []string{}
	last := -1
	for i := range lines {
		if !matched[i] {
			continue
		}
		from, to := max(0, i-g.before), min(len(lines)-1, i+g.after)
		for l := from; l <= to; l++ {
			if l <= last {
				continue
			}
			separator := "-"
			if matched[l] {
				separator = ":"
			}
			entry := display + separator + clipSearchLine(lines[l])
			if g.args.LineNumbers == nil || *g.args.LineNumbers {
				entry = fmt.Sprintf("%s%s%d%s%s", display, separator, l+1, separator, clipSearchLine(lines[l]))
			}
			entries = append(entries, entry)
			last = l
		}
	}
	return entries
}

// lines splits the text of a file into its lines and marks the ones that
// match. A multiline match marks every line it reaches.
func (g *grepSearch) lines(text string) ([]string, []bool) {
	// Windows line endings end their lines as any other does.
	body := strings.ReplaceAll(text, "\r\n", "\n")
	body = strings.TrimSuffix(body, "\n")
	lines := strings.Split(body, "\n")
	matched := make([]bool, len(lines))
	if !g.args.Multiline {
		for i, line := range lines {
			matched[i] = g.matches(line)
		}
		return lines, matched
	}
	for _, at := range g.findAll(body) {
		start, end := at[0], at[1]
		first := strings.Count(body[:start], "\n")
		last := first
		if end > start {
			last = strings.Count(body[:end-1], "\n")
		}
		for l := first; l <= last && l < len(lines); l++ {
			matched[l] = true
		}
	}
	return lines, matched
}

// matches reports whether the text holds the pattern.
func (g *grepSearch) matches(text string) bool {
	if g.literal != "" {
		return strings.Contains(text, g.literal)
	}
	return g.expression.MatchString(text)
}

// findAll returns the places the pattern holds in the text.
func (g *grepSearch) findAll(text string) [][2]int {
	if g.literal == "" {
		places := g.expression.FindAllStringIndex(text, -1)
		out := make([][2]int, len(places))
		for i, place := range places {
			out[i] = [2]int{place[0], place[1]}
		}
		return out
	}
	var out [][2]int
	for at := 0; at < len(text); {
		index := strings.Index(text[at:], g.literal)
		if index < 0 {
			break
		}
		out = append(out, [2]int{at + index, at + index + len(g.literal)})
		at += index + len(g.literal)
	}
	return out
}
