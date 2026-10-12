# 行为与所有权审查记录

日期：2026-10-12（Asia/Shanghai）

## 结论

本轮没有发现当前源码中的确定性生产缺陷。S2 的跨层和事件归属证据已达到本地协议替身范围内的闭环；RunID、render snapshot、主动取消 warning 和 pending presentation 的最小行为证据均已补上。最新全仓 test/vet/fmt、受影响 desktop/controller race、package、codesign 和对应最新 macOS Intel 原生记录均通过各自声明的范围。剩余内容是测试证据边界：并发测试没有强制物理重叠，Markdown 复用仍以 block 指针作为核心缓存判据，ActiveRun 仍依赖只读前提，原生记录不覆盖长授权内容滚动、真实 API 或用户视觉认可。

本记录由只读源码审查和 root 已提供的日志、包及原生记录组成。本代理没有重新运行命令，因此“通过”均指向记录中的 exit 0 或 PASS；最新原生证据使用与 package manifest 相同的 `d0102a642e6c73f6a41b76070dce74e1a6de0fcfaaef9d1738a9a96fe805666a` executable，旧的 native-pending-ui/run-identity-final 仍只作历史记录。

## 一、S2 共享编辑

### 生产调用链事实

测试注释明确声明 HTTP server 只替代模型协议，不替代工具执行：internal/application/shared_edit_integration_test.go:33-36。

newSharedEditService 用真实 application.New 初始化 Service，保存 fixture provider，创建真实项目和 session：internal/application/shared_edit_integration_test.go:358-413。Service.Send 直接进入 controller：internal/application/service.go:80-85；Controller.Send 建立 active run 并启动 execute：internal/controller/controller.go:518-582；execute 将 session、cwd、provider 和回调交给 runtime runner：internal/controller/controller.go:993-1032。

runtime run 为每次运行创建真实 tools.FileTracker，并把 registry、tracker 和 session ID 交给 agent.Run：internal/runtime/run.go:34-61、internal/runtime/run.go:207-216。read_file 会记录真实文件 hash：internal/tools/read_file.go:126-175；edit_file 和 write_file 在写入前用 tracker 检查磁盘是否变化：internal/tools/edit_file.go:63-80、internal/tools/write_file.go:77-95。因此 fixture provider 只产生模型 tool call，read_file、edit_file、write_file 仍由生产 registry 执行。

### 先读后竞争和场景事实

每个参与 session 由 sharedEditSendConcurrently 并发调用 Service.Send：internal/application/shared_edit_integration_test.go:323-356。provider 第一次请求只返回 read_file；第二次请求先检查 body 包含真实 read 输出，再在 sharedEditBarrier.Arrive 处等待全部参与者到达，随后才返回写工具：internal/application/shared_edit_integration_test.go:212-250、277-320。这个 barrier 证明双方都已完成 read_file 语义后才进入写入决策；它不能被描述成 CPU 层面的持续物理并发。

三个场景和两种写工具均被覆盖：同文件旧读碰撞要求一个成功、一个带 changed on disk since you last read it 的冲突，并核对最终内容是完整胜者内容：internal/application/shared_edit_integration_test.go:52-96；外部编辑在第二轮 provider 请求前直接写真实项目路径，要求生产 tracker 拒绝陈旧写入并保留 outside-editor 内容：internal/application/shared_edit_integration_test.go:98-131；不同文件场景要求两个完整写入成功并分别核对内容和 git dirty 状态：internal/application/shared_edit_integration_test.go:133-163。

屏障、并发发送等待和 session 轮询均有 context 或 timer 截止时间；provider 使用 httptest.Server，Service 使用 Close，项目和数据目录来自 t.TempDir：internal/application/shared_edit_integration_test.go:181-191、277-356。外部编辑 fixture 与生产工具使用真实文件路径，未替代 tracker：internal/application/shared_edit_integration_test.go:98-105。

### 最新事件归属断言

最新 sharedEditAssertSession 已把每个 session 限定为一个精确 prompt、一个 read_file 和一个当前写工具，并检查 read/write tool ID、JSON 参数、read 输出、write 状态和输出：internal/application/shared_edit_integration_test.go:507-569。路径、old_string/new_string 或 content/overwrite 由当前 marker 的 fixture 精确生成：internal/application/shared_edit_integration_test.go:509-547。

