//go:build !darwin

package wallpaper

import (
	"context"
	"image"
)

func readNativeWallpaper(context.Context, string) (image.Image, bool, error) {
	return nil, false, nil
}
