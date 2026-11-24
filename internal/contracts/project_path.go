package contracts

import "strings"

// SameProjectPath reports whether two absolute project paths name the same
// folder. A host resolves the path with its own rules before storing it: a
// Windows host answers GetFullPathName, which turns the slashes the folder
// browser reported into backslashes. Comparing the browsed path with the
// stored one therefore cannot depend on either platform's separator.
func SameProjectPath(a, b string) bool {
	return normalizeProjectPath(a) == normalizeProjectPath(b)
}

func normalizeProjectPath(path string) string {
	return strings.TrimRight(strings.ReplaceAll(path, `\`, "/"), "/")
}
