package desktop

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSidebarEmptyProjectsAndConversationEntry(t *testing.T) {
	for _, scenario := range []string{"empty", "search"} {
		for _, dark := range []bool{false, true} {
			for _, size := range []float64{14, 20} {
				t.Run(fmt.Sprintf("%s-dark-%v-font-%g", scenario, dark, size), func(t *testing.T) {
					app := sidebarLayoutApp(t)
					app.state.Projects = nil
					switch scenario {
					case "empty":
						app.state.Sessions = nil
					case "search":
						app.search = "no matching conversations"
					}
					app.state.Settings.Appearance.FontSize = size
					app.state.Settings.Appearance.Mode = "light"
					if dark {
						app.state.Settings.Appearance.Mode = "dark"
					}
					app.state.Settings.Appearance.ThemeID = ""
					tt := ui.NewTester(app.View, 1200, 1400)
					if _, ok := tt.Find("对话"); !ok {
						t.Fatal("empty conversations heading is missing")
					}
					if _, ok := tt.Find("新建对话"); !ok {
						t.Fatal("new conversation entry is missing")
					}
					heading, ok := tt.Find("项目")
					if !ok {
						t.Fatal("projects heading is missing")
					}
					empty, ok := tt.Find("未添加项目")
					if !ok {
						t.Fatal("empty projects message is missing")
					}
					gap := empty.Y - (heading.Y + heading.H)
					if gap < 0 || gap > 16 {
						t.Fatalf("project empty state gap=%g", gap)
					}

					if dir := os.Getenv("RCODE_SIDEBAR_EVIDENCE"); dir != "" && scenario == "empty" {
						if err := os.MkdirAll(dir, 0755); err != nil {
							t.Fatal(err)
						}
						file, err := os.Create(filepath.Join(dir, fmt.Sprintf("empty-dark-%v-font-%g.png", dark, size)))
						if err != nil {
							t.Fatal(err)
						}
						err = png.Encode(file, tt.Image())
						closeErr := file.Close()
						if err != nil || closeErr != nil {
							t.Fatalf("capture: %v %v", err, closeErr)
						}
					}
					if err := clickAccessible(tt, ui.RoleButton, "项目"); err != nil {
						t.Fatal(err)
					}
					if tt.HasText("未添加项目") {
						t.Fatal("collapsed projects section retained its empty state")
					}
				})
			}
		}
	}
}
