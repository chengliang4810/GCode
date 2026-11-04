package mock

import (
	"fmt"
	"strings"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/i18n"
)

func DefaultSettings() contracts.Settings {
	settings := contracts.Settings{
		Appearance:    contracts.AppearanceSettings{RenderMath: true, MathFont: "Fira Math", MathScale: 100, Mode: "dark", ThemeID: "rcode-dark", LightThemeID: "rcode-light", DarkThemeID: "rcode-dark", Accent: "theme", EffortTheme: contracts.EffortThemePurple, Surface: "theme", Font: "Geist", FontSize: 14, CodeFont: "Geist Mono", CodeFontSize: 12, TerminalFont: "Geist Mono", TerminalFontSize: 13, TranscriptWidth: 736, BackgroundEffect: "none"},
		General:       contracts.GeneralSettings{Language: i18n.Chinese, SendBehavior: "enter", ActiveTurnSendBehavior: "steer", StopWithEscape: true, ThreadNamingModel: "current-model"},
		Notifications: contracts.NotificationSettings{Errors: true, Enabled: true, Completion: true, Questions: true, Sound: false},
		Files:         contracts.FileSettings{Autosave: true, AutosaveDelay: 900, LineNumbers: true, FontSize: 12},
		Appshots:      contracts.AppshotSettings{Destination: "lastSession", Shortcut: "Cmd+Shift+2", IncludeCursor: false},
		Shortcuts: []contracts.Shortcut{
			{ID: "newSession", Label: "New session", Keys: "Mod+N", Group: "Navigation"},
			{ID: "commandPalette", Label: "Command palette", Keys: "Mod+K", Group: "Navigation"},
			{ID: "settings", Label: "Open settings", Keys: "Mod+,", Group: "Navigation"},
			{ID: "toggleSidebar", Label: "Toggle sidebar", Keys: "Mod+B", Group: "Panels"},
			{ID: "files", Label: "Toggle files", Keys: "Mod+Shift+E", Group: "Panels"},
			{ID: "terminal", Label: "Toggle terminal", Keys: "Mod+J", Group: "Panels"},
			{ID: "find", Label: "Find in conversation", Keys: "Mod+F", Group: "Conversation"},
			{ID: "stop", Label: "Stop agent", Keys: "Escape", Group: "Conversation"},
		}, SidebarWidth: 256, FilesWidth: 286, RightPaneWidth: 520, TerminalHeight: 280, SidebarGrouping: "projects", ShowIcons: true,
	}
	settings.Appearance.MotionPreference = "system"
	settings.Appearance.BackgroundAdjustment = contracts.BackgroundAdjustment{FocalX: .5, FocalY: .5, Zoom: 1}
	settings.Appearance.Mode = "system"
	settings.Appearance.ThemeID = ""
	settings.General.StopWithEscape = false
	settings.Files.Autosave = false
	settings.Appshots.Sound = true
	settings.Appshots.Destination = "automatic"
	settings.Notifications.Sound = true
	settings.Appearance.PauseInBackground = false
	return settings
}

