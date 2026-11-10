package model

import "testing"

func TestPromptCloneOwnsNestedToolDefinitions(t *testing.T) {
	original := Prompt{Tools: []Tool{{Name: "Read", Parameters: map[string]any{
		"nested": map[string]any{"enum": []string{"one"}, "anyOf": []any{map[string]any{"type": "string"}}},
	}}}}
	cloned := ClonePrompt(original)
	nested := cloned.Tools[0].Parameters["nested"].(map[string]any)
	nested["enum"].([]string)[0] = "changed"
	nested["anyOf"].([]any)[0].(map[string]any)["type"] = "changed"
	cloned.Tools[0].Name = "changed"
	prior := original.Tools[0].Parameters["nested"].(map[string]any)
	if original.Tools[0].Name != "Read" || prior["enum"].([]string)[0] != "one" ||
		prior["anyOf"].([]any)[0].(map[string]any)["type"] != "string" {
		t.Fatal("saved request prefix shares mutable tool definitions")
	}
}

func TestOutputLimitMatchesProtocolDefaults(t *testing.T) {
	for _, protocol := range []Protocol{Chat, Responses, Messages} {
		t.Run(string(protocol), func(t *testing.T) {
			want := 0
			if protocol == Messages {
				want = 8192
			}
			if got := (Model{}).OutputLimit(protocol); got != want {
				t.Fatalf("default output reservation differs from wire: %d, want %d", got, want)
			}
			if got := (Model{MaxOutput: 4096}).OutputLimit(protocol); got != 4096 {
				t.Fatal("configured output limit was not reserved")
			}
		})
	}
}
