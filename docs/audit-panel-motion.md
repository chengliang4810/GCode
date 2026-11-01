# 面板开合与拖拽调整

2026-11-01 实现记录。参考为只读的 Zeron v0.2.128，固定提交
`ab189159f10cf8e63c0502be75a59bbc487e0eaf`，副本在
`output/reference-sources/zeron`。本文件记录源码对照、实现与检查；
参考窗口与原生窗口的并排视觉验收尚未进行。

## 参考的做法

| Source | Connected behavior |
| --- | --- |
| `shell.rs:1472-1512` `WidthTween` / `panel` | 每次开合都由渲染逐帧求值的宽度补间，不走 `with_animation`：元素树重新挂载不会重播。时长取 `PANEL_RESIZE`（180ms，`DIALOG_IN` 的曲线，即 CSS `ease` `cubic-bezier(.25,.1,.25,1)`），再按 `sqrt(距离/256)` 放大，夹在 1–4/3 倍，即 180–240ms。 |
| `shell.rs:3467` `sidebar_target` / `3515` `right_target` | 目标宽度来自当前 fit 与用户偏好；关闭时目标为 0。 |
| `shell.rs:3564`、`3630` | 点击开关时以**当前画出的宽度**为起点开补间；动画进行中再点一次从当前值反向，不会闪回。 |
| `shell.rs:7799-7851` `ease_toward` | 每帧把补间终点换成当帧目标，邻居变化时列仍落在正确位置；`reduced_motion` 直接返回目标。 |
| `panel_layout.rs:10` `panel_content_tween` | 开合期间被遮罩的列**保持内容布局宽度**：打开时按目标宽度布局，关闭时保持当前宽度，移动的只是遮罩边缘，内容不会每帧重排；两端都开着（重分栏）时内容跟随目标。 |
| `shell.rs:5168`、`5256` | 拖拽时清空补间，`on_*_drag` 直接跟指针；只有到达上下限才有 220ms/5px 的边缘回弹（`motion.rs:501-577`，GCode 的 `panel_resize.go` 已移植）。 |
| `shell.rs:3386` `observe_viewport_width` | 窗口尺寸变化是直接操作：清掉补间，列宽立即跟随，不落后于窗口边缘。 |
| `shell.rs:250-320` fit exit | 窗口变窄导致面板被 fit 隐藏时，也用同一个补间滑出（`:250-320` 的 `fit_exits`）。 |

## GCode 的实现

`internal/desktop/panel_tween.go`：

- `panelWidthTween`：180ms 基长、`ui.CubicBezier(.25,.1,.25,1)`、按距离开方放大且
  夹在 1–4/3 倍；`reduced` 时不起补间；每帧读当帧目标，终点随布局变化。
- `layoutPanels`：每帧用 settled allocation 求三列目标（关闭/被 fit 隐藏为 0），
  只在 `setPanel` / `toggleWorkspaceExpanded` 标记的开关上启动补间；第一帧按
  settled 宽度出现。正在关闭的列在补间结束前继续留在 fit 里，邻居随着它收缩而
  释放宽度。
- `panelContentWidth` 移植 `panel_content_tween`：打开按目标宽度、关闭保持当前
  宽度、两端都开时跟随目标；`panelContentBox` 用固定宽度包住移动中的列内容，
  由列的裁剪边缘揭示，避免每帧重排。补间期间用普通 Row + 固定宽度 Box 绘制
  三列，而不是 `ui.Split`：后者的夹取会让收起过程在最后 10 DIP 处留一条
  边线，静止后再换回 split 以保留分隔条、拖拽与键盘操作。
- 侧栏、右侧工作区、Files 停靠栏共用 `frameColumns`（`paintPanels`），标题栏和
  背景绘制读同一份列宽，因此开合时标题、标签条和壁纸切口一起移动。
- 工作区的 takeover（`rightExpanded`）也走右列的同一补间：宽度从手动宽度长到
  整列，结束时才切到 takeover 分支；关闭时反向。
- 拖拽仍直接跟指针：补间进行中不接收 split 的拖拽采样，静止后恢复
  `panel_resize.go` 的边缘回弹与 400ms 宽度保存。
- 窗口尺寸变化不补间：`layoutPanels` 每帧用新目标重算，未起补间的列立即落位。

