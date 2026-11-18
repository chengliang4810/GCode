package workbench

import (
	"testing"

	"gcode/internal/contracts"

	"github.com/egoist/mygo/ui/editor"
)

func TestGroupedChanges(t *testing.T) {
	s := New(nil, nil)
	defer s.Close()
	changes := []contracts.Change{
		{ID: "first", ProjectID: "a", Path: "file.go", OldContent: "one\nkeep\ntwo\n", NewContent: "temp\nkeep\ntwo\n"},
		{ID: "other", ProjectID: "b", Path: "file.go", NewContent: "other\n"},
		{ID: "second", ProjectID: "a", Path: "file.go", OldContent: "temp\nkeep\ntwo\n", NewContent: "one\nkeep\nthree\n"},
	}
	s.Update(contracts.Snapshot{Changes: changes})
	rows := s.groupedChanges()
	if len(rows) != 2 || rows[0].ID != "first" || rows[1].ID != "other" {
		t.Fatalf("unexpected file groups: %+v", rows)
	}
	if added, deleted := rows[0].stats(); added != 1 || deleted != 1 {
		t.Fatalf("net stats = +%d -%d, want +1 -1", added, deleted)
	}
	s.OpenChange("first")
	d := s.diffs["first"]
	if d.Old.String() != changes[0].OldContent || d.New.String() != changes[2].NewContent {
		t.Fatal("diff does not use the first and last file versions")
	}
	if changes[0].NewContent != "temp\nkeep\ntwo\n" {
		t.Fatal("grouping mutated a per-edit record")
	}

	restored := contracts.Change{ID: "restore", ProjectID: "a", Path: "file.go", NewContent: changes[0].OldContent}
	s.Update(contracts.Snapshot{Changes: append(changes, restored)})
	row := s.fileChange("first")
	if added, deleted := row.stats(); added != 0 || deleted != 0 {
		t.Fatalf("restored file stats = +%d -%d", added, deleted)
	}
	d.Layout = editor.DiffSplit
	s.OpenChange("first")
	if s.diffs["first"] != d || d.Layout != editor.DiffSplit {
		t.Fatal("reopening reset the existing diff view")
	}
	if d.New.String() != d.Old.String() {
		t.Fatal("reopened diff still shows the previous file version")
	}
}
