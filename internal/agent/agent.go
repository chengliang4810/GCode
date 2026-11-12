// Package agent provides an in-process Go agent loop following Pi's event,
// tool-result, steering and follow-up lifecycle. It has no UI or transport dependency.
package agent

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

type Tool interface {
	Definition() model.Tool
	Concurrent() bool
	Execute(context.Context, string, json.RawMessage) (model.ToolOutput, error)
}

// modelTool is implemented by tools that need the turn's model while they run.
// WebFetch uses it to condense a page around the caller's prompt instead of
// pasting the whole text into the next request. The method is unexported, so
// only this package's tools can claim the model.
type modelTool interface {
	withModel(Model) Tool
}

// Model is the Agent's only model dependency. Channel selection, authentication
// and wire protocols belong to the bound implementation.
type Model interface {
	Stream(context.Context, model.Request, func(model.Delta) error) (model.Result, error)
}

type Event struct {
	Kind        string
	Text        string
	Reasoning   string
	Call        model.Call
	DisplayName string
	Output      string
	// Data is the same typed result the tool returned, for hosts that project
	// structured fields. It is in-process only and never serialized here.
	Data  any
	Error bool
	Usage model.Usage
}

type Options struct {
	Model Model
	// ContextLimits reads the current model before each step. Unknown capacity
	// disables automatic compaction without inventing a token limit.
	ContextLimits func() ContextLimits
	Request       model.Request
	CWD           string
	Tools         []Tool
	ToolAllowed   func(string) bool
	// Record commits complete execution messages before the next effect.
	Record         func(model.Message) error
	ReplaceContext func([]model.Message) error
	// Memory is a worker-owned snapshot; nil keeps ordinary compaction.
	// ReplaceMemory atomically persists both the view and archive before use.
	Memory        *ContextMemory
	ReplaceMemory func([]model.Message, ContextMemory) error
	Emit          func(Event) error
	// Pending drains steering at step boundaries, and follow-ups only at idle.
	Pending func(idle bool) []model.Message
	// BeforeStep (idle after a final answer) and Idle control execution without
	// introducing application state or goal semantics into the Agent loop.
	BeforeStep func(context.Context, bool) (bool, error)
	Idle       func(context.Context, model.Request) ([]model.Message, error)
	// OwnsCompletion lets an owner such as a goal runtime make the sole idle
	// continuation decision, including its unfinished-todo policy.
	OwnsCompletion bool
	// Hook runs user-configured lifecycle policies outside the model protocol.
	Hook             func(context.Context, HookInput) (HookResult, error)
	HookSource       string
	SkipSessionStart bool
	// PrepareInput expands an accepted invocation after admission hooks, on
	// the worker, so dynamic context cannot run on the UI thread.
	PrepareInput        func(context.Context, model.Message) (model.Message, error)
	CommitPreparedInput func(model.Message) error
	// StoreToolOutput durably stores oversized text in the owning session.
	// The returned path must be readable by this host's Read tool.
	StoreToolOutput   func(context.Context, string) (string, error)
	prepareToolResult func(model.Call, model.Message) (model.Message, error)
}

