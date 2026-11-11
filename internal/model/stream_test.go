package model

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestStreamSnapshotsAndDeltasRemainImmutable(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		adapter adapter
		first   string
		second  string
		end     []string
	}{
		{"chat", chatAdapter{},
			`{"choices":[{"delta":{"content":"first","reasoning_content":"think"}}]}`,
			`{"choices":[{"delta":{"content":" second","reasoning_content":" more"}}]}`,
			[]string{`{"choices":[{"delta":{},"finish_reason":"stop"}]}`, `[DONE]`}},
		{"responses", responsesAdapter{},
			`{"type":"response.output_text.delta","delta":"first"}`,
			`{"type":"response.output_text.delta","delta":" second"}`,
			[]string{`{"type":"response.completed","response":{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"first second final"}]}]}}`}},
		{"messages", messagesAdapter{},
			`{"type":"content_block_delta","index":0,"delta":{"text":"first"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"text":" second"}}`,
			[]string{`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`, `{"type":"message_stop"}`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decoder := tc.adapter.decoder()
			first, err := decoder.consume(tc.first)
			if err != nil || first.Text != "first" {
				t.Fatalf("first live delta: %+v, %v", first, err)
			}
			snapshot := decoder.snapshot()
			second, err := decoder.consume(tc.second)
			if err != nil || second.Text != " second" {
				t.Fatalf("second live delta: %+v, %v", second, err)
			}
			for _, payload := range tc.end {
				if _, err := decoder.consume(payload); err != nil {
					t.Fatal(err)
				}
			}
			result, err := decoder.finish()
			if err != nil || !strings.HasPrefix(result.Message.Text, "first second") {
				t.Fatalf("finished text: %+v, %v", result, err)
			}
			if first.Text != "first" || snapshot.Message.Text != "first" {
				t.Fatalf("later appends changed a retained delta/snapshot: %+v, %+v", first, snapshot)
			}
			if tc.name == "chat" && (snapshot.Message.Reasoning != "think" || result.Message.Reasoning != "think more") {
				t.Fatalf("reasoning changed across snapshots: %+v, %+v", snapshot, result)
			}
		})
	}
}

