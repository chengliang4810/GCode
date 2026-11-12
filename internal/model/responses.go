package model

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type responsesAdapter struct{}

func (responsesAdapter) path() string { return "/responses" }
func (responsesAdapter) headers(header http.Header, key string) {
	if key != "" {
		header.Set("Authorization", "Bearer "+key)
	}
}

func (responsesAdapter) encode(r Request, target adapterTarget) (map[string]any, error) {
	effort, err := target.model.ResolveEffort(r.Effort)
	if err != nil {
		return nil, err
	}
	items := []any{}
	for _, m := range r.History {
		if m.Role == "tool" {
			var output any = m.Text
			if len(m.Images) > 0 {
				content := []any{}
				if m.Text != "" {
					content = append(content, map[string]any{"type": "input_text", "text": m.Text})
				}
				for _, image := range m.Images {
					content = append(content, map[string]any{
						"type": "input_image", "image_url": "data:" + image.MIME + ";base64," + image.Data,
					})
				}
				output = content
			}
			items = append(items, map[string]any{
				"type": "function_call_output", "call_id": m.CallID, "output": output,
			})
			continue
		}
		if raw := replayData(m, target.scope); len(raw) > 0 && m.Role == "assistant" {
			var output []json.RawMessage
			if err := json.Unmarshal(raw, &output); err != nil {
				return nil, errors.New("invalid model continuation")
			}
			if len(output) > 0 {
				for _, item := range output {
					items = append(items, item)
				}
				continue
			}
		}
		if m.Text != "" || len(m.Images) > 0 {
			kind := "input_text"
			if m.Role == "assistant" {
				kind = "output_text"
			}
			content := []any{map[string]any{"type": kind, "text": m.Text}}
			for _, image := range m.Images {
				content = append(content, map[string]any{
					"type": "input_image", "image_url": "data:" + image.MIME + ";base64," + image.Data,
				})
			}
			items = append(items, map[string]any{"role": m.Role, "content": content})
		}
		for _, call := range m.Calls {
			items = append(items, map[string]any{
				"type": "function_call", "call_id": call.ID, "name": call.Name, "arguments": call.Arguments,
			})
		}
	}
	body := map[string]any{
		"model": target.model.ID, "instructions": systemWithoutBoundary(r.System), "input": items,
		"stream": true, "store": false, "include": []string{"reasoning.encrypted_content"},
	}
	tools := []any{}
	for _, tool := range r.Tools {
		tools = append(tools, map[string]any{
			"type": "function", "name": tool.Name, "description": tool.Description,
			"parameters": tool.Parameters, "strict": false,
		})
	}
	if len(tools) > 0 {
		body["tools"] = tools
	}
	if target.model.MaxOutput > 0 {
		body["max_output_tokens"] = target.model.MaxOutput
	}
	if effort != "" {
		body["reasoning"] = map[string]any{"effort": effort, "summary": "auto"}
	}
	return body, nil
}

func (responsesAdapter) decoder() streamDecoder {
	return &responsesDecoder{streamState: newStreamState(), output: make(map[int]json.RawMessage), streamed: make(map[int]*streamCall)}
}

type responsesDecoder struct {
	streamState
	output map[int]json.RawMessage
	// streamed accumulates function_call items that arrive as deltas, so a host
	// that never repeats them in the final output still executes the calls.
	streamed map[int]*streamCall
	// functionDeltas counts streamed tool arguments, which must never end in a
	// silent answer without an executable call.
	functionDeltas int
}

