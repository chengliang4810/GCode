package model

import (
	"fmt"
	"strings"
	"testing"
)

func TestDecodersCachePresence(t *testing.T) {
	for _, protocol := range []struct {
		name    string
		adapter adapter
		fixture func(input, cache string) string
	}{
		{"chat", chatAdapter{}, func(input, cache string) string {
			usage := `"completion_tokens":7`
			if input != "" {
				usage += `,"prompt_tokens":` + input
			}
			if cache != "" {
				usage += `,"prompt_tokens_details":{"cached_tokens":` + cache + `}`
			}
			return `data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{` + usage + "}}\n\ndata: [DONE]\n\n"
		}},
		{"responses", responsesAdapter{}, func(input, cache string) string {
			usage := `"output_tokens":7`
			if input != "" {
				usage += `,"input_tokens":` + input
			}
			if cache != "" {
				usage += `,"input_tokens_details":{"cached_tokens":` + cache + `}`
			}
			return `data: {"type":"response.completed","response":{"status":"completed","usage":{` + usage + "}}}\n\n"
		}},
		{"messages", messagesAdapter{}, func(input, cache string) string {
			var fields []string
			if input != "" {
				fields = append(fields, `"input_tokens":`+input)
			}
			if cache != "" {
				fields = append(fields, `"cache_read_input_tokens":`+cache)
			}
			return `data: {"type":"message_start","message":{"usage":{` + strings.Join(fields, ",") + "}}}\n\n" +
				"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":7}}\n\n" +
				"data: {\"type\":\"message_stop\"}\n\n"
		}},
	} {
		t.Run(protocol.name, func(t *testing.T) {
			for _, tc := range []struct {
				name, input, cache string
				known              bool
				cached             int
			}{
				{"missing", "100", "", false, 0},
				{"null", "100", "null", false, 0},
				{"zero", "100", "0", true, 0},
				{"hit", "100", "60", true, 60},
				{"missing input", "", "60", false, 60},
				{"null input", "null", "60", false, 60},
				{"invalid negative", "100", "-1", false, 0},
			} {
				t.Run(tc.name, func(t *testing.T) {
					result, err := decodeStream(strings.NewReader(protocol.fixture(tc.input, tc.cache)), protocol.adapter.decoder(), nil)
					if err != nil || result.Usage.CacheKnown != tc.known || result.Usage.Cached != tc.cached {
						t.Fatalf("cache presence lost: %+v %v", result.Usage, err)
					}
				})
			}
		})
	}
}

func TestChatNativeCacheReadField(t *testing.T) {
	for _, cached := range []int{0, 60} {
		t.Run(fmt.Sprint(cached), func(t *testing.T) {
			fixture := fmt.Sprintf("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":7,\"prompt_cache_hit_tokens\":%d}}\n\ndata: [DONE]\n\n", cached)
			result, err := decodeStream(strings.NewReader(fixture), chatAdapter{}.decoder(), nil)
			if err != nil || !result.Usage.CacheKnown || result.Usage.Cached != cached {
				t.Fatalf("native cache count was not retained: %+v %v", result.Usage, err)
			}
		})
	}
}

func TestChatCacheHitDoesNotRequireOutputUsage(t *testing.T) {
	fixture := "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":100,\"prompt_tokens_details\":{\"cached_tokens\":60}}}\n\ndata: [DONE]\n\n"
	result, err := decodeStream(strings.NewReader(fixture), chatAdapter{}.decoder(), nil)
	if err != nil || result.Usage.Known || result.Usage.OutputKnown || !result.Usage.CacheKnown {
		t.Fatalf("cache availability incorrectly depends on output accounting: %+v %v", result.Usage, err)
	}
}
