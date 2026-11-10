package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

// HookInput is the Claude event payload. Internal routing identifiers never
// cross the hook boundary. Event-specific optional payloads stay typed here.
type HookInput struct {
	Event               contracts.HookEvent `json:"hook_event_name"`
	SessionID           string              `json:"session_id"`
	TranscriptPath      string              `json:"transcript_path"`
	CWD                 string              `json:"cwd"`
	PromptID            string              `json:"prompt_id,omitempty"`
	ScratchpadDir       string              `json:"scratchpad_dir,omitempty"`
	PermissionMode      string              `json:"permission_mode,omitempty"`
	Effort              *HookEffort         `json:"effort,omitempty"`
	AgentID             string              `json:"agent_id,omitempty"`
	SubagentType        string              `json:"agent_type,omitempty"`
	Source              string              `json:"source,omitempty"`
	Model               string              `json:"model,omitempty"`
	Prompt              string              `json:"prompt,omitempty"`
	ToolName            string              `json:"tool_name,omitempty"`
	ToolCallID          string              `json:"tool_use_id,omitempty"`
	ToolInput           json.RawMessage     `json:"tool_input,omitempty"`
	ToolOutput          any                 `json:"tool_response,omitempty"`
	ToolError           string              `json:"error,omitempty"`
	IsInterrupt         bool                `json:"is_interrupt,omitempty"`
	Response            string              `json:"last_assistant_message,omitempty"`
	StopHookActive      bool                `json:"stop_hook_active,omitempty"`
	AgentTranscriptPath string              `json:"agent_transcript_path,omitempty"`
	NotificationType    string              `json:"notification_type,omitempty"`
	Message             string              `json:"message,omitempty"`
	Title               string              `json:"title,omitempty"`
	Reason              string              `json:"reason,omitempty"`
	Trigger             string              `json:"trigger,omitempty"`
	Command             string              `json:"command,omitempty"`
	FromModel           string              `json:"from_model,omitempty"`
	ToModel             string              `json:"to_model,omitempty"`
	ErrorType           string              `json:"-"`
	ErrorDetails        string              `json:"error_details,omitempty"`
	TurnID              string              `json:"turn_id,omitempty"`
	MessageID           string              `json:"message_id,omitempty"`
	Index               int                 `json:"index,omitempty"`
	Final               bool                `json:"final,omitempty"`
	Delta               string              `json:"delta,omitempty"`
	ToolCalls           []HookToolCall      `json:"tool_calls,omitempty"`
	CustomInstructions  *string             `json:"custom_instructions,omitempty"`
	CompactSummary      string              `json:"compact_summary,omitempty"`
	MCPServerName       string              `json:"mcp_server_name,omitempty"`
	FilePath            string              `json:"file_path,omitempty"`
	LoadReason          string              `json:"load_reason,omitempty"`
	TaskID              string              `json:"task_id,omitempty"`
	TaskSubject         string              `json:"task_subject,omitempty"`
	TaskDescription     string              `json:"task_description,omitempty"`
	TeammateName        string              `json:"teammate_name,omitempty"`
	TeamName            string              `json:"team_name,omitempty"`
	RunID               string              `json:"-"`
	ParentSessionID     string              `json:"-"`
	UserInputID         string              `json:"-"`
	PermissionReason    string              `json:"-"`
	EnvFile             string              `json:"-"`
	Environment         map[string]string   `json:"-"`
}

type HookEffort struct {
	Level string `json:"level"`
}

type HookToolCall struct {
	ToolName     string          `json:"tool_name"`
	ToolInput    json.RawMessage `json:"tool_input"`
	ToolCallID   string          `json:"tool_use_id"`
	ToolResponse any             `json:"tool_response"`
}

func (input HookInput) MarshalJSON() ([]byte, error) {
	type wireInput HookInput
	value := wireInput(input)
	switch input.Event {
	case contracts.HookPermissionRequest:
		value.ToolCallID = ""
	case contracts.HookMessageDisplay:
		return json.Marshal(struct {
			wireInput
			Index int    `json:"index"`
			Final bool   `json:"final"`
			Delta string `json:"delta"`
		}{value, input.Index, input.Final, input.Delta})
	case contracts.HookPreCompact:
		return json.Marshal(struct {
			wireInput
			CustomInstructions *string `json:"custom_instructions"`
		}{value, input.CustomInstructions})
	case contracts.HookStopFailure:
		value.ToolError = input.ErrorType
	}
	return json.Marshal(value)
}