func seed(opts Options) contracts.Snapshot {
	now := opts.Now().UTC()
	s := contracts.Snapshot{SchemaVersion: contracts.SchemaVersion, Revision: 1,
		ModelCatalog:        contracts.ModelCatalogState{State: contracts.CatalogReady},
		AppshotCapabilities: appshotCapabilities(opts.Platform),
		User:                contracts.User{ID: "alex", Name: "Alex", Email: "alex@example.test", Initials: "A", SignedIn: true},
		Capabilities:        contracts.Capabilities{Appshots: opts.Platform == "darwin", Cloud: true, Browser: true},
		Settings:            DefaultSettings(), Messages: make(map[string][]contracts.Message),
		Projects: []contracts.Project{
			{ID: "fieldnotes", Name: "Fieldnotes", Path: "~/projects/fieldnotes", DeviceID: "local", Branch: "main", Branches: []string{"main", "fieldnotes/workspace", "fieldnotes/settings-search"}, Worktrees: []contracts.Worktree{{ID: "checkout", Name: "Current checkout", Path: "~/projects/fieldnotes", Branch: "main"}, {ID: "workspace-tree", Name: "Workspace design", Path: "~/worktrees/fieldnotes-workspace", Branch: "fieldnotes/workspace"}}},
			{ID: "rcode", Name: "GCode", Path: "~/projects/rcode", DeviceID: "local", Branch: "main", Branches: []string{"main", "refine-composer", "native-workspace"}},
			{ID: "backend", Name: "API server", Path: "/projects/backend", DeviceID: "remote", Branch: "main", Branches: []string{"main", "api/auth"}},
		},
		Devices: []contracts.Device{{ID: "local", Name: "This device", Platform: opts.Platform, Architecture: opts.Architecture, Status: "connected", Current: true, LastSeen: now}, {ID: "macbook", Name: "MacBook Pro", Platform: "darwin", Architecture: "arm64", Status: "connected", LastSeen: now.Add(-2 * time.Minute)}, {ID: "remote", Name: "Build server", Platform: "linux", Architecture: "amd64", Status: "offline", LastSeen: now.Add(-2 * time.Hour)}},
		Models: []contracts.Model{
			{ID: "claude-opus-4-6", ProviderID: "claude", Label: "Opus 4.6", Description: "Most capable for complex work", ReasoningLevels: []string{"Low", "Medium", "High", "Max"}, ContextLimit: 200000},
			{ID: "claude-sonnet-4-6", ProviderID: "claude", Label: "Sonnet 4.6", Description: "Fast and capable for everyday work", ReasoningLevels: []string{"Low", "Medium", "High"}, ContextLimit: 200000},
			{ID: "gpt-5.4", ProviderID: "openai", Label: "GPT-5.4", Description: "Reasoning and coding", ReasoningLevels: []string{"Low", "Medium", "High", "Extra high"}, ContextLimit: 1000000},
			{ID: "gpt-5.3-codex", ProviderID: "openai", Label: "GPT-5.3 Codex", Description: "Designed for agentic coding", ReasoningLevels: []string{"Low", "Medium", "High", "Extra high"}, ContextLimit: 400000},
			{ID: "cursor-auto", ProviderID: "cursor", Label: "Auto", Description: "A model selected for the task", ReasoningLevels: []string{"Auto"}, ContextLimit: 200000},
		},
		Providers: []contracts.Provider{{ID: "claude", Name: "Claude Code", Status: "connected", Account: "alex@example.test", Enabled: true, Models: []string{"claude-opus-4-6", "claude-sonnet-4-6"}}, {ID: "openai", Name: "Codex", Status: "connected", Account: "alex@example.test", Enabled: true, Models: []string{"gpt-5.4", "gpt-5.3-codex"}}, {ID: "cursor", Name: "Cursor", Status: "notConnected", Enabled: false, Models: []string{"cursor-auto"}}},
		Account:   contracts.Account{Plan: "Pro", CloudStatus: "connected", StorageUsed: 214748364, StorageLimit: 10737418240, Credits: 24.50, Usage: []contracts.UsageWindow{{Label: "5 hour", UsedPercent: 24, ResetsAt: now.Add(3 * time.Hour)}, {Label: "Weekly", UsedPercent: 38, ResetsAt: now.Add(120 * time.Hour)}}, Invoices: []contracts.Invoice{{ID: "invoice-202610", Date: now.Add(-20 * 24 * time.Hour), Amount: 20, Status: "paid"}, {ID: "invoice-202609", Date: now.Add(-50 * 24 * time.Hour), Amount: 20, Status: "paid"}}},
		Update:    contracts.Update{CurrentVersion: "0.1.0", Status: "upToDate"},
	}
	titles := []string{"Build the Fieldnotes workspace", "Polish the command palette", "Fix authentication redirects", "Add deployment status", "Review pull request comments", "Improve keyboard navigation", "Update project documentation", "Refine composer spacing", "Audit chat sync", "Build settings search"}
	for i, title := range titles {
		id := fmt.Sprintf("chat-%d", i)
		project := "fieldnotes"
		if i == 6 || i == 7 {
			project = "rcode"
		}
		if i == 8 {
			project = "backend"
		}
		updated := now.Add(-time.Duration(i*17) * time.Minute)
		s.Sessions = append(s.Sessions, contracts.Session{ID: id, Title: title, ProjectID: project, DeviceID: "local", ModelID: "claude-opus-4-6", Branch: "fieldnotes/" + strings.ReplaceAll(strings.ToLower(title), " ", "-"), Status: contracts.Idle, Pinned: i < 2, PinOrder: i, Unread: i == 3, Archived: i == 9, CreatedAt: updated.Add(-time.Hour), UpdatedAt: updated, LastOutcome: "completed"})
	}
	seedAdditionalProviders(&s)
	// One deterministic group demonstrates the grouped sidebar view.
	s.SessionGroups = []contracts.SessionGroup{{ID: "group-research", Title: "Research", Order: 0}}
	for i := range s.Sessions {
		if s.Sessions[i].ID == "chat-2" || s.Sessions[i].ID == "chat-3" {
			s.Sessions[i].GroupID = "group-research"
			s.Sessions[i].GroupOrder = len(s.Sessions[i].ID) % 2
		}
	}
	s.Files = seedFiles()
	s.Changes = []contracts.Change{{ID: "change-layout", ProjectID: "fieldnotes", Path: "internal/layout.go", Status: "modified", Additions: 12, Deletions: 4, OldContent: "package layout\n\nconst SidebarWidth = 240\nconst PanelWidth = 480\n", NewContent: "package layout\n\nconst SidebarWidth = 256\nconst PanelWidth = 520\n\nfunc ContentWidth(viewport float64) float64 {\n\treturn viewport - SidebarWidth - PanelWidth\n}\n", Hunks: []contracts.DiffHunk{{Header: "@@ -1,4 +1,8 @@", OldStart: 1, NewStart: 1, Lines: []contracts.DiffLine{{Kind: "context", Text: "package layout", OldLine: 1, NewLine: 1}, {Kind: "context", Text: "", OldLine: 2, NewLine: 2}, {Kind: "remove", Text: "const SidebarWidth = 240", OldLine: 3}, {Kind: "add", Text: "const SidebarWidth = 256", NewLine: 3}, {Kind: "remove", Text: "const PanelWidth = 480", OldLine: 4}, {Kind: "add", Text: "const PanelWidth = 520", NewLine: 4}, {Kind: "add", Text: "", NewLine: 5}, {Kind: "add", Text: "func ContentWidth(viewport float64) float64 {", NewLine: 6}, {Kind: "add", Text: "\treturn viewport - SidebarWidth - PanelWidth", NewLine: 7}, {Kind: "add", Text: "}", NewLine: 8}}}}}, {ID: "change-readme", ProjectID: "fieldnotes", Path: "README.md", Status: "modified", Additions: 3, Deletions: 1, OldContent: "# Fieldnotes\n\nA place for ideas.\n", NewContent: "# Fieldnotes\n\nMake room for good ideas.\n\nA calm workspace for your projects.\n"}, {ID: "change-theme", ProjectID: "fieldnotes", Path: "internal/theme.go", Status: "added", Additions: 18, NewContent: "package theme\n\nconst Accent = \"#8b84ff\"\n"}}
	for i, title := range []string{"Refine workspace panel layout", "Preserve focus when switching tabs", "Add session search", "Update theme tokens", "Initial workspace"} {
		sha := fmt.Sprintf("%040x", 1000-i)
		parents := []string{}
		if i < 4 {
			parents = append(parents, fmt.Sprintf("%040x", 999-i))
		}
		refs := []string{}
		if i == 0 {
			refs = []string{"main", "origin/main"}
		}
		s.Commits = append(s.Commits, contracts.Commit{SHA: sha, Parents: parents, Subject: title, Author: "Alex", AuthoredAt: now.Add(-time.Duration(i+1) * time.Hour), Refs: refs})
	}
	s.Terminals = []contracts.Terminal{{ID: "terminal-1", ProjectID: "fieldnotes", Title: "Terminal", CWD: "~/projects/fieldnotes", Columns: 80, Rows: 24, Status: "idle", Output: "\x1b[38;2;139;132;255m~/projects/fieldnotes\x1b[0m \x1b[32mmain\x1b[0m\r\n❯ go test ./...\r\n\x1b[32mok\x1b[0m  fieldnotes/internal/layout  0.012s\r\n\x1b[32mok\x1b[0m  fieldnotes/internal/session  0.008s\r\n\r\n\x1b[38;2;139;132;255m❯\x1b[0m "}}
	for i := range s.Sessions {
		s.Sessions[i].ReasoningLevel = "High"
		for j := range s.Projects {
			if s.Projects[j].ID == s.Sessions[i].ProjectID {
				branch := s.Sessions[i].Branch
				exists := false
				for _, b := range s.Projects[j].Branches {
					if b == branch {
						exists = true
						break
					}
				}
				if !exists {
					s.Projects[j].Branches = append(s.Projects[j].Branches, branch)
				}
			}
		}
	}
	// The editor and diff are two views of the same virtual file state.
	for i := range s.Changes {
		change := &s.Changes[i]
		for fi := range s.Files {
			file := &s.Files[fi]
			if change.ProjectID == file.ProjectID && change.Path == file.Path {
				change.NewContent = file.Content
				change.Additions, change.Deletions, change.Hunks = contiguousDiff(change.OldContent, change.NewContent)
				file.Modified = true
				break
			}
		}
	}
	seedAgentTranscripts(&s)
	applyScenario(&s, opts)
	reconcileAgentTurns(&s, opts)
	return s
}

