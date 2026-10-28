# 命令面板、项目选择与文件搜索交接

日期：2026-10-28。事实源为只读 `D:/zeron` 的 **v0.2.128 / ab189159f10cf8e63c0502be75a59bbc487e0eaf**。框架固定 MyGo **v0.6.4 + zeron-native-ui.2**，本轮未修改框架。用户要求换机继续后，停止新增功能，保留当前可编译实现及未完成清单。

## 源码依据

- `crates/ui/src/shell/command_palette.rs`：四页、分组顺序与数量限制、至少两个字符的工作区查询、80ms 内容查询节流、行列跳转、键盘操作、卡片位置和尺寸。
- `crates/ui/src/shell/thread_search.rs`：跨标题/项目/分支/设备/PR 的逐词匹配、字段权重、词首加分、归档降权和活动时间排序。
- `crates/ui/src/shell/spaces.rs`：项目过滤器、搜索时隐藏 All projects、固定 New project 底行、离线图标、设备→位置→目录选择、路径编辑及目录面包屑。
- `crates/ui/src/files/search.rs` 与 `files/tree.rs`：文件名搜索的祖先层级、27px 行高、14px 缩进、200 项上限、200ms 节流、搜索键盘与拖动。
- `crates/ui/src/theme.rs`、`popover.rs`、`surface_chrome.rs`：浅色/深色 scrim、选中中性 wash、文字命中底色、10px 结果圆角、24px 页签、键帽与逻辑尺寸。当前页签实现高度仍为26px，待对齐下列未完成项。
- 官方 MyGo `ui.List`、`Tree`、`TextInputBase`、`DialogBase`、`Popover`、`RichText`、`HandleInput` 与原生 editor `GoTo`。没有嵌入网页实现这些界面。

## 已实现且有回归覆盖

| 小功能 | 当前结果 |
| --- | --- |
| 四页 | All / Threads / Commands / Files；点击、Mod+1–4、Tab/Shift+Tab 切换并把输入焦点交还搜索框 |
| 空 All | Recent Files → Quick Actions → Recent Threads；文件与会话预览各5项，近期会话排除当前、归档、子会话 |
| 查询 All | Threads → Commands → Files → In Files；各结果类型独立，文件名显示 basename 与 parent，内容显示路径、行号与文字预览 |
| 命令 | New chat、New project、Open settings、切换到相反主题、Archive thread；主题操作保留面板打开，其余激活后关闭 |
| 线程 | 逐词跨字段评分，标题100、项目40、分支30、设备/PR20，词首1.5倍，归档减半，同分按活动时间排序；全局搜索不受侧栏过滤器影响 |
| 工作区查询 | 通过 typed `WorkspaceSearchService` 查询同一 mock 可变文件集合；当前项目隔离、固定数量、内容每文件3条，UTF-8原文范围、1基行/0基字节列 |
| 打开 | 文件→原生编辑器，内容→对应行列，目录→独立 Files 面板并展开祖先；最近打开文件顺序按会话隔离 |
| 文件树搜索 | 独立200ms原生查询任务，只搜文件名；匹配结果保留实际 DTO 的祖先目录，目录折叠、上下键/Enter、Escape、拖动和上下文菜单 |
| 项目过滤器 | 空查询显示 All projects，非空隐藏；原版单行项目名，设备/路径为 tooltip，离线为 wifi-off 图标；New project 底行始终固定并能键盘选择 |
| 新建项目 | 设备→Home→真实虚拟目录；路径编辑、目录面包屑、目录名过滤、Tab补全、末尾 `/` 下钻、上下/Enter/左键返回、Mod+Enter 添加当前目录 |
| 业务变更 | typed `ProjectCreate` 保留所选设备；后端拒绝离线设备和同设备同路径重复，失败留在选择器；排队载荷 Clone，不引用调用方可变字段 |
| 材质与生命周期 | 工作区与 Files 使用 `design.PanelBackground`，遵循根已解析主题；session tab 仅关闭时触发 `OnCloseSession`，打开/切换不会误清 composer picker |

异步查询由打开、输入、按钮等事件启动；绘制函数不创建业务文件或启动查询。原生结果通过 `Window.Update` 回到 UI 线程，取消和 generation 检查拒绝旧结果。`win == nil` 的 Tester 路径同步查询同一服务，便于确定性测试；它不证明原生任务时序已实机通过。

## 新增接口

定义位于 `internal/contracts/workspace_search.go` 与 `search_ranges.go`：

