//go:build linux || darwin

package wallpaper

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func desktopCommand(ctx context.Context, name string, args ...string) (string, error) {
	query, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	data, err := exec.CommandContext(query, name, args...).Output()
	return strings.TrimSpace(string(data)), err
}

func currentPath(ctx context.Context, dark bool) (string, error) {
	if runtime.GOOS == "darwin" {
		value, err := desktopCommand(ctx, "osascript", "-e", `tell application "System Events" to get picture of current desktop`)
		if err != nil {
			return "", fmt.Errorf("query macOS wallpaper: %w", err)
		}
		return localWallpaperPath(value)
	}
	config, _ := os.UserConfigDir()
	return linuxWallpaperPath(ctx, dark, desktopProviders(os.Getenv("XDG_CURRENT_DESKTOP"), os.Getenv("DESKTOP_SESSION")), config, desktopCommand)
}
