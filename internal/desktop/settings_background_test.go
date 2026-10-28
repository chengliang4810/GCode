package desktop

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/mock"
	"github.com/egoist/mygo/ui"
)

func backgroundTestFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	pixels := image.NewNRGBA(image.Rect(0, 0, 33, 19))
	for y := 0; y < 19; y++ {
		for x := 0; x < 33; x++ {
			pixels.SetNRGBA(x, y, color.NRGBA{uint8(x * 7), uint8(y * 13), 90, 255})
		}
	}
	if err = png.Encode(f, pixels); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBackgroundAdjustmentAppliesDraftAndCancelPreservesSavedCrop(t *testing.T) {
	app, backend := settingsTestApp(t)
	if err := backend.Execute(context.Background(), contracts.Command{Kind: "background.install", Path: backgroundTestFile(t, "landscape.png")}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := backend.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	app.apply(snapshot)
	app.section = "Appearance"
	app.palette = design.Palette("zeron-dark", true)
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.settingsView(c) }, 900, 1800)
	if tt.HasText("Image path") || tt.HasText("Background image path") || !tt.HasText("Replace image") {
		t.Fatal("background choice flow differs from source")
	}
	if err = tt.Click("Adjust background"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Drag to reposition. Scroll or pinch to zoom.") {
		t.Fatal("native adjustment dialog missing")
	}
	if err = tt.Click("Background zoom in"); err != nil {
		t.Fatal(err)
	}
	bounds, ok := tt.Find("Background crop preview")
	if !ok {
		t.Fatal("missing crop preview")
	}
	x, y := bounds.X+bounds.W/2, bounds.Y+bounds.H/2
	tt.Press(x, y)
	tt.Move(x+30, y+12)
	tt.Release(x+30, y+12)
	before, err := backend.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if before.Settings.Appearance.BackgroundAdjustment.Zoom != 1 {
		t.Fatal("draft was persisted before Apply")
	}
	if err = tt.Click("Apply background adjustment"); err != nil {
		t.Fatal(err)
	}
	saved, err := backend.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if saved.Settings.Appearance.BackgroundAdjustment.Zoom <= 1 || saved.Settings.Appearance.BackgroundAdjustment.FocalX == .5 {
		t.Fatal("crop pan and zoom did not persist")
	}
	if err = tt.Click("Adjust background"); err != nil {
		t.Fatal(err)
	}
	if err = tt.Click("Reset background adjustment"); err != nil {
		t.Fatal(err)
	}
	if err = tt.Click("Cancel background adjustment"); err != nil {
		t.Fatal(err)
	}
	after, err := backend.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if saved.Settings.Appearance.BackgroundAdjustment != after.Settings.Appearance.BackgroundAdjustment {
		t.Fatal("Cancel committed the reset draft")
	}
	if err = tt.Click("Remove background"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Choose image") || tt.HasText("Background effect") {
		t.Fatal("Remove did not restore no-image state")
	}
}

func TestBackgroundAdjustmentCannotApplyToReplacementImage(t *testing.T) {
	app, backend := settingsTestApp(t)
	ctx := context.Background()
	if err := backend.Execute(ctx, contracts.Command{Kind: "background.install", Path: backgroundTestFile(t, "first.png")}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := backend.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	app.apply(snapshot)
	app.section = "Appearance"
	app.palette = design.Palette("zeron-dark", true)
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.settingsView(c) }, 900, 1800)
	if err = tt.Click("Adjust background"); err != nil {
		t.Fatal(err)
	}
	if err = tt.Click("Background zoom in"); err != nil {
		t.Fatal(err)
	}
	if err = backend.Execute(ctx, contracts.Command{Kind: "background.install", Path: backgroundTestFile(t, "replacement.png")}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = backend.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	app.apply(snapshot)
	tt.Frame()
	if !tt.HasText("Background changed. Close and adjust the new image.") {
		t.Fatal("stale image warning missing")
	}
	_ = tt.Click("Apply background adjustment")
	snapshot, err = backend.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Settings.Appearance.BackgroundAdjustment.Zoom != 1 {
		t.Fatal("stale draft was applied to replacement image")
	}
}

func TestBackgroundAdjustmentFitsMinimumWindow(t *testing.T) {
	app, backend := settingsTestApp(t)
	ctx := context.Background()
	if err := backend.Execute(ctx, contracts.Command{Kind: "background.install", Path: backgroundTestFile(t, "small.png")}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := backend.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	app.apply(snapshot)
	app.section = "Appearance"
	app.palette = design.Palette("zeron-dark", true)
	app.expanded["settings-target:New thread background"] = true
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.settingsView(c) }, 900, 600)
	if err = tt.Click("Adjust background"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Background crop preview", "Background zoom", "Apply background adjustment", "Close background adjustment"} {
		r, ok := tt.Find(name)
		if !ok || r.W <= 0 || r.H <= 0 || r.X < 0 || r.Y < 0 || r.X+r.W > 900 || r.Y+r.H > 600 {
			t.Fatalf("%s outside minimum window: %+v", name, r)
		}
	}
}

type backgroundCountingService struct {
	contracts.Service
	reader contracts.ResourceService
	reads  int
}

func (s *backgroundCountingService) ReadResource(ctx context.Context, ref string) ([]byte, error) {
	s.reads++
	return s.reader.ReadResource(ctx, ref)
}

func TestBackgroundViewsDoNotStartResourceWork(t *testing.T) {
	backend, err := mock.New(mock.Options{Instant: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = backend.Execute(context.Background(), contracts.Command{Kind: "background.install", Path: backgroundTestFile(t, "controller.png")}); err != nil {
		t.Fatal(err)
	}
	service := &backgroundCountingService{Service: backend, reader: backend}
	app, err := New(service)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	app.section = "Appearance"
	app.palette = design.Palette("zeron-dark", true)
	preparedReads := service.reads
	if preparedReads != 1 || app.background.Bitmap == nil {
		t.Fatal("controller did not prepare background before view")
	}
	tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(app.palette.UI()); app.settingsView(c) }, 900, 900)
	for range 10 {
		tt.Frame()
	}
	if service.reads != preparedReads {
		t.Fatal("view started background resource I/O")
	}
}
