disposition: fix

## persistence

- 文档基线一致：`PRODUCT.md` 和 `DESIGN.md` 都把 Go + mygo 原生桌面、中文三栏、深浅主题、窄窗收起资源面板和 T3 暗黑截图列为当前视觉约束；`docs/verification/visual-v2.md` 仍明确写着“实现中，未验收”。本轮不能标记为交付通过。
- 主题实现大体遵循文档：`internal/desktop/theme.go:30-73` 使用近黑/中性浅色背景、细边界、灰色选中态、蓝色操作色和绿/红状态色；`app.go:307-309` 按设置将主题真正写入原生 UI。宽屏截图的三栏比例接近 `DESIGN.md` 的目标，窄窗会按 `app.go:311-316` 收起右栏并保留顶部展开控件。
- 证据可追溯：`output/acceptance/visual-v2/round-1/build-manifest.json` 记录了 source fingerprint `2ef417132566bbe5c877563a1c4c0154a99776dc8e7fe7f1b4ae108d2e5e83fa`、binary SHA `d36a0dcc1b43c265bf553b3167afeaf8dad0b2e9b26fcb30bcf43add61d904ad`、Go `1.27.2 darwin/amd64` 和 macOS `14.8.7`。`captures.json` 中七张 native 图的 SHA 与实际文件逐一一致。
- 缺失单独记录：仓库没有发现供设计持久化使用的 DESIGN JSON；现有 `mygo.json` 和 `third_party` 下的 JSON 是构建/依赖配置，不是视觉设计记录。按任务约定留给后续 documenter 补齐，不把它当成本轮实现缺陷。

## fidelity

| 主体元素 | 判定 | 图证与源码依据 |
|---|---|---|
| 原生窗口边界与 mac trafficlights | match | 七张 native 图都保留左上 trafficlights，标题栏控件没有压住红黄绿按钮；`layout.go:61-83` 和 `settings_page.go:12-39` 使用 `TitleBar` 计算内边距。 |
| 宽屏三栏工作台 | match | `dark-1440x900-native.png`、`light-1440x900-native.png` 显示项目/会话、正文/输入、资源/Diff 三栏；`app.go:323-345` 真实构建两个原生 Split，右栏最小宽度来自 `layout.go:193-207`。 |
| 深色主体层级、直接助手正文、低调用户气泡 | match | `dark-1440x900-native.png` 和 `dark-1280x800-native.png` 中助手内容直接落在正文面，用户消息为中性圆角面，工具行紧凑折叠；`conversation.go:141-177`、`conversation.go:179-243` 与 `theme.go:34-51` 对应 T3 的主要组织方式。 |
| 底部 composer 与真实操作控件 | match | 五张工作台图都显示文本输入、纠正/队列、模型、完全访问、停止/发送按钮；`conversation.go:404-467` 是真实 TextArea、Segmented、Select、Checkbox 和事件处理，不是静态截图。 |
| 运行中 composer 的停止/发送双按钮 | adaptation | `dark-1440x900-native.png`、`dark-1280x800-native.png` 中同时出现停止和发送圆钮；这不判为范围外业务缺陷：`PRODUCT.md` 规定“纠正”会等待当前任务停止后发送，`conversation.go:454-463` 的两个按钮分别表达停止当前运行和提交纠正。保留双按钮，但后续应让两者的状态/间距更明确。 |
| Diff 文件/hunk/行号/增删着色 | adaptation | `dark-1440x900-native.png` 和 `light-1440x900-native.png` 能看到文件分组、`+4/-1`、独立行号、hunk 和语法颜色；`diff.go:68-157` 实现了这些。右侧保留 GCode 已确认的面板式 Diff，而不是照搬 T3 的全部顶部工具栏，属于范围内适配。 |
| 工具行摘要 | contradicted | `dark-1024x700-native.png`、`dark-1280x800-native.png` 的“运行测试”行直接显示 `{"cmd":"go test ...","workdir":...}`；`conversation.go:288-303` 只对名称为 `exec_command`/`shell`/`bash`/`sh` 的工具提取命令，而样例名称是“运行测试”，因此真实用户可见层暴露了内部 JSON。 |
| Select 控件表面 | adaptation | composer 的模型 Select 和设置页 API 类型 Select 是真实原生控件，但 `dark-1280x800-native.png`、`settings-light-native.png` 可见默认边框比 T3/Droppy 的轻量 capsule 更重；这属于当前视觉 ceiling，不能误判成需要删除 Select 的业务要求。 |
| 窄窗资源面板行为 | adaptation | `dark-1024x700-native.png`、`light-1024x700-native.png` 中右栏收起，标题栏保留面板按钮，符合 `DESIGN.md`；但左栏仍固定约 240 DIP，窄窗占比明显变成约 23%，不是文档期望的 17–19%。 |
| 设置页的紧凑表单与层级 | contradicted | `settings-light-native.png`、`settings-dark-native.png` 显示横跨全窗的大空白 section、外层 section 面内再套 provider editor 面；`settings_page.go:78-96`、`settings_page.go:191-196` 明确产生这组嵌套边框/圆角，违背 `DESIGN.md` 关于避免每个组件都包卡片的约束。 |
| 初始可读的 Diff 内容 | missing | `dark-1280x800-native.png` 右栏多行代码在窗口右边被截断，截图没有明显的水平滚动提示；`diff.go:103-115` 和 `diff.go:121-155` 强制 `MinWidth(760)` 并对代码 `NoWrap`，视觉上没有达到参考图中“打开就能读”的完成度。 |
| 短 Diff 的折叠交互 | contradicted | `diff.go:72` 对不超过 200 行的 Diff 每一帧都设为打开，尽管 `diff.go:95-96` 会记录用户折叠结果；因此点击折叠后下一次重绘会重新打开。这是源码确认的控件行为缺陷，不是截图猜测。 |

