# 文件浏览器与桌面工作区逐项审查

最新R16已修非选中文件图片`.78`和无详情标签的错误标题提示；五门槛与四目标包
校验通过。计时原图和GPU控制实验见 `audit-tabstrip-file-tooltip.md`；Tooltip
位置/样式、浅色实际UI适配器与完整视觉仍待验收。下方旧路径和`.2`检查保留
各原始阶段范围；当前为`.6`。

审查日期：2026-10-28。事实源为只读 `D:/zeron` 的 **v0.2.128 / ab189159f10cf8e63c0502be75a59bbc487e0eaf**。
同时检查官方 `D:/projects/mygo` 的组件与 gallery（本次读取提交 `43199d0b0fb95fd0b71e5a225d2807c04ebcc609`），实际使用 API 再对仓库固定的 MyGo v0.6.4 源码确认。

本轮没有启动应用、安装器或 Computer Use，也没有重新构建发布包。此前 `build/coexist-qa-r2` 的包先保留，**它们不包含本轮 UI 修复**。

## 输入证据与版本边界

用户图 2 已原样保存为 [`reported-before.png`](../output/audit-workbench/reported-before.png)。图中可见：

1. 独立 Files 标题和搜索框位于聊天左边。
2. 浅色聊天旁的工作区为黑色。
3. 工具标签持续横排，末尾超出视口，没有独立关闭槽和受控溢出。

图中的 Files 标题、顶部多入口按钮与当前源码均存在结构差异。当前源码在本轮开始之前已经按 `sidebar | chat | tools | files` 排位，因此该图不能直接证明当前源码仍把文件树放在左侧。运行文件归属及构建戳由主任务核对；截图本身不包含足以唯一识别构建的标记。

本轮仍对当前源码建立实际渲染测试，独立发现并修复 system 主题误判、宽度分配、终端组合和标签溢出等问题。

## 源码对照入口

| 项目 | Zeron 事实源 | MyGo 组件依据 |
| --- | --- | --- |
| 动态列显隐和优先级 | `crates/ui/src/shell/panel_layout.rs` 的 `horizontal_panel_fit` | 应用侧纯布局函数；`ui.Split` 负责原生拖动/键盘分隔条 |
| 文件栏先让出偏好宽度 | `crates/ui/src/shell/files_panel.rs` 的 `files_panel_layout`、`files_width_for` | 以分隔线 1 DIP 计入列宽；不把窗口挤压结果写回偏好 |
| 工具栏宽度与 takeover | `shell.rs` 的 `right_target_for_columns`、`right_pane_takeover_width` | 展开时移除聊天/工具拖动分隔线 |
| 底部终端所属范围 | `shell.rs` 的 `render_main` / `render_terminal_container` 与并列的 right/files | `ui.SplitVertical` 放入聊天列内部 |
| 工具标签 | `shell/tabs.rs`、`shell.rs::render_right_tab_strip`、`surface_chrome.rs` | `ui.TabsBase` + `ui.ScrollHorizontal` + `ui.ScrollState`，没有 HTML |
| 文件树 | `files/tree.rs`：27 DIP 行高、14 DIP 缩进、Symbols 图标 | 官方 `ui.Tree` / `ui.TreeItem`（gallery Tree 示例），保留键盘和可访问性路径 |
| 文件搜索工具条 | `files/mod.rs::render_explorer_header`、`surface_chrome.rs::toolbar` | `ui.TextInputBase` + 原生按钮，38 DIP 工具条、24 DIP 输入框 |
| 编辑与 diff | `files/editor.rs` / `preview.rs` | 官方 `ui/editor.State`、`editor.View`、`editor.DiffView` |

## 布局、尺寸与拖动

