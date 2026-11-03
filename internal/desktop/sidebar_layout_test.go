package desktop

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func sidebarLayoutApp(t *testing.T) *App {
	t.Helper()
	app := testApp(t, "default")
	app.state.Settings.General.Language = "zh-CN"
	app.state.Projects = nil
	app.state.Sessions = nil
	for i, name := range []string{"fnbot", "bzaqsc-new", "terax-ai", "GCode", "rx", "autoregister", "keen-code"} {
		app.state.Projects = append(app.state.Projects, contracts.Project{ID: fmt.Sprint(i), Name: name, DeviceID: "local"})
	}
	titles := []string{"了解对话输出目录内容", "优化当前构建策略", "使用本机代理加速下载", "检查模型服务配置", "识别工作目录中的工具", "继续检查文件变更", "整理对话记录"}
	now := time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)
	for i, title := range titles {
		app.state.Sessions = append(app.state.Sessions, contracts.Session{
			ID: fmt.Sprintf("layout-%d", i), Title: title, ProjectID: "0",
			DeviceID: "local", CreatedAt: now.Add(-time.Duration(i) * time.Hour),
			UpdatedAt: now.Add(-time.Duration(i) * time.Hour),
		})
	}
	for i, title := range []string{"第二项目的对话", "未归属项目的对话"} {
		project := "1"
		if i == 1 {
			project = ""
		}
		app.state.Sessions = append(app.state.Sessions, contracts.Session{
			ID: fmt.Sprintf("extra-%d", i), Title: title, ProjectID: project, DeviceID: "local",
			CreatedAt: now, UpdatedAt: now,
		})
	}
	app = modelSnapshotApp(t, app.state)
	app.now = func() time.Time { return now.Add(17 * time.Hour) }
	app.sidebarWidth = 350
	return app
}

// A nested conversation indents its content, not its box: the selected row's
// wash must stop at the same right edge as a top-level row instead of running
// past the list's padding to the sidebar edge.
func TestSidebarNestedRowKeepsTheListEdge(t *testing.T) {
	for _, width := range []float32{264, 350} {
		t.Run(fmt.Sprintf("width-%g", width), func(t *testing.T) {
			app := sidebarLayoutApp(t)
			app.sidebarWidth = width
			tt := ui.NewTester(app.View, 1440, 1400)
			wash := func(label string) (int, int) {
				t.Helper()
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				row, ok := tt.Find(label)
				if !ok {
					t.Fatalf("missing row %q", label)
				}
				// Sample above the text: the selected row's wash is the only
				// violet-tinted band there.
				y := int(row.Y) + 3
				image := tt.Image()
				first, last := -1, -1
				for x := 0; x < int(width); x++ {
					_, g, b, _ := image.At(x, y).RGBA()
					if int(b>>8)-int(g>>8) >= 6 {
						if first < 0 {
							first = x
						}
						last = x
					}
				}
				if first < 0 {
					t.Fatalf("%q drew no selected wash to measure", label)
				}
				return first, last
			}
			ink := func(label string, band ui.Rect) int {
				t.Helper()
				image := tt.Image()
				left := -1
				for y := int(band.Y) + 4; y < int(band.Y+band.H)-4; y++ {
					for x := 0; x < int(width); x++ {
						r, g, b, _ := image.At(x, y).RGBA()
						if int(r>>8)+int(g>>8)+int(b>>8) < 420 {
							if left < 0 || x < left {
								left = x
							}
							break
						}
					}
				}
				if left < 0 {
					t.Fatalf("%q drew no text to measure", label)
				}
				return left
			}
			measure := func(label string) (first, last, text int) {
				t.Helper()
				first, last = wash(label)
				row, ok := tt.Find(label)
				if !ok {
					t.Fatalf("missing row %q", label)
				}
				return first, last, ink(label, row)
			}
			// The unassigned conversation is the top-level reference; the two
			// project conversations below are both nested.
			topFirst, topLast, topText := measure("未归属项目的对话")
			nestedFirst, nestedLast, nestedText := measure("第二项目的对话")
			if topLast != nestedLast {
				t.Fatalf("nested row wash ends at %d, top-level at %d: the indented row overran the list edge", nestedLast, topLast)
			}
			if got, want := nestedFirst-topFirst, int(design.Sidebar().ChildInset); got != want {
				t.Fatalf("nested row wash starts %dpx past the top-level row, want %d", got, want)
			}
			if got, want := nestedText-topText, int(design.Sidebar().ChildInset); got != want {
				t.Fatalf("nested title indents %dpx past the top-level title, want %d", got, want)
			}
		})
	}
}

