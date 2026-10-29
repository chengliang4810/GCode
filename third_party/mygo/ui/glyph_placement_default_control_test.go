package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"unsafe"
)

// This byte-identical test is run on the sealed C4 default path and candidate.
// It emits CPU/Tester control evidence only, never a visual-parity reference.
func TestNativeDefaultPaintControl(t *testing.T) {
	target := os.Getenv("QUARTER_DEFAULT_CONTROL_OUT")
	if target == "" {
		t.Skip("explicit fresh output directory is required")
	}
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	records := []map[string]any{}
	for _, dark := range []bool{false, true} {
		for _, scale := range []float32{1, 1.25, 2} {
			var run *node
			tt := coreNewTester(func(c *context) {
				coreBox(c).Fill().Background(RGB(39, 39, 42)).Children(func() {
					run = coreText(c, "AV To fi Tab02.txt 中文 e\u0301 🎉").Label("default").Font("Microsoft YaHei UI").FontSize(10.5).
						FontWeight(400).FontFeatures("kern=0").Absolute().Left(20.2).Top(10)
				})
			}, 400, 90)
			tt.SetDark(dark)
			tt.SetScale(scale)
			tt.Frame()
			image := tt.Image()
			data := image.Pix
			hash := sha256.Sum256(data)
			name := fmt.Sprintf("default-dark%v-scale%v.rgba", dark, scale)
			if err := os.WriteFile(filepath.Join(target, name), data, 0644); err != nil {
				t.Fatal(err)
			}
			bounds, _ := tt.Find("default")
			records = append(records, map[string]any{"dark": dark, "scale": scale, "file": name, "sha256": hex.EncodeToString(hash[:]), "width": image.Rect.Dx(), "height": image.Rect.Dy(), "bytes": len(data), "bounds": bounds, "textParams": run.textParams(0)})
		}
	}
	result := map[string]any{"nodeBytes": unsafe.Sizeof(node{}), "textStyleBytes": unsafe.Sizeof(textStyle{}), "records": records, "meaning": "default implementation consistency; not reference UI or visual parity"}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "result.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}
