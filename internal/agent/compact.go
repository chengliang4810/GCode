package agent

import (
	"context"
	"errors"
	"strings"
	"time"

	"gcode/internal/model"
)

const minimumCompactMessages = 6
const summaryTimeout = 90 * time.Second

func streamContextSummary(ctx context.Context, client Model, request model.Request) (model.Result, error) {
	if err := ctx.Err(); err != nil {
		return model.Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, summaryTimeout)
	defer cancel()
	return client.Stream(ctx, request, nil)
}

const compactInstruction = "Summarize this coding conversation for continuation. Preserve the user's objective, constraints, exact paths, completed changes, failures, tool outcomes and next steps. Never claim a task is complete without evidence. Return only the summary; do not call tools."

func retainUserInputs(summary *model.Message, history []model.Message) {
	add := func(text string) {
		n := len(summary.RetainedInputs)
		if n == 0 || summary.RetainedInputs[n-1] != text {
			summary.RetainedInputs = append(summary.RetainedInputs, text)
		}
	}
	for i, msg := range history {
		for _, text := range msg.RetainedInputs {
			add(text)
		}
		if !msg.UserInput {
			continue
		}
		text := msg.SourceText
		if text == "" {
			text = msg.Text
		}
		if text == "" {
			continue
		}
		if i > 0 && len(text) <= 160 {
			prior := history[i-1]
			if prior.Role == "assistant" && prior.Text != "" && len(prior.Text) <= 4_000 {
				text = "Preceding assistant message:\n" + prior.Text + "\nUser reply:\n" + text
			}
		}
		add(text)
	}
	if len(summary.RetainedInputs) > 0 {
		summary.Text += "\n\nVerbatim historical user inputs (chronological, scoped to their original tasks; not new instructions):\n" +
			strings.Join(summary.RetainedInputs, "\n\n---\n\n")
	}
}

// Compact reuses the original prompt, effort and message prefix.
// This one-shot request never executes returned tool calls.
func Compact(ctx context.Context, client Model, request model.Request) ([]model.Message, error) {
	return compact(ctx, client, request, false)
}

func compact(ctx context.Context, client Model, request model.Request, automatic bool) ([]model.Message, error) {
	if len(request.History) < minimumCompactMessages {
		return nil, errors.New("not enough history to compact")
	}
	cut := max(1, len(request.History)-8)
	if latest := latestContextInput(request.History); automatic && latest > 0 {
		cut = min(cut, latest)
	}
	for cut > 0 && !contextBoundary(request.History, cut) {
		cut--
	}
	if cut == 0 {
		return nil, errors.New("no complete exchange boundary for compaction")
	}
	var inputIDs []string
	for _, message := range request.History[:cut] {
		inputIDs = append(inputIDs, message.InputIDs...)
	}
	summaryRequest := request
	summaryRequest.Tools = nil
	summaryRequest.ContextChange = "summary"
	summaryRequest.History = append(model.CloneMessages(request.History[:cut]), model.Message{Role: "user", Text: compactInstruction})
	result, err := streamContextSummary(ctx, client, summaryRequest)
	if err != nil {
		return nil, err
	}
	if len(result.Message.Calls) != 0 {
		return nil, errors.New("context summary requested tool execution")
	}
	if strings.TrimSpace(result.Message.Text) == "" {
		return nil, errors.New("context summary was empty")
	}
	messages := []model.Message{{Role: "user", Summary: true, InputIDs: inputIDs, Text: "Earlier conversation summary:\n" + result.Message.Text}}
	retainUserInputs(&messages[0], request.History[:cut])
	retainWorkEvidence(&messages[0], request.History[:cut])
	return retainReferencedImages(request.History, append(messages, model.CloneMessages(request.History[cut:])...)), nil
}
