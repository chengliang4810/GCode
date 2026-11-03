package desktop

import (
	"fmt"
	"math"
	"testing"

	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestSidebarSharedLeftRail(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, width := range []int{900, 1320} {
			for _, font := range []float64{14, 20} {
				t.Run(fmt.Sprintf("dark-%v-width-%d-font-%g", dark, width, font), func(t *testing.T) {
					a := sidebarLayoutApp(t)
					a.state.Sessions[0].Pinned = true
					a.state.Settings.Appearance.FontSize = font
					a.state.Settings.Appearance.Mode = "light"
					if dark {
						a.state.Settings.Appearance.Mode = "dark"
					}
					tt := ui.NewTester(a.View, width, 1400)
					tt.SetTitleBar(ui.TitleBar{Height: 44, Left: 68})
					s := design.Sidebar()
					expected := float32(22) - s.RowPaddingX - s.IconSize/2
					for _, label := range []string{"新建对话", "已置顶", "项目", "对话", "fnbot", "未归属项目的对话", "了解对话输出目录内容", "第二项目的对话"} {
						r, ok := tt.Find(label)
						if !ok {
							t.Fatalf("missing %s", label)
						}
						if math.Abs(float64(r.X-expected)) > 0.5 {
							t.Fatalf("%s left=%g want=%g", label, r.X, expected)
						}
					}
				})
			}
		}
	}
}
