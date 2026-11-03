// Package engine is the authoritative Go application core shared by desktop and headless hosts.
package engine

import (
	"context"
	"crypto/rand"
	"errors"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/modelcatalog"
	"gcode/internal/plugins"
	"gcode/internal/storage"
	"gcode/internal/terminalhost"
)

type Options struct {
	ToolGuard ToolGuard
	Storage   *storage.Store
	Client    *model.Client
	Catalog   *modelcatalog.Cache
}

type activeRun struct {
	toolAllowed func(string) bool
	plugins     []plugins.Bundle
	hook        func(context.Context, agent.HookInput) (agent.HookResult, error)
	cancel      context.CancelFunc
	id          string
	answers     map[string]chan string
	edits       map[string]editBefore
	compacting  bool
	done        chan struct{}
	err         error
	output      string
	subagent    *subagentExecution
	selection   contracts.ModelSelection
	profiles    []contracts.SubagentProfile
	providers   []model.Provider
	model       model.Model
	goal        goalScope
	pending     map[string]contracts.QueuedMessage
	wake        chan struct{}
	// afterStop is a queue row whose Send now stopped this run: it opens
	// the next turn once the run has settled.
	afterStop *contracts.QueuedMessage
	// Side chats inherit through the latest accepted human input, before the
	// active reply and its tools. These snapshots also survive live compaction.
	sideChatHistory []model.Message
	sideChatContext []model.Message
}

type Engine struct {
	toolGuard         ToolGuard
	mu                sync.Mutex
	storage           *storage.Store
	sessionHashes     map[string][32]byte
	sessionRecords    map[string]string
	persistErrors     []error
	loadedSessions    map[string]bool
	client            *model.Client
	state             contracts.Snapshot
	providers         []model.Provider
	history           map[string][]model.Message
	context           map[string][]model.Message
	runs              map[string]*activeRun
	naming            map[string]*namingRun
	gitOperations     map[string]context.CancelFunc
	checkoutProjects  map[string]int
	checkoutRequests  map[string]bool
	subs              map[uint64]chan contracts.Event
	subNext           uint64
	sequence          uint64
	closed            bool
	done              chan struct{}
	wg                sync.WaitGroup
	terminals         map[string]*terminalhost.Session
	resources         userResourceCatalog
	modelChecks       map[string]*modelCheckRun
	modelCheckResults map[string]contracts.ModelCheck
	modelFetchRuns    map[string]*modelFetchRun
	modelFetchResults map[string]contracts.ModelFetch
	catalog           *modelcatalog.Cache
	catalogCancel     context.CancelFunc
	catalogDone       chan struct{}
	maintenanceDone   chan struct{}
	plugins           *plugins.Manager
	pluginOperations  map[string]context.CancelFunc
	pluginConfig      map[string]map[string]string
	sessionHookOnce   map[string]*agent.HookOnceState
	sessionStarted    map[string]bool
	sessionTools      map[string]*sessionToolSet
	projectScans      map[string]*projectScan
	projectScanner    func(context.Context, contracts.Project) ([]contracts.File, error)
}

var _ contracts.Service = (*Engine)(nil)

