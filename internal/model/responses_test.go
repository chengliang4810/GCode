package model

import (
	"strings"
	"testing"
)

func TestResponsesUsageDoesNotAddCachedTokens(t *testing.T) {
	t.Parallel()
	fixture := `data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":100,"output_tokens":7,"input_tokens_details":{"cached_tokens":70}}}}` + "\n\n"
	result, err := decodeStream(strings.NewReader(fixture), responsesAdapter{}.decoder(), nil)
	want := Usage{Known: true, OutputKnown: true, CacheKnown: true, Input: 100, Output: 7, Cached: 70}
	if err != nil || result.Usage != want {
		t.Fatalf("usage = %+v, error = %v; want %+v", result.Usage, err, want)
	}
}

func TestResponsesPlaceholderUsageDoesNotMakeUsageKnown(t *testing.T) {
	t.Parallel()
	fixture := `data: {"type":"response.created","response":{"usage":{"input_tokens":0,"output_tokens":0}}}` + "\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"
	result, err := decodeStream(strings.NewReader(fixture), responsesAdapter{}.decoder(), nil)
	if err != nil || result.Usage != (Usage{}) {
		t.Fatalf("placeholder usage became final usage: %+v %v", result.Usage, err)
	}
}

func TestResponsesKeepsToolCallsThatOnlyArriveAsDeltas(t *testing.T) {
	t.Parallel()
	// A compatible endpoint streams the tool arguments and never repeats the
	// call in the final output array. The call must still execute.
	fixture := `data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","call_id":"call-1","name":"Read"}}` + "\n\n" +
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"path\":"}` + "\n\n" +
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":"\"a.txt\"}"}` + "\n\n" +
		`data: {"type":"response.completed","response":{"status":"completed"}}` + "\n\n"
	result, err := decodeStream(strings.NewReader(fixture), responsesAdapter{}.decoder(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Message.Calls) != 1 {
		t.Fatalf("streamed tool call was dropped: %+v", result)
	}
	call := result.Message.Calls[0]
	if call.ID != "call-1" || call.Name != "Read" || call.Arguments != `{"path":"a.txt"}` {
		t.Fatalf("streamed tool call changed: %+v", call)
	}
	if result.Message.Continuation == nil || !strings.Contains(string(result.Message.Continuation.data), "function_call") {
		t.Fatalf("streamed tool call is missing from the replayed output: %+v", result.Message.Continuation)
	}
}

func TestResponsesReportsStreamedToolArgumentsWithoutACall(t *testing.T) {
	t.Parallel()
	fixture := `data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":"{}"}` + "\n\n" +
		`data: {"type":"response.completed","response":{"status":"completed"}}` + "\n\n"
	if _, err := decodeStream(strings.NewReader(fixture), responsesAdapter{}.decoder(), nil); err == nil {
		t.Fatal("streamed tool arguments without a call were reported as a complete answer")
	}
}

func TestResponsesFinalOutputExcludesInterimCalls(t *testing.T) {
	for _, output := range []string{
		`[]`,
		`[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Finished"}]}]`,
		`[{"type":"function_call","call_id":"final","name":"Read","arguments":"{}"}]`,
	} {
		t.Run(output, func(t *testing.T) {
			fixture := `data: {"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","call_id":"stale","name":"Write"}}` + "\n\n" +
				`data: {"type":"response.function_call_arguments.delta","output_index":1,"delta":"{}"}` + "\n\n" +
				`data: {"type":"response.completed","response":{"status":"completed","output":` + output + `}}` + "\n\n"
			result, err := decodeStream(strings.NewReader(fixture), responsesAdapter{}.decoder(), nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, call := range result.Message.Calls {
				if call.ID == "stale" {
					t.Fatal("final response resurrected an excluded tool call")
				}
			}
			if result.Message.Continuation != nil && strings.Contains(string(result.Message.Continuation.data), "stale") {
				t.Fatal("continuation replay contains an excluded call")
			}
			if strings.Contains(output, `"final"`) && (len(result.Message.Calls) != 1 || result.Message.Calls[0].ID != "final") {
				t.Fatalf("final call missing: %+v", result.Message.Calls)
			}
		})
	}
}
