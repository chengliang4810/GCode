package agent

import (
	"encoding/json"
	"testing"

	"gcode/internal/contracts"
)

func BenchmarkHookConditionMatches(b *testing.B) {
	runner := NewHookRunner([]contracts.HookRule{{
		ID: "bench", Enabled: true, Event: contracts.HookPreToolUse,
		Type: "command", If: "Bash(git push *)", Command: "unused",
	}}, nil)
	input := HookInput{Event: contracts.HookPreToolUse, ToolName: "Bash",
		ToolInput: json.RawMessage(`{"command":"git status && git diff && git push origin main"}`)}
	b.ReportAllocs()
	for b.Loop() {
		if !runner.conditions["Bash(git push *)"].matches(input) {
			b.Fatal("condition did not match")
		}
	}
}
