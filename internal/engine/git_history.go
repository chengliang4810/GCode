package engine

import (
	"context"
	"errors"
	"strings"
	"time"

	"gcode/internal/contracts"
)

func (e *Engine) ListGitHistory(ctx context.Context, request contracts.GitHistoryRequest) (contracts.GitHistoryResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	e.mu.Lock()
	if err := e.checkLocked(ctx); err != nil {
		e.mu.Unlock()
		return contracts.GitHistoryResult{}, err
	}
	directory := ""
	if request.SessionID != "" {
		index := e.sessionIndex(request.SessionID)
		if index < 0 {
			e.mu.Unlock()
			return contracts.GitHistoryResult{}, errors.New("session not found")
		}
		directory = e.state.Sessions[index].CWD
	} else {
		for _, project := range e.state.Projects {
			if project.ID == request.ProjectID {
				directory = project.Path
				break
			}
		}
	}
	e.mu.Unlock()
	if directory == "" {
		return contracts.GitHistoryResult{}, errors.New("working directory is unavailable")
	}
	return readGitHistory(ctx, directory)
}

func readGitHistory(ctx context.Context, directory string) (contracts.GitHistoryResult, error) {
	result := contracts.GitHistoryResult{}
	root, err := gitOutput(ctx, directory, "rev-parse", "--show-toplevel")
	if err != nil {
		if strings.Contains(err.Error(), "not a git repository") {
			return result, nil
		}
		return result, err
	}
	result.Root, result.Repository = root, true
	output, err := gitOutput(ctx, directory, "log", "--no-color", "--no-show-signature", "--decorate=short", "--all", "--topo-order", "--max-count=501", "--format=%H%x00%P%x00%s%x00%an%x00%aI%x00%D%x00")
	if err != nil {
		return result, err
	}
	if output == "" {
		return result, nil
	}
	fields := strings.Split(output, "\x00")
	for len(fields) >= 6 {
		date, err := time.Parse(time.RFC3339, fields[4])
		if err != nil {
			return result, err
		}
		commit := contracts.Commit{SHA: strings.TrimSpace(fields[0]), Parents: strings.Fields(fields[1]), Subject: fields[2], Author: fields[3], AuthoredAt: date}
		for _, ref := range strings.Split(fields[5], ",") {
			if ref = strings.TrimSpace(ref); ref != "" {
				commit.Refs = append(commit.Refs, ref)
			}
		}
		result.Commits = append(result.Commits, commit)
		fields = fields[6:]
	}
	if len(result.Commits) > 500 {
		result.Truncated = true
		result.Commits = result.Commits[:500]
	}
	return result, nil
}
