package controller

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gcode/internal/appmodel"
	"gcode/internal/redaction"
	"gcode/internal/runtime"
	"gcode/internal/sessions"
	"gcode/internal/worktrees"
)

type WorkspaceSession struct {
	Cwd      string
	Branch   string
	Worktree bool
}

type Workspace interface {
	Prepare(appmodel.Project, appmodel.NewSessionOptions) (WorkspaceSession, error)
}

type Runner interface {
	Run(context.Context, runtime.Input, runtime.Callbacks) (runtime.Result, error)
}

type Credentials interface {
	Set(string, string) error
	Delete(string) (bool, error)
	Has(string) bool
}

type Options struct {
	DataDir     string
	Runtime     *runtime.Engine
	Runner      Runner
	Credentials Credentials
	Workspace   Workspace
	// WorktreeManager is used when no custom Workspace is supplied and a new
	// session requests an isolated worktree. It also rebinds persisted
	// worktree sessions during controller startup so deletion remains guarded
	// after a restart.
	WorktreeManager *worktrees.Manager
	Settings        appmodel.Settings
	Projects        []appmodel.Project
}

type Controller struct {
	mu          sync.Mutex
	dataDir     string
	statePath   string
	runner      Runner
	engine      *runtime.Engine
	credentials Credentials
	workspace   Workspace
	worktrees   *worktrees.Manager
	settings    appmodel.Settings
	projects    []appmodel.Project
	sessions    map[string]*sessionState
	order       []string
	subscribers map[uint64]func(appmodel.Snapshot)
	nextSub     uint64
	closed      bool
	wg          sync.WaitGroup
}

type sessionState struct {
	model      appmodel.Session
	active     *activeRun
	worktreeOp bool
	nextRun    uint64
	permission map[string]*pendingPermission
	ask        map[string]*pendingAsk
}

type sessionStateSnapshot struct {
	model        appmodel.Session
	active       *activeRun
	activeCancel cancelReason
	worktreeOp   bool
	nextRun      uint64
	permission   map[string]*pendingPermission
	ask          map[string]*pendingAsk
}

type activeRun struct {
	runID        string
	generation   uint64
	ctx          context.Context
	cancel       context.CancelFunc
	cancelReason cancelReason
	prompt       string
	config       appmodel.RunConfig
}

type cancelReason uint8

const (
	cancelNone cancelReason = iota
	cancelReplace
	cancelStop
)

const (
	messageStateSubmitted     = "submitted"
	messageStateCancelPending = "cancel_pending"
	messageStateQueued        = "queued"
	messageStateRunning       = "running"
	messageStateCompleted     = "completed"
	messageStateInterrupted   = "interrupted"
	messageStateFailed        = "failed"
	messageStateWithdrawn     = "withdrawn"
	messageStateStreaming     = "streaming"
)

const (
	pauseReasonStop          = "任务已停止，队列已暂停"
	pauseReasonFailure       = "任务失败，队列已暂停"
	pauseReasonRestart       = "应用重启后任务已暂停"
	pauseReasonCancelFailure = "取消未完成，队列已暂停"
	pauseReasonPersistence   = "状态保存失败，队列已暂停"
)

type pendingPermission struct {
	generation uint64
	request    runtime.PermissionRequest
	response   chan string
}

type pendingAsk struct {
	generation uint64
	request    runtime.AskRequest
	response   chan []string
}

type storedState struct {
	Version  int               `json:"version"`
	Snapshot appmodel.Snapshot `json:"snapshot"`
}

func New(options Options) (*Controller, error) {
	engine := options.Runtime
	if engine == nil && options.Runner == nil {
		created, err := runtime.New(runtime.Options{DataDir: options.DataDir})
		if err != nil {
			return nil, err
		}
		engine = created
	}
	dataDir := strings.TrimSpace(options.DataDir)
	if dataDir == "" && engine != nil {
		dataDir = engine.DataDir()
	}
	if dataDir == "" {
		dataDir, _ = runtime.DefaultDataDir()
	}
	if dataDir == "" {
		return nil, errors.New("GCode data directory is required")
	}
	absolute, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, err
	}
	c := &Controller{
		dataDir:     filepath.Clean(absolute),
		statePath:   filepath.Join(filepath.Clean(absolute), "controller.json"),
		engine:      engine,
		workspace:   options.Workspace,
		worktrees:   options.WorktreeManager,
		settings:    normalizeSettings(options.Settings),
		sessions:    map[string]*sessionState{},
		subscribers: map[uint64]func(appmodel.Snapshot){},
	}
	if options.Runner != nil {
		c.runner = options.Runner
	} else {
		c.runner = engine
	}
	if options.Credentials != nil {
		c.credentials = options.Credentials
	} else if engine != nil {
		c.credentials = engineCredentials{engine: engine}
	}
	if err := c.load(); err != nil {
		return nil, err
	}
	if len(options.Projects) > 0 && len(c.projects) == 0 {
		for _, project := range options.Projects {
			if strings.TrimSpace(project.Path) == "" {
				continue
			}
			c.projects = append(c.projects, project)
		}
		if err := c.persistLocked(); err != nil {
			return nil, fmt.Errorf("persist initial controller state: %w", err)
		}
	}
	return c, nil
}

type engineCredentials struct {
	engine *runtime.Engine
}

func (credentials engineCredentials) Set(provider, key string) error {
	return credentials.engine.SetCredential(provider, key)
}

func (credentials engineCredentials) Delete(provider string) (bool, error) {
	had := credentials.engine.HasCredential(provider)
	err := credentials.engine.RemoveCredential(provider)
	return had, err
}

func (credentials engineCredentials) Has(provider string) bool {
	return credentials.engine.HasCredential(provider)
}

func (c *Controller) Snapshot() appmodel.Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshotLocked()
}

func (c *Controller) Subscribe(callback func(appmodel.Snapshot)) func() {
	if callback == nil {
		return func() {}
	}
	c.mu.Lock()
	c.nextSub++
	id := c.nextSub
	c.subscribers[id] = callback
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	callback(snapshot)
	return func() {
		c.mu.Lock()
		delete(c.subscribers, id)
		c.mu.Unlock()
	}
}

func (c *Controller) AddProject(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("project path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("project path is not a directory: %s", abs)
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("controller is closed")
	}
	for _, project := range c.projects {
		if project.Path == abs {
			c.mu.Unlock()
			return nil
		}
	}
	project := appmodel.Project{ID: newID("project"), Name: filepath.Base(abs), Path: abs}
	previousProjects := append([]appmodel.Project(nil), c.projects...)
	c.projects = append(c.projects, project)
	persistErr := c.persistLocked()
	if persistErr != nil {
		c.projects = previousProjects
	}
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	if persistErr != nil {
		return fmt.Errorf("persist project: %w", persistErr)
	}
	c.emit(snapshot)
	return nil
}

