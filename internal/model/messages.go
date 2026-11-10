package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type messagesAdapter struct{}

func (messagesAdapter) path() string { return "/messages" }
func (messagesAdapter) headers(header http.Header, key string) {
	header.Set("anthropic-version", "2023-06-01")
	if key != "" {
		header.Set("x-api-key", key)
	}
}

func (messagesAdapter) encode(r Request, target adapterTarget) (map[string]any, error) {
	effort, err := target.model.ResolveEffort(r.Effort)
	if err != nil {
		return nil, err
	}
	if effort == "minimal" {
		return nil, errors.New("anthropic messages does not support minimal effort")
	}
	messages := []any{}
	for _, m := range r.History {
		content := []any{}
		role := m.Role
		if role == "tool" {
			role = "user"
			var result any = m.Text
			if len(m.Images) > 0 {
				blocks := []any{}
				if m.Text != "" {
					blocks = append(blocks, map[string]any{"type": "text", "text": m.Text})
				}
				for _, image := range m.Images {
					blocks = append(blocks, map[string]any{"type": "image", "source": map[string]any{
						"type": "base64", "media_type": image.MIME, "data": image.Data,
					}})
				}
				result = blocks
			}
			content = append(content, map[string]any{
				"type": "tool_result", "tool_use_id": m.CallID, "content": result, "is_error": m.IsError,
			})
		} else if raw := replayData(m, target.scope); len(raw) > 0 && role == "assistant" {
			var blocks []json.RawMessage
			if err := json.Unmarshal(raw, &blocks); err != nil {
				return nil, errors.New("invalid model continuation")
			}
			for _, block := range blocks {
				content = append(content, block)
			}
		} else {
			if m.Text != "" {
				content = append(content, map[string]any{"type": "text", "text": m.Text})
			}
			for _, image := range m.Images {
				content = append(content, map[string]any{"type": "image", "source": map[string]any{
					"type": "base64", "media_type": image.MIME, "data": image.Data,
				}})
			}
			for _, call := range m.Calls {
				content = append(content, map[string]any{
					"type": "tool_use", "id": call.ID, "name": call.Name, "input": json.RawMessage(call.Arguments),
				})
			}
		}
		if len(content) > 0 {
			messages = append(messages, map[string]any{"role": role, "content": content})
		}
	}
	cacheConversationPrefix(messages, r.ContextChange == "summary")
	output := target.model.OutputLimit(Messages)
	body := map[string]any{
		"model": target.model.ID, "system": cachedSystemBlocks(r.System), "messages": messages,
		"stream": true, "max_tokens": output,
	}
	tools := []any{}
	for _, tool := range r.Tools {
		tools = append(tools, map[string]any{
			"name": tool.Name, "description": tool.Description, "input_schema": tool.Parameters,
		})
	}
	if len(tools) > 0 {
		body["tools"] = tools
	}
	switch effort {
	case "":
	case "none":
		body["thinking"] = map[string]any{"type": "disabled"}
	default:
		body["thinking"] = map[string]any{"type": "adaptive"}
		body["output_config"] = map[string]any{"effort": effort}
	}
	return body, nil
}

// A summary reads the existing prefix but does not cache its one-use
// instruction. Never put cache_control directly on thinking or empty text.
func cacheConversationPrefix(messages []any, summary bool) {
	last := len(messages) - 1
	if summary {
		last--
	}
	for i := last; i >= 0; i-- {
		message, _ := messages[i].(map[string]any)
		content, _ := message["content"].([]any)
		for j := len(content) - 1; j >= 0; j-- {
			block, ok := content[j].(map[string]any)
			if !ok {
				raw, ok := content[j].(json.RawMessage)
				if !ok {
					continue
				}
				decoder := json.NewDecoder(bytes.NewReader(raw))
				decoder.UseNumber()
				if decoder.Decode(&block) != nil {
					continue
				}
			}
			switch block["type"] {
			case "thinking", "redacted_thinking":
				continue
			case "text":
				if block["text"] == "" {
					continue
				}
			}
			block["cache_control"] = map[string]any{"type": "ephemeral"}
			content[j] = block
			return
		}
	}
}

func (messagesAdapter) decoder() streamDecoder {
	return &messagesDecoder{streamState: newStreamState(), blocks: make(map[int]*messageBlock)}
}

// messageBlock retains provider ordering and reasoning signatures entirely inside
// the adapter. The Agent sees only text, reasoning and complete calls.
type messageBlock struct {
	Type      string          `json:"type"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"`
	Data      string          `json:"data,omitempty"`
	text      strings.Builder
	thinking  strings.Builder
	signature strings.Builder
}

type messagesDecoder struct {
	streamState
	blocks map[int]*messageBlock
}

