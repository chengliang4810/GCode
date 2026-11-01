package workbench

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"

	"github.com/egoist/mygo/ui"
)

func tabTooltipFile(t *testing.T, s *State, snapshot contracts.Snapshot) contracts.File {
	t.Helper()
	for _, file := range snapshot.Files {
		if !file.ReadOnly && file.Kind != "directory" && file.Kind != "image" {
			s.OpenFile(file.ID)
			return file
		}
	}
	t.Fatal("fixture has no editable file")
	return contracts.File{}
}

func requireTabTooltip(t *testing.T, tt *tabTooltipTestWindow, visible bool) ui.Rect {
	t.Helper()
	r, ok := tt.card(false)
	if ok != visible {
		t.Fatalf("detail visible=%t, want%t; texts=%v", ok, visible, tt.Texts())
	}
	return r
}

func TestTabTooltipSourceDelayAndCapturedPoint(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, scale := range []float32{1, 2} {
			t.Run(fmt.Sprintf("dark%t-scale%g", dark, scale), func(t *testing.T) {
				s, _, snapshot := testWorkspace(t)
				snapshot.Settings.Appearance.Mode = "light"
				if dark {
					snapshot.Settings.Appearance.Mode = "dark"
				}
				snapshot.Settings.Appearance.ThemeID = ""
				s.Update(snapshot)
				file := tabTooltipFile(t, s, snapshot)
				tt := tabTooltipStripTester(s, 360, false)
				tt.SetScale(scale)
				chip, ok := tt.Find(file.Name)
				if !ok {
					t.Fatal("file chip missing")
				}
				x, y := chip.X+3, chip.Y+chip.H/2
				tt.Move(x, y)
				tt.Advance(349 * time.Millisecond)
				requireTabTooltip(t, tt, false)
				tt.Advance(time.Millisecond)
				tip := requireTabTooltip(t, tt, true)
				if !tt.HasText(file.Path) || tip.X != x+1 || tip.Y != y+1 {
					t.Fatalf("350ms detail/point mismatch: tip%+v mouse%g,%g texts%v", tip, x, y, tt.Texts())
				}
				tt.Move(x+7, y+1)
				if moved := requireTabTooltip(t, tt, true); moved != tip {
					t.Fatalf("visible tip followed pointer: before%+v after%+v", tip, moved)
				}
				tt.Move(340, 65)
				requireTabTooltip(t, tt, false)
				tt.Move(x, y)
				tt.Advance(349 * time.Millisecond)
				requireTabTooltip(t, tt, false)
				tt.Advance(time.Millisecond)
				requireTabTooltip(t, tt, true)
			})
		}
	}
}

func TestTabTooltipPendingKeepsCapturedTextAndDeadline(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	file := tabTooltipFile(t, s, snapshot)
	tt := tabTooltipStripTester(s, 360, false)
	chip, _ := tt.Find(file.Name)
	x, y := chip.X+3, chip.Y+chip.H/2
	tt.Move(x, y)
	tt.Advance(200 * time.Millisecond)
	changed := file.Path + ".renamed"
	s.documents[file.ID].file.Path = changed
	tt.Frame()
	tt.Move(x+2, y)
	tt.Advance(150 * time.Millisecond)
	requireTabTooltip(t, tt, true)
	if !tt.HasText(file.Path) || tt.HasText(changed) {
		t.Fatalf("pending task replaced its captured text: %v", tt.Texts())
	}
	s.documents[file.ID].file.Path += ".again"
	tt.Frame()
	if !tt.HasText(file.Path) {
		t.Fatal("visible task refreshed text without clearing")
	}
	tt.Move(340, 65)
	tt.Move(x, y)
	tt.Advance(350 * time.Millisecond)
	if !tt.HasText(s.documents[file.ID].file.Path) {
		t.Fatal("rehover did not build current resource detail")
	}
}

