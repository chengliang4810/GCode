# 正式包原生工作区验收

日期：2026-10-12。环境：macOS 14.8.7（23J520）、MacBookPro16,1、Intel darwin/amd64、Go 1.27.2。

结论：当前项目树/布局包重新验证了文件浏览、目录切换、中文/emoji 预览、二进制提示、真实 Git Diff，以及从产品打开 TextEdit、保存后刷新预览/Diff 的链路。随后独立终端驱动完成合成 GUI ASCII 输入、实际 cwd、GUI 到 PTY resize、键盘翻页和退出清理的限定复核。视觉候选及根目标仍未获得用户验收。

## 当前项目树包复核

当前 canonical 包为 `project-tree-layout-confirmed`，executable SHA `00b310de56c927242e8a0cf95bb31807cc251dbf0e0b3fa91b2d29e4c7e965e5`，code SHA `19ba9bb09ed4c91335896ec399ab11ffb6776553cfb457f76d9243662b6f1e44`。成功产物位于 [project-tree-current-confirmed](../../output/acceptance/native-workspace/project-tree-current-confirmed/)，命令为：

```sh
python3 docs/verification/scripts/native-workspace-qa.py --bundle build/darwin-amd64/GCode.app --output output/acceptance/native-workspace/project-tree-current-confirmed
```

隔离身份为 `GCodeWorkspaceQA1791769437987254000`，应用 PID `21394` 正常退出 0 后消失。driver SHA 为 `0fe7feb92122e893e7263eed6f3cff278074198b2fa81b354f737db592655d94`，执行脚本副本在产物中保存为 `.py.txt`。4 张真实原生截图均已实际打开并核对 SHA；tracked/untracked Diff、TextEdit 精确窗口保存、预览和真实 Git Diff 的内容一致。`review.json` 为 `PASS_FOR_RECORDED_SCOPE`，832 个生产文件逐项匹配 canonical 清单，源码和原包未变。

之前 [project-tree-current](../../output/acceptance/native-workspace/project-tree-current/) 运行失败：旧脚本访问 `text field 1 of window 1`，项目树调整后的实际 AX 位于 `scroll area 1` 内。仅纠正 driver 定位，在独立 confirmed 目录重跑；未修改产品代码，失败目录保留。

随后 [native-terminal/project-tree-current](../../output/acceptance/native-terminal/project-tree-current/) 复用此身份、数据目录、包和项目，root 已完成限定复核。1440×900 与 1280×800 的 `stty size` 分别为 51×59 和 45×40；详情和完整交互限制见 [terminal-native-probe.md](terminal-native-probe.md)。

## 历史 final 包记录

下方 `native-workspace/final` 和 `native-terminal/final` 保留当时的 `f1269b5c…` / `d792ee80…` 身份与操作，不属于当前项目树包。历史段落中的正式包结论只覆盖该轮记录。

## 身份与隔离

驱动：[native-workspace-qa.py](scripts/native-workspace-qa.py)。命令：

```sh
python3 docs/verification/scripts/native-workspace-qa.py --output output/acceptance/native-workspace/final
```

使用 `build/darwin-amd64/GCode.app` 的副本，只替换 bundle identity 元数据并重新 ad hoc 签名。隔离身份 `GCodeWorkspaceQA1791748189026796000`，应用 PID `24120`；数据目录为该身份独立的 Application Support 目录。没有配置模型服务或密钥，没有 API 请求，不写用户 Keychain，不使用生产 GCode/zero 数据。

- 原正式可执行文件 SHA-256：`f1269b5c8a132a898565c9e06ab4660b389163158e814ee29986ac1f69b3a7b8`。
- 实际 `__TEXT,__text` SHA-256：`d792ee8083451dc9b1a027f9f973edfe6f8784bd9aac0591731ad56609ad4fb5`。
- manifest 验证隔离副本的实际代码与全部 Resources 相同，原正式包及源码在本驱动结束后未变。
- 场景使用唯一 QA Git 项目，基线提交由驱动创建，只操作该目录；已跟踪改动、未跟踪文件和二进制文件均为夹具。

## 已完成的原生交互

| 场景 | 结果 | 原始证据 |
| --- | --- | --- |
| 原生打开项目、新会话绑定实际 cwd | 通过 | `created-session-ax.txt`、隔离 `controller.json` |
| 文件列表包含已跟踪、未跟踪、子目录和二进制文件 | 通过 | `initial-files-ax.txt` |
| 进入 nested、查看 child.txt、返回项目根目录 | 通过 | `nested-preview-ax.txt` |
| 二进制预览只显示明确提示 | 通过 | `binary-preview-ax.txt` |
| UTF-8 中文/emoji 文件预览 | 通过 | AX exact text 断言 |
| 实际 Git 已跟踪/未跟踪 Diff，与 git diff 相符 | 通过 | `git-before.txt`、`tracked-diff-ax.txt`、`tracked-diff-native.png` |
| 产品“编辑”默认打开唯一 QA 文件的 TextEdit 窗口 | 通过 | `textedit-qa-ax.txt`、manifest TextEdit PID/路径/hash |
| 对唯一 TextEdit 窗口 AX 修改并读回，用 Command+S 保存 | 通过 | 精确 `AXFocusedWindow` 断言，磁盘内容逐字匹配，保存 SHA |
| 只关闭该 QA TextEdit 窗口，不退出 TextEdit | 通过 | 精确 AXCloseButton，窗口消失断言 |
| 返回 GCode，刷新后预览和 Diff 显示 TextEdit 新内容 | 通过 | `edited-preview-ax.txt`、`edited-diff-ax.txt`、`git-after-edit.txt` 及两张原生图 |
| 正常应用菜单退出 | 通过 | `results.json` 的 `normal_exit: true`、manifest `qa_exit_code: 0` |

