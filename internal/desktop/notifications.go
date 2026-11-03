package desktop

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/i18n"
)

type notificationKind string

const (
	notificationCompleted notificationKind = "completed"
	notificationQuestion  notificationKind = "question"
	notificationError     notificationKind = "error"
)

// desktopNotice contains only immutable display data. No UI values or business
// state cross into the delivery worker.
type desktopNotice struct {
	ID, Title, Body string
	Kind            notificationKind
	Desktop, Sound  bool
}

type notificationSink interface {
	Deliver(context.Context, desktopNotice) error
}

func (a *App) notificationWindowFocused() bool {
	// A window reference alone does not establish a running native event loop;
	// focus is only needed after notification delivery has started.
	return a.notifications != nil && a.notifications.cancel != nil &&
		a.win != nil && a.win.IsFocused()
}

// desktopNotifications is owned by the UI thread; its worker receives values.
// The initial snapshot is a baseline, so restarting never replays old alerts.
type desktopNotifications struct {
	runs         map[string]string
	sessions     map[string]contracts.SessionStatus
	questions    map[string]string
	disconnected bool
	queue        chan desktopNotice
	cancel       context.CancelFunc
	done         chan struct{}
}

func newDesktopNotifications(snapshot contracts.Snapshot) *desktopNotifications {
	n := &desktopNotifications{}
	n.remember(snapshot)
	return n
}

func (n *desktopNotifications) Start(ctx context.Context, sink notificationSink) {
	if n == nil || n.cancel != nil {
		return
	}
	workerCtx, cancel := context.WithCancel(ctx)
	n.cancel = cancel
	// Delivery is deliberately bounded; notification floods must not grow
	// memory or hold up rendering and accepted business work.
	n.queue = make(chan desktopNotice, 64)
	n.done = make(chan struct{})
	go func() {
		defer close(n.done)
		for {
			select {
			case <-workerCtx.Done():
				return
			case notice := <-n.queue:
				if workerCtx.Err() != nil {
					return
				}
				if err := sink.Deliver(workerCtx, notice); err != nil && !errors.Is(err, context.Canceled) {
					slog.Warn("desktop notification failed", "kind", notice.Kind, "error", err)
				}
			}
		}
	}()
}

func (n *desktopNotifications) Close() {
	if n != nil && n.cancel != nil {
		n.cancel()
	}
}

func (n *desktopNotifications) Observe(snapshot contracts.Snapshot, focused bool) {
	if n == nil {
		return
	}
	terminalSessions := make(map[string]bool)
	for _, run := range snapshot.Runs {
		if n.runs[run.ID] == run.Status {
			continue
		}
		session, ok := notificationSession(snapshot, run.SessionID)
		if !ok || session.SubagentType != "" {
			continue
		}
		switch run.Status {
		case "completed":
			n.emit(snapshot.Settings, focused, session.ID, session.Title, notificationCompleted)
			terminalSessions[session.ID] = true
		case "failed":
			n.emit(snapshot.Settings, focused, session.ID, session.Title, notificationError)
			terminalSessions[session.ID] = true
		}
	}
	for _, session := range snapshot.Sessions {
		if session.SubagentType != "" {
			continue
		}
		previous := n.sessions[session.ID]
		question := pendingQuestion(snapshot.Messages[session.ID])
		switch {
		case session.Status == contracts.AwaitingInput && (previous != contracts.AwaitingInput || question != n.questions[session.ID]):
			n.emit(snapshot.Settings, focused, session.ID, session.Title, notificationQuestion)
		case !terminalSessions[session.ID] && session.Status == contracts.Errored && previous != contracts.Errored:
			n.emit(snapshot.Settings, focused, session.ID, session.Title, notificationError)
		case !terminalSessions[session.ID] && session.Status == contracts.Idle && previous == contracts.Working && session.LastOutcome == "completed":
			n.emit(snapshot.Settings, focused, session.ID, session.Title, notificationCompleted)
		}
	}
	n.remember(snapshot)
}

func (n *desktopNotifications) ObserveEvent(event contracts.Event, settings contracts.Settings, focused bool) {
	if n == nil {
		return
	}
	switch event.Kind {
	case "connection.disconnected":
		if !n.disconnected {
			n.disconnected = true
			n.emit(settings, focused, "remote-connection", "", notificationError)
		}
	case "connection.connected":
		n.disconnected = false
	}
}

func (n *desktopNotifications) remember(snapshot contracts.Snapshot) {
	n.runs = make(map[string]string, len(snapshot.Runs))
	for _, run := range snapshot.Runs {
		n.runs[run.ID] = run.Status
	}
	n.sessions = make(map[string]contracts.SessionStatus, len(snapshot.Sessions))
	n.questions = make(map[string]string, len(snapshot.Sessions))
	for _, session := range snapshot.Sessions {
		n.sessions[session.ID] = session.Status
		if session.Status == contracts.AwaitingInput {
			n.questions[session.ID] = pendingQuestion(snapshot.Messages[session.ID])
		}
	}
}

func pendingQuestion(messages []contracts.Message) string {
	var result strings.Builder
	for _, message := range messages {
		for _, block := range message.Blocks {
			if block.Question != nil && !block.Question.Resolved {
				result.WriteString(block.Question.ID)
				result.WriteByte(0)
			}
		}
	}
	return result.String()
}

func notificationSession(snapshot contracts.Snapshot, id string) (contracts.Session, bool) {
	for _, session := range snapshot.Sessions {
		if session.ID == id {
			return session, true
		}
	}
	return contracts.Session{}, false
}

func (n *desktopNotifications) emit(settings contracts.Settings, focused bool, sessionID, sessionTitle string, kind notificationKind) {
	prefs := settings.Notifications
	sound := prefs.Sound && (kind == notificationCompleted && prefs.Completion || kind == notificationQuestion && prefs.Questions || kind == notificationError && prefs.Errors)
	desktop := prefs.Enabled && (!focused || prefs.WhenFocused)
	if !desktop && !sound || n.queue == nil {
		return
	}
	title := "Task completed"
	switch kind {
	case notificationQuestion:
		title = "Input required"
	case notificationError:
		title = "Errors and disconnections"
	}
	body := sessionTitle
	if sessionID == "remote-connection" {
		body = i18n.In(settings.General.Language, "Remote connection lost")
	}
	notice := desktopNotice{
		ID: "rcode-session-" + sessionID, Title: "GCode · " + i18n.In(settings.General.Language, title),
		Body: body, Kind: kind, Desktop: desktop, Sound: sound,
	}
	select {
	case n.queue <- notice:
	default:
		slog.Warn("desktop notification queue full", "kind", kind)
	}
}
