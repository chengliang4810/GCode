package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestTabStripSourceControlGeometry(t *testing.T) {
	for _, width := range []int{360, 520} {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("width%d-dark%t", width, dark), func(t *testing.T) {
				s, _, snapshot := testWorkspace(t)
				snapshot.Settings.Appearance.Mode = "light"
				if dark {
					snapshot.Settings.Appearance.Mode = "dark"
				}
				snapshot.Settings.Appearance.ThemeID = ""
				s.Update(snapshot)
				s.OpenSurface("changes")
				s.OnToggleExpand = func() {}
				tt := tabStripTester(s, width, false)
				geometry := map[string]ui.Rect{}
				for _, label := range []string{"Workspace tabs", "Scroll workspace tabs", "Changes", "New surface", "Expand workspace"} {
					var ok bool
					geometry[label], ok = tt.Find(label)
					if !ok {
						t.Fatalf("missing %q", label)
					}
				}
				captureTabStripGeometry(t, geometry)
				if got, want := geometry["Changes"], (ui.Rect{X: 8, Y: 9, W: 112, H: 24}); got != want {
					t.Errorf("chip geometry = %+v, want %+v", got, want)
				}
				if got, want := geometry["New surface"], (ui.Rect{X: 124, Y: 9, W: 24, H: 24}); got != want {
					t.Errorf("scrolling plus geometry = %+v, want %+v", got, want)
				}
				if got, want := geometry["Expand workspace"], (ui.Rect{X: float32(width - 32), Y: 7, W: 28, H: 28}); got != want {
					t.Errorf("expand geometry = %+v, want %+v", got, want)
				}
				scroll, expand := geometry["Scroll workspace tabs"], geometry["Expand workspace"]
				if expand.X-(scroll.X+scroll.W) != 4 {
					t.Errorf("scroll/expand gap is not 4: scroll=%+v expand=%+v", scroll, expand)
				}
				pixel := tt.Image().RGBAAt(width/2, 37)
				if pixel.R != 70 || pixel.G != 80 || pixel.B != 90 {
					t.Errorf("tab strip repainted its host material or added a bottom border: %+v", pixel)
				}
			})
		}
	}
}

func TestTabStripSourceEmptyAndNonGitMenu(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	s.OnToggleExpand = func() {}
	tt := tabStripTester(s, 360, false)
	if _, ok := tt.Find("New surface"); ok {
		t.Fatal("empty picker retained the redundant add-tab control")
	}
	for i := range snapshot.Projects {
		snapshot.Projects[i].GitDetected = false
	}
	s.Update(snapshot)
	s.OpenSurface("files")
	tt.Frame()
	if err := tt.Click("New surface"); err != nil {
		t.Fatal(err)
	}
	if got, want := tt.Menu(), []string{"Files", "Browser", "Terminal"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("non-Git menu = %v, want %v", got, want)
	}
	if err := tt.ChooseMenuItem("Files"); err != nil {
		t.Fatal(err)
	}
	if s.ActiveKind() != "files" || len(tt.Menu()) != 0 {
		t.Fatal("source-valid Files action did not finish")
	}
	// Git detection can precede the branch list's first successful load.
	snapshot.Projects[0].GitDetected = true
	snapshot.Projects[0].Branch, snapshot.Projects[0].Branches = "", nil
	s.ProjectID = snapshot.Projects[0].ID
	s.Update(snapshot)
	tt.Frame()
	if err := tt.Click("New surface"); err != nil {
		t.Fatal(err)
	}
	if got, want := tt.Menu(), []string{"Files", "Browser", "Terminal", "Diffs", "History"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("detected Git with an unloaded branch list lost entries: %v", got)
	}
	if err := tt.ChooseMenuItem("Diffs"); err != nil {
		t.Fatal(err)
	}
	if s.ActiveKind() != "changes" {
		t.Fatal("detected Git's Diffs action did not open its surface")
	}
}

func TestTabStripMenuRevealSurvivesRebuild(t *testing.T) {
	for _, width := range []int{320, 360} {
		for _, scale := range []float32{1, 2} {
			for _, existing := range []bool{false, true} {
				t.Run(fmt.Sprintf("width%d-scale%g-existing%t", width, scale, existing), func(t *testing.T) {
					s, _, _ := testWorkspace(t)
					s.ProjectID = "fieldnotes"
					s.OnToggleExpand = func() {}
					if existing {
						s.OpenSurface("history")
					}
					s.OpenSurface("changes")
					s.OpenChange("change-layout")
					s.OpenChange("change-readme")
					type build struct{ BeforeCount, AfterCount, BeforeActive, AfterActive int }
					var builds []build
					tt := ui.NewTester(func(c *ui.Context) {
						before := build{BeforeCount: s.TabCount(), BeforeActive: s.active}
						c.SetTheme(s.palette().UI())
						c.DisableAnimations()
						ui.Column(c).Fill().Children(func() { s.TabStrip(c, false) })
						before.AfterCount, before.AfterActive = s.TabCount(), s.active
						builds = append(builds, before)
					}, width, 80)
					tt.SetScale(scale)
					scroll, _ := tt.Find("Scroll workspace tabs")
					tt.Wheel(scroll.X+scroll.W/2, 32, 10000, 0)
					tt.Advance(250 * time.Millisecond)
					plus, _ := tt.Find("New surface")
					if plus.W != 24 || plus.H != 24 {
						t.Fatalf("old scrolling plus is not fully reachable: %+v", plus)
					}
					oldCount, oldMax := s.TabCount(), s.tabScroll.MaxX
					if existing {
						history, _ := tt.Find("History")
						if history.W != 0 {
							t.Fatalf("existing History must begin offscreen: %+v", history)
						}
					}
					tt.ClickAt(plus.X+plus.W/2, 32)
					if len(tt.Menu()) != 5 {
						t.Fatalf("production Git menu did not open: %v", tt.Menu())
					}
					beforeChoice := len(builds)
					if err := tt.ChooseMenuItem("History"); err != nil {
						t.Fatal(err)
					}
					history, _ := tt.Find("History")
					geometry := map[string]any{
						"history": history, "scroll": scroll, "scrollState": s.tabScroll,
						"oldCount": oldCount, "newCount": s.TabCount(), "oldMax": oldMax,
						"choiceBuilds": builds[beforeChoice:], "framePending": tt.FramePending(),
					}
					captureTabStripGeometry(t, geometry)
					wantCount := oldCount + 1
					if existing {
						wantCount = oldCount
					}
					if s.ActiveKind() != "history" || s.TabCount() != wantCount {
						t.Fatalf("menu chose wrong surface or duplicated it: active=%s count=%d want=%d", s.ActiveKind(), s.TabCount(), wantCount)
					}
					if len(builds)-beforeChoice < 2 {
						t.Fatal("menu choice did not exercise multiple build passes")
					}
					mutatedDuringBuild := false
					for _, pass := range builds[beforeChoice:] {
						mutatedDuringBuild = mutatedDuringBuild || pass.BeforeCount != pass.AfterCount || pass.BeforeActive != pass.AfterActive
					}
					if !mutatedDuringBuild {
						t.Fatal("menu regression bypassed the synchronous chosen callback")
					}
					if existing && s.tabScroll.MaxX != oldMax || !existing && s.tabScroll.MaxX <= oldMax {
						t.Fatalf("new layout did not retain/grow content extent: old=%g new=%g", oldMax, s.tabScroll.MaxX)
					}
					assertTabStripRevealed(t, tt, "History")
					geometry["manualScroll"] = assertTabStripManualScrollStays(t, s, tt)
					captureTabStripGeometry(t, geometry)
				})
			}
		}
	}
}

