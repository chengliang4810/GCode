package desktop

import (
	"time"

	"gcode/internal/appmodel"
)

func NewVisualSample() *App {
	app := Open(nil, OpenOptions{
		Initial:          SampleSnapshot(),
		InitialWorkspace: SampleWorkspaceSnapshot(),
		VisualSample:     true,
	})
	app.right = rightDiff
	return app
}

func SampleSnapshot() appmodel.Snapshot {
	now := time.Now()
	return appmodel.Snapshot{
		Projects: []appmodel.Project{
			{ID: "project-gcode", Name: "GCode", Path: "/Users/demo/Projects/gcode"},
			{ID: "project-atlas", Name: "Atlas Notes", Path: "/Users/demo/Projects/atlas-notes"},
			{ID: "project-signal", Name: "Signal H5", Path: "/Users/demo/Projects/signal-h5"},
		},
		Settings: appmodel.Settings{
			DefaultProviderID: "provider-openai",
			DefaultSendMode:   appmodel.Correction,
			Theme:             "system",
			Providers: []appmodel.Provider{
				{ID: "provider-openai", Name: "OpenAI API", Format: appmodel.Responses, BaseURL: "https://api.openai.com", Model: "gpt-5.2-codex", HasAPIKey: true},
				{ID: "provider-local", Name: "本地评审", Format: appmodel.ChatCompletions, BaseURL: "http://127.0.0.1:11434", Model: "qwen3-coder", HasAPIKey: true},
			},
		},
		Sessions: []appmodel.Session{
			{
				ID: "session-auth", ProjectID: "project-gcode", Title: "修复设置页保存错误", Cwd: "/Users/demo/Projects/gcode", Branch: "work/settings-fix", ProviderID: "provider-openai", State: appmodel.Running, RunID: "run-27", UpdatedAt: now.Add(-2 * time.Minute),
				Messages: []appmodel.Message{
					{ID: "m1", Role: "user", Text: "检查设置页保存逻辑，修复错误后运行相关测试。", CreatedAt: now.Add(-4 * time.Minute)},
					{ID: "m2", Role: "assistant", Text: "我先核对保存路径和当前测试覆盖，再给出最小修改。\n\n这次会重点确认：\n\n- 草稿不会覆盖正在编辑的字段\n- 保存完成后模型选择仍然可见\n- 失败时保留可恢复的错误提示\n\n参考 [Go 错误处理指南](https://go.dev/blog/go1.13-errors)，我会把底层错误保留在 `%w` 链路里。\n\n~~~go\nfunc (a *App) saveSettings(ctx context.Context) error {\n\tif err := a.store.Write(ctx, a.settings); err != nil {\n\t\treturn fmt.Errorf(\"save settings: %w\", err)\n\t}\n\treturn nil\n}\n~~~", CreatedAt: now.Add(-3 * time.Minute), Tools: []appmodel.ToolCall{
						{ID: "tool-1", Name: "读取文件", Arguments: "internal/settings/store.go", Output: "已读取 184 行", State: "done"},
						{ID: "tool-2", Name: "运行测试", Arguments: `{"cmd":"go test ./internal/settings/...","workdir":"/Users/demo/Projects/gcode"}`, Output: "正在执行…", State: "running"},
					}},
					{ID: "m3", Role: "assistant", Text: "保存入口已经定位到 `settings_controller.go`。下一步会补一个回归测试，验证服务端返回错误时草稿仍能继续编辑。", CreatedAt: now.Add(-45 * time.Second)},
				},
			},
			{
				ID: "session-permission", ProjectID: "project-gcode", Title: "整理工作区文档", Cwd: "/Users/demo/Projects/gcode", Branch: "docs/cleanup", ProviderID: "provider-openai", State: appmodel.WaitingPermission, UpdatedAt: now.Add(-9 * time.Minute),
				Permissions: []appmodel.Permission{{ID: "permission-1", RunID: "run-18", Tool: "运行命令", Arguments: "git diff --check", Options: []appmodel.PermissionOption{{ID: "allow-once", Label: "本次允许"}, {ID: "deny", Label: "拒绝"}}}},
			},
			{
				ID: "session-failed", ProjectID: "project-gcode", Title: "升级依赖并验证构建", Cwd: "/Users/demo/Projects/gcode", Branch: "work/dependency-refresh", ProviderID: "provider-openai", State: appmodel.Failed, Error: "测试命令退出码为 1", UpdatedAt: now.Add(-24 * time.Minute),
				Queue: []appmodel.QueuedMessage{{ID: "queued-1", Text: "补充失败原因和修复建议", Mode: appmodel.Queue, CreatedAt: now.Add(-1 * time.Minute)}},
			},
			{
				ID: "session-runtime", ProjectID: "project-gcode", Title: "检查会话恢复与流式更新", Cwd: "/Users/demo/Projects/gcode", Branch: "main", ProviderID: "provider-local", State: appmodel.Idle, UpdatedAt: now.Add(-52 * time.Minute),
				Messages: []appmodel.Message{{ID: "m4", Role: "assistant", Text: "恢复测试覆盖了关闭窗口、重新打开和队列继续执行三个路径。", CreatedAt: now.Add(-52 * time.Minute)}},
			},
			{
				ID: "session-atlas", ProjectID: "project-atlas", Title: "重排知识库索引", Cwd: "/Users/demo/Projects/atlas-notes", Branch: "search-v2", ProviderID: "provider-local", State: appmodel.Running, UpdatedAt: now.Add(-2 * time.Hour),
				Messages: []appmodel.Message{{ID: "m5", Role: "assistant", Text: "索引器正在合并重复标签，并保留原始文档的更新时间。", CreatedAt: now.Add(-2 * time.Hour)}},
			},
			{
				ID: "session-atlas-ui", ProjectID: "project-atlas", Title: "调整阅读页快捷键", Cwd: "/Users/demo/Projects/atlas-notes", Branch: "keyboard-nav", ProviderID: "provider-local", State: appmodel.Idle, UpdatedAt: now.Add(-5 * time.Hour),
			},
			{
				ID: "session-signal", ProjectID: "project-signal", Title: "核对告警聚合规则", Cwd: "/Users/demo/Projects/signal-h5", Branch: "alerts", ProviderID: "provider-openai", State: appmodel.Interrupted, UpdatedAt: now.Add(-26 * time.Hour),
			},
			{
				ID: "session-signal-api", ProjectID: "project-signal", Title: "补齐 webhook 重试测试", Cwd: "/Users/demo/Projects/signal-h5", Branch: "webhook-retry", ProviderID: "provider-openai", State: appmodel.Idle, UpdatedAt: now.Add(-3 * 24 * time.Hour),
			},
		},
	}
}

