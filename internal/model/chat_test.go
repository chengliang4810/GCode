package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestChatReasoningContinuationWithoutToolCalls(t *testing.T) {
	for _, reasoning := range []string{"ordinary reply reasoning", ""} {
		name := "nonempty"
		if reasoning == "" {
			name = "explicit empty"
		}
		t.Run(name, func(t *testing.T) {
			payload, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
				"delta": map[string]any{"content": "answer", "reasoning_content": reasoning}, "finish_reason": "stop",
			}}})
			result, err := decodeStream(strings.NewReader("data: "+string(payload)+"\n\ndata: [DONE]\n\n"), chatAdapter{}.decoder(), nil)
			if err != nil || result.Message.Continuation == nil {
				t.Fatalf("missing reasoning continuation: %+v, %v", result, err)
			}
			result.Message.Continuation.scope = "target"
			result.Message.Reasoning = "display text must not replace adapter-owned reasoning"
			body, err := (chatAdapter{}).encode(Request{History: []Message{result.Message}}, adapterTarget{scope: "target"})
			if err != nil {
				t.Fatal(err)
			}
			message := body["messages"].([]any)[0].(map[string]any)
			if got, ok := message["reasoning_content"]; !ok || got != reasoning {
				t.Fatalf("reasoning replay = %q, %t; want %q", got, ok, reasoning)
			}
		})
	}
	t.Run("not reported", func(t *testing.T) {
		result, err := decodeStream(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"), chatAdapter{}.decoder(), nil)
		if err != nil || result.Message.Continuation != nil {
			t.Fatalf("unreported reasoning invented continuation: %+v, %v", result, err)
		}
	})
}

func TestChatUsageAccumulatesPresenceAndDoesNotAddCachedTokens(t *testing.T) {
	t.Parallel()
	fixture := `data: {"usage":{"prompt_tokens":100,"prompt_tokens_details":{"cached_tokens":70}}}` + "\n\n" +
		`data: {"usage":{"completion_tokens":7}}` + "\n\n" +
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0}}` +
		"\n\ndata: [DONE]\n\n"
	result, err := decodeStream(strings.NewReader(fixture), chatAdapter{}.decoder(), nil)
	want := Usage{Known: true, OutputKnown: true, CacheKnown: true, Input: 100, Output: 7, Cached: 70}
	if err != nil || result.Usage != want {
		t.Fatalf("usage = %+v, error = %v; want %+v", result.Usage, err, want)
	}
}

func TestChatRejectsReusedToolCallIndexForAnotherCall(t *testing.T) {
	t.Parallel()
	// Compatible gateways that omit or repeat tool_calls[].index would otherwise
	// merge two calls into one with concatenated arguments.
	fixture := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-a","function":{"name":"Read","arguments":"{\"path\":\"a\"}"}}]}}]}` + "\n\n" +
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-b","function":{"name":"Read","arguments":"{\"path\":\"b\"}"}}]}}]}` + "\n\n" +
		"data: [DONE]\n\n"
	if _, err := decodeStream(strings.NewReader(fixture), chatAdapter{}.decoder(), nil); err == nil {
		t.Fatal("two calls sharing one index were merged silently")
	}
}

func TestSystemPromptBoundaryIsNotSentToChatOrResponses(t *testing.T) {
	t.Parallel()
	// The cache seam belongs to the Anthropic block layout; the other protocols
	// send one string and must not show the model a marker literal.
	system := "stable instructions\n\n" + SystemPromptDynamicBoundary + "\n\nper-request context"
	body, err := (chatAdapter{}).encode(Request{System: system}, adapterTarget{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(body)
	if strings.Contains(string(encoded), SystemPromptDynamicBoundary) {
		t.Fatalf("chat request carries the boundary marker: %s", encoded)
	}
	if !strings.Contains(string(encoded), "stable instructions") || !strings.Contains(string(encoded), "per-request context") {
		t.Fatalf("chat request lost system text: %s", encoded)
	}
	responses, err := (responsesAdapter{}).encode(Request{System: system}, adapterTarget{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ = json.Marshal(responses)
	if strings.Contains(string(encoded), SystemPromptDynamicBoundary) {
		t.Fatalf("responses request carries the boundary marker: %s", encoded)
	}
}
