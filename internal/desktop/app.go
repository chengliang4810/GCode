package desktop

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"

	"gcode/internal/appmodel"
	"github.com/egoist/mygo/ui"
)

type terminalLoad struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	canceled bool
	terminal TerminalSurface
}

func (l *terminalLoad) finish(term TerminalSurface) bool {
	l.mu.Lock()
	if l.canceled {
		l.mu.Unlock()
		if term != nil {
			_ = term.Close()
		}
		return false
	}
	l.terminal = term
	l.mu.Unlock()
	return true
}

func (l *terminalLoad) take() (TerminalSurface, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.canceled {
		return nil, false
	}
	term := l.terminal
	l.terminal = nil
	return term, true
}

func (l *terminalLoad) stop() {
	l.mu.Lock()
	if l.canceled {
		l.mu.Unlock()
		return
	}
	l.canceled = true
	term := l.terminal
	l.terminal = nil
	l.mu.Unlock()

	l.cancel()
	if term != nil {
		_ = term.Close()
	}
}

type App struct {
	mu sync.RWMutex

	service                 Service
	workspace               Workspace
	terminalFactory         TerminalFactory
	dispatch                func(func())
	invalidate              func()
	data                    appmodel.Snapshot
	workspaceData           WorkspaceSnapshot
	workspaceRoot           string
	workspacePath           string
	workspacePreviewPath    string
	workspaceLoadVersion    uint64
	visualSample            bool
	activeSessionID         string
	page                    screen
	right                   rightTab
	rightOpen               bool
	rightResponsiveOverride bool
	leftWidth               float32
	centerWidth             float32
	input                   string
	projectPath             string
	sendMode                appmodel.SendMode
	// sendModeDirty records a one-session composer override. It must remain
	// independent from settingsDirty: changing the composer is a per-send
	// choice and must not be replaced by a controller snapshot refresh.
	sendModeDirty        bool
	settingsDraft        SettingsDraft
	settingsDirty        bool
	settingsSaveVersion  uint64
	codeHighlights       map[string][]ui.Span
	codeHighlightPending map[string]bool
	conversationLists    map[string]*ui.ListState
	diffLists            map[string]*ui.ListState
	diffCache            map[string]*cachedDiff
	diffExpanded         map[string]bool
	toolExpanded         map[string]bool
	projectExpanded      map[string]bool
	markdownCache        map[string]cachedMessageRender
	markdownPending      map[string]bool
	errorText            string
	terminal             TerminalSurface
	terminalCWD          string
	terminalLoading      bool
	terminalLoad         *terminalLoad
	closed               bool
}

func New(service Service) *App {
	return Open(service, OpenOptions{})
}

func Open(service Service, options OpenOptions) *App {
	data := cloneSnapshot(options.Initial)
	if data.Settings.DefaultSendMode == "" {
		data.Settings.DefaultSendMode = appmodel.Correction
	}
	if data.Settings.Theme == "" {
		data.Settings.Theme = "system"
	}
	a := &App{
		service:              service,
		workspace:            options.Workspace,
		terminalFactory:      options.TerminalFactory,
		dispatch:             options.Dispatch,
		invalidate:           options.Invalidate,
		data:                 data,
		workspaceData:        cloneWorkspaceSnapshot(options.InitialWorkspace),
		visualSample:         options.VisualSample,
		rightOpen:            true,
		leftWidth:            256,
		centerWidth:          650,
		sendMode:             data.Settings.DefaultSendMode,
		codeHighlights:       make(map[string][]ui.Span),
		codeHighlightPending: make(map[string]bool),
		conversationLists:    make(map[string]*ui.ListState),
		diffLists:            make(map[string]*ui.ListState),
		diffCache:            make(map[string]*cachedDiff),
		diffExpanded:         make(map[string]bool),
		toolExpanded:         make(map[string]bool),
		projectExpanded:      make(map[string]bool),
		markdownCache:        make(map[string]cachedMessageRender),
		markdownPending:      make(map[string]bool),
	}
	if len(data.Sessions) > 0 {
		a.activeSessionID = data.Sessions[0].ID
		a.workspaceRoot = data.Sessions[0].Cwd
		a.workspacePath = data.Sessions[0].Cwd
	}
	a.settingsDraft = settingsDraft(data.Settings)
	return a
}

