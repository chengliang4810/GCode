# /goal：GCode 原生终端交互探针

状态：正式包原生终端探针已完成当前 Intel 设备上的限定范围复核，root 的人工截图复核为
`PASS_FOR_RECORDED_SCOPE`；驱动原始结果仍保留 `PASS_PENDING_VISUAL_REVIEW`。wheel/触控板、
拖选/全选、系统剪贴板、物理键盘和中文 IME 未验收，`P4.3`/`A14` 继续为部分。

日期：2026-10-12（Asia/Shanghai）

## 目标

在当前 macOS Intel 设备上，针对真实打包的 GCode.app 右侧终端面板，补齐从原生窗口输入面到终端 PTY 的可复核证据：

- 通过 AX 树先确认窗口内终端元素的实际 role、label、frame 和路径，再确认点击后的 focused element。
- 通过真实 AppKit surface 的 GUI 键盘事件验证命令输入、回车、焦点和终端输出 marker；把合成事件与物理键盘、中文输入法候选窗口分开记录。
- 通过窗口 resize 后 shell 的 `stty size` 输出验证视图尺寸确实传入 PTY。
- 分别验证 `Shift+PageUp/PageDown` 和真实 wheel 的 scrollback 行为；遇到 alternate screen 或 mouse tracking 时记录程序接管滚轮的结果。
- 验证拖选或“全选”后的可视选区反馈。当前任务禁止操作系统剪贴板，因此不读取或比较 `pbpaste`，也不把“复制函数存在”写成剪贴板验收通过。
- 通过终端内 `exit` 和应用菜单“退出 GCode”验证 shell/PTY 释放；窗口红色关闭按钮只证明隐藏，不作为释放证据。

本目标只覆盖当前 macOS 14.8.7、`darwin/amd64` 的 GCode 原生窗口。Apple Silicon、Windows、真实物理键盘和真实中文 IME 候选交互应单独记录，不能由本机合成事件或离屏测试代替。

## 实现边界

