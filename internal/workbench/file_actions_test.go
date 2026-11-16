package workbench

import (
	"fmt"
	"slices"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/linktarget"
	"gcode/internal/linkui"
	"github.com/egoist/mygo/ui"
)

func TestFileContextRevealsOnlyLocalFiles(t *testing.T) {
	for _, local := range []bool{true, false} {
		t.Run(fmt.Sprint(local), func(t *testing.T) {
			s := linkWorkspace(t)
			s.ProjectID = "project"
			s.snapshot.Files = []contracts.File{{ID: "file", ProjectID: "project", Path: "report.xlsx", Name: "report.xlsx", Kind: "file"}}
			var revealed string
			s.OnLinkConfig = func(string, string) linkui.Config {
				return linkui.Config{
					Resolve: func(raw string) (linkui.Destination, bool) {
						return linkui.Destination{Kind: linktarget.File, Local: local}, true
					},
					Reveal: func(raw string) { revealed = raw },
				}
			}
			tt := ui.NewTester(s.FilesView, 400, 600)
			if err := tt.RightClick("report.xlsx"); err != nil {
				t.Fatal(err)
			}
			if slices.Contains(tt.Menu(), "Show in folder") != local {
				t.Fatalf("incorrect reveal availability: %v", tt.Menu())
			}
			if local {
				if err := tt.ChooseMenuItem("Show in folder"); err != nil {
					t.Fatal(err)
				}
				if revealed != "report.xlsx" {
					t.Fatalf("reveal targeted wrong file: %q", revealed)
				}
			}
		})
	}
}
