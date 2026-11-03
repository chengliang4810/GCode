// Command app-icon renders GCode's small-size GC artwork from vector paths.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/image/vector"
)

var sizes = []int{16, 20, 24, 32, 40, 48, 64, 96, 128, 256, 1024}

// Paths use a 32-pixel grid. The enlarged counters and separated C shoulder
// retain a full pixel of negative space at 16px instead of a thin crossover.
const mark = `M20.5 6.8
C18.2 3.3 13.5 1.8 9.3 3.5
C4.8 5.3 2 9.8 2 15
C2 22 6.1 27 12.1 27
C14.8 27 17 26.2 18.6 24.8
L16.6 21.3
C15.3 22.4 13.8 23 12.1 23
C8.3 23 6 19.8 6 15
C6 11.3 8.1 8 11 7.2
C13.5 6.5 15.9 7.2 17.4 9.3
C18.8 11.1 21.9 8.9 20.5 6.8 Z
M11 12.6 L15.6 12.6
C19.3 12.6 21 14.5 21 18
C21 21.2 22.5 23 25 23
C26.4 23 27.5 22.4 28.2 21.6
C29.8 20 32 22.7 30.3 24.4
C28.9 26.1 27.2 27 24.9 27
C20.2 27 17 23.3 17 18.3
C17 17.1 16.6 16.6 15.2 16.6
L11 16.6 C8.8 16.6 8.8 12.6 11 12.6 Z
M22.4 13.3
C25.1 11.7 28.5 12.2 30.4 14.5
C31.9 16.3 29.3 18.4 27.8 16.8
C26.7 15.6 25.1 15.6 23.6 16.7 Z`

const tile = `M6 0 L26 0
C29.3137 0 32 2.6863 32 6
L32 26 C32 29.3137 29.3137 32 26 32
L6 32 C2.6863 32 0 29.3137 0 26
L0 6 C0 2.6863 2.6863 0 6 0 Z`

func main() {
	out := flag.String("out", "internal/assets/branding", "Output directory")
	flag.Parse()
	if err := generate(*out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(out string) error {
	if err := os.MkdirAll(out, 0755); err != nil {
		return err
	}
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">` +
		`<path fill="#111111" d="` + tile + `"/>` +
		`<path fill="#ffffff" d="` + mark + `"/></svg>` + "\n"
	if err := os.WriteFile(filepath.Join(out, "app-icon-small.svg"), []byte(svg), 0644); err != nil {
		return err
	}
	for _, size := range sizes {
		img := image.NewNRGBA(image.Rect(0, 0, size, size))
		if err := paint(img, tile, color.NRGBA{R: 17, G: 17, B: 17, A: 255}); err != nil {
			return err
		}
		if err := paint(img, mark, color.NRGBA{R: 255, G: 255, B: 255, A: 255}); err != nil {
			return err
		}
		var data bytes.Buffer
		if err := png.Encode(&data, img); err != nil {
			return err
		}
		name := fmt.Sprintf("app-icon-%d.png", size)
		if size == 1024 {
			name = "app-icon.png"
		}
		if err := os.WriteFile(filepath.Join(out, name), data.Bytes(), 0644); err != nil {
			return err
		}
		fmt.Printf("%s: %dx%d\n", filepath.Join(out, name), size, size)
	}
	return generateMacIcon(out)
}

// generateMacIcon keeps the tile inside a centered 832/1024 footprint.
// Render each ICNS representation from the paths, rather than downscaling a PNG.
func generateMacIcon(out string) error {
	var entries bytes.Buffer
	for _, entry := range []struct {
		size int
		kind string
	}{
		{16, "icp4"}, {32, "icp5"}, {64, "icp6"}, {128, "ic07"},
		{256, "ic08"}, {512, "ic09"}, {1024, "ic10"},
	} {
		size := entry.size
		inset := int(float64(size)*96/1024 + 0.5)
		tileImage := image.NewNRGBA(image.Rect(0, 0, size-2*inset, size-2*inset))
		if err := paint(tileImage, tile, color.NRGBA{R: 17, G: 17, B: 17, A: 255}); err != nil {
			return err
		}
		if err := paint(tileImage, mark, color.NRGBA{R: 255, G: 255, B: 255, A: 255}); err != nil {
			return err
		}
		canvas := image.NewNRGBA(image.Rect(0, 0, size, size))
		draw.Draw(canvas, image.Rect(inset, inset, size-inset, size-inset), tileImage, image.Point{}, draw.Src)
		var data bytes.Buffer
		if err := png.Encode(&data, canvas); err != nil {
			return err
		}
		entries.WriteString(entry.kind)
		if err := binary.Write(&entries, binary.BigEndian, uint32(data.Len()+8)); err != nil {
			return err
		}
		entries.Write(data.Bytes())
		if size == 1024 {
			if err := os.WriteFile(filepath.Join(out, "app-icon-macos.png"), data.Bytes(), 0644); err != nil {
				return err
			}
		}
	}
	var icns bytes.Buffer
	icns.WriteString("icns")
	if err := binary.Write(&icns, binary.BigEndian, uint32(entries.Len()+8)); err != nil {
		return err
	}
	icns.Write(entries.Bytes())
	return os.WriteFile(filepath.Join(out, "app-icon-macos.icns"), icns.Bytes(), 0644)
}

func paint(dst *image.NRGBA, path string, fill color.NRGBA) error {
	scale := float32(dst.Bounds().Dx()) / 32
	raster := vector.NewRasterizer(dst.Bounds().Dx(), dst.Bounds().Dy())
	fields := strings.Fields(path)
	var command string
	for index := 0; index < len(fields); {
		token := fields[index]
		switch token[0] {
		case 'M', 'L', 'C', 'Z':
			command = token[:1]
			if len(token) > 1 {
				fields[index] = token[1:]
			} else {
				index++
			}
		default:
			return fmt.Errorf("expected path command at %q", token)
		}
		if command == "Z" {
			raster.ClosePath()
			continue
		}
		count := 2
		if command == "C" {
			count = 6
		}
		if index+count > len(fields) {
			return fmt.Errorf("incomplete %s path command", command)
		}
		points := make([]float32, count)
		for offset := range count {
			value, err := strconv.ParseFloat(fields[index+offset], 32)
			if err != nil {
				return err
			}
			points[offset] = float32(value) * scale
		}
		switch command {
		case "M":
			raster.MoveTo(points[0], points[1])
		case "L":
			raster.LineTo(points[0], points[1])
		case "C":
			raster.CubeTo(points[0], points[1], points[2], points[3], points[4], points[5])
		}
		index += count
	}
	raster.Draw(dst, dst.Bounds(), image.NewUniform(fill), image.Point{})
	return nil
}
