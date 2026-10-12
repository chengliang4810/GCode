# /goal：正式包持续流式与同项目多会话原生验收

状态：round-18-integrated-final-completion 和 round-19-integrated-final-confirmation 已在同一确认正式包、同一 driver 和同一三会话负载上连续达到严格 completion 规则的 `PASS_FOR_RECORDED_SCOPE`（2026-10-12）；round-15-selection-fixed 和 round-16-ax-role-tree 仍为 FAIL，round-17-parsed-prefix 仍是旧 tail-token 规则下的限定通过。两轮重复通过只确认当前记录范围的可复现性，不构成整体性能或所有原生交互通过。未统计 FPS/frame time，未证明滚动效果、阅读位置恢复或视觉交付，整体性能结论仍未通过。

目标：在当前 macOS Intel 正式 `GCode.app` 的隔离副本中，通过真实原生设置页保存一个仅监听 `127.0.0.1` 的 Chat Completions SSE fixture，在同一个临时项目中创建三个会话并近同时发送唯一标记的任务。驱动必须经过真实 `Service → Controller → Runtime → Provider` 路径，持续观察三条流式请求、controller 快照、原生 AX 读回和 session journal 的归属。

完成判据：

- 正式包副本只改变 `Info.plist` bundle identity 并重新 ad-hoc 签名；原包的可执行文件代码段、Resources 和生产源码规范化清单 hash 在运行前后相同。
- 三个会话属于同一个项目和工作目录；三条请求在 loopback fixture 中同时保持打开，默认持续 30 秒、每秒 10 个 SSE 内容 chunk。每个请求有独立 marker、run ID 和脱敏的开始/首 chunk/末 chunk/完成记录；最后一个既有 content chunk 额外附带该 session 唯一的 final completion token，不增加 chunk 数、时长或速率。
- 流式期间 controller snapshot 观察到三个独立 active run，消息不出现其他会话 marker 或 tail token；四次 AX 读回（三个会话各一次，另一次为切回第一个会话的 `before-scroll` 样本）均读到当前会话专属 tail token，且没有 foreign marker/tail token。允许首个 prompt marker 因列表虚拟化暂时不可见；完整 prompt marker 仍由 controller snapshot 和 journal 严格归属。AX 读回按同步顺序执行，round-4 首次读回耗时 `27686.087632 ms`，因此不能声称四次读回都发生在三条流同时保持 `running` 期间。合成滚轮只证明 `cghidEventTap` helper 已注入事件并恢复指针位置；没有滚动效果、viewport 对比或阅读位置恢复断言。
- 流结束后三个会话均为 `idle` 且 `active_run == nil`，各自拥有完整中文、emoji、Markdown 列表/段落/fenced code/table 响应；每个 `events.jsonl` 可解析，run ID、user/assistant/run-end 角色和 session 归属一致。此后必须用真实 last-row AX 逐 session 读回 final completion token，记录 own/foreign completion 与 marker 归属；旧 tail token 可见不能替代该检查。失败时仍应保留失败窗口诊断截图；截图失败必须单独记为 diagnostic error，不能覆盖 primary error。
- 应用经菜单正常退出并返回 exit code 0，QA PID 消失，fixture server/thread 停止；隔离凭据目录删除，controller、journal、AX、chunk、性能采样、持续流式/最终会话窗口截图及其 SHA-256、manifest 证据保留。

驱动是 `docs/verification/scripts/native-streaming-qa.py`。`--self-test` 只测试 SSE 构造、marker 映射、final completion token 唯一性、旧 tail 可见但 completion 缺失时失败、剩余 query timeout 传递、脱敏、snapshot/journal 归属和三条并发记录，不打开窗口、不绑定 socket。默认参数是 `--duration 30 --hz 10 --sessions 3`，参数是可复现输入，不是性能通过门槛。

