package mygo

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWebViewDataDirectoryConfiguration(t *testing.T) {
	app := &Application{}
	if err := app.SetWebViewDataDirectory("relative"); err == nil {
		t.Fatal("relative browser profile accepted")
	}
	path := filepath.Join(t.TempDir(), "browser")
	err := app.SetWebViewDataDirectory(path)
	if runtime.GOOS != "windows" {
		if !errors.Is(err, errors.ErrUnsupported) {
			t.Fatalf("unsupported platform error = %v", err)
		}
	} else {
		if err != nil {
			t.Fatal(err)
		}
		if app.webViewDataDirectory != path {
			t.Fatalf("profile = %q, want %q", app.webViewDataDirectory, path)
		}
	}
	if err := app.SetWebViewDataDirectory(""); err != nil {
		t.Fatal(err)
	}
	if app.webViewDataDirectory != "" {
		t.Fatal("clearing the custom browser profile retained the old directory")
	}
	app.running = true
	if err := app.SetWebViewDataDirectory(""); err == nil {
		t.Fatal("running app allowed its browser profile to change")
	}
}
