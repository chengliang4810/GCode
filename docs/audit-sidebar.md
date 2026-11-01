# 侧栏分项审查与本轮修复

历史审查记录。2026-10-30 起，项目筛选及列表/设备分组已由项目折叠面板替换，
sidebarShowProjectFilter、sidebarGrouping 字段已删除；同日删除了会话行运行框架
（Harness）图标、sidebarShowHarness/sidebarShowArchived 字段与侧栏 Archived 分组，
归档数据改为只在设置页“已归档对话”中查看、还原和删除。当前规则与验收见
[左侧栏排版](sidebar-layout.md)。下文保留 2026-10-28 的源代码审查与历史证据，
其中旧分组、项目筛选、Harness 图标、侧栏归档分组、29/45/61/63 DIP 尺寸和
账户 footer 不描述当前实现。

日期：2026-10-28。参考源码固定为 Zeron **v0.2.128 / ab189159f10cf8e63c0502be75a59bbc487e0eaf**，`D:/zeron` 只读。
优先核对官方 `D:/projects/mygo`，实际 API 再检查固定 MyGo v0.6.4 的 `ui/list.go`、`list_builder.go`、`menu.go`、`textinput.go` 和 gallery List 示例。

本轮没有启动原生应用、Computer Use 或安装器，没有重新 build。按主任务要求，完成这组功能、测试与证据后停止扩展，交由统一测试/race/构建收口。

## 事实源与字段

| 项目 | Zeron 源码依据 | 当前实现 |
| --- | --- | --- |
| 默认视图 | `settings.rs`：Compact=true、InOneList、LastUpdated；Show 项默认 true | backend 已将默认 compact/list 对齐，UI 不额外创建默认假值 |
| 分组及排序 | `shell/spaces.rs::compare_sidebar_chats`、`sidebar_project_group`、`promote_local_device_group` | 置顶顺序优先；常规组按当前活动/创建排序的首次出现顺序，不按项目 ID 字典序；本机 device 组前置 |
| 视图菜单 | `spaces.rs::render_sidebar_view_menu`：Organize / Sort / Show | 使用官方原生 `ui.Menu.Submenu`，每个显示项连接真实 settings command |
| 侧栏行尺寸 | `shell.rs::sidebar_row_height`、`chat_row_height` | compact 29；regular 基础 45，Branch 61，PR 63；隐藏 Location 减 16 DIP |
| 分组标题 | `spaces.rs::sidebar_disclosure_header` | 28 DIP 标题、12 DIP 组间距、4 DIP 内容衔接；折叠显示计数 |
| Archived | `shell.rs` 构造 archived_open=true；`spaces.rs::render_archived` 先 10，再每页 +25 | 默认展开内联 Archived 分组，保留 pinned 身份与位置；Show 选项只改变可见性，未选中且未悬停的归档行透明度 0.4 |
| 标签与元数据 | `shell.rs::render_chat_row` | Harness 来自 model/provider，Location、Branch、PR、项目图标按独立强类型设置显示 |
| 项目筛选 | `spaces.rs::render_spaces_filter` / `render_spaces_menu` | 独立 sidebarProjectID；搜索和键盘选择不改 composer 的目标项目 |

本轮 UI 使用已有 `Settings.SidebarCompact/SidebarGrouping/SidebarSort/ShowProjectLabels/ShowIcons`，并接入 backend 新增的
`SidebarShowHarness`、`SidebarShowBranch`、`SidebarShowPullRequest`、`SidebarShowProjectFilter`、`SidebarShowArchived`。
Location 对应 ShowProjectLabels，Project icon 对应 ShowIcons；契约中的排序值为 `updated / created`。

## 逐项结果