// ListBranches returns the local branches for a registered project's
// repository. Repository inspection stays in worktrees.Manager; the
// controller only resolves the project identity and owns the UI-facing API.
func (c *Controller) ListBranches(projectID string) ([]string, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("controller is closed")
	}
	project, ok := c.projectLocked(projectID)
	c.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}
	if c.worktrees == nil {
		return nil, errors.New("worktree manager is unavailable")
	}
	return c.worktrees.ListBranches(context.Background(), project.Path)
}

// MergeWorktree merges a session's branch into a local target branch. The
// bound session remains the owner for the duration of the manager operation;
// other sessions using the same checkout still block it.
func (c *Controller) MergeWorktree(sessionID, target string) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("controller is closed")
	}
	session, ok := c.sessions[sessionID]
	if !ok {
		c.mu.Unlock()
		return fmt.Errorf("session not found: %s", sessionID)
	}
	if !session.model.Worktree {
		c.mu.Unlock()
		return errors.New("session is not a worktree session")
	}
	if session.active != nil {
		c.mu.Unlock()
		return errors.New("cannot merge a worktree while its session is running")
	}
	if session.worktreeOp {
		c.mu.Unlock()
		return errors.New("worktree operation is already in progress")
	}
	path := session.model.Cwd
	session.worktreeOp = true
	c.mu.Unlock()
	if c.worktrees == nil {
		c.mu.Lock()
		session.worktreeOp = false
		c.mu.Unlock()
		return errors.New("worktree manager is unavailable")
	}
	_, err := c.worktrees.MergeForSession(context.Background(), path, target, sessionID)
	c.mu.Lock()
	session.worktreeOp = false
	c.mu.Unlock()
	return err
}

// RemoveWorktree removes a session's manager-owned worktree after checking
// that no task is active. The session remains in history with its original
// worktree path so the removal is explicit and inspectable; a later send fails
// until the caller creates a new session/workspace.
func (c *Controller) RemoveWorktree(sessionID string) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("controller is closed")
	}
	session, ok := c.sessions[sessionID]
	if !ok {
		c.mu.Unlock()
		return fmt.Errorf("session not found: %s", sessionID)
	}
	if !session.model.Worktree {
		c.mu.Unlock()
		return errors.New("session is not a worktree session")
	}
	if session.active != nil {
		c.mu.Unlock()
		return errors.New("cannot remove a worktree while its session is running")
	}
	if session.worktreeOp {
		c.mu.Unlock()
		return errors.New("worktree operation is already in progress")
	}
	path := session.model.Cwd
	session.worktreeOp = true
	c.mu.Unlock()
	if c.worktrees == nil {
		c.mu.Lock()
		session.worktreeOp = false
		c.mu.Unlock()
		return errors.New("worktree manager is unavailable")
	}
	if err := c.worktrees.RemoveForSession(context.Background(), path, sessionID); err != nil {
		c.mu.Lock()
		session.worktreeOp = false
		c.mu.Unlock()
		return err
	}
	c.mu.Lock()
	session.worktreeOp = false
	if current, ok := c.sessions[sessionID]; ok {
		current.model.Error = "worktree 已删除，无法继续运行此会话"
		current.model.UpdatedAt = time.Now()
		snapshot := c.snapshotLocked()
		persistErr := c.persistLocked()
		c.mu.Unlock()
		c.emit(snapshot)
		return persistErr
	}
	c.mu.Unlock()
	return nil
}

func (c *Controller) NewSession(options appmodel.NewSessionOptions) (string, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return "", errors.New("controller is closed")
	}
	project, ok := c.projectLocked(options.ProjectID)
	c.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("project not found: %s", options.ProjectID)
	}
	id := newID("session")
	options.SessionID = id
	workspace := WorkspaceSession{Cwd: project.Path}
	if c.workspace != nil {
		prepared, err := c.workspace.Prepare(project, options)
		if err != nil {
			return "", err
		}
		workspace = prepared
	} else if options.Worktree {
		if c.worktrees == nil {
			return "", errors.New("worktree manager is required when worktree mode is enabled")
		}
		created, err := c.worktrees.Create(context.Background(), worktrees.CreateOptions{
			SourceDir:  project.Path,
			Name:       id,
			BaseBranch: options.BaseBranch,
		})
		if err != nil {
			return "", err
		}
		workspace = WorkspaceSession{Cwd: created.Path, Branch: created.Branch, Worktree: true}
	}
	if workspace.Cwd == "" {
		return "", errors.New("workspace returned empty working directory")
	}
	providerID := strings.TrimSpace(options.ProviderID)
	c.mu.Lock()
	if providerID == "" {
		providerID = c.settings.DefaultProviderID
	}
	title := strings.TrimSpace(options.Title)
	if title == "" {
		title = "新会话"
	}
	session := &sessionState{model: appmodel.Session{
		ID:         id,
		ProjectID:  project.ID,
		Title:      title,
		Cwd:        workspace.Cwd,
		Worktree:   options.Worktree || workspace.Worktree,
		Branch:     workspace.Branch,
		ProviderID: providerID,
		FullAccess: options.FullAccess,
		State:      appmodel.Idle,
		Messages:   []appmodel.Message{},
		Queue:      []appmodel.QueuedMessage{},
		UpdatedAt:  time.Now(),
	}, permission: map[string]*pendingPermission{}, ask: map[string]*pendingAsk{}}
	if c.worktrees != nil && session.model.Worktree {
		if err := c.worktrees.Bind(session.model.Cwd, id); err != nil {
			return "", err
		}
	}
	c.sessions[id] = session
	c.order = append(c.order, id)
	if c.engine != nil && c.engine.SessionStore() != nil {
		_, err := c.engine.SessionStore().Create(sessionCreateInput(session.model))
		if err != nil {
			if c.worktrees != nil && session.model.Worktree {
				c.worktrees.Unbind(session.model.Cwd, id)
			}
			delete(c.sessions, id)
			c.order = c.order[:len(c.order)-1]
			c.mu.Unlock()
			return "", err
		}
	}
	persistErr := c.persistLocked()
	if persistErr != nil {
		delete(c.sessions, id)
		c.order = c.order[:len(c.order)-1]
		if c.worktrees != nil && session.model.Worktree {
			c.worktrees.Unbind(session.model.Cwd, id)
		}
	}
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	if persistErr != nil {
		return "", fmt.Errorf("persist session: %w", persistErr)
	}
	c.emit(snapshot)
	return id, nil
}