func SampleWorkspaceSnapshot() WorkspaceSnapshot {
	return WorkspaceSnapshot{
		Files: []appmodel.FileEntry{
			{Name: "cmd", Path: "cmd", IsDir: true},
			{Name: "internal", Path: "internal", IsDir: true},
			{Name: "go.mod", Path: "go.mod", Size: 892},
			{Name: "settings_controller.go", Path: "internal/settings/settings_controller.go", Size: 4260},
			{Name: "store.go", Path: "internal/settings/store.go", Size: 5080},
			{Name: "store_test.go", Path: "internal/settings/store_test.go", Size: 3310},
		},
		Preview: &appmodel.FilePreview{Path: "internal/settings/store.go", Text: "package settings\n\nfunc (s *Store) Write(ctx context.Context, value Settings) error {\n\tif err := s.file.WriteJSON(ctx, value); err != nil {\n\t\treturn fmt.Errorf(\"save settings: %w\", err)\n\t}\n\treturn nil\n}\n", Truncated: false},
		Git: appmodel.GitState{
			IsGit: true, Branch: "work/settings-fix",
			Changes: []appmodel.Change{
				{Path: "internal/settings/store.go", Status: "M", Diff: "@@ -42,6 +42,10 @@ func (s *Store) Write\n func (s *Store) Write(ctx context.Context, value Settings) error {\n-\treturn s.file.WriteJSON(value)\n+\tif err := s.file.WriteJSON(ctx, value); err != nil {\n+\t\treturn fmt.Errorf(\"save settings: %w\", err)\n+\t}\n+\treturn nil\n }"},
				{Path: "internal/settings/store_test.go", Status: "M", Diff: "@@ -18,3 +18,15 @@ func TestStoreWrite\n func TestStoreWrite(t *testing.T) {\n \tt.Parallel()\n }\n+\n+func TestStoreWriteKeepsContext(t *testing.T) {\n+\tstore := newTestStore(t)\n+\terr := store.Write(context.Background(), Settings{Theme: \"dark\"})\n+\tif err != nil {\n+\t\tt.Fatalf(\"write settings: %v\", err)\n+\t}\n+}"},
			},
		},
	}
}
