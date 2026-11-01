package mock

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gcode/internal/contracts"
)

func writeTestBackground(t *testing.T, dir, name string) (string, []byte) {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, 8, 4))
	im.Set(2, 1, color.NRGBA{R: 110, G: 80, B: 220, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path, buf.Bytes()
}

func TestBackgroundResourceLifecycleAndStaleAdjustment(t *testing.T) {
	s := newTestServer(t, Options{})
	ctx := context.Background()
	path, original := writeTestBackground(t, t.TempDir(), "wallpaper.png")
	if err := s.Execute(ctx, contracts.Command{Kind: "background.install", Path: path}); err != nil {
		t.Fatal(err)
	}
	v, _ := s.Snapshot(ctx)
	a := v.Settings.Appearance
	if a.BackgroundName != "wallpaper.png" || a.Background == "" || a.BackgroundAssetRef != a.Background || a.BackgroundAdjustment != (contracts.BackgroundAdjustment{FocalX: .5, FocalY: .5, Zoom: 1}) {
		t.Fatalf("incorrect installed image: %+v", a)
	}
	resource, err := s.ReadResource(ctx, a.BackgroundAssetRef)
	if err != nil || !bytes.Equal(resource, original) {
		t.Fatal("resource lost original image bytes", err)
	}
	resource[0] ^= 0xff
	again, _ := s.ReadResource(ctx, a.BackgroundAssetRef)
	if !bytes.Equal(again, original) {
		t.Fatal("resource bytes alias backend storage")
	}
	adjust := contracts.BackgroundAdjustment{FocalX: -4, FocalY: 6, Zoom: 8}
	if err = s.Execute(ctx, contracts.Command{Kind: "background.adjust", Path: a.Background, Adjustment: &adjust}); err != nil {
		t.Fatal(err)
	}
	updated, _ := s.Snapshot(ctx)
	if updated.Settings.Appearance.BackgroundAdjustment != (contracts.BackgroundAdjustment{FocalX: 0, FocalY: 1, Zoom: 4}) {
		t.Fatal("adjustment was not normalized")
	}
	stale := updated.Settings
	if err = s.Execute(ctx, contracts.Command{Kind: "background.install", Path: path}); err != nil {
		t.Fatal(err)
	}
	if err = s.Execute(ctx, contracts.Command{Kind: "background.adjust", Path: a.Background, Adjustment: &adjust}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale dialog adjusted replacement", err)
	}
	stale.Appearance.BackgroundEffect = "blur"
	if err = s.UpdateSettings(ctx, stale); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Snapshot(ctx)
	if current.Settings.Appearance.Background == a.Background || current.Settings.Appearance.BackgroundAdjustment != (contracts.BackgroundAdjustment{FocalX: .5, FocalY: .5, Zoom: 1}) || current.Settings.Appearance.BackgroundEffect != "blur" {
		t.Fatal("stale settings restored retired image/crop or lost independent setting")
	}
	if _, err = s.ReadResource(ctx, a.BackgroundAssetRef); !errors.Is(err, ErrNotFound) {
		t.Fatal("retired resource remains accessible", err)
	}
	if err = s.Execute(ctx, contracts.Command{Kind: "background.remove"}); err != nil {
		t.Fatal(err)
	}
	removed, _ := s.Snapshot(ctx)
	if removed.Settings.Appearance.Background != "" || removed.Settings.Appearance.BackgroundAssetRef != "" || removed.Settings.Appearance.BackgroundName != "" {
		t.Fatal("remove retained image metadata")
	}
	if _, err = s.ReadResource(ctx, "resource://managed/../../secret.png"); !errors.Is(err, ErrNotFound) {
		t.Fatal("arbitrary resource path was accepted")
	}
}

func TestBackgroundPersistsIndependentCopyAndRetiresManagedImage(t *testing.T) {
	ctx := context.Background()
	statePath := filepath.Join(t.TempDir(), "replica", "state.json")
	s := newTestServer(t, Options{StatePath: statePath})
	path, original := writeTestBackground(t, t.TempDir(), "reference.png")
	if err := s.Execute(ctx, contracts.Command{Kind: "background.install", Path: path}); err != nil {
		t.Fatal(err)
	}
	v, _ := s.Snapshot(ctx)
	ref := v.Settings.Appearance.BackgroundAssetRef
	managed, ok := s.managedResourcePath(ref)
	if !ok || managed == path {
		t.Fatal("image was not managed independently")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := newTestServer(t, Options{StatePath: statePath})
	actual, err := restarted.ReadResource(ctx, ref)
	if err != nil || !bytes.Equal(actual, original) {
		t.Fatal("restart depended on source image", err)
	}
	if err = restarted.Execute(ctx, contracts.Command{Kind: "background.remove"}); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(managed); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("retired managed file was not removed")
	}
}

func TestBackgroundInvalidImageAndPersistenceFailurePreserveState(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := newTestServer(t, Options{})
	path, _ := writeTestBackground(t, dir, "valid.png")
	if err := s.Execute(ctx, contracts.Command{Kind: "background.install", Path: path}); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Snapshot(ctx)
	broken := filepath.Join(dir, "broken.png")
	if err := os.WriteFile(broken, []byte("not an image"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Execute(ctx, contracts.Command{Kind: "background.install", Path: broken}); err == nil {
		t.Fatal("invalid image accepted")
	}
	after, _ := s.Snapshot(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("rejected image mutated state")
	}
	blocked := filepath.Join(t.TempDir(), "state.json")
	failServer := newTestServer(t, Options{StatePath: blocked})
	if err := os.Mkdir(blocked, 0700); err != nil {
		t.Fatal(err)
	}
	failedBefore, _ := failServer.Snapshot(ctx)
	if err := failServer.Execute(ctx, contracts.Command{Kind: "background.install", Path: path}); err == nil {
		t.Fatal("failed persistence reported success")
	}
	failedAfter, _ := failServer.Snapshot(ctx)
	if !reflect.DeepEqual(failedBefore, failedAfter) {
		t.Fatal("failed save replaced active image")
	}
	files, err := os.ReadDir(filepath.Join(filepath.Dir(blocked), "backgrounds"))
	if err != nil || len(files) != 0 {
		t.Fatal("failed save leaked prepared image", err)
	}
	// Allow the test cleanup to persist the unmodified state normally.
	if err = os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
}

func TestModelPickerOwnershipAndPersistence(t *testing.T) {
	ctx := context.Background()
	statePath := filepath.Join(t.TempDir(), "state.json")
	s := newTestServer(t, Options{StatePath: statePath})
	v, _ := s.Snapshot(ctx)
	v.Settings.ModelPicker = contracts.ModelPickerSettings{ProviderID: "anthropic", Options: map[string]string{"claude-sonnet/effort": "high"}}
	v.Settings.ModelPicker.Selections = map[string]contracts.ModelSelection{"anthropic": {ModelID: "claude-sonnet", Reasoning: "high"}}
	command := contracts.Command{Kind: "settings.update", Settings: &v.Settings}.Clone()
	v.Settings.ModelPicker.Options["claude-sonnet/effort"] = "low"
	v.Settings.ModelPicker.Selections["anthropic"] = contracts.ModelSelection{ModelID: "changed"}
	if err := s.Execute(ctx, command); err != nil {
		t.Fatal(err)
	}
	command.Settings.ModelPicker.Options["claude-sonnet/effort"] = "later"
	actual, _ := s.Snapshot(ctx)
	if actual.Settings.ModelPicker.Options["claude-sonnet/effort"] != "high" {
		t.Fatal("settings retained command aliases")
	}
	actual.Settings.ModelPicker.Options["claude-sonnet/effort"] = "snapshot mutation"
	actual.Settings.ModelPicker.Selections["anthropic"] = contracts.ModelSelection{ModelID: "snapshot mutation"}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := newTestServer(t, Options{StatePath: statePath})
	saved, _ := restarted.Snapshot(ctx)
	if saved.Settings.ModelPicker.Options["claude-sonnet/effort"] != "high" {
		t.Fatal("model picker settings did not persist independently")
	}
	if saved.Settings.ModelPicker.Selections["anthropic"].ModelID != "claude-sonnet" || saved.Settings.ModelPicker.Selections["anthropic"].Reasoning != "high" {
		t.Fatal("provider selections were aliased or lost during persistence")
	}
}

func TestBackgroundAdjustmentRejectsNonfiniteValuesAndAmbiguousCommand(t *testing.T) {
	v := normalizeAdjustment(contracts.BackgroundAdjustment{FocalX: float32(math.NaN()), FocalY: float32(math.Inf(1)), Zoom: float32(math.Inf(-1))})
	if v != (contracts.BackgroundAdjustment{FocalX: .5, FocalY: .5, Zoom: 1}) {
		t.Fatal("nonfinite adjustment escaped normalization")
	}
	if err := (contracts.Command{Kind: "theme.reload", ID: "theme", Adjustment: &v}).Validate(); err == nil {
		t.Fatal("mismatched payload accepted")
	}
}
