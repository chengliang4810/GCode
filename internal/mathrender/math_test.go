package mathrender

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderEveryBundledFace(t *testing.T) {
	seen := make(map[string]string)
	for _, family := range []string{"Fira Math", "STIX Two", "Computer Modern", "Libertinus", "Palatino", "Euler"} {
		t.Run(family, func(t *testing.T) {
			formula, err := Render(`\frac{x^2 + \alpha}{\sqrt{y}} = \sum_{i=1}^{n} i`, family)
			if err != nil {
				t.Fatal(err)
			}
			if formula.Width < 20 || formula.Height < 20 || !bytes.Contains(formula.SVG, []byte("<path")) || bytes.Contains(formula.SVG, []byte("<text")) {
				t.Fatalf("no native glyph outlines: %.2f x %.2f", formula.Width, formula.Height)
			}
			if previous, exists := seen[string(formula.SVG)]; exists {
				t.Fatalf("%s reused %s's outlines", family, previous)
			}
			seen[string(formula.SVG)] = family
		})
	}
}

func TestRenderRejectsInvalidAndUnsupportedInput(t *testing.T) {
	for _, source := range []string{"", "$x$", "{x", `\notAnExistingCommand{x}`, strings.Repeat("{", 65) + strings.Repeat("}", 65), strings.Repeat("x", 8193)} {
		t.Run(source[:min(24, len(source))], func(t *testing.T) {
			if _, err := Render(source, "Fira Math"); err == nil {
				t.Fatal("accepted unsupported input")
			}
		})
	}
}

func TestRenderSupportsIndependentModelExpressions(t *testing.T) {
	for _, source := range []string{`E=mc^2`, `\int_0^\infty e^{-x}\,dx=1`, `\sqrt{a^2+b^2}`, `\left(\frac{a}{b}\right)`, `\sin x + \cos x`, `\mathbf{A} \cdot \vec{x}`} {
		t.Run(source, func(t *testing.T) {
			if _, err := Render(source, "STIX Two"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRuntimeMathFontsMatchFrozenAssetManifest(t *testing.T) {
	data, err := os.ReadFile("../assets/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		SourceCommit     string `json:"sourceCommit"`
		RuntimeMathFonts struct {
			Root  string `json:"root"`
			Files []struct {
				Path, Source, SHA256 string
				Bytes                int
			} `json:"files"`
		} `json:"runtimeMathFonts"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SourceCommit != "ab189159f10cf8e63c0502be75a59bbc487e0eaf" || manifest.RuntimeMathFonts.Root != "internal/mathrender" || len(manifest.RuntimeMathFonts.Files) != len(faceFiles) {
		t.Fatal("runtime font provenance is incomplete")
	}
	seen := map[string]bool{}
	for _, file := range manifest.RuntimeMathFonts.Files {
		data, err := fonts.ReadFile(file.Path)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(data)
		if len(data) != file.Bytes || hex.EncodeToString(hash[:]) != file.SHA256 || file.Source != "crates/zatex/fonts/"+filepath.Base(file.Path) {
			t.Fatalf("frozen font changed: %s", file.Path)
		}
		seen[file.Path] = true
	}
	for _, file := range faceFiles {
		if !seen["fonts/"+file] {
			t.Fatalf("font missing from provenance: %s", file)
		}
	}
}
