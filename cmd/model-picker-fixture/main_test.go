//go:build uifixture

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/desktop"
	"github.com/egoist/mygo"
)

func TestSceneReadinessRequiresMatchingCommittedGeometry(t *testing.T) {
	scene := scene{appearance: "light", page: "list", width: 360, height: 560}
	box := desktop.CompactPickerFixtureBounds{X: 24, Y: 496, Width: 120, Height: 32}
	state := desktop.CompactPickerFixtureState{
		Appearance: "light", Page: "list", Width: 360, Height: 560, PopoverOpen: true, GeometryStable: true,
		Geometry: desktop.CompactPickerFixtureGeometry{Header: box, Row: box, Anchor: box, Popup: box},
	}
	if !scene.ready(state) {
		t.Fatal("matching committed scene did not become ready")
	}
	for _, missing := range []string{"appearance", "page", "width", "height", "open", "header", "row", "anchor", "popup", "stable", "error"} {
		t.Run(missing, func(t *testing.T) {
			stale := state
			switch missing {
			case "appearance":
				stale.Appearance = "dark"
			case "page":
				stale.Page = "panel"
			case "width":
				stale.Width = 840
			case "height":
				stale.Height = 600
			case "open":
				stale.PopoverOpen = false
			case "header":
				stale.Geometry.Header = desktop.CompactPickerFixtureBounds{}
			case "row":
				stale.Geometry.Row = desktop.CompactPickerFixtureBounds{}
			case "anchor":
				stale.Geometry.Anchor = desktop.CompactPickerFixtureBounds{}
			case "popup":
				stale.Geometry.Popup = desktop.CompactPickerFixtureBounds{}
			case "stable":
				stale.GeometryStable = false
			case "error":
				stale.LayoutError = "geometry could not settle"
			}
			if scene.ready(stale) {
				t.Fatal("a stale or unmeasured scene was marked ready")
			}
		})
	}
}

func TestCaptureModesKeepOSAppearanceSeparateFromScene(t *testing.T) {
	client, err := newCaptureSettings("client", "")
	if err != nil || client.title() != "Compact picker fixture" || client.theme("dark") != mygo.ThemeDark || client.theme("light") != mygo.ThemeLight {
		t.Fatalf("default client capture behavior changed: %+v, %v", client, err)
	}
	bridge := filepath.Join(t.TempDir(), "fixture-capture.dll")
	data := []byte("fixture-only bridge hash test")
	if err := os.WriteFile(bridge, data, 0600); err != nil {
		t.Fatal(err)
	}
	wgc, err := newCaptureSettings("wgc", bridge)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if wgc.title() != "" || wgc.theme("dark") != mygo.ThemeSystem || wgc.theme("light") != mygo.ThemeSystem || wgc.bridgeSHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("WGC capture did not retain blank/system native frame and bridge identity: %+v", wgc)
	}
	for _, args := range [][2]string{{"unknown", ""}, {"client", bridge}, {"wgc", ""}, {"wgc", "relative.dll"}, {"wgc", filepath.Join(t.TempDir(), "missing.dll")}} {
		if _, err := newCaptureSettings(args[0], args[1]); err == nil {
			t.Fatalf("invalid capture mode/bridge accepted: %q", args)
		}
	}
}
