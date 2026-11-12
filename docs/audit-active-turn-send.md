# 运行中发送：队列与纠正

2026-11-01 实现记录。参考为只读的 Zeron v0.2.128，固定提交
`ab189159f10cf8e63c0502be75a59bbc487e0eaf`，副本在
`output/reference-sources/zeron`。本文件记录源码对照、实现与可执行检查。

2026-11-10 更新：每条用户输入（含引导）现在各自开始独立任务，并保留发送时选择的模型
和思考强度。引导在当前模型请求及工具批次完成后的步骤边界交接；普通排队消息在当前回答
结束后交接。`handoffUserInputLocked` 选择下一输入，运行收尾通过 `run.nextInput` 开始
下一任务；只有显式 Send now 会取消当前执行。当前规则与验证见 `model-adapters.md`、
`backend-v1.md` 及 `output/user-input-task-boundaries-final/`。

2026-11-12 更新：交接语义不变，改变的是引导输入的呈现。引导消息发送后立刻作为
对话流里的用户气泡出现，不再显示为输入框上方的队列行；气泡下标注“正在纠正当前回复”
并提供取消入口。输入框上方的队列区只保留真正等待下一轮的普通排队消息。实现见
`internal/desktop/local_send.go` 的 `steeringMessage` / `localSendStatus`，
`queue_rows.go` 与 `transcript_side.go` 跳过承载引导的队列行。

以下内容保留为 2026-11-01 的历史实现及验收记录。

## 参考的两个时期

| Source | Connected behavior |
| --- | --- |
| `settings.rs`（提交 `b77826c8` 之前） | `ActiveTurnSendBehavior`：`Steer`（默认）——运行中发送立刻纠正当前回合；`Queue`——留在队列里可编辑，直到回合结束或用户对某一行选 Steer / Send now。旧设置文件里的该键被当作过时字段忽略（`settings.rs:3267` 的测试）。 |
| `composer.rs`（提交 `b77826c8` 之前） | 发送按钮三态：`Send` / `Steer`（"Send (steers the current run)"）/ `Stop`（只有空输入时才是 Stop）；忙时 `SendButtonMode::Steer` 走 steer，`Queue` 走队列。 |
| `queue.rs`（提交 `b77826c8` 之前） | 队列行主操作三态：文本行 **Steer**（"keep current work running"）、带附件 **Send now**（"interrupt"）、模型不支持回合内注入时 **Send next**；另有编辑、删除与拖动排序。 |
| 提交 `b77826c8`（2026-09-09，"Always queue active-turn messages and remove steering controls (#284)"） | **现行参考删除了这套设置与纠正控件**：忙时发送一律排队，队列行不再有 Steer / Send now，`activeTurnSendBehavior` 只作为过时键被容忍。 |

本轮按需求保留旧 RCode 的形态：设置可选 queue / steer，**默认 steer**。

## GCode 的实现

| Reference | GCode |
| --- | --- |
| `UiSettings.active_turn_send_behavior` | `contracts.GeneralSettings.ActiveTurnSendBehavior`（JSON `activeTurnSendBehavior`），值 `steer`/`queue`，空值读作 steer；默认写入 `contracts.DefaultSettings` 与 mock 种子。 |
| 忙时 steer 立刻并入当前回合 | `App.send()` 在会话 `Working`/`AwaitingInput` 且设置为 steer 时给 `SendRequest.Delivery` 填 `"steer"`；engine 的 `drainPendingLocked`（`engine/queue.go:11`）在 `idle=false`（回合进行中）只放行 steer 行，于是它进入当前回合的下一步（`engine/run.go:635` 的 `Pending(false)`）。 |
| 忙时排队 | Delivery 为空，消息进入 `state.Queue`，回合结束后按 `idle=true` 排空。 |
| 引导的呈现 | 引导的队列行由 UI 投影为对话流中的用户气泡（`steeringMessage`），气泡下显示“正在纠正当前回复”与取消按钮（`queue.remove`）；队列区不渲染引导行，交接后由核心自己的消息接管，不重复显示。 |
| 发送按钮三态 | `composerSendMode`（`composer_surface.go`）：空输入且运行中为 **Stop**；有内容且运行中按设置显示 **Steer the running response** 或 **Queue for the next turn**；否则 **Send message**。侧边会话的输入框同规则。 |
| 忙时提示 | 运行中的提示语区分两种：steer 为 "Steering the running response"，queue 为 "Queued — will send automatically"（`working_trailer.go`）。 |
| 队列行的 Steer / 编辑 / 删除 | 行上的「Send as steering」(`queue.steer`)、「Edit queued message」（取回输入框并出队）、「Remove queued message」(`queue.remove`)。 |
| 队列行的 **Send now** | 行上的「Send now (interrupt)」(`queue.send`)：行先出队，`run.cancel()` 只结束当前回合（正在进行的模型请求或工具调用），运行收尾后再用该行开启下一回合；它不是回退整段对话，也不是 `message.stop`。`engine/queue.go` 的 `sendQueuedNowLocked` 与 `run.go` 收尾里的 `run.afterStop` 一起完成交接，启动失败时行回到队列。 |
| 自然队列一次一条 | `drainPendingLocked(sessionID, true)` 每轮只放行第一条，其余保留原顺序；`run.go` 回合收尾的自动交接同样只取第一条。steer 行仍由 `Pending(false)` 在下一步前注入。 |
| 队列拖动排序 | 整行可拖（`controlReorderRow` + `controlReorderTarget`），落在另一行的上半/下半决定前/后；派发 `queue.move`，`engine.moveQueuedLocked` 只允许同一会话内移动并持久化。 |

