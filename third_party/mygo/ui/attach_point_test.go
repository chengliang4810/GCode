package ui_test

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestAttachAtPointFitsCurrentMeasuredBox(t *testing.T) {
	for _, test := range []struct {
		name          string
		ww, wh        int
		x, y, gap     float32
		width, height float32
		want          ui.Rect
	}{
		{"interior", 300, 200, 100, 50, 1, 80, 30, ui.Rect{X: 101, Y: 51, W: 80, H: 30}},
		{"overflow flips each axis", 300, 200, 280, 170, 1, 100, 60, ui.Rect{X: 179, Y: 109, W: 100, H: 60}},
		{"exact edge does not flip", 300, 200, 219, 169, 1, 80, 30, ui.Rect{X: 220, Y: 170, W: 80, H: 30}},
		{"one pixel beyond edge flips", 300, 200, 220, 170, 1, 80, 30, ui.Rect{X: 139, Y: 139, W: 80, H: 30}},
		{"fractional point and dimensions", 300, 200, 279.25, 169.5, 0.5, 100.5, 60.25, ui.Rect{X: 178.25, Y: 108.75, W: 100.5, H: 60.25}},
		{"oversized remains natural size", 100, 80, 80, 20, 1, 150, 120, ui.Rect{W: 150, H: 120}},
		{"no implicit edge inset", 300, 200, 0, 0, 1, 20, 20, ui.Rect{X: 1, Y: 1, W: 20, H: 20}},
		{"negative point clamps to zero", 300, 200, -8, -6, 1, 20, 20, ui.Rect{W: 20, H: 20}},
		{"negative gap is zero", 300, 200, 0, 0, -5, 20, 20, ui.Rect{W: 20, H: 20}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var handle ui.Handle
			var committed ui.Rect
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Overlay(c, func() {
					ui.Box(c).Size(test.width, test.height).AttachAtPoint(test.x, test.y, test.gap).Label("point panel").Bind(&handle)
				})
				committed = handle.Bounds(c)
			}, test.ww, test.wh)
			snapWantCommittedRect(t, tt, "point panel", &committed, test.want)
			tt.Frame()
			snapWantCommittedRect(t, tt, "point panel", &committed, test.want)
		})
	}
}

func TestAttachAtPointUsesNewAutoHeightAndPlacesChildrenAfterFlip(t *testing.T) {
	rows, x, y := 2, float32(120), float32(110)
	var panel, child ui.Handle
	var committed ui.Rect
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Overlay(c, func() {
			ui.Column(c).Width(100).Padding(4).BorderWidth(2).AttachAtPoint(x, y, 1).Label("point panel").Bind(&panel).Children(func() {
				for range rows {
					ui.Box(c).Size(20, 20).Shrink(0)
				}
				ui.Box(c).Size(10, 8).Absolute().Left(5).Top(3).Label("point child").Bind(&child)
			})
		})
		committed = panel.Bounds(c)
	}, 300, 200)
	snapWantCommittedRect(t, tt, "point panel", &committed, ui.Rect{X: 121, Y: 111, W: 100, H: 52})
	rows = 5
	tt.Frame()
	snapWantCommittedRect(t, tt, "point panel", &committed, ui.Rect{X: 121, W: 100, H: 112})
	snapWantRect(t, tt, "point child", ui.Rect{X: 128, Y: 5, W: 10, H: 8})
	rows, x, y = 1, 260, 180
	tt.Frame()
	snapWantCommittedRect(t, tt, "point panel", &committed, ui.Rect{X: 159, Y: 147, W: 100, H: 32})
	tt.SetSize(300, 80)
	tt.Frame()
	snapWantCommittedRect(t, tt, "point panel", &committed, ui.Rect{X: 159, Y: 147, W: 100, H: 32})
}

func TestAttachAtPointUsesWindowCoordinatesWithinParentAndMargins(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Absolute().Left(80).Top(70).Size(100, 90).Children(func() {
			ui.Box(c).Size(60, 30).Margin(3, 5, 7, 11).AttachAtPoint(260, 180, 1).Label("point panel")
		})
	}, 300, 200)
	// The margin box is76x40 and flips before the window point; its border
	// begins eleven right/three down from that origin, regardless of parent.
	snapWantRect(t, tt, "point panel", ui.Rect{X: 194, Y: 142, W: 60, H: 30})
}