sharedEditAssertHistory 读取真实 sessions store 的 events.jsonl，并要求每个事件的 event.SessionID 等于当前 session ID；它还逐项核对用户消息、assistant toolCalls、tool_call 事件、两个 tool_result 的 ID、工具名、状态和输出：internal/application/shared_edit_integration_test.go:583-669。sharedEditAssertNoOtherSessionFields 会拒绝其他 session marker、不同路径和不同写入值混入该历史：internal/application/shared_edit_integration_test.go:797-825。

provider 第三轮请求体由 writeBodies 保存并由 sharedEditAssertProviderBody 实际读取。该断言检查最终 provider 请求的用户消息、assistant function tool call 的 type/ID/name/arguments，以及 tool result 的 ID/content；同样拒绝其他 session marker、路径和写入值：internal/application/shared_edit_integration_test.go:245-250、564-568、691-784。writeBody 的读取受 mutex 保护：internal/application/shared_edit_integration_test.go:841-845。

因此，上一轮“事件只按工具名筛选、provider body 未使用”的 P2 缺口已经关闭到当前审查范围。ProviderCallID 字段没有单独做独立一致性断言，但 session ID、tool ID、参数、结果和最终请求体均已逐项绑定。

### 现成验证记录

root 提供的 S2 记录均为 exit 0：

- output/acceptance/shared-edit-service/focused-ownership-regular.txt：edit_file/write_file 的同文件、外部编辑、不同文件六个子场景全部 PASS。
- output/acceptance/shared-edit-service/focused-ownership-race.txt：同一六个子场景在 race 模式全部 PASS。
- output/acceptance/shared-edit-service/focused-regular.txt、focused-race.txt、vet.txt：同一测试链的普通、race 和 vet 记录也为通过。

这些记录证明的是本地 Chat Completions 协议替身和生产内置工具的行为，不证明真实 provider API。

## 三、RunID 单次生成与跨层归属

### 生产路径事实

Controller 在 startRunLocked 中为 activeRun 生成一次 runID，并把它同时写入 session.RunID、ActiveRun 状态和 user message：internal/controller/controller.go:969-990。execute 构造 runtime.Input 时原样传入 run.RunID，随后把同一 input 交给 runner：internal/controller/controller.go:993-1032。

Engine.Run 在校验、载入历史和创建 provider 后读取 input.RunID；只有输入为空时才调用 runtime.newRunID，并把结果回写 input.RunID：internal/runtime/runtime.go:272-309。源码中 Engine.Run 是独立 runtime 空输入的唯一生成点；runAgent 不再生成 ID，而是直接捕获 input.RunID，并将其写入用户、assistant、tool call、permission、tool result、warning、error、usage 和 run_end journal payload：internal/runtime/run.go:16-34、59-92、106-212。

childRunner 每次 child invocation 只调用一次 newRunID，放入 child Input.RunID；同一个 childRunID 被用于 child progress、returned events 和 Engine.Run 产生的 child journal 入口：internal/runtime/subagent.go:17-68、69-118。嵌套 child 也沿用同一边界，因为 child 的 Input 会成为下一层 parent。

### Service/journal 集成测试证据

TestServiceRunIdentityStaysStableAcrossControllerRuntimeAndSuccessiveRuns 使用真实 application.New、provider 保存、project/session 创建、Service.Send、权限回复和同一会话的第二次连续运行；HTTP server 只替代 Chat Completions provider，不替代 Controller 或 Runtime：internal/application/run_identity_integration_test.go:20-122、140-161。第一轮收到 permission 后，测试调用 allow，等待 session 进入 `Idle` 且 `ActiveRun == nil`，再发送第二轮；因此这里的 successive runs 是首轮完成后的连续两轮运行，不覆盖运行中 correction、cancellation replacement 或运行中队列替换。

第一次运行通过真实 exec_command permission 暂停。测试检查 pending snapshot 的 session.RunID、permission.RunID 和 first user message RunID 相等：internal/application/run_identity_integration_test.go:125-137。调用使用的 `appmodel.Correction` 是发送模式名称，不改变本测试对“运行中 correction”的覆盖边界。第一轮 allow 后，测试等待首轮文本出现、session 回到 `Idle` 且 `ActiveRun == nil`，然后才发送第二轮；第二轮必须产生非空且不同的 newRunID，并在最终 snapshot 中保持 first/second user message 各自归属：internal/application/run_identity_integration_test.go:148-184。

