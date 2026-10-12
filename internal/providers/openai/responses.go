package openai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"gcode/internal/providers/providerio"
	"gcode/internal/zeroruntime"
)

const (
	responsesEventCreated           = "response.created"
	responsesEventInProgress        = "response.in_progress"
	responsesEventOutputItemAdded   = "response.output_item.added"
	responsesEventContentPartAdded  = "response.content_part.added"
	responsesEventOutputTextDelta   = "response.output_text.delta"
	responsesEventReasoningDelta    = "response.reasoning_summary_text.delta"
	responsesEventOutputTextDone    = "response.output_text.done"
	responsesEventFunctionArgsDelta = "response.function_call_arguments.delta"
	responsesEventCustomInputDelta  = "response.custom_tool_call_input.delta"
	responsesEventContentPartDone   = "response.content_part.done"
	responsesEventOutputItemDone    = "response.output_item.done"
	responsesEventCompleted         = "response.completed"
	responsesEventFailed            = "response.failed"
	responsesEventError             = "response.error"
	responsesEventIncomplete        = "response.incomplete"
)

const responsesProviderName = "openai-responses"

const applyPatchLarkGrammar = `start: begin_patch hunk+ end_patch
begin_patch: "*** Begin Patch" LF
end_patch: "*** End Patch" LF?
hunk: add_hunk | delete_hunk | update_hunk
add_hunk: "*** Add File: " filename LF add_line+
delete_hunk: "*** Delete File: " filename LF
update_hunk: "*** Update File: " filename LF change_move? change?
filename: /(.+)/
add_line: "+" /(.*)/ LF -> line
change_move: "*** Move to: " filename LF
change: (change_context | change_line)+ eof_line?
change_context: ("@@" | "@@ " /(.+)/) LF
change_line: ("+" | "-" | " ") /(.*)/ LF
eof_line: "*** End of File" LF
%import common.LF`

type responsesRequest struct {
	Type               string              `json:"type,omitempty"`
	Model              string              `json:"model"`
	Instructions       string              `json:"instructions"`
	PreviousResponseID string              `json:"previous_response_id,omitempty"`
	Input              []inputItem         `json:"input"`
	Stream             bool                `json:"stream"`
	Store              bool                `json:"store"`
	ParallelToolCalls  bool                `json:"parallel_tool_calls"`
	MaxOutputTokens    int                 `json:"max_output_tokens,omitempty"`
	Tools              []responsesTool     `json:"tools,omitempty"`
	Reasoning          *responsesReasoning `json:"reasoning,omitempty"`
	ServiceTier        string              `json:"service_tier,omitempty"`
	PromptCacheKey     string              `json:"prompt_cache_key,omitempty"`
}

