// Package desktop implements the native application shell.
package desktop

import (
	"context"
	"crypto/rand"
	"fmt"
	"maps"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
	localize "gcode/internal/i18n/view"
	"gcode/internal/linkui"
	"gcode/internal/workbench"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

type App struct {
	math            mathViewState
	notifications   *desktopNotifications
	wallpaper       wallpaperState
	checkForUpdates func()
	updateAvailable bool
	connections     contracts.ConnectionSettingsService
	runtimeUI       runtimeSettingsState
	modelsUI        modelSettingsState
	capabilityUI    capabilitySettingsState
	settingsRemoval settingsRemovalState
	settingsImport  settingsImportState
	cloudPage       cloudSettingsPageState
	agentsDraft     string
	agentsLoadedFor string
	branchCreation  *branchCreation
	now             func() time.Time
	pendingSends    map[string]primaryPendingSend
	composerControls
	footerUsage         composerUsageState
	appUsage            appUsageState
	footerGit           composerFooterGitState
	sideComposers       map[string]*sideComposerState
	background          backgroundSettingsState
	pickAttachmentFiles func() ([]string, error)
	attachmentImports   map[string]int
	draftEpoch          uint64
	mediaLoading        map[string]bool
	sidebarList         ui.ListState
	sidebarUI           sidebarState
	commands            commandPaletteState
	newProject          newProjectState
	annotations         []contracts.Annotation
	hoveredAnnotation   uint32
	focusComposer       bool
	visibleSidebarWidth float32
	sidebarContentWidth float32
	// sidebarSplit is the split size the last built frame left, which tells
	// a keyboard step landing after that frame from a drag it already drew.
	sidebarSplit      float32
	imageZoom         float32
	conversationQuery string
	findIndex         int
	fit               panelAllocation
	panelOpened       [3]uint64
	rightExpanded     bool
	chatWidth         float32
	// Panel columns ease between widths on open and close (panel_tween.go).
	// painted is the width drawn this frame, content the width each moving
	// pane lays itself out at, motion the toggles waiting for this frame's
	// targets, and frameColumns the allocation the frame draws.
	panelTween            [3]panelWidthTween
	panelPainted          [3]float32
	panelContent          [3]float32
	panelMotion           [3]bool
	panelReady            bool
	frameColumns          panelWidths
	sessionStates         map[string]sessionViewState
	canvasTarget          *canvasTarget
	chatWorkspacePending  bool
	desiredSettings       *contracts.Settings
	authoritativeSettings contracts.Settings
	settingsPending       int
	bottom                *workbench.State
	pendingNew            bool
	pendingNewRequestID   string
	pendingNewEpoch       uint64
	pendingNewRuns        map[string]bool
	service               contracts.Service
	ctx                   context.Context
	cancel                context.CancelFunc
	win                   *mygo.Window
	// captionButtons reports that the window hides its native caption
	// controls, so the title bar draws the shared SVG ones instead.
	captionButtons                                                bool
	systemDark                                                    bool
	state                                                         contracts.Snapshot
	jobs                                                          chan contracts.Command
	closed                                                        bool
	closeOnce                                                     sync.Once
	workspace                                                     *workbench.State
	route, section, sessionID, projectID, deviceID                string
	history                                                       []string
	historyIndex                                                  int
	sidebar, files, right, terminal                               bool
	sidebarWidth, filesWidth, rightWidth, terminalHeight          float32
	sidebarResize, terminalResize, rightResize, filesResize       splitResize
	paneSaveAt                                                    time.Time
	paneDirty                                                     paneDirty
	draft, search, settingsSearch, paletteQuery, errorText, toast string
	attachments                                                   []contracts.Attachment
	drafts                                                        map[string]string
	expanded                                                      map[string]bool
	integrationUI                                                 integrationUIState
	messages                                                      ui.ListState
	md                                                            markdown.Options
	userMD                                                        markdown.Options
	linkOptions                                                   map[string]*markdown.Options
	linkSource                                                    string
	linkSelection                                                 *linkui.Selection
	// transcriptSelections are the selection scopes of the open transcripts
	// by pane, and transcriptHover the row the pointer is in on each.
	transcriptSelections map[string]*linkui.Selection
	transcriptHover      map[string]string
	sideTranscripts      map[string]*transcriptCache
	activityDetails      activityDetailsCache
	transcriptCache      transcriptCache
	activityMotion       *activityFoldMotion
	activityReveals      map[string]time.Time
	composer             ui.Handle
	composerSurface      ui.Handle
	conversationViewport ui.Handle
	// composerDock owns the blank-canvas ↔ thread handoff. launchingNewSession
	// starts it at the send key, before the host mints the session; the
	// handoff remembers the route being left so the pill morphs its height and
	// chrome instead of switching both in one frame.
	composerDock                                               composerDockState
	composerFrame                                              composerDockFrame
	composerHandoff                                            composerHandoff
	composerPillBox                                            ui.Rect
	composerPillHeight                                         float32
	composerExpanded                                           bool
	launchingNewSession                                        bool
	chooseTableExportPath                                      func() (string, error)
	nestedMarkdownTable                                        bool
	projectMenu, deviceMenu, branchMenu, viewMenu, paletteOpen bool
	renameOpen, projectOpen, attachmentOpen, imageOpen         bool
	renameID, renameText, imageRef                             string
	branch, checkout                                           string
	toastUntil                                                 time.Time
	palette                                                    design.Tokens
	services                                                   ui.Services
}

type Options struct {
	Now             func() time.Time
	Connections     contracts.ConnectionSettingsService
	CheckForUpdates func()
}

func New(service contracts.Service) (*App, error) { return NewWithOptions(service, Options{}) }

func NewWithOptions(service contracts.Service, options Options) (*App, error) {
	_ = conversationBackgroundShader()
	ctx, cancel := context.WithCancel(context.Background())
	snapshot, err := service.Bootstrap(ctx)
	if err != nil {
		cancel()
		return nil, err
	}
	snapshot.Settings.General.Language = i18n.Normalize(snapshot.Settings.General.Language)
	// The reference defaults an unset busy send to a steer.
	if snapshot.Settings.General.ActiveTurnSendBehavior == "" {
		snapshot.Settings.General.ActiveTurnSendBehavior = "steer"
	}
	a := &App{service: service, ctx: ctx, cancel: cancel, state: snapshot, jobs: make(chan contracts.Command, 128), panelOpened: [3]uint64{1, 2, 3},
		sidebar: true, route: "chat", section: "General", checkout: "Current checkout",
		drafts: make(map[string]string), expanded: make(map[string]bool), history: []string{"chat"}}
	if snapshot.Runtime == nil {
		a.reasoning = "High"
	}
	a.connections = options.Connections
	a.notifications = newDesktopNotifications(snapshot)
	a.checkForUpdates = options.CheckForUpdates
	if a.connections != nil {
		info, infoErr := a.connections.ConnectionInfo(ctx)
		if infoErr != nil {
			cancel()
			return nil, infoErr
		}
		a.runtimeUI.connection = info
		a.runtimeUI.remoteURL = info.URL
	}
	a.now = options.Now
	if a.now == nil {
		a.now = time.Now
	}
	a.authoritativeSettings = copySettings(snapshot.Settings)
	a.sidebarWidth = float32(snapshot.Settings.SidebarWidth)
	a.filesWidth = float32(snapshot.Settings.FilesWidth)
	a.rightWidth = float32(snapshot.Settings.RightPaneWidth)
	a.terminalHeight = float32(snapshot.Settings.TerminalHeight)
	if len(snapshot.Projects) > 0 {
		a.projectID = snapshot.Projects[0].ID
		a.branch = snapshot.Projects[0].Branch
	}
	if len(snapshot.Devices) > 0 {
		a.deviceID = snapshot.Devices[0].ID
	}
	a.restoreModelSelection()
	a.workspace = workbench.New(nil, service)
	a.bottom = workbench.New(nil, service)
	a.bottom.Update(snapshot)
	a.bottom.Dispatch = a.dispatch
	a.bottom.OnCloseSurface = func() { a.terminal = false }
	a.workspace.Dispatch = a.dispatch
	a.workspace.Update(snapshot)
	a.workspace.ProjectID = a.projectID
	a.workspace.OnOpenSurface = func() { a.setPanel(1, true) }
	a.workspace.OnCloseSurface = func() { a.setPanel(1, false) }
	a.workspace.OnOpenSession = a.selectSession
	a.workspace.RenderSession = a.sideTranscript
	a.workspace.OnCloseSession = a.dismissSideComposer
	a.workspace.OnNewSideChat = a.newSideChat
	a.workspace.CanNewSideChat = a.canNewSideChat
	a.workspace.OnToggleExpand = a.toggleWorkspaceExpanded
	a.workspace.OnAnnotation = a.attachAnnotation
	a.configureWorkspaceLinks()
	a.refreshBackground()
	return a, nil
}

func (a *App) Attach(win *mygo.Window) error {
	if err := setApplicationIcon(win); err != nil {
		return fmt.Errorf("set application icon: %w", err)
	}
	a.win = win
	a.captionButtons = HideNativeWindowButtons
	a.systemDark = mygo.Theme.IsDark()
	a.refreshWallpaperColors()
	a.refreshBackground()
	offTheme := mygo.Theme.OnUpdated(func() {
		win.Update(func() {
			if !a.closed {
				a.systemDark = mygo.Theme.IsDark()
				a.refreshWallpaperColors()
				a.refreshBackground()
			}
		})
	})
	win.OnClosed(offTheme)
	// The title bar swaps the maximize and restore glyphs with the window
	// state, so a state change from any source (caption button, Snap Layouts,
	// double-click or the window menu) redraws it.
	offMaximize := win.OnMaximize(win.Invalidate)
	offUnmaximize := win.OnUnmaximize(win.Invalidate)
	win.OnClosed(func() {
		offMaximize()
		offUnmaximize()
	})
	_ = a.workspace.Close()
	a.workspace = workbench.New(win, a.service)
	_ = a.bottom.Close()
	a.bottom = workbench.New(win, a.service)
	a.bottom.Update(a.state)
	a.bottom.Dispatch = a.dispatch
	a.bottom.OnCloseSurface = func() { a.terminal = false }
	a.workspace.Dispatch = a.dispatch
	a.workspace.Update(a.state)
	a.workspace.ProjectID = a.projectID
	a.workspace.OnOpenSurface = func() { a.setPanel(1, true) }
	a.workspace.OnCloseSurface = func() { a.setPanel(1, false) }
	a.workspace.OnOpenSession = a.selectSession
	a.workspace.RenderSession = a.sideTranscript
	a.workspace.OnCloseSession = a.dismissSideComposer
	a.workspace.OnNewSideChat = a.newSideChat
	a.workspace.CanNewSideChat = a.canNewSideChat
	a.syncNativeAppearance()
	a.syncNativeLanguage()
	a.workspace.OnToggleExpand = a.toggleWorkspaceExpanded
	a.workspace.OnAnnotation = a.attachAnnotation
	a.configureWorkspaceLinks()
	events, err := a.service.Subscribe(a.ctx)
	if err != nil {
		return err
	}
	a.notifications.Start(a.ctx, nativeNotificationSink{})
	go func() {
		for {
			select {
			case <-a.ctx.Done():
				return
			case command := <-a.jobs:
				err := a.service.Execute(a.ctx, command)
				snapshot, snapshotErr := a.service.Snapshot(a.ctx)
				win.Update(func() {
					if a.closed {
						return
					}
					a.commandFinished(command, err)
					if snapshotErr == nil {
						a.apply(snapshot)
						a.creationCommandApplied(command, err)
					}
					a.branchSnapshotFailed(command, snapshotErr)
				})
			}
		}
	}()
	go func() {
		for {
			select {
			case <-a.ctx.Done():
				return
			case event, ok := <-events:
				if !ok {
					return
				}
				win.Update(func() {
					if !a.closed {
						a.applyEvent(event)
					}
				})
			}
		}
	}()
	win.OnClosed(func() { a.Close() })
	a.prefetchMedia(snapshotMediaRefs(a.state))
	return nil
}

func (a *App) Close() {
	a.closeOnce.Do(func() {
		if !a.paneSaveAt.IsZero() {
			// A drag changed a width and the debounce has not run yet: the
			// window is closing, so write it now, before the worker stops.
			// A remote host that does not answer must not hold the window
			// open, so the write has its own short deadline.
			a.paneSaveAt = time.Time{}
			a.applyPaneWidths()
			settings := a.state.Settings
			ctx, cancel := context.WithTimeout(context.WithoutCancel(a.ctx), time.Second)
			_ = a.service.Execute(ctx, contracts.Command{Kind: "settings.update", Settings: &settings})
			cancel()
		}
		a.closed = true
		a.cancel()
		a.notifications.Close()
		_ = a.workspace.Close()
		_ = a.bottom.Close()
		_ = a.service.Close()
	})
}
func (a *App) apply(s contracts.Snapshot) {
	if s.Revision > a.state.Revision {
		a.authoritativeSettings = copySettings(s.Settings)
		if a.settingsPending > 0 && a.desiredSettings != nil {
			s.Settings = *a.desiredSettings
		}
		languageChanged := a.state.Settings.General.Language != s.Settings.General.Language
		appearanceChanged := a.state.Settings.Appearance != s.Settings.Appearance
		a.projectFooterAccountSwitches(&s)
		a.notifications.Observe(s, a.notificationWindowFocused())
		a.state = s
		a.refreshVisibleAppUsage()
		a.finishSidebarPullRequests()
		a.refreshWallpaperColors()
		a.restoreModelSelection()
		a.prefetchMedia(snapshotMediaRefs(s))
		a.workspace.Update(s)
		a.bottom.Update(s)
		if languageChanged {
			a.syncNativeLanguage()
		}
		if appearanceChanged {
			a.syncNativeAppearance()
			a.refreshBackground()
		}
	}
	if a.pendingNew && a.pendingNewEpoch == a.draftEpoch {
		for _, run := range a.state.Runs {
			if run.RequestID != "" && run.RequestID == a.pendingNewRequestID && !a.pendingNewRuns[run.ID] {
				a.settleCreatedSession(run.SessionID)
				break
			}
		}
	}
	// Reconcile against accepted state, including an unchanged frame that finds
	// an already dangling selection. Ignored snapshots cannot remove a row.
	if a.sessionID != "" && !a.hasSession(a.sessionID) {
		removed := a.sessionID
		a.setSelectedSession("", false)
		delete(a.sessionStates, removed)
		delete(a.drafts, removed)
		delete(a.pendingSends, removed)
	}
	a.reconcileBranchCreation()
}

func (a *App) applyEvent(event contracts.Event) {
	a.notifications.ObserveEvent(event, a.state.Settings, a.notificationWindowFocused())
	if event.Kind == "session.naming.failed" {
		a.notification(event.Text)
	}
	if event.Snapshot != nil {
		a.apply(*event.Snapshot)
	} else if event.Revision > a.state.Revision && (event.Message != nil || event.Kind == "message.delta") {
		// Streaming updates copy only the affected message, never serialize or
		// clone the entire conversation history between the Go core and UI.
		s := a.state
		s.Messages = maps.Clone(s.Messages)
		messages := slices.Clone(s.Messages[event.SessionID])
		if event.Message != nil {
			message := *event.Message
			message.Blocks = contracts.CloneBlocks(message.Blocks)
			index := slices.IndexFunc(messages, func(m contracts.Message) bool { return m.ID == message.ID })
			if index < 0 {
				messages = append(messages, message)
			} else {
				messages[index] = message
			}
		} else {
			for i := range messages {
				if messages[i].ID != event.MessageID {
					continue
				}
				messages[i].Blocks = contracts.CloneBlocks(messages[i].Blocks)
				index := slices.IndexFunc(messages[i].Blocks, func(b contracts.Block) bool { return b.Kind == event.BlockKind })
				if index < 0 {
					messages[i].Blocks = append(messages[i].Blocks, contracts.Block{ID: event.MessageID + "-" + event.BlockKind, Kind: event.BlockKind})
					index = len(messages[i].Blocks) - 1
				}
				messages[i].Blocks[index].Text += event.Text
			}
		}
		s.Messages[event.SessionID] = messages
		s.Revision = event.Revision
		a.state = s
		if event.Kind != "message.delta" {
			a.refreshVisibleAppUsage()
		}
		a.workspace.Update(s)
		a.bottom.Update(s)
	}
	if !a.pendingNew || event.RequestID == "" || event.RequestID != a.pendingNewRequestID || a.pendingNewEpoch != a.draftEpoch || a.pendingNewRuns[event.RunID] || event.Kind != "run.started" && event.Kind != "request.created" && event.Kind != "queue.changed" {
		return
	}
	if event.Snapshot != nil || a.hasSession(event.SessionID) {
		a.settleCreatedSession(event.SessionID)
	}
}

func (a *App) settleCreatedSession(id string) {
	// Durable run/request identity survives the backend's snapshot coalescing.
	// An older ACK may settle its request, but cannot resurrect a removed row.
	for _, session := range a.state.Sessions {
		if session.ID == id {
			if session.SideChat || session.ParentSessionID != "" {
				return
			}
			a.pendingNew = false
			a.pendingNewRequestID = ""
			a.launchingNewSession = false
			if a.sessionID == "" && a.route == "chat" {
				a.selectSession(session.ID)
			}
			return
		}
	}
	a.pendingNew = false
	a.pendingNewRequestID = ""
	a.launchingNewSession = false
}

func (a *App) creationCommandApplied(command contracts.Command, err error) {
	a.branchCommandApplied(command, err)
	if err == nil && a.pendingNew && command.Kind == "message.send" && command.Send != nil && command.Send.SessionID == "" && command.Send.RequestID == a.pendingNewRequestID {
		// A concurrent deletion can remove both the created row and its run
		// before this complete snapshot. A finished command must still settle.
		a.pendingNew = false
		a.pendingNewRequestID = ""
		a.launchingNewSession = false
	}
}

func copySettings(settings contracts.Settings) contracts.Settings {
	return *contracts.Command{Settings: &settings}.Clone().Settings
}
func (a *App) dispatch(cmd contracts.Command) {
	cmd = cmd.Clone()
	if a.closed {
		return
	}
	if a.win == nil {
		err := a.service.Execute(a.ctx, cmd)
		a.commandFinished(cmd, err)
		if s, snapshotErr := a.service.Snapshot(a.ctx); snapshotErr == nil {
			a.apply(s)
			a.creationCommandApplied(cmd, err)
		} else {
			a.branchSnapshotFailed(cmd, snapshotErr)
		}
		return
	}
	select {
	case a.jobs <- cmd:
	default:
		a.commandFinished(cmd, fmt.Errorf("an operation is still pending; please try again"))
	}
}
func (a *App) saveSettings() {
	a.saveSettingsWithID("")
}

// paneWidthSaveDelay is how long a divider drag rests before the width it
// left reaches the settings store. A drag changes the width on every frame
// of the pointer's travel, and each save writes the whole settings state.
const paneWidthSaveDelay = 400 * time.Millisecond

// schedulePaneSave remembers that a pane width changed. The width reaches
// the store once the drag rests, not on every frame of it.
func (a *App) schedulePaneSave(c *ui.Context, pane paneDirty) {
	a.paneDirty |= pane
	a.paneSaveAt = c.Now().Add(paneWidthSaveDelay)
}

// flushPaneSave writes the widths the app keeps once the drag has rested.
func (a *App) flushPaneSave(c *ui.Context) {
	if a.paneSaveAt.IsZero() {
		return
	}
	now := c.Now()
	if now.Before(a.paneSaveAt) {
		c.After(a.paneSaveAt.Sub(now))
		return
	}
	a.paneSaveAt = time.Time{}
	a.applyPaneWidths()
	a.saveSettings()
}

// shownSidebarWidth is the sidebar's width as this frame lays it out: the
// width the app keeps, plus the bounce a divider at its limit is playing.
// The columns beside the sidebar size from the same number, so the seam and
// the panes around it move together.
func (a *App) shownSidebarWidth() float32 {
	if a.fit.Sidebar {
		return a.visibleSidebarWidth
	}
	return 0
}

// paneDirty records the dividers the user dragged since the last save.
type paneDirty uint8

const (
	paneSidebar paneDirty = 1 << iota
	paneFiles
	paneRight
	paneTerminal
)

// applyPaneWidths puts the dragged widths into the settings it saves: a
// snapshot that arrives between the drag and its save carries the width the
// store still holds. Only panes the user dragged are written, so a width
// another surface changed is not overwritten with the value this window read
// when it opened.
func (a *App) applyPaneWidths() {
	if a.paneDirty&paneSidebar != 0 {
		a.state.Settings.SidebarWidth = float64(a.sidebarWidth)
	}
	if a.paneDirty&paneFiles != 0 {
		a.state.Settings.FilesWidth = float64(a.filesWidth)
	}
	if a.paneDirty&paneRight != 0 {
		a.state.Settings.RightPaneWidth = float64(a.rightWidth)
	}
	if a.paneDirty&paneTerminal != 0 {
		a.state.Settings.TerminalHeight = float64(a.terminalHeight)
	}
	a.paneDirty = 0
}

func (a *App) saveSettingsWithID(requestID string) {
	s := a.state.Settings
	s.Shortcuts = append([]contracts.Shortcut(nil), s.Shortcuts...)
	a.desiredSettings = &s
	a.settingsPending++
	a.syncNativeAppearance()
	a.syncNativeLanguage()
	a.refreshWallpaperColors()
	a.dispatch(contracts.Command{Kind: "settings.update", ID: requestID, Settings: &s})
	a.refreshBackground()
}
func (a *App) OpenRoute(route string) {
	if route != "settings" && !strings.HasPrefix(route, "settings/") {
		a.capabilityUI.plugins.configID, a.capabilityUI.plugins.config = "", nil
	}
	// A session pick or annotation may leave settings before the next View.
	// Retire that visit before replacing the old route with Cloud again.
	if a.cloudPage.active && !a.cloudSettingsVisible() {
		a.syncCloudSettingsPage()
	}
	defer a.syncCloudSettingsPage()
	a.refreshBackground()
	if route != "settings" && !strings.HasPrefix(route, "settings/") {
		a.clearShortcutRecording()
		if a.chatWorkspacePending {
			a.syncChatWorkspace()
		}
	}
	if route == "settings" || strings.HasPrefix(route, "settings/") {
		a.workspace.StopAnnotations()
	}
	switch route {
	case "browser", "changes", "history", "files":
		a.route = "chat"
		a.setPanel(1, true)
		a.workspace.OpenSurface(route)
		return
	case "terminal":
		a.route = "chat"
		if !a.terminal {
			a.toggleTerminal()
		}
		return
	}
	if strings.HasPrefix(route, "settings/") {
		a.route = "settings"
		a.selectSettingsSection(strings.TrimPrefix(route, "settings/"))
		for _, section := range a.sections() {
			if strings.EqualFold(section, a.section) {
				a.section = section
				break
			}
		}
		return
	}
	if route == "settings" {
		a.route = "settings"
		return
	}
	if strings.HasPrefix(route, "chat/") {
		a.selectSession(strings.TrimPrefix(route, "chat/"))
		return
	}
	a.route = "chat"
	a.selectSession("")
}
func (a *App) navigate(route string) {
	a.history = a.history[:a.historyIndex+1]
	a.history = append(a.history, route)
	a.historyIndex = len(a.history) - 1
	a.OpenRoute(route)
}
func (a *App) back(delta int) {
	i := a.historyIndex + delta
	if i >= 0 && i < len(a.history) {
		a.historyIndex = i
		a.OpenRoute(a.history[i])
	}
}
func (a *App) selectSession(id string) {
	for _, session := range a.state.Sessions {
		if session.ID == id {
			a.revealSidebarSession(session)
			break
		}
	}
	a.setSelectedSession(id, false)
	a.route = "chat"
}

// Background reconciliation changes the chat selection without navigating.
// A visible PR surface stays in its workspace until explicit chat navigation.
func (a *App) setSelectedSession(id string, preserveWorkspace bool) {
	if id != "" && !a.hasSession(id) {
		id = ""
	}
	if id == a.sessionID {
		if !preserveWorkspace && a.chatWorkspacePending {
			a.syncChatWorkspace()
		}
		return
	}
	if a.sessionID == "" || a.hasSession(a.sessionID) {
		a.storeSessionView()
	}
	if a.sessionID == "" && id != "" {
		a.rememberCanvasTarget()
	} else if id == "" {
		a.restoreCanvasTarget()
	}
	a.workspace.ResetAnnotations()
	a.sessionID = id
	a.modelMenu = false
	a.effortMenu = false
	a.modelProvider, a.modelQuery = "", ""
	a.projectMenu, a.deviceMenu, a.branchMenu = false, false, false
	for _, s := range a.state.Sessions {
		if s.ID == id {
			a.revealSidebarSession(s)
			a.invalidateCanvasCloudTarget(s)
			a.projectID = s.ProjectID
			a.deviceID = s.DeviceID
			a.modelID = s.ModelID
			a.reasoning = "High"
			if a.state.Runtime != nil {
				a.reasoning = ""
			}
			a.branch = s.Branch
			if s.ReasoningLevel != "" {
				a.reasoning = s.ReasoningLevel
			}
			if s.WorktreeID != "" {
				a.checkout = "New worktree"
			} else {
				a.checkout = "Current checkout"
			}
			if s.Unread {
				unread := false
				a.dispatch(contracts.Command{Kind: "session.update", Session: &contracts.SessionUpdate{ID: id, Unread: &unread}})
			}
			break
		}
	}
	if id == "" {
		a.reconcileComposerCloudDevice()
	}
	a.restoreModelSelection()
	a.normalizeReasoning()
	if a.bottom != nil {
		a.bottom.ProjectID = a.projectID
	}
	a.bottom.SwitchSession(id)
	if preserveWorkspace {
		a.chatWorkspacePending = true
	} else {
		a.syncChatWorkspace()
	}
	a.restoreSessionView(id)
}

func (a *App) syncChatWorkspace() {
	a.workspace.ProjectID = a.projectID
	a.workspace.SwitchSession(a.sessionID)
	a.chatWorkspacePending = false
}

func (a *App) hasSession(id string) bool {
	for _, session := range a.state.Sessions {
		if session.ID == id {
			return true
		}
	}
	return false
}
func (a *App) currentSession() contracts.Session {
	for _, s := range a.state.Sessions {
		if s.ID == a.sessionID {
			return s
		}
	}
	return contracts.Session{}
}
func (a *App) projectLabel() string {
	for _, p := range a.state.Projects {
		if p.ID == a.projectID {
			return p.Name
		}
	}
	return a.t("Don't work in a project")
}
func (a *App) deviceLabel() string {
	for _, d := range a.state.Devices {
		if d.ID == a.deviceID {
			if d.Current && d.Name == "This device" {
				return a.t("This device")
			}
			return d.Name
		}
	}
	return a.t("This device")
}
func (a *App) modelLabel() string {
	if a.state.Runtime != nil {
		return a.centralControls().modelLabel()
	}
	if a.sessionID == "" && a.modelNoAgents() {
		return a.t("No models configured")
	}
	for _, m := range a.state.Models {
		if m.ID == a.modelID {
			return m.Label
		}
	}
	if a.state.ModelCatalog.State == contracts.CatalogLoading {
		return a.t("Loading models…")
	}
	if a.modelID != "" {
		return a.modelID
	}
	return "Select model"
}
func (a *App) notification(s string) { a.toast = s; a.toastUntil = time.Now().Add(3 * time.Second) }

func (a *App) View(c *ui.Context) {
	localize.Set(c, a.state.Settings.General.Language)
	a.syncCloudSettingsPage()
	a.services = c.Services()
	a.configureMotion(c)
	if a.focusComposer {
		a.composer.Focus(c)
		a.focusComposer = false
	}
	settings := a.state.Settings.Appearance
	systemDark := c.Theme().Dark
	if a.win != nil {
		systemDark = a.systemDark
	}
	dark := strings.EqualFold(settings.Mode, "dark") || (strings.EqualFold(settings.Mode, "system") && systemDark)
	themeID := settings.ThemeID
	if themeID == "" {
		if dark {
			themeID = settings.DarkThemeID
		} else {
			themeID = settings.LightThemeID
		}
	}
	a.palette = design.Palette(themeID, dark)
	a.palette = design.ApplyAccent(a.palette, settings.Accent)
	a.palette = a.wallpaperPalette(a.palette)
	theme := a.palette.UI()
	theme.Font = design.Font(settings.Font)
	reduced := a.reduceMotion(c)
	if reduced {
		c.DisableAnimations()
	}
	if settings.FontSize > 0 {
		theme.FontSize = float32(settings.FontSize)
	}
	c.SetTheme(theme)
	c.Root().Background(a.palette.Background).Font(theme.Font).FontSize(theme.FontSize).TextColor(theme.Text)
	design.ApplyReferenceTypography(c)
	session := a.currentSession()
	provider := ""
	if session.ID != "" {
		provider = a.selectedModel().ProviderID
	}
	a.trackFooterUsage(c, &a.footerUsage, session, provider, false)
	c.Root().Background(design.Glass(a.palette, settings.Surface, c.Vibrancy()))
	a.registerSettingsShortcuts(c)
	viewport, _ := c.Size()
	showChatPanels := a.route == "chat" && a.currentSession().ID != ""
	open := [3]bool{a.sidebar, a.right && showChatPanels, a.files && showChatPanels}
	keyboardStep := a.sidebarContentWidth - a.sidebarSplit
	if open[0] && !a.panelAnimating(0) && a.sidebarSplit > 0 &&
		max(keyboardStep, -keyboardStep) > 0.5 {
		// A keyboard step of the sidebar divider lands after the frame that
		// built the split: adopt it here, before this frame lays anything
		// out, instead of asking the split to report the move again. A drag
		// also leaves the split size changed, but in the pass that drew it;
		// sidebarSplit remembers that value, so only a later step adopts.
		a.sidebarWidth = min(max(a.sidebarContentWidth+1, design.SidebarMin), design.SidebarMax)
		a.state.Settings.SidebarWidth = float64(a.sidebarWidth)
		a.schedulePaneSave(c, paneSidebar)
	}
	a.fit = fitPanels(viewport, a.sidebarWidth, open, a.panelOpened, a.rightExpanded)
	a.layoutPanels(c, viewport, open, reduced)
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		sidebar := a.panelPainted[0]
		if sidebar > 0.5 && !a.panelAnimating(0) {
			// A drag tracks the pointer directly; the edge bounce is a
			// picture, so the width the app keeps stays where the pointer
			// asked and the columns beside it move with the painted seam.
			sidebar = a.sidebarResize.paint(c, sidebar, reduced)
			a.panelPainted[0] = sidebar
			a.visibleSidebarWidth = sidebar
			a.frameColumns = paintPanels(viewport, a.panelPainted, a.fit)
		}
		if a.fit.Sidebar && sidebar > 0.5 {
			// The source width includes its right border. The divider owns
			// that one-pixel line, so only the interior belongs to the pane.
			a.sidebarContentWidth = max(0, sidebar-1)
			pane := func() { a.panelContentBox(c, 0, func() { a.sidebarView(c) }) }
			if a.panelAnimating(0) {
				// While the column eases, a plain row can paint it all the
				// way to zero: the split keeps ten DIPs for its grip, which
				// would leave a sliver behind at the end of a close.
				ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
					ui.Box(c).Width(sidebar).Shrink(0).Clip().Children(pane)
					ui.Column(c).Grow(1).Children(func() { a.mainView(c) })
				})
			} else {
				before := a.sidebarContentWidth
				ui.Split(c, &a.sidebarContentWidth, func() { a.panelContentBox(c, 0, func() { a.sidebarView(c) }) }, func() { a.mainView(c) }).Fill()
				a.sidebarSplit = a.sidebarContentWidth
				if a.sidebarContentWidth != before {
					// The split applied this frame's pointer move before it
					// built the panes. The size carries the bounce painted
					// this frame: take it back out so the width the app
					// keeps, and saves, is the one the pointer asked for.
					requested := a.sidebarContentWidth + 1 - a.sidebarResize.painted
					a.sidebarWidth = a.sidebarResize.observe(c, requested, design.SidebarMin, design.SidebarMax, reduced)
					a.state.Settings.SidebarWidth = float64(a.sidebarWidth)
					a.schedulePaneSave(c, paneSidebar)
				}
			}
		} else {
			a.sidebarResize.reset()
			a.mainView(c)
		}
	})
	a.overlays(c)
	a.sessionDeleteDialog(c)
	a.sessionInfo(c)
	a.imageDialog(c)
	if a.toast != "" && time.Now().Before(a.toastUntil) {
		ui.Overlay(c, func() {
			ui.Text(c, tr(c, a.toast)).Absolute().Bottom(30).AlignSelf(ui.Center).Padding(10, 16).Radius(10).Background(a.palette.Raised).Border(1, a.palette.Border)
		})
		c.After(time.Until(a.toastUntil))
	}
	a.flushPaneSave(c)
}

