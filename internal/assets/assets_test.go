package assets

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestAppIconMatchesDistributionAndDecodes(t *testing.T) {
	data, err := os.ReadFile("../../mygo.json")
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Icon string `json:"icon"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	packaged, err := os.ReadFile("../../" + config.Icon)
	if err != nil {
		t.Fatal(err)
	}
	icon := AppIcon()
	if !bytes.Equal(icon, packaged) {
		t.Fatal("native window and distribution use different application icons")
	}
	original, err := Read("app-icon.png")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(icon, original) {
		t.Fatal("application still uses frozen upstream branding")
	}
	image, err := png.Decode(bytes.NewReader(icon))
	if err != nil {
		t.Fatal(err)
	}
	bounds := image.Bounds()
	if bounds.Dx() != bounds.Dy() || bounds.Dx() < 256 {
		t.Fatalf("application icon must be square and at least 256 pixels: %v", bounds)
	}
	for _, point := range [][2]int{{0, 0}, {bounds.Max.X - 1, 0}, {0, bounds.Max.Y - 1}, {bounds.Max.X - 1, bounds.Max.Y - 1}} {
		if _, _, _, alpha := image.At(point[0], point[1]).RGBA(); alpha != 0 {
			t.Fatal("rounded application icon must have transparent outer corners")
		}
	}
	icon[0] ^= 0xff
	if !bytes.Equal(AppIcon(), packaged) {
		t.Fatal("caller can mutate the embedded application icon")
	}
}

func TestAppIconNativeSizes(t *testing.T) {
	for _, size := range []int{16, 20, 24, 32, 40, 48, 64, 96, 128, 256} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			data, err := AppIconForSize(size)
			if err != nil {
				t.Fatal(err)
			}
			img, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if img.Bounds().Dx() != size || img.Bounds().Dy() != size {
				t.Fatalf("native size %d uses %v image", size, img.Bounds())
			}
			// Only the outside of the rounded tile is translucent. Generated
			// source artwork previously carried alpha noise across the interior.
			for y := size / 4; y < size*3/4; y++ {
				for x := size / 4; x < size*3/4; x++ {
					if _, _, _, alpha := img.At(x, y).RGBA(); alpha != 0xffff {
						t.Fatalf("interior pixel (%d,%d) is translucent", x, y)
					}
				}
			}
		})
	}
}

func TestImportedAssetsMatchFrozenManifest(t *testing.T) {
	data, err := os.ReadFile("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		SourceCommit string `json:"sourceCommit"`
		Files        []struct {
			Path, Source, SHA256 string
			Bytes                int
		} `json:"files"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SourceCommit != "ab189159f10cf8e63c0502be75a59bbc487e0eaf" {
		t.Fatal("source drift")
	}
	var imported int
	for _, name := range Names() {
		if !strings.HasPrefix(name, "artifacts/") {
			imported++
		}
	}
	if len(manifest.Files) != imported {
		t.Fatalf("manifest %d files, embedded imports %d", len(manifest.Files), imported)
	}
	for _, file := range manifest.Files {
		t.Run(file.Path, func(t *testing.T) {
			data, err := Read(file.Path)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(data)
			if len(data) != file.Bytes || hex.EncodeToString(hash[:]) != file.SHA256 {
				t.Fatal("imported bytes differ from source manifest")
			}
			if file.Source == "" {
				t.Fatal("missing original path")
			}
		})
	}
}

func TestEveryControlIconCanBeParsed(t *testing.T) {
	for _, name := range Names() {
		if !strings.HasPrefix(name, "icons/") || !strings.HasSuffix(name, ".svg") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			data, err := Read(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ui.ParseSVG(data); err != nil {
				t.Fatal(err)
			}
			if Icon(strings.TrimPrefix(name, "icons/")) == nil {
				t.Fatal("icon resolution failed")
			}
		})
	}
	if Icon("nonexistent-icon") != nil {
		t.Fatal("unknown icon should not be approximated")
	}
}

// The caption glyphs are GCode's own control artwork, so they must stay out
// of the frozen import manifest while still parsing and staying currentColor
// strokes the theme can tint.
func TestCaptionGlyphsParseOutsideTheFrozenManifest(t *testing.T) {
	for _, name := range []string{"caption-minimize", "caption-maximize", "caption-restore", "caption-close"} {
		t.Run(name, func(t *testing.T) {
			data, err := glyphs.ReadFile("glyphs/" + name + ".svg")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(data, []byte("currentColor")) {
				t.Fatal("caption glyph must inherit the control's text color")
			}
			if _, err := ui.ParseSVG(data); err != nil {
				t.Fatal(err)
			}
			if Glyph(name) == nil {
				t.Fatal("glyph resolution failed")
			}
			if _, err := Read("icons/" + name + ".svg"); err == nil {
				t.Fatal("caption glyphs must not enter the frozen upstream icon set")
			}
		})
	}
	if Glyph("nonexistent-caption") != nil {
		t.Fatal("unknown glyph should not be approximated")
	}
}

func TestReadRejectsPathTraversal(t *testing.T) {
	for _, name := range []string{"../go.mod", "/app-icon.png", "icons/../../go.mod"} {
		if _, err := Read(name); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
}
