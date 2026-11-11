package engine

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/mcp"
	"gcode/internal/model"
	"gcode/internal/streamtext"
)

func (e *Engine) sendLocked(send contracts.SendRequest) error {
	return e.sendWithCheckoutLocked(send, nil)
}

func (e *Engine) sendWithCheckoutLocked(send contracts.SendRequest, checkout *checkoutResult) error {
	for _, refs := range e.state.RepoRefs {
		if refs.Scope.ProjectID == send.ProjectID && refs.Switching != "" {
			return errors.New("wait for the project's branch switch to finish")
		}
	}
	if strings.TrimSpace(contracts.PlainPrompt(send.Text)) == "/compact" {
		return e.compactLocked(send.SessionID, send.RequestID)
	}
	if send.Delivery != "" && send.Delivery != "follow-up" && send.Delivery != "steer" {
		return errors.New("invalid message delivery")
	}
	if send.RequestID != "" {
		for _, run := range e.state.Runs {
			if run.RequestID == send.RequestID {
				return nil
			}
		}
		for _, queued := range e.state.Queue {
			if queued.RequestID == send.RequestID {
				return nil
			}
		}
	}
	if handled, err := e.goalTextLocked(send, checkout); handled {
		if err == nil && checkout != nil {
			checkout.accepted = true
		}
		return err
	}
	if strings.TrimSpace(send.Text) == "" && len(send.Attachments) == 0 {
		return errors.New("message is empty")
	}
	sessionID := send.SessionID
	index := e.sessionIndex(sessionID)
	created := index < 0
	var session contracts.Session
	if sessionID != "" && index < 0 {
		return errors.New("session not found")
	}
	if index >= 0 {
		if err := e.ensureSessionDataLocked(sessionID); err != nil {
			return err
		}
		for _, message := range e.state.Messages[sessionID] {
			if send.RequestID != "" && message.RequestID == send.RequestID {
				return nil
			}
		}
		session = e.state.Sessions[index]
		if session.SubagentType != "" {
			return errors.New("subagent sessions are read-only")
		}
	} else {
		title := []rune(strings.TrimSpace(contracts.DisplayPrompt(send.Text)))
		if len(title) > 48 {
			title = title[:48]
		}
		rootID := ""
		if checkout != nil {
			rootID = checkout.storageRootID
		}
		for _, attachment := range send.Attachments {
			if attachment.StorageRootID != "" {
				if rootID != "" && rootID != attachment.StorageRootID {
					return errors.New("draft attachments belong to different storage roots")
				}
				rootID = attachment.StorageRootID
			}
		}
		var err error
		session, err = e.newSessionLocked(send.ProjectID, string(title), rootID)
		if err != nil {
			return err
		}
		sessionID = session.ID
		index = 0
	}
	if checkout != nil && created {
		session.CWD, session.Branch, session.WorktreeID = checkout.cwd, checkout.branch, checkout.worktreeID
		session.SourceContext = checkout.source
		e.applyGitProjectLocked(session.ProjectID, checkout.repo)
	}
	var selectedModel, selectedReasoning *string
	if send.ModelID != "" {
		selectedModel = &send.ModelID
	}
	if send.ReasoningLevel != "" {
		selectedReasoning = &send.ReasoningLevel
	}
	if selectedModel != nil || selectedReasoning != nil {
		if err := e.setSessionModelLocked(&session, selectedModel, selectedReasoning); err != nil {
			return err
		}
	}
	e.state.Sessions[index] = session
	prompt := ""
	if checkout != nil && checkout.input != nil {
		prompt = checkout.prompt
	} else {
		var err error
		prompt, err = expandPrompt(e.storage.ConfigDirectory(), session.CWD, send.Text)
		if err != nil {
			return err
		}
	}
	if run := e.runs[sessionID]; run != nil {
		if run.compacting {
			return errors.New("wait for context compaction to finish before sending")
		}
		if len(e.state.Queue) >= 100 {
			return errors.New("session message queue is full")
		}
		e.state.Queue = append(e.state.Queue, contracts.QueuedMessage{ID: id("queue"), RequestID: send.RequestID, SessionID: sessionID, Text: send.Text, Prompt: prompt, Delivery: send.Delivery, Attachments: send.Attachments, ModelID: session.ModelID, ReasoningLevel: session.ReasoningLevel, CreatedAt: time.Now()})
		if err := e.persistLocked(session.ID); err != nil {
			return err
		}
		e.publishLocked(contracts.Event{Kind: "queue.changed", SessionID: sessionID, RequestID: send.RequestID, RunID: run.id})
		e.wakeRunLocked(sessionID)
		return nil
	}
	var message model.Message
	if checkout != nil && checkout.input != nil {
		message = *checkout.input
	} else {
		var inputErr error
		message, inputErr = e.inputMessageLocked(prompt, send.Attachments)
		if inputErr != nil {
			return inputErr
		}
	}
	message.SourceText = send.Text
	e.history[sessionID] = append(e.history[sessionID], message)
	e.context[sessionID] = append(e.context[sessionID], message)
	e.appendUserLocked(sessionID, send.Text)
	messages := e.state.Messages[sessionID]
	messages[len(messages)-1].RequestID = send.RequestID
	if len(send.Attachments) > 0 {
		messages := e.state.Messages[sessionID]
		for _, item := range send.Attachments {
			attachment := item
			messages[len(messages)-1].Blocks = append(messages[len(messages)-1].Blocks, contracts.Block{ID: id("block"), Kind: "attachment", Attachment: &attachment})
		}
	}
	if err := e.startLocked(session, send.RequestID); err != nil {
		return err
	}
	if checkout != nil {
		checkout.accepted = true
	}
	if created {
		e.startNamingLocked(session, send.Text)
	}
	return nil
}