func TestTabStripDirectAndKeyboardRevealRemainOneShot(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		for _, route := range []string{"direct-new", "direct-existing", "keyboard-End"} {
			t.Run(fmt.Sprintf("scale%g-%s", scale, route), func(t *testing.T) {
				s, _, _ := testWorkspace(t)
				s.ProjectID = "fieldnotes"
				s.OnToggleExpand = func() {}
				if route != "direct-new" {
					s.OpenSurface("history")
				}
				s.OpenSurface("changes")
				s.OpenChange("change-layout")
				s.OpenChange("change-readme")
				if route == "keyboard-End" {
					s.OpenChange("change-theme")
					s.OpenSurface("history")
				}
				tt := tabStripTester(s, 360, false)
				tt.SetScale(scale)
				oldCount := s.TabCount()
				label, wantCount := "History", oldCount
				if route == "keyboard-End" {
					if err := tt.Click("History"); err != nil {
						t.Fatal(err)
					}
					tt.Key(0, ui.KeyEnd)
					label = "theme.go"
					if s.active != oldCount-1 || !tt.Focused(label) {
						t.Fatal("End did not select and focus the final native tab")
					}
				} else {
					s.OpenSurface("history")
					tt.Frame()
					if route == "direct-new" {
						wantCount++
					}
				}
				if s.TabCount() != wantCount || s.tabs[s.active].title != label {
					t.Fatalf("selection changed identity/count: active=%+v count=%d", s.tabs[s.active], s.TabCount())
				}
				assertTabStripRevealed(t, tt, label)
				captureTabStripGeometry(t, map[string]any{"label": label, "count": s.TabCount(), "manualScroll": assertTabStripManualScrollStays(t, s, tt)})
			})
		}
	}
}

func assertTabStripRevealed(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	chip, ok := tt.Find(label)
	scroll, _ := tt.Find("Scroll workspace tabs")
	if !ok || chip.W != 112 || chip.H != 24 || chip.X < scroll.X || chip.X+chip.W > scroll.X+scroll.W {
		t.Fatalf("selected %s was not fully revealed: chip=%+v scroll=%+v", label, chip, scroll)
	}
}

func assertTabStripManualScrollStays(t *testing.T, s *State, tt *ui.Tester) map[string]any {
	t.Helper()
	active, count := s.active, s.TabCount()
	label := s.tabs[active].title
	revealed := s.tabScroll
	dx := float32(-10000)
	if active == 0 {
		dx = 10000
	}
	scroll, _ := tt.Find("Scroll workspace tabs")
	tt.Wheel(scroll.X+scroll.W/2, 32, dx, 0)
	tt.Advance(250 * time.Millisecond)
	moved := s.tabScroll
	chip, _ := tt.Find(label)
	if moved.X == revealed.X || chip.W >= 112 {
		t.Fatalf("user wheel did not move selected tab out of view: before=%+v after=%+v chip=%+v", revealed, moved, chip)
	}
	tt.Frame()
	tt.Advance(time.Second)
	if s.tabScroll.X != moved.X || s.active != active || s.TabCount() != count {
		t.Fatalf("reveal kept pulling the wheel position or changed selection: before=%+v after=%+v", moved, s.tabScroll)
	}
	return map[string]any{"revealed": revealed, "afterWheel": moved, "afterFrameAndIdle": s.tabScroll, "active": label}
}

