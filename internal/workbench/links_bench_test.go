package workbench

import (
	"fmt"
	"testing"

	"gcode/internal/contracts"
)

func BenchmarkLinkRoot(b *testing.B) {
	for _, count := range []int{1, 100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			s := New(nil, nil)
			b.Cleanup(func() { _ = s.Close() })
			snapshot := contracts.Snapshot{}
			for i := range count {
				snapshot.Sessions = append(snapshot.Sessions, contracts.Session{
					ID: fmt.Sprint(i), CWD: "/workspace",
				})
			}
			s.Update(snapshot)
			s.SwitchSession("0")
			b.ReportAllocs()
			for b.Loop() {
				if root, err := s.LinkRoot("0"); err != nil || root != "/workspace" {
					b.Fatalf("root=%q error=%v", root, err)
				}
			}
		})
	}
}
