package agent

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ignoreRule is one pattern of an ignore file, .gitignore or .ignore.
type ignoreRule struct {
	base     string // the directory holding the ignore file
	pattern  string // slash separated, without its trailing slash
	negate   bool   // a pattern starting with ! includes what others exclude
	dirOnly  bool   // a pattern ending with a slash matches directories only
	anchored bool   // a pattern with a slash matches from the base directory
}

// readIgnoreRules reads the ignore files of a directory.
func readIgnoreRules(directory string) []ignoreRule {
	var rules []ignoreRule
	for _, name := range []string{".gitignore", ".ignore"} {
		rules = append(rules, readIgnoreFile(filepath.Join(directory, name), directory)...)
	}
	return rules
}

// readParentIgnoreRules reads the ignore files above a search root, so that
// searching a subdirectory of a repository keeps the rules of its parents,
// as the programs this replaces did.
func readParentIgnoreRules(root string) []ignoreRule {
	var directories []string
	for directory := filepath.Dir(root); ; directory = filepath.Dir(directory) {
		directories = append(directories, directory)
		if filepath.Dir(directory) == directory {
			break
		}
	}
	var rules []ignoreRule
	// The outermost directory's rules come first, so that the rules of a
	// deeper directory override them.
	for i := len(directories) - 1; i >= 0; i-- {
		rules = append(rules, readIgnoreRules(directories[i])...)
		rules = append(rules, readIgnoreFile(filepath.Join(directories[i], ".git", "info", "exclude"), directories[i])...)
	}
	return rules
}

// readIgnoreFile reads one ignore file, its rules relative to base.
func readIgnoreFile(name, base string) []ignoreRule {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil
	}
	var rules []ignoreRule
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rule := ignoreRule{base: base}
		if strings.HasPrefix(line, "!") {
			rule.negate = true
			line = line[1:]
		}
		if strings.HasSuffix(line, "/") {
			rule.dirOnly = true
			line = strings.TrimSuffix(line, "/")
		}
		if strings.HasPrefix(line, "/") {
			line = strings.TrimPrefix(line, "/")
			rule.anchored = true
		} else if strings.Contains(line, "/") {
			rule.anchored = true
		}
		if line == "" {
			continue
		}
		rule.pattern = line
		rules = append(rules, rule)
	}
	return rules
}

// ignoredBy reports whether the ignore rules exclude a path, the last rule
// that matches it deciding, as git does.
func ignoredBy(rules []ignoreRule, name string, isDir bool) bool {
	ignored := false
	for _, rule := range rules {
		if rule.matches(name, isDir) {
			ignored = !rule.negate
		}
	}
	return ignored
}

// matches reports whether the rule covers the path.
func (r ignoreRule) matches(name string, isDir bool) bool {
	if r.dirOnly && !isDir {
		return false
	}
	relative, err := filepath.Rel(r.base, name)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false
	}
	relative = filepath.ToSlash(relative)
	if !r.anchored {
		matched, err := path.Match(r.pattern, path.Base(relative))
		return err == nil && matched
	}
	return matchIgnoreSegments(strings.Split(r.pattern, "/"), strings.Split(relative, "/"))
}

// matchIgnoreSegments matches a pattern's segments against a path's, a **
// segment standing for zero or more directories.
func matchIgnoreSegments(pattern, parts []string) bool {
	if len(pattern) == 0 {
		return len(parts) == 0
	}
	if pattern[0] == "**" {
		if matchIgnoreSegments(pattern[1:], parts) {
			return true
		}
		for i := 1; i <= len(parts); i++ {
			if matchIgnoreSegments(pattern[1:], parts[i:]) {
				return true
			}
		}
		return false
	}
	if len(parts) == 0 {
		return false
	}
	matched, err := path.Match(pattern[0], parts[0])
	return err == nil && matched && matchIgnoreSegments(pattern[1:], parts[1:])
}
