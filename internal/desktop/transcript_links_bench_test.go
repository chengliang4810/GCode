package desktop

import (
	"fmt"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/workbench"
	"github.com/egoist/mygo/ui"
)

func BenchmarkTranscriptFileLinks(b *testing.B) {
	for _, count := range []int{100, 5000} {
		b.Run(fmt.Sprintf("files=%d", count), func(b *testing.B) {
			a := &App{workspace: workbench.New(nil, nil)}
			b.Cleanup(func() { _ = a.workspace.Close() })
			a.state.Projects = []contracts.Project{{ID: "project", Path: "/workspace"}}
			a.state.Sessions = []contracts.Session{{ID: "session", ProjectID: "project", CWD: "/workspace"}}
			for i := range count {
				a.state.Files = append(a.state.Files, contracts.File{
					ID: fmt.Sprint(i), ProjectID: "project", Path: fmt.Sprintf("src/package%d/file.go", i), Kind: "file",
				})
			}
			a.workspace.Update(a.state)
			a.workspace.SwitchSession("session")
			targets := []string{"src/package0/file.go", "src/package1/file.go", "src/package2/file.go", "unknown.go"}
			// Warm the same metadata that scrolling an already opened reply uses.
			for _, target := range targets {
				a.linkConfig("session", "", ui.Services{}).Resolve(target)
			}
			b.ReportAllocs()
			for b.Loop() {
				for _, target := range targets {
					a.linkConfig("session", "", ui.Services{}).Resolve(target)
				}
			}
		})
	}
}
