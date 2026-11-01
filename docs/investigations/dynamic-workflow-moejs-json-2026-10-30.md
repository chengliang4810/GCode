**Dynamic Workflow 最新源码与 moejs / JSON 可行性复核**

日期：2026-10-30。目标已明确为 `/workflow` 对应的 Dynamic Workflow；旧 `Workflow` 脚本工具和 Expert 工作流不纳入本次目标。

结论：**Go 业务核心 + moejs 脚本执行 + JSON/JSONL 持久化可作为完整 Dynamic Workflow 的实现路线，不需要默认引入 Node 或 SQLite。** moejs 和 JSON 分别提供运行基础与存储介质，Actor、Schema、分析、调度和重放语义仍需由项目实现。本轮完成更新调查和独立实验，未把该功能或新依赖接入产品。

**已拉取的实际版本**

在 `D:/projects/ZCode` 的干净 `main` 执行 `git pull --ff-only`：

- 旧：`3.16.2 / 3c5b227649371507ddc4eb46ac5133afe6db1cab`，2026-10-20。
- 新：`3.17.1 / a60ba6a7e5a06742270ca605f5d2cc2766afa7e1`，2026-10-30。
- 检查时 HEAD 与 `origin/main` 为 ahead 0 / behind 0，工作区保持干净。
- 整体 diff 为 5,231 个文件；其中 dynamic-workflow 与 dynamic-workflow-runtime 两包合计 112 个变更文件。整体删除量还包含 App 业务单测退役，不能据文件数量推断功能大量消失。

本次没有更新 `D:/projects/moejs`：只读取用户指定的现有干净源码，HEAD `b9fa0469d1df0900d7a7a92fb43994a6e65e9319`，describe `v0.1.0-alpha.11-2-gb9fa046`，提交日期 2026-10-30。模块路径是 `github.com/MoeForge/moejs`，最低 Go 1.25。

**`/workflow` 没有换成别的机制**

命令正文仍要求：加载 dynamic-workflows skill → 先确定子代理拓扑与上下文共享/结果 → 写脚本 → `CreateWorkflow`，并明确禁止替换为旧 `Workflow` 或普通 `Agent` 工具。

证据：[命令正文](D:/projects/ZCode/apps/zcode-cli/packages/bootstrap/src/builtin-workflow-command.ts:32)。本次新增的是 `workflow_cmd` 会话特征记录：[接入位置](D:/projects/ZCode/apps/zcode-cli/packages/bootstrap/src/app/create-app.ts:977)。

用户可见的主要更新：

| 更新 | 当前行为与限制 | 源码 |
| --- | --- | --- |
| 配额后自动续跑 | 有 resetAt 时在重置后一分钟恢复；冷启动补查本会话最近 64 个 run。不是脱离宿主的永久定时服务 | [quota resume](D:/projects/ZCode/apps/zcode-cli/packages/bootstrap/src/app/dynamic-workflow-quota-resume.ts:47) |
| 子 Agent 问题归口 | Actor 不再直接使用 AskUserQuestion，统一 escalate 给主 Agent；工具权限确认仍按会话模式执行 | [Actor 工具](D:/projects/ZCode/apps/zcode-cli/packages/bootstrap/src/app/workflow-actor-tools.ts:35) |
| world.run 输出 | full 从每路 256 KiB 提升至 16 MiB；增加 discard 和 tail 模式 | [命令输出](D:/projects/ZCode/apps/zcode-cli/packages/bootstrap/src/app/workflow-world-read.ts:303) |
| 脚本运行身份 | 新增冻结的 `run.id`、`run.resumedFrom`，便于报告及外部关联 | [Facade](D:/projects/ZCode/apps/zcode-cli/packages/dynamic-workflow/src/facade/dts.ts:309) |
| 未识别 provider 错误 | 同一请求连续十次未识别业务错误后停止，避免无限重试 | [失败策略](D:/projects/ZCode/apps/zcode-cli/packages/adapters/src/model/workflow-model-failure-policy.ts:57) |
| 公平限流扩大 | 普通 Agent 子任务也使用 workflow 进程 governor，同父会话共用公平队列 | [Governor](D:/projects/ZCode/apps/zcode-cli/packages/bootstrap/src/app/workflow-concurrency-governor.ts:443) |
| 关闭与资源释放 | 区分 run terminal 和资源释放，关闭等待 Actor、工具、world I/O、产物发布完成，收口关闭竞态 | [Lifecycle](D:/projects/ZCode/apps/zcode-cli/packages/bootstrap/src/app/dynamic-workflow-run-lifecycle.ts:150) |
| 首轮运行卡 | Hub 直接启动没有用户气泡，首轮留空间避免被状态胶囊挡住按钮 | [布局](D:/projects/ZCode/packages/ui/src/v4/conversationLayout.ts:14) |
| 流式 UI 成本 | workflow 行联接按 history/tail 分区缓存 | [卡片 join](D:/projects/ZCode/packages/ui/src/v4/workflowRunCardJoin.ts:313) |
| 产物文件动作 | 文件和 Markdown 支持仅有 saveFile 的 Web 平台，菜单合并到共享 file-menu | [产物侧栏](D:/projects/ZCode/packages/ui/src/app-shell/WorkflowArtifactSidePane.tsx:211) |

