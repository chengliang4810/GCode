package desktop

import (
	"encoding/json"
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestTitlebarSettingsClearsNativeButtonsDuringSidebarMotion(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprintf("dark-%t", dark), func(t *testing.T) {
			a := testApp(t, "default")
			a.route, a.section, a.sidebar = "settings", "General", true
			a.state.Settings.Appearance.MotionPreference = "full"
			bar := ui.TitleBar{Height: 52, Left: 80}
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(design.Palette("", dark).UI())
				width, _ := c.Size()
				a.fit = fitPanels(width, a.sidebarWidth, [3]bool{a.sidebar}, a.panelOpened, false)
				a.layoutPanels(c, width, [3]bool{a.sidebar}, false)
				ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
					ui.Box(c).Width(a.frameColumns.Sidebar).Shrink(0)
					ui.Column(c).Grow(1).FillHeight().Children(func() { a.titlebar(c) })
				})
			}, 900, 600)
			tt.SetTitleBar(bar)
			for _, open := range []bool{false, true, false} {
				a.setPanel(0, open)
				tt.Frame()
				for frame := 0; frame < 20; frame++ {
					tt.Advance(panelTweenFrame)
					title, ok := tt.Find("General")
					wantX := max(a.frameColumns.Sidebar, titlebarNavigationLeft(bar)+design.Controls().IconButtonSize*2+5) + 10
					if !ok || !titlebarNear(title.X, wantX) || title.Y+title.H > bar.Height {
						t.Fatalf("open=%t frame=%d sidebar=%g title=%+v: want x=%g clear of native buttons",
							open, frame, a.frameColumns.Sidebar, title, wantX)
					}
				}
			}
		})
	}
}

func TestTitlebarPrimaryToggleAndSecondaryMenu(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession(a.state.Sessions[0].ID)
	a.right, a.files, a.rightExpanded = false, false, false
	tt := ui.NewTester(a.View, 1320, 880)
	tt.SetTitleBar(ui.TitleBar{Right: 108})
	if err := tt.Click("Toggle right pane"); err != nil {
		t.Fatal(err)
	}
	if !a.right || a.rightExpanded || len(tt.Menu()) != 0 {
		t.Fatalf("primary toggle: right=%v expanded=%v menu=%v", a.right, a.rightExpanded, tt.Menu())
	}
	if err := tt.RightClick("Toggle right pane"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tt.Menu(), []string{"Expand pane", "Terminal panel"}) {
		t.Fatalf("secondary menu = %v", tt.Menu())
	}
	if err := tt.ChooseMenuItem("Expand pane"); err != nil {
		t.Fatal(err)
	}
	if !a.right || !a.rightExpanded {
		t.Fatal("secondary Expand did not enter takeover")
	}
	if err := tt.Click("Toggle right pane"); err != nil {
		t.Fatal(err)
	}
	if a.right || a.rightExpanded || len(tt.Menu()) != 0 {
		t.Fatalf("primary close after takeover: right=%v expanded=%v menu=%v", a.right, a.rightExpanded, tt.Menu())
	}
	if err := tt.RightClick("Toggle right pane"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Terminal panel"); err != nil {
		t.Fatal(err)
	}
	if !a.terminal {
		t.Fatal("secondary Terminal action was lost")
	}
}

