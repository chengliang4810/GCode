package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gcode/internal/contracts"
)

func BenchmarkCollectProjectFiles(b *testing.B) {
	root := b.TempDir()
	for dir := range 24 {
		folder := filepath.Join(root, "nested-directory-for-project-index", fmt.Sprintf("directory-%03d", dir))
		if err := os.MkdirAll(folder, 0700); err != nil {
			b.Fatal(err)
		}
		for i := range 64 {
			if err := os.WriteFile(filepath.Join(folder, fmt.Sprintf("project-source-file-%04d.txt", i)), nil, 0600); err != nil {
				b.Fatal(err)
			}
		}
	}
	project := contracts.Project{ID: "project-benchmark-file-index", Path: root}
	b.ReportAllocs()
	for b.Loop() {
		files, err := collectProjectFiles(b.Context(), project)
		if err != nil {
			b.Fatal(err)
		}
		runtime.KeepAlive(files)
	}
}