func TestTabTooltipNestedCloseHasIndependent500msTask(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	file := tabTooltipFile(t, s, snapshot)
	tt := tabTooltipStripTester(s, 360, false)
	close, _ := tt.Find("Close tab " + file.Name)
	tt.Move(close.X+close.W/2, close.Y+close.H/2)
	tt.Advance(350 * time.Millisecond)
	requireTabTooltip(t, tt, true)
	if !tt.HasText(file.Path) {
		t.Fatal("parent detail did not mature before nested close")
	}
	tt.Advance(149 * time.Millisecond)
	if _, ok := tt.card(true); ok {
		t.Fatal("nested close appeared before500ms")
	}
	tt.Advance(time.Millisecond)
	if _, ok := tt.card(true); !ok || !tt.HasText("Close tab") {
		t.Fatal("nested close did not win at500ms")
	}
	if _, ok := tt.card(false); ok {
		t.Fatal("both parent and child cards rendered")
	}
	if err := tt.Click("Close tab " + file.Name); err != nil {
		t.Fatal(err)
	}
	if len(s.tabs) != 0 {
		t.Fatal("tooltip intercepted native close action")
	}
}

func TestTabTooltipKeyboardModalityIsGlobalAndNoWarmStart(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	file := tabTooltipFile(t, s, snapshot)
	tt := newTabTooltipTestWindow(s, func(c *ui.Context) {
		c.SetTheme(s.palette().UI())
		c.DisableAnimations()
		s.View(c)
	}, 640, 360)
	if active, _ := tt.TextInputActive(); !active {
		t.Fatal("production file editor did not receive initial focus")
	}
	chip, _ := tt.Find(file.Name)
	x, y := chip.X+3, chip.Y+chip.H/2
	tt.Move(x, y)
	tt.Advance(350 * time.Millisecond)
	requireTabTooltip(t, tt, true)
	before := s.documents[file.ID].editor.Buffer.String()
	tt.TypeKey(0, ui.KeyQ, "q")
	requireTabTooltip(t, tt, false)
	if s.documents[file.ID].editor.Buffer.String() == before {
		t.Fatal("focused production editor did not consume typed key")
	}
	tt.KeyUp(ui.KeyQ)
	tt.Advance(time.Second)
	requireTabTooltip(t, tt, false)
	tt.Move(x+1, y)
	tt.Advance(349 * time.Millisecond)
	requireTabTooltip(t, tt, false)
	tt.Advance(time.Millisecond)
	requireTabTooltip(t, tt, true)
}

func TestTabTooltipWaitingSurvivesTransientKeyboardPressAndWheel(t *testing.T) {
	for _, action := range []string{"keyboard-return", "quick-press", "wheel"} {
		t.Run(action, func(t *testing.T) {
			s, _, snapshot := testWorkspace(t)
			file := tabTooltipFile(t, s, snapshot)
			tt := tabTooltipStripTester(s, 360, false)
			chip, _ := tt.Find(file.Name)
			x, y := chip.X+3, chip.Y+chip.H/2
			tt.Move(x, y)
			tt.Advance(200 * time.Millisecond)
			switch action {
			case "keyboard-return":
				tt.KeyDown(ui.KeyF1)
				tt.KeyUp(ui.KeyF1)
				tt.Move(x+1, y)
			case "quick-press":
				tt.Press(x, y)
				tt.Release(x, y)
			case "wheel":
				tt.Wheel(x, y, 0, 1)
			}
			tt.Advance(150 * time.Millisecond)
			requireTabTooltip(t, tt, true)
		})
	}
}

func TestTabTooltipHeldAtDeadlineNeedsNewMove(t *testing.T) {
	for _, held := range []string{"keyboard", "pointer"} {
		t.Run(held, func(t *testing.T) {
			s, _, snapshot := testWorkspace(t)
			file := tabTooltipFile(t, s, snapshot)
			tt := tabTooltipStripTester(s, 360, false)
			chip, _ := tt.Find(file.Name)
			x, y := chip.X+3, chip.Y+chip.H/2
			tt.Move(x, y)
			tt.Advance(200 * time.Millisecond)
			if held == "keyboard" {
				tt.KeyDown(ui.KeyQ)
			} else {
				tt.Press(x, y)
			}
			tt.Advance(150 * time.Millisecond)
			requireTabTooltip(t, tt, false)
			if held == "keyboard" {
				tt.KeyUp(ui.KeyQ)
			} else {
				tt.Release(x, y)
			}
			tt.Advance(time.Second)
			requireTabTooltip(t, tt, false)
			tt.Move(x+1, y)
			tt.Advance(350 * time.Millisecond)
			requireTabTooltip(t, tt, true)
		})
	}
}

