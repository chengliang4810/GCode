# macOS TextEdit 外部编辑器探针

日期：2026-10-12（Asia/Shanghai）

状态：静态证据和可重放脚本已完成；root 正式包隔离驱动已完成唯一 TextEdit 文档的原生编辑、保存、关闭与 GCode 回读。

## 结论

GCode 当前 macOS 默认编辑器路径是 `open -a TextEdit <absolute-path>`。应用层会先把相对路径解析到当前 workspace 并拒绝目录；默认调用是异步的。TextEdit 的系统 bundle 提供 AppleScript 文档/文本/保存术语，但 `save ... in ...` 只有静态字典和编译证据：round-2 实际调用长时间等待，root 终止了 `osascript`，底层原因未确定。正式包最终通过的链路是精确窗口 AX `text area` 设置/读回，`AXRaise` 后等待 process 的 `AXFocusedWindow` 名称匹配，发送 Command+S，等待磁盘内容 hash 一致，再点击唯一窗口的 `AXCloseButton`。

本记录同时登记 root 的正式包 S3 结果。本文作者没有执行窗口操作；实际窗口证据来自 `output/acceptance/native-workspace/final/`，且没有读取用户既有文档内容、没有操作系统剪贴板。

## root 正式包实际证据

驱动：`docs/verification/scripts/native-workspace-qa.py`；命令为：

```sh
python3 docs/verification/scripts/native-workspace-qa.py --output output/acceptance/native-workspace/final
```

这次驱动只在隔离正式包副本和唯一 QA Git 项目中操作。结果来自 `output/acceptance/native-workspace/final/results.json` 与 `manifest.json`：

- 隔离身份为 `GCodeWorkspaceQA1791748189026796000`；GCode PID `24120`；TextEdit PID `20990`。
- 唯一文档为 `output/acceptance/native-workspace/final/project-GCodeWorkspaceQA1791748189026796000/GCodeWorkspaceQA1791748189026796000-edit.txt`，窗口标题精确匹配 `GCodeWorkspaceQA1791748189026796000-edit.txt`。
- `textedit-qa-ax.txt` 记录该窗口的 `text area 1 of scroll area 1`。驱动设置 AX value 后逐字读回 `QA edited by TextEdit` 和中文保存行，未使用前台文档或 `window 1` 选择器操作 TextEdit。
- `AXRaise` 后，驱动轮询 **process** 的 `name of (value of attribute "AXFocusedWindow")`，匹配唯一 QA 标题后才发送 `keystroke "s" using command down`。窗口的 `AXFocused` 属性在 round-3 已证明不适合这次焦点判断，最终路径没有使用它。
- 磁盘内容从 `before_sha256=5e5a16e41444a173bfc4cff55cf6df9e4e9eb2aeb9ef096b6e5de1e4ffb3d16d` 变为 `saved_sha256=d88667713e5777ff5f1fe485812e49468f25f0096b7f8fd18b564ffeaed75bbb`；逐字内容断言通过。
- 关闭使用唯一窗口中 `subrole is "AXCloseButton"` 的按钮，随后轮询该标题窗口不存在；没有退出 TextEdit。回到 GCode 后，`edited-preview-ax.txt` 和 `edited-diff-ax.txt` 均显示保存后的内容，`git-after-edit.txt` 与真实 Diff 一致。
- `results.json` 标记 `external_textedit_saved=PASS`、`preview_and_diff_refresh=PASS`、`clipboard=NOT TOUCHED`、`normal_exit=true`；`manifest.json` 标记 `source_unchanged=true`、`original_bundle_unchanged=true`、`qa_exit_code=0`。

这组证据支持当前 Intel 正式包的唯一 QA 文档打开→AX 修改/读回→Command+S 保存→磁盘 hash→精确 AXCloseButton→GCode 预览/Diff 刷新的 S3 结论。它不证明 AppleScript `save ... in ...` 运行成功。

## 当前源码证据

### GCode 外部编辑器命令

- `internal/workspace/editor.go:13-23` 按 `EditorOptions.Command`、`GCODE_EDITOR`、`VISUAL`、`EDITOR` 顺序选择显式编辑器。
- `internal/workspace/editor.go:24-31` 在 macOS 默认返回：

  ```text
  command: open
  args:    -a TextEdit <path>
  ```

- `internal/workspace/workspace.go:352-365` 先检查 context、解析 workspace 内路径、`os.Stat` 目标，并拒绝目录；这意味着 QA 必须先创建一个真实普通文件。
- `internal/workspace/workspace.go:367-383` 启动编辑器；只有调用方明确设置 `EditorOptions.Wait` 才等待子进程。默认设置页/工作台打开编辑器时不应把 TextEdit 退出当作打开成功条件。

### TextEdit 系统字典

本机读取到的文件如下；没有使用 `sdef`，也没有安装或请求 Xcode：

