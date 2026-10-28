// Parity produces evidence; it never creates or updates reference goldens.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"gcode/internal/visual"
	"image"
	_ "image/jpeg"
	"image/png"
	"log"
	"os"
	"path/filepath"
)

func read(path string) image.Image {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	im, _, err := image.Decode(f)
	if err != nil {
		log.Fatal(err)
	}
	return im
}
func main() {
	reference := flag.String("reference", "", "Reference capture")
	actual := flag.String("actual", "", "Implementation capture")
	out := flag.String("out", "output/comparison", "Evidence directory")
	threshold := flag.Float64("threshold", .995, "Required whole-image SSIM")
	flag.Parse()
	if *reference == "" || *actual == "" {
		log.Fatal("--reference and --actual are required")
	}
	if filepath.Clean(*reference) == filepath.Clean(*actual) {
		log.Fatal("reference and actual must be different files")
	}
	result, diff, pair, overlay, err := visual.Compare(read(*reference), read(*actual))
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*out, 0755); err != nil {
		log.Fatal(err)
	}
	for name, im := range map[string]image.Image{"diff.png": diff, "side-by-side.png": pair, "overlay.png": overlay} {
		f, err := os.Create(filepath.Join(*out, name))
		if err != nil {
			log.Fatal(err)
		}
		err = png.Encode(f, im)
		closeErr := f.Close()
		if err != nil {
			log.Fatal(err)
		}
		if closeErr != nil {
			log.Fatal(closeErr)
		}
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*out, "metrics.json"), data, 0644); err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(data))
	if result.SSIM < *threshold {
		os.Exit(1)
	}
}