func (c *Controller) Send(sessionID, text string, mode appmodel.SendMode) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("message is empty")
	}
	if mode != appmodel.Queue && mode != appmodel.Correction {
		mode = appmodel.Correction
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("controller is closed")
	}
	session, ok := c.sessions[sessionID]
	if !ok {
		c.mu.Unlock()
		return fmt.Errorf("session not found: %s", sessionID)
	}
	if session.worktreeOp {
		c.mu.Unlock()
		return errors.New("worktree operation is already in progress")
	}
	previous := snapshotSessionState(session)
	message := appmodel.QueuedMessage{ID: newID("message"), Text: text, Mode: mode, State: messageStateSubmitted, CreatedAt: time.Now()}
	var start *activeRun
	if session.active == nil {
		start = c.startRunLocked(session, message)
	} else {
		message.State = messageStateQueued
		if mode == appmodel.Correction {
			session.model.Queue = insertCorrection(session.model.Queue, message)
			session.active.cancelReason = cancelReplace
			setRunMessageState(&session.model, session.active.runID, messageStateCancelPending, false)
			session.model.QueuePaused = false
			session.model.PauseReason = ""
			session.model.State = appmodel.Cancelling
		} else {
			session.model.Queue = append(session.model.Queue, message)
		}
		session.model.UpdatedAt = time.Now()
	}
	persistErr := c.persistLocked()
	if persistErr != nil {
		if start != nil {
			start.cancel()
		}
		restoreSessionState(session, previous)
	}
	// Publish the cancellation before releasing c.mu. Otherwise the active run
	// can finish between this state update and cancel(), and be misclassified as
	// a cancellation failure.
	if persistErr == nil && mode == appmodel.Correction && previous.active != nil {
		previous.active.cancel()
	}
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	if persistErr != nil {
		return fmt.Errorf("persist message: %w", persistErr)
	}
	c.emit(snapshot)
	if start != nil {
		c.wg.Add(1)
		go c.execute(sessionID, start)
	}
	return nil
}

func (c *Controller) Stop(sessionID string) error {
	c.mu.Lock()
	session, ok := c.sessions[sessionID]
	if !ok {
		c.mu.Unlock()
		return fmt.Errorf("session not found: %s", sessionID)
	}
	previous := snapshotSessionState(session)
	if session.active != nil {
		session.active.cancelReason = cancelStop
		setRunMessageState(&session.model, session.active.runID, messageStateCancelPending, false)
		session.model.QueuePaused = true
		session.model.PauseReason = pauseReasonStop
		session.model.State = appmodel.Cancelling
		session.model.UpdatedAt = time.Now()
	}
	persistErr := c.persistLocked()
	if persistErr != nil {
		restoreSessionState(session, previous)
	}
	// Publish the cancellation before releasing c.mu. Otherwise the active run
	// can finish while Stop is emitting its pending state and observe no cancel.
	if persistErr == nil && previous.active != nil {
		previous.active.cancel()
	}
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	if persistErr != nil {
		return fmt.Errorf("persist stop state: %w", persistErr)
	}
	c.emit(snapshot)
	return nil
}

func (c *Controller) Withdraw(sessionID, messageID string) error {
	c.mu.Lock()
	session, ok := c.sessions[sessionID]
	if !ok {
		c.mu.Unlock()
		return fmt.Errorf("session not found: %s", sessionID)
	}
	index := -1
	for i, message := range session.model.Queue {
		if message.ID == messageID {
			index = i
			break
		}
	}
	if index < 0 {
		c.mu.Unlock()
		return fmt.Errorf("queued message not found: %s", messageID)
	}
	previous := snapshotSessionState(session)
	message := session.model.Queue[index]
	message.State = messageStateWithdrawn
	session.model.Queue = append(session.model.Queue[:index], session.model.Queue[index+1:]...)
	session.model.Messages = append(session.model.Messages, appmodel.Message{
		ID:        message.ID,
		State:     messageStateWithdrawn,
		Role:      "user",
		Text:      message.Text,
		CreatedAt: message.CreatedAt,
	})
	session.model.UpdatedAt = time.Now()
	persistErr := c.persistLocked()
	if persistErr != nil {
		restoreSessionState(session, previous)
	}
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	if persistErr != nil {
		return fmt.Errorf("persist withdrawn message: %w", persistErr)
	}
	c.emit(snapshot)
	return nil
}

func (c *Controller) ResumeQueue(sessionID string) error {
	c.mu.Lock()
	session, ok := c.sessions[sessionID]
	if !ok {
		c.mu.Unlock()
		return fmt.Errorf("session not found: %s", sessionID)
	}
	previous := snapshotSessionState(session)
	session.model.QueuePaused = false
	session.model.PauseReason = ""
	var start *activeRun
	if session.active == nil && len(session.model.Queue) > 0 {
		message := session.model.Queue[0]
		session.model.Queue = session.model.Queue[1:]
		start = c.startRunLocked(session, message)
	}
	session.model.UpdatedAt = time.Now()
	persistErr := c.persistLocked()
	if persistErr != nil {
		if start != nil {
			start.cancel()
		}
		restoreSessionState(session, previous)
	}
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	if persistErr != nil {
		return fmt.Errorf("persist queue state: %w", persistErr)
	}
	c.emit(snapshot)
	if start != nil {
		c.wg.Add(1)
		go c.execute(sessionID, start)
	}
	return nil
}

func (c *Controller) SetProvider(sessionID, providerID string) error {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return errors.New("provider id is required")
	}
	c.mu.Lock()
	session, ok := c.sessions[sessionID]
	if !ok {
		c.mu.Unlock()
		return fmt.Errorf("session not found: %s", sessionID)
	}
	previous := snapshotSessionState(session)
	if !c.providerLocked(providerID) {
		c.mu.Unlock()
		return fmt.Errorf("provider not found: %s", providerID)
	}
	session.model.ProviderID = providerID
	session.model.UpdatedAt = time.Now()
	persistErr := c.persistLocked()
	if persistErr != nil {
		restoreSessionState(session, previous)
	}
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	if persistErr != nil {
		return fmt.Errorf("persist provider selection: %w", persistErr)
	}
	c.emit(snapshot)
	return nil
}

func (c *Controller) SetFullAccess(sessionID string, fullAccess bool) error {
	c.mu.Lock()
	session, ok := c.sessions[sessionID]
	if !ok {
		c.mu.Unlock()
		return fmt.Errorf("session not found: %s", sessionID)
	}
	previous := snapshotSessionState(session)
	session.model.FullAccess = fullAccess
	session.model.UpdatedAt = time.Now()
	persistErr := c.persistLocked()
	if persistErr != nil {
		restoreSessionState(session, previous)
	}
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	if persistErr != nil {
		return fmt.Errorf("persist access mode: %w", persistErr)
	}
	c.emit(snapshot)
	return nil
}

