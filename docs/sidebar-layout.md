# 左侧栏排版

2026-10-30：按用户要求，以 `D:/projects/ZCode` 当前源码
`a60ba6a7e5a06742270ca605f5d2cc2766afa7e1` 分析侧栏结构，并在 Go/MyGo 中独立实现。
截图只用于核对层级和比例，没有复制参考实现。

## 源码依据

| 参考源码 | 核对内容 |
| --- | --- |
| `packages/ui/src/project-sidebar/ProjectSidebarNavigation.tsx` | 顶部导航独立于列表，水平 8px、垂直 12px 留白；自动化入口取决于真实业务回调 |
| `packages/ui/src/NewTaskButtonGroup.tsx` | 整行新建入口、32px 基准高度、右侧读取实际快捷键 |
| `packages/ui/src/project-sidebar/ProjectSidebarListHeader.tsx` | 列表标题、搜索、视图菜单固定在滚动内容上方 |
| `packages/ui/src/project-sidebar/PurposeSections.tsx` | 项目、任务分区独立，标题弱于内容，分区可折叠 |
| `packages/ui/src/project-sidebar/ProjectNode.tsx`、`ProjectNodeContent.tsx` | 项目标题控制 Collapsible，内容是该项目的对话；32px 项目行、较弱的项目名称和不可用状态 |
| `packages/ui/src/project-sidebar/ProjectSections.tsx`、`view.ts` | 项目成员按项目 ID 派生，项目分区外仅放无项目对话；置顶不在项目内重复 |
| `packages/ui/src/project-sidebar/ProjectTaskRows.tsx` | 每个项目独立初始 5 条，显示更多按 5 条增加；入口与内容缩进对齐 |
| `packages/ui/src/project-sidebar/useProjectSidebarPreferences.ts` | 项目默认展开，各自收起，另有批量展开/收起 |
| `packages/ui/src/TaskListItem.tsx` | 主要标题和右侧较弱时间信息分层，状态不挤占标题 |
| `packages/ui/src/app-shell/WorkspaceShellLayout.tsx` | 默认侧栏宽 264px，可调整；截图宽度不能当作唯一固定宽度 |

## GCode 的实现

聊天侧栏依次为原生窗口导航、新建对话、对话列表工具栏、置顶、项目折叠面板、
未归属项目的对话。没有内容的置顶分区不显示；“对话”分区始终显示，标题右侧提供新建对话入口；没有项目时，项目分区显示“未添加项目”。
2026-10-30 起，会话行的运行框架（Harness）图标及其显示设置已删除，
侧栏不再包含已归档分组；归档数据只能在设置页“已归档对话”中查看、还原和删除。
同日起侧栏只有紧凑单行形态，视图选项菜单是四选一单选（对齐 ZCode 侧栏并增加设备视图）：

- **按项目**（默认）：置顶 + 项目折叠面板 + 未归属对话，即下文的项目树形态。
- **按分组**：置顶 + 用户自定义会话分组（`contracts.SessionGroup`，标题、手动顺序）+
  未分组对话。分组头右键可重命名/删除；分组头折叠展开；会话右键“移到分组/移出分组”。
  分组通过 `group.create` / `group.rename` / `group.delete` 命令持久化，
  成员关系保存在会话的 `groupId` / `groupOrder`；删除分组不删除会话。
- **按最近更新**：全部活动对话按更新时间排序的单列时间线，日期段标题为
  今天/昨天/N天前/本周/上周/本月/上月/更早，按本地日历日切桶。
- **按设备**：置顶在前，其余按所属设备分组；本机设备排第一，其后按设备列表顺序，
  未知设备的会话进入“本设备”末组。

视图选项不再是排序菜单：`sidebarSort` 已删除，`sidebarGrouping` 取代它
（projects/grouped/timeline/device）。此前一轮删除的 sidebarShowHarness、
sidebarShowArchived、sidebarCompact、sidebarShowBranch、showProjectLabels 保持删除。
对话行只显示状态点、标题和时间（悬浮时为归档按钮）；项目行显示文件夹与名称。
原有元数据移入共享悬浮卡片（`controlSidebarHoverCard`）：悬浮项目行显示项目名称、
聊天数和路径；悬浮对话行显示位置（项目 @ 设备）、分支和最近活动时间。
底部设置、PR、用量和更新入口保持原有功能。设置路由继续显示现有设置导航。
没有对应业务的自动化和插件市场未添加占位入口。

项目文字弱于对话标题，分区标题更弱；时间使用辅助字号。
所有中文界面文案统一使用“对话”，业务内部的 Session 类型与实际执行任务概念不改名。
共享行尺寸、留白、圆角和图标尺寸位于 `internal/design/controls.go` 的 `Sidebar()`，
控件外观位于 `internal/desktop/settings_controls.go`；不在业务页覆盖共享视觉属性。
大字号按文字内容增加行高，保留现有用户调整的侧栏宽度。

