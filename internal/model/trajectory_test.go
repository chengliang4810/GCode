package model

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInvocationRecordsHTTPRetriesAndRedactsCredentials(t *testing.T) {
	calls := 0
	const secret = "private-provider-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer "+secret {
			t.Error("request lost the real credential")
		}
		if calls == 1 {
			w.WriteHeader(503)
			return
		}
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":3}}\n\ndata: [DONE]\n\n", "answer "+secret)
	}))
	defer server.Close()
	provider := Provider{ID: "custom", Name: "Custom", Protocol: Chat, APIKey: secret, BaseURL: server.URL, Models: []Model{{ID: "model"}}}
	gateway, err := newTestClient().Bind(provider, provider.Models[0])
	if err != nil {
		t.Fatal(err)
	}
	var records []Invocation
	gateway.Observe = func(record Invocation) { records = append(records, record) }
	result, err := gateway.Stream(context.Background(), Request{
		System: "system " + secret, History: []Message{{Role: "user", Text: "prompt " + secret,
			Images: []Image{{MIME: "image/png", Data: "raw-image-data"}}}},
		Tools: []Tool{{Name: "Read", Description: "read", Parameters: map[string]any{"key": secret}}},
	}, nil)
	if err != nil || !strings.Contains(result.Message.Text, secret) {
		t.Fatalf("diagnostics altered the actual model response: %v", err)
	}
	if len(records) != 4 || records[0].CompletedAt != nil || records[1].HTTPStatus != 503 ||
		records[1].Error == "" || records[2].Attempt != 2 || records[3].Response == nil ||
		records[3].Response.Usage.Input != 12 || records[0].ID != records[3].ID {
		t.Fatalf("retry lifecycle was not recorded: calls=%d records=%d", calls, len(records))
	}
	data, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret) || strings.Contains(string(data), "raw-image-data") {
		t.Fatal("credential or attachment bytes leaked into call trajectory")
	}
}

func TestInvocationPreservesPartialInterruptedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial output\"}}]}\n\n")
	}))
	defer server.Close()
	provider := Provider{ID: "p", Name: "P", Protocol: Chat, BaseURL: server.URL, Models: []Model{{ID: "m"}}}
	gateway, err := newTestClient().Bind(provider, provider.Models[0])
	if err != nil {
		t.Fatal(err)
	}
	var completed Invocation
	gateway.Observe = func(record Invocation) {
		if record.CompletedAt != nil {
			completed = record
		}
	}
	if _, err := gateway.Stream(context.Background(), Request{}, nil); err == nil {
		t.Fatal("partial stream unexpectedly succeeded")
	}
	if completed.Error == "" || completed.Response == nil || completed.Response.Message.Text != "partial output" {
		t.Fatal("stream failure lost partial model output")
	}
}

func TestInvocationPreservesInvalidToolCallAndRedactsCredentials(t *testing.T) {
	const secret = "private-secret"
	gateway := retryGateway(t, retryTransport(func(*http.Request) (*http.Response, error) {
		return retryResponse(200, `data: {"choices":[{"delta":{"tool_calls":[{"index":2,"id":"bad","function":{"name":"Write","arguments":"{\"content\":\"private-secret\""}}]},"finish_reason":"tool_calls"}]}`+"\n\ndata: [DONE]\n\n"), nil
	}))
	var records []Invocation
	gateway.Observe = func(record Invocation) {
		if record.CompletedAt != nil {
			records = append(records, record)
		}
	}
	result, err := gateway.Stream(t.Context(), Request{}, nil)
	if err == nil || !strings.Contains(err.Error(), "arguments") || len(result.Message.Calls) != 0 {
		t.Fatalf("invalid arguments lack a field-specific failure: %+v %v", result, err)
	}
	if len(records) != DefaultMaxRetries+1 {
		t.Fatalf("invalid calls escaped the shared retry budget: attempts=%d", len(records))
	}
	data, marshalErr := json.Marshal(records[len(records)-1])
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	var record map[string]json.RawMessage
	if json.Unmarshal(data, &record) != nil || len(record["invalidToolCall"]) == 0 {
		t.Fatalf("diagnostics dropped the invalid call: %s", data)
	}
	if strings.Contains(string(data), secret) || !strings.Contains(string(record["invalidToolCall"]), "[redacted]") {
		t.Fatal("invalid call diagnostic leaked credentials or omitted arguments")
	}
	if records[len(records)-1].Response == nil || len(records[len(records)-1].Response.Message.Calls) != 0 {
		t.Fatal("diagnostics published invalid calls as an executable response")
	}
}
