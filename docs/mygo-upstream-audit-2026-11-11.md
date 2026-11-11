# MyGo 上游版本与自建组件收敛审计

> 本文保留升级前的审计判断；后续实施与验收见 [升级记录](mygo-upgrade-2026-11-11.md)，
> 当前固定版本和补丁链见 [mygo-patches.md](mygo-patches.md)。

检查日期：2026-11-11，北京时间。结论来自官方发布、固定 tag 源码和当前 checkout；
此次不修改依赖、框架源码、补丁或产品行为，不构成升级验收。

## 结论

上游最新正式版本为 **v0.8.3**，2026-11-07 **16:56:09 +08:00** 发布，
对应提交 `5add3817c74bb7505d0b473bc5fb28abe2201b99`。GitHub API 的 `releases/latest`
返回该版本，`prerelease=false`。仓库还有更晚的 `native-sqlitecore` 原生模块产物发布，
它不是新的 MyGo 框架版本，不能用来替代框架版本号。

当前项目固定 **v0.7.4 + rcode-native-ui.23**，通过 `replace` 使用本地框架，
源码锁列出 19 项补丁。升级需要重建整条补丁链，不能只改 `go.mod` 中的版本。

- 推荐目标为正式 tag v0.8.3。
- 最有价值的收敛点是 Markdown 链接菜单、引用图标内联排版，以及部分通用菜单。
- 数学内联排版的 `InlineMath` 在 v0.7.4 已存在，是当前即可研究的重复实现，
  不能把它计为新版本收益。
- 新版通用 `Element.TabSize` 是缩减代码块制表位补丁的候选：在应用的
  `CodeBlock` 回调中按语言设置继承样式，可能替代手工空格展开及显示列回源映射。
- 可搜索的双列模型选择器、可移入滚动的链接提示卡、表格导出/放大预览等，
  仍包含框架接口没有等价覆盖的需求，不能整体删除。
- 具体补丁判定另见 [逐项补丁审计](mygo-upstream-patches-2026-11-11.md)。