// responsesEvent is one streamed Responses API event.
type responsesEvent struct {
	Type        string          `json:"type"`
	Delta       string          `json:"delta"`
	Item        json.RawMessage `json:"item"`
	ItemID      string          `json:"item_id"`
	OutputIndex int             `json:"output_index"`
	Response    struct {
		Status string            `json:"status"`
		Output []json.RawMessage `json:"output"`
		Usage  struct {
			Input   *int `json:"input_tokens"`
			Output  *int `json:"output_tokens"`
			Details struct {
				Cached *int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
		} `json:"usage"`
	} `json:"response"`
}

func (d *responsesDecoder) consume(payload string) (Delta, error) {
	var event responsesEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return Delta{}, errors.New("invalid model streaming event")
	}
	delta := Delta{}
	switch event.Type {
	case "response.completed", "response.incomplete", "response.failed":
		d.recordUsage(event.Response.Usage.Input, event.Response.Usage.Output, event.Response.Usage.Details.Cached)
	}
	switch event.Type {
	case "error", "response.failed":
		return Delta{}, &streamProviderError{payload: payload}
	case "response.output_text.delta":
		delta.Text = event.Delta
	case "response.reasoning_summary_text.delta":
		delta.Reasoning = event.Delta
	case "response.output_item.added":
		var item struct {
			Type   string `json:"type"`
			CallID string `json:"call_id"`
			Name   string `json:"name"`
		}
		if err := json.Unmarshal(event.Item, &item); err != nil {
			return Delta{}, errors.New("invalid model output item")
		}
		if item.Type == "function_call" {
			d.streamed[event.OutputIndex] = newStreamCall(Call{ID: item.CallID, Name: item.Name})
		}
	case "response.function_call_arguments.delta":
		if call := d.streamedCall(event); call != nil {
			call.append("", event.Delta)
		}
		if event.Delta != "" {
			d.functionDeltas++
		}
		delta.Output = event.Delta != ""
	case "response.reasoning_text.delta":
		delta.Output = event.Delta != ""
	case "response.output_item.done":
		d.output[event.OutputIndex] = event.Item
	case "response.completed":
		if event.Response.Status != "completed" {
			return Delta{}, errors.New("model response did not complete")
		}
		d.complete, d.stop = true, StopComplete
		if event.Response.Output != nil {
			// The final array is authoritative; never execute stale interim items.
			d.output = make(map[int]json.RawMessage, len(event.Response.Output))
			for index, item := range event.Response.Output {
				d.output[index] = item
			}
		}
	case "response.incomplete":
		return Delta{}, errors.New("model response was truncated")
	}
	d.add(delta)
	return delta, nil
}

// streamedCall resolves the accumulated call for a function call delta. Endpoints
// that omit response.output_item.added are handled by item_id or output index; a
// delta without either creates the call for its index so the arguments are kept
// and reported as an incomplete call instead of being dropped.
func (d *responsesDecoder) streamedCall(event responsesEvent) *streamCall {
	if call := d.streamed[event.OutputIndex]; call != nil {
		return call
	}
	if event.ItemID != "" {
		for _, call := range d.streamed {
			if call.ID == event.ItemID {
				return call
			}
		}
	}
	call := newStreamCall(Call{ID: event.ItemID})
	d.streamed[event.OutputIndex] = call
	return call
}

// hasCallID reports whether the final output already carries this call, so a
// renumbered item is not executed twice.
func (d *responsesDecoder) hasCallID(id string) bool {
	for _, call := range d.calls {
		if call.ID == id {
			return true
		}
	}
	return false
}

func (d *responsesDecoder) finish() (Result, error) {
	if !d.complete {
		return d.result, ErrStreamInterrupted
	}
	output := make([]json.RawMessage, 0, len(d.output))
	var finalText strings.Builder
	hasFinalMessage := false
	for _, index := range sortedKeys(d.output) {
		raw := d.output[index]
		var item struct {
			Type      string `json:"type"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
			Content   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return d.result, errors.New("invalid model output item")
		}
		output = append(output, raw)
		if item.Type == "function_call" {
			d.calls[index] = newStreamCall(Call{ID: item.CallID, Name: item.Name, Arguments: item.Arguments})
		}
		if item.Type == "message" {
			hasFinalMessage = true
			for _, part := range item.Content {
				if part.Type == "output_text" {
					finalText.WriteString(part.Text)
				}
			}
		}
	}
	if hasFinalMessage {
		if !strings.HasPrefix(finalText.String(), d.result.Message.Text) {
			return d.result, errors.New("model final text does not match streamed text")
		}
		d.result.Message.Text = finalText.String()
	}
	// Calls that only ever arrived as deltas still execute, and stay in the
	// replayed output so the next request carries the assistant tool use.
	for _, index := range sortedKeys(d.streamed) {
		call := d.streamed[index]
		if _, ok := d.calls[index]; ok {
			continue
		}
		if call.ID != "" && d.hasCallID(call.ID) {
			continue
		}
		d.calls[index] = call
		item, err := json.Marshal(map[string]any{
			"type": "function_call", "call_id": call.ID, "name": call.Name, "arguments": call.Arguments,
		})
		if err != nil {
			return d.result, err
		}
		output = append(output, item)
	}
	result, err := d.streamState.finish()
	if err == nil && d.functionDeltas > 0 && len(result.Message.Calls) == 0 {
		return result, errors.New("model streamed tool arguments without a complete call")
	}
	if err == nil && len(output) > 0 {
		data, marshalErr := json.Marshal(output)
		if marshalErr != nil {
			return result, marshalErr
		}
		result.Message.Continuation = &Continuation{data: data}
	}
	return result, err
}
