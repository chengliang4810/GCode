package agent

import (
	"encoding/json"
	"testing"

	"gcode/internal/model"
)

func TestHookToolOutputRejectsInvalidBuiltinReplacement(t *testing.T) {
	original := model.ToolOutput{Text: "original", Data: map[string]any{"stdout": "original", "stderr": "", "interrupted": false}}
	for _, test := range []struct {
		name, raw string
		valid     bool
		want      string
	}{
		{"string", `"hidden"`, false, "original"},
		{"missing stderr", `{"stdout":"changed","interrupted":false}`, false, "original"},
		{"wrong interrupted", `{"stdout":"changed","stderr":"","interrupted":"no"}`, false, "original"},
		{"valid", `{"stdout":"changed","stderr":"warn","interrupted":false}`, true, "changedwarn"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateHookToolOutput("Bash", json.RawMessage(test.raw), original.Data)
			if (err == nil) != test.valid {
				t.Fatalf("validation: %v", err)
			}
			result := applyHookToolOutput("Bash", original, json.RawMessage(test.raw))
			if result.Text != test.want {
				t.Fatalf("got %q", result.Text)
			}
		})
	}
	result := applyHookToolOutput("mcp__fixture__tool", original, json.RawMessage(`"server-owned-shape"`))
	if result.Text != "server-owned-shape" {
		t.Fatalf("%+v", result)
	}
}

func TestHookToolOutputReadPreservesLineNumberFormat(t *testing.T) {
	raw := json.RawMessage(`{"type":"text","file":{"filePath":"proof.txt","content":"changed\nline two\n","startLine":4,"numLines":2,"totalLines":8}}`)
	result := applyHookToolOutput("Read", model.ToolOutput{Text: "original"}, raw)
	if result.Text != "4: changed\n5: line two\n" {
		t.Fatalf("%+v", result)
	}
}
