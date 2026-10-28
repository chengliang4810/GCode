// Package assets embeds the unmodified artwork and typefaces of the frozen Zeron reference.
package assets

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

//go:embed all:data
var files embed.FS

var fontOnce sync.Once
var fontError error
var iconCache sync.Map

// Read returns an embedded source asset relative to data (for example icons/paperclip.svg).
func Read(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, fmt.Errorf("invalid asset path %q", name)
	}
	return files.ReadFile("data/" + name)
}

// RegisterFonts installs the bundled faces once. Call before opening the first window.
func RegisterFonts() error {
	fontOnce.Do(func() {
		entries, err := fs.Glob(files, "data/fonts/*.ttf")
		if err != nil {
			fontError = err
			return
		}
		for _, name := range entries {
			data, err := files.ReadFile(name)
			if err != nil {
				fontError = err
				return
			}
			if err := ui.RegisterFont(data, ""); err != nil {
				fontError = fmt.Errorf("register %s: %w", name, err)
				return
			}
		}
	})
	return fontError
}

// Icon returns the original SVG by its filename stem. An unknown name returns nil;
// callers should select an existing asset instead of drawing an approximate glyph.
func Icon(name string) *ui.SVG {
	name = strings.TrimSuffix(name, ".svg")
	if cached, ok := iconCache.Load(name); ok {
		return cached.(*ui.SVG)
	}
	data, err := Read("icons/" + name + ".svg")
	if err != nil {
		return nil
	}
	icon, err := ui.ParseSVG(data)
	if err != nil {
		return nil
	}
	actual, _ := iconCache.LoadOrStore(name, icon)
	return actual.(*ui.SVG)
}

// AppIcon returns a fresh copy of the original desktop application icon.
func AppIcon() []byte {
	data, _ := Read("app-icon.png")
	return data
}

// Names lists all embedded file paths, without the data prefix.
func Names() []string {
	var names []string
	_ = fs.WalkDir(files, "data", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			names = append(names, strings.TrimPrefix(path, "data/"))
		}
		return nil
	})
	return names
}
