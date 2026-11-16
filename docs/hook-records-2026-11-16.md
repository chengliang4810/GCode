# Hook 执行记录修复（2026-11-16）

## 问题与范围

会话 `r4vn62lsdbs6` 的启动 Hook 把 Superpowers `using-superpowers` 内容通过
`additionalContext` 注入模型上下文。问题在于 `runHookRunner` 同时把原始 stdout
拼成 `kind: "text"` 的助手消息，导致完整 JSON 和转义换行直接铺在对话中。

本次修复输出展示，不禁用 Superpowers，也不改变技能内容或 Hook 的业务决策。
主运行与手动压缩共用 `runHookRunner`；33 种已声明事件、插件/用户/技能规则以及
command/http/prompt/agent/mcp_tool handler 共用其完成回调，因此均有相同的展示问题。
异步通知及唤醒也经过该回调；内部队列按 `Source: "hook"` 投递上下文，不另建可见用户消息。

本机配置只读核查：启用的已安装插件 Hook 为 Superpowers 的 `SessionStart`，
用户设置中的直接 Hook 规则为空。其余事件由代码追踪及隔离测试核验，
没有在用户会话中人为触发全部生命周期事件。

核查另发现 `async`/`asyncRewake` 过滤迟到决策时丢失 `SuppressOutput`，
导致回调忽略输出隐藏要求。本次保留该展示标记，仍丢弃不应追溯生效的后台决策。

## 实现

- 核心保存 `kind: "hook"` 的 `HookRecord`，保留规则、插件、名称、事件、状态、
  脱敏详情及真实退出码。普通回复正文不再承载 Hook 日志。
- 桌面沿用现有活动行和 MyGo 原生折叠控件；默认显示名称、事件及状态。
  阻止和失败有明确状态及错误颜色，详情复用最多 24 行的滚动、选择视口。
- 名称在窄窗收缩，事件及状态保留；中文界面翻译状态，插件名称与事件名保持原值。
- 日志详情不参与复制回复或模型工具调用、进度说明计数；模型上下文注入不变。
- 快照深克隆记录及退出码指针，JSON 持久化及远程序列化保留类型。
- 不解析或改写已保存为普通文本的旧 Hook 消息。

## 验证

已完成：

- `go test ./internal/contracts ./internal/agent ./internal/engine ./internal/desktop
  -run 'Test(Hook|Hooks|TranscriptHook|SettingsControls|InspectSettingsControls)'
  -count=1 -timeout=120s` 通过。
- 记录投影测试遍历 33 个事件与其支持的 handler 类型，检查身份、状态、stdout、
  stderr、脱敏及真实退出码边界。这是确定性结构测试，不冒充所有事件已在产品中真实发生。
- `TestHookAsyncPreservesOutputSuppressionForReports` 通过真实命令子进程验证
  `async` 和 `asyncRewake` 的输出隐藏、报告及后台上下文通知。
- `TestDesktopLocalHookRecords` 深浅主题原生窗口通过。私有真实核心执行启动及 Stop
  命令 Hook，模型使用回环 HTTP 夹具；验证启动上下文送达模型、执行记录落盘、
  原生应用重开后默认折叠、两类 Hook 独立展开及完整长输出独立滚动。
- `ui.Tester` 覆盖深浅主题、360/900 宽、紧凑/普通模式、中文状态、
  Enter/Space、展开状态跨流式完成保持，以及复制回复排除 Hook。
- `go vet ./internal/contracts ./internal/agent ./internal/engine ./internal/desktop` 通过。
- `go tool mygo vet .` 通过。
- `go test -race ./internal/contracts ./internal/agent
  -run 'Test(HookRecord|HookAsyncPreservesOutput)' -count=1 -timeout=90s` 通过。
- Linux amd64 的 `CGO_ENABLED=0 go build ./cmd/gcode-server` 交叉构建通过，
  产物保存在本次输出目录，未部署。

全仓 `CGO_ENABLED=0 go test ./... -timeout=120s` 未全部通过：
desktop 与 engine 触及整包两分钟时限；i18n 的
`TestLiteralInterfaceMessagesHaveTranslations` 报告已有
`internal/desktop/bots_sidebar.go` 中的 `Actions` 缺少翻译。
该 Bots 文件在本任务开始时已为未跟踪文件，本次未修改。
完整日志：`output/hook-records-20261116/go-test-all.log`。

延长时限后的 `CGO_ENABLED=0 go test ./internal/engine ./internal/desktop -timeout=5m`：
desktop 完整包通过（275.293 秒），engine 仍触及整包五分钟时限，不能标为完整通过。
日志：`output/hook-records-20261116/go-test-engine-desktop.log`。
该轮日志还包含修正前编译的订阅测试断言失败；测试现已改为订阅明确的会话作用域，
以符合核心对非当前会话隐藏消息正文的既有规则，单独复跑通过。
最终重新编译的 contracts/agent/engine Hook、用户输入拒绝、队列拒绝及目标拒绝
专项全部通过（engine 30.525 秒）：
`output/hook-records-20261116/go-test-final-hooks.log`。

原生证据目录：
`output/hook-records-20261116/native/20261116-173507-674447918/`。
其中 `evidence.json`、`tests.log` 保存执行结果，light/dark 子目录保存折叠、
展开尾部及 Stop 详情截图和无障碍树；已目视检查浅色折叠与深色长输出截图。

本轮未执行真实外部供应商、远程宿主、系统输入法验收；未替换日常使用的应用，
未迁移用户历史，工作区已有的其他未提交改动保留。