func TestTabStripBottomClicksAfterWheelAndHover(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		for _, target := range []string{"chip", "plus"} {
			t.Run(fmt.Sprintf("scale%g-%s", scale, target), func(t *testing.T) {
				s, _, _ := testWorkspace(t)
				for i := 0; i < 14; i++ {
					s.choose("changes", fmt.Sprint(i), fmt.Sprintf("Bottom click tab %02d", i))
				}
				expandCalls := 0
				s.OnToggleExpand = func() { expandCalls++ }
				tt := tabStripTester(s, 360, false)
				tt.SetScale(scale)
				expand, _ := tt.Find("Expand workspace")
				scroll, _ := tt.Find("Scroll workspace tabs")
				tt.Wheel(scroll.X+scroll.W/2, scroll.Y+scroll.H/2, 10000, 0)
				tt.Advance(250 * time.Millisecond)
				chip, ok := tt.Find(s.tabs[len(s.tabs)-2].title)
				if !ok || chip.Y != 9 || chip.H != 24 || chip.W != 112 {
					t.Fatalf("fixture lost the fully visible 24px chip: %+v", chip)
				}
				plus, _ := tt.Find("New surface")
				captureTabStripGeometry(t, map[string]ui.Rect{"chip": chip, "plus": plus, "scroll": scroll, "expand": expand})
				x := chip.X + 30
				if target == "plus" {
					x = plus.X + plus.W/2
				}
				// y32 is inside the last DIP of both 24px controls (y9..33).
				// Wheel plus hover exposes the old 12px overlay-track hitbox.
				tt.Move(x, 32)
				tt.ClickAt(x, 32)
				if target == "chip" {
					if s.active != len(s.tabs)-2 || !tt.Focused(s.tabs[s.active].title) {
						t.Fatalf("bottom chip click was intercepted: active=%d want=%d", s.active, len(s.tabs)-2)
					}
					tt.Key(0, ui.KeyEnd)
					if s.active != len(s.tabs)-1 {
						t.Fatal("End did not select/reveal the final tab after bottom click")
					}
					tt.Key(0, ui.KeyTab)
					tt.Key(0, ui.KeyEnter)
				} else if !tt.Focused("New surface") {
					t.Fatal("bottom plus click was intercepted before menu focus")
				}
				if len(tt.Menu()) == 0 {
					t.Fatal("add-tab menu did not open after bottom click or keyboard reveal")
				}
				tt.CloseMenu()
				if got, _ := tt.Find("Expand workspace"); got != expand {
					t.Fatalf("fixed expand slot moved: %+v -> %+v", expand, got)
				}
				if err := tt.Click("Expand workspace"); err != nil {
					t.Fatal(err)
				}
				if expandCalls != 1 {
					t.Fatal("fixed expand slot was not reachable after bottom click")
				}
			})
		}
	}
}

func TestTabStripOpaqueFadesUseSourceBoundsAndThresholds(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("scale%g-dark%t", scale, dark), func(t *testing.T) {
				s, _, snapshot := testWorkspace(t)
				snapshot.Settings.Appearance.Mode, snapshot.Settings.Appearance.ThemeID = "light", ""
				if dark {
					snapshot.Settings.Appearance.Mode = "dark"
				}
				snapshot.Settings.Appearance.Surface = "opaque"
				s.Update(snapshot)
				for i := 0; i < 8; i++ {
					s.choose("changes", fmt.Sprint(i), fmt.Sprintf("Fade tab %02d", i))
				}
				s.selectTab(0)
				s.OnToggleExpand = func() {}
				tt := tabStripTester(s, 360, false)
				tt.SetScale(scale)
				scroll, _ := tt.Find("Scroll workspace tabs")
				base, shell := ui.RGB(70, 80, 90), s.palette().Shell
				for _, tc := range []struct {
					name        string
					x           float32
					left, right bool
				}{
					{name: "start", right: true},
					{name: "left-exact-threshold", x: 1, right: true},
					{name: "left-beyond-threshold", x: 1.25, left: true, right: true},
					{name: "middle", x: 80, left: true, right: true},
					{name: "right-before-threshold", x: s.tabScroll.MaxX - 1.25, left: true, right: true},
					{name: "right-exact-threshold", x: s.tabScroll.MaxX - 1, left: true},
					{name: "end", x: s.tabScroll.MaxX, left: true},
				} {
					t.Run(tc.name, func(t *testing.T) {
						s.tabScroll.X = tc.x
						tt.Frame()
						for _, side := range []struct {
							left, on bool
						}{{left: true, on: tc.left}, {on: tc.right}} {
							for _, delta := range []float32{0, 18, 35, 37} {
								x := scroll.X + delta
								if !side.left {
									x = scroll.X + scroll.W - 1/scale - delta
								}
								want := base
								if side.on && delta < 36 {
									want = shell.Alpha(1 - (delta+.5/scale)/36).Over(base)
								}
								for _, y := range []float32{5, 37} {
									if got := tt.Image().RGBAAt(int(x*scale), int(y*scale)); !tabStripPixelNear(got, want) {
										t.Errorf("left=%v delta=%g y=%g fade pixel=%+v want=%+v", side.left, delta, y, got, want)
									}
								}
							}
						}
						for _, point := range [][2]float32{{scroll.X - 1/scale, 5}, {scroll.X + scroll.W, 5}, {scroll.X + 1, 3}, {scroll.X + 1, 38}} {
							if got := tt.Image().RGBAAt(int(point[0]*scale), int(point[1]*scale)); !tabStripPixelNear(got, base) {
								t.Errorf("fade escaped its full-height region at %v: %+v", point, got)
							}
						}
					})
				}
				for len(s.tabs) > 1 {
					s.closeTab(0)
				}
				tt.Frame()
				if s.tabScroll.MaxX != 0 {
					t.Fatal("fixture did not remove overflow")
				}
				for _, x := range []float32{scroll.X, scroll.X + scroll.W - 1/scale} {
					if got := tt.Image().RGBAAt(int(x*scale), int(5*scale)); !tabStripPixelNear(got, base) {
						t.Fatalf("content shrink left a stale fade: %+v", got)
					}
				}
			})
		}
	}
}

