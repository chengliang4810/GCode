# MyGo v0.8.3 上游补丁逐项审计

> 本文保留升级前的逐项判断；最终 16 项补丁链见 [mygo-patches.md](mygo-patches.md)，
> 产品替换与验收见 [升级记录](mygo-upgrade-2026-11-11.md)。

检查日期：2026-11-11，北京时间。审计对象是当前固定的 MyGo v0.7.4 + `rcode-native-ui.23` 补丁链，目标是官方最新正式 release v0.8.3。此次只做源码语义比对，不修改 `third_party/mygo`、`patches/`、源码锁或产品代码，也没有把 dry-run 冲突当作已吸收证据。

## 版本与来源

官方 release API 快照 `output/mygo-upstream-audit-20261111/release-latest.json` 显示最新正式 release 为 **v0.8.3**，`prerelease=false`，`published_at=2026-11-07T08:56:09Z`，即北京时间 **2026-11-07 16:56:09**。官方 tags API 快照 `output/mygo-upstream-audit-20261111/tags.json` 显示 tag `v0.8.3` 指向提交：

`5add3817c74bb7505d0b473bc5fb28abe2201b99`

源码快照位于 `output/mygo-upstream-audit-20261111/upstream-v0.8.3`。所有上游链接均固定到上述 SHA：