测试随后读取真实 sessions/sessionID/events.jsonl，拒绝空或未知 RunID，要求旧/新 run 至少分别拥有 user、assistant、run_end 和 warning 事件；tool call、tool result、permission request、permission decision 和 permission 只允许归属于第一次运行：internal/application/run_identity_integration_test.go:186-258。invalid .gcode/hooks.json 产生的 warning 被纳入同一 run identity 断言，覆盖 runtime 在 provider 之前写 journal 的路径。

这是真实 Service/controller/runtime/journal 证据，能复现并拦截 controller ID 与 runtime journal ID 分叉；但当前 journal 断言按每个 RunID 的事件类别设置布尔标记，没有逐条验证事件 payload 的具体文本、工具 ID、Event.SessionID 和顺序。因此它是对本次缺陷充分的回归网，但不是完整的逐事件内容归属证明。

### Child progress 与 journal 测试证据

TestRuntimeChildMonitorUsesOneIndependentRunID 直接通过生产 Engine.childRunner 进入 Engine.Run，使用真实 session store、child session、specialist prompt 文件和 scripted provider；它没有启动外部 child 进程：internal/runtime/run_identity_test.go:16-62。

测试要求 childRunID 非空且不同于 parent RunID，并逐项检查 progressEvents 与 returned result.Events 的每一个 RunID；随后读取 child session 的 ReadRehydratedEvents，逐项检查 durable event payload 的 runId：internal/runtime/run_identity_test.go:70-103。普通和 race 日志都记录 3 个 progress events、3 个 journal events 使用同一个 child RunID。

该测试直接覆盖了 child wrapper 每次 emit 重新生成 ID 的回归风险，并能检测 child wrapper ID 与 Engine.Run journal ID 不一致。它没有覆盖 child error/cancellation 分支的 ID 断言，但这些分支共用同一 emit/Engine.Run ID 变量。

### 独立 runtime 调用的边界

源码审查确认独立 Engine.Run 的空 RunID 只在 internal/runtime/runtime.go:294-298 生成一次，之后 runAgent 使用回写后的 input.RunID；现有新增 child 测试则显式传入 child RunID，以便同时验证 wrapper 与 Engine.Run 不重复生成。

现有新增测试没有单独调用 Engine.Run 时保持 RunID 为空，并读取首个 journal/callback RunID 再验证所有事件一致。因此“空输入独立 runtime 调用只生成一次”目前由源码路径和既有 runtime 普通包记录间接支持，仍缺少专门的回归断言，属于 P2 测试证据限制，不是已复现的生产错误。

### 现成验证记录

root 提供的 run identity 记录均通过：

- output/acceptance/run-identity/successive-runs-regular.log：Service/journal 连续两轮运行集成测试普通模式 PASS，日志给出不同的 old/new RunID。
- output/acceptance/run-identity/successive-runs-race.log：同一 Service/journal 连续两轮运行集成测试 race 模式 PASS，日志给出不同的 old/new RunID。
- output/acceptance/run-identity/runtime-child.log：child progress/journal 普通模式 PASS，日志记录 progressEvents=3、journalEvents=3。
- output/acceptance/run-identity/race-runtime-child.log：child progress/journal race 模式 PASS。
- output/acceptance/run-identity/runtime-controller.log：runtime 和 controller 包记录 exit 0。
- output/acceptance/run-identity/vet.log：当前文件为空，本轮不把它作为可读的 run-identity vet 通过证据；全仓 vet 以本记录第六节列出的 go-vet-result.json 为准。

本代理没有重新执行这些命令；结论基于记录内容和当前源码。

## 四、取消 warning 与 pending presentation

### Controller 取消 warning

`onRuntimeWarning` 先要求 runtime error 非空并通过 active run 归属检查；`expectedCancellationWarning` 只有在 run 存在、runtime error 满足 `errors.Is(runtimeErr, context.Canceled)`、run context 已满足 `errors.Is(run.ctx.Err(), context.Canceled)`，且 cancel reason 为主动 `cancelReplace` 或 `cancelStop` 时返回 true：internal/controller/controller.go:1035-1073。命中后只跳过 warning 写入。普通 runtime error 和没有主动取消原因的错误继续写入 session Error 与 warning message。

