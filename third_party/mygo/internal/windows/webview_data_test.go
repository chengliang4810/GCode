//go:build windows && (amd64 || arm64)

package windows

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWebViewUserDataDirectory(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	backend := &Backend{name: "GCode"}
	if got := backend.webViewUserDataDirectory(); got != filepath.Join(local, "GCode", "WebView2") {
		t.Fatalf("default profile = %q", got)
	}
	custom := filepath.Join(t.TempDir(), "运行目录", "browser")
	backend.webViewDataDirectory = custom
	if got := backend.webViewUserDataDirectory(); got != custom {
		t.Fatalf("configured profile = %q, want %q", got, custom)
	}
	backend.webViewDataDirectory = ""
	t.Setenv("LOCALAPPDATA", "")
	if got := backend.webViewUserDataDirectory(); got != filepath.Join(os.TempDir(), "GCode", "WebView2") {
		t.Fatalf("fallback profile = %q", got)
	}
}