func TestTitlebarSlotsKeepVisibleControlsReachable(t *testing.T) {
	for _, width := range []int{900, 1320, 1600} {
		for _, sidebar := range []bool{false, true} {
			for _, files := range []bool{false, true} {
				for _, takeover := range []bool{false, true} {
					for _, captions := range []float32{0, 108} {
						name := fmt.Sprintf("%d-sidebar%t-files%t-takeover%t-caption%g", width, sidebar, files, takeover, captions)
						t.Run(name, func(t *testing.T) {
							a := testApp(t, "default")
							a.selectSession(a.state.Sessions[0].ID)
							// The source adds '+' only after an existing chip.
							a.workspace.OpenSurface("changes")
							a.sidebar, a.right, a.files, a.rightExpanded = sidebar, true, files, takeover
							a.panelOpened = [3]uint64{1, 2, 3}
							tt := ui.NewTester(a.View, width, 880)
							tt.SetTitleBar(ui.TitleBar{Right: captions})
							geometry := titlebarGeometry(t, tt, float32(width)-captions-6)
							tools := titlebarRect(t, tt, "Tools workspace")
							tabs := titlebarRect(t, tt, "Workspace tabs")
							if !takeover {
								info := titlebarRect(t, tt, "Session information")
								// MyGo Split owns the 1 DIP divider before its
								// second pane; the header starts at that seam.
								if info.X+info.W > tabs.X || !titlebarNear(tabs.X+1, tools.X) {
									t.Fatalf("info/tabs left their columns: info=%+v tabs=%+v tools=%+v", info, tabs, tools)
								}
							} else if _, ok := tt.Find("Session information"); ok {
								t.Fatal("takeover built an obscured session-info control")
							}
							if a.fit.Files {
								explorer := titlebarRect(t, tt, "Files explorer")
								if !titlebarNear(tabs.X+tabs.W+1, explorer.X) {
									t.Fatalf("tabs crossed Files seam: tabs=%+v explorer=%+v", tabs, explorer)
								}
							}
							if !a.fit.Sidebar {
								back := titlebarRect(t, tt, "Back")
								if tabs.X < back.X+back.W {
									t.Fatalf("tabs covered navigation: tabs=%+v back=%+v", tabs, back)
								}
							}
							// Use the actual center of New surface: an overlapping
							// header control would consume this pointer click.
							if err := tt.Click("New surface"); err != nil {
								t.Fatal(err)
							}
							if !reflect.DeepEqual(tt.Menu(), []string{"Side chat", "Files", "Browser", "Terminal", "Changes", "History"}) {
								t.Fatalf("New surface center was not reachable: %v", tt.Menu())
							}
							tt.CloseMenu()
							captureTitlebarGeometry(t, name, geometry)
						})
					}
				}
			}
		}
	}
}

func TestTitlebarTakeoverRemovesAndRestoresSessionChrome(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession(a.state.Sessions[0].ID)
	a.sidebar, a.right, a.files = false, true, true
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == a.sessionID {
			a.state.Sessions[i].Title = "Unique header title for takeover"
		}
	}
	tt := ui.NewTester(a.View, 1320, 880)
	tt.SetTitleBar(ui.TitleBar{Left: 72, Right: 108})
	if !tt.HasText("Unique header title for takeover") {
		t.Fatal("fixture has no header title")
	}
	if err := tt.Click("Session information"); err != nil {
		t.Fatal(err)
	}
	if !a.expanded["session-info-user"] {
		t.Fatal("session-info center was not reachable in split mode")
	}
	a.rightExpanded = true
	tt.Frame()
	if tt.HasText("Unique header title for takeover") {
		t.Fatal("takeover still built the session title")
	}
	if _, ok := tt.Find("Session information"); ok {
		t.Fatal("takeover still built session-info button")
	}
	titlebarGeometry(t, tt, 1320-108-6)
	if err := tt.Click("Expand workspace"); err != nil {
		t.Fatal(err)
	}
	if a.rightExpanded || !a.right || !tt.HasText("Unique header title for takeover") {
		t.Fatal("collapse did not restore the split header")
	}
	titlebarRect(t, tt, "Session information")
}

