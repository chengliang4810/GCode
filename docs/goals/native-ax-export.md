# /goal：原生 AX 导出与 H4 只读调查

日期：2026-10-12（Asia/Shanghai）

状态：静态源码调查、正式 binary 历史 sample 符号化、round-15/16 诊断、round-17 仅 tail 的记录范围复验，以及 round-18/19 含 final completion 的双轮记录范围确认已整理完成。H4 的“每次 AX 查询都会读取并排版完整增长历史”已降级；“外部 AX 请求等待主线程完成当前布局/AX 缓存更新”仍保留为次级待证伪假设。round-18/19 均为 `PASS_FOR_RECORDED_SCOPE`；视觉审批、real API、性能和滚动效果仍未完成，当前主题的原生流式验收仍为 pending。责任：root。

范围：本目标只调查原生 macOS AX 导出、`ui.List` 虚拟化、UI 主线程队列和既有 AX 超时证据。本次文档编辑不启动窗口、不运行测试、不构建，不修改生产源码、原生驱动或测试；只整理已有源码和 round-15/16/17/18/19 运行证据。

## 结论

当前生产会话视图明确使用 `ui.List`。`internal/desktop/conversation.go:20-64` 把消息总数交给 `ui.List`，但每一帧只通过 row builder 构造计划范围内的行；`third_party/mygo/ui/list.go:11-29,291-314,467-502` 说明该范围是视口和两侧 overscan，计划范围单帧还有 `listMaxRows=4096` 上限（必要的 focused/header 行可能另行补建）。因此，源码不支持“AX 查询随着完整历史增长而主动读取、排版所有消息”这一版 H4。

AX 导出消费的是最近一帧的 `AccessTree` 和 `accessElement.node` 缓存。`accessTree()` 只递归当前 frame 中已经存在的节点，列表行通过 `PosInSet` 和 `SetSize` 描述它在完整列表中的位置；Darwin `UpdateAccessibility` 更新缓存的 children、rows、visible rows 和 row count，单元素 getter 直接读取缓存值。`accessibilityStringForRange:` 也只切片当前元素的 `el.node.Value`，没有调用 `runFrame`、列表布局或历史重读。

正式 binary 的 round-5 sample 进一步把“当时主线程在做什么”落到了布局和 CoreText：从 AppKit `updateLayer` 的 `SurfaceFrame` 进入 `windowHost.event`、`engine.surfaceFrame`、`runFrame`，随后出现 `layoutList`、`heightAt`、`contentHeight`、`intrinsic`、`leafWidths`、`internal/text.System.Layout`、`paragraph` 和 Darwin CoreText `shape/line`。符号化映射中没有出现 `accessibilityChildren`、`UpdateAccessibility`、`accessTree` 或 AX getter。该 sample 是一个瞬时栈快照，映射列表不是完整调用栈；结论只采用源码能够确认的 caller nesting，不把单独出现的函数名当成完整堆栈。

所以应把 H4 分成两层判断：

- **H4a：全历史 AX 导出触发完整历史排版。** 在当前 `ui.List` 路径下，源码和 sample 都不支持它，降级为低优先级；除非后续发现生产路径绕过 `ui.List` 或 AppKit 在缓存之外触发了另一路完整内容 materialization。
- **H4b：外部 AX/System Events 请求等待应用主线程。** 仍然成立为可行解释。`Window.Update` 异步把更新投递到主线程，`runFrame` 在同一线程完成 build/layout/paint/present 后更新 AX 缓存；外部 `osascript` 还包含 AX server 和跨进程等待。CoreText 布局 sample 与第二次 AX click 的等待时间重合使该解释有证据，但没有证明 AX 请求本身触发了布局。

当前更强的工作方向是“流式更新期间主线程 frame/layout/text shaping 竞争，外部 AX 读回因此等待”；这加强了对 H1/主线程竞争的关注，但单个 OS sample 不能证明总耗时、触发关系或最终根因。

round-13/14/15 把这一判断的边界进一步收窄：失败会话中外部查询多次成功返回，却始终没有当前 tail，最后才因严格 deadline 失败。因此，18 秒级总耗时不能直接写成 AX IPC 卡住；单次较慢查询仍可能等待主线程或 AX server，但当前应优先核对 `last row` 选择、上一帧 AX cache、session 的列表 anchor/following 和查询形状。round-15 还提供了反向证据：同一失败截图中目标会话已选中，tail 已在窗口视口内绘制，因而“内容当时没有画出”不能解释这次失败。

round-16 的失败后 role-tree 诊断在 path `[0,14]` 找到了 message `AXTable`、两行中的 `AXIndex=1` 末行，以及该行下多个直接 `AXStaticText` tail token。这降低了“`table 1` 或 `last row` 必然选错”以及“tail 只能在不可遍历的嵌套节点中”作为唯一解释的优先级；但诊断发生在失败查询和截图之后，且同时观察到大量 unknown role 与元素失效错误，没有 cache/tree 版本或对象 identity 将它们和 29 次查询逐一对应，因此不能把失效节点写成已证实根因。

round-17 在保留 parsed append prefix、避免每个 chunk 在 raw/Markdown 之间交替，并让 `renderTextBlocks` 的解析完成按队列发布后，三会话正式包和四次 AX tail 读取均通过。这个干预结果支持“流式期间文本树反复替换或完成顺序竞争参与了失败”的方向，但不能单独区分 prefix 保留、completion queue 或其他时序变化的因果贡献；round-17 的证据范围仍只有 tail readback，当前主题验收仍需由 root 按完整范围收口。

## 证据链

