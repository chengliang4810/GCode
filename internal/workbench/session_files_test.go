package workbench

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

type sessionFilesBackend struct {
	contracts.Service
	requests []string
	opened   contracts.LinkedFileRequest
}

func (b *sessionFilesBackend) ReadLinkedFile(_ context.Context, request contracts.LinkedFileRequest) (contracts.LinkedFileResult, error) {
	b.opened = request
	return contracts.LinkedFileResult{
		File:         contracts.File{ID: "opened:" + request.SessionID, Name: request.SessionID + ".txt", Path: request.Path, Kind: "file", Loaded: true, ReadOnly: true},
		AbsolutePath: "/sessions/" + request.SessionID + "/" + request.Path,
	}, nil
}

func (b *sessionFilesBackend) ListSessionFiles(_ context.Context, request contracts.SessionFilesRequest) (contracts.SessionFilesResult, error) {
	b.requests = append(b.requests, request.SessionID)
	id := request.SessionID
	return contracts.SessionFilesResult{Root: "/sessions/" + id, Files: []contracts.File{
		{ID: id + ":docs", Name: "docs", Path: "docs", Kind: "directory"},
		{ID: id + ":file", ParentID: id + ":docs", Name: id + ".txt", Path: "docs/" + id + ".txt", Kind: "file"},
	}}, nil
}

func TestSessionExplorerSeparatesRootsAndPreservesFolderState(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprint(dark), func(t *testing.T) {
			s, _, snapshot := testWorkspace(t)
			backend := &sessionFilesBackend{Service: s.backend}
			s.backend = backend
			snapshot.Sessions = []contracts.Session{{ID: "one", CWD: "/sessions/one"}, {ID: "two", CWD: "/sessions/two"}}
			snapshot.Files = []contracts.File{
				{ID: "other:docs", Name: "docs", Path: "docs", Kind: "directory", ProjectID: "other"},
				{ID: "another:docs", Name: "docs", Path: "docs", Kind: "directory", ProjectID: "another"},
			}
			snapshot.Settings.Appearance.Mode = "light"
			if dark {
				snapshot.Settings.Appearance.Mode = "dark"
			}
			s.Update(snapshot)
			s.SwitchSession("one")
			tt := ui.NewTester(s.FilesView, 320, 700)
			if roots := s.filesIn(""); len(roots) != 1 || roots[0].ID != "one:docs" {
				t.Fatalf("mixed project roots: %+v", roots)
			}
			if tt.HasText("one.txt") {
				t.Fatal("directory started expanded")
			}
			if err := tt.Click("docs"); err != nil {
				t.Fatal(err)
			}
			if !tt.HasText("one.txt") {
				t.Fatal("click did not expand directory")
			}
			if err := tt.Click("one.txt"); err != nil {
				t.Fatal(err)
			}
			if backend.opened.SessionID != "one" || backend.opened.Path != "docs/one.txt" {
				t.Fatalf("file opened against wrong session: %+v", backend.opened)
			}
			requests := len(backend.requests)
			snapshot.Sessions[0].Status = contracts.Working
			snapshot.Sessions[0].UpdatedAt = time.Unix(10, 0)
			s.Update(snapshot)
			if len(backend.requests) != requests {
				t.Fatal("streaming update rescanned the directory")
			}
			snapshot.Sessions[0].Status = contracts.Idle
			s.Update(snapshot)
			if len(backend.requests) != requests+1 {
				t.Fatal("completed task did not refresh generated files")
			}
			s.SwitchSession("two")
			tt.Frame()
			if tt.HasText("one.txt") || tt.HasText("two.txt") || len(s.filesIn("")) != 1 {
				t.Fatal("switch leaked files or expansion state")
			}
			s.fileSearch = "two.txt"
			s.refreshFileSearch()
			if len(s.search.result.Files) != 1 || s.search.result.Files[0].FileID != "two:file" {
				t.Fatal("search did not use current session")
			}
			s.SwitchSession("one")
			tt.Frame()
			if !tt.HasText("one.txt") || tt.HasText("two.txt") {
				t.Fatal("return did not restore only the first session's folder state")
			}
			s.ProjectID = ""
			s.SwitchSession("")
			if len(s.filesIn("")) != 0 {
				t.Fatal("empty canvas aggregated all projects")
			}
		})
	}
}