范围边界：loopback fixture 不是厂商 API，也不代表真实网络、模型质量、厂商限流或真实 API 性能；AX 往返包含点击、`osascript` IPC、原生视图等待和 AX 读取，round-4 的首个 AX 读回为 `27686.087632 ms`，后续三次为 `1663.861772`、`760.924816`、`876.567313 ms`，这些数值只能作为原始样本，不能作为性能通过结论。round-4 只有两次 controller poll 和一次 process sample（`66572 KiB`、`17.8%`），同步 AX 读回占住了测量循环，不能据此声称持续监控、稳定延迟或资源性能达标。截图通过真实 QA PID 对应的 `CGWindow` 和 `screencapture -l` 保存，驱动只确认窗口归属，不把 AX 文本、截图或失败截图自动判为视觉通过。失败后、正常清理前会尝试保存 `captures/failure-window.png`，记录 PID、CGWindow 和 SHA-256；若应用/helper 已退出或 capture 异常，只写独立 diagnostic error，不改写 primary error。脚本不触碰系统剪贴板，不测试物理键盘、中文 IME、物理滚轮/触控板惯性、GPU/frame time、Apple Silicon、Windows 或用户视觉认可。中文和 emoji 的 AX 读回不等于中文输入法通过。

证据目录使用新的、不覆盖旧记录的 `output/acceptance/native-streaming/<round>/`。默认命令会拒绝已有非空目录；root 应在包身份核对后选择新目录执行，不能用历史长会话包的 SHA 替代本次流式包。

round-4 证据：[`results.json`](../../output/acceptance/native-streaming/round-4/results.json)、[`manifest.json`](../../output/acceptance/native-streaming/round-4/manifest.json)、[`review.json`](../../output/acceptance/native-streaming/round-4/review.json)、[`measurement-samples.json`](../../output/acceptance/native-streaming/round-4/measurement-samples.json)、[`controller-final.json`](../../output/acceptance/native-streaming/round-4/controller-final.json)、[`provider-records.json`](../../output/acceptance/native-streaming/round-4/provider-records.json)、[`stream-chunks.json`](../../output/acceptance/native-streaming/round-4/stream-chunks.json)、[`streaming-window.png`](../../output/acceptance/native-streaming/round-4/captures/streaming-window.png)、[`final-window.png`](../../output/acceptance/native-streaming/round-4/captures/final-window.png)。review 状态为 `PASS_FOR_RECORDED_SCOPE`，`performance_scope` 为 `NOT_ACCEPTED`；截图只供人工复核，manifest 明确标记为 `visual_approval: NOT CLAIMED`。

round-5 诊断子目标（观测，不宣布性能达标）：保留当前轻量 `last row` AX 查询、marker/tail/foreign 断言；点击单独受 12 秒预算限制，点击完成后 tail-query 使用 18 秒严格外层预算，每次 last-row AX 查询再按剩余时间限幅。分别记录会话点击耗时、每次 AX attempt 的起止时间/耗时/实际 `effective_timeout_ms`/成功状态/脱敏错误类型/tail 可见性/文本字节数，以及每次 attempt 前后的目标 controller 状态和 fixture 请求完成/活动计数。每个成功 attempt 只记录已知 fixture token 的 `own_marker_visible`、`foreign_markers`、`foreign_tail_tokens`，不再写入原始 AX UI 文本；`text_bytes` 和 tail 可见性仍保留。剩余预算耗尽后不再发起查询。三条任务发送前启动独立 controller 约 200 ms、进程约 1 s 的后台 collector，AX 同步调用不得暂停它；collector 必须在退出时 stop/join，错误写入专属 `diagnostic-samples.json`，不写入产品日志，也不把读取错误解释为生产故障。`ready.json` 只提供 QA PID、identity、隔离 data directory 和阶段状态，首个 AX 点击前写入 `ax_readback`，清理后同步进程退出状态。严格 deadline 自测使用可控时钟，不等待十几秒；这项诊断只用于判断首个 AX 等待是否跨过流结束，不构成持续性能或响应阈值结论。

