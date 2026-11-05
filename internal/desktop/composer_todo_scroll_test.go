package desktop

import (
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestTodoScrollFadesActualPixelsAtOverflowEdges(t *testing.T) {
	var scroll ui.ScrollState
	contentHeight := float32(300)
	tt := ui.NewTester(func(c *ui.Context) {
		c.DisableAnimations()
		c.Root().Background(ui.RGB(0, 0, 255))
		controlTodoScroll(c.Key("scroll"), &scroll, 100).Children(func() {
			ui.Box(c).FillWidth().Height(contentHeight).Shrink(0).Background(ui.RGB(255, 0, 0))
		})
	}, 100, 100)
	tt.Frame()
	red := func(y int) uint32 {
		r, _, _, _ := tt.Image().At(20, y).RGBA()
		return r
	}
	check := func(top, bottom bool) {
		t.Helper()
		if red(50) < 60000 {
			t.Fatal("fade dimmed the center of the scroll viewport")
		}
		if (red(1) < 10000) != top || (red(98) < 10000) != bottom {
			t.Fatalf("wrong overflow fades: Y=%g MaxY=%g top=%d bottom=%d", scroll.Y, scroll.MaxY, red(1), red(98))
		}
	}
	check(false, true)
	scroll.Y = 100
	tt.Frame()
	check(true, true)
	scroll.Y = scroll.MaxY
	tt.Frame()
	check(true, false)
	contentHeight = 100
	tt.Frame()
	check(false, false)
}

func TestTodoScrollSurvivesCollapseAndConversationNavigation(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	session := a.currentSession()
	a.state.Messages[session.ID] = []contracts.Message{{ID: "write", Role: "assistant", Blocks: []contracts.Block{
		{ID: "todos", Kind: "todos", Todos: testTodos(60, 30)},
	}}}
	a.state.Revision++
	state := a.todoPanel("main", session.ID)
	state.expanded, state.earlier, state.later = true, true, true
	visible := true
	tt := ui.NewTester(func(c *ui.Context) {
		c.DisableAnimations()
		if visible {
			ui.Column(c).Fill().Justify(ui.End).Children(func() { a.todoTray(c, session, "main") })
		}
	}, 600, 600)
	tt.Frame()
	tt.Frame()
	state.scroll.Y = 180
	tt.Frame()
	offset := state.scroll.Y
	if offset <= 0 {
		t.Fatal("fixture did not overflow")
	}
	if err := tt.Click("Collapse todos"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Expand todos"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if state.scroll.Y != offset {
		t.Fatal("collapsing discarded the reading position")
	}
	visible = false
	tt.Frame()
	visible = true
	tt.Frame()
	if state.scroll.Y != offset {
		t.Fatal("conversation navigation discarded the reading position")
	}
}