type responsesReasoning struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type inputItem struct {
	Type      string          `json:"type"`
	Role      string          `json:"role,omitempty"`
	Content   []contentItem   `json:"content,omitempty"`
	ID        string          `json:"id,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments string          `json:"arguments,omitempty"`
	Input     string          `json:"input,omitempty"`
	Output    any             `json:"output,omitempty"`
	Status    string          `json:"status,omitempty"`
	Raw       json.RawMessage `json:"-"`
}

// MarshalJSON preserves the complete reasoning item returned by Responses.
// Reasoning items contain provider-owned fields such as encrypted_content and
// summary that must be sent back byte-for-byte when store:false is used.
func (item inputItem) MarshalJSON() ([]byte, error) {
	if item.Type == "reasoning" && len(item.Raw) > 0 {
		return append([]byte(nil), item.Raw...), nil
	}
	type alias inputItem
	return json.Marshal(alias(item))
}

type contentItem struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

type responsesTool struct {
	Type        string                            `json:"type"`
	Name        string                            `json:"name"`
	Description string                            `json:"description,omitempty"`
	Parameters  map[string]any                    `json:"parameters,omitempty"`
	Format      *zeroruntime.ToolDefinitionFormat `json:"format,omitempty"`
}

type responsesEvent struct {
	Type        string           `json:"type"`
	Delta       string           `json:"delta,omitempty"`
	ItemID      string           `json:"item_id,omitempty"`
	CallID      string           `json:"call_id,omitempty"`
	OutputIndex *int             `json:"output_index,omitempty"`
	Item        *itemPayload     `json:"item,omitempty"`
	Response    *responsePayload `json:"response,omitempty"`
	Code        string           `json:"code,omitempty"`
	Message     string           `json:"message,omitempty"`
	Param       string           `json:"param,omitempty"`
}

type itemPayload struct {
	Type             string          `json:"type"`
	ID               string          `json:"id,omitempty"`
	CallID           string          `json:"call_id,omitempty"`
	Name             string          `json:"name,omitempty"`
	Arguments        string          `json:"arguments,omitempty"`
	Input            string          `json:"input,omitempty"`
	Status           string          `json:"status,omitempty"`
	Summary          json.RawMessage `json:"summary,omitempty"`
	Content          json.RawMessage `json:"content,omitempty"`
	EncryptedContent string          `json:"encrypted_content,omitempty"`
	Raw              json.RawMessage `json:"-"`
}

func (item *itemPayload) UnmarshalJSON(data []byte) error {
	type alias itemPayload
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*item = itemPayload(decoded)
	item.Raw = append([]byte(nil), data...)
	return nil
}

type responsePayload struct {
	ID                string             `json:"id"`
	Status            string             `json:"status"`
	Usage             *usagePayload      `json:"usage,omitempty"`
	Error             *errorPayload      `json:"error,omitempty"`
	IncompleteDetails *incompleteDetails `json:"incomplete_details,omitempty"`
}

type incompleteDetails struct {
	Reason string `json:"reason,omitempty"`
}

type usagePayload struct {
	InputTokens         int            `json:"input_tokens"`
	OutputTokens        int            `json:"output_tokens"`
	TotalTokens         int            `json:"total_tokens"`
	OutputTokensDetails *outputDetails `json:"output_tokens_details,omitempty"`
	InputTokensDetails  *inputDetails  `json:"input_tokens_details,omitempty"`
}

type outputDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type inputDetails struct {
	CachedTokens     int `json:"cached_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
}

type errorPayload struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type toolCallBuilder struct {
	ID        string
	CallID    string
	Name      string
	Arguments strings.Builder
	Freeform  bool
	started   bool
	ended     bool
}

type responsesState struct {
	toolCalls       map[string]*toolCallBuilder
	toolOrder       []string
	text            strings.Builder
	reasoningBlocks []zeroruntime.ReasoningBlock
	responseID      string
	finishReason    string
	usage           *usagePayload
	emitted         bool
	done            bool
}

type ResponsesProvider struct {
	inner *Provider
}

func newResponsesState() *responsesState {
	return &responsesState{toolCalls: map[string]*toolCallBuilder{}}
}

func (state *responsesState) outputInputItems() []inputItem {
	items := make([]inputItem, 0, len(state.toolOrder)+len(state.reasoningBlocks)+1)
	for _, block := range state.reasoningBlocks {
		if block.Provider != responsesProviderName || block.Type != "reasoning" || strings.TrimSpace(block.Data) == "" {
			continue
		}
		items = append(items, inputItem{Type: "reasoning", Raw: json.RawMessage(block.Data)})
	}
	if state.text.Len() > 0 {
		items = append(items, inputItem{
			Type:    "message",
			Role:    "assistant",
			Content: []contentItem{{Type: "output_text", Text: state.text.String()}},
		})
	}
	for _, key := range state.toolOrder {
		builder := state.toolCalls[key]
		if builder == nil || builder.ID == "" || builder.Name == "" {
			continue
		}
		callID := builder.CallID
		if callID == "" {
			callID = builder.ID
		}
		itemType := "function_call"
		if builder.Freeform {
			itemType = "custom_tool_call"
		}
		item := inputItem{
			Type:   itemType,
			ID:     builder.ID,
			CallID: callID,
			Name:   builder.Name,
		}
		if builder.Freeform {
			item.Input = builder.Arguments.String()
		} else {
			item.Arguments = builder.Arguments.String()
		}
		items = append(items, item)
	}
	return items
}

func NewResponsesProvider(options Options) (*ResponsesProvider, error) {
	inner, err := New(options)
	if err != nil {
		return nil, err
	}
	inner.endpoint = inner.baseURL + "/responses"
	return &ResponsesProvider{inner: inner}, nil
}

