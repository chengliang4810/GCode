package workbench

import (
	"testing"

	"gcode/internal/contracts"
)

func TestFileChildrenIndexRefreshesWithSnapshotAndVisibility(t *testing.T) {
	s, _, snapshot := testWorkspace(t)
	s.ProjectID = "one"
	snapshot.Settings.Files.ShowHidden = false
	snapshot.Files = []contracts.File{
		{ID: "a", ProjectID: "one", Name: "a.go", Path: "src/a.go", ParentID: "src"},
		{ID: "hidden", ProjectID: "one", Name: ".env", Path: "src/.env", ParentID: "src"},
		{ID: "b", ProjectID: "two", Name: "b.go", Path: "src/b.go", ParentID: "src"},
	}
	s.Update(snapshot)
	if got := s.filesIn("src"); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("initial children: %+v", got)
	}
	first := s.filesIn("src")
	if next := s.filesIn("src"); &first[0] != &next[0] {
		t.Fatal("unchanged tree rebuilt its children")
	}
	s.UpdateTranscript(snapshot)
	if next := s.filesIn("src"); &first[0] != &next[0] {
		t.Fatal("message-only update invalidated file children")
	}
	s.ProjectID = "two"
	if got := s.filesIn("src"); len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("project switch retained stale children: %+v", got)
	}
	s.ProjectID = "one"
	s.snapshot.Settings.Files.ShowHidden = true
	if got := s.filesIn("src"); len(got) != 2 {
		t.Fatalf("hidden setting retained stale children: %+v", got)
	}
	snapshot.Files = []contracts.File{{ID: "new", ProjectID: "one", Name: "new.go", Path: "src/new.go", ParentID: "src"}}
	s.Update(snapshot)
	if got := s.filesIn("src"); len(got) != 1 || got[0].ID != "new" {
		t.Fatalf("snapshot update retained stale children: %+v", got)
	}
	if got := s.filesIn("missing"); len(got) != 0 {
		t.Fatalf("missing directory has children: %+v", got)
	}
}