func TestTabStripGlassDoesNotPaintAnOpaqueFade(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	snapshot.Settings.Appearance.Mode, snapshot.Settings.Appearance.ThemeID = "light", "zeron-light"
	snapshot.Settings.Appearance.Surface = "frosted"
	s.Update(snapshot)
	for i := 0; i < 8; i++ {
		s.choose("changes", fmt.Sprint(i), fmt.Sprintf("Glass tab %02d", i))
	}
	s.selectTab(0)
	s.OnToggleExpand = func() {}
	tt := tabStripTester(s, 360, false)
	tt.SetVibrancy(true)
	s.tabScroll.X = 80
	tt.Frame()
	scroll, _ := tt.Find("Scroll workspace tabs")
	base := ui.RGB(70, 80, 90)
	for _, x := range []float32{scroll.X, scroll.X + scroll.W - 1} {
		if got := tt.Image().RGBAAt(int(x), 5); !tabStripPixelNear(got, base) {
			t.Fatalf("glass was replaced with an opaque gradient plate: %+v", got)
		}
	}
	// Unsupported glass uses the host's actual opaque fallback treatment.
	tt.SetVibrancy(false)
	if got := tt.Image().RGBAAt(int(scroll.X), 5); tabStripPixelNear(got, base) {
		t.Fatal("opaque host fallback did not paint its source shell fade")
	}
}

func TestTabStripFadesDoNotTakeWheelOrBottomPresses(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	snapshot.Settings.Appearance.Surface = "opaque"
	s.Update(snapshot)
	for i := 0; i < 8; i++ {
		s.choose("changes", fmt.Sprint(i), fmt.Sprintf("Fade hit tab %02d", i))
	}
	s.OnToggleExpand = func() {}
	tt := tabStripTester(s, 360, false)
	s.tabScroll.X = 40
	tt.Frame()
	scroll, _ := tt.Find("Scroll workspace tabs")
	x, y := scroll.X+12, float32(32)
	before := s.tabScroll.X
	tt.Wheel(x, y, 12, 0)
	tt.Advance(250 * time.Millisecond)
	if s.tabScroll.X <= before {
		t.Fatal("the painted left fade intercepted wheel input")
	}
	tt.Move(x, y)
	tt.ClickAt(x, y)
	if s.active != 0 || !tt.Focused(s.tabs[0].title) {
		t.Fatal("the painted left fade intercepted the underlying chip's bottom press")
	}
}

func TestTabStripNarrowFadeKeepsIts36DIPRampAndClip(t *testing.T) {
	base, shell := ui.RGB(70, 80, 90), ui.RGB(230, 220, 210)
	tt := ui.NewTester(func(c *ui.Context) {
		c.Root().Background(base)
		ui.Box(c).Absolute().Left(10).Top(4).Width(20).Height(34).DrawOver(func(painter *ui.Painter, r ui.Rect) {
			paintTabStripFades(painter, r, shell, true, false)
		})
	}, 60, 60)
	want := shell.Alpha(1 - 19.5/36).Over(base)
	if got := tt.Image().RGBAAt(29, 5); !tabStripPixelNear(got, want) {
		t.Fatalf("narrow region compressed the source 36px ramp: %+v want %+v", got, want)
	}
	for _, point := range [][2]int{{9, 5}, {30, 5}, {10, 3}, {10, 38}} {
		if got := tt.Image().RGBAAt(point[0], point[1]); !tabStripPixelNear(got, base) {
			t.Fatalf("narrow fade painted outside its region at %v: %+v", point, got)
		}
	}
}

func TestTabStripFadeSettlesWithoutExtraHostFrames(t *testing.T) {
	for _, tc := range []struct {
		name              string
		width, nextWidth  int
		count             int
		parent, wantRight bool
	}{
		{name: "mount-overflow", width: 360, count: 3, wantRight: true},
		{name: "close-inactive-last", width: 360, count: 3},
		{name: "window-grow", width: 360, nextWidth: 500, count: 3},
		{name: "window-shrink", width: 500, nextWidth: 360, count: 3, wantRight: true},
		{name: "parent-grow-same-window", width: 360, nextWidth: 500, count: 3, parent: true},
		{name: "parent-shrink-same-window", width: 500, nextWidth: 360, count: 3, parent: true, wantRight: true},
		{name: "scrolled-content-clamp", width: 360, count: 8},
		{name: "theme-change", width: 360, count: 3, wantRight: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, snapshot := testWorkspace(t)
			snapshot.Settings.Appearance.Mode, snapshot.Settings.Appearance.ThemeID = "light", ""
			snapshot.Settings.Appearance.Surface = "opaque"
			s.Update(snapshot)
			for i := 0; i < tc.count; i++ {
				s.choose("changes", fmt.Sprint(i), fmt.Sprintf("Idle tab %02d", i))
			}
			s.selectTab(0)
			s.OnToggleExpand = func() {}
			width, builds := tc.width, 0
			windowWidth := width
			if tc.parent {
				windowWidth = 600
			}
			base := ui.RGB(70, 80, 90)
			tt := ui.NewTester(func(c *ui.Context) {
				builds++
				c.SetTheme(s.palette().UI())
				c.DisableAnimations()
				c.Root().Background(base)
				parent := ui.Box(c).Height(38)
				if tc.parent {
					parent.Width(float32(width))
				} else {
					parent.FillWidth()
				}
				parent.Children(func() { s.TabStrip(c, false) })
			}, windowWidth, 80)
			if tc.name != "mount-overflow" {
				// A normal off-strip pointer event establishes the initial
				// scene without leaving tooltip or hover-animation timers.
				tt.Move(2, 70)
			}
			before := builds
			switch tc.name {
			case "close-inactive-last":
				s.closeTab(len(s.tabs) - 1)
				tt.Frame() // The single host invalidation for this action.
			case "window-grow", "window-shrink":
				tt.SetSize(tc.nextWidth, 80) // Resize renders normally itself.
			case "parent-grow-same-window", "parent-shrink-same-window":
				width = tc.nextWidth
				tt.Frame() // Window size stays unchanged; the pane was resized.
			case "scrolled-content-clamp":
				scroll, _ := tt.Find("Scroll workspace tabs")
				tt.Scroll(scroll.X+scroll.W/2, scroll.Y+scroll.H/2, 10000, 0)
				tt.Move(2, 70)
				if s.tabScroll.X == 0 {
					t.Fatal("fixture did not scroll before clamping")
				}
				for len(s.tabs) > 2 {
					s.closeTab(len(s.tabs) - 1)
				}
				tt.Frame()
			case "theme-change":
				snapshot.Settings.Appearance.Mode = "dark"
				s.Update(snapshot)
				tt.Frame()
			}
			settledBuilds, pending := builds, tt.FramePending()
			// Advance only services requests made by the host/application.
			// There is no second forced Frame to repair a stale fade.
			tt.Advance(time.Second)
			scroll, _ := tt.Find("Scroll workspace tabs")
			pixel := tt.Image().RGBAAt(int(scroll.X+scroll.W-1), 5)
			want := base
			if tc.wantRight {
				want = s.palette().Shell.Alpha(1 - .5/36).Over(base)
			}
			captureTabStripGeometry(t, map[string]any{
				"buildsBeforeAction": before, "buildsAfterNormalRender": settledBuilds, "buildsAfterIdleAdvance": builds,
				"hostFramePendingAfterRender": pending, "hostFramePendingAfterIdle": tt.FramePending(),
				"offsetX": s.tabScroll.X, "maxX": s.tabScroll.MaxX, "scroll": scroll, "rightPixel": pixel, "expectedPixel": want,
			})
			if !tabStripPixelNear(pixel, want) {
				t.Errorf("idle fade stale after normal render: pixel=%+v want=%+v X=%g MaxX=%g builds=%d->%d->%d pending=%v", pixel, want, s.tabScroll.X, s.tabScroll.MaxX, before, settledBuilds, builds, pending)
			}
			if builds != settledBuilds || tt.FramePending() {
				t.Errorf("idle strip continued requesting frames: builds=%d->%d pending=%v", settledBuilds, builds, tt.FramePending())
			}
			if s.tabScroll.X != 0 {
				t.Fatalf("fixture did not settle at X=0: %+v", s.tabScroll)
			}
		})
	}
}

