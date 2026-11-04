# 统一模型重试

模型层拥有唯一重试循环，三种协议和桌面、无桌面宿主共用。
每个逻辑模型请求首次请求后最多追加重试 10 次，共 11 次物理尝试；
下一次模型步骤重新计数。不是整轮 Agent 或整段会话共用 10 次，
也不是连接、HTTP、断流各自再允许 10 次。

## 策略与分类

`internal/model/retry.go` 定义默认值、退避、Retry-After 和可取消等待。
策略在绑定模型时冻结；默认基准 2 秒、指数倍增、上限 60 秒并加抖动。
普通等待为封顶退避的 50%–100%。有效 Retry-After（秒数或 HTTP 日期）
优先于普通退避，允许超过本地 60 秒上限，额外抖动不超过其 20% 或 5 秒。
超过 5 分钟且不小于未封顶退避的 Retry-After 被忽略，避免异常头无限挂起。

`internal/model/failure.go` 的有序分类表是扩展入口。
新情况新增一个小分类函数并登记到表中，调用方和 UI 不重复判断错误文本。
特定终止规则排在通用网络规则前：

| 情况 | 处理 |
| --- | --- |
| 用户取消、父请求截止 | 立即退出，包括正在等待或收流时 |
| 证书无效、鉴权/权限、额度耗尽、参数/上下文错误 | 不重试 |
| HTTP 408、429、5xx | 在共享预算内重试 |
| 连接/收流超时、连接中断、HTTP/2 stream reset | 在共享预算内重试 |
| 缺少完成标记、提前 EOF、供应商声明暂时过载 | 在共享预算内重试 |
| 未知错误、协议校验失败、输出限制、展示或持久化回调失败 | 不重试 |

HTTP 客户端自身超时与用户取消分开处理：父上下文仍有效的网络超时可重试。
供应商结构化错误码优先区分额度和限流，不能只见 HTTP 429 就重试额度耗尽。
错误和轨迹仍在持有凭据的模型边界脱敏。

## 执行与计量

重试沿用同一份已编码请求和逻辑请求 ID；轨迹的 Attempt 从 1 递增。
每次物理尝试独立记录用量及耗时，包括断流前已上报的用量。
仅完整并通过协议校验的响应进入执行历史，工具结果已经提交的步骤不会重跑。
重试前清除当前步骤的半截展示，保留此前步骤和工具结果。
模型重试不自动重试工具、不重新启动整个 Agent，也不在重开程序后恢复失败运行。

## 界面

沿用 ZCode 的 turn loading 规则：
前两次重试保留普通加载提示；第 3 次起在同一位置显示
“重新连接中... 3/10”，随后随实际重试次数更新。
没有倒计时；HTTP 状态存在时悬浮提示显示状态码。
使用现有主题的次级文字及轻微文字动效，减少动态效果时静止。
主对话与侧聊独立显示。

核心通过类型化 `model.retry.updated` 事件发布 `Session.ModelRetry`，
本机 UI 直接接收快照投影；远程仅在既有传输边界编码。
收到实际输出、成功结束、取消、失败或压缩结束时清除状态。
重试状态不写入配置索引或会话记录，历史会话恢复时也会清理。

## 验证

模型测试覆盖 HTTP/2 连接错误、503、半截流混合重试、11 次上限、
下个请求计数重置、Retry-After、取消和回调失败。
真实核心测试覆盖已完成 Write 只执行一次、失败尝试用量、状态清除与快照隔离。
`ui.Tester` 覆盖第 3 次显示阈值、深浅主题窄窗和主/侧聊状态独立。
`TestDesktopLocalModelRetry` 用独立原生窗口、私有核心与本地 HTTP 夹具，
验证前两次普通加载、第 3 次计数、HTTP 悬浮提示及成功/取消后的清除。
这些本地 HTTP 夹具证据不代表真实供应商已验收。

2026-11-04 本机验证：

- 模型、Agent、核心、contracts 全包通过：
  `output/unified-retry-core-final.log`。
- 共享控件边界、重试加载与侧聊组件回归通过；
  持久化夹具 `TestFixture` 通过。
- 原生深浅主题成功/取消路径通过，截图和可访问树：
  `output/unified-retry-final-native/20261104-161722-2500049684/evidence.json`。
- Go vet、MyGo vet 与冻结框架源码校验通过；
  普通 Windows 构建位于 `build/unified-retry/windows-amd64/GCode.exe`。
- 全源码套件仍有两项既有桌面用例失败：
  `TestRealCoreSettingsAndTypedMessageProjection` 查找已移除的 Apply；
  `TestSoftwareUpdatesUseDesktopCallback` 仍从 General 查找已迁走的更新按钮。
  完整记录 `output/unified-retry-source-tests.log`，不能将此轮称为全仓测试通过。
- 未调用真实供应商；本机没有 C 编译器，未运行 race detector。
  测试只管理独立进程，未替换或重启用户现有程序。

2026-11-04 复核（当前工作副本，含其他并行改动）：

- `TestDesktopLocalModelRetry` 深浅主题再次通过，
  `output/desktop-e2e/20261104-175006-1002877949/evidence.json`。
- `go test ./internal/{model,engine,desktop} -run 'Retry|Failure|ListModels'` 通过；
  `go vet` 与 `go tool mygo vet ./internal/desktop` 通过。
- 完整原生套件仍有既有失败，均与本轮重试无关：
  `TestDesktopScenarioSidebarAlignment`、`TestDesktopLocalPromptReferences`、
  `TestDesktopLocalProjectCreationWithIgnoredArtifacts`、
  `TestDesktopLocalTranscriptActivity`、`TestDesktopLocalTranscriptFoldMotion`
  在未含本轮改动的 `ac26b06` 隔离副本中同样失败；
  `TestDesktopLocalStorageSwitchAndContinuation` 在该副本中三次运行一过两败，
  属既有不稳定项。因此不能据此把完整套件记为通过。
- 上面两条既有桌面断言已按当前界面修正：数据目录改用失焦提交
  （`TestRealCoreSettingsAndTypedMessageProjection`），
  更新入口改查关于页（`TestSoftwareUpdatesUseDesktopCallback`）。
  两者现通过；源码范围 `go test ./cmd/... ./internal/... ./scripts/...` 除
  `TestRealShellInputResizeInterruptDetachAndExit`（需要真实交互式 PowerShell，
  在 `ac26b06` 上同样失败）外全部通过。