func Run(ctx context.Context, options Options) (runErr error) {
	defer func() {
		var stopped HookStopError
		if errors.As(runErr, &stopped) {
			runErr = nil
		}
	}()
	request := options.Request
	if options.Memory != nil {
		if options.ReplaceMemory == nil {
			return errors.New("context memory requires durable replacement")
		}
		request.History = model.CloneMessages(request.History)
		AssignContextRefs(request.History)
		if err := options.ReplaceMemory(request.History, *options.Memory); err != nil {
			return err
		}
		request.System += contextMemoryInstructions
		commit := func(history []model.Message, state ContextMemory) error {
			if err := options.ReplaceMemory(history, state); err != nil {
				return err
			}
			request.History, *options.Memory = history, state
			request.ContextChange = "compacted"
			return nil
		}
		options.Tools = append(append([]Tool(nil), options.Tools...), contextMemoryTools(&request, options.Memory, commit)...)
	}
	turnID := hookUUID()
	request.Tools = nil
	tools := map[string]Tool{}
	for _, tool := range options.Tools {
		definition := tool.Definition()
		if tools[definition.Name] != nil {
			return fmt.Errorf("duplicate tool name %q", definition.Name)
		}
		tools[definition.Name] = tool
		request.Tools = append(request.Tools, definition)
	}
	refreshTools := func() {
		request.Tools = nil
		for _, tool := range options.Tools {
			if options.ToolAllowed == nil || options.ToolAllowed(tool.Definition().Name) {
				request.Tools = append(request.Tools, tool.Definition())
			}
		}
	}
	options.prepareToolResult = func(call model.Call, message model.Message) (model.Message, error) {
		return prepareToolResult(ctx, options.StoreToolOutput, request.History, call, message)
	}
	record := func(message model.Message) error {
		if options.Memory != nil && message.ContextRef == "" {
			message.ContextRef = "m_" + hookUUID()
		}
		if err := options.Record(message); err != nil {
			return err
		}
		request.History = append(request.History, message)
		return nil
	}
	applyHook := func(input HookInput) (HookResult, error) {
		if options.Hook == nil {
			return HookResult{}, nil
		}
		return options.Hook(ctx, input)
	}
	initialHooks := []HookInput{}
	if !options.SkipSessionStart {
		initialHooks = append(initialHooks, HookInput{Event: contracts.HookSessionStart, Source: options.HookSource})
	}
	initialUserIndex := -1
	if len(request.History) > 0 {
		last := request.History[len(request.History)-1]
		if last.UserInput {
			initialUserIndex = len(request.History) - 1
			initialHooks = append(initialHooks, userPromptHook(last))
		}
	}
	for _, input := range initialHooks {
		result, err := applyHook(input)
		if err != nil {
			return err
		}
		if result.Deny {
			return fmt.Errorf("%s hook blocked execution: %s", input.Event, result.Reason)
		}
		if result.Continue != nil && !*result.Continue {
			return nil
		}
		for _, message := range hookMessages(result) {
			if err := record(message); err != nil {
				return err
			}
		}
	}
	if options.PrepareInput != nil && initialUserIndex >= 0 {
		if request.History[initialUserIndex].UserInput {
			prepared, err := options.PrepareInput(ctx, request.History[initialUserIndex])
			if err != nil {
				return err
			}
			if options.Memory != nil && prepared.ContextRef == "" {
				prepared.ContextRef = "m_" + hookUUID()
			}
			request.History[initialUserIndex] = prepared
			if options.CommitPreparedInput != nil {
				if err := options.CommitPreparedInput(prepared); err != nil {
					return err
				}
			}
		}
	}
	recordPending := func(messages []model.Message) error {
		var admissionErr error
		for _, message := range messages {
			result := HookResult{}
			if message.UserInput {
				var err error
				result, err = applyHook(userPromptHook(message))
				if err != nil {
					admissionErr = errors.Join(admissionErr, err)
					continue
				}
				if result.Deny {
					admissionErr = errors.Join(admissionErr, fmt.Errorf("UserPromptSubmit hook blocked execution: %s", result.Reason))
					continue
				}
				if result.Continue != nil && !*result.Continue {
					return HookStopError{Reason: result.StopReason}
				}
				if options.PrepareInput != nil {
					message, err = options.PrepareInput(ctx, message)
					if err != nil {
						admissionErr = errors.Join(admissionErr, err)
						continue
					}
				}
				turnID = hookUUID()
			}
			if err := record(message); err != nil {
				return err
			}
			for _, context := range hookMessages(result) {
				if err := record(context); err != nil {
					return err
				}
			}
		}
		return admissionErr
	}
	stopContinuations := 0
	todoClosures := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if options.BeforeStep != nil {
			proceed, err := options.BeforeStep(ctx, false)
			if err != nil {
				return err
			}
			if !proceed {
				return nil
			}
		}
		if err := recordPending(options.Pending(false)); err != nil {
			return err
		}
		refreshTools()
		limits := ContextLimits{}
		if options.ContextLimits != nil {
			limits = options.ContextLimits()
		}
		if (options.ReplaceContext != nil || options.Memory != nil && options.ReplaceMemory != nil) && len(request.History) >= minimumCompactMessages &&
			limits.Window > 0 && contextPressure(request, limits) >= compactThreshold(limits) {
			before, err := applyHook(HookInput{Event: contracts.HookPreCompact, Trigger: "auto"})
			if err != nil {
				return err
			}
			if !before.Deny {
				if err := options.Emit(Event{Kind: "context.compacting"}); err != nil {
					return err
				}
				// Compaction summarizes the TodoWrite calls away; retain the
				// authoritative checklist state beside the summary.
				var retained *model.Message
				if _, hasTodos := tools["TodoWrite"]; hasTodos {
					if state := todoStateFromHistory(request.History); state.Current != nil {
						message := todoMessage(state, "retained")
						retained = &message
					}
				}
				var compacted []model.Message
				var nextMemory ContextMemory
				var err error
				if options.Memory != nil {
					// Preserve any observed excess over our text estimate
					// until a new real model response establishes a baseline.
					excess := max(0, contextPressure(request, limits)-contextTokens(request))
					compacted, nextMemory, err = CompactContext(ctx, options.Model, request, *options.Memory)
					// A single large segment may not reclaim enough space.
					// Continue folding before sending the normal model step.
					for rounds := 1; err == nil && excess+contextTokens(model.Request{System: request.System, Tools: request.Tools, History: compacted}) >= compactThreshold(limits); rounds++ {
						if rounds >= 32 {
							err = errors.New("context remains above budget after 32 folds; recent or protected content is too large")
							break
						}
						nextRequest := request
						nextRequest.History = compacted
						compacted, nextMemory, err = CompactContext(ctx, options.Model, nextRequest, nextMemory)
					}
				} else {
					compacted, err = Compact(ctx, options.Model, request)
				}
				if err != nil {
					return err
				}
				if retained != nil {
					if options.Memory != nil && retained.ContextRef == "" {
						retained.ContextRef = "m_" + hookUUID()
					}
					if err := options.Record(*retained); err != nil {
						return err
					}
					compacted = append(compacted, *retained)
				}
				if options.Memory != nil {
					err = options.ReplaceMemory(compacted, nextMemory)
				} else {
					err = options.ReplaceContext(compacted)
				}
				if err != nil {
					return err
				}
				if options.Memory != nil {
					*options.Memory = nextMemory
				}
				request.History = compacted
				request.ContextChange = "compacted"
				summary := ""
				if len(compacted) > 0 {
					summary = compacted[0].Text
				}
				if options.Memory != nil && len(nextMemory.Blocks) > 0 {
					summary = nextMemory.Blocks[len(nextMemory.Blocks)-1].Summary
				}
				_, err = applyHook(HookInput{Event: contracts.HookPostCompact, Trigger: "auto", CompactSummary: summary})
				if err != nil {
					return err
				}
				started, err := applyHook(HookInput{Event: contracts.HookSessionStart, Source: "compact"})
				if err != nil {
					return err
				}
				if started.Continue != nil && !*started.Continue {
					return nil
				}
				for _, message := range hookMessages(started) {
					if err := record(message); err != nil {
						return err
					}
				}
			}
		}
		if request.ContextChange == "" && needsCompactionBaseline(request.History) {
			request.ContextChange = "compacted"
		}
		if options.Memory != nil && estimateMessages(request.History) >= max(50000, options.Memory.NextNudge) {
			message := model.Message{Role: "user", Text: "<system-reminder>Context has grown by about 50K tokens. Use ContextStatus and CompressContext to summarize completed work while retaining constraints and evidence. Recover exact details with SearchContext and DecompressContext when needed.</system-reminder>"}
			if err := record(message); err != nil {
				return err
			}
			next := *options.Memory
			next.NextNudge = estimateMessages(request.History) + 50000
			if err := options.ReplaceMemory(request.History, next); err != nil {
				return err
			}
			*options.Memory = next
		}
		// The checklist lives in the history, not in a separate store: rebuild
		// it each round so reminders and the stop check track real progress.
		if _, hasTodos := tools["TodoWrite"]; hasTodos {
			if state := todoStateFromHistory(request.History); state.reminderDue() {
				if err := record(todoMessage(state, "reminder")); err != nil {
					return err
				}
			}
		}
		if err := options.Emit(Event{Kind: "message.start"}); err != nil {
			return err
		}
		result, err := streamResponse(ctx, options, request)
		if err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				kind := modelFailureHookKind(err)
				_, hookErr := applyHook(HookInput{Event: contracts.HookStopFailure, ToolError: kind, ErrorType: kind, ErrorDetails: err.Error(), Response: err.Error()})
				err = errors.Join(err, hookErr)
			}
			return err
		}
		result.Message.ContextUsage = &model.ContextUsage{
			Known: result.Usage.Known, InputTokens: result.Usage.Input, OutputTokens: result.Usage.Output,
			HistoryLength: len(request.History), ModelKey: limits.ModelKey, PromptHash: promptHash(request),
		}
		if err := record(result.Message); err != nil {
			return err
		}
		request.ContextChange = ""
		if err := options.Emit(Event{Kind: "message.end", Usage: result.Usage}); err != nil {
			return err
		}
		if result.Message.Text != "" {
			display, err := applyHook(HookInput{Event: contracts.HookMessageDisplay, TurnID: turnID, MessageID: hookUUID(), Index: 0, Final: true, Delta: result.Message.Text})
			if err != nil {
				return err
			}
			if display.DisplayContent != nil {
				if err := options.Emit(Event{Kind: "message.display", Text: *display.DisplayContent}); err != nil {
					return err
				}
			}
		}
		if len(result.Message.Calls) == 0 {
			if options.BeforeStep != nil {
				proceed, err := options.BeforeStep(ctx, true)
				if err != nil {
					return err
				}
				if !proceed {
					return nil
				}
			}
			pending := options.Pending(true)
			if len(pending) > 0 {
				if err := recordPending(pending); err != nil {
					return err
				}
				continue
			}
			stop, hookErr := applyHook(HookInput{
				Event: contracts.HookStop, Response: result.Message.Text, StopHookActive: stopContinuations > 0,
			})
			if hookErr != nil {
				return hookErr
			}
			shouldContinue := stop.Deny || stop.Context != ""
			if shouldContinue {
				stopContinuations++
				if stopContinuations > 8 {
					return errors.New("Stop hooks requested more than eight consecutive continuations")
				}
				if stop.Deny {
					stop.Context = firstHookText(stop.Context, stop.Reason, "Continue the task before stopping.")
				}
			} else {
				stopContinuations = 0
			}
			if stop.Continue != nil && !*stop.Continue {
				return nil
			}
			pending = hookMessages(stop)
			if len(pending) > 0 {
				for _, message := range pending {
					if err := record(message); err != nil {
						return err
					}
				}
				pending = nil
			}
			if shouldContinue {
				continue
			}
			if len(pending) == 0 && options.Idle != nil {
				var err error
				pending, err = options.Idle(ctx, request)
				if err != nil {
					return err
				}
			}
			if len(pending) == 0 {
				// A stop with unfinished todos and no resolution continues the
				// run once more: finish the work or record a resolution. After
				// the bound, a model that keeps ignoring the reminder stops.
				if _, hasTodos := tools["TodoWrite"]; !options.OwnsCompletion && hasTodos && todoClosures < todoClosureLimit {
					if state := todoStateFromHistory(request.History); state.unfinished() && state.Current.Resolution == nil {
						if err := record(todoMessage(state, "closure")); err != nil {
							return err
						}
						todoClosures++
						continue
					}
				}
				return nil
			}
			if err := recordPending(pending); err != nil {
				return err
			}
			continue
		}
		if err := executeTools(ctx, options, tools, result.Message.Calls, record); err != nil {
			return err
		}
		todoClosures = 0
	}
}

