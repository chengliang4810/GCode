# 长会话与流式增长性能验证

日期：2026-10-12（Asia/Shanghai）

## 结论

当前证据包括 S1 测试宿主代码验证和正式包的限定 S3 原生交互复核。`ui.List` 的可视范围、底部/中段位置和 Markdown 缓存复用均有可重放测试；最小 render snapshot 修复已移除每帧对历史消息/工具/队列等 slice 的全量复制。正式包 `output/acceptance/native-conversation/final/` 的五张截图已由 root 实际打开，`review.json` 为 `PASS_FOR_RECORDED_SCOPE`：100/1000/5000 条合成持久化历史、中文/emoji AX 输入、合成滚动、阅读位置恢复和正常退出均通过。变化消息的流式路径仍会产生一次新的 Markdown 解析；持续 controller 流式、多个同时运行会话、物理输入和真实 API 性能仍未闭环，因此不支持 A13/A16 的完整 S3 通过结论。

性能修复包 executable SHA 为 `597842904bb16beb7b3d3c511e3ef785885a7c32f6ecbf329c03b415018e34ec`。该身份不是后续运行归属与授权/中断呈现修复包；最新正式包见 [current-package.md](current-package.md)，本节不倒填为新包的长历史实测。机械 manifest 仍保留 `PASS_PENDING_CAPTURE_REVIEW`，root 的人工复核为 `PASS_FOR_RECORDED_SCOPE`；原始 manifest 和人工复核分别见 [`manifest.json`](../../output/acceptance/native-conversation/final/manifest.json) 与 [`review.json`](../../output/acceptance/native-conversation/final/review.json)，详细边界见 [`native-conversation.md`](native-conversation.md)。

这轮原生记录的启动到 AX ready 为 3246.57 ms；切换到末尾 AX 读回为 5000/1000/100 条分别 470.08/460.01/642.06 ms，输入往返中位数分别为 265.62/248.23/310.97 ms。AX 往返包含 osascript IPC 和等待，不能当作帧时间、GPU 合成时间或物理输入延迟；三个会话共加载 6100 条消息，RSS 也不能当作独立历史规模的内存曲线。

## 可复核事实

### 代码路径

- `internal/desktop/conversation.go` 使用 `ui.ListState`/`ui.List`，行 key 来自消息 ID；这使 `ui.List` 能根据可视范围和 overscan 构建行，并保持底部跟随、滚动锚点和历史 prepend 行为。
- `internal/desktop/app.go` 的 `view` 现在执行 `snapshotForView(a.data)`：复制 `Settings/Providers` 和 session 元数据 slice，复用 `Open/Update` 已深拷贝并由 `App` 拥有的 `Messages/Tools/Queue/Permissions/Questions` 历史 slice。`Projects` 也只读共享。
- `internal/desktop/markdown.go` 以 `sessionID:rowKey` 缓存已完成的 Markdown 文档；源码不变时复用缓存，源码变化时重新解析。生产入口设置 `Window.Update` dispatcher，测试宿主基准没有启动原生窗口。
- 正式包复核通过生产持久化加载路径读取 100、1000、5000 条合成历史；五张截图的 hash 一致，100/1000/5000 均显示末尾消息，AX 逐字读回中文/emoji，合成系统滚轮实际改变历史 marker，切换离开再回来保持同一阅读范围，指针位置有独立恢复读回。该复核证明已记录的原生交互行为，不证明持续流式性能。

### 所有权审计

`Open` 和 `Update` 进入 `App` 前均调用 `cloneSnapshot`，因此调用方后续修改不会触碰 `a.data`。逐项搜索 `a.data` 写入后，当前桌面本地路径只有：

| 数据 | 本地写入 | view 是否复制 |
| --- | --- | --- |
| `Settings.Theme`、`DefaultSendMode` | `updateSettingsDraft` | `Settings` 值复制 |
| `Settings.Providers` | `addProviderDraft`、保存设置 | `Providers` slice 复制 |
| `Session.FullAccess` | composer checkbox | session 元数据 slice 复制 |
| `Messages/Tools` | 无 UI 原地写入 | 共享只读 |
| `Queue/Permissions/Questions` | 无 UI 原地写入 | 共享只读 |
| `Projects`、`Session.ActiveRun` | 无 UI 原地写入 | 共享只读 |

`render_snapshot_test.go` 覆盖了外部快照在 `Open/Update` 后修改不泄漏、32 次流式 Update 不改变历史、捕获的 frame 文本在本地 metadata 修改前保持不变，以及并发 `Update`/`Tester.Frame`。这些是行为断言，不以 slice pointer 相等作为正确性的唯一依据。

新增 `TestRenderSnapshotOldViewStaysStableAcrossGatedUpdate` 在锁内直接捕获 `snapshotForView`，然后用 channel gate 让 `App.Update` 改写嵌套 History、工具输出、权限参数、问题文本和 `ActiveRun`，并追加一条历史消息。Update 完成后旧快照仍与期望深比较相等；该旧快照还直接交给真实 `conversation` renderer，Tester 能看到旧最新正文，且看不到新正文和新增历史。

