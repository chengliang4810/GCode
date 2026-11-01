package agent

import (
	"encoding/json"
	"testing"

	"gcode/internal/contracts"
)

func TestHookConditionMatchesToolArguments(t *testing.T) {
	for _, test := range []struct {
		name, condition, tool, args string
		match                       bool
	}{
		{"simple", "Bash(git commit *)", "Bash", `{"command":"git commit -m done"}`, true},
		{"compound", "Bash(git push *)", "Bash", `{"command":"git status && git push origin main"}`, true},
		{"leading assignments", "Bash(git *)", "Bash", `{"command":"FOO='two words' BAR=1 git push origin main"}`, true},
		{"quoted text", "Bash(git push *)", "Bash", `{"command":"echo 'a && git push origin main'"}`, false},
		{"substitution", "Bash(git push *)", "Bash", `{"command":"echo \"$(git push origin main)\""}`, true},
		{"detailed pattern uncertain expansion", "Bash(git push *)", "Bash", `{"command":"echo $(date)"}`, true},
		{"command-only pattern known substitution", "Bash(rm *)", "Bash", `{"command":"echo $(date)"}`, false},
		{"quoted literal expansion", "Bash(git push *)", "Bash", `{"command":"echo '$USER'"}`, false},
		{"wrong tool", "Bash(*)", "Read", `{"file_path":"a"}`, false},
		{"path", "Write(*.go)", "Write", `{"file_path":"src/main.go"}`, true},
		{"search path", "Glob(*src*)", "Glob", `{"path":"src","pattern":"*.go"}`, true},
		{"prefix", "Bash(git:*)", "Bash", `{"command":"git status"}`, true},
		{"prefix boundary", "Bash(git:*)", "Bash", `{"command":"github status"}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := HookInput{Event: contracts.HookPreToolUse, ToolName: test.tool, ToolInput: json.RawMessage(test.args)}
			if got := hookConditionMatches(test.condition, input); got != test.match {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestHookMCPInputSubstitutionPreservesJSON(t *testing.T) {
	result, err := substituteHookInput(json.RawMessage(`{"command":"${tool_input.command}","session":"${session_id}","meta":"${tool_input.meta}","missing":"${no_such_field}","nested":["${tool_name}"]}`),
		HookInput{SessionID: "s", ToolName: "Bash", ToolInput: json.RawMessage(`{"command":"echo \"hi\"","meta":{"safe":true}}`)})
	if err != nil {
		t.Fatal(err)
	}
	var args map[string]any
	if err := json.Unmarshal(result, &args); err != nil {
		t.Fatal(err)
	}
	if args["command"] != `echo "hi"` || args["session"] != "s" || args["meta"] != `{"safe":true}` || args["missing"] != "" {
		t.Fatalf("%s", result)
	}
}