func TestTabTooltipVisibleWheelChecksTooltipBounds(t *testing.T) {
	for _, overCard := range []bool{false, true} {
		t.Run(fmt.Sprintf("over-card%t", overCard), func(t *testing.T) {
			s, _, snapshot := testWorkspace(t)
			file := tabTooltipFile(t, s, snapshot)
			tt := tabTooltipStripTester(s, 360, false)
			chip, _ := tt.Find(file.Name)
			x, y := chip.X+3, chip.Y+chip.H/2
			tt.Move(x, y)
			tt.Advance(350 * time.Millisecond)
			requireTabTooltip(t, tt, true)
			if overCard {
				x, y = x+3, y+3
				tt.Move(x, y)
			}
			tt.Wheel(x, y, 0, 1)
			requireTabTooltip(t, tt, overCard)
		})
	}
}

func TestTabTooltipStateFollowsSlotAndDropsMissingEligibility(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	first := tabTooltipFile(t, s, snapshot)
	var second contracts.File
	for _, file := range snapshot.Files {
		if file.ID != first.ID && file.Kind != "directory" && file.Kind != "image" && !file.ReadOnly {
			second = file
			s.OpenFile(file.ID)
			break
		}
	}
	if second.ID == "" {
		t.Fatal("fixture needs two editable files")
	}
	tt := tabTooltipStripTester(s, 360, false)
	chip, _ := tt.Find(first.Name)
	x, y := chip.X+3, chip.Y+chip.H/2
	tt.Move(x, y)
	tt.Advance(350 * time.Millisecond)
	s.tabs[0], s.tabs[1] = s.tabs[1], s.tabs[0]
	tt.Frame()
	requireTabTooltip(t, tt, true)
	if !tt.HasText(first.Path) || tt.HasText(second.Path) {
		t.Fatal("reordered eligible slot replaced source's captured tooltip")
	}
	tt.Move(340, 65)
	tt.Move(x, y)
	tt.Advance(350 * time.Millisecond)
	if !tt.HasText(second.Path) {
		t.Fatal("new hover did not use reordered slot's resource")
	}
	s.tabs[0] = surface{kind: "history", id: "without-detail", title: "History"}
	tt.Frame()
	requireTabTooltip(t, tt, false)
	s.tabs = nil
	tt.Frame()
	if _, ok := tt.card(true); ok {
		t.Fatal("removed slot retained its close card")
	}
}

func TestTabTooltipUnmountAndScopeDoNotReviveOldTasks(t *testing.T) {
	for _, change := range []string{"unmount", "scope"} {
		t.Run(change, func(t *testing.T) {
			s, _, snapshot := testWorkspace(t)
			file := tabTooltipFile(t, s, snapshot)
			mounted := true
			tt := newTabTooltipTestWindow(s, func(c *ui.Context) {
				c.SetTheme(s.palette().UI())
				ui.Column(c).Fill().Children(func() {
					if mounted {
						s.TabStrip(c, false)
					}
				})
			}, 360, 80)
			chip, _ := tt.Find(file.Name)
			x, y := chip.X+3, chip.Y+chip.H/2
			tt.Move(x, y)
			tt.Advance(350 * time.Millisecond)
			requireTabTooltip(t, tt, true)
			if change == "unmount" {
				mounted = false
				tt.Frame()
				mounted = true
			} else {
				// The existing application mount is keyed by scopeID. This
				// regression is not an assertion of GPUI's cross-scope identity.
				s.scopeID = "another-mounted-strip"
			}
			tt.Frame()
			tt.Advance(time.Second)
			requireTabTooltip(t, tt, false)
			tt.Move(x+1, y)
			tt.Advance(350 * time.Millisecond)
			requireTabTooltip(t, tt, true)
		})
	}
}

func TestTabTooltipRetainsCapturedBoundsAcrossOwnerMove(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	file := tabTooltipFile(t, s, snapshot)
	top := float32(0)
	tt := newTabTooltipTestWindow(s, func(c *ui.Context) {
		c.SetTheme(s.palette().UI())
		ui.Box(c).Absolute().Top(top).Width(360).Height(38).Children(func() { s.TabStrip(c, false) })
	}, 400, 180)
	chip, _ := tt.Find(file.Name)
	x, y := chip.X+3, chip.Y+chip.H/2
	tt.Move(x, y)
	tt.Advance(200 * time.Millisecond)
	top = 70
	tt.Frame()
	tt.Advance(150 * time.Millisecond)
	tip := requireTabTooltip(t, tt, true)
	if tip.X != x+1 || tip.Y != y+1 {
		t.Fatalf("task replaced its captured bounds or show point: %+v", tip)
	}
	tt.Move(390, 160)
	requireTabTooltip(t, tt, false)
}

