package desktop

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestComposerFlipSourceThresholds(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		expanded, newline, resizing bool
		text, capacity              float32
		want                        bool
	}{
		{name: "short compact", text: 150, capacity: 300},
		{name: "capacity exact", text: 300, capacity: 300},
		{name: "overflow expands", text: 301, capacity: 300, want: true},
		{name: "newline expands", newline: true, capacity: 300, want: true},
		{name: "below minimum", capacity: 199, want: true},
		{name: "minimum exact", capacity: 200},
		{name: "hysteresis exact", expanded: true, text: 268, capacity: 300, want: true},
		{name: "inside collapse boundary", expanded: true, text: 267, capacity: 300},
		{name: "resize holds expanded", expanded: true, capacity: 300, resizing: true, want: true},
		{name: "resize still expands", text: 301, capacity: 300, resizing: true, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := composerFlip(tc.expanded, tc.text, tc.capacity, tc.newline, tc.resizing); got != tc.want {
				t.Fatalf("flip = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestComposerResizeSettlesBeforeCollapse(t *testing.T) {
	now := time.Unix(100, 0)
	var state composerLayoutState
	if expanded, _ := state.update(now, 10, 199, false); !expanded {
		t.Fatal("narrow composer did not immediately expand")
	}
	if expanded, wait := state.update(now, 10, 300, false); !expanded || wait != 150*time.Millisecond {
		t.Fatalf("resize did not hold expanded: %v, %v", expanded, wait)
	}
	if expanded, _ := state.update(now.Add(149*time.Millisecond), 10, 300, false); !expanded {
		t.Fatal("composer collapsed before settling")
	}
	if expanded, wait := state.update(now.Add(150*time.Millisecond), 10, 300, false); expanded || wait != 0 {
		t.Fatalf("settled composer did not collapse: %v, %v", expanded, wait)
	}
	if expanded, _ := state.update(now.Add(151*time.Millisecond), 10, 199, false); !expanded {
		t.Fatal("narrowing expansion waited for settling")
	}
}

func TestComposerExpandedHeightMatchesNativeUnicodeWrap(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		width      float32
	}{
		{name: "ascii wrap", text: strings.Repeat("word ", 30), width: 180},
		{name: "unicode wrap", text: strings.Repeat("中文かな🙂 ", 20), width: 180},
		{name: "explicit lines", text: "first\n第二行\n", width: 180},
		{name: "trailing spaces", text: "text" + strings.Repeat(" ", 40), width: 180},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t, "default")
			a.selectSession("chat-0")
			draft := tc.text
			a.draft = draft
			font := ui.Font{Family: "Geist", Size: composerTextSize}
			var surface ui.Rect
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(a.palette.UI())
				ui.Column(c).Children(func() {
					ui.TextAreaBase(c, &draft).Label("Measured draft").Width(tc.width).Lines(1, 100).Padding(0).Font(font.Family).FontSize(font.Size).FixedLineHeight(composerLineHeight)
					ui.Column(c).Width(tc.width + 34).Children(func() {
						a.nativeComposerSurface(c, composerSurfaceProps{
							draft: &a.draft, input: &a.composer, surface: &a.composerSurface,
							controls: a.centralControls(), widthHint: tc.width + 34,
							inputLabel: "Composer draft", attachLabel: "Attach", sendLabel: "Send", stopLabel: "Stop",
							onSend: func(string) bool { return false }, onChooseAttach: func() {},
						})
					})
				})
				surface = a.composerSurface.Bounds(c)
			}, 500, 1500)
			tt.Advance(time.Second)
			tt.Frame()
			input, _ := tt.Find("Measured draft")
			want := min(max(input.H+20, 60), 260) + 42 + 2
			if math.Abs(float64(surface.H-want)) > .01 {
				t.Fatalf("composer height %v differs from native content %v plus source insets/actions = %v", surface.H, input.H, want)
			}
		})
	}
}

