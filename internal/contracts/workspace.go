package contracts

import "time"

type File struct {
	Loaded    bool   `json:"loaded"`
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Path      string `json:"path"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Language  string `json:"language"`
	Content   string `json:"content"`
	Version   uint64 `json:"version"`
	ReadOnly  bool   `json:"readOnly"`
	Modified  bool   `json:"modified"`
	ParentID  string `json:"parentId"`
	AssetRef  string `json:"assetRef,omitempty"`
}
type Change struct {
	ID         string     `json:"id"`
	SessionID  string     `json:"sessionId,omitempty"`
	ProjectID  string     `json:"projectId"`
	Path       string     `json:"path"`
	OldPath    string     `json:"oldPath,omitempty"`
	Status     string     `json:"status"`
	Additions  int        `json:"additions"`
	Deletions  int        `json:"deletions"`
	OldContent string     `json:"oldContent"`
	NewContent string     `json:"newContent"`
	Hunks      []DiffHunk `json:"hunks"`
}
type DiffHunk struct {
	Header   string     `json:"header"`
	OldStart int        `json:"oldStart"`
	NewStart int        `json:"newStart"`
	Lines    []DiffLine `json:"lines"`
}
type DiffLine struct {
	Kind    string `json:"kind"`
	Text    string `json:"text"`
	OldLine int    `json:"oldLine"`
	NewLine int    `json:"newLine"`
}
type Commit struct {
	SHA        string    `json:"sha"`
	Parents    []string  `json:"parents"`
	Subject    string    `json:"subject"`
	Author     string    `json:"author"`
	AuthoredAt time.Time `json:"authoredAt"`
	Refs       []string  `json:"refs"`
}
type Terminal struct {
	StorageRootID    string `json:"storageRootId"`
	OutputRef        string `json:"outputRef,omitempty"`
	OutputIncomplete bool   `json:"outputIncomplete,omitempty"`
	StorageError     string `json:"storageError,omitempty"`
	SessionID        string `json:"sessionId"`
	ExitCode         int    `json:"exitCode"`
	ID               string `json:"id"`
	ProjectID        string `json:"projectId"`
	Title            string `json:"title"`
	CWD              string `json:"cwd"`
	Columns          int    `json:"columns"`
	Rows             int    `json:"rows"`
	Status           string `json:"status"`
	Output           string `json:"output"`
}
type Account struct {
	Plan         string        `json:"plan"`
	CloudStatus  string        `json:"cloudStatus"`
	StorageUsed  int64         `json:"storageUsed"`
	StorageLimit int64         `json:"storageLimit"`
	Credits      float64       `json:"credits"`
	Usage        []UsageWindow `json:"usage"`
	Invoices     []Invoice     `json:"invoices"`
}
type UsageWindow struct {
	Label       string    `json:"label"`
	UsedPercent float64   `json:"usedPercent"`
	ResetsAt    time.Time `json:"resetsAt"`
}
type Invoice struct {
	ID     string    `json:"id"`
	Date   time.Time `json:"date"`
	Amount float64   `json:"amount"`
	Status string    `json:"status"`
}
type Update struct {
	CurrentVersion   string  `json:"currentVersion"`
	AvailableVersion string  `json:"availableVersion"`
	Status           string  `json:"status"`
	Progress         float64 `json:"progress"`
}
type BrowserTab struct {
	ID           string `json:"id"`
	URL          string `json:"url"`
	Title        string `json:"title"`
	Loading      bool   `json:"loading"`
	CanGoBack    bool   `json:"canGoBack"`
	CanGoForward bool   `json:"canGoForward"`
	Error        string `json:"error,omitempty"`
}
