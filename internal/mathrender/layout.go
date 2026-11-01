package mathrender

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"

	"github.com/go-latex/latex"
	"github.com/go-latex/latex/ast"
	"github.com/go-latex/latex/drawtex"
	"github.com/go-latex/latex/font"
	"github.com/go-latex/latex/font/ttf"
	stdfont "golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// The dependency's AST and symbol backend cover mathematical TeX, but its mtex
// visitor does not implement script AST nodes. This small box compositor owns
// that missing layer; unknown commands fail rather than disappearing.
type mathBox struct {
	width, height, depth float64
	ops                  []drawtex.Op
}

func (box *mathBox) add(child mathBox, x, baseline float64) {
	for _, operation := range child.ops {
		switch op := operation.(type) {
		case drawtex.GlyphOp:
			op.X, op.Y = op.X+x, op.Y+baseline
			box.ops = append(box.ops, op)
		case drawtex.RectOp:
			op.X1, op.X2, op.Y1, op.Y2 = op.X1+x, op.X2+x, op.Y1+baseline, op.Y2+baseline
			box.ops = append(box.ops, op)
		}
	}
	box.width = max(box.width, x+child.width)
	box.height = max(box.height, child.height-baseline)
	box.depth = max(box.depth, child.depth+baseline)
}

type compositor struct {
	face    *sfnt.Font
	backend *ttf.Backend
	canvas  *drawtex.Canvas
}

var mathCommand = regexp.MustCompile(`\\[a-zA-Z]+`)

func layout(source string, face *sfnt.Font, backend *ttf.Backend, canvas *drawtex.Canvas, size float64) (mathBox, error) {
	// These delimiters affect stretchy fence sizing in a complete TeX engine.
	// The native subset uses ordinary fence glyphs and still lays out contents.
	source = mathCommand.ReplaceAllStringFunc(source, func(command string) string {
		switch command {
		case `\left`, `\right`:
			return ""
		case `\mathrm`:
			return `\mathregular`
		case `\text`:
			return `\textregular`
		}
		return command
	})
	node, err := latex.ParseExpr("$" + source + "$")
	if err != nil {
		return mathBox{}, err
	}
	c := compositor{face: face, backend: backend, canvas: canvas}
	return c.node(node, size, "italic"), nil
}

func (c *compositor) node(node ast.Node, size float64, style string) mathBox {
	switch value := node.(type) {
	case ast.List:
		return c.list(value, size, style)
	case *ast.MathExpr:
		return c.list(value.List, size, style)
	case *ast.Arg:
		return c.list(value.List, size, style)
	case *ast.OptArg:
		return c.list(value.List, size, style)
	case *ast.Word:
		return c.text(value.Text, size, style)
	case *ast.Literal:
		return c.text(value.Text, size, style)
	case *ast.Symbol:
		return c.text(value.Text, size, style)
	case *ast.Macro:
		return c.macro(value, size, style)
	default:
		panic(fmt.Errorf("unsupported math node %T", node))
	}
}

func (c *compositor) list(nodes ast.List, size float64, style string) mathBox {
	var out mathBox
	for i := 0; i < len(nodes); i++ {
		var base mathBox
		if macro, ok := nodes[i].(*ast.Macro); ok && accentSymbol(macro.Name.Name) != 0 {
			if i+1 >= len(nodes) {
				panic("missing accent body")
			}
			base = c.node(nodes[i+1], size, style)
			accent := c.glyph(accentSymbol(macro.Name.Name), size*.75)
			var decorated mathBox
			decorated.add(base, 0, 0)
			decorated.add(accent, max(0, (base.width-accent.width)/2), -base.height-size*.08-accent.depth)
			base, i = decorated, i+1
		} else {
			base = c.node(nodes[i], size, style)
		}
		var sub, sup *mathBox
		for i+1 < len(nodes) {
			switch next := nodes[i+1].(type) {
			case *ast.Sub:
				if sub != nil {
					panic("duplicate subscript")
				}
				box := c.node(next.Node, size*.7, style)
				sub = &box
			case *ast.Sup:
				if sup != nil {
					panic("duplicate superscript")
				}
				box := c.node(next.Node, size*.7, style)
				sup = &box
			default:
				goto scriptsDone
			}
			i++
		}
	scriptsDone:
		if sub != nil || sup != nil {
			var grouped mathBox
			grouped.add(base, 0, 0)
			x := base.width + size*.04
			if sup != nil {
				grouped.add(*sup, x, -max(size*.5, base.height*.65)-sup.depth)
			}
			if sub != nil {
				grouped.add(*sub, x, max(size*.25, base.depth+sub.height*.6))
			}
			base = grouped
		}
		out.add(base, out.width, 0)
	}
	return out
}

