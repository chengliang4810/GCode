package engine

import (
	"strings"

	"gcode/internal/agent"
	"gcode/internal/contracts"
)

func hookExecutionRecord(execution agent.HookExecution, redact func(string) string) contracts.HookRecord {
	name := execution.Rule.Name
	if name == "" {
		name = execution.Rule.ID
	}
	record := contracts.HookRecord{
		RuleID: execution.Rule.ID, PluginID: execution.Rule.PluginID,
		Name: redact(name), Event: execution.Input.Event, Status: "completed",
	}
	var details []string
	switch {
	case execution.Err != nil:
		record.Status = "failed"
		details = append(details, execution.Err.Error())
	case execution.Result.Deny:
		record.Status = "blocked"
		details = append(details, execution.Result.Reason)
	case execution.Result.Continue != nil && !*execution.Result.Continue:
		record.Status = "stopped"
		details = append(details, execution.Result.StopReason)
	}
	if fields := unsupportedHookOutputs(execution.Result); len(fields) > 0 {
		details = append(details, "Host action is not implemented: "+strings.Join(fields, ", ")+".")
	}
	if raw := execution.Result.UpdatedToolOutput; raw != nil {
		if err := agent.ValidateHookToolOutput(execution.Input.ToolName, raw, execution.Input.ToolOutput); err != nil {
			details = append(details, "Ignored invalid updatedToolOutput: "+err.Error())
		}
	}
	if execution.Result.UpdatedMCPToolOutput != nil && !strings.HasPrefix(execution.Input.ToolName, "mcp__") {
		details = append(details, "Ignored updatedMCPToolOutput for a builtin tool.")
	}
	if text := strings.TrimSpace(execution.Result.SystemMessage); text != "" {
		details = append(details, text)
	}
	if output := strings.TrimSpace(execution.Output); output != "" && !execution.Result.SuppressOutput {
		details = append(details, output)
	}
	if stderr := strings.TrimSpace(execution.Stderr); stderr != "" {
		details = append(details, stderr)
	}
	record.Details = redact(strings.Join(details, "\n\n"))
	if execution.ExitCodeKnown {
		code := execution.ExitCode
		record.ExitCode = &code
	}
	return record
}
