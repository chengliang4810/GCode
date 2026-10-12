package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gcode/internal/agent"
	"gcode/internal/appmodel"
	"gcode/internal/background"
	"gcode/internal/config"
	"gcode/internal/credstore"
	"gcode/internal/execution"
	"gcode/internal/hooks"
	"gcode/internal/mcp"
	"gcode/internal/plugins"
	"gcode/internal/providers"
	"gcode/internal/redaction"
	"gcode/internal/sandbox"
	"gcode/internal/sessions"
	"gcode/internal/skills"
	"gcode/internal/specialist"
	"gcode/internal/streamjson"
	"gcode/internal/swarm"
	"gcode/internal/tools"
)

type Options struct {
	DataDir          string
	MaxTurns         int
	DeferThreshold   int
	MCP              config.MCPConfig
	PluginRoots      []plugins.Root
	SkillDir         string
	ProviderFactory  func(appmodel.Provider, string) (agent.Provider, error)
	ExecutionFactory func(string, string, *sandbox.Engine) (*execution.Runner, func(), error)
}

type Input struct {
	RunID            string
	SessionID        string
	CallingSessionID string
	CallingToolUseID string
	Depth            int
	Title            string
	Prompt           string
	Cwd              string
	Provider         appmodel.Provider
	FullAccess       bool
	PermissionMode   string
	Autonomy         string
	EnabledTools     []string
	DisabledTools    []string
}

type ToolCall struct {
	ID             string
	ProviderCallID string
	Name           string
	Arguments      string
	Freeform       bool
	Signature      string
}

type PermissionOption struct {
	ID    string
	Label string
}

type PermissionRequest struct {
	ID        string
	Tool      string
	Arguments string
	Options   []PermissionOption
}

type PermissionEvent struct {
	ID     string
	Tool   string
	Action string
	Reason string
}

type QuestionOption struct {
	Label       string
	Description string
}

type Question struct {
	Header  string
	Text    string
	Options []QuestionOption
}

type AskRequest struct {
	ID        string
	Questions []Question
}

type ToolResult struct {
	ID             string
	ProviderCallID string
	Name           string
	Status         string
	Output         string
	Error          string
	ChangedFiles   []string
}

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type Callbacks struct {
	OnText              func(string)
	OnReasoning         func(string)
	OnToolCall          func(ToolCall)
	OnPermissionRequest func(context.Context, PermissionRequest) (string, error)
	OnPermission        func(PermissionEvent)
	OnAskUser           func(context.Context, AskRequest) ([]string, error)
	OnToolResult        func(ToolResult)
	OnUsage             func(Usage)
	OnToolProgress      func(string, streamjson.Event)
	OnError             func(error)
	// OnWarning reports extension and other non-fatal runtime setup failures.
	// The run continues when a warning is reported. Fatal setup failures are
	// returned from Run as errors as usual.
	OnWarning func(error)
}

type Result struct {
	FinalAnswer string
	Turns       int
	Interrupted bool
}

type redactedError struct {
	message string
	cause   error
}

func (err redactedError) Error() string { return err.message }

func (err redactedError) Unwrap() error { return err.cause }

type Engine struct {
	dataDir         string
	maxTurns        int
	deferTools      int
	mcpConfig       config.MCPConfig
	mcpExplicit     bool
	configPath      string
	oauthPath       string
	pluginRoots     []plugins.Root
	pluginsExplicit bool
	skillDir        string
	provider        func(appmodel.Provider, string) (agent.Provider, error)
	execution       func(string, string, *sandbox.Engine) (*execution.Runner, func(), error)
	store           *sessions.Store
	credentials     *credstore.Store
	specialistRun   *specialist.Runtime
	mu              sync.RWMutex
	closed          bool
}