func TestChatFragmentedToolNameAndArguments(t *testing.T) {
	t.Parallel()
	fixture := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call","function":{"name":"Re","arguments":"{\"path\":"}}]}}]}` + "\n\n" +
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"ad","arguments":"\"中文.txt\"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n"
	var outputDeltas int
	result, err := decodeStream(strings.NewReader(fixture), chatAdapter{}.decoder(), func(delta Delta) error {
		if delta.Output {
			outputDeltas++
		}
		return nil
	})
	if err != nil || len(result.Message.Calls) != 1 || result.Message.Calls[0].Name != "Read" ||
		result.Message.Calls[0].Arguments != `{"path":"中文.txt"}` || outputDeltas != 2 {
		t.Fatalf("fragmented tool call changed: %+v, %v, output deltas=%d", result, err, outputDeltas)
	}
}

func TestInvalidToolCallIdentifiesFieldWithoutPublishingBatch(t *testing.T) {
	for _, test := range []struct {
		name, id, tool, args, field string
	}{
		{"missing id", "", "Write", "{}", "id"},
		{"missing name", "call", "", "{}", "name"},
		{"partial JSON", "call", "Write", "{", "arguments"},
		{"array", "call", "Write", "[]", "arguments"},
		{"null", "call", "Write", "null", "arguments"},
	} {
		t.Run(test.name, func(t *testing.T) {
			event, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
				"delta": map[string]any{"tool_calls": []any{
					map[string]any{"index": 0, "id": "valid", "function": map[string]any{"name": "Write", "arguments": "{}"}},
					map[string]any{"index": 1, "id": test.id, "function": map[string]any{"name": test.tool, "arguments": test.args}},
				}}, "finish_reason": "tool_calls",
			}}})
			result, err := decodeStream(strings.NewReader("data: "+string(event)+"\n\ndata: [DONE]\n\n"), chatAdapter{}.decoder(), nil)
			var invalid *ToolCallError
			if !errors.As(err, &invalid) || invalid.Index != 1 || invalid.Field != test.field || invalid.Call.Arguments != test.args {
				t.Fatalf("invalid call diagnostic=%+v, error=%v", invalid, err)
			}
			if len(result.Message.Calls) != 0 || result.Stop != StopTools {
				t.Fatalf("failed batch exposed calls or lost stop reason: %+v", result)
			}
		})
	}
}

func TestDecodeStreamEmitsBeforeReadingNextEvent(t *testing.T) {
	t.Parallel()
	first := `data: {"choices":[{"delta":{"content":"first"}}]}` + "\n\n"
	second := `data: {"choices":[{"delta":{"content":" second"},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n"
	var emitted []string
	reader := io.MultiReader(strings.NewReader(first), &afterEmissionReader{
		reader: strings.NewReader(second),
		beforeRead: func() {
			if len(emitted) != 1 || emitted[0] != "first" {
				t.Fatal("decoder read ahead without emitting the first delta")
			}
		},
	})
	result, err := decodeStream(reader, chatAdapter{}.decoder(), func(delta Delta) error {
		emitted = append(emitted, delta.Text)
		return nil
	})
	if err != nil || result.Message.Text != "first second" || len(emitted) != 2 {
		t.Fatalf("live stream changed: %+v, %v, %v", result, err, emitted)
	}
}

type afterEmissionReader struct {
	reader     io.Reader
	beforeRead func()
}

func (r *afterEmissionReader) Read(data []byte) (int, error) {
	if r.beforeRead != nil {
		r.beforeRead()
		r.beforeRead = nil
	}
	return r.reader.Read(data)
}

func TestDecodeTimingUsesFirstActualOutput(t *testing.T) {
	for _, tc := range []struct {
		name                string
		adapter             adapter
		before, output, end string
	}{
		{"chat reasoning", chatAdapter{}, `{"choices":[{"delta":{"role":"assistant"}}]}`, `{"choices":[{"delta":{"reasoning_content":"think"}}]}`, "{\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]"},
		{"chat tool", chatAdapter{}, `{}`, `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"t","function":{"name":"Read","arguments":"{}"}}]}}]}`, "{\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]"},
		{"responses tool", responsesAdapter{}, `{"type":"response.created"}`, `{"type":"response.function_call_arguments.delta","delta":"{}"}`, `{"type":"response.completed","response":{"status":"completed"}}`},
		{"messages tool", messagesAdapter{}, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"t","name":"Read"}}`, `{"type":"content_block_delta","index":0,"delta":{"partial_json":"{}"}}`, "{\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"}}\n\ndata: {\"type\":\"message_stop\"}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader, writer := io.Pipe()
			defer reader.Close()
			go func() {
				defer writer.Close()
				_, _ = io.WriteString(writer, "data: "+tc.before+"\n\n")
				// Simulate provider prefill separately from decode.
				time.Sleep(60 * time.Millisecond)
				_, _ = io.WriteString(writer, "data: "+tc.output+"\n\n")
				time.Sleep(20 * time.Millisecond)
				_, _ = io.WriteString(writer, "data: "+tc.end+"\n\n")
			}()
			started := time.Now()
			result, err := decodeStream(reader, tc.adapter.decoder(), nil)
			if err != nil || result.DecodeDuration <= 0 || time.Since(started)-result.DecodeDuration < 50*time.Millisecond {
				t.Fatalf("timing included first-token wait or missed tool output: %v, %+v", err, result)
			}
		})
	}
	for _, fixture := range []string{
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n",
		`{"choices":[{"message":{"content":"buffered"}}],"usage":{"prompt_tokens":10,"completion_tokens":3}}`,
	} {
		result, _ := decodeStream(strings.NewReader(fixture), chatAdapter{}.decoder(), nil)
		if result.DecodeDuration != 0 {
			t.Fatal("non-streaming output or empty deltas invented speed")
		}
	}
}

func TestDecoderRejectsIncompleteStreamsAndCalls(t *testing.T) {
	cases := []struct {
		name    string
		adapter adapter
		fixture string
	}{
		{
			name: "chat missing completion", adapter: chatAdapter{},
			fixture: `data: {"choices":[{"delta":{"content":"partial"},"finish_reason":"stop"}]}` + "\n\n",
		},
		{
			name: "responses missing completion", adapter: responsesAdapter{},
			fixture: `data: {"type":"response.output_text.delta","delta":"partial"}` + "\n\n",
		},
		{
			name: "messages missing completion", adapter: messagesAdapter{},
			fixture: `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}` + "\n\n",
		},
		{
			name: "chat provider error", adapter: chatAdapter{},
			fixture: "data: {\"error\":{\"message\":\"rejected\"}}\n\ndata: [DONE]\n\n",
		},
		{
			name: "chat error event after finish", adapter: chatAdapter{},
			fixture: "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: {\"type\":\"error\"}\n\ndata: [DONE]\n\n",
		},
		{
			name: "responses incomplete", adapter: responsesAdapter{},
			fixture: "data: {\"type\":\"response.incomplete\"}\n\n",
		},
		{
			name: "messages output limit", adapter: messagesAdapter{},
			fixture: `data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call","name":"write","input":{}}}` + "\n\n" +
				`data: {"type":"message_delta","delta":{"stop_reason":"max_tokens"}}` + "\n\n" +
				`data: {"type":"message_stop"}` + "\n\n",
		},
		{
			name: "responses nonobject arguments", adapter: responsesAdapter{},
			fixture: `data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"function_call","call_id":"call","name":"write","arguments":"null"}]}}` + "\n\n",
		},
		{
			name: "messages missing call id", adapter: messagesAdapter{},
			fixture: `data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","name":"write","input":{}}}` + "\n\n" +
				`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"}}` + "\n\n" +
				`data: {"type":"message_stop"}` + "\n\n",
		},
		{
			name: "chat filtered", adapter: chatAdapter{},
			fixture: "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"content_filter\"}]}\n\ndata: [DONE]\n\n",
		},
		{
			name: "messages paused", adapter: messagesAdapter{},
			fixture: "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"pause_turn\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n",
		},
		{
			name: "chat duplicate calls", adapter: chatAdapter{},
			fixture: `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"same","function":{"name":"write","arguments":"{}"}},{"index":1,"id":"same","function":{"name":"write","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n",
		},
		{
			name: "messages duplicate calls", adapter: messagesAdapter{},
			fixture: `data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"same","name":"write","input":{}}}` + "\n\n" +
				`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"same","name":"write","input":{}}}` + "\n\n" +
				`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"}}` + "\n\n" +
				`data: {"type":"message_stop"}` + "\n\n",
		},
		{
			name: "responses duplicate calls", adapter: responsesAdapter{},
			fixture: `data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"function_call","call_id":"same","name":"write","arguments":"{}"},{"type":"function_call","call_id":"same","name":"write","arguments":"{}"}]}}` + "\n\n",
		},
		{
			name: "responses invalid second call", adapter: responsesAdapter{},
			fixture: `data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"function_call","call_id":"one","name":"write","arguments":"{}"},{"type":"function_call","call_id":"two","name":"write","arguments":"{"}]}}` + "\n\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := decodeStream(strings.NewReader(tc.fixture), tc.adapter.decoder(), nil)
			if err == nil || len(result.Message.Calls) != 0 {
				t.Fatalf("incomplete stream exposed executable calls: %+v %v", result, err)
			}
			if strings.Contains(tc.name, "missing completion") && !errors.Is(err, ErrStreamInterrupted) {
				t.Fatalf("missing common interrupted error: %v", err)
			}
		})
	}
}