func TestAttachAtPointCanSwitchBackToLegacyAttachmentAndReopen(t *testing.T) {
	point, open := true, true
	tt := ui.NewTester(func(c *ui.Context) {
		anchor := ui.Box(c).Absolute().Left(100).Top(90).Size(20, 20)
		if !open {
			return
		}
		ui.Overlay(c, func() {
			c.Key("point panel")
			panel := ui.Box(c).Size(40, 20).AttachAtPoint(200, 150, 1).Label("point panel")
			if !point {
				panel.AttachTo(anchor, ui.AnchorBottomLeft, ui.AnchorTopLeft)
			}
		})
	}, 300, 200)
	snapWantRect(t, tt, "point panel", ui.Rect{X: 201, Y: 151, W: 40, H: 20})
	point = false
	tt.Frame()
	snapWantRect(t, tt, "point panel", ui.Rect{X: 100, Y: 110, W: 40, H: 20})
	open = false
	tt.Frame()
	if _, found := tt.Find("point panel"); found {
		t.Fatal("closed point panel retained an element")
	}
	open, point = true, true
	tt.Frame()
	snapWantRect(t, tt, "point panel", ui.Rect{X: 201, Y: 151, W: 40, H: 20})
}

func TestAttachAtPointRemainsPhysicalAcrossScaleAndDirection(t *testing.T) {
	for _, scale := range []float32{1, 1.25, 2} {
		var handle ui.Handle
		var committed ui.Rect
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Overlay(c, func() {
				ui.Box(c).Size(100.5, 60.25).AttachAtPoint(279.25, 169.5, .5).Label("point panel").Bind(&handle)
			})
			committed = handle.Bounds(c)
		}, 300, 200)
		tt.SetScale(scale)
		want := ui.Rect{X: 178.25, Y: 108.75, W: 100.5, H: 60.25}
		snapWantCommittedRect(t, tt, "point panel", &committed, want)
		tt.SetDirection(ui.RightToLeft)
		snapWantCommittedRect(t, tt, "point panel", &committed, want)
	}
}

func TestAttachAtPointRemainsAtWindowPointWhenFixedParentMoves(t *testing.T) {
	spacer := float32(20)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Children(func() {
			ui.Box(c).Size(100, spacer).Shrink(0)
			ui.Box(c).Size(100, 100).Shrink(0).Children(func() {
				ui.Box(c).Size(20, 20).AttachAtPoint(150, 150, 1).Label("point panel")
			})
		})
	}, 300, 300)
	snapWantRect(t, tt, "point panel", ui.Rect{X: 151, Y: 151, W: 20, H: 20})
	spacer = 70
	tt.Frame()
	snapWantRect(t, tt, "point panel", ui.Rect{X: 151, Y: 151, W: 20, H: 20})
}

func TestAttachAtPointRefitsOnViewportResizeWithinFixedParent(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Size(100, 100).Children(func() {
			ui.Box(c).Size(100, 40).AttachAtPoint(150, 80, 1).Label("point panel")
		})
	}, 300, 200)
	snapWantRect(t, tt, "point panel", ui.Rect{X: 151, Y: 81, W: 100, H: 40})
	tt.SetSize(200, 100)
	snapWantRect(t, tt, "point panel", ui.Rect{X: 49, Y: 39, W: 100, H: 40})
}

func TestAttachAtPointWorksInTooltipBaseCallback(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		anchor := ui.ButtonBase(c).Absolute().Left(20).Top(20).Size(40, 20).Label("point anchor")
		ui.TooltipBase(c, anchor, func(tip ui.Element) {
			tip.Size(80, 30).AttachAtPoint(150, 150, 1).Label("point tooltip")
		})
	}, 300, 220)
	tt.Move(30, 30)
	tt.Advance(700 * time.Millisecond)
	snapWantRect(t, tt, "point tooltip", ui.Rect{X: 151, Y: 151, W: 80, H: 30})
}
