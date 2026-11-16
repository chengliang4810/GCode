package contracts

import "context"

// SessionFilesService lists the owning host's current session working directory.
type SessionFilesService interface {
	ListSessionFiles(context.Context, SessionFilesRequest) (SessionFilesResult, error)
}

type SessionFilesRequest struct {
	SessionID string `json:"sessionId"`
}

type SessionFilesResult struct {
	Root  string `json:"root"`
	Files []File `json:"files"`
}