func TestResponsesReconcilesFinalText(t *testing.T) {
	for _, tc := range []struct {
		name    string
		delta   string
		final   string
		wantErr bool
	}{
		{name: "no deltas", final: "你好 world"},
		{name: "missing suffix", delta: "你好", final: "你好 world"},
		{name: "already complete", delta: "你好 world", final: "你好 world"},
		{name: "conflicting final", delta: "different", final: "你好 world", wantErr: true},
		{name: "conflicting empty final", delta: "different", final: "", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			delta, _ := json.Marshal(map[string]any{"type": "response.output_text.delta", "delta": tc.delta})
			completed, _ := json.Marshal(map[string]any{
				"type": "response.completed",
				"response": map[string]any{
					"status": "completed",
					"output": []any{map[string]any{
						"type":    "message",
						"content": []any{map[string]any{"type": "output_text", "text": tc.final}},
					}},
				},
			})
			fixture := "data: " + string(delta) + "\n\ndata: " + string(completed) + "\n\n"
			var displayed strings.Builder
			result, err := decodeStream(
				strings.NewReader(fixture), responsesAdapter{}.decoder(),
				func(delta Delta) error { displayed.WriteString(delta.Text); return nil },
			)
			if tc.wantErr {
				if err == nil || len(result.Message.Calls) != 0 {
					t.Fatalf("conflicting final accepted: %+v %v", result, err)
				}
			} else if err != nil || displayed.String() != tc.final || result.Message.Text != tc.final {
				t.Fatalf("unified/final/displayed text diverged: %+v %q %v", result, displayed.String(), err)
			}
		})
	}
}

