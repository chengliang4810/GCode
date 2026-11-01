package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"golang.org/x/image/bmp"

	"gcode/internal/model"
)

func TestReadImageFormatsReturnActualPixels(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for i := range source.Pix {
		source.Pix[i] = 255
	}
	source.SetNRGBA(0, 0, color.NRGBA{R: 220, G: 30, B: 20, A: 255})
	formats := []struct {
		name   string
		encode func(*bytes.Buffer) error
	}{
		{"png", func(b *bytes.Buffer) error { return png.Encode(b, source) }},
		{"jpeg", func(b *bytes.Buffer) error { return jpeg.Encode(b, source, nil) }},
		{"gif", func(b *bytes.Buffer) error { return gif.Encode(b, source, nil) }},
		{"bmp", func(b *bytes.Buffer) error { return bmp.Encode(b, source) }},
		{"webp", func(b *bytes.Buffer) error {
			// Generated for this test from a uniform 3x2 RGBA image.
			data, err := base64.StdEncoding.DecodeString("UklGRh4AAABXRUJQVlA4TBEAAAAvAkAAEAdQwVIUvqCBiOh/AAA=")
			b.Write(data)
			return err
		}},
	}
	for _, format := range formats {
		t.Run(format.name, func(t *testing.T) {
			var data bytes.Buffer
			if err := format.encode(&data); err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			// Content detection must work without an image extension.
			path := filepath.Join(root, "图片 sample.data")
			if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			output, err := executeRead(t, root, map[string]any{"file_path": filepath.Base(path)})
			if err != nil || len(output.Images) != 1 {
				t.Fatalf("read %s: %+v %v", format.name, output, err)
			}
			decoded := decodeReadImage(t, output.Images[0])
			if decoded.Bounds().Size() != image.Pt(3, 2) ||
				output.Images[0].Width != 3 || output.Images[0].Height != 2 ||
				!strings.Contains(output.Text, "Original: 3x2; displayed: 3x2") {
				t.Fatalf("lost image dimensions: %+v", output)
			}
			if format.name == "png" {
				original, _ := base64.StdEncoding.DecodeString(output.Images[0].Data)
				if !bytes.Equal(original, data.Bytes()) {
					t.Fatal("small PNG was unnecessarily recompressed")
				}
			}
			if format.name == "bmp" && output.Images[0].MIME != "image/png" {
				t.Fatal("BMP was not converted into a portable inline format")
			}
			if format.name == "webp" {
				r, g, b, a := decoded.At(0, 0).RGBA()
				if r == 0 || g == 0 || b == 0 || a == 0 || a == 65535 {
					t.Fatal("WebP pixels or transparency were lost")
				}
			}
		})
	}
}

func TestReadImageResizeAndTransparency(t *testing.T) {
	root := t.TempDir()
	source := image.NewNRGBA(image.Rect(0, 0, 4096, 64))
	source.SetNRGBA(2048, 32, color.NRGBA{R: 255, A: 150})
	path := filepath.Join(root, "wide.png")
	writePNG(t, path, source)
	output, err := executeRead(t, root, map[string]any{"file_path": path})
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodeReadImage(t, output.Images[0])
	if decoded.Bounds().Size() != image.Pt(2048, 32) || output.Images[0].MIME != "image/png" ||
		!strings.Contains(output.Text, "x=2.0000, y=2.0000") {
		t.Fatalf("incorrect resize/coordinate mapping: %+v", output)
	}
	_, _, _, alpha := decoded.At(0, 0).RGBA()
	if alpha != 0 {
		t.Fatal("transparent pixels were flattened")
	}
}

func TestReadImageEncodedBudgetPreservesAlpha(t *testing.T) {
	root := t.TempDir()
	source := image.NewNRGBA(image.Rect(0, 0, 1100, 1100))
	state := uint32(12345)
	for i := range source.Pix {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		source.Pix[i] = byte(state)
	}
	path := filepath.Join(root, "noisy-alpha.png")
	writePNG(t, path, source)
	output, err := executeRead(t, root, map[string]any{"file_path": path})
	if err != nil {
		t.Fatal(err)
	}
	result := output.Images[0]
	if len(result.Data) > maxReadImageBase64 || result.MIME != "image/png" || result.Width >= 1100 {
		t.Fatalf("image did not fit its encoded budget: mime=%s size=%d dimensions=%dx%d",
			result.MIME, len(result.Data), result.Width, result.Height)
	}
	decoded := decodeReadImage(t, result)
	_, _, _, alpha := decoded.At(10, 10).RGBA()
	if alpha == 65535 {
		t.Fatal("budget reduction erased transparency")
	}
}

