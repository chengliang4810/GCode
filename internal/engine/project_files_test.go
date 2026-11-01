package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/storage"
)

func projectFilesTestCore(t *testing.T) *Engine {
	t.Helper()
	store, err := storage.Open(storage.Options{Directory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := Open(Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := core.Close(); err != nil {
			t.Error(err)
		}
	})
	return core
}

func waitForProjectFiles(t *testing.T, core *Engine) contracts.Snapshot {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		state, err := core.Snapshot(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		loading := false
		for _, project := range state.Projects {
			loading = loading || project.FilesState == contracts.LoadLoading
		}
		if !loading {
			return state
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("project file loading did not reach a terminal state")
	return contracts.Snapshot{}
}

func refreshProjectFilesForTest(t *testing.T, core *Engine) {
	t.Helper()
	core.mu.Lock()
	core.startProjectFilesLocked(core.state.Projects[0])
	core.mu.Unlock()
	waitForProjectFiles(t, core)
}

func TestProjectCreationCommitsBeforeBackgroundFileLoading(t *testing.T) {
	core := projectFilesTestCore(t)
	workspace := t.TempDir()
	started, release := make(chan struct{}), make(chan struct{})
	core.projectScanner = func(ctx context.Context, project contracts.Project) ([]contracts.File, error) {
		close(started)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return []contracts.File{{ID: project.ID + ":ready.txt", ProjectID: project.ID, Path: "ready.txt", Kind: "file"}}, nil
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	if err := core.Execute(ctx, contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace}}); err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel() // An accepted scan belongs to the host, not the request.
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("background collector never started")
	}
	state, err := core.Snapshot(t.Context())
	if err != nil || len(state.Projects) != 1 || state.Projects[0].FilesState != contracts.LoadLoading {
		t.Fatalf("project creation waited for collection: %+v, %v", state.Projects, err)
	}
	if data, err := os.ReadFile(filepath.Join(core.storage.ConfigDirectory(), "engine.json")); err != nil || len(data) == 0 {
		t.Fatalf("project registration was not committed: %v", err)
	}
	settings := state.Settings
	settings.General.Language = "zh-CN"
	if err := core.Execute(t.Context(), contracts.Command{Kind: "settings.update", Settings: &settings}); err != nil {
		t.Fatal("file loading blocked another core command:", err)
	}
	close(release)
	state = waitForProjectFiles(t, core)
	if len(state.Files) != 1 || state.Projects[0].FilesState != contracts.LoadReady {
		t.Fatalf("background files were not published: %+v, %+v", state.Projects, state.Files)
	}
}

func TestProjectFileLoadingFailureDoesNotRollbackRegistration(t *testing.T) {
	core := projectFilesTestCore(t)
	core.projectScanner = func(ctx context.Context, project contracts.Project) ([]contracts.File, error) {
		return []contracts.File{{ID: project.ID + ":partial", ProjectID: project.ID, Path: "partial", Kind: "file"}}, errors.New("fixture file enumeration limit")
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: t.TempDir()}}); err != nil {
		t.Fatal(err)
	}
	state := waitForProjectFiles(t, core)
	if len(state.Projects) != 1 || len(state.Files) != 1 || state.Projects[0].FilesState != contracts.LoadError || state.Projects[0].FilesError == "" {
		t.Fatalf("scan failure lost the project or its diagnostic: %+v", state.Projects)
	}
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(storage.Options{Directory: core.storage.ConfigDirectory()})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored, err := reopened.Snapshot(t.Context())
	if err != nil || len(restored.Projects) != 1 || restored.Projects[0].FilesState != contracts.LoadError {
		t.Fatalf("scan failure registration did not survive restart: %+v %v", restored.Projects, err)
	}
}

func TestProjectFileLoadingCancelsOnDeleteAndShutdown(t *testing.T) {
	for _, action := range []string{"delete", "shutdown"} {
		t.Run(action, func(t *testing.T) {
			core := projectFilesTestCore(t)
			started, canceled := make(chan struct{}), make(chan struct{})
			core.projectScanner = func(ctx context.Context, project contracts.Project) ([]contracts.File, error) {
				close(started)
				<-ctx.Done()
				close(canceled)
				return nil, ctx.Err()
			}
			if err := core.Execute(t.Context(), contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: t.TempDir()}}); err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("collector did not start")
			}
			if action == "shutdown" {
				if err := core.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				state, _ := core.Snapshot(t.Context())
				if err := core.Execute(t.Context(), contracts.Command{Kind: "project.delete", ID: state.Projects[0].ID}); err != nil {
					t.Fatal(err)
				}
				core.wg.Wait()
				state, _ = core.Snapshot(t.Context())
				if len(state.Projects) != 0 || len(state.Files) != 0 {
					t.Fatal("deleted project reappeared from a late scan")
				}
			}
			select {
			case <-canceled:
			default:
				t.Fatal("host did not cancel its file collector")
			}
		})
	}
}

