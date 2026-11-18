package design

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The interface never separates parts with a middle dot: the layout separates
// them, so the separator would only add a second, weaker hierarchy. The rule
// covers every non-test Go source file and the Chinese catalog, which is where
// the pattern crept in as message prefixes and joined summaries.
func TestInterfaceCopyHasNoSeparatorDots(t *testing.T) {
	root := filepath.Join("..", "..")
	var offenders []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "output", "third_party":
				return filepath.SkipDir
			}
			return nil
		}
		switch {
		case strings.HasSuffix(relative, ".go"):
			if strings.HasSuffix(relative, "_test.go") || strings.HasPrefix(relative, "internal"+string(filepath.Separator)+"design") {
				return nil
			}
		case relative == filepath.Join("internal", "i18n", "zh-CN.json"):
		default:
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "\u00b7") {
				offenders = append(offenders, fmt.Sprintf("%s:%d", relative, i+1))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Fatalf("interface copy separates parts with a middle dot:\n%s", strings.Join(offenders, "\n"))
	}
}