`TestRenderSnapshotCapturedFrameAndLocalMetadataAreIsolated` 通过真实 Tester 依次点击“完全访问”、进入“设置”、选择“深色”和“队列”，验证控件定位和本地状态改变。服务为 nil 时只拒绝服务副作用，不影响本地控件状态；测试同时验证 caller 的输入快照及历史消息不变，并确认点击前捕获的 frame 仍包含旧会话正文。`TestRenderSnapshotConcurrentUpdatesAndFrames` 用每轮 `begin/done` channel 配对 128 次 Update 和 Frame，未使用 `runtime.Gosched`。

### 行为测试

`TestConversationPerformanceViewportAndMarkdownReuse` 已验证：

- 5000 条会话首次显示位于底部附近，`Visible` 返回 bounded viewport，首屏之外的消息不会因为全量历史而全部构建。
- 初始 frame 真实显示最新消息正文；替换最后一条消息并刷新后，新的“流式增量已到达”正文可见，旧的最新消息标记不再可见。
- 修改最后一条消息并刷新后，未变化的可见消息仍指向同一份 Markdown 文档缓存。

`TestConversationPerformanceMiddleViewport` 已验证中段 `ScrollTo` 能使目标行进入可视范围，且该范围没有错误地覆盖会话首尾。测试只断言虚拟化/位置行为，不设置延迟通过线。

### 基准结果

命令：

```text
go test ./internal/desktop -run "^$" -bench "BenchmarkConversation(InitialFrame|WarmFrameAtEnd|WarmFrameInMiddle|StreamingUpdate|SnapshotClone|MarkdownParse)$" -benchmem -benchtime=500ms -count=1 -timeout=180s
```

宿主：`darwin/amd64`，`MacBookPro16,1`，12 logical CPUs，Go `go1.27.2`；完整环境见 [`environment.txt`](../../output/acceptance/conversation-performance/environment.txt)。下表是单次 500 ms benchmark 的原始输出摘要，单位为 `ns/op`、`B/op`、`allocs/op`。`修复前`来自 `bench-500ms.log`，`修复后`来自同命令的 `bench-after-500ms.log`；单次 benchmark 只用于记录当前样本，不构成任意性能门槛。

| 路径 | 100 条 | 1000 条 | 5000 条 |
| --- | ---: | ---: | ---: |
| 初始 frame（前） | 6,842,804 / 4,488,865 / 15,711 | 6,910,604 / 4,754,128 / 16,005 | 8,163,388 / 6,122,319 / 17,635 |
| 初始 frame（后） | 7,489,061 / 4,500,075 / 15,763 | 6,656,233 / 4,567,502 / 15,773 | 6,705,965 / 5,257,786 / 16,623 |
| 热底部 frame（前） | 428,316 / 48,778 / 285 | 457,909 / 205,605 / 473 | 773,451 / 888,634 / 1,273 |
| 热底部 frame（后） | 338,539 / 30,473 / 264 | 337,875 / 30,751 / 272 | 341,199 / 30,760 / 272 |
| 热中段 frame（前） | 560,887 / 70,801 / 471 | 655,652 / 228,063 / 665 | 961,191 / 911,405 / 1,466 |
| 热中段 frame（后） | 774,552 / 52,461 / 450 | 516,810 / 52,951 / 464 | 516,360 / 52,952 / 464 |
| 流式 Update + frame（前） | 543,010 / 73,061 / 313 | 694,889 / 443,988 / 688 | 1,565,969 / 2,244,595 / 2,297 |
| 流式 Update + frame（后） | 601,732 / 54,792 / 292 | 639,535 / 269,080 / 486 | 1,209,902 / 1,386,713 / 1,296 |
| `cloneSnapshot`（前） | 6,028 / 18,624 / 22 | 63,638 / 175,168 / 202 | 358,336 / 858,176 / 1,002 |
| `cloneSnapshot`（后） | 5,875 / 18,624 / 22 | 52,750 / 175,168 / 202 | 339,168 / 858,176 / 1,002 |

单条稳定 Markdown 文本解析修复前为 `9,682 ns/op`、`7,264 B/op`、`63 allocs/op`，修复后为 `8,924 ns/op`、`7,264 B/op`、`63 allocs/op`。完整原始结果见 [`bench-500ms.log`](../../output/acceptance/conversation-performance/bench-500ms.log) 和 [`bench-after-500ms.log`](../../output/acceptance/conversation-performance/bench-after-500ms.log)。

这些数字不能相加成“原生帧时间”：初始 frame 包含 Tester 建立和首批可见 Markdown 解析；热 frame 包含离屏 UI 构建/排版及快照复制；流式路径包含 `App.Update` 的复制和变化文本解析。它们没有包含 OS 原生窗口、GPU 合成、真实 controller 发布频率、触控板惯性或输入法。

## 性能判断

### 已验证