func TestTabStripSourceHoverAndExpandedGlyph(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprintf("dark%t", dark), func(t *testing.T) {
			s, _, snapshot := testWorkspace(t)
			snapshot.Settings.Appearance.Mode = "light"
			if dark {
				snapshot.Settings.Appearance.Mode = "dark"
			}
			snapshot.Settings.Appearance.ThemeID = ""
			s.Update(snapshot)
			s.ProjectID = snapshot.Projects[0].ID
			s.OpenSurface("changes")
			expanded, calls := false, 0
			s.OnToggleExpand = func() { expanded = !expanded; calls++ }
			base := ui.RGB(70, 80, 90)
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(s.palette().UI())
				c.Root().Background(base)
				ui.Column(c).Fill().Children(func() { s.TabStrip(c, expanded) })
			}, 360, 80)
			expand, ok := tt.Find("Expand workspace")
			if !ok {
				t.Fatal("missing expand control")
			}
			initialGlyph := tabStripGlyphPixels(tt.Image(), expand, 16)
			wash := ui.RGBA(26, 26, 26, .11)
			if dark {
				wash = ui.RGBA(235, 235, 235, .11)
			}
			for _, label := range []string{"Expand workspace", "New surface"} {
				button, ok := tt.Find(label)
				if !ok {
					t.Fatalf("missing %q", label)
				}
				x, y := int(button.X+3), int(button.Y+button.H/2)
				tt.Move(float32(x), float32(y))
				tt.Advance(75 * time.Millisecond)
				mid := tt.Image().RGBAAt(x, y)
				tt.Advance(75 * time.Millisecond)
				end := tt.Image().RGBAAt(x, y)
				want := wash.Over(base)
				if mid == end || mid == (color.RGBA{R: base.R, G: base.G, B: base.B, A: 255}) {
					t.Errorf("%s hover skipped the 150ms blend: mid=%+v end=%+v", label, mid, end)
				}
				if !tabStripPixelNear(end, want) {
					t.Errorf("%s hover wash = %+v, want %+v", label, end, want)
				}
				tt.Move(2, 70)
				tt.Advance(150 * time.Millisecond)
				if got := tt.Image().RGBAAt(x, y); !tabStripPixelNear(got, base) {
					t.Errorf("%s retained hover wash: %+v", label, got)
				}
			}
			if err := tt.Click("Expand workspace"); err != nil {
				t.Fatal(err)
			}
			tt.Move(2, 70)
			tt.Advance(150 * time.Millisecond)
			if !expanded || calls != 1 {
				t.Fatal("expand click did not invoke its host once")
			}
			if got, _ := tt.Find("Expand workspace"); got != expand {
				t.Fatalf("expanded control moved: %+v -> %+v", expand, got)
			}
			if reflect.DeepEqual(initialGlyph, tabStripGlyphPixels(tt.Image(), expand, 16)) {
				t.Fatal("expanded control retained the expand-arrows glyph")
			}
			tt.Key(0, ui.KeySpace)
			if expanded || calls != 2 || !reflect.DeepEqual(initialGlyph, tabStripGlyphPixels(tt.Image(), expand, 16)) {
				t.Fatal("keyboard collapse did not restore the glyph and host state")
			}
			if err := tt.Click("New surface"); err != nil {
				t.Fatal(err)
			}
			if got, want := tt.Menu(), []string{"Files", "Browser", "Terminal", "Diffs", "History"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("Git menu = %v, want %v", got, want)
			}
			tt.Move(2, 70)
			tt.Advance(150 * time.Millisecond)
			plus, _ := tt.Find("New surface")
			if got := tt.Image().RGBAAt(int(plus.X+3), int(plus.Y+plus.H/2)); !tabStripPixelNear(got, base) {
				t.Fatalf("open menu introduced an active plate absent from the source: %+v", got)
			}
		})
	}
}