| 编号 | 小功能 | 结果与证据 |
| --- | --- | --- |
| S01 | 侧栏默认形态 | 已对齐 source 的 compact + 单列表；不再默认为详细项目组 |
| S02 | 顶层会话范围 | 排除 ParentSessionID 与 SideChat 行，真实子会话保留在聊天/侧会话入口 |
| S03 | 分组 | Pinned 独立；None / By project / By device 均有效；按 recency/created 的组顺序，本机设备优先 |
| S04 | 折叠与计数 | 组 header 可切换；折叠计数跟随 projection，收起子项不继续构建 widget |
| S05 | 排序 | `SidebarSort` 已真正生效；同时间以稳定 ID 作 tie-break；pin order 不被普通排序打乱 |
| S06 | 文本搜索 | 不再把字面 `Pinned`、`Working`、`Unread` 当隐式筛选器；匹配标题/分支/项目/设备；命令面板搜索可键盘打开会话 |
| S07 | 无来源的筛选入口 | 移除了项目行上旧的状态筛选按钮；source 项目筛选行只承担项目范围筛选 |
| S08 | 项目筛选独立性 | 搜索项目名/路径/设备，Up/Down/Enter 选择，Escape 退出；不会修改 composer 项目 |
| S09 | 新建项目入口 | 保留可执行的原生项目对话框入口；完整 source 设备/目录选择器仍列为待补齐 |
| S10 | Native List 键盘 | 接入 ListState.Selected/Header/Key/Label；Up/Down 跳过组标题并打开会话；保留虚拟化 |
| S11 | F2 重命名目标 | 已修复“Down 后仍重命名旧焦点行”的问题：快捷键按 List 当前选择执行，键盘导航后焦点返回列表 |
| S12 | 行内重命名 | 双击/右键 Rename/F2 开始，原生 TextInput 获得焦点并选中标题；Enter 提交，Escape 取消；不再用独立 Rename modal |
| S13 | Pin / Unpin | 原生上下文菜单使用准确的动作名称，后端状态真实改变 |
| S14 | Pinned 拖动 | 使用官方 ListState.Reorder；在输入回调执行状态变更，不在绘制循环直接发 pin command；保持其他全局 pins 的相对顺序 |
| S15 | Archive 当前会话 | 状态改变并切换到另一活动会话/新画布，避免详情继续占用归档前的活动位置 |
| S16 | Unarchive | Archived 分组内恢复，原 pinned 标识和 PinOrder 保持 |
| S17 | Archived 分页 | 初始 10、Show more 每次 +25；关闭 Show/Archived 不修改业务数据 |
| S18 | Delete | 有原生确认对话框；Cancel 不修改数据，确认调用 typed session.delete |
| S19 | Mark read/unread 与 Copy | 操作接真实 DTO/剪贴板；Copy 的原版完整菜单字段和外观尚需继续核对 |
| S20 | 行高度和元数据 | 紧凑行固定 29 DIP，详细 Branch/PR 元数据放在标题下一行，PR badge 可打开原生 PR 工作区 |
| S21 | Harness | 按会话 ModelID 查 provider.Icon，不再统一显示 Claude |
| S22 | 选中外观 | Tester 图暴露官方 List 的默认纯 Accent 填充；已在 List 的局部样式中去掉该填充，保留 source 的半透明 row wash 与可访问选择/重排行为 |
| S23 | 宿主材质 | 局部 List theme 只改变默认选择色，行内输入仍使用原 theme；每次构建和虚拟化 late build 都恢复宿主 Glass 背景 |
| S24 | footer 与图标 | avatar 16 DIP、菜单 hit target 28 DIP；profile pill 不把中间空白变成按钮；PR 入口使用实际 pull-request 图标 |
| S25 | 标题栏/命令/快捷键一致性 | overlays 的 Toggle sidebar / Files 和 settings 的两处快捷键均已统一到 togglePanel，fit-hidden 面板会提升优先级而非被关闭 |
| S26 | 命令面板可操作性 | 已将既有命令/会话列表接官方 List 和输入 Up/Down/Enter；过滤顶层会话，结果为空不能执行无效索引 |

## 测试与检查

