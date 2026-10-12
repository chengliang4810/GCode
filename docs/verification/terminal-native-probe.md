# GCode 原生终端交互验证记录

状态：限定范围通过，整体仍为部分。当前项目树包和历史 final 轮次的 `results.json` 均保留
`PASS_PENDING_VISUAL_REVIEW`，root 对截图和证据复核后在 `review.json` 记录
`PASS_FOR_RECORDED_SCOPE`。本结论只覆盖当前 Intel 打包应用和合成原生事件；wheel/触控板、
拖选/全选、系统剪贴板、物理键盘、中文 IME、真实 API、Apple Silicon/Windows 和用户最终验收
仍未完成，`P4.3`/`A14` 继续为部分。

日期：2026-10-12（Asia/Shanghai）

## 当前结论

当前 canonical 为 `project-tree-layout-confirmed`：executable SHA `00b310de56c927242e8a0cf95bb31807cc251dbf0e0b3fa91b2d29e4c7e965e5`、code SHA `19ba9bb09ed4c91335896ec399ab11ffb6776553cfb457f76d9243662b6f1e44`。原生终端的限定范围已经重新取得该包的运行证据：AX Terminal group、焦点、合成 GUI ASCII
输入、cwd/PID marker、共享文件刷新后的预览/Diff、两组窗口 resize 与 `stty size`、
Shift+PageUp/PageDown 截图以及终端和应用菜单退出释放均已复核。结果仍不能支持完整
`P4.3`/`A14`，因为未验收项目仍然存在。

当前证据位于 [native-terminal/project-tree-current](../../output/acceptance/native-terminal/project-tree-current/)。历史 [native-terminal/final](../../output/acceptance/native-terminal/final/) 对应旧 `f1269b5c…` / `d792ee80…` 包，下方历史执行章节继续保留其原身份。
本记录不把低层 PTY 证据扩大为完整原生交互结论，也不把 `review.json` 的限定范围状态改写成
驱动的 `PASS_PENDING_VISUAL_REVIEW`。

## 当前项目树包执行记录

```sh
python3 docs/verification/scripts/native-terminal-qa.py --workspace-manifest output/acceptance/native-workspace/project-tree-current-confirmed/manifest.json --output output/acceptance/native-terminal/project-tree-current
```

- 复用唯一隔离身份 `GCodeWorkspaceQA1791769437987254000`。driver SHA `49223823ce2cc3c7fa37b4bec23f4702f6faf929e343a935deb9e1be3dfbe84e`，脚本副本保存为 `.py.txt`。
- Terminal 为 `AXGroup`，实际 observed path 为 `group 2 of window GCode of application process GCode`。点击后 process 的 `AXFocusedWindow=GCode`、`AXFocusedUIElement=Terminal`，焦点断言通过。
- 合成 GUI ASCII 命令完成 `NATIVE_READY`、cwd 和 PID marker。交互 shell PID `21950`，cwd 精确等于工作区 manifest 的临时 Git 项目。Shell 追加 `QA_SHELL_EDIT_GCodeWorkspaceQA1791769437987254000` 后，预览及 Diff 读回一致；截图中为第三行新增，保留 TextEdit 已保存的两行。
- 两种实际窗口大小与 `stty size` 成对记录：1440×900 → 51×59，1280×800 → 45×40（rows×cols）。1440 的 bottom 为 marker `143..180`，PageUp 为 `093..143`，PageDown 恢复 `143..180` 与 prompt；1280 对应 `165..180` → `121..165` → `165..180` 与 prompt。
- 正常 shell exit、应用菜单退出与长 `sleep 300` child 清理通过。root 复核 app/shell/child PID `21926`、`22813`、`21950`、`22844`、`23167` 均消失，没有 fallback cleanup errors。
- 输入源 before/after 均为 `com.sogou.inputmethod.sogou.pinyin`，root 用 TIS 独立读回再次确认恢复。没有读写系统剪贴板；合成事件不等于物理键盘或中文 IME 验收。
- 10 张截图实际打开并核对 SHA；源码全部 832 个生产文件、原包和资源与 canonical 清单一致。机械结果保持 `PASS_PENDING_VISUAL_REVIEW`，root [review.json](../../output/acceptance/native-terminal/project-tree-current/review.json) 为 `PASS_FOR_RECORDED_SCOPE`。本轮无产品代码变更，未重复全仓回归或重打包。