GCode 的 [`NativeTerminalFactory`](../../internal/desktop/terminal_native.go#L10-L32) 用会话 `cwd` 创建 `terminal.Terminal`，滚动缓存为 10 MiB；终端视图调用 `terminal.View(...).Fill().AutoFocus()`。右侧资源面板将路径标题和终端视图放进同一列，[`right_panel.go`](../../internal/desktop/right_panel.go#L144-L166) 是实际产品入口。

MyGo 的 [`terminal.View`](../../third_party/mygo/plugins/terminal/view.go#L13-L39) 是一个 `Focusable`、`Label("Terminal")`、`Draw` 自绘的 `ui.Box`。自动 AX 角色规则会把“有 label 或可聚焦、不可编辑且不可点击”的节点归为 `RoleGroup`，[`ui/access.go`](../../third_party/mygo/ui/access.go#L158-L182) 再由 Darwin 映射为 `AXGroup`，[`internal/darwin/access.go`](../../third_party/mygo/internal/darwin/access.go#L79-L94)。因此源码预期存在名为 `Terminal` 的可聚焦 group；实际 AX 索引和嵌套路径必须以本次 `entire contents of window 1` 输出为准，不能预先假设 `scroll area 1` 或某个 `UI element` 编号。

终端画面在 [`view.go:36`](../../third_party/mygo/plugins/terminal/view.go#L25-L38) 通过 `Draw(v.paint)` 绘制，不是 `ui.Text` 或 `RichText` 节点。AX 树预计不会提供每一行 shell 输出、终端选区或可读 `AXValue`；marker 应通过原生窗口截图或外部 fixture 文件确认。终端根节点也没有 `Scroll` 标记，不能把 AX scroll action 当作滚动路径。

AppKit surface 将 first responder、`scrollWheel:`, `keyDown:`, `keyUp:` 和 `interpretKeyEvents:` 转为 MyGo surface event，[`surface.go`](../../third_party/mygo/internal/darwin/surface.go#L676-L767)；UI runtime 再转交 `InputKeyDown`、`InputText`、`InputCompose`、`InputScroll` 等事件，[`ui/input.go`](../../third_party/mygo/ui/input.go#L55-L90)。终端 view 对 Command+C/V、`selectAll`、Shift+PageUp/PageDown、文本输入、组合输入、pointer drag 和 wheel 都有明确处理，[`view.go`](../../third_party/mygo/plugins/terminal/view.go#L240-L331)。

## 证据等级和判定

| 项目 | 可支持的证据 | 当前状态 |
| --- | --- | --- |
| terminal factory、session `cwd`、PTY、Unicode、resize、shell exit、`Close` | 直接 terminal/PTY 驱动和 MyGo 定向测试 | 已有；见 [`terminal-runtime.md`](terminal-runtime.md) |
| 原生窗口发现终端和焦点 | 实际 GCode.app 的 AX tree、点击后 focused element | 限定范围通过；`AXGroup` description/AXDescription 为 `Terminal`，frame 为 1144,200,592×806，路径记录为 `group 2 of window GCode of application process GCode`；进程焦点为 `GCode`/`Terminal` |
| GUI 键盘命令输入 | 经过 AppKit surface 的合成 key/text event，外部 marker 与退出状态 | 限定范围通过；System Events ASCII 事件写出命令、cwd 和 PID marker；不等同于物理键盘或中文 IME |
| 物理键盘和中文 IME | 物理键盘输入、候选窗口、组合态、Enter/Escape 选择或取消 | 未验证；合成 Unicode 不能提升此项 |
| resize 到 PTY | 真实窗口尺寸 + 终端内 `stty size` 记录 | 限定范围通过；1440×900 为 51×68，1280×800 为 45×49 |
| scrollback | Shift+PageUp/PageDown 或实际 wheel 后旧 marker 可见 | PageUp/PageDown 限定范围通过；1440×900 为 bottom 140..180、PageUp 090..140、PageDown 返回；1280×800 为 bottom 157..180、PageUp 113..157、PageDown 返回 |
| 选择反馈 | 原生拖选或 Select All 后的可视高亮 | 未验收 |
| 系统剪贴板 | `SelectionText` 经 `WriteClipboard` 后的 OS clipboard 内容 | 本轮受限；不执行、不宣称通过 |
| 退出释放 | 原生终端内退出、应用菜单退出后 shell PID 消失 | 限定范围通过；交互 shell、两次应用 PID、长 shell 和 `sleep 300` child 共五个记录 PID 均消失 |

完成后，只有同时具备实际窗口事件、外部 marker/进程记录和必要截图的项目才能标记为原生通过。源码存在、低层测试、AX 控件可点击或单张截图都不单独构成 A14 的完整证据。

## 探针执行约束

1. 先启动当前打包 GCode.app，取得其 PID；使用 System Events 导出 `entire contents of window 1`。从输出记录 Terminal 的实际 AX role/name/description/frame/path，再进行点击和焦点查询。
2. 点击观察到的终端可视 frame 后，查询窗口的 focused UI element 或该节点的 `AXFocused`。AX 首次查询会触发 accessibility tree 建立，[`accessRequested`](../../third_party/mygo/internal/darwin/access.go#L333-L340) 的行为不能被“没有预先看到节点”误判为终端不存在。
3. 终端命令使用本次探针输出目录下的 `fixture/` 文件作为外部断言。命令必须由 GUI keyboard event 输入；不得调用 `Terminal.Send`、`Terminal.Paste`、`ui.Tester.Key/Type/Scroll` 或其他直接写 PTY 的路径。
4. resize 后把原生窗口尺寸和终端内 `stty size` 成对记录。视图尺寸由 [`paint.go`](../../third_party/mygo/plugins/terminal/paint.go#L61-L105) 的 cell 计算间接驱动 PTY resize。
5. 先用 Shift+PageUp/PageDown 验证稳定的 page scroll，再在终端可视坐标发送真实 wheel event。输出带唯一 marker 的多行内容，滚动前后用截图或可视记录确认旧 marker；如果程序打开 mouse tracking 或 alternate screen，按程序收到的鼠标/箭头序列记录结果。
6. 在未启用 mouse tracking 的 shell 页面上做原生 mouse drag 或 Command+A，记录选区高亮。复制和 `pbpaste` 属于本轮禁用操作，不能为了闭环写入或读取系统剪贴板。
7. 终端内记录 shell PID 后通过 GUI 输入 `exit` 并回车，外部轮询 PID 消失；另启动长睡眠 shell，走应用菜单“退出 GCode”，验证 [`App.Close`](../../internal/desktop/app.go#L271-L287) 和 [`closeResources`](../../cmd/gcode/main.go#L216-L233) 到达 terminal close。点击窗口关闭按钮只会触发隐藏逻辑，[`main.go`](../../cmd/gcode/main.go#L112-L122) 不足以证明资源释放。

## 正式驱动和最终证据

正式驱动位于 [`native-terminal-qa.py`](../verification/scripts/native-terminal-qa.py)。它读取
`native-workspace-qa.py` 生成的 workspace manifest，先校验当前源码、原始包和 isolated bundle
的身份，再复用 manifest 中的 isolated bundle、data directory 和 `textedit.qa_file`。驱动通过
System Events 向真实原生窗口发送 ASCII 键盘事件，并从 AX 树、进程记录、fixture marker、窗口
截图和重启后的 PID 清理结果写出证据。驱动不调用真实 API、不读写系统剪贴板，也不把合成输入
当作物理键盘或中文 IME 证据。

root 已在仓库根目录完成最终轮次：

```sh
python3 docs/verification/scripts/native-terminal-qa.py \
  --workspace-manifest output/acceptance/native-workspace/final/manifest.json \
  --output output/acceptance/native-terminal/final
```

最终输出目录是 `output/acceptance/native-terminal/final/`，包括 `manifest.json`、`results.json`、
`review.json`、AX 全树文本、应用日志、fixture marker 和原生窗口截图。源码、原始包和 isolated
bundle 的最终完整性均为 true；原始输入源为 `com.sogou.inputmethod.sogou.pinyin`，驱动在每次
合成 ASCII 命令前临时选择 TIS 的 ASCII-capable 输入源，并在 `finally` 中恢复搜狗输入源，
`input_source_restored=true`。

驱动当前机械覆盖范围是 AX 终端 group 和焦点、GUI 键盘命令输入、cwd/PID marker、窗口尺寸与
`stty size`、180 行 scroll marker、Shift+PageUp/PageDown、终端内 `exit`、应用菜单退出后的
shell/child PID 清理，以及 shell 修改文件后的文件面板预览和 Diff marker。真实 wheel、拖选或
全选反馈、物理键盘、中文 IME 和系统剪贴板仍保持未验证，不能由该驱动的截图或 marker 推断通过。

驱动只有在所有机械步骤完成、必要截图已经生成且退出清理通过时，才把 `results.json` 和
`manifest.json` 的结果写为 `PASS_PENDING_VISUAL_REVIEW`。该状态表示等待 root 对截图进行人工
视觉复核，尤其是终端可视区域、滚动前后的 marker 和窗口状态；视觉复核完成前不能把它改写成
原生终端全面通过。最终 `results.json` 仍保留 `PASS_PENDING_VISUAL_REVIEW`；root 另写入
`review.json`，其 `PASS_FOR_RECORDED_SCOPE` 只表示当前 Intel 打包应用、合成原生事件和已复核
截图覆盖的限定范围通过，不能覆盖未验收项目或提升 `P4.3`/`A14`。

最终记录的限定范围证据包括：

- AX 树把终端识别为 `AXGroup`，description/`AXDescription` 为 `Terminal`；实际终端 frame 为
  `x=1144,y=200,width=592,height=806`，观察路径记录为 `group 2 of window GCode of application process GCode`。点击 frame 后，进程 frontmost 为 true，`AXFocusedWindow=GCode`、`AXFocusedUIElement=Terminal`，终端焦点为 true。
- GUI ASCII 命令写出 `NATIVE_READY`、shell cwd 和 PID marker；shell cwd 为 workspace manifest
  的临时项目目录，交互 shell PID 为 `30033`，通过终端 `exit` 后消失。
- 同一隔离 workspace 文件由终端 shell 追加 `QA_SHELL_EDIT_GCodeWorkspaceQA1791748189026796000`；刷新文件面板后预览读回，Diff 截图确认该 marker 作为新增第三行出现。
- 1440×900 的 `stty size` 为 51×68，1280×800 为 45×49。180 行 marker 的截图复核显示：
  1440×900 bottom 为 140..180、PageUp 为 090..140、PageDown 返回 140..180 和 prompt；
  1280×800 bottom 为 157..180、PageUp 为 113..157、PageDown 返回 157..180 和 prompt。
  对应截图为 `terminal-1440x900-{bottom,pageup,pagedown}-native.png` 和
  `terminal-1280x800-{bottom,pageup,pagedown}-native.png`。
- 最终应用退出后，root 在 `review.json` 中确认五个记录 PID `30017`、`30989`、`30033`、`31018`、
  `31420` 均已消失；其中 `31018` 为长 shell，`31420` 为 `sleep 300` child。

历史轮次保留为失败证据：round-1 在 `focus-and-keyboard` 阶段因嵌套的 AppleScript process
作用域和 AX 对象文本化查询失败，未产生截图；修正焦点查询后，round-2 在同一阶段未等到
native GUI keyboard marker。最终轮次通过使用精确 Terminal frame/焦点记录和 TIS 临时 ASCII
输入源完成上述机械步骤，并在 `finally` 恢复原始搜狗输入源。任何机械步骤失败都写为 `FAIL`，
并保留已生成的证据和失败 phase。

本节描述 root 已执行的最终轮次和它支持的限定范围，不表示完整原生终端功能或跨平台验收已经完成。

## 完成条件

- 正式驱动覆盖的 AX 实际路径、终端焦点、GUI 键盘输入、resize、Shift+PageUp/PageDown scrollback 和两种退出路径各有独立记录。
- 真实 wheel、拖选或全选反馈仍是独立的未验证项目，不因 `PASS_PENDING_VISUAL_REVIEW` 自动提升。
- 物理键盘/中文 IME 和合成输入分别标注，未执行项保持“未验证”。
- 系统剪贴板在本轮保持“受限/未验收”。
- 记录只引用当前应用包、当前源码和本次 fixture；不把已有直接 PTY 或离屏 MyGo 证据扩大为原生窗口结论。
