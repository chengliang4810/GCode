package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMessagesFragmentedCallsAndSignaturesReplayExactly(t *testing.T) {
	t.Parallel()
	events := []string{
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"a","signature":"sig"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"thinking":"b","signature":"-part"}}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call","name":"Read","input":{}}}`,
		`{"type":"content_block_delta","index":1,"delta":{"partial_json":"{\"path\":"}}`,
		`{"type":"content_block_delta","index":1,"delta":{"partial_json":"\"中文.txt\"}"}}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"}}`,
		`{"type":"message_stop"}`,
	}
	decoder := messagesAdapter{}.decoder()
	for _, payload := range events {
		if _, err := decoder.consume(payload); err != nil {
			t.Fatal(err)
		}
	}
	result, err := decoder.finish()
	if err != nil || len(result.Message.Calls) != 1 || result.Message.Calls[0].Arguments != `{"path":"中文.txt"}` {
		t.Fatalf("fragmented call: %+v, %v", result, err)
	}
	var blocks []messageBlock
	if err := json.Unmarshal(result.Message.Continuation.data, &blocks); err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 || blocks[0].Thinking != "ab" || blocks[0].Signature != "sig-part" ||
		string(blocks[1].Input) != `{"path":"中文.txt"}` {
		t.Fatalf("provider replay changed: %s", result.Message.Continuation.data)
	}
}

