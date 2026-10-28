package contracts

import "context"

// WorkspaceSearchRequest searches only the selected virtual project. Offsets
// are one-based lines and zero-based UTF-8 byte columns, matching the reference protocol.
type WorkspaceSearchRequest struct {
	ProjectID      string `json:"projectId"`
	Query          string `json:"query"`
	FileLimit      int    `json:"fileLimit"`
	ContentLimit   int    `json:"contentLimit"`
	MatchesPerFile int    `json:"matchesPerFile"`
	Recent         bool   `json:"recent"`
	IncludeHidden  bool   `json:"includeHidden"`
	NamesOnly      bool   `json:"namesOnly"`
}
type FileSearchMatch struct {
	FileID string `json:"fileId"`
	Path   string `json:"path"`
	Kind   string `json:"kind"`
}
type MatchRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}
type ContentSearchMatch struct {
	FileID string       `json:"fileId"`
	Path   string       `json:"path"`
	Line   int          `json:"line"`
	Column int          `json:"column"`
	Text   string       `json:"text"`
	Ranges []MatchRange `json:"ranges"`
}
type WorkspaceSearchResult struct {
	Files    []FileSearchMatch    `json:"files"`
	Content  []ContentSearchMatch `json:"content"`
	Revision uint64               `json:"revision"`
}
type WorkspaceSearchService interface {
	SearchWorkspace(context.Context, WorkspaceSearchRequest) (WorkspaceSearchResult, error)
}

type DirectoryRequest struct {
	DeviceID string `json:"deviceId"`
	Path     string `json:"path"`
}
type DirectoryEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Hidden  bool   `json:"hidden"`
	Project bool   `json:"project"`
}
type DirectoryListing struct {
	DeviceID string           `json:"deviceId"`
	Path     string           `json:"path"`
	Parent   string           `json:"parent"`
	Home     string           `json:"home"`
	Entries  []DirectoryEntry `json:"entries"`
	Revision uint64           `json:"revision"`
}
type ProjectDirectoryService interface {
	ListDirectories(context.Context, DirectoryRequest) (DirectoryListing, error)
}
type ProjectCreate struct {
	DeviceID string `json:"deviceId"`
	Name     string `json:"name"`
	Path     string `json:"path"`
}
