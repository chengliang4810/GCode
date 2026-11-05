package desktop

import (
	"fmt"
	"math"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestActivityManualFoldsKeepTailStableAndRestoreStreamFollowing(t *testing.T) {
	for _, test := range []struct {
		name    string
		width   int
		dark    bool
		reduced bool
	}{
		{name: "light", width: 760},
		{name: "narrow-dark", width: 420, dark: true},
		{name: "reduced", width: 760, reduced: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := testApp(t, "default")
			a.palette = design.Palette("rcode", test.dark)
			a.state.Settings.Appearance.MotionPreference = "off"
			a.state.Settings.Appearance.ReduceMotion = test.reduced
			message := activityFixture()
			reply := message.Blocks[len(message.Blocks)-1].Text
			list := ui.ListState{FollowEnd: true, Key: func(i int) any { return i }}
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(a.palette.UI())
				a.configureMotion(c)
				a.prepareTranscriptMarkdown(c)
				root := ui.Column(c).Fill()
				a.transcriptListScope(c, root, &list, "root", func() {
					ui.List(c, &list, 9, func(i int) {
						if i < 8 {
							ui.Text(c, fmt.Sprintf("Earlier message %d", i)).Height(90)
						} else {
							a.transcriptMessageForTest(c, message)
						}
					}).Fill().Padding(24)
				})
			}, test.width, 600)
			tt.Advance(time.Second)
			position, ok := tt.Find(reply)
			if !ok {
				t.Fatal("tail is missing")
			}
			summary := "Activity: " + contracts.SummarizeActivity(message.Blocks[:len(message.Blocks)-1]).Label()
			for _, control := range []string{summary, "Thought process: thought-1", "Thought process: thought-1", summary} {
				if err := tt.Click(control); err != nil {
					t.Fatal(err)
				}
				tt.Move(float32(test.width-5), 595)
				for step := 0; step < 32; step++ {
					current, exists := tt.Find(reply)
					if !exists || math.Abs(float64(current.Y-position.Y)) > .1 {
						t.Fatalf("%s moved tail during step %d: y=%v want=%v visible=%t", control, step, current.Y, position.Y, exists)
					}
					tt.Advance(10 * time.Millisecond)
				}
				if test.reduced {
					if list.FollowAnimation != nil {
						t.Fatal("reduced motion enabled a follow animation")
					}
				} else if list.FollowAnimation != &ui.Smooth {
					t.Fatal("manual fold disabled subsequent normal stream following")
				}
			}
		})
	}
}

func TestActivityFoldMotionBelongsToOwningPaneAndHandlesReverseClicks(t *testing.T) {
	a := testApp(t, "default")
	a.state.Settings.Appearance.MotionPreference = "off"
	a.state.Settings.Appearance.ReduceMotion = false
	mainList, sideList := ui.ListState{}, ui.ListState{}
	mainOpen, sideOpen := false, false
	tt := ui.NewTester(func(c *ui.Context) {
		a.configureMotion(c)
		ui.Row(c).Fill().Children(func() {
			for _, pane := range []struct {
				id   string
				list *ui.ListState
				open *bool
			}{{"main", &mainList, &mainOpen}, {"side", &sideList, &sideOpen}} {
				root := ui.Column(c.Key(pane.id)).Grow(1)
				a.transcriptListScope(c, root, pane.list, pane.id, func() {
					parts := ui.CollapsibleBaseWithAnimation(c, pane.open, activityFoldAnimation)
					parts.Trigger.FillWidth().Height(activityHeaderHeight).Label(pane.id).OnClick(a.manualActivityFold(c, pane.id, pane.open, ""))
					parts.Trigger.Children(func() { ui.Text(c, pane.id) })
					parts.Panel(func() { ui.Text(c, pane.id+" body").Height(160) })
				})
			}
		})
	}, 760, 500)
	if err := tt.Click("main"); err != nil {
		t.Fatal(err)
	}
	if mainList.FollowAnimation != nil || sideList.FollowAnimation != &ui.Smooth {
		t.Fatalf("main fold suppression crossed into the side pane: main=%p side=%p smooth=%p opens=%t/%t", mainList.FollowAnimation, sideList.FollowAnimation, &ui.Smooth, mainOpen, sideOpen)
	}
	tt.Advance(45 * time.Millisecond)
	if err := tt.Click("main"); err != nil {
		t.Fatal(err)
	}
	if mainOpen {
		t.Fatal("reverse click did not close the panel")
	}
	if mainList.FollowAnimation != nil {
		t.Fatal("reverse click prematurely restored follow glide")
	}
	tt.Move(750, 490)
	tt.Advance(activityFoldDuration + 40*time.Millisecond)
	if tt.HasText("main body") || mainList.FollowAnimation != &ui.Smooth || sideOpen {
		t.Fatal("reverse fold did not finish or modified the other pane")
	}
	tt.Advance(time.Second)
	if tt.FramePending() {
		t.Fatal("settled manual fold kept requesting frames")
	}
}

func TestActivityConnectorParts(t *testing.T) {
	incoming, branch := activityConnectorParts(0, false)
	if incoming != 0 || branch != 0 {
		t.Fatalf("first arrival at rest draws %v/%v", incoming, branch)
	}
	incoming, _ = activityConnectorParts(0.62, false)
	if incoming != 1 {
		t.Fatalf("first arrival trunk %v, want complete at 0.62", incoming)
	}
	incoming, branch = activityConnectorParts(0.45, true)
	if incoming != 0 {
		t.Fatalf("later arrival trunk %v, want waiting at 0.45", incoming)
	}
	incoming, branch = activityConnectorParts(1, true)
	if incoming != 1 || branch != 1 {
		t.Fatalf("later arrival at rest draws %v/%v", incoming, branch)
	}
	if got := activityContinuation(0.45); got != 1 {
		t.Fatalf("continuation %v, want complete at the next row's 0.45", got)
	}
	if got := activityContinuation(0); got != 0 {
		t.Fatalf("continuation %v, want none while the next row waits", got)
	}
}

func TestActivityShimmerAmount(t *testing.T) {
	if got := activityShimmerAmount(0.5, 0.5); got != 1 {
		t.Fatalf("peak %v, want 1 at the sweep center", got)
	}
	if got := activityShimmerAmount(0, 0); got != 0 {
		t.Fatalf("leading edge %v, want dark before the sweep arrives", got)
	}
	if got := activityShimmerAmount(0.68, 0.5); got <= 0 || got >= 1 {
		t.Fatalf("shoulder %v, want part of the triangular peak", got)
	}
	if got := activityShimmerAmount(10, 0.5); got != 0 {
		t.Fatalf("beyond the copies %v, want dark", got)
	}
}

func TestActivityBranchPointsTruncateByDistance(t *testing.T) {
	full := activityBranchPoints(1)
	if len(full) != 26 {
		t.Fatalf("full branch has %d points, want the 24-step bend plus its tip", len(full))
	}
	previous := activityBranchPoints(0.2)
	half := activityBranchPoints(0.5)
	last := func(points [][2]float32) float32 {
		length := float32(0)
		for i := 1; i < len(points); i++ {
			dx, dy := points[i][0]-points[i-1][0], points[i][1]-points[i-1][1]
			length += float32(math.Hypot(float64(dx), float64(dy)))
		}
		return length
	}
	total := last(full)
	if last(previous) >= last(half) || last(half) >= total {
		t.Fatalf("drawn lengths %v/%v/%v, want increasing with progress", last(previous), last(half), total)
	}
}