// HookResult is an in-process interpretation of Claude JSON, not a second wire
// protocol. Deny means event-specific block; Continue=false ends processing.
type HookResult struct {
	Context              string
	Deny                 bool
	Reason               string
	PermissionDecision   string
	UpdatedInput         json.RawMessage
	UpdatedToolOutput    json.RawMessage
	UpdatedMCPToolOutput json.RawMessage
	UpdatedPermissions   json.RawMessage
	Continue             *bool
	StopReason           string
	SystemMessage        string
	SuppressOutput       bool
	TerminalSequence     string
	InitialUserMessage   string
	SessionTitle         string
	WatchPaths           []string
	ReloadSkills         bool
	DisplayContent       *string
	Retry                bool
	WorktreePath         string
	Action               string
	Content              json.RawMessage
}

type HookExecution struct {
	Rule          contracts.HookRule
	Input         HookInput
	Result        HookResult
	Output        string
	Stderr        string
	ExitCode      int
	ExitCodeKnown bool
	Err           error
}

// HookEvaluation is the text result of an injected prompt/agent/MCP evaluator.
// Prompt/agent Output must be {"ok":bool,"reason":...}; MCP Output uses normal
// hook JSON. Error denotes execution failure, never a policy decision.
type HookEvaluation struct {
	Output        string
	Stderr        string
	ExitCode      int
	ExitCodeKnown bool
}

type HookEvaluator func(context.Context, contracts.HookRule, HookInput) (HookEvaluation, error)
type HookRunnerOption func(*HookRunner)

func WithHookEvaluator(evaluate HookEvaluator) HookRunnerOption {
	return func(h *HookRunner) { h.evaluate = evaluate }
}

func WithHookHTTPClient(client *http.Client) HookRunnerOption {
	return func(h *HookRunner) {
		if client != nil {
			h.http = client
		}
	}
}

// WithHookAsyncResult delivers notification-only results to the session owner.
func WithHookAsyncResult(deliver func(context.Context, HookExecution) error) HookRunnerOption {
	return func(h *HookRunner) { h.asyncResult = deliver }
}

func WithHookRewake(wake func(context.Context, HookExecution) error) HookRunnerOption {
	return func(h *HookRunner) { h.rewake = wake }
}

func WithHookPermissionRequest(ask func(context.Context, HookInput, string) (bool, error)) HookRunnerOption {
	return func(h *HookRunner) { h.ask = ask }
}

// HookOnceState can be retained by a session across separate run-owned runners.
// It contains rule IDs only; plugin configuration and credentials are absent.
type HookOnceState struct {
	mu        sync.Mutex
	completed map[string]bool
	running   map[string]bool
}

func WithHookOnceState(state *HookOnceState) HookRunnerOption {
	return func(h *HookRunner) {
		if state != nil {
			h.onceState = state
		}
	}
}

// HookRunner owns background children. The owner must cancel its context and
// call Wait at teardown. Report and injected callbacks must be concurrency safe.
type HookRunner struct {
	rules       []contracts.HookRule
	conditions  map[string]hookCondition
	report      func(HookExecution) error
	evaluate    HookEvaluator
	http        *http.Client
	asyncResult func(context.Context, HookExecution) error
	rewake      func(context.Context, HookExecution) error
	ask         func(context.Context, HookInput, string) (bool, error)
	wg          sync.WaitGroup
	mu          sync.Mutex
	err         error
	onceState   *HookOnceState
	llm         chan struct{}
}

func NewHookRunner(rules []contracts.HookRule, report func(HookExecution) error, options ...HookRunnerOption) *HookRunner {
	h := &HookRunner{rules: contracts.CloneHookRules(rules), report: report, http: &http.Client{},
		conditions: compileHookConditions(rules), onceState: &HookOnceState{}, llm: make(chan struct{}, 12)}
	for _, option := range options {
		if option != nil {
			option(h)
		}
	}
	return h
}

// AddRules registers session-owned rules. PushRules is preferable for a skill
// whose activation scope has a definite end.
func (h *HookRunner) AddRules(rules []contracts.HookRule) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	combined := append(contracts.CloneHookRules(h.rules), contracts.CloneHookRules(rules)...)
	if err := contracts.ValidateHookRules(combined); err != nil {
		return err
	}
	h.rules = combined
	h.conditions = compileHookConditions(combined)
	return nil
}

