package engine

import (
	"errors"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func TestFileIOAllowsPriorSnapshotsAndSerializesMutations(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			core := openStorageTestCore(t, t.TempDir())
			defer core.Close()
			core.mu.Lock()
			core.state.Projects = []contracts.Project{{ID: "project"}}
			core.state.Files = []contracts.File{{ID: "file", ProjectID: "project", Content: "before"}}
			core.mu.Unlock()
			started, release := make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			done := make(chan error, 1)
			injected := errors.New("read failed")
			go func() {
				core.mu.Lock()
				defer core.mu.Unlock()
				done <- core.fileCommandWithIOLocked(contracts.Command{Kind: "file.open", ID: "file"}, func(_ contracts.Command, _ contracts.Project, file contracts.File, exists bool) (fileCommandResult, error) {
					close(started)
					<-release
					if fail {
						return fileCommandResult{}, injected
					}
					file.Content = "after"
					return fileCommandResult{file: file, updated: exists}, nil
				})
			}()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("file I/O did not start")
			}
			snapshots := make(chan contracts.Snapshot, 1)
			go func() { state, _ := core.Snapshot(t.Context()); snapshots <- state }()
			select {
			case state := <-snapshots:
				if len(state.Files) != 1 || state.Files[0].Content != "before" {
					t.Fatalf("exposed pending file state: %+v", state.Files)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("snapshot blocked on file I/O")
			}
			if core.mu.TryLock() {
				core.mu.Unlock()
				t.Fatal("mutation entered in-flight file operation")
			}
			close(release)
			err := <-done
			if (fail && !errors.Is(err, injected)) || (!fail && err != nil) {
				t.Fatalf("operation: %v", err)
			}
			core.mu.Lock()
			actual := core.state.Files[0].Content
			core.mu.Unlock()
			want := "after"
			if fail {
				want = "before"
			}
			if actual != want {
				t.Fatalf("file state = %q, want %q", actual, want)
			}
		})
	}
}
