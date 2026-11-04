package model

import (
	"encoding/json"
	"errors"
	"net/http"
)

type chatAdapter struct{}

func (chatAdapter) path() string { return "/chat/completions" }
func (chatAdapter) headers(header http.Header, key string) {
	if key != "" {
		header.Set("Authorization", "Bearer "+key)
	}
}

func (chatAdapter) encode(r Request, target adapterTarget) (map[string]any, error) {
	effort, err := target.model.ResolveEffort(r.Effort)
	if err != nil {
		return nil, err
	}
	messages := []any{}
	if r.System != "" {
		messages = append(messages, map[string]any{"role": "system", "content": r.System})
	}
	toolImages := []any{}
	for index, m := range r.History {
		if m.Role == "tool" {
			messages = append(messages, map[string]any{
				"role": "tool", "tool_call_id": m.CallID, "content": m.Text,
			})
			if len(m.Images) > 0 {
				toolImages = append(toolImages, map[string]any{
					"type": "text", "text": "Images from tool call " + m.CallID + ":",
				})
				for _, image := range m.Images {
					toolImages = append(toolImages, map[string]any{
						"type":      "image_url",
						"image_url": map[string]any{"url": "data:" + image.MIME + ";base64," + image.Data},
					})
				}
			}
			lastTool := index+1 == len(r.History) || r.History[index+1].Role != "tool"
			if lastTool && len(toolImages) > 0 {
				// Finish every tool result before inserting a user image message.
				messages = append(messages, map[string]any{"role": "user", "content": toolImages})
				toolImages = []any{}
			}
			continue
		}
		item := map[string]any{"role": m.Role, "content": m.Text}
		if raw := replayData(m, target.scope); len(raw) > 0 && m.Role == "assistant" {
			var continuation chatContinuation
			if err := json.Unmarshal(raw, &continuation); err != nil {
				return nil, errors.New("invalid model continuation")
			}
			item["reasoning_content"] = continuation.Reasoning
		}
		if len(m.Images) > 0 {
			parts := []any{map[string]any{"type": "text", "text": m.Text}}
			for _, image := range m.Images {
				parts = append(parts, map[string]any{
					"type":      "image_url",
					"image_url": map[string]any{"url": "data:" + image.MIME + ";base64," + image.Data},
				})
			}
			item["content"] = parts
		}
		if m.CallID != "" {
			item["tool_call_id"] = m.CallID
		}
		if len(m.Calls) > 0 {
			calls := []any{}
			for _, call := range m.Calls {
				calls = append(calls, map[string]any{
					"id": call.ID, "type": "function",
					"function": map[string]any{"name": call.Name, "arguments": call.Arguments},
				})
			}
			item["tool_calls"] = calls
		}
		messages = append(messages, item)
	}
	body := map[string]any{
		"model": target.model.ID, "messages": messages, "stream": true,
		"stream_options": map[string]any{"include_usage": true},
	}
	tools := []any{}
	for _, tool := range r.Tools {
		tools = append(tools, map[string]any{"type": "function", "function": map[string]any{
			"name": tool.Name, "description": tool.Description, "parameters": tool.Parameters,
		}})
	}
	if len(tools) > 0 {
		body["tools"] = tools
	}
	if target.model.MaxOutput > 0 {
		body["max_completion_tokens"] = target.model.MaxOutput
	}
	if effort != "" {
		body["reasoning_effort"] = effort
	}
	return body, nil
}

func (chatAdapter) decoder() streamDecoder {
	return &chatDecoder{streamState: newStreamState()}
}

type chatDecoder struct {
	streamState
	reasoningReported bool
}

type chatContinuation struct {
	Reasoning string `json:"reasoning_content"`
}

func (d *chatDecoder) finish() (Result, error) {
	result, err := d.streamState.finish()
	if err == nil && d.reasoningReported {
		data, marshalErr := json.Marshal(chatContinuation{Reasoning: result.Message.Reasoning})
		if marshalErr != nil {
			return result, marshalErr
		}
		result.Message.Continuation = &Continuation{data: data}
	}
	return result, err
}

func (d *chatDecoder) consume(payload string) (Delta, error) {
	if payload == "[DONE]" {
		d.complete = true
		return Delta{}, nil
	}
	var event struct {
		Type    string          `json:"type"`
		Error   json.RawMessage `json:"error"`
		Choices []struct {
			Delta struct {
				Content   string  `json:"content"`
				Reasoning *string `json:"reasoning_content"`
				Calls     []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
			Finish string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			Prompt     *int `json:"prompt_tokens"`
			Completion *int `json:"completion_tokens"`
			CacheHit   *int `json:"prompt_cache_hit_tokens"`
			Details    struct {
				Cached *int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return Delta{}, errors.New("invalid model streaming event")
	}
	cached := event.Usage.Details.Cached
	if cached == nil {
		cached = event.Usage.CacheHit
	}
	d.recordUsage(event.Usage.Prompt, event.Usage.Completion, cached)
	if event.Type == "error" || (len(event.Error) > 0 && string(event.Error) != "null") {
		return Delta{}, &streamProviderError{payload: payload}
	}
	delta := Delta{}
	for _, choice := range event.Choices {
		delta.Text += choice.Delta.Content
		if choice.Delta.Reasoning != nil {
			d.reasoningReported = true
			delta.Reasoning += *choice.Delta.Reasoning
		}
		for _, change := range choice.Delta.Calls {
			delta.Output = delta.Output || change.Function.Arguments != ""
			call := d.calls[change.Index]
			if call == nil {
				call = &Call{}
				d.calls[change.Index] = call
			}
			if change.ID != "" {
				call.ID = change.ID
			}
			call.Name += change.Function.Name
			call.Arguments += change.Function.Arguments
		}
		if choice.Finish != "" {
			switch choice.Finish {
			case "stop":
				d.stop = StopComplete
			case "tool_calls":
				d.stop = StopTools
			case "length":
				d.stopErr = errOutputLimit
			default:
				d.stopErr = errors.New("unsupported model stop reason")
			}
		}
	}
	d.add(delta)
	return delta, nil
}
