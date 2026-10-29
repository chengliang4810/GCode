// Package desktop implements the native application shell.
package desktop

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"gcode/internal/i18n"
	localize "gcode/internal/i18n/view"
	"gcode/internal/workbench"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

type App struct {
	cloudPage      cloudSettingsPageState
	branchCreation *branchCreation
	now            func() time.Time
	pendingSends   map[string]primaryPendingSend
	sendSequence   uint64
	composerControls
	footerUsage                                                   composerUsageState
	footerGit                                                     composerFooterGitState
	sideComposers                                                 map[string]*sideComposerState
	background                                                    backgroundSettingsState
	pickAttachmentFiles                                           func() ([]string, error)
	attachmentImports                                             map[string]int
	draftEpoch                                                    uint64
	mediaLoading                                                  map[string]bool
	sidebarProjectID                                              string
	sidebarProjectMenu                                            bool
	sidebarList                                                   ui.ListState
	sidebarUI                                                     sidebarState
	commands                                                      commandPaletteState
	newProject                                                    newProjectState
	annotations                                                   []contracts.Annotation
	focusComposer                                                 bool
	visibleSidebarWidth                                           float32
	sidebarContentWidth                                           float32
	imageZoom                                                     float32
	conversationQuery                                             string
	findIndex                                                     int
	actionOpen                                                    bool
	actionName, actionCommand                                     string
	fit                                                           panelAllocation
	panelOpened                                                   [3]uint64
	rightExpanded                                                 bool
	chatWidth                                                     float32
	sessionStates                                                 map[string]sessionViewState
	canvasTarget                                                  *canvasTarget
	chatWorkspacePending                                          bool
	desiredSettings                                               *contracts.Settings
	authoritativeSettings                                         contracts.Settings
	settingsPending                                               int
	bottom                                                        *workbench.State
	pendingNew                                                    bool
	pendingNewRequestID                                           string
	pendingNewEpoch                                               uint64
	pendingNewRuns                                                map[string]bool
	service                                                       contracts.Service
	ctx                                                           context.Context
	cancel                                                        context.CancelFunc
	win                                                           *mygo.Window
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
	draft, search, settingsSearch, paletteQuery, errorText, toast string
	attachments                                                   []contracts.Attachment
	drafts                                                        map[string]string
	expanded                                                      map[string]bool
	integrationUI                                                 integrationUIState
	messages                                                      ui.ListState
	md                                                            markdown.Options
	userMD                                                        markdown.Options
	composer                                                      ui.Handle
	composerSurface                                               ui.Handle
	projectMenu, deviceMenu, branchMenu, viewMenu, paletteOpen    bool
	renameOpen, projectOpen, attachmentOpen, imageOpen            bool
	renameID, renameText, imageRef                                string
	branch, checkout                                              string
	toastUntil                                                    time.Time
	palette                                                       design.Tokens
	services                                                      ui.Services
}

type Options struct{ Now func() time.Time }

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
	a := &App{service: service, ctx: ctx, cancel: cancel, state: snapshot, jobs: make(chan contracts.Command, 128), panelOpened: [3]uint64{1, 2, 3},
		sidebar: true, route: "chat", section: "General", checkout: "Current checkout",
		drafts: make(map[string]string), expanded: make(map[string]bool), history: []string{"chat"}}
	a.reasoning = "High"
	a.now = options.Now
	if a.now == nil {
		a.now = time.Now
	}
	a.authoritativeSettings = copySettings(snapshot.Settings)
	a.sidebarUI.archivedOpen = true
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
	a.workspace.OnToggleExpand = func() { a.rightExpanded = !a.rightExpanded; a.setPanel(1, true) }
	a.workspace.OnAnnotation = a.attachAnnotation
	a.accessMode = contracts.AccessFullAccess
	a.refreshBackground()
	return a, nil
}

func (a *App) Attach(win *mygo.Window) error {
	a.win = win
	a.systemDark = mygo.Theme.IsDark()
	a.refreshBackground()
	offTheme := mygo.Theme.OnUpdated(func() {
		win.Update(func() {
			if !a.closed {
				a.systemDark = mygo.Theme.IsDark()
				a.refreshBackground()
			}
		})
	})
	win.OnClosed(offTheme)
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
	a.syncNativeAppearance()
	a.syncNativeLanguage()
	a.workspace.OnToggleExpand = func() { a.rightExpanded = !a.rightExpanded; a.setPanel(1, true) }
	a.workspace.OnAnnotation = a.attachAnnotation
	events, err := a.service.Subscribe(a.ctx)
	if err != nil {
		return err
	}
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
		a.closed = true
		a.cancel()
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
		a.state = s
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
		a.setSelectedSession("", a.route == "pull-requests")
		delete(a.sessionStates, removed)
		delete(a.drafts, removed)
		delete(a.pendingSends, removed)
	}
	a.reconcileBranchCreation()
}