// The window hides its native caption buttons on Windows, so the title bar
// draws them from the shared icon buttons: same 28px target, same 4px rhythm
// and the same center line as the Files and pane toggles beside them.
func TestTitlebarCaptionControlsMatchAdjacentIconSlots(t *testing.T) {
	if !HideNativeWindowButtons {
		t.Skip("the title bar draws caption controls only where the window hides its native ones")
	}
	a := testApp(t, "default")
	a.selectSession(a.state.Sessions[0].ID)
	a.sidebar, a.right, a.files, a.rightExpanded = true, true, true, false
	a.captionButtons = true
	tt := ui.NewTester(a.View, 1320, 880)
	tt.SetTitleBar(ui.TitleBar{})

	const width, target, topPad, height = float32(1320), float32(28), float32(4), float32(38)
	center := (height + topPad) / 2
	files := titlebarRect(t, tt, "Files")
	pane := titlebarRect(t, tt, "Toggle right pane")
	minimize := titlebarRect(t, tt, "Minimize window")
	maximize := titlebarRect(t, tt, "Maximize window")
	closeButton := titlebarRect(t, tt, "Close window")
	for label, r := range map[string]ui.Rect{
		"Minimize window": minimize,
		"Maximize window": maximize,
		"Close window":    closeButton,
	} {
		if r.W != target || r.H != target {
			t.Fatalf("%q = %+v, want the shared %gpx caption target", label, r, target)
		}
		if !titlebarNear(r.Y+r.H/2, center) {
			t.Fatalf("%q = %+v, want the %gpx trailing-control center line", label, r, center)
		}
		if r.Y != files.Y || r.H != files.H {
			t.Fatalf("%q = %+v, adjacent Files control = %+v", label, r, files)
		}
	}
	if !titlebarNear(maximize.X-(minimize.X+minimize.W), titlebarControlGap) ||
		!titlebarNear(closeButton.X-(maximize.X+maximize.W), titlebarControlGap) ||
		!titlebarNear(minimize.X-(pane.X+pane.W), titlebarControlGap) {
		t.Fatalf("caption rhythm: minimize=%+v maximize=%+v close=%+v pane=%+v", minimize, maximize, closeButton, pane)
	}
	if !titlebarNear(closeButton.X+closeButton.W, width-titlebarEdgeInset) {
		t.Fatalf("close = %+v, want the %gpx edge inset", closeButton, titlebarEdgeInset)
	}
	if tabs, ok := tt.Find("Workspace tabs"); ok && tabs.X+tabs.W > minimize.X {
		t.Fatalf("tabs crossed the caption cluster: tabs=%+v minimize=%+v", tabs, minimize)
	}
	captureTitlebarImage(t, "caption-controls", tt)

	// The shared close variant paints the system red under the pointer; the
	// other caption buttons keep the titlebar's neutral hover.
	sample := func(x, y float32) (uint8, uint8, uint8) {
		r, g, b, _ := tt.Image().At(int(x), int(y)).RGBA()
		return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
	}
	tt.Move(closeButton.X+closeButton.W/2, closeButton.Y+closeButton.H/2)
	tt.Frame()
	if r, g, b := sample(closeButton.X+3, closeButton.Y+closeButton.H/2); !titlebarNear(float32(r), 0xE8) || !titlebarNear(float32(g), 0x11) || !titlebarNear(float32(b), 0x23) {
		t.Fatalf("close hover = #%02X%02X%02X, want the system close red", r, g, b)
	}
	tt.Move(minimize.X+minimize.W/2, minimize.Y+minimize.H/2)
	tt.Frame()
	if r, g, b := sample(minimize.X+3, minimize.Y+minimize.H/2); r > 0xE0 && g < 0x40 && b < 0x60 {
		t.Fatalf("minimize hover = #%02X%02X%02X, want the neutral titlebar hover", r, g, b)
	}

	a.navigate("settings/Appearance")
	tt.Frame()
	for _, label := range []string{"Minimize window", "Maximize window", "Close window"} {
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("Settings lost the window caption control %q", label)
		}
	}

	// The glyphs follow the theme's text color in both appearances.
	for _, mode := range []string{"light", "dark"} {
		a.state.Settings.Appearance.Mode = mode
		a.navigate("chat")
		tt.Frame()
		for _, label := range []string{"Minimize window", "Maximize window", "Close window"} {
			titlebarRect(t, tt, label)
		}
	}
}