func accentSymbol(command string) rune {
	switch command {
	case `\vec`:
		return '→'
	case `\hat`, `\widehat`:
		return '^'
	case `\bar`, `\widebar`:
		return '¯'
	case `\tilde`, `\widetilde`:
		return '~'
	case `\dot`:
		return '˙'
	case `\ddot`:
		return '¨'
	}
	return 0
}

func (c *compositor) text(text string, size float64, style string) mathBox {
	var out mathBox
	for _, symbol := range text {
		if symbol == '-' {
			symbol = '−'
		}
		space := float64(0)
		if strings.ContainsRune("=+−<>", symbol) {
			space = size * .15
		}
		r := styledRune(symbol, style)
		box := c.glyph(r, size)
		out.add(box, out.width+space, 0)
		out.width += space
	}
	return out
}

func (c *compositor) glyph(symbol rune, size float64) mathBox {
	var buffer sfnt.Buffer
	index, err := c.face.GlyphIndex(&buffer, symbol)
	if err != nil || index == 0 {
		panic(fmt.Errorf("selected math font has no glyph for %q", symbol))
	}
	ppem := fixed.Int26_6(math.Round(size * 64))
	bounds, advance, err := c.face.GlyphBounds(&buffer, index, ppem, stdfont.HintingNone)
	if err != nil {
		panic(err)
	}
	box := mathBox{width: float64(advance) / 64, height: max(0, -float64(bounds.Min.Y)/64), depth: max(0, float64(bounds.Max.Y)/64)}
	if !unicode.IsSpace(symbol) {
		box.ops = []drawtex.Op{drawtex.GlyphOp{Glyph: drawtex.Glyph{Font: c.face, Size: size, Num: index, Symbol: string(symbol)}}}
	}
	return box
}

func (c *compositor) command(symbol string, size float64) mathBox {
	font := font.Font{Name: "default", Type: "rm", Size: size}
	metrics := c.backend.Metrics(symbol, font, 72, true)
	c.backend.RenderGlyph(0, 0, font, symbol, 72)
	operation := c.canvas.Ops()[len(c.canvas.Ops())-1].(drawtex.GlyphOp)
	return mathBox{width: metrics.Advance, height: max(0, metrics.Iceberg), depth: max(0, metrics.Height-metrics.Iceberg), ops: []drawtex.Op{operation}}
}