## ceiling

当前已超过旧样板的基础完成度，但距离用户给出的 T3/Droppy 交付底线仍有明确上限：

- 宽屏工作台的骨架、暗色调、真实内容和交互位置已经可信；但设置页仍像一张拉满窗口的表单画布，`settings-light-native.png`/`settings-dark-native.png` 中“外观”和“发送方式”每行只占左侧一小段，右侧留下大面积无语义空白，模型服务又叠加第二层面板。这是密度、层级和组件表面没有收敛，不是换一个黑色背景就能解决的问题。
- 两张设置图底部的“保存设置”操作只露出上半部，按钮被视口底边截断。用户第一次打开页面无法确认主要保存动作，说明滚动内容与操作区没有形成完成的原生布局。
- `light-1024x700-native.png` 和 `light-1440x900-native.png` 的代码块复制图标几乎与深色代码面同色；正文中的 `%w` 和 `settings_controller.go` 还出现黑色选区矩形。`markdown.go:497-540`、`markdown.go:586-610` 只调用通用 `Selectable`/`iconButton`，没有为代码面提供可读的选择和控件状态，这直接破坏浅色主题的精致感。
- `dark-1280x800-native.png` 的 Diff 代码默认截断在右边，长行需要用户自行发现不可见的水平滚动；右侧面板是主体内容时，这种首屏不可读会把“有 Diff”降成“有一个代码框”。
- `dark-1024x700-native.png` 和 `light-1024x700-native.png` 的左栏固定宽度压缩正文，虽然没有重叠或越界，却让窄窗布局不像经过断点设计。原生 trafficlights、标题和面板按钮本轮没有发现相互遮挡，问题集中在内容布局和状态细节。
- 会话表头固定 50 DIP 却放入标题、路径和分支三行，四张工作台图里标题组都贴近窗口上边缘；`conversation.go:104-126` 与按 `TitleBar` 高度适配的 `layout.go:61-75` 不一致。它未造成 trafficlights 重叠，但降低了原生窗口的完成感。
- 工具摘要把内部 JSON 暴露在工作流正文中；这种“能看见数据”不等于“真实内容呈现完成”。同时，composer 的双按钮语义本身符合纠正模式，不应为了接近参考图而删掉停止或发送。