// A full-screen window shows no caption controls, and a platform that keeps
// its own controls leaves the drawing to the platform.
func TestTitlebarCaptionControlsFollowWindowState(t *testing.T) {
	for _, test := range []struct {
		hidesNative, fullscreen bool
		nativeRight             float32
		want                    bool
	}{
		{true, false, 0, true},
		{true, true, 0, false},
		{false, false, 0, false},
		{false, true, 0, false},
		{true, false, 108, false},
	} {
		if got := captionButtonsDrawn(test.hidesNative, test.fullscreen, test.nativeRight); got != test.want {
			t.Fatalf("drawn(hidesNative=%v fullscreen=%v nativeRight=%g) = %v, want %v", test.hidesNative, test.fullscreen, test.nativeRight, got, test.want)
		}
	}
	for _, test := range []struct {
		maximized bool
		want      string
	}{
		{false, "caption-maximize"},
		{true, "caption-restore"},
	} {
		if got := captionMaximizeGlyph(test.maximized); got != test.want {
			t.Fatalf("maximized=%v glyph = %q, want %q", test.maximized, got, test.want)
		}
	}
	// The cluster reserves exactly what it draws: three targets, two gaps and
	// the separation from the adjacent Files toggle.
	controls := design.Controls()
	targets := controls.IconButtonSize*3 + titlebarControlGap*2
	if titlebarCaptionWidth() != targets {
		t.Fatalf("caption width = %g, want %g", titlebarCaptionWidth(), targets)
	}
	if titlebarCaptionWidth()+titlebarControlGap+titlebarEdgeInset <= targets {
		t.Fatal("caption clearance must cover the cluster plus its edge insets")
	}
}

func TestTitlebarLongTitleAndLargeFontStayWithinSlots(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession(a.state.Sessions[0].ID)
	a.sidebar, a.right, a.files = false, true, true
	a.state.Settings.Appearance.FontSize = 20
	for i := range a.state.Sessions {
		if a.state.Sessions[i].ID == a.sessionID {
			a.state.Sessions[i].Title = "A very long session title that must yield to the session information and workspace controls"
		}
	}
	tt := ui.NewTester(a.View, 900, 600)
	tt.SetTitleBar(ui.TitleBar{Left: 72, Right: 108})
	titlebarGeometry(t, tt, 900-108-6)
	if err := tt.Click("Session information"); err != nil {
		t.Fatal(err)
	}
	if !a.expanded["session-info-user"] {
		t.Fatal("large title consumed session-info click")
	}
}

func TestTitlebarSettingsAfterTakeoverKeepsItsOwnChrome(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession(a.state.Sessions[0].ID)
	a.sidebar, a.right, a.files, a.rightExpanded = false, true, true, true
	tt := ui.NewTester(a.View, 1320, 880)
	tt.SetTitleBar(ui.TitleBar{Left: 72, Right: 108})
	a.navigate("settings/Appearance")
	tt.Frame()
	if !a.rightExpanded || a.fit.Right {
		t.Fatal("fixture did not retain the chat's takeover state outside the Settings route")
	}
	heading := titlebarRect(t, tt, "Appearance")
	if heading.Y >= titlebarHeight || heading.X < 72 || heading.X+heading.W > 1320-108-6 {
		t.Fatalf("Settings heading left its header: %+v", heading)
	}
	for _, label := range []string{"Session information", "Workspace tabs", "Toggle right pane", "New surface"} {
		if _, ok := tt.Find(label); ok {
			t.Fatalf("Settings retained chat header control %q", label)
		}
	}
	if err := tt.Click("Toggle sidebar"); err != nil {
		t.Fatal(err)
	}
	if !a.sidebar || !a.fit.Sidebar {
		t.Fatal("Settings sidebar button was not reachable")
	}
	tt.Advance(250 * time.Millisecond)
	titlebarRect(t, tt, "General")
}

