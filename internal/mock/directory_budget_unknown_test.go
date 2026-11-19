package mock

import (
	"context"
	"testing"

	"gcode/internal/contracts"
)

func TestDirectoryMockDoesNotClaimMissingBeyondScanBudget(t *testing.T) {
	s := directoryTestServer()
	for range contracts.DirectoryScanLimit - 1 {
		s.state.Files = append(s.state.Files, contracts.File{ProjectID: "project", Path: "earlier/file.go"})
	}
	s.state.Files = append(s.state.Files, contracts.File{ProjectID: "project", Path: "beyond-limit", Kind: "directory"})
	request := contracts.DirectoryRequest{DeviceID: "local", Path: "/workspace/project/beyond-limit"}
	listing, err := s.ListDirectories(context.Background(), request)
	if err != nil || !listing.Truncated || listing.Path != request.Path || len(listing.Entries) != 0 {
		t.Fatalf("budget exhaustion incorrectly claims a missing path: %+v, %v", listing, err)
	}
	s.state.Files = nil
	if _, err := s.ListDirectories(context.Background(), request); err == nil {
		t.Fatal("a fully scanned missing folder should still be rejected")
	}
}
