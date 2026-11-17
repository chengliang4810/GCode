package filepreview

import "testing"

func TestBuiltinKeepsRenderedFilesOutOfSystemApplications(t *testing.T) {
	for _, name := range []string{"report.pdf", "REPORT.PDF", "index.html", "PAGE.HTM", "book.xhtml"} {
		t.Run(name, func(t *testing.T) {
			if !Builtin(name) {
				t.Fatal("renderable file did not select the embedded browser")
			}
			// A PDF is not UTF-8 text, so the binary fallback must not move it
			// back to a system application.
			if External(name, []byte("%PDF-1.7\n\x00binary")) || External(name, []byte("<html>中文</html>")) {
				t.Fatal("renderable file was routed to a system application")
			}
		})
	}
	for _, name := range []string{"icon.svg", "photo.png", "main.go", "README.md", "manual.docx"} {
		t.Run(name, func(t *testing.T) {
			if Builtin(name) {
				t.Fatal("non-rendered file was treated as a browser preview")
			}
		})
	}
}

func TestExternalPreservesTextAndRecognizesUnsupportedFiles(t *testing.T) {
	for _, name := range []string{
		"report.xlsx", "REPORT.XLSX", "slides.pptx", "document.docx",
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