func (p *ResponsesProvider) StreamCompletion(ctx context.Context, request zeroruntime.CompletionRequest) (<-chan zeroruntime.StreamEvent, error) {
	responsesReq, err := p.buildResponsesRequest(request)
	if err != nil {
		return nil, fmt.Errorf("encode responses request: %w", err)
	}
	body, err := json.Marshal(responsesReq)
	if err != nil {
		return nil, fmt.Errorf("encode responses request: %w", err)
	}
	events := make(chan zeroruntime.StreamEvent, 16)
	go func() {
		defer close(events)
		p.streamResponses(ctx, body, events)
	}()
	return events, nil
}

func (p *ResponsesProvider) buildResponsesRequest(request zeroruntime.CompletionRequest) (*responsesRequest, error) {
	if p.inner == nil || strings.TrimSpace(p.inner.model) == "" {
		return nil, errors.New("responses provider: model is required")
	}
	req := &responsesRequest{
		Model:             p.inner.model,
		Stream:            true,
		ParallelToolCalls: true,
		MaxOutputTokens:   p.inner.maxTokens,
		PromptCacheKey:    request.PromptCacheKey,
	}
	instructions := []string{}
	customCallIDs := map[string]string{}
	for _, msg := range request.Messages {
		switch msg.Role {
		case zeroruntime.MessageRoleSystem:
			if content := strings.TrimSpace(msg.Content); content != "" {
				instructions = append(instructions, content)
			}
		case zeroruntime.MessageRoleUser:
			req.Input = append(req.Input, p.userInputItem(msg))
		case zeroruntime.MessageRoleAssistant:
			items, err := p.assistantInputItems(msg)
			if err != nil {
				return nil, err
			}
			req.Input = append(req.Input, items...)
			for _, call := range msg.ToolCalls {
				if call.Freeform && call.ID != "" {
					callID := call.ProviderCallID
					if callID == "" {
						callID = call.ID
					}
					customCallIDs[call.ID] = callID
				}
			}
		case zeroruntime.MessageRoleTool:
			if msg.ToolCallID == "" {
				return nil, fmt.Errorf("responses provider: tool message missing tool call id")
			}
			outputType := "function_call_output"
			callID := msg.ToolCallProviderID
			if callID == "" {
				callID = msg.ToolCallID
			}
			if customID, custom := customCallIDs[msg.ToolCallID]; custom {
				outputType = "custom_tool_call_output"
				callID = customID
			}
			req.Input = append(req.Input, inputItem{
				Type:   outputType,
				CallID: callID,
				Output: msg.Content,
			})
		default:
			return nil, fmt.Errorf("responses provider: unsupported message role %q", msg.Role)
		}
	}
	req.Instructions = strings.Join(instructions, "\n\n")
	for _, tool := range request.Tools {
		definition := tool
		if definition.Name == "apply_patch" && definition.Type == zeroruntime.ToolDefinitionFreeform {
			definition.Description = "The apply_patch tool edits files from a raw structured patch. Use absolute file paths in patch headers to target a granted extra write root. Do not wrap the patch in JSON."
			definition.Format = &zeroruntime.ToolDefinitionFormat{Type: "grammar", Syntax: "lark", Definition: applyPatchLarkGrammar}
		}
		wireTool := responsesTool{
			Type:        string(definition.Type),
			Name:        definition.Name,
			Description: definition.Description,
			Parameters:  definition.Parameters,
			Format:      definition.Format,
		}
		if wireTool.Type == "" {
			wireTool.Type = string(zeroruntime.ToolDefinitionFunction)
		}
		if wireTool.Type == string(zeroruntime.ToolDefinitionFreeform) {
			wireTool.Parameters = nil
		}
		req.Tools = append(req.Tools, wireTool)
	}
	if effort := openAIReasoningEffort(request.ReasoningEffort); effort != "" {
		req.Reasoning = &responsesReasoning{Effort: effort, Summary: "auto"}
	}
	if tier := openAIServiceTier(request.ServiceTier); tier != "" {
		req.ServiceTier = tier
	}
	return req, nil
}

