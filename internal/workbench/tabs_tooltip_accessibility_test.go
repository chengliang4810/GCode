package workbench

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestTabTooltipHasContentWithoutGenericLabel(t *testing.T) {
	for _, close := range []bool{false, true} {
		t.Run(fmt.Sprintf("close%t", close), func(t *testing.T) {
			s, _, snapshot := testWorkspace(t)
			file := tabTooltipFile(t, s, snapshot)
			tt := tabTooltipStripTester(s, 360, false)
			name, content := file.Name, file.Path
			delay := 350 * time.Millisecond
			if close {
				name, content, delay = "Close tab "+file.Name, "Close tab", 500*time.Millisecond
			}
			anchor, ok := tt.Find(name)
			if !ok {
				t.Fatal("tooltip anchor missing")
			}
			x := anchor.X + 3
			if close {
				x = anchor.X + anchor.W/2
			}
			tt.Move(x, anchor.Y+anchor.H/2)
			tt.Advance(delay)
			if _, ok := tt.card(close); !ok || !tt.HasText(content) {
				t.Fatalf("actual tooltip content missing: %v", tt.Texts())
			}
			for _, generic := range []string{"Workspace tab detail", "Close tab tooltip"} {
				if tt.HasText(generic) {
					t.Errorf("generic label still overrides the actual tooltip content: %q", generic)
				}
			}
		})
	}
}

func TestTabTooltipCardHandleDoesNotSelectSameNamedAnchor(t *testing.T) {
	s, _, _ := testWorkspace(t)
	owner := &tabTooltipOwner{phase: tabTooltipVisible, text: "Tab02.txt", x: 200, y: 20}
	s.tabTooltip.owners = []*tabTooltipOwner{owner}
	tt := newTabTooltipTestWindow(s, func(c *ui.Context) {
		c.SetTheme(s.palette().UI())
		ui.Text(c, owner.text).Absolute().Left(5).Top(100)
		s.renderTabTooltip(c, tabTooltipPass{visible: owner})
	}, 500, 180)
	anchor, ok := tt.Find(owner.text)
	if !ok || anchor.X != 5 || anchor.Y != 100 {
		t.Fatalf("test needs the earlier same-named anchor: %+v exists%t", anchor, ok)
	}
	card, ok := tt.card(false)
	if !ok || card.X != 201 || card.Y != 21 || card == anchor {
		t.Fatalf("durable card identity selected same-named anchor: card%+v anchor%+v exists%t", card, anchor, ok)
	}
}
