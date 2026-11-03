package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"gcode/internal/agent"
	"gcode/internal/contracts"
	"gcode/internal/model"
)

func (e *Engine) compactLocked(sessionID, requestID string) error {
	if err := e.ensureSessionDataLocked(sessionID); err != nil {
		return err
	}
	index := e.sessionIndex(sessionID)
	if index < 0 {
		return errors.New("select a conversation before compacting")
	}
	if e.runs[sessionID] != nil {
		return errors.New("stop the current run before compacting")
	}
	session := e.state.Sessions[index]
	provider, m, err := e.resolveModel(session.ModelID)
	if err != nil {
		return err
	}
	gateway, err := e.client.Bind(provider, m)
	if err != nil {
		return err
	}
	history := model.CloneMessages(e.context[sessionID])
	if len(history) < 6 {
		return errors.New("not enough history to compact")
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &activeRun{id: id("run"), cancel: cancel, answers: map[string]chan []contracts.QuestionAnswer{}, compacting: true, pending: map[string]contracts.QueuedMessage{}, wake: make(chan struct{}, 1), model: m}
	gateway.Observe = e.trajectoryRecorder(session, run.id, "compact")
	hookRules := contracts.CloneHookRules(e.state.Settings.Hooks)
	configSnapshot := clonePluginConfig(e.pluginConfig)
	profileSnapshot := contracts.CloneSubagentProfiles(e.state.Settings.Subagents)
	root := e.storage.ConfigDirectory()
	if e.sessionHookOnce == nil {
		e.sessionHookOnce = map[string]*agent.HookOnceState{}
	}
	if e.sessionHookOnce[sessionID] == nil {
		e.sessionHookOnce[sessionID] = &agent.HookOnceState{}
	}
	onceState := e.sessionHookOnce[sessionID]
	e.runs[sessionID] = run
	e.state.Sessions[index].Status = contracts.Working
	e.state.Runs = append(e.state.Runs, contracts.Run{ID: run.id, SessionID: sessionID, RequestID: requestID, Status: "running", StartedAt: time.Now()})
	if err = e.persistLocked(sessionID); err != nil {
		delete(e.runs, sessionID)
		cancel()
		return err
	}
	e.publishLocked(contracts.Event{Kind: "run.started", SessionID: sessionID, RunID: run.id, RequestID: requestID})
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer cancel()
		bound := meteredModel{inner: gateway, core: e, sessionID: session.ID, turnID: run.id, provider: provider, modelID: m.ID}
		resources, tools, cleanup, err := e.compactHookResources(ctx, session, root, configSnapshot, profileSnapshot)
		defer cleanup()
		tools, protocolWarnings := filterMCPToolsForProtocol(tools, provider.Protocol)
		resources.warnings = append(resources.warnings, protocolWarnings...)
		hookRules = append(hookRules, resources.hooks...)
		hooks, invoke := e.runHookRunner(session, run, hookRules, hookRuntimeOptions{Model: bound, Protocol: provider.Protocol, Tools: tools, Warnings: resources.warnings, EnvFile: resources.envFile, OnceState: onceState})
		var compacted []model.Message
		var startupContext []model.Message
		if err == nil {
			before, hookErr := invoke(ctx, agent.HookInput{Event: contracts.HookPreCompact, Trigger: "manual"})
			err = hookErr
			if err == nil && before.Deny {
				err = fmt.Errorf("PreCompact hook blocked compaction: %s", before.Reason)
			}
		}
		if err == nil {
			compacted, err = agent.Compact(ctx, hookRuntimeDiagnosticModel{inner: bound, warnings: resources.warnings}, model.Request{History: history})
		}
		if err == nil {
			summary := ""
			if len(compacted) > 0 {
				summary = compacted[0].Text
			}
			_, err = invoke(ctx, agent.HookInput{Event: contracts.HookPostCompact, Trigger: "manual", CompactSummary: summary})
		}
		if err == nil {
			started, hookErr := invoke(ctx, agent.HookInput{Event: contracts.HookSessionStart, Source: "compact"})
			err = hookErr
			if err == nil && started.Context != "" {
				startupContext = []model.Message{{Role: "user", Text: "<system-reminder>\n" + started.Context + "\n</system-reminder>"}}
			}
		}
		err = errors.Join(err, hooks.Wait())
		e.mu.Lock()
		defer e.mu.Unlock()
		if err == nil {
			e.context[sessionID] = append(model.CloneMessages(compacted), model.CloneMessages(startupContext)...)
			e.history[sessionID] = append(e.history[sessionID], model.CloneMessages(startupContext)...)
		}
		if index := e.sessionIndex(sessionID); index >= 0 {
			e.state.Sessions[index].Status = contracts.Idle
		}
		for i := range e.state.Runs {
			r := &e.state.Runs[i]
			if r.ID == run.id {
				now := time.Now()
				r.FinishedAt = &now
				r.DurationMS = now.Sub(r.StartedAt).Milliseconds()
				r.Status = "completed"
				if err != nil {
					r.Status = "failed"
					r.Error = err.Error()
					if errors.Is(err, context.Canceled) {
						r.Status = "interrupted"
					}
				}
			}
		}
		label := "Context compacted. Full execution history is retained."
		if e.state.Settings.General.Language == "zh-CN" {
			label = "上下文已压缩，完整执行历史仍保留。"
		}
		if err != nil {
			label = "Context compaction failed: " + err.Error()
			if e.state.Settings.General.Language == "zh-CN" {
				label = "上下文压缩未完成：" + err.Error()
			}
		}
		e.state.Messages[sessionID] = append(e.state.Messages[sessionID], contracts.Message{ID: id("message"), SessionID: sessionID, TurnID: run.id, Role: "assistant", CreatedAt: time.Now(), Blocks: []contracts.Block{{ID: id("block"), Kind: "text", Text: label}}})
		delete(e.runs, sessionID)
		if persistErr := e.persistLocked(sessionID); persistErr != nil {
			for i := range e.state.Runs {
				if e.state.Runs[i].ID == run.id {
					e.state.Runs[i].Status = "failed"
					e.state.Runs[i].Error = persistErr.Error()
				}
			}
		}
		e.publishLocked(contracts.Event{Kind: "run.finished", SessionID: sessionID, RunID: run.id, RequestID: requestID})
		if !e.closed && slices.ContainsFunc(e.state.Queue, func(message contracts.QueuedMessage) bool {
			return message.SessionID == sessionID && message.Source == "hook" && strings.HasPrefix(message.ID, "hook-rewake-")
		}) {
			if index := e.sessionIndex(sessionID); index >= 0 && !e.state.Sessions[index].Archived {
				_ = e.startLocked(e.state.Sessions[index], "")
			}
		}
	}()
	return nil
}
