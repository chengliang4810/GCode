package contracts

import "context"

type GitHistoryService interface {
	ListGitHistory(context.Context, GitHistoryRequest) (GitHistoryResult, error)
}

type GitHistoryRequest struct {
	SessionID string `json:"sessionId"`
	ProjectID string `json:"projectId"`
}

type GitHistoryResult struct {
	Root       string   `json:"root"`
	Repository bool     `json:"repository"`
	Commits    []Commit `json:"commits"`
	Truncated  bool     `json:"truncated"`
}
