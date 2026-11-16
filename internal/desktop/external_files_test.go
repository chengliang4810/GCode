package desktop

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/contracts"
)

type externalFileReader struct {
	contracts.Service
	ref string
}

func (s *externalFileReader) ReadResource(_ context.Context, ref string) ([]byte, error) {
	s.ref = ref
	return []byte("original binary\x00contents"), nil
}

func TestExternalFilePathKeepsLocalAndDownloadsRemote(t *testing.T) {
	service := &externalFileReader{}
	result := contracts.LinkedFileResult{
		File:         contracts.File{Name: "报告.xlsx", Kind: contracts.FileExternal, AssetRef: "managed"},
		AbsolutePath: "/host-only/报告.xlsx",
	}
	local, err := externalFilePath(t.Context(), service, result, true)
	if err != nil || local != filepath.FromSlash(result.AbsolutePath) || service.ref != "" {
		t.Fatalf("local file was copied or changed: %s %v", local, err)
	}
	remote, err := externalFilePath(t.Context(), service, result, false)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(filepath.Dir(remote))
	data, err := os.ReadFile(remote)
	if err != nil || string(data) != "original binary\x00contents" || remote == local || filepath.Base(remote) != "报告.xlsx" || service.ref != "managed" {
		t.Fatalf("remote file did not use downloaded bytes: %s %v", remote, err)
	}
	result.File.Name = "../escape.xlsx"
	if _, err := externalFilePath(t.Context(), service, result, false); err == nil {
		t.Fatal("remote filename escaped temporary directory")
	}
}
