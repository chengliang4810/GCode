# Composer refinement 行为审阅目标

日期：2026-10-12（Asia/Shanghai）

本记录只覆盖本轮输入区和紧邻上下文层级的行为审阅，不修改生产代码、测试代码或其他代理的文件。审阅对象为 `internal/desktop/conversation.go`、`internal/desktop/composer_controls_test.go`，以及会影响窄窗右侧资源栏的 `internal/desktop/right_panel.go`、`internal/desktop/diff.go`、`internal/desktop/app_test.go` 和品牌/新会话按钮所在的 `internal/desktop/layout.go`。

## 审阅判定

当前源码没有发现会阻止本轮正式包的确定性生产缺陷。`SelectBase`、`SegmentedBase`、`Checkbox`、停止和发送动作均使用了 mygo 已有的交互 API；模型服务选择只写入会话的下一任务 `ProviderID`，不会改写已经捕获的 `ActiveRun`；纠正、队列、权限、停止和发送的生产调用链保持分离。

此前发现的重复 provider 显示标签问题已修复：选择器现在以 provider ID 作为 `SelectBase` 的选中值和 `parts.Item` 值，trigger 和弹层仍显示人类可读的 label。新增的 `TestComposerSelectsProviderByIDWithDuplicateDisplayLabels` 用同名、同模型的两个 provider 选择第二项，并断言服务收到 `provider-local`，因此同标签不再吞掉第二个选项。

当前结论仍不把候选用户视觉认可、物理键盘/IME、跨平台行为或真实 provider API 写成已完成验收。最新 visual-v3/confirmed 已提供 9 张原生截图、AX 读回和交互控制记录；其中 AX 记录表明鼠标 AXPress 打开弹层后按 Escape，焦点实际回到“任务输入”TextArea，不能声称焦点返回模型选择 trigger。

## 已确认的实现边界

- 主输入区的模型选择器用 `ui.SelectBase`。`providerChoice.id` 作为选择值，`providerChoice.label` 只负责 trigger 和 Item 的可读显示；方向键、Home/End、Enter、Space、Escape 和点击由 mygo 处理。
- 下方发送方式用 `ui.SegmentedBase`，保持“纠正/队列”的 radio group 语义；临时选择保存在 `sendMode`，并通过 `sendModeDirty` 与全局默认值分离。
- 完全访问仍是当前会话的 Checkbox；分支移动到辅助行并提供完整值 Tooltip。标题表头保留实际 cwd，避免重复显示分支。
- 运行中保留独立“停止”按钮；发送按钮按 `strings.TrimSpace(draft) == ""` 禁用，业务层 `send` 仍再次拒绝空文本。
- 1024 DIP 窄窗默认收起资源面板；显式打开时中心宽度按右侧面板最小宽度限制，右侧 `文件`、`Diff`、`终端` 使用 mygo Tabs。
- 侧栏在 1100 DIP 以下使用带 `GCode` 可访问标签的 G 图标，新会话仍调用同一个 `newSession` 入口，只改变按钮外观。

## 最新证据

- `output/acceptance/current-package-regression/visual-refinement-confirmed/package-manifest.json` 记录的正式包 source fingerprint 为 `02d2ecc2f9ff000176aa644ba0570ed5643c44a05c545a227cf249b17707f27c`，可执行文件 SHA-256 为 `7bb8174524b12fec71a6b3bf5c653319f017394c7050b9f467e1091ebb5fd885`，代码签名 SHA-256 为 `5e9391a92029c0eb3b3b8d3ae0007cbe376d1021c8a0d05e37e5d955e39da1d7`。
- 同一 acceptance 目录记录 `go test ./...`、desktop race、`go vet ./...`、`make fmt-check`、`make package` 和 `codesign --verify --deep --strict` 全部 exit 0；本次只读审阅没有重新执行这些命令。
- `output/acceptance/visual-v3/confirmed/` 提供 `dark-1440x900`、`dark-1280x800`、`dark-1024x700`、`light-1440x900`、`light-1024x700`、Composer 输入、模型弹层以及两张设置页共 9 张截图。其 build manifest 声明 capture 后 source unchanged，native QA exit 0；该 visual QA 可执行文件 SHA-256 为 `991ac75c55468bdbccd2cb94ed0f7704b5e19e67ff564605b16cc63a6c4681e0`，与正式包可执行文件 hash 分开记录。
- `control-checks.json` 记录模型弹层实际打开、Escape 关闭、焦点读回为 `AXTextArea:任务输入`，空 draft 的发送按钮 disabled、填写内容后 enabled，以及中文/emoji 的 AX 输入读回通过；`physical_input_ime` 明确为 `NOT_TESTED`。

## 后续证据边界

用户视觉认可和物理键盘/IME 仍待补充。原生截图和 AX 记录可以支持当前视图、弹层关闭、输入读回及发送按钮状态的原生证据，但不能替代物理输入或证明焦点回到了模型选择 trigger。正式包与 native visual QA 使用的可执行文件 hash 也必须分开标记，不能互相冒充。