## material_fixes

1. 修正短 Diff 的折叠状态：把“默认展开”和“用户明确折叠”分成两个状态，重绘时尊重 `false`，并为首次出现的文件设置默认值。绑定 `internal/desktop/diff.go:68-96`；源码风险已由 `dark-1280x800-native.png` 的 Diff 主视图定位。
2. 重做设置页的表面层级：给内容设置合理的最大阅读宽度或两列约束，去掉 `settingsSection` 外层与 provider editor 的重复面板，保留输入框和选中服务的必要边界。绑定 `internal/desktop/settings_page.go:78-96`、`:191-196`；对照 `settings-light-native.png`、`settings-dark-native.png`。
3. 让设置操作区完整可见：增加滚动内容底部安全间距，或将“保存设置/保存服务/删除”放在与滚动区分离的原生 footer，并验证首次进入和滚动到底两种状态。绑定 `settings_page.go:40`、`:97-109`；对照两张 settings native 图底部被切断的按钮。
4. 修复浅色代码面的控件和选区对比：代码块复制按钮使用 `codeText` 或专用反色按钮样式，明确设置可读的文本选区颜色并覆盖链接/内联代码状态。绑定 `internal/desktop/markdown.go:497-540`、`:586-610` 与 `widgets.go:13-20`；对照 `light-1024x700-native.png`、`light-1440x900-native.png`。
5. 为 Diff 长行提供可发现且不影响首屏阅读的溢出策略：优先让常见行适配可视宽度，确需横向滚动时显示稳定的滚动提示/控件并保留行号；复核 `MinWidth(760)`、`NoWrap` 和滚动容器之间的关系。绑定 `internal/desktop/diff.go:103-155`；对照 `dark-1280x800-native.png`。
6. 给窄窗增加左栏断点或可控压缩范围，使 1024 宽度下正文保留更多可读宽度，同时保留右栏显式展开按钮。绑定 `internal/desktop/app.go:74-85`、`:310-345` 和 `layout.go:193-207`；对照 `dark-1024x700-native.png`、`light-1024x700-native.png`。
7. 将工具摘要转换成面向用户的短命令/路径摘要，并保留原始 JSON 只在展开面板和复制动作中出现；不要按工具名称猜测，至少覆盖当前真实工具名称“运行测试”。绑定 `conversation.go:212-223`、`:288-303`；对照 `dark-1024x700-native.png`、`dark-1280x800-native.png`。
8. 统一工作台各栏与原生 TitleBar 的垂直基线，并减轻常规 Select 的默认边框：会话栏应容纳标题/路径/分支三行，Select 应沿用 composer 的 surface、border、radius 语义；保留运行时停止/发送双按钮及其产品语义。绑定 `conversation.go:104-138`、`:430-463`、`layout.go:61-75`、`right_panel.go:17-32`、`settings_page.go:154-169`；复核所有宽/窄 native 图。

## keep

- 保留 Go + mygo 原生渲染和真实事件链；不要引入 HTML、JavaScript、WebView，也不要用截图替代控件。
- 保留已经成立的三栏职责：左侧项目/会话，中间正文与 composer，右侧文件/Diff/终端；窄窗收起右栏并保留展开入口的行为继续沿用。
- 保留暗色近黑、中性灰选中、蓝色操作、绿/红状态的语义调色方向，以及代码等宽字体、语法色和独立行号。
- 保留助手正文无外层大卡片、用户消息轻量容器、工具默认紧凑折叠、composer 单一底部表面的组织方式；这些是当前最接近 T3/Droppy 的主体元素。
- 保留 native trafficlights 与窗口边界的当前处理；本轮没有发现窗口控制和应用内容重叠。
- 保留明确的“示例数据”标记和当前截图中的示例内容归属；不要把样例截图当成真实厂商 API、Apple Silicon 或 Windows 验收证据。
- 不扩展订阅账号、Snoozed、Turn 历史、PR、Hydra 等参考产品业务；本轮修复只处理已确认的视觉、布局和控件行为。


