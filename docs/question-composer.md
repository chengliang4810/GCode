# 底部提问面板

2026-11-04：按旧版 RCode `e8399dc` 的交互结构恢复，参考
`crates/ui/src/composer.rs` 的 Wizard 和 `transcript.rs` 的 InputChip。
使用当前 Go/MyGo 与共享控件实现；不改变其他页面的视觉布局。

- 核心一次发布一组 1–4 个问题；完整交互面板临时替换底部普通输入框。
- 单选保留 220ms 选中反馈后自动前进；多选和自定义回答手动前进或提交。
- 多题显示页码，支持返回修改；最后一题提交时统一校验整组答案。
- 非空自定义文字优先于选项。数字键在答案输入框以外选择选项；
  Enter 前进或提交，Shift+Enter 换行，输入法组合期间不提交。
- 普通消息草稿和附件不参与答案编辑。切换会话保留各自题目页与答案；
  提交失败保留内容并允许重试，成功后恢复普通输入框及草稿。
- 主对话和可编辑侧聊使用同一个面板实现。子 Agent 的只读侧栏不开放编辑。
- 对话正文只保留简短、无交互的提问记录。已结束或取消的请求不阻塞继续发消息。
- `Question.pages`、`Question.answers`、`Question.resolved` 和
  `Command.QuestionAnswers` 为当前契约；本机调用传递类型化 Go 值。
  模型工具结果仍使用 `questions` 与按题目文本索引的 `answers`。

共享入口是 `settings_controls.go` 的 `controlQuestionOption`、
`controlTextArea`、`textButton` 和 `primaryButton`。

## 验证

组件回归覆盖底部位置、深浅主题、900×600/1320×880、完整题目只出现一次、
分页返回、多选、自定义覆盖、失败重试、会话隔离、普通草稿恢复、数字键与输入法。
核心回归覆盖整组发布、拒绝部分答案、只提交一次、模型继续执行、快照和命令的
嵌套数据所有权，以及宿主重开后取消失效等待。

`TestDesktopLocalQuestionComposer` 使用独立原生窗口、数据目录和真实 Go 核心，
仅模型端是本地 HTTP 夹具。英文浅色 1320×880、中文深色 900×600 均验证：
底部面板替换、分页返回修改、多选和自定义回答、落盘、模型继续执行、草稿恢复、
历史只保留记录且不重复计算工具调用。2026-11-04 原生测试通过，证据位于
`output/question-composer-native/20261104-030546-3337031612/evidence.json`，
同目录保留两种主题的完整截图、无障碍树和日志。
该证据不覆盖真实外部供应商、远程设备或操作系统输入法。

## 同步修正

原去重实现和维护文档写 `ask_user`，核心实际工具名为 `AskUserQuestion`；
已按真实工具名统一，避免提问又成为普通工具活动。
原有测试只验证完整正文卡片的去重，不能证明旧版底部提问交互正确，现已增加
位置和生命周期回归。`backend-v1.md` 的逐问等待描述已更新为整组提交。

当前全量回归另有未涉及区域的失败：设置页测试仍寻找 `Apply` 和软件更新按钮，
综合原生用例在进入提问前失败于思考预览断言；根目录的 `go test ./...` 还会扫描
历史 `output/desktop-e2e` 的中文路径 Go 夹具，导致非法导入路径错误。
这些不计作本次提问功能通过的证据，也不在本次修改中扩展修复。

排除上述两个设置页测试并限定源码目录后，
`go test ./cmd/... ./internal/... ./scripts/... -skip 'Test(RealCoreSettingsAndTypedMessageProjection|SoftwareUpdatesUseDesktopCallback)$' -p 2 -timeout 3m`
通过。`go vet ./cmd/... ./internal/... ./scripts/...`、
`go tool mygo vet ./internal`、共享控件边界回归及框架固定版本校验通过。
根目录 `go tool mygo vet .` 内部同样使用 `./...`，会被上述历史夹具路径阻断；
因此单独对 `internal` 执行原生 UI 生命周期校验，不删除历史验收数据。
