package openai

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gcode/internal/zeroruntime"
)

func TestResponsesProviderStreamsEventsAndUsesAPIKey(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("path = %q, want /v1/responses", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-responses" {
			t.Errorf("Authorization = %q, want bearer API key", got)
		}
		if got := r.Header.Get("Cookie"); got != "" {
			t.Errorf("Cookie = %q, OAuth or session cookie must not be sent", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, payload := range []string{
			`{"type":"response.created","response":{"id":"resp_1"}}`,
			`{"type":"response.reasoning_summary_text.delta","delta":"plan"}`,
			`{"type":"response.output_text.delta","delta":"hello"}`,
			`{"type":"response.output_item.added","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup"}}`,
			`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"q\":"}`,
			`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"\"go\"}"}`,
			`{"type":"response.output_item.done","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":"{\"q\":\"go\"}"}}`,
			`{"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":11,"output_tokens":7,"input_tokens_details":{"cached_tokens":3},"output_tokens_details":{"reasoning_tokens":2}}}}`,
		} {
			_, _ = w.Write([]byte("data: " + payload + "\n\n"))
			flusher.Flush()
		}
	}))
	defer server.Close()

	provider, err := NewResponsesProvider(Options{
		APIKey:  "sk-responses",
		BaseURL: server.URL + "/v1",
		Model:   "gpt-5",
	})
	if err != nil {
		t.Fatalf("NewResponsesProvider: %v", err)
	}
	stream, err := provider.StreamCompletion(context.Background(), zeroruntime.CompletionRequest{
		Messages: []zeroruntime.Message{{Role: zeroruntime.MessageRoleUser, Content: "hi"}},
		Tools:    []zeroruntime.ToolDefinition{{Name: "lookup", Parameters: map[string]any{"type": "object"}}},
	})
	if err != nil {
		t.Fatalf("StreamCompletion: %v", err)
	}
	var events []zeroruntime.StreamEvent
	for event := range stream {
		events = append(events, event)
		if event.Type == zeroruntime.StreamEventError {
			t.Fatalf("stream error: %s", event.Error)
		}
	}

	if requestBody["messages"] != nil {
		t.Fatalf("Responses request carried chat-completions messages: %#v", requestBody["messages"])
	}
	if requestBody["model"] != "gpt-5" {
		t.Fatalf("model = %#v, want gpt-5", requestBody["model"])
	}
	input, ok := requestBody["input"].([]any)
	if !ok || len(input) != 1 {
		t.Fatalf("input = %#v, want one user message", requestBody["input"])
	}

	var text, reasoning string
	var usage *zeroruntime.Usage
	var start, delta, end bool
	for _, event := range events {
		switch event.Type {
		case zeroruntime.StreamEventText:
			text += event.Content
		case zeroruntime.StreamEventReasoning:
			reasoning += event.Content
		case zeroruntime.StreamEventToolCallStart:
			start = event.ToolCallID == "fc_1" && event.ToolCallProviderID == "call_1" && event.ToolName == "lookup"
		case zeroruntime.StreamEventToolCallDelta:
			delta = delta || event.ToolCallID == "fc_1"
		case zeroruntime.StreamEventToolCallEnd:
			end = event.ToolCallID == "fc_1"
		case zeroruntime.StreamEventUsage:
			copy := event.Usage
			usage = &copy
		case zeroruntime.StreamEventDone:
			if event.ResponseID != "resp_1" {
				t.Fatalf("ResponseID = %q, want resp_1", event.ResponseID)
			}
		}
	}
	if text != "hello" || reasoning != "plan" {
		t.Fatalf("text/reasoning = %q/%q, want hello/plan", text, reasoning)
	}
	if !start || !delta || !end {
		t.Fatalf("tool events missing: start=%v delta=%v end=%v", start, delta, end)
	}
	if usage == nil || usage.InputTokens != 11 || usage.OutputTokens != 7 || usage.CachedInputTokens != 3 || usage.ReasoningTokens != 2 {
		t.Fatalf("usage = %#v, want normalized Responses usage", usage)
	}
}

