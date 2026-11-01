package desktop

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSidebarEmptyStateFollowsConversationHeading(t *testing.T) {
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
					heading, ok := tt.Find("对话")
					if !ok {
						t.Fatal("conversation heading is missing")
					}
					empty, ok := tt.Find("暂无对话")
					if !ok {
						t.Fatal("empty conversation message is missing")
					}
					gap := empty.Y - (heading.Y + heading.H)
					if gap < 0 || gap > 16 {
						t.Fatalf("empty state must follow its heading, gap=%g heading=%+v empty=%+v", gap, heading, empty)
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
					if err := tt.Click("对话"); err != nil {
						t.Fatal(err)
					}
					if tt.HasText("暂无对话") {
						t.Fatal("collapsed conversation section retained its empty state")
					}
				})
			}
		}
	}
}
