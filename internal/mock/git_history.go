package mock

import (
	"context"
	"gcode/internal/contracts"
)

func (s *Server) ListGitHistory(ctx context.Context, request contracts.GitHistoryRequest) (contracts.GitHistoryResult, error) {
	snapshot, err := s.Snapshot(ctx)
	return contracts.GitHistoryResult{Repository: true, Commits: snapshot.Commits}, err
}
