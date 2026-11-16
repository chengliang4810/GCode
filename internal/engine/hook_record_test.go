package engine

import (
	"errors"
	"strings"
	"testing"

	"gcode/internal/agent"
	"gcode/internal/contracts"
)

func TestHookExecutionRecordPreservesDiagnosticsForEveryEventAndHandler(t *testing.T) {
	for _, event := range contracts.HookEvents() {
		t.Run(string(event), func(t *testing.T) {
			for _, kind := range []string{"command", "http", "prompt", "agent", "mcp_tool"} {
				if !contracts.HookTypeSupported(event, kind) {
					continue
				}
				t.Run(kind, func(t *testing.T) {
					execution := agent.HookExecution{
						Rule:  contracts.HookRule{ID: "rule", Name: "Plugin hook", PluginID: "plugin@market", Type: kind},
						Input: agent.HookInput{Event: event}, Output: `{"raw":"secret"}`, Stderr: "diagnostic secret",
						Result: agent.HookResult{SystemMessage: "visible notice"}, ExitCodeKnown: kind == "command",
					}
					record := hookExecutionRecord(execution, func(text string) string {
						return strings.ReplaceAll(text, "secret", "[REDACTED]")
					})
					if record.Event != event || record.Name != "Plugin hook" || record.RuleID != "rule" ||
						record.PluginID != "plugin@market" || record.Status != "completed" ||
						!strings.Contains(record.Details, "visible notice") || !strings.Contains(record.Details, `{"raw":"[REDACTED]"}`) ||
						!strings.Contains(record.Details, "diagnostic [REDACTED]") || strings.Contains(record.Details, "secret") {
						t.Fatalf("lost hook identity or diagnostics: %+v", record)
					}
					if (record.ExitCode != nil) != (kind == "command") {
						t.Fatalf("invented or lost process exit code: %+v", record)
					}
				})
			}
		})
	}
}

func TestHookExecutionRecordStatusesAndSuppression(t *testing.T) {
	stopped := false
	for _, test := range []struct {
		name, status, detail string
		result               agent.HookResult
		err                  error
	}{
		{name: "allowed", status: "completed"},
		{name: "denied", status: "blocked", detail: "local policy", result: agent.HookResult{Deny: true, Reason: "local policy"}},
		{name: "stopped", status: "stopped", detail: "stop here", result: agent.HookResult{Continue: &stopped, StopReason: "stop here"}},
		{name: "failed", status: "failed", detail: "invalid JSON", err: errors.New("invalid JSON")},
		{name: "suppressed", status: "completed", result: agent.HookResult{SuppressOutput: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			execution := agent.HookExecution{
				Rule: contracts.HookRule{ID: "fallback-name"}, Input: agent.HookInput{Event: contracts.HookPreToolUse},
				Result: test.result, Err: test.err, Output: "raw-output",
			}
			record := hookExecutionRecord(execution, func(text string) string { return text })
			if record.Status != test.status || record.Name != "fallback-name" || !strings.Contains(record.Details, test.detail) {
				t.Fatalf("wrong status or diagnostic: %+v", record)
			}
			if strings.Contains(record.Details, "raw-output") == test.result.SuppressOutput {
				t.Fatalf("stdout suppression not respected: %+v", record)
			}
		})
	}
}
