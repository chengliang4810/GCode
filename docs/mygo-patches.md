# MyGo v0.7.3 固定补丁

2026-10-31：框架自 v0.6.4 整体升级到官方 **v0.7.3**（提交 `6f28d7bb…8016`，
2026-10-27..29 发布），本地修订号 `rcode-native-ui.21`（第 27 项把文本选区改为自带 span 文本快照，第 28 项展开代码块里的制表符，第 29 项让 split 的拖拽在构建窗格前生效并把命中区扩到 20 DIP）。原第 5 项独立安装名补丁由上游
`windows.displayName`/`windows.installDirName` 与 slug 文件命名取代（应用名 GCode 为
ASCII，上游按 `name` 默认值命名全部产物），原第 15 项 automation 反向移植补丁由上游
原生 `plugins/automation` 取代，两者删除；v0.6.4 时期工作树里未补丁化的渲染对齐改动
（LinkWeight、DottedUnderline、`Context.Instant` 等）固化为第 21 项 rendering-parity 补丁。
升级在 v0.7.3 原版上按序重放全部补丁：`ui`、`ui/richtext`、`internal/...` 与根包测试通过
（上游 `cmd/mygo TestLanguagePackaging` 在本机原版同样失败，属环境性问题，与补丁无关）；
应用 `go build`、`-tags e2e`、`go tool mygo vet`、设置组件回归通过。
真机 desktop-e2e（官方 automation 插件）跑通设置、排版、transcript 链接、折叠动效、
prompt picker 等场景；`TestDesktopLocalPromptReferences` 与 `TestDesktopLocalTranscriptActivity`
失败，但用升级前框架（重放树指纹 `03a2c5d1…`）构建的应用 A/B 复测同样失败，
属未提交的 composer/transcript 功能自身问题，与本次升级无关。
原生窗口与视觉验收仍未运行，待后续单独验收。状态：上游为 `github.com/egoist/mygo v0.7.3`。
`go.mod` 只允许该模块替换到 `./third_party/mygo`。
完整框架源码约 17 MB，保留上游 LICENSE、go.mod、go.sum、API 生成器、构建工具、
平台资源和测试；没有修改全局 Go 模块缓存。

按顺序应用的补丁（29 个条目；11–13、21–23 为 v0.7.3 适配中产生的修正条目）：

