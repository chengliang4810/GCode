package workbench

import (
	"context"

	"gcode/internal/contracts"
)

type gitHistoryState struct {
	key        string
	loading    bool
	error      string
	result     contracts.GitHistoryResult
	cancel     context.CancelFunc
	generation uint64
}

func (s *State) refreshGitHistory(force bool) {
	opened := false
	for _, tab := range s.tabs {
		if tab.kind == "history" {
			opened = true
			break
		}
	}
	if !opened {
		return
	}
	directory := ""
	for _, session := range s.snapshot.Sessions {
		if session.ID == s.scopeID {
			directory = session.CWD
			break
		}
	}
	if s.scopeID == "" {
		for _, project := range s.snapshot.Projects {
			if project.ID == s.ProjectID {
				directory = project.Path
				break
			}
		}
	}
	key := s.scopeID + "\x00" + s.ProjectID + "\x00" + directory
	if !force && s.gitHistory.key == key {
		return
	}
	if s.gitHistory.cancel != nil {
		s.gitHistory.cancel()
	}
	generation := s.gitHistory.generation + 1
	s.gitHistory = gitHistoryState{key: key, generation: generation}
	s.commitOpen = false
	service, ok := s.backend.(contracts.GitHistoryService)
	if !ok {
		s.gitHistory.error = s.t("Git history is unavailable")
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.gitHistory.loading, s.gitHistory.cancel = true, cancel
	finish := func(result contracts.GitHistoryResult, err error) {
		if s.closed.Load() || ctx.Err() != nil || s.gitHistory.generation != generation {
			return
		}
		defer cancel()
		s.gitHistory.loading, s.gitHistory.cancel = false, nil
		s.gitHistory.result = result
		if err != nil {
			s.gitHistory.error = err.Error()
		}
	}
	request := contracts.GitHistoryRequest{SessionID: s.scopeID, ProjectID: s.ProjectID}
	if s.win == nil {
		result, err := service.ListGitHistory(ctx, request)
		finish(result, err)
		return
	}
	win := s.win
	go func() {
		result, err := service.ListGitHistory(ctx, request)
		if ctx.Err() == nil {
			win.Update(func() { finish(result, err) })
		}
	}()
}