func New(options Options) (*Engine, error) {
	dataDir := strings.TrimSpace(options.DataDir)
	if dataDir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("resolve GCode data directory: %w", err)
		}
		dataDir = filepath.Join(base, "gcode")
	}
	absolute, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve GCode data directory: %w", err)
	}
	dataDir = filepath.Clean(absolute)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create GCode data directory: %w", err)
	}
	credentials, err := credstore.New(credstore.Options{
		Dir:     filepath.Join(dataDir, "credentials"),
		Storage: "encrypted-file",
	})
	if err != nil {
		return nil, fmt.Errorf("initialize GCode credentials: %w", err)
	}
	maxTurns := options.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 100
	}
	return &Engine{
		dataDir:         dataDir,
		maxTurns:        maxTurns,
		deferTools:      options.DeferThreshold,
		mcpConfig:       options.MCP,
		mcpExplicit:     options.MCP.Servers != nil,
		configPath:      filepath.Join(dataDir, "config.json"),
		oauthPath:       filepath.Join(dataDir, "oauth-tokens.json"),
		pluginRoots:     append([]plugins.Root(nil), options.PluginRoots...),
		pluginsExplicit: options.PluginRoots != nil,
		skillDir:        strings.TrimSpace(options.SkillDir),
		provider:        options.ProviderFactory,
		execution:       options.ExecutionFactory,
		store:           sessions.NewStore(sessions.StoreOptions{RootDir: filepath.Join(dataDir, "sessions")}),
		credentials:     credentials,
	}, nil
}

func DefaultDataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "gcode"), nil
}

func (engine *Engine) DataDir() string {
	if engine == nil {
		return ""
	}
	return engine.dataDir
}

func (engine *Engine) SessionStore() *sessions.Store {
	if engine == nil {
		return nil
	}
	return engine.store
}

func (engine *Engine) SetCredential(providerID, key string) error {
	if engine == nil || engine.credentials == nil {
		return errors.New("runtime is not initialized")
	}
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return errors.New("provider id is required")
	}
	if strings.TrimSpace(key) == "" {
		return nil
	}
	return engine.credentials.Set(providerID, key)
}

func (engine *Engine) RemoveCredential(providerID string) error {
	if engine == nil || engine.credentials == nil {
		return errors.New("runtime is not initialized")
	}
	_, err := engine.credentials.Delete(providerID)
	return err
}

func (engine *Engine) HasCredential(providerID string) bool {
	if engine == nil || engine.credentials == nil {
		return false
	}
	_, found, err := engine.credentials.Get(providerID)
	return err == nil && found
}