版本依据：
[官方 v0.8.3 发布](https://github.com/egoist/mygo/releases/tag/v0.8.3)、
[发布 API](https://api.github.com/repos/egoist/mygo/releases/latest)；
本地依据为 `go.mod`、`third_party/mygo.lock.json`、`scripts/frameworkpin/pin.go`。
API 原始响应保存在 `output/mygo-upstream-audit-20261111/`，源码快照为其中的
`upstream-v0.8.3/`。

## 自建组件逐项判断

| 优先级 | 现有实现 | 框架能力与判断 | 边界 |
| --- | --- | --- | --- |
| 高 | `internal/linkui/link.go`、`markdown.go` 中显式 Markdown 链接的自建菜单接线 | 新版 `markdown.Options.LinkMenu` 能在框架渲染的链接上构建“复制路径、内部打开、显示目录”等菜单，减少为菜单而接管整段 Markdown 的必要性。 | 导航策略、来源会话校验与路径解析仍属于 GCode；当前绝对路径行内代码转链接、缩短标签后恢复原路径复制、点状下划线没有被这个接口覆盖。 |
| 高 | `internal/desktop/transcript_prompt.go` 中引用图标占位、手工拆分样式和 `ShapeRichText` 二次排版 | 新版 `Options.LinkIcon` 提供与基线对齐的内联对象，并把图标排除在复制文本外，可承接图标的尺寸、换行和绘制位置。 | `gcode://` 引用的解析、原子编辑和禁用导航语义仍需保留。框架会把它当链接；当前应用将引用显示为不可点击样式，需先验证焦点、点击、复制和无障碍语义，不能直接声称可删整份文件。 |
| 高，需专项验证 | `16-code-block-tabs` 补丁中的 `codeDisplay` 和复制偏移映射 | 新版 `Element.TabSize` 支持按空格宽度设置 tab stop，并从父元素继承。可以评估在 `Options.CodeBlock` 回调的容器上按 `editor.LanguageNamed(...).TabSize` 设置后调用 `DefaultCodeBlock`，源码仍保留原始 tab。这样可能把框架内的展开/回源算法缩减为应用层样式配置。 | 标准 Markdown 没有自动按语言设置该属性，因此直接删除补丁会改变行为。须验证 Go 4、JS/TS/JSON 2、未知语言 4、局部选择复制、流式代码、换行和长行性能；尚未运行替代实现。 |
| 高，但无需升级 | `internal/desktop/transcript_math.go` 的 `mathInlineRuns`、`mathMarkdownBlock` | 当前 v0.7.4 就有 `Options.InlineMath`；可让标准 Markdown 负责内联公式的占位、基线、换行、样式与复制，减少手工遍历和第二次 shaping。 | `internal/mathrender` 的 TeX→SVG 引擎、字体、异步缓存、尺寸限制、失败展示和公式设置仍需保留；链接的专有交互与精确行高要一并验证。`mathInlineRuns` 还有 prompt 调用方，不能先删后适配。 |
| 中 | 自建弹出菜单中的通用开关、定位和单行条目 | `PopoverMenuButtonOptions.At/Self/Open`、`MenuItem.ID/Subtitle/Trailing/Danger/Content/Highlighted`、`Menu.MaxWidth` 覆盖更多常见需求；适用于普通操作列表和菜单行。 | `projectPopover` 有搜索、虚拟列表和固定底部操作；`modelPopover` 有双列布局、搜索、供应商联动；推理选项有滑块；用量卡是信息面板。不能全部换成普通菜单。应通过现有共享控件入口接入。 |
| 中，需行为决策 | `internal/desktop/transcript_motion.go` 的手动折叠滚动策略 | v0.8.1 加入 `ListState.HoldPlace`，并在每次 layout 读取 `FollowEnd`。可避免点击最后一行展开时被强制带到列表底部。 | 现有逻辑是暂时取消 140ms 跟随动画后继续跟随，`HoldPlace` 是保住锚点、直到尾部重新可见才恢复跟随，两者不等价。若保持当前交互，不能直接用一次调用替换整个状态对象。 |
| 低，部分可收敛 | `internal/workbench/tabs_tooltip.go` 的标签提示状态机 | 新版 `Theme.TooltipDelay` 可配置停留时长，`TooltipBase` 管理焦点、Escape、嵌套优先级和相邻控件提示。 | 现有 tab 与关闭按钮分别 350/500ms，使用点定位、按槽位捕获内容及特定输入取消规则；新版普通 tooltip 使用锚点定位和 warm-up 策略。保持这些行为时仍需状态。 |
| 暂不替换 | `internal/linkui/link.go` 的可滚动目的地址提示卡 | `Options.Preview` 可提供内容，但内部仍走 `TooltipBase`。 | v0.8.3 的 `TooltipBase` 创建 `IgnoreInput(true)` 提示层；现有卡片允许指针移入、180ms 离开宽限和滚动最长 24 行内容。不能以普通 hover preview 等价替换。 |
| 暂不整体替换 | `internal/desktop/transcript_table.go` | 新版标准 Markdown 表格仍是横向滚动的网格；`Element.Sticky` 可作为后续简化局部定位的候选。 | 当前 CSV 导出、Markdown 复制、扩宽至会话视口、独立原生预览、固定表头及避让输入框的横向滚动条均不由标准表格统一提供。Sticky 的最近滚动容器约束也不同，未验证替换。 |

上述是源码级可行性判断，不是“已完成替换”的清单，也没有估算未经实现验证的删行数量。
落地还必须更新 `internal/desktop/transcript_links.go` 的 `transcriptOptions`：
它逐字段传递 Markdown 配置，目前不传递 `InlineMath`，也没有新版的
`LinkMenu`、`LinkIcon`、`Resolve`、`Preview`。只给 `a.md` 设置回调不会自动作用于
每个会话的实际 options；需要同时核对助手消息、用户消息和文件预览入口。

主要上游源码依据均固定到 v0.8.3 提交：

- [Markdown Options、LinkMenu、LinkIcon、Preview、Resolve](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/markdown/view.go#L17-L112)。
  `Resolve` 只解析 mentions 和 issue/PR references，不是把任意 inline code 转换为文件链接的通用回调。
- [菜单条目能力](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/menu.go)、
  [PopoverMenuButtonOptions](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/popovermenu.go#L134-L162)。
- [HoldPlace 的真实恢复条件](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/list.go#L339-L345)。
- [TooltipBase 的输入穿透与锚点定位](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/tooltip.go#L37-L94)、
  [TooltipDelay 与 warm-up 的关系](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/theme.go#L137-L151)。
- [标准 Markdown 表格](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/markdown/table.go)。
- [TabSize API](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/element.go#L1775-L1787)、
  [父级继承测试](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/textstroke_test.go#L105-L144)。
- [v0.7.4 已有 InlineMath](https://github.com/egoist/mygo/blob/9bd34e1eb9525d9a1fe698aadc964c49c8ff8e7e/ui/markdown/view.go#L50-L80)。

## 升级成本与执行顺序

1. 先以 v0.8.3 官方源码为底重建补丁，删除已完整收录的补丁，对部分能力只保留缺口；
   同步版本身份、补丁摘要、源码锁和检查脚本。
2. 编译并完成框架与 GCode 回归，再分功能收敛自建实现。先处理标准 Markdown 的菜单接线；
   然后处理公式和引用图标，最后再评估菜单、tooltip 与滚动行为。
3. 每个功能点单独验证与提交。此次检查没有恢复暂停的 UI 视觉一致性优化。

跨 v0.8.0 的注意点来自正式发布说明：

- `HandleInput`/`ObserveInput` 现在接收更多按键，包括只有物理位置、
  `KeyUnknown` 的修饰键事件；检查快捷键录制、输入框与终端处理器是否吞掉未使用按键。
  当前 `recordShortcut` 对无法生成名称的 keydown 返回 true，是升级时要复核的具体入口。
- 主键拖拽、取消按压和不活动窗口的强调色语义有变化，应复验分割条、tab 拖拽和焦点状态。
- v0.8.3 默认 Markdown parser 新增 alerts、mentions、references。项目自行遍历 inline
  和拦截 block 的代码需要覆盖新增类型，不能只确认编译通过。
- v0.8.1/v0.8.3 另有 GPU 纹理与 clip mask 回收修复；这是升级的额外理由，
  但本次没有测量 GCode 的 CPU/GPU 或内存变化，不引用上游 benchmark 数字作为本项目收益。

依据：[v0.8.0 正式变更](https://github.com/egoist/mygo/releases/tag/v0.8.0)、
[v0.8.1 正式变更](https://github.com/egoist/mygo/releases/tag/v0.8.1)、
[v0.8.3 正式变更](https://github.com/egoist/mygo/releases/tag/v0.8.3)、
[官方迁移指南](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/docs/ui/migration.md)。
新版与当前框架的 `go.mod` 都声明 Go 1.27.3；未发现需要为了这个版本先改 Go 版本的依据。

## 已运行的核查

在下载的 v0.8.3 源码目录运行：

```sh
go test ./ui ./ui/markdown \
  -run '^(TestListHoldPlace|TestPopoverMenuButtonPlacement|TestPopoverMenuButtonOpenState|TestViewLinkMenu|TestViewPreview)$' \
  -count=1
```

两个包通过，覆盖上游的保持滚动位置、菜单定位/受控开关、Markdown 链接菜单和预览用例。
日志为 `output/mygo-upstream-audit-20261111/upstream-capability-tests.log`。
这只证明下载版本的这些框架能力通过其自带测试，不证明 GCode 完成升级或跨平台原生验收。
另运行 `go test ./ui ./ui/markdown -run '^(TestTabSize|TestLinkIcon.*)$' -count=1`，
两个包通过，日志为同目录的 `upstream-tab-and-icon-tests.log`；这也没有验证
GCode 代码块补丁的替代实现。

在当前 `third_party/mygo` 运行 `go test ./ui/markdown -run '^TestInlineMath' -count=1`
亦通过，确认数学内联对象并非必须升级才能使用。
`go run ./scripts/check-framework.go` 通过；当前框架源码锁保持有效，
日志为 `output/mygo-upstream-audit-20261111/current-framework-check.log`。

## 补丁链结果摘要

- 可整条移除：07（两端对齐初始化）、14（旧 benchmark 闭合括号）。
- 需拆分或改写：02、05、12、16。05 的 UIA provider **生产修复**已收录，但本地
  COM/focus 测试和契约注释未完整收录；不能把“生产修复已存在”直接说成整条补丁都能删。
- 其余 13 项仍保留缺口，其中 17 是随本地 API 重新生成的包装，19 是项目专属发布身份。

“直接可删两项”主要减少补丁维护条目，不等于已经减少了两个大型自建 UI 组件；
真正的组件代码收敛仍以本报告的 Markdown、代码块和菜单候选为主。