---

disposition: fix

## persistence

- round-2 原生验收使用的构建记录为 `output/acceptance/visual-v2/round-2/build-manifest.json`：source fingerprint 为 `356c2e9b00492573d92733120fa5d04cfccc4791a8235818d7fd5ca54812a57d`，binary SHA-256 为 `09a771af4e3df146063f3b8a275ce97fdcfd5f1d0340439483584f13037b3711`；运行时为 Go `1.27.2 darwin/amd64`、macOS `14.8.7`。
- 进程/窗口证据保持一致：PID `91799`、CGWindow `21810`，manifest 标记 `source_unchanged_after_capture: true`；没有在本轮重新启动窗口或重新生成实现。
- `captures.json` 的七张 native 截图与实际文件 SHA-256 一致：`dark-1024x700-native.png`=`7c79937ae15ce49754f3d56b5b68ff5ae0e748b7dd623d448682f9c91853751e`，`dark-1280x800-native.png`=`f9ebf6204f2c31c76120358c64f0b2af5ab16f64f5db809cd898f410fd8bd1f3`，`dark-1440x900-native.png`=`c2ce99bbdf288f1678bc18e6d9ef199f820d56a37c9a079c8287d8fb7df084a8`，`light-1024x700-native.png`=`325fab2ab3afc3b491429ede2fe7c56a1803688119611e2ae7df2a7cb5997e74`，`light-1440x900-native.png`=`cddfdf010cd1b004f791cac9d628289c83ac5147222f3aab3fcd7e8db97a1e93`，`settings-dark-native.png`=`17e42ddbeaa95fe82fbcb06028fd0eb7a733728854ffe294dac0805e80aff53c`，`settings-light-native.png`=`d7b3ab37dca1210c1523fb5ebc42b25187b9eb99627a359fcc23665a3ba74299`。
- `diff-collapse.txt` 给出真实原生交互证据：`initial=true`、`after_click=false`、`after_redraw=false`、`restored=true`，并报告 composer 中文输入触发重绘后折叠状态仍保持，剪贴板未被触碰。
- `.impeccable/design.json` 已补齐为 `schemaVersion: 2`，其中真实 token、断点和源码位置已经记录；但其状态仍为 `candidate`，明确等待“用户视觉反馈”。这份记录不能代替用户正式认可，因此本轮 disposition 仍为 `fix`。

## fidelity

