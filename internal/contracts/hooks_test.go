package contracts

import (
	"encoding/json"
	"math"
	"testing"
)

func TestHookRulesValidateAndClone(t *testing.T) {
	base := HookRule{ID: "policy", Event: HookPreToolUse, Type: "command", Command: "policy", Args: []string{"arg"}, Headers: map[string]string{"X-Test": "one"}, Enabled: true}
	if err := ValidateHookRules([]HookRule{base}); err != nil {
		t.Fatal(err)
	}
	clone := CloneHookRules([]HookRule{base})
	clone[0].Args[0] = "changed"
	clone[0].Headers["X-Test"] = "two"
	if base.Args[0] != "arg" {
		t.Fatal("hook arguments escaped clone ownership")
	}
	if base.Headers["X-Test"] != "one" {
		t.Fatal("hook headers escaped clone ownership")
	}
	for _, test := range []struct {
		name string
		edit func(*HookRule)
	}{
		{"event", func(rule *HookRule) { rule.Event = "NotAnEvent" }},
		{"type", func(rule *HookRule) { rule.Type = "script" }},
		{"timeout", func(rule *HookRule) { rule.TimeoutSeconds = -1 }},
		{"NaN timeout", func(rule *HookRule) { rule.TimeoutSeconds = math.NaN() }},
		{"infinite timeout", func(rule *HookRule) { rule.TimeoutSeconds = math.Inf(1) }},
		{"overflowing timeout", func(rule *HookRule) { rule.TimeoutSeconds = 1e20 }},
		{"removed process type", func(rule *HookRule) { rule.Type = "process" }},
		{"shell", func(rule *HookRule) { rule.Shell = "cmd" }},
		{"failure policy", func(rule *HookRule) { rule.OnFailure = "allow" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			rule := base
			test.edit(&rule)
			if err := ValidateHookRules([]HookRule{rule}); err == nil {
				t.Fatal("invalid executable hook accepted")
			}
		})
	}
}

func TestHookFractionalTimeoutSurvivesPersistence(t *testing.T) {
	rule := HookRule{ID: "fractional", Event: HookPreToolUse, Type: "command", Command: "policy", TimeoutSeconds: 0.125}
	if err := ValidateHookRules([]HookRule{rule}); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(rule)
	if err != nil {
		t.Fatal(err)
	}
	var decoded HookRule
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.TimeoutSeconds != 0.125 {
		t.Fatalf("fractional timeout changed: %s", data)
	}
}

func TestHookArgsPresenceSurvivesPersistence(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{"shell", nil}, {"exec no arguments", []string{}}, {"exec arguments", []string{"one"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			rule := HookRule{Args: test.args}
			data, err := json.Marshal(rule)
			if err != nil {
				t.Fatal(err)
			}
			var decoded HookRule
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			if (decoded.Args == nil) != (test.args == nil) {
				t.Fatalf("exec form changed: %s", data)
			}
			cloned := CloneHookRules([]HookRule{decoded})
			if (cloned[0].Args == nil) != (test.args == nil) {
				t.Fatal("clone changed execution form")
			}
		})
	}
}

func TestHookMatchesClaudePatterns(t *testing.T) {
	for _, test := range []struct {
		name           string
		event          HookEvent
		matcher, value string
		match          bool
	}{
		{"literal exact", HookPreToolUse, "Edit", "NotebookEdit", false},
		{"literal list", HookPreToolUse, "Edit, Write", "Write", true},
		{"literal pipe", HookSubagentStart, "Explore|Plan", "Plan", true},
		{"regex unanchored", HookPreToolUse, "Edit.*", "NotebookEdit", true},
		{"mcp prefix exact", HookPreToolUse, "mcp__memory", "mcp__memory__save", false},
		{"mcp regex", HookPreToolUse, "mcp__memory__.*", "mcp__memory__save", true},
		{"unsupported matcher ignored", HookStop, "not final text", "done", true},
		{"prompt matcher ignored", HookUserPromptSubmit, "private", "hello", true},
		{"invalid regex substring fallback", HookPreToolUse, "[", "Read[", true},
		{"JavaScript lookahead", HookPreToolUse, "^(?=Read$)Read", "Read", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := HookMatches(test.event, test.matcher, test.value); got != test.match {
				t.Fatalf("got %v", got)
			}
		})
	}
	if len(HookEvents()) != 33 {
		t.Fatalf("expected current 33 public events, got %d", len(HookEvents()))
	}
}
