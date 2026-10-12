# Composer refinement 行为审阅记录

日期：2026-10-12（Asia/Shanghai）

## 结论

本轮只读复核没有发现阻止正式包的确定性生产缺陷。MyGo API 调用方式正确，队列/纠正、停止/发送、权限和下一任务 provider 之间的职责边界成立；已有 focused Tester、最新正式包回归和 visual-v3 原生记录覆盖了主要行为。

此前发现的重复 provider 显示标签缺陷已修复。`composerProviderSelect` 现在把 provider ID 同时作为 `SelectBase` 的选中值和 `parts.Item` 的值，Item 文本仍使用可读 label；`TestComposerSelectsProviderByIDWithDuplicateDisplayLabels` 在两个 provider 的 Name 和 Model 都相同的 fixture 中选择第二项，并断言服务收到 `provider-local`。因此 display label 相同不再导致第二个 provider 无法选择。

最新正式包 `visual-refinement-confirmed` 的 source fingerprint、可执行文件 hash 和代码 hash 已记录，普通测试、desktop race、vet、fmt、package 和 codesign 结果均为 exit 0。`visual-v3/confirmed` 还提供 9 张原生截图、AX 树和交互控制记录。原生记录确认弹层实际打开、Escape 关闭、中文/emoji AX 读回以及发送按钮 disabled/enabled；鼠标 AXPress 后按 Escape 的焦点实际读回为“任务输入”TextArea，不能把它描述成焦点回到模型选择 trigger。候选用户视觉认可和物理键盘/IME 仍未完成。

## 生产事实

### API、选项值与显示标签

Composer 在 `internal/desktop/conversation.go:441-518` 构建 TextArea、主操作行和辅助行。模型选择器在 `:520-547` 使用 `ui.SelectBase`：

- `selected` 保存 provider ID。
- trigger 用 `providerOptionLabel(choices, *selected)` 取得人类可读名称，并通过 `Label("下一任务模型")` 暴露访问名称。
- `parts.Item(choice.id)` 直接以唯一 provider ID 作为选项值，Item 内部文本显示 `choice.label`。
- `providerOptionLabel` 只按 ID 查找显示名称，不再存在以 label 反查 provider ID 的选择路径。

mygo 的 `SelectBase` 支持 trigger 焦点下的方向键打开和移动、高亮项的 Home/End、Enter/Space 选择，以及 Escape 或外部点击关闭。当前调用没有绕过这些弹层机制。`providerChoices` 仍可在可读 label 后追加模型后缀，但选择身份由 ID 保证唯一。

发送方式在 `conversation.go:477-497` 使用 `ui.SegmentedBase(c.Key("send-mode"), &index, 2)`。Track 标记为“发送方式”，两个 Segment 分别设为“纠正”和“队列”；mygo 将其视为一个 radio group，`Changed` 再映射为 `Correction` 或 `Queue`。`ui.Checkbox` 位于同一辅助行，仍以当前 session 的 `FullAccess` 为值绑定。

### 下一任务 provider 与 ActiveRun

选择器发生变化时，`conversation.go:456-462` 只把 `provider` ID 交给 `App.setProvider`。`internal/desktop/app.go:779-787` 再把该 ID 交给 Service。

Controller 的 `SetProvider` 在 `internal/controller/controller.go:699-727` 只更新 `session.model.ProviderID` 并持久化；已经运行的配置在 `startRunLocked` 的 `:969-990` 创建 active run 时由 `runConfigFor` 捕获为 `run.config` 和 `session.model.ActiveRun`。`SetProvider` 不触碰 `session.active.config` 或 `ActiveRun`，所以模型选择只影响下一次启动的任务。

`TestComposerKeyboardModelChoiceKeepsRunningConfigAndSeparateActions` 在 `internal/desktop/composer_controls_test.go:28-73` 用真实 `ui.Tester` 打开弹层、End/Enter 选择“本地评审”，检查服务收到原 session ID 和 `provider-local`，再检查 `ActiveRun` 仍为 `captured-running-model/provider-openai`。

