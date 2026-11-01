// Package mathrender lays out TeX with a pinned, pure Go engine and the six
// bundled math faces. Output is vector glyph outlines, never a web page.
package mathrender

import (
	"embed"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/go-latex/latex/drawtex"
	"github.com/go-latex/latex/font/ttf"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

//go:embed fonts/*.otf
var fonts embed.FS

var faceFiles = map[string]string{
	"Fira Math":       "FiraMath-Regular.otf",
	"STIX Two":        "STIXTwoMath-Regular.otf",
	"Computer Modern": "NewCMMath-Book.otf",
	"Libertinus":      "LibertinusMath-Regular.otf",
	"Palatino":        "texgyrepagella-math.otf",
	"Euler":           "Euler-Math.otf",
}

type faceEntry struct {
	once sync.Once
	font *sfnt.Font
	err  error
}

var faces = map[string]*faceEntry{
	"Fira Math": {}, "STIX Two": {}, "Computer Modern": {},
	"Libertinus": {}, "Palatino": {}, "Euler": {},
}

func loadFace(name string) (*sfnt.Font, error) {
	entry := faces[name]
	if entry == nil {
		return nil, fmt.Errorf("unknown math font %q", name)
	}
	entry.once.Do(func() {
		data, err := fonts.ReadFile("fonts/" + faceFiles[name])
		if err != nil {
			entry.err = err
			return
		}
		entry.font, entry.err = sfnt.Parse(data)
	})
	return entry.font, entry.err
}

// Formula contains a size-independent monochrome vector image. SVG paths use
// currentColor so selection of a light/dark theme does not rerun TeX layout.
type Formula struct {
	SVG           []byte
	Width, Height float32
}

// Render accepts math expression content without dollar delimiters. The engine
// supports common mathematical TeX, not LaTeX documents, packages or user macros.
// Invalid or unsupported source returns an error for the UI's source fallback.
func Render(source, family string) (formula Formula, err error) {
	// The upstream parser uses panic for unsupported symbols. A model's text is
	// untrusted input, so isolate that API boundary and show the original source.
	defer func() {
		if failure := recover(); failure != nil {
			formula = Formula{}
			err = fmt.Errorf("unsupported math expression: %v", failure)
		}
	}()
	if err := validate(source); err != nil {
		return Formula{}, err
	}
	face, err := loadFace(family)
	if err != nil {
		return Formula{}, err
	}
	canvas := drawtex.New()
	backend := ttf.NewFrom(canvas, &ttf.Fonts{Default: face, Rm: face, It: face, Bf: face, BfIt: face})
	const em = 20
	box, err := layout(source, face, backend, canvas, em)
	if err != nil {
		return Formula{}, err
	}
	var paths strings.Builder
	minX, minY := 0.0, -box.height
	maxX, maxY := box.width, box.depth
	var buffer sfnt.Buffer
	for _, operation := range box.ops {
		switch op := operation.(type) {
		case drawtex.GlyphOp:
			if op.Glyph.Num == 0 {
				return Formula{}, fmt.Errorf("font %s has no glyph for %s", family, op.Glyph.Symbol)
			}
			segments, err := op.Glyph.Font.LoadGlyph(&buffer, op.Glyph.Num, fixed.Int26_6(math.Round(op.Glyph.Size*64)), nil)
			if err != nil {
				return Formula{}, err
			}
			var path strings.Builder
			open := false
			point := func(p fixed.Point26_6) (float64, float64) {
				x, y := op.X+float64(p.X)/64, op.Y+float64(p.Y)/64
				minX, minY, maxX, maxY = min(minX, x), min(minY, y), max(maxX, x), max(maxY, y)
				return x, y
			}
			for _, segment := range segments {
				switch segment.Op {
				case sfnt.SegmentOpMoveTo:
					if open {
						path.WriteByte('Z')
					}
					x, y := point(segment.Args[0])
					fmt.Fprintf(&path, "M%.4f %.4f", x, y)
					open = true
				case sfnt.SegmentOpLineTo:
					x, y := point(segment.Args[0])
					fmt.Fprintf(&path, "L%.4f %.4f", x, y)
				case sfnt.SegmentOpQuadTo:
					x1, y1 := point(segment.Args[0])
					x, y := point(segment.Args[1])
					fmt.Fprintf(&path, "Q%.4f %.4f %.4f %.4f", x1, y1, x, y)
				case sfnt.SegmentOpCubeTo:
					x1, y1 := point(segment.Args[0])
					x2, y2 := point(segment.Args[1])
					x, y := point(segment.Args[2])
					fmt.Fprintf(&path, "C%.4f %.4f %.4f %.4f %.4f %.4f", x1, y1, x2, y2, x, y)
				}
			}
			if open {
				path.WriteByte('Z')
				fmt.Fprintf(&paths, `<path d="%s"/>`, path.String())
			}
		case drawtex.RectOp:
			x, y := min(op.X1, op.X2), min(op.Y1, op.Y2)
			w, h := math.Abs(op.X2-op.X1), math.Abs(op.Y2-op.Y1)
			minX, minY, maxX, maxY = min(minX, x), min(minY, y), max(maxX, x+w), max(maxY, y+h)
			fmt.Fprintf(&paths, `<rect x="%.4f" y="%.4f" width="%.4f" height="%.4f"/>`, x, y, w, h)
		}
	}
	width, height := maxX-minX+2, maxY-minY+2
	if math.IsNaN(width) || math.IsInf(width, 0) || width > 200_000 || height > 200_000 || width <= 0 || height <= 0 {
		return Formula{}, errors.New("math layout exceeds display limits")
	}
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="%.4f %.4f %.4f %.4f" width="%.4f" height="%.4f"><g fill="currentColor">%s</g></svg>`, minX-1, minY-1, width, height, width, height, paths.String())
	return Formula{SVG: []byte(svg), Width: float32(width), Height: float32(height)}, nil
}

func validate(source string) error {
	if strings.TrimSpace(source) == "" {
		return errors.New("empty math expression")
	}
	if len(source) > 8192 || !utf8.ValidString(source) || strings.Contains(source, "$") {
		return errors.New("math expression exceeds supported input limits")
	}
	depth := 0
	for _, r := range source {
		switch r {
		case '{':
			depth++
			if depth > 64 {
				return errors.New("math expression is nested too deeply")
			}
		case '}':
			depth--
			if depth < 0 {
				return errors.New("unbalanced math groups")
			}
		}
	}
	if depth != 0 {
		return errors.New("unbalanced math groups")
	}
	return nil
}