func (c *Controller) ReplyPermission(sessionID, permissionID, optionID string) error {
	c.mu.Lock()
	session, ok := c.sessions[sessionID]
	if !ok {
		c.mu.Unlock()
		return fmt.Errorf("session not found: %s", sessionID)
	}
	pending, ok := session.permission[permissionID]
	if !ok || session.active == nil || pending.generation != session.active.generation {
		c.mu.Unlock()
		return errors.New("permission request is no longer active")
	}
	valid := false
	for _, option := range pending.request.Options {
		if option.ID == optionID {
			valid = true
			break
		}
	}
	if !valid {
		c.mu.Unlock()
		return fmt.Errorf("unknown permission option: %s", optionID)
	}
	previous := snapshotSessionState(session)
	delete(session.permission, permissionID)
	removePermission(&session.model, permissionID)
	if session.model.State == appmodel.WaitingPermission {
		session.model.State = appmodel.Running
	}
	session.model.UpdatedAt = time.Now()
	persistErr := c.persistLocked()
	if persistErr != nil {
		restoreSessionState(session, previous)
	}
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	if persistErr != nil {
		return fmt.Errorf("persist permission response: %w", persistErr)
	}
	c.emit(snapshot)
	pending.response <- optionID
	return nil
}

func (c *Controller) ReplyAsk(sessionID, askID string, answers []string) error {
	c.mu.Lock()
	session, ok := c.sessions[sessionID]
	if !ok {
		c.mu.Unlock()
		return fmt.Errorf("session not found: %s", sessionID)
	}
	pending, ok := session.ask[askID]
	if !ok || session.active == nil || pending.generation != session.active.generation {
		c.mu.Unlock()
		return errors.New("question is no longer active")
	}
	previous := snapshotSessionState(session)
	delete(session.ask, askID)
	removeAsk(&session.model, askID)
	if session.model.State == appmodel.WaitingQuestion {
		session.model.State = appmodel.Running
	}
	session.model.UpdatedAt = time.Now()
	persistErr := c.persistLocked()
	if persistErr != nil {
		restoreSessionState(session, previous)
	}
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	if persistErr != nil {
		return fmt.Errorf("persist question response: %w", persistErr)
	}
	c.emit(snapshot)
	pending.response <- append([]string(nil), answers...)
	return nil
}

func (c *Controller) SaveSettings(settings appmodel.Settings) error {
	c.mu.Lock()
	previous := cloneSettings(c.settings)
	settings = normalizeSettings(settings)
	for index := range settings.Providers {
		settings.Providers[index].HasAPIKey = c.hasCredential(settings.Providers[index].ID)
	}
	c.settings = settings
	persistErr := c.persistLocked()
	if persistErr != nil {
		c.settings = previous
	}
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	if persistErr != nil {
		return fmt.Errorf("persist settings: %w", persistErr)
	}
	c.emit(snapshot)
	return nil
}

func (c *Controller) SaveProvider(provider appmodel.Provider, key string) error {
	provider.ID = strings.TrimSpace(provider.ID)
	if provider.ID == "" {
		return errors.New("provider id is required")
	}
	if strings.TrimSpace(key) != "" {
		if c.credentials == nil {
			return errors.New("credential store is unavailable")
		}
		if err := c.credentials.Set(provider.ID, key); err != nil {
			return err
		}
	}
	c.mu.Lock()
	previous := cloneSettings(c.settings)
	provider.HasAPIKey = c.hasCredential(provider.ID)
	found := false
	for index := range c.settings.Providers {
		if c.settings.Providers[index].ID == provider.ID {
			c.settings.Providers[index] = provider
			found = true
			break
		}
	}
	if !found {
		c.settings.Providers = append(c.settings.Providers, provider)
	}
	if c.settings.DefaultProviderID == "" {
		c.settings.DefaultProviderID = provider.ID
	}
	persistErr := c.persistLocked()
	if persistErr != nil {
		c.settings = previous
	}
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	if persistErr != nil {
		return fmt.Errorf("persist provider: %w", persistErr)
	}
	c.emit(snapshot)
	return nil
}

func (c *Controller) RemoveProvider(providerID string) error {
	providerID = strings.TrimSpace(providerID)
	c.mu.Lock()
	index := -1
	for i, provider := range c.settings.Providers {
		if provider.ID == providerID {
			index = i
			break
		}
	}
	if index < 0 {
		c.mu.Unlock()
		return fmt.Errorf("provider not found: %s", providerID)
	}
	previous := cloneSettings(c.settings)
	c.settings.Providers = append(c.settings.Providers[:index], c.settings.Providers[index+1:]...)
	if c.settings.DefaultProviderID == providerID {
		c.settings.DefaultProviderID = ""
		if len(c.settings.Providers) > 0 {
			c.settings.DefaultProviderID = c.settings.Providers[0].ID
		}
	}
	if c.credentials != nil {
		if _, err := c.credentials.Delete(providerID); err != nil {
			c.settings = previous
			c.mu.Unlock()
			return err
		}
	}
	persistErr := c.persistLocked()
	if persistErr != nil {
		c.settings = previous
	}
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	if persistErr != nil {
		return fmt.Errorf("persist provider removal: %w", persistErr)
	}
	c.emit(snapshot)
	return nil
}

func (c *Controller) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	for _, session := range c.sessions {
		if session.active != nil {
			session.active.cancelReason = cancelStop
			setRunMessageState(&session.model, session.active.runID, messageStateCancelPending, false)
			session.model.QueuePaused = true
			session.model.PauseReason = pauseReasonStop
			session.model.State = appmodel.Cancelling
		}
	}
	persistErr := c.persistLocked()
	// Close is irreversible, so active runs must still be cancelled after the
	// persistence attempt even when it fails. Keep the cancellation under c.mu
	// so finish cannot race the close transition.
	for _, session := range c.sessions {
		if session.active != nil {
			session.active.cancel()
		}
	}
	c.mu.Unlock()
	c.wg.Wait()
	if c.engine != nil {
		return errors.Join(persistErr, c.engine.Close())
	}
	return persistErr
}

func (c *Controller) startRunLocked(session *sessionState, message appmodel.QueuedMessage) *activeRun {
	session.nextRun++
	ctx, cancel := context.WithCancel(context.Background())
	run := &activeRun{
		runID:      newID("run"),
		generation: session.nextRun,
		ctx:        ctx,
		cancel:     cancel,
		prompt:     message.Text,
		config:     runConfigFor(session.model, c.providerLockedValue(session.model.ProviderID)),
	}
	session.active = run
	session.model.RunID = run.runID
	session.model.ActiveRun = cloneRunConfig(&run.config)
	session.model.State = appmodel.Running
	session.model.Error = ""
	session.model.QueuePaused = false
	session.model.PauseReason = ""
	message.State = messageStateRunning
	session.model.Messages = append(session.model.Messages, appmodel.Message{ID: message.ID, RunID: run.runID, State: messageStateRunning, Role: "user", Text: message.Text, CreatedAt: message.CreatedAt})
	session.model.UpdatedAt = time.Now()
	return run
}