func TestComposerSettledNativeGeometry(t *testing.T) {
	for _, tc := range []struct {
		name                string
		hero                bool
		width               float32
		draft               string
		height, inputHeight float32
		inset               float32
	}{
		{name: "hero empty", hero: true, width: 736, height: 120, inputHeight: 76, inset: 12},
		{name: "thread compact", width: 736, height: 49, inputHeight: 47, inset: 8},
		{name: "narrow thread expanded", width: 380, height: 104, inputHeight: 60, inset: 12},
		{name: "hero cap", hero: true, width: 736, draft: strings.Repeat("a line\n", 30), height: 304, inputHeight: 260, inset: 12},
		{name: "thread cap", width: 736, draft: strings.Repeat("a line\n", 30), height: 304, inputHeight: 260, inset: 12},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t, "default")
			a.selectSession("chat-0")
			a.draft = tc.draft
			var surface ui.Rect
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(a.palette.UI())
				ui.Column(c).Width(tc.width).Children(func() {
					a.nativeComposerSurface(c, composerSurfaceProps{
						draft: &a.draft, input: &a.composer, surface: &a.composerSurface,
						controls: a.centralControls(), hero: tc.hero, widthHint: tc.width,
						inputLabel: "Draft", attachLabel: "Attach", sendLabel: "Send", stopLabel: "Stop",
						onSend: func(string) bool { return false }, onChooseAttach: func() {},
					})
				})
				surface = a.composerSurface.Bounds(c)
			}, 900, 700)
			tt.Advance(time.Second)
			tt.Frame()
			input, _ := tt.Find("Draft")
			attach, _ := tt.Find("Attach")
			send, _ := tt.Find("Send")
			near := func(name string, got, want float32) {
				t.Helper()
				if math.Abs(float64(got-want)) > .01 {
					t.Errorf("%s = %v, want %v; surface %v input %v attach %v send %v", name, got, want, surface, input, attach, send)
				}
			}
			near("surface height", surface.H, tc.height)
			near("input height", input.H, tc.inputHeight)
			near("attach left inset", attach.X-surface.X, tc.inset+1)
			near("send right inset", surface.X+surface.W-send.X-send.W, tc.inset+1)
			near("attach width", attach.W, 28)
			near("send width", send.W, 28)
			near("attach and send alignment", attach.Y, send.Y)
		})
	}
}

func TestComposerPixelHeightCapSurvivesInterfaceFontSizes(t *testing.T) {
	for _, size := range []float32{12, 13, 14, 15} {
		for _, hero := range []bool{false, true} {
			route := "dock"
			if hero {
				route = "hero"
			}
			t.Run(strconv.Itoa(int(size))+"/"+route, func(t *testing.T) {
				a := testApp(t, "default")
				a.selectSession("chat-0")
				a.draft = strings.Repeat("a line\n", 100)
				var surface ui.Rect
				tt := ui.NewTester(func(c *ui.Context) {
					theme := a.palette.UI()
					theme.FontSize = 13 * size / 16 // Same mapping as App.View.
					c.SetTheme(theme)
					ui.Column(c).Width(736).Children(func() {
						a.nativeComposerSurface(c, composerSurfaceProps{
							draft: &a.draft, input: &a.composer, surface: &a.composerSurface,
							controls: a.centralControls(), hero: hero, widthHint: 736,
							inputLabel: "Draft", attachLabel: "Attach", sendLabel: "Send", stopLabel: "Stop",
							onSend: func(string) bool { return false }, onChooseAttach: func() {},
						})
					})
					surface = a.composerSurface.Bounds(c)
				}, 900, 700)
				tt.Advance(time.Second)
				tt.Frame()
				input, _ := tt.Find("Draft")
				if input.H != 260 || surface.H != 304 {
					t.Fatalf("interface font %v hit a line cap before the source pixel cap: textarea %v, surface %v", size, input.H, surface.H)
				}
			})
		}
	}
}

