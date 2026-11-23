package desktop

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestToastTypesLayoutAndExpiry(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, width := range []int{320, 900} {
			for _, kind := range []toastKind{toastInfo, toastSuccess, toastWarning, toastError} {
				t.Run(fmt.Sprintf("dark=%t/width=%d/kind=%d", dark, width, kind), func(t *testing.T) {
					app, _ := settingsTestApp(t)
					message := "已保存"
					if width == 320 {
						message = strings.Repeat("保存失败，请检查文件权限。", 6)
					}
					app.notification(kind, message)
					if app.toastKind != kind || app.toastUntil.Before(time.Now().Add(2*time.Second)) {
						t.Fatal("notification did not retain its type or lifetime")
					}
					tt := ui.NewTester(func(c *ui.Context) {
						theme := ui.LightTheme()
						if dark {
							theme = ui.DarkTheme()
						}
						c.SetTheme(theme)
						c.Root().Background(theme.Background)
						app.toastView(c)
					}, width, 600)
					box, ok := tt.Find(message)
					if !ok || box.Y < 48 || box.Y > 64 || box.X < 15 || box.X+box.W > float32(width)-15 {
						t.Fatalf("toast is not near the top within the viewport: %+v", box)
					}
					if box.X+box.W/2 < float32(width)/2-2 || box.X+box.W/2 > float32(width)/2+2 {
						t.Fatalf("toast is not horizontally centered: %+v", box)
					}
					if dir := os.Getenv("GCODE_TOAST_EVIDENCE"); dir != "" {
						if err := os.MkdirAll(dir, 0755); err != nil {
							t.Fatal(err)
						}
						f, err := os.Create(filepath.Join(dir, fmt.Sprintf("toast-%t-%d-%d.png", dark, width, kind)))
						if err != nil {
							t.Fatal(err)
						}
						defer f.Close()
						if err := png.Encode(f, tt.Image()); err != nil {
							t.Fatal(err)
						}
					}
					app.toastUntil = time.Now().Add(-time.Second)
					tt.Frame()
					if tt.HasText(message) {
						t.Fatal("expired toast is still visible")
					}
					app.notification(toastSuccess, "替换后的提示")
					tt.Frame()
					if tt.HasText(message) || !tt.HasText("替换后的提示") {
						t.Fatal("new toast did not replace expired message")
					}
				})
			}
		}
	}
}
