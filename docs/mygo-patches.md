# MyGo v0.7.4 固定补丁

2026-11-02：框架自 v0.7.3 升级到官方 **v0.7.4**（提交 `9bd34e1eb9…`,
2026-11-01 发布），本地修订号 `rcode-native-ui.23`。上游 v0.7.4 已收录本仓库此前
提交的 13 个 PR 能力（#949、#953、#960、#962、#963、#969、#971、#972、#967、#952、
#943、#944、#945、#947、#975 等），对应的本地补丁**删除**：

- `PlaceholderColor`、`CaretWidth`、`Scrollbars(false)`、`DottedUnderline`、
  `Context.Instant`、`markdown.Options.Anchor`、`markdown.Theme.LinkWeight`、
  `Notification.ShowContext`、`updater.AvailableVersion`、
  `ui.Scrollbar`/编辑器滚动条样式、Windows WebView2 首选配色、
  `mygo build` 的包重命名重试，均已由上游原生实现提供。

上游未收录、本仓库仍需要的补丁继续保留并重放到 v0.7.4 原版上：窗口级动效覆盖、
Markdown 精确行高、`SnapToWindow`、输入显示快照、Windows UIA 服务端 provider、
富文本 composer 钩子、正文链接拖拽手势、折叠动效、WebView2 数据目录、
Windows ClearType quarter 定位、链接 hover 点状下划线、点定位 tooltip 与输入事实、
UIA benchmark 收尾、选区 span 快照、代码块制表位、Split 拖动同帧生效。

上游明确建议以轮询替代回调的两项也已按其意见改写：应用改用上游的 `Hovered()` 与
`DroppedFiles()` 查询，本地 `OnHover`、`OnFilesDrop` 两个补丁随之删除，
补丁链再减两项。

升级在 v0.7.4 原版上按序重放全部 19 个补丁：框架 `go build ./...`、应用
`go build ./...`、`go tool mygo vet .`、设置组件回归通过。原生窗口与视觉验收
仍未运行，待后续单独验收。状态：上游为 `github.com/egoist/mygo v0.7.4`。
`go.mod` 只允许该模块替换到 `./third_party/mygo`。
完整框架源码约 17 MB，保留上游 LICENSE、go.mod、go.sum、API 生成器、构建工具、
平台资源和测试；没有修改全局 Go 模块缓存。

按顺序应用的补丁（19 个条目）：

1. [`mygo-v0.7.4-01-motion.patch`](../patches/mygo-v0.7.4-01-motion.patch)：窗口级动效与滚轮缓动偏好。
2. [`mygo-v0.7.4-02-markdown-line-height.patch`](../patches/mygo-v0.7.4-02-markdown-line-height.patch)：Markdown 可选 DIP 精确行高（与上游倍数行高并存）。
3. [`mygo-v0.7.4-03-snap-to-window.patch`](../patches/mygo-v0.7.4-03-snap-to-window.patch)：可选按本轮实际 margin box 贴窗，不自动翻转。
4. [`mygo-v0.7.4-04-input-display-text.patch`](../patches/mygo-v0.7.4-04-input-display-text.patch)：只读原生输入显示快照，包含未提交预编辑文本。
5. [`mygo-v0.7.4-05-windows-uia-provider.patch`](../patches/mygo-v0.7.4-05-windows-uia-provider.patch)：Windows UIA 服务端 provider（`ServerSideProvider|UseComThreading` = `0x22`）与 COM 契约测试。
6. [`mygo-v0.7.4-06-richtext-composer.patch`](../patches/mygo-v0.7.4-06-richtext-composer.patch)：原生富文本键盘/粘贴回调、IME 与内容测量快照、固定行高；与上游段落缩进/两端对齐并集。
7. [`mygo-v0.7.4-07-richtext-usejustify.patch`](../patches/mygo-v0.7.4-07-richtext-usejustify.patch)：保留上游 `text.Shared().UseJustify()` 调用。
8. [`mygo-v0.7.4-08-inline-link-selection.patch`](../patches/mygo-v0.7.4-08-inline-link-selection.patch)：正文 `RoleLink` 越过拖拽阈值后手势交回段落；不触发打开。
9. [`mygo-v0.7.4-09-collapsible-animation.patch`](../patches/mygo-v0.7.4-09-collapsible-animation.patch)：`ui.CollapsibleBaseWithAnimation`，默认仍 200ms，可设 140ms 独立曲线。
10. [`mygo-v0.7.4-10-webview-data-directory.patch`](../patches/mygo-v0.7.4-10-webview-data-directory.patch)：`Application.SetWebViewDataDirectory`（Windows WebView2 目录）。
11. [`mygo-v0.7.4-11-windows-glyph-placement.patch`](../patches/mygo-v0.7.4-11-windows-glyph-placement.patch)：checked `ui.WithGlyphPlacement` 与 ClearType quarter 定位。
12. [`mygo-v0.7.4-12-rendering-parity.patch`](../patches/mygo-v0.7.4-12-rendering-parity.patch)：链接 hover 改为点状下划线；quarter 定位的绘制路径同步 `dotted` 装饰。
13. [`mygo-v0.7.4-13-tooltip-primitives.patch`](../patches/mygo-v0.7.4-13-tooltip-primitives.patch)：点定位、按需输入事实和 committed Handle 主键按下归属。
14. [`mygo-v0.7.4-14-uia-benchmark-close.patch`](../patches/mygo-v0.7.4-14-uia-benchmark-close.patch)：补齐 UIA benchmark 合并后的函数收尾。
15. [`mygo-v0.7.4-15-selection-spans.patch`](../patches/mygo-v0.7.4-15-selection-spans.patch)：文本选区自带 span 文本快照，虚拟化列表滚出构建范围后选区仍可绘制与复制。
16. [`mygo-v0.7.4-16-code-block-tabs.patch`](../patches/mygo-v0.7.4-16-code-block-tabs.patch)：Markdown 代码块按语言制表位（Go 4、JS/TS/JSON 2，未知 4）把 tab 展开成空格排版，源码与“复制代码”保留 tab；选区复制把显示列映射回源码偏移。
17. [`mygo-v0.7.4-17-api-gen.patch`](../patches/mygo-v0.7.4-17-api-gen.patch)：`go generate ./ui` 重新生成公开 API 包装，纳入本链新增的 `Element`/`Context` 方法。
18. [`mygo-v0.7.4-18-split-drag.patch`](../patches/mygo-v0.7.4-18-split-drag.patch)：Split 拖动在布局前写入尺寸，命中带扩大到 20 DIP，并用 `draggedBy` 同帧报告；含 `TestSplitDragAppliesBeforeThePanesBuild` 回归测试。
19. [`mygo-v0.7.4-19-patch-identity.patch`](../patches/mygo-v0.7.4-19-patch-identity.patch)：固定 `v0.7.4` + `rcode-native-ui.23` 身份。

