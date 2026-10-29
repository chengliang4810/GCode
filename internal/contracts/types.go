// Package contracts defines versioned, framework-independent desktop data.
package contracts

import (
	"context"
	"time"
)

const SchemaVersion = 1

type SessionStatus string

const (
	Idle          SessionStatus = "idle"
	Working       SessionStatus = "working"
	AwaitingInput SessionStatus = "awaitingInput"
	Errored       SessionStatus = "errored"
)

type Snapshot struct {
	Integrations        IntegrationState            `json:"integrations"`
	ModelCatalog        ModelCatalogState           `json:"modelCatalog"`
	AppshotCapabilities AppshotCapabilities         `json:"appshotCapabilities"`
	ThemeLibrary        []ThemeLibraryEntry         `json:"themeLibrary"`
	ThemeImport         *ThemeImportPreview         `json:"themeImport,omitempty"`
	SchemaVersion       int                         `json:"schemaVersion"`
	Revision            uint64                      `json:"revision"`
	User                User                        `json:"user"`
	Capabilities        Capabilities                `json:"capabilities"`
	Projects            []Project                   `json:"projects"`
	Devices             []Device                    `json:"devices"`
	Sessions            []Session                   `json:"sessions"`
	Messages            map[string][]Message        `json:"messages"`
	Models              []Model                     `json:"models"`
	Providers           []Provider                  `json:"providers"`
	Settings            Settings                    `json:"settings"`
	Files               []File                      `json:"files"`
	Changes             []Change                    `json:"changes"`
	Commits             []Commit                    `json:"commits"`
	PullRequests        []PullRequest               `json:"pullRequests"`
	RepoRefs            map[string]RepoRefs         `json:"repoRefs,omitempty"`
	CheckoutPR          map[string]CheckoutPRStatus `json:"checkoutPR,omitempty"`
	Terminals           []Terminal                  `json:"terminals"`
	Queue               []QueuedMessage             `json:"queue"`
	Runs                []Run                       `json:"runs"`
	Account             Account                     `json:"account"`
	Appshots            []Attachment                `json:"appshots"`
	Attachments         []Attachment                `json:"attachments"`
	Update              Update                      `json:"update"`

	ContextUsage map[string]ContextUsage `json:"contextUsage"`
}

type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Initials string `json:"initials"`
	SignedIn bool   `json:"signedIn"`
}
type Capabilities struct {
	Appshots bool `json:"appshots"`
	Cloud    bool `json:"cloud"`
	Browser  bool `json:"browser"`
	Voice    bool `json:"voice"`
}
type Project struct {
	GitDetected     bool            `json:"gitDetected"`
	CloudRepository string          `json:"cloudRepository,omitempty"`
	DefaultBranch   string          `json:"defaultBranch,omitempty"`
	Actions         []ProjectAction `json:"actions"`
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Path            string          `json:"path"`
	DeviceID        string          `json:"deviceId"`
	Icon            string          `json:"icon"`
	Branch          string          `json:"branch"`
	Branches        []string        `json:"branches"`
	Worktrees       []Worktree      `json:"worktrees"`
}
type Worktree struct {
	BaseRef string `json:"baseRef,omitempty"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Branch  string `json:"branch"`
}
type Device struct {
	Version      string    `json:"version,omitempty"`
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Platform     string    `json:"platform"`
	Architecture string    `json:"architecture"`
	Status       string    `json:"status"`
	Current      bool      `json:"current"`
	LastSeen     time.Time `json:"lastSeen"`
}
type Session struct {
	CreationRequestID string                     `json:"creationRequestId,omitempty"`
	CWD               string                     `json:"cwd,omitempty"`
	SourceContext     *ConversationSourceContext `json:"sourceContext,omitempty"`
	AccessMode        AccessMode                 `json:"accessMode"`
	Plan              bool                       `json:"plan"`
	ParentSessionID   string                     `json:"parentSessionId,omitempty"`
	SideChat          bool                       `json:"sideChat"`
	ID                string                     `json:"id"`
	Title             string                     `json:"title"`
	ProjectID         string                     `json:"projectId"`
	DeviceID          string                     `json:"deviceId"`
	ModelID           string                     `json:"modelId"`
	ReasoningLevel    string                     `json:"reasoningLevel"`
	Branch            string                     `json:"branch"`
	WorktreeID        string                     `json:"worktreeId"`
	Status            SessionStatus              `json:"status"`
	Unread            bool                       `json:"unread"`
	Pinned            bool                       `json:"pinned"`
	Archived          bool                       `json:"archived"`
	PinOrder          int                        `json:"pinOrder"`
	CreatedAt         time.Time                  `json:"createdAt"`
	UpdatedAt         time.Time                  `json:"updatedAt"`
	LastOutcome       string                     `json:"lastOutcome"`
	RunningSubagents  int                        `json:"runningSubagents"`
	PullRequestNumber int                        `json:"pullRequestNumber"`
}
type Message struct {
	ContinuationOf string    `json:"continuationOf,omitempty"`
	DurationMS     int64     `json:"durationMs,omitempty"`
	ID             string    `json:"id"`
	SessionID      string    `json:"sessionId"`
	TurnID         string    `json:"turnId"`
	Role           string    `json:"role"`
	CreatedAt      time.Time `json:"createdAt"`
	Blocks         []Block   `json:"blocks"`
	Streaming      bool      `json:"streaming"`
	Usage          Usage     `json:"usage"`
}

// Block is a tagged union. Exactly the payload matching Kind is populated.
type Block struct {
	Annotation *Annotation `json:"annotation,omitempty"`
	ID         string      `json:"id"`
	Kind       string      `json:"kind"`
	Text       string      `json:"text,omitempty"`
	Language   string      `json:"language,omitempty"`
	Tool       *ToolCall   `json:"tool,omitempty"`
	Question   *Question   `json:"question,omitempty"`
	Permission *Permission `json:"permission,omitempty"`
	Todos      []Todo      `json:"todos,omitempty"`
	Subagent   *Subagent   `json:"subagent,omitempty"`
	Attachment *Attachment `json:"attachment,omitempty"`
	Artifact   *Artifact   `json:"artifact,omitempty"`
}
type ToolCall struct {
	Kind       ToolKind   `json:"kind,omitempty"`
	Args       string     `json:"args,omitempty"`
	Language   string     `json:"language,omitempty"`
	IsError    bool       `json:"isError"`
	ExitCode   *int       `json:"exitCode,omitempty"`
	Diff       *Change    `json:"diff,omitempty"`
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Summary    string     `json:"summary"`
	Command    string     `json:"command"`
	Path       string     `json:"path"`
	Status     string     `json:"status"`
	Output     string     `json:"output"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}
