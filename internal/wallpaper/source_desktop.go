package wallpaper

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func localWallpaperPath(value string) (string, error) {
	value = strings.Trim(strings.TrimSpace(value), "'\"")
	if strings.HasPrefix(value, "file:") {
		uri, err := url.Parse(value)
		if err != nil || uri.Scheme != "file" || uri.Host != "" && uri.Host != "localhost" {
			return "", errors.New("wallpaper is not a local file")
		}
		value = uri.Path
	}
	if !filepath.IsAbs(value) {
		return "", errors.New("desktop has no local wallpaper")
	}
	return filepath.Clean(value), nil
}

func desktopProviders(desktop, session string) []string {
	desktop = strings.ToLower(desktop + ":" + session)
	switch {
	case strings.Contains(desktop, "kde"), strings.Contains(desktop, "plasma"):
		return []string{"kde"}
	case strings.Contains(desktop, "xfce"):
		return []string{"xfce"}
	case strings.Contains(desktop, "cinnamon"):
		return []string{"cinnamon"}
	case strings.Contains(desktop, "gnome"), strings.Contains(desktop, "unity"), strings.Contains(desktop, "budgie"):
		return []string{"gnome"}
	default:
		return []string{"gnome", "cinnamon", "kde", "xfce"}
	}
}

func existingWallpaper(value string) (string, error) {
	path, err := localWallpaperPath(value)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("wallpaper path is not a regular file")
	}
	return path, nil
}

func linuxWallpaperPath(ctx context.Context, dark bool, providers []string, config string, command func(context.Context, string, ...string) (string, error)) (string, error) {
	for _, provider := range providers {
		switch provider {
		case "gnome", "cinnamon":
			if path := gsettingsWallpaper(ctx, dark, provider, command); path != "" {
				return path, nil
			}
		case "kde":
			if path := kdeWallpaper(config); path != "" {
				return path, nil
			}
		case "xfce":
			if path := xfceWallpaper(ctx, command); path != "" {
				return path, nil
			}
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("no local wallpaper found for desktop %s", strings.Join(providers, "/"))
}

func gsettingsWallpaper(ctx context.Context, dark bool, provider string, command func(context.Context, string, ...string) (string, error)) string {
	key := "picture-uri"
	if dark {
		key = "picture-uri-dark"
	}
	// GNOME/Cinnamon expose wallpaper URIs. A light-only GNOME wallpaper is
	// also valid when the desktop has not set a separate dark image.
	schema := "org.gnome.desktop.background"
	keys := []string{key}
	if dark {
		keys = append(keys, "picture-uri")
	}
	if provider == "cinnamon" {
		schema, keys = "org.cinnamon.desktop.background", []string{"picture-uri"}
	}
	for _, key := range keys {
		if value, err := command(ctx, "gsettings", "get", schema, key); err == nil {
			if path, err := existingWallpaper(value); err == nil {
				return path
			}
		}
	}
	return ""
}

func kdeWallpaper(config string) string {
	// Plasma stores each desktop's configured image in its local applet
	// configuration. Pick the first usable local wallpaper, deterministically.
	if config != "" {
		file, err := os.Open(filepath.Join(config, "plasma-org.kde.plasma.desktop-appletsrc"))
		if err == nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				key, value, found := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
				if found && key == "Image" {
					if path, err := existingWallpaper(value); err == nil {
						return path
					}
				}
			}
		}
	}
	return ""
}

func xfceWallpaper(ctx context.Context, command func(context.Context, string, ...string) (string, error)) string {
	// XFCE property paths include monitor/workspace identifiers; discover
	// those rather than assuming the machine's monitor name.
	if properties, err := command(ctx, "xfconf-query", "-c", "xfce4-desktop", "-l"); err == nil {
		for _, property := range strings.Fields(properties) {
			if !strings.HasSuffix(property, "/last-image") {
				continue
			}
			if value, err := command(ctx, "xfconf-query", "-c", "xfce4-desktop", "-p", property); err == nil {
				if path, err := existingWallpaper(value); err == nil {
					return path
				}
			}
		}
	}
	return ""
}