// SetDispatch connects the App to the native window's main-thread queue. A
// mygo Window.Update value is the intended implementation. All results from
// Refresh, workspace I/O, service operations, and terminal creation use this
// hook before changing view state.
func (a *App) SetDispatch(dispatch func(func())) {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.dispatch = dispatch
	a.mu.Unlock()
}

// SetInvalidate optionally requests a redraw after a dispatched mutation.
// Window.Update already redraws, so callers that pass Window.Update normally
// do not need this hook.
func (a *App) SetInvalidate(invalidate func()) {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.invalidate = invalidate
	a.mu.Unlock()
}

// SetUIHooks is a convenience for native hosts that expose both a main-thread
// dispatcher and an explicit redraw callback.
func (a *App) SetUIHooks(dispatch func(func()), invalidate func()) {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.dispatch = dispatch
	a.invalidate = invalidate
	a.mu.Unlock()
}

func (a *App) View() *ui.Content {
	return ui.View(a.view)
}

func (a *App) Refresh(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	a.mu.RLock()
	service := a.service
	a.mu.RUnlock()
	if service == nil {
		return errors.New("服务尚未连接")
	}
	snapshot, err := service.Snapshot(ctx)
	if err != nil {
		return err
	}
	a.post(func() { a.updateSnapshot(snapshot) })
	a.mu.RLock()
	workspacePath := a.workspacePath
	a.mu.RUnlock()
	if workspacePath != "" {
		a.loadWorkspace(workspacePath)
	}
	return nil
}

func (a *App) Update(snapshot appmodel.Snapshot) {
	a.updateSnapshot(snapshot)
}

func (a *App) updateSnapshot(snapshot appmodel.Snapshot) {
	if a == nil {
		return
	}
	var loadDir string
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	previousData := a.data
	a.data = cloneSnapshot(snapshot)
	if a.data.Settings.DefaultSendMode == "" {
		a.data.Settings.DefaultSendMode = appmodel.Correction
	}
	if a.data.Settings.Theme == "" {
		a.data.Settings.Theme = "system"
	}
	previousID := a.activeSessionID
	if a.activeSessionID == "" || !hasSession(a.data.Sessions, a.activeSessionID) {
		if len(a.data.Sessions) > 0 {
			a.activeSessionID = a.data.Sessions[0].ID
		} else {
			a.activeSessionID = ""
		}
	}
	if active, ok := activeSession(a.data.Sessions, a.activeSessionID); ok &&
		(previousID != a.activeSessionID || a.workspaceRoot == "" || a.workspacePath == "") {
		a.workspaceRoot = active.Cwd
		a.workspacePath = active.Cwd
		a.workspacePreviewPath = ""
		a.workspaceData = WorkspaceSnapshot{}
		loadDir = active.Cwd
	}
	if loadDir == "" && workspaceSnapshotNeedsRefresh(previousData, a.data, a.activeSessionID) {
		loadDir = a.workspacePath
	}
	if !a.settingsDirty {
		a.settingsDraft = settingsDraft(a.data.Settings)
	} else {
		// A controller snapshot can update derived provider state while the
		// editor still owns the user's draft. Refresh only the selected
		// provider's credential marker so other draft fields remain untouched.
		refreshDraftProviderHasAPIKey(&a.settingsDraft, a.data.Settings.Providers)
	}
	if !a.sendModeDirty {
		a.sendMode = a.data.Settings.DefaultSendMode
	}
	a.mu.Unlock()
	if loadDir != "" {
		a.loadWorkspace(loadDir)
	}
}

func (a *App) UpdateWorkspace(snapshot WorkspaceSnapshot) {
	a.updateWorkspace(snapshot)
}