func (engine *Engine) Run(ctx context.Context, input Input, callbacks Callbacks) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := engine.validateInput(input); err != nil {
		return Result{}, err
	}
	engine.mu.RLock()
	closed := engine.closed
	engine.mu.RUnlock()
	if closed {
		return Result{}, errors.New("runtime is closed")
	}
	initialMessages, err := engine.sessionHistory(input.SessionID)
	if err != nil {
		return Result{}, fmt.Errorf("load session history: %w", err)
	}

	provider, key, err := engine.newProvider(input.Provider)
	if err != nil {
		return Result{}, err
	}
	runID := strings.TrimSpace(input.RunID)
	if runID == "" {
		runID = newRunID()
	}
	input.RunID = runID
	reportWarning := func(warning error) {
		if warning == nil {
			return
		}
		_ = engine.appendEvent(input.SessionID, sessions.EventMessage, map[string]any{
			"runId":   runID,
			"role":    "warning",
			"content": redact(warning.Error(), key),
		}, key)
		engine.reportWarning(callbacks, warning)
	}
	workspace := filepath.Clean(input.Cwd)
	var scope *sandbox.Scope
	if !input.FullAccess {
		scope, err = sandbox.NewScope(workspace, nil)
		if err != nil {
			return Result{}, err
		}
	}
	policy := sandbox.DefaultPolicy()
	if input.FullAccess {
		policy.Mode = sandbox.ModeDisabled
		policy.EnforceWorkspace = false
	}
	grantStore, err := sandbox.NewGrantStore(sandbox.StoreOptions{FilePath: filepath.Join(engine.dataDir, "sandbox-grants.json")})
	if err != nil {
		return Result{}, err
	}
	sandboxEngine := sandbox.NewEngine(sandbox.EngineOptions{
		WorkspaceRoot: workspace,
		Policy:        policy,
		Store:         grantStore,
		Backend:       sandbox.SelectBackend(sandbox.BackendOptions{}),
		Scope:         scope,
		SensitiveEnvKeys: []string{
			"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY",
		},
	})

	executionRunner := execution.NewRunner(nil)
	cleanupExecution := func() {}
	if engine.execution != nil {
		executionRunner, cleanupExecution, err = engine.execution(input.SessionID, workspace, sandboxEngine)
		if err != nil {
			return Result{}, err
		}
		if executionRunner == nil {
			return Result{}, errors.New("execution factory returned nil runner")
		}
	}
	executionRunner.SetPreparer(sandboxEngine)
	defer cleanupExecution()

	processManager := execution.NewProcessManager(execution.ProcessManagerOptions{})
	defer func() {
		_, _ = processManager.StopAllOwned(input.SessionID)
	}()

	registry := tools.NewRegistry()
	for _, tool := range tools.CoreToolsScopedWithManager(workspace, scope, processManager) {
		registry.Register(tool)
	}

	loadedPlugins, pluginErr := plugins.Load(plugins.LoadOptions{
		Roots: engine.pluginRootsFor(workspace),
		Cwd:   workspace,
	})
	activation := plugins.ActivationResult{}
	if pluginErr != nil {
		return Result{}, fmt.Errorf("load GCode plugins: %w", pluginErr)
	}
	activation = plugins.Activate(registry, loadedPlugins.Plugins, plugins.ActivateOptions{Cwd: workspace, Execution: executionRunner})
	for _, diagnostic := range loadedPlugins.Diagnostics {
		reportWarning(fmt.Errorf("plugin %s: %s", diagnostic.PluginID, diagnostic.Message))
	}
	for _, warning := range activation.Warnings {
		reportWarning(errors.New(warning))
	}

	hookResult, hookErr := hooks.LoadConfig(hooks.LoadOptions{
		Cwd:               workspace,
		UserConfigPath:    filepath.Join(engine.dataDir, "hooks.json"),
		ProjectConfigPath: filepath.Join(workspace, ".gcode", "hooks.json"),
	})
	if hookErr != nil {
		return Result{}, fmt.Errorf("load GCode hooks: %w", hookErr)
	}
	for _, diagnostic := range hookResult.Diagnostics {
		reportWarning(fmt.Errorf("hook %s: %s", diagnostic.HookID, diagnostic.Message))
	}
	if len(activation.Hooks) > 0 {
		hookResult.Config.Hooks = append(hookResult.Config.Hooks, activation.Hooks...)
		hookResult.Config.Enabled = true
	}
	hookDispatcher := hooks.NewDispatcher(hooks.DispatcherOptions{
		Config:    hookResult.Config,
		Cwd:       workspace,
		Execution: executionRunner,
	})

	defaultSkillDir := engine.skillDir
	if defaultSkillDir == "" {
		defaultSkillDir = filepath.Join(engine.dataDir, "skills")
	}
	skillRoots := []string{defaultSkillDir, filepath.Join(workspace, ".gcode", "skills")}
	if agents := skills.AgentsDir(nil); agents != "" {
		skillRoots = append(skillRoots, agents)
	}
	skillRoots = append(skillRoots, activation.SkillRoots...)
	loadedSkills, duplicates, skillsErr := skills.LoadFromRoots(skillRoots)
	if skillsErr != nil {
		return Result{}, fmt.Errorf("load GCode skills: %w", skillsErr)
	}
	for _, duplicate := range duplicates {
		reportWarning(fmt.Errorf("skill %q from %s shadows %s", duplicate.Name, duplicate.Winner, duplicate.Loser))
	}
	if skillTool := plugins.NewSkillToolWithRoots(defaultSkillDir, []string{filepath.Join(workspace, ".gcode", "skills")}, activation.SkillRoots); skillTool != nil {
		registry.Register(skillTool)
	}

	mcpRuntime, err := engine.registerMCP(ctx, registry, executionRunner, workspace)
	if err != nil {
		return Result{}, err
	}
	if mcpRuntime != nil {
		for _, skipped := range mcpRuntime.Skipped() {
			if skipped.Err == nil {
				reportWarning(fmt.Errorf("MCP server %s was skipped", skipped.Name))
				continue
			}
			reportWarning(fmt.Errorf("MCP server %s was skipped: %w", skipped.Name, skipped.Err))
		}
		defer mcpRuntime.Close()
	}

	paths := specialist.Paths{
		UserDir:    filepath.Join(engine.dataDir, "specialists"),
		ProjectDir: filepath.Join(workspace, ".gcode", "specialists"),
	}
	specialistRuntime, specialistErr := engine.specialistRuntime()
	if specialistErr != nil {
		return Result{}, specialistErr
	}
	executor := specialist.Executor{
		Paths:             paths,
		SessionStore:      engine.store,
		RunChild:          engine.childRunner(input, callbacks, provider, key),
		BackgroundRuntime: specialistRuntime,
	}
	if _, specialistErr := specialist.RegisterTools(registry, executor); specialistErr != nil {
		return Result{}, specialistErr
	}
	childSwarm, swarmErr := swarm.New(swarm.Options{
		BaseDir:  filepath.Join(engine.dataDir, "swarm", input.SessionID),
		Launcher: swarm.NewSpecialistLauncher(executor),
	})
	if swarmErr != nil {
		return Result{}, swarmErr
	}
	defer childSwarm.Close()
	swarm.RegisterTools(registry, childSwarm)

	specialistInfoList, specialistWarnings, specialistInfoErr := loadSpecialistInfos(paths)
	for _, warning := range specialistWarnings {
		reportWarning(redactedWarning(fmt.Errorf("specialist: %s", warning), key))
	}
	if specialistInfoErr != nil {
		reportWarning(redactedWarning(fmt.Errorf("load GCode specialists: %w", specialistInfoErr), key))
	}
	skillInfos := make([]agent.SkillInfo, 0, len(loadedSkills))
	for _, skill := range loadedSkills {
		skillInfos = append(skillInfos, agent.SkillInfo{Name: skill.Name, Description: skill.Description})
	}
	permissionMode := agent.PermissionMode(strings.TrimSpace(input.PermissionMode))
	if permissionMode == "" {
		permissionMode = agent.PermissionModeAsk
	}
	if input.FullAccess {
		permissionMode = agent.PermissionModeUnsafe
	}

	result, runErr := engine.runAgent(ctx, input, provider, key, registry, sandboxEngine, hookDispatcher, permissionMode, specialistInfoList, skillInfos, initialMessages, callbacks)
	if runErr != nil {
		// The provider key is available only inside the runtime. Redact once at
		// this boundary so callers such as the controller cannot accidentally
		// persist an authentication value carried by a provider/network error.
		redactedErr := redactedError{
			message: redaction.RedactString(runErr.Error(), redaction.Options{ExtraSecretValues: []string{key}}),
			cause:   runErr,
		}
		return Result{FinalAnswer: result.FinalAnswer, Turns: result.Turns, Interrupted: errors.Is(runErr, context.Canceled) || ctx.Err() != nil}, redactedErr
	}
	return Result{FinalAnswer: result.FinalAnswer, Turns: result.Turns}, nil
}