func TestTitlebarCanvasKeepsDraftVisibleWithoutSessionPanels(t *testing.T) {
	for _, sessionID := range []string{"", "removed-session"} {
		for _, sidebar := range []bool{false, true} {
			t.Run(fmt.Sprintf("session%q-sidebar%t", sessionID, sidebar), func(t *testing.T) {
				a := testApp(t, "default")
				// Exercise the render guard independently of public navigation,
				// which now normalizes unknown IDs before accepting a selection.
				a.sessionID = sessionID
				a.sidebar = sidebar
				a.right, a.files, a.rightExpanded = true, true, true
				a.draft = "Canvas draft"
				tt := ui.NewTester(a.View, 1320, 880)
				tt.SetTitleBar(ui.TitleBar{Right: 108})
				for _, label := range []string{"Session information", "Workspace tabs", "Expand workspace", "New surface", "Files", "Toggle right pane", "Tools workspace", "Files explorer"} {
					if _, ok := tt.Find(label); ok {
						t.Fatalf("canvas retained session panel %q", label)
					}
				}
				if a.fit.Right || a.fit.Files || !a.right || !a.files || !a.rightExpanded {
					t.Fatalf("effective visibility changed panel intent: fit=%+v right=%v files=%v expanded=%v", a.fit, a.right, a.files, a.rightExpanded)
				}
				titlebarRect(t, tt, "Message")
				if err := tt.Click("Message"); err != nil {
					t.Fatal(err)
				}
				tt.Type(" remains editable")
				if a.draft != "Canvas draft remains editable" {
					t.Fatalf("canvas lost native editing: %q", a.draft)
				}
				if sessionID != "" {
					restored := a.state.Sessions[0]
					restored.ID, restored.Title = sessionID, "Restored session"
					a.state.Sessions = append(a.state.Sessions, restored)
					tt.Frame()
					if !a.fit.Right || !a.rightExpanded || a.draft != "Canvas draft remains editable" {
						t.Fatal("restoring the selected session lost panel intent or draft")
					}
					titlebarRect(t, tt, "Toggle right pane")
					titlebarRect(t, tt, "Workspace tabs")
				}
			})
		}
	}
}

func TestNewConversationResetsCanvasTerminalAndAllowsReopening(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 880)
	tt.Key(ui.Cmd, ui.KeyJ)
	if !a.terminal {
		t.Fatal("canvas terminal shortcut did not open the terminal")
	}
	titlebarRect(t, tt, "Bottom terminal")
	tt.Key(ui.Cmd, ui.KeyN)
	if a.terminal || a.sessionID != "" {
		t.Fatal("fresh new conversation retained the canvas terminal")
	}
	if _, ok := tt.Find("Bottom terminal"); ok {
		t.Fatal("fresh canvas retained terminal geometry")
	}
	titlebarRect(t, tt, "Message")
	tt.Key(ui.Cmd, ui.KeyJ)
	if !a.terminal {
		t.Fatal("fresh canvas lost its terminal shortcut")
	}
	titlebarRect(t, tt, "Bottom terminal")
}

func TestCanvasRoundTripRestoresWorkspaceAndUnsavedEditor(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.draft = "Saved conversation draft"
	a.workspace.OpenFile("file-layout")
	a.files, a.rightExpanded = true, true
	tt := ui.NewTester(a.View, 1600, 1000)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("R10 unsaved workspace buffer")
	if !tt.HasText("Unsaved") {
		t.Fatal("fixture did not edit the native workspace buffer")
	}
	titlebarRect(t, tt, "Close tab layout.go")
	a.newConversation()
	tt.Frame()
	if a.fit.Right || a.fit.Files {
		t.Fatal("canvas retained session panel geometry")
	}
	if _, ok := tt.Find("Workspace tabs"); ok {
		t.Fatal("canvas retained the session tab strip")
	}
	if err := tt.Click("Message"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Separate canvas draft")
	a.selectSession("chat-0")
	tt.Frame()
	if !a.right || !a.files || !a.rightExpanded || !a.fit.Right || !a.fit.Files || a.draft != "Saved conversation draft" {
		t.Fatalf("session panel/draft intent was lost: fit=%+v draft=%q", a.fit, a.draft)
	}
	if a.workspace.ActiveKind() != "file" || !tt.HasText("Unsaved") {
		t.Fatal("returning to the session lost its active dirty file")
	}
	titlebarRect(t, tt, "Close tab layout.go")
	a.workspace.OpenFile("file-layout")
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(ui.Cmd, ui.KeyC)
	if tt.Clipboard() != "R10 unsaved workspace buffer" {
		t.Fatalf("workspace buffer did not survive unmount/remount: %q", tt.Clipboard())
	}
	if err := tt.Click("Expand workspace"); err != nil {
		t.Fatal(err)
	}
	if a.rightExpanded || !a.right {
		t.Fatal("restored workspace expand control was not reachable")
	}
	if err := tt.Click("Toggle right pane"); err != nil {
		t.Fatal(err)
	}
	if a.right || !a.files {
		t.Fatal("restored pane toggle changed the wrong panel")
	}
}

func TestTitlebarFilesOnlyKeepsInfoAndGlobalActionsApart(t *testing.T) {
	for _, captions := range []float32{0, 108} {
		t.Run(fmt.Sprintf("caption%g", captions), func(t *testing.T) {
			a := testApp(t, "default")
			a.selectSession(a.state.Sessions[0].ID)
			a.sidebar, a.right, a.files, a.rightExpanded = false, false, true, false
			tt := ui.NewTester(a.View, 900, 600)
			tt.SetTitleBar(ui.TitleBar{Right: captions})
			titlebarGeometry(t, tt, 900-captions-6)
			if _, ok := tt.Find("Workspace tabs"); ok {
				t.Fatal("Files-only mode built a workspace strip")
			}
			info, explorer := titlebarRect(t, tt, "Session information"), titlebarRect(t, tt, "Files explorer")
			if info.X+info.W >= explorer.X {
				t.Fatalf("session-info crossed the Files seam: info=%+v explorer=%+v", info, explorer)
			}
			if err := tt.Click("Session information"); err != nil {
				t.Fatal(err)
			}
			if !a.expanded["session-info-user"] {
				t.Fatal("Files-only info center was not reachable")
			}
			if err := tt.Click("Files"); err != nil {
				t.Fatal(err)
			}
			if a.files || len(tt.Menu()) != 0 {
				t.Fatal("Files-only primary close was not reachable")
			}
		})
	}
}

func titlebarRect(t *testing.T, tt *ui.Tester, label string) ui.Rect {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("missing %q", label)
	}
	return r
}

