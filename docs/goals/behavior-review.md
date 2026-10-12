# 行为与所有权审查目标

日期：2026-10-12（Asia/Shanghai）

## 审查范围

本审查覆盖本轮变更及其验证记录：

- internal/application/shared_edit_integration_test.go 及其验证记录，确认 S2 是否经过真实 Service、Controller、runtime.Engine 和生产内置文件工具，并检查同文件竞争、外部编辑、不同文件并发、事件归属和清理。
- internal/controller/controller.go、internal/runtime/runtime.go、internal/runtime/run.go、internal/runtime/subagent.go 以及新增的 run identity 集成测试，确认 controller 生成的 RunID 是否经过 runtime.Input、Engine.Run、runAgent 和 child progress/journal 单次稳定传递。
- internal/controller/controller.go、internal/controller/cancel_warning_test.go 以及 controller-cancel-warning 普通/race 记录，确认主动 stop/replace 的取消 warning 抑制不会吞掉普通错误或取消失败。
- internal/desktop/app.go 的 view/snapshotForView 所属最小性能改动，以及 internal/desktop/render_snapshot_test.go、internal/desktop/conversation_performance_test.go 对快照所有权、流式更新、旧 view 稳定性和渲染缓存的证据。
- internal/desktop/conversation.go、internal/desktop/pending_presentation_test.go 以及最新 desktop/full-package 记录，确认 permission 参数展示、长命令滚动和 interrupted tool 诊断的确定性呈现行为。

本代理只读检查源码和已有记录，没有修改生产代码或测试代码，没有启动原生窗口，也没有重新执行测试矩阵。最新全仓 test/vet/fmt、受影响包 race、package、codesign 和 `presentation-confirmed` 原生记录已由 root 提供；原生结论只限于该记录声明的四个 macOS Intel formal-package 场景。

## 最终判断

截至本轮读取的最新源码和记录：

- S2 的真实调用链、先读后竞争、同文件冲突、外部编辑、不同文件并发、事件归属和清理证据均已补齐到可审查范围。最新断言按 session、tool ID、路径、写入参数、tool result、events.jsonl 的 SessionID 和最终 provider 请求体逐项核对。
- RunID 的生产路径已闭合：controller 在 activeRun 生成一次 ID，execute 原样写入 runtime.Input；Engine.Run 仅在输入为空时生成一次，并把同一 ID交给 runAgent；childRunner 每次 child invocation 生成一次独立 ID，progress、returned events 和 child journal 共用该 ID。最新的 Service/journal 连续两轮普通与 race 记录（successive-runs-regular.log、successive-runs-race.log）以及 runtime child 普通和 race 记录均通过。
- Controller 的取消 warning 规则已收窄到主动 `stop`/`replace`、运行 context 已为 canceled 且 runtime error 满足 `errors.Is(..., context.Canceled)`；普通 runtime error 和取消失败仍保留 warning/error。新增普通与 race 记录均通过。
- render snapshot 的 App 输入所有权和视图边界成立。Open/Update 深拷贝外部输入；view 只复制本地会修改的 settings/providers 和 session value，历史、tools、queue、permissions、questions、projects 作为只读数据共享。新增测试覆盖旧 view 在 gated Update 后的结构和用户可见文本、非空 ActiveRun fixture、真实 Tester 控件交互以及 caller snapshot 隔离。
- pending presentation 的最小修正满足当前确定性行为：permission card 解码并展示完整 command 与可选 workdir，长内容在 180 高度滚动区内保留授权按钮；父消息已 interrupted 且 tool error/output 以 `context canceled` 结尾时显示中性“已中断”、默认折叠但保留可展开诊断；普通错误继续强制展开。focused 普通、全仓普通和受影响 desktop/controller race 均通过。
- 没有发现现有源码中被记录复现的共享可变历史 slice、队列/权限/问题泄漏或 UI dispatcher race，也没有发现阻止当前正式包的生产实现问题。

仍需保留的证据边界是：

