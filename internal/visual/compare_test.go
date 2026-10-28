package visual

import (
	"image"
	"image/color"
	"testing"
)

func TestCompare(t *testing.T) {
	a := image.NewRGBA(image.Rect(0, 0, 16, 16))
	b := image.NewRGBA(a.Bounds())
	result, _, _, _, err := Compare(a, b)
	if err != nil || result.SSIM != 1 || result.ChangedPixels != 0 {
		t.Fatalf("identical: %+v %v", result, err)
	}
	b.SetRGBA(4, 4, color.RGBA{R: 255, A: 255})
	result, _, _, _, err = Compare(a, b)
	if err != nil || result.SSIM >= 1 || result.ChangedPixels != 1 {
		t.Fatalf("changed: %+v %v", result, err)
	}
	if _, _, _, _, err := Compare(a, image.NewRGBA(image.Rect(0, 0, 8, 8))); err == nil {
		t.Fatal("dimension mismatch accepted")
	}
}