func TestTabStripControlsKeepIdentityAndExpandSlotAtOverflow(t *testing.T) {
	s, _, _ := testWorkspace(t)
	for i := 0; i < 14; i++ {
		s.choose("changes", fmt.Sprint(i), fmt.Sprintf("Overflow tab %02d", i))
	}
	expandCalls := 0
	s.OnToggleExpand = func() { expandCalls++ }
	tt := tabStripTester(s, 360, false)
	expand, _ := tt.Find("Expand workspace")
	scroll, _ := tt.Find("Scroll workspace tabs")
	tt.Scroll(scroll.X+scroll.W/2, scroll.Y+scroll.H/2, 10000, 0)
	if err := tt.Click("New surface"); err != nil {
		t.Fatal(err)
	}
	tt.CloseMenu()
	if !tt.Focused("New surface") {
		t.Fatal("menu trigger did not retain its native focus")
	}
	s.closeTab(0)
	tt.Frame()
	if !tt.Focused("New surface") {
		t.Fatal("closing a preceding chip changed the add-tab control's identity")
	}
	if got, _ := tt.Find("Expand workspace"); got != expand {
		t.Fatalf("fixed expand slot moved with scroll or close: %+v -> %+v", expand, got)
	}
	if err := tt.Click("Expand workspace"); err != nil {
		t.Fatal(err)
	}
	if expandCalls != 1 {
		t.Fatal("fixed expand slot became unreachable at overflow")
	}
}

func TestTabStripSourceMotionPreferences(t *testing.T) {
	for _, tc := range []struct {
		name   string
		system bool
		motion ui.MotionPreference
		snap   bool
	}{
		{name: "system-reduced", system: true, motion: ui.MotionSystem, snap: true},
		{name: "app-reduced", motion: ui.MotionReduced, snap: true},
		{name: "app-full-overrides-system", system: true, motion: ui.MotionFull},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, snapshot := testWorkspace(t)
			snapshot.Settings.Appearance.Mode, snapshot.Settings.Appearance.ThemeID = "light", ""
			s.Update(snapshot)
			s.OpenSurface("files")
			s.OnToggleExpand = func() {}
			base := ui.RGB(70, 80, 90)
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetMotionPreferences(&ui.MotionPreferences{Motion: tc.motion})
				c.SetTheme(s.palette().UI())
				c.Root().Background(base)
				ui.Column(c).Fill().Children(func() { s.TabStrip(c, false) })
			}, 360, 80)
			tt.SetPreferences(ui.Preferences{ReduceMotion: tc.system})
			for _, label := range []string{"Expand workspace", "New surface"} {
				button, _ := tt.Find(label)
				x, y := int(button.X+3), int(button.Y+button.H/2)
				tt.Move(float32(x), float32(y))
				want := base
				if tc.snap {
					want = ui.RGBA(26, 26, 26, .11).Over(base)
				}
				if got := tt.Image().RGBAAt(x, y); !tabStripPixelNear(got, want) {
					t.Errorf("%s immediate hover = %+v, want %+v", label, got, want)
				}
				tt.Advance(150 * time.Millisecond)
				tt.Move(2, 70)
				tt.Advance(150 * time.Millisecond)
			}
		})
	}
}

func TestTabStripGitDetectionDoesNotBorrowAnotherProject(t *testing.T) {
	for _, project := range []string{"", "missing-project", "fieldnotes"} {
		t.Run(fmt.Sprintf("project%q", project), func(t *testing.T) {
			s, _, snapshot := testWorkspace(t)
			snapshot.Projects[0].GitDetected = false
			s.Update(snapshot)
			s.ProjectID = project
			tt := ui.NewTester(s.View, 360, 600)
			if tt.HasText("Diffs") || tt.HasText("History") || !tt.HasText("Browser") || !tt.HasText("Terminal") {
				t.Fatal("launcher borrowed Git detection from a different project")
			}
			s.OpenSurface("files")
			tt.Frame()
			if err := tt.Click("New surface"); err != nil {
				t.Fatal(err)
			}
			if got, want := tt.Menu(), []string{"Files", "Browser", "Terminal"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("menu borrowed Git detection from a different project: %v", got)
			}
		})
	}
}

func tabStripPixelNear(got color.RGBA, want ui.Color) bool {
	near := func(a, b uint8) bool { return int(max(a, b))-int(min(a, b)) <= 1 }
	return near(got.R, want.R) && near(got.G, want.G) && near(got.B, want.B) && got.A == want.A
}

func tabStripGlyphPixels(img *image.RGBA, button ui.Rect, size int) []color.RGBA {
	x, y := int(button.X+(button.W-float32(size))/2), int(button.Y+(button.H-float32(size))/2)
	pixels := make([]color.RGBA, 0, size*size)
	for row := y; row < y+size; row++ {
		for col := x; col < x+size; col++ {
			pixels = append(pixels, img.RGBAAt(col, row))
		}
	}
	return pixels
}

func tabStripTester(s *State, width int, expanded bool) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		c.SetTheme(s.palette().UI())
		c.DisableAnimations()
		c.Root().Background(ui.RGB(70, 80, 90))
		ui.Column(c).Fill().Children(func() { s.TabStrip(c, expanded) })
	}, width, 80)
}

