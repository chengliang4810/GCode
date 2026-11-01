package wallpaper

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDesktopProvidersSelectActiveSession(t *testing.T) {
	for _, test := range []struct {
		name, desktop, session string
		want                   []string
	}{
		{"Plasma", "KDE", "plasma", []string{"kde"}},
		{"XFCE", "XFCE", "xfce", []string{"xfce"}},
		{"Cinnamon", "X-Cinnamon", "", []string{"cinnamon"}},
		{"GNOME", "ubuntu:GNOME", "ubuntu", []string{"gnome"}},
		{"session fallback", "", "plasmawayland", []string{"kde"}},
		{"unknown", "", "", []string{"gnome", "cinnamon", "kde", "xfce"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := desktopProviders(test.desktop, test.session); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("providers=%v, want %v", got, test.want)
			}
		})
	}
}

func TestLinuxWallpaperDoesNotUseOtherDesktopStaleSettings(t *testing.T) {
	dir := t.TempDir()
	kde, gnome := filepath.Join(dir, "kde.jpg"), filepath.Join(dir, "gnome.jpg")
	for _, path := range []string{kde, gnome} {
		if err := os.WriteFile(path, []byte("existing wallpaper"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	config := filepath.Join(dir, "plasma-org.kde.plasma.desktop-appletsrc")
	if err := os.WriteFile(config, []byte("[Wallpaper]\nImage="+kde+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	queries := 0
	command := func(context.Context, string, ...string) (string, error) {
		queries++
		return "'" + gnome + "'", nil
	}
	path, err := linuxWallpaperPath(context.Background(), false, desktopProviders("KDE", ""), dir, command)
	if err != nil || path != kde || queries != 0 {
		t.Fatalf("KDE used wrong desktop: path=%s queries=%d err=%v", path, queries, err)
	}
	if err := os.Remove(kde); err != nil {
		t.Fatal(err)
	}
	path, err = linuxWallpaperPath(context.Background(), false, desktopProviders("KDE", ""), dir, command)
	if err == nil || path != "" || queries != 0 {
		t.Fatalf("missing KDE file fell back to stale GNOME: %q %v", path, err)
	}
}

func TestLinuxWallpaperTriesExistingLightImageAndUnknownDesktopProviders(t *testing.T) {
	dir := t.TempDir()
	light := filepath.Join(dir, "light.jpg")
	if err := os.WriteFile(light, []byte("existing wallpaper"), 0600); err != nil {
		t.Fatal(err)
	}
	command := func(_ context.Context, name string, args ...string) (string, error) {
		if name == "gsettings" && args[len(args)-1] == "picture-uri-dark" {
			return filepath.Join(dir, "missing.jpg"), nil
		}
		return "'" + light + "'", nil
	}
	if path, err := linuxWallpaperPath(context.Background(), true, []string{"gnome"}, dir, command); err != nil || path != light {
		t.Fatalf("missing dark wallpaper did not use current light: %q %v", path, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plasma-org.kde.plasma.desktop-appletsrc"), []byte("Image="+light+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	command = func(_ context.Context, name string, args ...string) (string, error) {
		if name == "gsettings" {
			return filepath.Join(dir, "missing.jpg"), nil
		}
		return "", errors.New("unavailable command: " + strings.Join(args, " "))
	}
	if path, err := linuxWallpaperPath(context.Background(), false, desktopProviders("", ""), dir, command); err != nil || path != light {
		t.Fatalf("unknown desktop stopped on missing GNOME file: %q %v", path, err)
	}
}
