package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"

	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"gcode/internal/model"
)

const (
	maxReadImageBytes  = 24 << 20
	maxReadImagePixels = 32_000_000
	maxReadImageSide   = 2048
	maxReadImageBase64 = 4 << 20
	maxSourceImageSide = 32768
)

// Text reads may overlap freely; large image decoding has a separate memory bound.
var imageReadSlots = make(chan struct{}, 2)

func readImage(ctx context.Context, file *os.File, size int64, path string) (model.ToolOutput, error) {
	if size > maxReadImageBytes {
		return model.ToolOutput{}, errors.New("image file exceeds 24 MiB")
	}
	select {
	case imageReadSlots <- struct{}{}:
		defer func() { <-imageReadSlots }()
	case <-ctx.Done():
		return model.ToolOutput{}, ctx.Err()
	}
	data, err := io.ReadAll(io.LimitReader(file, maxReadImageBytes+1))
	if err != nil {
		return model.ToolOutput{}, err
	}
	if len(data) > maxReadImageBytes {
		return model.ToolOutput{}, errors.New("image file exceeds 24 MiB")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return model.ToolOutput{}, errors.New("invalid or unsupported image; supported formats: PNG, JPEG, GIF, WebP, BMP")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxSourceImageSide ||
		cfg.Height > maxSourceImageSide || cfg.Width > maxReadImagePixels/cfg.Height {
		return model.ToolOutput{}, errors.New("image dimensions exceed the 32 million pixel or 32768 pixel side limit")
	}
	if err := ctx.Err(); err != nil {
		return model.ToolOutput{}, err
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return model.ToolOutput{}, fmt.Errorf("decode image: %w", err)
	}
	if format == "gif" {
		decoded = gifCanvas(decoded, cfg, data)
	}
	orientation := 1
	if format == "jpeg" {
		orientation = jpegOrientation(data)
		decoded = orientImage(decoded, orientation)
	}
	original := decoded.Bounds().Size()
	width, height := fitImage(original.X, original.Y, maxReadImageSide)
	displayed := decoded
	if width != original.X || height != original.Y {
		displayed = scaleImage(decoded, width, height)
	}
	if err := ctx.Err(); err != nil {
		return model.ToolOutput{}, err
	}
	mime := imageMIME(format)
	if mime == "" {
		return model.ToolOutput{}, errors.New("unsupported image format")
	}
	if orientation != 1 || format == "gif" || format == "bmp" ||
		width != original.X || height != original.Y ||
		base64.StdEncoding.EncodedLen(len(data)) > maxReadImageBase64 {
		data, mime, displayed, err = encodeReadImage(ctx, displayed)
		if err != nil {
			return model.ToolOutput{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return model.ToolOutput{}, err
	}
	dimensions := displayed.Bounds().Size()
	note := fmt.Sprintf(
		"Read image %q (%s). Original: %dx%d; displayed: %dx%d.",
		path, mime, original.X, original.Y, dimensions.X, dimensions.Y,
	)
	if dimensions != original {
		note += fmt.Sprintf(
			" Multiply displayed coordinates by x=%.4f, y=%.4f to map to the original image.",
			float64(original.X)/float64(dimensions.X), float64(original.Y)/float64(dimensions.Y),
		)
	}
	if orientation != 1 {
		note += " JPEG orientation normalized; original dimensions refer to the oriented image."
	}
	if format == "gif" {
		note += " GIF first frame only."
	}
	return model.ToolOutput{
		Text: note,
		Data: map[string]any{"type": "image", "file": map[string]any{
			"base64": base64.StdEncoding.EncodeToString(data), "type": mime, "originalSize": size,
			"dimensions": map[string]any{"originalWidth": original.X, "originalHeight": original.Y, "displayWidth": dimensions.X, "displayHeight": dimensions.Y},
		}},
		Images: []model.Image{{
			MIME: mime, Data: base64.StdEncoding.EncodeToString(data),
			Width: dimensions.X, Height: dimensions.Y,
		}},
	}, nil
}

func gifCanvas(frame image.Image, cfg image.Config, data []byte) image.Image {
	bounds := image.Rect(0, 0, cfg.Width, cfg.Height)
	if frame.Bounds() == bounds {
		return frame
	}
	canvas := image.NewNRGBA(bounds)
	// Transparent GIFs keep an empty canvas transparent. Otherwise use the
	// logical screen's global background before placing the first subframe.
	hasTransparency := false
	if paletted, ok := frame.(*image.Paletted); ok {
		for _, entry := range paletted.Palette {
			_, _, _, alpha := entry.RGBA()
			hasTransparency = hasTransparency || alpha == 0
		}
	}
	if !hasTransparency && len(data) >= 13 && data[10]&0x80 != 0 {
		entries := 1 << ((data[10] & 7) + 1)
		background := int(data[11])
		if background < entries && len(data) >= 13+entries*3 {
			at := 13 + background*3
			fill := image.NewUniform(color.NRGBA{R: data[at], G: data[at+1], B: data[at+2], A: 255})
			draw.Draw(canvas, bounds, fill, image.Point{}, draw.Src)
		}
	}
	draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
	return canvas
}

func imageMIME(format string) string {
	switch format {
	case "png", "jpeg", "gif", "webp", "bmp":
		return "image/" + format
	default:
		return ""
	}
}

func fitImage(width, height, side int) (int, int) {
	if width <= side && height <= side {
		return width, height
	}
	if width >= height {
		return side, max(1, int(int64(height)*int64(side)/int64(width)))
	}
	return max(1, int(int64(width)*int64(side)/int64(height))), side
}

func scaleImage(source image.Image, width, height int) image.Image {
	target := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(target, target.Bounds(), source, source.Bounds(), draw.Src, nil)
	return target
}

func encodeReadImage(ctx context.Context, source image.Image) ([]byte, string, image.Image, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, "", nil, err
		}
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, source); err != nil {
			return nil, "", nil, err
		}
		if base64.StdEncoding.EncodedLen(encoded.Len()) <= maxReadImageBase64 {
			return encoded.Bytes(), "image/png", source, nil
		}
		// PNG keeps transparency and sharp text. JPEG is used only for opaque
		// images that do not fit the lossless budget.
		opaque, ok := source.(interface{ Opaque() bool })
		if ok && opaque.Opaque() {
			for _, quality := range []int{85, 70, 55} {
				if err := ctx.Err(); err != nil {
					return nil, "", nil, err
				}
				encoded.Reset()
				if err := jpeg.Encode(&encoded, source, &jpeg.Options{Quality: quality}); err != nil {
					return nil, "", nil, err
				}
				if base64.StdEncoding.EncodedLen(encoded.Len()) <= maxReadImageBase64 {
					return encoded.Bytes(), "image/jpeg", source, nil
				}
			}
		}
		bounds := source.Bounds()
		width, height := max(1, bounds.Dx()*3/4), max(1, bounds.Dy()*3/4)
		source = scaleImage(source, width, height)
	}
}