func (a *App) updateWorkspace(snapshot WorkspaceSnapshot) {
	if a == nil {
		return
	}
	a.mu.Lock()
	if snapshot.Preview == nil && a.workspaceData.Preview != nil {
		snapshot.Preview = a.workspaceData.Preview
	}
	a.workspaceData = cloneWorkspaceSnapshot(snapshot)
	a.mu.Unlock()
}

// post applies a view-state mutation on the host UI thread. The direct path
// exists for tests and hosts that deliberately run without a native window;
// production hosts should provide Dispatch.
func (a *App) post(fn func()) {
	if a == nil || fn == nil {
		return
	}
	a.mu.RLock()
	dispatch := a.dispatch
	invalidate := a.invalidate
	a.mu.RUnlock()
	if dispatch != nil {
		dispatch(func() {
			fn()
			if invalidate != nil {
				invalidate()
			}
		})
		return
	}
	fn()
	if invalidate != nil {
		invalidate()
	}
}

func (a *App) Close() error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	term := a.terminal
	a.terminal = nil
	load := a.terminalLoad
	a.terminalLoad = nil
	a.terminalLoading = false
	a.mu.Unlock()
	var closeErr error
	if term != nil {
		closeErr = term.Close()
	}
	if load != nil {
		load.stop()
	}
	return closeErr
}

func (a *App) view(c *ui.Context) {
	a.mu.RLock()
	data := snapshotForView(a.data)
	workspace := cloneWorkspaceSnapshot(a.workspaceData)
	workspaceRoot := a.workspaceRoot
	workspacePath := a.workspacePath
	activeID := a.activeSessionID
	page := a.page
	right := a.right
	rightPreference := a.rightOpen
	rightResponsiveOverride := a.rightResponsiveOverride
	leftWidth := a.leftWidth
	centerWidth := a.centerWidth
	input := a.input
	sendMode := a.sendMode
	settingsDraftValue := a.settingsDraft
	errorText := a.errorText
	terminal := a.terminal
	terminalLoading := a.terminalLoading
	visualSample := a.visualSample
	a.mu.RUnlock()

	p := newPalette(themeIsDark(c.Theme().Dark, data.Settings.Theme))
	c.SetTheme(p.theme())
	c.Root().Background(p.background).TextColor(p.text)
	width, _ := c.Size()
	const rightPanelBreakpoint float32 = 1180
	narrow := width < rightPanelBreakpoint
	if narrow {
		leftWidth = clampSidebarWidth(width, leftWidth)
	}
	rightOpen := rightPreference
	if narrow && !rightResponsiveOverride {
		rightOpen = false
	}

	ui.Column(c).Fill().Background(p.background).Children(func() {
		if page == screenSettings {
			a.settingsPage(c, p, data.Settings, settingsDraftValue, errorText, visualSample)
			return
		}
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			leftSplit := ui.Split(c, &leftWidth, func() {
				a.sidebar(c, p, data, activeID, visualSample)
			}, func() {
				if rightOpen {
					// The split's second pane contains a minimum-width resource
					// panel. Clamp the conversation pane before building it so an
					// explicit narrow-window expand cannot push the panel off-screen.
					centerWidth = clampCenterWidth(width, leftWidth, centerWidth)
					centerSplit := ui.Split(c, &centerWidth, func() {
						a.conversation(c, p, data, activeID, input, sendMode, errorText, rightOpen, narrow)
					}, func() {
						a.rightPanel(c, p, data, workspace, workspaceRoot, workspacePath, activeID, right, terminal, terminalLoading)
					}).Fill()
					if centerSplit.Changed() {
						a.mu.Lock()
						a.centerWidth = clampCenterWidth(width, leftWidth, centerWidth)
						a.mu.Unlock()
					}
				} else {
					a.conversation(c, p, data, activeID, input, sendMode, errorText, rightOpen, narrow)
				}
			}).Fill()
			if leftSplit.Changed() {
				a.mu.Lock()
				a.leftWidth = leftWidth
				a.mu.Unlock()
			}
		})
	})
}

func (a *App) invoke(name string, fn func(context.Context) error) {
	a.invokeAsync(name, fn, nil)
}

