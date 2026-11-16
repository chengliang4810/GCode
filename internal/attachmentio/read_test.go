package attachmentio

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/contracts"
)

func TestReadEnforcesDeclaredAndActualSizes(t *testing.T) {
	for _, tc := range []struct {
		name, data  string
		size, limit int64
		ok          bool
	}{
		{"exact", "abc", 3, 3, true},
		{"empty file", "", 0, 3, true},
		{"short", "ab", 3, 3, false},
		{"growth", "abcd", 3, 5, false},
		{"metadata over limit", "abc", 3, 2, false},
		{"unknown exact", "abc", -1, 3, true},
		{"unknown over limit", "abcd", -1, 3, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Read(strings.NewReader(tc.data), tc.size, tc.limit)
			if (err == nil) != tc.ok || tc.ok && string(got) != tc.data {
				t.Fatalf("data=%q error=%v", got, err)
			}
		})
	}
	if _, err := Read(strings.NewReader("ab"), 3, 3); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
}

func TestPlanRejectsOversizeUsingMetadataAndOpenRechecksGrowth(t *testing.T) {
	var paths []string
	for i := 0; i < 5; i++ {
		path := filepath.Join(t.TempDir(), "large.bin")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = f.Truncate(contracts.MaxAttachmentBytes); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
		paths = append(paths, path)
	}
	if _, err := Plan(contracts.AttachmentImportRequest{Paths: paths}); err == nil {
		t.Fatal("oversized batch accepted")
	}
	files, err := Plan(contracts.AttachmentImportRequest{Paths: paths[:1]})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Truncate(paths[0], contracts.MaxAttachmentBytes+1); err != nil {
		t.Fatal(err)
	}
	if f, _, err := Open(files[0], contracts.MaxAttachmentBatchBytes); !errors.Is(err, ErrLimit) {
		if f != nil {
			_ = f.Close()
		}
		t.Fatalf("grown file accepted: %v", err)
	}
}
