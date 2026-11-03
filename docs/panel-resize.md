# 面板拖拽手感

2026-11-01：用户反馈侧边栏拖拽"非常僵硬"，并要求参考 `D:/projects/zeron` 的
侧边栏拖拽与动效做法。本文记录参考行为、旧实现的问题、现在的实现和验证边界。

## 参考实现（D:/projects/zeron，只读）

| 方面 | 做法 | 依据 |
| --- | --- | --- |
| 拖拽跟随 | 宽度直接取指针位置，不做补间 | `crates/ui/src/shell.rs:5168`：`self.sidebar_tween = None; // live drag tracks the pointer directly` |
| 指针到宽度 | `motion::resize_drag_sample`：宽度夹在 min/max，越过边界时锁定是哪一侧 | `crates/ui/src/motion.rs:520`、`crates/ui/src/shell.rs:1248` |
| 触边回弹 | 越过 min/max 时播放一次 220 ms、5 px、0.32 分割点的双段 smoothstep 脉冲；`starts_bounce` 只在刚触边时为真，按住不放不会每帧重启 | `crates/ui/src/motion.rs:501-577`、`crates/ui/src/shell.rs:5169-5175` |
| 分割线反馈 | seam 高亮随悬停淡入淡出（`motion::hover_blend`），不是瞬间换色 | `crates/ui/src/shell.rs:11482` 起 `resize_handle` |
| 持久化 | `SavePolicy::Debounced`，`SAVE_DEBOUNCE_MS = 400`，拖拽期间不落盘 | `crates/ui/src/settings.rs:71`、`:866` |
| 直接操作期间 | 取消所有宽度补间，避免动画与指针打架 | `crates/ui/src/shell/panel_layout.rs:230-243` |

结论：参考实现的拖拽宽度本身没有动画，动画只在触边回弹和悬停反馈上；
手感来自"1:1 跟手 + 软边界 + 不落盘"。

## 旧实现的问题

`internal/desktop/app.go` 在侧栏、右栏、文件坞、终端四个 `ui.Split` 的
`split.Changed()` 里都直接调用 `a.saveSettings()`。于是每个指针移动帧都产生一条
`settings.update`：核心无条件走 `persistLocked`（`internal/engine/session_storage.go:157`），
对每个已加载会话整份 `MarshalIndent` + sha256，再把整份 `engine.json` 以 fsync 原子写回，
最后取三次全量快照回灌 UI 并触发整帧重建（`internal/desktop/app.go:241-262`）。

本次调查在本机 `~/.gcode` 上实测（Go `encoding/json`，best-of-5）：
`engine.json` 2.95 MB 的 `MarshalIndent` ≈ 17 ms、写入 + fsync ≈ 3–7 ms；
最大的已加载会话记录 5.34 MB 的 `MarshalIndent` ≈ 31 ms。
即一条只改宽度的 `settings.update` 单是持久化就约 50 ms 量级，而 60 fps 拖拽每秒产生约
60 条：单消费者队列（容量 128）以约 40 条/秒堆积，约 3 秒拖拽即触及上限，
`dispatch` 走 `default` 分支报 "an operation is still pending" 并丢弃那一次宽度变化。
这是"僵硬"的主因，不是缺少补间。

## 现在的实现

- `internal/desktop/panel_resize.go`：`splitResize` 为每个分割条保存边界锁存与回弹脉冲。
  `paint` 把回弹叠加到分割条在本帧显示的尺寸上并按时钟继续请求帧；
  `observe` 取回一条拖拽样本，把回弹减掉后再夹到 min/max，所以回弹只是画面，
  永不进入应用保存的宽度。参数与参考实现一致：220 ms、5 px、0.32 分割点。
  一旦开始的脉冲会走完，即使指针已回到范围内——中途截断会按剩余行程把接缝跳一下。
  面板隐藏时 `reset` 清掉锁存，重新打开后同一侧边界仍会回弹。