func (h *HookRunner) PushRules(rules []contracts.HookRule) (func(), error) {
	if err := h.AddRules(rules); err != nil {
		return nil, err
	}
	ids := make(map[string]bool, len(rules))
	for _, rule := range rules {
		ids[rule.ID] = true
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.rules = slices.DeleteFunc(h.rules, func(rule contracts.HookRule) bool { return ids[rule.ID] })
			h.conditions = compileHookConditions(h.rules)
		})
	}, nil
}

func (h *HookRunner) HasEvent(event contracts.HookEvent) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, rule := range h.rules {
		if rule.Enabled && rule.Event == event && contracts.HookTypeSupported(event, rule.Type) {
			return true
		}
	}
	return false
}

func (h *HookRunner) Run(ctx context.Context, input HookInput) (HookResult, error) {
	type outcome struct {
		result HookResult
		err    error
	}
	h.mu.Lock()
	rules := contracts.CloneHookRules(h.rules)
	conditions := h.conditions
	h.mu.Unlock()
	outcomes := make([]outcome, len(rules))
	var foreground sync.WaitGroup
	seenHandlers := make(map[string]bool)
	for index, rule := range rules {
		if !rule.Enabled || rule.Event != input.Event || !contracts.HookTypeSupported(rule.Event, rule.Type) ||
			!contracts.HookMatches(input.Event, rule.Matcher, hookMatchValue(input)) || !conditions[rule.If].matches(input) {
			continue
		}
		key := hookHandlerIdentity(rule, input)
		if seenHandlers[key] {
			continue
		}
		seenHandlers[key] = true
		if rule.Once && rule.Source == "skill" {
			state := h.onceState
			state.mu.Lock()
			if state.running == nil {
				state.running = map[string]bool{}
			}
			skip := state.completed[rule.ID] || state.running[rule.ID]
			if !skip {
				state.running[rule.ID] = true
			}
			state.mu.Unlock()
			if skip {
				continue
			}
		}
		if rule.Async || rule.AsyncRewake {
			h.wg.Add(1)
			go func() {
				defer h.wg.Done()
				execution, fatal := h.execute(ctx, rule, input)
				h.completeOnce(rule, execution)
				if h.asyncResult != nil && (execution.Result.Context != "" || execution.Result.SystemMessage != "") {
					fatal = errors.Join(fatal, h.asyncResult(ctx, execution))
				}
				if rule.AsyncRewake && execution.ExitCode == 2 && h.rewake != nil {
					fatal = errors.Join(fatal, h.rewake(ctx, execution))
				}
				h.mu.Lock()
				h.err = errors.Join(h.err, fatal)
				h.mu.Unlock()
			}()
			continue
		}
		foreground.Add(1)
		go func() {
			defer foreground.Done()
			execution, fatal := h.execute(ctx, rule, input)
			h.completeOnce(rule, execution)
			outcomes[index] = outcome{execution.Result, fatal}
		}()
	}
	foreground.Wait()
	result := HookResult{}
	var resultErr error
	// Stable aggregation avoids nondeterministic output and policy precedence
	// when several parallel hooks finish at different times.
	for _, item := range outcomes {
		result = mergeHookResults(result, item.result)
		resultErr = errors.Join(resultErr, item.err)
	}
	if input.Event == contracts.HookPermissionRequest && result.PermissionDecision != "allow" && !result.Deny {
		result.Reason = firstHookText(result.Reason, input.PermissionReason)
		if h.ask == nil {
			result.Deny = true
			result.Reason = firstHookText(result.Reason, "Hook requires a permission decision, but no permission handler is configured.")
		} else {
			allowed, err := h.ask(ctx, input, result.Reason)
			resultErr = errors.Join(resultErr, err)
			result.Deny = !allowed || err != nil
			if allowed && err == nil {
				result.PermissionDecision = "allow"
			}
		}
	}
	return result, resultErr
}

func hookHandlerIdentity(rule contracts.HookRule, input HookInput) string {
	// Display identity and matcher groups do not change handler execution.
	// Preserve resolved plugin/config context: identical relative scripts from
	// different plugins are different executable handlers.
	rule.ID, rule.Name, rule.Matcher, rule.Source, rule.If = "", "", "", "", ""
	rule.Command = expandHookPaths(rule.Command, rule, input, rule.Args != nil)
	data, _ := json.Marshal(rule)
	return string(data)
}

