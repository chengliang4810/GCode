# round-4 AX 反馈循环只读审阅

日期：2026-10-12。范围：只读审阅当前 native-streaming-qa.py 的 AX deadline、分段 measurement、controller/fixture 上下文、独立 collector、ready 文件、清理和脱敏；不运行窗口、测试或构建，也不修改驱动或生产代码。当前冻结 driver SHA 已核对为 `613200b2064a85ab6b60b08cc5960307d5a8c2af78809ff8429b41ffc207933c`；每次 AX tail query 的 remaining-budget 限幅和真实调用形状 self-test 已在该快照中存在。

结论：round-4 的流式归属、最终状态、退出和凭据清理在已记录范围内通过。当前驱动已经补上分段 AX 记录、bounded controller/fixture context、独立 collector、严格 `wait_until` 语义，以及每次 tail query 的 remaining-budget 限幅；self-test 也已经沿 `read_session_ax` → `read_ax_last_message_row` → `ax` → `subprocess.run(timeout=...)` 验证 timeout 传递。因此“click 完成后开始计算 18 秒 tail-query outer budget”的定义下，per-query deadline P0 已有代码和自测证据。round-5/round-6 的原生运行则暴露了两个不同的失败点：round-5 第二次 session click 的 AX 调用在 12 秒固定单次上限内超时，尚未进入 tail query；round-6 第三次 session click 成功，但 tail query 在多次无 tail 返回后连续两次超时并触发 18 秒 outer deadline。故 AX 读回功能 P0 仍不能标记通过。仍未闭环的是总 `read_session_ax` 预算的定义：当前 `click_session` 在 `query_deadline` 建立前执行，click 不计入这 18 秒；如果要求 18 秒覆盖 click 加全部 query，则还不能把总 readback deadline 写成通过。

## round-4 原始边界

原始证据位于 [round-4/measurement-samples.json](../../output/acceptance/native-streaming/round-4/measurement-samples.json)、[results.json](../../output/acceptance/native-streaming/round-4/results.json)、[manifest.json](../../output/acceptance/native-streaming/round-4/manifest.json) 和 [diagnosis-baseline/observed-outlier.json](../../output/acceptance/native-streaming/diagnosis-baseline/observed-outlier.json)。样本显示：

| 信号 | round-4 记录 | 可以支持的判断 |
| --- | --- | --- |
| SSE | 3 个请求各 300 chunks，峰值并发 3，均完成 | loopback fixture、请求并发和流结束路径通过 |
| controller | 2 次 foreground poll；latency_ms 为 0.776871、0.850895 | 两个离散时点看到了三个 running session 和独立 run ID |
| process | 1 次 foreground sample；RSS 66572 KiB、CPU 17.8% | 一个进程时点样本，不能形成资源曲线 |
| AX | 27686.087632、1663.861772、760.924816、876.567313 ms | 四次当前 session tail-token 读回均无 foreign marker/tail token；耗时是混合聚合样本 |
| 退出/凭据 | exit code 0、QA PID 消失、fixture 停止、隔离 credentials 删除 | 本轮清理断言通过 |

首个 controller poll 的 request_count 是 2，但三会话 snapshot 已为 running；下一次 poll 才看到 3 个请求。controller snapshot 和 fixture 记录不是同一时钟，不能从一次 poll 推导完整 provider 时序。首次 AX 聚合耗时从 session click 开始，旧证据没有 click、单次 AX query 和 retry 的分段时间，因此不能把 27686.087632 ms 直接归因到 UI、System Events IPC、controller、runtime 或 provider。

## 当前驱动的反馈信号

### 分段 AX 和上下文