静态分析也有实质修复：按 helper activation 解析 callback 绑定；修正扇出区域归属；SCC 处理参数 placeholder；消费已 join 的 CFG fork 来源，减少虚假边；缓存共享 helper 的 iteration region。整体仍是 TypeScript Program、类型/Schema、抽象解释、lowering、脚本进程及宿主 AgentRuntime，未更换架构。

证据：[helper bindings](D:/projects/ZCode/apps/zcode-cli/packages/dynamic-workflow/src/analysis/causality-order-bindings.ts:15)、[扇出归属](D:/projects/ZCode/apps/zcode-cli/packages/dynamic-workflow/src/analysis/interpret.ts:43)、[SCC](D:/projects/ZCode/apps/zcode-cli/packages/dynamic-workflow/src/analysis/state.ts:705)、[CFG join](D:/projects/ZCode/apps/zcode-cli/packages/dynamic-workflow/src/analysis/flow-graph.ts:265)。

本次 `dynamic-workflow/src/engine/`、fill、retune、Actor 模型与权限模块没有 diff。既有 journal、FIFO、动态补全、修订缓存等语义继续保留，没有因为这次更新自动解决外部副作用 exactly-once。

**moejs 能补上什么**

| 部分 | 结论 |
| --- | --- |
| JavaScript / 可擦除 TypeScript 执行 | 可由纯 Go moejs 承担 |
| Promise、async/await、循环、条件、异常 | 有语言运行基础，实验已覆盖其中的并行、循环、异步 |
| Go 异步模型/工具调用 | 由宿主立即返回 Promise，后台执行，经 eventloop settle 回 VM owner |
| 受控脚本能力 | 可由 Go 注入 facade，限制 import、eval、网络、文件与动态源码 |
| JS AST、源码位置和词法绑定 | 可作为自研站点识别、分析和 lowering 的基础 |
| TS 类型检查 | **没有**；CompileTS 是类型擦除 |
| `ask<T>` 的 T → JSON Schema | 类型节点被丢弃，必须有独立类型/Schema 层 |
| 四图分析与站点实例身份 | **没有现成实现**，不能把 AST 当成分析器 |
| workflow journal / resume / amend / fill | **不是引擎库自动提供的能力**，仍由 GCode 实现 |
| VM/闭包/Promise 持久快照 | 没有现成 API，应重建 Runtime 后通过日志重放 |

源码证据：[类型擦除](D:/projects/moejs/syntax/typescript.go:8)、[CompileTS](D:/projects/moejs/module.go:50)、[AST 遍历](D:/projects/moejs/syntax/walk.go:20)、[Runtime goroutine 边界](D:/projects/moejs/runtime.go:55)、[异步 settle](D:/projects/moejs/eventloop/eventloop.go:445)。

