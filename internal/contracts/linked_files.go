package contracts

import "context"

// LinkedFileService reads a link on the owning host without routing a local
// desktop through a transport. Relative paths belong to the source session CWD.
type LinkedFileService interface {
	ReadLinkedFile(context.Context, LinkedFileRequest) (LinkedFileResult, error)
}

// LinkedFileRequest contains a filesystem path with any line/column anchor
// already removed. Absolute paths are interpreted on the service host.
type LinkedFileRequest struct {
	SessionID string `json:"sessionId"`
	Path      string `json:"path"`
}

type LinkedFileResult struct {
	File         File   `json:"file"`
	AbsolutePath string `json:"absolutePath"`
}