func captureTabStripGeometry(t *testing.T, geometry any) {
	t.Helper()
	dir := os.Getenv("ZERON_TABSTRIP_AUDIT_DIR")
	if dir == "" {
		return
	}
	path := filepath.Join(dir, t.Name()+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(geometry, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSystemThemeUsesHostAppearanceForAllWorkspaceSurfaces(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprint(dark), func(t *testing.T) {
			s, _, snapshot := testWorkspace(t)
			snapshot.Settings.Appearance.Mode = "system"
			snapshot.Settings.Appearance.ThemeID = ""
			snapshot.Settings.Appearance.LightThemeID = "zeron-light"
			snapshot.Settings.Appearance.DarkThemeID = "zeron-dark"
			s.Update(snapshot)
			host := design.Palette("zeron", dark)
			tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(host.UI()); s.View(c) }, 520, 600)
			if s.palette().Dark != dark || s.palette().Background != host.Background {
				t.Fatal("workspace ignored the host's resolved system appearance")
			}
			s.OpenSurface("changes")
			tt.Frame()
			if s.palette().Dark != dark {
				t.Fatal("tab navigation changed resolved appearance")
			}
			tt = ui.NewTester(func(c *ui.Context) { c.SetTheme(host.UI()); s.FilesView(c) }, 286, 600)
			if s.palette().Dark != dark {
				t.Fatal("explorer ignored host appearance")
			}
			if _, ok := tt.Find("Search files"); !ok {
				t.Fatal("file search absent")
			}
		})
	}
}

func TestExplorerPreservesHostRootMaterialAndTheme(t *testing.T) {
	s, _, _ := testWorkspace(t)
	host := design.Palette("zeron-light", false).UI()
	rootColor := ui.RGB(23, 45, 67)
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(host)
		c.Root().Background(rootColor)
		ui.Box(c).Width(100).Height(100).Children(func() { s.FilesView(c) })
		if c.Theme() != host {
			t.Fatal("explorer replaced host theme")
		}
	}, 300, 250)
	pixel := tt.Image().RGBAAt(250, 200)
	if pixel.R != rootColor.R || pixel.G != rootColor.G || pixel.B != rootColor.B {
		t.Fatalf("explorer overwrote host material: %+v", pixel)
	}
}

func TestOverflowTabsKeepFixedGeometryAndKeyboardReveal(t *testing.T) {
	s, _, _ := testWorkspace(t)
	for i := 0; i < 14; i++ {
		s.choose("changes", fmt.Sprint(i), fmt.Sprintf("Long workspace tab %02d", i))
	}
	tt := ui.NewTester(s.View, 360, 600)
	if s.tabScroll.MaxX <= 0 {
		t.Fatal("tabs were squeezed instead of scrolling")
	}
	last, ok := tt.Find(s.tabs[s.active].title)
	scroll, _ := tt.Find("Scroll workspace tabs")
	if !ok || last.W != 112 || last.H != 24 || last.X < scroll.X || last.X+last.W > scroll.X+scroll.W {
		t.Fatalf("active tab was not revealed: %+v", last)
	}
	// The add-tab button belongs to the strip's content, after the last chip.
	// It is reached by scrolling, rather than occupying a fixed outer slot.
	tt.Scroll(scroll.X+scroll.W/2, scroll.Y+scroll.H/2, 10000, 0)
	plus, _ := tt.Find("New surface")
	last, _ = tt.Find(s.tabs[s.active].title)
	if plus.X != last.X+last.W+4 || plus.X+plus.W != scroll.X+scroll.W || plus.W != 24 {
		t.Fatalf("add button did not scroll after the last tab: last=%+v plus=%+v scroll=%+v", last, plus, scroll)
	}
	s.selectTab(0)
	tt.Frame()
	if err := tt.Click(s.tabs[0].title); err != nil {
		t.Fatal(err)
	}
	if !tt.Focused(s.tabs[0].title) {
		first, _ := tt.Find(s.tabs[0].title)
		t.Fatalf("first tab click did not focus its rendered target: rect=%+v scroll=%+v", first, s.tabScroll)
	}
	tt.Key(0, ui.KeyEnd)
	if s.active != len(s.tabs)-1 {
		t.Fatalf("End did not select last tab: %d", s.active)
	}
	last, _ = tt.Find(s.tabs[s.active].title)
	if last.X < scroll.X || last.X+last.W > scroll.X+scroll.W {
		t.Fatalf("keyboard selection did not scroll into view: %+v", last)
	}
	tt.Key(0, ui.KeyTab)
	if tt.Focused("Close tab " + s.tabs[s.active].title) {
		tt.Key(0, ui.KeyTab)
	}
	if !tt.Focused("New surface") {
		t.Fatal("the scrolling add-tab control was not reachable after the last tab")
	}
	tt.Key(0, ui.KeyEnter)
	if len(tt.Menu()) == 0 {
		t.Fatal("the scrolling add-tab control did not open from the keyboard")
	}
}

func TestReorderAndClosingInactiveTabsPreserveActiveIdentity(t *testing.T) {
	s, _, _ := testWorkspace(t)
	for _, title := range []string{"First", "Second", "Third"} {
		s.choose("changes", title, title)
	}
	active := s.tabs[s.active]
	s.moveTab(tabDrag{"changes", "Third"}, tabDrag{"changes", "First"})
	if s.tabs[s.active] != active || s.active != 0 {
		t.Fatal("moving tabs changed the active surface")
	}
	tt := ui.NewTester(s.View, 520, 600)
	if err := tt.Click("Close tab Second"); err != nil {
		t.Fatal(err)
	}
	if s.tabs[s.active] != active || len(s.tabs) != 2 {
		t.Fatal("closing inactive tab changed the active surface")
	}
}

func TestTabsDragAndWheelKeepSelection(t *testing.T) {
	s, _, _ := testWorkspace(t)
	for _, title := range []string{"First", "Second", "Third", "Fourth", "Fifth"} {
		s.choose("changes", title, title)
	}
	s.selectTab(0)
	tt := ui.NewTester(s.View, 520, 600)
	first, _ := tt.Find("First")
	second, _ := tt.Find("Second")
	x, y := first.X+30, first.Y+12
	tt.Press(x, y)
	tt.Move(x+12, y)
	tt.Move(second.X+50, y)
	tt.Release(second.X+50, y)
	if s.tabs[0].title != "Second" || s.tabs[1].title != "First" {
		t.Fatalf("native tab drag did not reorder: %+v", s.tabs)
	}
	active := s.tabs[s.active]
	tt.Scroll(180, 20, 180, 0)
	tt.Advance(time.Second)
	if s.tabScroll.X <= 0 || s.tabs[s.active] != active {
		t.Fatal("overflow wheel did not scroll independently of active selection")
	}
}