round-5 至 round-8 结果：round-5 的三条 SSE 均完成 300 个 chunk，backend final snapshot 和 journal 归属证据通过，但第二个 session 的 AX 点击耗时 `12005.453 ms`，tail query 未进入；应用正常退出失败，最终被强制退出，exit code `-9`。round-6 使用三会话且未启用 profiler，三条 SSE、最终 snapshot、journal 和正常退出均通过；第三个 session 的 AX tail-query 在点击 `510.268 ms` 后耗时 `18518.624 ms`，超过点击后的 18 秒严格预算，因此本轮仍为 FAIL。round-7-single 只运行一个 session，SSE 为 300 个 chunk、峰值并发为 1；两次 AX 读回聚合耗时分别为 `614.418 ms` 和 `684.725 ms`，正常退出为 0，结果为 `PASS_FOR_RECORDED_SCOPE`。round-8-two 只运行两个 session，两条 SSE 各 300 个 chunk、峰值并发为 2，最终 snapshot、journal、正常退出和清理均通过；三次 AX 读回耗时为 `2448.633 ms`、`3677.418 ms`、`821.170 ms`，结果为 `PASS_FOR_RECORDED_SCOPE`。single/two 的通过结果不能替代三会话验收，也不能宣布整体性能通过。

round-13/round-14 结果（H1 最小排版修复后）：round-13 的 session 1 AX 读回通过，session 2 的 tail-query 耗尽 18 秒外层预算；round-14 的 session 1、session 2 单次 query 均通过，session 3 的 tail-query 耗尽 18 秒外层预算。通过的单次 AX query 约为 `0.3–0.7 s`，未再出现 round-6 那样的 query timeout；这说明排版热点下降后，仍存在内容/可视状态导致的 tail 不可见问题。两轮 backend 最终状态均为 `interrupted`，journal 缺 assistant/run_end，正常退出码均为 `0`，collector 正常，且均保留 streaming 截图；因此两轮都不能宣称三会话通过。

round-5 至 round-8 证据：[`round-5 review`](../../output/acceptance/native-streaming/round-5/review.json)、[`round-5 results`](../../output/acceptance/native-streaming/round-5/results.json)、[`round-6 review`](../../output/acceptance/native-streaming/round-6/review.json)、[`round-6 results`](../../output/acceptance/native-streaming/round-6/results.json)、[`round-7-single review`](../../output/acceptance/native-streaming/round-7-single/review.json)、[`round-7-single results`](../../output/acceptance/native-streaming/round-7-single/results.json)、[`round-8-two review`](../../output/acceptance/native-streaming/round-8-two/review.json)、[`round-8-two results`](../../output/acceptance/native-streaming/round-8-two/results.json)。各轮的 `manifest.json`、`measurement-samples.json`、`diagnostic-samples.json` 和截图均保留在对应目录；截图仍标记为 `visual_approval: NOT_CLAIMED`。

round-13/round-14 证据目录：[`round-13-layout-fixed`](../../output/acceptance/native-streaming/round-13-layout-fixed/)、[`round-14-fixed-frame-stats`](../../output/acceptance/native-streaming/round-14-fixed-frame-stats/)。这些轮次的 streaming 截图、脱敏 measurement/diagnostic evidence 和失败时的 controller/journal 证据必须与 `primary_error` 一起阅读；新驱动的 AX attempt 不再生成原始 AX 文本文件。

round-15/round-16 结果：round-15-selection-fixed 已观察到三个 session 同时 running，session 1 的 AX 读回通过，但 session 2 的 tail-query 在 18 秒严格预算内始终不可见，累计 29 次 attempt 后失败；三条 fixture 只完成 263、251、239 个 chunk，随后因清理断开，最终 session 均为 interrupted。round-16-ax-role-tree 的 session 1 tail-query 同样耗尽 18 秒预算，最后一次 attempt 为 `osascript` timeout；session 1/2 的 fixture 各完成 300 个 chunk，session 3 完成 292 个后断开，最终 session 3 为 interrupted。两轮都保存了 failure-window 截图，正常退出码为 0、collector 和清理通过；round-16 的 AX role-tree 仅为失败后的 `DIAGNOSTIC_ONLY` 证据，不能单独证明生产根因。