func titlebarGeometry(t *testing.T, tt *ui.Tester, rightEdge float32) map[string]ui.Rect {
	t.Helper()
	geometry := map[string]ui.Rect{}
	labels := []string{"Files", "Toggle right pane", "Expand workspace", "New surface", "Session information", "Toggle sidebar", "Back"}
	for _, label := range labels {
		r, ok := tt.Find(label)
		if !ok {
			continue
		}
		if r.X < 0 || r.Y < 0 || r.X+r.W > rightEdge+.01 || r.Y+r.H > titlebarHeight+.01 {
			t.Fatalf("%q left its visible header slot: %+v right=%g", label, r, rightEdge)
		}
		for other, bounds := range geometry {
			if r.X < bounds.X+bounds.W && bounds.X < r.X+r.W && r.Y < bounds.Y+bounds.H && bounds.Y < r.Y+r.H {
				t.Fatalf("interactive header controls overlap: %q=%+v %q=%+v", label, r, other, bounds)
			}
		}
		geometry[label] = r
	}
	files, pane := titlebarRect(t, tt, "Files"), titlebarRect(t, tt, "Toggle right pane")
	if files.W != 28 || pane.W != 28 || files.H != 28 || pane.H != 28 || !titlebarNear(pane.X-files.X-files.W, 4) || !titlebarNear(pane.X+pane.W, rightEdge) {
		t.Fatalf("global 28+4+28 slots/caption clearance: files=%+v pane=%+v edge=%g", files, pane, rightEdge)
	}
	for _, label := range []string{"Workspace tabs", "Tools workspace", "Files explorer"} {
		if r, ok := tt.Find(label); ok {
			geometry[label] = r
		}
	}
	return geometry
}

func titlebarNear(a, b float32) bool { return math.Abs(float64(a-b)) < .02 }

func captureTitlebarImage(t *testing.T, name string, tt *ui.Tester) {
	t.Helper()
	dir := os.Getenv("RCODE_TITLEBAR_AUDIT_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func captureTitlebarGeometry(t *testing.T, name string, geometry map[string]ui.Rect) {
	t.Helper()
	dir := os.Getenv("RCODE_TITLEBAR_AUDIT_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	b, err := json.MarshalIndent(struct {
		Kind     string
		Geometry map[string]ui.Rect
	}{"real App.View, ui.Tester committed visible rectangles; not native UIA/parity", geometry}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".json"), append(b, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}
