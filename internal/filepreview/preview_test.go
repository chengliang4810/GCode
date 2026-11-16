package filepreview

import "testing"

func TestExternalPreservesTextAndRecognizesUnsupportedFiles(t *testing.T) {
	for _, name := range []string{
		"report.xlsx", "REPORT.XLSX", "slides.pptx", "document.docx", "manual.pdf",
		"archive.zip", "video.mp4", "audio.mp3", "photo.heic", "icon.ico",
		"font.woff2", "data.sqlite", "drawing.dxf", "model.gltf", "book.epub",
	} {
		t.Run(name, func(t *testing.T) {
			if !External(name, []byte("even a text-based container needs its own application")) {
				t.Fatal("unsupported type was treated as plain text")
			}
		})
	}
	for _, name := range []string{"file.txt", "README.md", "main.go", "data.json", "data.csv", "Dockerfile", "unknown.extension"} {
		t.Run(name, func(t *testing.T) {
			if External(name, []byte("UTF-8 中文文本")) {
				t.Fatal("readable text lost its internal preview")
			}
			if !External(name, []byte{0xff, 0xfe}) || !External(name, []byte("a\x00b")) {
				t.Fatal("unreadable content did not route externally")
			}
		})
	}
}
