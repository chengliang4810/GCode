# 对话滚动卡顿调查

2026-11-01。对象：会话 `geiesphxcoqt`（190 条消息、840 KB 块文本、7.1 MB 会话文件，
投影为 4 个展示条目、39 行）。参考实现为只读的 `D:/zeron`（ab18915）。

## 结论及证据强度

三个独立因素，量级依次递减：

1. **`transcriptEntries` 的 O(n²) 合并（已修）**。每合并一条续写消息，就把已累计的
   全部 block 重新分配并拷贝一遍；190 条消息的会话每次调用分配 **5.4 MB / 187 次**，
   而它每帧都跑。滚动时它占全部堆分配的 **84%**。
   改为「首次合并复制一次、之后原地 append」后为 **171 KB / 8 次**（32×↓）。
2. **平滑滚动把一次滚轮格放大成约 20 次整窗重建**。MyGo 的 `wheelAnimation`
   是 150 ms ease-out 滑行（`third_party/mygo/ui/wheel.go:9`），滑行期间每帧
   `rt.animating = true`，整棵视图重建。组件测试里 30 格滚轮产生 **628 帧**，
   关闭平滑滚动同样 30 格只有 **30 帧**。
3. **列表按「消息」虚拟化，不是按行**。参考实现一行一个块，`OVERDRAW_PX = 320`；
   GCode 的 `ui.List` 以展示条目为项，一个合并后的助手回合（本例 39 行）每帧整段重建。
   实测每帧成本与视口高度几乎无关（400/880/1200 DIP 下 1.55/1.68/1.78 ms），
   说明成本是「每条消息的投影 + 元素构建」，不是可见像素。

证据强度：以上是 Windows 上 `ui.Tester` 内存绘制 + `MYGO_FRAME_STATS` + pprof 的
组件数据，不是当前用户窗口的 FPS、DWM 合成或输入延迟。没有把内存绘制时长当成
真机 A/B 结论；真机仍需一次带帧统计的复现。

## 测量

| 项 | 数值 |
| --- | --- |
| `transcriptEntries`（190 条消息） | 修前 5,403,361 B / 187 allocs，修后 170,726 B / 8 allocs |
| 空闲帧，1320×880，该会话 | 修前 6,228 KB / 2.19 ms，修后 1,091 KB / 1.35 ms |
| 空闲帧，小会话（8 条消息） | 67 KB / 0.22 ms |
| 空闲帧 vs 视口高度 | 400 DIP 1.55 ms、880 DIP 1.68 ms、1200 DIP 1.78 ms |
| 一次滚轮格（平滑开） | 19–21 帧，约 4.5 ms/帧 |
| 一次滚轮格（平滑关） | 1 帧 |
| 30 格滚轮 | 平滑开 628 帧 / 2.80 s，平滑关 30 帧 / 0.13 s |

修前几乎每帧日志都有 `GC: 1 cycles ended`，修后 GC 不再逐帧发生。

## 参考实现（zeron）

`crates/ui/src/transcript.rs` 的注释即设计说明：

- **一行一个块**：`{msgId}#{partId}.{blockIx}`（助手）与 `{msgId}#g{groupIx}`
  （工具组），列表项即块行，`ListState::new(..., px(OVERDRAW_PX))`，`OVERDRAW_PX = 320`。
  流式回复只重建可见尾部，成本与回复长度无关（`transcript.rs:1-20`）。
- **行缓存按内容指纹**：只有变化的行重建（`transcript.rs:13`、`rows_for_entry`），
  完成后整行走 settled cache，不再解析（`transcript.rs:2047`）。
- **活动行高度解析式**（`CHIP_HEIGHT` 等），折叠不需要测量（`transcript.rs:96`）。
- **滚轮不做补间**：`on_scroll_wheel` 只处理「脱离底部」逻辑（`transcript.rs:4323-4338`），
  列表按事件即时滚动；平滑跟随只用于 stick-to-bottom 的弹簧（`transcript.rs:19-25`）。

GCode 对应差异：`internal/desktop/chat.go:56` 以消息为列表项、
`internal/desktop/transcript.go:28` 一次构建条目的全部行、
`internal/desktop/transcript_model.go:22` 每帧重投影。

## 已改

- `internal/desktop/transcript_model.go`：`transcriptEntries` 每次合并只复制一次
  block 切片，之后原地增长；回归测试
  `TestTranscriptEntriesJoinsLongTurnInOnePass`（190 条消息 ≤16 次分配）。
- `internal/desktop/transcript_model.go`：`transcriptCache` 按快照 revision 与内容
  指纹缓存「续写合并后的条目」和「每条目的行投影」，滚动帧不再重算；
  `TestTranscriptCacheReusesRowsUntilContentChanges` 覆盖复用、流式增量失效和
  compact 形态切换。效果（同一探针）：空闲帧分配 1,093 → 661 KB，GC 次数
  120 帧 13 → 7 次；帧时间 1.68 → 1.59 ms（Tester 内存绘制）。

## 已改（滚轮）

`SmoothScrolling` 已删除（设置项、字段、默认值、fixture 与测试引用一并移除）：

- `internal/desktop/motion.go` 恒传 `SmoothScrolling: false`，滚轮格在收到它的那一帧
  直接滚动，与参考实现的 `List::scroll_by` 一致。