func (engine *Engine) sessionHistory(sessionID string) ([]agent.Message, error) {
	if engine == nil || engine.store == nil || strings.TrimSpace(sessionID) == "" {
		return nil, nil
	}
	metadata, err := engine.store.Get(sessionID)
	if err != nil {
		return nil, err
	}
	if metadata == nil || metadata.EventCount == 0 {
		return nil, nil
	}
	events, err := engine.store.ReadRehydratedEvents(sessionID)
	if err != nil {
		return nil, err
	}
	messages := sessions.CompactionMessages(events)
	if len(messages) == 0 {
		return nil, nil
	}
	return messages, nil
}

func (engine *Engine) registerMCP(ctx context.Context, registry *tools.Registry, runner *execution.Runner, workspace string) (*mcp.Runtime, error) {
	mcpConfig, err := engine.mcpConfigFor(workspace)
	if err != nil {
		return nil, err
	}
	if len(mcpConfig.Servers) == 0 {
		return nil, nil
	}
	permissions, err := mcp.NewPermissionStore(mcp.StoreOptions{FilePath: filepath.Join(engine.dataDir, "mcp-permissions.json")})
	if err != nil {
		return nil, err
	}
	return mcp.RegisterTools(ctx, registry, mcpConfig, mcp.RegisterOptions{
		PermissionStore: permissions,
		Autonomy:        mcp.AutonomyLow,
		Execution:       runner,
		WorkspaceRoot:   workspace,
		OAuthTokenPath:  engine.oauthPath,
	})
}