- `shownSidebarWidth`：回弹是真布局，`mainView`、标题栏与侧栏对话列都按
  「本帧显示的侧栏宽度」分配列宽，接缝与相邻面板一起移动，不会出现两套宽度。
- `internal/desktop/app.go`：四个分割条都改为
  `paint` 取显示尺寸、`observe` 取回真实宽度、`schedulePaneSave(c, paneXxx)` 记录待保存。
  `paneWidthSaveDelay` 为 400 ms（参考实现同值）：拖拽停稳后写一次设置，
  拖拽过程中只改内存里的宽度与 `state.Settings`。
- `flushPaneSave` 由每帧 `View` 末尾驱动，用 `c.After` 排到到期时刻；到期时
  `applyPaneWidths` 只把**拖过的**面板宽度（`paneDirty` 位）补回设置再 `saveSettings()`：
  避免拖拽期间到达的快照把宽度覆盖回旧值，也避免用开窗时读到的旧值覆盖别的
  界面改过的未拖拽面板。
- `Close()` 在取消 worker 之前写一次仍在防抖窗口内的宽度，用 1 s 上限的独立
  context，远端宿主不可达时不会卡住关窗。

## 验证

`internal/desktop/panel_resize_test.go`：

- `TestResizeBounceOffsetPulsesAtTheEdge`：脉冲从 0 出发、在 0.32 处达到 5 px、回到 0，min 边反向。
- `TestSplitResizeKeepsTheWidthInsideItsLimits`：越界宽度夹到边界；同一边界按住不重启脉冲；
  回到范围内接缝立即跟手，已在播放的脉冲走完再停。
- `TestSplitResizeBouncesAgainAfterReset`：面板隐藏再打开后，同一侧边界仍会回弹。
- `TestPaneSaveKeepsWidthsTheUserDidNotDrag`：拖侧栏落盘时不会把文件坞宽度改写成开窗时的旧值。
- `TestSidebarDragSavesOnceTheDragRests`：连续三次 `Move` 期间 `settings.update` 为 0 次，防抖到期后恰好 1 次且携带停稳后的宽度。
- `TestSidebarDragBouncesPastItsMaximum`：越过上限后宽度保持 400，回弹期间显示宽度超过 400，脉冲结束后回到 400。
- 既有 `TestSidebarDragPersistsLogicalWidth` 继续通过（拖拽后 `a.sidebarWidth` 与偏好一致）。
- `go test ./internal/desktop`、`go vet ./internal/desktop` 通过；当时 `go test ./...` 仅
  `internal/agent` 的 Glob/Grep 用例因本机 PATH 无 `ripgrep` 失败，与本次改动无关；
  2026-11-01 搜索工具改为进程内实现后，这些用例在本机无 `ripgrep` 的环境下通过。

组件测试证明进程内的布局、状态和保存时序，不证明真实窗口的帧节奏、像素或手感。

## 未覆盖 / 待验收

- 分割线悬停渐隐需要改 `third_party/mygo/ui/split.go` 的绘制路径；MyGo 由
  `scripts/frameworkpin` 的补丁链与树指纹锁定，本轮未加补丁，分割线仍是即时换色。
- 真实窗口下的拖拽手感需要单独验收（本机真实数据、真实 GPU 呈现），
  组件测试与编译通过不能代替。

## 2026-11-03：收放期间保持全高与原宽度

用户反馈左右侧栏收放时，内容先挤在上方，动画结束后才撑到底部。
逐帧回归复现：1000 DIP 高窗口中的左侧栏在动画期间变为 746 DIP，
设置按钮从 Y=959 上移到 Y=705；右侧工作区和文件栏共用同一错误容器。
`panelContentBox` 原先只固定内容宽度，现同时使用 `FillHeight()`，
让可伸展列表及底栏在宽度裁切期间继续按窗口全高布局。