1. [`mygo-v0.7.3-01-motion.patch`](../patches/mygo-v0.7.3-01-motion.patch)：窗口级动效与滚轮缓动偏好。
2. [`mygo-v0.7.3-02-hover.patch`](../patches/mygo-v0.7.3-02-hover.patch)：原生 hover 回调及构建来源标识；并集合并 v0.7.3 字段清单与 `hoverFn`。
3. [`mygo-v0.7.3-03-files-drop.patch`](../patches/mygo-v0.7.3-03-files-drop.patch)：原生文件拖放事件回调；字段清单并集含 `filesDropFn`。
4. [`mygo-v0.7.3-04-markdown-line-height.patch`](../patches/mygo-v0.7.3-04-markdown-line-height.patch)：Markdown 可选 DIP 精确行高（与上游 `ShapeText` 行高并存）。
5. [`mygo-v0.7.3-05-snap-to-window.patch`](../patches/mygo-v0.7.3-05-snap-to-window.patch)：可选按本轮实际 margin box 贴窗，不自动翻转。
6. [`mygo-v0.7.3-06-placeholder-color.patch`](../patches/mygo-v0.7.3-06-placeholder-color.patch)：当前原生输入框的可选占位色。
7. [`mygo-v0.7.3-07-input-display-text.patch`](../patches/mygo-v0.7.3-07-input-display-text.patch)：只读原生输入显示快照，包含未提交预编辑文本。
8. [`mygo-v0.7.3-08-caret-width.patch`](../patches/mygo-v0.7.3-08-caret-width.patch)：可选 DIP caret 宽度；`api_gen` 与上游 `Element.Cache` 相邻插入缝合。
9. [`mygo-v0.7.3-09-windows-uia-provider.patch`](../patches/mygo-v0.7.3-09-windows-uia-provider.patch)：Windows UIA 服务端 provider；保留上游重写的树更新测试并附加 COM 契约测试。
10. [`mygo-v0.7.3-10-scrollbars.patch`](../patches/mygo-v0.7.3-10-scrollbars.patch)：滚动容器 indicator/命中开关与拖动取消；字段清单并集。
11. [`mygo-v0.7.3-11-notification-context.patch`](../patches/mygo-v0.7.3-11-notification-context.patch)：`Notification.ShowContext` 取消权限等待，退出不遗留协程。
12. [`mygo-v0.7.3-12-richtext-composer.patch`](../patches/mygo-v0.7.3-12-richtext-composer.patch)：原生富文本键盘/粘贴回调、IME 与内容测量快照、固定行高；与上游段落缩进/两端对齐并集（`FixedLineHeight` + `Justify`/`Indent`），保留 `UseJustify`。
13. [`mygo-v0.7.3-13-richtext-usejustify.patch`](../patches/mygo-v0.7.3-13-richtext-usejustify.patch)：保留上游 `text.Shared().UseJustify()` 调用。
14. [`mygo-v0.7.3-14-updater-availability.patch`](../patches/mygo-v0.7.3-14-updater-availability.patch)：只读 `AvailableVersion()`，沿用 `OnChange` 主线程通知。
15. [`mygo-v0.7.3-15-markdown-anchor.patch`](../patches/mygo-v0.7.3-15-markdown-anchor.patch)：`markdown.Options.Anchor` 标题定位与重复跳转；正文 `#heading` 链接优先由 Markdown 导航。
16. [`mygo-v0.7.3-16-inline-link-selection.patch`](../patches/mygo-v0.7.3-16-inline-link-selection.patch)：正文 `RoleLink` 越过拖拽阈值后手势交回段落；不触发打开。
17. [`mygo-v0.7.3-17-collapsible-animation.patch`](../patches/mygo-v0.7.3-17-collapsible-animation.patch)：`ui.CollapsibleBaseWithAnimation`，默认仍 200ms，可设 140ms 独立曲线。
18. [`mygo-v0.7.3-18-webview-data-directory.patch`](../patches/mygo-v0.7.3-18-webview-data-directory.patch)：`Application.SetWebViewDataDirectory`（Windows WebView2 目录）；移植到 v0.7.3 的 `app.go`/Windows 后端（`startEnvironment` 重写）。
19. [`mygo-v0.7.3-19-package-stage-rename.patch`](../patches/mygo-v0.7.3-19-package-stage-rename.patch)：Windows build/dev 共用 `replacePath` 在 error5/32 时有限重试。
20. [`mygo-v0.7.3-20-windows-glyph-placement.patch`](../patches/mygo-v0.7.3-20-windows-glyph-placement.patch)：checked `ui.WithGlyphPlacement` 与 ClearType quarter 定位；`paint.go` 与上游 `objectFn` 分支缝合。
21. [`mygo-v0.7.3-21-rendering-parity.patch`](../patches/mygo-v0.7.3-21-rendering-parity.patch)：固化 v0.6.4 末工作树未补丁化的渲染对齐改动（`LinkWeight`、DottedUnderline、`Context.Instant`、`DeviceScale` 等）。
22. [`mygo-v0.7.3-22-tooltip-primitives.patch`](../patches/mygo-v0.7.3-22-tooltip-primitives.patch)：点定位、按需输入事实和 committed Handle 主键按下归属；`api_gen`/`layout.go` 手工移植（补 12 的漏排），并修复第 8 项缝合残留。
23. [`mygo-v0.7.3-23-layoutcache-union-spacing.patch`](../patches/mygo-v0.7.3-23-layoutcache-union-spacing.patch)：修复字段清单并集合并丢失的行尾空格。
24. [`mygo-v0.7.3-24-textclient-a11y-guard.patch`](../patches/mygo-v0.7.3-24-textclient-a11y-guard.patch)：`textClient` 的 a11y SetValue/编辑派发仅对无 editor 的自定义文本客户端生效；上游 `TestReadOnlyInput` 通过，富文本客户端行为保留。
25. [`mygo-v0.7.3-25-uia-benchmark-close.patch`](../patches/mygo-v0.7.3-25-uia-benchmark-close.patch)：补齐 UIA benchmark 合并后的函数收尾。
26. [`mygo-v0.7.3-26-patch-identity-final.patch`](../patches/mygo-v0.7.3-26-patch-identity-final.patch)：固定 `v0.7.3` + `rcode-native-ui.18` 身份。
27. [`mygo-v0.7.3-27-selection-spans.patch`](../patches/mygo-v0.7.3-27-selection-spans.patch)：文本选区自带 span 文本快照，虚拟化列表滚出构建范围后选区仍可绘制与复制；身份升为 `rcode-native-ui.19`。
28. [`mygo-v0.7.3-28-code-block-tabs.patch`](../patches/mygo-v0.7.3-28-code-block-tabs.patch)：Markdown 代码块按语言制表位（Go 4、JS/TS/JSON 2，未知 4）把 tab 展开成空格排版，源码与“复制代码”保留 tab；选区复制把显示列映射回源码偏移。身份升为 `rcode-native-ui.20`。
29. [`mygo-v0.7.3-29-split-drag.patch`](../patches/mygo-v0.7.3-29-split-drag.patch)：split 拖拽在构建窗格前应用（新增 `draggedBy`，不再用 `Dragged()` 触发第二次构建），命中区从主题线宽扩到 20 DIP；`Changed()` 只报告方向键步进，调用方在 split 返回后读取 `*size`。身份升为 `rcode-native-ui.21`。

