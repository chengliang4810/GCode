package engine

import (
	"os"
	"path/filepath"
	"testing"

	"gcode/internal/contracts"
)

func TestSessionFilesUseExactCWD(t *testing.T) {
	for _, projectBound := range []bool{false, true} {
		name := "unbound"
		if projectBound {
			name = "different-checkout"
		}
		t.Run(name, func(t *testing.T) {
			core, projectRoot, session := linkedFileTestCore(t)
			cwd := t.TempDir()
			for _, root := range []string{cwd, projectRoot} {
				if err := os.Mkdir(filepath.Join(root, "docs"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(cwd, "docs", "current.txt"), []byte("current"), 0600); err != nil {
				t.Fatal(err)
			}
			core.mu.Lock()
			index := core.sessionIndex(session.ID)
			core.state.Sessions[index].CWD = cwd
			if !projectBound {
				core.state.Sessions[index].ProjectID = ""
			}
			core.mu.Unlock()
			result, err := core.ListSessionFiles(t.Context(), contracts.SessionFilesRequest{SessionID: session.ID})
			if err != nil {
				t.Fatal(err)
			}
			if result.Root != cwd || len(result.Files) != 2 {
				t.Fatalf("wrong directory: %+v", result)
			}
			if result.Files[0].Name != "docs" || result.Files[1].ParentID != result.Files[0].ID {
				t.Fatalf("directory hierarchy lost: %+v", result.Files)
			}
			if _, err := core.ListSessionFiles(t.Context(), contracts.SessionFilesRequest{SessionID: "missing"}); err == nil {
				t.Fatal("unknown session was accepted")
			}
		})
	}
}
