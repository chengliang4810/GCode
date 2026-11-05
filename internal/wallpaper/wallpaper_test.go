package wallpaper

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestAccentChoosesActualColorRatherThanComplementaryAverage(t *testing.T) {
	pixels := image.NewNRGBA(image.Rect(0, 0, 200, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			pixel := color.NRGBA{R: 230, G: 45, B: 30, A: 255}
			if x > 120 {
				pixel = color.NRGBA{R: 30, G: 200, B: 220, A: 255}
			}
			pixels.SetNRGBA(x, y, pixel)
		}
	}
	accent, err := Accent(pixels)
	if err != nil {
		t.Fatal(err)
	}
	if accent.R < 200 || accent.G > 60 || accent.B > 60 {
		t.Fatalf("accent averaged complementary colors: %+v", accent)
	}
	if _, err := Accent(image.NewNRGBA(image.Rect(0, 0, 4, 4))); err == nil {
		t.Fatal("accepted fully transparent image")
	}
}

func TestWatchPublishesImageChangeAndStopsOnCancellation(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "wallpaper.png")
	write := func(pixel color.NRGBA) {
		t.Helper()
		image := image.NewNRGBA(image.Rect(0, 0, 2, 2))
		for i := 0; i < len(image.Pix); i += 4 {
			image.Pix[i], image.Pix[i+1], image.Pix[i+2], image.Pix[i+3] = pixel.R, pixel.G, pixel.B, pixel.A
		}
		temporary := filepath.Join(directory, "next.png")
		file, err := os.Create(temporary)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, image); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		// Ensure a changed timestamp even on filesystems with coarse clocks.
		stamp := time.Now().Add(time.Duration(pixel.R) * time.Second)
		if err := os.Chtimes(temporary, stamp, stamp); err != nil {
			t.Fatal(err)
		}
		// The watcher must see a complete PNG, rather than a truncated file
		// while the fixture encoder is still writing it.
		if err := os.Rename(temporary, path); err != nil {
			t.Fatal(err)
		}
	}
	write(color.NRGBA{R: 220, G: 30, B: 20, A: 255})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := make(chan Result, 8)
	done := make(chan struct{})
	var queries atomic.Int32
	go func() {
		defer close(done)
		watch(ctx, false, 2*time.Millisecond, func(context.Context, bool) (string, error) {
			queries.Add(1)
			return path, nil
		}, func(result Result) { results <- result })
	}()
	next := func() Result {
		t.Helper()
		select {
		case result := <-results:
			if result.Err != nil {
				t.Fatal(result.Err)
			}
			return result
		case <-time.After(time.Second):
			t.Fatal("watcher missed wallpaper change")
			return Result{}
		}
	}
	if initial := next(); initial.Color.R != 220 {
		t.Fatal(initial)
	}
	for deadline := time.Now().Add(time.Second); queries.Load() < 3 && time.Now().Before(deadline); {
		time.Sleep(time.Millisecond)
	}
	select {
	case extra := <-results:
		t.Fatalf("unchanged wallpaper emitted %+v", extra)
	default:
	}
	write(color.NRGBA{R: 20, G: 40, B: 220, A: 255})
	if changed := next(); changed.Color.B != 220 {
		t.Fatal(changed)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watcher leaked after cancellation")
	}
}

func TestLiveWallpaperEvidence(t *testing.T) {
	if os.Getenv("RCODE_WALLPAPER_EVIDENCE") == "" {
		t.Skip("opt-in read of this machine's desktop wallpaper")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	path, err := currentPath(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	result := Read(path)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	t.Logf("wallpaper=%s color=#%02x%02x%02x", path, result.Color.R, result.Color.G, result.Color.B)
}
