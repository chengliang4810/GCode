package contracts

// HookRecord is a host execution record, separate from model reply text and
// tool calls. Details contain redacted diagnostics and permitted raw output.
type HookRecord struct {
	RuleID   string    `json:"ruleId"`
	PluginID string    `json:"pluginId,omitempty"`
	Name     string    `json:"name"`
	Event    HookEvent `json:"event"`
	Status   string    `json:"status"`
	Details  string    `json:"details,omitempty"`
	ExitCode *int      `json:"exitCode,omitempty"`
}