`TestComposerSelectsProviderByIDWithDuplicateDisplayLabels` 在 `composer_controls_test.go:111-135` 将所有 fixture provider 设置为同一 Name 和 Model，打开选择器后用 End/Enter 选择第二项，断言 service probe 收到 `provider-local`。这个回归测试直接覆盖了此前 P2 的触发条件。

### Queue、Correction、Stop、Send 和权限

`conversation.go:471-474` 只有非空草稿才让发送按钮可用；`internal/desktop/app.go:650-675` 再次 trim 并拒绝空文本，成功后只有在当前 draft 仍等于发送文本时才清空输入。Controller `Send` 在 `internal/controller/controller.go:518-583` 保留两种模式：运行中 Correction 插入纠正队列并取消当前 run，Queue 追加到 FIFO 队列；发送服务调用没有隐式调用 Stop。

运行状态为 `Running` 或 `Cancelling` 时，`conversation.go:465-469` 单独创建“停止”按钮；`app.go:678-686` 再向当前 session 调用 Service.Stop。focused 测试先点击“队列”并输入中文/emoji，断言发送模式和正文正确且 stop channel 为空，再单独点击“停止”断言只收到当前 session ID。

完全访问 Checkbox 在 `conversation.go:498-509` 更新当前 session 的下一次 `FullAccess` 并调用 Service.SetFullAccess。与 provider 一样，已经启动的 `ActiveRun` 配置由 Controller 在启动时捕获，不会被该 UI 写入点原地改写；当前测试覆盖了本地 metadata 隔离，但没有单独用 service probe 断言 SetFullAccess 请求及其失败回滚。

### 窄窗、分支、资源 Tabs 和品牌

分支在 `conversation.go:512-515` 使用 `MinWidth(0).SingleLine().Tooltip(branch)`；可用宽度不足时文本可省略，但 Tooltip 仍保留完整 branch。右侧资源栏在 `right_panel.go:23-35` 设有 `rightPanelMinimumWidth`，`ui.Tabs(..., "文件", "Diff", "终端")` 设为可缩小并带“资源面板”标签；Diff 头部统计在 `diff.go:64-83` 使用可缩小的文件数文本和不可缩小的增删计数。`internal/desktop/app_test.go:394-432` 的 1024×700 Tester 检查资源栏不越界、右边缘填充、停止动作仍可读，并依次确认四个右侧控件文本存在。

`layout.go:83-97` 在窄窗隐藏文字品牌，仅保留带 `Label("GCode")` 的 accentSoft G 图标；新会话按钮仍通过同一 `a.newSession(activeProjectID)` 入口，只使用 neutral `iconButton` 外观。现有 `TestNarrowSidebarKeepsBrandUnambiguous` 检查窄侧栏不会生成 `GC...`，并同时存在 GCode 标签和 G 图标。

## 已修复的重复 provider label 问题

旧路径把 `choice.label` 同时作为 `SelectBase` value，并由 `providerOptionID` 按 label 返回第一个 ID；Name 和 Model 都相同时，第二个 provider 会被吞掉。

当前路径在 `conversation.go:538-542` 使用 `parts.Item(choice.id)`，trigger 通过 `providerOptionLabel` 按 ID 找到 label，Item 文本仍显示 `choice.label`。因此显示文字和选择身份已经分离：两个 provider 即使 label 完全相同，也会产生不同的 Select value。新增 focused test 以同名、同模型 fixture 验证第二项实际调用 `provider-local`，该缺陷不再列为当前风险。

## 测试和正式包证据

### `visual-refinement-confirmed` 正式包

`output/acceptance/current-package-regression/visual-refinement-confirmed/package-manifest.json` 记录：

- source fingerprint：`02d2ecc2f9ff000176aa644ba0570ed5643c44a05c545a227cf249b17707f27c`
- executable SHA-256：`7bb8174524b12fec71a6b3bf5c653319f017394c7050b9f467e1091ebb5fd885`
- code SHA-256：`5e9391a92029c0eb3b3b8d3ae0007cbe376d1021c8a0d05e37e5d955e39da1d7`
- `source_unchanged_after_build: true`
- `test_files_unchanged_after_build: true`

