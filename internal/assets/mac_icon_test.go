package assets

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"testing"
)

func TestMacAppIconFootprintAndBundle(t *testing.T) {
	data := MacAppIcon()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, 1024, 1024) {
		t.Fatalf("canvas=%v", img.Bounds())
	}
	minX, minY, maxX, maxY := 1024, 1024, -1, -1
	for y := 0; y < 1024; y++ {
		for x := 0; x < 1024; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a > 0 {
				minX = min(minX, x)
				minY = min(minY, y)
				maxX = max(maxX, x)
				maxY = max(maxY, y)
			}
		}
	}
	if got := image.Rect(minX, minY, maxX+1, maxY+1); got != image.Rect(96, 96, 928, 928) {
		t.Fatalf("tile footprint=%v", got)
	}
	configData, err := os.ReadFile("../../mygo.json")
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		MacOS struct {
			Icon string `json:"icon"`
		} `json:"macos"`
	}
	if err := json.Unmarshal(configData, &config); err != nil {
		t.Fatal(err)
	}
	icns, err := os.ReadFile("../../" + config.MacOS.Icon)
	if err != nil {
		t.Fatal(err)
	}
	if len(icns) < 8 || string(icns[:4]) != "icns" || int(binary.BigEndian.Uint32(icns[4:8])) != len(icns) {
		t.Fatal("invalid ICNS container")
	}
	count := 0
	for offset := 8; offset < len(icns); {
		if len(icns)-offset < 8 {
			t.Fatal("truncated ICNS entry")
		}
		length := int(binary.BigEndian.Uint32(icns[offset+4 : offset+8]))
		if length < 8 || offset+length > len(icns) {
			t.Fatal("invalid ICNS entry length")
		}
		payload := icns[offset+8 : offset+length]
		entry, err := png.Decode(bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		if entry.Bounds().Dx() == 1024 && !bytes.Equal(payload, data) {
			t.Fatal("Dock and bundle icons differ")
		}
		count++
		offset += length
	}
	if count != 7 {
		t.Fatalf("ICNS representations=%d", count)
	}
	data[0] ^= 0xff
	if bytes.Equal(data, MacAppIcon()) {
		t.Fatal("caller can mutate embedded Mac icon")
	}
}