| 前轮项目 | round-2 判定 | 截图与最新源码依据 |
|---|---|---|
| 1. 短 Diff 折叠在重绘后恢复展开 | **resolved** | `dark-1280x800-native.png` 的 Diff 文件可折叠；`diff-collapse.txt` 的 AX 结果通过；`internal/desktop/diff.go:72-75` 仅首次未记录时默认展开，`:318-331` 持久化显式状态。 |
| 2. 设置页全宽空白和重复外层卡片 | **resolved** | `settings-light-native.png`、`settings-dark-native.png` 的内容收敛到居中的 980 DIP 列，外观/发送方式密度明显改善；`internal/desktop/settings_page.go:40-49` 使用 `MaxWidth(980)`，模型服务结构位于 `:81-99`。模型编辑器仍保留必要边界，不再把它误判为必须删除的业务面板。 |
| 3. 设置页底部操作被裁切 | **resolved** | 两张 settings native 图都完整显示“保存设置”“保存服务”和删除控件；`internal/desktop/settings_page.go:102-117` 使用独立 54 DIP footer。 |
| 4. 浅色代码块复制按钮和选区不可读 | **resolved** | `light-1024x700-native.png`、`light-1440x900-native.png` 中复制图标和代码文字可辨，`%w` 与 `settings_controller.go` 的选区使用浅色主题对比；`internal/desktop/markdown.go:497-498`、`:603-610` 已绑定主题文本/选区颜色。 |
| 5. 工具行暴露原始 JSON | **resolved** | `dark-1024x700-native.png`、`dark-1280x800-native.png` 显示为“运行测试  go test ./internal/settings/...”等短摘要；`internal/desktop/conversation.go:293-313` 按 JSON 中的命令/路径字段提取摘要，不再按工具名称猜测。 |
| 6. TitleBar 三行内容挤压 | **resolved** | 宽、窄工作台图中标题、路径和分支均位于稳定的表头区域；`internal/desktop/layout.go:11-19` 提供至少 52 DIP 的统一高度，`conversation.go:104-143` 使用同一高度和安全内边距。 |
| 7. Select 边框与表面不统一 | **resolved** | `dark-1280x800-native.png`、两张 settings native 图中的 Select 已有一致的 surface/border/radius 语义；`conversation.go:443-444` 与 `settings_page.go:170-174` 显式设置主题表面、边框和 8 DIP 圆角。默认原生边框仍是次要精致度 ceiling。 |
| 8. 运行中停止/发送双按钮语义不清 | **resolved（保留产品适配）** | `dark-1440x900-native.png`、`dark-1280x800-native.png` 中停止为 neutral、发送为蓝色 primary，区分度足够；`conversation.go:464-472` 保留停止当前运行与提交纠正两个真实动作，符合 `PRODUCT.md` 的“纠正等待停止后发送”语义。 |
| 9. 1024 窄窗品牌文案被截断 | **unresolved，material** | `dark-1024x700-native.png`、`light-1024x700-native.png` 的侧栏标题显示为 `GC...`，用户看不到完整产品名；`internal/desktop/layout.go:83-92` 只压缩字号/宽度而仍渲染单行 `GCode`。这不是功能缺失，但属于明显的窄窗完成度问题。 |
| 10. 1280 Diff 换行后的首屏高度不足 | **unresolved，material** | `dark-1280x800-native.png` 的第一文件 Diff 在面板底缘附近截断，新增的 `return nil` 行没有完整首屏可读；`internal/desktop/diff.go:107-119` 仍按 `renderedRows*22+4` 固定列表高度，而 `:150-157` 的 `RichText` 允许换行，行高与列表高度不一致。1440 图基本完整，说明问题集中在 1280 断点下的动态高度。 |

## ceiling

- round-2 已把工作台骨架、真实原生控件、设置页内容列、footer、浅色代码面、工具摘要和短 Diff 持久状态推进到可复核的水平；宽屏的三栏职责和深浅主题组织没有新的阻断证据。
- 交付底线仍被两个具体视觉问题卡住：1024 窄窗的产品名被截断，1280 Diff 首屏的换行内容被面板边界截断。前者破坏品牌识别，后者破坏主内容的即时可读性；两者都能从 native 截图和当前源码同时定位。
- 其他差异属于适配或主观精致度：Select 的原生边框仍比参考产品重，设置模型编辑器仍是一个边界清楚的编辑面，运行中 composer 保留停止/发送双动作。它们不应继续扩展成 T3 的订阅、搜索、Snoozed、Turn 历史、PR 或 Hydra 业务清单。
- `.impeccable/design.json` 的 `candidate` 状态和“等待用户视觉反馈”仍是验收事实；即使下面两项修复完成，也需要用户确认后才能把设计记录视为正式认可。

## material_fixes

1. **修复 1280 Diff 的动态换行高度（优先级 P1）。** 让 Diff 行的实际绘制高度与列表/滚动容器高度一致，或对需要换行的行采用可测量的行高并保留滚动可见性；验证 `dark-1280x800-native.png` 第一文件的 hunk、增量行和末行都完整可读。绑定 `internal/desktop/diff.go:107-119`、`:150-157`。
2. **修复 1024 窄窗的品牌截断（优先级 P2）。** 在 `176` 至 `min(19% window width, 220)` 的窄窗范围内，为品牌文字提供真实可用宽度，或采用明确的完整图标/文字响应式状态，确保不再出现无意义的 `GC...`；验证 `dark-1024x700-native.png` 和 `light-1024x700-native.png`。绑定 `internal/desktop/layout.go:81-95`。

