package engine

import (
	"context"
	"errors"
	"time"

	"gcode/internal/contracts"
)

func (e *Engine) ListSessionFiles(ctx context.Context, request contracts.SessionFilesRequest) (contracts.SessionFilesResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	e.mu.Lock()
	if err := e.checkLocked(ctx); err != nil {
		e.mu.Unlock()
		return contracts.SessionFilesResult{}, err
	}
	index := e.sessionIndex(request.SessionID)
	if index < 0 {
		e.mu.Unlock()
		return contracts.SessionFilesResult{}, errors.New("session not found")
	}
	session := e.state.Sessions[index]
	e.mu.Unlock()
	if session.CWD == "" {
		return contracts.SessionFilesResult{}, errors.New("session working directory is unavailable")
	}
	files, err := collectProjectFiles(ctx, contracts.Project{ID: "session:" + session.ID, Path: session.CWD})
	for i := range files {
		files[i].ProjectID = session.ProjectID
		files[i].ReadOnly = true
	}
	return contracts.SessionFilesResult{Root: session.CWD, Files: files}, err
}