type Question struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Options     []string `json:"options"`
	MultiSelect bool     `json:"multiSelect"`
	Answer      string   `json:"answer"`
}
type Permission struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Command     string `json:"command"`
	Answer      string `json:"answer"`
}
type Todo struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Status string `json:"status"`
}
type Subagent struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	Name      string `json:"name"`
	Task      string `json:"task"`
	Status    string `json:"status"`
	Summary   string `json:"summary"`
}
type Attachment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MIME        string `json:"mime"`
	Size        int64  `json:"size"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	AssetRef    string `json:"assetRef"`
	Status      string `json:"status"`
	SourceApp   string `json:"sourceApp,omitempty"`
	WindowTitle string `json:"windowTitle,omitempty"`
}
type Artifact struct {
	Source   string  `json:"source"`
	AssetRef string  `json:"assetRef"`
	Width    float64 `json:"width"`
	Height   float64 `json:"height"`
	Baseline float64 `json:"baseline"`
}
type Usage struct {
	InputTokens   int     `json:"inputTokens"`
	OutputTokens  int     `json:"outputTokens"`
	CachedTokens  int     `json:"cachedTokens"`
	ContextTokens int     `json:"contextTokens"`
	ContextLimit  int     `json:"contextLimit"`
	Cost          float64 `json:"cost"`
}
type Run struct {
	RequestID    string          `json:"requestId,omitempty"`
	DurationMS   int64           `json:"durationMs"`
	Summary      ActivitySummary `json:"summary"`
	AccessMode   AccessMode      `json:"accessMode"`
	Plan         bool            `json:"plan"`
	ID           string          `json:"id"`
	SessionID    string          `json:"sessionId"`
	Status       string          `json:"status"`
	FinishReason string          `json:"finishReason"`
	Error        string          `json:"error,omitempty"`
	StartedAt    time.Time       `json:"startedAt"`
	FinishedAt   *time.Time      `json:"finishedAt,omitempty"`
}
type QueuedMessage struct {
	AccessMode     AccessMode   `json:"accessMode"`
	Plan           bool         `json:"plan"`
	Annotations    []Annotation `json:"annotations,omitempty"`
	ModelID        string       `json:"modelId,omitempty"`
	ReasoningLevel string       `json:"reasoningLevel,omitempty"`
	DeviceID       string       `json:"deviceId,omitempty"`
	Branch         string       `json:"branch,omitempty"`
	WorktreeID     string       `json:"worktreeId,omitempty"`
	ID             string       `json:"id"`
	SessionID      string       `json:"sessionId"`
	Text           string       `json:"text"`
	Attachments    []Attachment `json:"attachments"`
	CreatedAt      time.Time    `json:"createdAt"`
}
type Model struct {
	ID              string        `json:"id"`
	ProviderID      string        `json:"providerId"`
	Label           string        `json:"label"`
	Description     string        `json:"description"`
	ReasoningLevels []string      `json:"reasoningLevels"`
	ContextLimit    int           `json:"contextLimit"`
	Options         []ModelOption `json:"options"`
}
type ModelOption struct {
	ID            string            `json:"id"`
	Label         string            `json:"label"`
	Choices       []string          `json:"choices"`
	ChoiceLabels  map[string]string `json:"choiceLabels,omitempty"`
	DefaultChoice string            `json:"defaultChoice"`
}
type Provider struct {
	AccessModes  []AccessMode `json:"accessModes"`
	SupportsPlan bool         `json:"supportsPlan"`
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Icon         string       `json:"icon"`
	Status       string       `json:"status"`
	Account      string       `json:"account"`
	Enabled      bool         `json:"enabled"`
	Models       []string     `json:"models"`
}

type Settings struct {
	ModelPicker              ModelPickerSettings  `json:"modelPicker"`
	Appearance               AppearanceSettings   `json:"appearance"`
	General                  GeneralSettings      `json:"general"`
	Notifications            NotificationSettings `json:"notifications"`
	Files                    FileSettings         `json:"files"`
	Appshots                 AppshotSettings      `json:"appshots"`
	Shortcuts                []Shortcut           `json:"shortcuts"`
	SidebarWidth             float64              `json:"sidebarWidth"`
	FilesWidth               float64              `json:"filesWidth"`
	RightPaneWidth           float64              `json:"rightPaneWidth"`
	TerminalHeight           float64              `json:"terminalHeight"`
	SidebarCompact           bool                 `json:"sidebarCompact"`
	SidebarShowHarness       bool                 `json:"sidebarShowHarness"`
	SidebarShowBranch        bool                 `json:"sidebarShowBranch"`
	SidebarShowPullRequest   bool                 `json:"sidebarShowPullRequest"`
	SidebarShowProjectFilter bool                 `json:"sidebarShowProjectFilter"`
	SidebarShowArchived      bool                 `json:"sidebarShowArchived"`
	SidebarGrouping          string               `json:"sidebarGrouping"`
	SidebarSort              string               `json:"sidebarSort"`
	ShowProjectLabels        bool                 `json:"showProjectLabels"`
	ShowIcons                bool                 `json:"showIcons"`
}
type AppearanceSettings struct {
	MotionPreference     string               `json:"motionPreference"`
	SmoothScrolling      bool                 `json:"smoothScrolling"`
	PauseInBackground    bool                 `json:"pauseInBackground"`
	WallpaperColors      bool                 `json:"wallpaperColors"`
	RenderMath           bool                 `json:"renderMath"`
	MathFont             string               `json:"mathFont"`
	MathScale            float64              `json:"mathScale"`
	Mode                 string               `json:"mode"`
	ThemeID              string               `json:"themeId"`
	LightThemeID         string               `json:"lightThemeId"`
	DarkThemeID          string               `json:"darkThemeId"`
	Accent               string               `json:"accent"`
	Surface              string               `json:"surface"`
	Font                 string               `json:"font"`
	FontSize             float64              `json:"fontSize"`
	CodeFont             string               `json:"codeFont"`
	CodeFontSize         float64              `json:"codeFontSize"`
	TerminalFont         string               `json:"terminalFont"`
	TerminalFontSize     float64              `json:"terminalFontSize"`
	TranscriptWidth      float64              `json:"transcriptWidth"`
	Background           string               `json:"background"`
	BackgroundName       string               `json:"backgroundName"`
	BackgroundAssetRef   string               `json:"backgroundAssetRef"`
	BackgroundAdjustment BackgroundAdjustment `json:"backgroundAdjustment"`
	BackgroundEffect     string               `json:"backgroundEffect"`
	ReduceMotion         bool                 `json:"reduceMotion"`
}
type GeneralSettings struct {
	Language               string `json:"language"`
	SendBehavior           string `json:"sendBehavior"`
	CompactMode            bool   `json:"compactMode"`
	CompactModelPicker     bool   `json:"compactModelPicker"`
	StopWithEscape         bool   `json:"stopWithEscape"`
	PRDestination          string `json:"prDestination"`
	ThreadNamingModel      string `json:"threadNamingModel"`
	CustomWorktreeLocation bool   `json:"customWorktreeLocation"`
	WorktreePath           string `json:"worktreePath"`
	CreditZeron            bool   `json:"creditZeron"`
	LaunchAtLogin          bool   `json:"launchAtLogin"`
	RestoreWindows         bool   `json:"restoreWindows"`
}
type NotificationSettings struct {
	AgentUpdates bool `json:"agentUpdates"`
	Errors       bool `json:"errors"`
	Enabled      bool `json:"enabled"`
	Completion   bool `json:"completion"`
	Questions    bool `json:"questions"`
	Sound        bool `json:"sound"`
	WhenFocused  bool `json:"whenFocused"`
}
type FileSettings struct {
	Autosave      bool    `json:"autosave"`
	AutosaveDelay int     `json:"autosaveDelay"`
	ShowHidden    bool    `json:"showHidden"`
	WordWrap      bool    `json:"wordWrap"`
	LineNumbers   bool    `json:"lineNumbers"`
	FontSize      float64 `json:"fontSize"`
}
type AppshotSettings struct {
	Enabled       bool   `json:"enabled"`
	Destination   string `json:"destination"`
	Shortcut      string `json:"shortcut"`
	IncludeCursor bool   `json:"includeCursor"`
	Sound         bool   `json:"sound"`
}
type Shortcut struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Keys  string `json:"keys"`
	Group string `json:"group"`
}

type Event struct {
	SchemaVersion int       `json:"schemaVersion"`
	EventID       string    `json:"eventId"`
	Sequence      uint64    `json:"sequence"`
	Revision      uint64    `json:"revision"`
	Kind          string    `json:"kind"`
	SessionID     string    `json:"sessionId,omitempty"`
	RunID         string    `json:"runId,omitempty"`
	RequestID     string    `json:"requestId,omitempty"`
	Text          string    `json:"text,omitempty"`
	Snapshot      *Snapshot `json:"snapshot,omitempty"`
}
type SendRequest struct {
	Checkout       *CheckoutPlan `json:"checkout,omitempty"`
	AccessMode     AccessMode    `json:"accessMode"`
	Plan           bool          `json:"plan"`
	Annotations    []Annotation  `json:"annotations,omitempty"`
	DeviceID       string        `json:"deviceId,omitempty"`
	Branch         string        `json:"branch,omitempty"`
	WorktreeID     string        `json:"worktreeId,omitempty"`
	ProjectID      string        `json:"projectId,omitempty"`
	SessionID      string        `json:"sessionId"`
	Text           string        `json:"text"`
	ModelID        string        `json:"modelId"`
	ReasoningLevel string        `json:"reasoningLevel"`
	Attachments    []Attachment  `json:"attachments"`
	RequestID      string        `json:"requestId"`
}
type SessionUpdate struct {
	AccessMode     *AccessMode `json:"accessMode,omitempty"`
	Plan           *bool       `json:"plan,omitempty"`
	ReasoningLevel *string     `json:"reasoningLevel,omitempty"`
	Branch         *string     `json:"branch,omitempty"`
	DeviceID       *string     `json:"deviceId,omitempty"`
	WorktreeID     *string     `json:"worktreeId,omitempty"`
	ID             string      `json:"id"`
	Title          *string     `json:"title,omitempty"`
	Pinned         *bool       `json:"pinned,omitempty"`
	Archived       *bool       `json:"archived,omitempty"`
	Unread         *bool       `json:"unread,omitempty"`
	PinOrder       *int        `json:"pinOrder,omitempty"`
	ModelID        *string     `json:"modelId,omitempty"`
}

// Command is a typed action envelope used by the application coordinator.
type Command struct {
	RequestID     string                `json:"requestId,omitempty"`
	Git           *GitRequest           `json:"git,omitempty"`
	ProjectCreate *ProjectCreate        `json:"projectCreate,omitempty"`
	Integration   *IntegrationRequest   `json:"integration,omitempty"`
	Adjustment    *BackgroundAdjustment `json:"adjustment,omitempty"`
	Action        *ProjectAction        `json:"action,omitempty"`
	TerminalSize  *TerminalSize         `json:"terminalSize,omitempty"`
	Comment       *CommentInput         `json:"comment,omitempty"`
	Kind          string                `json:"kind"`
	ID            string                `json:"id,omitempty"`
	SessionID     string                `json:"sessionId,omitempty"`
	ProjectID     string                `json:"projectId,omitempty"`
	Text          string                `json:"text,omitempty"`
	Name          string                `json:"name,omitempty"`
	Path          string                `json:"path,omitempty"`
	Enabled       bool                  `json:"enabled"`
	Version       uint64                `json:"version,omitempty"`
	Send          *SendRequest          `json:"send,omitempty"`
	Settings      *Settings             `json:"settings,omitempty"`
	Session       *SessionUpdate        `json:"session,omitempty"`
	Attachment    *Attachment           `json:"attachment,omitempty"`
}

type TerminalSize struct {
	Columns int `json:"columns"`
	Rows    int `json:"rows"`
}
type CommentInput struct {
	Body string `json:"body"`
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
}

type Service interface {
	Bootstrap(context.Context) (Snapshot, error)
	Snapshot(context.Context) (Snapshot, error)
	Execute(context.Context, Command) error
	Subscribe(context.Context) (<-chan Event, error)
	Close() error
}
