package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func executeTools(ctx context.Context, options Options, tools map[string]Tool, calls []model.Call, record func(model.Message) error) error {
	var toolCalls []HookToolCall
	safe := func(call model.Call) bool { tool := tools[call.Name]; return tool != nil && tool.Concurrent() }
	for start := 0; start < len(calls); {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := start + 1
		if safe(calls[start]) {
			for end < len(calls) && end-start < 8 && safe(calls[end]) {
				end++
			}
		}
		batch := calls[start:end]
		for _, call := range batch {
			if err := options.Emit(Event{Kind: "tool.start", Call: call}); err != nil {
				return err
			}
		}
		type outcome struct {
			output model.ToolOutput
			err    error
			call   model.Call
		}
		results := make([]outcome, len(batch))
		var work sync.WaitGroup
		for index, call := range batch {
			work.Add(1)
			go func() {
				defer work.Done()
				tool := tools[call.Name]
				if tool == nil {
					results[index].err = fmt.Errorf("unknown tool %q", call.Name)
					return
				}
				if options.ToolAllowed != nil && !options.ToolAllowed(call.Name) {
					results[index].err = fmt.Errorf("tool %q is disabled by the active skill", call.Name)
					return
				}
				if options.Hook != nil {
					tool = hookedTool{Tool: tool, hook: options.Hook, callID: call.ID,
						inputUpdated: func(raw json.RawMessage) {
							call.Arguments = string(raw)
							results[index].err = errors.Join(results[index].err, options.Emit(Event{Kind: "tool.input.changed", Call: call}))
						}}
				}
				output, err := tool.Execute(ctx, options.CWD, json.RawMessage(call.Arguments))
				results[index].output, results[index].err = output, errors.Join(results[index].err, err)
				results[index].call = call
			}()
		}
		// Started calls are drained even after cancellation or recording failure.
		work.Wait()
		var firstError error
		for index, result := range results {
			call := batch[index]
			if result.call.ID != "" {
				call = result.call
			}
			output := result.output
			modelText := strings.TrimSuffix(output.Text, output.HookContext)
			if result.err != nil {
				output.Text = strings.TrimSpace(output.Text + "\n" + result.err.Error())
				modelText = strings.TrimSpace(modelText + "\n" + result.err.Error())
				output.Images, output.Data = nil, nil
			}
			var response any = output.Text
			if len(output.Images) > 0 {
				content := []map[string]any{{"type": "text", "text": output.Text}}
				for _, image := range output.Images {
					content = append(content, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": image.MIME, "data": image.Data}})
				}
				response = content
			}
			toolCalls = append(toolCalls, HookToolCall{ToolName: call.Name, ToolInput: json.RawMessage(call.Arguments), ToolCallID: call.ID, ToolResponse: response})
			message := model.Message{
				Role: "tool", CallID: call.ID, Text: modelText, Images: output.Images, OutputFile: output.OutputFile, IsError: result.err != nil,
			}
			if options.prepareToolResult != nil {
				prepared, err := options.prepareToolResult(call, message)
				if err == nil {
					message = prepared
				} else if firstError == nil {
					// Preserve the completed effect and full output before
					// stopping; never claim unavailable disk output was saved.
					firstError = err
				}
			}
			message.Text += output.HookContext
			if err := record(message); err != nil && firstError == nil {
				firstError = err
			}
			if err := options.Emit(Event{Kind: "tool.end", Call: call, Output: output.Text, Data: output.Data, Error: result.err != nil}); err != nil && firstError == nil {
				firstError = err
			}
		}
		if firstError != nil {
			return firstError
		}
		for _, result := range results {
			var stop HookStopError
			if errors.As(result.err, &stop) {
				return stop
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		start = end
	}
	if options.Hook != nil && len(toolCalls) > 0 {
		result, err := options.Hook(ctx, HookInput{Event: contracts.HookPostToolBatch, ToolCalls: toolCalls})
		if err != nil {
			return err
		}
		if result.Deny {
			result.Context = firstHookText(result.Context, result.Reason, "Tool batch blocked the next model request.")
		}
		for _, message := range hookMessages(result) {
			if err := record(message); err != nil {
				return err
			}
		}
		if result.Deny || result.Continue != nil && !*result.Continue {
			return HookStopError{Reason: firstHookText(result.StopReason, result.Reason)}
		}
	}
	return nil
}