| 编号 | 小项 | 当前结果 | 验证 |
| --- | --- | --- | --- |
| L01 | 列顺序为 sidebar/chat/tools/files | 已核对并约束，Files 固定在最右 | `TestWorkspaceRenderedColumnOrderAndResizeRestoration` |
| L02 | 900×600、1320×880、1600×1000 的实际矩形不重叠、不越窗 | 通过无窗口渲染检查 | 同上，浅色/深色各一遍 |
| L03 | Sidebar/Tools/Files 的 8 种开关组合 | 已逐种验证三种宽度，共 24 个组合 | `TestWorkspacePanelCombinationMatrix` |
| L04 | 文件栏偏好先收缩，给聊天 300 和工具 360 DIP 留空间 | 已修复；900 宽的最大文件偏好显示为 240，保留原偏好 | `TestPanelWidthsPreserveConversationAndPreferences` |
| L05 | 1320 宽，Sidebar 400 / Files 440 / Tools 800 的极值组合 | 显示为 400 / chat 300 / tools 360 / files 260 | 同上 |
| L06 | 缩小再放大时恢复被 fit 隐藏的列和偏好宽度 | 通过；渲染分配与持久偏好分离 | 实际 `Tester.SetSize` |
| L07 | Sidebar 拖动保存 242–400 DIP 范围 | 已收紧写回边界；保留分隔线宽度 | `TestSidebarDragPersistsLogicalWidth` |
| L08 | 文件栏拖动更新偏好，范围 220–440 DIP | 通过真实 Tester pointer press/move/release | `TestWorkspacePaneDragAndTakeoverLifecycle` |
| L09 | 工具栏拖动保留聊天最小宽度 | 通过；窗口挤压不再永久缩小工具偏好 | 同上 |
| L10 | Takeover 仅消费聊天宽度，保留文件栏分配 | 已核对；无工具区手动分隔条 | 同上及窄窗口纯分配测试 |
| L11 | 关闭最后一个工具标签仅收起工具 host | 已修复；Files 保留，takeover 清除，重新打开恢复普通宽度 | 同上 |
| L12 | 被 fit 隐藏但仍 open 的面板再次点击应提升优先级 | 标题栏已修复，避免反而关闭；命令面板/设置快捷键仍需统一调用 `togglePanel` | `TestFitHiddenPanelTogglePromotesRatherThanCloses` |
| L13 | 已经可见的程序化 open 不改变优先级 | 已修复 | 同上 |
| L14 | 底部终端只占聊天列宽度，不截短工具区或 Files | 已修复并验证矩形；终端原生库显示另验 | `TestWorkspaceTerminalDrawerDoesNotShortenOtherColumns` |
| L15 | 工具标签在标题栏 38 DIP 带内，不额外占一行 | 已修复；工具正文起点 Y=38 | `TestWorkspaceOverflowEvidence` |
| L16 | 列显隐/fit 的 WidthTween、fit exit 动画 | 尚未完整移植 | 当前检查约束 settled 几何，不能据此宣称动效一致 |

## 主题和文字

| 编号 | 小项 | 当前结果 | 验证 |
| --- | --- | --- | --- |
| C01 | Mode=system 使用主窗口已经解析的明暗外观 | 已修复；不再使用 `Mode != light` 推断深色 | `TestSystemThemeUsesHostAppearanceForAllWorkspaceSurfaces` |
| C02 | 工作区、文件栏、diff、原生终端主题同步 | 使用同一 token 解析；terminal 外观缓存随有效主题更新 | system light/dark 测试与终端既有测试 |
| C03 | 全部内置主题的工作区/diff token | 逐个主题通过 | `TestWorkspaceThemeCatalogAndConfiguredEditorFont` |
| C04 | diff 使用配置的代码字体/字号 | 已修复原固定 12.5 DIP；与文件编辑器采用同一字体解析 | 同上，20 的配置值经过原比例映射 |
| C05 | 文件树不会通过临时 SetTheme 改写整个窗口的磨砂背景 | 已避免该路径；主窗口 theme/root material 保持 | `TestExplorerPreservesHostRootMaterialAndTheme` |
| C06 | 暗色与浅色的真实系统材质和字体栅格化 | 待原生验收 | 无窗口图片不能证明系统合成与 DPI 栅格化 |

## 标签逐项行为