func (a *App) invokeAsync(name string, fn func(context.Context) error, onSuccess func()) {
	a.mu.RLock()
	service := a.service
	a.mu.RUnlock()
	if service == nil {
		a.setError("服务尚未连接，无法" + name)
		return
	}
	go func() {
		err := fn(context.Background())
		if err != nil {
			a.setError(name + "失败：" + err.Error())
			return
		}
		if onSuccess != nil {
			a.post(onSuccess)
		}
	}()
}

func (a *App) setError(message string) {
	a.post(func() { a.setErrorNow(message) })
}

func (a *App) setErrorNow(message string) {
	a.mu.Lock()
	a.errorText = message
	a.mu.Unlock()
}

func (a *App) clearError() {
	a.mu.Lock()
	a.errorText = ""
	a.mu.Unlock()
}

func (a *App) selectSession(id string) {
	if id == "" {
		return
	}
	a.mu.Lock()
	if !hasSession(a.data.Sessions, id) {
		a.mu.Unlock()
		return
	}
	selected, _ := activeSession(a.data.Sessions, id)
	a.activeSessionID = id
	a.projectExpanded[selected.ProjectID] = true
	// A composer choice belongs to the current session. Selecting another
	// session starts from the current global default and drops the old
	// session's transient override.
	a.sendMode = a.data.Settings.DefaultSendMode
	a.sendModeDirty = false
	a.workspaceRoot = selected.Cwd
	a.workspacePath = selected.Cwd
	a.workspacePreviewPath = ""
	a.workspaceData = WorkspaceSnapshot{}
	a.mu.Unlock()
	a.loadWorkspace(selected.Cwd)
}

func (a *App) loadWorkspace(cwd string) {
	if strings.TrimSpace(cwd) == "" {
		return
	}
	a.mu.RLock()
	workspace := a.workspace
	a.mu.RUnlock()
	if workspace == nil {
		return
	}
	a.mu.Lock()
	a.workspaceLoadVersion++
	loadVersion := a.workspaceLoadVersion
	a.mu.Unlock()
	go func() {
		snapshot, err := workspace.Snapshot(context.Background(), cwd)
		if err != nil {
			a.post(func() { a.setErrorNow("读取工作区失败：" + err.Error()) })
			return
		}
		a.post(func() {
			a.mu.RLock()
			activePath := a.workspacePath
			closed := a.closed
			staleLoad := loadVersion != a.workspaceLoadVersion
			a.mu.RUnlock()
			if closed || activePath != cwd || staleLoad {
				return
			}
			a.updateWorkspace(snapshot)
			a.mu.RLock()
			previewPath := a.workspacePreviewPath
			a.mu.RUnlock()
			if previewPath != "" {
				// Keep the selected preview visible while the directory listing
				// refreshes, then replace its contents from the same cwd.
				a.readWorkspaceFile(previewPath)
			}
		})
	}()
}

func (a *App) refreshWorkspace() {
	_, current, ok := a.workspaceDirectory()
	if !ok {
		return
	}
	a.loadWorkspace(current)
}

func workspaceSnapshotNeedsRefresh(previous, current appmodel.Snapshot, activeID string) bool {
	before, beforeOK := activeSession(previous.Sessions, activeID)
	after, afterOK := activeSession(current.Sessions, activeID)
	if !beforeOK || !afterOK {
		return false
	}
	if before.Cwd != after.Cwd {
		return true
	}
	if before.State != after.State && workspaceTerminalState(after.State) {
		return true
	}
	return toolResultCompleted(before, after)
}

func workspaceTerminalState(state appmodel.RunState) bool {
	switch state {
	case appmodel.Idle, appmodel.Failed, appmodel.Interrupted:
		return true
	default:
		return false
	}
}

func toolResultCompleted(previous, current appmodel.Session) bool {
	previousTools := make(map[string]appmodel.ToolCall)
	for _, message := range previous.Messages {
		for _, tool := range message.Tools {
			previousTools[tool.ID] = tool
		}
	}
	for _, message := range current.Messages {
		for _, tool := range message.Tools {
			before, ok := previousTools[tool.ID]
			if ok && isActiveToolState(before.State) && !isActiveToolState(tool.State) {
				return true
			}
		}
	}
	return false
}

