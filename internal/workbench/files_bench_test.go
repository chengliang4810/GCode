package workbench

import (
	"fmt"
	"testing"

	"gcode/internal/contracts"
)

func BenchmarkFileTreeChildren(b *testing.B) {
	s := &State{}
	for i := 0; i < 500; i++ {
		id := fmt.Sprintf("dir-%d", i)
		s.snapshot.Files = append(s.snapshot.Files, contracts.File{ID: id, Name: id, Path: id, Kind: "directory"})
		for j := 0; j < 10; j++ {
			name := fmt.Sprintf("file-%d.go", j)
			s.snapshot.Files = append(s.snapshot.Files, contracts.File{ID: id + "/" + name, ParentID: id, Name: name, Path: id + "/" + name})
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		for _, dir := range s.filesIn("") {
			if len(s.filesIn(dir.ID)) != 10 {
				b.Fatal("missing children")
			}
		}
	}
}