func TestSidebarHoverStaysInsideConversationRow(t *testing.T) {
	for _, width := range []float32{264, 350} {
		t.Run(fmt.Sprintf("width-%g", width), func(t *testing.T) {
			app := sidebarLayoutApp(t)
			app.sidebarWidth = width
			tt := ui.NewTester(app.View, 1440, 1400)
			row, ok := tt.Find("第二项目的对话")
			if !ok {
				t.Fatal("nested conversation missing")
			}
			x, y := int(row.X)-2, int(row.Y+row.H/2)
			before := tt.Image().RGBAAt(x, y)
			tt.Move(row.X+row.W/2, row.Y+row.H/2)
			tt.Frame()
			if after := tt.Image().RGBAAt(x, y); after != before {
				t.Fatalf("hover painted outside the shared row: before=%v after=%v", before, after)
			}
		})
	}
}

func TestSidebarLayoutHierarchyAndPagination(t *testing.T) {
	app := sidebarLayoutApp(t)
	tt := ui.NewTester(app.View, 1440, 1400)
	rect := func(label string) ui.Rect {
		t.Helper()
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("missing sidebar control %q", label)
		}
		return r
	}
	newConversation := rect("新建对话")
	list := rect("对话列表")
	projects := rect("项目")
	conversations := rect("对话")
	project := rect("fnbot")
	first := rect("了解对话输出目录内容")
	secondProject := rect("bzaqsc-new")
	unassigned := rect("未归属项目的对话")
	if !(newConversation.Y < list.Y && list.Y < projects.Y && projects.Y < project.Y &&
		project.Y < first.Y && first.Y < secondProject.Y && secondProject.Y < conversations.Y &&
		conversations.Y < unassigned.Y) {
		t.Fatal("project conversations are not nested between their project and the next project")
	}
	if project.H != design.Sidebar().RowHeight || first.H != design.Sidebar().RowHeight {
		t.Fatalf("project/conversation rows do not share reference density: %+v %+v", project, first)
	}
	if tt.HasText("继续检查文件变更") {
		t.Fatal("conversation pagination rendered more than the initial five")
	}
	if err := tt.Click("显示更多"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("继续检查文件变更") || !tt.HasText("整理对话记录") {
		t.Fatal("show more did not reveal the remaining conversations")
	}
	if err := tt.Click("项目"); err != nil {
		t.Fatal(err)
	}
	if r, ok := tt.Find("fnbot"); ok && r.X < app.sidebarWidth {
		t.Fatal("collapsed project section retained project rows")
	}
	if _, ok := tt.Find("了解对话输出目录内容"); ok {
		t.Fatal("collapsing projects retained project conversations")
	}
	if _, ok := tt.Find("未归属项目的对话"); !ok {
		t.Fatal("collapsing projects hid unassigned conversations")
	}
}

func TestSidebarProjectAccordionPreservesDraftAndComposerTarget(t *testing.T) {
	app := sidebarLayoutApp(t)
	app.draft = "保留这段草稿"
	target := app.projectID
	tt := ui.NewTester(app.View, 1440, 1400)
	if err := tt.Click("bzaqsc-new"); err != nil {
		t.Fatal(err)
	}
	if !app.expanded["project:1"] || app.projectID != target || app.draft != "保留这段草稿" {
		t.Fatal("project collapse changed the composer target or draft")
	}
	if tt.HasText("第二项目的对话") || !tt.HasText("了解对话输出目录内容") {
		t.Fatal("project collapse hid the wrong conversations")
	}
	if err := tt.Click("bzaqsc-new"); err != nil {
		t.Fatal(err)
	}
	if app.expanded["project:1"] || !tt.HasText("第二项目的对话") {
		t.Fatal("clicking the closed project did not restore its conversations")
	}
}

func TestSidebarLayoutRenderEvidence(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, width := range []float32{264, 350} {
			for _, size := range []float64{14, 20} {
				t.Run(fmt.Sprintf("dark-%v-width-%g-font-%g", dark, width, size), func(t *testing.T) {
					app := sidebarLayoutApp(t)
					app.sidebarWidth = width
					app.state.Settings.Appearance.FontSize = size
					app.state.Settings.Appearance.Mode = "light"
					if dark {
						app.state.Settings.Appearance.Mode = "dark"
					}
					app.state.Settings.Appearance.ThemeID = ""
					tt := ui.NewTester(app.View, 1440, 960)
					if _, ok := tt.Find("新建对话"); !ok {
						t.Fatal("new conversation navigation is missing")
					}
					if _, ok := tt.Find("搜索对话"); !ok {
						t.Fatal("conversation search action is missing")
					}
					dir := os.Getenv("RCODE_SIDEBAR_EVIDENCE")
					if dir == "" {
						return
					}
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatal(err)
					}
					path := filepath.Join(dir, fmt.Sprintf("dark-%v-width-%g-font-%g.png", dark, width, size))
					file, err := os.Create(path)
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(file, tt.Image())
					closeErr := file.Close()
					if err != nil || closeErr != nil {
						t.Fatalf("sidebar capture: %v %v", err, closeErr)
					}
				})
			}
		}
	}
}