这比先前评估中的“需要额外 JS 运行层”更具体：当前本地 moejs 已能在 Go 进程内承担执行层；不能沿用旧 alpha.9 对 Node/Web API 缺失的历史结论判断本版。是否安装这些 API 由宿主决定，workflow 并不需要开放全部 Node 兼容面。

**本轮独立实验：7 项通过**

代码位于 [scripts/experiments/moejs-workflow](D:/projects/GCode/scripts/experiments/moejs-workflow/README.md)，独立 `go.mod` replace 指向相邻 moejs，不修改 GCode 主模块。代码为本次独立验证实现，没有搬用 ZCode 业务代码。

| 用例 | 实际证明 |
| --- | --- |
| AsyncTypeScriptJSONReplay | TS Promise.all + 循环能调 Go 异步任务；观测到 2 个宿主任务并行；重建 Runtime 后复用 JSON 结果 |
| RebuildRetriesOnlyUnfinishedNode | 宿主函数返回错误、记录未完成后，重建只重做未完成记录；不是进程崩溃测试 |
| NaiveJSONCacheDoesNotPreservePromiseRace | 首次 B 胜出，立即兑现缓存后 A 胜出。缓存路径与实时路径均返回 Promise，反例不依赖 API 返回类型变化 |
| CompileTSErasesTypesWithoutChecking | `const n: number = "not-a-number"` 编译并返回字符串，证明没有类型检查 |
| DirectEvalCapturesLocalBinding | direct eval 创建的函数能访问调用点局部变量 seed；未证明 TS hole 编译/批准完整流程 |
| ContextInterruptsInfiniteScript | 等待脚本在循环中发出 entered 信号后取消，运行返回 InterruptedError |
| JSONJournalRejectsChangedIdentity | 脚本哈希或同一显式 site 的输入变化不能误命中缓存 |

执行环境与结果：

```text
Go 1.27.3 / Windows amd64 / CGO_ENABLED=0
go test -v ./...      PASS，7 项
go vet ./...         通过
GOOS=linux GOARCH=amd64 go test -c  通过交叉编译
```

Linux 二进制没有实际运行；未运行 race；没有真实 Agent、真实供应商、MyGo 窗口或完整 workflow 验收。

本实验中 `commitSequence` 仅为结果提交顺序，不是完整 JS Promise release 顺序；同一循环静态站点的 ordinal 也未实现。不能把这份约 350 行的验证程序直接变成产品运行器。

**JSON 存储完全可用，但需要存执行事实**

不需要为了复刻产品功能照搬 ZCode SQLite。建议按 run 拆分，避免把所有流式数据反复写进整份 `engine.json`：

```text
workflows/
  projects/<workspace-id>/<name>/definition.json
  global/<name>/definition.json
  sources/<source-hash>.ts
workflow-runs/<run-id>/
  run.json
  events.jsonl
  actors/<actor-id>.json
  results/<instance-id>.json
  artifacts/manifest.json
  artifacts/blobs/<content-hash>
```

这是建议布局，尚未成为生产数据格式：

- definition：名称、描述、参数、源代码引用、模型/并发默认配置。
- run：父会话/宿主/目录、有效源码、状态、修订关系、检查点 cursor、预算。
- events：单调序号、接受/开始/结算/Promise 释放、phase、question、fill、retune、产物变化。
- actors：身份、会话上下文及实际模型配置引用；不把凭据交给 JS。
- results：站点、ordinal、输入哈希、结果/错误、缓存来源及大结果引用。
- artifacts：声明、版本、流式数据序号和不可变字节内容。

必须满足的顺序是：

1. 接受实例并持久化其身份/输入。
2. 派发宿主能力，业务 Context 和工具进程归 Go 核心管理。
3. 大结果先写入内容文件；在串行协调器中提交结果及明确的释放顺序。
4. 日志提交成功后，才在 VM owner 上兑现对应 Promise。
5. 重启创建新 Runtime，按原有效脚本与日志重放；已完成结果按历史释放顺序返回。