func modelFailureHookKind(err error) string {
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "429"), strings.Contains(text, "rate limit"):
		return "rate_limit"
	case strings.Contains(text, "503"), strings.Contains(text, "overloaded"):
		return "overloaded"
	case strings.Contains(text, "401"), strings.Contains(text, "authentication"):
		return "authentication_failed"
	case strings.Contains(text, "billing"):
		return "billing_error"
	case strings.Contains(text, "model not found"), strings.Contains(text, "model_not_found"):
		return "model_not_found"
	case strings.Contains(text, "400"), strings.Contains(text, "invalid request"):
		return "invalid_request"
	case strings.Contains(text, "500"), strings.Contains(text, "502"), strings.Contains(text, "504"):
		return "server_error"
	default:
		return "unknown"
	}
}

func hookUUID() string {
	var value [16]byte
	_, _ = rand.Read(value[:])
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[:4], value[4:6], value[6:8], value[8:10], value[10:])
}

// Retry only the incomplete model step. No assistant tool call from this step
// is committed or executed until a complete response has been validated.
func streamResponse(ctx context.Context, options Options, request model.Request) (model.Result, error) {
	return options.Model.Stream(ctx, request, func(delta model.Delta) error {
		if delta.Retry != nil {
			return options.Emit(Event{Kind: "message.retry"})
		}
		return options.Emit(Event{Kind: "message.delta", Text: delta.Text, Reasoning: delta.Reasoning})
	})
}
