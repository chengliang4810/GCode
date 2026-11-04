package engine

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/storage"
)

func TestDirectoryLocationsLeadWithHomeThenHostRoots(t *testing.T) {
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	locations, err := core.ListDirectoryLocations(context.Background(), contracts.DirectoryRequest{DeviceID: "local"})
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if len(locations.Locations) == 0 || locations.Locations[0].Kind != contracts.LocationHome || locations.Locations[0].Path != filepath.ToSlash(home) {
		t.Fatalf("home location: %+v", locations.Locations)
	}
	roots := map[string]bool{}
	for _, location := range locations.Locations[1:] {
		if location.Kind != contracts.LocationVolume {
			t.Fatalf("unexpected location: %+v", location)
		}
		if location.Name == "" {
			t.Fatalf("volume without a label: %+v", location)
		}
		if _, err := os.Stat(location.Path); err != nil {
			t.Fatalf("offered volume %q does not exist: %v", location.Path, err)
		}
		if _, err := core.ListDirectories(context.Background(), contracts.DirectoryRequest{DeviceID: "local", Path: location.Path}); err != nil {
			t.Fatalf("offered volume %q does not browse: %v", location.Path, err)
		}
		roots[location.Path] = true
	}
	if runtime.GOOS == "windows" && !roots["C:/"] {
		t.Fatalf("system volume missing: %+v", locations.Locations)
	}
	if _, err := core.ListDirectoryLocations(context.Background(), contracts.DirectoryRequest{DeviceID: "remote"}); err == nil {
		t.Fatal("unknown device offered locations")
	}
}

func TestWindowsVolumeName(t *testing.T) {
	for root, want := range map[string]string{"C:\\": "C:", "D:\\": "D:", "/": "/", "/mnt/data": "/mnt/data"} {
		if got := volumeName(root); got != want {
			t.Errorf("volumeName(%q) = %q, want %q", root, got, want)
		}
	}
}
