# MyGo v0.6.4 固定补丁

状态：**已采用**，上游为 `github.com/egoist/mygo v0.6.4`，本地修订
`zeron-native-ui.2`。`go.mod` 只允许该模块替换到 `./third_party/mygo`。
完整框架源码约 17 MB，保留上游 LICENSE、go.mod、go.sum、API 生成器、构建工具、
平台资源和测试；没有修改全局 Go 模块缓存。

按顺序应用的补丁：

1. [`mygo-v0.6.4-motion.patch`](../patches/mygo-v0.6.4-motion.patch)：窗口级动效与滚轮缓动偏好。
2. [`mygo-v0.6.4-hover.patch`](../patches/mygo-v0.6.4-hover.patch)：原生 hover 回调及构建来源标识。
3. [`mygo-v0.6.4-files-drop.patch`](../patches/mygo-v0.6.4-files-drop.patch)：原生文件拖放事件回调。
4. [`mygo-v0.6.4-markdown-line-height.patch`](../patches/mygo-v0.6.4-markdown-line-height.patch)：Markdown 可选 DIP 精确行高。
5. [`mygo-v0.6.4-coexistence.patch`](../patches/mygo-v0.6.4-coexistence.patch)：独立的 Windows 安装名与 macOS 包文件名。

源码锁为 [`third_party/mygo.lock.json`](../third_party/mygo.lock.json)。当前 1,830 个文件的
SHA-256 树指纹为 `fd197203233f09d5cede40fc69fad6e6bac8f6dc797e1770f1637e4687ab133e`。
树指纹按 UTF-8 相对路径的字节序排序，再对每个 `路径 + NUL + 文件 SHA-256 十六进制 + LF`
记录做 SHA-256。`.gitattributes` 禁止框架文件换行转换，确保跨平台字节一致。

此文档记录框架能力与验证。设置及 annotation chip 在 Desktop 的实际接线由主任务完成，
原生操作与视觉一致性仍需单独验收。

## 原因与应用侧边界

检查对象为固定的 `github.com/egoist/mygo v0.6.4`，Go `1.27.3`。

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

## 安装共存与打包名称

`zeron-native-ui.2` 在构建配置中新增两个可选字段；省略时保留上游默认命名。

```json
{
  "name": "Zeron",
  "identifier": "dev.gcode.zeronui",
  "windows": { "installName": "Zeron Native UI" },
  "macos": { "bundleName": "Zeron Native UI" }
}
```

- `windows.installName` 只改变 NSIS 安装目录、开始菜单/桌面快捷方式文件名和卸载显示名。
  当前目录为 `%LOCALAPPDATA%\Programs\Zeron Native UI`，快捷方式为 `Zeron Native UI.lnk`。
  安装器仍将 `Zeron.exe` 作为启动目标，应用界面名和程序名保持 Zeron。
- `macos.bundleName` 是省略 `.app` 扩展名的文件名 stem。当前产物为 `Zeron Native UI.app`，
  内部仍使用 `Contents/MacOS/Zeron`，`CFBundleName`、`CFBundleDisplayName` 与
  `CFBundleExecutable` 均为 Zeron；bundle identifier 为 `dev.gcode.zeronui`。
  DMG 目标路径读取实际包名，开发构建的 executable 路径也不再从 `.app` 名字推导。
- Darwin 开发构建 fingerprint 包含最终 `.app` 文件名；只改 `macos.bundleName` 也会重建包，
  不会被 unchanged 检查跳过。显式写出默认名与省略字段具有相同 fingerprint。

纯测试保留一份原版 `Zeron.app` 哨兵并生成独立包，核对原版目录未改动及内部身份一致；
Windows 测试实际编译 NSIS 脚本，核对创建/删除快捷方式、安装目录、卸载标识及启动路径。
测试不启动或安装应用。`scripts/distribution` 额外拒绝与原版同名的本项目配置，验证真实
`Info.plist`，并拒绝把旧 `Zeron.app` 混入新输出目录。构建脚本先运行此配置检查。

所有采用本修订之前的安装包及 ZIP 均须重新构建，不能作为可共存的最终交付物。
原生安装、与原版并存启动、快捷方式和卸载的实际行为仍待 Windows/macOS 实机验收。

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
git apply --reverse --check <checkout>/patches/mygo-v0.6.4-motion.patch
```

以上 Motion 检查通过，完整 UI 测试运行约 35.6 秒。
加入 Hover、文件拖放及 Markdown 行高后，`go test ./ui ./ui/markdown` 通过；
项目 `go test ./...`、`go vet ./...` 通过。打包结果在本次交付说明中单独记录。

重新构造源码副本时，应从校验和为
`h1:VzCqZKgvXcym5cSYKoC6IkTgcVeGZquPiui/a3oE36o=` 的 v0.6.4 Go 模块归档解压，
按顺序应用五个补丁，并执行：

```text
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.6.4-motion.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.6.4-hover.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.6.4-files-drop.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.6.4-markdown-line-height.patch
git -c core.autocrlf=false apply <checkout>/patches/mygo-v0.6.4-coexistence.patch
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

`scripts/frameworkpin` 固定上游版本、模块校验和、本地修订、五个补丁哈希和完整源码树哈希。
它核对 Go 实际选中的模块，拒绝其他 replace 目录、版本、未替换上游、符号链接及源码增删改。
有意修改框架必须更新补丁、修订、锁文件和校验器中的固定值，作为新的可审查变更。

Windows 与 macOS 构建脚本先执行上述校验，设置 `GOWORK=off`，再通过 `MYGO_LDFLAGS`
写入 `github.com/egoist/mygo.sourcePatchRevision`、`sourcePatchSHA256` 与 `sourcePatchIdentity`。
`verify-package.go` 逐个架构读取 Go build info，要求 Go `1.27.3`、上游 MyGo `v0.6.4`、
精确的本地 replace（本地替换版本为 `(devel)`）和不重复的完整修订/指纹标记。
Go 的 `-trimpath` 产物省略 build settings 中的 `-ldflags`，因此校验器还逐个 PE/Mach-O slice
核对保留在框架初始化变量中的唯一 `ZERON_MYGO_PATCH[...]` 标记；不靠可缺省的命令行元数据。
若 build settings 保留了 linker flags，则也要求标记参数值完整匹配且无重复覆盖。
其他 replace 或未标记的旧产物不能通过。
发布 manifest 同时记录源码锁、补丁哈希、修订和各文件/归档哈希。

二进制标记是经过构建前后源码校验的来源记录，不替代代码签名或原生验收。

交付仍须验证三个目标架构可构建，并在 Windows/macOS 原生环境检查鼠标滚轮、触控板、
中文输入、嵌套列表、聊天跟随输出、编辑器、终端，以及设置切换/重启后的行为。
原生运行、视觉一致性、性能和用户验收均仍待单独完成。