func (c *Controller) execute(sessionID string, run *activeRun) {
	defer c.wg.Done()
	c.mu.Lock()
	session, ok := c.sessions[sessionID]
	if !ok || session.active != run {
		c.mu.Unlock()
		return
	}
	input := runtime.Input{
		RunID:     run.runID,
		SessionID: sessionID,
		Title:     session.model.Title,
		Prompt:    run.prompt,
		Cwd:       run.config.Cwd,
		Provider: appmodel.Provider{
			ID:      run.config.ProviderID,
			Name:    run.config.ProviderName,
			Format:  run.config.Format,
			BaseURL: run.config.BaseURL,
			Model:   run.config.Model,
		},
		FullAccess: run.config.FullAccess,
	}
	ctx := run.ctx
	c.mu.Unlock()
	result, err := c.runner.Run(ctx, input, runtime.Callbacks{
		OnText:       func(text string) { c.onText(sessionID, run, text) },
		OnReasoning:  func(text string) { c.onReasoning(sessionID, run, text) },
		OnToolCall:   func(call runtime.ToolCall) { c.onToolCall(sessionID, run, call) },
		OnToolResult: func(tool runtime.ToolResult) { c.onToolResult(sessionID, run, tool) },
		OnPermissionRequest: func(waitContext context.Context, request runtime.PermissionRequest) (string, error) {
			return c.waitPermission(sessionID, run, waitContext, request)
		},
		OnAskUser: func(waitContext context.Context, request runtime.AskRequest) ([]string, error) {
			return c.waitAsk(sessionID, run, waitContext, request)
		},
		OnError:   func(runtimeErr error) { c.onRuntimeWarning(sessionID, run, runtimeErr) },
		OnWarning: func(runtimeErr error) { c.onRuntimeWarning(sessionID, run, runtimeErr) },
	})
	c.finish(sessionID, run, result, err)
}

func (c *Controller) onRuntimeWarning(sessionID string, run *activeRun, runtimeErr error) {
	if runtimeErr == nil {
		return
	}
	c.mu.Lock()
	if !c.acceptLocked(sessionID, run) {
		c.mu.Unlock()
		return
	}
	if expectedCancellationWarning(run, runtimeErr) {
		c.mu.Unlock()
		return
	}
	session := c.sessions[sessionID]
	message := redaction.RedactString(runtimeErr.Error(), redaction.Options{})
	session.model.Error = message
	session.model.Messages = append(session.model.Messages, appmodel.Message{
		ID:        newID("warning"),
		RunID:     run.runID,
		State:     messageStateCompleted,
		Role:      "warning",
		Text:      message,
		CreatedAt: time.Now(),
	})
	session.model.UpdatedAt = time.Now()
	if err := c.persistLocked(); err != nil {
		c.markPersistenceFailureLocked(session, err)
	}
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	c.emit(snapshot)
}

func expectedCancellationWarning(run *activeRun, runtimeErr error) bool {
	if run == nil || !errors.Is(runtimeErr, context.Canceled) || run.ctx == nil || !errors.Is(run.ctx.Err(), context.Canceled) {
		return false
	}
	return run.cancelReason == cancelReplace || run.cancelReason == cancelStop
}

func (c *Controller) waitPermission(sessionID string, run *activeRun, waitContext context.Context, request runtime.PermissionRequest) (string, error) {
	c.mu.Lock()
	session, ok := c.sessions[sessionID]
	if !ok || session.active != run || run.generation != session.nextRun {
		c.mu.Unlock()
		return "", context.Canceled
	}
	response := make(chan string, 1)
	session.permission[request.ID] = &pendingPermission{generation: run.generation, request: request, response: response}
	session.model.Permissions = append(session.model.Permissions, appmodel.Permission{ID: request.ID, RunID: run.runID, Tool: request.Tool, Arguments: request.Arguments, Options: toAppPermissionOptions(request.Options)})
	session.model.State = appmodel.WaitingPermission
	session.model.UpdatedAt = time.Now()
	persistErr := c.persistLocked()
	snapshot := c.snapshotLocked()
	if persistErr != nil {
		delete(session.permission, request.ID)
		removePermission(&session.model, request.ID)
		c.markPersistenceFailureLocked(session, persistErr)
		snapshot = c.snapshotLocked()
	}
	c.mu.Unlock()
	c.emit(snapshot)
	if persistErr != nil {
		return "", fmt.Errorf("persist permission request: %w", persistErr)
	}
	select {
	case action := <-response:
		return action, nil
	case <-waitContext.Done():
		c.clearPermission(sessionID, run, request.ID)
		return "", waitContext.Err()
	}
}

func (c *Controller) waitAsk(sessionID string, run *activeRun, waitContext context.Context, request runtime.AskRequest) ([]string, error) {
	c.mu.Lock()
	session, ok := c.sessions[sessionID]
	if !ok || session.active != run || run.generation != session.nextRun {
		c.mu.Unlock()
		return nil, context.Canceled
	}
	response := make(chan []string, 1)
	session.ask[request.ID] = &pendingAsk{generation: run.generation, request: request, response: response}
	questions := make([]appmodel.Question, 0, len(request.Questions))
	for _, question := range request.Questions {
		id := newID("question")
		options := make([]appmodel.QuestionOption, 0, len(question.Options))
		for _, option := range question.Options {
			options = append(options, appmodel.QuestionOption{Label: option.Label, Description: option.Description})
		}
		questions = append(questions, appmodel.Question{ID: id, Header: question.Header, Text: question.Text, Options: options})
	}
	session.model.Questions = append(session.model.Questions, appmodel.AskRequest{ID: request.ID, RunID: run.runID, Questions: questions})
	session.model.State = appmodel.WaitingQuestion
	session.model.UpdatedAt = time.Now()
	persistErr := c.persistLocked()
	snapshot := c.snapshotLocked()
	if persistErr != nil {
		delete(session.ask, request.ID)
		removeAsk(&session.model, request.ID)
		c.markPersistenceFailureLocked(session, persistErr)
		snapshot = c.snapshotLocked()
	}
	c.mu.Unlock()
	c.emit(snapshot)
	if persistErr != nil {
		return nil, fmt.Errorf("persist question request: %w", persistErr)
	}
	select {
	case answers := <-response:
		return answers, nil
	case <-waitContext.Done():
		c.clearAsk(sessionID, run, request.ID)
		return nil, waitContext.Err()
	}
}

