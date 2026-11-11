package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestContextReferencesStayOffAllProviderWires(t *testing.T) {
	for _, test := range []struct {
		name string
		wire adapter
	}{
		{"chat", chatAdapter{}}, {"responses", responsesAdapter{}}, {"messages", messagesAdapter{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, err := test.wire.encode(Request{History: []Message{
				{Role: "user", Text: "task", ContextRef: "private-context-ref"},
			}}, adapterTarget{model: Model{ID: "fixture"}})
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "private-context-ref") || strings.Contains(string(raw), "contextRef") {
				t.Fatal("host identity leaked to provider", string(raw))
			}
		})
	}
}
