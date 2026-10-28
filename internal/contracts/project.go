package contracts

type ProjectAction struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Command          string `json:"command"`
	Icon             string `json:"icon"`
	Enabled          bool   `json:"enabled"`
	RunOnNewWorktree bool   `json:"runOnNewWorktree"`
	LastRunStatus    string `json:"lastRunStatus"`
}