func (engine *Engine) pluginRootsFor(workspace string) []plugins.Root {
	if engine.pluginsExplicit {
		return append([]plugins.Root(nil), engine.pluginRoots...)
	}
	return []plugins.Root{
		{Source: plugins.SourceUser, Path: filepath.Join(engine.dataDir, "plugins")},
		{Source: plugins.SourceProject, Path: filepath.Join(workspace, ".gcode", "plugins")},
	}
}

func (engine *Engine) mcpConfigFor(workspace string) (config.MCPConfig, error) {
	if engine.mcpExplicit {
		return engine.mcpConfig, nil
	}
	userPath, err := existingFile(engine.configPath)
	if err != nil {
		return config.MCPConfig{}, err
	}
	projectPath, err := existingFile(filepath.Join(workspace, ".gcode", "config.json"))
	if err != nil {
		return config.MCPConfig{}, err
	}
	if userPath == "" && projectPath == "" {
		return config.MCPConfig{}, nil
	}
	resolved, err := config.ResolveMCP(config.ResolveOptions{
		UserConfigPath:    userPath,
		ProjectConfigPath: projectPath,
	})
	if err != nil {
		return config.MCPConfig{}, fmt.Errorf("resolve GCode MCP config: %w", err)
	}
	return resolved, nil
}

func existingFile(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect GCode config %s: %w", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("GCode config path %s is a directory, want a file", path)
	}
	return path, nil
}

func (engine *Engine) reportWarning(callbacks Callbacks, err error) {
	if err == nil {
		return
	}
	if callbacks.OnWarning != nil {
		callbacks.OnWarning(err)
		return
	}
	if callbacks.OnError != nil {
		callbacks.OnError(err)
	}
}

func (engine *Engine) reportRedactedWarning(callbacks Callbacks, err error, key string) {
	if err == nil {
		return
	}
	engine.reportWarning(callbacks, redactedWarning(err, key))
}

func redactedWarning(err error, key string) error {
	if err == nil {
		return nil
	}
	return redactedError{
		message: redaction.RedactString(err.Error(), redaction.Options{ExtraSecretValues: []string{key}}),
		cause:   err,
	}
}

func (engine *Engine) newProvider(provider appmodel.Provider) (agent.Provider, string, error) {
	key := ""
	if engine.credentials != nil {
		stored, found, err := engine.credentials.Get(provider.ID)
		if err != nil {
			return nil, "", err
		}
		if found {
			key = stored
		}
	}
	if engine.provider != nil {
		created, err := engine.provider(provider, key)
		return created, key, err
	}
	profile := config.ProviderProfile{
		Name:         provider.ID,
		ProviderKind: providerKind(provider),
		BaseURL:      provider.BaseURL,
		Model:        provider.Model,
		APIFormat:    string(provider.Format),
		APIKey:       key,
	}
	created, err := providers.New(profile, providers.Options{UserAgent: "gcode"})
	return created, key, err
}