func (a *App) mainView(c *ui.Context) {
	ui.Column(c).Grow(1).FillHeight().AlignItems(ui.Stretch).Children(func() {
		if a.route == "settings" {
			a.titlebar(c)
			a.settingsView(c)
			return
		}
		width, height := c.Size()
		reduced := a.reduceMotion(c)
		columns := a.frameColumns
		if a.fit.Sidebar && !a.panelAnimating(0) {
			// The split has already applied this frame's pointer move: size
			// the columns beside it from that width, not from the one the
			// frame began with, so a drag never leaves them a frame behind.
			columns = paintPanels(width, [3]float32{a.sidebarContentWidth + 1, a.panelPainted[1], a.panelPainted[2]}, a.fit)
			a.frameColumns = columns
		}
		a.chatWidth = columns.Chat
		band := titlebarBandHeight()
		workspaceView := func() {
			ui.Column(c).Label(tr(c, "Tools workspace")).Fill().MinWidth(0).Children(func() {
				if a.fit.Right {
					a.workspace.BodyBelow(c, band)
				} else {
					a.workspace.View(c)
				}
			})
		}
		conversation := func() {
			ui.Column(c).Label(tr(c, "Conversation pane")).Fill().Padding(band, 0, 0, 0).Children(func() {
				if !a.terminal {
					a.terminalResize.reset()
					a.chatView(c)
					return
				}
				terminalHeight := max(design.TerminalMin, min(a.terminalHeight, height*design.TerminalMaxFraction))
				// The chat pane above the terminal takes the rest of the
				// column, so the terminal's own limits bound it.
				topHeight := a.terminalResize.paint(c, height-38-terminalHeight, reduced)
				minTop := max(0, height-38-height*design.TerminalMaxFraction)
				maxTop := height - 38 - design.TerminalMin
				c.Key("conversation-terminal-split")
				before := topHeight
				ui.SplitVertical(c, &topHeight, func() { a.chatView(c) }, func() { ui.Column(c).Label(tr(c, "Bottom terminal")).Fill().Children(func() { a.bottom.View(c) }) }).Grow(1).FillWidth()
				if topHeight != before {
					requested := topHeight - a.terminalResize.painted
					topHeight = a.terminalResize.observe(c, requested, minTop, maxTop, reduced)
					a.terminalHeight = max(design.TerminalMin, min(height*design.TerminalMaxFraction, height-38-topHeight))
					a.state.Settings.TerminalHeight = float64(a.terminalHeight)
					a.schedulePaneSave(c, paneTerminal)
				}
			})
		}
		content := func() {
			// Takeover takes the whole column once its widening tween has
			// landed; until then the split shows the conversation leaving.
			if a.fit.Right && a.rightExpanded && !a.panelAnimating(1) {
				workspaceView()
				return
			}
			if a.fit.Right && columns.Right > 0.5 {
				chatWidth := columns.Chat
				if a.panelAnimating(1) {
					c.Key("conversation-workspace-split")
					// A plain row paints the easing column all the way to zero;
					// the split would stop ten DIPs short for its grip.
					ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
						ui.Column(c).Grow(1).Children(conversation)
						ui.Box(c).Width(columns.Right).Shrink(0).Clip().Children(func() {
							a.panelContentBox(c, 1, workspaceView)
						})
					})
					return
				}
				// The conversation pane keeps at least its own minimum, and
				// the pane beside it keeps at least the right pane's.
				maxChat := columns.Chat + columns.Right - design.RightMin
				chatWidth = a.rightResize.paint(c, chatWidth, reduced)
				c.Key("conversation-workspace-split")
				before := chatWidth
				ui.Split(c, &chatWidth, conversation, workspaceView).Grow(1).FillWidth()
				if chatWidth != before {
					requested := chatWidth - a.rightResize.painted
					chatWidth = a.rightResize.observe(c, requested, design.ChatMin, maxChat, reduced)
					a.rightWidth = max(design.RightMin, min(columns.Chat+columns.Right-design.ChatMin, columns.Chat+columns.Right-chatWidth))
					a.state.Settings.RightPaneWidth = float64(a.rightWidth)
					a.schedulePaneSave(c, paneRight)
				}
			} else {
				a.rightResize.reset()
				conversation()
			}
		}
		mainContent := func() {
			files := func() { a.panelContentBox(c, 2, func() { a.workspace.FilesDockBelow(c, band) }) }
			if a.fit.Files && columns.Files > 0.5 {
				available := width - columns.Sidebar
				firstWidth := available - columns.Files
				if a.panelAnimating(2) {
					c.Key("files-dock-split")
					ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
						ui.Column(c).Grow(1).Children(content)
						ui.Box(c).Width(columns.Files).Shrink(0).Clip().Children(files)
					})
					return
				}
				// The content pane keeps what the files dock's own limits leave.
				minFirst, maxFirst := max(0, available-design.FilesMax), available-design.FilesMin
				firstWidth = a.filesResize.paint(c, firstWidth, reduced)
				c.Key("files-dock-split")
				before := firstWidth
				ui.Split(c, &firstWidth, content, func() { a.workspace.FilesDockBelow(c, band) }).Grow(1).FillWidth()
				if firstWidth != before {
					requested := firstWidth - a.filesResize.painted
					firstWidth = a.filesResize.observe(c, requested, minFirst, maxFirst, reduced)
					a.filesWidth = max(design.FilesMin, min(design.FilesMax, available-firstWidth))
					a.state.Settings.FilesWidth = float64(a.filesWidth)
					a.schedulePaneSave(c, paneFiles)
				}
			} else {
				a.filesResize.reset()
				content()
			}
		}
		// The content row spans the full window height and the title bar
		// overlays it: the pane surfaces reach the top edge like the source
		// right pane, while the panes keep their content clear of the band.
		ui.Column(c).Grow(1).FillWidth().Children(mainContent)
		a.titlebarOverlay(c)
	})
}

