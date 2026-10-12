# 桌面宿主中的后台 specialist 生命周期与隔离

状态：源码研究完成；当前只保留方案和验收边界，等待 root 在 AX 诊断冻结结束后批准生产实现。

## 目标

让长期运行的 Go 桌面宿主完整支持 specialist 的后台模式，同时保留已有 foreground specialist、swarm、权限、provider 和 session journal 语义：

- `Task` 的 `run_in_background` 在桌面进程内启动并持续运行，不依赖自身可执行文件再次进入 `exec` 子命令。
- 后台任务的 metadata、NDJSON 输出和临时 prompt 文件全部落在当前 runtime 的 `DataDir` 下，不与开发、生产或其他测试实例共享默认目录。
- parent run 结束后，新的 parent run 仍可通过现有 `TaskOutput` 查询任务，并通过 `TaskStop` 停止任务；`ParentID` 继续作为审计元数据，而不是查询权限边界。
- `Service.Close` 在取消并收束 active parent run 后停止所有后台 specialist，清理 prompt 文件，写回任务状态，并且重复调用保持幂等。
- 不缩小子代理能力，不删除后台模式，不通过 CLI/TUI 旁路满足桌面需求。

本轮不修改生产源码、不运行测试或窗口、不打包、不提交、不推送。根目录当前仍在等待 AX 诊断，生产改动必须等 root 明确 go-ahead。

## 当前事实

### 启动入口

`cmd/gcode/main.go` 在创建 mygo 应用前只处理两个 Windows 子命令：

- `sandbox.WindowsCommandRunnerSubcommand`
- `sandbox.WindowsSandboxSetupSubcommand`

正常桌面链路是：

```text
cmd/gcode
  -> application.New(dataDir)
  -> runtime.New(runtime.Options{DataDir: dataDir})
  -> controller.New(... Runtime: engine ...)
  -> desktop
```

没有 specialist 默认构造的 `exec` 分发入口。`specialist.Executor.binaryPath` 默认取 `os.Executable()`，后台参数包含 `exec ...`，所以把后台 specialist 当作独立 CLI 子进程启动时，当前桌面宿主无法接收这条命令。

### 每次 parent run 的 runtime 边界

`internal/runtime/runtime.go` 的 `Engine.Run` 每次都会：

1. 创建本次运行的 execution runner、process manager 和 tools registry。
2. 构造 `specialist.Executor`，设置当前 paths、共享 session store 和 `engine.childRunner(...)`。
3. 调用 `specialist.RegisterTools`。
4. 在本次 `Run` 返回时 `defer specialistRuntime.Close()`。
5. 创建并在本次运行结束时关闭 swarm。

这条 `defer` 对当前 foreground specialist 是合理的资源边界，但对后台任务是错误的：`Runtime.Close` 会调用 `manager.KillRunning`，因此 parent runtime 返回就会杀掉刚刚交给后台运行的任务。后台 manager 目前既不属于 `Engine`，也不属于 `Service`。

`Engine.Close` 当前只将 `engine.closed` 置为 true，没有关闭长期后台 manager，也没有清理后台 specialist 资源。

### 可复用的 Go child 链路

`internal/runtime/subagent.go` 的 `engine.childRunner` 已经在同一 Go 宿主内构造 child `runtime.Input`，并继承：

- parent session ID、parent tool call ID；
- cwd、provider、model；
- depth、permission mode；
- child session ID 和 child RunID。

它把 child callbacks 转成 stream-json 事件并通过 progress callback传回 parent。`internal/swarm/launcher_specialist.go` 已通过 `specialist.Executor.Run` 使用该 foreground `RunChild` 路径，说明桌面宿主已有可复用的 specialist 执行能力。

### 当前后台实现是外部进程

`internal/specialist/exec.go` 的 `runFresh` 在 `RunInBackground` 为 true 时直接进入 `runBackground`。现有流程是：