func (c *compositor) macro(macro *ast.Macro, size float64, style string) mathBox {
	name := macro.Name.Name
	arg := func(index int, scale float64, style string) mathBox {
		if index >= len(macro.Args) {
			panic("missing argument for " + name)
		}
		return c.node(macro.Args[index], size*scale, style)
	}
	switch name {
	case `\frac`, `\dfrac`, `\tfrac`, `\binom`:
		scale := .85
		if name == `\dfrac` {
			scale = 1
		}
		numerator, denominator := arg(0, scale, style), arg(1, scale, style)
		width := max(numerator.width, denominator.width) + size*.2
		axis, gap, thickness := -size*.23, size*.15, max(.6, size*.045)
		var out mathBox
		out.add(numerator, (width-numerator.width)/2, axis-gap-numerator.depth)
		out.add(denominator, (width-denominator.width)/2, axis+gap+denominator.height)
		out.width = width
		if name != `\binom` {
			out.ops = append(out.ops, drawtex.RectOp{X1: 0, Y1: axis - thickness/2, X2: width, Y2: axis + thickness/2})
		} else {
			left, right := c.glyph('(', size*1.4), c.glyph(')', size*1.4)
			var surrounded mathBox
			surrounded.add(left, 0, 0)
			surrounded.add(out, left.width, 0)
			surrounded.add(right, surrounded.width, 0)
			return surrounded
		}
		return out
	case `\sqrt`:
		bodyIndex := len(macro.Args) - 1
		body := arg(bodyIndex, 1, style)
		gap := size * .1
		root := c.glyph('√', max(size, (body.height+body.depth+gap)*1.12))
		var out mathBox
		x := float64(0)
		if len(macro.Args) == 2 {
			index := arg(0, .5, style)
			out.add(index, 0, -size*.5)
			x = index.width * .65
		}
		out.add(root, x, body.depth)
		x += root.width - size*.1
		out.add(body, x, 0)
		out.ops = append(out.ops, drawtex.RectOp{X1: x, Y1: -body.height - gap, X2: out.width, Y2: -body.height - gap + max(.6, size*.04)})
		out.height = max(out.height, body.height+gap)
		return out
	case `\overline`:
		body := arg(0, 1, style)
		body.ops = append(body.ops, drawtex.RectOp{X1: 0, Y1: -body.height - size*.12, X2: body.width, Y2: -body.height - size*.08})
		body.height += size * .12
		return body
	case `\mathbf`, `\textbf`:
		return arg(0, 1, "bold")
	case `\mathit`, `\textit`:
		return arg(0, 1, "italic")
	case `\mathregular`, `\mathdefault`, `\textregular`, `\textdefault`, `\operatorname`:
		return arg(0, 1, "regular")
	case `\mathbb`, `\textbb`:
		return arg(0, 1, "double")
	case `\mathcal`, `\mathscr`, `\textcal`, `\textscr`:
		return arg(0, 1, "script")
	case `\mathsf`, `\textsf`:
		return arg(0, 1, "sans")
	case `\mathtt`, `\texttt`:
		return arg(0, 1, "mono")
	case `\,`, `\thinspace`:
		return mathBox{width: size / 6}
	case `\;`:
		return mathBox{width: size * .28}
	case `\:`, `\>`:
		return mathBox{width: size * .22}
	case `\!`:
		return mathBox{width: -size / 6}
	case `\ `:
		return mathBox{width: size / 3}
	case `\quad`:
		return mathBox{width: size}
	case `\qquad`:
		return mathBox{width: 2 * size}
	}
	for _, function := range []string{"sin", "cos", "tan", "log", "ln", "exp", "lim", "max", "min", "det", "gcd", "arcsin", "arccos", "arctan", "sinh", "cosh", "tanh"} {
		if name == `\`+function {
			return c.text(function, size, "regular")
		}
	}
	if len(macro.Args) != 0 {
		panic("unsupported math command " + name)
	}
	return c.command(name, size)
}

func styledRune(r rune, style string) rune {
	upper, lower, digit := rune(0), rune(0), rune(0)
	switch style {
	case "italic":
		upper, lower = 0x1d434, 0x1d44e
		if r == 'h' {
			return 'ℎ'
		}
	case "bold":
		upper, lower, digit = 0x1d400, 0x1d41a, 0x1d7ce
	case "double":
		upper, lower, digit = 0x1d538, 0x1d552, 0x1d7d8
		if replacement := map[rune]rune{'C': 'ℂ', 'H': 'ℍ', 'N': 'ℕ', 'P': 'ℙ', 'Q': 'ℚ', 'R': 'ℝ', 'Z': 'ℤ'}[r]; replacement != 0 {
			return replacement
		}
	case "script":
		upper, lower = 0x1d49c, 0x1d4b6
		if replacement := map[rune]rune{'B': 'ℬ', 'E': 'ℰ', 'F': 'ℱ', 'H': 'ℋ', 'I': 'ℐ', 'L': 'ℒ', 'M': 'ℳ', 'R': 'ℛ', 'e': 'ℯ', 'g': 'ℊ', 'o': 'ℴ'}[r]; replacement != 0 {
			return replacement
		}
	case "sans":
		upper, lower, digit = 0x1d5a0, 0x1d5ba, 0x1d7e2
	case "mono":
		upper, lower, digit = 0x1d670, 0x1d68a, 0x1d7f6
	}
	switch {
	case upper != 0 && r >= 'A' && r <= 'Z':
		return upper + r - 'A'
	case lower != 0 && r >= 'a' && r <= 'z':
		return lower + r - 'a'
	case digit != 0 && r >= '0' && r <= '9':
		return digit + r - '0'
	default:
		return r
	}
}
