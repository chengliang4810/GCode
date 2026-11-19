package engine

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/contracts"
)

func TestBackgroundLifecycle(t *testing.T) {
	core, _, _ := linkedFileTestCore(t)
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "background.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	execute := func(command contracts.Command) {
		t.Helper()
		if err := core.Execute(ctx, command); err != nil {
			t.Fatal(err)
		}
	}
	appearance := func() contracts.AppearanceSettings {
		t.Helper()
		snapshot, err := core.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return snapshot.Settings.Appearance
	}
	execute(contracts.Command{Kind: "background.install", Path: path})
	ref := appearance().BackgroundAssetRef
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := core.ReadResource(ctx, ref); err != nil {
		t.Fatal(err)
	}
	execute(contracts.Command{Kind: "background.adjust", Path: ref, Adjustment: &contracts.BackgroundAdjustment{Zoom: 2}})
	if got := appearance().BackgroundAdjustment.Zoom; got != 2 {
		t.Fatalf("zoom = %v, want 2", got)
	}
	execute(contracts.Command{Kind: "background.remove"})
	if appearance().BackgroundAssetRef != "" {
		t.Fatal("background reference was not cleared")
	}
	if _, err := core.ReadResource(ctx, ref); !os.IsNotExist(err) {
		t.Fatalf("removed resource read error = %v, want not exist", err)
	}
}