func TestTabTooltipNaturalCardWidthDoesNotFollowWindowRoom(t *testing.T) {
	for _, width := range []int{220, 840} {
		for _, scale := range []float32{1, 2} {
			t.Run(fmt.Sprintf("width%d-scale%g", width, scale), func(t *testing.T) {
				s, _, snapshot := testWorkspace(t)
				file := tabTooltipFile(t, s, snapshot)
				s.documents[file.ID].file.Path = strings.Repeat("long-component-path/", 4) + "actual-file.txt"
				var committed ui.Rect
				tt := newTabTooltipTestWindow(s, func(c *ui.Context) {
					c.SetTheme(s.palette().UI())
					ui.Column(c).Fill().Children(func() { s.TabStrip(c, false) })
					if index := s.tabTooltip.shown; index >= 0 {
						committed = s.tabTooltip.owners[index].card.Bounds(c)
					}
				}, width, 120)
				tt.SetScale(scale)
				chip, _ := tt.Find(file.Name)
				tt.Move(chip.X+3, chip.Y+chip.H/2)
				tt.Advance(350 * time.Millisecond)
				requireTabTooltip(t, tt, true)
				// Read the public Handle again in the next build
				// to inspect the actual committed natural box.
				tt.Frame()
				if committed.W != 380 || committed.H <= 31 {
					t.Fatalf("natural max380 wrapped card was squeezed by window: %+v", committed)
				}
				if width == 220 && committed.X != 0 {
					t.Fatalf("oversized natural card did not keep width with origin0: %+v", committed)
				}
			})
		}
	}
}

func TestTabTooltipActualNaturalSizeFlipsAtWindowEdges(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("scale%g", scale), func(t *testing.T) {
			s, _, snapshot := testWorkspace(t)
			first := tabTooltipFile(t, s, snapshot)
			var file contracts.File
			for _, candidate := range snapshot.Files {
				if candidate.ID != first.ID && !candidate.ReadOnly && candidate.Kind != "directory" && candidate.Kind != "image" {
					file = candidate
					s.OpenFile(file.ID)
					break
				}
			}
			if file.ID == "" {
				t.Fatal("fixture needs a second editable file")
			}
			s.documents[file.ID].file.Path = strings.Repeat("long-component-path/", 4) + "actual-file.txt"
			var committed ui.Rect
			tt := newTabTooltipTestWindow(s, func(c *ui.Context) {
				c.SetTheme(s.palette().UI())
				ui.Box(c).Absolute().Left(400).Top(510).Width(400).Height(38).Children(func() { s.TabStrip(c, false) })
				if index := s.tabTooltip.shown; index >= 0 {
					committed = s.tabTooltip.owners[index].card.Bounds(c)
				}
			}, 840, 560)
			tt.SetScale(scale)
			chip, _ := tt.Find(file.Name)
			x, y := chip.X+chip.W/2, chip.Y+chip.H/2
			tt.Move(x, y)
			tt.Advance(350 * time.Millisecond)
			requireTabTooltip(t, tt, true)
			tt.Frame()
			if committed.W != 380 || committed.X != x-1-committed.W || committed.Y != y-1-committed.H {
				t.Fatalf("natural card did not flip against actual width/height: card%+v showpoint%g,%g", committed, x, y)
			}
		})
	}
}