func isActiveToolState(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "", "running", "pending", "in_progress":
		return true
	default:
		return false
	}
}

func (a *App) workspaceDirectory() (root, current string, ok bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.workspaceRoot, a.workspacePath, a.workspaceRoot != "" && a.workspacePath != ""
}

func (a *App) enterDirectory(path string) {
	root, current, ok := a.workspaceDirectory()
	if !ok || strings.TrimSpace(path) == "" {
		return
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(current, path)
	}
	path = filepath.Clean(path)
	if !isWithinPath(root, path) {
		a.setError("不能离开当前项目目录")
		return
	}
	a.mu.Lock()
	a.workspacePath = path
	a.workspacePreviewPath = ""
	a.workspaceData = WorkspaceSnapshot{}
	a.mu.Unlock()
	a.loadWorkspace(path)
}

func (a *App) parentDirectory() {
	root, current, ok := a.workspaceDirectory()
	if !ok || filepath.Clean(root) == filepath.Clean(current) {
		return
	}
	parent := filepath.Dir(current)
	if !isWithinPath(root, parent) {
		parent = root
	}
	a.enterDirectory(parent)
}

func (a *App) readWorkspaceFile(path string) {
	root, current, ok := a.workspaceDirectory()
	a.mu.RLock()
	workspace := a.workspace
	a.mu.RUnlock()
	if !ok || workspace == nil || strings.TrimSpace(path) == "" {
		return
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(current, path)
	}
	path = filepath.Clean(path)
	if !isWithinPath(root, path) {
		a.setError("不能读取项目目录之外的文件")
		return
	}
	relative, err := filepath.Rel(current, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		a.setError("不能读取当前目录之外的文件")
		return
	}
	a.mu.Lock()
	a.workspacePreviewPath = path
	a.mu.Unlock()
	go func(cwd, target, targetAbsolute string) {
		preview, err := workspace.ReadFile(context.Background(), cwd, target)
		if err != nil {
			a.post(func() { a.setErrorNow("读取文件失败：" + err.Error()) })
			return
		}
		a.post(func() {
			a.mu.RLock()
			stale := a.closed || a.workspacePath != cwd || a.workspacePreviewPath != targetAbsolute
			a.mu.RUnlock()
			if stale {
				return
			}
			a.mu.Lock()
			copyPreview := preview
			a.workspaceData.Preview = &copyPreview
			a.mu.Unlock()
		})
	}(current, relative, path)
}

func (a *App) openWorkspaceFile(path string) {
	root, current, ok := a.workspaceDirectory()
	a.mu.RLock()
	workspace := a.workspace
	a.mu.RUnlock()
	if !ok || workspace == nil || strings.TrimSpace(path) == "" {
		return
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(current, path)
	}
	path = filepath.Clean(path)
	if !isWithinPath(root, path) {
		a.setError("不能打开项目目录之外的文件")
		return
	}
	relative, err := filepath.Rel(current, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		a.setError("不能打开当前目录之外的文件")
		return
	}
	go func(cwd, target string) {
		if err := workspace.OpenEditor(context.Background(), cwd, target); err != nil {
			a.post(func() { a.setErrorNow("打开编辑器失败：" + err.Error()) })
		}
	}(current, relative)
}

