package wallpaper

import (
	"context"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func readNativeWallpaper(ctx context.Context, path string) (image.Image, bool, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".heic", ".heif":
	default:
		return nil, false, nil
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, true, err
	}
	dir, err := os.MkdirTemp("", "gcode-wallpaper-")
	if err != nil {
		return nil, true, err
	}
	defer os.RemoveAll(dir)
	preview := filepath.Join(dir, "preview.png")
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "/usr/bin/sips", "-s", "format", "png", "-Z", "256", path, "--out", preview).Run(); err != nil {
		return nil, true, err
	}
	f, err := os.Open(preview)
	if err != nil {
		return nil, true, err
	}
	defer f.Close()
	pixels, err := png.Decode(f)
	return pixels, true, err
}