func TestComposerLineHeightStaysAbsoluteAcrossInterfaceFontSizes(t *testing.T) {
	for _, size := range []float32{13, 16, 20} {
		for _, hero := range []bool{false, true} {
			route := "dock"
			if hero {
				route = "hero"
			}
			t.Run(strconv.Itoa(int(size))+"/"+route, func(t *testing.T) {
				a := testApp(t, "default")
				a.selectSession("chat-0")
				a.draft = "first line\nsecond line\nthird line\nfourth line"
				tt := ui.NewTester(func(c *ui.Context) {
					theme := a.palette.UI()
					theme.FontSize = 13 * size / 16
					c.SetTheme(theme)
					ui.Column(c).Width(736).Children(func() {
						a.nativeComposerSurface(c, composerSurfaceProps{
							draft: &a.draft, input: &a.composer, surface: &a.composerSurface,
							controls: a.centralControls(), hero: hero, widthHint: 736,
							inputLabel: "Draft", attachLabel: "Attach", sendLabel: "Send", stopLabel: "Stop",
							onSend: func(string) bool { return false }, onChooseAttach: func() {},
						})
					})
				}, 900, 700)
				tt.Advance(time.Second)
				// ComposerInput::layout_text uses px(configured_line_height)
				// even though its surrounding Div declares ui_rems leading.
				// Four explicit lines plus the native input's16+4px padding.
				if input, ok := tt.Find("Draft"); !ok || input.H != 4*composerLineHeight+20 {
					t.Fatalf("font size%g scaled absolute source leading: %+v, %v", size, input, ok)
				}
			})
		}
	}
}

func TestComposerNativeCapacityUsesCompactViewport(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	font := ui.Font{Family: "Geist", Size: composerTextSize}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.palette.UI())
		ui.Column(c).Width(736).Children(func() {
			a.nativeComposerSurface(c, composerSurfaceProps{
				draft: &a.draft, input: &a.composer, surface: &a.composerSurface,
				controls: a.centralControls(), widthHint: 736,
				inputLabel: "Draft", attachLabel: "Attach", sendLabel: "Send", stopLabel: "Stop",
				onSend: func(string) bool { return false }, onChooseAttach: func() {},
			})
		})
	}, 900, 700)
	tt.Advance(time.Second)
	input, _ := tt.Find("Draft")
	// Measure the native compact editor, remove its actual 8px horizontal
	// insets, then the reference's additional 8px capacity allowance.
	capacity := input.W - 16 - 8
	below := func(limit float32) string {
		text := ""
		for {
			next := text + "W"
			w, _ := ui.ShapeText(next, font, 0).Size()
			if w > limit {
				return text
			}
			text = next
		}
	}
	check := func(text string, expanded bool) {
		t.Helper()
		a.draft = text
		tt.Frame()
		tt.Advance(time.Second)
		got, _ := tt.Find("Draft")
		if (got.H > 47) != expanded {
			w, _ := ui.ShapeText(text, font, 0).Size()
			t.Fatalf("native viewport capacity %v, text width %v produced input height %v; expanded %v", capacity, w, got.H, expanded)
		}
	}
	fit := below(capacity)
	check(fit, false)
	check(fit+"W", true)
	check(below(capacity-16), true)
	check(below(capacity-40), false)
}

func TestComposerModesKeepNativeIMEUndoAndFocus(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	hero, width, sends := false, float32(736), 0
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.palette.UI())
		ui.Column(c).Width(width).Children(func() {
			a.nativeComposerSurface(c, composerSurfaceProps{
				draft: &a.draft, input: &a.composer, surface: &a.composerSurface,
				controls: a.centralControls(), hero: hero, widthHint: width,
				inputLabel: "Draft", attachLabel: "Attach", sendLabel: "Send", stopLabel: "Stop",
				onSend: func(string) bool { sends++; return false }, onChooseAttach: func() {},
			})
		})
	}, 900, 700)
	if err := tt.Click("Draft"); err != nil {
		t.Fatal(err)
	}
	tt.Type("before")
	tt.Key(ui.Shift, ui.KeyEnter)
	tt.Advance(time.Second)
	beforeCommit := a.draft
	tt.Compose("你好", 2)
	hero, width = true, 380
	tt.Frame()
	tt.Advance(200 * time.Millisecond)
	tt.Key(0, ui.KeyEnter)
	if sends != 0 || a.draft != beforeCommit || !tt.Focused("Draft") {
		t.Fatalf("mode change lost native composition/focus: sends %d, draft %q, focus %v", sends, a.draft, tt.Focused("Draft"))
	}
	tt.Compose("", 0)
	tt.Type("你好")
	if a.draft != beforeCommit+"你好" {
		t.Fatalf("composition commit lost after geometry change: %q", a.draft)
	}
	tt.Command("undo")
	if a.draft != beforeCommit {
		t.Fatalf("mode change lost native undo: %q, want %q", a.draft, beforeCommit)
	}
	tt.Command("selectAll")
	tt.Command("cut")
	hero, width = false, 736
	tt.Frame()
	tt.Advance(time.Second)
	if input, _ := tt.Find("Draft"); input.H != 47 || !tt.Focused("Draft") {
		t.Fatalf("collapse did not retain the editor: %v, focused %v", input, tt.Focused("Draft"))
	}
	tt.Command("undo")
	if a.draft != beforeCommit || !tt.Focused("Draft") {
		t.Fatalf("compact undo did not restore the expanded draft: %q", a.draft)
	}
}

