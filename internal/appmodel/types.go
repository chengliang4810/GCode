package appmodel

import "time"

type SendMode string

const (
	Correction SendMode = "correction"
	Queue      SendMode = "queue"
)

type APIFormat string

const (
	Responses       APIFormat = "responses"
	ChatCompletions APIFormat = "chat-completions"
	Anthropic       APIFormat = "anthropic"
	Gemini          APIFormat = "gemini"
)

type Provider struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Format    APIFormat `json:"format"`
	BaseURL   string    `json:"base_url"`
	Model     string    `json:"model"`
	HasAPIKey bool      `json:"-"`
}

type Settings struct {
	Providers         []Provider `json:"providers"`
	DefaultProviderID string     `json:"default_provider_id"`
	DefaultSendMode   SendMode   `json:"default_send_mode"`
	Theme             string     `json:"theme"`
}

type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type RunState string

const (
	Idle              RunState = "idle"
	Running           RunState = "running"
	Cancelling        RunState = "cancelling"
	WaitingPermission RunState = "waiting_permission"
	WaitingQuestion   RunState = "waiting_question"
	Failed            RunState = "failed"
	Interrupted       RunState = "interrupted"
)

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Output    string `json:"output"`
	State     string `json:"state"`
	Error     string `json:"error,omitempty"`
}

type Message struct {
	ID          string     `json:"id"`
	RunID       string     `json:"run_id,omitempty"`
	State       string     `json:"state,omitempty"`
	Role        string     `json:"role"`
	Text        string     `json:"text"`
	Reasoning   string     `json:"reasoning,omitempty"`
	Tools       []ToolCall `json:"tools,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	Interrupted bool       `json:"interrupted,omitempty"`
}

type QueuedMessage struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	Mode      SendMode  `json:"mode"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
}

type PermissionOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type Permission struct {
	ID        string             `json:"id"`
	RunID     string             `json:"run_id"`
	Tool      string             `json:"tool"`
	Arguments string             `json:"arguments"`
	Options   []PermissionOption `json:"options"`
}

type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

type Question struct {
	ID      string           `json:"id"`
	Header  string           `json:"header"`
	Text    string           `json:"text"`
	Options []QuestionOption `json:"options"`
}

type AskRequest struct {
	ID        string     `json:"id"`
	RunID     string     `json:"run_id"`
	Questions []Question `json:"questions"`
}

type RunConfig struct {
	ProviderID   string    `json:"provider_id"`
	ProviderName string    `json:"provider_name"`
	Format       APIFormat `json:"format"`
	BaseURL      string    `json:"base_url"`
	Model        string    `json:"model"`
	Cwd          string    `json:"cwd"`
	FullAccess   bool      `json:"full_access"`
}

type Session struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	Title       string          `json:"title"`
	Cwd         string          `json:"cwd"`
	Worktree    bool            `json:"worktree"`
	Branch      string          `json:"branch,omitempty"`
	ProviderID  string          `json:"provider_id"`
	FullAccess  bool            `json:"full_access"`
	State       RunState        `json:"state"`
	RunID       string          `json:"run_id,omitempty"`
	ActiveRun   *RunConfig      `json:"active_run,omitempty"`
	Error       string          `json:"error,omitempty"`
	Messages    []Message       `json:"messages"`
	Queue       []QueuedMessage `json:"queue"`
	QueuePaused bool            `json:"queue_paused"`
	PauseReason string          `json:"pause_reason,omitempty"`
	Permissions []Permission    `json:"permissions,omitempty"`
	Questions   []AskRequest    `json:"questions,omitempty"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type Snapshot struct {
	Projects []Project `json:"projects"`
	Sessions []Session `json:"sessions"`
	Settings Settings  `json:"settings"`
}

type NewSessionOptions struct {
	ProjectID  string
	Title      string
	ProviderID string
	Worktree   bool
	BaseBranch string
	FullAccess bool
	// SessionID is assigned by the controller before workspace preparation so
	// worktree implementations can use the durable session identity for their
	// branch and path. Callers creating a session leave it empty.
	SessionID string `json:"-"`
}

type FileEntry struct {
	Name  string
	Path  string
	IsDir bool
	Size  int64
}

type FilePreview struct {
	Path      string
	Text      string
	Truncated bool
	Binary    bool
}

type Change struct {
	Path   string
	Status string
	Diff   string
}

type GitState struct {
	IsGit   bool
	Branch  string
	Changes []Change
}
