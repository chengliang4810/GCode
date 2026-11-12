package engine

import (
	"slices"

	"gcode/internal/contracts"
)

// persistedSnapshot contains owned settings and identities, never refreshable
// file trees or conversation bodies. Conversation records commit separately.
type persistedSnapshot struct {
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
	Terminals        []contracts.Terminal                     `json:"terminals"`
	Attachments      []contracts.Attachment                   `json:"attachments"`
	Appshots         []contracts.Attachment                   `json:"appshots"`
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
	return persistedSnapshot{
		Metrics: snapshot.Metrics, ApplicationUsage: snapshot.ApplicationUsage,
		SchemaVersion: snapshot.SchemaVersion, Revision: snapshot.Revision,
		User: snapshot.User, Projects: projects, Sessions: sessions,
		SessionGroups: snapshot.SessionGroups, Settings: snapshot.Settings, Runs: runs,
		Terminals: snapshot.Terminals, Attachments: snapshot.Attachments,
		Appshots: snapshot.Appshots,
	}
}

func (disk persistedSnapshot) restore(defaults contracts.Snapshot) contracts.Snapshot {
	defaults.Metrics, defaults.ApplicationUsage = disk.Metrics, disk.ApplicationUsage
	defaults.SchemaVersion, defaults.Revision = disk.SchemaVersion, disk.Revision
	defaults.User, defaults.Settings = disk.User, disk.Settings
	defaults.Projects, defaults.Sessions = disk.Projects, disk.Sessions
	defaults.SessionGroups = disk.SessionGroups
	defaults.Terminals = disk.Terminals
	defaults.Runs = disk.Runs
	for i := range defaults.Runs {
		if defaults.Runs[i].Status == "running" {
			defaults.Runs[i].Status = "interrupted"
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
