package model

import (
	"errors"
	"strings"
	"time"
)

// Invocation describes one real HTTP attempt, including retryable failures.
// It contains no transport credentials or request headers.
type Invocation struct {
	ID              string               `json:"id"`
	Attempt         int                  `json:"attempt"`
	StartedAt       time.Time            `json:"startedAt"`
	CompletedAt     *time.Time           `json:"completedAt,omitempty"`
	DurationMS      int64                `json:"durationMs"`
	ProviderID      string               `json:"providerId"`
	Provider        string               `json:"provider"`
	ModelID         string               `json:"modelId"`
	Protocol        Protocol             `json:"protocol"`
	System          string               `json:"system,omitempty"`
	Effort          string               `json:"effort,omitempty"`
	Messages        []Message            `json:"messages"`
	Tools           []Tool               `json:"tools,omitempty"`
	Response        *Result              `json:"response,omitempty"`
	HTTPStatus      int                  `json:"httpStatus,omitempty"`
	Error           string               `json:"error,omitempty"`
	InvalidToolCall *ToolCallError       `json:"invalidToolCall,omitempty"`
	Truncated       bool                 `json:"truncated,omitempty"`
	Request         RequestDiagnostics   `json:"request"`
	Transport       TransportDiagnostics `json:"transport"`
	ContextChange   string               `json:"contextChange,omitempty"`
	CacheChange     *CacheChange         `json:"cacheChange,omitempty"`
}

func traceInvalidToolCall(err error, secret string, truncated *bool) *ToolCallError {
	var invalid *ToolCallError
	if !errors.As(err, &invalid) {
		return nil
	}
	copy := *invalid
	copy.Call.ID = traceText(copy.Call.ID, secret, truncated)
	copy.Call.Name = traceText(copy.Call.Name, secret, truncated)
	copy.Call.Arguments = traceText(copy.Call.Arguments, secret, truncated)
	return &copy
}

func (g *Gateway) invocation(id string, attempt int, input Request, started time.Time) Invocation {
	trace := Invocation{ID: id, Attempt: attempt, StartedAt: started, ProviderID: g.provider.ID,
		Provider: g.provider.Name, ModelID: g.target.model.ID, Protocol: g.provider.Protocol, Effort: input.Effort}
	trace.ContextChange = input.ContextChange
	trace.System = traceText(strings.ReplaceAll(input.System, SystemPromptDynamicBoundary, ""), g.provider.APIKey, &trace.Truncated)
	trace.Messages = traceMessages(input.History, g.provider.APIKey, &trace.Truncated)
	for _, tool := range input.Tools {
		tool.Description = traceText(tool.Description, g.provider.APIKey, &trace.Truncated)
		tool.Parameters, _ = traceValue(tool.Parameters, g.provider.APIKey, &trace.Truncated).(map[string]any)
		trace.Tools = append(trace.Tools, tool)
	}
	return trace
}

func traceMessages(messages []Message, secret string, truncated *bool) []Message {
	result := make([]Message, 0, len(messages))
	for _, message := range messages {
		// Execution-only continuation state and raw attachment bytes are not
		// useful in a text trajectory and must not multiply diagnostic storage.
		copy := Message{Role: message.Role, CallID: message.CallID, IsError: message.IsError,
			Text: traceText(message.Text, secret, truncated), Reasoning: traceText(message.Reasoning, secret, truncated)}
		for _, call := range message.Calls {
			call.Arguments = traceText(call.Arguments, secret, truncated)
			copy.Calls = append(copy.Calls, call)
		}
		for _, image := range message.Images {
			image.Data = "[attachment data omitted]"
			copy.Images = append(copy.Images, image)
		}
		result = append(result, copy)
	}
	return result
}

func traceText(text, secret string, truncated *bool) string {
	if secret != "" {
		text = strings.ReplaceAll(text, secret, "[redacted]")
	}
	const limit = 64 << 10
	if len(text) > limit {
		runes := []rune(text)
		if len(runes) > limit/4 {
			runes = runes[:limit/4]
		}
		*truncated = true
		return string(runes) + "\n[content truncated]"
	}
	return text
}

func traceValue(value any, secret string, truncated *bool) any {
	switch value := value.(type) {
	case map[string]any:
		copy := make(map[string]any, len(value))
		for key, item := range value {
			copy[key] = traceValue(item, secret, truncated)
		}
		return copy
	case []any:
		copy := make([]any, len(value))
		for i, item := range value {
			copy[i] = traceValue(item, secret, truncated)
		}
		return copy
	case string:
		return traceText(value, secret, truncated)
	default:
		return value
	}
}
