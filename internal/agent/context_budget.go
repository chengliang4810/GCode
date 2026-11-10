package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"unicode/utf8"

	"gcode/internal/model"
)

type ContextLimits struct {
	Window    int
	MaxOutput int
	ModelKey  string
}

func promptHash(request model.Request) string {
	data, err := json.Marshal(struct {
		System string
		Tools  []model.Tool
		Effort string
	}{request.System, request.Tools, request.Effort})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func estimateMessages(messages []model.Message) int {
	characters := 0
	for _, message := range messages {
		characters += utf8.RuneCountInString(message.Text) + utf8.RuneCountInString(message.Reasoning)
		for _, call := range message.Calls {
			characters += len(call.Name) + len(call.Arguments)
		}
		for _, image := range message.Images {
			pixels := int64(max(image.Width, 0)) * int64(max(image.Height, 0))
			characters += 2 * int(1024+min(pixels/256, 16384))
		}
	}
	return characters/2 + len(messages)*16
}

func contextTokens(request model.Request) int {
	characters := utf8.RuneCountInString(request.System)
	for _, tool := range request.Tools {
		parameters, _ := json.Marshal(tool.Parameters)
		characters += len(tool.Description) + len(tool.Name) + len(parameters)
	}
	return characters/2 + estimateMessages(request.History)
}

func contextPressure(request model.Request, limits ContextLimits) int {
	for i := len(request.History) - 1; i >= 0; i-- {
		anchor := request.History[i].ContextUsage
		if anchor == nil {
			continue
		}
		// Compaction, prepended input, model changes or changed prompt/tools
		// invalidate the old count. Input already includes cached tokens.
		if anchor.Known && anchor.HistoryLength == i && anchor.ModelKey == limits.ModelKey &&
			anchor.PromptHash != "" && anchor.PromptHash == promptHash(request) {
			return anchor.InputTokens + anchor.OutputTokens + estimateMessages(request.History[i+1:])
		}
		break
	}
	return contextTokens(request)
}

func compactThreshold(limits ContextLimits) int {
	output := limits.MaxOutput
	if output <= 0 {
		output = limits.Window / 20
	}
	// Adapters actually send the configured output limit. Reserve it in
	// full instead of assuming a large configured limit will not be used.
	output = min(output, max(0, limits.Window-1))
	growth := min(16_000, limits.Window/20)
	return max(1, limits.Window-output-growth)
}

func needsCompactionBaseline(history []model.Message) bool {
	if len(history) == 0 || !history[0].Summary {
		return false
	}
	for i, message := range history {
		if message.ContextUsage != nil && message.ContextUsage.HistoryLength == i {
			return false
		}
	}
	return true
}
