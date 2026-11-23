package mock

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"gcode/internal/contracts"
)

func directoryTestServer() *Server {
	return &Server{state: contracts.Snapshot{
		Devices:  []contracts.Device{{ID: "local", Status: "online", Current: true}, {ID: "offline", Status: "offline"}},
		Projects: []contracts.Project{{ID: "project", DeviceID: "local", Path: "/workspace/project"}},
	}}
}

func TestDirectoryMockResultLimitsAndSorting(t *testing.T) {
	for _, count := range []int{499, 500, 501} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			s := directoryTestServer()
			for i := count - 1; i >= 0; i-- {
				name := fmt.Sprintf("dir-%04d", i)
				if i == 0 {
					name = ".hidden"
				}
				s.state.Files = append(s.state.Files, contracts.File{ProjectID: "project", Path: name + "/file.go"})
			}
			got, err := s.ListDirectories(context.Background(), contracts.DirectoryRequest{DeviceID: "local", Path: "/workspace/project"})
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Entries) != min(count, contracts.DirectoryResultLimit) || got.Truncated != (count > contracts.DirectoryResultLimit) {
				t.Fatalf("directory cap: count=%d, entries=%d, truncated=%v", count, len(got.Entries), got.Truncated)
			}
			if !slices.IsSortedFunc(got.Entries, func(a, b contracts.DirectoryEntry) int { return strings.Compare(a.Path, b.Path) }) {
				t.Fatal("virtual paths are not sorted")
			}
			if count <= contracts.DirectoryResultLimit && (got.Entries[0].Name != ".hidden" || !got.Entries[0].Hidden) {
				t.Fatalf("hidden directory metadata missing: %+v", got.Entries[0])
			}
		})
	}
}

func TestDirectoryMockScanBudgetIncludesProjectsAndUnrelatedFiles(t *testing.T) {
	s := directoryTestServer()
	// The project consumes one scan slot. Unrelated files consume slots too;
	// the final virtual directory must not slip past the total scan budget.
	for range contracts.DirectoryScanLimit - 1 {
		s.state.Files = append(s.state.Files, contracts.File{ProjectID: "unrelated", Path: "ignored.go"})
	}
	s.state.Files = append(s.state.Files, contracts.File{ProjectID: "project", Path: "beyond-limit", Kind: "directory"})
	got, err := s.ListDirectories(context.Background(), contracts.DirectoryRequest{DeviceID: "local", Path: "/workspace/project"})
	if err != nil || !got.Truncated || len(got.Entries) != 0 {
		t.Fatalf("scan budget: %+v, %v", got, err)
	}
	for len(s.state.Projects) < contracts.DirectoryScanLimit {
		s.state.Projects = append(s.state.Projects, contracts.Project{DeviceID: "offline", Path: "ignored"})
	}
	s.state.Projects = append(s.state.Projects, contracts.Project{DeviceID: "local", Path: "Z:/beyond-limit"})
	locations, err := s.ListDirectoryLocations(context.Background(), contracts.DirectoryRequest{DeviceID: "local"})
	if err != nil || !locations.Truncated || len(locations.Locations) != 2 || locations.Locations[1].Path != "/" {
		t.Fatalf("location scan budget: %+v, %v", locations, err)
	}
}

func TestDirectoryMockPreservesProjectMarkersAndDeviceIsolation(t *testing.T) {
	s := directoryTestServer()
	s.state.Projects = append(s.state.Projects,
		contracts.Project{ID: "nested", DeviceID: "local", Path: "/workspace/project/nested"},
		contracts.Project{ID: "foreign", DeviceID: "offline", Path: "/workspace/project/foreign"},
	)
	s.state.Files = []contracts.File{
		{ProjectID: "project", Path: "nested/file.go"},
		{ProjectID: "foreign", Path: "leak/file.go"},
	}
	request := contracts.DirectoryRequest{DeviceID: "local", Path: "/workspace/project"}
	got, err := s.ListDirectories(context.Background(), request)
	if err != nil || got.Truncated || len(got.Entries) != 1 || got.Entries[0].Name != "nested" || !got.Entries[0].Project {
		t.Fatalf("project marker/device isolation: %+v, %v", got, err)
	}
	for _, device := range []string{"offline", "unknown"} {
		request.DeviceID = device
		if _, err := s.ListDirectories(context.Background(), request); err == nil {
			t.Fatalf("browsed unavailable device %q", device)
		}
		if _, err := s.ListDirectoryLocations(context.Background(), request); err == nil {
			t.Fatalf("offered unavailable device %q", device)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request.DeviceID = "local"
	if _, err := s.ListDirectories(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled directory query: %v", err)
	}
	if _, err := s.ListDirectoryLocations(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled location query: %v", err)
	}
}
