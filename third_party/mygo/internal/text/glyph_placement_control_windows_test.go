//go:build windows && (amd64 || arm64)

package text

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"testing"
)

type quarterVector struct {
	Name          string
	PenBits       uint32
	GlyphID       uint32
	IntegerOrigin int
	PhaseIndex    int
	Bounds        [4]int
	RGBHex        string
}

type quarterOracle struct {
	FontFamily     string
	SizeDIP, Scale float32
	Vectors        []quarterVector
}

func loadQuarterOracle(t *testing.T) quarterOracle {
	t.Helper()
	data, err := os.ReadFile("testdata/quarter-coverage.json")
	if err != nil {
		t.Fatal(err)
	}
	var oracle quarterOracle
	if err := json.Unmarshal(data, &oracle); err != nil {
		t.Fatal(err)
	}
	if len(oracle.Vectors) != 76 {
		t.Fatalf("oracle vectors=%d", len(oracle.Vectors))
	}
	return oracle
}

func quarterTestSystem(t *testing.T) (*System, *Font, quarterOracle) {
	t.Helper()
	oracle := loadQuarterOracle(t)
	s := newSystem()
	s.TextParams()
	f := s.font(Style{Family: oracle.FontFamily, Size: oracle.SizeDIP, Weight: 400})
	e := s.engine().(*dwrite)
	if !s.subpixel || !e.clearType || e.aliased || e.bgr || e.clearLevel != 1 {
		t.Fatalf("captured ClearType RGB/level1 premise unavailable: subpixel=%v clear=%v aliased=%v bgr=%v level=%v", s.subpixel, e.clearType, e.aliased, e.bgr, e.clearLevel)
	}
	return s, f, oracle
}

func quarterImageRGB(s *System, g GlyphImage) []byte {
	result := make([]byte, 0, int(g.W)*int(g.H)*3)
	for y := 0; y < int(g.H); y++ {
		for x := 0; x < int(g.W); x++ {
			at := 4 * ((int(g.Y)+y)*s.ColorAtlas.W + int(g.X) + x)
			result = append(result, s.ColorAtlas.Pix[at:at+3]...)
		}
	}
	return result
}

// The adapter requests quarter in the candidate and intentionally runs the
// unmodified native path in C4. The oracle/test itself is byte-identical.
func TestQuarterCoverageExternalSourceControl(t *testing.T) {
	s, f, oracle := quarterTestSystem(t)
	for _, vector := range oracle.Vectors {
		t.Run(vector.Name, func(t *testing.T) {
			pen := math.Float32frombits(vector.PenBits)
			var g GlyphImage
			for attempt := 0; attempt < 8; attempt++ {
				g = quarterRequestedForTest(s, f, vector.GlyphID, oracle.Scale, pen, 0, true)
				if !s.Full() {
					break
				}
				s.MakeRoom()
			}
			got := [4]int{int(float32(math.Floor(float64(pen))) + g.Left), int(g.Top), int(float32(math.Floor(float64(pen)))+g.Left) + int(g.W), int(g.Top) + int(g.H)}
			want := vector.Bounds
			want[0] += vector.IntegerOrigin
			want[2] += vector.IntegerOrigin
			raw, err := hex.DecodeString(vector.RGBHex)
			if err != nil {
				t.Fatal(err)
			}
			actual := quarterImageRGB(s, g)
			if (len(raw) > 0 && (!g.Subpixel || got != want)) || !bytes.Equal(actual, raw) {
				t.Errorf("actual native coverage: bounds=%v want=%v subpixel=%v bytesEqual=%v pen=%v", got, want, g.Subpixel, bytes.Equal(actual, raw), pen)
			}
		})
	}
}
