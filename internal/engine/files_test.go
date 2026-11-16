package engine

import (
	"testing"

	"gcode/internal/contracts"
)

func TestWorkspaceSearchFiltersHiddenPathSegments(t *testing.T) {
	core := &Engine{state: contracts.Snapshot{Files: []contracts.File{
		{ID: "visible", Path: "notes/report.txt", Name: "report.txt"},
		{ID: "hidden-parent", Path: ".private/report.txt", Name: "report.txt"},
		{ID: "nested-hidden", Path: "notes/.private/report.txt", Name: "report.txt"},
		{ID: "windows-hidden", Path: `notes\.private\report.txt`, Name: "report.txt"},
		{ID: "hidden-name", Path: "notes/.report", Name: ".report"},
		{ID: "dotted-name", Path: "notes.v2/report.txt", Name: "report.txt"},
	}}}
	for _, include := range []bool{false, true} {
		result, err := core.SearchWorkspace(t.Context(), contracts.WorkspaceSearchRequest{
			Query: "report", NamesOnly: true, IncludeHidden: include,
		})
		want := 2
		if include {
			want = 6
		}
		if err != nil || len(result.Files) != want {
			t.Fatalf("IncludeHidden=%v: %+v, %v", include, result.Files, err)
		}
	}
}