源码锁为 [`third_party/mygo.lock.json`](../third_party/mygo.lock.json)。当前 `.21` 的
2,034 个文件的 SHA-256 树指纹为
`2f3ca58c451bedec8a0791cdffc6e084f70a45ae2570ab2043886f868bc441a1`。
补丁按上述顺序在 v0.7.3 原版上重放得到与锁定树逐字节一致的结果；
各补丁的 SHA-256 以源码锁与 `scripts/frameworkpin/pin.go` 的 `patchChain` 为准。
树指纹按 UTF-8 相对路径的字节序排序，再对每个 `路径 + NUL + 文件 SHA-256 十六进制 + LF`
记录做 SHA-256。`.gitattributes` 禁止框架文件换行转换，确保跨平台字节一致。

此文档记录框架能力与验证。设置及 annotation chip 在 Desktop 的实际接线由主任务完成，
原生操作与视觉一致性仍需单独验收。

`.6`新增 `Element.Scrollbars(visible)`，只控制当前原生 Scroll/Horizontal/Both/List 的
指示条和对应命中区域；默认行为保持，策略不继承，也不影响自绘的代码/富文本编辑器。
隐藏后仍支持滚轮、TrackScroll、键盘导航和辅助功能滚入。最终提交后的光标更新检查
正在拖动的稳定节点，隐藏时取消拖动并恢复内容光标；配置先后顺序和稳定 Key 不会提前
取消另一个节点的拖动。应用目前仅在工作区 TabStrip 使用该开关。

八类框架回归覆盖默认/隐藏、双轴、1x/2x、拖动中切换、链式最终值、无重新布局、
邻居/父子隔离、虚拟 List、键盘/辅助功能和指示条像素边界。完整 `ui` 与 `ui/markdown`
测试、Go vet 通过；`go generate ./ui` 再生成逐字节不变。上游归档顺序重放全部11补丁
得到 R12 的1,836文件快照；撤销第11补丁精确回到`.5`，再应用精确恢复`.6`，见
`output/framework-scrollbars-r12/final-replay.json`。在 checkout 内的忽略副本重放时，
须从 Git 根使用 `git apply --directory=<副本相对路径>`；`git -C` 进入副本可能受父仓库
前缀过滤而静默跳过路径，必须核对完整树指纹，不能只看 apply 的退出码。

