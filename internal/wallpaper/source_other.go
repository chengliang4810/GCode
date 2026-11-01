//go:build !windows && !linux && !darwin

package wallpaper

import (
	"context"
	"errors"
)

func currentPath(context.Context, bool) (string, error) {
	return "", errors.New("wallpaper discovery is unavailable on this platform")
}
