package mock

import (
	"context"
	"errors"
	"testing"

	"gcode/internal/contracts"
)

func TestWorkspaceSearchUsesMutableFilesAndByteLocations(t *testing.T) {
	s, err := New(Options{Instant: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	request := contracts.WorkspaceSearchRequest{ProjectID: "fieldnotes", Query: "layout", FileLimit: 50, ContentLimit: 100, MatchesPerFile: 3}
	result, err := s.SearchWorkspace(ctx, request)
	if err != nil || len(result.Files) == 0 {
		t.Fatalf("name search: %+v %v", result, err)
	}
	if result.Files[0].FileID != "file-layout" {
		t.Fatalf("wrong file identity: %+v", result.Files)
	}
	if err := s.Execute(ctx, contracts.Command{Kind: "file.save", ID: "file-layout", Version: 1, Text: "package layout\n// 中文 İNDEX needle\nneedle\nneedle\nneedle\n"}); err != nil {
		t.Fatal(err)
	}
	request.Query = "index needle"
	result, err = s.SearchWorkspace(ctx, request)
	if err != nil || len(result.Content) != 1 {
		t.Fatalf("content search: %+v %v", result, err)
	}
	match := result.Content[0]
	if match.Line != 2 || match.Column != 10 || match.Text[match.Ranges[0].Start:match.Ranges[0].End] != "İNDEX" {
		t.Fatalf("UTF8 original byte location: %+v", match)
	}
	request.Query = "needle"
	result, err = s.SearchWorkspace(ctx, request)
	if err != nil || len(result.Content) != 3 {
		t.Fatalf("per-file cap: %+v %v", result, err)
	}
	request.ProjectID = "zeron"
	result, err = s.SearchWorkspace(ctx, request)
	if err != nil || len(result.Content) != 0 {
		t.Fatalf("cross-project leak: %+v %v", result, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = s.SearchWorkspace(cancelled, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled search: %v", err)
	}
}
func TestDirectoryBrowseAndDeviceScopedProjectCreation(t *testing.T) {
	s, err := New(Options{Instant: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	listing, err := s.ListDirectories(ctx, contracts.DirectoryRequest{DeviceID: "local"})
	if err != nil || len(listing.Entries) < 2 {
		t.Fatalf("home listing: %+v %v", listing, err)
	}
	if _, err = s.ListDirectories(ctx, contracts.DirectoryRequest{DeviceID: "remote"}); err == nil {
		t.Fatal("offline browse succeeded")
	}
	if _, err = s.ListDirectories(ctx, contracts.DirectoryRequest{DeviceID: "local", Path: "/not-in-virtual-workspace"}); err == nil {
		t.Fatal("invented directory exposed")
	}
	if _, err = s.ListDirectories(ctx, contracts.DirectoryRequest{DeviceID: "local", Path: "~/projects/fieldnotes/internal"}); err != nil {
		t.Fatal(err)
	}
	request := contracts.ProjectCreate{DeviceID: "macbook", Name: "Fieldnotes", Path: "~/projects/fieldnotes"}
	if err = s.Execute(ctx, contracts.Command{Kind: "project.create", ProjectCreate: &request}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := s.Snapshot(ctx)
	found := false
	for _, project := range snapshot.Projects {
		if project.DeviceID == "macbook" && project.Path == request.Path {
			found = true
		}
	}
	if !found {
		t.Fatal("selected project device was discarded")
	}
	request.DeviceID = "remote"
	if err = s.Execute(ctx, contracts.Command{Kind: "project.create", ProjectCreate: &request}); err == nil {
		t.Fatal("created project on offline device")
	}
	request.DeviceID = "macbook"
	if err = s.Execute(ctx, contracts.Command{Kind: "project.create", ProjectCreate: &request}); err == nil {
		t.Fatal("duplicate device/path accepted")
	}
}