1. `BuildArgs` 创建 child session、参数和可能的大 prompt 文件。
2. 从 `os.Executable()` 得到当前二进制路径。
3. 调用 `launchBackgroundProcess`，传入 `exec --init-session-id ... --tag specialist ...`。
4. 先向 background manager 注册任务，再记录 PID。
5. 外部进程退出后调用 `MarkExited`，读取 NDJSON，汇总结果，记录 specialist accounting，清理 prompt 文件。

foreground 的 `RunChild` 只由 `runBuiltArgs` 使用；现有 `runBackground` 完全绕过它。因此设置 `Executor.RunChild` 不能修复桌面后台路径。

### manager、工具和生命周期

`internal/specialist.Runtime` 懒加载一个 `*background.Manager`，并负责跟踪大 prompt 临时文件。`Close` 会杀 running 任务并删除已跟踪 prompt 文件。

`specialist.RegisterTools` 在 executor 没有 `BackgroundRuntime` 时新建一个 runtime。`Task`、`TaskOutput` 和 `TaskStop` 都绑定这个 runtime 的 manager。由于它当前在每次 `Engine.Run` 内创建，parent A 的工具注册和 parent B 的工具注册不会天然共享 manager。

`TaskOutput` 按 task ID 从 manager 读取任务及 output 文件，支持轮询；当前没有 parent ID 校验。`TaskStop` 按 task ID 调用 manager 的 `Kill`。这已经符合“跨 parent 查询”的语义方向，但 manager 必须是 Engine/Service 级共享实例。

`background.Manager` 的 `Kill` 目前只接受有 PID 的任务。Go 宿主内的后台任务没有 PID，会返回 `background task ... has no pid`，所以复用 Go child 链路时必须补一个 task ID 到 cancel func 的最小登记机制。

### 数据目录与重载

`background.DefaultRoot` 默认使用 `$XDG_DATA_HOME/gcode/background` 或 `$HOME/.local/share/gcode/background`，没有使用 `runtime.Engine.DataDir()`。因此当前开发、生产和测试实例可能共享 metadata 和 output 文件。

manager 重新加载 metadata 时，会把持久化的 `running` 任务改为 `error`、清零 PID，并记录“原进程所有权丢失”的 warning。当前设计不支持跨进程恢复正在运行的任务控制；本目标只要求同一长期 Service 进程内跨 parent run，不能把重启恢复误报为已实现。

### Service.Close 的实际顺序

`controller.Close` 当前先在锁内将所有 active parent run 标记为停止并调用 cancel，释放锁后等待 `c.wg.Wait()`，最后调用 `engine.Close()`。因此 Engine 级 specialist runtime 应在 `Engine.Close` 中关闭：此时 parent `Run` 已经退出，不会再创建工具或启动后台任务；关闭错误可以和 controller 的持久化错误合并返回。

`application.Service.Close` 只是转发到 `controller.Close`。桌面退出路径 `cmd/gcode/main.go` 的 `host.closeResources` 调用它，所以修复 Engine.Close 即可接入现有桌面关闭链，无需另造第二套退出钩子。

## 缺口判断

事实上的阻塞点有四个：

1. 外部 `exec` 子命令没有桌面入口，当前独立进程后台路径不能在正常 desktop host 中闭环。
2. specialist runtime 在 parent run 结束时关闭，后台任务无法越过 parent 生命周期。
3. manager 未按 Engine.DataDir 隔离，数据会落到全局默认目录。
4. Go 内部任务没有 PID，现有 TaskStop 无法取消。

跨 parent 的 `TaskOutput`/`TaskStop` 本身无需新增 parent 权限模型。只要两个 parent run 在同一 Engine 上拿到同一个 manager，按 task ID 查询和取消即可；`ParentID` 保留用于列表、审计和 UI 展示。

## 最小实现分层

### 第一层：Engine 级共享 specialist runtime

在 `runtime.Engine` 持有一个惰性创建的共享 `*specialist.Runtime`，其 manager root 固定为：

```text
filepath.Join(engine.DataDir(), "background")
```

