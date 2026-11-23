package engine

import (
	"slices"

	"gcode/internal/contracts"
)

// persistedSnapshot contains owned settings and identities, never refreshable
// file trees or conversation bodies. Conversation records commit separately.
type persistedSnapshot struct {
	Automations      []contracts.Automation                   `json:"automations"`
	AutomationRuns   []contracts.AutomationRun                `json:"automationRuns"`
	Metrics          map[string]contracts.ConversationMetrics `json:"metrics"`
	ApplicationUsage *contracts.ApplicationUsage              `json:"applicationUsage,omitempty"`
	SchemaVersion    int                                      `json:"schemaVersion"`
	Revision         uint64                                   `json:"revision"`
	User             contracts.User                           `json:"user"`
	Projects         []contracts.Project                      `json:"projects"`
	Sessions         []contracts.Session                      `json:"sessions"`
	SessionGroups    []contracts.SessionGroup                 `json:"sessionGroups,omitempty"`
	Settings         contracts.Settings                       `json:"settings"`
	Runs             []contracts.Run                          `json:"runs"`
	// 工作流定义与运行是用户自己攒下的事实：定义关了应用还在，运行的历史也要在。
	WorkflowDefinitions []contracts.WorkflowDefinition `json:"workflowDefinitions,omitempty"`
	WorkflowRuns        []contracts.WorkflowRun        `json:"workflowRuns,omitempty"`
	Terminals           []contracts.Terminal           `json:"terminals"`
	Attachments         []contracts.Attachment         `json:"attachments"`
	Appshots            []contracts.Attachment         `json:"appshots"`
}

func persistenceIndex(snapshot contracts.Snapshot) persistedSnapshot {
	sessions := slices.Clone(snapshot.Sessions)
	for i := range sessions {
		sessions[i].StorageError = ""
		sessions[i].ModelRetry = nil
		sessions[i].Goal = nil
	}
	projects := slices.Clone(snapshot.Projects)
	for i := range projects {
		projects[i].FilesState = ""
		projects[i].FilesError = ""
	}
	runs := slices.Clone(snapshot.Runs)
	for i := range runs {
		// Request identities and completion metadata remain available for
		// idempotency even when conversation bodies are not loaded.
		runs[i].Error = ""
	}
	workflowDefinitions, workflowRuns := cloneWorkflowStateForDisk(snapshot.WorkflowDefinitions, snapshot.WorkflowRuns)
	return persistedSnapshot{
		Automations: snapshot.Automations, AutomationRuns: snapshot.AutomationRuns,
		Metrics: snapshot.Metrics, ApplicationUsage: snapshot.ApplicationUsage,
		SchemaVersion: snapshot.SchemaVersion, Revision: snapshot.Revision,
		User: snapshot.User, Projects: projects, Sessions: sessions,
		SessionGroups: snapshot.SessionGroups, Settings: snapshot.Settings, Runs: runs,
		WorkflowDefinitions: workflowDefinitions, WorkflowRuns: workflowRuns,
		Terminals: snapshot.Terminals, Attachments: snapshot.Attachments,
		Appshots: snapshot.Appshots,
	}
}

func (disk persistedSnapshot) restore(defaults contracts.Snapshot) contracts.Snapshot {
	defaults.Automations, defaults.AutomationRuns = disk.Automations, disk.AutomationRuns
	defaults.Metrics, defaults.ApplicationUsage = disk.Metrics, disk.ApplicationUsage
	defaults.SchemaVersion, defaults.Revision = disk.SchemaVersion, disk.Revision
	defaults.User, defaults.Settings = disk.User, disk.Settings
	defaults.Projects, defaults.Sessions = disk.Projects, disk.Sessions
	defaults.SessionGroups = disk.SessionGroups
	defaults.Terminals = disk.Terminals
	defaults.Runs = disk.Runs
	defaults.WorkflowDefinitions, defaults.WorkflowRuns = disk.WorkflowDefinitions, disk.WorkflowRuns
	for i := range defaults.Runs {
		// A stopping run never settles on its own once the host is gone, so it
		// is normalized exactly like a running one.
		if defaults.Runs[i].Status == contracts.RunStatusRunning || defaults.Runs[i].Status == contracts.RunStatusStopping {
			defaults.Runs[i].Status = contracts.RunStatusInterrupted
			defaults.Runs[i].FinishReason = "engineRestarted"
		}
	}
	freezeRecoveredMetrics(&defaults)
	defaults.Attachments, defaults.Appshots = disk.Attachments, disk.Appshots
	for i := range defaults.Projects {
		defaults.Projects[i].FilesState = contracts.LoadLoading
		defaults.Projects[i].FilesError = ""
	}
	return defaults
}

// cloneWorkflowStateForDisk 拷贝写盘用的工作流事实。运行里带着阶段、实例与
// 留白这些嵌套切片，执行线程会就地改写它们，写盘的那份因此必须自成一份。
func cloneWorkflowStateForDisk(definitions []contracts.WorkflowDefinition, runs []contracts.WorkflowRun) ([]contracts.WorkflowDefinition, []contracts.WorkflowRun) {
	copiedDefinitions := slices.Clone(definitions)
	for i := range copiedDefinitions {
		copiedDefinitions[i].Arguments = slices.Clone(copiedDefinitions[i].Arguments)
	}
	copiedRuns := slices.Clone(runs)
	for i := range copiedRuns {
		run := &copiedRuns[i]
		run.Args = slices.Clone(run.Args)
		run.Phases = slices.Clone(run.Phases)
		run.Actors = slices.Clone(run.Actors)
		run.Questions = slices.Clone(run.Questions)
		run.Holes = slices.Clone(run.Holes)
		run.Artifacts = slices.Clone(run.Artifacts)
		run.Edges = slices.Clone(run.Edges)
	}
	return copiedDefinitions, copiedRuns
}
