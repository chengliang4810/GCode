// Package wallpaper reads the current desktop wallpaper and extracts its accent.
// It has no graphics-session dependency and is only called by the desktop client.
package wallpaper

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"time"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

// Result is a prepared, small desktop projection; it never holds the image.
type Result struct {
	Color color.NRGBA
	Path  string
	Err   error
}

type fingerprint struct {
	path    string
	size    int64
	modTime time.Time
}

// Watch reports an initial result and changed wallpapers until ctx is cancelled.
// Call it from a worker. Unchanged files are not decoded again. Desktop-query
// errors are reported once, and are retried on the next poll.
func Watch(ctx context.Context, dark bool, publish func(Result)) {
	watch(ctx, dark, time.Minute, currentPath, publish)
}

func watch(ctx context.Context, dark bool, interval time.Duration, source func(context.Context, bool) (string, error), publish func(Result)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var previous fingerprint
	var previousError string
	initialized := false
	for {
		path, err := source(ctx, dark)
		var info os.FileInfo
		if err == nil {
			info, err = os.Stat(path)
		}
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			key := fingerprint{path: path, size: info.Size(), modTime: info.ModTime()}
			if !initialized || key != previous || previousError != "" {
				result := Read(path)
				if result.Err == nil {
					previous = key
				}
				publish(result)
				initialized = true
				previousError = errorString(result.Err)
			}
		} else if !initialized || err.Error() != previousError {
			publish(Result{Path: path, Err: err})
			previousError, initialized = err.Error(), true
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func errorString(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}

// Read decodes one wallpaper with resource bounds and chooses a representative
// saturated color. It does no network access and never changes desktop settings.
func Read(path string) Result {
	result := Result{Path: path}
	file, err := os.Open(path)
	if err != nil {
		result.Err = err
		return result
	}
	defer file.Close()
	const maxBytes = 64 << 20
	config, _, err := image.DecodeConfig(io.LimitReader(file, maxBytes))
	if err != nil {
		result.Err = fmt.Errorf("read wallpaper: %w", err)
		return result
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 64_000_000 {
		result.Err = errors.New("wallpaper exceeds 64 million pixels")
		return result
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		result.Err = err
		return result
	}
	pixels, _, err := image.Decode(io.LimitReader(file, maxBytes))
	if err != nil {
		result.Err = fmt.Errorf("decode wallpaper: %w", err)
		return result
	}
	result.Color, result.Err = Accent(pixels)
	return result
}

// Accent samples at most about 16k pixels. A weighted RGB histogram avoids
// averaging complementary colors into an unrelated gray, while low-chroma
// wallpapers still produce their actual neutral shade.
func Accent(pixels image.Image) (color.NRGBA, error) {
	if pixels == nil || pixels.Bounds().Empty() {
		return color.NRGBA{}, errors.New("wallpaper has no pixels")
	}
	type bin struct{ weight, r, g, b uint64 }
	var histogram [4096]bin
	bounds := pixels.Bounds()
	stepX, stepY := max(1, (bounds.Dx()+127)/128), max(1, (bounds.Dy()+127)/128)
	for y := bounds.Min.Y; y < bounds.Max.Y; y += stepY {
		for x := bounds.Min.X; x < bounds.Max.X; x += stepX {
			pixel := color.NRGBAModel.Convert(pixels.At(x, y)).(color.NRGBA)
			if pixel.A < 128 {
				continue
			}
			r, g, b := uint64(pixel.R), uint64(pixel.G), uint64(pixel.B)
			chroma := max(r, g, b) - min(r, g, b)
			weight := uint64(32) + chroma
			index := int(pixel.R>>4)<<8 | int(pixel.G>>4)<<4 | int(pixel.B>>4)
			bin := &histogram[index]
			bin.weight += weight
			bin.r, bin.g, bin.b = bin.r+r*weight, bin.g+g*weight, bin.b+b*weight
		}
	}
	best := bin{}
	for _, candidate := range histogram {
		if candidate.weight > best.weight {
			best = candidate
		}
	}
	if best.weight == 0 {
		return color.NRGBA{}, errors.New("wallpaper has no opaque pixels")
	}
	return color.NRGBA{R: uint8(best.r / best.weight), G: uint8(best.g / best.weight), B: uint8(best.b / best.weight), A: 255}, nil
}
