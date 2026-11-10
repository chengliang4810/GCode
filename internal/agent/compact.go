package agent

import (
	"context"
	"errors"
	"strings"

	"gcode/internal/model"
)

const minimumCompactMessages = 6

const compactInstruction = "Summarize this coding conversation for continuation. Preserve the user's objective, constraints, exact paths, completed changes, failures, tool outcomes and next steps. Never claim a task is complete without evidence. Return only the summary; do not call tools."

// Compact reuses the original prompt, tools, effort and message prefix.
// This one-shot request never executes returned tool calls.
func Compact(ctx context.Context, client Model, request model.Request) ([]model.Message, error) {
	if len(request.History) < minimumCompactMessages {
		return nil, errors.New("not enough history to compact")
	}
	cut := max(1, len(request.History)-8)
	for cut < len(request.History) && request.History[cut].Role == "tool" {
		cut++
	}
	if cut >= len(request.History) {
		return nil, errors.New("no complete exchange boundary for compaction")
	}
	var inputIDs []string
	for _, message := range request.History[:cut] {
		inputIDs = append(inputIDs, message.InputIDs...)
	}
	summaryRequest := request
	summaryRequest.ContextChange = "summary"
	summaryRequest.History = append(model.CloneMessages(request.History[:cut]), model.Message{Role: "user", Text: compactInstruction})
	result, err := client.Stream(ctx, summaryRequest, nil)
	if err != nil {
		return nil, err
	}
	if len(result.Message.Calls) != 0 {
		return nil, errors.New("context summary requested tool execution")
	}
	if strings.TrimSpace(result.Message.Text) == "" {
		return nil, errors.New("context summary was empty")
	}
	lastUser := ""
	hasHuman := false
	for _, message := range request.History[:cut] {
		if message.UserInput || message.Role == "user" && !hasHuman {
			lastUser = message.Text
		}
		hasHuman = hasHuman || message.UserInput
	}
	messages := []model.Message{{Role: "user", Summary: true, InputIDs: inputIDs, Text: "Earlier conversation summary:\n" + result.Message.Text + "\n\nMost recent user instruction before this window:\n" + lastUser}}
	return append(messages, model.CloneMessages(request.History[cut:])...), nil
}