```text
/System/Applications/TextEdit.app/Contents/Info.plist
/System/Applications/TextEdit.app/Contents/Resources/TextEdit.scriptSuite
/System/Applications/TextEdit.app/Contents/Resources/TextEdit.scriptTerminology
```

关键内容：

- `Info.plist`：`CFBundleIdentifier=com.apple.TextEdit`、`NSAppleScriptEnabled=1`。
- `TextEdit.scriptSuite`：`Document` 继承 `NSCoreSuite.NSDocument`，默认子容器是 `textStorage`，并把 `textStorage` 编码为 `ctxt`。
- `TextEdit.scriptTerminology`：`Document` 的 `textStorage` 对外名称是 `text`。

因此以下 AppleScript 语法已用 `osacompile` 编译成功（仅编译，没有执行）。它只证明应用字典术语可解析，不证明 `save ... in ...` 能在当前 TextEdit 实例中完成运行：

```applescript
set qaPath to "/tmp/GCode-native-editor-qa-UNIQUE.txt"
set qaFile to POSIX file qaPath

tell application "TextEdit"
    set qaDoc to open qaFile
    set openedPath to POSIX path of (path of qaDoc)
    if openedPath is not qaPath then error "unexpected document path"
    set text of qaDoc to "GCODE_EDITOR_QA_MARKER"
    save qaDoc in qaFile
    close qaDoc saving no
end tell
```

编译命令使用 `/usr/bin/osacompile`，输出为 `AppleScript compiled`。编译证据不等于运行证据；脚本中的固定路径仅为语法夹具，正式 QA 必须注入当轮唯一绝对路径和 marker。

正式包 round-2 曾运行过 `save ... in ...` 变体；该 `osascript` 长时间等待后由 root 终止，底层原因没有证据判定。因此本片段保留为静态字典参考，不再作为推荐或运行通过方法。

## 推荐的安全重放顺序

root 的正式包驱动应将以下步骤串行执行；所有等待都应有超时，异常时只清理本轮唯一文件和本轮 TextEdit 目标窗口。

1. **创建唯一文件。** 在隔离 QA 临时目录中生成类似 `GCode-editor-qa-<nonce>.txt` 的普通文件，写入 `before=<nonce>`。不要使用项目中的现有文件，不要复用固定 `/tmp` 路径。
2. **通过产品打开。** 走 GCode 的 `OpenEditor`，记录实际传入的绝对路径。若驱动直接执行默认命令，必须等价于 `open -a TextEdit <absolute-path>`。
3. **按精确标题定位 AX 窗口。** 只在 `System Events` 中访问 `process "TextEdit"` 的 `window <qaTitle>`；不要使用 `window 1`、frontmost document 或所有窗口枚举。标题不存在时超时失败。
4. **AX 修改。** 对目标窗口的 `text area 1 of scroll area 1` 设置 `value` 为 `after=<nonce>`，然后用 AX 读回同一控件并断言逐字一致。若目标系统的 role/index 不同，应先针对唯一窗口查找 text area；不能降级为前台窗口。
5. **AX 聚焦并保存。** 对唯一窗口执行 `set frontmost to true` 和 `perform action "AXRaise"`，轮询 process 的 `name of (value of attribute "AXFocusedWindow")`，必须等于唯一 QA 文件名。确认匹配后执行 `keystroke "s" using command down`，轮询磁盘内容直至等于 after marker，再记录 SHA-256/字节数。不要把窗口的 `AXFocused` 属性当作焦点证明，也不要把 AppleScript `save ... in ...` 当作最终保存路径。
6. **AX 关闭。** 只点击精确窗口中 `subrole is "AXCloseButton"` 的第一个按钮。等待该唯一标题窗口不再存在，再读取一次 QA 文件确认内容仍为 after marker。不要执行 `quit TextEdit`，因为 TextEdit 可能承载用户文档。
7. **收尾。** 记录 GCode PID、TextEdit PID（若可得）、唯一文件路径、保存前后 hash、窗口出现/消失结果和异常状态。确认没有把 marker 写入剪贴板；删除 QA 临时文件只能由驱动在全部证据落盘后执行。

## 可直接嵌入驱动的 AppleScript 片段

下列片段中的 `QA_PATH`, `QA_TITLE`, `AFTER_MARKER` 必须由驱动通过安全的 AppleScript 字符串转义注入。不要通过 shell 拼接未转义用户输入。

### 精确窗口的 AX 修改与读回（正式包已通过）

```applescript
tell application "System Events"
    tell process "TextEdit"
        if not (exists window QA_TITLE) then error "QA TextEdit window not found"
        tell window QA_TITLE
            if not (exists text area 1 of scroll area 1) then error "QA TextEdit text area not found"
            set value of text area 1 of scroll area 1 to AFTER_MARKER
            if value of text area 1 of scroll area 1 is not AFTER_MARKER then error "AX text readback mismatch"
        end tell
    end tell
end tell
```