| 编号 | 小项 | 当前结果 | 验证 |
| --- | --- | --- | --- |
| T01 | 标签固定 112×24 DIP，间距 4 DIP | 已按 source 移植，不缩成拥挤文字 | `TestOverflowTabsKeepFixedGeometryAndKeyboardReveal` |
| T02 | 溢出时水平滚动，加号随标签滚动、展开按钮固定 | R11 按冻结 source 修正顺序与位置；旧“加号独立固定”结论为历史 | `TestOverflowTabsKeepFixedGeometryAndKeyboardReveal`、`TestTabStripControlsKeepIdentityAndExpandSlotAtOverflow` |
| T03 | 新建/切换/键盘选择主动显示选中标签 | 已接 `ScrollIntoView`，不是每帧强制吸回 | End 键和自主滚轮测试 |
| T04 | Left/Right/Home/End 的官方焦点选择行为 | 使用 `TabsBase`；End 与选中标签显露通过 | 同上 |
| T05 | 每个标签有独立关闭槽，悬停显示 ×，未保存时显示点 | 已实现；关闭按钮不改变标签尺寸 | UI 渲染与关闭行为测试 |
| T06 | 关闭非当前标签保留当前 surface identity | 通过 | `TestReorderAndClosingInactiveTabsPreserveActiveIdentity` |
| T07 | 拖动重排，保留当前 surface identity | 通过实际原生 Tester drag/drop；按 kind/id 定位，构建循环结束后再应用移动 | `TestTabsDragAndWheelKeepSelection` |
| T08 | 右键关闭/关闭其他标签 | 已接真实关闭路径；未保存文件沿用确认流程 | 关闭/未保存既有测试 |
| T09 | 新标签按钮原生菜单 | R11 无 Git 时省略 Diffs/History；读取所选项目 typed `GitDetected`，旧“按能力禁用”结论为历史 | `TestTabStripSourceEmptyAndNonGitMenu`、`TestTabStripGitDetectionDoesNotBorrowAnotherProject`；原生菜单视觉/完整流程待验 |
| T10 | 浏览器页面标题更新标签 | 既有 OnPageTitleUpdated 保留，长标题固定宽度省略 | 原生命中、favicon、悬停提示待实机验 |
| T11 | 文件重命名更新标签/路径，保留未保存缓冲区 | 已修复 | `TestFileRenameUpdatesTabAndRetainsUnsavedBuffer` |
| T12 | 溢出边缘渐隐及拖动滑动动画 | 未宣称完成 | 当前为官方原生滚动/拖放，source 的渐隐和 150ms sibling slide 尚需补齐/视觉验收 |
| T13 | 每个会话独立的工具标签/编辑器/滚动恢复 | 已补齐会话 scope；保存 tabs、编辑器缓冲区/选区、diff、文件树、搜索、标签滚动；切换停止旧 autosave 计时器并恢复当前 surface 的焦点请求。App 同时恢复该会话 takeover 和 fit 优先级，新建画布不恢复底部终端 | `TestSessionWorkspaceRestoresTabsAndUnsavedEditorIndependently`、`TestConversationSwitchRestoresOwnedWorkspaceAndTakeover` |
| T14 | 子会话/子代理在原生侧栏标签显示 | 已提供 `OpenSession` 与 `RenderSession`；没有 renderer 时沿原中央跳转，避免空标签 | `TestSideSessionRendererAndCentralFallback`；主任务接线与子代理卡片另验 |

### R11 TabStrip 源码更正与实施边界

本节对应 [R11 独立审计](../output/tabstrip-r11-audit/result.json)，事实源仍固定为
`ab189159f10cf8e63c0502be75a59bbc487e0eaf`。此前 T02 的加号固定占位、T09 的
非 Git 项禁用，以及 R8 复用 24px 展开按钮的记录保留为阶段历史，不能作为当前实现结论。

冻结 `shell.rs:13586–13633` 的展开按钮是 **28×28 DIP、16px muted 图标、radius6**；
`shell.rs:12944–13076` 的加号则是 **24×24 DIP、13px muted 图标、radius6**，并非两者都为28。
加号仅在已有标签时出现，位于同一水平滚动内容末尾，右侧才是固定展开按钮。
`shell/tabs.rs:36–41,497–535,634–641,684–687` 给出 expanded 时的 collapse 图标、
一次 top4、left8/right4/gap4，以及透明且无独立底线的标题栏。R11 已接入这些稳定态事实。
两按钮使用 wash(.11) 的150ms hover，effective reduced motion 立即切换；展开/菜单打开本身
不增加常驻选中底色。无项目、失效项目和非 Git 项目均不借用其他项目的 Git 状态；
已检测 Git 但分支列表尚未加载时仍保留 Git 入口。

`CGO_ENABLED=0 go test ./internal/workbench -count=1 -v` 的45个本包顶层测试通过
（1.038s），scoped Go vet、MyGo vet 均 exit0。审计保存了修复前失败日志、360/520 DIP
与 light/dark 的实际 Tester 几何，以及滚动后点击、End、Tab/Enter、展开/收回、关闭前序标签
后的焦点和固定槽回归。这些是实现侧回归，**不是原生或像素一致性验收**；R11 主线全包构建与
原生验证在本节写入时仍由主 agent 执行，不能据本节记为通过。