实现上优先保留懒加载：没有使用 specialist 时不创建 background 目录；一旦第一次注册或运行后台任务，manager 才创建。manager 的所有权必须属于 Engine，而不是某个 `Run`。

每次 `Engine.Run` 构造 executor 时传入这个 `BackgroundRuntime`，让 `RegisterTools` 注册的 `Task`、`TaskOutput` 和 `TaskStop` 使用同一个 manager；移除对共享 runtime 的本次 run `defer Close()`。foreground child 和本次 run 的 swarm 仍按现有 run 生命周期关闭。

### 第二层：Go 宿主内后台执行

后台 specialist 应复用 `engine.childRunner`，而不是再次执行 `os.Executable()`：

1. manager 注册任务并创建 output 文件，保留 task ID、ParentID、specialist name、description 等现有 metadata。
2. 为 task 创建独立 context 和 cancel func，并在共享 specialist runtime 中按 task ID 登记。
3. 启动 goroutine 调用 child runner；把 child progress 事件按 NDJSON 增量写入 manager output 文件，同时保留 `TaskOutput` 现有摘要和 raw output 语义。
4. child 正常完成、返回错误或收到取消时，统一更新 manager 状态和 exit code，复用已有 accounting 和 prompt cleanup。
5. goroutine 收尾时删除 task 的 cancel 登记；`TaskStop` 先将任务置为 killed，再调用 cancel，避免 child 收尾把用户主动停止覆盖成 completed/error。

这里需要保留现有“先记录停止意图，再等待退出”的竞态原则。不要把 goroutine 的取消当作 PID 杀进程的简单替换后就直接复用 `MarkExited`，必须确保 killed 状态不会被晚到的 child completion 覆盖。

### 第三层：TaskStop 的最小取消能力

优先在现有 `specialist.Runtime` 或 `background.Manager` 内增加 task ID 到 cancel func 的登记、取出和清理；不另造完整后台调度器。PID 任务继续走现有 manager.Kill，Go 内部任务走登记的 cancel func，二者共享状态持久化和竞态保护。

建议的状态约束：

- `Register` 后 task 为 running，登记 cancel func；
- `TaskStop` 将 running 任务原子标记为 killed，再触发 cancel；
- child 收尾只允许从 running 转 completed/error；
- runtime/Service.Close 遍历仍为 running 的 task，触发相同停止路径；
- cancel 登记在收尾后删除，但 task metadata 保留，供后续 parent 的 TaskOutput 读取。

### 第四层：Engine/Service 关闭

`Engine.Close` 应在一次成功的 closed 状态转换后关闭共享 specialist runtime，并汇总关闭错误；重复调用直接返回 nil。controller 已经保证 active parent run 在调用 Engine.Close 前经 `wg.Wait` 收束，因此关闭顺序是：

```text
Service.Close
  -> Controller.Close: 标记并 cancel active parent runs
  -> Controller.Close: wg.Wait
  -> Engine.Close: 关闭共享 specialist runtime
  -> Runtime.Close: 停止 running background tasks、清理 prompt 文件
```

若 Engine.Close 关闭失败，必须返回错误但不能恢复 `closed=false`；桌面退出路径会记录错误并继续完成应用退出。

## 验收测试设计

本轮未运行以下测试。得到 root go-ahead 后，只增加能证明生命周期和隔离的最小测试，不以改写已有单元测试来制造通过结论。

### specialist 层

- 保留并调整 `internal/specialist/lifecycle_test.go`、`runtime_test.go`、`task_tool_test.go` 对外部 PID fake 的覆盖，证明原有 manager 状态、accounting、summary 和 prompt cleanup 没有回退。
- 增加 Go child background fake：`RunChild` 阻塞到 context 取消，确认 `Task` 立即返回 task ID，output 能看到 progress，`TaskStop` 能取消无 PID task，最终状态为 killed。
- 增加 completion/stop 竞态断言：用户停止后迟到的 child exit 不得覆盖 killed。

### runtime/application 层