func (c *Controller) finish(sessionID string, run *activeRun, result runtime.Result, runErr error) {
	c.mu.Lock()
	session, ok := c.sessions[sessionID]
	if !ok || session.active != run {
		c.mu.Unlock()
		return
	}
	clearPending(session)
	session.active = nil
	session.model.RunID = ""
	session.model.ActiveRun = nil

	shouldContinue := false
	switch run.cancelReason {
	case cancelReplace:
		// A replacement is allowed to start only after the old run has actually
		// observed cancellation. A generic error or a normal return without a
		// cancelled context is evidence that cancellation did not complete.
		cancelled := run.ctx != nil && run.ctx.Err() != nil
		cancelled = cancelled || errors.Is(runErr, context.Canceled)
		if cancelled && (runErr == nil || errors.Is(runErr, context.Canceled)) {
			session.model.State = appmodel.Idle
			session.model.QueuePaused = false
			session.model.PauseReason = ""
			session.model.Error = ""
			setRunMessageState(&session.model, run.runID, messageStateInterrupted, true)
			shouldContinue = true
		} else {
			session.model.State = appmodel.Failed
			session.model.QueuePaused = true
			session.model.PauseReason = pauseReasonCancelFailure
			session.model.Error = redaction.RedactString(cancellationError(runErr), redaction.Options{})
			setRunMessageState(&session.model, run.runID, messageStateFailed, false)
		}
	case cancelStop:
		session.model.State = appmodel.Interrupted
		session.model.QueuePaused = true
		session.model.PauseReason = pauseReasonStop
		session.model.Error = ""
		setRunMessageState(&session.model, run.runID, messageStateInterrupted, true)
	case cancelNone:
		if runErr != nil {
			session.model.State = appmodel.Failed
			session.model.QueuePaused = true
			session.model.PauseReason = pauseReasonFailure
			session.model.Error = redaction.RedactString(runErr.Error(), redaction.Options{})
			setRunMessageState(&session.model, run.runID, messageStateFailed, false)
		} else {
			session.model.State = appmodel.Idle
			session.model.QueuePaused = false
			session.model.PauseReason = ""
			session.model.Error = ""
			setRunMessageState(&session.model, run.runID, messageStateCompleted, false)
			shouldContinue = true
		}
	}
	finalizeAssistant(&session.model, run.runID, result.FinalAnswer, terminalMessageState(run, runErr))
	var next *activeRun
	if shouldContinue && !session.model.QueuePaused && len(session.model.Queue) > 0 {
		message := session.model.Queue[0]
		session.model.Queue = session.model.Queue[1:]
		next = c.startRunLocked(session, message)
	}
	session.model.UpdatedAt = time.Now()
	persistErr := c.persistLocked()
	if persistErr != nil {
		if next != nil {
			next.cancel()
			setRunMessageState(&session.model, next.runID, messageStateInterrupted, true)
			session.active = nil
			session.model.RunID = ""
			session.model.ActiveRun = nil
			next = nil
		}
		c.markPersistenceFailureLocked(session, persistErr)
	}
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	c.emit(snapshot)
	if next != nil {
		c.wg.Add(1)
		go c.execute(sessionID, next)
	}
}

func (c *Controller) onText(sessionID string, run *activeRun, text string) {
	c.mu.Lock()
	if !c.acceptLocked(sessionID, run) {
		c.mu.Unlock()
		return
	}
	session := c.sessions[sessionID]
	message := ensureAssistant(&session.model, run.runID)
	message.Text += text
	message.CreatedAt = time.Now()
	message.Interrupted = false
	session.model.UpdatedAt = time.Now()
	persistErr := c.persistLocked()
	if persistErr != nil {
		c.markPersistenceFailureLocked(session, persistErr)
	}
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	c.emit(snapshot)
}

func (c *Controller) onReasoning(sessionID string, run *activeRun, text string) {
	c.mu.Lock()
	if !c.acceptLocked(sessionID, run) {
		c.mu.Unlock()
		return
	}
	session := c.sessions[sessionID]
	message := ensureAssistant(&session.model, run.runID)
	message.Reasoning += text
	session.model.UpdatedAt = time.Now()
	persistErr := c.persistLocked()
	if persistErr != nil {
		c.markPersistenceFailureLocked(session, persistErr)
	}
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	c.emit(snapshot)
}

func (c *Controller) onToolCall(sessionID string, run *activeRun, call runtime.ToolCall) {
	c.mu.Lock()
	if !c.acceptLocked(sessionID, run) {
		c.mu.Unlock()
		return
	}
	session := c.sessions[sessionID]
	message := ensureAssistant(&session.model, run.runID)
	message.Tools = append(message.Tools, appmodel.ToolCall{ID: call.ID, Name: call.Name, Arguments: call.Arguments, State: "running"})
	session.model.UpdatedAt = time.Now()
	persistErr := c.persistLocked()
	if persistErr != nil {
		c.markPersistenceFailureLocked(session, persistErr)
	}
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	c.emit(snapshot)
}

func (c *Controller) onToolResult(sessionID string, run *activeRun, result runtime.ToolResult) {
	c.mu.Lock()
	if !c.acceptLocked(sessionID, run) {
		c.mu.Unlock()
		return
	}
	session := c.sessions[sessionID]
	message := assistantForToolResult(&session.model, run.runID, result.ID)
	for index := range message.Tools {
		if message.Tools[index].ID == result.ID {
			message.Tools[index].Output = result.Output
			message.Tools[index].State = result.Status
			message.Tools[index].Error = result.Error
			break
		}
	}
	session.model.UpdatedAt = time.Now()
	persistErr := c.persistLocked()
	if persistErr != nil {
		c.markPersistenceFailureLocked(session, persistErr)
	}
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	c.emit(snapshot)
}

func assistantForToolResult(session *appmodel.Session, runID, toolCallID string) *appmodel.Message {
	for index := len(session.Messages) - 1; index >= 0; index-- {
		message := &session.Messages[index]
		if message.Role != "assistant" || message.RunID != runID {
			continue
		}
		for _, tool := range message.Tools {
			if tool.ID == toolCallID {
				return message
			}
		}
	}
	return ensureAssistant(session, runID)
}

func (c *Controller) clearPermission(sessionID string, run *activeRun, id string) {
	c.mu.Lock()
	session, ok := c.sessions[sessionID]
	if !ok || session.active != run {
		c.mu.Unlock()
		return
	}
	delete(session.permission, id)
	removePermission(&session.model, id)
	if len(session.permission) == 0 && session.model.State == appmodel.WaitingPermission {
		session.model.State = appmodel.Running
	}
	if err := c.persistLocked(); err != nil {
		c.markPersistenceFailureLocked(session, err)
	}
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	c.emit(snapshot)
}

func (c *Controller) clearAsk(sessionID string, run *activeRun, id string) {
	c.mu.Lock()
	session, ok := c.sessions[sessionID]
	if !ok || session.active != run {
		c.mu.Unlock()
		return
	}
	delete(session.ask, id)
	removeAsk(&session.model, id)
	if len(session.ask) == 0 && session.model.State == appmodel.WaitingQuestion {
		session.model.State = appmodel.Running
	}
	if err := c.persistLocked(); err != nil {
		c.markPersistenceFailureLocked(session, err)
	}
	snapshot := c.snapshotLocked()
	c.mu.Unlock()
	c.emit(snapshot)
}

