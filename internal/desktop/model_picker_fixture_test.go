//go:build uifixture

package desktop

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestModelPickerFixtureProductionMatrix(t *testing.T) {
	for _, appearance := range []string{"dark", "light"} {
		for _, width := range []int{840, 360} {
			t.Run(appearance+"/"+strconv.Itoa(width), func(t *testing.T) {
				fixture, err := NewModelPickerFixture(appearance)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(fixture.Close)
				tt := ui.NewTester(fixture.View, width, 560)
				tt.Advance(time.Second)
				state := fixture.State()
				if !state.PopoverOpen || state.Appearance != appearance || state.Width != float32(width) || state.Height != 560 || !state.Geometry.Ready() || !state.GeometryStable || state.LayoutError != "" {
					t.Fatalf("controlled scene: %+v", state)
				}
				panel, ok := tt.Find("Model picker")
				anchor, _ := tt.Find("Select model")
				if !ok || panel.W <= 0 || panel.W > min(modelPickerMaxWidth, float32(width-16))+.5 || panel.X < 0 || panel.X+panel.W > float32(width) || panel.Y < 0 || panel.Y+panel.H > anchor.Y {
					t.Fatalf("production popover outside client/above anchor: %+v / %+v", panel, anchor)
				}
				for label, bounds := range map[string]ModelPickerFixtureBounds{"Fixture header": state.Geometry.Header, "Fixture trigger row": state.Geometry.Row, "Select model": state.Geometry.Anchor, "Model picker": state.Geometry.Popup} {
					actual, ok := tt.Find(label)
					if !ok || bounds != modelPickerFixtureBounds(actual) {
						t.Fatalf("stale geometry for %s", label)
					}
				}
				for _, label := range []string{"Model management", "GPT-5.3 Codex", "GPT-5.2", "GPT-5.1 Codex Mini", "Fast mode", "Context window"} {
					if _, ok := tt.Find(label); !ok {
						t.Fatalf("missing production control: %s", label)
					}
				}
				if _, ok := tt.Find("Change model"); ok {
					t.Fatal("obsolete first page remains")
				}
			})
		}
	}
}

func TestModelPickerFixtureUsesProductionMutations(t *testing.T) {
	fixture, err := NewModelPickerFixture("dark")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fixture.Close)
	tt := ui.NewTester(fixture.View, 360, 560)
	tt.Advance(time.Second)
	if err := tt.Click("Fast mode"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(10 * time.Millisecond)
	if !fixture.State().Fast {
		t.Fatal("committed fixture state did not read production Fast option")
	}
	snapshot, err := fixture.app.service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Settings.ModelPicker.Options["gpt-5.4/serviceTier"] != "fast" {
		t.Fatal("production Fast click did not persist the source option id/value")
	}
	controls := fixture.app.centralControls()
	model := controls.selectedModel()
	controls.pickModelOption(model, model.Options[1], "1m")
	tt.Frame()
	tt.Advance(10 * time.Millisecond)
	snapshot, err = fixture.app.service.Snapshot(context.Background())
	if err != nil || fixture.State().ContextWindow != "1m" || snapshot.Settings.ModelPicker.Options["gpt-5.4/contextWindow"] != "1m" {
		t.Fatalf("committed fixture state did not read the production context-window id: %+v, %v", fixture.State(), err)
	}
	if err := fixture.SetScene("dark"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Select model GPT-5.3 Codex"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(10 * time.Millisecond)
	snapshot, err = fixture.app.service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if fixture.State().ModelID != "gpt-5.3-codex" || snapshot.Settings.ModelPicker.Selections["codex"].ModelID != "gpt-5.3-codex" {
		t.Fatal("production model click did not update the in-memory typed backend")
	}
	if len(snapshot.Providers) != 1 || len(snapshot.Models) != 4 || len(snapshot.Integrations.Providers) != 1 || len(snapshot.Sessions) != 0 {
		t.Fatal("fixture contains unrelated product scenario state")
	}
	snapshot.Models[0].ID = "mutated caller copy"
	other, err := fixture.app.service.Snapshot(context.Background())
	if err != nil || other.Models[0].ID != "gpt-5.4" {
		t.Fatalf("fixture snapshot ownership failed: %v", err)
	}
}

func TestModelPickerFixturePublishesCommittedSceneGeometry(t *testing.T) {
	fixture, err := NewModelPickerFixture("dark")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fixture.Close)
	tt := ui.NewTester(fixture.View, 840, 560)
	if fixture.State().Geometry.Ready() {
		t.Fatal("first build claimed committed geometry before a frame was measured")
	}
	tt.Advance(time.Second)
	before := fixture.State()
	if !before.Geometry.Ready() {
		t.Fatal("initial frame did not publish measured geometry")
	}
	if err := fixture.SetScene("light"); err != nil {
		t.Fatal(err)
	}
	if fixture.State() != before {
		t.Fatal("requesting a new scene changed ready state before any rendered frame")
	}
	tt.SetSize(360, 560)
	if state := fixture.State(); state.Appearance == "light" && state.Width == 360 {
		t.Fatal("new scene was published together with previous-frame geometry")
	}
	tt.Advance(time.Second)
	state := fixture.State()
	if state.Appearance != "light" || state.Width != 360 || !state.Geometry.Ready() || !state.GeometryStable || state.LayoutError != "" {
		t.Fatalf("new committed scene did not become ready: %+v", state)
	}
	popup, ok := tt.Find("Model picker")
	if !ok || state.Geometry.Popup != modelPickerFixtureBounds(popup) || state.Geometry.Row.Width != 312 {
		t.Fatalf("new scene retained stale geometry: %+v; popup %v", state.Geometry, popup)
	}
}

func TestModelPickerFixtureGeometrySettlesAfterHeightChanges(t *testing.T) {
	fixture, err := NewModelPickerFixture("dark")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fixture.Close)
	tt := ui.NewTester(fixture.View, 360, 560)
	for _, height := range []int{560, 280, 360, 560} {
		tt.SetSize(360, height)
		tt.Advance(time.Second)
		state := fixture.State()
		popup, ok := tt.Find("Model picker")
		if !ok || state.Height != float32(height) || !state.GeometryStable || state.LayoutError != "" || state.Geometry.Popup != modelPickerFixtureBounds(popup) {
			t.Fatalf("height %d published an intermediate popup budget: %+v / %v", height, state, popup)
		}
	}
}

func TestModelPickerFixtureMissingGeometryStopsRetrying(t *testing.T) {
	fixture, err := NewModelPickerFixture("dark")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fixture.Close)
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) {
		builds++
		fixture.View(c)
		// Simulate a product control no longer returning a bound popup.
		fixture.popup = ui.Handle{}
	}, 360, 560)
	tt.Advance(time.Second)
	state := fixture.State()
	if state.GeometryStable || state.LayoutError == "" || builds > 20 {
		t.Fatalf("missing geometry did not fail within a bounded number of frames: %d, %+v", builds, state)
	}
	before := builds
	tt.Advance(time.Second)
	if builds != before {
		t.Fatalf("failed fixture kept requesting frames: %d -> %d", before, builds)
	}
}