[read_session_ax](../verification/scripts/native-streaming-qa.py#L936-L1059) 现在为每个读回记录：

- click_ms、click 起止时间、click 前后 controller/fixture context；
- 每次 tail query 的 attempt 编号、起止时间、耗时、成功/异常类型、文本字节数和 tail 是否可见；
- wait_started_at、wait_ended_at、总 latency_ms、outer budget 和最终状态；
- 每个 attempt 前后的目标 session 状态、run ID、active 状态、fixture 请求数、active/completed 数量和错误计数。

[ax_diagnostic_context](../verification/scripts/native-streaming-qa.py#L913-L933) 只读取目标 session 的 bounded controller 字段和 fixture 计数，不复制完整 snapshot、journal、请求体或响应正文。这足以把 AX attempt 与运行上下文关联起来，但仍有两个时序限制：context 的绝对时间是 UTC wall clock，段耗时使用 perf_counter，两者没有一个可重放的统一 monotonic timestamp；context 也没有记录 journal sequence 或具体 chunk 序号。因此它支持“AX 前后运行状态是什么”的判断，不支持把一次 AX 查询精确对齐到某个 provider chunk 或 journal 事件。

### 严格等待和 self-test

[wait_until](../verification/scripts/native-streaming-qa.py#L166-L197) 已改为每轮检查剩余预算，在 condition 返回后再次检查 deadline，并把 sleep 限制在剩余时间内；晚于 deadline 才成功或 timeout 消耗完预算都会返回 WaitDeadlineExceeded。这解决了旧实现接受 late success 的逻辑缺口。

当前 [self_test](../verification/scripts/native-streaming-qa.py#L1418-L1737) 先用 ControlledClock 验证条件晚于 deadline 成功和 AX timeout 消耗预算时必须失败，然后替换 `click_session`、`ax_diagnostic_context` 和 `subprocess.run`，使用 `FakeAXApp` 实际调用 `read_session_ax` → `read_ax_last_message_row` → `ax` → 被替换的 `subprocess.run`。第一次 fake query 推进 15 秒并返回不含 tail 的结果，第二次 query 收到约 3 秒的剩余 timeout 并返回 tail；另一个用例推进 18.5 秒后确认抛出 `WaitDeadlineExceeded`，且只发起一次 query。它验证了生产调用形状、`timeout` 参数传递和 query budget 限幅，但没有启动真实 app 或执行真实 `osascript`，因此仍不能证明 macOS AX IPC 的运行时行为。

当前 [ax](../verification/scripts/native-streaming-qa.py#L138-L163) 把传入 timeout 直接交给 `subprocess.run`，默认单次 AX 上限仍是 `MAX_AX_SECONDS = 12`。`read_session_ax` 在 click 完成后建立 18 秒 `query_deadline`，每次 query 计算 `remaining = query_deadline - clock()`，并把 `min(MAX_AX_SECONDS, remaining)` 传给 [read_ax_last_message_row](../verification/scripts/native-streaming-qa.py#L897-L910)，预算耗尽后跳过 query。这个部分已经由上述 self-test 覆盖；但 `click_session` 使用的 AX 调用发生在 `query_deadline` 建立之前，所以 `outer_budget_ms: 18000` 不是从 `read_session_ax` 入口开始覆盖 click 加所有 query 的总硬截止时间。是否将 click 纳入同一个 outer budget，必须作为 round-5 判定口径明确记录。

### 独立 DiagnosticCollector

[DiagnosticCollector](../verification/scripts/native-streaming-qa.py#L1062-L1203) 在三会话创建后、发送前启动（[run_native](../verification/scripts/native-streaming-qa.py#L1867-L1869)），使用独立 daemon thread：

- 每 0.2s 读取 controller snapshot 和 fixture bounded status；
- 每 1.0s 通过带 5 秒 subprocess timeout 的 ps 记录 RSS/CPU；
- 不调用 AX、不操作窗口，因此 foreground AX 阻塞时仍能推进 controller/process 采样；
- 用锁保存 samples/errors，异常只保存 error_type，不保存原始异常文本；
- stop_join(timeout=8) 设置 stop event、join 并记录 join timeout。

这已经满足“采样不再与 AX 共用同一控制线程”的结构要求。当前仍有边界：没有记录计划 tick、实际 tick、missed tick 或采样 jitter；controller 文件读取没有独立 IO timeout；collector thread 仍是 daemon；collector 发生采样错误或 thread error 时，summary["checks"]["diagnostic_collector"] 会留下 ERRORS_RECORDED，但 cleanup failure gate 只检查 stop/join、server、credentials 和 QA PID，并不会因为 error_count > 0 自动把整轮标为 FAIL。若 collector 是诊断证据而非功能 gate，应在 review 中明确这一点；若要求采样完整性，则必须把错误状态纳入验收条件。

### 主循环和原生操作顺序

主循环仍按 snapshot/fixture 检查、截图、四次 read_session_ax、geometry AX 和 synthetic scroll 串行运行（[native-streaming-qa.py#L1870-L1993](../verification/scripts/native-streaming-qa.py#L1870-L1993)）。独立 collector 会在这段期间采样，但 foreground loop 自身不再代表持续 poll 频率。AX 读回完成后才执行 geometry 和 scroll；因此即使 collector 样本充足，也不能把四次 AX 读回或合成滚轮写成全部发生在三流同时 running 的同一时间窗口。

## ready 文件和过期 handle

[write_ready](../verification/scripts/native-streaming-qa.py#L1211-L1229) 当前写入 qa_identity、PID、Application Support 路径、phase、更新时间、写入时的 process_alive 和 exit_code。流程会写 app_started、ready_before_send、第一次 ax_readback 和 finally 的 stopped。

这能帮助外部观察者知道最近一次阶段，但 ready.json 还不是可安全复用的 live handle：

- 没有 schema_version、generation/sequence、expires_at 或 monotonic heartbeat；
- 没有进程 start time、可执行文件 hash 或 command identity 来防 PID reuse；
- AX 阶段只在第一次读回前写一次 sample=session-1-streaming，后续 session/attempt 没有更新 ready；如果进程在 finally 前崩溃，文件会永久停留在旧 phase；
- process_alive 是写入时快照，不是读取时验证；外部消费者若把它当成当前状态，可能接受过期 handle。

qa_identity 和唯一输出目录降低了不同 run 混淆的概率，但不能替代过期判断。后续原生复现应把 ready 当作 checkpoint evidence：读取者必须同时核对 qa identity、PID 当前命令行/启动时间和最近更新时间；不能仅凭 phase 或 process_alive 继续操作。若要让 ready 成为自动化控制面，至少需要单调 sequence、run nonce、有效期和每个阶段的开始/结束状态；当前代码尚未提供这些约束。

## 清理和脱敏

### 清理

finally 现在先停止并 join collector，再退出 app，停止 fixture，关闭 server，join server_thread，删除隔离 credentials，并写入 diagnostic-samples.json、ready.json、manifest/results（[native-streaming-qa.py#L2073-L2224](../verification/scripts/native-streaming-qa.py#L2073-L2224)）。collector 的 join 结果进入 manifest 和 ready；server thread 也有 8 秒 join 检查。

仍未形成所有线程的严格清理证明：FixtureHTTPServer.daemon_threads = True（[native-streaming-qa.py#L718-L725](../verification/scripts/native-streaming-qa.py#L718-L725)），handler 数量、handler 终止时间和 handler join 结果没有记录；server thread 与 collector 都没有统一的全局 cleanup deadline。当前能证明的是已知 collector/server thread 的 stop/join 结果和 QA PID 状态，不能证明不存在残留 daemon handler。

### 脱敏

当前边界基本保持正确：

- [redact_record](../verification/scripts/native-streaming-qa.py#L496-L513) 只允许 request marker、时间、计数、并发和字节数，不保存 Authorization、请求体或响应正文；
- ax_diagnostic_context 只写 controller 状态、run ID、active 标志和 fixture 计数；
- [redacted_error_type](../verification/scripts/native-streaming-qa.py#L125-L135) 供 collector/AX record 使用，避免把异常正文写进诊断字段；
- [assert_no_secret](../verification/scripts/native-streaming-qa.py#L1401-L1410) 会扫描 evidence 中的固定 fixture key，退出后删除隔离 credentials。

剩余风险需要在 review 中保留：summary["error"]、manifest["error"] 和 primary_error 仍由异常类型加原始 str(error) 组成；AX CalledProcessError 的 stderr 被截断后进入 AxError，如果未来 System Events 返回 UI 文本或敏感内容，可能通过失败 evidence 泄露。固定 fixture key 扫描也不能发现任意动态 secret、Authorization 或请求正文；而且 evidence scan 跳过 .app 路径。当前 round-4 fixture 没有证据表明 key 已泄露，但这不能替代对通用 secret 的过滤。后续若扩展诊断字段，应继续只写分类、长度和计数，不把异常正文纳入持久化 evidence。

## 本地 mygo 原生 AX 测试入口

这些入口可以在后续“若证据指向 AppKit/mygo accessibility 或 AX IPC”时构造最小原生复现；它们不能直接证明 GCode 或 System Events 的根因。

| 入口 | 能力 | 与当前 QA 的关系 |
| --- | --- | --- |
| [internal/darwin/testhooks.go#L568-L616](../../third_party/mygo/internal/darwin/testhooks.go#L568-L616) | TestAccessNode 和 TestAccessibility 在进程内 depth-first 读取 NSAccessibility tree，包含 role/subrole、label、value、frame、focus、depth 和 children | 可绕过外部 osascript，检查 mygo/AppKit 是否生成正确的 AX 树；不是 System Events IPC 测试 |
| [internal/darwin/testhooks.go#L618-L670](../../third_party/mygo/internal/darwin/testhooks.go#L618-L670) | TestAccessibilityPerform 支持 press/increment/decrement/focus/value；TestAccessibilityAttribute 读取旧式 AX 属性和 settable/named 状态 | 可验证 action/value/属性行为，不覆盖 GCode 的长流式 table 序列化 |
| [internal/darwin/testhooks.go#L672-L703](../../third_party/mygo/internal/darwin/testhooks.go#L672-L703) | TestObserve 对 window frame 和每个 accessibility value 做 KVO，返回显式 stop 函数 | 可观察原生对象值变化是否持续到 AppKit 层，适合区分 UI 更新问题与外部 AX 查询问题 |
| [internal/e2e/hooks_darwin_test.go#L180-L217](../../third_party/mygo/internal/e2e/hooks_darwin_test.go#L180-L217) | e2e adapter 通过 w.NativeHandle() 和 mygo.RunOnMain 调用上述 native hooks | 这是现成的主线程调用方式；不能从后台 collector 直接调用这些 hook |
| [internal/e2e/e2e_test.go#L2473-L2542](../../third_party/mygo/internal/e2e/e2e_test.go#L2473-L2542) | TestContentWindowAccessibility 验证按钮、checkbox、text field、slider 的 tree/value/action | 最小真实窗口 AX action/value 闭环 |
| [internal/e2e/e2e_test.go#L2544-L2600](../../third_party/mygo/internal/e2e/e2e_test.go#L2544-L2600) | TestContentWindowObserved 验证 resize、KVO observe、value 更新和旧式 AX attribute | 最接近“原生元素更新期间是否仍可观察”的现成 e2e |
| [internal/e2e/e2e_test.go#L2602-L2638](../../third_party/mygo/internal/e2e/e2e_test.go#L2602-L2638) | TestContentWindowListAccessibility 验证虚拟化 list rows、AXRow/AXTableRow 和按 row press | 可对照当前 stream table 的虚拟化/row 读取边界 |
| [ui/access_test.go#L27-L173](../../third_party/mygo/ui/access_test.go#L27-L173) 和 [#L175-L238](../../third_party/mygo/ui/access_test.go#L175-L238) | 无窗口的 AccessTree、action/focus、list scroll-into-view、row virtualization 单测 | 可快速验证 UI access tree/scroll 语义，但不经过 AppKit、Window、System Events 或 osascript |
| [internal/e2e/e2e_test.go#L1-L7](../../third_party/mygo/internal/e2e/e2e_test.go#L1-L7) | 真实桌面 e2e 入口为 MYGO_E2E=1 go test ./internal/e2e | 需要 desktop session；本轮按任务要求未运行 |

mygo 的 AX 实现本身在 [internal/darwin/access.go#L16-L27](../../third_party/mygo/internal/darwin/access.go#L16-L27) 说明 list 是 AX table/rows、只构建可视行，并通过 AXVisibleRows、AXRowCount 和 AXScrollToVisible 表达虚拟化。该源码可帮助后续判断，但不能把代码结构当作当前 GCode 原生运行证据。

## round-5/round-6 原生失败证据

round-5 和 round-6 使用同一个冻结 driver SHA；两轮都证明 loopback provider 能建立三条并发流，但分别在不同的 AX 阶段失败。证据入口为 [round-5/measurement-samples.json](../../output/acceptance/native-streaming/round-5/measurement-samples.json)、[round-5/results.json](../../output/acceptance/native-streaming/round-5/results.json)、[round-5/diagnostic-samples.json](../../output/acceptance/native-streaming/round-5/diagnostic-samples.json)、[round-5/profile-observer.json](../../output/acceptance/native-streaming/round-5/profile-observer.json)、[round-5/ax-phase-process-sample.txt](../../output/acceptance/native-streaming/round-5/ax-phase-process-sample.txt) 以及 [round-6/measurement-samples.json](../../output/acceptance/native-streaming/round-6/measurement-samples.json)、[round-6/results.json](../../output/acceptance/native-streaming/round-6/results.json)、[round-6/diagnostic-samples.json](../../output/acceptance/native-streaming/round-6/diagnostic-samples.json)。两轮正式包、源码 hash 和隔离 bundle code/resources 均保持不变，不能把失败归因于验收期间源码或原包被改写。

### round-5：第二次 session click 在 tail query 之前超时

round-5 的 session-1 click 从 `23:08:17.309+00:00` 到 `23:08:17.806+00:00`，耗时 496.159 ms；随后一次 tail query 耗时 730.415 ms，读回自己的 tail token，状态为 PASS。此时 controller/fixture context 显示三个 session 都是 running/active，fixture 有 3 个活动请求。第二次 `read_session_ax` 的 click 从 `23:08:18.555+00:00` 开始，到 `23:08:30.561+00:00` 以 12005.453 ms 失败，`click_success=false`、`error_type=ax_timeout`，`attempts=[]`；它没有进入 `read_ax_last_message_row`，所以这次失败不能归因于 tail-token query。对应目标在 [click_session](../verification/scripts/native-streaming-qa.py#L814-L841) 中是 sidebar 的具体 `button 2 of scroll area 1` AX 读取和 click。

失败前的第二个 send 流程仍完成了 `select_and_send_ms=1189.431`，第三个 send 流程也完成了 1177.057 ms；这只能说明发送阶段返回，不能把这两个总时长当成独立 click 延迟。第二次 click 超时结束时 context 仍显示 session-2 running/active、fixture 三请求 active。第三次 AX readback 没有开始，因为主循环在第二次 click 抛错后直接进入清理。

round-5 的 primary error 是 `AxError: osascript 超时`。清理阶段的 `quit_app` 也未能正常退出，最终 `exit_code=-9`，`cleanup_errors` 为 `QA 应用未通过正常退出流程`；server thread、隔离 credentials、collector 和 QA PID 的最终状态仍已清理。`ready.json` 的 `phase=stopped` 不能把 SIGKILL 后的进程状态写成正常退出。三条 provider record 最终各有 300 chunks 且无断连，但 `results.json` 没有走到 `server_streams`、journals 和 normal-exit 的主成功断言，这些记录只能说明 fixture 后续完成，不能把整轮判为 PASS。

round-5 的独立 collector 在前台 click 阻塞期间没有停止：90 次 controller poll、18 次 process sample、0 个错误，controller 间隔中位数 200 ms（193–209 ms），process 间隔中位数约 1001 ms（995–1008 ms）。样本时间从 `23:08:12.676+00:00` 到 `23:08:30.487+00:00`，最后一次 poll 仍看到三个 session running、三个请求 active。RSS 从 61128 KiB 上升到 90312 KiB，CPU 样本范围为 7.5%–103.4%；脚本没有性能门槛，这些数值不能直接写成性能失败。

### round-5 profile 与第二次 click 的时间重合

`profile-observer.json` 记录的采样区间为 `2026-10-11T23:08:22.819703+00:00` 至 `23:08:25.809503+00:00`；`ax-phase-process-sample.txt` 的 sample header 为 `2026-10-12 07:08:23.252 +0800`，两者是同一时间段。第二次 click 的 measurement 区间为 `23:08:18.555–23:08:30.561+00:00`，因此这次 2 秒 profile 采样与第二次 click 重合约 2.99 秒。call graph 的主线程样本包含应用代码后进入 `CTTypesetterCreateWithAttributedString` 及 CoreText 排版栈。

这只能证明采样窗口与第二次 click 重合，不能证明采样栈就是 click 超时的原因。profile 是系统 `sample` 的瞬时栈观察，不能区分“AX 请求触发了 UI layout”“流式内容更新正在排版”“sidebar AX 读取等待主线程”或其他主线程工作。

即使按 sample header 判断它与第二次 click 重合，CoreText 栈也只证明采样瞬间主线程正在执行排版相关代码；它不能区分“AX 请求触发了 UI layout”“流式内容更新正在排版”“sidebar AX 读取等待主线程”或其他主线程工作，更不能确认 Go/mygo 是根因。后续报告必须把这段作为 profile observation，不能作为根因结论。

### round-6：click 全部通过，第三次 tail query 耗尽 outer budget

round-6 把失败阶段推到了 tail query：session-1 click/query 分别为 405.483/426.459 ms 和 PASS；session-2 为 496.557/449.307 ms 和 PASS；session-3 click 也成功（510.268 ms），但 tail query 共 8 次。前 6 次 query 都正常返回、`success=true`、`tail_visible=false`，耗时 355.784–479.537 ms，remaining 从 17999.691 ms 降到 15000.701 ms；第 7 次收到 12000 ms，实际 12006.837 ms 后报 `ax_timeout`；第 8 次只收到 2097.989 ms，实际 2103.610 ms 后再次 `ax_timeout`。整个 session-3 readback 延迟 18518.624 ms，最终为 `WaitDeadlineExceeded`。

每个 session-3 attempt 的前后 context 都仍为目标 run running/active、fixture request_count=3、active_request_count=3；round-6 的 3 条 provider record 后续都完整收到 300 chunks，且 journal 各有 user、assistant、run_end 三条事件，最终 controller snapshot 三个 session 均回到 idle。由此可以确认“第三次 tail readback 在三流仍 active 时失败，provider 最终仍完成”，但不能确认是 table 内容长度、AX 可见行、主线程排版、System Events IPC 还是其他因素造成。

round-6 collector 有 126 次 controller poll、26 次 process sample、0 个错误并成功 join；controller 采样间隔中位数 200 ms（190–211 ms），process 间隔中位数 1000 ms（985–1024 ms），RSS 范围 60444–98412 KiB，CPU 范围 4.2%–106.5%。`exit_code=0`、`cleanup_errors=[]`、credentials/server/collector 均清理成功，但由于主流程在 tail readback 抛错，`results.json` 仍为 FAIL，且没有执行正常路径的 `server_streams`、journals、normal-exit 断言；最终文件只能证明失败后的清理和 fixture/journal 收尾。

### 当前能下的诊断结论

- round-5 和 round-6 不是同一个失败点：前者是 session-2 sidebar click 的 AX 超时，后者是 session-3 last-row tail query 在多次无 tail 读取后超时；不能把两轮合并为单一“AX 慢”或单一 CoreText 根因。
- round-5 在第二次 click 失败前，三流已经同时 running；round-6 的三次 click 都成功后，第三次 tail query 才失败。两轮都显示 collector 能独立采样，说明前台 AX 阻塞没有让诊断线程停止，但不说明 AX/UI 功能安全。
- round-6 前六次查询是快速“没有 tail”，第七、八次才阻塞到 timeout；这支持“可见 tail 尚未满足后，后续 AX query 进入长等待”的现象描述，但不能从当前 evidence 判断等待发生在哪一层。
- controller snapshot/journal 只能证明 bounded session/run ownership 和最终 provider/journal 收尾。journal 是每个 session 的少量聚合事件，缺少 chunk 序号、AX 调用事件和可与 attempt 对齐的 monotonic sequence；`controller-last.json` 是最终 idle snapshot，不能替代 timeout 瞬间 snapshot。

### 最小化下一步

root 已完成 `sessions=1` 的 round-7，并继续按单变量顺序缩小三 session 场景；移除 sidebar row index、三 session 并发和跨 session AX 切换的方向是合理的。后续结果仍不应修改生产代码或把 CoreText profile 当作根因。

后续只需按一变量顺序推进：如果单 session 仍在 tail query 失败，继续保持同一 session/同一 AX 断言，降低 stream 速率或内容增长量一次一项；如果单 session 通过，再用相同三 session 配置只改变 active stream 数量，区分“并发 active stream”与“sidebar/row index”影响。若需要重新取 profile，让采样窗口覆盖已由 measurement 明确标记的 click 或 query；保留 sample 原文但只作辅助观察。每轮继续分别报告 click、tail attempts、collector 完整性、provider/journal 收尾和清理状态，不能用后续 fixture 完成掩盖前台 AX 失败。

## P0/P1 判定和后续顺序

| 优先级 | 当前判定 | 还需的证据 |
| --- | --- | --- |
| P0 严格 deadline | **query budget 已完成；总 readback budget 取决于口径** | self-test 已沿 wait_until → read_session_ax → read_ax_last_message_row → ax → subprocess.run(timeout=...) 验证 `timeout <= remaining`、预算消耗后的失败和只发起一次 query。当前 18 秒从 click 完成后开始；若要求覆盖 click 加所有 query，还需把 click 纳入同一 deadline 后再验收 |
| P0 AX attempt 归属 | **分段 evidence 已生成，但 AX 功能仍未通过** | round-5 已定位到 session-2 click 超时且没有 tail attempt；round-6 已记录 session-3 的 8 次 tail attempt 和前后 controller/fixture context。仍需最小化运行确认失败条件，不能把两种失败合并成单一根因 |
| P0 独立采样 | **两轮原生结构证据通过；不构成功能 PASS** | round-5 为 90 controller/18 process samples、round-6 为 126/26，均 0 error 且成功 join；继续分别报告采样完整性与 AX 结果，不把 collector PASS 当作 AX PASS |
| P0 timeout 后状态安全 | **失败路径可收尾，功能判定未通过** | round-5 在 click timeout 后强制 kill，round-6 在 tail deadline 后正常清理；仍需保留失败时 session/run、attempt 和最终 journal 证据，不能把最终 idle snapshot 当成 timeout 时状态 |
| P1 ready freshness | **未完成** | run nonce、sequence/expiry、PID start identity 或明确声明 ready 仅为 checkpoint evidence |
| P1 清理 | **依赖失败路径：round-5 未通过，round-6 通过** | round-5 `exit_code=-9` 且 quit cleanup 失败；round-6 `exit_code=0`、cleanup_errors 为空。两轮仍缺 handler daemon thread 和统一 cleanup deadline 证据 |
| P1 脱敏 | **固定 fixture key 通过，通用 secret 边界有限** | 避免 raw exception text 进入持久化 evidence，并验证动态 Authorization/request body 不会进入新字段 |
| P1 性能/滚动 | **继续 NOT_ACCEPTED / INJECTED_ONLY** | 足量独立采样、预定义门槛、viewport 前后 marker、scroll 后 AX 和阅读位置证据 |

变量控制顺序：driver SHA 已冻结，self-test 已证明 per-query timeout 传递；round-5/round-6 已完成原生失败分段但尚未完成根因定位。round-7 单会话、round-8 双会话和 round-9 短三会话已通过，下一步继续按单 session → 单 active stream/低增长 → 三 session 的顺序一次只改变一个变量；这些通过只代表对应输入下的结果，不能覆盖 round-5/round-6 的失败。profile 只作为已覆盖目标 AX 段的辅助证据。round-5/round-6 报告必须分别写明 click 与 tail query 的失败阶段、outer budget 是否包含 click、collector 完整性、provider/journal 收尾和退出状态。若后续轮次未复现，只能报告“本轮未复现及已采样范围”，不能写成历史 AX 异常已修复。无论流式归属是否 PASS，持续 AX 延迟、RSS/CPU 和整体性能都继续单独判定。

## 当前最小 Markdown pending fallback 只读审阅

日期：2026-10-12。范围：只读审阅 [internal/desktop/markdown.go](../../internal/desktop/markdown.go)、[long_message_performance_test.go](../../internal/desktop/long_message_performance_test.go)、相关调用和本地 `third_party/mygo` 选择实现；不修改生产代码、测试或 benchmark，不运行测试、构建、窗口或其他验证进程。当前冻结 native driver SHA 仍为 `613200b2064a85ab6b60b08cc5960307d5a8c2af78809ff8429b41ffc207933c`。

结论：按行 pending fallback 的性能方向得到 benchmark 支持，但实现引入了确定性的跨行选择和复制回归，当前不能直接通过功能审阅。ready 后的 Markdown 渲染路径没有被这次改动触及；问题集中在异步 Markdown 结果尚未 ready 时的临时显示树。

### 改动边界和 benchmark 证据

当前 [markdown.go:352-371](../../internal/desktop/markdown.go#L352-L371) 在异步 Markdown 结果未 ready 时先执行 `strings.Split(source, "\n")`：非空行各自构造一个 `ui.RichText`，空行构造 `ui.Box(c).Height(15.5 * 1.55)`，并放入一个宽度为 100%、`MinWidth(0)` 的 `ui.Column`。ready 后仍走原有 `renderMarkdownBlocks`。修改前副本 [markdown-before.go.txt](../../output/acceptance/native-streaming/diagnostic-tools/markdown-before.go.txt) 使用一个包含完整 `source` 的 `ui.RichText(...).Selectable()`。

[long_message_performance_test.go:16-39](../../internal/desktop/long_message_performance_test.go#L16-L39) 将 `Dispatch` 设为空操作，因此解析结果不会进入 UI queue，测量持续落在 pending fallback；每轮执行 `app.Update(snapshot)` 和 `tester.Frame()`。fixture 包含中文、emoji、空行、Go fenced code 和 table，测试经过当前 Darwin 文本栈及 MyGo layout/leaf width 路径。已记录的 300 chunks 结果约从修改前的 `270 ms / 28 MB` 降到 `4.94 ms / 287 KB`，30、100、300 chunks 均有改善。这支持“pending fallback 的构建和布局成本显著下降”的判断。

该计时还包含 source 拼接中的 `fmt.Sprintf` 和 `app.Update` 的 snapshot clone，不能单独解释为纯布局时间。benchmark 没有断言显示文本、空行高度、选择、复制、Markdown 语义或 AX，因此不能把它升级为完整功能或 native 验收证据。

### 内容和空行的静态判断

`strings.Split` 只按换行分隔，不切分 Unicode rune，因此静态上中文和 emoji 不会因分行被截断；pending 状态仍显示原始 Markdown，fenced code、table 和列表语法仍按原文显示；ready 后的解析路径未变。CRLF 没有被显式规范化，按 `\n` 分出的普通行会保留末尾 `\r`；MyGo 文本层是否丢弃、显示或将其计入宽度和选择范围尚未验证。当前也没有测试证明超长无空格行、尾部换行、code block 内容以及 Unicode 内容在这个 fallback 下的实际显示完整性。

`ui.RichText` 的显式字号和行高为 `15.5 * 1.55 = 24.025 DIP`。本地 MyGo layout 源码显示 line height 按 `style.LineHeight * font size` 计算，空段落也推进一条 line height，因此普通 1x 文本缩放下，空行 `Box` 在行数上可以与原始 RichText 对齐，包括连续空行、尾部换行和单独的 `"\n"`。这仍是源码推导：硬编码 DIP 高度没有经过真实布局验证，也没有证据证明所有平台或系统文本缩放偏好下都与 RichText 保持一致；benchmark 同样没有高度断言。

### AX 导出层级和键盘语义

pending fallback 的 UI 树从“一个可选择的 RichText leaf”变成“一个 Column，下面有多个 RichText leaf 和代表空行的 Box”。这会改变原生/辅助功能树的层级和文本叶子数量；静态代码只能确认树形发生变化，不能确认 MyGo 最终导出的 AX role、value 拼接、子节点顺序以及空 `Box` 是否出现在 AX 文本中。round-12b 包含的是修改前源码，当前没有对应的 native AX 读取证据，因此不能把这次布局树变化解释成 AX 文本完整或 AX 回归已验证。

同一个 selection scope 的缺失也会影响键盘语义：`Cmd+A` 和 `Cmd+C` 受当前 RichText scope 限制，跨行的 Shift 组合、方向键扩展和拖选无法形成消息级范围；空行没有 selectable text 节点，复制时也没有可供复制逻辑连接的段落。需要在当前源码重新打包后，通过真实 AX 文本、焦点/selection 状态和键盘操作分别验收，不能由 benchmark 或 before 版本 frame stats 推断。

### 确定性的 selection/copy 回归

当前 fallback 的外层 `ui.Column` 没有 `.Selectable()`，每个非空行的 `RichText` 单独调用 `.Selectable()`。本地 [textinput.go:161-180](../../third_party/mygo/ui/textinput.go#L161-L180) 规定，直接对 Text/RichText 调用 `Selectable()` 时选择停留在该段落；只有对容器调用 `Selectable()`，其 Text/RichText 后代才共享同一 selection scope。[textselection.go:333-380](../../third_party/mygo/ui/textselection.go#L333-L380) 的复制实现也只遍历当前 scope，并以换行连接 scope 内段落。

由此可以直接推出以下功能回归：

- 跨多行拖选无法形成一个消息级 selection；
- `Cmd+A` 只能选择当前行；
- 跨行 `Cmd+C` 不能复制完整 pending 消息；
- 空行是普通 `Box`，不进入 selectable text 列表，复制时会丢失对应的空行数量；
- 单行内部的中文、emoji 和文本选择仍可工作。

[conversation.go:173-175](../../internal/desktop/conversation.go#L173-L175) 的“复制消息”按钮直接调用 `c.WriteClipboard(message.Text)`，该按钮仍可复制完整原文；回归影响文本拖选、上下文菜单和键盘选择路径。该项属于高优先级功能回归，性能收益不足以抵消它，当前不能标记为可接受。

### async cache 归属

当前 [markdown.go:374-440](../../internal/desktop/markdown.go#L374-L440) 的 cache 仍按 `sessionID + ":" + rowKey` 归属。`conversation.go` 使用 message ID，缺少 ID 时使用 index；新 source 会覆盖 entry 的 `text` 并标记 pending；异步完成只有在 `cached.text == source` 时才发布 ready；发现 source 已更新时会为最新 source 排队下一次 parse。session ID 也纳入 key。就静态代码而言，source freshness 和 stale async result 丢弃逻辑保持合理，本次 pending fallback 改动没有改变它。

尚未验证的风险包括：`post` 没有在入口检查 `closed`，close 与 `markdownDocumentForRender` 竞态时可能在 close 后启动一次最终不会写回 cache 的 parse goroutine；没有测试 pending 到 ready 切换期间 selection 状态、source 变为新行时 selection 的迁移，也没有重复 message ID 异常数据测试。这些是现有 async 生命周期的静态风险，不能据此推断本次 fallback 已造成对应故障。

### round-12b 证据的版本身份

round-12b 的 [manifest.json](../../output/acceptance/native-streaming/round-12b-frame-stats/manifest.json) 记录的 `internal/desktop/markdown.go` SHA 是 `7c51f3c11ba3f268e4d2b5318e0cb7ce96fef085ca99c7977b8b34be6566c254`，与 [markdown-before.go.txt](../../output/acceptance/native-streaming/diagnostic-tools/markdown-before.go.txt) 一致；该证据副本只改了文件名后缀，源码字节和 SHA 未变；当前工作树文件的 SHA 是 `2783490ceacddfa8b8dcc8dda4acc655d31f593fdf730bc6cf69b3c84866d9b8`。因此 round-12b 正式包使用的是修改前版本，不能证明当前按行 fallback 的中文、emoji、code block、table、空行高度、native AX 或首帧/更新帧行为。

round-12b 的 `frame-diagnostic.json` 自身标为 `DIAGNOSTIC_ONLY`，记录 `frame_count = 303`、`max_frame_ms = 2520.7`、`max_layout_ms = 254.2`，观察到的最高 `build_ms` 约为 `20.6 ms`。文件同时声明存在 logging/runtime-metrics overhead 和粗粒度本地时间戳，因此这些数据只能说明修改前包的诊断观察范围，不能写成当前修复的性能验收或 FPS 通过。

### 未验证范围和后续验收条件

后续实现需要让一个消息级 selectable container 统一承载 pending 文本，避免每一行成为 nested selectable scope；空行也应作为同一 selection scope 中的空 text paragraph，使复制结果保留换行数量。完成实现后至少需要重新核对：

1. pending 状态下中文、emoji、fenced code、table、CRLF、超长无空格行和尾部换行的完整显示；
2. 跨行拖选、`Cmd+A`、`Cmd+C`、上下文菜单复制，以及连续空行的复制结果；
3. 不同文本缩放和 native 布局下的空行高度，以及 pending → ready 切换时的布局和 selection 行为；
4. 使用当前源码 hash 重新生成 native 包，再分别记录 AX 文本、首帧/更新帧和 frame/layout 数据，避免继续引用 before 版本的 round-12b 证据。

在这些条件完成前，当前修复只能记为“pending fallback 性能 benchmark 改善，功能审阅未通过”；本节所述判断均来自静态代码和既有证据，未进行运行时验证。

## MyGo 共享 selection scope 与按段落 layout cache 的最小 seam

日期：2026-10-12。范围：继续只读核对当前 `internal/desktop/markdown.go`、本地 `third_party/mygo` 的 text/selection/layout/CoreText 实现、既有 pending benchmark 和已保存的 focused selection 记录；不修改 MyGo selection/cache/CoreText 实现，不新增 conversation 专用 cache。本节是对上一节的增量复核：上一节“确定性的 selection/copy 回归”描述的是旧的按行实现，当前源码已经采用下面的最小 seam，不能把旧失败直接当作当前代码的结论。

结论：当前 [markdown.go:352-366](../../internal/desktop/markdown.go#L352-L366) 的实现方向正确。pending fallback 由一个 `ui.Column(...).Selectable()` 统一承载；`strings.Split(source, "\n")` 的每个结果，包括空字符串，都创建带相同 `FontSize(15.5)`、`LineHeight(1.55)`、文本色和选择色的 `ui.RichText`；子 `RichText` 不再调用 `.Selectable()`。这同时复用了 MyGo 已有的消息级 selection scope、空 paragraph 处理和全局 text layout cache，满足最小改动原则。当前剩余的验收缺口在真实 native AX、窗口交互和性能证据版本身份，不在于需要再设计一套 MyGo selection 或 conversation cache。

### 静态可确认的 selection seam

- [textinput.go:161-180](../../third_party/mygo/ui/textinput.go#L161-L180) 规定：把 `Selectable()` 放在容器上时，Text/RichText 后代共享一个 selection；直接放在 Text/RichText 上时，选择只属于该段落；嵌套 selectable container 才会建立独立 scope。
- [textselection.go:27-88](../../third_party/mygo/ui/textselection.go#L27-L88) 的 `prepareSelectable` 会沿父级查找 selectable `kindBox`，把后代 paragraph 绑定到同一个 `textScope`，并为 selectable 的空 Text/RichText 创建只读 multiline editor。因此当前每行 RichText（含空行）会被纳入同一 scope。
- [textselection.go:90-100](../../third_party/mygo/ui/textselection.go#L90-L100) 会按 build order 收集非 inline selectable text；[textselection.go:333-380](../../third_party/mygo/ui/textselection.go#L333-L380) 的 `selectAll` 和 `copy` 遍历同一 scope，并在 paragraph 之间写入 `\n`。空 paragraph 仍占一个段落位置，所以连续空行和尾部换行不会因 UI 树中的空节点而丢失。
- [textselection.go:394-479](../../third_party/mygo/ui/textselection.go#L394-L479) 让 Shift+方向键、Shift-click 和跨 paragraph 上下移动使用共享 scope；本地已有 [textselection_test.go:25-71](../../third_party/mygo/ui/textselection_test.go#L25-L71)、[textselection_test.go:207-245](../../third_party/mygo/ui/textselection_test.go#L207-L245) 覆盖跨段拖选/复制、空段落、键盘扩展和 Shift-click。这些测试证明 MyGo seam 的行为，不等于当前 GCode 原生窗口已经通过。

当前代码保留每个 `line` 的原文，包括 `\r`。在 [internal/text/layout.go:743-750](../../third_party/mygo/internal/text/layout.go#L743-L750) 中，paragraph shaping 前会去掉末尾 carriage return，所以 CRLF 的 `\r` 不会成为可见 glyph；editor/source 仍保留它，copy 再以 paragraph 文本拼接 `\n`，静态上可还原 CRLF。把 `"\r"` 当作空行会丢失这个字符，当前实现没有这样做。

### 静态可确认的空行高度和 native layout

[ui/layout.go:384-389](../../third_party/mygo/ui/layout.go#L384-L389) 的 text content height 直接使用 `textSystem().Layout(...).Height`；[ui/layout.go:425-439](../../third_party/mygo/ui/layout.go#L425-L439) 在最终布局时对 text node 执行 `prepareSelectable` 并复用同一个 layout 给 selectable editor。`internal/text/layout.go` 的 [metricsOf:661-671](../../third_party/mygo/internal/text/layout.go#L661-L671) 按 `LineHeight * FontSize` 计算显式行高，当前样式为 `1.55 * 15.5 = 24.025 DIP`；[layout:685-705](../../third_party/mygo/internal/text/layout.go#L685-L705) 和 [paragraph:739-792](../../third_party/mygo/internal/text/layout.go#L739-L792) 对空字符串也创建一条空 shaped line 并推进一条 native line height。因此空行使用同样样式的空 RichText，能让显示高度、selection paragraph 和复制换行共享同一条 native text 路径；继续用固定高度 Box 会绕开这条路径。

### text.System、leafWidths 与 CoreText cache 的复用边界

- [internal/text/layout.go:506-535](../../third_party/mygo/internal/text/layout.go#L506-L535) 的 `System.Layout` 以完整 `text.Params` 为 key，命中则直接返回已有 layout；预算为 8 MiB、4096 entries（[layout.go:539-542](../../third_party/mygo/internal/text/layout.go#L539-L542)）。常规 UI layout 走 `Layout`，不是不缓存的 `Shape` 入口。
- [ui/layout.go:253-280](../../third_party/mygo/ui/layout.go#L253-L280) 的 `leafWidths` 对 text node 使用 width=0 的 max-content probe、width=1 且 `NoBreakWords=true` 的 min-content probe，随后再按实际可用宽度布局。`leafOK`、`measures`、`tl` 是当前 build/frame 的临时 node 状态；可跨 frame 复用的是 `text.System` 的 Params LRU。
- [internal/text/coretext_darwin.go:816-895](../../third_party/mygo/internal/text/coretext_darwin.go#L816-L895) 的 Darwin backend 接收一个 paragraph，创建 attributed string 和 CoreText typesetter，再按建议换行生成 runs/glyph positions。MyGo 外层负责按换行拆分 paragraph；当前按行 RichText 正好复用这个既有 paragraph seam，不需要改 CoreText backend。

由此得到的性能判断是推断而非运行时保证：只要未变化行的 `Params.Text`、字体/行高、span 编码和 width 完全相同，它们的 width probes 和最终 layout 可以命中全局 cache；累计 source 增长时，通常只有最后一个变化行产生新的 Params 并重新 shaping。不同实际宽度、样式、span 或 LRU 淘汰会造成 miss，不能把“按行”写成所有行都必然零 layout 成本。这个边界已经足以支持复用现有 cache，不足以支持新增一层 conversation paragraph cache。

### 旧回归与当前 focused 证据边界

旧实现的失败记录保存在 [selection-before.log](../../output/acceptance/native-streaming/diagnostic-tools/selection-before.log)：旧的每行独立 selectable/空 Box 形态在 Unicode Markdown、CRLF、长行、跨空行拖选四个场景中分别只复制当前行或首段。当前源码对应的 [pending_text_selection_test.go](../../internal/desktop/pending_text_selection_test.go) 通过 `ui.NewTester` 调用真实 `renderTextBlocks`；[selection-after.log](../../output/acceptance/native-streaming/diagnostic-tools/selection-after.log) 记录该 focused 包测试通过。它支持“当前共享 scope 修复了已复现的 UI tester selection/copy 回归”，不能扩展为真实 AppKit、AX 或物理键盘通过。

现有 pending benchmark 仍可直接复用，因为 [long_message_performance_test.go:16-39](../../internal/desktop/long_message_performance_test.go#L16-L39) 的空 `Dispatch` 让渲染保持在 pending fallback，并经过当前 MyGo layout、`leafWidths` 和 Darwin text stack。当前保存的 [growing-selection-fixed.log](../../output/acceptance/native-streaming/diagnostic-tools/growing-selection-fixed.log) 记录 30/100/300 chunks 分别约 `1.34/2.59/4.64 ms`；这说明当前实现仍保留按行拆分的性能方向。该 benchmark 不断言 selection、copy、空行高度、AX value 或 ready transition，也不能替代同一机器、同一源码身份下的 before/after 正式对照。

### 当前仍未验证的风险和验收条件

当前 focused 记录没有验证以下事实：真实 native AX 是否把多个 RichText 按正确顺序导出，空 paragraph 和 CRLF 在 AX value 中如何表现，AppKit 窗口中的拖选/上下文菜单/Shift+方向键是否与 `ui.NewTester` 一致，以及 pending → ready 重建后 selection 是否按产品预期迁移。`round-12b` 仍是修改前 `markdown.go` 的包和 frame stats，不能证明当前源码；native driver SHA 继续冻结为 `613200b2064a85ab6b60b08cc5960307d5a8c2af78809ff8429b41ffc207933c`。

当前实现要进入功能通过，至少要用当前源码重新取得以下证据：

1. pending 跨行拖选、`Cmd+A`、`Cmd+C`、Shift+方向键和上下文菜单复制；
2. 中间空行、连续空行、尾部空行及尾部换行的复制结果，覆盖 LF、CRLF、单独 `\n`、中文和 emoji；
3. fenced code、table、超长无空格行的显示，以及不同文本缩放下空 RichText 的实际高度；
4. 同一 benchmark fixture 的 before/after 数据，明确区分 build/layout、allocation、source 拼接和 snapshot clone 的成本；
5. 重新打包当前源码后取 native AX tree/value、首帧/更新帧和 frame/layout stats，禁止继续引用 round-12b 的 before 包作为当前证据。

因此，当前判定应从“旧按行 fallback 有确定 selection/copy 回归”更新为“最小共享 scope 修复已通过 focused UI tester 边界，按段落 cache 复用有源码依据，真实 native AX、窗口交互和当前包性能仍未验收”。

## Markdown async freshness、同 key replacement→append 与空 source 只读复核

日期：2026-10-12。范围：只读核对当前 [markdown.go:353-452](../../internal/desktop/markdown.go#L353-L452) 的 `documentText`/`markdownPending` 生命周期、[conversation.go:141-163](../../internal/desktop/conversation.go#L141-L163) 的同 key 渲染入口和 [markdown_test.go:156-238](../../internal/desktop/markdown_test.go#L156-L238) 的 focused 场景；不修改生产代码、测试或 benchmark，不运行测试、构建或窗口验证。

结论分两层：`documentText` 保留已解析 prefix、stale completion 只在最新 source 以旧 source 开头时发布、随后始终继续 parse 最新 source，这个并发方向成立；但当前 replacement 判定仍有一个确定的静态漏洞，而且真实 conversation 的空 source 不会进入该判定路径。当前不能把“普通 append → stale completion → latest completion”通过，扩展成所有同 key replacement/empty source 交错都安全。

### 当前实现已经建立的 freshness 事实

- `cachedMessageRender.text` 表示该 key 最近一次看到的完整 source，`documentText` 表示 `document` 实际由哪个 source 解析而来；pending frame 在 `document.blocks` 非空时继续显示已解析 document，否则回退到按行 RichText。
- [markdown.go:398-418](../../internal/desktop/markdown.go#L398-L418) 在 source 变化时保留已有 document，随后把最新 source 写入 `cached.text` 并标记 `ready=false`。`markdownPending` 在锁内只允许一个解析链路继续，避免每个 SSE frame 都启动一个 parser。
- [markdown.go:429-445](../../internal/desktop/markdown.go#L429-L445) 在 completion 回调中先比较 `cached.text == source`；只有相等时才把结果标记为完整 ready。source 已经前进时，只有 `strings.HasPrefix(cached.text, source)` 才发布该旧 document 作为最新 source 的 parsed prefix，然后无条件以 `cached.text` 排队下一次 parse。旧 completion 因此不能把一个不相关 replacement 标记成 ready，也不会因为没有下一帧而永久丢掉最新 source。
- callback 和 cache/pending 状态均在 `a.mu` 下更新；`a.closed` 会阻止 completion 写回。parser goroutine 本身仍会跑完，但关闭后的 callback 不再发布 document。这是源码层的状态顺序事实，不是 native 窗口退出证据。

### 确定的 replacement 判定漏洞

当前 append 保留条件是：

```go
if current.documentText == "" || !strings.HasPrefix(source, current.documentText) {
    current.document, current.documentText = markdownDocument{}, ""
}
```

这里比较的是新 source 与“最后已经解析的 prefix”，不是与 `current.text`（上一个最新 source）比较。假设 `current.text = A+B`、`current.documentText = A`，随后同一 key 被 replacement 为 `A+C`：`A+C` 仍以 `documentText` `A` 开头，旧 document 会被保留；但新 source 并不是旧最新 source `A+B` 的 append。pending frame 会继续显示只代表 `A` 的旧 Markdown 结构，并隐藏 `C`，直到 replacement parse 完成。若 replacement 改写了 `A` 之后的 fenced code、table 或 block 边界，这个中间结果还可能把不属于新 generation 的结构展示出来。

因此“replacement 立即清掉旧 prefix”的注释和当前实现并不完全一致。最小静态修复方向是以 `current.text` 判定 append 关系（并把空的 `current.text` 当作无可复用 prefix），而不是以 `current.documentText` 判定；stale completion 继续以“旧 parsed source 是最新 source 的 prefix”作为发布条件。这里不提出或实施生产代码修改，仅记录当前审阅阻塞点。

### 空 source 与真实 conversation 入口

如果空 source 直接经过 `markdownDocumentForRender`，当前路径的行为基本安全：旧 `documentText` 非空时，`source == ""` 不满足 prefix，document 会被清掉；旧非空 completion 看到 `cached.text == ""` 时不会通过 `HasPrefix("", oldSource)`，所以不能把旧 body 写回。空 source 的 parser 结果本身 `documentText` 为空，不含可泄漏的旧文本；之后新 source 会继续排队解析。

真实 conversation 存在另一条未覆盖路径。[conversation.go:156-163](../../internal/desktop/conversation.go#L156-L163) 在 `message.Text == "" && len(message.Tools) == 0` 时直接显示“正在生成…” spinner，不调用 `renderTextBlocks`，也就不会把该 key 的最新 source 记录为 `""` 或清理旧 cache。若同一 message key 经历 `A → "" → B`，且 B 恰好以旧 parsed `A` 开头，下一次 `renderTextBlocks` 会把它误判为 A 的 append 并继续复用旧 document；这正是 replacement→append 回退与空 source 竞态的组合。若 B 不以 A 开头，旧 prefix 会被清掉，问题不出现，但不能把这个分支差异当作完整 freshness 保证。

`conversationMessageKey` 对有 ID 的消息按 `sessionID + "message:" + ID` 归属，对无 ID 的消息按 session 内 index 归属（[conversation.go:86-90](../../internal/desktop/conversation.go#L86-L90)）。因此同一 ID 的重新生成、或无 ID 列表中 index 被复用，都会命中同一个 Markdown cache；当前 cache 没有独立 generation/empty reservation。是否所有 controller replacement 都会更新 message ID 尚未由本审阅确认，不能用通常的 streaming append 假设覆盖这一风险。

### 现有 focused 测试的边界

[TestMarkdownAsyncResultCannotReplaceNewerSource](../../internal/desktop/markdown_test.go#L156-L183) 覆盖完全不相关的 `old **value** → new _value_`，验证旧 completion 不能替换 newer source。[TestMarkdownAppendKeepsParsedBodyWhileCompletionWaits](../../internal/desktop/markdown_test.go#L185-L238) 覆盖已有 parsed body、一次 append、下一次 append 在前一个 completion 等待期间到达、发布有效 prefix、最终发布 latest，以及最后切换到完全不相关的 replacement。它能捕获先前“pending append 直接退回 raw Markdown”的回归，但没有覆盖：

- `A+B → A+C` 这种与已解析 prefix 相同、但与上一个完整 source 不同的 replacement；
- 真实 `renderMessage` 的 `A → "" → B`，尤其 B 以 A 开头的情况；
- 空 source parser completion 与旧非空 parser completion 交错时的 frame 顺序；
- fenced code/table 等 Markdown 结构在 prefix parse 后被后续 source 改写的中间显示；
- native AX value、ready transition、窗口关闭期间的 dispatch callback。

现有测试源码是 focused `ui.NewTester` 证据；本轮没有把未完成的全 desktop 结果写成 PASS，也没有运行测试。`long_message_performance_test.go` 仍只能说明 pending layout 的成本方向，不能证明上述 freshness 状态机。

### 需要补充的最小验收条件

1. 在同一 key 上先得到 `current.text=A+B、documentText=A`，再输入 `A+C`，确认 pending frame 不再显示 A 的旧 parsed body；随后确认 latest `A+C` 成为唯一 ready document。
2. 通过真实 `renderMessage` 覆盖 `A → "" → B`，且 B 以 A 开头；验证空 source 已成为该 key 的新 generation，旧 completion 和旧 prefix 都不能回退到 A。
3. 交错旧 parse、空 source parse、replacement parse 和 append parse 的 completion 顺序，逐次断言 `cached.text` 始终是最新 source、`documentText` 始终对应 `document`、非空 document 只能在其 source 是当前 source 的 prefix 时发布。
4. 覆盖 replacement 改写 fenced code、table、尾部换行、CRLF、中文和 emoji 的情况下，最终 source 和 ready document 一致；中间 prefix 只作为暂态优化，不得成为最终状态。
5. 当前源码重新打包后，再补 native AX/value、pending→ready 和关闭窗口证据；round-12b 的 before 包不能证明这条 freshness 修复。

当前判定：append freshness 的源码方向可接受；`documentText` 与 `current.text` 混用、以及真实空 source 未登记，仍是同 key replacement/append 的 P1 风险。未补齐这两类 focused 场景前，不能把当前 Markdown cache 记为完整通过。

## Markdown cache generation 修复后的只读复核

日期：2026-10-12。范围：复核 root 后续更新的 [markdown.go:19-28](../../internal/desktop/markdown.go#L19-L28)、[markdown.go:399-461](../../internal/desktop/markdown.go#L399-L461)、[conversation.go:141-173](../../internal/desktop/conversation.go#L141-L173) 和 [markdown_test.go:240-325](../../internal/desktop/markdown_test.go#L240-L325)；不运行测试、构建或窗口验证，不修改生产代码。本节 supersede 上一节中关于 `documentText`/`current.text` 混用和空 source 未登记的两个当前风险判断；上一节描述的是修复前状态。

结论：前一节指出的两项确定性漏洞已经在当前源码中被针对性修复。现在 append 判定比较前一个完整 `current.text`，非 append 会递增 generation 并立即清除 parsed document；真实 message 进入空文本 spinner 时也会为同一 cache key 写入空 entry 和新 generation；stale completion 只有在 generation 相同且 source 确实是当前 source 的 prefix 时才可发布。已有 focused red/green 记录支持这三个场景的 UI tester 行为，但这仍是 desktop package 级证据，不是 native AX 或真实窗口通过。

### 当前源码已修复的状态边界

- [cachedMessageRender](../../internal/desktop/markdown.go#L19-L28) 新增 `generation`。`text` 仍表示最新 source，`documentText` 表示 document 的解析 source；generation 把同一 key 的新一轮 replacement/empty reset 与旧 parser 结果隔离。
- [markdown.go:399-412](../../internal/desktop/markdown.go#L399-L412) 现在以 `appendOnly := strings.HasPrefix(source, current.text)` 判断是否为前一个完整 source 的 append。非 append 会 `generation++` 并清空 `document`/`documentText`；只要 source 不是旧 source 的扩展，就不会因为共享较短 parsed prefix 而保留旧 Markdown 树。
- [markdown.go:427-459](../../internal/desktop/markdown.go#L427-L459) 将 generation 传入 parser。completion 只有在 `cached.generation == generation && cached.text == source` 时才能发布 ready；stale append 只有在 generation 仍相同且 `cached.text` 以旧 source 开头时才发布 parsed prefix。generation 不匹配时仅取当前 `cached.text`/generation 继续 parse，不会把旧 generation 的 document 写回。
- [conversation.go:143-151](../../internal/desktop/conversation.go#L143-L151) 在空 message 进入 spinner 前，为已有非空 cache entry 写入空 `cachedMessageRender` 并递增 generation。这样 `A → "" → B` 即使 B 以 A 开头，也不会把 A 当作新一轮 append；旧 parser completion 也无法凭相同 source 或 prefix 重新激活旧 document。
- 空 entry 保留在 map 中而不是删除，避免旧 completion 读到不存在的零值 entry 后以 generation 0 误判为当前结果。这是当前 reset 设计中必要的归属事实。

### 既有失败与当前 focused 证据

前一版的两个确定性问题有对应失败记录：[cache-replacement-before.log](../../output/acceptance/native-streaming/diagnostic-tools/cache-replacement-before.log) 记录共享 prefix replacement 仍显示旧 parsed body，[cache-reset-before.log](../../output/acceptance/native-streaming/diagnostic-tools/cache-reset-before.log) 记录空 spinner 重建和旧 completion 复活均失败。当前 [TestMarkdownReplacementWithSharedPrefixClearsParsedBody](../../internal/desktop/markdown_test.go#L240-L265)、[TestEmptyMessageInvalidatesParsedBody](../../internal/desktop/markdown_test.go#L267-L293) 和 [TestEmptyMessageRejectsEarlierCompatibleParse](../../internal/desktop/markdown_test.go#L295-L325) 分别覆盖了这三条回归边界。

已有 [cache-generation-desktop-after.log](../../output/acceptance/native-streaming/diagnostic-tools/cache-generation-desktop-after.log) 记录 `ok gcode/internal/desktop 1.013s`。结合测试源码可确认：

- shared-prefix replacement 在 pending frame 显示 raw replacement source，不再显示旧 parsed body；
- `renderMessage` 的空 spinner 会清掉同 key 的旧 parsed prefix，随后新的 source 即使以旧 source 开头也不会复用旧 document；
- empty reset 之后到达的旧 compatible parse completion 不会发布旧 prefix，当前 generation 会继续排最新 source。

这些记录只证明 focused `ui.NewTester` 与 `internal/desktop` 测试边界；不能证明 CoreText/native AX、真实 AppKit 输入、上下文菜单、pending→ready 的原生 selection 或关闭窗口时的 UI queue 行为。

### 当前仍需保留的限制

- parsed prefix 是暂态显示优化。Markdown 的 fenced code、table、列表和 block 边界可能因后续 append 改变；当前逻辑最终会解析 latest source，但本轮 focused 测试没有覆盖每一种结构在 prefix 阶段的视觉过渡。
- `markdownCache` 仍按 `sessionID + rowKey` 归属，缺少 ID 时按 index；generation 能隔离同 key 的 source generation，不能修复错误的 row key 或跨 session/session 重建时的 cache 生命周期。session/message ID 复用和删除后的容量回收仍需单独审阅。
- `dispatch == nil` 的同步解析路径没有建立独立 parser generation；当前生产入口在启动后使用稳定的 native dispatch，代码层仍未通过“解析进行中动态切换 dispatch”场景验证。该场景不影响本轮已覆盖的固定 dispatch replacement/empty 流程，但不能把同步测试结果扩展为任意 host hook 生命周期保证。
- `cache-generation-desktop-after.log` 不是 native 包、AX tree/value、真实窗口性能或 frozen driver SHA 的证据；round-12b 仍是 before 包，不能用来验收本修复。

当前判定应更新为：前述 shared-prefix replacement、空 spinner reset 和旧 completion 复活问题已完成 focused desktop 修复并有保存的 green 记录；Markdown cache freshness 在源码和测试边界内可接受。native AX、真实窗口 selection/菜单、Markdown 结构过渡、cache 生命周期和当前源码打包性能仍保持未验收状态。

## 后台 specialist 集成后的只读核验

日期：2026-10-12。范围：只读核对后台 specialist 集成清单、主 checkout 的职责链和 CLI 入口；不运行测试、构建或窗口验证，不修改生产代码和测试代码。

### 集成清单身份

[integration-manifest.json](../../output/acceptance/background-specialist/integrated/integration-manifest.json) 记录了本次集成的边界：staging 为 `/tmp/gcode-background-specialist-stage`，`baseline_sha256` 为 `66e316e13d92c2cf116fc54d9b1ec1ab1752310477dc162e61e3b4b8c432e3f1`，状态为 `INTEGRATED_NOT_YET_MAIN_REGRESSION_TESTED`。清单包含以下 13 个精确集成路径：

- `internal/application/background_specialist_lifecycle_test.go`
- `internal/background/manager.go`
- `internal/background/manager_test.go`
- `internal/runtime/background_lifecycle_test.go`
- `internal/runtime/runtime.go`
- `internal/specialist/accounting.go`
- `internal/specialist/exec.go`
- `internal/specialist/lifecycle_test.go`
- `internal/specialist/runtime.go`
- `internal/specialist/runtime_test.go`
- `internal/specialist/stop_tool.go`
- `internal/specialist/task_tool.go`
- `internal/specialist/task_tool_test.go`

清单另记 `desktop_files_unchanged: 21`。因此该清单表达的是已完成代码集成、尚未经过主回归测试的状态，不能把它写成主回归或产品验收 PASS。

### 主 checkout 的职责链

静态链路可以从 Application 层一直追到持久化后台管理器：

1. [internal/application/service.go:18-30](../../internal/application/service.go#L18-L30) 创建共享 `runtime.Engine`，再把它注入 `controller.Controller`。
2. [internal/controller/controller.go:151-211](../../internal/controller/controller.go#L151-L211) 优先复用传入的 Engine；没有传入时才创建共享 runtime，避免为 controller 另起一套后台 runtime。
3. [internal/runtime/runtime.go:436-451](../../internal/runtime/runtime.go#L436-L451) 每次 Engine run 获取共享 specialist runtime，并将其注入 `specialist.Executor.BackgroundRuntime`。
4. [internal/specialist/runtime.go:33-67](../../internal/specialist/runtime.go#L33-L67) 延迟取得共享 background manager；[internal/specialist/exec.go:458-555](../../internal/specialist/exec.go#L458-L555) 注册后台任务，并通过共享 runtime 启动 in-process child。
5. [internal/background/manager.go:69-95](../../internal/background/manager.go#L69-L95) 及 [manager.go:201-229](../../internal/background/manager.go#L201-L229) 负责任务 metadata 的持久化和读取；[manager.go:257-382](../../internal/background/manager.go#L257-L382) 负责 in-process cancel、PID kill 和状态收尾。后台目录由 `engine.DataDir()/background` 确定。

这条链路说明后台任务由 Application 创建的共享 Engine 统一承载，specialist 执行器和 background manager 没有从 CLI 入口另行分叉。它是静态职责关系，仍不等于主回归运行结果。

### 关闭、取消和清理关系

[internal/specialist/runtime.go:69-125](../../internal/specialist/runtime.go#L69-L125) 用 `StartTask`/`FinishTask` 维护 in-process child owner。关闭 specialist runtime 时，[runtime.go:155-212](../../internal/specialist/runtime.go#L155-L212) 先执行 `KillRunning`，等待正在运行的任务，清理 prompt 文件，等待上限为 5 秒；[internal/runtime/runtime.go:746-787](../../internal/runtime/runtime.go#L746-L787) 的 `Engine.Close` 负责关闭共享 specialist runtime，并处理其 lazy 生命周期。这样，任务的进程终止、owner 收尾、prompt 清理和持久化 metadata 状态收尾属于同一条关闭边界。

[internal/specialist/exec.go:558-622](../../internal/specialist/exec.go#L558-L622) 继续负责 child 输出、状态和 accounting；后台 manager 的 in-process cancel 与 PID kill 则提供任务级终止入口。静态检查能够确认这些职责相互接上，但由于 manifest 明确标记为 `INTEGRATED_NOT_YET_MAIN_REGRESSION_TESTED`，不能据此声称关闭竞态、跨进程恢复或 accounting 已通过主回归。

### 与 Markdown cache 修复的边界

Markdown 路径仍保持当前源码中的 generation/append 判定和 empty reset：

- [internal/desktop/markdown.go:19-28](../../internal/desktop/markdown.go#L19-L28) 的 generation 仍隔离同一 key 的 replacement、append 和旧 completion；[markdown.go:399-461](../../internal/desktop/markdown.go#L399-L461) 仍以完整 `current.text` 判定 append，并按 generation gate completion。
- [internal/desktop/conversation.go:141-173](../../internal/desktop/conversation.go#L141-L173) 的空 message spinner 仍会为同一 key 建立空 entry 和新 generation，防止 `A → "" → B` 复活旧 parsed document。
- 集成清单未包含 `internal/desktop`，并明确记录 21 个 desktop 文件未改变；因此后台 specialist 集成没有覆盖或改变这条 Markdown cache freshness 路径。

### CLI 入口残留检查

[cmd/gcode/main.go:33-109](../../cmd/gcode/main.go#L33-L109) 只创建 `application.Service`、desktop view 和 native dispatch。对 `cmd` 的只读搜索没有发现 `background.NewManager`、`specialist.NewRuntime` 或其他独立后台启动入口；当前静态证据不支持存在 CLI 专用后台 manager、specialist runtime 或重复启动残留。

当前结论：后台 specialist 的静态集成边界成立，主回归测试尚未完成。该结论不扩展到 AX、真实窗口交互、产品行为或性能验收。