func (engine *Engine) validateInput(input Input) error {
	if engine == nil {
		return errors.New("runtime is nil")
	}
	if strings.TrimSpace(input.SessionID) == "" || !sessions.ValidSessionID(input.SessionID) {
		return errors.New("valid session id is required")
	}
	if strings.TrimSpace(input.Prompt) == "" {
		return errors.New("prompt is required")
	}
	if strings.TrimSpace(input.Cwd) == "" {
		return errors.New("working directory is required")
	}
	return nil
}

func newRunID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "run_" + strings.ReplaceAll(fmt.Sprintf("%d", os.Getpid()), "-", "")
	}
	return "run_" + hex.EncodeToString(bytes)
}

func providerProfile(provider appmodel.Provider, key string) config.ProviderProfile {
	return config.ProviderProfile{
		Name:         provider.ID,
		ProviderKind: providerKind(provider),
		BaseURL:      provider.BaseURL,
		Model:        provider.Model,
		APIFormat:    string(provider.Format),
		APIKey:       key,
	}
}

func providerKind(provider appmodel.Provider) config.ProviderKind {
	baseURL := strings.TrimSpace(provider.BaseURL)
	switch provider.Format {
	case appmodel.Responses, appmodel.ChatCompletions:
		if baseURL == "" || officialProviderHost(baseURL, "api.openai.com") {
			return config.ProviderKindOpenAI
		}
		return config.ProviderKindOpenAICompatible
	case appmodel.Anthropic:
		if baseURL == "" || officialProviderHost(baseURL, "api.anthropic.com") {
			return config.ProviderKindAnthropic
		}
		return config.ProviderKindAnthropicCompat
	case appmodel.Gemini:
		return config.ProviderKindGoogle
	default:
		return ""
	}
}

func officialProviderHost(rawURL, wantHost string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(parsed.Hostname()), wantHost)
}

func encode(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

func redact(value string, key string) string {
	options := redaction.Options{}
	if strings.TrimSpace(key) != "" {
		options.ExtraSecretValues = []string{key}
	}
	return redaction.RedactString(value, options)
}

func loadSpecialistInfos(paths specialist.Paths) ([]agent.SpecialistInfo, []string, error) {
	loaded, err := specialist.Load(specialist.LoadOptions{Paths: paths})
	if err != nil {
		return nil, nil, err
	}
	warnings := append([]string(nil), loaded.Warnings...)
	result := make([]agent.SpecialistInfo, 0, len(loaded.Specialists))
	for _, manifest := range loaded.Specialists {
		for _, warning := range manifest.Warnings {
			warnings = append(warnings, fmt.Sprintf("specialist %q: %s", manifest.Metadata.Name, warning))
		}
		if strings.TrimSpace(manifest.Metadata.Name) == "" {
			continue
		}
		result = append(result, agent.SpecialistInfo{Name: manifest.Metadata.Name, WhenToUse: manifest.Metadata.Description})
	}
	return result, warnings, nil
}

func (engine *Engine) Close() error {
	if engine == nil {
		return nil
	}
	engine.mu.Lock()
	if engine.closed {
		specialistRuntime := engine.specialistRun
		engine.mu.Unlock()
		if specialistRuntime == nil {
			return nil
		}
		return specialistRuntime.Close()
	}
	engine.closed = true
	specialistRuntime := engine.specialistRun
	engine.mu.Unlock()

	if specialistRuntime == nil {
		return nil
	}
	return specialistRuntime.Close()
}

func (engine *Engine) specialistRuntime() (*specialist.Runtime, error) {
	if engine == nil {
		return nil, errors.New("runtime is not initialized")
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.closed {
		return nil, errors.New("runtime is closed")
	}
	if engine.specialistRun != nil {
		return engine.specialistRun, nil
	}
	rootDir := filepath.Join(engine.dataDir, "background")
	engine.specialistRun = specialist.NewRuntime(specialist.RuntimeOptions{
		ManagerFunc: func() (*background.Manager, error) {
			return background.NewManager(rootDir)
		},
	})
	return engine.specialistRun, nil
}
