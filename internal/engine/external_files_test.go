package engine

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/contracts"
)

func TestUnsupportedLinkedFilesReturnDownloadableExternalFiles(t *testing.T) {
	core, workspace, session := linkedFileTestCore(t)
	for _, test := range []struct {
		name string
		data []byte
	}{
		{"report.xlsx", []byte("PK\x03\x04\x00workbook")},
		{"custom.unknown", []byte{0, 1, 0xff}},
		{"legacy.txt", []byte{0xc4, 0xe3, 0xba, 0xc3}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(workspace, test.name), test.data, 0600); err != nil {
				t.Fatal(err)
			}
			result, err := core.ReadLinkedFile(t.Context(), contracts.LinkedFileRequest{SessionID: session.ID, Path: test.name})
			if err != nil {
				t.Fatal(err)
			}
			if result.File.Kind != contracts.FileExternal || result.File.Content != "" || result.File.AssetRef == "" {
				t.Fatalf("unsupported file did not resolve externally: %+v", result)
			}
			data, err := core.ReadResource(t.Context(), result.File.AssetRef)
			if err != nil || !bytes.Equal(data, test.data) {
				t.Fatalf("external file bytes changed: %v", err)
			}
		})
	}
}

// HTML and PDF content stays out of the editor buffer but reaches the embedded
// browser through the same managed resource path.
func TestRenderableLinkedFilesUseManagedPreviewResources(t *testing.T) {
	core, workspace, session := linkedFileTestCore(t)
	for _, test := range []struct {
		name string
		data []byte
	}{
		{"report.html", []byte("<!doctype html><title>报告</title><h1>预览</h1>")},
		{"manual.pdf", []byte("%PDF-1.7\n\x00\xffbinary PDF")},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(workspace, test.name), test.data, 0600); err != nil {
				t.Fatal(err)
			}
			result, err := core.ReadLinkedFile(t.Context(), contracts.LinkedFileRequest{SessionID: session.ID, Path: test.name})
			if err != nil {
				t.Fatal(err)
			}
			if result.File.Kind != contracts.FilePreview || result.File.Content != "" || result.File.AssetRef == "" {
				t.Fatalf("renderable file did not resolve to a preview resource: %+v", result)
			}
			data, err := core.ReadResource(t.Context(), result.File.AssetRef)
			if err != nil || !bytes.Equal(data, test.data) {
				t.Fatalf("preview resource bytes changed: %v", err)
			}
		})
	}
}

func TestProjectFileOpenMarksRenderableFileAsPreview(t *testing.T) {
	core, workspace, session := linkedFileTestCore(t)
	if err := os.WriteFile(filepath.Join(workspace, "index.html"), []byte("<h1>报告</h1>"), 0600); err != nil {
		t.Fatal(err)
	}
	refreshProjectFilesForTest(t, core)
	fileID := session.ProjectID + ":index.html"
	if err := core.Execute(t.Context(), contracts.Command{Kind: "file.open", ID: fileID}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := core.Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range snapshot.Files {
		if file.ID == fileID && file.Kind == contracts.FilePreview && file.Loaded && file.Content == "" {
			return
		}
	}
	t.Fatal("renderable project file was not marked for the embedded preview")
}

func TestProjectFileOpenMarksBinaryForExternalApplication(t *testing.T) {
	core, workspace, session := linkedFileTestCore(t)
	if err := os.WriteFile(filepath.Join(workspace, "unknown.custom"), []byte{0xff, 0, 1}, 0600); err != nil {
		t.Fatal(err)
	}
	refreshProjectFilesForTest(t, core)
	fileID := session.ProjectID + ":unknown.custom"
	if err := core.Execute(t.Context(), contracts.Command{Kind: "file.open", ID: fileID}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := core.Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range snapshot.Files {
		if file.ID == fileID && file.Kind == contracts.FileExternal && file.Loaded && file.Content == "" {
			return
		}
	}
	t.Fatal("binary file was not handed off to the external application path")
}