func (h *HookRunner) completeOnce(rule contracts.HookRule, execution HookExecution) {
	if !rule.Once || rule.Source != "skill" {
		return
	}
	state := h.onceState
	state.mu.Lock()
	delete(state.running, rule.ID)
	if execution.Err == nil && execution.ExitCode == 0 && !execution.Result.Deny {
		if state.completed == nil {
			state.completed = map[string]bool{}
		}
		state.completed[rule.ID] = true
	}
	state.mu.Unlock()
}

func (h *HookRunner) Wait() error {
	h.wg.Wait()
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.err
}

func hookMatchValue(input HookInput) string {
	switch input.Event {
	case contracts.HookSessionStart, contracts.HookSetup, contracts.HookConfigChange, contracts.HookDirectoryAdded:
		return input.Source
	case contracts.HookSessionEnd:
		return input.Reason
	case contracts.HookSubagentStart, contracts.HookSubagentStop:
		return input.SubagentType
	case contracts.HookPreCompact, contracts.HookPostCompact:
		return input.Trigger
	case contracts.HookNotification:
		return input.NotificationType
	case contracts.HookPreModelSwitch, contracts.HookPostModelSwitch:
		return input.ToModel
	case contracts.HookStopFailure:
		return firstHookText(input.ErrorType, input.ToolError)
	case contracts.HookInstructionsLoaded:
		return input.LoadReason
	case contracts.HookUserPromptExpansion:
		return input.Command
	case contracts.HookElicitation, contracts.HookElicitationResult:
		return input.MCPServerName
	case contracts.HookFileChanged:
		return input.FilePath
	default:
		return input.ToolName
	}
}

func mergeHookResults(result, next HookResult) HookResult {
	result.Context = strings.TrimSpace(result.Context + "\n" + next.Context)
	result.SystemMessage = strings.TrimSpace(result.SystemMessage + "\n" + next.SystemMessage)
	if next.Deny {
		result.Deny, result.Reason = true, next.Reason
	}
	rank := map[string]int{"": 0, "allow": 1, "ask": 2, "deny": 3}
	if rank[next.PermissionDecision] > rank[result.PermissionDecision] {
		result.PermissionDecision = next.PermissionDecision
		if next.Reason != "" {
			result.Reason = next.Reason
		}
	}
	if next.Continue != nil && (result.Continue == nil || !*next.Continue) {
		value := *next.Continue
		result.Continue, result.StopReason = &value, next.StopReason
	}
	if next.UpdatedInput != nil {
		result.UpdatedInput = slices.Clone(next.UpdatedInput)
	}
	if next.UpdatedToolOutput != nil {
		result.UpdatedToolOutput = slices.Clone(next.UpdatedToolOutput)
	}
	if next.UpdatedMCPToolOutput != nil {
		result.UpdatedMCPToolOutput = slices.Clone(next.UpdatedMCPToolOutput)
	}
	if next.UpdatedPermissions != nil {
		result.UpdatedPermissions = slices.Clone(next.UpdatedPermissions)
	}
	if next.InitialUserMessage != "" {
		result.InitialUserMessage = next.InitialUserMessage
	}
	if next.SessionTitle != "" {
		result.SessionTitle = next.SessionTitle
	}
	if next.WorktreePath != "" {
		result.WorktreePath = next.WorktreePath
	}
	if next.DisplayContent != nil {
		result.DisplayContent = next.DisplayContent
	}
	if next.Action != "" {
		result.Action, result.Content = next.Action, slices.Clone(next.Content)
	}
	if next.WatchPaths != nil {
		result.WatchPaths = slices.Clone(next.WatchPaths)
	}
	result.ReloadSkills = result.ReloadSkills || next.ReloadSkills
	result.SuppressOutput = result.SuppressOutput || next.SuppressOutput
	result.Retry = result.Retry || next.Retry
	if next.TerminalSequence != "" {
		result.TerminalSequence = next.TerminalSequence
	}
	return result
}

