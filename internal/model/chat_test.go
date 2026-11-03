package model

import (
	"strings"
	"testing"
)

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
