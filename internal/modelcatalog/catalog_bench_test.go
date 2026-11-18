package modelcatalog

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func BenchmarkCatalogOpen(b *testing.B) {
	for _, warm := range []bool{false, true} {
		name := "first-launch"
		if warm {
			name = "cached-launch"
		}
		b.Run(name, func(b *testing.B) {
			dir := b.TempDir()
			path := filepath.Join(dir, "models-dev.json")
			if warm {
				if err := os.WriteFile(path, seed, 0600); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			for b.Loop() {
				if !warm {
					if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
						b.Fatal(err)
					}
				}
				cache, err := Open(Options{Directory: dir})
				if err != nil {
					b.Fatal(err)
				}
				runtime.KeepAlive(cache)
			}
		})
	}
}