func TestMessagesReplayPreservesSeparateSignaturesAndBlockOrder(t *testing.T) {
	fixture := `data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"first"}}` + "\n\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-1"}}` + "\n\n" +
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"redacted_thinking","data":"sealed"}}` + "\n\n" +
		`data: {"type":"content_block_start","index":2,"content_block":{"type":"thinking","thinking":"second","signature":"sig-2"}}` + "\n\n" +
		`data: {"type":"content_block_start","index":3,"content_block":{"type":"text","text":"answer"}}` + "\n\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}` + "\n\n" +
		`data: {"type":"message_stop"}` + "\n\n"
	result, err := decodeStream(strings.NewReader(fixture), messagesAdapter{}.decoder(), nil)
	if err != nil {
		t.Fatal(err)
	}
	result.Message.Continuation.scope = "matching-target"
	body, err := (messagesAdapter{}).encode(
		Request{History: []Message{result.Message}}, adapterTarget{scope: "matching-target"},
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(body)
	var request struct {
		Messages []struct {
			Content []messageBlock `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(encoded, &request); err != nil {
		t.Fatal(err)
	}
	blocks := request.Messages[0].Content
	if len(blocks) != 4 || blocks[0].Signature != "sig-1" ||
		blocks[1].Type != "redacted_thinking" || blocks[1].Data != "sealed" ||
		blocks[2].Signature != "sig-2" || blocks[3].Text != "answer" {
		t.Fatalf("signed blocks were merged/reordered: %s", encoded)
	}
}

func TestMessagesUsageIncludesAllInputAndKeepsCumulativeOutput(t *testing.T) {
	t.Parallel()
	// Anthropic documents these as disjoint input counts, with cumulative
	// output_tokens in message_delta. Cache creation breakdown is not extra input.
	fixture := `data: {"type":"message_start","message":{"usage":{"input_tokens":12,"output_tokens":1,"cache_read_input_tokens":20,"cache_creation_input_tokens":30,"cache_creation":{"ephemeral_5m_input_tokens":10,"ephemeral_1h_input_tokens":20}}}}` + "\n\n" +
		`data: {"type":"message_delta","usage":{"output_tokens":3}}` + "\n\n" +
		`data: {"type":"message_delta","usage":{"output_tokens":7}}` + "\n\n" +
		`data: {"type":"message_delta","usage":{}}` + "\n\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}` + "\n\n" +
		"data: {\"type\":\"message_stop\"}\n\n"
	result, err := decodeStream(strings.NewReader(fixture), messagesAdapter{}.decoder(), nil)
	want := Usage{Known: true, OutputKnown: true, CacheKnown: true, Input: 62, Output: 7, Cached: 20}
	if err != nil || result.Usage != want {
		t.Fatalf("usage = %+v, error = %v; want %+v", result.Usage, err, want)
	}
}

func TestMessagesCacheOnlyUsageRemainsPartial(t *testing.T) {
	t.Parallel()
	fixture := `data: {"type":"message_start","message":{"usage":{"cache_read_input_tokens":20,"cache_creation_input_tokens":30}}}` + "\n\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}` + "\n\n" +
		"data: {\"type\":\"message_stop\"}\n\n"
	result, err := decodeStream(strings.NewReader(fixture), messagesAdapter{}.decoder(), nil)
	want := Usage{OutputKnown: true, Input: 50, Output: 7, Cached: 20}
	if err != nil || result.Usage != want {
		t.Fatalf("usage = %+v, error = %v; want %+v", result.Usage, err, want)
	}
}

func TestMessagesInitialOutputPlaceholderDoesNotMakeUsageKnown(t *testing.T) {
	t.Parallel()
	fixture := `data: {"type":"message_start","message":{"usage":{"input_tokens":12,"output_tokens":0}}}` + "\n\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}` + "\n\n" +
		"data: {\"type\":\"message_stop\"}\n\n"
	result, err := decodeStream(strings.NewReader(fixture), messagesAdapter{}.decoder(), nil)
	want := Usage{Input: 12}
	if err != nil || result.Usage != want {
		t.Fatalf("initial output placeholder made usage known: %+v %v", result.Usage, err)
	}
}

func TestMessagesRetainsInitialOutputLowerBoundOnInterruption(t *testing.T) {
	t.Parallel()
	fixture := `data: {"type":"message_start","message":{"usage":{"input_tokens":12,"output_tokens":1}}}` + "\n\n"
	result, err := decodeStream(strings.NewReader(fixture), messagesAdapter{}.decoder(), nil)
	want := Usage{Input: 12, Output: 1}
	if err == nil || result.Usage != want {
		t.Fatalf("initial output lower bound lost: %+v %v", result.Usage, err)
	}
}

func TestCachedSystemBlocksSplitsAtDynamicBoundary(t *testing.T) {
	// No boundary: one block, cached, so the whole instruction set is reused.
	blocks, ok := cachedSystemBlocks("stable instructions").([]any)
	if !ok || len(blocks) != 1 {
		t.Fatalf("plain system blocks: %#v", blocks)
	}
	block := blocks[0].(map[string]any)
	if block["text"] != "stable instructions" || block["cache_control"] == nil {
		t.Fatalf("plain block: %#v", block)
	}

	// A boundary caches the stable prefix and leaves the dynamic tail uncached:
	// a breakpoint after changing text would never hit.
	system := "stable prefix\n\n" + SystemPromptDynamicBoundary + "\n\nMCP status for this request"
	blocks, _ = cachedSystemBlocks(system).([]any)
	if len(blocks) != 2 {
		t.Fatalf("split blocks: %#v", blocks)
	}
	first := blocks[0].(map[string]any)
	second := blocks[1].(map[string]any)
	if first["text"] != "stable prefix" || first["cache_control"] == nil {
		t.Fatalf("stable block: %#v", first)
	}
	if second["text"] != "MCP status for this request" || second["cache_control"] != nil {
		t.Fatalf("dynamic block: %#v", second)
	}
	// The marker itself never reaches the provider.
	for _, candidate := range []map[string]any{first, second} {
		if text, _ := candidate["text"].(string); strings.Contains(text, SystemPromptDynamicBoundary) {
			t.Fatalf("boundary marker leaked: %q", text)
		}
	}

	// An empty dynamic half must not produce an empty block.
	blocks, _ = cachedSystemBlocks("stable only\n\n" + SystemPromptDynamicBoundary).([]any)
	if len(blocks) != 1 {
		t.Fatalf("empty dynamic half produced %d blocks", len(blocks))
	}
	if got := cachedSystemBlocks(""); got != "" {
		t.Fatalf("empty system: %#v", got)
	}
}

func TestMessagesMergesAdjacentSameRoleMessages(t *testing.T) {
	t.Parallel()
	// One assistant turn with two parallel tool calls produces two tool results,
	// and a hook reminder arrives as another user message. The Messages API
	// rejects consecutive messages with the same role.
	body, err := (messagesAdapter{}).encode(Request{History: []Message{
		{Role: "user", Text: "read two files"},
		{Role: "assistant", Calls: []Call{{ID: "call-1", Name: "Read", Arguments: `{"path":"a"}`}, {ID: "call-2", Name: "Read", Arguments: `{"path":"b"}`}}},
		{Role: "tool", CallID: "call-1", Text: "first"},
		{Role: "tool", CallID: "call-2", Text: "second"},
		{Role: "user", Text: "reminder"},
	}}, adapterTarget{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(body)
	var request struct {
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type      string `json:"type"`
				ToolUseID string `json:"tool_use_id"`
				Text      string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(encoded, &request); err != nil {
		t.Fatal(err)
	}
	if len(request.Messages) != 3 {
		t.Fatalf("messages = %d: %s", len(request.Messages), encoded)
	}
	for i := 1; i < len(request.Messages); i++ {
		if request.Messages[i-1].Role == request.Messages[i].Role {
			t.Fatalf("adjacent %s messages: %s", request.Messages[i].Role, encoded)
		}
	}
	merged := request.Messages[2].Content
	if len(merged) != 3 || merged[0].ToolUseID != "call-1" || merged[1].ToolUseID != "call-2" || merged[2].Text != "reminder" {
		t.Fatalf("merged tool results: %s", encoded)
	}
}
