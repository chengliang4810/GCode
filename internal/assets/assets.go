// Package assets embeds GCode branding and the frozen Zeron artwork and typefaces.
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

//go:embed branding/app-icon.png
var appIcon []byte

//go:embed branding/app-icon-macos.png
var macAppIcon []byte

//go:embed branding/app-icon-*.png
var smallAppIcons embed.FS

// glyphs holds control glyphs drawn for GCode rather than imported from the
// frozen reference artwork, including window caption and reasoning controls.
// These match the app's icon grid and inherit the current theme's text color.
//
//go:embed glyphs/*.svg
var glyphs embed.FS

// Additional licensed brand artwork is kept apart from the frozen reference.
//
//go:embed brands/*.svg
var brands embed.FS

var fontOnce sync.Once
var fontError error
var iconCache sync.Map
var glyphCache sync.Map

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

// Glyph returns a control glyph drawn for GCode by its filename stem. These
// are not part of the frozen upstream artwork and are not subject to the
// import manifest. An unknown name returns nil.
func Glyph(name string) *ui.SVG {
	name = strings.TrimSuffix(name, ".svg")
	if cached, ok := glyphCache.Load(name); ok {
		return cached.(*ui.SVG)
	}
	data, err := glyphs.ReadFile("glyphs/" + name + ".svg")
	if err != nil {
		return nil
	}
	icon, err := ui.ParseSVG(data)
	if err != nil {
		return nil
	}
	actual, _ := glyphCache.LoadOrStore(name, icon)
	return actual.(*ui.SVG)
}

// Icon returns a bundled SVG by its filename stem. An unknown name returns nil;
// callers should select an existing asset instead of drawing an approximate glyph.
func Icon(name string) *ui.SVG {
	name = strings.TrimSuffix(name, ".svg")
	if cached, ok := iconCache.Load(name); ok {
		return cached.(*ui.SVG)
	}
	data, err := Read("icons/" + name + ".svg")
	if err != nil {
		data, err = brands.ReadFile("brands/" + name + ".svg")
		if err != nil {
			return nil
		}
	}
	icon, err := ui.ParseSVG(data)
	if err != nil {
		return nil
	}
	actual, _ := iconCache.LoadOrStore(name, icon)
	return actual.(*ui.SVG)
}

// AppIcon returns a fresh copy of the GCode desktop application icon.
func AppIcon() []byte {
	return append([]byte(nil), appIcon...)
}

// MacAppIcon returns the macOS artwork with transparent space around the tile.
func MacAppIcon() []byte {
	return append([]byte(nil), macAppIcon...)
}

// AppIconForSize returns artwork rasterized for the requested native icon size.
func AppIconForSize(size int) ([]byte, error) {
	for _, available := range []int{16, 20, 24, 32, 40, 48, 64, 96, 128, 256} {
		if size <= available {
			return smallAppIcons.ReadFile(fmt.Sprintf("branding/app-icon-%d.png", available))
		}
	}
	return AppIcon(), nil
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