func isWithinPath(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	if root == path {
		return true
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func (a *App) send(text string, mode appmodel.SendMode) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	a.mu.RLock()
	sessionID := a.activeSessionID
	service := a.service
	a.mu.RUnlock()
	if sessionID == "" {
		a.setError("请先选择或新建会话")
		return
	}
	if service == nil {
		a.setError("服务尚未连接，无法发送消息")
		return
	}
	a.invokeAsync("发送消息", func(ctx context.Context) error {
		return service.Send(ctx, sessionID, text, mode)
	}, func() {
		a.mu.Lock()
		if strings.TrimSpace(a.input) == text {
			a.input = ""
		}
		a.mu.Unlock()
	})
}

func (a *App) stop() {
	a.mu.RLock()
	sessionID := a.activeSessionID
	a.mu.RUnlock()
	if sessionID == "" {
		return
	}
	a.invoke("停止任务", func(ctx context.Context) error { return a.service.Stop(ctx, sessionID) })
}

func (a *App) withdraw(messageID string) {
	a.mu.RLock()
	sessionID := a.activeSessionID
	a.mu.RUnlock()
	a.invoke("撤回排队消息", func(ctx context.Context) error {
		return a.service.Withdraw(ctx, sessionID, messageID)
	})
}

func (a *App) resumeQueue() {
	a.mu.RLock()
	sessionID := a.activeSessionID
	a.mu.RUnlock()
	a.invoke("继续队列", func(ctx context.Context) error { return a.service.ResumeQueue(ctx, sessionID) })
}

func (a *App) permission(permission appmodel.Permission, optionID string) {
	a.mu.RLock()
	sessionID := a.activeSessionID
	a.mu.RUnlock()
	a.invoke("提交权限选择", func(ctx context.Context) error {
		return a.service.ReplyPermission(ctx, sessionID, permission.ID, optionID)
	})
}

func (a *App) ask(request appmodel.AskRequest, option string) {
	a.mu.RLock()
	sessionID := a.activeSessionID
	a.mu.RUnlock()
	a.invoke("提交问题选择", func(ctx context.Context) error {
		return a.service.ReplyAsk(ctx, sessionID, request.ID, []string{option})
	})
}

func (a *App) newSession(projectID string) {
	if projectID == "" {
		a.setError("请先打开项目")
		return
	}
	options := appmodel.NewSessionOptions{ProjectID: projectID}
	a.mu.RLock()
	service := a.service
	knownSessions := make(map[string]struct{}, len(a.data.Sessions))
	for _, session := range a.data.Sessions {
		knownSessions[session.ID] = struct{}{}
	}
	a.mu.RUnlock()
	if service == nil {
		a.setError("服务尚未连接，无法新建会话")
		return
	}
	a.invokeAsync("新建会话", func(ctx context.Context) error {
		if err := service.NewSession(ctx, options); err != nil {
			return err
		}
		snapshot, err := service.Snapshot(ctx)
		if err != nil {
			return err
		}
		newID := newestSessionID(snapshot.Sessions, knownSessions, projectID)
		a.post(func() {
			a.updateSnapshot(snapshot)
			if newID != "" {
				a.selectSession(newID)
			}
		})
		return nil
	}, nil)
}

func newestSessionID(sessions []appmodel.Session, known map[string]struct{}, projectID string) string {
	for index := len(sessions) - 1; index >= 0; index-- {
		session := sessions[index]
		if session.ProjectID == projectID {
			if _, exists := known[session.ID]; !exists {
				return session.ID
			}
		}
	}
	return ""
}

func (a *App) addProject(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		a.setError("请输入项目目录")
		return
	}
	a.invoke("打开项目", func(ctx context.Context) error { return a.service.AddProject(ctx, path) })
}

func (a *App) setProvider(sessionID, providerID string) {
	a.invoke("切换模型服务", func(ctx context.Context) error {
		return a.service.SetProvider(ctx, sessionID, providerID)
	})
}

func (a *App) setFullAccess(sessionID string, enabled bool) {
	a.invoke("更新权限模式", func(ctx context.Context) error {
		return a.service.SetFullAccess(ctx, sessionID, enabled)
	})
}

func (a *App) saveSettings() {
	a.mu.Lock()
	draft := a.settingsDraft
	settings := cloneSettings(a.data.Settings)
	a.settingsSaveVersion++
	saveVersion := a.settingsSaveVersion
	service := a.service
	a.mu.Unlock()
	settings.Theme = draft.Theme
	settings.DefaultSendMode = draft.SendMode
	if draft.ProviderID != "" {
		settings.DefaultProviderID = draft.ProviderID
	}
	if service == nil {
		a.setError("服务尚未连接，无法保存设置")
		return
	}
	a.invokeAsync("保存设置", func(ctx context.Context) error { return service.SaveSettings(ctx, settings) }, func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.settingsSaveVersion != saveVersion || a.settingsDraft != draft {
			return
		}
		a.settingsDirty = false
		a.data.Settings = cloneSettings(settings)
		if !a.sendModeDirty || a.sendMode == settings.DefaultSendMode {
			a.sendMode = settings.DefaultSendMode
			a.sendModeDirty = false
		}
	})
}

