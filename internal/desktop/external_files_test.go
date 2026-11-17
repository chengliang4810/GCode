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

type resolvingFileReader struct {
	contracts.Service
	requests  []contracts.LinkedFileRequest
	result    contracts.LinkedFileResult
	resources map[string][]byte
}

func (s *resolvingFileReader) ReadLinkedFile(_ context.Context, request contracts.LinkedFileRequest) (contracts.LinkedFileResult, error) {
	s.requests = append(s.requests, request)
	return s.result, nil
}

func (s *resolvingFileReader) ReadResource(_ context.Context, ref string) ([]byte, error) {
	return s.resources[ref], nil
}

// A preview opens this machine's file in place and downloads a remote host's
// resource first, so the embedded browser never reads a host path.
func TestResolveWorkspaceFileReadsOnlyRemoteResources(t *testing.T) {
	service := &resolvingFileReader{
		result: contracts.LinkedFileResult{
			File:         contracts.File{Name: "index.html", Kind: contracts.FilePreview, AssetRef: "downloaded"},
			AbsolutePath: "/host-only/index.html",
		},
		resources: map[string][]byte{"downloaded": []byte("<h1>预览</h1>"), "existing": []byte("<h1>已缓存</h1>")},
	}
	result := contracts.LinkedFileResult{
		File:         contracts.File{Name: "index.html", Kind: contracts.FilePreview},
		AbsolutePath: "/host-only/index.html",
	}
	local, err := resolveWorkspaceFile(t.Context(), service, "session", result, true)
	if err != nil || local != filepath.FromSlash(result.AbsolutePath) || len(service.requests) != 0 {
		t.Fatalf("local preview was copied or resolved remotely: %s %v", local, err)
	}
	remote, err := resolveWorkspaceFile(t.Context(), service, "session", result, false)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(filepath.Dir(remote))
	data, err := os.ReadFile(remote)
	if err != nil || string(data) != "<h1>预览</h1>" || remote == local || filepath.Base(remote) != "index.html" {
		t.Fatalf("remote preview did not use read bytes: %s %v", remote, err)
	}
	if len(service.requests) != 1 || service.requests[0].SessionID != "session" || service.requests[0].Path != "/host-only/index.html" {
		t.Fatalf("preview read the wrong source: %+v", service.requests)
	}
	result.File.AssetRef = "existing"
	cached, err := resolveWorkspaceFile(t.Context(), service, "session", result, false)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(filepath.Dir(cached))
	if data, err := os.ReadFile(cached); err != nil || string(data) != "<h1>已缓存</h1>" || len(service.requests) != 1 {
		t.Fatalf("existing resource was re-read: %s %v %+v", data, err, service.requests)
	}
}
