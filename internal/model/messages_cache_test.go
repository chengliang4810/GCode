package model

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestMessagesCacheBreakpointFollowsConversationAndPrecedesSummary(t *testing.T) {
	target := adapterTarget{model: Model{ID: "m"}}
	history := []Message{{Role: "user", Text: "task"}, {Role: "assistant", Calls: []Call{{ID: "r", Name: "Read", Arguments: "{}"}}},
		{Role: "tool", CallID: "r", Text: "content"}}
	for _, summary := range []bool{false, true} {
		name := "conversation"
		request := Request{System: "stable", History: CloneMessages(history)}
		if summary {
			name = "summary"
			request.ContextChange = "summary"
			request.History = append(request.History, Message{Role: "user", Text: "summarize"})
		}
		t.Run(name, func(t *testing.T) {
			body, err := (messagesAdapter{}).encode(request, target)
			if err != nil {
				t.Fatal(err)
			}
			messages := body["messages"].([]any)
			marked := 0
			for i, message := range messages {
				content := message.(map[string]any)["content"].([]any)
				for _, block := range content {
					if block.(map[string]any)["cache_control"] != nil {
						marked++
						if i != 2 {
							t.Fatalf("cache breakpoint moved onto one-use summary content: %d", i)
						}
					}
				}
			}
			if marked != 1 || !reflect.DeepEqual(history, request.History[:len(history)]) {
				t.Fatal("cache annotation changed source history")
			}
		})
	}
}

func TestDiagnosticMessageHashesIgnoreMovingCacheMarkersOnly(t *testing.T) {
	first := Request{History: []Message{{Role: "user", Text: "first"}}}
	payload, _ := (messagesAdapter{}).encode(first, adapterTarget{model: Model{ID: "m"}})
	body, _ := json.Marshal(payload)
	before := requestDiagnostics(payload, body, first)
	second := first
	second.History = append(CloneMessages(first.History), Message{Role: "user", Text: "second"})
	payload, _ = (messagesAdapter{}).encode(second, adapterTarget{model: Model{ID: "m"}})
	body, _ = json.Marshal(payload)
	after := requestDiagnostics(payload, body, second)
	if before.SHA256 == after.SHA256 || before.MessageHashes[0] != after.MessageHashes[0] {
		t.Fatal("moving a cache marker was confused with rewriting history")
	}
}

func TestDiagnosticCacheMarkerNormalizationPreservesToolInput(t *testing.T) {
	block := map[string]any{"type": "tool_use", "cache_control": map[string]any{"type": "ephemeral"},
		"input": map[string]any{"cache_control": "business-data", "number": json.RawMessage("9007199254740993")}}
	message := map[string]any{"role": "assistant", "content": []any{block}}
	first := diagnosticValueHash(withoutCacheMarkers(message))
	block["cache_control"] = map[string]any{"type": "ephemeral", "ttl": "1h"}
	if first != diagnosticValueHash(withoutCacheMarkers(message)) {
		t.Fatal("marker metadata affected content comparison")
	}
	block["input"].(map[string]any)["cache_control"] = "changed business data"
	if first == diagnosticValueHash(withoutCacheMarkers(message)) {
		t.Fatal("normalization hid a changed tool argument")
	}
}

func TestMessagesCacheAnnotationPreservesOpaqueIntegerArguments(t *testing.T) {
	original := json.RawMessage(`[{"type":"tool_use","id":"c","name":"Read","input":{"number":9007199254740993}}]`)
	request := Request{History: []Message{{Role: "assistant", Continuation: &Continuation{scope: "same", data: original}}}}
	body, err := (messagesAdapter{}).encode(request, adapterTarget{model: Model{ID: "m"}, scope: "same"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(body)
	if !strings.Contains(string(encoded), "9007199254740993") || string(request.History[0].Continuation.data) != string(original) {
		t.Fatal("cache annotation rounded or mutated opaque continuation")
	}
}