func TestTabTooltipCardColorsAndScalableTypography(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, scale := range []float32{1, 2} {
			t.Run(fmt.Sprintf("dark%t-scale%g", dark, scale), func(t *testing.T) {
				s, _, snapshot := testWorkspace(t)
				snapshot.Settings.Appearance.Mode = "light"
				if dark {
					snapshot.Settings.Appearance.Mode = "dark"
				}
				snapshot.Settings.Appearance.ThemeID = ""
				s.Update(snapshot)
				file := tabTooltipFile(t, s, snapshot)
				fontSize := float32(13)
				tt := newTabTooltipTestWindow(s, func(c *ui.Context) {
					theme := s.palette().UI()
					theme.FontSize = fontSize
					c.SetTheme(theme)
					c.Root().Background(ui.RGB(70, 80, 90))
					ui.Column(c).Fill().Children(func() { s.TabStrip(c, false) })
				}, 400, 120)
				tt.SetScale(scale)
				chip, _ := tt.Find(file.Name)
				x, y := chip.X+3, chip.Y+chip.H/2
				tt.Move(x, y)
				tt.Advance(350 * time.Millisecond)
				tip := requireTabTooltip(t, tt, true)
				p := s.palette()
				fill := p.Input.Over(p.Background)
				inside := tt.Image().RGBAAt(int((tip.X+tip.W/2)*scale), int((tip.Y+4)*scale))
				want := color.RGBA{R: fill.R, G: fill.G, B: fill.B, A: fill.A}
				if inside != want {
					t.Fatalf("source opaque popover interior%+v want%+v", inside, want)
				}
				outside := tt.Image().RGBAAt(int((tip.X+tip.W+2)*scale), int((tip.Y+tip.H/2)*scale))
				if outside != (color.RGBA{R: 70, G: 80, B: 90, A: 255}) {
					t.Fatalf("opaque tooltip added an external shadow: %+v", outside)
				}
				fontSize = 20
				tt.Frame()
				if after := requireTabTooltip(t, tt, true); after.H <= tip.H {
					t.Fatalf("tooltip did not follow interface font: before%+v after%+v", tip, after)
				}
			})
		}
	}
}

func TestTabTooltipDoesNotCaptureUnderlyingPointerOrFocus(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	first := tabTooltipFile(t, s, snapshot)
	s.OpenBrowser("https://example.com/current")
	tt := tabTooltipStripTester(s, 360, false)
	chip, _ := tt.Find(first.Name)
	x, y := chip.X+3, chip.Y+chip.H/2
	tt.Move(x, y)
	tt.Advance(350 * time.Millisecond)
	tip := requireTabTooltip(t, tt, true)
	if tip.Y >= chip.Y+chip.H-1 {
		t.Fatal("test pointer does not overlap both tooltip and chip")
	}
	// This point overlaps the tooltip's pass-through layer and the first
	// real chip. The original tab's action and native focus must still run.
	tt.ClickAt(x+3, chip.Y+chip.H-2)
	if s.tabs[s.active].id != first.ID || !tt.Focused(first.Name) {
		t.Fatal("tooltip intercepted the underlying tab selection/focus")
	}
}

func TestTabTooltipReleasesAndModifiersDoNotChangeModality(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	file := tabTooltipFile(t, s, snapshot)
	tt := tabTooltipStripTester(s, 360, false)
	chip, _ := tt.Find(file.Name)
	tt.Move(chip.X+3, chip.Y+chip.H/2)
	tt.Advance(350 * time.Millisecond)
	tip := requireTabTooltip(t, tt, true)
	tt.KeyUp(ui.KeyF1)
	tt.HoldModifiers(ui.Shift)
	tt.Frame()
	if after := requireTabTooltip(t, tt, true); after != tip {
		t.Fatal("release or modifier-only input changed a mouse tooltip")
	}
	tt.HoldModifiers(0)
	if after := requireTabTooltip(t, tt, true); after != tip {
		t.Fatal("releasing modifiers changed a mouse tooltip")
	}
}

func TestTabTooltipHeldCloseDoesNotPressParentOwner(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	file := tabTooltipFile(t, s, snapshot)
	tt := tabTooltipStripTester(s, 360, false)
	close, _ := tt.Find("Close tab " + file.Name)
	x, y := close.X+close.W/2, close.Y+close.H/2
	tt.Move(x, y)
	tt.Advance(100 * time.Millisecond)
	tt.Press(x, y)
	tt.Advance(250 * time.Millisecond)
	requireTabTooltip(t, tt, true)
	if !tt.HasText(file.Path) {
		t.Fatal("child's stopped primary press incorrectly rejected parent task")
	}
	tt.Advance(150 * time.Millisecond)
	if _, ok := tt.card(true); ok {
		t.Fatal("pressed close owner displayed its own pending tooltip")
	}
	requireTabTooltip(t, tt, true)
	// Release outside the button, so this test does not close the document.
	tt.Move(340, 65)
	tt.Release(340, 65)
	if len(s.tabs) != 1 {
		t.Fatal("read-only input observation changed native release routing")
	}
}

