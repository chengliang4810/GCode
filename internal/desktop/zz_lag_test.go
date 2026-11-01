package desktop

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestZZDragPasses(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession(a.state.Sessions[0].ID)
	var views int
	tt := ui.NewTester(func(c *ui.Context) { views++; a.View(c) }, 1320, 880)
	tt.Frame()
	divider, _ := tt.Find("Divider")
	start := divider.X
	tt.Press(start, 300)
	defer tt.Release(start, 300)
	for _, dx := range []float32{5, 10, 20, 40, 20, 0, -20, -40} {
		views = 0
		tt.Move(start+dx, 300)
		tt.Frame()
		d, _ := tt.Find("Divider")
		t.Logf("dx=%5.0f views=%d divider=%6.1f expected=%6.1f sidebar=%g", dx, views, d.X, start+dx, a.sidebarWidth)
	}
}