func (e *Engine) appendUserLocked(sessionID, text string) {
	message := contracts.Message{ID: id("message"), SessionID: sessionID, Role: "user", CreatedAt: time.Now(), Blocks: []contracts.Block{{ID: id("block"), Kind: "text", Text: text}}}
	e.state.Messages[sessionID] = append(e.state.Messages[sessionID], message)
}

func (e *Engine) resolveModel(key string) (model.Provider, model.Model, error) {
	return resolveConfiguredModel(e.providers, key)
}

func resolveConfiguredModel(providers []model.Provider, key string) (model.Provider, model.Model, error) {
	for _, provider := range providers {
		if !provider.Ready() {
			continue
		}
		for _, m := range provider.Models {
			if m.Disabled {
				continue
			}
			if provider.ID+"/"+m.ID == key {
				return provider, m, nil
			}
		}
	}
	return model.Provider{}, model.Model{}, errors.New("configure a provider and select a model in Settings")
}

func (e *Engine) startLocked(session contracts.Session, requestID string) error {
	return e.startRunLocked(context.Background(), session, requestID, nil)
}

func (e *Engine) startRunLocked(
	ctx context.Context,
	session contracts.Session,
	requestID string,
	child *subagentExecution,
) error {
	return e.startRunWithProvidersLocked(ctx, session, requestID, child, nil)
}

