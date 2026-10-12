# /goal：收敛工作台输入区与上下文层级

状态：实现、正式包回归和限定原生复核完成；用户视觉认可 pending。

责任：root。拥有 `internal/desktop/conversation.go`、`internal/desktop/composer_controls_test.go` 和本记录。其他代理分别负责 Diff 表头和原生持续流式驱动，不回滚其修改。

## 目标

以用户 T3 暗黑截图为最低视觉标准，解决已登记的 composer 控件同级拥挤、模型 Select 边框过重、表头路径/分支重复问题。保留纠正、FIFO 队列、模型选择、单会话权限、停止与发送的真实语义，继续使用 Go + mygo。

## 当前实现

- 主输入面仅保留输入、轻量模型选择器、独立停止和发送；发送方式和权限放在下方辅助行。
- 选择器使用已有 `ui.SelectBase`，保留 popup、方向键/Enter/Escape、选择事件与可访问标签；采用无静止边框的触发器和低反差原生弹层。选项直接使用唯一 provider ID，label 只显示名称，避免同名同模型服务无法选择。
- 辅助行使用已有 `ui.SegmentedBase`，保持“纠正/队列” radio group 和默认偏好/临时覆盖语义；权限仍为原生 Checkbox。
- 表头保留标题和实际 cwd，分支移至输入区辅助行，避免表头的重复工作区/分支串。
- 空白输入的发送按钮禁用，运行中的停止动作独立保留。

## 验证

`TestComposerKeyboardModelChoiceKeepsRunningConfigAndSeparateActions` 通过真实 Tester 操作验证：方向键/Enter 选择下一任务模型，传入服务的 session/provider ID 正确，当前运行配置不变；切换队列后输入中文/emoji 并发送，模式与正文正确，未同时触发停止；显式停止只作用于当前会话。

focused 命令 `go test ./internal/desktop -run TestComposerKeyboardModelChoice -count=1 -timeout=30s` 已 exit 0。最终完整回归见 `visual-refinement-confirmed`，六项均 exit 0；原生弹层/深浅主题/窄窗九张截图及 AX 见 [visual-v3.md](../verification/visual-v3.md)。同名 provider Tester 测试先复现失败、修复后通过。原生和 Tester 都不能替代用户视觉认可。