round-17-parsed-prefix 结果：root review 离线核对 832 个 production 文件、正式包与隔离副本代码边界；源可执行文件 SHA-256 为 `b02f442a7c05ec7c6a8db313b691a0a44d914513525c314827c041fe9fa23b4b`，`__TEXT,__text` SHA-256 为 `45f943c874bbabc248878cf99477f2213b310684aeab59621a8cf5dc12e2c408`。三条 SSE 各完成 300 个 chunk，独立 run/journal 终态为 idle，正常退出码为 0，QA PID、fixture 和凭据清理通过。四次旧 running AX 均一次 attempt：`session-1-streaming`、`session-2-streaming`、`session-3-streaming`、`session-1-before-scroll` 的 click+query 聚合耗时分别为 `739.832 ms`、`772.926 ms`、`675.421 ms`、`819.115 ms`，query 本身分别为 `399.2 ms`、`360.5 ms`、`297.7 ms`、`413.0 ms`；当前 marker/tail 均可见且无 foreign token。该结果为旧 tail-only 规则下的 `PASS_FOR_RECORDED_SCOPE`，没有 final completion record，不能作为新严格验收通过，也不能扩展为 FPS/frame-time、滚动效果、阅读位置恢复或视觉验收通过。

round-18-integrated-final-completion 结果：当前确认正式包的 executable SHA-256 为 `db479d18dbde05705cf7bfced02c8ab6164f650d1f4bf3e28e0d481a76eeb216`，`__TEXT,__text` 代码段 SHA-256 为 `40d66e826fe9117eaaee6be0cc4757831221d84c0463af621412348616ca8b8e`，832 个 production 文件的 source fingerprint 为 `32e08f88522775e0cfb2568393ea93371b773d31222191f4e8533b2792f13a67`，driver SHA-256 为 `3a2377dd9b5881841c3f88fea26a916350e9f785e406510aadba02db02c1419b`。正式包隔离副本的代码和 Resources 校验通过。三条 SSE 各持续 30 秒、10 Hz、300 个 content chunk，共 900 个 chunk，峰值并发为 3，三条流均 complete 且未断开；三会话最终均为 idle、`active_run == nil`，run ID 和 journal 归属通过。

本轮四次 running AX 读回和三次 final completion AX 读回均为一次 attempt。四次 running 读回的 click+query 聚合耗时为 `690.649–874.547 ms`，三次 final completion 聚合耗时为 `905.878–934.383 ms`；最终读取均通过真实 `last row of table 1` static text 读回各自唯一 completion token，own marker 可见，foreign marker/completion token 均为空。最终 snapshot、三份 journal、正常退出码 `0`、QA PID 消失、fixture listener 关闭、隔离凭据删除、collector stop/join 和 evidence secret scan 均通过。该轮 review 为 `PASS_FOR_RECORDED_SCOPE`，证据见 [`round-18-integrated-final-completion`](../../output/acceptance/native-streaming/round-18-integrated-final-completion/)。streaming/final 截图已保存且由 root 打开，但 `visual_approval` 仍为 `NOT_CLAIMED`；real API 为 `NOT_TESTED`，FPS/frame time 未测，合成滚轮只保留事件注入和指针恢复证据，不能宣称实际滚动效果或阅读位置恢复。