- `WorkspaceSearchService.SearchWorkspace(ctx, WorkspaceSearchRequest) (WorkspaceSearchResult, error)`。
- `ProjectDirectoryService.ListDirectories(ctx, DirectoryRequest) (DirectoryListing, error)`。
- `Command.ProjectCreate *ProjectCreate`，仅用于 `project.create`，Clone 冻结载荷。
- 搜索结果包含 `FileID/Path/Kind` 或 `FileID/Path/Line/Column/Text/Ranges` 和当前 `Revision`。目录来自 mock 项目路径及虚拟文件树，不枚举真实电脑目录。
- `contracts.SearchRanges` 在原文 UTF-8 字节空间返回合并范围，避免 Unicode 大小写映射改变编码长度后定位错误。
- `workbench.State.OpenFileAt(id, line, column)`：1基行、0基 UTF-8 字节列；line=0 保留原有光标。
- `RevealFile(id)`、`RecentFiles()`、`OnCloseSession func(string)` 为应用接线点。主任务已接子会话关闭回调。

## 验证记录

- 本组 desktop 定向交互测试通过；workbench / contracts / mock 全包测试通过。`go vet` 四包通过。
- `go run ./scripts/check-framework.go` 通过：`.2`，1830文件，SHA-256 `fd197203233f09d5cede40fc69fad6e6bac8f6dc797e1770f1637e4687ab133e`。
- race 默认 CGO=0 首次无法运行；随后使用已有 w64devkit GCC 与临时 CGO=1，workbench/contracts/mock 全包及本组 desktop 定向 race 全部通过。CGO/CC/PATH 在 finally 中恢复，没有改变普通构建环境配置。终态记录在证据目录的 `validation.txt`。
- 四包全量 desktop 测试曾在并行编辑期间出现附件移除两项失败（`TestFilePickerAdapterUsesSameImportPath`、`TestSideNativePickerStagesOnlyChildAttachments`）；本组定向通过不能替代主任务最终全包结果。
- 本组文件未留下 lint 报告项；统一 lint 当次还有其他并行 composer/integrations 文件问题，须由主任务最终统一收口。
- 截图为 MyGo `ui.NewTester` 的实际实现绘制，**没有本轮原版原生截图、原生窗口/输入法/模糊效果/SSIM通过结论**。用户要求停止后未启动应用、Computer Use 或构建发布包。

## 已入仓的证据与复跑

`docs/evidence/command-palette/` 保存17张PNG与相应17个JSON：浅/深四页8张，新项目四步4张，文件树查询浅/深2张，项目过滤浅/深2张，900×600离线错误1张。`manifest.json` 列出源文件与图片哈希。它们是**实现证据**，不能当作参考 golden。

```powershell
$env:ZERON_WORKBENCH_EVIDENCE = Join-Path (Get-Location) 'docs/evidence/command-palette'
go test ./internal/desktop -run 'TestCommandPaletteEvidence|TestProjectPickerEvidence|TestExplorerAndOfflinePickerEvidence' -count=1
go test ./internal/desktop ./internal/workbench ./internal/contracts ./internal/mock -run 'TestCommandPalette|TestNewProject|TestProjectFilter|TestProjectPicker|TestExplorer|TestOpenSearch|TestRevealDirectory|TestClosingSide|TestSearchRanges|TestProjectCreate|TestWorkspaceSearch|TestDirectoryBrowse' -count=1
go vet ./internal/desktop ./internal/workbench ./internal/contracts ./internal/mock
```

## 换机后继续项，均未验收

1. 在同平台、尺寸、DPI、主题、数据下与原版实机逐图对照。卡片磨砂/阴影/边缘淡出、源页签24px、搜索栏/分组/滚动条、文字换行与图标还需要实际像素比较；原生控件主题与跨平台键位需复验。
2. 当前原生文件名/内容搜索合并为一个80ms请求；原版文件名先返回、内容80ms后追加，带独立加载/错误/索引状态。待拆分并验证晚到结果保持 active key，内容范围及文件模糊评分也需与原版算法对照。
3. Files 正常树仍使用官方 eager `Tree`；搜索使用官方虚拟 `List`。长树虚拟化、多选、完整键盘恢复、搜索分支排序/高亮、Git装饰与自动 reveal 滚动未全部复刻。文件改名/保存后搜索结果自动重查尚待补齐。
4. 新项目缺原版挂载卷、Cloud GitHub 仓库/clone、Home 全局文件夹索引、长面包屑折叠菜单、路径补全鬼影。当前路径字段只在目录步出现，Mod+L 在位置步无可见字段；路径字段 Mod+Enter 添加已输入路径、相对路径解析、路径错误保留/重试和 Windows 大小写/分隔符去重需继续收口。不要把这些当成已完成。
5. 项目过滤器尚未按 repository 合并多个设备的同项目；当前路径/设备元数据属于单个 Project。需要完整项目成员、来源设备、原版 fuzzy ranking、上下文管理动作。
6. 窗口关闭/会话切换/快速键入时的原生异步取消、IME候选Enter与长按Enter防重复、焦点恢复、屏幕阅读器，以及 macOS 双架构真实输入仍待原生验收。

没有重新发布包。旧包不包含这批源码，不能据其判断当前 UI，也不能据这些 Tester 图认定“完美复刻”。
