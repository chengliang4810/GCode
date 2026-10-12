# /goal：长会话视图与流式增长性能证据

状态：测试宿主证据、最小 render snapshot 修复和正式包限定原生交互复核已完成；A13/A16 仍不能据此标记完整通过。

日期：2026-10-12（Asia/Shanghai）

## 目标

用真实 `mygo ui.Tester` 和当前桌面构建链测量 100、1000、5000 条中文 Markdown 会话，覆盖底部和中段可视范围、单条流式消息增高及 controller 快照刷新。报告必须把消息快照复制、可视行构建/排版、Markdown 解析和原生窗口性能分开，保留稳定可重放输入和原始日志，不使用人为阈值宣称性能通过。

## 所有权与边界

- 专属测试代码：`internal/desktop/conversation_performance_test.go`。
- 专属文档：本文件、`docs/verification/conversation-performance.md`。
- 专属原始输出：`output/acceptance/conversation-performance/`。
- root 明确允许后，仅修改 `internal/desktop/app.go` 的 view 数据获取；不修改视觉 token、布局、controller、settings/conversation 本地修改路径；本专项测试不启动原生窗口；正式包原生复核由 root 独立执行并记录；不读取凭据、不访问外部 API；不提交或推送。
- `ui.NewTester`、Go benchmark、test、race 和 vet 只属于测试宿主证据（S1）。不能把它们扩大成 macOS 原生窗口、物理滚轮/触控板、输入法、GPU 或 OS 调度证据（S3）。

## 输入与测量面

fixture 由固定种子逻辑生成，每条消息包含中文段落、列表、Go fenced code block 和 Markdown 表格，每五条包含带 JSON 参数和输出的 `exec_command` 工具调用。消息 ID、时间戳和文本顺序稳定，规模为 100/1000/5000。

测量分为以下路径：

1. `BenchmarkConversationInitialFrame`：创建 `App`、离屏 Tester、快照复制、首屏可视行构建、排版和可见消息 Markdown 解析的合计。
2. `BenchmarkConversationWarmFrameAtEnd` 与 `BenchmarkConversationWarmFrameInMiddle`：缓存已热、分别位于底部和中段的 `Tester.Frame`；可观察每次 `App.view` 快照复制和可见行构建/排版。
3. `BenchmarkConversationStreamingUpdate`：修改最后一条消息并调用 `App.Update`，再绘制一帧，覆盖快照复制、流式消息 Markdown 重解析和末行更新。
4. `BenchmarkConversationSnapshotClone`：单独测量 `cloneSnapshot` 的全量消息/工具复制成本。
5. `BenchmarkConversationMarkdownParse`：单独测量稳定 Markdown 文本解析成本。

可视范围测试还断言 5000 条会话仍只构建末尾附近的 bounded viewport，中段 `ScrollTo` 能落在目标行，并且流式更新后未变化的可见 Markdown 仍复用缓存。断言用于证明行为和缓存复用，不是延迟门槛。

## 当前判断

- 已有真实 mygo 组件、稳定输入、`b.ReportAllocs`、普通测试、race 和 vet 证据；结果见 `docs/verification/conversation-performance.md`。修复前 `bench-500ms.log` 和修复后 `bench-after-500ms.log` 均保留。
- 修复前真实瓶颈是 `App.view` 每帧对整个 `appmodel.Snapshot` 执行 `cloneSnapshot`，其成本随所有消息和工具数量增长，即使 `ui.List` 只构建可见行也无法消除这项复制。
- 当前 `snapshotForView` 只复制 `Settings/Providers` 和 session 元数据 slice，复用由 `Open/Update` 深拷贝拥有的历史 slice；ownership 测试覆盖调用方修改、连续流式 Update、frame 捕获、本地 `FullAccess/settings` 修改和并发 Update/frame。
- 新增行为证据直接捕获锁内的 `snapshotForView` 旧快照，并在 channel-gated `Update` 改写多层 History、Tools、Permissions、Questions、`ActiveRun` 后，用 `reflect.DeepEqual` 和真实 `conversation` renderer 检查旧正文仍可见、新正文及新增历史不可见。
- `TestRenderSnapshotCapturedFrameAndLocalMetadataAreIsolated` 通过真实 `ui.Tester` 点击“完全访问”、设置页“深色”和“队列”，再验证本地状态生效、调用方输入和历史消息没有被改写；并发测试用每轮 `begin/done` channel 配对 `Update` 与 `Tester.Frame`，不依赖 `runtime.Gosched`。
- `TestConversationPerformanceViewportAndMarkdownReuse` 额外确认 5000 条会话初始最新正文可见，替换后的流式正文可见且旧最新正文消失，同时保留未变化可见消息的 Markdown 文档 pointer 复用断言。
- 本轮普通与 race 行为复核原始日志分别保存在 `output/acceptance/conversation-performance/render-snapshot-review.log` 和 `output/acceptance/conversation-performance/render-snapshot-review-race.log`，两条命令均 exit 0。
- 变化消息在没有 dispatcher 的测试宿主中会走同步 Markdown 解析；生产入口设置了 `Window.Update` dispatcher，异步解析降低了 UI 线程阻塞风险。正式包复核已补充启动、AX 读回、合成滚动和位置恢复样本，但 native queue、持续事件频率和原生帧时间仍未测量。
- 正式包 `output/acceptance/native-conversation/final/` 已完成限定原生交互复核，root 实际查看五张截图并将 `review.json` 标为 `PASS_FOR_RECORDED_SCOPE`。100、1000、5000 条合成持久化历史均显示末尾消息；中文/emoji AX 输入逐字读回；合成系统滚轮能上移历史；切换会话再回来保留阅读 marker；指针位置独立读回并恢复；正常菜单退出为 exit 0。该包 executable SHA 为 `597842904bb16beb7b3d3c511e3ef785885a7c32f6ecbf329c03b415018e34ec`。
- 原生记录中的启动到 AX ready 为 3246.57 ms，切换到末尾 AX 读回为 470.08/460.01/642.06 ms，输入往返中位数为 265.62/248.23/310.97 ms；这些耗时包含 AX/osascript IPC 和等待，不能解释为帧时间或物理输入延迟。持续 controller 流式、多同时运行会话、物理 trackpad/keyboard/IME 和真实 API 性能仍未闭环；6100 条消息同时加载也不能给出独立历史规模 RSS 曲线。
- 当前结果足以向 root 提供 S1 代码性能边界和已记录范围的 S3 原生交互证据，仍不能把 A13/A16 的持续流式响应、并发任务内存、物理输入或真实 API 性能标记为完整通过。

## 后续动作

root 负责把本结果与剩余的持续 controller 流式、多个同时运行会话、物理输入和真实 API 性能证据合并；若后续采样发现 dispatcher 线程归属或共享只读约束不成立，应回滚该最小修复并重新设计数据所有权。不要把 AX 往返或单轮 RSS 改写成原生帧时间、独立历史规模曲线或完整 S3 性能结论。