func (a *App) saveProvider() {
	a.mu.RLock()
	draft := a.settingsDraft
	a.mu.RUnlock()
	if draft.Provider.ID == "" {
		draft.Provider.ID = draft.ProviderID
	}
	a.invoke("保存模型服务", func(ctx context.Context) error {
		return a.service.SaveProvider(ctx, draft.Provider, draft.APIKey)
	})
}

func (a *App) removeProvider(id string) {
	a.invoke("删除模型服务", func(ctx context.Context) error { return a.service.RemoveProvider(ctx, id) })
}

func (a *App) updateSettingsDraft(fn func(*SettingsDraft)) {
	a.mu.Lock()
	fn(&a.settingsDraft)
	a.data.Settings.Theme = a.settingsDraft.Theme
	a.data.Settings.DefaultSendMode = a.settingsDraft.SendMode
	a.settingsDirty = true
	a.mu.Unlock()
}

func (a *App) openSettings() {
	a.mu.Lock()
	a.page = screenSettings
	a.mu.Unlock()
}

func (a *App) closeSettings() {
	a.mu.Lock()
	a.page = screenConversation
	a.mu.Unlock()
}

func (a *App) setRightTab(tab rightTab) {
	a.mu.Lock()
	a.right = tab
	a.rightOpen = true
	a.mu.Unlock()
}

func (a *App) ensureTerminal(cwd string) {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	if a.terminalCWD == cwd && (a.terminal != nil || a.terminalLoading) {
		a.mu.Unlock()
		return
	}
	factory := a.terminalFactory
	oldLoad := a.terminalLoad
	a.terminalLoad = nil
	a.terminalLoading = true
	a.terminalCWD = cwd
	old := a.terminal
	a.terminal = nil
	var load *terminalLoad
	if factory != nil && cwd != "" {
		ctx, cancel := context.WithCancel(context.Background())
		load = &terminalLoad{ctx: ctx, cancel: cancel}
		a.terminalLoad = load
	} else {
		a.terminalLoading = false
	}
	a.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	if oldLoad != nil {
		oldLoad.stop()
	}
	if load == nil {
		return
	}
	go func() {
		term, err := factory.Open(load.ctx, cwd)
		if !load.finish(term) {
			return
		}
		a.post(func() {
			a.mu.Lock()
			if a.closed || a.terminalLoad != load || a.terminalCWD != cwd {
				a.mu.Unlock()
				load.stop()
				return
			}
			term, ok := load.take()
			a.terminalLoad = nil
			a.terminalLoading = false
			if !ok {
				a.mu.Unlock()
				return
			}
			if err != nil {
				a.errorText = "打开终端失败：" + err.Error()
				if term != nil {
					_ = term.Close()
				}
				a.mu.Unlock()
				return
			}
			a.terminal = term
			a.mu.Unlock()
		})
	}()
}

func settingsDraft(settings appmodel.Settings) SettingsDraft {
	draft := SettingsDraft{ProviderID: settings.DefaultProviderID, Theme: settings.Theme, SendMode: settings.DefaultSendMode}
	if draft.Theme == "" {
		draft.Theme = "system"
	}
	if draft.SendMode == "" {
		draft.SendMode = appmodel.Correction
	}
	for _, provider := range settings.Providers {
		if provider.ID == draft.ProviderID {
			draft.Provider = provider
			break
		}
	}
	if draft.Provider.ID == "" && len(settings.Providers) > 0 {
		draft.Provider = settings.Providers[0]
		draft.ProviderID = draft.Provider.ID
	}
	return draft
}