Go1.27.3、CGO关闭、生产标记下，before/after各5目标×2示例的20次构建全部成功。
未采用API的 counter 在 Windows/Linux 体积不变，macOS amd64增加16字节、arm64增加
32字节、WebAssembly增加101字节；hello五目标均不变。**不能称为全平台零体积成本。**
初始Windows候选曾增加512字节，已移出大提交函数并重测为0；原失败候选和符号诊断保留。
准确 argv、二进制哈希、构建前后源码树核对及增长分析见
`output/framework-scrollbars-r12/size-cross/`。这些产物未运行，不代替对应平台原生验收。
三增长目标的 `-dumpdep` 对比没有新增可达函数或重依赖；新增节点为`.6`身份字符串
及两个字段/方法名字，原已可达的滚动条绘制、按压和光标更新函数增加条件分支。
Mach-O符号大小和磁盘增量受对齐影响，不能直接相加；wasm保留函数数5,867，code段
增加242字节、data段减少141字节，净增加101字节。增长原因由诊断记录支持，未以
剥除生产功能、扩大全局状态或改变既有默认滚动行为来追求字节归零。

`.4`新增三个独立opt-in样式能力：`Element.SnapToWindow(margin)`、
`Element.PlaceholderColor(color)` 和 `Element.CaretWidth(width)`。前者按本轮实际
margin box仅在越界时贴窗、不自动flip；输入样式仅当前editable input的paint使用，
不改变全局theme、文本/撤销/IME布局或native blink。CaretWidth默认1DIP，正宽度
按设备像素取整且至少1pixel，0/负值隐藏。未设置仍走原有行为，显式透明占位色
与unset分开。详细source差异、应用接线和取证限制见 `audit-native-popup-style.md`。

标准上游归档按原6patch重放精确还原`.3`，再顺序应用7–9及`.4`标记得到R7阶段
1,834文件完整树。反向撤销7–9又精确回`.3`，重新正向与当时live tree逐字节一致；
证据为 `output/snap-framework-qa/final-replay.json`。各API测试/生成/二进制体积
记录在 `output/framework-snap-to-window/validation.json` 和
`output/framework-caret-width/final-validation.json`。API9原始补丁与root追加revision
后的最终补丁hash分开保存，不能把API-only重放日志说成最终identity重放。

未opt-in的counter示例从`.3`到三API最终共增加2,048字节；hello不变。新增量来自
native UI布局/分支及editor元数据，无新依赖；CaretWidth单独加入未再增加体积。
这些是构建/链接证据，没有运行示例窗口，也不代表新平台原生验收。

