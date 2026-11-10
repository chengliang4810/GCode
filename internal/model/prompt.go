package model

// Prompt is the small, durable request prefix used by manual compaction. It
// owns definitions only; it does not duplicate history or retain tool objects.
type Prompt struct {
	System string `json:"system"`
	Tools  []Tool `json:"tools"`
	Effort string `json:"effort"`
}

func ClonePrompt(prompt Prompt) Prompt {
	tools := make([]Tool, len(prompt.Tools))
	for i, tool := range prompt.Tools {
		tools[i] = tool
		tools[i].Parameters, _ = clonePromptValue(tool.Parameters).(map[string]any)
	}
	prompt.Tools = tools
	return prompt
}

func clonePromptValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		copy := make(map[string]any, len(value))
		for key, item := range value {
			copy[key] = clonePromptValue(item)
		}
		return copy
	case []any:
		copy := make([]any, len(value))
		for i, item := range value {
			copy[i] = clonePromptValue(item)
		}
		return copy
	case []string:
		return append([]string(nil), value...)
	default:
		return value
	}
}

// ContextKey invalidates usage anchors when the selected tokenizer or effort
// changes. Credentials remain inside a one-way account/model scope digest.
func ContextKey(provider Provider, selected Model, effort string) string {
	return continuationScope(provider, selected) + ":" + effort
}

// OutputLimit is the value actually sent by the protocol adapter. Zero means
// the service owns the default, so the host can only reserve an estimate.
func (m Model) OutputLimit(protocol Protocol) int {
	if m.MaxOutput == 0 && protocol == Messages {
		return 8192
	}
	return m.MaxOutput
}