`finish` 对 replacement 还要求运行确实观察到取消，并且 `runErr` 为空或可用 `errors.Is(runErr, context.Canceled)` 证明取消；generic error 会进入 Failed、`pauseReasonCancelFailure` 和可见 error 路径。Stop 则进入 Interrupted 并保留无错误状态：internal/controller/controller.go:1152-1205。

`TestExpectedCancellationDoesNotCreateWarning` 用 fakeRunner 覆盖 correction replacement 和 stop 两条主动取消路径，fake runner 在 context canceled 后回调 `OnError(context.Canceled)`；测试要求最终状态分别为 Idle/Interrupted、没有 session error，也没有 warning 或 `context canceled` 文本：internal/controller/cancel_warning_test.go:14-76。`TestRuntimeErrorRemainsVisibleAfterNormalRun` 检查普通 provider setup error 仍进入 Failed、session Error 和 warning；`TestCancellationFailureRemainsVisible` 在 replacement 已请求取消但 runner 返回 generic error 时检查 Failed、`pauseReasonCancelFailure`、错误文本和 warning 都保留：internal/controller/cancel_warning_test.go:78-141。

普通和 race 记录均通过：

- output/acceptance/controller-cancel-warning/regular.log：三个行为测试及 correction/stop 子场景 PASS。
- output/acceptance/controller-cancel-warning/race.log：同一组行为在 race 模式 PASS。

这些测试使用 fakeRunner，证明 Controller 的取消归类和 warning 状态边界，不证明真实 provider、网络或外部进程在取消时的行为。

### Pending presentation

permission card 在 `permissionArgumentsText` 中先解码 JSON，完整展示 `cmd`/`command`/`script` 字符串，并在存在 `workdir`/`cwd` 时追加工作目录；解码失败或没有命令字段时保留原始参数文本。UI 将该内容放入 `MaxHeight(180)` 的滚动区域，原始 `permission.Arguments` 不被改写：internal/desktop/conversation.go:339-381。

`TestPermissionCardShowsCompleteDecodedCommand` 使用包含 JSON escape、中文和 120 行命令的参数，检查完整解码文本与工作目录出现、协议字段不泄漏、授权/拒绝按钮仍在 700 高视口内，并直接确认 App 内保存的 Arguments 与 raw JSON 完全相同：internal/desktop/pending_presentation_test.go:11-39。

`renderTool` 只在父 message 的 `Interrupted` 为 true、原始状态为 error/failed，且 Output 或 Error 去除尾部空白后以 `context canceled` 结尾时将显示状态改为 interrupted；原始 Arguments、Output 和 Error 仍在可展开 panel 中保留。此状态不触发 error 的 forced-open，因此新捕获的 expected cancellation 默认折叠并显示“已中断”；普通错误、Interrupted 但非取消错误以及未 Interrupted 的取消文本继续按失败状态处理：internal/desktop/conversation.go:187-257、289-301。

`TestInterruptedToolPresentationKeepsFailureAndDiagnosticDistinct` 覆盖 expected cancellation、ordinary failure、failure before interruption 和 unexpected cancellation 四种状态；expected cancellation 初始显示“已中断”且折叠，点击工具后仍能读取原始诊断，真实失败继续显示失败并强制展开：internal/desktop/pending_presentation_test.go:41-80。root 提供的 focused 普通结果通过；本轮全仓普通与 desktop/controller race 也通过。最新 macOS 原生记录进一步在 permission-correction 和 ask-correction 的 AX 中确认中性“已中断”和折叠诊断，但长 permission 滚动仍只由 Tester 覆盖。

## 五、render snapshot 所有权与缓存

### 生产事实

Open 在接管 Initial 时调用 cloneSnapshot：internal/desktop/app.go:66-103。Update 在锁内先用 cloneSnapshot 复制输入，再替换 a.data：internal/desktop/app.go:171-227。cloneSnapshot 深拷贝 projects、settings/providers、session slice、ActiveRun、messages/tools、queue、permissions/options 和 questions/options：internal/desktop/app.go:957-987。