1. 5000 条会话没有让 `ui.List` 构建 5000 行；可视范围测试和中段定位测试通过。
2. 修复前 5000 条热底部/中段 frame 分别约 889 KB/911 KB 分配；修复后分别约 31 KB/53 KB，且两种规模的热帧分配基本不再随历史消息数增长。
3. 5000 条流式 Update + frame 从约 1.57 ms、2.24 MB 降至约 1.21 ms、1.39 MB；剩余成本来自 `App.Update` 的深拷贝和变化消息 Markdown 解析。
4. `cloneSnapshot` 与 Markdown parser 基准本身未被修复改变；它们用于隔离总成本中的深拷贝和解析部分。
5. 不变可见消息跨流式更新复用 Markdown 文档；不会因为最后一条消息变化而重新解析所有可见消息。
6. 正式包在 MacBookPro16,1 / macOS 14.8.7 上通过已记录范围的原生交互复核：三种合成历史规模可读到底部，中文/emoji AX 输入可逐字读回，系统 HID 合成滚轮可移动历史，切换会话后阅读 marker 和指针位置恢复，正常退出无残留 QA PID。`review.json` 只对该记录范围给出 `PASS_FOR_RECORDED_SCOPE`。

### 推断

- 修复前离屏宿主尚未显示出必然的单帧秒级阻塞，但高频 controller 事件下每帧全量复制造成了可测的线性分配增长；修复后该项已从 view 路径移除。
- 生产入口的 `SetDispatch(h.window.Update)` 让后台 Markdown 解析不在正常 UI 构建路径上同步执行。正式包已经有启动、AX 读回和合成滚动的限定原生样本，但还没有连续 controller 流式 trace 或持续事件采样来证明 UI queue、帧合成和 GC 在长时间流式更新中保持响应。
- 已应用的最小行为保持修复是 `snapshotForView`：只复制本地 UI 会改写的 settings/providers/session 元数据，历史 slice 保持 App-owned 只读共享。若原生采样或后续功能引入历史 slice 原地写入，需要撤销该假设并重新设计所有权；当前没有证据要求版本缓存或 UI 配置项。

### 未验证

- 持续 controller 流式更新下的原生 UI queue、帧时间、GC、GPU 合成和输入响应；当前 AX 耗时不能替代这些指标。
- 多个同时运行会话、真实 controller/Service 事件频率、工具输出持续增长、长行和大 Diff 的原生性能；6100 条合并加载不能替代独立历史规模 RSS 曲线。
- 物理 trackpad/滚轮惯性、物理键盘、中文 IME 候选、剪贴板、主题切换组合场景，以及真实用户持续阅读位置。
- 真实厂商 API、Apple Silicon、Windows 或任何外部 API 的性能。

## 执行记录

以下命令均设置了 Go 测试超时或 shell alarm，结果保留在专属输出目录：

```text
go test ./internal/desktop -count=1 -timeout=120s
go test -race ./internal/desktop -count=1 -timeout=180s
LC_ALL=C perl -e 'alarm 120; exec @ARGV' go vet ./internal/desktop
go test ./internal/desktop -run '^$' -bench 'BenchmarkConversation(InitialFrame|WarmFrameAtEnd|WarmFrameInMiddle|StreamingUpdate|SnapshotClone|MarkdownParse)$' -benchmem -benchtime=500ms -count=1 -timeout=180s
go test ./internal/desktop -run 'TestRenderSnapshot|TestConversationPerformanceViewportAndMarkdownReuse' -count=1 -v -timeout=120s
go test -race ./internal/desktop -run 'TestRenderSnapshot|TestConversationPerformanceViewportAndMarkdownReuse' -count=1 -v -timeout=180s
```

对应文件：修复前的 [`desktop-test.log`](../../output/acceptance/conversation-performance/desktop-test.log)、[`desktop-race.log`](../../output/acceptance/conversation-performance/desktop-race.log)、[`desktop-vet.log`](../../output/acceptance/conversation-performance/desktop-vet.log)、[`bench-500ms.log`](../../output/acceptance/conversation-performance/bench-500ms.log)；修复后的 [`desktop-test-after.log`](../../output/acceptance/conversation-performance/desktop-test-after.log)、[`desktop-race-after.log`](../../output/acceptance/conversation-performance/desktop-race-after.log)、[`desktop-vet-after.log`](../../output/acceptance/conversation-performance/desktop-vet-after.log)、[`bench-after-500ms.log`](../../output/acceptance/conversation-performance/bench-after-500ms.log) 和 [`render-conversation-after.log`](../../output/acceptance/conversation-performance/render-conversation-after.log)。普通测试、race、vet 和两个 benchmark 本次均为 exit 0。它们是测试宿主结果；正式包原生复核的固定产物见 `output/acceptance/native-conversation/final/`，仍不能替代持续流式、并发任务、物理输入和真实 API 性能证据。

新增行为复核的原始日志为 [`render-snapshot-review.log`](../../output/acceptance/conversation-performance/render-snapshot-review.log) 和 [`render-snapshot-review-race.log`](../../output/acceptance/conversation-performance/render-snapshot-review-race.log)，对应命令均为 exit 0。新增日志只证明上述测试宿主行为和 race 安全；正式包原生交互证据以 `output/acceptance/native-conversation/final/` 的 manifest、截图和 review 为准，仍不覆盖持续流式、并发任务、物理输入或真实 API 性能。
