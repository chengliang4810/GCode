package workbench

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"testing"
	"time"

	"gcode/internal/assets"
	"gcode/internal/design"

	"github.com/egoist/mygo/ui"
)

func leafCeilControlFeatures() string {
	if runtime.GOOS == "windows" {
		return "kern=0"
	}
	return ""
}

func leafCeilControlSpan(c *ui.Context, text string, size float32) ui.Span {
	return ui.Span{Text: text, Font: tabTooltipFontFamily(), Size: size, Weight: 400,
		Features: leafCeilControlFeatures(), Lang: c.Locale()}
}

func leafCeilControlText(c *ui.Context, kind string, size, maxWidth float32) string {
	switch kind {
	case "latin":
		return "Tab02.txt"
	case "unicode":
		return "文件说明-αβ.txt"
	case "combining":
		return "Cafe\u0301 e\u0301.txt"
	case "emoji":
		return "📁 👨‍👩‍👧‍👦.txt"
	case "rtl-text":
		return "مرحبا بالعالم"
	case "near-fit", "near-overflow":
		low, high := 0, 512
		for low+1 < high {
			mid := (low + high) / 2
			w, _ := c.MeasureText(0, leafCeilControlSpan(c, strings.Repeat("i", mid), size))
			if w <= maxWidth {
				low = mid
			} else {
				high = mid
			}
		}
		if kind == "near-overflow" {
			low++
		}
		return strings.Repeat("i", low)
	case "newline":
		return "Tab02.txt\nnext.txt"
	case "wrapped":
		return "workspace/long-component-path/long-component-path/long-component-path/Tab02.txt"
	case "rtl-context":
		return "12345.67"
	}
	panic("unknown text case")
}

func TestTabTooltipCandidateMeasuresActualStyledLeaf(t *testing.T) {
	if err := assets.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	for _, size := range []float32{10.5, 11} {
		for _, scale := range []float32{1, 1.25, 2} {
			for _, kind := range []string{"latin", "unicode", "combining", "emoji", "rtl-text", "near-fit", "near-overflow", "newline", "wrapped", "rtl-context"} {
				t.Run(fmt.Sprintf("size%g-scale%g-%s", size, scale, kind), func(t *testing.T) {
					measured := float32(0)
					tt := ui.NewTester(func(c *ui.Context) {
						c.SetTheme(design.Palette("", false).UI())
						c.Root().Font("Geist").FontSize(19).FontWeight(700)
						design.ApplyReferenceTypography(c)
						text := leafCeilControlText(c, kind, size, 360)
						measured, _ = c.MeasureText(0, leafCeilControlSpan(c, text, size))
						ui.Text(c, text).Label("native leaf control").Absolute().Left(5).Top(5).NoWrap().
							Font(tabTooltipFontFamily()).FontSize(size).FontWeight(400).
							FontFeatures(leafCeilControlFeatures()).Lang(c.Locale()).
							FixedLineHeight(tabTooltipLineHeight(size, c.DeviceScale()))
					}, 800, 120)
					tt.SetScale(scale)
					if kind == "rtl-context" {
						tt.SetDirection(ui.RightToLeft)
					}
					leaf, ok := tt.Find("native leaf control")
					if !ok || math.Abs(float64(leaf.W-measured)) > .0001 {
						t.Fatalf("explicit Span and actual native leaf differ: leaf%+v measured%g exists%t", leaf, measured, ok)
					}
				})
			}
		}
	}
}

func TestTabTooltipShortLeafUsesSourceLogicalCeil(t *testing.T) {
	if err := assets.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	for _, close := range []bool{false, true} {
		for _, scale := range []float32{1, 1.25, 2} {
			for _, kind := range []string{"latin", "unicode", "combining", "emoji", "rtl-text", "near-fit"} {
				t.Run(fmt.Sprintf("close%t-scale%g-%s", close, scale, kind), func(t *testing.T) {
					s, _, _ := testWorkspace(t)
					size, maxWidth := float32(10.5), float32(380)
					if close {
						size, maxWidth = 11, 320
					}
					owner := &tabTooltipOwner{phase: tabTooltipVisible, x: 20, y: 20, close: close}
					s.tabTooltip.owners = []*tabTooltipOwner{owner}
					tt := newTabTooltipTestWindow(s, func(c *ui.Context) {
						c.SetTheme(s.palette().UI())
						c.Root().Font("Geist").FontSize(19).FontWeight(700)
						design.ApplyReferenceTypography(c)
						owner.text = leafCeilControlText(c, kind, size, maxWidth-20)
						ui.Text(c, owner.text).Label("independent natural leaf").Absolute().Left(5).Top(100).NoWrap().
							Font(tabTooltipFontFamily()).FontSize(size).FontWeight(400).
							FontFeatures(leafCeilControlFeatures()).Lang(c.Locale()).
							FixedLineHeight(tabTooltipLineHeight(size, c.DeviceScale()))
						s.renderTabTooltip(c, tabTooltipPass{visible: owner})
					}, 800, 220)
					tt.SetScale(scale)
					card, ok := tt.card(close)
					if !ok {
						t.Fatal("production card missing")
					}
					leaf, ok := tt.Find("independent natural leaf")
					if !ok || leaf.W > maxWidth-20 {
						t.Fatalf("test did not create an actually fitting leaf: %+v max%g", leaf, maxWidth-20)
					}
					want := float32(math.Ceil(float64(leaf.W))) + 20
					if math.Abs(float64(card.W-want)) > .0001 || math.Abs(float64(card.H-leaf.H-14)) > .0001 {
						t.Fatalf("source logical leaf ceil missing: actual card%+v native leaf%+v want cardW%g", card, leaf, want)
					}
					actualLeaf, ok := tt.Find(owner.text)
					if !ok || math.Abs(float64(actualLeaf.W-(want-20))) > .0001 {
						t.Fatalf("actual tooltip child did not receive logical leaf width: child%+v want%g exists%t", actualLeaf, want-20, ok)
					}
				})
			}
		}
	}
}

