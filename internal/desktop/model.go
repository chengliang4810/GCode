package desktop

import (
	"context"

	"gcode/internal/appmodel"
	"github.com/egoist/mygo/ui"
)

type Service interface {
	Snapshot(context.Context) (appmodel.Snapshot, error)
	AddProject(context.Context, string) error
	NewSession(context.Context, appmodel.NewSessionOptions) error
	Send(context.Context, string, string, appmodel.SendMode) error
	Stop(context.Context, string) error
	Withdraw(context.Context, string, string) error
	ResumeQueue(context.Context, string) error
	SetProvider(context.Context, string, string) error
	SetFullAccess(context.Context, string, bool) error
	ReplyPermission(context.Context, string, string, string) error
	ReplyAsk(context.Context, string, string, []string) error
	SaveSettings(context.Context, appmodel.Settings) error
	SaveProvider(context.Context, appmodel.Provider, string) error
	RemoveProvider(context.Context, string) error
	Close(context.Context) error
}

type WorkspaceSnapshot struct {
	Files   []appmodel.FileEntry
	Preview *appmodel.FilePreview
	Git     appmodel.GitState
	Error   string
}

type Workspace interface {
	// Snapshot lists the directory represented by cwd and returns its current
	// Git state. cwd is the concrete directory for the active session (or a
	// subdirectory selected in the file browser).
	Snapshot(context.Context, string) (WorkspaceSnapshot, error)
	// ReadFile loads a preview for path inside cwd. Keeping cwd and path
	// separate lets the controller enforce the session workspace boundary.
	ReadFile(context.Context, string, string) (appmodel.FilePreview, error)
	// OpenEditor opens path inside cwd in the user's configured editor.
	OpenEditor(context.Context, string, string) error
}

type TerminalSurface interface {
	View(*ui.Context) ui.Element
	Close() error
}

type TerminalFactory interface {
	Open(context.Context, string) (TerminalSurface, error)
}

type OpenOptions struct {
	Initial          appmodel.Snapshot
	InitialWorkspace WorkspaceSnapshot
	VisualSample     bool
	Workspace        Workspace
	TerminalFactory  TerminalFactory
	// Dispatch runs a state mutation on the native UI thread. The desktop
	// entry point should set this to Window.Update before showing the window.
	Dispatch func(func())
	// Invalidate requests a redraw when Dispatch is not responsible for it.
	Invalidate func()
}

type SettingsDraft struct {
	ProviderID string
	Provider   appmodel.Provider
	APIKey     string
	Theme      string
	SendMode   appmodel.SendMode
}

type screen uint8

const (
	screenConversation screen = iota
	screenSettings
)

type rightTab uint8

const (
	rightFiles rightTab = iota
	rightDiff
	rightTerminal
)