优先扩展 `internal/application/extensions_integration_test.go` 的真实 `Service -> Controller -> Runtime` 链路，使用临时 `dataDir` 和本地 provider fixture：

1. parent A 通过真实 provider 启动 `run_in_background` specialist，provider 调用返回后 parent A 完成。
2. parent B 在同一 Service 中调用 `TaskOutput`，确认仍能读取 A 创建的 task、ParentID 和已写入 output；该 task 不因 A 的 `Engine.Run` 返回而被杀掉。
3. parent B 调用 `TaskStop`，确认无 PID Go task 被取消、状态为 killed，并且 provider/child 请求收到 context cancellation。
4. 在独立 `dataDir` 中创建第二个 Service，确认看不到第一个 Service 的 task metadata；同时检查文件实际位于 `<dataDir>/background`。
5. 启动一个仍在运行的后台 specialist，调用 `Service.Close`，确认关闭在 bounded timeout 内返回、child 请求被取消、task 不再保持 running、prompt 临时目录被清理；再次调用 `Service.Close` 确认幂等。

### 关闭与真实残留

只要测试涉及真实子进程或 provider fixture，应在退出后检查：

- manager metadata 的最终 status 和 exit code；
- output 文件可读且包含收尾事件；
- specialist prompt 临时目录不存在；
- fake provider 的请求 context 已取消；
- 没有把“进程 PID 不存在”误当成 Go goroutine 已停止，二者分别以 context、状态和 bounded wait 证明。

## 所有权与等待边界

本目标的生产实现预计涉及以下文件，但在 root go-ahead 前不得修改：

- `internal/runtime/runtime.go`
- `internal/runtime/subagent.go`（仅在复用 child runner 需要最小接口调整时）
- `internal/specialist/exec.go`
- `internal/specialist/runtime.go`
- `internal/specialist/registry.go`
- `internal/specialist/output_tool.go`
- `internal/specialist/stop_tool.go`
- `internal/background/manager.go` 及其必要的测试
- `internal/application/extensions_integration_test.go` 及必要的 specialist/runtime 测试

当前不应修改 `cmd/gcode/main.go` 来补 `exec` 入口；桌面目标优先在同一 Go 宿主内运行后台 specialist，避免为当前问题重新引入 CLI 分发和跨进程所有权。也不应改变已有 `Service` API，除非实际桌面调用证明只靠 runtime tool 注册不足。

完成实现后必须由 root 决定是否运行 targeted tests、race、全仓回归、原生窗口和打包验证；本专属研究目标到此结束，AX 诊断期间保持源码冻结。

## 风险与未决项

- 子 agent 的 `runtime.Engine.Run` 会创建本次 child 的 execution/process manager、MCP、plugins、hooks、skills 和 swarm。后台 goroutine 必须等待 child `Run` 返回，不能只取消 provider 请求就提前释放这些 run 级资源。
- `RunChild` 当前通过 parent callbacks 报告 warning；后台任务没有可见 parent callback 时，warning 应进入 task output 或 session journal 的既有可读路径，不能静默丢弃。
- manager 的 metadata 持久化仍是同一进程内 task registry 的磁盘快照；不要把本目标描述成跨重启恢复。重启后原 running task 被标记 error 的现状需要另一个明确目标。
- `TaskOutput` 轮询读取 output 文件时要允许 child 正在追加；写入必须按完整 NDJSON 行完成，避免读取半行后破坏摘要。
- 关闭时如果某个 child 无法在已有 bounded wait 内收束，必须返回可诊断错误并保留 task 状态证据，不能无限阻塞 `Service.Close`。

## 当前结论

当前代码已经有可复用的 foreground Go child 能力和成熟的 manager metadata/output/accounting 结构；缺口集中在后台任务的所有权、取消登记、DataDir 隔离和 Engine/Service 生命周期。最小可行方案是把 specialist runtime 提升到 Engine 级、把后台执行切换到 Go child goroutine、给无 PID task 增加 cancel 登记，并让 Controller 的现有等待顺序自然托管 Engine.Close。生产改动等待 root 明确授权后再开始。