同时修正左侧栏重新展开后从 256 缩到 242 DIP 的问题。
开始展开时，上一轮关闭动画留下的临时宽度被当成键盘分隔条调整；
现在待执行的收放动画与正在播放的动画一样，不进入键盘宽度采纳分支。
真实键盘调整、指针拖拽和减少动效的原有行为继续由回归覆盖。

验证：

- `TestPanelToggleKeepsContentHeight` 覆盖三栏、深浅主题、900×600 与
  1600×1000 窗口，分别检查收起、展开的连续帧，以及左侧底部按钮的 Y 坐标。
  `TestPanelColumnsEaseOpenAndClose` 同时检查重新展开后恢复原宽度。
- 面板、侧栏拖拽及共享控件边界专项通过；Go vet、带 `e2e` tag 的测试驱动
  Go vet 和 MyGo vet 通过。
- `TestDesktopLocalPanelToggleHeight` 使用独立数据目录、真实本机核心与
  macOS 原生窗口，深浅主题下三栏的收起、展开均通过。
  采样检查中间状态的高度及底栏位置，保存动画期间和落位后的真实窗口截图。
  原生无障碍树返回未裁切的内容宽度，因此通过相邻列的接缝测量实际展开宽度。
  MyGo 会暂停被完全遮挡窗口的纯计时动画帧，测试通过插件发送无点击的指针移动，
  驱动采样和最终落位；不据此声称窗口被遮挡时的自然帧率或系统鼠标输入验收。
- 原生证据：`output/panel-toggle-height/20261103-153940-1310388422/`。
  早期失败记录保留为测试采样方式调整的过程证据。
- `CGO_ENABLED=0 go test ./...` 未全绿，完整日志为
  `output/panel-toggle-height/validation/all-tests.log`。
  其中四个桌面失败用例在 Go overlay 撤去本次两处业务修复后仍同样失败，
  对照日志为 `other-desktop-tests-before.log`，同目录保留 overlay 和修复前复现日志。
  其他包的失败涉及临时目录、Windows 路径断言、真实终端、壁纸监听及许可证校验；
  本次未扩展修改这些功能。

## 2026-11-03：拖过最小宽度关闭侧栏

左侧栏拖至最小宽度后继续向左拖，立即触发关闭；右侧工作区和 Files 栏
则继续向右拖关闭。恰好到达最小宽度仍保持打开，无需松开鼠标即可触发。
关闭复用 `setPanel` 的宽度动画并清除该分隔条的回弹状态；
减少动态效果时直接收起。保存的宽度保持最小值，重新展开时恢复该宽度。

`TestPanelDragPastMinimumCloses` 覆盖三栏、正常/减少动效、触边不关闭、
继续拖动关闭、相邻栏保持打开、宽度保存、重新打开和再次快速拖动关闭。
原有同帧跟随测试的左拖范围调整到最小宽度以内，越界关闭由新用例负责。
面板、侧栏拖拽与共享控件专项，以及 Go vet、E2E 驱动 Go vet 和 MyGo vet 均通过。

`TestDesktopLocalPanelDragToClose` 在私有数据目录、真实本机核心和 macOS 原生窗口中，
分别验证深浅主题下三栏触边、松开前关闭、相邻栏保留和重新展开到最小宽度。
展开完成条件同时核对内容宽度和相邻接缝，避免将动画中未裁切的无障碍内容宽度
当作实际可见宽度。最终原生证据与实窗截图位于
`output/panel-drag-close/20261103-173614-862132784/`；
早期测试等待调整的记录保留在同一上级目录。

桌面完整测试仍有四项失败，撤去本次三处关闭逻辑的 Go overlay 对照同样复现：
模型推理滑块、真实核心设置用例的旧 Apply 按钮、子智能体编辑行对齐、输出跨行复制。
日志与对照 overlay 位于 `output/panel-drag-close/validation/`。
本轮不将桌面完整测试记作通过，也不构成系统鼠标输入或视觉一致性验收。