func firstHookText(texts ...string) string {
	for _, text := range texts {
		if strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func hookMessages(result HookResult) []model.Message {
	if result.Context == "" {
		return nil
	}
	return []model.Message{{Role: "user", Text: "<system-reminder>\n" + result.Context + "\n</system-reminder>"}}
}

func userPromptHook(message model.Message) HookInput {
	input := HookInput{Event: contracts.HookUserPromptSubmit, Prompt: message.Text}
	if message.SourceText != "" {
		input.Prompt = message.SourceText
	}
	if len(message.InputIDs) > 0 {
		input.UserInputID, input.PromptID = message.InputIDs[0], message.InputIDs[0]
	}
	return input
}

type hookedTool struct {
	Tool
	hook         func(context.Context, HookInput) (HookResult, error)
	callID       string
	inputUpdated func(json.RawMessage)
}

// ExecuteHookedTool applies the same tool permission and hook boundary to
// host-initiated tools, including a skill's dynamic shell context.
func ExecuteHookedTool(ctx context.Context, tool Tool, cwd string, raw json.RawMessage,
	hook func(context.Context, HookInput) (HookResult, error), callID string) (model.ToolOutput, error) {
	if hook == nil {
		return tool.Execute(ctx, cwd, raw)
	}
	return (hookedTool{Tool: tool, hook: hook, callID: callID}).Execute(ctx, cwd, raw)
}

func (t hookedTool) Execute(ctx context.Context, cwd string, raw json.RawMessage) (model.ToolOutput, error) {
	input := HookInput{ToolName: t.Definition().Name, ToolInput: slices.Clone(raw), ToolCallID: t.callID, CWD: cwd}
	var contexts []string
	needsPermission := false
	for _, event := range []contracts.HookEvent{contracts.HookPreToolUse, contracts.HookPermissionRequest} {
		if event == contracts.HookPermissionRequest && !needsPermission {
			continue
		}
		input.Event = event
		result, err := t.hook(ctx, input)
		if err != nil {
			return model.ToolOutput{}, err
		}
		if result.Continue != nil && !*result.Continue {
			return model.ToolOutput{}, HookStopError{Reason: result.StopReason}
		}
		if result.Context != "" {
			contexts = append(contexts, result.Context)
		}
		if result.Deny {
			return hookToolContextOutput(contexts), errors.New(firstHookText(result.Reason, "Hook blocked execution."))
		}
		if event == contracts.HookPermissionRequest && result.PermissionDecision != "allow" {
			return model.ToolOutput{}, errors.New(firstHookText(result.Reason, "Hook requires a permission decision."))
		}
		if result.PermissionDecision == "ask" {
			if event == contracts.HookPermissionRequest {
				return model.ToolOutput{}, errors.New(firstHookText(result.Reason, "Hook requires permission."))
			}
			needsPermission, input.PermissionReason = true, result.Reason
		}
		if result.UpdatedInput != nil {
			raw = slices.Clone(result.UpdatedInput)
			input.ToolInput = slices.Clone(raw)
			if t.inputUpdated != nil {
				t.inputUpdated(slices.Clone(raw))
			}
		}
	}
	output, toolErr := t.Tool.Execute(ctx, cwd, raw)
	input.Event, input.ToolOutput = contracts.HookPostToolUse, output.Text
	if output.Data != nil {
		input.ToolOutput = output.Data
	}
	if toolErr != nil {
		input.Event, input.ToolError = contracts.HookPostToolUseFailure, toolErr.Error()
	}
	result, err := t.hook(ctx, input)
	if result.Context != "" {
		contexts = append(contexts, result.Context)
	}
	if result.UpdatedToolOutput != nil {
		output = applyHookToolOutput(t.Definition().Name, output, result.UpdatedToolOutput)
	} else if result.UpdatedMCPToolOutput != nil && strings.HasPrefix(t.Definition().Name, "mcp__") {
		output = applyHookToolOutput(t.Definition().Name, output, result.UpdatedMCPToolOutput)
	}
	if result.Continue != nil && !*result.Continue {
		err = errors.Join(err, HookStopError{Reason: result.StopReason})
	}
	if result.Deny {
		err = errors.Join(err, errors.New(firstHookText(result.Reason, "Hook blocked execution.")))
	}
	if len(contexts) > 0 {
		output.HookContext = "\n<system-reminder>\n" + strings.Join(contexts, "\n") + "\n</system-reminder>"
		output.Text += output.HookContext
	}
	return output, errors.Join(toolErr, err)
}

func hookToolContextOutput(contexts []string) model.ToolOutput {
	if len(contexts) == 0 {
		return model.ToolOutput{}
	}
	return model.ToolOutput{Text: "<system-reminder>\n" + strings.Join(contexts, "\n") + "\n</system-reminder>"}
}

// HookStopError is a typed control signal, distinct from a tool failure or a
// Stop hook's request to continue working.
type HookStopError struct{ Reason string }

func (e HookStopError) Error() string { return firstHookText(e.Reason, "Processing stopped by hook.") }