func seedFiles() []contracts.File {
	return []contracts.File{
		{ID: "dir-cmd", ProjectID: "fieldnotes", Name: "cmd", Path: "cmd", Kind: "directory"},
		{ID: "dir-app", ProjectID: "fieldnotes", Name: "fieldnotes", Path: "cmd/fieldnotes", Kind: "directory", ParentID: "dir-cmd"},
		{ID: "file-main", ProjectID: "fieldnotes", Name: "main.go", Path: "cmd/fieldnotes/main.go", ParentID: "dir-app", Kind: "file", Language: "go", Version: 1, Content: "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"Make room for good ideas.\")\n}\n"},
		{ID: "dir-internal", ProjectID: "fieldnotes", Name: "internal", Path: "internal", Kind: "directory"},
		{ID: "file-layout", ProjectID: "fieldnotes", Name: "layout.go", Path: "internal/layout.go", ParentID: "dir-internal", Kind: "file", Language: "go", Version: 1, Modified: true, Content: "package layout\n\nconst (\n\tSidebarWidth = 256\n\tContentWidth = 736\n\tPanelWidth = 520\n)\n\nfunc ContentWidthFor(viewport float64) float64 {\n\treturn viewport - SidebarWidth - PanelWidth\n}\n"},
		{ID: "file-theme", ProjectID: "fieldnotes", Name: "theme.go", Path: "internal/theme.go", ParentID: "dir-internal", Kind: "file", Language: "go", Version: 1, Modified: true, Content: "package theme\n\nconst Accent = \"#8b84ff\"\n"},
		{ID: "dir-resources", ProjectID: "fieldnotes", Name: "resources", Path: "resources", Kind: "directory"},
		{ID: "file-readme", ProjectID: "fieldnotes", Name: "README.md", Path: "README.md", Kind: "file", Language: "markdown", Version: 1, Content: "# Fieldnotes\n\nMake room for good ideas.\n\n## A calmer workspace\n\nCollect your thoughts in one place.\n\n- Organize work by project\n- Keep context close\n- Review changes with confidence\n\n| Feature | Status |\n| --- | --- |\n| Native workspace | Ready |\n| Keyboard navigation | Ready |\n| Project groups | Ready |\n"},
		{ID: "file-gomod", ProjectID: "fieldnotes", Name: "go.mod", Path: "go.mod", Kind: "file", Language: "go", Version: 1, Content: "module fieldnotes\n\ngo 1.27.3\n"},
		{ID: "file-lock", ProjectID: "fieldnotes", Name: "go.sum", Path: "go.sum", Kind: "file", Language: "text", Version: 1, ReadOnly: true, Content: "# Dependency checksums are managed by the package manager.\n"},
		{ID: "file-gitignore", ProjectID: "fieldnotes", Name: ".gitignore", Path: ".gitignore", Kind: "file", Language: "text", Version: 1, Content: "build/\n*.exe\n"},
	}
}