func TestTabTooltipAnyButtonReleaseClearsOwnerPendingPress(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	file := tabTooltipFile(t, s, snapshot)
	tt := tabTooltipStripTester(s, 360, false)
	chip, _ := tt.Find(file.Name)
	x, y := chip.X+3, chip.Y+chip.H/2
	tt.Move(x, y)
	tt.Advance(100 * time.Millisecond)
	tt.Press(x, y)
	if !s.tabTooltip.owners[0].primaryDown {
		t.Fatal("native primary target was not observed")
	}
	tt.Advance(100 * time.Millisecond)
	// GPUI's capture MouseUp clears the owner's pending primary press
	// without checking which button was released. The actual primary button
	// is still down here; the secondary click must not change that fact.
	tt.RightClickAt(x, y)
	tt.CloseMenu()
	if !s.tabTooltip.activity.PrimaryDown {
		t.Fatal("secondary release changed the actual primary held state")
	}
	if s.tabTooltip.owners[0].primaryDown {
		t.Fatal("secondary MouseUp retained source owner pending press")
	}
	tt.Advance(150 * time.Millisecond)
	requireTabTooltip(t, tt, true)
	if !tt.HasText(file.Path) {
		t.Fatal("any-button release restarted or rejected the original350ms task")
	}
	tt.Move(340, 65)
	tt.Release(340, 65)
}

func TestTabTooltipBlurRetainsSlotUntilUpOrUnheldMove(t *testing.T) {
	for _, secondPress := range []bool{false, true} {
		t.Run(fmt.Sprintf("other-press%t", secondPress), func(t *testing.T) {
			s, _, snapshot := testWorkspace(t)
			file := tabTooltipFile(t, s, snapshot)
			tt := newTabTooltipTestWindow(s, func(c *ui.Context) {
				c.SetTheme(s.palette().UI())
				ui.Column(c).Fill().Children(func() {
					s.TabStrip(c, false)
					ui.ButtonBase(c).Key("other-press-owner").Label("Other press owner").Size(120, 24)
				})
			}, 360, 100)
			chip, _ := tt.Find(file.Name)
			x, y := chip.X+3, chip.Y+chip.H/2
			tt.Move(x, y)
			tt.Advance(100 * time.Millisecond)
			tt.Press(x, y)
			tt.SetFocused(false)
			tt.SetFocused(true)
			if secondPress {
				other, _ := tt.Find("Other press owner")
				tt.Press(other.X+3, other.Y+other.H/2)
			}
			// Source does not clear an already captured slot press on blur.
			// A later primary Down elsewhere is not a MouseUp or MouseMove.
			if !s.tabTooltip.owners[0].primaryDown {
				t.Fatal("blur incorrectly erased the source slot's pending press")
			}
			tt.Advance(250 * time.Millisecond)
			requireTabTooltip(t, tt, false)
			if secondPress {
				tt.Release(340, 90)
				if s.tabTooltip.owners[0].primaryDown {
					t.Fatal("any MouseUp failed to clear the captured slot")
				}
			}
			tt.Move(x+1, y)
			if s.tabTooltip.owners[0].primaryDown {
				t.Fatal("real unheld MouseMove failed to clear the captured slot")
			}
			if !secondPress {
				// This first unheld move only clears the old pending state.
				tt.Advance(time.Second)
				requireTabTooltip(t, tt, false)
				tt.Move(x+2, y)
			}
			tt.Advance(349 * time.Millisecond)
			requireTabTooltip(t, tt, false)
			tt.Advance(time.Millisecond)
			requireTabTooltip(t, tt, true)
		})
	}
}

func TestTabTooltipUnheldMoveClearsPendingBeforeNextHoverTask(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	file := tabTooltipFile(t, s, snapshot)
	tt := tabTooltipStripTester(s, 360, false)
	chip, _ := tt.Find(file.Name)
	x, y := chip.X+3, chip.Y+chip.H/2
	tt.Move(x, y)
	tt.Advance(100 * time.Millisecond)
	tt.Press(x, y)
	tt.SetFocused(false)
	tt.SetFocused(true)
	tt.Advance(100 * time.Millisecond)
	tt.Move(x+1, y)
	// TooltipMove sees the captured owner press first and cancels its task;
	// the later generic Move handler clears pending. That same event cannot
	// retain the old deadline or start another hover task.
	tt.Advance(150 * time.Millisecond)
	requireTabTooltip(t, tt, false)
	tt.Advance(time.Second)
	requireTabTooltip(t, tt, false)
	tt.Move(x+2, y)
	tt.Advance(349 * time.Millisecond)
	requireTabTooltip(t, tt, false)
	tt.Advance(time.Millisecond)
	requireTabTooltip(t, tt, true)
}

