package desktop

import (
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestProjectPickerTruncationChineseNarrowWindow(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		t.Run(theme, func(t *testing.T) {
			a := testApp(t, "default")
			a.state.Settings.General.Language = "zh-CN"
			a.state.Settings.Appearance.Mode = theme
			a.refreshBackground()
			a.service = &truncatedDirectoryService{Service: a.service, listing: contracts.DirectoryListing{
				DeviceID: "local", Path: "/host", Home: "/host", Parent: "/", Truncated: true,
			}}
			a.openNewProject()
			a.projectBrowse("/host")
			tt := ui.NewTester(a.View, 560, 880)
			for _, text := range []string{"部分列表", "文件夹列表不完整", "输入完整路径可直接浏览。"} {
				if !tt.HasText(text) {
					t.Fatalf("missing localized truncation hint: %s", text)
				}
			}
		})
	}
}