func (a *App) send(text string) bool {
	if a.pendingNew || a.attachmentImports[a.attachmentScope()] > 0 || a.modelID == "" || a.sessionID == "" && a.modelNoAgents() {
		return false
	}
	text = strings.TrimSpace(text)
	if text == "" && len(a.attachments) == 0 && len(a.annotations) == 0 {
		return false
	}
	cmd := contracts.Command{Kind: "message.send", ProjectID: a.projectID, Send: &contracts.SendRequest{SessionID: a.sessionID, Text: text, ModelID: a.modelID, ReasoningLevel: a.reasoning, DeviceID: a.deviceID, Branch: a.branch, Attachments: append([]contracts.Attachment(nil), a.attachments...)}}
	cmd.Send.Annotations = append([]contracts.Annotation(nil), a.annotations...)
	cmd.Send.RequestID = "primary:" + rand.Text()
	busy := a.currentSession().Status == contracts.Working || a.currentSession().Status == contracts.AwaitingInput
	// A steer joins the live response at its next step; otherwise the send
	// waits in the queue until the turn ends (General.ActiveTurnSendBehavior).
	if busy && a.activeTurnSend() == "steer" {
		cmd.Send.Delivery = "steer"
	}
	if a.pendingSends == nil {
		a.pendingSends = make(map[string]primaryPendingSend)
	}
	a.pendingSends[a.sessionID] = primaryPendingSend{RequestID: cmd.Send.RequestID, DraftEpoch: a.draftEpoch, StartedAt: a.now().UTC(), Queued: busy, Steer: cmd.Send.Delivery == "steer"}
	cmd.Send.Checkout = a.composerFooterCheckoutPlan()
	if a.sessionID == "" {
		a.pendingNew = true
		a.pendingNewRequestID = cmd.Send.RequestID
		a.pendingNewEpoch = a.draftEpoch
		a.pendingNewRuns = make(map[string]bool, len(a.state.Runs))
		for _, run := range a.state.Runs {
			a.pendingNewRuns[run.ID] = true
		}
		// The dock clock answers the send key, not the host round trip: the
		// composer starts travelling while the session is still being minted.
		a.launchingNewSession = true
	}
	a.draft = ""
	a.attachments = nil
	a.annotations = nil
	a.workspace.ResetAnnotations()
	a.drafts[a.sessionID] = ""
	a.dispatch(cmd)
	return a.draft == "" && len(a.attachments) == 0 && len(a.annotations) == 0
}

