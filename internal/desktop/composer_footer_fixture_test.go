//go:build uifixture

package desktop

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestFooterFixtureMatrixUsesProductionComponentsAndSettles(t *testing.T) {
	fixture, err := NewFooterUsageFixture("dark", "known")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fixture.Close)
	tt := ui.NewTester(fixture.View, 840, 560)
	scenes := []string{"known", "unknown", "capacity", "zero", "over", "warning", "danger", "known-accounts", "known-context", "capacity-context", "zero-context", "over-context", "warning-context", "danger-context"}
	for _, appearance := range []string{"dark", "light"} {
		for _, scene := range scenes {
			if err := fixture.SetScene(appearance, scene); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			tt.Advance(time.Second)
			state := fixture.State()
			if !state.GeometryStable || state.LayoutError != "" || state.Scene != scene || state.Appearance != appearance || state.Row.X != 24 || state.Row.Y != 512 || state.Row.Width != 792 || state.Row.Height != 24 {
				t.Fatalf("scene %s/%s did not settle: %+v", appearance, scene, state)
			}
			if state.ContextOpen != strings.HasSuffix(scene, "-context") || state.AccountsOpen != strings.HasSuffix(scene, "-accounts") {
				t.Fatalf("wrong card state %+v", state)
			}
			if strings.HasSuffix(scene, "-context") && !tt.HasText("Context window menu") {
				t.Fatal("fixture substituted a different context card")
			}
			if strings.HasSuffix(scene, "-accounts") && !tt.HasText("Second account") {
				t.Fatal("fixture substituted a different account card")
			}
		}
	}
	state := fixture.State()
	state.Accounts[0].Usage[0].UsedPercent = 100
	*state.Context.Tokens = 999
	if got := fixture.State(); got.Accounts[0].Usage[0].UsedPercent != 20 || *got.Context.Tokens != 180000 {
		t.Fatal("fixture State leaked mutable UI data")
	}
}