剩余边界：MyGo 默认水平滚动条仍可见。34px 滚动区域避开了标签中心，但可见 hit track 的
`y26..38` 仍覆盖 chip/plus 底部 `y26..33`，约 **7 DIP**，未宣称完整矩形点击一致。
冻结源码的 **36px 边缘渐隐**、**150ms 拖动 sibling slide** 尚未补齐；加号菜单仍使用现有
MyGo 原生菜单，未移植源码的 **168px 自绘菜单、下方10px间距**及其完整焦点/重复按压语义。
这些项目和多 DPI/平台原生视觉验收继续待办。

### R12 滚动条与渐隐更正

上述7DIP遮挡和缺失渐隐是R11时的状态。R12只在TabStrip设置原生
`Scrollbars(false)`，相同y32的1x/2x底部点击4项修复后均通过；不透明背景的
36px全高shell渐隐已实现。绘制时读取本轮ScrollState还修复了6种MaxX变化但
无额外host frame时留下旧渐隐的问题，8项idle回归与51项workbench测试通过。
完整五项主线检查、四目标r2包和DPI192的确切Windows EXE原生底部点击/菜单/
展开收回观察分别封存于 `reference/validation/scrollbars-r12-result.json`。

原生还发现History插入和面板收回后活动标签可保持裁切，手动wheel能滚入；该项
继续修复，不能把既有键盘回归解释成所有自动滚入路径通过。玻璃alpha遮罩、
拖动/滚入动画、自绘菜单、配对原版像素与完整跨平台验收仍待完成。详细证据与
源码时序限制见 [本阶段审计](audit-tabstrip-scrollbars.md)。

### R13 菜单滚入后续

R12的History菜单路径已单独修复：入口消费旧reveal请求，让构建后段Chosen产生的
新请求留给重建。8项原失败菜单回归和6项direct/End对照均通过，手动wheel不被拉回。
53项本包测试、五项主线门槛、默认diffcheck及四目标包通过；确切Windows EXE在
DPI192的同一路径无需补救输入即可完整显示活动History，actualexit0/stderr空。
缩窄/收回没有新selection请求，原版自动揭示要求尚未建立，R13未改变此语义；
plus常驻可见也不是本修复的要求。见 [R13审计](audit-tabstrip-reveal.md)。

## 文件树和编辑器

| 编号 | 小项 | 当前结果 | 验证 |
| --- | --- | --- | --- |
| F01 | 原生 Tree/TreeItem，避免重复自制键盘导航 | 使用官方组件，27 DIP 行高/14 DIP 层级缩进 | gallery 与 pinned source 核对 |
| F02 | Symbols 文件/目录图标 | 复用原版注册表、缓存和资源；另导出只读 `workbench.FileIcon` 给聊天复用 | 既有图标测试 |
| F03 | 文件名横向起点 | 官方 disclosure slot 为 16 DIP，以前内边距扣除 2 DIP，保留图标/文字起点 | 当前截图；箭头形状仍需原生对照 |
| F04 | 目录优先，其内按名称大小写无关排序 | 已修复 | `TestExplorerFiltersHiddenAncestorsAndSortsDirectoriesFirst` |
| F05 | 隐藏目录的子项不会从搜索泄漏 | 已修复，按整个路径的隐藏段过滤 | 同上 |
| F06 | Search 的 Escape 清空并恢复树 | 已实现 | 同上 |
| F07 | 点击目录名称也能展开/折叠 | 已接原生 item 的 OnClick；箭头/键盘由 Tree 处理 | `TestExplorerDirectoryToggleAndRowGeometry` |
| F08 | 新建文件/目录、重命名、删除 | 既有 typed command 和对话框保留；readonly 守卫在 backend | `TestFileActionsCreateRenameDelete` 及 mock CRUD 测试 |
| F09 | 文件拖到工作区打开/拖入输入框引用 | 保留 `FileDrag`，不伪装为真实磁盘文件 | 本轮未新增文件拖到输入区的实机验收；标签拖放另有实际 Tester 测试 |
| F10 | 编辑/保存/未保存关闭/冲突不覆盖本地内容 | 既有闭环保留，新打开文件请求原生编辑器焦点 | `TestFileSaveAndUnsavedClose`、`TestConflictPreservesLocalBuffer` |
| F11 | Markdown、图片缩放/lightbox、diff/history/PR | 既有原生实现和测试保留 | 本轮不是这些页面的完整视觉通过结论 |
| F12 | 文件树多选、跨目录文件移动、长树虚拟化 | 待补齐 | 当前官方 Tree 全量构建；不能据本轮小样本认定长树性能通过 |
| F13 | 搜索结果行、文件上下文菜单和内联重命名完全复刻 | 待继续对照 | 现有操作有效，交互样式尚未逐状态对比 |