未实现（与现行参考一致地缺失）：**Send next**、空输入时 Cmd/Ctrl+Enter
激活最近一条队列。队列行操作在 mock/fixture 里不显示（与 `queue.steer`
相同，`Runtime` 为空的快照不算真实后端）；真实引擎路径显示全部操作。

## 检查

- `go test ./internal/desktop -run 'TestActiveTurnSendFollowsTheSetting'`：steer
  默认 → 派发的 `message.send` 带 `Delivery="steer"`；选 queue → 不带；空闲会话
  → 两种设置下都不带。
- `go test ./internal/desktop -run 'TestComposerButtonReflectsTheActiveTurnSetting'`：
  运行中空输入为 Stop，有内容为 Steer，改设置为 queue 后为 Queue。
- `go test ./internal/desktop -run 'TestWorkingTrailerNamesTheSteer'`：提示语区分
  steer 与 queue。
- `go test ./internal/desktop -run 'TestSteering'`：引导的队列行投影为对话流中的
  用户气泡，位置在普通队列行之上；队列区只渲染普通排队行；气泡的取消按钮派发
  `queue.remove`；交接后消息只出现一次。
- `go test ./internal/i18n`：界面文案门禁无缺失条目。
- `go test ./internal/engine -run 'TestIdleDrainHandsOverOneQueuedMessage'`：
  `idle=true` 每次只交出一条并保留其余顺序；`idle=false` 只放行 steer 行。
- `go test ./internal/engine -run 'TestQueueMoveReordersWithinTheSession'`：
  越前/越后移动的顺序，跨会话移动被拒绝且顺序不变。
- `go test ./internal/engine -run 'TestSendQueuedNowStopsTheRunAndOpensTheNextTurn'`：
  运行中入队两条后对第二条 Send now，队列立即只剩第一条；放行首个请求后
  模型请求顺序为 first → queued two → queued one，首回合状态为 interrupted，
  队列最终排空。
- `go test ./internal/desktop -run TestQueueRow`：行上出现 Send now 并派发
  `queue.send`；整行拖到另一行下半部派发 `queue.move`（after=true）。

## 未验收

- steer 的实际注入时机只在 engine 单元层面验证过（`drainPendingLocked` 的
  `idle=false` 分支）；原生窗口里一次真实纠正的端到端行为尚未录制证据。
- Send now 的取消只由 engine 测试覆盖（首个请求被取消、下一回合按行启动）；
  原生窗口中真实模型请求与工具进程被打断的时序尚未录制。
- 队列拖动在 `ui.Tester` 里验证了落点与命令，但真实窗口的指针拖动、
  自动滚动与跨行手感尚未验收。
- 设置项在原生设置页的渲染没有截图证据。
- 引导气泡的呈现只由 `ui.Tester` 组件测试覆盖（投影、位置与取消命令）；原生窗口中
  一次真实纠正的气泡、状态行与取消按钮尚未截图验收。
