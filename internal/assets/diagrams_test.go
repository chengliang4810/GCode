package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

type diagramOutlineManifest struct {
	SourceCommit string
	USVG         string `json:"usvg"`
	FontDB       string `json:"fontdb"`
	Fonts        []struct{ Name, Source, SHA256 string }
	Diagrams     []struct {
		ID, Asset, RawSVG, RawSHA256, SHA256, TextColor, BackgroundColor string
		Width, Height                                                    float64
		Labels                                                           []struct {
			Text         string
			OutlinePaths int
			Bounds       struct{ X, Y, Width, Height float64 }
		}
	}
}

func diagramManifest(t *testing.T) diagramOutlineManifest {
	t.Helper()
	data, err := Read("artifacts/diagram-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest diagramOutlineManifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func svgElements(t *testing.T, data []byte) (map[string]int, float64, float64) {
	t.Helper()
	counts := map[string]int{}
	var width, height float64
	d := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		counts[start.Name.Local]++
		if start.Name.Local == "svg" {
			for _, attr := range start.Attr {
				switch attr.Name.Local {
				case "width":
					width, _ = strconv.ParseFloat(attr.Value, 64)
				case "height":
					height, _ = strconv.ParseFloat(attr.Value, 64)
				}
			}
		}
	}
	return counts, width, height
}

func assertDiagramHash(t *testing.T, data []byte, want string) {
	t.Helper()
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != want {
		t.Fatal("diagram bytes differ from generation manifest")
	}
}

func TestDiagramTextIsOutlinedWithOriginalFonts(t *testing.T) {
	manifest := diagramManifest(t)
	if manifest.SourceCommit != "ab189159f10cf8e63c0502be75a59bbc487e0eaf" || manifest.USVG != "0.46.0" || manifest.FontDB != "0.23.0" || len(manifest.Diagrams) != 30 || len(manifest.Fonts) != 16 {
		t.Fatal("incomplete frozen SVG preparation provenance")
	}
	for _, font := range manifest.Fonts {
		data, err := Read("fonts/" + font.Name)
		if err != nil {
			t.Fatal(err)
		}
		assertDiagramHash(t, data, font.SHA256)
	}
	for _, entry := range manifest.Diagrams {
		t.Run(entry.ID, func(t *testing.T) {
			data, err := Read(entry.Asset)
			if err != nil {
				t.Fatal(err)
			}
			assertDiagramHash(t, data, entry.SHA256)
			raw, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(entry.RawSVG)))
			if err != nil {
				t.Fatal(err)
			}
			assertDiagramHash(t, raw, entry.RawSHA256)
			counts, w, h := svgElements(t, data)
			original, rw, rh := svgElements(t, raw)
			if counts["text"] != 0 || counts["tspan"] != 0 || counts["image"] != 0 || counts["foreignObject"] != 0 {
				t.Fatal("text or raster content remains in native SVG")
			}
			if original["text"] != 6 || len(entry.Labels) != 6 || counts["path"] <= original["path"] {
				t.Fatal("node/edge label outlines were not added")
			}
			if w != rw || h != rh || w != entry.Width || h != entry.Height {
				t.Fatalf("SVG canvas changed: %gx%g vs %gx%g", w, h, rw, rh)
			}
			wantLabels := map[string]bool{"Yes": true, "No": true, "Request": true, "Ready?": true, "Response": true, "Wait": true}
			for _, label := range entry.Labels {
				if !wantLabels[label.Text] || label.OutlinePaths < 1 || label.Bounds.Width <= 0 || label.Bounds.Height <= 0 {
					t.Fatalf("missing outline for label %q", label.Text)
				}
				delete(wantLabels, label.Text)
			}
			if len(wantLabels) != 0 {
				t.Fatal("missing node or edge label metadata")
			}
		})
	}
}

func TestNativeSVGRendererShowsEveryDiagramLabel(t *testing.T) {
	for _, entry := range diagramManifest(t).Diagrams {
		t.Run(entry.ID, func(t *testing.T) {
			data, err := Read(entry.Asset)
			if err != nil {
				t.Fatal(err)
			}
			svg, err := ui.ParseSVG(data)
			if err != nil {
				t.Fatal(err)
			}
			width, height := float32(entry.Width), float32(entry.Height)
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Box(c).Fill().Background(ui.Hex(entry.BackgroundColor)).Children(func() { ui.Image(c, svg).Size(width, height) })
			}, int(math.Ceil(entry.Width)), int(math.Ceil(entry.Height)))
			pixels := tt.Image()
			if directory := os.Getenv("ZERON_DIAGRAM_EVIDENCE"); directory != "" && (entry.ID == "zeron-dark" || entry.ID == "zeron-light") {
				if err := os.MkdirAll(directory, 0755); err != nil {
					t.Fatal(err)
				}
				file, err := os.Create(filepath.Join(directory, "mermaid-textpaths-"+entry.ID+".png"))
				if err != nil {
					t.Fatal(err)
				}
				if err = png.Encode(file, pixels); err != nil {
					t.Fatal(err)
				}
				if err = file.Close(); err != nil {
					t.Fatal(err)
				}
			}
			foreground := ui.Hex(entry.TextColor)
			for _, label := range entry.Labels {
				ink := 0
				for y := max(0, int(math.Floor(label.Bounds.Y))); y < min(pixels.Bounds().Dy(), int(math.Ceil(label.Bounds.Y+label.Bounds.Height))); y++ {
					for x := max(0, int(math.Floor(label.Bounds.X))); x < min(pixels.Bounds().Dx(), int(math.Ceil(label.Bounds.X+label.Bounds.Width))); x++ {
						p := pixels.RGBAAt(x, y)
						delta := func(a, b uint8) int {
							v := int(a) - int(b)
							if v < 0 {
								return -v
							}
							return v
						}
						if p.A > 128 && delta(p.R, foreground.R) < 32 && delta(p.G, foreground.G) < 32 && delta(p.B, foreground.B) < 32 {
							ink++
						}
					}
				}
				if ink < 6 {
					t.Fatalf("native renderer lost %q label (%d foreground pixels)", label.Text, ink)
				}
			}
		})
	}
}