func (e *Engine) startRunWithProvidersLocked(
	ctx context.Context,
	session contracts.Session,
	requestID string,
	child *subagentExecution,
	providers []model.Provider,
) error {
	if err := e.ensureSessionDataLocked(session.ID); err != nil {
		return err
	}
	for _, refs := range e.state.RepoRefs {
		if refs.Scope.ProjectID == session.ProjectID && refs.Switching != "" {
			return errors.New("wait for the project's branch switch to finish")
		}
	}
	if providers == nil {
		providers = e.providers
	}
	if child != nil {
		providers = child.providers
	}
	provider, m, err := resolveConfiguredModel(providers, session.ModelID)
	if err != nil {
		return err
	}
	effort, err := m.ResolveEffort(session.ReasoningLevel)
	if err != nil {
		return err
	}
	session.ReasoningLevel = model.ReasoningLabel(effort)
	if _, err := e.client.Bind(provider, m); err != nil {
		return err
	}
	if _, err := os.Stat(session.CWD); err != nil {
		return fmt.Errorf("workspace directory: %w", err)
	}
	processEnv, err := e.processEnvironment(session.StorageRootID, session.ID)
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(ctx)
	run := &activeRun{
		cancel: cancel, id: id("run"), answers: map[string]chan []contracts.QuestionAnswer{},
		done: make(chan struct{}), subagent: child,
		wake: make(chan struct{}, 1), pending: map[string]contracts.QueuedMessage{},
		selection: contracts.ModelSelection{ModelID: session.ModelID, Reasoning: session.ReasoningLevel},
		profiles:  contracts.EffectiveSubagentProfiles(e.state.Settings.Subagents), model: m,
	}
	if child != nil {
		run.goal = child.goal
	} else if session.Goal != nil && session.Goal.Status == contracts.GoalActive {
		run.goal = goalScope{session.ID, session.Goal.ID}
	}
	for _, provider := range providers {
		run.providers = append(run.providers, model.CloneProvider(provider))
	}
	e.runs[session.ID] = run
	index := e.sessionIndex(session.ID)
	priorSession := e.state.Sessions[index]
	priorSession.Goal = priorSession.Goal.Clone()
	priorHistory, priorContext := e.history[session.ID], e.context[session.ID]
	priorDirty := maps.Clone(e.sessionDirty)
	priorRunCount := len(e.state.Runs)
	e.state.Sessions[index].Status = contracts.Working
	e.state.Sessions[index].UpdatedAt = time.Now()
	if run.goal.id != "" && child == nil {
		goal := e.state.Sessions[index].Goal
		now := time.Now()
		goal.ActiveSince, goal.LastSeenAt, goal.UpdatedAt = &now, now, now
	}
	e.state.Runs = append(e.state.Runs, contracts.Run{ID: run.id, SessionID: session.ID, RequestID: requestID, Status: "running", StartedAt: time.Now()})
	e.history[session.ID] = closeInterruptedCalls(e.history[session.ID])
	e.context[session.ID] = closeInterruptedCalls(e.context[session.ID])
	run.sideChatHistory = throughLatestHumanInput(e.history[session.ID])
	run.sideChatContext = throughLatestHumanInput(e.context[session.ID])
	history := model.CloneMessages(e.context[session.ID])
	if err := e.persistLocked(session.ID); err != nil {
		delete(e.runs, session.ID)
		e.state.Sessions[index] = priorSession
		e.state.Runs = e.state.Runs[:priorRunCount]
		e.history[session.ID], e.context[session.ID] = priorHistory, priorContext
		e.sessionDirty = priorDirty
		cancel()
		return err
	}
	if child == nil {
		if e.taskModels == nil {
			e.taskModels = make(map[string]taskModel)
		}
		e.taskModels[session.ID] = taskModel{selection: run.selection, providers: run.providers}
	}
	e.publishLocked(contracts.Event{Kind: "run.started", SessionID: session.ID, RunID: run.id, RequestID: requestID})
	root := e.storage.ConfigDirectory()
	hookRules := contracts.CloneHookRules(e.state.Settings.Hooks)
	configSnapshot := clonePluginConfig(e.pluginConfig)
	profileSnapshot := contracts.CloneSubagentProfiles(e.state.Settings.Subagents)
	searchSettings := e.state.Settings.General.WebSearch
	memoryEnabled := e.state.Settings.Personalization.MemoryEnabled && child == nil
	memoryDir := memoryDirectory(e.storage.ConfigDirectory(), session.CWD)
	if e.sessionHookOnce == nil {
		e.sessionHookOnce = map[string]*agent.HookOnceState{}
	}
	if e.sessionStarted == nil {
		e.sessionStarted = map[string]bool{}
	}
	if e.sessionHookOnce[session.ID] == nil {
		e.sessionHookOnce[session.ID] = &agent.HookOnceState{}
	}
	onceState := e.sessionHookOnce[session.ID]
	skipSessionStart := child != nil || e.sessionStarted[session.ID]
	e.sessionStarted[session.ID] = true
	hookSource := "startup"
	for _, previous := range e.state.Runs[:priorRunCount] {
		if previous.SessionID == session.ID {
			hookSource = "resume"
			break
		}
	}
	session.Goal = session.Goal.Clone()
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer cancel()
		defer e.signalSubagentStarted(child)
		liveModel := sessionModel{core: e, sessionID: session.ID, run: run, trackReply: true}
		var bound agent.Model = liveModel
		var auxiliary agent.Model = sessionModel{core: e, sessionID: session.ID, run: run}
		if run.goal.id != "" {
			bound = goalModel{inner: bound, core: e, scope: run.goal, sessionID: session.ID}
			auxiliary = goalModel{inner: auxiliary, core: e, scope: run.goal, sessionID: session.ID}
			if child == nil {
				stopHeartbeat := e.startGoalHeartbeat(runCtx, run.goal)
				defer stopHeartbeat()
			}
		}
		bundles, resourcesErr := e.plugins.Bundles()
		if child != nil {
			bundles = child.plugins
		}
		resources, err := e.prepareRuntimeResources(root, session, bundles, configSnapshot, profileSnapshot)
		if resourcesErr == nil {
			resourcesErr = err
		}
		e.mu.Lock()
		run.plugins = bundles
		run.profiles = resources.profiles
		e.mu.Unlock()
		hookRules = append(hookRules, resources.hooks...)
		if child != nil {
			hookRules = append(hookRules, contracts.CloneHookRules(child.skillScope.hooks)...)
		}
		system := basePrompt
		system += "\n" + agent.EnvironmentInstructions(session.CWD)
		if child == nil || !child.profile.SkipProjectInstructions {
			system += loadInstructions(root, session.CWD)
		}
		system += resources.instructions()
		if memoryEnabled {
			_ = os.MkdirAll(memoryDir, 0o700)
			system += "\n\n" + memorySection(memoryDir)
		}
		// Everything above is stable for the session and worth caching; what
		// follows depends on the request (side chat framing, MCP discovery
		// results, subagent identity). The marker lets model adapters put a
		// prompt-cache breakpoint at the seam instead of re-paying the whole
		// instruction block on every request.
		system += "\n\n" + model.SystemPromptDynamicBoundary + "\n\n"
		if session.SideChat && child == nil {
			system += "\n\n" + sideChatInstructions
		}
		currentMessage := ""
		var tools []agent.Tool
		closeTools := func() {}
		var toolErr error
		var mcpWarnings []string
		if child != nil && contracts.IsReadOnlySubagent(child.profile.ID) {
			tools = readOnlyTools()
		} else {
			tools, closeTools, mcpWarnings, toolErr = loadRunTools(runCtx, root, session.CWD, processEnv)
		}
		defer closeTools()
		if toolErr == nil {
			toolErr = resourcesErr
		}
		runtimeTools, runtimeToolsErr := e.sessionRuntimeTools(session, resources.envFile)
		if toolErr == nil {
			toolErr = runtimeToolsErr
		}
		if child != nil && contracts.IsReadOnlySubagent(child.profile.ID) {
			runtimeTools = readOnlyTools()
		}
		for _, tool := range tools {
			if strings.HasPrefix(tool.Definition().Name, "mcp__") {
				runtimeTools = append(runtimeTools, tool)
			}
		}
		tools = runtimeTools
		// Add optional tools after selecting the session-owned runtime set.
		// Search credentials remain independent of the model provider.
		if toolErr == nil && searchSettings.Enabled && (child == nil || !contracts.IsReadOnlySubagent(child.profile.ID)) {
			tools = append(tools, agent.NewWebSearchTool(&agent.WebSearchConfig{Endpoint: searchSettings.Endpoint, Token: searchSettings.Token}))
		}
		var pluginClients []*mcp.Client
		defer func() {
			for _, client := range pluginClients {
				_ = client.Close()
			}
		}()
		if child == nil || !contracts.IsReadOnlySubagent(child.profile.ID) {
			for _, config := range resources.mcp {
				client, connectErr := connectMCP(runCtx, mcpProcessEnvironment(config, processEnv), session.CWD)
				if connectErr == nil {
					var remote []mcp.RemoteTool
					listCtx, finish := context.WithTimeout(runCtx, 20*time.Second)
					remote, connectErr = client.Tools(listCtx)
					finish()
					mcpWarnings = append(mcpWarnings, mcpDiscoveryWarnings(client)...)
					if connectErr == nil {
						pluginClients = append(pluginClients, client)
						for _, tool := range remote {
							tools = append(tools, tool)
						}
					} else {
						_ = client.Close()
					}
				}
				if connectErr != nil {
					system += "\nPlugin MCP unavailable (" + config.Name + "): " + connectErr.Error()
				}
			}
		}
		if len(mcpWarnings) > 0 {
			system += "\nMCP discovery and protocol limitations:\n" + strings.Join(mcpWarnings, "\n")
		}
		if child != nil {
			system += "\n\nSubagent task instructions:\n" + child.profile.Prompt
			system += "\nYou are a subagent. Return the result to your parent; you cannot delegate further."
		} else {
			tools = append(tools, e.goalReadTool(goalScope{sessionID: session.ID}))
			if profiles := enabledSubagentProfiles(run.profiles); len(profiles) > 0 {
				tools = append(tools, e.delegationTool(session.ID, run, profiles))
			}
			if run.goal.id != "" {
				system += "\n\n" + goalContinuation(session.Goal)
			}
		}
		tools = append(tools, agent.QuestionTool{Ask: func(ctx context.Context, questions []contracts.QuestionPage) ([]contracts.QuestionAnswer, error) {
			e.mu.Lock()
			questionID := id("question")
			request := &contracts.Question{ID: questionID, Pages: questions}
			answer := make(chan []contracts.QuestionAnswer, 1)
			run.answers[questionID] = answer
			messages := e.state.Messages[session.ID]
			foundMessage := false
			for i := range messages {
				if messages[i].ID == currentMessage {
					foundMessage = true
					messages[i].Blocks = append(messages[i].Blocks, contracts.Block{ID: questionID, Kind: "question", Question: request})
				}
			}
			if !foundMessage {
				e.state.Messages[session.ID] = append(messages, contracts.Message{ID: id("message"), SessionID: session.ID, TurnID: run.id, Role: "assistant", CreatedAt: time.Now(), Blocks: []contracts.Block{{ID: questionID, Kind: "question", Question: request}}})
			}
			if index := e.sessionIndex(session.ID); index >= 0 {
				e.state.Sessions[index].Status = contracts.AwaitingInput
			}
			if err := e.persistLocked(session.ID); err != nil {
				delete(run.answers, questionID)
				e.mu.Unlock()
				return nil, err
			}
			e.publishLocked(contracts.Event{Kind: "input.requested", SessionID: session.ID})
			e.mu.Unlock()
			defer func() {
				e.mu.Lock()
				delete(run.answers, questionID)
				if index := e.sessionIndex(session.ID); index >= 0 {
					e.state.Sessions[index].Status = contracts.Working
				}
				e.mu.Unlock()
			}()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case value := <-answer:
				return value, nil
			}
		}})
		if child != nil {
			tools = filterSubagentTools(tools, child.profile)
		}
		tools = e.guardTools(tools)
		runErr := toolErr
		hooks, invokeHook := e.runHookRunner(session, run, hookRules, hookRuntimeOptions{Model: auxiliary, Tools: tools, Warnings: mcpWarnings, EnvFile: resources.envFile, OnceState: onceState})
		e.mu.Lock()
		run.hook = invokeHook
		e.mu.Unlock()
		var skillCleanups []func()
		var skillMu sync.Mutex
		activeSkills := map[string]bool{}
		skillDenials := map[string][]string{}
		var inheritedDenials []string
		if child != nil {
			inheritedDenials = slices.Clone(child.skillScope.disallowed)
		}
		defer func() {
			for _, cleanup := range skillCleanups {
				cleanup()
			}
		}()
		activateSkill := func(skill runtimeSkill) (func(), error) {
			skillMu.Lock()
			defer skillMu.Unlock()
			if activeSkills[skill.name] {
				return nil, nil
			}
			rules := componentHooks(skill.name, skill.component.Metadata.Hooks, skill.bundle, skill.data, skill.config, "skill")
			cleanup, err := hooks.PushRules(rules)
			if err != nil {
				return nil, err
			}
			skillDenials[skill.name] = slices.Clone(skill.component.Metadata.DisallowedTools)
			activeSkills[skill.name] = true
			if skill.component.Metadata.Context != "fork" {
				skillCleanups = append(skillCleanups, cleanup)
				return nil, nil
			}
			return func() {
				skillMu.Lock()
				defer skillMu.Unlock()
				cleanup()
				delete(activeSkills, skill.name)
				delete(skillDenials, skill.name)
			}, nil
		}
		toolAllowed := func(name string) bool {
			if child != nil {
				if contracts.IsReadOnlySubagent(child.profile.ID) &&
					!slices.Contains([]string{"Read", "ListDirectory", "Glob", "Grep"}, name) {
					return false
				}
				if len(child.profile.Tools) > 0 && !matchesSubagentTool(child.profile.Tools, name) {
					return false
				}
				if deniedSubagentTool(child.profile.DisallowedTools, name) {
					return false
				}
			}
			skillMu.Lock()
			defer skillMu.Unlock()
			if deniedSubagentTool(inheritedDenials, name) {
				return false
			}
			for _, denied := range skillDenials {
				if deniedSubagentTool(denied, name) {
					return false
				}
			}
			return true
		}
		run.toolAllowed = toolAllowed
		tools = append(tools, e.guardTool(e.runtimeSkillTool(session, run, resources, invokeHook, activateSkill)))
		if child != nil {
			tools = filterSubagentTools(tools, child.profile)
		}
		startupStopped := false
		if child != nil {
			started, startErr := e.subagentStartHook(runCtx, session, child)
			runErr = errors.Join(runErr, startErr)
			startupStopped = started.Continue != nil && !*started.Continue
			if startErr == nil && started.Context != "" {
				contextMessages := []model.Message{{Role: "user", Text: "<system-reminder>\n" + started.Context + "\n</system-reminder>"}}
				history = append(model.CloneMessages(contextMessages), history...)
				e.mu.Lock()
				e.history[session.ID] = append(model.CloneMessages(contextMessages), e.history[session.ID]...)
				e.context[session.ID] = append(model.CloneMessages(contextMessages), e.context[session.ID]...)
				runErr = errors.Join(runErr, e.persistLocked(session.ID))
				e.mu.Unlock()
			}
		}
		if memoryEnabled && runErr == nil {
			if index := loadMemoryIndex(memoryDir); index != "" {
				e.mu.Lock()
				// The map is shared by all sessions, even when this run is
				// the only writer of its own conversation.
				if !hasMemoryIndex(e.history[session.ID]) {
					contextMessages := []model.Message{memoryContextMessage(index)}
					history = append(model.CloneMessages(contextMessages), history...)
					e.history[session.ID] = append(model.CloneMessages(contextMessages), e.history[session.ID]...)
					e.context[session.ID] = append(model.CloneMessages(contextMessages), e.context[session.ID]...)
					runErr = errors.Join(runErr, e.persistLocked(session.ID))
				}
				e.mu.Unlock()
			}
		}
		if runErr == nil && !startupStopped {
			runErr = agent.Run(runCtx, agent.Options{
				ToolAllowed: toolAllowed,
				Hook:        invokeHook, HookSource: hookSource, SkipSessionStart: skipSessionStart,
				PrepareInput: func(ctx context.Context, input model.Message) (model.Message, error) {
					return e.prepareRuntimeInput(ctx, input, session, run, resources, invokeHook, activateSkill)
				},
				CommitPreparedInput: func(input model.Message) error {
					e.mu.Lock()
					defer e.mu.Unlock()
					for i := len(e.history[session.ID]) - 1; i >= 0; i-- {
						if e.history[session.ID][i].UserInput {
							e.history[session.ID][i] = input
							break
						}
					}
					for i := len(e.context[session.ID]) - 1; i >= 0; i-- {
						if e.context[session.ID][i].UserInput {
							e.context[session.ID][i] = input
							break
						}
					}
					return e.persistLocked(session.ID)
				},
				Model: bound, ContextLimits: liveModel.contextLimits,
				Request: model.Request{System: system, History: history, Effort: session.ReasoningLevel},
				CWD:     session.CWD, Tools: tools,
				StoreToolOutput: func(ctx context.Context, text string) (string, error) {
					return e.storeToolOutput(ctx, session, text)
				},
				Record: func(message model.Message) error {
					e.mu.Lock()
					defer e.mu.Unlock()
					e.history[session.ID] = append(e.history[session.ID], message)
					e.context[session.ID] = append(e.context[session.ID], message)
					if message.UserInput {
						run.sideChatHistory = model.CloneMessages(e.history[session.ID])
						run.sideChatContext = model.CloneMessages(e.context[session.ID])
					}
					return e.persistLocked(session.ID)
				},
				ReplaceContext: func(messages []model.Message) error {
					e.mu.Lock()
					defer e.mu.Unlock()
					e.context[session.ID] = model.CloneMessages(messages)
					return e.persistLocked(session.ID)
				},
				Pending: func(idle bool) []model.Message {
					e.mu.Lock()
					defer e.mu.Unlock()
					return e.drainPendingLocked(session.ID, idle)
				},
				BeforeStep: func(ctx context.Context, idle bool) (bool, error) {
					if run.goal.id != "" {
						proceed, err := e.goalBeforeStep(ctx, run.goal)
						if !proceed || err != nil {
							if !proceed && err == nil && child != nil {
								err = errGoalHalted
							}
							return proceed, err
						}
					}
					e.mu.Lock()
					handoff := e.handoffUserInputLocked(session.ID, run, idle)
					e.mu.Unlock()
					if handoff {
						return false, ctx.Err()
					}
					return true, nil
				},
				Idle: func(ctx context.Context, request model.Request) ([]model.Message, error) {
					if run.goal.id == "" || child != nil {
						return nil, nil
					}
					return e.goalIdle(ctx, run, auxiliary, request)
				},
				OwnsCompletion: run.goal.id != "" && child == nil,
				Emit: func(event agent.Event) error {
					e.mu.Lock()
					defer e.mu.Unlock()
					return e.applyAgentEventLocked(session.ID, run.id, &currentMessage, event)
				},
			})
		}
		if runErr != nil {
			cancel()
		}
		runErr = errors.Join(runErr, hooks.Wait())
		if errors.Is(runErr, errGoalHalted) {
			runErr = context.Canceled
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		defer e.evictIdleSessionsLocked()
		if runErr == nil {
			runErr = runCtx.Err()
		}
		if run.goal.id != "" && child == nil {
			e.restoreGoalInputsLocked(session.ID, run)
			if goal := e.goalLocked(run.goal); goal != nil {
				settleGoalTime(goal, time.Now())
				if run.nextInput == nil && (goal.Status == contracts.GoalActive || goal.Status == contracts.GoalVerifying) {
					goal.Status = contracts.GoalPaused
					goal.Reason = "Execution stopped before completion was verified."
					if runErr != nil {
						goal.Reason = runErr.Error()
					}
				}
			}
		}
		index := e.sessionIndex(session.ID)
		if index >= 0 {
			e.state.Sessions[index].Status = contracts.Idle
			e.state.Sessions[index].LastOutcome = "completed"
			if runErr != nil {
				e.state.Sessions[index].LastOutcome = "failed"
				if errors.Is(runErr, context.Canceled) {
					e.state.Sessions[index].LastOutcome = "interrupted"
				} else {
					e.state.Sessions[index].Status = contracts.Errored
				}
			}
		}
		for i := range e.state.Runs {
			r := &e.state.Runs[i]
			if r.ID == run.id {
				now := time.Now()
				r.FinishedAt = &now
				r.DurationMS = now.Sub(r.StartedAt).Milliseconds()
				r.Status = "completed"
				if runErr != nil {
					r.Status = "failed"
					r.Error = runErr.Error()
					if errors.Is(runErr, context.Canceled) {
						r.Status = "interrupted"
					}
				}
			}
		}
		for i := range e.state.Messages[session.ID] {
			e.state.Messages[session.ID][i].Streaming = false
			for j := range e.state.Messages[session.ID][i].Blocks {
				tool := e.state.Messages[session.ID][i].Blocks[j].Tool
				if question := e.state.Messages[session.ID][i].Blocks[j].Question; question != nil && e.state.Messages[session.ID][i].TurnID == run.id {
					question.Resolved = true
				}
				if tool != nil && tool.Status == "running" {
					now := time.Now()
					tool.Status = "interrupted"
					tool.FinishedAt = &now
				}
			}
		}
		if index := e.sessionIndex(session.ID); index >= 0 {
			e.state.Sessions[index].ModelRetry = nil
		}
		if runErr != nil && !errors.Is(runErr, context.Canceled) {
			label := "Run failed: "
			detail := runErr.Error()
			if errors.Is(runErr, model.ErrStreamInterrupted) {
				detail = "The model connection closed before the reply finished, and retries did not recover it. Completed tool results are saved; send a message to continue."
			}
			if e.state.Settings.General.Language == "zh-CN" {
				label = "运行失败："
				if errors.Is(runErr, model.ErrStreamInterrupted) {
					detail = "模型连接在回复完成前断开，重试后仍未恢复。已完成的工具结果已保留，可继续发送消息。"
				}
			}
			message := contracts.Message{ID: id("message"), SessionID: session.ID, TurnID: run.id, Role: "assistant", CreatedAt: time.Now(), Blocks: []contracts.Block{{ID: id("block"), Kind: "error", Text: label + detail}}}
			e.state.Messages[session.ID] = append(e.state.Messages[session.ID], message)
		}
		delete(e.runs, session.ID)
		run.output = finalReply(e.history[session.ID])
		if child != nil {
			e.finishSubagentLocked(session, run, runErr)
		}
		e.refreshProjectFilesLocked(session.ProjectID)
		terminalSaved := true
		if persistErr := e.persistLocked(session.ID); persistErr != nil {
			terminalSaved = false
			runErr = errors.Join(runErr, fmt.Errorf("cannot save execution history: %w", persistErr))
			for i := range e.state.Runs {
				if e.state.Runs[i].ID == run.id {
					e.state.Runs[i].Status = "failed"
					e.state.Runs[i].Error = runErr.Error()
				}
			}
			if index := e.sessionIndex(session.ID); index >= 0 {
				e.state.Sessions[index].Status = contracts.Errored
				e.state.Sessions[index].LastOutcome = "failed"
			}
			if child != nil {
				e.finishSubagentLocked(session, run, runErr)
			}
		}
		run.err = runErr
		if child != nil {
			e.publishLocked(contracts.Event{Kind: "subagent.finished", SessionID: child.parentSessionID})
			if terminalSaved {
				e.resumeSubagentParentLocked(run)
			}
		}
		e.publishLocked(contracts.Event{Kind: "run.finished", SessionID: session.ID, RunID: run.id, RequestID: requestID})
		close(run.done)
		goalHandoff := true
		humanOnly := false
		if run.goal.id != "" {
			goal := e.goalLocked(run.goal)
			goalHandoff = goal != nil && (goal.Status == contracts.GoalActive || goal.Status == contracts.GoalComplete)
			humanOnly = goal != nil && goal.Status == contracts.GoalComplete
		}
		nextInput := run.nextInput
		run.nextInput = nil
		if nextInput != nil {
			admitted := runErr == nil || run.inputInterrupt && errors.Is(runErr, context.Canceled)
			if !e.closed && terminalSaved && goalHandoff && admitted {
				e.state.Queue = slices.DeleteFunc(e.state.Queue, func(q contracts.QueuedMessage) bool { return q.ID == nextInput.ID })
				_ = e.handoffQueuedLocked(session.ID, *nextInput)
			} else {
				if !slices.ContainsFunc(e.state.Queue, func(q contracts.QueuedMessage) bool { return q.ID == nextInput.ID }) {
					e.state.Queue = append([]contracts.QueuedMessage{*nextInput}, e.state.Queue...)
				}
			}
		} else if runErr == nil && !e.closed && goalHandoff {
			// A message can arrive after the final idle drain but before the
			// runtime is retired. Hand it to a new run rather than stranding it.
			for i, queued := range e.state.Queue {
				internal := queued.Source == "subagent" || queued.Source == "hook"
				if queued.SessionID == session.ID && (!humanOnly || !internal) {
					e.state.Queue = append(e.state.Queue[:i], e.state.Queue[i+1:]...)
					_ = e.handoffQueuedLocked(session.ID, queued)
					break
				}
			}
		}
	}()
	return nil
}