func Open(options Options) (*Engine, error) {
	var err error
	if options.Storage == nil {
		return nil, errors.New("data storage is required")
	}
	client := options.Client
	if client == nil {
		client = model.NewClient()
	}
	e := &Engine{toolGuard: options.ToolGuard, storage: options.Storage, client: client, runs: map[string]*activeRun{}, subs: map[uint64]chan contracts.Event{}, done: make(chan struct{}), history: map[string][]model.Message{}, context: map[string][]model.Message{}, terminals: map[string]*terminalhost.Session{}}
	e.modelChecks, e.modelCheckResults = map[string]*modelCheckRun{}, map[string]contracts.ModelCheck{}
	e.modelFetchRuns, e.modelFetchResults = map[string]*modelFetchRun{}, map[string]contracts.ModelFetch{}
	e.projectScans = map[string]*projectScan{}
	e.projectScanner = collectProjectFiles
	e.naming = map[string]*namingRun{}
	if err := os.MkdirAll(filepath.Join(options.Storage.ConfigDirectory(), "skills"), 0700); err != nil {
		return nil, err
	}
	if err := storage.PrepareRuntime(options.Storage); err != nil {
		return nil, err
	}
	for _, name := range []string{"workspaces", "uploads", "logs", "sessions"} {
		if err := os.MkdirAll(filepath.Join(options.Storage.Directory(), name), 0700); err != nil {
			return nil, err
		}
	}
	e.state = contracts.Snapshot{SchemaVersion: contracts.SchemaVersion, Revision: 1, Settings: contracts.DefaultSettings(), Messages: map[string][]contracts.Message{}, ContextUsage: map[string]contracts.ContextUsage{}, Capabilities: contracts.Capabilities{Browser: true}, User: contracts.User{ID: "local", Name: "Local", Initials: "G"}, ModelCatalog: contracts.ModelCatalogState{State: contracts.CatalogEmpty}, Integrations: contracts.IntegrationState{ProvidersState: contracts.LoadReady}}
	if err := e.loadPersistence(); err != nil {
		return nil, err
	}
	e.loadStandaloneTerminals()
	hostname, _ := os.Hostname()
	e.state.Devices = []contracts.Device{{ID: "local", Name: hostname, Platform: runtime.GOOS, Architecture: runtime.GOARCH, Current: true, Status: "connected", LastSeen: time.Now()}}
	e.refreshProvidersLocked()
	for i := range e.state.Terminals {
		if e.state.Terminals[i].Status == "running" {
			e.state.Terminals[i].Status = "interrupted"
		}
	}
	for key, refs := range e.state.RepoRefs {
		if refs.State == contracts.LoadLoading || refs.Switching != "" {
			refs.State, refs.Error, refs.Switching = contracts.LoadError, "Repository operation was interrupted when the host stopped; refresh to retry.", ""
			e.state.RepoRefs[key] = refs
		}
	}
	e.refreshStorageLocked()
	e.resources, err = readResourceCatalog(e.storage.ConfigDirectory())
	if err != nil {
		return nil, err
	}
	e.refreshUserResourcesLocked()
	e.plugins, err = plugins.NewManager(filepath.Join(e.storage.ConfigDirectory(), "plugins"))
	if err != nil {
		return nil, err
	}
	e.pluginOperations = map[string]context.CancelFunc{}
	e.sessionHookOnce = map[string]*agent.HookOnceState{}
	e.sessionStarted = map[string]bool{}
	e.sessionTools = map[string]*sessionToolSet{}
	e.pluginConfig, err = readPluginConfiguration(e.storage.ConfigDirectory())
	if err != nil {
		return nil, err
	}
	e.state.Plugins = e.pluginProjection()
	e.applyPluginConfiguredLocked()
	if err := e.persistLocked(); err != nil {
		return nil, err
	}
	e.catalog = options.Catalog
	if e.catalog == nil {
		e.catalog, err = modelcatalog.Open(modelcatalog.Options{Directory: filepath.Join(options.Storage.Directory(), "cache", "gcode")})
		if err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.catalogCancel = cancel
	e.catalogDone = make(chan struct{})
	go func() {
		defer close(e.catalogDone)
		e.catalog.Run(ctx)
	}()
	maintenance := storage.StartRuntimeMaintenance(ctx, options.Storage)
	e.maintenanceDone = make(chan struct{})
	go func() {
		defer close(e.maintenanceDone)
		for err := range maintenance {
			log.Printf("runtime storage maintenance: %v", err)
		}
	}()
	e.mu.Lock()
	for _, project := range e.state.Projects {
		if project.FilesState == contracts.LoadLoading {
			e.startProjectFilesLocked(project)
		}
	}
	e.mu.Unlock()
	return e, nil
}

func (e *Engine) Bootstrap(ctx context.Context) (contracts.Snapshot, error) { return e.Snapshot(ctx) }
func (e *Engine) Snapshot(ctx context.Context) (contracts.Snapshot, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.checkLocked(ctx); err != nil {
		return contracts.Snapshot{}, err
	}
	return contracts.CloneSnapshot(e.state), nil
}
func (e *Engine) Subscribe(ctx context.Context) (<-chan contracts.Event, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.checkLocked(ctx); err != nil {
		return nil, err
	}
	e.subNext++
	id := e.subNext
	ch := make(chan contracts.Event, 256)
	e.subs[id] = ch
	snapshot := contracts.CloneSnapshot(e.state)
	ch <- contracts.Event{SchemaVersion: contracts.SchemaVersion, Kind: "snapshot", Revision: snapshot.Revision, Sequence: e.sequence, Snapshot: &snapshot}
	go func() {
		select {
		case <-ctx.Done():
		case <-e.done:
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		if current, ok := e.subs[id]; ok {
			delete(e.subs, id)
			close(current)
		}
	}()
	return ch, nil
}
func (e *Engine) Close() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.closed = true
	for _, scan := range e.projectScans {
		scan.cancel()
	}
	e.catalogCancel()
	close(e.done)
	for _, check := range e.modelChecks {
		check.cancel()
	}
	for _, run := range e.runs {
		run.cancel()
	}
	for _, run := range e.naming {
		run.cancel()
	}
	for _, cancel := range e.gitOperations {
		cancel()
	}
	for _, cancel := range e.pluginOperations {
		cancel()
	}
	hosts := make([]*terminalhost.Session, 0, len(e.terminals))
	for _, host := range e.terminals {
		hosts = append(hosts, host)
	}
	toolOwners := make([]*sessionToolSet, 0, len(e.sessionTools))
	for _, owner := range e.sessionTools {
		toolOwners = append(toolOwners, owner)
	}
	e.mu.Unlock()
	for _, owner := range toolOwners {
		owner.cleanup()
	}
	for _, host := range hosts {
		_ = host.Close()
	}
	e.wg.Wait()
	<-e.catalogDone
	<-e.maintenanceDone
	e.mu.Lock()
	// Flush available roots even when an unrelated historical disk vanished.
	// Report any unsaved records after committing the roots that remain online.
	err := e.persistLocked()
	err = errors.Join(err, errors.Join(e.persistErrors...))
	e.mu.Unlock()
	if err != nil {
		log.Printf("save runtime state before shutdown: %v", err)
	}
	return errors.Join(err, e.storage.Close())
}
func (e *Engine) checkLocked(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e.closed {
		return errors.New("engine is closed")
	}
	return nil
}
func id(prefix string) string { return prefix + "-" + strings.ToLower(rand.Text()) }
func (e *Engine) refreshStorageLocked() {
	e.state.Runtime = &contracts.RuntimeInfo{ConfigDirectory: e.storage.ConfigDirectory(), DataDirectory: e.storage.Directory(), PendingDirectory: e.storage.Pending(), StorageError: e.storage.ChangeError()}
}
func (e *Engine) refreshProvidersLocked() {
	e.state.CustomProviders = nil
	e.state.Providers = nil
	e.state.Models = nil
	e.state.Integrations.Providers = nil
	for _, config := range e.providers {
		public := model.CloneProvider(config)
		public.APIKey = ""
		e.state.CustomProviders = append(e.state.CustomProviders, contracts.ProviderConfiguration{Provider: public, HasAPIKey: config.APIKey != "", ModelChecks: e.providerModelChecksLocked(config.ID), ModelFetch: e.providerModelFetchLocked(config.ID)})
		provider := contracts.Provider{ID: config.ID, Name: config.Name, Icon: "widget", Enabled: !config.Disabled, Status: "ready"}
		ready := config.Ready()
		if config.Disabled {
			provider.Status = "disabled"
		} else if !ready {
			provider.Status = "incomplete"
		}
		for _, m := range config.Models {
			if !ready || m.Disabled {
				continue
			}
			key := config.ID + "/" + m.ID
			levels := make([]string, 0, len(m.ReasoningLevels))
			for _, level := range m.ReasoningLevels {
				levels = append(levels, model.ReasoningLabel(level))
			}
			e.state.Models = append(e.state.Models, contracts.Model{ID: key, ProviderID: config.ID, Label: m.ID, ContextLimit: m.ContextWindow, ReasoningLevels: levels})
			provider.Models = append(provider.Models, key)
		}
		e.state.Providers = append(e.state.Providers, provider)
		e.state.Integrations.Providers = append(e.state.Integrations.Providers, contracts.ProviderDetails{ProviderID: config.ID, TargetID: "local", Installed: true, Enabled: !config.Disabled, InstallState: "idle", AccountsState: contracts.LoadReady})
	}
	e.state.ModelCatalog.State = contracts.CatalogReady
	if len(e.state.Models) == 0 {
		e.state.ModelCatalog.State = contracts.CatalogEmpty
	}
}
func (e *Engine) publishLocked(event contracts.Event) {
	e.state.Revision++
	e.sequence++
	event.SchemaVersion = contracts.SchemaVersion
	event.Revision = e.state.Revision
	event.Sequence = e.sequence
	event.EventID = id("event")
	if event.Message == nil && event.Kind != "message.delta" {
		snapshot := contracts.CloneSnapshot(e.state)
		event.Snapshot = &snapshot
	}
	for _, ch := range e.subs {
		owned := event
		if event.ExitCode != nil {
			value := *event.ExitCode
			owned.ExitCode = &value
		}
		if event.Snapshot != nil {
			snapshot := contracts.CloneSnapshot(*event.Snapshot)
			owned.Snapshot = &snapshot
		}
		if event.Message != nil {
			message := *event.Message
			message.Blocks = contracts.CloneBlocks(message.Blocks)
			owned.Message = &message
		}
		select {
		case ch <- owned:
		default:
			// A slow consumer gets a complete resynchronization instead of a gap.
			for len(ch) > 0 {
				<-ch
			}
			snapshot := contracts.CloneSnapshot(e.state)
			ch <- contracts.Event{SchemaVersion: contracts.SchemaVersion, Kind: "snapshot", Revision: snapshot.Revision, Sequence: e.sequence, Snapshot: &snapshot}
		}
	}
}
func (e *Engine) sessionIndex(id string) int {
	for i := range e.state.Sessions {
		if e.state.Sessions[i].ID == id {
			return i
		}
	}
	return -1
}
func (e *Engine) sessionGroupIndex(id string) int {
	for i := range e.state.SessionGroups {
		if e.state.SessionGroups[i].ID == id {
			return i
		}
	}
	return -1
}
func (e *Engine) projectPath(projectID string) string {
	for _, project := range e.state.Projects {
		if project.ID == projectID {
			return project.Path
		}
	}
	return filepath.Join(e.storage.Directory(), "workspaces")
}

func closeInterruptedCalls(history []model.Message) []model.Message {
	pending := map[string]bool{}
	for _, message := range history {
		for _, call := range message.Calls {
			pending[call.ID] = true
		}
		if message.Role == "tool" {
			delete(pending, message.CallID)
		}
	}
	for callID := range pending {
		history = append(history, model.Message{Role: "tool", CallID: callID, IsError: true, Text: "Execution was interrupted before a result was recorded. Inspect current state before retrying side effects."})
	}
	return history
}