`go test ./internal/desktop -run TestSidebar -count=1` 通过，包含：

- 29/45/61/63 DIP 尺寸纯规则与实际渲染矩形。
- 原生 Organize / Sort / Show / Compact 菜单选择后读取 backend 验证持久状态。
- activity/created 排序、分组次序、literal Pinned 搜索与子会话排除。
- List Down→F2→Enter / 右键 Rename→Escape。
- Archive→切换→Archived→Unarchive→Unpin→Delete Cancel→Delete Confirm。
- 项目搜索→Down→Enter，验证 sidebar 筛选和 composer 目标互不污染。
- 真正的 Tester pointer press/move/release 触发官方 List.Reorder，核对 backend pin 顺序。
- 命令面板 Files 提升被 fit 隐藏的 Files；Ctrl/Cmd+B 的侧栏入口；命令搜索 Enter 打开会话。
- 1,000 会话只构建可见行，测试显式选择 project 分组，不再依赖旧默认。

最终 `go vet ./internal/desktop` 和 `golangci-lint run ./internal/desktop/...` 通过，lint 为 0 issues。没有把侧栏定向通过写成全项目最终通过；统一全包/race/发布检查由主任务收口。

## 实现图与复跑

[`output/audit-sidebar/evidence.json`](../output/audit-sidebar/evidence.json) 记录 16 张实现图、同名矩形 JSON 和侧栏源码的 SHA-256。
这些图由真实 MyGo 组件经 `ui.NewTester` 绘制；没有运行原生窗口，不能证明系统材质、菜单、DPI 或视觉相似度通过。

- [`默认浅色紧凑单列表`](../output/audit-sidebar/sidebar-light-list-compact-true.png)
- [`深色详细项目分组`](../output/audit-sidebar/sidebar-dark-project-compact-false.png)
- [`设备分组`](../output/audit-sidebar/sidebar-light-device-compact-true.png)
- [`项目搜索`](../output/audit-sidebar/sidebar-project-search.png)
- [`行内重命名`](../output/audit-sidebar/sidebar-inline-rename.png)
- [`Archived 展开`](../output/audit-sidebar/sidebar-archived-open.png)
- [`命令面板 Files`](../output/audit-sidebar/sidebar-command-files.png)

其余文件覆盖 light/dark × list/project/device × compact/regular 的全部 12 种组合。

```powershell
$env:RCODE_WORKBENCH_EVIDENCE = "$PWD/output/audit-sidebar"
go test ./internal/desktop -run '^TestSidebarAudit(Evidence|TransientEvidence)$' -count=1
Remove-Item Env:RCODE_WORKBENCH_EVIDENCE
```

## 待继续验收/补齐

1. 同平台同数据的原版截图对比、实际 Windows caption/菜单与 DPI 100/125/150/200%、键盘/IME/剪贴板。当前原生 Menu 使用 MyGo 系统菜单；原版 GPUI 自绘菜单的图标、字体、尺寸及 submenu 动效尚未视觉收敛。
2. source 的自定义 sections（新建/重命名/拖动/归档整组）、跨组会话 transfer、拖动 sibling 动效和 disclosure 动效尚未完整移植。
3. source 命令面板 All/Threads/Commands/Files 四页、分组 preview 行、workspace 文件/内容检索尚未完整移植；本轮只修既有命令/会话入口及键盘闭环。
4. 项目筛选器的原版设备标签、项目图标和连接/离线状态；新建项目的设备页签与 mock 文件夹浏览器仍需完整原生实现。
5. 文字边缘 fade、jump hints、working/completed/subagent 的全部 status/hover 组合，以及行内重命名失焦的全部路径仍需逐状态核对。
6. 当前 relativeAge 仍以数据集中最新更新时间作为确定性参照；普通运行下独立时钟的分钟进位尚需接入统一时钟契约。

以上待办没有用编译或实现图标为验收通过，本轮也没有给出完整侧栏视觉复刻结论。