func (a *App) applyEvent(event contracts.Event) {
	if event.Snapshot != nil {
		a.apply(*event.Snapshot)
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
			if a.sessionID == "" && a.route == "chat" {
				a.selectSession(session.ID)
			}
			return
		}
	}
	a.pendingNew = false
	a.pendingNewRequestID = ""
}

func (a *App) creationCommandApplied(command contracts.Command, err error) {
	a.branchCommandApplied(command, err)
	if err == nil && a.pendingNew && command.Kind == "message.send" && command.Send != nil && command.Send.SessionID == "" && command.Send.RequestID == a.pendingNewRequestID {
		// A concurrent deletion can remove both the created row and its run
		// before this complete snapshot. A finished command must still settle.
		a.pendingNew = false
		a.pendingNewRequestID = ""
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
	s := a.state.Settings
	s.Shortcuts = append([]contracts.Shortcut(nil), s.Shortcuts...)
	a.desiredSettings = &s
	a.settingsPending++
	a.syncNativeAppearance()
	a.syncNativeLanguage()
	a.dispatch(contracts.Command{Kind: "settings.update", Settings: &s})
	a.refreshBackground()
}
func (a *App) OpenRoute(route string) {
	// A session pick or annotation may leave settings before the next View.
	// Retire that visit before replacing the old route with Cloud again.
	if a.cloudPage.active && !a.cloudSettingsVisible() {
		a.syncCloudSettingsPage()
	}
	defer a.syncCloudSettingsPage()
	a.refreshBackground()
	if route != "settings" && !strings.HasPrefix(route, "settings/") {
		a.clearShortcutRecording()
		if a.chatWorkspacePending && route != "pull-requests" {
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
		a.section = strings.TrimPrefix(route, "settings/")
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
	if route == "pull-requests" {
		a.route = route
		a.right = true
		a.workspace.OpenSurface("pullrequests")
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
	a.modelProvider, a.modelQuery = "", ""
	a.projectMenu, a.deviceMenu, a.branchMenu, a.accessMenu = false, false, false, false
	for _, s := range a.state.Sessions {
		if s.ID == id {
			a.invalidateCanvasCloudTarget(s)
			a.projectID = s.ProjectID
			a.deviceID = s.DeviceID
			a.modelID = s.ModelID
			a.accessMode = s.AccessMode
			a.plan = s.Plan
			a.reasoning = "High"
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
	if a.sessionID == "" && a.modelNoAgents() {
		return a.t("No agents available")
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
	theme := a.palette.UI()
	theme.Font = design.Font(settings.Font)
	if a.reduceMotion(c) {
		c.DisableAnimations()
	}
	if settings.FontSize > 0 {
		theme.FontSize = 13 * float32(settings.FontSize) / 16
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
	a.fit = fitPanels(viewport, a.sidebarWidth, open, a.panelOpened, a.rightExpanded)
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		if a.fit.Sidebar {
			a.visibleSidebarWidth = max(design.SidebarMin, min(design.SidebarMax, min(a.sidebarWidth, a.fit.SidebarLimit)))
			// The source width includes its right border. MyGo's split owns
			// that one-pixel line, so only the interior belongs to the pane.
			a.sidebarContentWidth = a.visibleSidebarWidth - 1
			split := ui.Split(c, &a.sidebarContentWidth, func() { a.sidebarView(c) }, func() { a.mainView(c) }).Fill()
			if split.Changed() {
				a.sidebarWidth = max(design.SidebarMin, min(design.SidebarMax, a.sidebarContentWidth+1))
				a.state.Settings.SidebarWidth = float64(a.sidebarWidth)
				a.saveSettings()
			}
		} else {
			a.mainView(c)
		}
	})
	a.overlays(c)
	a.sessionInfo(c)
	a.imageDialog(c)
	if a.toast != "" && time.Now().Before(a.toastUntil) {
		ui.Overlay(c, func() {
			ui.Text(c, tr(c, a.toast)).Absolute().Bottom(30).AlignSelf(ui.Center).Padding(10, 16).Radius(10).Background(a.palette.Raised).Border(1, a.palette.Border)
		})
		c.After(time.Until(a.toastUntil))
	}
}

func (a *App) mainView(c *ui.Context) {
	ui.Column(c).Grow(1).FillHeight().AlignItems(ui.Stretch).Children(func() {
		if a.route == "settings" {
			a.titlebar(c)
			a.settingsView(c)
			return
		}
		width, height := c.Size()
		columns := allocatePanels(width, a.sidebarWidth, a.filesWidth, a.rightWidth, a.fit, a.rightExpanded)
		a.chatWidth = columns.Chat
		a.titlebar(c)
		workspaceView := func() {
			ui.Column(c).Label(tr(c, "Tools workspace")).Fill().MinWidth(0).Children(func() {
				if a.fit.Right {
					a.workspace.Body(c)
				} else {
					a.workspace.View(c)
				}
			})
		}
		conversation := func() {
			ui.Column(c).Label(tr(c, "Conversation pane")).Fill().Children(func() {
				if !a.terminal {
					a.chatView(c)
					return
				}
				terminalHeight := max(design.TerminalMin, min(a.terminalHeight, height*design.TerminalMaxFraction))
				topHeight := height - 38 - terminalHeight
				c.Key("conversation-terminal-split")
				split := ui.SplitVertical(c, &topHeight, func() { a.chatView(c) }, func() { ui.Column(c).Label(tr(c, "Bottom terminal")).Fill().Children(func() { a.bottom.View(c) }) }).Grow(1).FillWidth()
				if split.Changed() {
					a.terminalHeight = max(design.TerminalMin, min(height*design.TerminalMaxFraction, height-38-topHeight))
					a.state.Settings.TerminalHeight = float64(a.terminalHeight)
					a.saveSettings()
				}
			})
		}
		content := func() {
			if a.route == "pull-requests" {
				workspaceView()
				return
			}
			if a.fit.Right && a.rightExpanded {
				workspaceView()
				return
			}
			if a.fit.Right {
				chatWidth := columns.Chat
				c.Key("conversation-workspace-split")
				split := ui.Split(c, &chatWidth, conversation, workspaceView).Grow(1).FillWidth()
				if split.Changed() {
					a.rightWidth = max(design.RightMin, min(columns.Chat+columns.Right-design.ChatMin, columns.Chat+columns.Right-chatWidth))
					a.state.Settings.RightPaneWidth = float64(a.rightWidth)
					a.saveSettings()
				}
			} else {
				conversation()
			}
		}
		mainContent := func() {
			if a.fit.Files {
				available := width - columns.Sidebar
				firstWidth := available - columns.Files
				c.Key("files-dock-split")
				split := ui.Split(c, &firstWidth, content, func() { a.workspace.FilesView(c) }).Grow(1).FillWidth()
				if split.Changed() {
					a.filesWidth = max(design.FilesMin, min(design.FilesMax, available-firstWidth))
					a.state.Settings.FilesWidth = float64(a.filesWidth)
					a.saveSettings()
				}
			} else {
				content()
			}
		}
		ui.Column(c).Grow(1).FillWidth().Children(mainContent)
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
	cmd.Send.AccessMode = a.accessMode
	cmd.Send.Plan = a.plan
	cmd.Send.Annotations = append([]contracts.Annotation(nil), a.annotations...)
	a.sendSequence++
	cmd.Send.RequestID = fmt.Sprintf("primary:%s:%d", a.sessionID, a.sendSequence)
	if a.pendingSends == nil {
		a.pendingSends = make(map[string]primaryPendingSend)
	}
	a.pendingSends[a.sessionID] = primaryPendingSend{RequestID: cmd.Send.RequestID, DraftEpoch: a.draftEpoch, StartedAt: a.now().UTC(), Queued: a.currentSession().Status == contracts.Working || a.currentSession().Status == contracts.AwaitingInput}
	cmd.Send.Checkout = a.composerFooterCheckoutPlan()
	if a.sessionID == "" {
		a.pendingNew = true
		a.pendingNewRequestID = cmd.Send.RequestID
		a.pendingNewEpoch = a.draftEpoch
		a.pendingNewRuns = make(map[string]bool, len(a.state.Runs))
		for _, run := range a.state.Runs {
			a.pendingNewRuns[run.ID] = true
		}
	}
	a.draft = ""
	a.attachments = nil
	a.annotations = nil
	a.workspace.ResetAnnotations()
	a.drafts[a.sessionID] = ""
	a.dispatch(cmd)
	return a.draft == "" && len(a.attachments) == 0 && len(a.annotations) == 0
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
	material := mygo.VibrancySidebar
	if strings.EqualFold(a.state.Settings.Appearance.Surface, "opaque") {
		material = mygo.VibrancyNone
	}
	a.win.SetVibrancy(material)
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
}

func (a *App) togglePanel(which int) {
	open := [3]bool{a.sidebar, a.right, a.files}[which]
	shown := [3]bool{a.fit.Sidebar, a.fit.Right, a.fit.Files}[which]
	a.setPanel(which, !open || !shown)
}
func (a *App) RuntimePlatform() string      { return runtime.GOOS }
func (a *App) Snapshot() contracts.Snapshot { return a.state }
func (a *App) String() string               { return fmt.Sprintf("Zeron %s revision %d", a.route, a.state.Revision) }