func TestTabTooltipLeafCeilLeavesConstrainedAndUnsupportedPathsUnchanged(t *testing.T) {
	for _, close := range []bool{false, true} {
		for _, scale := range []float32{1, 1.25, 2} {
			for _, kind := range []string{"near-overflow", "wrapped", "newline", "rtl-context"} {
				t.Run(fmt.Sprintf("close%t-scale%g-%s", close, scale, kind), func(t *testing.T) {
					s, _, _ := testWorkspace(t)
					size, maxWidth := float32(10.5), float32(380)
					if close {
						size, maxWidth = 11, 320
					}
					owner := &tabTooltipOwner{phase: tabTooltipVisible, x: 20, y: 20, close: close}
					s.tabTooltip.owners = []*tabTooltipOwner{owner}
					tt := newTabTooltipTestWindow(s, func(c *ui.Context) {
						c.SetTheme(s.palette().UI())
						c.Root().Font("Geist").FontSize(19).FontWeight(700)
						design.ApplyReferenceTypography(c)
						owner.text = leafCeilControlText(c, kind, size, maxWidth-20)
						ui.Box(c).Label("C3 native card control").Absolute().Left(20).Top(100).
							MaxWidth(maxWidth).Padding(6, 9).Radius(6).Border(1, s.palette().Border).
							Font(tabTooltipFontFamily()).FontSize(size).FontWeight(400).
							FixedLineHeight(tabTooltipLineHeight(size, c.DeviceScale())).
							Children(func() { ui.Text(c, owner.text) })
						s.renderTabTooltip(c, tabTooltipPass{visible: owner})
					}, 800, 240)
					tt.SetScale(scale)
					if kind == "rtl-context" {
						tt.SetDirection(ui.RightToLeft)
					}
					card, ok := tt.card(close)
					if !ok {
						t.Fatal("production card missing")
					}
					control, ok := tt.Find("C3 native card control")
					if !ok || math.Abs(float64(card.W-control.W)) > .0001 || math.Abs(float64(card.H-control.H)) > .0001 {
						t.Fatalf("unsupported/wrapped native path changed: card%+v control%+v exists%t", card, control, ok)
					}
				})
			}
		}
	}
}

func TestTabTooltipLeafCeilKeepsSlotTaskDeadlines(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	file := tabTooltipFile(t, s, snapshot)
	tt := tabTooltipStripTester(s, 360, false)
	close, ok := tt.Find("Close tab " + file.Name)
	if !ok {
		t.Fatal("close target missing")
	}
	tt.Move(close.X+close.W/2, close.Y+close.H/2)
	tt.Advance(349 * time.Millisecond)
	requireTabTooltip(t, tt, false)
	tt.Advance(time.Millisecond)
	requireTabTooltip(t, tt, true)
	parentDeadline := s.tabTooltip.owners[0].deadline
	s.documents[file.ID].file.Path += ".renamed"
	tt.Frame()
	if !tt.HasText(file.Path) || s.tabTooltip.owners[0].deadline != parentDeadline {
		t.Fatal("ceil build refreshed a captured slot task")
	}
	tt.Advance(149 * time.Millisecond)
	if _, ok := tt.card(true); ok {
		t.Fatal("close tooltip appeared before500ms")
	}
	tt.Advance(time.Millisecond)
	if _, ok := tt.card(true); !ok || s.tabTooltip.owners[0].phase != tabTooltipVisible {
		t.Fatal("independent close500ms task or parent350ms lifetime changed")
	}
}