该目录的结果文件记录以下命令全部 exit 0：

- `go test ./... -count=1 -timeout=180s`
- `go test -race ./internal/desktop -count=1 -timeout=180s`
- `go vet ./...`
- `make fmt-check`
- `make package`
- `codesign --verify --deep --strict build/darwin-amd64/GCode.app`

这些是已有 acceptance 记录；本代理本轮没有重新运行测试、构建或签名检查。

### `visual-v3/confirmed` 原生记录

`output/acceptance/visual-v3/confirmed/captures.json` 列出 9 张截图：

- `dark-1440x900-native.png`
- `dark-1280x800-native.png`
- `dark-1024x700-native.png`
- `light-1440x900-native.png`
- `light-1024x700-native.png`
- `composer-input-native.png`
- `model-popup-native.png`
- `settings-dark-native.png`
- `settings-light-native.png`

`build-manifest.json` 声明 native QA exit 0、capture 后 source unchanged，运行环境为 macOS 14.8.7、Go 1.27.2 darwin/amd64；该 visual QA 构建的二进制 SHA-256 为 `991ac75c55468bdbccd2cb94ed0f7704b5e19e67ff564605b16cc63a6c4681e0`。这是原生视觉 QA 二进制，不能与正式包的 `7bb817...` executable hash 混用。

`control-checks.json` 的直接记录为：

- `model_popup_open_ax: true`
- `escape_popup_closed: true`
- `focused_ui_element_after_escape: AXTextArea:任务输入`
- `empty_send_enabled: false`
- `filled_send_enabled: true`
- `ax_input_readback: true`
- `physical_input_ime: NOT_TESTED`

`model-popup-ax.txt` 还读回了“下一任务模型”弹层、`OpenAI API`、`本地评审`、发送方式 radio group、完全访问 checkbox、右侧 `文件`/`Diff`/`终端` tabs 和窄侧栏品牌元素。`native-input.txt` 的中文/emoji 内容为程序化 AX 输入读回，文件本身明确说明它不替代物理键盘/IME 验收。

## 证据边界和未覆盖项

1. 原生 control check 已直接确认空 draft 时发送按钮 disabled、填入内容后 enabled；仍没有 service probe 通过“激活 disabled 按钮”证明 Service.Send 不会被调用，业务层 trim guard 仍是第二道保护。
2. 原生 control check 已确认模型弹层实际打开和 Escape 关闭，AX 焦点回到“任务输入”TextArea；没有证据证明焦点返回模型选择 trigger，也没有单独 focused 断言 SegmentedBase 通过 Tab/方向键切换。
3. 重复 provider Name/Model 已由 `TestComposerSelectsProviderByIDWithDuplicateDisplayLabels` 覆盖；超长 provider 名称和未知 session ProviderID 仍没有专门 fixture。
4. visual-v3 已提供 9 张原生截图、AX 树和 1024 DIP 场景记录，但这些截图来自独立 native visual QA 构建；正式包 hash 已单独记录。候选用户视觉认可仍 pending。
5. `physical_input_ime` 明确为 `NOT_TESTED`。程序化 AX 输入读回不能替代物理键盘或真实 IME。
6. `TextArea` 是多行输入；mygo 的 `Submitted` 语义只针对单行输入，Composer 中 `field.Submitted()`（`conversation.go:452-454`）不是 Enter 发送路径。当前行为依靠可访问的“发送”按钮；若产品要求 Enter/Cmd+Enter 发送，仍需单独定义和验证快捷键。
7. FullAccess 没有独立 service probe 验证请求及失败回滚；真实 provider API 和跨平台窗口行为也不在本轮证据范围内。

本次只读复核没有运行原生窗口，也没有清理或重置共享工作区；没有修改生产代码和测试代码。
