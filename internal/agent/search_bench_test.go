package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// These benchmarks walk this checkout, which is the corpus the tools meet
// in practice here. Run them with:
//
//	go test ./internal/agent -run '^$' -bench BenchmarkSearch -benchtime 5x
func benchmarkRoot(b *testing.B) string {
	b.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		b.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		b.Skip("benchmark walks the checkout above the package")
	}
	return root
}

func benchmarkTool(b *testing.B, tool Tool, raw string) {
	b.Helper()
	root := benchmarkRoot(b)
	// A warm run, as the tools meet warm caches in use.
	if _, err := tool.Execute(context.Background(), root, json.RawMessage(raw)); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for range b.N {
		if _, err := tool.Execute(context.Background(), root, json.RawMessage(raw)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSearchGlob(b *testing.B) {
	benchmarkTool(b, globTool{}, `{"pattern":"**/*.go","head_limit":0}`)
}

func BenchmarkSearchGlobScoped(b *testing.B) {
	benchmarkTool(b, globTool{}, `{"pattern":"internal/**/*.go","head_limit":0}`)
}

func BenchmarkSearchGrepFiles(b *testing.B) {
	benchmarkTool(b, grepTool{}, `{"pattern":"settingsPageViewport","head_limit":0}`)
}

func BenchmarkSearchGrepContent(b *testing.B) {
	benchmarkTool(b, grepTool{}, `{"pattern":"settingsPageViewport","output_mode":"content","head_limit":0}`)
}

func BenchmarkSearchGrepCount(b *testing.B) {
	benchmarkTool(b, grepTool{}, `{"pattern":"func\\s+\\w+\\(","output_mode":"count","head_limit":0}`)
}