截图发现自定义 shell 提示符的部分符号显示为带问号方框。[terminal_native.go](../../internal/desktop/terminal_native.go) 未显式传入 `Font`，[font.go](../../third_party/mygo/plugins/terminal/font.go) 将空字体归一为系统 `monospace`；缺少私有区 Powerline/Nerd glyph 是当前主要推断，尚未读取具体 prompt codepoint，不能将其写成已复现的精确原因。这里的通过只涉及上述命令、文件、resize、键盘翻页与生命周期范围，不包含完整终端字体显示或用户视觉认可。wheel、选择/复制、物理键盘/IME 与其他平台仍缺，`P4.3`/`A14` 保持部分。

## 源码核对

### 产品入口和生命周期

- [`internal/desktop/terminal_native.go:12-25`](../../internal/desktop/terminal_native.go#L12-L26) 从会话 `cwd` 调用 `terminal.New`，并用 `terminal.View(...).Fill().AutoFocus()` 构造视图；`Close` 在 [`:28-32`](../../internal/desktop/terminal_native.go#L28-L32) 调用 MyGo terminal close。
- [`internal/desktop/right_panel.go:144-166`](../../internal/desktop/right_panel.go#L144-L166) 证明终端在右侧资源面板中，路径标题和终端 view 同处外层 column。
- [`internal/desktop/app.go:271-287`](../../internal/desktop/app.go#L271-L287) 的 `App.Close` 取出当前终端并调用 `term.Close()`。
- macOS 的窗口关闭回调在 [`cmd/gcode/main.go:112-122`](../../cmd/gcode/main.go#L112-L122) 对非退出状态调用 `PreventDefault` 后隐藏窗口；应用菜单的“退出 GCode”进入 `cleanup`，最终在 [`cmd/gcode/main.go:216-233`](../../cmd/gcode/main.go#L216-L233) 调用 service 和 view close。因此红色关闭按钮不能作为终端释放证据。

### AX 形态和限制

- [`third_party/mygo/plugins/terminal/view.go:25-38`](../../third_party/mygo/plugins/terminal/view.go#L25-L38) 创建 `ui.Box(c).Focusable().FocusRing(false).Clip().Label("Terminal")`，之后只绑定输入、caret、context menu 和 `Draw`。
- [`third_party/mygo/ui/access.go:158-182`](../../third_party/mygo/ui/access.go#L158-L182) 将这个有 label/焦点、不可点击的自动角色归为 `RoleGroup`；Darwin 表中 [`RoleGroup`](../../third_party/mygo/internal/darwin/access.go#L79-L94) 映射为 `AXGroup`。预期是一个名为 `Terminal` 的 group，但实际索引和父子路径必须先从窗口完整 AX 内容观察，不能硬编码。
- 终端内容由 `Draw(v.paint)` 绘制，不是静态文本子节点；因此 AX 不提供每行 shell 输出、选区文本或可靠的 `AXValue`。可视 marker 应由原生截图或外部 fixture 证明。
- 终端根节点没有 scroll role。AppKit `scrollWheel:` 在 [`surface.go:728-741`](../../third_party/mygo/internal/darwin/surface.go#L728-L741) 转为 `PointerScroll`，而终端 view 在 [`view.go:685-745`](../../third_party/mygo/plugins/terminal/view.go#L685-L745) 决定滚动 scrollback，或在 mouse tracking/alternate screen 下向程序发送鼠标/箭头序列。
- 第一次 AX 请求会在 [`access.go:333-340`](../../third_party/mygo/internal/darwin/access.go#L333-L340) 打开 accessibility 更新；最终探针在点击终端后从应用进程查询 `AXFocusedWindow` 和 `AXFocusedUIElement`，分别比较窗口名 `GCode` 与 UI element description `Terminal`。不要把 window 的 `AXFocused` 属性当作终端焦点依据；窗口内 UI runtime focused node 的实现仍见 [`access.go:772-781`](../../third_party/mygo/internal/darwin/access.go#L772-L781)。

### 输入、resize、选择和复制实现

- Darwin surface 的 `keyDown:` 先发送键事件，并通过 `interpretKeyEvents:` 让 IME 处理输入；`keyUp:` 发送释放事件，见 [`surface.go:742-767`](../../third_party/mygo/internal/darwin/surface.go#L742-L767)。UI runtime 将键、文本、组合、命令和滚轮交付给终端 view，见 [`ui/input.go:55-90`](../../third_party/mygo/ui/input.go#L55-L90)。
- 终端 view 对 macOS Command+C/V、`selectAll`、Shift+PageUp/PageDown 以及 text/compose/pointer/scroll 有分支处理，见 [`view.go:240-331`](../../third_party/mygo/plugins/terminal/view.go#L240-L331)。
- 视图尺寸在 [`paint.go:61-85`](../../third_party/mygo/plugins/terminal/paint.go#L61-L85) 按字体 cell 计算，`paint` 在 [`paint.go:90-105`](../../third_party/mygo/plugins/terminal/paint.go#L90-L105) 调用 `t.resize`；因此只有原生窗口改变后再读 shell 的 `stty size`，才能验证 GUI resize 到 PTY 的路径。
- 拖选由 [`view.go:513-595`](../../third_party/mygo/plugins/terminal/view.go#L513-L595) 更新 VT selection；`copy` 从 `SelectionText()` 读取后在 [`view.go:467-478`](../../third_party/mygo/plugins/terminal/view.go#L467-L478) 调用 `WriteClipboard`。本轮禁止系统剪贴板操作，只能记录可视选区，不能读取剪贴板作等值断言。
- [`terminal.go:577-597`](../../third_party/mygo/plugins/terminal/terminal.go#L577-L597) 的 `Send`/`Paste` 直接向 PTY 输入；它们适合低层测试，不能当作 native keyboard 证据。

## 已有低层证据

### GCode 打包资源和 PTY 主路径

[`terminal-qa-unicode.log`](../../output/acceptance/stage-4/terminal/terminal-qa-unicode.log) 记录了当前打包 `GCode.app` 资源在临时真实目录上的结果：

```text
terminal_load=ok
shell_cwd=pass
shell_input=pass
shell_unicode=pass
resize_1=pass cols=80 rows=24
resize_2=pass cols=120 rows=40
resize_3=pass cols=40 rows=10
resize_4=pass cols=100 rows=30
shell_exit_done=true exit_code=0
close_done=true elapsed=2.001s
close_case_pid_after=gone
result=PASS
```

这份结果支持 terminal load、cwd、输入、CJK/emoji、四组直接 PTY resize、shell 正常退出和 `Close` 后 PID 消失。它没有 GCode 原生窗口 PID、AX tree、GUI 事件、窗口尺寸、wheel、物理键盘或 OS clipboard 记录。

### MyGo terminal 层

- [`mygo-pty-unicode.log`](../../output/acceptance/stage-4/terminal/mygo-pty-unicode.log) 证明原始 UTF-8 命令字节和输出有效。
- [`mygo-vt-unicode.log`](../../output/acceptance/stage-4/terminal/mygo-vt-unicode.log) 证明 Ghostty VT 的 UTF-8/CJK/emoji cell 以及 `SelectionText` 低层结果。
- [`mygo-terminal-unicode.log`](../../output/acceptance/stage-4/terminal/mygo-terminal-unicode.log) 通过真实 `/bin/sh -i` PTY 和 MyGo View 测试验证 UTF-8、选区、复制调用路径及低层 resize。

这些日志的证据等级限于 MyGo/PTY 测试宿主。`Terminal.Send`、`Terminal.Paste`、`ui.Tester` 和测试宿主的 clipboard 都绕过或替代 GCode 原生 AppKit window，不能提升 A14 的 native 交互状态。

## 历史 final 正式包原生探针执行记录

下方证据目录为 [`output/acceptance/native-terminal/final/`](../../output/acceptance/native-terminal/final/)，source executable 为 `f1269b5c8a132a898565c9e06ab4660b389163158e814ee29986ac1f69b3a7b8`、code 为 `d792ee8083451dc9b1a027f9f973edfe6f8784bd9aac0591731ad56609ad4fb5`，不属于当前项目树包。
驱动 `results.json` 的机械结果为 `PASS_PENDING_VISUAL_REVIEW`；root 核对截图哈希、marker、
进程和输入源恢复后，在 `review.json` 记录 `PASS_FOR_RECORDED_SCOPE`。两个状态分别表示驱动
等待复核和当前限定范围已经复核通过，不能合并成完整产品通过。

### 最终轮次的证据

- AX 全树中的终端候选实际为 `AXGroup`，description/`AXDescription` 为 `Terminal`。首次终端
  frame 是 `x=1144,y=200,width=592,height=806`，记录路径为 `group 2 of window GCode of
  application process GCode`；点击 frame 中心后，进程 frontmost 为 true，`AXFocusedWindow`
  为 `GCode`，`AXFocusedUIElement` 为 `Terminal`，终端焦点为 true。重启后的终端仍以同一
  observed path 重新发现，frame 为 1144,200,432×706。
- 合成 GUI ASCII 命令写出 `NATIVE_READY`、cwd 和 PID marker。shell cwd 是 workspace manifest
  对应的临时项目目录，交互 shell PID 为 `30033`；fixture 和截图保存在最终输出目录。
- shell 追加 `QA_SHELL_EDIT_GCodeWorkspaceQA1791748189026796000` 到共享 QA 文件；刷新文件
  面板后 preview readback 和 Diff readback 均为 true，`shell-edit-diff-native.png` 复核该
  marker 作为新增第三行出现。
- 实际窗口尺寸和 PTY 尺寸成对记录：1440×900 的 `stty size` 为 51×68，1280×800 为 45×49。
  180 行 marker 的截图复核为：1440×900 bottom 140..180、PageUp 090..140、PageDown 返回
  140..180 和 prompt；1280×800 bottom 157..180、PageUp 113..157、PageDown 返回 157..180
  和 prompt。对应截图为 `terminal-1440x900-{bottom,pageup,pagedown}-native.png` 和
  `terminal-1280x800-{bottom,pageup,pagedown}-native.png`。
- 终端 `exit` 后 PID `30033` 消失。应用菜单退出的长任务也完成清理；`review.json` 确认五个
  记录 PID `30017`、`30989`、`30033`、`31018`、`31420` 全部消失，其中 `31018` 为长 shell，
  `31420` 为 `sleep 300` child。
- 为避免搜狗输入源影响合成 ASCII，驱动每次输入命令前通过 macOS TIS 选择当前
  ASCII-capable source。`manifest.json` 记录输入源 before/after 均为
  `com.sogou.inputmethod.sogou.pinyin`，`input_source_restored=true`；这不是物理键盘或中文
  IME 验收。
- source executable、isolated executable、资源和源码指纹最终一致性均为 true；root 核对的
  截图哈希也全部一致。

### 失败轮次和修正

- round-1 在 `focus-and-keyboard` 失败：焦点脚本在已作用域的 application process 内再次声明
  `first application process`，并直接把 AX 对象转成文本，导致焦点查询失败，未产生截图。
- round-2 修正焦点查询后进入同一阶段，但 `native GUI keyboard marker did not arrive`，因此
  没有把该轮机械结果当作通过。
- final 轮次固定为实际 Terminal frame/焦点断言，使用 TIS 临时 ASCII 输入，并在 `finally` 中
  恢复搜狗输入源后完成上述证据。失败轮次目录继续保留，不能覆盖最终证据。

## 剩余未验收项目和判定

| 证据 | 当前判定 |
| --- | --- |
| AX Terminal 节点、点击后焦点、GUI ASCII 命令、cwd/PID marker | 限定范围通过 |
| 共享文件 shell 修改、文件预览和 Diff marker | 限定范围通过 |
| 1440×900、1280×800 resize 与 `stty size` | 限定范围通过 |
| Shift+PageUp/PageDown scrollback 及截图复核 | 限定范围通过 |
| 终端 `exit`、应用菜单退出和五个记录 PID 清理 | 限定范围通过 |
| wheel/触控板 | 未验收 |
| 拖选/全选及系统剪贴板 | 未验收；本轮未触碰 OS clipboard |
| 物理键盘和中文 IME 候选/组合输入 | 未验收 |
| 真实 API、Apple Silicon/Windows、用户最终验收 | 未验收 |

因此验收矩阵中的 `A14`/`P4.3` 必须保持“部分”。

## 当前内置字体包复核（2026-10-12）

当前 canonical 已更新为 `terminal-font-bundled`，不是前文项目树包。工作区、固定 glyph 和完整终端记录分别位于 `output/acceptance/native-workspace/terminal-font-bundled`、`output/acceptance/terminal-font/bundled-native`、`output/acceptance/native-terminal/terminal-font-bundled`，root 均已限定范围复核。新包的 stty 为 1440×900 下 61×63、1280×800 下 54×43；翻页 marker 为 131..180→071..131→131..180 与 154..180→101..154→154..180。5 个 app/shell/child PID 消失，输入源恢复且独立读回。固定四 PUA 缺字已闭环；物理键盘/IME、鼠标选择/滚轮/剪贴板及任意字符仍缺，A14 仍为部分。身份与截图见 [terminal-font-bundled.md](terminal-font-bundled.md)，历史结果保持原包归属。