| 层次 | 已确认事实 | 位置或证据 | 对 H4 的含义 |
| --- | --- | --- | --- |
| 生产视图 | 消息 `rowCount` 可随历史增长，但行 builder 只为 `ui.List` 当前计划范围执行；当前消息正文进入该行 | [conversation.go](../../internal/desktop/conversation.go#L20-L64)、[renderMessage](../../internal/desktop/conversation.go#L141-L180) | 完整历史数量不等于完整历史节点已建成 |
| 列表规划 | `List` 只构建视口及两侧 2 行 overscan；`plan` 计算 `[first,last)`，计划范围单帧最多 4096 行，必要的 focused/header 行可能另行补建 | [list.go](../../third_party/mygo/ui/list.go#L11-L29)、[list.go](../../third_party/mygo/ui/list.go#L291-L367)、[list.go](../../third_party/mygo/ui/list.go#L467-L502) | 不支持每次 AX 读取全量历史的前提 |
| AccessTree | `accessTree` 从 `rt.c.root` 递归；`accessElement` 只遍历当前 frame 中存在的节点；row 的 label 来自当前 row 的 `innerText`，`PosInSet/SetSize` 表示完整列表元数据 | [access.go](../../third_party/mygo/ui/access.go#L223-L267)、[access.go](../../third_party/mygo/ui/access.go#L285-L337)、[access.go](../../third_party/mygo/ui/access.go#L384-L419) | AX tree 节点规模预期受可见/overscan 行约束 |
| Darwin 缓存 | `UpdateAccessibility` 复用 element ID、更新 children/rows/visible rows/selected rows/row count，并保存 `s.access`；view children、hit test、focused element 都读缓存 | [access.go](../../third_party/mygo/internal/darwin/access.go#L342-L495)、[access.go](../../third_party/mygo/internal/darwin/access.go#L741-L781) | getter 没有主动重排整段历史 |
| 单元素 getter | frame、attribute、range、text range 都使用 `el.node`；文本 range 只对当前 element value 做 UTF-16 切片 | [access.go](../../third_party/mygo/internal/darwin/access.go#L573-L617)、[access.go](../../third_party/mygo/internal/darwin/access.go#L714-L737) | 不支持 AX getter 本身遍历完整会话 |
| 首次 AX | `accessibilityChildren` 首次调用 `accessRequested`；`accessRequested` 只在 false→true 时发送 `AccessibilityOn`；runtime 立即把当前 `accessTree` 交给 host | [access.go](../../third_party/mygo/internal/darwin/access.go#L333-L340)、[access.go](../../third_party/mygo/internal/darwin/access.go#L747-L757)、[ui/access.go](../../third_party/mygo/ui/access.go#L464-L474) | 首次开启可能触发当前 frame 的 AX 建立，但仍没有全历史读取证据 |
| frame 路径 | `runFrame` 在同一 UI 线程执行 view build、layout、paint、present，最后更新 AX tree；`surfaceFrame` 在不能只 repaint 时进入完整 `runFrame` | [runtime.go](../../third_party/mygo/ui/runtime.go#L358-L484)、[runtime.go](../../third_party/mygo/ui/runtime.go#L538-L548) | AX 等待主线程忙于 frame 是可行残余解释 |
| 更新队列 | `Window.Update` 异步投递并明确不等待；`RunOnMain` 投递后等待；Darwin run-loop callback 先 drain 主队列再 Dispatch | [content.go](../../third_party/mygo/content.go#L145-L180)、[loop.go](../../third_party/mygo/loop.go#L17-L54)、[loop.go](../../third_party/mygo/loop.go#L75-L100)、[app.go](../../third_party/mygo/internal/darwin/app.go#L111-L138) | 外部 AX IPC 与 Go 更新队列不是同一同步 API，但可能共同等待主线程 |
| round-15 运行证据 | 失败截图中 session-2 已选中且 tail 在窗口视口内；同一轮 29 次外部查询均成功返回但没有 own marker/tail，最后由严格外层 deadline 失败 | [results.json](../../output/acceptance/native-streaming/round-15-selection-fixed/results.json)、[measurement-samples.json](../../output/acceptance/native-streaming/round-15-selection-fixed/measurement-samples.json)、[failure-window.png](../../output/acceptance/native-streaming/round-15-selection-fixed/captures/failure-window.png) | 排除“tail 尚未绘制”；优先检查 table/row 选择、嵌套文本路径和 AX cache 时序 |
| round-16 AX role-tree | 失败后 traversal 识别 message table `[0,14]`、`row_count=2`、末行 `AXIndex=1`，并在 `[0,14,1,37]` 等路径看到直接 tail static text；同时有 `unknown=680`、`AXRole:-25202=2090` 等失效元素错误 | [failure-ax-role-tree.json](../../output/acceptance/native-streaming/round-16-ax-role-tree/failure-ax-role-tree.json)、[ax-tree-diagnostic.json](../../output/acceptance/native-streaming/round-16-ax-role-tree/ax-tree-diagnostic.json) | 支持“后续树中存在正确 row/text”；失效观察与早先查询失败的因果仍未建立 |
| round-17 parsed-prefix | parsed-prefix/cache 和 completion queue 调整后，三会话、30 秒、10 Hz 通过；四次 AX query 各一次尝试，约 `297.723–413.032 ms`，均读到各自 tail | [review.json](../../output/acceptance/native-streaming/round-17-parsed-prefix/review.json)、[measurement-samples.json](../../output/acceptance/native-streaming/round-17-parsed-prefix/measurement-samples.json) | 支持干预后失败模式消失，但仅是记录范围内结果，当前主题验收仍 pending |
| round-18/19 integrated final completion | 同一正式包和冻结 driver 的两轮独立记录均为三会话、30 秒、10 Hz、每会话恰好 300 chunks；每轮 4 次 streaming tail + 3 次 final completion AX，各一次尝试；3/3 final completion token 唯一、own marker/token 可见、foreign marker/token 为空；最终三会话 idle 且无 active run | [round-18 review/results/manifest/measurement](../../output/acceptance/native-streaming/round-18-integrated-final-completion/)、[round-19 review/results/manifest/measurement](../../output/acceptance/native-streaming/round-19-integrated-final-confirmation/) | 把 final completion readback 从未证实推进到双轮记录范围内通过；仍不证明视觉、真实 API、性能或 H4b 根因 |

## 正式 binary sample 与既有失败事实

### round-5 符号化

[go-frame-symbolication.json](../../output/acceptance/native-streaming/round-5/go-frame-symbolication.json) 针对正式包自身的 `build/darwin-amd64/GCode.app/Contents/MacOS/GCode` 使用 Go pclntab 和 `go tool addr2line` 符号化历史 OS sample；可执行文件 SHA-256 为 `7bb8174524b12fec71a6b3bf5c653319f017394c7050b9f467e1091ebb5fd885`，sample SHA-256 为 `998e2c9cb34ec81fd6174992ddcea6ce61aa3caf26a23785802e417823fd4d40`（以文件记录为准；该证据未重新构建 binary）。关键 caller nesting 为：

```text
AppKit updateLayer
  → MyGo registerSurfaceClass.func14 (surface.go:655)
  → windowHandler.SurfaceEvent (content.go:222)
  → windowHost.event (window.go:139)
  → engine.event (input.go:114)
  → engine.surfaceFrame (runtime.go:548)
  → engine.runFrame (runtime.go:440)
  → layoutTree / layoutBox / boxLayout / flexLayout
  → node.layoutList (list.go:1069)
  → heightAt / contentHeight / intrinsic / leafWidths
  → text.System.Layout / layout / paragraph
  → coreText.shape / coreText.line
```

`surface.go:633-655` 显示该 Darwin callback 是 `updateLayer` 发出 `SurfaceFrame`；它不是 `accessibilityChildren`、AX attribute getter 或 `UpdateAccessibility` 的入口。`runFrame` 的映射行落在 layout/paint/present 区域，且该 sample 没有映射出 AX 方法。因 sample 是瞬时栈，不能据此断言整个 12 秒期间主线程始终在该调用链，也不能据此断言 AX 查询没有并发等待；它只能把观测到的主线程活动归类为 frame layout/CoreText。

[round-5 review](../../output/acceptance/native-streaming/round-5/review.json) 记录：首个 session AX 聚合耗时 `1245.197 ms`；第二个 session click 耗时 `12005.453 ms`，在 tail query 前超时，`normal_exit=false`，最终退出码 `-9`；独立 collector 仍完成诊断采样。review 明确把 profile 限制为“Main-thread CoreText activity; no Go root cause established; profiler has overhead”。这支持“AX/主线程交互确实可能卡住”的事实，不支持“AX 自己排版全历史”的结论。

### round-11-three-25

[round-11 review](../../output/acceptance/native-streaming/round-11-three-25/review.json) 与 [measurement-samples.json](../../output/acceptance/native-streaming/round-11-three-25/measurement-samples.json) 记录了三会话、25 秒、10 Hz 的另一种失败：

- 首个 session click 约 `364.953 ms`；随后 4 次 AX 查询成功返回但没有当前 tail，耗时约 `450.372/677.163/615.851/392.405 ms`。
- 第 5 次 query 约 `12006.567 ms` 超时，第 6 次按剩余预算约 `2828.034 ms` 超时，tail-query 总等待 `18372.889 ms`；controller 仍看到目标 session running，fixture 三条请求仍 active。
- 三会话 backend 在取消前已有各自 marker/run 归属证据，但第三个 session 最终为 interrupted，不能把整轮写成完整后台通过；正常退出为 0，collector 有 `138` samples（`115` controller polls、`23` process samples、`0` errors）。

这些样本证明外部 AX tail query 可能在流式更新期间卡住或反复读不到 tail；它们没有区分 `osascript` IPC、System Events traversal、AppKit AX server、应用主线程布局、AX cache update 和 query shape 的耗时。round-5 的符号化只把重合 sample 落到 frame/CoreText，不能替代这一区分。

### round-13-layout-fixed 与 round-14-fixed-frame-stats

[round-13 review](../../output/acceptance/native-streaming/round-13-layout-fixed/review.json)、[round-13 results](../../output/acceptance/native-streaming/round-13-layout-fixed/results.json)、[round-14 review](../../output/acceptance/native-streaming/round-14-fixed-frame-stats/review.json) 和 [round-14 results](../../output/acceptance/native-streaming/round-14-fixed-frame-stats/results.json) 给出了更新后的失败形状。两轮均使用 executable SHA-256 `84801c207dd39cd2c5274838dfb9cfa80a58a1ec0e68892b74d18024e99b5d50` 和 source code SHA-256 `741d49c5f9bf4ae25cd011a023f45a189c38b05e37e5e47732cc54c20706f764`，三会话、30 秒、10 Hz，正常退出码为 0，collector 无错误。

- round-13 的 session-1 成功，聚合耗时 `1687.505 ms`，第 3 次查询约 `370 ms` 返回 tail。session-2 在 click `383.380 ms` 后有 30 次查询快速成功返回，但 tail 始终不可见，最终经过 `18393.848 ms` 的严格 deadline 失败；这些查询没有 `ax_timeout`。
- round-14 的 session-1 成功，聚合耗时 `708.231 ms`，一次查询 `300.795 ms`、返回 `3445 bytes` 且 tail 可见。session-2 成功，聚合耗时 `904.838 ms`，一次查询 `483.451 ms`、返回 `1321 bytes` 且 tail 可见。session-3 在 click `361.919 ms` 后有 28 次查询快速成功返回但 tail 不可见，最终经过 `18372.960 ms` 的严格 deadline 失败。
- 原始 AX 文本中反复出现 `missing value`：round-13 session-1 为 `3610 bytes`、22 次 tail、68 个；round-14 session-1 为 `3445 bytes`、24 次 tail、38 个；round-14 session-2 为 `1321 bytes`、1 次 tail、85 个。返回成功只证明这一次外部调用拿到了某个 AX 响应，不证明拿到的是当前逻辑末行或完整正文。

这使 round-13/14 的失败模式更准确地表述为“AX 查询很快，但返回的缓存/树内容不含当前 tail”，而不是“AX IPC 一直阻塞”。当前 driver 查询是 [`value of every static text of last row of table 1 of window 1`](../verification/scripts/native-streaming-qa.py#L897-L910)，它同时假定 `table 1` 是外层 conversation list、`last row` 是最新消息，并且正文和 tail 都是该 row 下可直接枚举的 static text；这些假定还没有进程内节点路径证据。`missing value` 和返回长度的大幅变化更适合作为树形状或缓存时序的信号，不能单独当作 AX server 错误。

对 focus、scroll 和列表状态的静态核查没有发现 AX getter 会同步重建完整历史：

- Darwin 的 `accessElement.act` 把 `AccessFocus` 或 `AccessScrollIntoView` 送回 `SurfaceEvent`；`accessAction` 对普通 focus 调 `focusOn`，choosable row 则先更新选择 click 状态；对列表行按已构建的 `l.list.rows` 调 `ListState.ScrollIntoView`，随后都只调用 `requestFrame`。`focusOn` 会标记 `focusVisible` 并登记 `reveal`；它没有直接更新 AX cache。[access.go](../../third_party/mygo/ui/access.go#L476-L517)、[access.go](../../third_party/mygo/ui/access.go#L546-L557)、[scroll.go](../../third_party/mygo/ui/scroll.go#L111-L118)
- `ListState.ScrollIntoView` 只设置 `{set: true, row, near: true}`，没有把 `FollowEnd` 打开；下一次布局按 `req.end` 或 `following` 决定是否从最后一行开始，`remember` 再按 `FollowEnd && atEnd` 保存 following。session 切换时，`sync` 还可能从旧 `scrollY` 恢复 offset。因此，focus/scroll action、旧 anchor 或未处于末尾的 session 都可能使当前 viewport 的最后已构建行不是逻辑末行。[list.go](../../third_party/mygo/ui/list.go#L160-L176)、[list.go](../../third_party/mygo/ui/list.go#L371-L440)、[list.go](../../third_party/mygo/ui/list.go#L467-L502)、[list.go](../../third_party/mygo/ui/list.go#L1089-L1137)、[list.go](../../third_party/mygo/ui/list.go#L1290-L1305)
- 真正的 AX tree 更新发生在 `runFrame` 完成 build/layout/paint/present 之后；`accessibilityChildren` 首次只开启 access 并返回已保存的 `topLevel`，单元素属性也读 `accessElement.node`。因此 action 触发的下一帧尚未到达时，紧接着的外部 getter 可以快速读到上一帧的 AX tree。这个时序与 round-13/14/15 的“快速成功但无 tail”一致，但尚未由运行时时间戳证明。[runtime.go](../../third_party/mygo/ui/runtime.go#L358-L483)、[access.go](../../third_party/mygo/internal/darwin/access.go#L333-L340)、[access.go](../../third_party/mygo/internal/darwin/access.go#L342-L494)、[access.go](../../third_party/mygo/internal/darwin/access.go#L741-L781)

正文本身也不是固定的单一 AX 叶节点。pending Markdown 按行创建多个 `RichText` 和空行 `Box`；parsed Markdown 的段落、列表、表格和代码块有不同的 `Column/Row`、`ScrollHorizontal/Table`、代码块等嵌套结构。[markdown.go](../../internal/desktop/markdown.go#L352-L372)、[markdown.go](../../internal/desktop/markdown.go#L487-L528)、[markdown.go](../../internal/desktop/markdown.go#L556-L575)、[markdown.go](../../internal/desktop/markdown.go#L599-L627) 只有在同一 frame 的真实 AX 节点树中确认 row、static text 和 nested table 的路径后，才能判断当前 query 是否真的取到了 tail。

### round-15-selection-fixed

[manifest](../../output/acceptance/native-streaming/round-15-selection-fixed/manifest.json)、[results](../../output/acceptance/native-streaming/round-15-selection-fixed/results.json) 和 [measurement samples](../../output/acceptance/native-streaming/round-15-selection-fixed/measurement-samples.json) 记录了三会话、30 秒、10 Hz 的正式包运行。该轮 driver SHA-256 为 `7b03159731350c5752a36bb0d7c13128e39e6c9026eaef484b1e2df4131a851b`，source executable SHA-256 为 `b73c3b3e889726fa1c8bba599ede7242250e9f0754716e25fddfc23fc5c261d2`，source code SHA-256 为 `6dfc02b9394b864555e8a72aa6650bc216f9e878b8cf6383818556c24ccf9986`。fixture 三个请求均启动并保持并发，controller/process collector 共 `152` 个样本、`0` 个错误；轮次最终因 `WaitDeadlineExceeded` 失败，退出码为 `0`，清理完成。

- session-1 的 click 为 `331.663 ms`，第一次 AX 查询在 `304.426 ms` 内成功返回 `2581 bytes`，own marker 和 tail 均可见。这说明同一驱动、同一窗口和同一查询形状在本轮能够读回流式 tail。
- session-2 的 click 为 `349.554 ms`；随后 `29` 次查询全部以 `success=true` 返回，单次耗时为 `270.302–1269.303 ms`，返回文本长度为 `0–1794 bytes`。29 次均为 `tail_visible=false`、`own_marker_visible=false`，foreign marker/tail 数组为空，没有 `ax_timeout`；累计 `18360.439 ms` 后由严格 `18000 ms` 外层 deadline 失败。换句话说，最终 18 秒不是某一个 AX 调用持续阻塞，而是反复读到不含当前内容的响应后耗尽预算。
- [failure-window.png](../../output/acceptance/native-streaming/round-15-selection-fixed/captures/failure-window.png) 的采集画面中间会话已高亮，窗口正文区域清楚显示 `NATIVE_STREAM_TAIL_71f16d32a65b60ab` 多次，底部时间戳和 copy 控件也在视口内；`controller-last.json` 同时记录了 session-2 的 assistant 文本包含该 tail。manifest 将视觉审批标记为 `NOT CLAIMED`，所以这张图只作为保留的像素证据，不作为产品视觉验收结论。

这轮证据排除了“目标 tail 在失败时尚未绘制”这一解释，但没有证明当前 AppleScript 的 `table 1`、`last row` 就是 conversation 的逻辑末行，也没有证明 tail 位于该 row 的直接 `AXStaticText` 子节点。截图与查询不是同一原子时刻，不能单凭截图证明查询时 AX cache 已发布同一内容。round-15 因此把 H4b 的重点从“AX 调用是否持续阻塞”进一步移到以下可观测分界：

1. **table/row 选择。** 从 window 递归列出全部 `AXTable` 的 object path，读取每个 table 的 `AXRows`、`AXVisibleRows`、`AXSelectedRows` 和 `AXRowCount`；对每个 row 记录 `AXIndex`，再和进程内的 `PosInSet`、`SetSize`、`AccessOffscreen` 对齐。不能在 probe 之前把 `table 1` 写成必然错误，也不能把它写成已确认正确。
2. **文本遍历路径。** 对同一 conversation row 分别记录 row 的 `AXLabel`/`AXValue`、直接 static-text children、递归 static-text descendants 以及 nested table cell 文本。`accessElement` 对 `RoleText` 会通过 `accessInline` 合并普通 inline text；Markdown 表格、列表和代码块还会保留嵌套 role，因此截图可见不等于 `static text of last row` 会访问到该文本。
3. **cache 与对象 identity。** 在 action、下一次 `SurfaceFrame`、`UpdateAccessibility` publish 和第一次含 tail 的 tree 之间记录单调时钟、frame 版本和 AX object identity。源码允许同一 `AccessNode.ID` 的 ObjC 对象动态更新 role/subrole/parent/children；已不在当前 tree 的 ID 则会销毁。需要运行时记录才能判断 session 切换中是否出现了实际复用或旧 cache 读取。

round-15 之后，只有在进程内 tree 已确认包含逻辑末行且 row/text path 稳定，而外部 direct-row 仍快速返回旧值或超时，才应把优先级转给 System Events、AX server 或 IPC；若进程内 row、anchor 或 cache 尚未包含末行，应继续定位列表状态和 frame publish 时序。

### round-16-ax-role-tree

[manifest](../../output/acceptance/native-streaming/round-16-ax-role-tree/manifest.json)、[results](../../output/acceptance/native-streaming/round-16-ax-role-tree/results.json) 和 [measurement samples](../../output/acceptance/native-streaming/round-16-ax-role-tree/measurement-samples.json) 是与 round-15 相同 driver/source executable/source code hash 的三会话、30 秒、10 Hz 运行。session-1 click 为 `309.703 ms`；前 `28` 次查询成功返回但没有 tail，第 `29` 次为 `ax_timeout`，总 `18318.919 ms` 后以 `deadline_exceeded` 结束。该轮 manifest 的主错误明确包含最后一次 `osascript` 超时；source executable、原始包和运行后 source files 均未被 QA 过程修改，collector 有 `161` 个样本、`0` 个错误。

本轮新增的 [failure-ax-role-tree.json](../../output/acceptance/native-streaming/round-16-ax-role-tree/failure-ax-role-tree.json) 和 [ax-tree-diagnostic.json](../../output/acceptance/native-streaming/round-16-ax-role-tree/ax-tree-diagnostic.json) 必须按诊断证据理解：状态为 `DIAGNOSTIC_ONLY`，阶段是“失败查询和截图之后、清理之前”，遍历节点数为 `823`，`raw_text_persisted=false`，并明确提示 streaming 期间值可能继续变化；它没有改写 round-16 的主失败断言。

在这次失败后的快照中，探针观察到：

- message `AXTable` 位于 path `[0,14]`，ancestors 为 `AXWindow`，`row_count=2`、`rows_length=2`、`visible_rows_length=1`；其第二行 `array_index=1`，`AXIndex=1`。这与“外层 message table 有两行，当前末行是 row 1”的结构一致。
- 目标 tail `NATIVE_STREAM_TAIL_b1bfafcda0662c11` 出现在该末行的 `19` 个直接 `AXStaticText` 路径中，包括 `[0,14,1,37]`、`[0,14,1,39]`、`[0,14,1,43]` 等。message row 内还发现四个 nested Markdown table，路径分别为 `[0,14,1,14,0]`、`[0,14,1,29,0]`、`[0,14,1,44,0]`、`[0,14,1,59,0]`。
- 同一遍历记录 `823` 个节点，其中 `AXStaticText=71`、`AXTable=5`、`AXRow=9`，另有 `unknown=680`。属性错误为 `AXRole:-25202=2090`、`AXChildren:-25202=681`、`AXChildren:-25212=106`；这些是探针在遍历期间看到的 AX 元素失效/销毁错误，不是一个已证明的应用层异常码映射。

事实和推断需要分开：正确的 `[0,14]` table、`AXIndex=1` row 和直接 static-text token 说明在诊断快照中，query 需要的对象路径确实存在；它降低了“table/row 永远选错”或“tail 永远不在直接 static text”作为唯一解释的优先级。另一方面，role-tree 是失败之后才读取的，未保存失败查询当时的 cache/tree 版本、AX object identity 或逐次 query 返回的 tree；`unknown` 和 `-25202` 错误只证明诊断 traversal 遇到失效元素，不能单独证明这正是 29 次查询漏读 tail 的原因。要建立因果链，必须在 query 同时记录 tree publish 版本、对象 identity 和元素失效时间，或在应用主线程和外部 AX 查询之间做同一状态的同步快照。

### round-17-parsed-prefix

round-17 使用 root 实施的 parsed-prefix/cache 处理：对流式 append 保留已经完成的 parsed prefix，最新解析尚未完成时不再让整棵 native tree 在 raw text 与 Markdown 结构之间交替；消息 replacement 会丢弃不相关 prefix，旧 completion 到达后发布可用 prefix 并继续排队最新 source。当前实现对应 [`renderTextBlocks`](../../internal/desktop/markdown.go#L353-L370) 和 [`markdownDocumentForRender/parseMarkdownAsync`](../../internal/desktop/markdown.go#L372-L453)。配套的 [`TestMarkdownAppendKeepsParsedBodyWhileCompletionWaits`](../../internal/desktop/markdown_test.go#L185-L238) 覆盖 append 等待、旧 completion 发布有效 prefix、最新 completion 接续以及 replacement 清理旧 prefix 这条真实 `renderTextBlocks` 队列路径；该测试是这次 red/green 修复边界的直接记录。

[review](../../output/acceptance/native-streaming/round-17-parsed-prefix/review.json) 将该轮标为 `PASS_FOR_RECORDED_SCOPE`。[manifest](../../output/acceptance/native-streaming/round-17-parsed-prefix/manifest.json) 记录 source executable SHA-256 `b02f442a7c05ec7c6a8db313b691a0a44d914513525c314827c041fe9fa23b4b`、source code SHA-256 `45f943c874bbabc248878cf99477f2213b310684aeab59621a8cf5dc12e2c408`，三会话、30 秒、10 Hz；三个 fixture stream 均完整收到 `300` chunks，峰值并发为 `3`，正常退出码为 `0`。运行期间 source files unchanged，collector 有 `212` 个样本（`177` controller、`35` process）、`0` 个错误。

[measurement-samples.json](../../output/acceptance/native-streaming/round-17-parsed-prefix/measurement-samples.json) 中四次 AX tail query 都是一次尝试并成功读到各自 token：session-1 streaming `399.204 ms`、session-2 streaming `360.520 ms`、session-3 streaming `297.723 ms`、session-1 before-scroll `413.032 ms`。四次均为 `tail_visible=true`、无 error；这里的数值是单次 query duration，click 时间另行记录，不能把 query duration 与完整 sample 的 `latency_ms` 混用。

round-17 是对 round-15/16 失败形状的有效干预复验：在相同三会话流式验收方向下，四次 AX query 不再出现连续 stale/missing 读取，也没有发生 query timeout；本轮没有重复执行 round-16 的 role-tree probe，所以不能据此断言失效 role/children 错误已经消失。它支持 parsed-prefix 保留和 completion queue 的时序稳定化参与了解决问题，但无法仅凭一次通过隔离出“prefix 保留”“completion 排队”或其他 timing 变化各自的因果贡献。review 仍标记 visual approval 为 `NOT_CLAIMED`、real API 为 `NOT_TESTED`，因此当前主题原生流式验收保持 pending，不能把 `PASS_FOR_RECORDED_SCOPE` 写成整体验收完成。

### round-18/19-integrated-final-completion

round-18 和 round-19 是同一正式 native bundle、同一历史冻结 driver 的两轮已有运行记录；本文只读核对既有 artifacts，没有把本次文档更新冒充成新的原生重跑。两轮的 [review](../../output/acceptance/native-streaming/round-18-integrated-final-completion/review.json) 和 [review](../../output/acceptance/native-streaming/round-19-integrated-final-confirmation/review.json) 均为 `PASS_FOR_RECORDED_SCOPE`。两轮 artifacts 绑定的 driver SHA 是 `3a2377dd9b5881841c3f88fea26a916350e9f785e406510aadba02db02c1419b`；本次只读核对时工作树中的 driver SHA 已是 `88a08fdcfb35cec5111b0d16938c117891c6ad0982e095803cfda48226ec2624`，因此不能把 round-18/19 结果归属于当前新 driver，也没有为新 driver 进行原生重跑。

两轮共同核验到：driver SHA-256 为 `3a2377dd9b5881841c3f88fea26a916350e9f785e406510aadba02db02c1419b`；832 个源码文件已核对，source fingerprint 均为 `32e08f88522775e0cfb2568393ea93371b773d31222191f4e8533b2792f13a67`，运行记录中的 `source_changes=[]`；正式包 executable SHA-256 为 `db479d18dbde05705cf7bfced02c8ab6164f650d1f4bf3e28e0d481a76eeb216`，`__TEXT,__text` code SHA-256 为 `40d66e826fe9117eaaee6be0cc4757831221d84c0463af621412348616ca8b8e`，隔离包的代码和资源核对通过。两轮 manifest 也都记录运行前后源码和原始正式包未变化。

两轮各有三个 session 共享同一项目目录，fixture 为 30 秒、10 Hz。`stream-chunks.json` 各有 900 个事件，每个 marker 的 chunk index 都是完整的 `1..300`；provider records 均为 `chunk_count=300`、`complete=true`、`disconnected=false`，峰值并发为 `3`。这把 round-17 的“仅 tail 读取”范围与完整的三会话 final completion 范围区分开来。

最终 controller snapshot 在两轮均显示三个 session `idle`、`active_run=false`，各自拥有自己的 user/assistant marker 和独立 run ID，没有 foreign marker 或 foreign tail token。每个 session 的 journal 都是 3 个事件（user、assistant、run_end），run 归属断言均为 `PASS`。两轮 `results.json` 的 `final_completion` 都是 `record_count=3`、`expected_record_count=3`、`assertion=PASS`；三个 final completion record 均为 `status=PASS`、一次 AX attempt、own marker 和 own completion token 可见、foreign marker/completion token 为空。

round-18 的三个 final token 是 `NATIVE_STREAM_COMPLETE_117c68279d93b407`、`NATIVE_STREAM_COMPLETE_a13b022b772f1adb`、`NATIVE_STREAM_COMPLETE_5afcf1ab1458d298`，三者唯一；其 AX query duration 为 `574.634–605.753 ms`。round-19 的三个 final token 是 `NATIVE_STREAM_COMPLETE_be645f5978cc69e0`、`NATIVE_STREAM_COMPLETE_6290e359ece0fe71`、`NATIVE_STREAM_COMPLETE_4c6b7e8af7eb2051`，三者唯一；其 AX query duration 为 `572.647–610.005 ms`。这些是 `attempt.duration_ms` 的单次 query 时间，click 时间另行记录；两轮均保留 `12s` 单次 AX 上限、click 后开始的 `18s` query outer deadline 以及 `min(12s, remaining)` 预算语义。

每轮还保留四次 streaming tail AX 读取（session-1/2/3 和 before-scroll）以及三次 final completion AX 读取，七次均为一次尝试。两轮正常退出码均为 `0`；diagnostic collector 均为 228 个样本（190 controller、38 process）、0 errors；QA 进程、fixture listener 和隔离凭据均完成清理。final window 图片已由 root 打开复核，但两轮都标记 `visual_approval=NOT_CLAIMED`；real API、FPS/frame time 和 scroll effect/reading position 仍未测试或声明。因此这两轮可以确认 driver 记录范围内的 final completion 闭环，不能把它们写成完整视觉或产品验收，也不能单独证明 H4b 的根因。

## 可证伪预测

1. **历史总数的平坦性。** 固定窗口、当前 viewport、可见 row 文本和 layout 状态，只增加更早历史和 `SetSize`。使用当前生产 `ui.List` 时，进程内 `TestAccessibility` 的节点数和 traversal 时间应主要由可见/overscan 行决定，随总行数增长保持近似平台；若随总数线性增长，则说明存在未预期的全量构建/遍历路径，H4a 重新升高。

2. **可见正文长度的敏感性。** 固定历史总数，只增加当前可见流式消息的 Markdown/CJK/emoji/代码长度。frame build/layout/paint、CoreText shaping 和 `UpdateAccessibility` 可能增加；AX getter 不应因此主动遍历不可见历史。若只有外部 AX 时间增长而进程内 frame/access traversal 不变，应优先怀疑 IPC/query shape；若两者随正文长度同步增长，应继续分析主线程布局与 AX 更新竞争。

3. **应用内和外部双通道分离。** 在同一个 app 状态下，用 `RunOnMain(TestAccessibility)` 记录应用内 depth-first traversal，再用 QA 驱动 `osascript` 记录 `last row` 查询。若应用内 traversal 稳定而外部调用超时，H4b 的 IPC/System Events/AX server 侧证据增强；若两者同时变慢，主线程 frame/layout/cache 更新竞争更可信；二者都快而仅完整树慢，则查询形状占主导。

4. **缓存 getter 的重复调用。** AX 已开启且没有新 frame 时，重复执行单元素 getter 或 `TestAccessibility` 不应触发新的 layout；重复成本应来自缓存 tree traversal 和 IPC。若无 frame 仍能观察到 layout/CoreText，说明还有未映射的 callback 或测量边界错误。

5. **并发会话的分界。** 单会话、两会话和三会话分别测量 in-process traversal、外部 last-row 和完整 tree。若前两者应用内稳定、三会话只在外部 AX 超时，并且独立 collector 的 controller/process 样本无错误，则 AX IPC 或主线程竞争优先级上升；这仍不能单独证明生产代码根因。

6. **查询形状对照。** 比较外部 `entire contents`、现有 `last row`、单 row 直接 static-text 和 in-process `TestAccessibility`。若只有完整外部遍历超时，说明 query shape/IPC materialization 占主导；若 `last row` 也超时且 in-process 同步变慢，说明应用主线程或缓存更新路径需要继续 profile。

7. **AX row 元数据。** 对同一 frame 记录外层每个 `AXTable` 的 identity、`AXRows` 数量、table 的 `AXRowCount`、row 的 `AXIndex`、`AXVisibleRows` 成员关系，以及子节点 role/label；同时保留进程内 `PosInSet`、`SetSize` 和 `AccessOffscreen`。若 query 返回的 `last row` 的 `PosInSet < SetSize`，而当前逻辑末行未在 `AXRows` 中，tail 缺失首先是虚拟化/anchor 选择问题；只有在末行已在 tree 中仍缺失时，才继续怀疑文本节点或 getter。

8. **session 切换前后状态。** 在每次外部 query 前后于 UI 线程记录对应 session 的 `Visible()`、`AtEnd()`、`anchor`、`anchorKey`、`following` 和当前 row count。若无 tail 总与 `AtEnd=false`、可见范围不含末行或 anchor 未回末尾重合，说明 `last row` 选择受列表状态影响；若这些状态正确仍无 tail，再转向 cache publish/query shape。

9. **action 到 cache publish 的时序。** 分别记录 AX action 返回、下一次 `SurfaceFrame` 开始/结束、`UpdateAccessibility` 发布和首次包含 tail 的 cache 版本。若无 tail 的快速 getter 总发生在下一帧 publish 之前，说明读取旧 cache 足以解释该失败；若 publish 已完成仍无 tail，需检查 row/tree 形状。

10. **pending 与 parsed 对照。** 让同一条消息在 pending 和 parsed 两种状态下保持相同 tail，比较 row label、`RoleText` 数量、nested children 和 tail 所在节点。若 tail 只在某种状态下不在当前 static-text 路径，问题是 Markdown AX 结构与 query shape 的组合，而不是历史全量排版。

11. **table identity 对照。** 遍历并标记所有 `AXTable`，验证 `table 1` 是否确实是外层 conversation list；若 Markdown table 先于 conversation table 或被 query 选中，当前 AppleScript 的 table 编号假设即已失效。

## 最小 native probe 设计（未执行）

1. **进程内树快照。** 在真实窗口的主线程调用现有 [`TestAccessibility`](../../third_party/mygo/internal/darwin/testhooks.go#L568-L616)，并用独立的原生 AX attribute walker 补录 `AXRows`、table `AXRowCount`、row `AXIndex`、`AXVisibleRows` 成员关系、每层 role/label/value 和 object path；应用内 tree 同时保留 `PosInSet`、`SetSize`、`AccessOffscreen`。现有 `TestAccessibility` 只返回 role、subrole、label、value、frame、focus、depth、children，不包含这些 row 元数据，不能从现有输出反推出它们。round-16 的 walker 已证明失败后快照可以看到正确 row/text，但仍需把 publish 版本和 object identity 带到 query 时刻，才能超越诊断观察。
2. **列表状态快照。** 只在 probe 中读取每个 session 的 `ListState.Visible()`/`AtEnd()`，并把 `anchor`、`anchorKey`、`following`、`req` 和 frame 序号与 AX tree 快照关联；session ID 必须与 `conversation.go` 的独立 `ListState` 一一对应。
3. **缓存时序快照。** 将 AX action、`SurfaceFrame`、`runFrame` 阶段、`UpdateAccessibility` 和包含 tail 的 tree 版本写入同一单调时钟。probe 只做观测，不把外部 query 的返回内容当成 frame 已更新的证明。
4. **查询形状矩阵。** 在同一状态按顺序比较外部 `last row`、已确认 identity 的直接 row static-text、外层 table rows 和完整 tree，并保留每次返回的 bytes、`missing value` 数量、tail 可见性和 timeout 分类。每次只改变查询形状或 session 数量一个变量。
5. **停止条件。** 若 in-process tree 已包含当前末行且 row/text path 稳定，而外部 direct-row 仍快速返回旧值或超时，再把优先级转给 System Events/AX server/IPC；若 in-process 的 row/anchor/cache 先失败，则继续定位应用 frame 和 AX cache 更新竞争。

## 可复用 native measurement 入口

| 入口 | 可测内容 | 限制 |
| --- | --- | --- |
| [`ax`](../verification/scripts/native-streaming-qa.py#L138-L163) | 绑定指定 QA PID 的 `osascript` 调用，记录 subprocess timeout | 聚合了 IPC、System Events、AppKit/AX 等待和应用响应，不能拆解各段 |
| [`read_ax_last_message_row`](../verification/scripts/native-streaming-qa.py#L893-L910) | 当前 driver 的小范围 `last row` direct static-text 查询 | 仍是外部 AX；不代表完整树、全历史或稳定性能 |
| [`read_session_ax`](../verification/scripts/native-streaming-qa.py#L936-L1059) | click、每次 attempt、remaining budget、effective timeout、错误类型、tail 可见性及前后 controller/fixture context | click 在 18 秒 tail-query deadline 建立前执行；总耗时不是统一 18 秒硬截止 |
| [`DiagnosticCollector`](../verification/scripts/native-streaming-qa.py#L1062-L1203) | 与同步 AX 调用分离的 controller 约 200 ms、进程约 1 s 采样 | 只是 observation-only；无 frame time、GPU、AX server 分段或性能门槛 |
| [`TestAccessibility`](../../third_party/mygo/internal/darwin/testhooks.go#L568-L616) | 进程内 depth-first 原生 AX tree：role、label、value、frame、focus、children | 需要在应用主线程调用；绕过外部 System Events IPC |
| [`TestAccessibilityAttribute`](../../third_party/mygo/internal/darwin/testhooks.go#L649-L670) | 先 traversal，再读单元素旧式 AX attribute | 同样不提供外部 IPC 时间 |
| [`TestObserve`](../../third_party/mygo/internal/darwin/testhooks.go#L672-L703) | 对 surface frame 和每个 AX element value 建立 KVO | 当前入口不记录耗时，只能观察值变化 |
| [`RunOnMain` wrapper](../../third_party/mygo/internal/e2e/hooks_darwin_test.go#L186-L217) | 在主线程调用上述 testhook，适合构造 in-process 样本 | 本轮未运行 e2e；需要真实桌面环境 |
| [`AccessTree/list` tests](../../third_party/mygo/ui/access_test.go#L27-L173)、[`list_test`](../../third_party/mygo/ui/list_test.go#L437-L450) | 离屏验证 AX tree 建立、列表虚拟化、`SetSize`、`ScrollIntoView` | 不经过 AppKit、原生窗口、System Events 或 `osascript`；本轮未运行 |

## 限制与完成边界

- 当前生产路径使用 `ui.List` 的结论来自静态源码；它不替代一次 real-window/in-process node-count measurement。
- round-5 的 Go pclntab 映射来自正式 binary 和历史 OS sample，没有重新构建；样本是瞬时 profile，且 profiler 自身有 overhead。它支持“观测时主线程处于 frame/layout/CoreText caller nesting”，不支持按持续时间积分，也不支持把 CoreText 唯一归因给 AX。
- 外部 AX timeout 仍可能来自 `osascript`、System Events traversal、AppKit AX server、应用主线程等待或 query shape；当前证据没有拆开这些时间。
- 本次文档编辑没有启动窗口、运行测试、构建或修改生产/驱动/测试；文中引用的 round-15/16/17/18/19 运行均来自已有 artifacts，本文没有新增运行时闭环，也没有宣布 H4b 或整体性能通过。
- round-16 的 role-tree 结果是在失败后读取，存在 `unknown` role、`AXRole/AXChildren` 失效错误和未持久化原始文本；它可以证明该时刻的 AX traversal 观察，不可以单独证明失败查询当时的对象状态或因果链。
- round-17 只证明 `PASS_FOR_RECORDED_SCOPE`：视觉审批未声明，real API 未测试，完整用户主题验收仍 pending。
- round-18/19 将 final completion 纳入了双轮记录范围，仍只证明 `PASS_FOR_RECORDED_SCOPE`：视觉审批未声明，real API、FPS/frame time 和 scroll effect/reading position 未测试或未声明；两轮不能替代完整产品验收或 H4b 因果证据。
- 后续若需要继续验证，应在独立 probe 中同时记录 `RunOnMain(TestAccessibility)`、单元素 getter、外部 `last row` 和完整 tree，固定 viewport 与输入规模，按一次只改变一个变量的方式对照；不应仅凭新的 `osascript` 总耗时修改 H4 结论。
