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
