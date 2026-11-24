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

func TestDirectoryMockCreateFolder(t *testing.T) {
	s := directoryTestServer()
	ctx := context.Background()
	created, err := s.CreateDirectory(ctx, contracts.DirectoryCreateRequest{DeviceID: "local", Parent: "/workspace", Name: "fresh"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Path != "/workspace/fresh" || created.DeviceID != "local" || len(created.Entries) != 0 {
		t.Fatalf("created listing: %+v", created)
	}
	parent, err := s.ListDirectories(ctx, contracts.DirectoryRequest{DeviceID: "local", Path: "/workspace"})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, entry := range parent.Entries {
		if entry.Name == "fresh" && entry.Path == "/workspace/fresh" {
			found = true
		}
	}
	if !found {
		t.Fatalf("created folder missing from its parent: %+v", parent.Entries)
	}
	if _, err := s.CreateDirectory(ctx, contracts.DirectoryCreateRequest{DeviceID: "local", Parent: "/workspace", Name: "fresh"}); err == nil {
		t.Fatal("the same folder name was created twice")
	}
	for _, name := range []string{"", "  ", ".", "..", "nested/folder"} {
		if _, err := s.CreateDirectory(ctx, contracts.DirectoryCreateRequest{DeviceID: "local", Parent: "/workspace", Name: name}); err == nil {
			t.Fatalf("name %q was accepted", name)
		}
	}
	if _, err := s.CreateDirectory(ctx, contracts.DirectoryCreateRequest{DeviceID: "offline", Parent: "/workspace", Name: "offline-folder"}); err == nil {
		t.Fatal("a folder was created for an offline device")
	}
	if _, err := s.CreateDirectory(ctx, contracts.DirectoryCreateRequest{DeviceID: "local", Parent: "/missing", Name: "orphan"}); err == nil {
		t.Fatal("a folder was created inside a missing parent")
	}
}

func TestDirectoryMockCreatedFolderBecomesProject(t *testing.T) {
	s, err := New(Options{Instant: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	created, err := s.CreateDirectory(ctx, contracts.DirectoryCreateRequest{DeviceID: "local", Parent: "~/projects", Name: "brand-new"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Path != "~/projects/brand-new" {
		t.Fatalf("created folder path: %q", created.Path)
	}
	if err := s.Execute(ctx, contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{DeviceID: "local", Path: created.Path, Name: "Brand new"}}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var registered bool
	for _, project := range snapshot.Projects {
		if project.Path == created.Path && project.DeviceID == "local" {
			registered = true
		}
	}
	if !registered {
		t.Fatalf("folder was not registered as a project: %+v", snapshot.Projects)
	}
	parent, err := s.ListDirectories(ctx, contracts.DirectoryRequest{DeviceID: "local", Path: "~/projects"})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range parent.Entries {
		if entry.Path == created.Path && !entry.Project {
			t.Fatalf("registered folder lost its project marker: %+v", entry)
		}
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