view 在持有 a.mu.RLock 时取得 snapshotForView 和其他 view 状态，然后解锁并构建 UI：internal/desktop/app.go:290-310。snapshotForView 复制 settings/providers 和 session value slice，但继续共享已经由 App 拥有的 messages/tools、queue、permissions、questions、projects 以及 ActiveRun 指针：internal/desktop/app.go:989-1003。Open/Update 已经在进入 App 时复制 caller 数据，所以该边界依赖渲染端只读。

当前 UI 对历史 messages/tools、queue、permissions、questions 和 projects 只读遍历；permission/question 卡片的按钮只调用 response service：internal/desktop/conversation.go:20-72、328-369、371-395。生产 UI 的本地写入点是 composer 对 session.FullAccess 的写入：internal/desktop/conversation.go:451-462；settings draft 对 data.Settings.Theme/DefaultSendMode 的写入：internal/desktop/app.go:839-845；settings provider 编辑器对 data.Settings.Providers 的写入：internal/desktop/settings_page.go:230-259。view 已复制这些可能被本地改写的外层数据，当前搜索未发现 UI 对共享历史或 ActiveRun 字段原地写入。

生产入口将 Window.Update 设置为 dispatcher，service 订阅也在该 dispatcher 中取 Snapshot 后调用 view.Update：cmd/gcode/main.go:94-108。

### 最新测试证据

renderSnapshotOwnershipFixture 提供 projects、settings/providers、queue、permission、question 和历史工具数据：internal/desktop/render_snapshot_test.go:14-58。TestRenderSnapshotCallerOwnershipSurvivesOpenAndUpdate 在 Open 和 Update 后分别修改 caller 输入，确认 App 拥有的数据不被反向修改：internal/desktop/render_snapshot_test.go:61-92。

TestRenderSnapshotOldViewStaysStableAcrossGatedUpdate 增加了非空 ActiveRun，并保存 oldView 与期望值；Update 替换历史正文、工具输出、最新消息、权限、问题和 ActiveRun 后，测试确认 oldView 结构不变，再把 oldView 送入真实 conversation renderer，检查旧最新正文可见且新正文和新增历史不可见：internal/desktop/render_snapshot_test.go:94-158。该测试直接补上上一轮缺少的旧 view 用户可见文本证据。

TestRenderSnapshotStreamingUpdatesPreserveHistory 在多轮流式替换后检查历史长度、所有旧消息内容和每个新片段的可见文本：internal/desktop/render_snapshot_test.go:160-189。TestRenderSnapshotCapturedFrameAndLocalMetadataAreIsolated 通过 ui.Tester 点击“完全访问”、进入“设置”、选择“深色”和“队列”，再检查本地 metadata 生效、caller snapshot 和历史消息不变：internal/desktop/render_snapshot_test.go:191-239。它已不再只靠直接写 app.data 来证明交互路径。

TestRenderSnapshotConcurrentUpdatesAndFrames 以 channel round 驱动 128 次 Update/frame 交替压力，并在最后检查 snapshot 形状和历史消息：internal/desktop/render_snapshot_test.go:241-285。这个测试能够让 goroutine 调度与 Frame 互相施压，但 channel 没有强制二者在同一时刻重叠。

### 未关闭的证据限制

TestConversationPerformanceViewportAndMarkdownReuse 仍以 before/after 的 cached Markdown block 指针相等作为未变化消息没有重新解析的核心判据：internal/desktop/conversation_performance_test.go:164-213。测试已有初始最新消息、替换后新消息和旧消息消失的用户可见断言，但没有对 unchangedIndex 的可见文本做独立断言，也没有 parser invocation counter 或内容等价断言。因此这里保留 P2 测试证据限制，不能把指针复用直接写成完整用户行为证明。

old-view gate 和 concurrent test 都是同步/调度证据，不是 CPU 物理并发证据。ActiveRun fixture 已加入 old-view 测试，但 snapshotForView 仍共享该指针；当前源码搜索只支持“现有 UI 没有原地写入”的判断。

## 六、root 已提供的整体验证与原生边界

以下结果由 root 提供并在本轮读取：