round-19-integrated-final-confirmation 结果：使用与 round-18 相同的正式包、driver 和三会话 30 秒/10 Hz 负载复跑，review 为 `PASS_FOR_RECORDED_SCOPE`。三条 SSE 再次各完成 300 个 chunk，共 900 个 chunk，峰值并发为 3，三流均 complete 且未断开；最终三会话均为 idle、`active_run == nil`，三份 journal 的 run/session 归属通过。四次 running AX 读回各一次 attempt，click+query 聚合耗时为 `649.507 ms`、`649.917 ms`、`586.090 ms`、`722.974 ms`；三次 final completion AX 读回各一次 attempt，聚合耗时为 `917.057 ms`、`918.033 ms`、`880.689 ms`。三个 final record 均 PASS，均从真实 `last row of table 1` static text 读回本 session 唯一 completion token，own marker/token 可见且 foreign marker/completion token 为空，`record_count` 与 `expected_record_count` 均为 3。正常退出码 `0`、QA PID 消失、fixture listener 关闭、隔离凭据删除、collector stop/join、源码/原包 unchanged 和 secret scan 均通过。证据见 [`round-19-integrated-final-confirmation`](../../output/acceptance/native-streaming/round-19-integrated-final-confirmation/)。streaming 截图 SHA-256 为 `762a5ae41a5110b9e1c11b07512cc307fcde3a3b6cc83dd1dd2e6b0f60ec002f`，final 截图 SHA-256 为 `3226ace094b670688daf2f5e09d1f966ddf7e9186c8284b901bd9d6aaff5d639`；截图已由 root 打开但 `visual_approval` 仍为 `NOT_CLAIMED`，real API 为 `NOT_TESTED`，FPS/frame time 未测，实际滚动效果和阅读位置恢复未断言。
round-18 和 round-19 的 manifest 使用旧 driver SHA-256 `3a2377dd9b5881841c3f88fea26a916350e9f785e406510aadba02db02c1419b`，两轮均由 root 以 `3/3` final completion 结果独立核验。随后当前 driver 只增加成功路径的显式 final completion 汇总门槛，并增加“不完整/FAIL 汇总必须阻止成功”的 self-test；当前 driver SHA-256 为 `88a08fdcfb35cec5111b0d16938c117891c6ad0982e095803cfda48226ec2624`，`py_compile` 与 `--self-test` 均通过。该修正未用于 round-18/round-19 原生重跑，不改变两轮历史 artifact 的 driver 身份。

严格 completion 验收规则：fixture 在最后一个既有 SSE content chunk 中附加 `stream_completion_token(marker)`，不减少正文、不增加 chunk、不改变时长或速率。三流 backend 均 complete 且三个 session 均 idle 后，驱动对每个 session 再执行真实 `last row` AX 读取，仍使用点击单独 12 秒上限、点击后 18 秒外层预算和按剩余预算限幅的 query timeout。`measurement-samples.json` 以独立 `kind: final_completion` 记录 token、attempt、`contains_completion_token`、`foreign_completion_token_check` 和 marker 归属；不保存 AX 原始正文，不用 controller snapshot 或 row label 代替正文。旧 tail 可见但 final completion 缺失必须失败。

round-15/round-16/round-17/round-18/round-19 证据目录：[`round-15-selection-fixed`](../../output/acceptance/native-streaming/round-15-selection-fixed/)、[`round-16-ax-role-tree`](../../output/acceptance/native-streaming/round-16-ax-role-tree/)、[`round-17-parsed-prefix`](../../output/acceptance/native-streaming/round-17-parsed-prefix/)、[`round-18-integrated-final-completion`](../../output/acceptance/native-streaming/round-18-integrated-final-completion/)、[`round-19-integrated-final-confirmation`](../../output/acceptance/native-streaming/round-19-integrated-final-confirmation/)。round-17、round-18 和 round-19 的截图仍只供人工复核，`visual_approval` 不构成视觉交付结论。

## H1/H3 假设调查（源码静态核查，2026-10-12）

### H1：流式文本是否被整段重复解析和排版

结论：源码确认存在“累计文本整段 Markdown 解析、可见消息整段重建、变化段整段文字 layout”的路径；没有证据表明每个 SSE chunk 都一定同步完成一次 native layout，因为正式路径会把 Markdown 解析放到异步 goroutine，并可能丢弃中间结果。性能热点应先按“每个被接受的累计 source 版本”计数，不能简单按 chunk 数量推断 CoreText 调用数。

事实链如下：