func TestResponsesFinalOutputReplacesInterimItems(t *testing.T) {
	fixture := `data: {"type":"response.output_item.done","output_index":1,"item":{"type":"function_call","call_id":"stale","name":"write","arguments":"{}"}}` + "\n\n" +
		`data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"done"}]}]}}` + "\n\n"
	result, err := decodeStream(strings.NewReader(fixture), responsesAdapter{}.decoder(), nil)
	if err != nil || len(result.Message.Calls) != 0 || result.Stop != StopComplete ||
		result.Message.Text != "done" {
		t.Fatalf("interim call survived final output: %+v %v", result, err)
	}
	data, err := json.Marshal(result.Message)
	if err != nil || strings.Contains(string(data), "stale") {
		t.Fatalf("stale tool survived in continuation: %s %v", data, err)
	}
}

func TestDecodeDoesNotWaitForEOFAfterCompletion(t *testing.T) {
	for _, wire := range []adapter{chatAdapter{}, responsesAdapter{}, messagesAdapter{}} {
		t.Run(wire.path(), func(t *testing.T) {
			fixture := "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
			switch wire.(type) {
			case responsesAdapter:
				fixture = "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n"
			case messagesAdapter:
				fixture = "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n"
			}
			reader := io.MultiReader(strings.NewReader(fixture), brokenReader{})
			if _, err := decodeStream(reader, wire.decoder(), nil); err != nil {
				t.Fatalf("read past protocol completion: %v", err)
			}
		})
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestCompletionIgnoresTrailingFramesAndPropagatesConsumerErrors(t *testing.T) {
	fixture := "data: {\"choices\":[{\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\ndata: invalid trailing data\n\n"
	result, err := decodeStream(strings.NewReader(fixture), chatAdapter{}.decoder(), nil)
	if err != nil || result.Stop != StopComplete || result.Message.Text != "done" {
		t.Fatalf("completed stream consumed trailing data: %+v %v", result, err)
	}
	consumerError := errors.New("consumer stopped")
	_, err = decodeStream(
		strings.NewReader(fixture), chatAdapter{}.decoder(),
		func(Delta) error { return consumerError },
	)
	if !errors.Is(err, consumerError) {
		t.Fatalf("consumer error was lost: %v", err)
	}
}

func TestDecodersUsagePresence(t *testing.T) {
	t.Parallel()
	protocols := []struct {
		name    string
		adapter adapter
		fixture func(string, string) string
	}{
		{
			name: "chat", adapter: chatAdapter{},
			fixture: func(input, output string) string {
				usage := strings.ReplaceAll(input+output, "input_tokens", "prompt_tokens")
				usage = strings.ReplaceAll(usage, "output_tokens", "completion_tokens")
				return `data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{` +
					strings.TrimSuffix(usage, ",") + "}}\n\ndata: [DONE]\n\n"
			},
		},
		{
			name: "responses", adapter: responsesAdapter{},
			fixture: func(input, output string) string {
				return `data: {"type":"response.completed","response":{"status":"completed","usage":{` +
					strings.TrimSuffix(input+output, ",") + "}}}\n\n"
			},
		},
		{
			name: "messages", adapter: messagesAdapter{},
			fixture: func(input, output string) string {
				return `data: {"type":"message_start","message":{"usage":{` +
					strings.TrimSuffix(input, ",") + "}}}\n\n" +
					`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{` +
					strings.TrimSuffix(output, ",") + "}}\n\n" +
					"data: {\"type\":\"message_stop\"}\n\n"
			},
		},
	}
	cases := []struct {
		name          string
		input, output string
		want          Usage
	}{
		{name: "empty usage"},
		{name: "null counters", input: `"input_tokens":null,`, output: `"output_tokens":null,`},
		{name: "input only", input: `"input_tokens":12,`, want: Usage{Input: 12}},
		{name: "output only", output: `"output_tokens":7,`, want: Usage{OutputKnown: true, Output: 7}},
		{name: "zero input only", input: `"input_tokens":0,`},
		{name: "zero output only", output: `"output_tokens":0,`, want: Usage{OutputKnown: true}},
		{name: "explicit zero", input: `"input_tokens":0,`, output: `"output_tokens":0,`, want: Usage{Known: true, OutputKnown: true}},
		{
			name: "complete", input: `"input_tokens":12,`, output: `"output_tokens":7,`,
			want: Usage{Known: true, OutputKnown: true, Input: 12, Output: 7},
		},
	}
	for _, protocol := range protocols {
		t.Run(protocol.name, func(t *testing.T) {
			t.Parallel()
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					result, err := decodeStream(
						strings.NewReader(protocol.fixture(tc.input, tc.output)),
						protocol.adapter.decoder(), nil,
					)
					if err != nil || result.Usage != tc.want {
						t.Fatalf("usage = %+v, error = %v; want %+v", result.Usage, err, tc.want)
					}
				})
			}
		})
	}
}