func refreshDraftProviderHasAPIKey(draft *SettingsDraft, providers []appmodel.Provider) {
	if draft == nil {
		return
	}
	providerID := draft.ProviderID
	if providerID == "" {
		providerID = draft.Provider.ID
	}
	provider, ok := providerByID(providers, providerID)
	if ok {
		draft.Provider.HasAPIKey = provider.HasAPIKey
	}
}

func providerByID(providers []appmodel.Provider, id string) (appmodel.Provider, bool) {
	if id == "" {
		return appmodel.Provider{}, false
	}
	for _, provider := range providers {
		if provider.ID == id {
			return provider, true
		}
	}
	return appmodel.Provider{}, false
}

func cloneSnapshot(snapshot appmodel.Snapshot) appmodel.Snapshot {
	copyValue := snapshot
	copyValue.Projects = append([]appmodel.Project(nil), snapshot.Projects...)
	copyValue.Settings = cloneSettings(snapshot.Settings)
	copyValue.Sessions = make([]appmodel.Session, len(snapshot.Sessions))
	for i, session := range snapshot.Sessions {
		copySession := session
		if session.ActiveRun != nil {
			run := *session.ActiveRun
			copySession.ActiveRun = &run
		}
		copySession.Messages = append([]appmodel.Message(nil), session.Messages...)
		for j := range copySession.Messages {
			copySession.Messages[j].Tools = append([]appmodel.ToolCall(nil), session.Messages[j].Tools...)
		}
		copySession.Queue = append([]appmodel.QueuedMessage(nil), session.Queue...)
		copySession.Permissions = append([]appmodel.Permission(nil), session.Permissions...)
		for j := range copySession.Permissions {
			copySession.Permissions[j].Options = append([]appmodel.PermissionOption(nil), session.Permissions[j].Options...)
		}
		copySession.Questions = append([]appmodel.AskRequest(nil), session.Questions...)
		for j := range copySession.Questions {
			copySession.Questions[j].Questions = append([]appmodel.Question(nil), session.Questions[j].Questions...)
			for k := range copySession.Questions[j].Questions {
				copySession.Questions[j].Questions[k].Options = append([]appmodel.QuestionOption(nil), session.Questions[j].Questions[k].Options...)
			}
		}
		copyValue.Sessions[i] = copySession
	}
	return copyValue
}

// snapshotForView copies the fields that local UI actions can mutate while
// retaining the history slices owned by App. Open and Update deep-copy caller
// snapshots before they become App-owned data; rendering only reads Messages,
// Tools, Queue, Permissions, Questions and Projects.
func snapshotForView(snapshot appmodel.Snapshot) appmodel.Snapshot {
	copyValue := snapshot
	copyValue.Settings = cloneSettings(snapshot.Settings)
	copyValue.Sessions = append([]appmodel.Session(nil), snapshot.Sessions...)
	return copyValue
}

func cloneSettings(settings appmodel.Settings) appmodel.Settings {
	settings.Providers = append([]appmodel.Provider(nil), settings.Providers...)
	return settings
}

func cloneWorkspaceSnapshot(snapshot WorkspaceSnapshot) WorkspaceSnapshot {
	copyValue := snapshot
	copyValue.Files = append([]appmodel.FileEntry(nil), snapshot.Files...)
	copyValue.Git.Changes = append([]appmodel.Change(nil), snapshot.Git.Changes...)
	if snapshot.Preview != nil {
		preview := *snapshot.Preview
		copyValue.Preview = &preview
	}
	return copyValue
}

func activeSession(sessions []appmodel.Session, id string) (appmodel.Session, bool) {
	for _, session := range sessions {
		if session.ID == id {
			return session, true
		}
	}
	return appmodel.Session{}, false
}

func hasSession(sessions []appmodel.Session, id string) bool {
	_, ok := activeSession(sessions, id)
	return ok
}

func themeIsDark(systemDark bool, mode string) bool {
	switch strings.ToLower(mode) {
	case "dark":
		return true
	case "light":
		return false
	default:
		return systemDark
	}
}