## keep

- 保留 Go + mygo 原生渲染、真实事件链和 native trafficlights；不要用 WebView、HTML 或截图替代控件。
- 保留左侧项目/会话、中间正文/composer、右侧资源/Diff/终端的职责边界，以及 1180 DIP 以下收起右栏并保留表头展开入口的行为。
- 保留当前深色近黑、浅色中性、蓝色操作、绿/红状态的语义调色，代码等宽字体、语法色和独立行号。
- 保留助手正文无大外层卡片、用户消息轻量容器、工具默认紧凑折叠、composer 单一底部表面；这些仍是最接近参考组织方式的部分。
- 保留短 Diff 的显式折叠状态和运行中停止/发送双动作；round-2 已有 AX 交互证据，不应为追逐参考截图而回退。
- 保留当前明确的“示例数据”标记。manifest、截图和设计记录只能证明本轮 native 证据链，不代表真实厂商 API、Apple Silicon、Windows 或用户正式视觉认可。
- 后续只处理上面两项 material fix，再做同一套 native 截图/AX/哈希复核；不要把本轮修复扩大成参考产品业务范围复制。


---

disposition: ready_for_user_review
readiness: user_review
technical_status: passed
user_visual_acceptance: pending

## closure

- 本轮只复核前一轮确定的两项 material fix，未发现同项功能缺陷残留：`dark-1280x800-native.png` 中短 Diff 的自然内容高度使第一文件的 hunk、增删行、`return nil` 和闭合行完整可读；`dark-1024x700-native.png`、`light-1024x700-native.png` 的窄窗侧栏改为 G 图标加新会话按钮，不再出现 `GC...` 截断。
- 短 Diff 的实现证据为 `internal/desktop/diff.go:109-119`：`renderedRows <= diffVirtualizationThreshold`（阈值 200）时使用自然高度的内容列，并统一置于外层 Diff 纵向滚动中；超过阈值才进入固定高度列表。折叠默认状态仍由 `:74-76` 控制。
- 窄窗品牌实现证据为 `internal/desktop/layout.go:83-95`：1100 以下隐藏文字品牌，G 图标通过 `Label("GCode")` 保留可访问名称；closure 三张目标截图均未出现 `GC...`。

## evidence

- closure manifest 为 `output/acceptance/visual-v2/closure/build-manifest.json`：source fingerprint `54bbd1062cf6642461f8c5b9696dbc3d847d5763abbf5c2c0b72960e968c70b3`，binary SHA-256 `5d6f241f7365a107594e75bc8979c65fef5cc906b92eca4fa092e258bdc0b9d9`，PID `99100`，CGWindow `21818`，`source_unchanged_after_capture: true`。
- 目标 native captures 已与 `captures.json` 核对：`dark-1280x800-native.png`=`ffb5fcabc7e1b1bb329245ac33d60ee6dbe86cad5d03e4b75437448d809f21ed`，`dark-1024x700-native.png`=`39f6cf3c3ddf2605d9bc32d7767c924638297032a656895452a30cac1c68d17d`，`light-1024x700-native.png`=`de2eb8e743d156cafb1e96bc58a516a70e759cd48b84bca946617cfae11eb7c7`。
- closure native QA 日志确认三张目标窗口截图来自同一 PID/CGWindow；本轮没有启动新窗口、重新截图或修改代码。

## decision

- 技术修复状态为 **passed**，因此当前结果可以交给用户做视觉评价；本轮没有同项功能缺陷需要继续修复。
- 用户视觉认可仍为 **pending**。`.impeccable/design.json` 仍是 `candidate` 只表示设计记录等待用户反馈，不构成代码缺陷，也不阻止交付用户评价。
