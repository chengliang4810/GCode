package ui

import (
	"slices"
	"testing"
)

func TestInlineLinkDragSelectsOriginalLabelWithoutClick(t *testing.T) {
	for _, shared := range []bool{false, true} {
		t.Run(map[bool]string{false: "paragraph", true: "shared"}[shared], func(t *testing.T) {
			clicks := 0
			var link *node
			tt := coreNewTester(func(c *context) {
				column := coreColumn(c).Padding(12)
				if shared {
					column.Selectable()
				}
				column.Children(func() {
					paragraph := coreRichText(c)
					if !shared {
						paragraph.Selectable()
					}
					paragraph.Children(func() {
						coreText(c, "Read ")
						link = coreRichText(c, Span{Text: "the original label"}).Role(RoleLink).Focusable()
						if link.Clicked() {
							clicks++
						}
						coreText(c, " carefully.")
					})
					coreText(c, "Next paragraph.")
				})
			}, 500, 180)
			first := link.frags[0]
			tt.Press(first.X+1, first.Y+first.H/2)
			tt.Move(first.X+first.W-1, first.Y+first.H/2)
			tt.Release(first.X+first.W-1, first.Y+first.H/2)
			tt.Key(Cmd, KeyC)
			if clicks != 0 || tt.Clipboard() != "the original label" {
				t.Fatalf("clicks=%d copied=%q", clicks, tt.Clipboard())
			}
		})
	}
}

func TestInlineLinkKeepsClickKeysAndMenu(t *testing.T) {
	clicks := 0
	var link *node
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Selectable().Children(func() {
			coreRichText(c).Children(func() {
				coreText(c, "Open ")
				link = coreRichText(c, Span{Text: "guide"}).Role(RoleLink).Focusable().
					ContextMenu(func(menu *Menu) { menu.Item("Copy link") })
				if link.Clicked() {
					clicks++
				}
			})
		})
	}, 320, 140)
	tt.ClickAt(center(link.frags[0]))
	tt.Key(0, KeyEnter)
	tt.Key(0, KeySpace)
	if clicks != 3 {
		t.Fatalf("pointer and keyboard clicks=%d", clicks)
	}
	tt.Key(Shift, KeyF10)
	if !slices.Contains(tt.Menu(), "Copy link") {
		t.Fatalf("context menu=%v", tt.Menu())
	}
}

func TestInlineLinkDragSelectsAcrossParagraphs(t *testing.T) {
	clicks := 0
	var link *node
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Selectable().Gap(10).Children(func() {
			coreRichText(c).Children(func() {
				coreText(c, "Start ")
				link = coreRichText(c, Span{Text: "original label"}).Role(RoleLink).Focusable()
				if link.Clicked() {
					clicks++
				}
			})
			coreText(c, "Next paragraph.")
		})
	}, 500, 200)
	first := link.frags[0]
	x, y := selectableAt(t, tt, "Next paragraph.", 4)
	tt.Press(first.X+1, first.Y+first.H/2)
	tt.Move(x, y)
	tt.Release(x, y)
	tt.Key(Cmd, KeyC)
	if clicks != 0 || tt.Clipboard() != "original label\nNext" {
		t.Fatalf("clicks=%d copied=%q", clicks, tt.Clipboard())
	}
}