func TestReadGIFKeepsFirstFrameCanvasAndOffset(t *testing.T) {
	root := t.TempDir()
	palette := color.Palette{
		color.NRGBA{A: 0}, color.NRGBA{R: 255, A: 255}, color.NRGBA{B: 255, A: 255},
	}
	first := image.NewPaletted(image.Rect(20, 10, 40, 30), palette)
	second := image.NewPaletted(image.Rect(0, 0, 100, 80), palette)
	for i := range first.Pix {
		first.Pix[i] = 1
	}
	for i := range second.Pix {
		second.Pix[i] = 2
	}
	var data bytes.Buffer
	err := gif.EncodeAll(&data, &gif.GIF{
		Image: []*image.Paletted{first, second}, Delay: []int{0, 0},
		Config: image.Config{ColorModel: palette, Width: 100, Height: 80},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "offset.gif")
	if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	output, err := executeRead(t, root, map[string]any{"file_path": path})
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodeReadImage(t, output.Images[0])
	red := color.NRGBAModel.Convert(decoded.At(25, 15)).(color.NRGBA)
	_, _, _, outsideAlpha := decoded.At(0, 0).RGBA()
	if decoded.Bounds().Size() != image.Pt(100, 80) || red.R != 255 || red.B != 0 ||
		outsideAlpha != 0 || !strings.Contains(output.Text, "first frame only") {
		t.Fatalf("GIF canvas/subframe was changed: bounds=%v pixel=%v alpha=%d", decoded.Bounds(), red, outsideAlpha)
	}
}

func TestReadRejectsUnsafeOrInvalidInputs(t *testing.T) {
	root := t.TempDir()
	writePNG(t, filepath.Join(root, "valid.png"), image.NewRGBA(image.Rect(0, 0, 2, 2)))
	tiny, _ := os.ReadFile(filepath.Join(root, "valid.png"))
	bomb := bytes.Clone(tiny)
	binary.BigEndian.PutUint32(bomb[16:20], 10000)
	binary.BigEndian.PutUint32(bomb[20:24], 10000)
	binary.BigEndian.PutUint32(bomb[29:33], crc32.ChecksumIEEE(bomb[12:29]))
	for name, data := range map[string][]byte{
		"bomb.png": bomb, "broken.png": []byte("\x89PNG\r\n\x1a\nbroken"),
		"binary.bin": {0, 1, 2}, "wrong.png": []byte("plain text"),
		"bad-utf8.txt": {0xff, 0xfe}, "sample.pdf": []byte("%PDF-1.7\nnot supported"),
	} {
		if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	oversized, err := os.Create(filepath.Join(root, "oversized.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := oversized.Truncate(maxReadImageBytes + 1); err != nil {
		t.Fatal(err)
	}
	oversized.Close()
	cases := []struct {
		name string
		args map[string]any
	}{
		{"pixel limit", map[string]any{"file_path": "bomb.png"}},
		{"file limit", map[string]any{"file_path": "oversized.png"}},
		{"corrupt image", map[string]any{"file_path": "broken.png"}},
		{"spoofed extension", map[string]any{"file_path": "wrong.png"}},
		{"binary", map[string]any{"file_path": "binary.bin"}},
		{"encoding", map[string]any{"file_path": "bad-utf8.txt"}},
		{"PDF", map[string]any{"file_path": "sample.pdf"}},
		{"directory", map[string]any{"file_path": "."}},
		{"missing", map[string]any{"file_path": "missing"}},
		{"image paging", map[string]any{"file_path": "valid.png", "offset": 1}},
		{"invalid offset", map[string]any{"file_path": "valid.png", "offset": 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := executeRead(t, root, tc.args)
			if err == nil || len(output.Images) != 0 {
				t.Fatalf("invalid read published images: %s %+v %v", tc.name, output, err)
			}
		})
	}
}

func TestReadTextPagingAndBoundedUnicodeOutput(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "code.txt")
	if err := os.WriteFile(path, []byte("一\n二\n三\n四\n"), 0600); err != nil {
		t.Fatal(err)
	}
	output, err := executeRead(t, root, map[string]any{"file_path": path, "offset": 2, "limit": 2})
	if err != nil || !strings.Contains(output.Text, "2: 二\n3: 三\n") ||
		!strings.Contains(output.Text, "offset=4") || len(output.Images) != 0 {
		t.Fatalf("paging changed: %+v %v", output, err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("中文", 40000)), 0600); err != nil {
		t.Fatal(err)
	}
	output, err = executeRead(t, root, map[string]any{"file_path": path})
	if err != nil || len(output.Text) > maxReadTextOutput || !utf8.ValidString(output.Text) ||
		!strings.Contains(output.Text, "line 1 truncated") {
		t.Fatalf("unbounded or invalid UTF-8 output: length=%d err=%v", len(output.Text), err)
	}
}

func TestReadImageCancellationWhileWaitingForDecoder(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "image.png")
	writePNG(t, path, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	for i := 0; i < cap(imageReadSlots); i++ {
		imageReadSlots <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(imageReadSlots); i++ {
			<-imageReadSlots
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	raw, _ := json.Marshal(map[string]any{"file_path": path})
	output, err := (readTool{}).Execute(ctx, root, raw)
	if err != context.DeadlineExceeded || len(output.Images) != 0 {
		t.Fatalf("decoder wait ignored cancellation: %+v %v", output, err)
	}
}

func executeRead(t *testing.T, cwd string, args map[string]any) (model.ToolOutput, error) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return (readTool{}).Execute(context.Background(), cwd, raw)
}

func decodeReadImage(t *testing.T, result model.Image) image.Image {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	decoded, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil || result.MIME != "image/"+format {
		t.Fatalf("MIME/data mismatch: mime=%s format=%s err=%v", result.MIME, format, err)
	}
	return decoded
}

func writePNG(t *testing.T, path string, source image.Image) {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, source); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}