与参考的差异：参考的 fit-exit（窗口变窄时被隐藏的列）也有补间；GCode 目前
在 fit 隐藏时直接落位。参考用 GPUI 的遮罩层与元素树，GCode 用裁剪 + 固定
内容宽度近似，三列在开合期间由手写行布局绘制，内容保持布局宽度而不是被
真实遮罩平移。

## 拖拽

参考的拖拽不做缓动：`on_sidebar_drag` / `on_right_pane_drag` 直接写宽度、清空补间，
再用 220ms/5px 的边缘脉冲收尾；命中区是 `PANE_RESIZE_HALF_WIDTH = 10`（20 DIP 宽）。

GCode 用 MyGo 的 `ui.Split`，同样直接跟指针并复用 `panel_resize.go` 的回弹。
在 `ui.Tester` 里逐步拖动并读取 `Divider` 的位置，正方向移动的分隔条与指针位置
完全一致（0px 误差），负方向到达 `SidebarMin` 后正确止住。

本轮修正的真实滞后：分隔条本身跟手，但旁边的列（对话、工作区、Files、composer）
原先按**上一帧**的侧栏宽度计算，拖动时它们总比接缝晚一帧。现在 `mainView` 在
侧栏显示时用 split 当帧的 `sidebarContentWidth` 重算 `frameColumns`，
`TestSidebarDragMovesColumnsInTheSameFrame` 锁住这一点。

框架层修正（`patches/mygo-v0.7.3-29-*`）：

- split 的拖拽命中区从主题的 1.5 DIP 线宽扩到 20 DIP（左右各约 10 DIP），
  与参考的 `PANE_RESIZE_HALF_WIDTH = 10` 一致；可视分隔线仍为 1 DIP。
- 拖拽样本在构建两侧窗格**之前**应用（`draggedBy`）：指针移动当帧就写入
  `*size`，窗格按新宽度布局，不再用 `Dragged()` 标记 `consumed` 触发第二次
  构建。`ui.Tester` 中每次 `Move` 的构建次数从 3 次降到 2 次，分隔条位置与
  指针完全一致（0px 误差）。第二次构建来自帧调度本身，属于即时模式重建整棵
  视图的固有成本；参考的 GPUI 是保留模式，只重排被拖的分栏。
- `Changed()` 现在只报告键盘方向键造成的移动（在窗格构建后经 `afterInput`
  生效），拖拽后的宽度由调用方在 split 返回后直接读取。GCode 的三处分栏
  （侧栏、对话/工作区、内容/Files）和上下分栏都改为比较 split 返回后的
  `*size`，不再依赖 `Changed()`。
- 侧栏键盘步进在下一帧开头采纳（`sidebarSplit` 区分“上一帧已画出的拖拽值”
  与“构建后才落下的方向键步进”），仍然只在静止后保存。

## 检查

- `go test ./internal/desktop -run TestSidebarDragMovesColumnsInTheSameFrame`：
  拖动侧栏时 `Conversation pane` 的宽度与当帧 `a.chatWidth` 一致，相邻列不再晚一帧。
- `go test ./internal/desktop -run TestPanelColumnsEaseOpenAndClose`：
  点击关闭后当帧不塌陷、内容宽度保持、90ms 时宽度在两端之间、300ms 后为 0；
  再打开 300ms 后回到侧栏最小宽度以上。
- `go test ./internal/desktop -run TestPanelToggleSnapsUnderReducedMotion`：
  `ReduceMotion` 下同一开关在一帧内落位，不启动补间。
- `go test ./internal/desktop -run 'TestFitPanels|TestPanelWidths|TestWorkspace|TestFitHiddenPanel|TestSidebarDrag|TestFilesWorkspace'`：
  既有列宽、fit、拖拽与接管测试保持通过（首帧按 settled 宽度出现，直接
  赋值面板标志的测试不经过补间）。

## 未验收

- 原生窗口中的开合节奏、侧栏内容是否在 180–240ms 内看起来像被“揭示”而不是
  缩放，尚未录制屏幕证据。
- 真实指针拖拽的手感与 split 边界回弹在原生窗口中的表现未验收。
- fit-exit 的补间未实现，窗口从宽变窄时被隐藏的列仍会直接消失。