func (p *ResponsesProvider) userInputItem(msg zeroruntime.Message) inputItem {
	parts := []contentItem{}
	if msg.Content != "" {
		parts = append(parts, contentItem{Type: "input_text", Text: msg.Content})
	}
	for _, image := range msg.Images {
		encoded := base64.StdEncoding.EncodeToString(image.Data)
		parts = append(parts, contentItem{
			Type:     "input_image",
			ImageURL: "data:" + image.MediaType + ";base64," + encoded,
		})
	}
	return inputItem{Type: "message", Role: "user", Content: parts}
}

func (p *ResponsesProvider) assistantInputItems(msg zeroruntime.Message) ([]inputItem, error) {
	items := make([]inputItem, 0, len(msg.Reasoning)+len(msg.ToolCalls)+1)
	for _, block := range msg.Reasoning {
		if block.Provider != responsesProviderName || block.Type != "reasoning" {
			continue
		}
		data := strings.TrimSpace(block.Data)
		if data == "" {
			return nil, errors.New("responses provider: reasoning block is missing its raw item")
		}
		var item itemPayload
		if err := json.Unmarshal([]byte(data), &item); err != nil {
			return nil, fmt.Errorf("responses provider: invalid reasoning item: %w", err)
		}
		if item.Type != "reasoning" {
			return nil, fmt.Errorf("responses provider: reasoning block has item type %q", item.Type)
		}
		items = append(items, inputItem{Type: "reasoning", Raw: json.RawMessage(data)})
	}
	if msg.Content != "" {
		items = append(items, inputItem{
			Type:    "message",
			Role:    "assistant",
			Content: []contentItem{{Type: "output_text", Text: msg.Content}},
		})
	}
	for _, tc := range msg.ToolCalls {
		if tc.ID == "" {
			return nil, errors.New("responses provider: assistant tool call missing id")
		}
		if tc.Name == "" {
			return nil, errors.New("responses provider: assistant tool call missing name")
		}
		callID := tc.ProviderCallID
		if callID == "" {
			callID = tc.ID
		}
		itemType := "function_call"
		if tc.Freeform {
			itemType = "custom_tool_call"
		}
		item := inputItem{
			Type:   itemType,
			ID:     tc.ID,
			CallID: callID,
			Name:   tc.Name,
		}
		if tc.Freeform {
			item.Input = tc.Arguments
		} else {
			item.Arguments = tc.Arguments
		}
		items = append(items, item)
	}
	return items, nil
}

func (p *ResponsesProvider) streamResponses(
	ctx context.Context,
	body []byte,
	events chan<- zeroruntime.StreamEvent,
) {
	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()

	inner := p.inner
	response, err := providerio.SendWithRetry(streamCtx, inner.httpClient, http.MethodPost, inner.endpoint, body,
		func(request *http.Request) {
			request.Header.Set("Content-Type", "application/json")
			if inner.userAgent != "" {
				request.Header.Set("User-Agent", inner.userAgent)
			}
			providerio.ApplyAuthHeaders(request, providerio.AuthHeaders{
				APIKey:            inner.apiKey,
				DefaultAuthHeader: "Authorization",
				DefaultAuthScheme: "Bearer",
				AuthHeader:        inner.authHeader,
				AuthScheme:        inner.authScheme,
				AuthHeaderValue:   inner.authHeaderValue,
				CustomHeaders:     inner.customHeaders,
			})
		}, 0)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
				Type:  zeroruntime.StreamEventError,
				Error: p.redact("provider stream error: " + ctxErr.Error()),
			})
			return
		}
		if humanized, ok := providerio.UpstreamUnreachable(err.Error()); ok {
			providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
				Type:  zeroruntime.StreamEventError,
				Error: p.redact(humanized),
			})
			return
		}
		providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
			Type:  zeroruntime.StreamEventError,
			Error: p.redact("provider stream error: " + err.Error()),
		})
		return
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		p.emitResponsesHTTPError(ctx, response, events)
		return
	}

	state := newResponsesState()
	err = providerio.ScanSSEDataWithContext(streamCtx, cancelStream, response.Body, inner.streamIdleTimeout, func(data string) bool {
		return p.emitResponsesEvent(ctx, data, state, events)
	})
	if errors.Is(err, providerio.ErrStreamIdle) || errors.Is(err, providerio.ErrStreamStalled) {
		providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
			Type:  zeroruntime.StreamEventError,
			Error: p.redact("provider stream error: " + providerio.StreamTimeoutMessage(err, inner.streamIdleTimeout)),
		})
		return
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
			Type:  zeroruntime.StreamEventError,
			Error: p.redact("provider stream error: " + err.Error()),
		})
		return
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
			Type:  zeroruntime.StreamEventError,
			Error: p.redact("provider stream error: " + ctxErr.Error()),
		})
		return
	}
	if !state.done {
		providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
			Type:            zeroruntime.StreamEventDone,
			ResponseID:      state.responseID,
			FinishReason:    zeroruntime.FinishReasonLength,
			ReasoningBlocks: state.reasoningBlocks,
		})
	}
}

