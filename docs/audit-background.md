# 背景、侧边栏 tone 与浮层毛玻璃

2026-11-01 实现记录。参考为只读的 Zeron v0.2.128，固定提交
`ab189159f10cf8e63c0502be75a59bbc487e0eaf`，副本在
`output/reference-sources/zeron`。本文件记录源码对照、实现与可执行检查；
参考窗口与原生窗口的并排视觉验收仍未进行。

参考文件 SHA-256：

| Reference file | SHA-256 |
| --- | --- |
| `crates/ui/src/theme.rs` | `215f3a729f86c8ee69adb80d5260cbf79235e5d43c4adfdf40f115f30bf90b45` |
| `crates/ui/src/frost.rs` | `4e5c7b36a7b7c606a6dec494853ae7462fb82eaa93adc8037b7d222f187a04ba` |
| `crates/ui/src/popover.rs` | `28dbac8e64a1c7c95bc1f9cababc223d84822c2a89bfcc9ae511d57d2134f206` |
| `crates/ui/src/shell.rs` | `2502b0e0d1e770ae8da8f82a40ad16cade4912e98da159b82025a32af369cbe6` |
| `crates/ui/src/composer.rs` | `4e201a019a4c6ac7847058f6589b27164367cdcb5aa28df44d7682b7aa04a26e` |

## 参考的背景分层

