# 会话运行时与控制器

状态：实现中（2026-10-12）。本文件记录 `internal/runtime` 与
`internal/controller` 的边界、已落地行为和当前验证范围。它不把尚未完成的
桌面 UI、工作区 worktree 或真实模型验收写成已完成。

## 所有权边界

- `internal/runtime` 只负责把 zero 的 Agent loop、provider、sandbox、执行器、
  Hooks、MCP、插件、Skills、specialist、swarm 和会话事件存储组装成一个可调用的
  Go 内核。
- `internal/controller` 负责主会话的生命周期、运行 context、generation、纠正与
  队列、权限/提问等待、UI 快照投影和控制器状态持久化。它不实现文件、Git、
  worktree 或托管进程的第二套机制。
- `Workspace` 由 `workspace_tools` 注入，负责创建会话工作目录（包括可选
  worktree）；`ExecutionFactory` 由执行资源实现注入，负责按会话创建托管命令的
  runner。controller/runtime 不拥有全局进程管理器。
- API key 只进入 runtime 的独立 credential store，不进入 controller 快照、消息
  或运行事件的明文 payload。运行输出在写入事件和回调前做最小脱敏。

## 公开 Go 接口

controller 对桌面层提供无 context 的命令接口，后台运行由 controller 自己管理：

```go
New(options) (*Controller, error)
Snapshot() appmodel.Snapshot
Subscribe(func(appmodel.Snapshot)) func()
AddProject(path string) error
NewSession(options appmodel.NewSessionOptions) (string, error)
Send(sessionID, text string, mode appmodel.SendMode) error
Stop(sessionID string) error
Withdraw(sessionID, messageID string) error
ResumeQueue(sessionID string) error
SetProvider(sessionID, providerID string) error
SetFullAccess(sessionID string, fullAccess bool) error
ReplyPermission(sessionID, permissionID, optionID string) error
ReplyAsk(sessionID, askID string, answers []string) error
SaveSettings(settings appmodel.Settings) error
SaveProvider(provider appmodel.Provider, key string) error
RemoveProvider(providerID string) error
Close() error
```

runtime 的核心入口是：

```go
New(options Options) (*Engine, error)
Run(ctx context.Context, input Input, callbacks Callbacks) (Result, error)
```

`Options.DataDir` 必须由应用入口注入 `mygo.App.PathUserData` 对应的 GCode
数据目录，以隔离开发和正式数据。未注入时的默认目录只作为库调用的兜底，不是
桌面入口的配置来源。

## 会话状态与顺序

每个运行拥有独立的 context、run ID 和 generation。controller 在开始运行时固定
provider、模型、格式、base URL、cwd 和 full-access 到 `Session.ActiveRun`，所以运行
中修改下一轮的 provider 或权限不会改变已经开始的任务。

- 空闲发送立即启动；运行中队列消息按 FIFO 保存为 `queued`。
- 纠正消息插入队列的纠正段前端，保留连续纠正的提交顺序；当前任务进入
  `cancel_pending`，取消完成后才启动下一轮。
- 纠正和显式停止在状态持久化成功后、释放控制器锁并发出快照前发布 active
  context 的取消信号，避免旧 run 在取消发布前先完成。对应的持久化失败会回滚
  本次操作，并保留原 active run；关闭流程先完成持久化尝试，再在锁内取消所有
  active run，并等待它们退出，以保证不可重试的关闭操作仍能释放运行资源。
- 正常完成自动消费队列；显式停止、运行失败和应用重启将队列保留并暂停，用户需
  调用 `ResumeQueue`。
- 用户消息写入 `submitted`、`queued`、`running`、`completed`、`interrupted`、
  `failed` 或 `withdrawn` 状态；旧 generation 的流式回调、授权回复和提问回复
  均被丢弃。
- 撤回的排队消息从待执行队列移除，并在会话消息历史留下 `withdrawn` 记录，避免
  已提交输入没有可追踪状态。

## 已实现范围

- 独立 sessions、credential、sandbox grants、MCP permissions 和 controller 状态
  路径。
- Agent 回调到 appmodel 快照的流式文本、reasoning、tool call/result、usage、
  permission 和 ask-user 投影。
- 前台 specialist 子代理通过 `specialist.Executor.RunChild` 递归调用同一个 Go
  runtime；后台 specialist 的宿主路径仍需 workspace/root 侧裁剪，不能据此宣称
  所有子代理模式都已完成。
- 重启恢复历史和排队消息，不自动重跑中断任务；运行中的历史消息会标记为
  `interrupted`，并写入中文暂停原因。
- 2026-10-12 定向取消验证已通过：纠正和停止的 subscriber barrier 测试各自验证
  active run 在快照发布前已观察到取消；停止持久化失败测试验证原 run 不被取消，
  关闭持久化失败测试验证运行资源仍会收束。普通定向测试 `-count=50`、race
  定向测试 `-count=20` 以及完整 controller 包普通/race 测试均通过。
- 已完成本地 S1 扩展验证：使用临时 `DataDir` 与项目目录，真实子进程运行 MCP、插件
  和 Hook，真实文件加载 Skill 与 specialist，并验证用户/项目 MCP 合并、工具结果、
  扩展加载失败告警及脱敏、child session、取消传播和 session history。具体证据见
  `docs/verification/stage-5-extensions.md`。该证据使用 scripted provider，不能替代真实
  厂商 API、MCP OAuth 或原生 UI 验收。

## 当前未验证与后续边界

- 真实 provider 和 MCP OAuth、插件/Skill 的桌面配置交互、worktree 以及注入式
  execution factory 仍需由对应目标包做端到端验证；本地 DataDir/项目根目录的扩展加载
  已由 S1 记录覆盖。
- MCP OAuth token path 已由 runtime 指向 GCode `DataDir`，但真实授权流程、刷新和
  脱敏日志仍未验证；禁止回落到旧 zero 用户目录。
- 后台 specialist 必须改为 Go 内核调度或明确从桌面可用工具集中移除 CLI 宿主
  路径；当前 runtime 只保证前台 `RunChild` seam。
- controller 行为测试使用注入的 fake runner 验证状态机和 generation；runtime 扩展测试
  使用 scripted provider，但真实扩展边界仍走生产实现。两者都不能替代真实 API 或原生
  窗口验收；controller 的纠正/停止取消发布屏障、停止持久化失败回滚和 race 定向
  测试已覆盖，完整 controller 包仍需持续回归。