- `internal/controller/controller.go` 的 `onText` 对同一个 assistant message 执行 `message.Text += text`，随后持久化、生成完整 snapshot 并 `emit`。`cmd/gcode/main.go` 的订阅回调收到每次 emit 后再次调用 `service.Snapshot` 和 `view.Update`。因此每个流式片段都会经过一次 controller snapshot 和一次桌面 snapshot 更新。
- `internal/desktop/app.go` 的 `updateSnapshot` 每次调用 `cloneSnapshot`，会复制所有 session、所有历史 message 及其 tool slice；这部分是每个片段的全历史复制成本，与 Markdown 是否变化无关。
- `internal/desktop/conversation.go` 只为当前可见的 `ui.List` 行调用 `renderMessage`。`internal/desktop/markdown.go` 的 `renderTextBlocks` 以 `sessionID + rowKey` 做缓存键，`markdownDocumentForRender` 只按完整 `source` 字符串命中。source 变化后会替换缓存项并安排新的 `parseMarkdownAsync`。
- `parseMarkdownDocument` 每次都对完整 `[]byte(source)` 调用 Goldmark parser，再由 `convertMarkdownBlocks` 递归转换所有 block、list、table 和 inline；没有保留 AST、block 前缀或增量 parser。异步解析完成时若 source 已更新，旧结果会丢弃并继续解析最新 source，所以中间结果可能被合并，但每个实际发布的版本仍是全量解析。
- 解析未完成时，`renderTextBlocks` 直接用一个 `ui.RichText` 承载完整累计 source；解析完成后，`renderMarkdownBlocks` 重新构建该消息的所有 block 和 inline paragraph。未变化的消息可以命中 `markdownCache`，现有 `TestConversationPerformanceViewportAndMarkdownReuse` 也验证了未变化可见 block 未被重新解析；这不改变正在增长消息的全量路径。
- MyGo 的 `third_party/mygo/ui/layout.go` 中，文本节点的 `leafWidths` 会用完整 `e.text` 调用 `text.System.Layout`，通常分别测量 width=0 和 width=1；`contentHeight`/`layoutBox` 随后按实际可用宽度再次调用 `Layout`。这些调用在单个 node/frame 内有 `leafOK`、`measures`，但 node 每次 build pass 都是临时对象。
- `third_party/mygo/internal/text/layout.go` 的全局 `text.System` 只按完整 `text.Params` 做有限 LRU（8 MiB、4096 entries）缓存。累计文本每个片段都改变 `Params.Text`，不会复用前缀；宽度、样式或 span 编码不同也会形成不同 key。现有 `TextLines` 的按 paragraph 保留 layout 只用于 text area/editor，本对话渲染路径没有使用它。
- `ui.List` 只构建 viewport 加 overscan 行，因此成本不是所有历史行每帧都重排；当前跟随尾部时，增长中的最后一行以及当时可见的其他行会反复参与测量。Profiler 中出现的 `SurfaceEvent → ui.engine.runFrame → layoutTree/flexLayout → layoutList → heightAt/contentHeight → intrinsic/leafWidths → text.System.Layout → paragraph → coreText.shape/line` 与上述调用链一致，能证明采样落在文字 layout 路径，不能单独证明 Go 业务层根因或每个 chunk 的固定耗时。

因此 H1 可以作为已确认的实现假设：热点是“全量 source 版本化解析 + 可见累计消息的全量排版”，并叠加每个 SSE 的全 snapshot clone。尚未确认的是各阶段在真实三会话流中分别占用多少时间，以及异步解析丢弃了多少中间版本；这些需要埋点或新的受控采样才能量化。

### H3：session 切换后 cache、scroll 和 native element state 的生命周期

结论：没有发现不同 session 直接共享 conversation list 或 Markdown 文档的代码路径；现有实现明确按 session 隔离并保留阅读位置。发现的风险是生命周期分层不一致：App 级 list/Markdown cache 不会在切换或删除 session 时失效，切换期间 MyGo 的临时 node state 会被 prune 后重建，文本系统的全局 LRU 还可能独立淘汰。因此“跨 session 串缓存”未证实，“切换后重建/重复测量或历史状态长期滞留”是有源码依据的性能风险。

事实链如下：