func (p *ResponsesProvider) emitResponsesHTTPError(
	ctx context.Context,
	response *http.Response,
	events chan<- zeroruntime.StreamEvent,
) {
	body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	message := strings.TrimSpace(string(body))
	var chatError struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &chatError); err == nil && chatError.Error.Message != "" {
		message = chatError.Error.Message
	}
	var responsesError responsesEvent
	if err := json.Unmarshal(body, &responsesError); err == nil && responsesError.Message != "" {
		message = responsesError.Message
	}
	if message == "" {
		message = response.Status
	}
	if humanized, ok := providerio.UpstreamUnreachable(message); ok {
		providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
			Type:  zeroruntime.StreamEventError,
			Error: p.redact(humanized),
		})
		return
	}
	providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
		Type:  zeroruntime.StreamEventError,
		Error: p.classifiedError(response.StatusCode, message),
	})
}

func (p *ResponsesProvider) emitResponsesEvent(
	ctx context.Context,
	data string,
	state *responsesState,
	events chan<- zeroruntime.StreamEvent,
) bool {
	var event responsesEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
			Type:  zeroruntime.StreamEventError,
			Error: p.redact("provider stream error: malformed Responses event: " + err.Error()),
		})
		state.done = true
		return false
	}
	return p.emitParsedResponsesEvent(ctx, &event, state, events)
}

func (p *ResponsesProvider) emitParsedResponsesEvent(
	ctx context.Context,
	event *responsesEvent,
	state *responsesState,
	events chan<- zeroruntime.StreamEvent,
) bool {
	if event.Type == "" {
		providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
			Type:  zeroruntime.StreamEventError,
			Error: p.redact("provider stream error: Responses event missing required `type` field"),
		})
		state.done = true
		return false
	}
	switch event.Type {
	case responsesEventCreated, responsesEventInProgress,
		responsesEventContentPartAdded, responsesEventContentPartDone,
		responsesEventOutputTextDone:
		if event.Response != nil && event.Response.ID != "" {
			state.responseID = event.Response.ID
		}
		return true
	case responsesEventOutputItemAdded:
		p.handleOutputItemAdded(ctx, event, state, events)
		return true
	case responsesEventOutputTextDelta:
		if event.Delta != "" {
			state.emitted = true
			state.text.WriteString(event.Delta)
			providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
				Type:    zeroruntime.StreamEventText,
				Content: event.Delta,
			})
		}
		return true
	case responsesEventReasoningDelta:
		if event.Delta != "" {
			state.emitted = true
			providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
				Type:    zeroruntime.StreamEventReasoning,
				Content: event.Delta,
			})
		}
		return true
	case responsesEventFunctionArgsDelta:
		p.handleToolInputDelta(ctx, event, state, events, false)
		return true
	case responsesEventCustomInputDelta:
		p.handleToolInputDelta(ctx, event, state, events, true)
		return true
	case responsesEventOutputItemDone:
		p.handleOutputItemDone(ctx, event, state, events)
		return true
	case responsesEventCompleted, responsesEventFailed, responsesEventIncomplete:
		return p.handleTerminalResponse(ctx, event, state, events)
	case responsesEventError:
		message := event.Message
		if event.Code != "" {
			message = strings.TrimSpace(event.Code + " " + message)
		}
		if message == "" {
			message = "provider error"
		}
		providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
			Type:  zeroruntime.StreamEventError,
			Error: p.redact(message),
		})
		state.done = true
		return false
	default:
		return true
	}
}

