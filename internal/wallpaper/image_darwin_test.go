package wallpaper

import (
	"os"
	"testing"
)

func TestReadMacOSDefaultHEICWallpaper(t *testing.T) {
	const path = "/System/Library/CoreServices/DefaultDesktop.heic"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("macOS default HEIC wallpaper is not installed")
	}
	result := read(t.Context(), path)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if result.Path != path || result.Color.A != 255 {
		t.Fatalf("expected an opaque accent for the original wallpaper, got %+v", result)
	}
}