- `internal/desktop/transcript_motion.go` 的跟尾动画不再看这个设置，只由
  「减少动态效果」决定：流式增长时尾部仍然平滑跟随，滚轮则即时。

实测（同一探针，190 条消息的真实会话）：40 格滚轮 **0 个额外帧 / 221 ms**
（此前 20 格滚轮 418 帧 / 1.21 s）。

## 已改（按块建行）

2026-11-01 第二项落地，与参考实现的 `{entry}#{blockIx}` 行模型对齐：

- `internal/desktop/transcript_model.go`：`conversationRow` 把一个条目摊成列表行——每个用户消息
  一行、助手条目每个投影块一行、没有块的助手条目一行（只带页脚）。行自带 `TopGap`，
  按 `top_gap_for` 的同一套规则：跨条目 16、工具组相邻 12、同一条目的相邻块 8。
  条目的行投影按内容指纹缓存（`transcriptCache.rowsFor`），流式 token 只重投影变化的那一条。
- `internal/desktop/chat.go`、`transcript.go`：`ui.List` 的项从「消息」改为「行」，
  `transcriptRowView` 只构建一行的内容；条目的时间戳与「Copy message」挂在它的**最后一行**。
- 选择域：每条消息原先各自 `Selectable()`，拆行后无法跨行。现在整个列表一个选择域
  （`transcriptScope`），跨块、跨消息的选区都在同一 `rt.texts` 序列里。
  `linkui.BeginSelection` 保留选区仍持有段落的 Markdown 源，未被构建的行也能 Markdown 复制。
- 悬停：行是独立列表项，`root.Hovered()` 不再覆盖整条消息。改为按行 `OnHover` 记录，
  条目任一行为悬停态即显示该条目最后一行的页脚（对应参考实现的 `hovered_entry`）。
- 查找：`conversationMatches` 返回**行**下标，`ScrollIntoView` 直接定位到命中块。
- 工作尾部：挂在整个列表最后一行（对应参考实现 trailer 在最后一行）。

框架侧（第 27 项 MyGo 补丁，`rcode-native-ui.19`）：文本选区改为自带 span 文本快照
（`textSpan{id, from, to, text}`）。列表只构建视口附近的行，滚出构建范围的行不再让
`syncTextSelection` 清空选区；绘制与复制都从 spans 取文本，拖动时按重叠段落合并
（对照 zeron `resolve_spans` / `extend_virtualized_drag`）。

证据（组件层）：
- `third_party/mygo/ui/textselection_list_test.go`：60 行列表内跨行选区，滚动到末尾
  （选中的行全部不在构建范围）后 Ctrl+C 仍得到 4 段全文；滚回后选区重新上色。
- `internal/desktop/transcript_rows_test.go`：行投影的键/间距/最后一行标记；
  60 块会话只构建视口内的行（首块未构建，滚到顶后末块不再构建）；
  跨行选区滚出构建范围后复制仍是 3 段全文；悬停第一行即可让条目页脚可见；
  查找命中返回块行下标。

待验收：以上是 `ui.Tester` 内存绘制下的组件证据，不是真实窗口的帧时、GPU 呈现或
像素对比；真实窗口滚动帧率与视觉一致性仍需单独验收。

真机 desktop-e2e（官方 automation 插件，改动后代码）：
`output/desktop-e2e/20261101-173322-3584585105` 一次跑通 `TestDesktopLocalTranscriptLinks`、
`TestDesktopLocalTranscriptFoldMotion`、`TestDesktopLocalMarkdownTables`、
`TestDesktopLocalSideChat`（各明/暗两外观）；更早一次
`output/desktop-e2e/20261101-135040-3790720148` 也跑通了前两项。
`TestDesktopLocalTranscriptActivity` 仍失败（两份外观同），断言为完成后思考条目
既不出现首行 `THOUGHT 000` 也不暴露尾部；`activity/final-tree.json` 里没有任何
`THOUGHT` 文本，只有 `Thought process: block-…`。该失败不是按块建行引入的：
改动前的同一套件 `output/desktop-e2e-preexisting/20261101-015232-1576964655`
（2026-11-01 01:52，早于本次 13:20 起的行投影改动）以同样信息失败，树里同样
没有 `THOUGHT`；`docs/mygo-patches.md` 也记录了升级时用升级前框架 A/B 复测同样失败。
按 AGENTS.md 记为遗留问题，需单独定位（与行虚拟化、选区跨度无关），这里不宣称已修复。

## 待决（需要产品决定，未擅自改）

1. **活动行解析高度**：`CHIP_HEIGHT` 一类常量在参考里避免折叠时测量，
   GCode 仍走 Collapsible 测量。
2. **每帧重建主题对象**：`markdown.DefaultTheme`/`editor.DefaultTheme`/
   `linkui.paragraphTheme` 每次构建都新建，占剩余分配的约三成。

## 复现

```sh
MYGO_FRAME_STATS=all go test ./internal/desktop -run TestZZFrameCost -count=1 -v
go test ./internal/desktop -run TestZZFrameCost -count=1 -cpuprofile /tmp/scroll.prof
go tool pprof -top -sample_index=alloc_space /tmp/scroll.mem
```

探针测试只用于本次调查，未提交：读取本机 `~/.gcode/sessions/<id>` 的真实会话，
用 `ui.Tester` 驱动 `Wheel`/`Advance`，因此只代表组件层。