- output/acceptance/conversation-performance/render-snapshot-review.log 和 render-snapshot-review-race.log：render ownership、old view、streaming、Tester metadata、concurrent frame 以及 Markdown 测试均 PASS；这些记录主要证明既有 snapshot/performance 范围。
- output/acceptance/current-package-regression/pending-presentation-final/go-test-result.json、go-vet-result.json、fmt-check-result.json、affected-race-result.json、package-result.json、codesign-result.json：本轮全仓 test、vet、fmt-check、desktop/controller race、package 和 codesign verify 均 `exit_code: 0`。`go-test.log` 也列出 desktop 与 controller 包通过。
- output/acceptance/current-package-regression/pending-presentation-final/package-manifest.json：source/test fingerprint 在构建后保持不变，最新 executable SHA-256 为 `d0102a642e6c73f6a41b76070dce74e1a6de0fcfaaef9d1738a9a96fe805666a`；manifest 明确包含 cancellation warning、interrupted tool display 和 permission card 三项生产改动。
- output/acceptance/native-pending-ui/presentation-confirmed/review.json：状态为 `PASS_FOR_RECORDED_SCOPE`，使用与最新 package manifest 相同的 executable SHA-256 `d0102a642e6c73f6a41b76070dce74e1a6de0fcfaaef9d1738a9a96fe805666a`。四个场景均完成实际记录：permission-allow-deny 的 `permission-allowed.txt` 实际写入、`permission-denied.txt` 未写入和 AX 中无协议字段；ask-history 的下一次 provider 回答与回答历史；permission-correction 与 ask-correction 的旧运行中断、替换 RunID（`run_7b0458b2d1e64cf8 → run_e423cf75df62c59e`、`run_0d09d391062e7671 → run_63709c8c59099d1e`）、事件顺序、AX 中性已中断/折叠诊断和正常退出清理。
- 同一 review 明确保留以下限制：`late_reply_injection`/`late_old_answer` 为 `NOT_INJECTED`，表示该注入场景没有执行，不能当作 late reply 行为已验证；`explicit_runner_end_acknowledgment` 为 `NOT_EXPLICIT_IN_JOURNAL`，表示 journal 没有显式确认。长 permission scrolling 只由 Tester 覆盖，真实 vendor API、物理键盘/IME、clipboard、Apple Silicon/Windows 和用户视觉认可均未验证。此前的 output/acceptance/native-pending-ui/run-identity-final/review.json 与截图仍属于旧包身份。

## 限定结论

本轮证据支持：

- S2 本地协议替身确实经过真实 Service、Controller、runtime.Engine、agent.Run 和内置 read/edit/write 工具；先读后竞争、同文件冲突、外部编辑、不同文件写入、历史事件和最终 provider 请求体均按 session 绑定。
- render snapshot 当前的 caller ownership、历史只读共享、旧 view 稳定性和真实 Tester metadata 交互有源码与测试支持；已有普通/race/vet 记录没有报告共享 slice 或 dispatcher race。
- RunID 当前生产路径已实现单次生成和跨层传递；Service journal 已证明 controller、runtime、同一会话连续两轮运行和 child progress/journal 在本次回归范围内使用稳定归属。
- Controller 主动 stop/replace 的 expected cancellation warning 已由源码、普通和 race fakeRunner 测试证明；普通 runtime error 与取消失败仍可见。
- pending presentation 的 command/workdir 解码、180 高度滚动、按钮视口保留、expected cancellation 折叠和真实错误展开已由源码、ui.Tester 测试及最新全仓/affected-race/package 记录支持。
- 最新 macOS Intel formal-package 原生记录已支持四个声明场景的实际文件、回答历史、替换 RunID、解码授权呈现、AX 中性已中断/折叠诊断以及正常退出清理；该结论绑定 `presentation-confirmed/review.json` 的 package hash。

本轮证据不支持：

- Markdown 指针复用已经等价于所有用户可见行为或真实解析次数；
- application journal 的事件类别布尔断言已经等价于逐条 payload、session、工具 ID 和事件顺序归属；
- 没有专门的空 RunID Engine.Run regression test 也能把独立 runtime 生成路径写成已由测试完整证明；
- channel 测试已经证明 CPU 层面的物理并发；
- 不能把 `presentation-confirmed` 的四场景原生记录扩展为覆盖所有用户要求的行为范围；
- 不能把原生记录写成证明长 permission 内容可在 macOS 窗口内滚动、真实 vendor API、late old reply injection 已被验证、runner end acknowledgment 已写入 journal 或用户已完成视觉认可；
- 真实 provider API、设备/GPU、输入法、物理滚轮、剪贴板或跨平台行为。