侧栏只从现有 Go 核心快照中的 ProjectID 派生树，不按目录名称猜测归属，
不复制业务状态。每个项目下面直接显示属于该项目的对话，项目标题只控制自身展开，
点击“项目”标题收起分区时，同时收起所有项目内的对话；再次展开分区后，各项目保持收起，用户按需单独展开；都不改变输入框所属项目、当前对话或草稿。
各项目、无项目列表的分页独立，收起后保留已展开的页数。
置顶对话只在置顶区出现；子会话和侧聊不进入顶层。
已移除项目的保留对话仍在无项目列表可达。

旧的项目筛选弹层、筛选状态及 Organize 列表/设备分组模式已由项目树替换，
对应的 sidebarShowProjectFilter、sidebarGrouping 字段和调用方一并删除，无兼容分支；
sidebarShowHarness、sidebarShowArchived 随运行框架图标与侧栏归档分组一并删除。
排序、置顶、归档、状态、重命名、拖动置顶排序和删除继续使用原业务命令。

分组视图的拖拽与置顶拖拽共用原生列表的整行拖拽：
拖分组标题可调整分组先后（`group.reorder`，顺序密集重编号）；
拖对话可在组内调整位置、拖到另一组（含未分组区）即改变归属
（`session.reorder`，携带目标组与相对锚点，整组 rank 重编号）。
拖到分组标题上表示加入该组顶部；拖到未分组区表示移出分组。
置顶与归档对话不参与分组内手动排序，仍各自占据自己的分区。
供应商页与模型列表使用同一种整行拖拽：按下行本身即可拖动，
插入指示线、半透明源行与取消（Escape）行为与侧栏一致，不再单设拖拽手柄。
搜索入口打开现有命令中心的对话标签，重置旧标签与选择状态。
列表容器与内部结果按钮使用不同 key 前缀。

显式打开、重命名、置顶/取消置顶后，仅展开目标分组并将目标滚入视口；
当前选中或正在重命名的对话不被分页隐藏。
后台组织操作只在目标 ID 和提交后的分组同时匹配时消费定位请求；
非当前旧对话取消置顶时，将目标组扩展到对应页，后续帧仍保留该页。
再次打开同一对话也会展开其祖先；普通渲染不会强制打开用户手动收起的项目。
新建项目等待权威快照后定位新行，不在后台命令回执早于快照时丢失定位请求。
对话的相对时间使用注入的当前时钟，每分钟刷新，未来时间显示“刚刚”。

项目与对话使用同一个虚拟列表滚动，顶部导航、工具栏及底部入口固定。
项目或独立对话分区没有匹配对话时，“暂无对话”作为所属分区内的不可选内容行，
紧跟标题，随所属面板折叠。空状态不放在撑满高度的列表之外。
原生列表容器使用 `sidebar-list-row:` 前缀，与项目和对话内部按钮的 key 分离；
避免无障碍树中出现重复业务 key。

## 验证

`TestSidebarLayoutHierarchyAndPagination` 核对顺序、行高、分页和分区折叠；
`TestSidebarNestedRowKeepsTheListEdge` 在 264/350px 下核对嵌套行只缩进内容：
选中底色与顶层行同右边缘、左缘恰好差 `ChildInset`，标题同一列缩进同一数值；
`TestSidebarProjectAccordionPreservesDraftAndComposerTarget` 核对折叠隔离；
`sidebar_tree_test.go` 覆盖项目归属、重复排除、独立分页、深处/同一对话导航、
定向重命名、归档排除、置顶恢复及真实时钟；
`TestSidebarEmptyStateFollowsConversationHeading` 在高窗口、深浅主题和 14/20px 字号下，
核对无对话、搜索无结果时的空状态位置与折叠；
`TestSidebarLayoutRenderEvidence` 覆盖 264/350px 侧栏、14/20px 字号和深浅主题。
实现渲染保存于 `output/sidebar-project-tree/component/`，不作为参考图像素一致性证明。

`TestDesktopLocalSidebarLayout` 使用独立数据目录和真实本地核心，实际创建项目、
对话后检查父子行几何、5→7 条独立分页、单项目与全部折叠/展开、空项目、
草稿保护、从搜索打开折叠项目中的对话、返回画布恢复草稿，并保存真实窗口截图。
模型菜单与底部入口另复用原有原生回归。
这些检查不访问真实模型供应商，不恢复已暂停的全面视觉一致性目标。

macOS 标题栏图标以系统按钮预留高度的中心线对齐，系统按钮右侧保留 12 DIP 间隔；新建对话图标中心与关闭按钮中心同列。项目分区不再提供单独的批量开合图标。