- `App.conversationLists` 是 `map[string]*ui.ListState`，`conversationListState(session.ID)` 按 session ID 分配并永久保留。`conversation` 还给列表使用 `ui.List(c.Key("message-list-" + session.ID), ...)`，所以 MyGo 的列表 element identity 也按 session 分开。
- `ui.ListState` 自身保存 `anchor`、`anchorKey`、`inset`、`heights`、`following` 和可见范围。`selectSession` 只更新 active ID 和 workspace，不清空旧 list。`ListState.sync` 在存在 `Key` 时尝试用 message key 重定位 anchor，`layoutList` 只在宽度变化时 `heights.clear()`。已有 `TestConversationPositionsAreIndependentPerSession` 证明 A/B 两个 session 各自滚动后切换回来，阅读位置保持独立。
- message row key 优先使用 message ID，缺 ID 时退化为该 session 内的 index；Markdown cache key 还带 `sessionID` 前缀。因此相同 message ID 出现在不同 session 时不会命中对方文档，异步 Markdown 结果也只有在缓存中的完整 source 仍相同才会发布。
- 另一方面，conversation 没有 Router keep-alive。切换 session 后旧的 MyGo element/node state（包括 node 自己的 `leafOK`、`measures`、`tl`，以及 state 中的 `spanCache`）不再被当前 frame 构建，runtime 的 `prune` 会回收未保留的 state；切回时 UI tree 需要重新构建。`ListState` 和 `markdownCache` 仍在 App 中，所以滚动锚点和已解析文档可以保留，但这不等于所有 native layout state 都保留。
- `markdownCache`、`markdownPending`、`conversationLists` 和 code highlight maps 都没有按 session 删除或容量回收。若 session 被删除后仍留在这些 map 中，会继续占用内存；若同一 session ID 的 message history 被整体替换，旧的 `ListState.heights` 可能按旧行 index 继续估算，除非 key 重定位或宽度变化触发清理。这是边界风险，当前正常 controller 流程和已有测试没有证明它会在普通 A→B→A 切换中发生。

所以 H3 的当前判断是：普通 session 切换的 scroll 隔离和 Markdown key 隔离已经有实现与测试依据，不能把 round-5/6 的 AX 慢直接归因于跨 session stale cache；需要额外验证切换后首次 frame 是否出现 node 重建、`text.System.LayoutsMade` 增量和 AX tail-query 等待。session 删除、历史整体替换以及窗口宽度变化属于未覆盖的 cache/anchor 失效边界。

### 给 root 的最小回归与量化方案（本轮未执行）

先做源码级轻量计数，再做最小 native 对照，不引入新的缓存抽象：

1. 在临时诊断分支记录每次 `onText → Update → frame` 的 session ID、message key、累计 source 字节数、snapshot clone 耗时、Markdown cache hit/miss、parse start/end、block 数、source 过期丢弃次数和 pending 深度。对当前可见行记录 `text.System.LayoutsMade()` 前后差值，并区分 `leafWidths` 的 width=0/1 与实际包裹宽度；这样可以把“chunk 到达”与“实际 shape/layout”分开。
2. 用同一 fixture 只改变并发会话数（1、2、3），固定 chunk 数和文本结构。分别测首次 frame、尾部继续流、每 10 个 chunk、流结束后的稳定 frame；输出 source 长度、解析次数、LayoutsMade 增量、frame/layout 耗时和 AX tail-query 时间。single/two 的现有 PASS 只能作为基线，不能替代三会话。
3. 对 H3 先暖机 A、B、C 三个 session，在 A/B 各滚动到固定 message key，再按 A→B→C→A 切换。每次记录 `ListState.Visible/AtEnd`、anchor message key、Markdown cache entry 是否复用、首次切换 frame 的 LayoutsMade 增量和 AX tail token；再改变窗口宽度，验证 `layoutList` 清空 row heights 后是否仍能回到正确 anchor。
4. 若计数显示主要耗时在 `cloneSnapshot` 或 Markdown parse/layout，先分别优化那条已证实路径；若只有 AX tail-query 超时而 frame/layout 预算稳定，应继续把 AX IPC/等待单列诊断，不要把 CoreText 采样树当成生产根因。