源码锁为 [`third_party/mygo.lock.json`](../third_party/mygo.lock.json)。当前 `.23` 的
2,050 个文件的 SHA-256 树指纹为
`c50af74375c6b2cbb2b8a50a09c407469dd419b29788d5774bed1e8c99deda50`。
补丁按上述顺序在 v0.7.4 原版上重放得到与锁定树逐字节一致的结果；
各补丁的 SHA-256 以源码锁与 `scripts/frameworkpin/pin.go` 的 `patchChain` 为准。
树指纹按 UTF-8 相对路径的字节序排序，再对每个 `路径 + NUL + 文件 SHA-256 十六进制 + LF`
记录做 SHA-256。`.gitattributes` 禁止框架文件换行转换，确保跨平台字节一致。

> 注：上游 v0.7.4 的 `UIAutomationCore` provider 取值仍为 `0x1 | 0x20`
> （`ClientSideProvider | UseComThreading`）。本仓库第 5 项补丁把取值改为
> `ServerSideProvider | UseComThreading` = `0x22`，依据
> [Microsoft ProviderOptions](https://learn.microsoft.com/en-us/windows/win32/api/uiautomationcore/ne-uiautomationcore-provideroptions)
> 与本机 Windows SDK 的 `UIAutomationCore.h`（`ProviderOptions_ServerSideProvider = 0x2`）。

此文档记录框架能力与验证。设置及 annotation chip 在 Desktop 的实际接线由主任务完成，
原生操作与视觉一致性仍需单独验收。

## 2026-11-05 上游追新审计

上游没有新的正式版本，本次只做审计，**不改动** `third_party/mygo`、`patches/`、
源码锁和 `scripts/frameworkpin` 的固定值（仍是 v0.7.4 + `rcode-native-ui.23`）。

| 项目 | 结果 |
| --- | --- |
| 上游 tag / release | 最新为 **v0.7.4**（2026-11-01 发布），即当前固定版本 |
| 上游 `main` | 比 v0.7.4 领先 **145 个提交**（`compare` 的文件清单在 300 项截断）；HEAD `fbcd4311`（2026-11-04） |
| 本仓库策略 | 只固定官方发布版本；未发版的 `main` 不作为升级目标 |

复现性与可移植性核实：

- 从 `h1:0qkDpDRo7C9g8PM8eu4V6FVouhLiTrUdqKD6I31Lv3Q=` 的 v0.7.4 模块归档解压后按序
  重放 19 个补丁，目录内容与当前 `third_party/mygo` 完全一致（仅 `core.autocrlf`
  造成的 CRLF 差异，`diff -r --strip-trailing-cr` 为 0 项）。说明锁定树仍可从官方归档复现。
- 把同一补丁链用 `git apply --3way` 试重放到 `main` HEAD：01、02、04、08、09、15、19
  可应用；**03、05、06、07、10、11、12、13、14、16、17、18 共 12 个冲突**。

`main` 已吸收、下次升级应删除或重估的补丁：

| 补丁 | `main` 现状 |
| --- | --- |
| `05-windows-uia-provider` | 已由 #1007 实现同样的 `ServerSideProvider\|UseComThreading`（`internal/windows/surface_access.go` 的 `0x2\|0x20`）；整条可删。 |
| `07-richtext-usejustify` | 上游 `ui/text_layout.go` 的 `shapeTextAligned` 已在 `shape.Justify` 时调用 `text.Shared().UseJustify()`；整条可删。 |
| `12-rendering-parity` | 链接 hover 上游改为整条实现下划线（`e.Underline()` 加 `e.ts.solid`），与本仓库点状下划线的取向不同；升级时要么接受上游行为删除该处，要么重写为独立冲突补丁。quarter 定位的 `dotted` 装饰仍需配合 11 号补丁重放。 |

`main` 仍未提供、升级后必须继续保留的 API：`Context.SetMotionPreferences`、
`markdown.Theme.FixedLineHeight`/`CodeFixedLineHeight`/`HeadingStyle.FixedLineHeight`、
`SnapToWindow`、`InputDisplayText`、`Application.SetWebViewDataDirectory`、
`ui.WithGlyphPlacement`、`CollapsibleBaseWithAnimation`（上游只有倍数行高
`LineHeight`/`CodeLineHeight`，无 DIP 精确行高）。

下次升级（上游发布下一 tag 时）的顺序：

1. 解压新 tag 的模块归档，按上表先删 05、07，重估 12。
2. 逐个重放余下补丁并解决冲突，重点核对 03、06、10、11、13、16、17、18 所在的
   `ui/element.go`、`ui/input.go`、`ui/richtext/*`、`ui/api_gen.go`。
3. `go generate ./ui` 重新生成 API 包装（17 号补丁应随之重做或删除）。
4. 更新补丁文件名中的版本前缀、`mygo.lock.json`、`scripts/frameworkpin/pin.go` 的
   `UpstreamVersion`/`Revision`/`UpstreamSum`/`patchChain`/`ExpectedTreeSHA256` 与本文件。
5. 跑 `go build ./...`、框架与应用测试、`go tool mygo vet .`、`go run ./scripts/check-framework.go`
   与 `go test ./scripts/frameworkpin`；原生窗口与视觉验收另列。

## 已被上游收录而删除的本地补丁

| 原补丁 | 上游来源 | 说明 |
| --- | --- | --- |
| `02-hover` | #964（关闭） | 上游建议用 `Hovered()` 轮询，应用已改写，第 2 项删除。 |
| `03-files-drop` | #956（关闭） | 上游建议用 `DroppedFiles()` 轮询，应用已改写，第 3 项删除。 |
| `06-placeholder-color` | #969 | `PlaceholderColor` 已由上游提供。 |
| `08-caret-width` | #972 | `CaretWidth` 已由上游提供。 |
| `10-scrollbars` | #972 | `Scrollbars(false)` 已由上游提供。 |
| `11-notification-context` | #952 | `Notification.ShowContext` 已由上游提供。 |
| `14-updater-availability` | #960 | `updater.AvailableVersion()` 已由上游提供。 |
| `15-markdown-anchor` | #969 | `markdown.Options.Anchor` 已由上游提供。 |
| `19-package-stage-rename` | #953 | Windows 包重命名重试已由上游提供。 |
| `21-rendering-parity`（大部分） | #969 | `LinkWeight`、`DottedUnderline`、`Context.Instant` 已由上游提供；本仓库只保留链接 hover 点状下划线一处差异。 |
| `23-layoutcache-union-spacing` | — | 字段清单并集随上游字段变化重新合并，无需单独补丁。 |
| `24-textclient-a11y-guard` | #967 | 上游已按“有 editor 走 editor 分支”实现同样的守卫。 |

## 原因与应用侧边界

检查对象为固定的 `github.com/egoist/mygo v0.7.4`，Go `1.27.3`。

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

## 悬停与拖放：改用上游轮询

上游不接受 `OnHover`、`OnFilesDrop` 回调（#964、#956）：`Hovered()` 只在悬停状态改变时
请求一帧，`DroppedFiles()` 在 drop 时请求一帧，轮询并不比回调更费。应用因此改用
上游查询，本地两个补丁删除：

```go
// 悬停：模型列表的行与标注 chip 各自读取自己的状态。
if button.Hovered() {
    a.modelCursor = i
}

// 拖放：composer 每帧读取自上次以来落入的路径。
if paths := composer.DroppedFiles(); len(paths) > 0 {
    p.onFilesDrop(paths)
}
```

标注 chip 与 transcript 行需要“离开”通知，因此应用自己保存上一帧的悬停项：
标注 chip 用 `App.hoveredAnnotation`，transcript 行用既有的 `transcriptHover` 映射；
指向的行消失时，下一次构建里重建该行会把它清掉。

## 文件拖放与 Markdown 行高 API

```go
// 拖放同样由框架轮询：drop 会请求一帧，路径在下一次构建里读取。
if paths := composer.DroppedFiles(); len(paths) > 0 {
    importer.Import(paths)
}
```

Markdown 新增可选字段：`markdown.Theme.FixedLineHeight`、`CodeFixedLineHeight` 和
`markdown.HeadingStyle.FixedLineHeight`，单位为逻辑像素；0 保留上游的倍数行高。
补丁只改变排版/测量，保留 parser、链接、复制和原始 Markdown。
专门测试覆盖主题变更重新布局、代码分块、复制、流式选择和虚拟列表阅读锚点。

`patch_identity.go` 提供 `mygo.FrameworkPatch() mygo.SourcePatch`，报告上游版本、本地修订与
发布构建注入的源码指纹。普通开发构建的指纹为空；发布校验拒绝这种未标记产物。

没有修改平台实现、原生库、WebView、布局算法或默认动效值。

## 输入显示快照

`Element.InputDisplayText() string` 由原有生成器产生 checked API。它读取当前原生输入控件的
`editor.displayText()`：包括尚未提交的 IME 预编辑文本；密码输入仍返回遮罩圆点，普通元素和
零值返回空字符串。它不提交预编辑，不修改绑定值、选区、焦点或 undo 历史。字符串是本轮
数据快照，不是可跨帧操作的控件。

Composer 用该快照测量 compact 容量及换行；原生 `TextAreaBase` 自己处理 expanded
模式的换行高度和滚动。若改用绑定 draft 测量，长中文/日文预编辑会一直挤在 compact
一行里；若把预编辑写回 draft，则会提前提交 IME。此 API 保留输入管线原有语义。

`ui/input_display_text_test.go` 使用生成后的公开 API，覆盖在中间插入 CJK/emoji
预编辑、连续只读帧、真正提交、undo、取消、快照稳定性、密码和非输入元素。
当同一 key 从输入控件换成普通或可选择文本时，即使复用了含旧 editor 的 state，
读 API 也会检查当前 editable 标志，返回空字符串，不暴露上一个控件的草稿。
应用层回归另覆盖预编辑期间不发送已有草稿，以及展开/折叠时保留原生输入身份。

## 验证与采用流程

补丁先在独立临时源码副本按序验证，再固定到仓库。检查命令包括：

```text
go generate ./ui
go build ./...
go test ./ui ./ui/markdown ./ui/richtext
go test ./...            # 应用仓库根目录
go tool mygo vet .
```

重新构造源码副本时，应从校验和为
`h1:0qkDpDRo7C9g8PM8eu4V6FVouhLiTrUdqKD6I31Lv3Q=` 的 v0.7.4 Go 模块归档解压，
按顺序应用上述十九个补丁，并执行 `go generate ./ui`。补丁已包含生成结果，再次生成
不应有差异。

由应用根视图把保存的 MotionPreference 与 SmoothScrolling 设置映射到上述 API。
所有独立的原生窗口都需在各自根视图设置相同策略，不能仅在设置页调用。

## 构建与发布校验

```text
go run ./scripts/check-framework.go
go test ./scripts/frameworkpin
```

`scripts/frameworkpin` 固定上游版本、模块校验和、本地修订、十九个补丁哈希和完整源码树哈希。
它核对 Go 实际选中的模块，拒绝其他 replace 目录、版本、未替换上游、符号链接及源码增删改。
有意修改框架必须更新补丁、修订、锁文件和校验器中的固定值，作为新的可审查变更。

Windows 与 macOS 构建脚本先执行上述校验，设置 `GOWORK=off`，再通过 `MYGO_LDFLAGS`
写入 `github.com/egoist/mygo.sourcePatchRevision`、`sourcePatchSHA256` 与 `sourcePatchIdentity`。
`verify-package.go` 逐个架构读取 Go build info，要求 Go `1.27.3`、上游 MyGo `v0.7.4`、
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
is empty. 36 native reference/Go pairs, actual before/after Win32 foreground checks
and 69 comparison guards are application fixture evidence, not framework pixel
acceptance. The last release packages remain R13. See `audit-tabstrip-fixture.md`.

## R15 follow-up

R15 changes application tab drawing through the existing shader API; the `.6`
framework tree and eleven patches are unchanged. Four package targets and a
separate exact Windows EXE tab interaction smoke pass. 36 original/Go baseline
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
