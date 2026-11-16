package workbench

import (
	"context"
	"strings"
	"time"

	"gcode/internal/contracts"
)

type sessionFileTree struct {
	active     bool
	key        string
	files      []contracts.File
	loading    bool
	error      string
	cancel     context.CancelFunc
	generation uint64
	updatedAt  time.Time
}

// RefreshFiles reloads the session directory when an explorer is reopened.
func (s *State) RefreshFiles() {
	s.sessionFiles.key = ""
	s.refreshSessionFiles()
}

func (s *State) explorerFiles() []contracts.File {
	if s.sessionFiles.active {
		return s.sessionFiles.files
	}
	if s.ProjectID == "" {
		return nil
	}
	return s.snapshot.Files
}

func (s *State) refreshSessionFiles() {
	var session contracts.Session
	for _, candidate := range s.snapshot.Sessions {
		if candidate.ID == s.scopeID {
			session = candidate
			break
		}
	}
	active := session.ID != "" && session.CWD != ""
	for _, project := range s.snapshot.Projects {
		if project.ID == session.ProjectID && project.Path == session.CWD {
			active = false
			break
		}
	}
	key := ""
	if active {
		key = session.ID + "\x00" + session.CWD
	}
	sameRoot := s.sessionFiles.key == key && s.sessionFiles.active == active
	if sameRoot && (!active || session.Status == contracts.Working || session.UpdatedAt.Equal(s.sessionFiles.updatedAt)) {
		return
	}
	if s.sessionFiles.cancel != nil {
		s.sessionFiles.cancel()
	}
	generation := s.sessionFiles.generation + 1
	var files []contracts.File
	if sameRoot {
		files = s.sessionFiles.files
	}
	s.sessionFiles = sessionFileTree{active: active, key: key, generation: generation, files: files, updatedAt: session.UpdatedAt}
	s.fileChildrenIndex = nil
	if !active {
		return
	}
	service, ok := s.backend.(contracts.SessionFilesService)
	if !ok {
		s.sessionFiles.error = "Session files are unavailable"
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.sessionFiles.cancel, s.sessionFiles.loading = cancel, true
	finish := func(result contracts.SessionFilesResult, err error) {
		if s.closed.Load() || ctx.Err() != nil || s.sessionFiles.generation != generation {
			return
		}
		defer cancel()
		s.sessionFiles.cancel, s.sessionFiles.loading = nil, false
		s.sessionFiles.files = result.Files
		s.fileChildrenIndex = nil
		if err != nil {
			s.sessionFiles.error = err.Error()
		}
		if strings.TrimSpace(s.fileSearch) != "" {
			s.refreshFileSearch()
		}
	}
	request := contracts.SessionFilesRequest{SessionID: session.ID}
	if s.win == nil {
		result, err := service.ListSessionFiles(ctx, request)
		finish(result, err)
		return
	}
	win := s.win
	go func() {
		result, err := service.ListSessionFiles(ctx, request)
		if ctx.Err() == nil {
			win.Update(func() { finish(result, err) })
		}
	}()
}

func (s *State) openExplorerFile(file contracts.File) {
	if s.sessionFiles.active {
		if err := s.OpenLink(s.scopeID, file.Path); err != nil {
			s.Error = err.Error()
		}
		return
	}
	s.OpenFile(file.ID)
}