| Source | Connected behavior |
| --- | --- |
| `theme.rs:1130` `window_background_appearance()` | 窗口级原生材质：Linux `Transparent`（只为圆角），macOS/Windows 且主题 frosted 时 `Blurred`，否则 `Opaque`。 |
| `theme.rs:852` `glass()` | 壳层 tint：`surface`(#0d0d0d) @ 0.80（macOS/Windows；Linux 1.0），并带对比度守卫。整窗一张，绘在 `shell.rs:13908` 的 `root.bg(frost)`。 |
| `shell.rs:14376-14378` | 对话列（记录 + 输入框）**不填色**，直接坐在壳玻璃上；#060606 的主面板底色只给内容面（编辑器、Files 列）。 |
| `shell.rs:8555-8574` `sidebar_tone()` | 侧边栏的 tone：`wash(0.05)` 铺满整窗高度，右缘一条 1px hairline；侧边栏内容本身透明。 |
| `theme.rs:1728` `wash()` | 深色为白 @5%，浅色为黑 @5%。 |
| `frost.rs:24` `frosted()` | 浮层的窗内毛玻璃：单个 scene layer 内先画 backdrop blur，再画内容与 tint；`MENU_BLUR = 16`。 |
| `theme.rs:1047` `composer_sidebar_tint()` | 深色 frosted 的 tint：把主面板色调压到壳玻璃上求解，按参考的 **15%** 场景覆盖率返回。 |
| `theme.rs:1036` `composer_surface_border()` | frosted 时药丸的冷色发丝线（深色 9%、浅色 10%），不透明时用主题边框。 |
| `theme.rs:1002` `glass_overlay()` | 浮层 tint：浅色固定 45%，深色按对比度从 50% 起提升。 |
| `popover.rs:317` `surface_bg()` | 弹层与对话框的 tint：深色 frosted 与药丸同色，浅色取 glass overlay，不透明时把 input tint 压平到页面。 |
| `composer.rs:10626`、`composer.rs:11846`、`composer.rs:11864`、`popover.rs:883` | 药丸与弹层都包在 `frosted(radius, MENU_BLUR, card)` 里；药丸的填色取 `composer_surface_bg()`，边线取 `composer_surface_border()`。 |
| `theme.rs:2818` | 参考自身断言：深色弹层 surface 的 alpha 为 0.15。 |

## GCode 的对应实现

共享定义集中在 `internal/design/material.go`（不透明与 frosted 两套配方一处维护）：

| Reference | GCode |
| --- | --- |
| `Theme::glass` | `design.Glass(t, preference, supported)`（既有） |
| `Theme::is_glass` | `design.Frost(t, preference, supported)`：需要窗口真的有材质，壳 tint 才算半透明 |
| `Theme::composer_sidebar_tint` | `design.ComposerSurface` 的深色分支 |
| `Theme::composer_surface_bg` | `design.ComposerSurface` |
| `Theme::composer_surface_border` | `design.ComposerBorder` |
| `Theme::glass_overlay` | `design.GlassOverlay` |
| `popover.rs::surface_bg` | `design.FloatingSurface` |
| `theme::wash(0.05)` | `design.SidebarTone` |

绘制入口 `internal/desktop/frost.go`：

- `frostedSurface` 用 MyGo 的 `plugins/glass` 的 `glass.Blur{Radius: design.MenuBlur}`
  作为材质（窗内 backdrop blur），再用 `Draw` 依次画 tint 与发丝线；顺序与参考
  的 frost 卡片一致（blur → tint → 边）。没有窗口材质时退回不透明填色与边框。
- `frostedComposer` 是药丸的 tint 与发丝线；`floatingSurface` 是弹层与对话框的
  共享 tint。浮层的投影只在不透明时保留（参考：半透明卡片下投影会透出来）。
- 药丸的 1px 边框始终留在布局里（frost 时边框色透明、发丝线由 `Draw` 画出），
  因此紧凑/展开几何与之前一致。

调用点：输入框药丸（`composer_surface.go`）、模型与推理选择器
（`model_picker.go`）、页脚 Git 与用量弹层（`composer_footer_git.go`、
`composer_footer_usage.go`）、设置对话框（`settings_dialogs.go`）、
集成登录对话框（`settings_integrations.go`）、背景调整对话框
（`settings_background.go`）、主题导入对话框（`settings.go`）、删除会话确认
（`sidebar_actions.go`）。

窗口材质：`desktop.NativeVibrancy()`。macOS 用侧栏 vibrancy；Windows 用
Acrylic——MyGo 默认的 Mica 只按壁纸染色，不模糊窗口后面的内容，而参考的壳
frost 在 macOS 与 Windows 上都坐在**模糊**的桌面之上
（`shell.rs:13735`）。选择 Opaque 时两者都退回 `VibrancyNone`。

与参考的一处偏差：参考的 `is_frost()`（窗内毛玻璃）与 `is_glass()`（窗口级
半透明）是两个判断，Linux 上前者为真而后者为假，因此参考在 Linux 上仍会画
窗内模糊。GCode 用一个判断（`design.Frost`：窗口真的有材质且壳 tint 半透明），
因此没有窗口材质的平台走不透明路径，不画模糊。Windows 与 macOS 两条路径与
参考一致。

侧边栏 tone：`sidebar_view.go` 用 `design.SidebarTone` 铺 tone，并在右缘画 1px
hairline（`palette.Border`），与 `sidebar_tone()` 的 wash + `border_r_1` 对应。
偏差：参考的 tone 是绝对定位的一层，带窗口左圆角；GCode 的 tone 是 split 左栏
自己的底色（窗口圆角由系统裁剪整窗），hairline 因此落在 split 分隔线内侧 1px。

## 检查

- `go test ./internal/design`：冻结参考数值——深色药丸 tint 为 15% 覆盖率的
  近黑、浅色 35%、不透明时压平 input tint、浅色浮层 45%、侧边栏 wash 5%。
- `go test ./internal/desktop -run TestComposerPillFollowsTheFrost`：有窗口材质
  时药丸像素为 11,11,11（玻璃上的 tint），无材质时为 39,39,42（压平的 input
  tint），两条路径可区分。
- `go test ./internal/design ./internal/desktop`：通过。
- 全仓 `go test ./...`：`internal/agent`、`internal/engine`、`internal/terminalhost`
  失败，原因是本机 PATH 上没有 ripgrep（`Glob requires ripgrep (rg) on PATH`）；
  这三个包不依赖 `internal/design` 或 `internal/desktop`。
  （2026-11-01：搜索工具改为进程内实现后，`internal/agent` 不再需要 ripgrep。）
- 原生 e2e（`go run ./scripts/desktop-e2e`）：17 个顶层用例通过，5 个失败
  （`TestDesktopLocalPromptReferences`、`TestDesktopLocalModelPickerLayout`、
  `TestDesktopLocalSidebarLayout`、`TestDesktopLocalStorageSwitchAndContinuation`、
  `TestDesktopLocalTranscriptActivity`）。逐项对照：
  `TestDesktopLocalModelPickerLayout` 与 `TestDesktopLocalSidebarLayout` 在把药丸
  临时换回旧实现（`Background(palette.Input)` + 主题边框）后仍以同样断言失败；
  `TestDesktopLocalStorageSwitchAndContinuation` 在模糊半径为 0、以及窗口材质退回
  Mica 时也会失败或偶发通过；`TestDesktopLocalPromptReferences` 与
  `TestDesktopLocalTranscriptActivity` 的失败在上一轮已用改动前二进制复现。
  因此这 5 项不是本轮背景改动引入的，属于既有/环境性失败。

证据（headless 渲染，深色主题，`output/frost/`）：

| File | Content |
| --- | --- |
| `composer-frosted.png` | 有材质：药丸是玻璃上的深色卡片，发丝线收边，侧边栏 tone 与 hairline 可见。 |
| `composer-opaque.png` | 无材质：药丸退回压平的 input tint 实体卡片。 |

## 未验收

- headless 渲染只证明 tint 与边线；**backdrop blur 需要原生窗口**，其像素尚未在
  Windows 原生窗口中核对。
- 参考窗口与 GCode 原生窗口的并排对比尚未进行；上面的数值来自参考源码与其
  自身断言，不是像素验收。
- 浅色主题的浮层配方（45%/35%）只有冻结数值检查，没有原生窗口截图。
