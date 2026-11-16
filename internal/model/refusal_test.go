package model

import (
	"strings"
	"testing"
)

func TestRefusalsPreserveTextAcrossProtocols(t *testing.T) {
	for _, tc := range []struct {
		name    string
		adapter adapter
		events  []string
	}{
		{"chat", chatAdapter{}, []string{`{"choices":[{"delta":{"refusal":"Cannot "}}]}`, `{"choices":[{"delta":{"refusal":"assist."},"finish_reason":"stop"}]}`, `[DONE]`}},
		{"responses streamed", responsesAdapter{}, []string{`{"type":"response.refusal.delta","delta":"Cannot "}`, `{"type":"response.refusal.delta","delta":"assist."}`, `{"type":"response.completed","response":{"status":"completed","output":[{"type":"message","content":[{"type":"refusal","refusal":"Cannot assist."}]}]}}`}},
		{"responses final only", responsesAdapter{}, []string{`{"type":"response.completed","response":{"status":"completed","output":[{"type":"message","content":[{"type":"refusal","refusal":"Cannot assist."}]}]}}`}},
		{"messages", messagesAdapter{}, []string{`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"Cannot assist."}}`, `{"type":"message_delta","delta":{"stop_reason":"refusal"}}`, `{"type":"message_stop"}`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stream strings.Builder
			for _, event := range tc.events {
				stream.WriteString("data: " + event + "\n\n")
			}
			result, err := decodeStream(strings.NewReader(stream.String()), tc.adapter.decoder(), nil)
			if err != nil || result.Message.Text != "Cannot assist." || result.Stop != StopComplete || len(result.Message.Calls) != 0 {
				t.Fatalf("refusal lost or treated as failed execution: %+v, %v", result, err)
			}
		})
	}
}

func TestRefusalNeverPublishesToolCalls(t *testing.T) {
	for _, tc := range []struct {
		name    string
		adapter adapter
		events  []string
	}{
		{"chat", chatAdapter{}, []string{`{"choices":[{"delta":{"refusal":"Cannot assist.","tool_calls":[{"index":0,"id":"call","function":{"name":"Read","arguments":"{}"}}]},"finish_reason":"stop"}]}`, `[DONE]`}},
		{"responses", responsesAdapter{}, []string{`{"type":"response.completed","response":{"status":"completed","output":[{"type":"function_call","call_id":"call","name":"Read","arguments":"{}"},{"type":"message","content":[{"type":"refusal","refusal":"Cannot assist."}]}]}}`}},
		{"messages", messagesAdapter{}, []string{`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call","name":"Read","input":{}}}`, `{"type":"message_delta","delta":{"stop_reason":"refusal"}}`, `{"type":"message_stop"}`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stream strings.Builder
			for _, event := range tc.events {
				stream.WriteString("data: " + event + "\n\n")
			}
			result, err := decodeStream(strings.NewReader(stream.String()), tc.adapter.decoder(), nil)
			if err == nil || len(result.Message.Calls) != 0 || ClassifyFailure(err).Retryable {
				t.Fatalf("mixed refusal executed or retried calls: %+v, %v", result, err)
			}
		})
	}
}

func TestContentFilterIsTerminalAndUnknownStopKeepsReason(t *testing.T) {
	for _, tc := range []struct {
		name    string
		adapter adapter
		events  []string
		reason  string
	}{
		{"chat filter", chatAdapter{}, []string{`{"choices":[{"delta":{},"finish_reason":"content_filter"}]}`, `[DONE]`}, "content_filter"},
		{"responses filter", responsesAdapter{}, []string{`{"type":"response.incomplete","response":{"incomplete_details":{"reason":"content_filter"}}}`}, "content_filter"},
		{"chat unknown", chatAdapter{}, []string{`{"choices":[{"delta":{},"finish_reason":"new_reason"}]}`, `[DONE]`}, "unknown"},
		{"messages unknown", messagesAdapter{}, []string{`{"type":"message_delta","delta":{"stop_reason":"new_reason"}}`, `{"type":"message_stop"}`}, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stream strings.Builder
			for _, event := range tc.events {
				stream.WriteString("data: " + event + "\n\n")
			}
			_, err := decodeStream(strings.NewReader(stream.String()), tc.adapter.decoder(), nil)
			failure := ClassifyFailure(err)
			if err == nil || failure.Reason != tc.reason || failure.Retryable {
				t.Fatalf("stop classification: %+v, %v", failure, err)
			}
			if tc.reason == "unknown" && !strings.Contains(err.Error(), "new_reason") {
				t.Fatalf("stop reason omitted: %v", err)
			}
		})
	}
}
