package agent

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"gcode/internal/model"
)

const minimumCompactMessages = 6

// microCompactMaxToolChars is the ceiling micro compaction applies to one tool
// result. It is also the reserve the marker needs, so a second pass is a no-op.
const microCompactMaxToolChars = 4_000

// microCompactKeepRecentTurns is how many trailing exchanges stay verbatim.
// An exchange is a user turn plus everything the agent did to answer it.
const microCompactKeepRecentTurns = 4

// microCompact shrinks oversized tool results in place and reports how many it
// shortened. It needs no model call, touches no message roles or tool-call
// pairing, and is therefore safe on every turn — most long conversations are
// long because of tool output, and this alone often keeps them running without
// ever paying for a summarization.
func microCompact(history []model.Message) ([]model.Message, int) {
	if len(history) <= microCompactKeepRecentTurns {
		return history, 0
	}
	// Only the older part is shortened: the recent turns are what the model is
	// actively working from. Boundaries fall on user turns so a tool call and
	// its result stay together.
	// Collect the indices of user turns; the last keep of them stay verbatim,
	// and everything before the first of those is stale.
	var turns []int
	for index := range history {
		if history[index].UserInput {
			turns = append(turns, index)
		}
	}
	keep := min(microCompactKeepRecentTurns, len(turns)-1)
	if keep <= 0 {
		return history, 0
	}
	staleEnd := turns[len(turns)-keep]
	shortened := 0
	var updated []model.Message
	for index := 0; index < staleEnd; index++ {
		message := history[index]
		if message.Role != "tool" || message.IsError {
			continue
		}
		if utf8.RuneCountInString(message.Text) <= microCompactMaxToolChars {
			continue
		}
		if updated == nil {
			updated = append([]model.Message(nil), history...)
		}
		updated[index].Text = truncateToolOutput(message.Text, microCompactMaxToolChars)
		shortened++
	}
	if updated == nil {
		return history, 0
	}
	return updated, shortened
}

// truncateToolOutput keeps the head and the tail of an oversized tool result.
// Errors and conclusions commonly appear at the end, so dropping only the
// middle loses less than keeping a single prefix.
func truncateToolOutput(text string, limit int) string {
	runes := []rune(text)
	// Reserve the marker inside the budget.
	budget := limit - 64
	head := budget * 3 / 4
	tail := budget - head
	dropped := len(runes) - budget
	return string(runes[:head]) + "\n\n… (" + strconv.Itoa(dropped) + " more characters truncated)\n\n" + string(runes[len(runes)-tail:])
}

func contextTokens(request model.Request) int {
	characters := utf8.RuneCountInString(request.System)
	for _, message := range request.History {
		characters += utf8.RuneCountInString(message.Text) + utf8.RuneCountInString(message.Reasoning)
		for _, call := range message.Calls {
			characters += len(call.Name) + len(call.Arguments)
		}
		for _, image := range message.Images {
			// A conservative pixel heuristic triggers compaction without treating
			// base64 as text tokens or claiming provider-reported vision usage.
			pixels := int64(max(image.Width, 0)) * int64(max(image.Height, 0))
			characters += 2 * int(1024+min(pixels/256, 16384))
		}
	}
	for _, tool := range request.Tools {
		characters += len(tool.Description) + len(tool.Name) + 256
	}
	// Conservative approximation is a trigger, not provider-reported usage.
	return characters/2 + len(request.History)*16
}

// Compact summarizes a complete prefix without changing the full execution log.
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
	var transcript strings.Builder
	var inputIDs []string
	for _, message := range request.History[:cut] {
		inputIDs = append(inputIDs, message.InputIDs...)
		transcript.WriteString("\n" + message.Role + ": " + message.Text)
		for _, image := range message.Images {
			transcript.WriteString("\n[image attachment " + image.MIME + "; use its recorded file path to read it again if needed]")
		}
		for _, call := range message.Calls {
			transcript.WriteString("\nTool " + call.Name + ": " + call.Arguments)
		}
	}
	summaryRequest := request
	summaryRequest.Tools = nil
	summaryRequest.Effort = ""
	summaryRequest.System = "Summarize this coding conversation for continuation. Preserve the user's objective, constraints, exact paths, completed changes, failures, tool outcomes and next steps. Never claim a task is complete without evidence."
	summaryRequest.History = []model.Message{{Role: "user", Text: transcript.String()}}
	result, err := client.Stream(ctx, summaryRequest, nil)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(result.Message.Text) == "" {
		return nil, errors.New("context summary was empty")
	}
	lastUser := ""
	for _, message := range request.History[:cut] {
		if message.Role == "user" {
			lastUser = message.Text
		}
	}
	messages := []model.Message{{Role: "user", InputIDs: inputIDs, Text: "Earlier conversation summary:\n" + result.Message.Text + "\n\nMost recent user instruction before this window:\n" + lastUser}}
	return append(messages, request.History[cut:]...), nil
}