`.5`只将本进程的Windows UIA provider从错误的ClientSideProvider标记改为
ServerSideProvider，保留UseComThreading，最终值0x22。依据为
[Microsoft ProviderOptions](https://learn.microsoft.com/en-us/windows/win32/api/uiautomationcore/ne-uiautomationcore-provideroptions)。
真实COM vtable回归覆盖声明以及64位不同ID的main→side→none→main发布焦点；
这是provider契约测试，不能证明外部客户端的旧focus报告已解决。Bounds注释从
“visible box”改为实际的transformed border box，按祖先clip裁剪和兄弟遮挡
检测均未新增。调查与API-only体积/依赖证据见
`output/framework-uia-provider-options-20261028-124947-256/`。

现有固定归档/九补丁重放树先重新核对为精确`.4`，加最终第10补丁得当前`.5`；
反向回`.4`再正向回`.5`均逐字节匹配，见 `output/uia-framework-qa/final-replay.json`。
API-only原补丁8d0bc626…另存该目录，正式第10补丁5372a13a…追加revision；不把
前者的build/体积日志追溯改称最终`.5`身份。

## 原因与应用侧边界

检查对象为固定的 `github.com/egoist/mygo v0.7.3`，Go `1.27.3`。

- `ui/context.go` 的 `DisableAnimations` 只设置当前构建帧的 `instantFrame`。
  它使朝目标运动的数值与布局立即完成，不覆盖系统 `ReduceMotion`，也不停止独立时间线。
- `ui/preferences.go` 的 `engine.preferences` 缓存系统偏好，没有公开的应用覆盖 API。
  `Context.Preferences` 为只读查询；`Tester.SetPreferences` 只属于无窗口测试器。
- `ui/input.go` 的 `engine.scroll` 对非精确滚轮采用
  `!precise && !rt.preferences().ReduceMotion`，没有检查 `instantFrame`。
- `ui/wheel.go` 的 `glideWheels` 驱动已经开始的滚轮缓动，不重新检查应用偏好。
  因此只在每帧调用 `DisableAnimations` 仍会产生鼠标滚轮缓动。
- 精确触控板滚动直接跟随手势；编辑器、终端等 `HandleInput` 控件先接收原始输入。

应用可以给个别 `Scroll` 绑定 `ScrollState` 并自行拦截滚轮，但无法通过公开 API
安全覆盖引擎的全部滚动入口。这个替代方案还会改变嵌套滚动边界传递、列表锚点、
横向/RTL 滚动及自定义输入控件的处理顺序。此补丁保留引擎的原有处理路径，
没有加入看似通用、实际只改变部分页面的应用侧拦截器。

## 补丁 API 与行为

新增公开 API，由原有生成器生成 Context 包装：

```go
type MotionPreference uint8

const (
    MotionSystem MotionPreference = iota
    MotionFull
    MotionReduced
)

type MotionPreferences struct {
    Motion          MotionPreference
    SmoothScrolling bool
}

// 在窗口视图开头调用；参数被复制，不保存应用侧指针。
c.SetMotionPreferences(&ui.MotionPreferences{
    Motion:          ui.MotionSystem,
    SmoothScrolling: true,
})

// 恢复该窗口原本的系统跟随行为。
c.SetMotionPreferences(nil)
```

覆盖状态属于单个 UI 引擎/窗口，保留到下一次设置。系统偏好缓存仍保存真实系统值，
不会被应用覆盖污染。`Context.Preferences().ReduceMotion`、MyGo 动画、列表跟随、
页面过渡及 Markdown reveal 因此读取同一有效动效值。

| 选择 | 有效 ReduceMotion | 非精确鼠标滚轮 |
| --- | --- | --- |
| 未设置 / `nil` | 跟随系统 | 保留原版 MyGo 行为 |
| `MotionSystem` | 跟随系统 | SmoothScrolling 为 true 且系统未减少动态时缓动 |
| `MotionFull` | false | 由 SmoothScrolling 决定 |
| `MotionReduced` | true | 立即滚动 |

`MotionFull` 是用户明确选择完整动效时的覆盖，能覆盖系统减少动态。
`MotionReduced` 优先于 SmoothScrolling。关闭缓动时，已经在途的滚轮运动立即完成到
原目标并清除其 wheel 状态，不丢失尚未显示的滚动距离。精确触控板输入、菜单与焦点命中、
嵌套边界传递以及自定义输入回调保持现有引擎路径。

系统 WebView 内网页的 CSS 动效与浏览器自己的滚动不由此补丁控制。
原生终端和编辑器已经自行处理其输入；补丁不会重编码或吞掉这些输入。
这一边界需要在实机回归中分别记录。

## 修改文件

- `ui/preferences.go`：覆盖类型、内部 Context 方法和有效偏好计算。
- `ui/runtime.go`：每个引擎保存一份复制的覆盖状态。
- `ui/input.go`：非精确滚轮使用统一的 `wheelGlides` 判断。
- `ui/wheel.go`：停止在途缓动，保持原有 List/ScrollState 接管逻辑。
- `ui/api_gen.go`：通过 `go generate ./ui` 生成的公开方法包装。
- `ui/motion_preferences_test.go`：覆盖/系统恢复、即时滚动、开关切换、减少动态优先级、公开生成 API。

## 原生 Hover API

```go
ui.Row(c).Key(annotation.ID).OnHover(func(over bool) {
    if over {
        workspace.FocusAnnotation(annotation.Index)
    } else {
        workspace.FocusAnnotation(0)
    }
})
```

`Element.OnHover(func(bool)) Element` 的回调在主线程处理输入之后或帧提交之后执行，
不会在 View 构建时启动脚本任务。它沿 `setHover` 使用已提交元素的稳定 identity，
遵循 `Hovered` 的按压行为；重建不重复进入通知。禁用、隐藏、移除或布局改变使元素离开时
发送一次 false。已回收的 state 不会收到旧的进入通知，离开通知使用对应旧回调完成清理。
回调不能保留跨帧 `Element` 或 `Context`。

`ui/hover_test.go` 覆盖进入/离开、重复帧、消失、state 重用、禁用和最新提交回调。

## 文件拖放与 Markdown 行高 API

```go
ui.Box(c).OnFilesDrop(func(paths []string) {
    // 此处已离开 View 构建，可启动附件导入工作。
    importer.Import(paths)
})
```

`Element.OnFilesDrop(func([]string)) Element` 沿用原有文件拖放命中路径，在主线程原生 drop
事件中调用最新已提交的处理函数，并复制路径切片。禁用、隐藏和已移除元素不接收事件。
回调消费该次事件，因此同一元素的 `DroppedFiles()` 不会重复返回这些路径。
未注册回调的原有 `DroppedFiles()` 查询保持原样，内部 `ui.Drop[FileDrag]` 也未改动。
`ui/files_drop_callback_test.go` 覆盖身份更新、路径所有权、禁用/移除及重复帧。

Markdown 新增可选字段：`markdown.Theme.FixedLineHeight`、`CodeFixedLineHeight` 和
`markdown.HeadingStyle.FixedLineHeight`，单位为逻辑像素；0 保留上游的倍数行高。
补丁只改变排版/测量，保留 parser、链接、复制和原始 Markdown。
专门测试覆盖主题变更重新布局、代码分块、复制、流式选择和虚拟列表阅读锚点。

`patch_identity.go` 提供 `mygo.FrameworkPatch() mygo.SourcePatch`，报告上游版本、本地修订与
发布构建注入的源码指纹。普通开发构建的指纹为空；发布校验拒绝这种未标记产物。

没有修改平台实现、原生库、WebView、布局算法或默认动效值。

## 输入显示快照

`zeron-native-ui.3` 增加 `Element.InputDisplayText() string`，由原有生成器产生
checked API。它读取当前原生输入控件的 `editor.displayText()`：包括尚未提交的 IME
预编辑文本；密码输入仍返回遮罩圆点，普通元素和零值返回空字符串。它不提交预编辑，
不修改绑定值、选区、焦点或 undo 历史。字符串是本轮数据快照，不是可跨帧操作的控件。

Composer 用该快照测量 compact 容量及换行；原生 `TextAreaBase` 自己处理 expanded
模式的换行高度和滚动。若改用绑定 draft 测量，长中文/日文预编辑会一直挤在 compact
一行里；若把预编辑写回 draft，则会提前提交 IME。此 API 保留输入管线原有语义。

`ui/input_display_text_test.go` 使用生成后的公开 API，覆盖在中间插入 CJK/emoji
预编辑、连续只读帧、真正提交、undo、取消、快照稳定性、密码和非输入元素。
当同一 key 从输入控件换成普通或可选择文本时，即使复用了含旧 editor 的 state，
读 API 也会检查当前 editable 标志，返回空字符串，不暴露上一个控件的草稿。
应用层回归另覆盖预编辑期间不发送已有草稿，以及展开/折叠时保留原生输入身份。
该读 API 补丁只涉及读 API、测试、生成结果和本地修订标记；上游为 v0.7.3。

## 安装共存与打包名称

v0.6.4 时期的 `zeron-native-ui.2` 曾在构建配置中新增 `windows.installName` 与
`macos.bundleName`。v0.7.3 已用上游 `windows.displayName`/`windows.installDirName`
与 slug 文件命名取代，第 5 项独立安装名补丁已删除。当前应用命名完全跟随
`mygo.json` 的 `name`（GCode）：Windows 安装目录为
`%LOCALAPPDATA%\Programs\GCode`，可执行文件为 `GCode.exe`；macOS 包为
`GCode.app`，内部 `Contents/MacOS/GCode`；bundle identifier 为 `dev.gcode.rcode`。
`scripts/distribution` 校验这些身份，构建脚本先运行该检查。

历史 RCode 安装包及 ZIP 保留原身份，但不能作为当前交付物。原生安装、与原版并存
启动、快捷方式和卸载的实际行为仍待 Windows/macOS 实机验收。

本次在 `build/coexist-qa-r2` 全新生成 Windows amd64、macOS amd64、arm64 和 Universal
检查包。四个目标均通过程序/终端库架构、框架标记、独立包路径、Info.plist 和许可资源校验；
Universal 的程序和终端库均包含 amd64/arm64 两个 slice。清单为该目录中的
`release-manifest-windows-amd64_darwin-amd64_darwin-arm64_darwin-universal.json`，
记录各 ZIP/文件 SHA-256、50 个许可文件、修订与源码树哈希。Windows NSIS 安装器已编译，
未运行安装器或应用。Windows 主机不能创建 macOS DMG，也没有执行 macOS 签名/实机验收。
首次输出到 `build/coexist-qa` 时曾遇到 Windows staging 目录 rename 的 Access denied；
随后改用上述全新目录，四个目标完整通过。失败目录不作为交付物。

## 验证与采用流程

Motion 补丁先在独立临时源码副本验证，再固定到仓库。检查命令包括：

```text
go generate ./ui
go test ./ui -run 'Test(ApplicationMotionPreferences|ApplicationSmoothScrolling|ApplicationReducedMotion|MotionPreferencesPublic|Wheel|DisableAnimations)' -count=1
go test ./ui
git apply --reverse --check <checkout>/patches/mygo-v0.7.3-01-motion.patch
```

以上 Motion 检查通过，完整 UI 测试运行约 35.6 秒。
加入 Hover、文件拖放及 Markdown 行高后，`go test ./ui ./ui/markdown` 通过；
项目 `go test ./...`、`go vet ./...` 通过。打包结果在本次交付说明中单独记录。

重新构造源码副本时，应从校验和为
`h1:Jch8vV8ZzlHHr6hABg+aOx9nFbdjZ2oD/abY5Cf5/0I=` 的 v0.7.3 Go 模块归档解压，
按顺序应用当前二十八个补丁，并执行：

```text
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-01-motion.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-02-hover.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-03-files-drop.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-04-markdown-line-height.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-05-snap-to-window.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-06-placeholder-color.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-07-input-display-text.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-08-caret-width.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-09-windows-uia-provider.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-10-scrollbars.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-11-notification-context.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-12-richtext-composer.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-13-richtext-usejustify.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-14-updater-availability.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-15-markdown-anchor.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-16-inline-link-selection.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-17-collapsible-animation.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-18-webview-data-directory.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-19-package-stage-rename.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-20-windows-glyph-placement.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-21-rendering-parity.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-22-tooltip-primitives.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-23-layoutcache-union-spacing.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-24-textclient-a11y-guard.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-25-uia-benchmark-close.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-26-patch-identity-final.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-27-selection-spans.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.7.3-28-code-block-tabs.patch
go generate ./ui
go test ./ui ./ui/markdown
```

补丁已包含生成结果，因此再次生成不应有差异。
由应用根视图把保存的 MotionPreference 与 SmoothScrolling 设置映射到上述 API。
所有独立的原生窗口都需在各自根视图设置相同策略，不能仅在设置页调用。

## 构建与发布校验

```text
go run ./scripts/check-framework.go
go test ./scripts/frameworkpin
```

`scripts/frameworkpin` 固定上游版本、模块校验和、本地修订、二十八个补丁哈希和完整源码树哈希。
它核对 Go 实际选中的模块，拒绝其他 replace 目录、版本、未替换上游、符号链接及源码增删改。
有意修改框架必须更新补丁、修订、锁文件和校验器中的固定值，作为新的可审查变更。

Windows 与 macOS 构建脚本先执行上述校验，设置 `GOWORK=off`，再通过 `MYGO_LDFLAGS`
写入 `github.com/egoist/mygo.sourcePatchRevision`、`sourcePatchSHA256` 与 `sourcePatchIdentity`。
`verify-package.go` 逐个架构读取 Go build info，要求 Go `1.27.3`、上游 MyGo `v0.7.3`、
精确的本地 replace（本地替换版本为 `(devel)`）和不重复的完整修订/指纹标记。
Go 的 `-trimpath` 产物省略 build settings 中的 `-ldflags`，因此校验器还逐个 PE/Mach-O slice
核对保留在框架初始化变量中的唯一 `RCODE_MYGO_PATCH[...]` 标记；不靠可缺省的命令行元数据。
若 build settings 保留了 linker flags，则也要求标记参数值完整匹配且无重复覆盖。
其他 replace 或未标记的旧产物不能通过。
发布 manifest 同时记录源码锁、补丁哈希、修订和各文件/归档哈希。

二进制标记是经过构建前后源码校验的来源记录，不替代代码签名或原生验收。

交付仍须验证三个目标架构可构建，并在 Windows/macOS 原生环境检查鼠标滚轮、触控板、
中文输入、嵌套列表、聊天跟随输出、编辑器、终端，以及设置切换/重启后的行为。
原生运行、视觉一致性、性能和用户验收均仍待单独完成。

## R14 fixture observations

R14 does not change the `.6` vendor tree or add a patch. Production TabStrip only
binds additional Handles through a tagged observer; the normal build's observer
is empty.36 native reference/Go pairs, actual before/after Win32 foreground checks
and69 comparison guards are application fixture evidence, not framework pixel
acceptance. The last release packages remain R13. See `audit-tabstrip-fixture.md`.

## R15 follow-up

R15 changes application tab drawing through the existing shader API; the `.6`
framework tree and eleven patches are unchanged. Four package targets and a
separate exact Windows EXE tab interaction smoke pass.36 original/Go baseline
pairs plus four supplemental states do not establish full visual acceptance;
see `audit-tabstrip-style.md`. Earlier evidence above retains its phase identity.

## R17 V5 接入边界

采用记录为 `output/tabstrip-r17-quarter-v5-adopted/attempt-01/result.json`。
当前2931项正式输入与审阅提案逐字节匹配；当前AX app961只新增原生提示leaf的
checked wrapper，未恢复通用Label，也未恢复旧959项app。该API只选择glyph定位，
没有接入隔离的换行候选。普通Text和不满足Windows/AA/color资格的路径保持native。

实际10项体积矩阵属于接入前V5完整树c18a…1165和继承`.8`的诊断身份：
9项raw delta0、Wasm counter−425B。它不等于runtime或metadata0；`.9`相对该V5
仅改变两处revision literal；新五门槛已按自身2931输入通过，新canonical四目标
候选包已有自己的`.9`来源、架构和文件/ZIP校验身份。该构建记录未运行新包GUI、
installer或Mac程序，不能重标旧native产物或宣布发布接受。
后续同一9c5800…f795主包已另做限定Windows smoke：14状态/15原JPEG、实际
README.md与Close tab名称、右键原生菜单关闭layout并保留README编辑器/文件树/
会话，Alt+F4退出0且stderr空。新记录为
`output/resume-validation/tabstrip-r17-quarter-v5-native/result.json`（0d850ab4…767c）；
它不借用旧c18夹具身份，不表示新DPI、完整视觉、installer、Mac或读屏事件验收。
完整执行、独审及当前接入见 `audit-tabstrip-tooltip-primitives.md`。