1. gated old-view 测试证明 Update 后已捕获 view 保持稳定并仍能渲染旧文本，但 channel 只证明同步顺序，不能证明每条 CPU 指令发生物理重叠。
2. concurrent updates/frames 测试提供并发调度压力和 race 证据，但没有可重复地强制 frame 与 Update 在同一时刻重叠。
3. Markdown 复用测试仍以未变化 block 的指针相等作为“不重新解析”的实现证据；它同时有最新消息可见文本和替换后文本断言，但没有独立的用户行为等价或解析计数断言。因此该项仍是 P2 级测试证据限制。
4. snapshotForView 仍共享已经由 cloneSnapshot 接管的 ActiveRun 指针；当前 UI 搜索只证明没有本地原地写入，不能把它表述为永久深拷贝契约。
5. run identity 的 application journal 断言以每个 RunID 的事件类别布尔集合为主；它能抓住 controller/runtime ID 分叉和 child 每事件重新生成 ID，但没有逐条把 journal payload 的文本、工具 ID、事件顺序与对应 run 建立完整映射。
6. 新增 child 测试显式传入 child RunID；独立 Engine.Run 的空 RunID 路径在生产源码中只有 Engine.Run 一个生成点，但现有新增测试没有单独断言“空输入生成的 ID 与所有 journal/callback 事件相同”。
7. Service 测试在首轮 permission allow 且 session 进入 idle 后才发送第二轮，语义是 successive runs（连续两轮运行）；它不覆盖运行中 correction、cancellation replacement 或运行中队列替换。
8. cancel warning 测试使用 fakeRunner，普通/race 记录证明 controller 状态和 warning 归属，不替代真实 provider 取消、进程终止或网络中断验证。
9. pending presentation 的 `ui.NewTester` 证据与最新 `presentation-confirmed/review.json` 原生记录互相补充；原生记录覆盖 permission allow/deny、ask history、permission correction、ask correction、实际文件/回答历史、替换 RunID、解码授权呈现、AX 已中断和正常退出清理，但不证明长授权内容在原生窗口滚动、真实 API、用户视觉认可或旧包长历史实测。旧的 native-pending-ui/run-identity-final 截图不得作为当前包身份证据。
10. 本轮最新 package manifest 与原生 review 对应的 executable SHA-256 为 `d0102a642e6c73f6a41b76070dce74e1a6de0fcfaaef9d1738a9a96fe805666a`，source/test fingerprint 在构建后保持不变；该包完成 build/sign/codesign 记录，但仍不扩大到真实 provider API、设备、GPU、输入法、物理滚轮、剪贴板或跨平台行为。

## 完成条件

本审查报告必须：

1. 给出精确文件和行号，并区分生产事实、测试证据、推断和未验证项。
2. 对 S2 说明真实 Service → Controller → runtime.Engine → agent.Run → tools.Registry/FileTracker 路径，说明先读后竞争、同文件、外部编辑、不同文件、事件记录和清理边界。
3. 对 RunID 说明 runtime.Input → Controller.execute → Engine.Run/runAgent 和 child progress/journal 的单次归属，并检查独立 runtime 调用的生成点。
4. 对 render 说明 cloneSnapshot、snapshotForView、本地 metadata 写入点，以及 queue/permission/question 的只读约束。
5. 对 controller cancellation warning 和 desktop pending presentation 给出生产事实、测试证据和原生验收边界。
6. 记录现有 focused、race、全仓 test/vet/fmt、package、codesign 和原生记录的实际边界，不把它们提升为未覆盖的平台或真实服务结论。

## 纠正取消时旧 runner 收束与队列保留的只读核验

日期：2026-10-12。范围：只读核对 `internal/controller/controller.go`、`internal/controller/controller_test.go`、`internal/controller/cancel_warning_test.go`、`internal/application/runtime_boundaries_test.go` 以及取消相关 verification 文档；不修改生产代码、测试或 driver，不运行 Go 测试、race、构建或窗口验证。

### 生产路径事实