func TestComposerPreeditControlsModeWithoutCommitting(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.draft = "before"
	sends := 0
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.palette.UI())
		ui.Column(c).Width(736).Children(func() {
			a.nativeComposerSurface(c, composerSurfaceProps{
				draft: &a.draft, input: &a.composer, surface: &a.composerSurface,
				controls: a.centralControls(), widthHint: 736,
				inputLabel: "Draft", attachLabel: "Attach", sendLabel: "Send", stopLabel: "Stop",
				onSend: func(string) bool { sends++; return false }, onChooseAttach: func() {},
			})
		})
	}, 900, 700)
	tt.Advance(time.Second)
	if err := tt.Click("Draft"); err != nil {
		t.Fatal(err)
	}
	preedit := strings.Repeat("中文かな🙂", 20)
	tt.Compose(preedit, 100)
	tt.Advance(200 * time.Millisecond)
	input, _ := tt.Find("Draft")
	if input.H <= 47 || a.draft != "before" || !tt.Focused("Draft") {
		t.Fatalf("long preedit failed to expand without committing: input %v draft %q", input, a.draft)
	}
	tt.Key(0, ui.KeyEnter)
	if sends != 0 || a.draft != "before" {
		t.Fatalf("preedit Enter submitted or changed the draft: sends %d draft %q", sends, a.draft)
	}
	tt.Compose("", 0)
	tt.Advance(200 * time.Millisecond)
	if input, _ := tt.Find("Draft"); input.H != 47 || a.draft != "before" {
		t.Fatalf("cancelled preedit did not collapse: input %v draft %q", input, a.draft)
	}
	tt.Compose("\n中", 2)
	tt.Advance(200 * time.Millisecond)
	if input, _ := tt.Find("Draft"); input.H <= 47 || a.draft != "before" {
		t.Fatalf("preedit newline did not expand: input %v draft %q", input, a.draft)
	}
	tt.Type("\n中")
	if a.draft != "before\n中" {
		t.Fatalf("native preedit commit lost its insertion: %q", a.draft)
	}
	tt.Command("undo")
	tt.Advance(200 * time.Millisecond)
	if input, _ := tt.Find("Draft"); input.H != 47 || a.draft != "before" || !tt.Focused("Draft") {
		t.Fatalf("undo failed to restore compact native editor: input %v draft %q", input, a.draft)
	}
}

func TestComposerOuterWidthMatchesSourceRoutes(t *testing.T) {
	for _, tc := range []struct {
		name              string
		panel, transcript float32
		hero              bool
		want              float32
	}{
		{name: "hero capped", panel: 1200, transcript: 900, hero: true, want: 768},
		{name: "hero narrow", panel: 500, transcript: 900, hero: true, want: 500},
		{name: "thread transcript plus gutters", panel: 1200, transcript: 900, want: 932},
		{name: "thread full panel", panel: 500, transcript: 900, want: 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := composerOuterWidth(tc.panel, tc.transcript, tc.hero); got != tc.want {
				t.Fatalf("outer width %v, want %v", got, tc.want)
			}
		})
	}
}