func TestProjectFileRefreshDiscardsSupersededResults(t *testing.T) {
	core := projectFilesTestCore(t)
	started, release := make(chan struct{}), make(chan struct{})
	var count atomic.Int32
	core.projectScanner = func(ctx context.Context, project contracts.Project) ([]contracts.File, error) {
		name := "current"
		if count.Add(1) == 1 {
			close(started)
			<-release // Emulate a slow filesystem call that ignores cancellation.
			name = "obsolete"
		}
		return []contracts.File{{ID: project.ID + ":" + name, ProjectID: project.ID, Path: name, Kind: "file"}}, nil
	}
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	if err := core.Execute(t.Context(), contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: t.TempDir()}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("collector did not start")
	}
	state, _ := core.Snapshot(t.Context())
	if err := core.Execute(t.Context(), contracts.Command{Kind: "project.files.refresh", ID: state.Projects[0].ID}); err != nil {
		t.Fatal(err)
	}
	waitForProjectFiles(t, core)
	close(release)
	core.wg.Wait()
	state, _ = core.Snapshot(t.Context())
	if len(state.Files) != 1 || state.Files[0].Path != "current" {
		t.Fatalf("obsolete scan replaced the refreshed tree: %+v", state.Files)
	}
}

func TestProjectFileCollectionHonorsGitIgnoredArtifacts(t *testing.T) {
	root := temporaryGitRepository(t)
	for name, text := range map[string]string{
		"tracked.tmp": "tracked content",
		"keep.tmp":    "included by negation",
		"skip.tmp":    "ignored content",
		"source.go":   "package source",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := gitOutput(t.Context(), root, "add", "tracked.tmp"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("/output/\n*.tmp\n!keep.tmp\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "output", "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "output", "nested", "artifact"), []byte("large generated tree"), 0600); err != nil {
		t.Fatal(err)
	}
	files, err := collectProjectFiles(t.Context(), contracts.Project{ID: "fixture", Path: root})
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, file := range files {
		found[file.Path] = true
	}
	for _, name := range []string{"source.go", "keep.tmp", "tracked.tmp"} {
		if !found[name] {
			t.Fatalf("dropped included source %q: %+v", name, found)
		}
	}
	for _, name := range []string{"skip.tmp", "output", "output/nested", "output/nested/artifact"} {
		if found[name] {
			t.Fatalf("ignored build artifact entered the tree: %q", name)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := collectProjectFiles(ctx, contracts.Project{Path: root}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled enumeration returned %v", err)
	}
}

func TestProjectLinkedFileOwnershipDoesNotDependOnBackgroundIndex(t *testing.T) {
	core := projectFilesTestCore(t)
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "source.txt"), []byte("project contents"), 0600); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	core.projectScanner = func(ctx context.Context, project contracts.Project) ([]contracts.File, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return []contracts.File{}, nil // No index entry for this on-demand file.
		}
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace}}); err != nil {
		t.Fatal(err)
	}
	state, _ := core.Snapshot(t.Context())
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create", ProjectID: state.Projects[0].ID}); err != nil {
		t.Fatal(err)
	}
	state, _ = core.Snapshot(t.Context())
	result, err := core.ReadLinkedFile(t.Context(), contracts.LinkedFileRequest{SessionID: state.Sessions[0].ID, Path: "source.txt"})
	if err != nil || result.File.ReadOnly || result.File.ProjectID != state.Projects[0].ID {
		t.Fatalf("pending index changed project ownership: %+v %v", result, err)
	}
	close(release)
	state = waitForProjectFiles(t, core)
	if len(state.Files) != 1 || !state.Files[0].Loaded || state.Files[0].Content != "project contents" {
		t.Fatalf("background scan discarded the on-demand editor buffer: %+v", state.Files)
	}
}