JSONL 同样需要单写者、Sync 策略、检查点、尾部半条记录处理及引用一致性。恢复时不得把损坏记录悄悄当空记录；无关旧格式兼容和迁移不加入当前开发范围。

本轮 Promise.race 反例直接证明了“只缓存结果”的不足：改变谁先兑现，可能改变分支、后续站点 ordinal、工具输入和最终产物。静态图 JSON 也不能替代这些执行记录。

外部命令已产生副作用、但结果尚未记账的崩溃窗口仍存在。对于这类 running 实例，需要明确“结果未知”、检查/人工决定或业务幂等政策，JSON 或 SQLite 本身都不能解决。

**实现边界应固定在 Go 核心**

建议职责为：

```text
原生 MyGo UI ──强类型 Go 调用──> Workflow 核心
                                   ├─ Actor / 模型 / 工具 / 权限
                                   ├─ 调度 / 预算 / JSON journal
                                   ├─ 分析与 Schema 层
                                   └─ moejs 脚本 Runtime + 受控 facade
远端客户端 ──真正的网络边界────> 同一 Workflow 核心
```

每个脚本 Runtime 只由一个 goroutine 使用，后台 I/O 不直接操作 JS 值，settle 回 eventloop。普通子 Agent 仍不能直接充当可多轮 ask 的 Actor；需要增加内部 Actor 执行接口，保留每 Actor FIFO 和独立上下文。

类型层有两种可选设计：保留 TS 泛型作者体验，单独实现类型保留/解析/Schema/必要诊断；或改为显式 schema facade。后者可以支持相同业务能力，但会改变作者接口，不能冒充已经支持参考的泛型类型语义。本轮不替用户默认删掉这项能力。

还需明确：

- eventloop.New 会安装定时器，Date/Math 也有非确定性；workflow 应主动封闭这些能力。
- direct eval 才能捕获现场闭包，Go 的 CompileFunctionTS 是全局环境函数；动态 TS body 还需先转换/lowering。
- Runtime 内存限制不包含所有 Go worker/模型输出/产物成本，宿主仍需预算。
- VM 中断不等于取消正在执行的 Go I/O 或工具进程，两条取消链都要接。
- 同进程 moejs 不等于 OS 子进程隔离；如果需要独立崩溃/资源隔离，可以使用 Go helper，业务状态仍由 Go 核心持有。

**新版仍存在的偏差与未验证问题**

前次六项文档/注释偏差仍需按代码解释：

1. 旧 Workflow Debugger 文档仍提不存在的 UI/service 接口。
2. 总述仍写 run 内无 prompt；Actor 虽不再直接问用户，工具权限确认仍可发生。
3. 总述仍称唯一硬约束是 facade 直接调用，实际还有多种 authoring 检查。
4. 总述仍称开跑前完整图固定，没有限定动态 hole。
5. 详细 execution-engine 已说明没有第二份运行时白名单，但 analysis/world-run.ts 仍写 driver 复验。
6. phase 的 Required 文案仍与允许无 marker 的代码冲突。

此外，moejs README 开头写 test262 79,385，后文及 `bench/test262/RESULTS.md` 记录 80,971 pass / 14,268 skip。这是文档计数不同步；本轮没有重跑 test262，不能宣传无条件“全测试通过”。

新 ZCode 的 world.run 仍未在当前调用中携带 run AbortSignal，而关闭会等待 world I/O；长命令是否会延迟关闭需要真实回归。新版默认 full 输出提升至每流 16 MiB，也增加 journal 与内存成本。配额时间解析的默认 UTC+8 及特定 provider 业务码不能直接作为 GCode 自定义供应商的统一规则。

ZCode 缺少依赖，本轮没有执行其测试或启动实窗；新 AGENTS 已退役 App 业务单测，保留 CLI/E2E/浏览器/工程门禁，不能沿用旧调查中的测试库存描述。

当前可以确定的是技术路线成立；完整交付仍需 Actor、类型/Schema、静态分析/lowering、journal 重放、amend/retune/fill、产物、原生展示及真实远程验收逐层完成。
