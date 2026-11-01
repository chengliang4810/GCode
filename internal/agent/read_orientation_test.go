package agent

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOrientImageAllEXIFTransforms(t *testing.T) {
	source := image.NewNRGBA(image.Rect(10, 20, 13, 22))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			source.SetNRGBA(10+x, 20+y, color.NRGBA{R: uint8(y*3 + x + 1), A: 255})
		}
	}
	cases := []struct {
		name        string
		orientation int
		width       int
		pixels      []byte
	}{
		{"normal", 1, 3, []byte{1, 2, 3, 4, 5, 6}},
		{"mirror", 2, 3, []byte{3, 2, 1, 6, 5, 4}},
		{"rotate180", 3, 3, []byte{6, 5, 4, 3, 2, 1}},
		{"flip", 4, 3, []byte{4, 5, 6, 1, 2, 3}},
		{"transpose", 5, 2, []byte{1, 4, 2, 5, 3, 6}},
		{"rotate90", 6, 2, []byte{4, 1, 5, 2, 6, 3}},
		{"transverse", 7, 2, []byte{6, 3, 5, 2, 4, 1}},
		{"rotate270", 8, 2, []byte{3, 6, 2, 5, 1, 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oriented := orientImage(source, tc.orientation)
			bounds := oriented.Bounds()
			if bounds.Dx() != tc.width || bounds.Dy() != len(tc.pixels)/tc.width {
				t.Fatal("incorrect oriented dimensions")
			}
			for i, want := range tc.pixels {
				pixel := color.NRGBAModel.Convert(oriented.At(
					bounds.Min.X+i%tc.width, bounds.Min.Y+i/tc.width,
				)).(color.NRGBA)
				if pixel.R != want {
					t.Fatalf("pixel %d: got %d want %d", i, pixel.R, want)
				}
			}
		})
	}
}

func TestReadJPEGUsesEXIFOrientation(t *testing.T) {
	for _, order := range []struct {
		name  string
		bytes binary.ByteOrder
	}{
		{"little endian", binary.LittleEndian},
		{"big endian", binary.BigEndian},
	} {
		t.Run(order.name, func(t *testing.T) {
			var encoded bytes.Buffer
			if err := jpeg.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 20, 10)), nil); err != nil {
				t.Fatal(err)
			}
			exif := make([]byte, 26)
			copy(exif[:2], "II")
			if order.bytes == binary.BigEndian {
				copy(exif[:2], "MM")
			}
			order.bytes.PutUint16(exif[2:4], 42)
			order.bytes.PutUint32(exif[4:8], 8)
			order.bytes.PutUint16(exif[8:10], 1)
			order.bytes.PutUint16(exif[10:12], 0x112)
			order.bytes.PutUint16(exif[12:14], 3)
			order.bytes.PutUint32(exif[14:18], 1)
			order.bytes.PutUint16(exif[18:20], 6)
			payload := append([]byte("Exif\x00\x00"), exif...)
			data := []byte{0xff, 0xd8, 0xff, 0xe1, 0, 0}
			binary.BigEndian.PutUint16(data[4:6], uint16(len(payload)+2))
			data = append(data, payload...)
			data = append(data, encoded.Bytes()[2:]...)
			root := t.TempDir()
			path := filepath.Join(root, "camera.jpg")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			output, err := executeRead(t, root, map[string]any{"file_path": path})
			if err != nil {
				t.Fatal(err)
			}
			decoded := decodeReadImage(t, output.Images[0])
			if decoded.Bounds().Size() != image.Pt(10, 20) || !strings.Contains(output.Text, "orientation normalized") {
				t.Fatalf("orientation metadata was ignored: bounds=%v text=%s", decoded.Bounds(), output.Text)
			}
		})
	}
}

func FuzzJPEGOrientationBounds(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0xff, 0xd8, 0xff, 0xe1, 0xff, 0xff})
	f.Add([]byte("Exif\x00\x00II*\x00\xff\xff\xff\xff"))
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, orientation := range []int{jpegOrientation(data), tiffOrientation(data)} {
			if orientation < 1 || orientation > 8 {
				t.Fatalf("invalid orientation %d", orientation)
			}
		}
	})
}
