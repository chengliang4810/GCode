# /goal：GCode 原生桌面 UI

状态：进行中。所有实现位于 `internal/desktop/`，只消费 `internal/appmodel` 的共享 DTO；UI 不持有 `*ui.Context`、`ui.Element` 或后端状态引用。

## 已完成

- 使用 Go + mygo `ui.View` 构建中文三栏工作台：项目/会话、对话/工具/权限/输入、文件/Diff/终端。
- 统一浅色、深色和跟随系统主题，使用中性阅读区域、蓝色强调色、8 DIP 控件圆角和 4/8/12/16/24 间距尺度。
- 对话展示用户/Agent 消息、代码块、工具状态、复制、授权卡片、Agent 提问、纠正/队列发送、停止、待执行消息撤回和暂停队列继续。
- 对话消息使用 mygo `ui.ListState` 虚拟化：每个会话独立保存滚动位置，默认跟随底部，用户上滑后新消息和流式末行增长不抢位置，稳定消息 ID 支持历史消息 prepend 锚定；可见消息的 Markdown 结构和行内样式按会话/主题缓存。
- 设置页覆盖主题、默认发送方式、多个 API 服务、API 类型、Base URL、模型和密钥保存入口；Snapshot 不承载密钥。
- 文件、只读预览、Git Diff 和真实 mygo terminal 插件适配；终端按会话 cwd 异步创建，切换 cwd 关闭旧实例，窗口关闭释放终端。
- 提供 `New`、`Open`、`View`、`Refresh`、`Update`、`UpdateWorkspace`、`Close`，后端通过最小 `Service` 接口注入。
- `NewVisualSample`、`SampleSnapshot` 和 `SampleWorkspaceSnapshot` 明确标记为“示例数据 · 仅视觉验证”，不会成为真实后端默认数据。

## 后端接口草案

`Service` 直接使用 `internal/appmodel`：

```go
Snapshot(context.Context) (appmodel.Snapshot, error)
AddProject(context.Context, string) error
NewSession(context.Context, appmodel.NewSessionOptions) error
Send(context.Context, sessionID, text string, mode appmodel.SendMode) error
Stop(context.Context, sessionID string) error
Withdraw(context.Context, sessionID, messageID string) error
ResumeQueue(context.Context, sessionID string) error
SetProvider(context.Context, sessionID, providerID string) error
SetFullAccess(context.Context, sessionID string, enabled bool) error
ReplyPermission(context.Context, sessionID, requestID, optionID string) error
ReplyAsk(context.Context, sessionID, requestID string, options []string) error
SaveSettings(context.Context, appmodel.Settings) error
SaveProvider(context.Context, appmodel.Provider, key string) error
RemoveProvider(context.Context, providerID string) error
Close(context.Context) error
```

工作区是独立接口：`Snapshot(context.Context, sessionID string) (WorkspaceSnapshot, error)`。终端由 `TerminalFactory` 创建，`NativeTerminalFactory` 已接到 mygo `plugins/terminal.New` 和 `terminal.View`。

controller 发布的新 `appmodel.Snapshot` 应在 `Window.Update` 内调用 `App.Update`；异步工作区结果也必须由 controller 或窗口更新入口按 session ID 校验后调用 `UpdateWorkspace`。UI 内部仍使用互斥保护，避免迟到 goroutine 破坏视图状态。

## 验证

- 当前本地 Go 版本通过隔离模块使用缓存的 mygo 源码执行 `go test ./internal/desktop`。
- `ui.NewTester` 已覆盖 1280×800 浅色、深色刷新和 860×620 窄窗口；检查示例标记、会话内容、Diff 标签、设置页往返和非空渲染像素。
- 对话虚拟化测试覆盖 5000 条消息的可见行范围、流式末行增长、上滑后追加消息、会话间独立位置和历史 prepend 锚定；详见 `docs/verification/stage-4-conversation.md`。
- 尚未替代 root 的原生窗口启动、标题栏、中文输入法、terminal library 打包及真实后端事件验证；这些属于入口和运行时集成验收。

## 待 root 集成

1. controller 实现 `Service`，首屏调用 `Refresh` 或直接把初始 `Snapshot` 传入 `OpenOptions.Initial`。
2. 用 `Window.Update` 投影 controller 会话事件，避免直接从 UI goroutine 触碰共享 controller 状态。
3. 创建窗口时传入 `desktop.NativeTerminalFactory{}` 和 workspace 适配器。
4. 在 macOS 原生窗口中验证 1280×800、1024×700、窄窗口、系统浅深主题、中文输入、终端 cwd/resize/关闭清理。

## 2026-10-12 用户反馈后的项目树与空间布局

用户已反馈 visual-v3 的布局和空间细节仍需调整，并明确左侧结构为“项目栏目 → 项目名称 → 多个对话”。已实现可折叠项目树及栏宽、标题、缩进、行高、共用阅读列调整；`visual-v4/confirmed` 保存 7 张实际原生截图，项目独立折叠、重新展开与跨项目对话切换 AX 检查通过，仍等待用户视觉评价。

当前 canonical 包为 `project-tree-layout-confirmed`：executable `00b310de56c927242e8a0cf95bb31807cc251dbf0e0b3fa91b2d29e4c7e965e5`、code `19ba9bb09ed4c91335896ec399ab11ffb6776553cfb457f76d9243662b6f1e44`、production/test fingerprint `12718290f02ee141239bed0b1b8cd431ef0670fbe6d473131b37f07db3d1ef63` / `2b80a01449cc0e86712441d51e55e9e8c04e56902e8698256851b317b8d1cbe5`，生产/测试文件 832/422，构建前后清单未变。全仓测试、受影响模块 race、vet、fmt、package 和 codesign 六项均 exit 0。 最新证据见 [visual-v4.md](../verification/visual-v4.md) 和 [current-package.md](../verification/current-package.md)。已保留历史视觉与包身份，未声称用户认可或首版交付完成。