真实 application 入口 [internal/application/service.go:80-92](../../internal/application/service.go#L80-L92) 直接调用 controller 的 `Send`/`Stop`。纠正发送命中活动 run 时，[internal/controller/controller.go:543-557](../../internal/controller/controller.go#L543-L557) 先把新消息插入队列、把旧 run 标记为 `cancelReplace` 和 `Cancelling`，再在持久化成功后调用 [controller.go:566-571](../../internal/controller/controller.go#L566-L571) 的 `previous.active.cancel()`；这里没有提前启动 replacement。

[internal/controller/controller.go:993-1032](../../internal/controller/controller.go#L993-L1032) 只有在 `c.runner.Run(...)` 返回后才调用 `c.finish(...)`，所以在“runner 最终返回”这一前提下，下一次 `Run` 不会与旧 `Run` 并行。`finish` 在 [controller.go:1209-1214](../../internal/controller/controller.go#L1209-L1214) 才从队列取出消息并调用 `startRunLocked`。

这里需要区分“旧 `Run` 调用已经返回”和“所有 runner-owned 外部资源都已退出”。[internal/controller/controller.go:33-35](../../internal/controller/controller.go#L33-L35) 的 `Runner` 契约以 `Run(...)` 返回作为调用边界；[controller.go:993-1032](../../internal/controller/controller.go#L993-L1032) 在返回前不会调用 `finish`，因此 controller 能确认旧调用已经收束后才调度 replacement。`run.ctx.Err()` 只参与取消分类：它本身不能证明底层 provider 或进程的资源状态，但也不能据此把 `runErr == nil` 的正常取消返回判定为错误。当前没有证据把某个已观察的 post-return 工作定义成 controller 必须等待的同步边界，因此不能把 [controller.go:1170-1172](../../internal/controller/controller.go#L1170-L1172) 认定为已发生的 P1 生产错误；运行时已有的有界清理和显式异步所有权见下文。

generic error 分支符合队列保留要求：[controller.go:1179-1185](../../internal/controller/controller.go#L1179-L1185) 将 session 置为 `Failed`、`QueuePaused=true`、`PauseReason=pauseReasonCancelFailure`；replacement 只有在后面的 `shouldContinue` 分支才会从 `session.model.Queue` 移除，因此取消返回 generic error 时 replacement 和其后的 queued 消息都会保留。

还有一个未定义的非返回分支：如果旧 runner 在收到 cancel 后一直不返回，[controller.go:550-553](../../internal/controller/controller.go#L550-L553) 设置的 session 会一直停在 `Cancelling`，而 correction 路径把 `QueuePaused` 清为 `false`；`finish` 永远不会执行，也就不会进入 `pauseReasonCancelFailure`。因此当前实现能保证“不在旧 `Run` 返回前启动新任务”，但没有超时或其他机制把永久忽略取消归类为“取消失败、队列暂停”。这属于需要明确产品策略的未闭环分支，不能作为已通过的失败处理。

真实 `runtime.Engine` 的常规取消路径会把取消返回给 controller：`internal/runtime/run.go:204-213` 将 agent 错误返回给 Engine，`internal/agent/loop.go:454-459` 和 [loop.go:920-923](../../internal/agent/loop.go#L920-L923) 在运行 context 被取消时返回 `ctx.Err()`，Engine 再在 [internal/runtime/runtime.go:482-493](../../internal/runtime/runtime.go#L482-L493) 将其包装后返回。因此真实 provider 在这些路径上通常会产生 `context.Canceled`。这能说明当前 Engine 的取消传播方向，不能单独证明 provider、进程或显式脱离当前调用的后台工作都已结束。

运行时存在需要单独记录的资源边界，但它们尚不足以把 controller 的串行调度结论改写成生产 bug：Engine 在 [internal/runtime/runtime.go:354-357](../../internal/runtime/runtime.go#L354-L357) 为每次运行创建 `ProcessManager`，并在返回前调用 `StopAllOwned`；[internal/execution/process_manager.go:304-323](../../internal/execution/process_manager.go#L304-L323) 会终止所属进程并最多等待 `processStopTimeout`，`waitForProcesses` 超时后仍会返回（[process_manager.go:725-745](../../internal/execution/process_manager.go#L725-L745)）。这证明清理有明确的有界等待，不证明异常进程树在超时后已经消失。`exec_command` 还把其可保留进程的 command context 建在 `context.Background()` 上（[internal/tools/exec_command.go:169-206](../../internal/tools/exec_command.go#L169-L206)），所以后续若要把“资源完全退出”纳入 correction 成功条件，需要单独定义超时和失败状态。
普通 captured execution 的同步边界更清楚：`execution.Runner.ExecuteCaptured` 在 [internal/execution/runner.go:74-135](../../internal/execution/runner.go#L74-L135) 中等待 `Command.Run`、收集 report，并通过 defer 执行 prepared cleanup 后才返回；hooks 则在 [internal/hooks/dispatch.go:152-213](../../internal/hooks/dispatch.go#L152-L213) 逐个等待 `runWithTimeout` 并记录完成结果。当前没有从这两条路径发现 `Run` 返回后仍会继续发起 provider 请求或 session 写入的证据。

另外，`Task` 的 `run_in_background` 是显式脱离当前 run 的产品语义：它在 [internal/specialist/exec.go:511-530](../../internal/specialist/exec.go#L511-L530) 使用独立的 `context.Background()` 子上下文启动 goroutine，生命周期交给共享 specialist runtime；该 runtime 只在 [internal/runtime/runtime.go:746-787](../../internal/runtime/runtime.go#L746-L787) 的 Engine.Close 中统一关闭。这种后台任务可以在旧 `Run` 返回后继续运行，属于已声明的异步所有权，不能当作 controller 提前启动 replacement 的证据。Swarm 的 [internal/swarm/team.go:203-249](../../internal/swarm/team.go#L203-L249) 则会在 `childSwarm.Close` 时取消并等待 watcher；MCP runtime 的 [internal/mcp/registry.go:239-257](../../internal/mcp/registry.go#L239-L257) 也会在 Engine 返回前关闭已保留 client。现有源码、日志和测试没有显示这些正常关闭路径在 correction 后产生了旧 run 的 provider 回调或 session 写入。

### 现有测试与 verification 证据

- [TestCorrectionPreservesOrderAndNeverRunsInParallel](../../internal/controller/controller_test.go#L134-L218) 的 fake runner 在收到 `ctx.Done()` 后再等待释放并返回 `context.Canceled`，断言调用顺序为 `first → fix-one → fix-two → queued` 且最大并发为 1。它证明“旧 runner 返回后才启动下一次”在会观察 context 的 runner 上成立。
- [TestCorrectionCancelsActiveRunBeforeEmission](../../internal/controller/controller_test.go#L220-L303) 让旧 fake runner 等待释放，释放后检查 `ctx.Err()` 再返回 `nil`，并断言 replacement 只有旧调用返回后才出现。它覆盖了“取消调用发生在 snapshot emission 前”和返回后的串行调度；`ctx.Err()` 检查属于该 fake 的行为约束，不能把它提升为所有 runner 或底层 provider 的资源收束证明。
- [TestCancellationFailureDoesNotStartReplacement](../../internal/controller/controller_test.go#L572-L607) 让 fake runner 在 `ctx.Done()` 后返回 generic error，断言 `Failed`、`QueuePaused`、`pauseReasonCancelFailure`、队列长度仍为 2 且调用次数为 1。这证明 generic error 分支的暂停和消息保留；它也没有覆盖永久不返回的 runner 或运行时资源清理超时。
- [cancel_warning_test.go:14-141](../../internal/controller/cancel_warning_test.go#L14-L141) 与 [controller-cancel-warning/regular.log](../../output/acceptance/controller-cancel-warning/regular.log)、[controller-cancel-warning/race.log](../../output/acceptance/controller-cancel-warning/race.log) 证明主动 stop/replace 的 `context.Canceled` warning 抑制、普通错误和 generic cancellation failure 的可见性；测试仍使用 fake runner，不能证明真实 provider 或外部进程收束。
- [TestServiceStopPersistenceFailureRetainsRunUntilSuccessfulStop](../../internal/application/runtime_boundaries_test.go#L176-L244) 走真实 `application.Service` 和 HTTP provider，覆盖的是 Stop 持久化失败时保留活动 run、随后成功 Stop 才进入 `Interrupted`；它没有发送 correction、没有队列 replacement，也没有验证旧 runner 收束后新任务的启动边界。`docs/verification/runtime-boundaries.md` 的结论同样只覆盖 Stop/Close 资源边界。
- `docs/verification/permission-question-service.md:15-27` 及 [TestServicePendingInteractionCancellationAndSessionOwnership](../../internal/application/permission_question_integration_test.go#L230-L371) 证明真实 Service 的 permission/ask 等待收到 correction 后旧请求失效、新请求出现、旧回复被拒绝；它用新 pending request 作为顺序代理，没有显式记录旧 `runner.Run` 返回/收束确认，也没有 cancellation failure 或队列保留场景。该文档的“旧 run 先完成取消”应限定为当前 fixture 可观察的 pending interaction 结果，不能外推为所有 runner 的底层收束证明。

因此，现有证据可以确认：旧 `Run` 返回后，controller 才会调度 replacement；generic error 返回时，队列会暂停并保留消息。`ctx.Err()` 是取消分类依据，不是底层资源收束证明；现有材料没有证明其导致已发生的 P1 生产错误，也没有证明所有 provider/进程都在 `Run` 返回前完全退出。当前仍不能把“所有外部资源均已收束后启动新任务”扩展为已验证结论。

### 最小有意义验证方案

在产品决定为“取消后永不返回”增加超时前，不应先写一个预设 `runErr == nil` 就失败的测试；这会把 `Runner.Run` 的返回契约与 context 观察细节混为一谈。最小有意义的下一步是先明确超时后的状态、`QueuePaused`/`PauseReason`、队列保留以及是否允许人工恢复，然后用一个永久阻塞的 fake runner 验证该状态转换。若要验证资源边界，再让本地 provider/进程 fixture分别记录“旧 `Run` 返回”“清理完成”“新 provider 请求”三个时间点，并把未在超时内完成的清理映射到已定义的失败策略；现有 HTTP fixture 和 controller fake 均未记录这三个独立边界。