`SwitchSession` 只切换本地 UI/widget state，不发 backend command；浏览器/终端的全局资源注册表
继续统一管理销毁，隐藏会话的文件 autosave 计时器停止。恢复时用最新 snapshot 重做版本/冲突
核对，避免旧快照覆盖未保存内容。关闭整个 State 同时停止当前及缓存会话的计时器。

文件编辑器还通过 `TestNewFileEditorReceivesFocusAndCommittedIMEInput` 验证新打开后的焦点、
组合预编辑、文本提交和一次撤销。此处按官方 `ui/editor/editor_test.go::TestCodeEditorComposition`
使用 Compose→Type 事件。直接在 Tester.Compose 之后注入 Tester.Key(Enter) 会把未经系统输入法
过滤的原始键送给自定义编辑器；Windows IMM32 与 macOS NSTextInput 的实际候选选词路径不由该
测试模拟，因此没有据此改写固定框架或宣称原生 Enter 选词已验收。

## 可重放证据

通过环境变量启用截图，默认测试不会写磁盘：

```powershell
$env:ZERON_WORKBENCH_EVIDENCE = "$PWD/output/audit-workbench"
go test ./internal/desktop -run 'Test(WorkspaceRenderedColumnOrderAndResizeRestoration|WorkspacePaneDragAndTakeoverLifecycle|WorkspaceTerminalDrawerDoesNotShortenOtherColumns|WorkspaceOverflowEvidence)$' -count=1
Remove-Item Env:ZERON_WORKBENCH_EVIDENCE
```

- [`columns-1320x880-light.png`](../output/audit-workbench/columns-1320x880-light.png) / [`dark`](../output/audit-workbench/columns-1320x880-dark.png)
- [`columns-900x600-light.png`](../output/audit-workbench/columns-900x600-light.png) / [`dark`](../output/audit-workbench/columns-900x600-dark.png)
- [`columns-1600x1000-light.png`](../output/audit-workbench/columns-1600x1000-light.png) / [`dark`](../output/audit-workbench/columns-1600x1000-dark.png)
- [`overflow-1320-light.png`](../output/audit-workbench/overflow-1320-light.png) / [`dark`](../output/audit-workbench/overflow-1320-dark.png)
- [`takeover-1600-light.png`](../output/audit-workbench/takeover-1600-light.png)
- [`terminal-combination-1600.png`](../output/audit-workbench/terminal-combination-1600.png)

这些均为当前组件经 `ui.NewTester` 生成的**实现图**，不是参考 golden，没有用它们计算或声称视觉 SSIM 通过。底部终端无窗口时显示其真实不可用状态，没有伪造终端原生运行成功。

同名 JSON 保存当前实际矩形，`output/audit-workbench/evidence.json` 保存截图、矩形文件和相关
源码的 SHA-256。实测 900×600 的边界样例为 chat `X=0,W=300`、tools `X=301,W=359`、
files `X=661,W=239`；工具与文件列各自还包含前一条 1 DIP 的 Split 分隔线。

本轮最终执行 `go test ./internal/workbench -count=1`，以及 desktop 中
`Test(FitPanels|PanelWidths|Workspace|FitHidden|ConversationSwitch|SidebarDrag)` 的定向回归，
均通过。这两个包的 `go vet`、`go tool mygo vet .` 和 golangci-lint 均通过，lint 为 0 issues。
完整桌面测试由主任务合并模型选择器/窄栏输入区修改后统一重跑，不把工作区定向通过等同于整包通过。
固定框架仍为 `zeron-native-ui.2`，源码锁校验通过；本轮没有改动框架指纹。

后续原生检查仍包括 Windows 标题栏系统按钮与标签命中、DPI 100/125/150/200%、真实滚轮/触控板、中文输入法、剪贴板、网页引擎与原生弹层、终端选择/resize，以及源图同平台同尺寸下的逐状态对比。当前未给出“视觉完美复刻”或平台验收通过结论。