func TestResponsesProviderReplaysToolHistory(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"status\":\"completed\"}}\n\n"))
	}))
	defer server.Close()
	provider, err := NewResponsesProvider(Options{APIKey: "sk", BaseURL: server.URL, Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := provider.StreamCompletion(context.Background(), zeroruntime.CompletionRequest{Messages: []zeroruntime.Message{
		{Role: zeroruntime.MessageRoleAssistant, Content: "working", ToolCalls: []zeroruntime.ToolCall{{ID: "fc", ProviderCallID: "pc", Name: "lookup", Arguments: `{"q":"go"}`}}},
		{Role: zeroruntime.MessageRoleTool, ToolCallID: "fc", ToolCallProviderID: "pc", Content: "result"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for event := range stream {
		if event.Type == zeroruntime.StreamEventError {
			t.Fatalf("stream error: %s", event.Error)
		}
	}
	input, ok := body["input"].([]any)
	if !ok || len(input) != 3 {
		t.Fatalf("input = %#v, want assistant message, function call, and result", body["input"])
	}
	call, _ := input[1].(map[string]any)
	if call["type"] != "function_call" || call["id"] != "fc" || call["call_id"] != "pc" {
		t.Fatalf("function call history = %#v", call)
	}
	result, _ := input[2].(map[string]any)
	if result["type"] != "function_call_output" || result["call_id"] != "pc" || result["output"] != "result" {
		t.Fatalf("function result history = %#v", result)
	}
}

func TestResponsesProviderReplaysReasoningItemWithToolHistory(t *testing.T) {
	var requestBodies []map[string]any
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		requestBodies = append(requestBodies, body)
		w.Header().Set("Content-Type", "text/event-stream")
		if requestCount == 1 {
			for _, payload := range []string{
				`{"type":"response.output_item.done","item":{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"plan"}],"content":[],"encrypted_content":"opaque-reasoning","status":"completed"}}`,
				`{"type":"response.output_item.added","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup"}}`,
				`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"q\":\"go\"}"}`,
				`{"type":"response.output_item.done","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":"{\"q\":\"go\"}"}}`,
				`{"type":"response.completed","response":{"id":"resp_1","status":"completed"}}`,
			} {
				_, _ = w.Write([]byte("data: " + payload + "\n\n"))
			}
			return
		}
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_2\",\"status\":\"completed\"}}\n\n"))
	}))
	defer server.Close()

	provider, err := NewResponsesProvider(Options{APIKey: "sk", BaseURL: server.URL, Model: "gpt-5"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := provider.StreamCompletion(context.Background(), zeroruntime.CompletionRequest{
		Messages: []zeroruntime.Message{{Role: zeroruntime.MessageRoleUser, Content: "find go"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	collected := zeroruntime.CollectStream(context.Background(), first)
	if collected.Error != "" || len(collected.ReasoningBlocks) != 1 {
		t.Fatalf("first response = %#v, want one preserved reasoning block", collected)
	}
	block := collected.ReasoningBlocks[0]
	if block.Provider != responsesProviderName || block.Type != "reasoning" || !strings.Contains(block.Data, "opaque-reasoning") {
		t.Fatalf("reasoning block = %#v", block)
	}

	second, err := provider.StreamCompletion(context.Background(), zeroruntime.CompletionRequest{Messages: []zeroruntime.Message{
		{Role: zeroruntime.MessageRoleUser, Content: "find go"},
		{Role: zeroruntime.MessageRoleAssistant, Reasoning: collected.ReasoningBlocks, ToolCalls: collected.ToolCalls},
		{Role: zeroruntime.MessageRoleTool, ToolCallID: "fc_1", ToolCallProviderID: "call_1", Content: "result"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for event := range second {
		if event.Type == zeroruntime.StreamEventError {
			t.Fatalf("second response error: %s", event.Error)
		}
	}
	if len(requestBodies) != 2 {
		t.Fatalf("got %d requests, want 2", len(requestBodies))
	}
	input, ok := requestBodies[1]["input"].([]any)
	if !ok || len(input) != 4 {
		t.Fatalf("second input = %#v, want user, reasoning, call, result", requestBodies[1]["input"])
	}
	reasoning, _ := input[1].(map[string]any)
	if reasoning["type"] != "reasoning" || reasoning["id"] != "rs_1" || reasoning["encrypted_content"] != "opaque-reasoning" || reasoning["status"] != "completed" {
		t.Fatalf("reasoning replay = %#v", reasoning)
	}
	if summary, _ := reasoning["summary"].([]any); len(summary) != 1 {
		t.Fatalf("reasoning summary replay = %#v", reasoning["summary"])
	}
	call, _ := input[2].(map[string]any)
	if call["type"] != "function_call" || call["id"] != "fc_1" || call["call_id"] != "call_1" {
		t.Fatalf("function call replay = %#v", call)
	}
}

func TestResponsesProviderReportsHTTPErrorAndCancellation(t *testing.T) {
	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer errorServer.Close()

	provider, err := NewResponsesProvider(Options{APIKey: "sk", BaseURL: errorServer.URL, Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := provider.StreamCompletion(context.Background(), zeroruntime.CompletionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var gotError string
	for event := range stream {
		if event.Type == zeroruntime.StreamEventError {
			gotError = event.Error
		}
	}
	if !strings.Contains(gotError, "auth error") || strings.Contains(gotError, "sk") {
		t.Fatalf("HTTP error = %q, want classified redacted auth error", gotError)
	}

	cancelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer cancelServer.Close()
	provider, err = NewResponsesProvider(Options{APIKey: "sk", BaseURL: cancelServer.URL, Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	stream, err = provider.StreamCompletion(ctx, zeroruntime.CompletionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	deadline := time.After(2 * time.Second)
	foundCancel := false
	for {
		select {
		case event, ok := <-stream:
			if !ok {
				if !foundCancel {
					t.Fatal("cancellation did not produce an error event")
				}
				return
			}
			if event.Type == zeroruntime.StreamEventError && strings.Contains(event.Error, "context canceled") {
				foundCancel = true
			}
		case <-deadline:
			t.Fatal("cancellation did not close stream")
		}
	}
}

func TestResponsesProviderIncompleteStreamFinishesWithLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writer := bufio.NewWriter(w)
		_, _ = writer.WriteString("data: {\"type\":\"response.incomplete\"}\n\n")
		_ = writer.Flush()
	}))
	defer server.Close()
	provider, err := NewResponsesProvider(Options{BaseURL: server.URL, Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := provider.StreamCompletion(context.Background(), zeroruntime.CompletionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var finish string
	for event := range stream {
		if event.Type == zeroruntime.StreamEventDone {
			finish = event.FinishReason
		}
		if event.Type == zeroruntime.StreamEventError {
			t.Fatalf("stream error: %s", event.Error)
		}
	}
	if finish != zeroruntime.FinishReasonLength {
		t.Fatalf("finish reason = %q, want length", finish)
	}
}

func TestResponsesProviderIncompleteReasonAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.incomplete\",\"response\":{\"id\":\"resp-incomplete\",\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"content_filter\"},\"usage\":{\"input_tokens\":12,\"output_tokens\":5}}}\n\n"))
	}))
	defer server.Close()
	provider, err := NewResponsesProvider(Options{BaseURL: server.URL, Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := provider.StreamCompletion(context.Background(), zeroruntime.CompletionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var finish, responseID string
	var usage *zeroruntime.Usage
	for event := range stream {
		if event.Type == zeroruntime.StreamEventDone {
			finish = event.FinishReason
			responseID = event.ResponseID
		}
		if event.Type == zeroruntime.StreamEventUsage {
			copy := event.Usage
			usage = &copy
		}
	}
	if finish != zeroruntime.FinishReasonContentFilter || responseID != "resp-incomplete" {
		t.Fatalf("terminal = finish %q, responseID %q", finish, responseID)
	}
	if usage == nil || usage.InputTokens != 12 || usage.OutputTokens != 5 {
		t.Fatalf("usage = %#v, want input/output usage", usage)
	}
}