- [v0.8.3 release](https://github.com/egoist/mygo/releases/tag/v0.8.3)
- [固定提交源码树](https://github.com/egoist/mygo/tree/5add3817c74bb7505d0b473bc5fb28abe2201b99)
- [release API](https://api.github.com/repos/egoist/mygo/releases/latest)
- [tags API](https://api.github.com/repos/egoist/mygo/tags)

补丁编号和名称对应升级前提交 `fe336ef` 的 `patches/mygo-v0.7.4-*.patch`；后续升级删除或重建这些文件时，原始差异仍可从该提交读取。

## 判定规则

- **完整吸收可删**：上游已提供同一行为或同一修复，当前补丁没有需要保留的独立语义。
- **部分吸收需改**：上游已有相邻底层能力或部分生产行为，但没有覆盖当前补丁的完整公开 API、交互语义、平台路径或回归约束；升级时需拆分、重放或重写。
- **仍缺失**：上游没有等价能力，或只有名称相近但行为不同的实现；补丁不能因为存在相邻 API 就删除。

确信度表示源码证据对该判定的把握程度，不代表真实 Windows、IME、WebView2、ClearType 或原生窗口运行验收已经完成。

## 19 项逐条判定

| # | 补丁 | 状态 | 语义依据与结论 | 确信度 |
|---:|---|---|---|---|
| 01 | `motion` | **仍缺失** | 补丁新增窗口级 `MotionPreferences`、`SetMotionPreferences`、`SmoothScrolling`，并让非精确滚轮及在途缓动读取有效覆盖。v0.8.3 的滚轮仍直接按 `!precise && !rt.preferences().ReduceMotion` 决定缓动；`Preferences` 只有系统只读值，没有窗口覆盖类型或设置入口。见 [`input.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/input.go#L725-L765)、[`preferences.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/preferences.go#L7-L90)。 | 高 |
| 02 | `markdown-line-height` | **部分吸收需改** | v0.8.3 已有通用 `Element.FixedLineHeight`，可承接基础文本布局；但 `markdown.Theme` 仍只有 `LineHeight`、`CodeLineHeight`，`HeadingStyle` 没有 DIP 固定行高，Markdown 代码块仍只调用倍数 `LineHeight`。因此 `FixedLineHeight` 底层接线可重用，Markdown 专用字段、标题/列表/代码传播和回归约束仍需保留。见 [`element.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/element.go#L1668-L1681)、[`markdown/theme.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/markdown/theme.go#L20-L102)、[`markdown/code.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/markdown/code.go#L145-L176)。 | 高 |
| 03 | `snap-to-window` | **仍缺失** | v0.8.3 新增的是 `AttachEdgeMargin`，仍由 `AttachTo` 的 `alongTarget` 先尝试反向锚点，再按窗口边距夹紧；当前补丁要求显式 `SnapToWindow`、不翻转锚点、按最终 margin box 贴边、超大盒保持自然尺寸。相邻能力改变了默认边距，但不等价于补丁语义。见 [`element.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/element.go#L1214-L1280)、[`layout.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/layout.go#L1331-L1417)。 | 高 |
| 04 | `input-display-text` | **仍缺失** | 上游有私有 `editor.displayText()`，供输入控件内部把 IME 预编辑插入显示文本并在密码模式转为圆点；没有公开 `Element.InputDisplayText()`，应用无法读取同一快照而不提交预编辑或改变绑定值。见 [`editor_layout.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/editor_layout.go#L37-L63)。 | 高 |
| 05 | `windows-uia-provider` | **部分吸收需改** | 生产行为已被上游吸收：`get_ProviderOptions` 现在直接返回 `0x2 | 0x20`，即 `ServerSideProvider | UseComThreading`。但补丁中的 `AccessNode.Bounds` 契约注释以及 UIA COM provider/focus 回归测试不在上游，且上游 benchmark 已单独闭合。升级时可删除生产实现 hunk，保留或重写本地测试与注释约束。见 [`surface_access.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/internal/windows/surface_access.go#L821-L840)、[`access.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/internal/platform/access.go#L25-L46)。 | 高 |
| 06 | `richtext-composer` | **仍缺失** | 补丁新增富文本 `OnKey`/`OnPaste` composer 钩子、`EditorState` 显示文本和内容高度快照、固定行高接线，并让附件复制使用附件名称。v0.8.3 的 `EditorOptions` 没有这些回调或状态对象，输入路径直接处理 key/text；`PlainText()` 仍把附件的 object replacement 直接删除。见 [`richtext/view.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/richtext/view.go#L17-L46)、[`richtext/input.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/richtext/input.go#L13-L55)、[`richtext/clipboard.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/richtext/clipboard.go#L181-L193)。 | 高 |
| 07 | `richtext-usejustify` | **完整吸收可删** | v0.8.3 的 `shapeTextAligned` 已在 `shape.Justify` 时调用 `text.Shared().UseJustify()`，与补丁增加的调用完全一致。见 [`text_layout.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/text_layout.go#L148-L167)。 | 高 |
| 08 | `inline-link-selection` | **仍缺失** | 补丁要求 `RoleLink` 的主键按下在越过拖拽阈值后，把同一按压交给可选择段落，保留链接点击/键盘/菜单行为但拖拽复制原文。v0.8.3 的 `textSelectionAt` 只从 hit chain 找可选择段落并命中其文本，没有 `dragInlineLinkSelection` 的按压转移；链接仍可独立拥有按压。见 [`textselection.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/textselection.go#L195-L225)。 | 高 |
| 09 | `collapsible-animation` | **仍缺失** | 补丁公开 `CollapsibleBaseWithAnimation`，允许折叠面板使用独立 `Animation`（应用当前为 140ms），同时保留 reduced motion 和 `DisableAnimations` 语义。v0.8.3 的 `coreCollapsibleBase` 仍固定 `200*time.Millisecond`，公开 API 只有 `CollapsibleBase`。见 [`collapsible.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/collapsible.go#L28-L45)、[`api_gen.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/api_gen.go#L1179-L1195)。 | 高 |
| 10 | `webview-data-directory` | **仍缺失** | 补丁提供 `Application.SetWebViewDataDirectory`，在 `Run` 前设置应用级持久 WebView2 profile。v0.8.3 默认按 `LOCALAPPDATA/<app>/WebView2` 生成目录，并支持进程启动时的 `WEBVIEW2_USER_DATA_FOLDER` 环境变量；这不是同一应用内的公开 setter，无法替代当前 API 的生命周期和绝对路径校验。见 [`windows/app.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/internal/windows/app.go#L180-L207)、[`windows/webview2.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/internal/windows/webview2.go#L322-L337)。 | 高 |
| 11 | `windows-glyph-placement` | **仍缺失** | 补丁新增 Windows ClearType quarter glyph policy、`RasterGlyph` 入口、quarter position/cache 和外部覆盖率 oracle。v0.8.3 的绘制仍调用 `sys.Glyph`，Windows DirectWrite 只按已有 native/6/8/4 个位置粒度处理；没有 `GlyphPlacement`、`WithGlyphPlacement` 或 quarter 绘制路径。见 [`paint.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/paint.go#L969-L1005)、[`dwrite_windows.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/internal/text/dwrite_windows.go#L1187-L1235)。 | 高 |
| 12 | `rendering-parity` | **部分吸收需改** | 通用点状下划线和 painter 的 dotted 绘制已在 v0.8.3 存在；但 `coreLink` hover 仍调用 `Underline()` 并设置 `solid=true`，行为不是当前补丁要求的点状链接下划线。补丁同时为 quarter glyph 路径传递 dotted，而 11 号 quarter 路径本身仍不存在。见 [`widgets.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/widgets.go#L188-L214)、[`element.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/element.go#L1675-L1705)、[`paint.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/paint.go#L1257-L1281)。 | 高 |
| 13 | `tooltip-primitives` | **仍缺失** | 补丁新增点坐标定位、设备缩放、按需 `InputActivitySnapshot`、窗口输入事实查询、主键按下归属和 Windows pointer-move button tag。v0.8.3 的 `TooltipBase` 仍按锚点 `AttachTo` 定位并 `IgnoreInput(true)`；输入公开查询只有 `Pressed`、`PressedButton`、`PressedPointer` 等当前控件事实，没有补丁要求的快照/归属 API。见 [`tooltip.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/tooltip.go#L37-L95)、[`input.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/input.go#L1550-L1603)。 | 中高 |
| 14 | `uia-benchmark-close` | **完整吸收可删** | v0.8.3 的 `BenchmarkUIATreeUpdate` 已完整关闭 `b.Run` 和函数体，后续测试从独立函数开始；该补丁只补齐的语法收尾已不再需要。见 [`surface_access_test.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/internal/windows/surface_access_test.go#L193-L219)。 | 高 |
| 15 | `selection-spans` | **仍缺失** | 补丁把每个段落的文本和范围持久化为 span，使虚拟化列表滚出构建范围后仍可绘制和复制。v0.8.3 的 `textSelection` 仍只有 anchor/caret 端点；端点不可见时会清空选区，`selectedTexts` 直接遍历当前构建的 `rt.texts` 并读取当前 editor source，没有持久 span 快照。见 [`textselection.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/textselection.go#L14-L25)、[`textselection.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/textselection.go#L159-L189)、[`textselection.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/textselection.go#L398-L421)。 | 高 |
| 16 | `code-block-tabs` | **部分吸收需改** | v0.8.3 已有通用 `Element.TabSize`、内部 `text.Style.TabSize` 和编辑器按语言读取 `Language.TabSize` 的能力；但 Markdown code block 仍把源码行直接放进 spans，没有按语言设置 tab size，也没有当前补丁的 `codeDisplay` 显示列映射、`codeSourceAt` 选区回源偏移和复制保留 tab 语义。见 [`element.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/element.go#L1775-L1787)、[`editor.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/editor/editor.go#L286-L295)、[`markdown/code.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/markdown/code.go#L145-L258)。 | 高 |
| 17 | `api-gen` | **仍缺失** | 该补丁是前述本地 API 的生成包装，不是独立业务语义。v0.8.3 的 `api_gen.go` 可看到 `CollapsibleBase`、`AttachTo` 等基础包装，但没有 `DeviceScale`、`InputActivity`、`SetMotionPreferences`、`CollapsibleBaseWithAnimation`、`AttachAtPoint`、`InputDisplayText`、`SnapToWindow`；生成器本身存在不等于这些本地接口已被上游收录。见 [`api_gen.go` 的 CollapsibleBase](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/api_gen.go#L1179-L1195)、[`api_gen.go` 的 AttachTo](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/api_gen.go#L3614-L3654)。 | 高 |
| 18 | `split-drag` | **仍缺失** | 补丁要求命中带至少 20 DIP、通过 `draggedBy` 在 pane 构建前写入尺寸，使同一 pass 的子布局使用新尺寸，并且 pointer drag 不触发 `Changed`。v0.8.3 仍使用主题 `1.5` DIP grip、普通 `Dragged()`，在 pane 已构建后由 `afterInput` 调 `set`，并将拖动标记为 changed/consumed。见 [`split.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/split.go#L74-L239)、[`input.go`](https://github.com/egoist/mygo/blob/5add3817c74bb7505d0b473bc5fb28abe2201b99/ui/input.go#L1719-L1737)。 | 高 |
| 19 | `patch-identity` | **仍缺失（项目专属）** | `patch_identity.go` 提供 `FrameworkPatch()`、`rcode-native-ui.23`、源码树 SHA-256 和 release linker stamp，是 GCode 的框架锁定与发布校验契约。v0.8.3 上游源码树没有对应文件或 `FrameworkPatch` API；这项不应期待上游吸收，升级时应由 GCode 重新生成新的本地身份。见 [固定 v0.8.3 源码树](https://github.com/egoist/mygo/tree/5add3817c74bb7505d0b473bc5fb28abe2201b99)。 | 高 |

## 可直接删除与必须保留的范围

在不重写语义、不做完整升级的前提下，当前有充分源码证据可以整条删除的只有：

- **07-richtext-usejustify**：调用已逐字存在于上游。
- **14-uia-benchmark-close**：benchmark 函数闭合结构已完整存在于上游。

第 05 项不能整条删除：`ServerSideProvider | UseComThreading` 的生产行为已吸收，但本地 `AccessNode.Bounds` 契约说明和 COM/focus 回归测试仍是本地约束。第 02、12、16 项应保留缺口并在升级时重写，不能因通用 `FixedLineHeight`、`DottedUnderline` 或 `TabSize` 出现就整条删除。第 17 项必须等其依赖 API 完成取舍后重新运行 API 生成器，不能单独按补丁冲突结果处理。第 19 项属于 GCode 发布身份，应继续保留并更新版本、修订号和源码指纹。

第 16 项另有缩减实现的候选路径：`Element.TabSize` 可从父容器继承，应用可在
`Options.CodeBlock` 回调中按语言设置 tab size 后调用 `DefaultCodeBlock`，
让框架直接排版原始 tab，避免先展开空格再映射回源。此路径尚未实现验证，
不改变上表“不能直接删原补丁”的判断；若通过各语言、流式、长行与局部复制回归，
则可以把这项框架补丁改成较小的应用接线。详见
[组件收敛审计](mygo-upstream-audit-2026-11-11.md)。

## 未验证点

- 本报告没有把 19 个补丁重放到 v0.8.3，也没有运行 GCode 对 v0.8.3 的编译、测试或原生桌面 E2E；判断依据是固定 tag 的源码语义。
- Windows UIA provider 的 `0x22` 生产行为已经通过源码确认，但本次没有 Windows COM 客户端、焦点切换或无障碍树运行验证。
- 输入显示、富文本 composer、tooltip/input activity、WebView2 目录和 split 同帧行为没有在真实窗口或 IME 下运行验证。
- ClearType quarter 定位没有 Windows 设备上的像素/覆盖率验证；“仍缺失”指固定 tag 中没有对应源码/API，不代表当前补丁在所有设备上的视觉结果已经重新测量。
- 19 项状态只回答“哪些本地补丁可以从升级补丁链删除、哪些语义必须保留”，不等于已经完成 v0.8.3 升级。