func (e *Engine) applyAgentEventLocked(sessionID, runID string, current *string, event agent.Event) error {
	if event.Kind == "context.compacting" {
		e.publishLocked(contracts.Event{Kind: "context.compacting", SessionID: sessionID, RunID: runID})
		return nil
	}
	messages := e.state.Messages[sessionID]
	switch event.Kind {
	case "message.start":
		message := contracts.Message{ID: id("message"), SessionID: sessionID, TurnID: runID, Role: "assistant", CreatedAt: time.Now(), Streaming: true}
		if run := e.runs[sessionID]; run != nil {
			message.ModelID = run.selection.ModelID
		}
		*current = message.ID
		if run := e.runs[sessionID]; run != nil {
			run.messageID, run.messageIndex = message.ID, len(messages)
			run.textBlocks = [2]streamtext.Buffer{}
		}
		e.state.Messages[sessionID] = append(messages, message)
		e.publishLocked(contracts.Event{Kind: "message.changed", SessionID: sessionID, RunID: runID, Message: &message})
		return nil
	}
	index := -1
	run := e.runs[sessionID]
	if run != nil && run.messageID == *current && run.messageIndex < len(messages) && messages[run.messageIndex].ID == *current {
		index = run.messageIndex
	} else {
		for i := range messages {
			if messages[i].ID == *current {
				index = i
				break
			}
		}
	}
	if index < 0 {
		return errors.New("assistant message is unavailable")
	}
	message := &messages[index]
	switch event.Kind {
	case "message.display":
		replaced := false
		for i := range message.Blocks {
			if message.Blocks[i].Kind == "text" {
				if !replaced {
					message.Blocks[i].Text = event.Text
					replaced = true
				} else {
					message.Blocks[i].Text = ""
				}
			}
		}
		if !replaced && event.Text != "" {
			message.Blocks = append(message.Blocks, contracts.Block{ID: id("block"), Kind: "text", Text: event.Text})
		}
	case "message.retry":
		message.Blocks = nil
		message.Streaming = true
	case "message.delta":
		for partIndex, part := range []struct{ kind, text string }{{"reasoning", event.Reasoning}, {"text", event.Text}} {
			if part.text == "" {
				continue
			}
			block := -1
			for i := range message.Blocks {
				if message.Blocks[i].Kind == part.kind {
					block = i
					break
				}
			}
			if block < 0 {
				message.Blocks = append(message.Blocks, contracts.Block{ID: id("block"), Kind: part.kind})
				block = len(message.Blocks) - 1
			}
			if run != nil {
				message.Blocks[block].Text = run.textBlocks[partIndex].Append(message.Blocks[block].Text, part.text)
			} else {
				message.Blocks[block].Text += part.text
			}
			e.publishLocked(contracts.Event{Kind: "message.delta", SessionID: sessionID, RunID: runID, MessageID: message.ID, BlockKind: part.kind, Text: part.text})
		}
		return nil
	case "message.end":
		// One message.end closes a single model round, not the turn: the
		// round's pending tool calls run straight after it. The message stays
		// Streaming until the run settles, so a turn that is still working
		// keeps its live activity group open between rounds.
		message.Usage = contracts.Usage{InputTokens: event.Usage.Input, OutputTokens: event.Usage.Output, CachedTokens: event.Usage.Cached, ContextTokens: event.Usage.Input}
		if index := e.sessionIndex(sessionID); index >= 0 {
			if run := e.runs[sessionID]; run != nil {
				m := run.model
				message.Usage.ContextLimit = m.ContextWindow
				message.Usage.Cost = (float64(event.Usage.Input)*m.InputPrice + float64(event.Usage.Output)*m.OutputPrice) / 1_000_000
				tokens := uint64(max(event.Usage.Input, 0))
				usage := contracts.ContextUsage{Tokens: &tokens}
				if m.ContextWindow > 0 {
					window := uint64(m.ContextWindow)
					usage.Window = &window
				}
				e.state.ContextUsage[sessionID] = usage
			}
		}
	case "tool.start":
		now := time.Now()
		path := e.beginEditLocked(sessionID, event.Call)
		tool := toolPresentation(event.Call, path, now)
		message.Blocks = append(message.Blocks, contracts.Block{ID: event.Call.ID, Kind: "tool", Tool: &tool})
	case "tool.input.changed":
		path := e.beginEditLocked(sessionID, event.Call)
		for i := range message.Blocks {
			if tool := message.Blocks[i].Tool; tool != nil && tool.ID == event.Call.ID {
				tool.Args, tool.Path = event.Call.Arguments, path
			}
		}
	case "tool.end":
		for i := range message.Blocks {
			block := &message.Blocks[i]
			if block.Tool != nil && block.Tool.ID == event.Call.ID {
				now := time.Now()
				block.Tool.Output = event.Output
				block.Tool.IsError = event.Error
				block.Tool.Status = "completed"
				if event.Error {
					block.Tool.Status = "failed"
				}
				block.Tool.FinishedAt = &now
				block.Tool.Diff = e.finishEditLocked(sessionID, event.Call)
				if block.Tool.Kind == contracts.ToolReadFile {
					block.Tool.ReadLines = readLineCount(event.Data)
				}
			}
		}
	}
	changed := *message
	// publishLocked synchronously projects and gives each subscriber its own
	// block payload before releasing the engine lock.
	e.publishLocked(contracts.Event{Kind: "message.changed", SessionID: sessionID, RunID: runID, Message: &changed})
	return nil
}

func loadInstructions(root, cwd string) string {
	var result strings.Builder
	for _, path := range []string{filepath.Join(root, "AGENTS.md"), filepath.Join(cwd, "AGENTS.md")} {
		data, err := os.ReadFile(path)
		if err == nil && len(data) <= 256<<10 {
			result.WriteString("\n\n")
			result.Write(data)
		}
	}
	return result.String()
}