func (c *Controller) acceptLocked(sessionID string, run *activeRun) bool {
	session, ok := c.sessions[sessionID]
	return ok && session.active == run && run.generation == session.nextRun && !c.closed
}

func (c *Controller) load() error {
	data, err := os.ReadFile(c.statePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var stored storedState
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	c.settings = normalizeSettings(stored.Snapshot.Settings)
	for index := range c.settings.Providers {
		c.settings.Providers[index].HasAPIKey = c.hasCredential(c.settings.Providers[index].ID)
	}
	c.projects = append([]appmodel.Project(nil), stored.Snapshot.Projects...)
	for _, model := range stored.Snapshot.Sessions {
		wasActive := model.ActiveRun != nil || model.State == appmodel.Running || model.State == appmodel.Cancelling || model.State == appmodel.WaitingPermission || model.State == appmodel.WaitingQuestion
		if wasActive {
			markInterruptedOnRestart(&model)
			model.State = appmodel.Interrupted
			model.RunID = ""
			model.ActiveRun = nil
			model.QueuePaused = true
			model.PauseReason = pauseReasonRestart
			model.Permissions = nil
			model.Questions = nil
			model.Error = pauseReasonRestart
		}
		if model.Queue == nil {
			model.Queue = []appmodel.QueuedMessage{}
		}
		if model.Messages == nil {
			model.Messages = []appmodel.Message{}
		}
		for index := range model.Queue {
			if model.Queue[index].State == "" {
				model.Queue[index].State = messageStateQueued
			}
		}
		for index := range model.Messages {
			if model.Messages[index].State == "" {
				model.Messages[index].State = messageStateCompleted
			}
		}
		if c.worktrees != nil && model.Worktree {
			if err := c.worktrees.Bind(model.Cwd, model.ID); err != nil && model.Error == "" {
				model.Error = fmt.Sprintf("工作区 worktree 不可用：%v", err)
			}
		}
		c.sessions[model.ID] = &sessionState{model: model, permission: map[string]*pendingPermission{}, ask: map[string]*pendingAsk{}}
		c.order = append(c.order, model.ID)
	}
	return nil
}

func (c *Controller) persistLocked() error {
	stored := storedState{Version: 1, Snapshot: c.snapshotLocked()}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(c.dataDir, "controller-*.tmp")
	if err != nil {
		return err
	}
	path := tmp.Name()
	defer os.Remove(path)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(path, c.statePath)
}

// markPersistenceFailureLocked makes an in-memory snapshot visibly failed
// when an asynchronous callback cannot save it. The next snapshot therefore
// carries a diagnostic even when the filesystem is currently unavailable.
// Callers hold c.mu and should not assume a second persist can succeed.
func (c *Controller) markPersistenceFailureLocked(session *sessionState, err error) {
	if session == nil || err == nil {
		return
	}
	session.model.State = appmodel.Failed
	session.model.QueuePaused = true
	session.model.PauseReason = pauseReasonPersistence
	session.model.Error = redaction.RedactString(fmt.Sprintf("状态保存失败：%v", err), redaction.Options{})
	session.model.UpdatedAt = time.Now()
}

func (c *Controller) snapshotLocked() appmodel.Snapshot {
	snapshot := appmodel.Snapshot{Projects: append([]appmodel.Project(nil), c.projects...), Settings: cloneSettings(c.settings), Sessions: make([]appmodel.Session, 0, len(c.order))}
	for _, id := range c.order {
		if session, ok := c.sessions[id]; ok {
			snapshot.Sessions = append(snapshot.Sessions, cloneSession(session.model))
		}
	}
	return snapshot
}

func (c *Controller) emit(snapshot appmodel.Snapshot) {
	c.mu.Lock()
	callbacks := make([]func(appmodel.Snapshot), 0, len(c.subscribers))
	for _, callback := range c.subscribers {
		callbacks = append(callbacks, callback)
	}
	c.mu.Unlock()
	for _, callback := range callbacks {
		callback(snapshot)
	}
}

func (c *Controller) projectLocked(id string) (appmodel.Project, bool) {
	for _, project := range c.projects {
		if project.ID == id {
			return project, true
		}
	}
	return appmodel.Project{}, false
}

func (c *Controller) providerLocked(id string) bool {
	for _, provider := range c.settings.Providers {
		if provider.ID == id {
			return true
		}
	}
	return false
}

func (c *Controller) providerLockedValue(id string) appmodel.Provider {
	for _, provider := range c.settings.Providers {
		if provider.ID == id {
			return provider
		}
	}
	return appmodel.Provider{ID: id}
}

func (c *Controller) hasCredential(id string) bool {
	return c.credentials != nil && c.credentials.Has(id)
}

func normalizeSettings(settings appmodel.Settings) appmodel.Settings {
	if settings.DefaultSendMode != appmodel.Queue && settings.DefaultSendMode != appmodel.Correction {
		settings.DefaultSendMode = appmodel.Correction
	}
	if settings.Theme == "" {
		settings.Theme = "auto"
	}
	if settings.Providers == nil {
		settings.Providers = []appmodel.Provider{}
	}
	return settings
}

func cloneSettings(settings appmodel.Settings) appmodel.Settings {
	settings.Providers = append([]appmodel.Provider(nil), settings.Providers...)
	return settings
}

func cloneSession(session appmodel.Session) appmodel.Session {
	session.Messages = append([]appmodel.Message(nil), session.Messages...)
	for index := range session.Messages {
		session.Messages[index].Tools = append([]appmodel.ToolCall(nil), session.Messages[index].Tools...)
	}
	session.Queue = append([]appmodel.QueuedMessage(nil), session.Queue...)
	session.ActiveRun = cloneRunConfig(session.ActiveRun)
	session.Permissions = append([]appmodel.Permission(nil), session.Permissions...)
	for index := range session.Permissions {
		session.Permissions[index].Options = append([]appmodel.PermissionOption(nil), session.Permissions[index].Options...)
	}
	session.Questions = append([]appmodel.AskRequest(nil), session.Questions...)
	for index := range session.Questions {
		session.Questions[index].Questions = append([]appmodel.Question(nil), session.Questions[index].Questions...)
		for q := range session.Questions[index].Questions {
			session.Questions[index].Questions[q].Options = append([]appmodel.QuestionOption(nil), session.Questions[index].Questions[q].Options...)
		}
	}
	return session
}

func snapshotSessionState(session *sessionState) sessionStateSnapshot {
	if session == nil {
		return sessionStateSnapshot{}
	}
	permissions := make(map[string]*pendingPermission, len(session.permission))
	for id, pending := range session.permission {
		permissions[id] = pending
	}
	asks := make(map[string]*pendingAsk, len(session.ask))
	for id, pending := range session.ask {
		asks[id] = pending
	}
	activeCancel := cancelNone
	if session.active != nil {
		activeCancel = session.active.cancelReason
	}
	return sessionStateSnapshot{
		model:        cloneSession(session.model),
		active:       session.active,
		activeCancel: activeCancel,
		worktreeOp:   session.worktreeOp,
		nextRun:      session.nextRun,
		permission:   permissions,
		ask:          asks,
	}
}

func restoreSessionState(session *sessionState, snapshot sessionStateSnapshot) {
	if session == nil {
		return
	}
	session.model = snapshot.model
	session.active = snapshot.active
	if session.active != nil {
		session.active.cancelReason = snapshot.activeCancel
	}
	session.worktreeOp = snapshot.worktreeOp
	session.nextRun = snapshot.nextRun
	session.permission = snapshot.permission
	session.ask = snapshot.ask
}

func ensureAssistant(session *appmodel.Session, runID string) *appmodel.Message {
	if len(session.Messages) > 0 {
		last := &session.Messages[len(session.Messages)-1]
		if last.Role == "assistant" && last.RunID == runID {
			return last
		}
	}
	session.Messages = append(session.Messages, appmodel.Message{
		ID:        newID("assistant"),
		RunID:     runID,
		State:     messageStateStreaming,
		Role:      "assistant",
		CreatedAt: time.Now(),
	})
	return &session.Messages[len(session.Messages)-1]
}

func finalizeAssistant(session *appmodel.Session, runID, finalAnswer, state string) {
	var message *appmodel.Message
	for index := len(session.Messages) - 1; index >= 0; index-- {
		candidate := &session.Messages[index]
		if candidate.Role == "assistant" && candidate.RunID == runID {
			message = candidate
			break
		}
	}
	if message == nil && strings.TrimSpace(finalAnswer) != "" {
		session.Messages = append(session.Messages, appmodel.Message{
			ID:        newID("assistant"),
			RunID:     runID,
			State:     state,
			Role:      "assistant",
			Text:      finalAnswer,
			CreatedAt: time.Now(),
		})
		return
	}
	if message == nil {
		return
	}
	if strings.TrimSpace(finalAnswer) != "" && strings.TrimSpace(message.Text) == "" {
		message.Text = finalAnswer
	}
	message.State = state
	message.Interrupted = state == messageStateInterrupted
	message.CreatedAt = time.Now()
}

func terminalMessageState(run *activeRun, runErr error) string {
	if run.cancelReason == cancelReplace || run.cancelReason == cancelStop {
		return messageStateInterrupted
	}
	if runErr != nil {
		return messageStateFailed
	}
	return messageStateCompleted
}

func cancellationError(runErr error) string {
	if runErr == nil {
		return pauseReasonCancelFailure
	}
	return fmt.Sprintf("%s：%v", pauseReasonCancelFailure, runErr)
}

func setRunMessageState(session *appmodel.Session, runID, state string, interrupted bool) {
	for index := range session.Messages {
		message := &session.Messages[index]
		if message.Role != "user" || message.RunID != runID {
			continue
		}
		message.State = state
		message.Interrupted = interrupted
	}
}

func markInterruptedOnRestart(session *appmodel.Session) {
	runID := session.RunID
	for index := range session.Messages {
		message := &session.Messages[index]
		if message.Role != "user" {
			continue
		}
		if (runID != "" && message.RunID == runID) || message.State == messageStateRunning || message.State == messageStateCancelPending {
			message.State = messageStateInterrupted
			message.Interrupted = true
		}
	}
	for index := range session.Messages {
		message := &session.Messages[index]
		if message.Role == "assistant" && (runID == "" || message.RunID == runID) && (message.State == messageStateStreaming || message.State == messageStateRunning) {
			message.State = messageStateInterrupted
			message.Interrupted = true
		}
	}
}

func removePermission(session *appmodel.Session, id string) {
	for index, permission := range session.Permissions {
		if permission.ID == id {
			session.Permissions = append(session.Permissions[:index], session.Permissions[index+1:]...)
			return
		}
	}
}

func removeAsk(session *appmodel.Session, id string) {
	for index, ask := range session.Questions {
		if ask.ID == id {
			session.Questions = append(session.Questions[:index], session.Questions[index+1:]...)
			return
		}
	}
}

func clearPending(session *sessionState) {
	session.permission = map[string]*pendingPermission{}
	session.ask = map[string]*pendingAsk{}
	session.model.Permissions = nil
	session.model.Questions = nil
}

func toAppPermissionOptions(options []runtime.PermissionOption) []appmodel.PermissionOption {
	result := make([]appmodel.PermissionOption, 0, len(options))
	for _, option := range options {
		result = append(result, appmodel.PermissionOption{ID: option.ID, Label: option.Label})
	}
	return result
}

func lastUserPrompt(session appmodel.Session) string {
	for index := len(session.Messages) - 1; index >= 0; index-- {
		if session.Messages[index].Role == "user" {
			return session.Messages[index].Text
		}
	}
	return ""
}

func runConfigFor(session appmodel.Session, provider appmodel.Provider) appmodel.RunConfig {
	return appmodel.RunConfig{
		ProviderID:   provider.ID,
		ProviderName: provider.Name,
		Format:       provider.Format,
		BaseURL:      provider.BaseURL,
		Model:        provider.Model,
		Cwd:          session.Cwd,
		FullAccess:   session.FullAccess,
	}
}

func cloneRunConfig(config *appmodel.RunConfig) *appmodel.RunConfig {
	if config == nil {
		return nil
	}
	copy := *config
	return &copy
}

func sessionCreateInput(session appmodel.Session) sessions.CreateInput {
	return sessions.CreateInput{
		SessionID: session.ID,
		Title:     session.Title,
		Cwd:       session.Cwd,
		ModelID:   session.ProviderID,
		Provider:  session.ProviderID,
	}
}

// insertCorrection keeps all correction messages ahead of queued work while
// retaining the order in which corrections were submitted. A correction that
// arrives while a previous correction is being cancelled must not reverse the
// user's input order.
func insertCorrection(queue []appmodel.QueuedMessage, message appmodel.QueuedMessage) []appmodel.QueuedMessage {
	index := 0
	for index < len(queue) && queue[index].Mode == appmodel.Correction {
		index++
	}
	queue = append(queue, appmodel.QueuedMessage{})
	copy(queue[index+1:], queue[index:])
	queue[index] = message
	return queue
}

func newID(prefix string) string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err == nil {
		return prefix + "_" + hex.EncodeToString(bytes)
	}
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}