func applyScenario(s *contracts.Snapshot, opts Options) {
	switch opts.Scenario {
	case "showcase", "transcript":
		s.Messages["chat-0"] = append(s.Messages["chat-0"], contracts.Message{ID: "showcase-message", SessionID: "chat-0", Role: "assistant", CreatedAt: opts.Now().UTC(), Blocks: []contracts.Block{
			{ID: "showcase-code", Kind: "code", Language: "go", Text: "type Workspace struct {\n    SidebarWidth float64\n    ContentWidth float64\n}\n"},
			{ID: "showcase-question", Kind: "question", Question: fixtureQuestion("question-showcase", "Which panel should open first?", "Files", "Changes", "Browser")},
			{ID: "showcase-permission", Kind: "permission", Permission: &contracts.Permission{ID: "permission-showcase", Title: "Run project checks", Description: "Allow this command in the current workspace?", Command: "go test ./...", Answer: "Allow once"}},
			{ID: "showcase-attachment", Kind: "attachment", Attachment: &contracts.Attachment{ID: "attachment-readme", Name: "README.md", MIME: "text/markdown", Size: 512, AssetRef: "file-readme", Status: "ready"}},
			{ID: "showcase-image", Kind: "image", Attachment: &contracts.Attachment{ID: "attachment-icon", Name: "GCode.png", MIME: "image/png", Width: 1024, Height: 1024, AssetRef: "asset://app-icon.png", Status: "ready"}},
			{ID: "showcase-math", Kind: "math", Artifact: artifactFor("math", s.Settings.Appearance)},
			{ID: "showcase-diagram", Kind: "diagram", Artifact: artifactFor("diagram", s.Settings.Appearance)},
		}})
		extendTranscriptScene(s, opts)
	case "empty":
		s.Sessions = []contracts.Session{}
		s.Messages = make(map[string][]contracts.Message)
		s.Queue = nil
		s.Runs = nil
	case "live":
		s.Sessions[0].Status = contracts.Working
		s.Sessions[0].LastOutcome = ""
		started := opts.Now().UTC()
		s.Messages["chat-0"] = append(s.Messages["chat-0"], contracts.Message{ID: "live-user", SessionID: "chat-0", TurnID: "live-turn", Role: "user", CreatedAt: started, Blocks: []contracts.Block{{ID: "live-prompt", Kind: "text", Text: "Verify the workspace panel constraints"}}})
		s.Messages["chat-0"] = append(s.Messages["chat-0"], contracts.Message{ID: "live-message", SessionID: "chat-0", TurnID: "live-turn", Role: "assistant", Streaming: true, CreatedAt: started, Blocks: []contracts.Block{{ID: "live-reply", Kind: "text"}}})
	case "loading":
		s.Sessions[0].Status = contracts.Working
		s.Messages["chat-0"] = []contracts.Message{{ID: "loading-message", SessionID: "chat-0", Role: "assistant", Streaming: true, CreatedAt: opts.Now().UTC(), Blocks: []contracts.Block{{ID: "loading-text", Kind: "text", Text: "Reviewing the workspace…"}}}}
	case "error":
		s.Sessions[0].Status = contracts.Errored
		s.Sessions[0].LastOutcome = "failed"
		s.Messages["chat-0"] = append(s.Messages["chat-0"], contracts.Message{ID: "error-message", SessionID: "chat-0", Role: "assistant", CreatedAt: opts.Now().UTC(), Blocks: []contracts.Block{{ID: "error-block", Kind: "error", Text: "The request could not be completed. Please try again."}}})
	case "offline":
		for i := range s.Devices {
			s.Devices[i].Status = "offline"
		}
		s.Account.CloudStatus = "disconnected"
	case "questions":
		s.Sessions[0].Status = contracts.AwaitingInput
		s.Messages["chat-0"] = append(s.Messages["chat-0"], contracts.Message{ID: "question-message", SessionID: "chat-0", Role: "assistant", CreatedAt: opts.Now().UTC(), Blocks: []contracts.Block{{ID: "question-block", Kind: "question", Question: fixtureQuestion("question-layout", "Which layout should we use?", "Keep the current layout", "Use a wider workspace", "Compare both approaches")}}})
	case "permissions":
		s.Sessions[0].Status = contracts.AwaitingInput
		s.Messages["chat-0"] = append(s.Messages["chat-0"], contracts.Message{ID: "permission-message", SessionID: "chat-0", Role: "assistant", CreatedAt: opts.Now().UTC(), Blocks: []contracts.Block{{ID: "permission-block", Kind: "permission", Permission: &contracts.Permission{ID: "permission-tests", Title: "Run project checks", Description: "Allow the agent to run the project's test suite?", Command: "go test ./..."}}}})
	case "stress":
		for i := len(s.Sessions); i < 1000; i++ {
			id := fmt.Sprintf("stress-%d", i)
			s.Sessions = append(s.Sessions, contracts.Session{ID: id, Title: fmt.Sprintf("Workspace review %d", i+1), ProjectID: "fieldnotes", DeviceID: "local", ModelID: "claude-opus-4-6", Status: contracts.Idle, UpdatedAt: opts.Now().UTC().Add(-time.Duration(i) * time.Minute)})
		}
		for i := len(s.Messages["chat-0"]); i < 5000; i++ {
			s.Messages["chat-0"] = append(s.Messages["chat-0"], contracts.Message{ID: fmt.Sprintf("stress-message-%d", i), SessionID: "chat-0", Role: []string{"user", "assistant"}[i%2], CreatedAt: opts.Now().UTC(), Blocks: []contracts.Block{{ID: fmt.Sprintf("stress-block-%d", i), Kind: "text", Text: fmt.Sprintf("Review item %d. Preserve the current layout, keyboard focus, and scroll position while updating the workspace.", i+1)}}})
		}
	}
}

func fixtureQuestion(id, title string, options ...string) *contracts.Question {
	page := contracts.QuestionPage{Question: title, Header: "Layout"}
	for _, label := range options {
		page.Options = append(page.Options, contracts.QuestionOption{Label: label})
	}
	question := &contracts.Question{ID: id, Pages: []contracts.QuestionPage{page}}
	if id == "question-showcase" {
		question.Resolved = true
		question.Answers = []contracts.QuestionAnswer{{Options: []string{options[0]}}}
	}
	return question
}