func TestFileRenameUpdatesTabAndRetainsUnsavedBuffer(t *testing.T) {
	s, backend, snapshot := testWorkspace(t)
	var file contracts.File
	for _, candidate := range snapshot.Files {
		if candidate.Kind == "file" && !candidate.ReadOnly {
			file = candidate
			break
		}
	}
	s.OpenFile(file.ID)
	s.documents[file.ID].editor.Buffer.Set("unsaved edit")
	if err := backend.Execute(context.Background(), contracts.Command{Kind: "file.rename", ID: file.ID, Name: "renamed.go"}); err != nil {
		t.Fatal(err)
	}
	next, err := backend.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s.Update(next)
	if s.tabs[s.active].title != "renamed.go" || s.documents[file.ID].file.Name != "renamed.go" || s.documents[file.ID].editor.Buffer.String() != "unsaved edit" {
		t.Fatal("rename lost the document identity or its unsaved edits")
	}
}

func TestExplorerFiltersHiddenAncestorsAndSortsDirectoriesFirst(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	snapshot.Files = []contracts.File{{ID: "z", Name: "z.go", Path: "z.go", Kind: "file"}, {ID: "folder", Name: "src", Path: "src", Kind: "directory"}, {ID: "a", Name: "a.go", Path: "a.go", Kind: "file"}, {ID: "hidden", Name: "config", Path: ".private/config", Kind: "file"}}
	snapshot.Settings.Files.ShowHidden = false
	s.Update(snapshot)
	got := s.filesIn("")
	if len(got) != 3 || got[0].ID != "folder" || got[1].ID != "a" {
		t.Fatalf("file tree order/filter: %+v", got)
	}
	s.fileSearch = "config"
	tt := ui.NewTester(s.FilesView, 286, 600)
	if tt.HasText(".private/config") {
		t.Fatal("search leaked a hidden ancestor")
	}
	if err := tt.Click("Search files"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEscape)
	if s.fileSearch != "" {
		t.Fatal("Escape did not close search")
	}
}

func TestExplorerDirectoryToggleAndRowGeometry(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	var dir, child contracts.File
	for _, candidate := range snapshot.Files {
		if candidate.Kind != "directory" || candidate.ParentID != "" {
			continue
		}
		for _, next := range snapshot.Files {
			if next.ParentID == candidate.ID {
				dir, child = candidate, next
				break
			}
		}
		if dir.ID != "" {
			break
		}
	}
	if dir.ID == "" {
		t.Fatal("fixture has no directory with children")
	}
	tt := ui.NewTester(s.FilesView, 286, 600)
	row, ok := tt.Find(dir.Name)
	if !ok || row.H != 27 {
		t.Fatalf("directory row differs from source geometry: %+v", row)
	}
	if _, ok := tt.Find(child.Name); !ok {
		t.Fatal("expanded directory omitted children")
	}
	if err := tt.Click(dir.Name); err != nil {
		t.Fatal(err)
	}
	if *s.folders[dir.ID] {
		t.Fatal("clicking directory name did not collapse it")
	}
	if _, ok := tt.Find(child.Name); ok {
		t.Fatal("collapsed directory retained child widgets")
	}
	tt.Key(0, ui.KeyRight)
	if !*s.folders[dir.ID] {
		t.Fatal("native Tree Right key did not expand directory")
	}
}

func TestNewFileEditorReceivesFocusAndCommittedIMEInput(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	var file contracts.File
	for _, candidate := range snapshot.Files {
		if candidate.Kind == "file" && !candidate.ReadOnly {
			file = candidate
			break
		}
	}
	s.OpenFile(file.ID)
	tt := ui.NewTester(s.View, 720, 560)
	document := s.documents[file.ID]
	before := document.editor.Buffer.String()
	tt.Compose("你好", 2)
	// As the official editor composition test does, commit through TextInput.
	// Tester.Key sends an unfiltered raw key; native IMM32/NSTextInput filters
	// candidate-selection Enter before this custom editor receives the key.
	tt.Type("您好")
	if document.editor.Buffer.String() == before || !document.editor.Buffer.Modified() {
		t.Fatal("newly opened file did not receive committed input")
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if document.editor.Buffer.String() != before {
		t.Fatal("committed IME edit did not undo as one operation")
	}
}

func TestWorkspaceThemeCatalogAndConfiguredEditorFont(t *testing.T) {
	for _, theme := range design.Themes() {
		t.Run(theme.ID, func(t *testing.T) {
			s, _, snapshot := testWorkspace(t)
			snapshot.Settings.Appearance.Mode = theme.Appearance
			snapshot.Settings.Appearance.ThemeID = theme.ID
			snapshot.Settings.Appearance.CodeFont = "Geist Mono"
			snapshot.Settings.Appearance.CodeFontSize = 20
			s.Update(snapshot)
			s.OpenChange(snapshot.Changes[0].ID)
			tt := ui.NewTester(s.View, 520, 600)
			p := design.Palette(theme.ID, theme.Appearance == "dark")
			if s.palette().Background != p.Background || s.diffs[snapshot.Changes[0].ID].Theme.Background != p.Background {
				t.Fatal("workspace/diff palette differs from selected theme")
			}
			if s.diffs[snapshot.Changes[0].ID].Font.Size != 20*13/12.5 {
				t.Fatal("diff ignored code font size")
			}
			if _, ok := tt.Find("Workspace tabs"); !ok {
				t.Fatal("theme removed tabs")
			}
		})
	}
}
