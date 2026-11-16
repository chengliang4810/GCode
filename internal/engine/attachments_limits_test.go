package engine

import (
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/contracts"
)

func TestAttachmentBatchLimitFailsBeforeCreatingManagedFiles(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	var paths []string
	for range 5 {
		path := filepath.Join(t.TempDir(), "large.bin")
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Truncate(contracts.MaxAttachmentBytes); err != nil {
			t.Fatal(err)
		}
		_ = file.Close()
		paths = append(paths, path)
	}
	_, err := core.ImportAttachments(t.Context(), contracts.AttachmentImportRequest{Paths: paths})
	if err == nil {
		t.Fatal("oversized batch accepted")
	}
	snapshot, err := core.Snapshot(t.Context())
	if err != nil || len(snapshot.Attachments) != 0 {
		t.Fatalf("failed batch left metadata: %v", err)
	}
	files, err := os.ReadDir(filepath.Join(core.storage.Directory(), "uploads"))
	if err != nil || len(files) != 0 {
		t.Fatalf("failed batch wrote managed bytes: %v", err)
	}
}
