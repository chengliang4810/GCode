package mygo

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo/internal/platform"
)

// SetWebViewDataDirectory selects the persistent browser profile on Windows.
// Call it before Run; an empty path keeps the platform's default. A non-empty
// path must be absolute. Other platforms currently return an error matching
// errors.ErrUnsupported.
func (a *Application) SetWebViewDataDirectory(path string) error {
	if path != "" {
		if !filepath.IsAbs(path) {
			return errors.New("mygo: webview data directory must be absolute")
		}
		if runtime.GOOS != "windows" {
			return fmt.Errorf("mygo: custom webview data directory: %w", platform.ErrUnsupported)
		}
		path = filepath.Clean(path)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running {
		return errors.New("mygo: webview data directory must be set before App.Run")
	}
	a.webViewDataDirectory = path
	return nil
}