func (p *ResponsesProvider) handleOutputItemAdded(
	ctx context.Context,
	event *responsesEvent,
	state *responsesState,
	events chan<- zeroruntime.StreamEvent,
) {
	if event.Item == nil {
		return
	}
	switch event.Item.Type {
	case "function_call", "custom_tool_call":
		key := p.toolCallKey(event)
		if key == "" {
			return
		}
		builder, exists := state.toolCalls[key]
		if !exists {
			builder = &toolCallBuilder{ID: key, Freeform: event.Item.Type == "custom_tool_call"}
			state.toolCalls[key] = builder
			state.toolOrder = append(state.toolOrder, key)
		}
		if event.Item.CallID != "" {
			builder.CallID = event.Item.CallID
		}
		if event.Item.Name != "" {
			builder.Name = event.Item.Name
		}
		builder.Freeform = builder.Freeform || event.Item.Type == "custom_tool_call"
		if builder.Freeform && event.Item.Input != "" && builder.Arguments.Len() == 0 {
			builder.Arguments.WriteString(event.Item.Input)
		}
		state.emitted = true
		if !builder.started {
			providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
				Type:               zeroruntime.StreamEventToolCallStart,
				ToolCallID:         key,
				ToolCallProviderID: builder.CallID,
				ToolName:           builder.Name,
				ToolCallFreeform:   builder.Freeform,
			})
			builder.started = true
		}
	}
}

func (p *ResponsesProvider) handleToolInputDelta(
	ctx context.Context,
	event *responsesEvent,
	state *responsesState,
	events chan<- zeroruntime.StreamEvent,
	freeform bool,
) {
	key := p.toolCallKey(event)
	if key == "" {
		return
	}
	builder, ok := state.toolCalls[key]
	if !ok {
		builder = &toolCallBuilder{ID: key, Freeform: freeform}
		state.toolCalls[key] = builder
		state.toolOrder = append(state.toolOrder, key)
	}
	builder.Freeform = builder.Freeform || freeform
	if event.CallID != "" && builder.CallID == "" {
		builder.CallID = event.CallID
	}
	builder.Arguments.WriteString(event.Delta)
	state.emitted = true
	providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
		Type:              zeroruntime.StreamEventToolCallDelta,
		ToolCallID:        key,
		ArgumentsFragment: event.Delta,
	})
}

func (p *ResponsesProvider) handleOutputItemDone(
	ctx context.Context,
	event *responsesEvent,
	state *responsesState,
	events chan<- zeroruntime.StreamEvent,
) {
	if event.Item == nil {
		return
	}
	if event.Item.Type == "reasoning" {
		data := event.Item.Raw
		if len(data) == 0 {
			data, _ = json.Marshal(event.Item)
		}
		if len(data) > 0 {
			state.reasoningBlocks = append(state.reasoningBlocks, zeroruntime.ReasoningBlock{
				Provider: responsesProviderName,
				Type:     "reasoning",
				Data:     string(data),
			})
		}
		return
	}
	if event.Item.Type != "function_call" && event.Item.Type != "custom_tool_call" {
		return
	}
	key := p.toolCallKey(event)
	if key == "" {
		return
	}
	builder, ok := state.toolCalls[key]
	if !ok {
		builder = &toolCallBuilder{ID: key, Freeform: event.Item.Type == "custom_tool_call"}
		state.toolCalls[key] = builder
		state.toolOrder = append(state.toolOrder, key)
	}
	if event.Item.Name != "" {
		builder.Name = event.Item.Name
	}
	if event.Item.CallID != "" {
		builder.CallID = event.Item.CallID
	}
	builder.Freeform = builder.Freeform || event.Item.Type == "custom_tool_call"
	finalInput := event.Item.Arguments
	if builder.Freeform {
		finalInput = event.Item.Input
	}
	if finalInput != "" && builder.Arguments.Len() == 0 {
		builder.Arguments.WriteString(finalInput)
	}
	if !builder.started {
		providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
			Type:               zeroruntime.StreamEventToolCallStart,
			ToolCallID:         key,
			ToolCallProviderID: builder.CallID,
			ToolName:           builder.Name,
			ToolCallFreeform:   builder.Freeform,
		})
		builder.started = true
	}
	providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
		Type:       zeroruntime.StreamEventToolCallEnd,
		ToolCallID: key,
		ToolName:   builder.Name,
	})
	builder.ended = true
	state.emitted = true
}