所有文件位于 `output/acceptance/native-workspace/final/`。三张正式包工作区原生图已实际打开复核；预览显示 `QA edited by TextEdit` 与中文保存行，Diff 显示两行删除/两行新增，和真实 `git diff` 一致。截图由 `screencapture -l <CGWindow>` 捕获，PID、CGWindow 与 SHA 均在 manifest 登记。

终端初始截图只证明面板已打开和实际 AX group 存在；0.5 秒固定等待不能证明 shell ready。其内容为自绘，不把 AX 空文本当作终端输出丢失。

## 夹具修正历史

保留 `round-1/`、`round-2/`、`round-3/` 的失败记录；它们未用于提升产品通过状态。

- round-1：驱动把 Diff trigger 当作 button。实际为带 `AXExpanded` 的 UI element；真实 Diff 已存在，后改为直接断言真实增删行的 AX static text。
- round-2：TextEdit AppleScript `save ... in ...` 调用等待，终止本轮 osascript 后，限定窗口的原生 Command+S 实际保存成功。没有证据判定该调用等待的底层原因，没有把它作为 GCode 编辑器故障。
- round-3：驱动误用 window 的 `AXFocused` 判定焦点。实际 process 的 `AXFocusedWindow` 指向正确 QA 窗口；最终改为 AXRaise 后等待并断言精确 focused window，再发送保存快捷键。
- round-3 最初写入 `final/`，失败后整体归档为 `round-3/`；其 manifest 中的绝对路径是捕获时的路径，由 `archive-location.json` 记录归档映射。没有重新使用其 controller 数据或包作为成功证据。

失败轮次的 QA 应用均已退出，相关唯一 TextEdit 文档已保存并关闭。最终脚本的每个子命令有 30 秒超时，所有 AX 等待有期限。

## 边界

此历史 final 记录证明当时 Intel 正式包的原生 UI、真实文件/Git 与 TextEdit 链路。AX 设置文字及合成快捷键不能代替物理键盘和中文输入法组合/候选验证。没有操作系统剪贴板。非 Git 项目的应用层证据见阶段 3，本轮没有额外重跑。真实模型 API、Apple Silicon/Windows、长会话滚动及用户视觉认可仍分别保持未验证。

## 原生终端与共享修改收口

后续使用 [native-terminal-qa.py](scripts/native-terminal-qa.py) 复用上面的隔离身份、包和项目，不再重建或重复文件/TextEdit 链路。成功证据：`output/acceptance/native-terminal/final/{manifest.json,results.json,review.json}`；完整操作与边界见 [terminal-native-probe.md](terminal-native-probe.md)。

- Terminal 真实为 `group 2 of window 1`，`AXDescription=Terminal`，process `AXFocusedUIElement` 的 description 精确匹配 Terminal。
- 合成原生键输入/Return 后 shell 写出 cwd 和 PID 文件；cwd 与同会话 QA 项目一致。
- 在同一 TextEdit 已修改文件追加唯一 `QA_SHELL_EDIT_...` 行，刷新后文件预览与真实 Git Diff 逐字包含该行；root 打开 `shell-edit-diff-native.png` 核对它作为第三行新增，与外部编辑内容共同保留。A7 的当前 Intel 共享变更最小链路通过。
- 1440×900 窗口对应 `stty size=51 68`（rows cols），1280×800 对应 `45 49`，证明 GUI resize 已到达 PTY。
- 两种尺寸均执行 Shift+PageUp/PageDown；root 实际查看六张图确认旧序号出现，并恢复底部提示符。此结论仅支持键盘翻页，wheel/触控板、选择/复制仍未验证。
- 正常 shell `exit` 后 PID 消失；第二次启动中真实 `sleep 300` 子进程随应用菜单退出清理，shell/child PID 均消失。
- 用户原输入源 `com.sogou.inputmethod.sogou.pinyin` 在驱动结束后恢复；没有操作剪贴板。此前合成 ASCII 输入被当前搜狗输入法转换的诊断图保留，不能作为产品 ASCII 输入丢失或 IME 通过证据。
- root 已复核全部截图 SHA、成功轮次记录的五个 app/shell/child PID 消失、源码和源包不变。没有生产代码变更，因此未重复已经通过的全仓构建/回归。
