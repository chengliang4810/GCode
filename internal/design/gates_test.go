package design

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The interface text levels and their exact sizes. The 10 px level is gone:
// the smallest level is 12 px, and the steps below the base size are carried
// by weight and gray level instead of by a smaller size.
func TestInterfaceTextLevelsMeetPixelFloor(t *testing.T) {
	levels := []struct {
		name string
		size float32
		want float32
	}{
		{"TextSmall", TextSmall, 12},
		{"TextCaption", TextCaption, 13},
		{"TextBase", TextBase, 14},
		{"TextLarge", TextLarge, 16},
		{"TextXL", TextXL, 18},
		{"TextTitle", TextTitle, 24},
		{"TextTitleLarge", TextTitleLarge, 30},
	}
	for _, level := range levels {
		if level.size != level.want {
			t.Errorf("%s = %g, want %g", level.name, level.size, level.want)
		}
		if level.size < MinTextSize {
			t.Errorf("%s = %g is below the %g px interface floor", level.name, level.size, MinTextSize)
		}
	}
	if MinTextSize != 12 {
		t.Errorf("MinTextSize = %g, want 12", MinTextSize)
	}
}

// TextSize keeps every level at or above the floor for every interface font
// size the user can pick, and keeps the pixel distance from the 14 px default
// for the levels that stay above it.
func TestTextSizeStaysAbovePixelFloor(t *testing.T) {
	levels := []float32{TextSmall, TextCaption, TextBase, TextLarge, TextXL, TextTitle, TextTitleLarge}
	tt := ui.NewTester(func(c *ui.Context) {
		theme := Palette("rcode", false).UI()
		for base := float32(12); base <= 20; base++ {
			theme.FontSize = base
			c.SetTheme(theme)
			for _, level := range levels {
				got := TextSize(c, level)
				if got < MinTextSize {
					t.Errorf("TextSize(level %g) at base %g = %g, below the %g px floor", level, base, got, MinTextSize)
				}
				if want := textSize(base, level); got != want {
					t.Errorf("TextSize(level %g) at base %g = %g, want %g", level, base, got, want)
				}
			}
		}
	}, 100, 100)
	tt.Frame()
}

// The desktop and workbench code may not spell a font size below the floor.
// Code and terminal text render user content rather than interface copy, so
// their size keeps the range the user configures for them.
func TestInterfaceCodeHasNoSubFloorFontSizeLiterals(t *testing.T) {
	for _, dir := range []string{"../desktop", "../workbench"} {
		root, err := filepath.Abs(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				// A file that does not parse yet is a compile error, not a
				// font size gate failure; the compiler reports it.
				return nil
			}
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok || len(call.Args) != 1 {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "FontSize" {
					return true
				}
				size, ok := intLiteral(call.Args[0])
				if !ok || size >= int(MinTextSize) {
					return true
				}
				if usesUserContentFontSize(call.Args[0]) {
					return true
				}
				t.Errorf("%s: FontSize(%d) is below the %g px interface floor", path, size, MinTextSize)
				return true
			})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// The removed 10 px level must not be referenced anywhere in the tree.
func TestRemovedTextLevelIsNotReferenced(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	skip := map[string]bool{".git": true, "output": true, "third_party": true, "node_modules": true, ".mygo": true, "build": true, ".tmp": true}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skip[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "TextXS") {
			t.Errorf("%s: references the removed TextXS level", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// intLiteral reads a plain integer literal, including a negated one.
func intLiteral(expr ast.Expr) (int, bool) {
	if unary, ok := expr.(*ast.UnaryExpr); ok && unary.Op == token.SUB {
		if value, ok := intLiteral(unary.X); ok {
			return -value, true
		}
		return 0, false
	}
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return 0, false
	}
	value, err := strconv.Atoi(lit.Value)
	if err != nil {
		return 0, false
	}
	return value, true
}

// usesUserContentFontSize reports whether the argument reads the code or
// terminal font size, which the user configures on its own range.
func usesUserContentFontSize(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		if ident.Name == "CodeFontSize" || ident.Name == "TerminalFontSize" {
			found = true
		}
		return true
	})
	return found
}