### AXRaise、process AXFocusedWindow 与 Command+S（正式包已通过）

```applescript
tell application "System Events"
    tell process "TextEdit"
        if not (exists window QA_TITLE) then error "QA TextEdit window not found"
        set frontmost to true
        perform action "AXRaise" of window QA_TITLE
    end tell
end tell
```

随后由驱动轮询下面的只读断言，超时即失败；这里使用 process 的 `AXFocusedWindow` 属性，不能改成窗口的 `AXFocused` 属性：

```applescript
tell application "System Events"
    tell process "TextEdit"
        if name of (value of attribute "AXFocusedWindow") is not QA_TITLE then error "unexpected focused editor window"
        keystroke "s" using command down
    end tell
end tell
```

Command+S 后必须以磁盘内容逐字匹配 `AFTER_MARKER` 并记录 hash 作为保存证据；快捷键发送本身不构成保存成功。

### AppleScript `save ... in ...`（仅静态术语参考，未通过运行）

```applescript
set qaPath to QA_PATH
set qaFile to POSIX file qaPath

tell application "TextEdit"
    set qaDocs to every document whose path is qaFile
    if count of qaDocs is not 1 then error "QA document count mismatch"
    set qaDoc to item 1 of qaDocs
    set openedPath to POSIX path of (path of qaDoc)
    if openedPath is not qaPath then error "unexpected document path"
    save qaDoc in qaFile
end tell
```

这段脚本假定 GCode 已经通过 `open -a TextEdit <qaPath>` 打开目标；它按精确路径绑定唯一文档，不会再次 `open` 或触碰 `front document`。它只在 `osacompile` 阶段通过；round-2 运行时长时间等待，root 终止了 `osascript`，底层原因未确定，不能作为正式保存方法。正式驱动生成脚本时，应沿用 `native-product-qa.py:172` 的 `json.dumps(value, ensure_ascii=False)` 思路为路径和 marker 做 AppleScript 字符串转义，禁止把未转义内容拼入 shell。

### 精确窗口的 AXCloseButton（正式包已通过）

```applescript
tell application "System Events"
    tell process "TextEdit"
        if not (exists window QA_TITLE) then error "QA TextEdit window not found"
        click (first button of window QA_TITLE whose subrole is "AXCloseButton")
    end tell
end tell
```

随后轮询 `exists window QA_TITLE` 直到为 `false`，再确认唯一 QA 文件内容和 hash 没有变化。关闭动作只针对该窗口，不退出 TextEdit。

## 局限与风险

- 本机 `sdef` 不可用且未重试；系统 bundle 已提供 `TextEdit.scriptSuite`/`TextEdit.scriptTerminology`，足以支持上述术语，但不同 macOS 版本的 TextEdit AX hierarchy、窗口标题本地化和保存提示仍需真实 QA 记录。
- `text area 1` 是当前已有原生 AX 脚本使用的角色/index 约定，但 TextEdit 的版本或富文本模式可能改变层级；驱动必须在唯一窗口内检查 role/标题，不能把 `window 1` 作为回退。
- TextEdit 可能保留已有用户窗口。脚本禁止 `activate`、`front document`、`quit`、所有窗口遍历和剪贴板读写；关闭 QA 文档后只需验证唯一标题消失，不能要求 TextEdit 进程退出。
- `save qaDoc in qaFile` 只完成静态编译，round-2 实际调用曾长时间等待并被 root 终止，底层原因未知；不得把它当作可运行保存路径。当前正式包保存证据只采用 AXRaise、process `AXFocusedWindow` 匹配、Command+S 和磁盘 hash。
- 通过 AX 设置 `value` 是程序化输入证据，不能替代物理键盘、中文输入法组合、候选框、Enter/Escape、滚动或焦点切换验收。

## 本轮验证记录

- 读取：`internal/workspace/editor.go`、`internal/workspace/workspace.go`、`docs/verification/scripts/native-product-qa.py`。
- 读取：`/System/Applications/TextEdit.app/Contents/Info.plist`、`TextEdit.scriptSuite`、`TextEdit.scriptTerminology`。
- `rg --files /System/Applications/TextEdit.app` 已确认上述 bundle 文件存在。
- `osacompile` 对最小打开/路径断言/文本修改/保存/关闭脚本返回 `AppleScript compiled`；本子目标没有执行 `osascript`，没有启动或操作 TextEdit，没有读取用户文档内容和剪贴板。
- root 正式包实际运行证据来自 `native-workspace-qa.py` 的 final 目录，独立记录了精确标题、TextEdit PID、AX text area 读回、AXRaise/`AXFocusedWindow`、Command+S、磁盘 hash、AXCloseButton 和回到 GCode 后的预览/Diff。
- 产品源码未修改；本目标只更新本文件和对应 goal 文档。