func (d *messagesDecoder) consume(payload string) (Delta, error) {
	var event struct {
		Type         string       `json:"type"`
		Index        int          `json:"index"`
		ContentBlock messageBlock `json:"content_block"`
		Delta        struct {
			Type      string `json:"type"`
			Text      string `json:"text"`
			Thinking  string `json:"thinking"`
			Signature string `json:"signature"`
			Partial   string `json:"partial_json"`
			Stop      string `json:"stop_reason"`
		} `json:"delta"`
		Message struct {
			Usage struct {
				Input    *int `json:"input_tokens"`
				Output   *int `json:"output_tokens"`
				Cached   *int `json:"cache_read_input_tokens"`
				Creation int  `json:"cache_creation_input_tokens"`
			} `json:"usage"`
		} `json:"message"`
		Usage struct {
			Output *int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return Delta{}, errors.New("invalid model streaming event")
	}
	delta := Delta{}
	switch event.Type {
	case "error":
		return Delta{}, &streamProviderError{payload: payload}
	case "message_start":
		// Anthropic input_tokens excludes cache reads and writes:
		// https://platform.claude.com/docs/en/build-with-claude/prompt-caching
		usage := event.Message.Usage
		input := usage.Creation
		if usage.Cached != nil {
			input += max(0, *usage.Cached)
		}
		if usage.Input != nil {
			input += *usage.Input
			d.recordUsage(&input, nil, usage.Cached)
		} else {
			d.result.Usage.Input = max(d.result.Usage.Input, input)
			d.recordUsage(nil, nil, usage.Cached)
		}
		if usage.Output != nil {
			// Initial output is only a lower bound; final output reporting arrives
			// in message_delta and is required before Usage.Known becomes true.
			d.result.Usage.Output = max(d.result.Usage.Output, *usage.Output)
		}
	case "content_block_start":
		block := event.ContentBlock
		block.text.WriteString(block.Text)
		block.thinking.WriteString(block.Thinking)
		block.signature.WriteString(block.Signature)
		d.blocks[event.Index] = &block
		delta.Text, delta.Reasoning = block.Text, block.Thinking
		if block.Type == "tool_use" {
			call := Call{ID: block.ID, Name: block.Name}
			if input := string(block.Input); input != "" && input != "{}" {
				call.Arguments = input
			}
			d.calls[event.Index] = newStreamCall(call)
		}
	case "content_block_delta":
		change := event.Delta
		delta.Text, delta.Reasoning = change.Text, change.Thinking
		delta.Output = change.Partial != ""
		block := d.blocks[event.Index]
		if block == nil {
			// Some compatible endpoints omit an empty text block start.
			block = &messageBlock{Type: "text"}
			if change.Thinking != "" {
				block.Type = "thinking"
			}
			d.blocks[event.Index] = block
		}
		if change.Text != "" {
			block.text.WriteString(change.Text)
			block.Text = block.text.String()
		}
		if change.Thinking != "" {
			block.thinking.WriteString(change.Thinking)
			block.Thinking = block.thinking.String()
		}
		if change.Signature != "" {
			block.signature.WriteString(change.Signature)
			block.Signature = block.signature.String()
		}
		if call := d.calls[event.Index]; call != nil {
			call.append("", change.Partial)
		}
	case "message_delta":
		switch event.Delta.Stop {
		case "":
		case "end_turn", "stop_sequence":
			d.stop = StopComplete
		case "tool_use":
			d.stop = StopTools
		case "max_tokens":
			d.stopErr = errOutputLimit
		default:
			d.stopErr = errors.New("unsupported model stop reason")
		}
		// message_delta usage is cumulative, not an increment.
		d.recordUsage(nil, event.Usage.Output, nil)
	case "message_stop":
		d.complete = true
	}
	d.add(delta)
	return delta, nil
}

func (d *messagesDecoder) finish() (Result, error) {
	result, err := d.streamState.finish()
	if err != nil {
		return result, err
	}
	blocks := make([]any, 0, len(d.blocks))
	for _, index := range sortedKeys(d.blocks) {
		block := d.blocks[index]
		switch block.Type {
		case "tool_use":
			call := d.calls[index]
			block.Input = json.RawMessage(call.Arguments)
			if len(block.Input) == 0 {
				block.Input = json.RawMessage("{}")
			}
			blocks = append(blocks, block)
		case "thinking":
			if block.Signature != "" {
				// Required fields must survive even when the reasoning text is empty.
				blocks = append(blocks, map[string]any{
					"type": "thinking", "thinking": block.Thinking, "signature": block.Signature,
				})
			}
		case "redacted_thinking":
			blocks = append(blocks, map[string]any{"type": "redacted_thinking", "data": block.Data})
		case "text":
			if block.Text != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": block.Text})
			}
		default:
			return result, errors.New("unsupported model content block")
		}
	}
	if len(blocks) > 0 {
		data, err := json.Marshal(blocks)
		if err != nil {
			return result, err
		}
		result.Message.Continuation = &Continuation{data: data}
	}
	return result, nil
}

// cachedSystemBlocks splits the system prompt at its dynamic boundary so the
// stable prefix can be cached across requests. Prompt caching charges writes
// once and reads afterwards; without a breakpoint every request re-pays the
// whole instruction block. The marker is the same seam the original project
// uses between static instructions and per-request context.
func cachedSystemBlocks(system string) any {
	if system == "" {
		return ""
	}
	boundary := SystemPromptDynamicBoundary
	index := strings.Index(system, boundary)
	if index < 0 {
		return []any{map[string]any{"type": "text", "text": system, "cache_control": map[string]any{"type": "ephemeral"}}}
	}
	stable := strings.TrimSpace(system[:index])
	dynamic := strings.TrimSpace(system[index+len(boundary):])
	blocks := []any{}
	if stable != "" {
		blocks = append(blocks, map[string]any{"type": "text", "text": stable, "cache_control": map[string]any{"type": "ephemeral"}})
	}
	if dynamic != "" {
		blocks = append(blocks, map[string]any{"type": "text", "text": dynamic})
	}
	if len(blocks) == 0 {
		return ""
	}
	return blocks
}