func TestDecodersMissingUsageRemainsUnknown(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		adapter adapter
		fixture string
	}{
		{
			name: "chat", adapter: chatAdapter{},
			fixture: `data: {"choices":[{"delta":{},"finish_reason":"stop"}]USAGE}` +
				"\n\ndata: [DONE]\n\n",
		},
		{
			name: "responses", adapter: responsesAdapter{},
			fixture: `data: {"type":"response.completed","response":{"status":"completed"USAGE}}` + "\n\n",
		},
		{
			name: "messages", adapter: messagesAdapter{},
			fixture: `data: {"type":"message_start","message":{USAGE}}` + "\n\n" +
				`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}USAGE}` + "\n\n" +
				"data: {\"type\":\"message_stop\"}\n\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, null := range []bool{false, true} {
				name := "omitted"
				replacement := ""
				if null {
					name, replacement = "null", `,"usage":null`
				}
				t.Run(name, func(t *testing.T) {
					fixture := strings.ReplaceAll(tc.fixture, "USAGE", replacement)
					// message_start's usage is the first property, without a comma.
					fixture = strings.ReplaceAll(fixture, `{,"usage"`, `{"usage"`)
					result, err := decodeStream(strings.NewReader(fixture), tc.adapter.decoder(), nil)
					if err != nil || result.Usage != (Usage{}) {
						t.Fatalf("missing usage = %+v, error = %v", result.Usage, err)
					}
				})
			}
		})
	}
}

func TestDecodersPreserveUsageOnStreamErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		adapter adapter
		fixture string
		want    Usage
	}{
		{
			name: "chat provider error", adapter: chatAdapter{},
			fixture: `data: {"usage":{"prompt_tokens":12,"completion_tokens":7}}` + "\n\n" +
				"data: {\"type\":\"error\"}\n\n",
			want: Usage{Known: true, OutputKnown: true, Input: 12, Output: 7},
		},
		{
			name: "chat interrupted partial", adapter: chatAdapter{},
			fixture: "data: {\"usage\":{\"prompt_tokens\":12}}\n\n",
			want:    Usage{Input: 12},
		},
		{
			name: "responses truncated", adapter: responsesAdapter{},
			fixture: `data: {"type":"response.incomplete","response":{"usage":{"input_tokens":12,"output_tokens":7}}}` + "\n\n",
			want:    Usage{Known: true, OutputKnown: true, Input: 12, Output: 7},
		},
		{
			name: "responses failed partial", adapter: responsesAdapter{},
			fixture: `data: {"type":"response.failed","response":{"usage":{"input_tokens":12}}}` + "\n\n",
			want:    Usage{Input: 12},
		},
		{
			name: "messages provider error", adapter: messagesAdapter{},
			fixture: `data: {"type":"message_start","message":{"usage":{"input_tokens":12,"cache_read_input_tokens":20,"cache_creation_input_tokens":30}}}` + "\n\n" +
				"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":7}}\n\n" +
				"data: {\"type\":\"error\"}\n\n",
			want: Usage{Known: true, OutputKnown: true, CacheKnown: true, Input: 62, Output: 7, Cached: 20},
		},
		{
			name: "messages interrupted partial", adapter: messagesAdapter{},
			fixture: "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":12}}}\n\n",
			want:    Usage{Input: 12},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := decodeStream(strings.NewReader(tc.fixture), tc.adapter.decoder(), nil)
			if err == nil || result.Usage != tc.want {
				t.Fatalf("usage = %+v, error = %v; want %+v and error", result.Usage, err, tc.want)
			}
		})
	}
}

func TestDecodePreservesUsageOnConsumerAbort(t *testing.T) {
	t.Parallel()
	fixture := `data: {"usage":{"prompt_tokens":12,"completion_tokens":7}}` + "\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"
	consumerError := errors.New("consumer stopped")
	result, err := decodeStream(strings.NewReader(fixture), chatAdapter{}.decoder(), func(Delta) error {
		return consumerError
	})
	want := Usage{Known: true, OutputKnown: true, Input: 12, Output: 7}
	if !errors.Is(err, consumerError) || result.Usage != want {
		t.Fatalf("consumer abort lost usage: %+v %v", result.Usage, err)
	}
}