func TestTabTooltipMoveUsesActualButtonWhenKnown(t *testing.T) {
	for _, test := range []struct {
		name    string
		known   bool
		button  int
		history bool
		primary bool
	}{
		{"known-primary-missing-down", true, 0, false, true},
		{"known-primary-held", true, 0, true, true},
		{"known-secondary-stale-primary", true, 1, true, false},
		{"known-middle-stale-primary", true, 2, true, false},
		{"known-x1-stale-primary", true, 3, true, false},
		{"known-x2-stale-primary", true, 4, true, false},
		{"known-none-missing-up", true, -1, true, false},
		{"unknown-primary-history", false, -1, true, true},
		{"unknown-no-primary-history", false, 0, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			activity := ui.InputActivitySnapshot{MoveButtonKnown: test.known, MoveButton: test.button, PrimaryDown: test.history}
			if got := tabTooltipMoveHasPrimary(activity); got != test.primary {
				t.Fatalf("move's primary fact=%t want%t, snapshot%+v", got, test.primary, activity)
			}
		})
	}
}

func TestTabTooltipOnlyShownOwnerGetsHoveredCardWheelException(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	file := tabTooltipFile(t, s, snapshot)
	tt := tabTooltipStripTester(s, 360, false)
	close, _ := tt.Find("Close tab " + file.Name)
	x, y := close.X+close.W/2, close.Y+close.H/2
	tt.Move(x, y)
	tt.Advance(500 * time.Millisecond)
	card, ok := tt.card(true)
	if !ok {
		t.Fatal("nested close tooltip missing")
	}
	// The close card overlaps a small part of its source close button.
	// Both tasks are logically visible, but only the child's actual card
	// owns TooltipId.is_hovered and the wheel clear exception in GPUI.
	x, y = card.X+1, card.Y+1
	if x >= close.X+close.W || y >= close.Y+close.H {
		t.Fatal("test needs the close card/owner overlap")
	}
	tt.Move(x, y)
	tt.Wheel(x, y, 0, 1)
	if _, ok := tt.card(true); !ok {
		t.Fatal("shown child card did not receive its own hover exception")
	}
	chip, _ := tt.Find(file.Name)
	tt.Move(chip.X+3, chip.Y+chip.H/2)
	requireTabTooltip(t, tt, false)
	tt.Advance(349 * time.Millisecond)
	requireTabTooltip(t, tt, false)
	tt.Advance(time.Millisecond)
	requireTabTooltip(t, tt, true)
}

func TestTabTooltipObserverPreservesNativeContextMenu(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	file := tabTooltipFile(t, s, snapshot)
	s.OpenBrowser("https://example.com/current")
	tt := tabTooltipStripTester(s, 360, false)
	chip, _ := tt.Find(file.Name)
	x, y := chip.X+3, chip.Y+chip.H/2
	tt.Move(x, y)
	tt.Advance(350 * time.Millisecond)
	requireTabTooltip(t, tt, true)
	tt.RightClickAt(x, y)
	if got := tt.Menu(); len(got) != 2 || got[0] != "Close tab" || got[1] != "Close other tabs" {
		t.Fatalf("read-only observer changed native context menu: %v", got)
	}
	if err := tt.ChooseMenuItem("Close tab"); err != nil {
		t.Fatal(err)
	}
	if len(s.tabs) != 1 || s.tabs[0].kind != "browser" || s.documents[file.ID] != nil {
		t.Fatal("native context action did not close only its target file")
	}
}

func TestTabTooltipDoesNotReplaceAncestorInputHandler(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	file := tabTooltipFile(t, s, snapshot)
	moves := 0
	tt := newTabTooltipTestWindow(s, func(c *ui.Context) {
		c.SetTheme(s.palette().UI())
		c.Root().HandleInput(func(event ui.InputEvent) bool {
			if event.Kind == ui.InputPointerMove {
				moves++
			}
			return false
		})
		ui.Column(c).Fill().Children(func() { s.TabStrip(c, false) })
	}, 360, 80)
	chip, _ := tt.Find(file.Name)
	x, y := chip.X+3, chip.Y+chip.H/2
	tt.Move(x, y)
	if moves != 1 {
		t.Fatalf("tooltip replaced existing ancestor input routing: moves%d want1", moves)
	}
	tt.Advance(350 * time.Millisecond)
	requireTabTooltip(t, tt, true)
	tt.Move(x+1, y)
	if moves != 2 {
		t.Fatal("visible tooltip replaced existing ancestor input routing")
	}
}
