package engine

import (
	"context"
	"log"
	"slices"
	"time"

	"gcode/internal/contracts"
)

type projectScan struct {
	cancel context.CancelFunc
}

// The host owns accepted scans; a client disconnect does not cancel them.
// Collection never holds the engine mutex or participates in project creation.
func (e *Engine) startProjectFilesLocked(project contracts.Project) {
	if e.closed {
		return
	}
	if old := e.projectScans[project.ID]; old != nil {
		old.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	scan := &projectScan{cancel: cancel}
	e.projectScans[project.ID] = scan
	index := slices.IndexFunc(e.state.Projects, func(p contracts.Project) bool { return p.ID == project.ID })
	if index < 0 {
		cancel()
		delete(e.projectScans, project.ID)
		return
	}
	e.state.Projects[index].FilesState = contracts.LoadLoading
	e.state.Projects[index].FilesError = ""
	collect := e.projectScanner
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer cancel()
		repository, _ := inspectGit(ctx, project.Path)
		files, scanErr := collect(ctx, project)
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.closed || e.projectScans[project.ID] != scan {
			return
		}
		delete(e.projectScans, project.ID)
		index := slices.IndexFunc(e.state.Projects, func(p contracts.Project) bool {
			return p.ID == project.ID && p.Path == project.Path
		})
		if index < 0 {
			return
		}
		e.applyGitProjectLocked(project.ID, repository)
		if files != nil || scanErr == nil {
			e.applyProjectFilesLocked(project, files)
		}
		e.state.Projects[index].FilesState = contracts.LoadReady
		if scanErr != nil {
			e.state.Projects[index].FilesState = contracts.LoadError
			e.state.Projects[index].FilesError = scanErr.Error()
			log.Printf("project file loading failed: project=%s path=%q: %v", project.ID, project.Path, scanErr)
		}
		if err := e.persistLocked(); err != nil {
			e.state.Projects[index].FilesState = contracts.LoadError
			e.state.Projects[index].FilesError = "Cannot save loaded file list: " + err.Error()
			log.Printf("project file loading persistence failed: project=%s: %v", project.ID, err)
		}
		e.publishLocked(contracts.Event{Kind: "project.files.changed", ResourceID: project.ID})
	}()
}