// activeTurnSend is what a send does while the agent runs: the reference's
// ActiveTurnSendBehavior, whose default steers the live response.
func (a *App) activeTurnSend() string {
	if a.state.Settings.General.ActiveTurnSendBehavior == "queue" {
		return "queue"
	}
	return "steer"
}

// activeTurnSendDescription names the selected behavior for its settings row.
func activeTurnSendDescription(behavior string) string {
	if behavior == "queue" {
		return "Keep the message in the queue until the running response ends."
	}
	return "Send as a steer into the running response."
}

func (a *App) syncNativeAppearance() {
	if a.win == nil {
		return
	}
	source := mygo.ThemeSystem
	switch strings.ToLower(a.state.Settings.Appearance.Mode) {
	case "dark":
		source = mygo.ThemeDark
	case "light":
		source = mygo.ThemeLight
	}
	mygo.Theme.SetSource(source)
	a.win.SetVibrancy(a.NativeVibrancy())
}
func (a *App) toggleTerminal() {
	a.terminal = !a.terminal
	if a.terminal {
		a.bottom.OpenSurface("terminal")
	}
}
func (a *App) setPanel(which int, open bool) {
	wasOpen := [3]bool{a.sidebar, a.right, a.files}[which]
	shown := [3]bool{a.fit.Sidebar, a.fit.Right, a.fit.Files}[which]
	switch which {
	case 0:
		a.sidebar = open
	case 1:
		a.right = open
		if !open {
			a.rightExpanded = false
			a.workspace.StopAnnotations()
		}
	case 2:
		a.files = open
	}
	if open && (!wasOpen || !shown) {
		a.panelOpened[which] = max(a.panelOpened[0], a.panelOpened[1], a.panelOpened[2]) + 1
	}
	// A toggle eases the column between its painted and settled widths; a
	// settled panel that is only being promoted has nothing to move yet.
	if open != wasOpen || (open && !shown) {
		a.panelMotion[which] = true
	}
}

// toggleWorkspaceExpanded hands the workspace the whole column or returns it
// to its own width. Both ends ride the right column's width tween.
func (a *App) toggleWorkspaceExpanded() {
	a.rightExpanded = !a.rightExpanded
	a.setPanel(1, true)
	a.panelMotion[1] = true
}

func (a *App) togglePanel(which int) {
	open := [3]bool{a.sidebar, a.right, a.files}[which]
	shown := [3]bool{a.fit.Sidebar, a.fit.Right, a.fit.Files}[which]
	a.setPanel(which, !open || !shown)
}
func (a *App) RuntimePlatform() string      { return runtime.GOOS }
func (a *App) Snapshot() contracts.Snapshot { return a.state }
func (a *App) String() string               { return fmt.Sprintf("GCode %s revision %d", a.route, a.state.Revision) }
