package contracts

func DefaultSettings() Settings {
	settings := Settings{
		Subagents:     DefaultSubagentProfiles(),
		Appearance:    AppearanceSettings{RenderMath: true, MathFont: "Fira Math", MathScale: 100, Mode: "dark", ThemeID: "rcode-dark", LightThemeID: "rcode-light", DarkThemeID: "rcode-dark", Accent: "theme", EffortTheme: EffortThemePurple, Surface: "theme", Font: "Geist", FontSize: 14, CodeFont: "Geist Mono", CodeFontSize: 12, TerminalFont: "Geist Mono", TerminalFontSize: 13, TranscriptWidth: 736, BackgroundEffect: "none"},
		General:       GeneralSettings{Language: "zh-CN", SendBehavior: "enter", ActiveTurnSendBehavior: "steer", StopWithEscape: true, ThreadNamingModel: "current-model"},
		Notifications: NotificationSettings{Errors: true, Enabled: true, Completion: true, Questions: true, Sound: false},
		Files:         FileSettings{Autosave: true, AutosaveDelay: 900, LineNumbers: true, FontSize: 12},
		Appshots:      AppshotSettings{Destination: "lastSession", Shortcut: "Cmd+Shift+2", IncludeCursor: false},
		Shortcuts: []Shortcut{
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
	settings.Appearance.BackgroundAdjustment = BackgroundAdjustment{FocalX: .5, FocalY: .5, Zoom: 1}
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
