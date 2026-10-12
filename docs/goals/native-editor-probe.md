# /goal：TextEdit 唯一临时文件原生编辑器探针

日期：2026-10-12（Asia/Shanghai）

状态：静态探查完成；root 已在正式包隔离 QA 驱动中完成唯一 TextEdit 文档的原生编辑、保存、关闭与回读。

## 目标

为正式包原生工作区 QA 提供一条可重放的 macOS 外部编辑器探针：只创建并打开 QA 驱动生成的唯一临时文件，完成一次原生打开、修改、保存、关闭，再用文件内容和 AX 窗口状态确认结果。探针不能触碰用户已有 TextEdit 文档、系统剪贴板或生产项目。

## 所有权

- 本目标唯一维护：`docs/goals/native-editor-probe.md`、`docs/verification/native-editor-probe.md`。
- 产品代码、正式包构建、`docs/verification/scripts/native-product-qa.py` 和其他验收文档由 root 维护；本目标不修改它们。

## 已确认事实

1. `internal/workspace/editor.go:24-31` 在 macOS 且未配置 `GCODE_EDITOR`、`VISUAL`、`EDITOR` 时返回 `open -a TextEdit <path>`。`Service.OpenEditor` 在 `:352-365` 先解析并 `stat` 目标，拒绝目录；默认 `EditorOptions.Wait=false`，因此 `:375-383` 不等待 TextEdit 退出。
2. `/System/Applications/TextEdit.app/Contents/Info.plist` 声明 `NSAppleScriptEnabled=1`。`TextEdit.scriptSuite` 声明 `Document` 的 `textStorage` 关系，`TextEdit.scriptTerminology` 将其命名为 AppleScript 的 `text`，所以 `set text of qaDoc to ...`、`save qaDoc in POSIX file ...` 具备静态字典术语且可由 `osacompile` 编译；`save ... in ...` 的实际运行不作为通过依据，round-2 中该调用长时间等待并由 root 终止，底层原因未确定。
3. 现有 `native-product-qa.py:70-72` 用 `System Events` 查找指定 PID 的 application process。编辑器探针应沿用 PID/精确窗口标题约束，禁止 `window 1`、`front document`、`quit` 等会误触已有文档的宽泛选择器。

## 安全边界

- QA 驱动先在隔离临时目录创建唯一文件名和唯一内容标记；路径和标题都必须包含随机 nonce，且运行前确认目标不存在或内容等于本轮初始标记。
- AppleScript/AX 只引用该绝对路径对应的 `qaDoc` 和该唯一文件名对应的窗口。脚本发现路径、标题或匹配文档数量不符时立即失败，不尝试猜测或切换到前台文档。
- 保存后由驱动读取这一个 QA 文件，检查它只包含修改后的标记；关闭只点击目标窗口的精确 `AXCloseButton`，不退出 TextEdit、不枚举用户窗口、不读取文档正文、不访问剪贴板。
- 本子目标没有操作实际窗口；root 的正式包驱动另行完成了 S3 运行并保留路径、PID、窗口标题、保存后 hash、关闭后窗口不存在和退出清理记录。最终运行采用 AX text area 设置/读回、`AXRaise` 后等待 process 的 `AXFocusedWindow` 名称匹配、Command+S、磁盘 hash 和精确 `AXCloseButton`，不使用窗口的 `AXFocused` 属性。

## root 正式包实际证据

证据目录：`output/acceptance/native-workspace/final/`；汇总为 `results.json` 和 `manifest.json`，驱动为 `docs/verification/scripts/native-workspace-qa.py`。

- 隔离身份：`GCodeWorkspaceQA1791748189026796000`；GCode PID `24120`；TextEdit PID `20990`。
- 唯一 QA 文件：`output/acceptance/native-workspace/final/project-GCodeWorkspaceQA1791748189026796000/GCodeWorkspaceQA1791748189026796000-edit.txt`。
- 文件 hash：保存前 `5e5a16e41444a173bfc4cff55cf6df9e4e9eb2aeb9ef096b6e5de1e4ffb3d16d`，Command+S 后 `d88667713e5777ff5f1fe485812e49468f25f0096b7f8fd18b564ffeaed75bbb`。
- `textedit-qa-ax.txt` 证明精确唯一标题和 `text area 1 of scroll area 1`；AX 写入逐字读回成功。`edited-preview-ax.txt`、`edited-diff-ax.txt` 和 `git-after-edit.txt` 证明回到 GCode 后预览/Diff 与磁盘内容一致。
- `results.json`：`external_textedit_saved=PASS`、`preview_and_diff_refresh=PASS`、`clipboard=NOT TOUCHED`、`normal_exit=true`；`manifest.json`：`source_unchanged=true`、`original_bundle_unchanged=true`、`qa_exit_code=0`。

## 完成判据

- 文档记录源码证据、系统字典证据、可重放脚本和局限。
- 脚本覆盖打开、修改、保存、关闭四个动作，且每个动作都绑定唯一 QA 文件。
- 明确区分“静态/可编译探针”和“实际原生运行证据”；不把前者标为通过。
- AppleScript `save ... in ...` 只保留为静态术语参考；正式运行通过证据只采用已成功的 AX 修改/读回、AXRaise/`AXFocusedWindow`、Command+S、磁盘 hash 和精确 AXCloseButton 链路。