func (p *ResponsesProvider) handleTerminalResponse(
	ctx context.Context,
	event *responsesEvent,
	state *responsesState,
	events chan<- zeroruntime.StreamEvent,
) bool {
	if event.Response == nil {
		if event.Type == responsesEventFailed {
			providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
				Type:  zeroruntime.StreamEventError,
				Error: "responses: response.failed with no error detail",
			})
		} else {
			providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
				Type:            zeroruntime.StreamEventDone,
				ResponseID:      state.responseID,
				FinishReason:    zeroruntime.FinishReasonLength,
				ReasoningBlocks: state.reasoningBlocks,
			})
		}
		state.done = true
		return false
	}
	state.usage = event.Response.Usage
	state.responseID = event.Response.ID
	if event.Response.Usage != nil {
		usage := zeroruntime.Usage{
			InputTokens:  event.Response.Usage.InputTokens,
			OutputTokens: event.Response.Usage.OutputTokens,
		}
		if event.Response.Usage.InputTokensDetails != nil {
			usage.CachedInputTokens = event.Response.Usage.InputTokensDetails.CachedTokens
			usage.CacheWriteTokens = event.Response.Usage.InputTokensDetails.CacheWriteTokens
		}
		if event.Response.Usage.OutputTokensDetails != nil {
			usage.ReasoningTokens = event.Response.Usage.OutputTokensDetails.ReasoningTokens
		}
		providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
			Type:  zeroruntime.StreamEventUsage,
			Usage: usage,
		})
	}
	if event.Response.Error != nil {
		providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
			Type:  zeroruntime.StreamEventError,
			Error: p.redact(event.Response.Error.Message),
		})
		state.done = true
		return false
	}
	if event.Type == responsesEventFailed || event.Response.Status == "failed" {
		msg := "responses: response.failed with no error detail"
		if status := strings.TrimSpace(event.Response.Status); status != "" {
			msg = fmt.Sprintf("responses: response.failed (status %q) with no error detail", status)
		}
		providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
			Type:  zeroruntime.StreamEventError,
			Error: msg,
		})
		state.done = true
		return false
	}
	finishReason := state.finishReason
	if event.Type == responsesEventIncomplete {
		reason := ""
		if event.Response.IncompleteDetails != nil {
			reason = event.Response.IncompleteDetails.Reason
		}
		state.finishReason = mapIncompleteReason(reason)
		finishReason = state.finishReason
	}
	providerio.SendEvent(ctx, events, zeroruntime.StreamEvent{
		Type:            zeroruntime.StreamEventDone,
		ResponseID:      event.Response.ID,
		FinishReason:    finishReason,
		ReasoningBlocks: state.reasoningBlocks,
	})
	state.done = true
	return false
}

func mapIncompleteReason(reason string) string {
	switch strings.TrimSpace(reason) {
	case "max_output_tokens", "max_tokens":
		return zeroruntime.FinishReasonLength
	case "content_filter":
		return zeroruntime.FinishReasonContentFilter
	case "":
		return zeroruntime.FinishReasonLength
	default:
		return strings.TrimSpace(reason)
	}
}

func (p *ResponsesProvider) toolCallKey(event *responsesEvent) string {
	if event.ItemID != "" {
		return event.ItemID
	}
	if event.Item != nil && event.Item.ID != "" {
		return event.Item.ID
	}
	if event.Item != nil && event.Item.CallID != "" {
		return event.Item.CallID
	}
	if event.CallID != "" {
		return event.CallID
	}
	if event.OutputIndex != nil {
		return fmt.Sprintf("output-%d", *event.OutputIndex)
	}
	return ""
}

func (p *ResponsesProvider) redact(message string) string {
	if p.inner == nil {
		return providerio.Redact(message)
	}
	return providerio.Redact(message, p.inner.apiKey, p.inner.authHeaderValue)
}

func (p *ResponsesProvider) classifiedError(statusCode int, message string) string {
	if p.inner == nil {
		return providerio.ClassifiedError(statusCode, message)
	}
	return providerio.ClassifiedError(statusCode, message, p.inner.apiKey, p.inner.authHeaderValue)
}
