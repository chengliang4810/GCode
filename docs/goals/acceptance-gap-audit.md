# /goal：GCode 首版剩余验收缺口核对

状态：只读核对完成，2026-10-12（Asia/Shanghai）。本文件只登记当前仍需完成的实现、验收和外部依赖，不修改产品代码、测试、原生窗口、用户数据或已有四份矩阵/计划文档。

## 结论

GCode 首版仍未闭环。当前已有可启动的 Go + mygo 原生入口、macOS Intel 生命周期、应用层会话/文件/worktree/扩展 S2 证据和最终包 `7bb817…` 的自动化回归。源码复核表明，session 的基本状态转换、run identity、队列取消屏障、旧授权/提问失效和重启暂停已经实现；剩余 session 工作集中在持久化事件合同、schema/version 校验和事件保存失败的可见性。前台 child specialist 与 swarm 已经走 Go runtime，但 `run_in_background` 仍需要一个真正由桌面拥有的长期宿主。除此之外，桌面 worktree/扩展管理入口、真实四类厂商 API 与 MCP OAuth、当前包的完整原生交互、持续性能和目标设备仍未闭环。

视觉方向是单独的用户决策门槛。设计要求先取得 T3 暗黑方向确认，再扩展 worktree 和其他管理页面；当前 `visual-v3/confirmed` 仍是 `candidate`，不能把内部复核或构建结果当作用户确认。

## 产品边界和 OAuth 判定

### OAuth 只保留 MCP 标准协议

“真实 API/OAuth”这个合并写法容易扩大范围，当前正确解释是：真实厂商 API 使用 API Key；OAuth 只用于标准 MCP server 的授权、刷新和 token 存储。它不包含 ChatGPT/Codex 订阅账户登录、订阅身份、Codex websocket 或 provider OAuth resolver。

- `PRODUCT.md:23-26` 明确删除模型订阅登录，同时保留四类 API Key 和标准 MCP OAuth。
- `docs/design/gcode-desktop.md:15-21` 明确移除订阅登录，模型接入为 API Key，MCP 认证独立保留。
- `docs/design/gcode-desktop.md:137-143` 要求删除订阅登录/刷新，并将通用 Responses 改为普通 API Key endpoint。
- `docs/goals/api-providers.md:3,40,50,78,81,91-93` 已移除订阅 OAuth 生产路径；当前 provider 只接受普通 API Key、明确的自定义认证或本地 provider，`MCPOAuthConfig` 和 MCP token store 独立保留。
- `docs/goals/session-runtime.md:98-104` 只把真实 provider 和 MCP OAuth 的授权、刷新、脱敏列为未验收边界，并要求 token 使用 GCode `DataDir`，不回落到 zero 数据目录。

因此，后续验收记录应写成“真实四类 API Key 与 MCP OAuth”，不要写成可能被理解为“支持订阅账号登录”的“真实 API/OAuth”。当前没有产品需求要求重新加入订阅登录，也不能为了完成 P5.1/P5.2 引入该路径。

### 物理输入的范围解释

设计第 10 节 `docs/design/gcode-desktop.md:229-238` 把中文输入法组合、焦点、终端滚动/复制和平台运行列为原生验收维度，但这首先是证据范围问题，不是所有后续实现的前置阻塞。AX、原生合成事件和可复现的自动化输入可以验证控件状态、事件归属、滚动注入和资源收束；只有在声称真实硬件键盘、IME 候选定位、触控板惯性或特定设备行为时，才需要相应硬件和人工环境。缺少物理设备时保留“未验证”，继续推进不依赖设备的运行时、API 和工作区工作。

## 本轮源码复核：session 状态/恢复与 specialist 宿主

### 已关闭的实现缺口

- `internal/appmodel/types.go:43-53,64-82,115-145` 已持久化 session 的运行状态、`RunID`、`ActiveRun`、队列/消息状态、`PauseReason`、权限和提问。正常完成的 session 状态是 `Idle`，由 user/assistant message 的 `completed` 表示完成；停止或重启使用 `Interrupted`，普通失败和取消失败使用 `Failed`，不能再把“没有状态字段”列为当前缺口。
- `internal/controller/controller.go:969-991,1152-1235,1666-1713` 已把正常完成、显式停止、纠正替换、失败、取消失败和重启分别映射到状态、消息状态和队列暂停原因。`internal/controller/controller.go:1377-1380` 以 active pointer 和 generation 丢弃迟到回调；`ReplyPermission`/`ReplyAsk` 在 `internal/controller/controller.go:753-827` 拒绝不属于当前 generation 的旧回答。
- 当前测试已经覆盖这些行为：`internal/controller/controller_test.go:517-728,764-879` 覆盖队列暂停、取消失败、旧回答、重启不自动重跑和持久化回滚；`internal/controller/cancel_warning_test.go:14-141` 覆盖正常取消告警抑制和取消失败可见性；`internal/application/permission_question_integration_test.go:23-228,230-371` 覆盖 Service/provider 边界的授权、提问回答历史和纠正后的旧回答拒绝；`internal/application/run_identity_integration_test.go:20-259` 覆盖跨 Controller、Runtime、连续运行和事件日志的 run identity。
- 前台 specialist child 已经不是 CLI 宿主路径：`internal/runtime/runtime.go:434-458` 把 `engine.childRunner` 注入 `specialist.Executor`，`internal/runtime/subagent.go:17-119` 在同一 Go runtime 中创建 child session、执行 `engine.Run`、传播事件和取消；`internal/swarm/launcher_specialist.go:29-73` 也通过同一个 Executor 运行 swarm member。`internal/runtime/runtime_test.go:167-394`、`internal/runtime/run_identity_test.go:16-104` 和 `internal/application/extensions_integration_test.go:199-400,425-570` 已覆盖 child 的权限、独立 run、Service 集成以及 Stop/Close 取消。

### 已实现但只缺特定验收或合同明确

- Service 层的本地协议替身已经证明状态和扩展链路；还缺真实四类 API Key、标准 MCP OAuth、当前包的原生窗口和目标设备证据。这些是外部验收边界，不能倒推为 session 基本状态尚未实现。
- `internal/controller/controller.go:1382-1437` 在重启时会把 active/cancelling/waiting session 标为 `Interrupted`，设置 `pauseReasonRestart`，标记进行中的消息为 interrupted，清空待处理权限/提问并保留队列；`internal/application/runtime_boundaries_test.go:87-174`、`internal/controller/controller_test.go:678-728` 已验证不自动重跑和显式再次发送。需要补的是当前包/原生 Service 的回放证据，以及把“正常完成 = Idle + ActiveRun=nil + terminal message”固定为 UI 合同。
- 权限/提问回答在 runtime session journal 中有请求、决定和回答事件（`internal/runtime/run.go:124-176`）；Controller snapshot 只保留当前 pending 项，回答后会移除。若产品要在 Controller snapshot 中直接展示完整回答审计，应单独定义该合同；当前不能再写成“旧回答完全没有历史”。
- `internal/specialist/lifecycle_test.go:15-220` 和 `internal/specialist/runtime_test.go:31-126` 已覆盖 background manager 的登记、输出、停止、resume、关闭清理和 killed 状态保留，但它们注入了假的 launcher/manager，尚未证明桌面真实宿主。

### 仍缺具体行为

- Controller snapshot 只有 `storedState.Version=1`（`internal/controller/controller.go:146-149,1439-1466`），`load`（`internal/controller/controller.go:1382-1437`）没有校验版本，也没有拒绝未知的 `Message.State`/`QueuedMessage.State`；空状态会被默认补齐，未知值则会继续进入内存。需要定义当前 schema/version 合同、非法状态的处理和恢复原因，不能把一个未使用的 `version` 字段当作完整版本合同。
- durable session event 本身只有 id、session、sequence、type、createdAt 和 payload（`internal/sessions/store.go:206-213`），`schemaVersion=2` 只属于 stream-json 进程协议（`internal/streamjson/streamjson.go:17,62-106,243-258`），没有进入 session journal 的事件 envelope。更严重的是 `internal/runtime/run.go:31-34,243-254` 忽略 `appendEvent` 错误；事件日志不可写时，Controller 仍可能完成并保存 snapshot，调用方看不到“状态成功但历史不完整”的失败。需要让事件保存失败进入当前 run/session 的可观察错误路径，并为事件格式定义明确版本和 replay 规则。
- 重启或纠正会清除 pending permission/question（`internal/controller/controller.go:1159-1162,1399-1410,1733-1738`），但没有追加“该请求因哪次 run/generation、何种原因失效”的持久化审计事件。现有 generation 检查足以阻止旧回答生效，剩余工作是补失效事实的 durable 记录，而不是重新设计 generation 过滤。
- `Task` 的 `run_in_background=true` 仍是具体的宿主缺口。`internal/specialist/exec.go:459-585,689-725` 会构造 `exec ... --output-format stream-json`，默认用 `os.Executable()`，再由 `launchBackgroundProcess` 启动外部进程；但当前唯一入口 `cmd/gcode/main.go:33-69` 只处理 Windows sandbox 子命令，然后进入 mygo GUI，没有 `exec` CLI 分发。桌面路径因此不能把自己的可执行文件当作 specialist CLI 宿主。并且 `internal/runtime/runtime.go:443-448` 在每个 parent `engine.Run` 结束时 defer `specialistRuntime.Close`，而 `internal/specialist/runtime.go:76-99` 会 `KillRunning`；即使外部进程能启动，后台任务也会被 parent run 的收尾清理，且默认 manager 根目录（`internal/background/manager.go:97-110`）不在 GCode `DataDir`，`internal/runtime/runtime.go:743-754` 的 `Engine.Close` 也没有全局 background manager 清理。需要实现由桌面 Service/Runtime 持有的长期 background specialist host、DataDir 隔离、跨 parent run 的 TaskOutput/TaskStop 和 Service.Close 清理；保留后台 specialist 能力，不得用删除模式来规避问题。

## 剩余必须完成项

| 类别 | 当前事实 | 剩余结果 | 依据与边界 | 依赖/责任 |
| --- | --- | --- | --- | --- |
| 会话状态合同与可审计恢复 | `internal/controller` 已实现 run/generation、队列、取消、权限/提问等待、消息状态、取消屏障和重启暂停；Controller 与 Service 测试也覆盖主要状态映射。剩余是 snapshot schema/version 校验、session journal 事件版本、append 错误可见性和 pending 请求失效事实。 | 保留现有状态映射；定义并实现当前 snapshot/event 合同，拒绝或显式处理未知状态/版本，把事件保存失败纳入 run/session 错误，追加重启/纠正造成 pending 失效的 durable 审计，然后补当前包的 Service/原生回放证据。 | 源码 `internal/appmodel/types.go:43-145`、`internal/controller/controller.go:969-1235,1377-1482`；测试 `internal/controller/controller_test.go:517-879`、`internal/application/runtime_boundaries_test.go:87-244`、`internal/application/permission_question_integration_test.go:23-371`。设计 `docs/design/gcode-desktop.md:48-57,158-168,214-227`；计划 `docs/plans/gcode-desktop-implementation.md:33-38,44-45`。 | session-runtime + root；不需要厂商凭据或目标设备。 |
| specialist/扩展宿主路径 | 前台 child 与 swarm 已经走 Go runtime；只有 `Task` 后台模式仍通过 `os.Executable()`/`exec` 参数启动外部进程。当前 `cmd/gcode` 没有 `exec` CLI 分发，parent `engine.Run` 结束还会关闭并杀掉 background manager 的 running task。 | 实现桌面 Service/Runtime 持有的长期 background specialist host，使用 GCode `DataDir` 保存 task metadata/output，跨 parent run 支持 TaskOutput/TaskStop，Stop/Close 能收束进程组、prompt 文件和 specialist 事件；保留并真正打通后台 specialist，不删除该能力。 | `internal/runtime/runtime.go:434-458`、`internal/runtime/subagent.go:17-119`、`internal/specialist/exec.go:459-585,689-745`、`internal/specialist/runtime.go:76-99`、`cmd/gcode/main.go:33-69`；现有 unit 仅见 `internal/specialist/lifecycle_test.go:15-220`。设计 `docs/design/gcode-desktop.md:135-160`；计划 `docs/plans/gcode-desktop-implementation.md:59-63`。 | runtime/执行资源改造可自主推进；真实 API、OAuth 和原生配置验收另列。 |
| 桌面 worktree 入口 | `application.Service`/`internal/worktrees.Manager` 已证明创建、绑定、重开、脏保护、合并和删除；桌面没有创建、分支选择、合并、删除入口。 | 在视觉基线确认后提供 worktree 开关、来源分支/提交/cwd 展示、创建失败保护、显式合并/删除及占用提示；不自动清理，不搬运源目录未提交内容。 | 设计 `docs/design/gcode-desktop.md:188-198`；计划 `docs/plans/gcode-desktop-implementation.md:54`；矩阵 P4.4/A17/A18。Service 证据不能当作桌面功能通过。 | 后端接口已有；页面扩展等待用户视觉确认，随后由 workspace-tools + desktop-ui + root 完成。 |
| 四类真实厂商 API | provider factory 和 `httptest` 协议替身覆盖 Responses、Chat Completions、Anthropic、Gemini 的流式文本、工具、错误、取消和历史协议；没有真实厂商凭据 S2。 | 每类 API Key endpoint 在临时真实项目完成读文件、修改文件、命令验证、工具调用、取消、错误、历史恢复和下一轮切换模型；记录脱敏请求、响应、目录和失败边界。 | 产品 `PRODUCT.md:25-28`；设计 `docs/design/gcode-desktop.md:19-20,216-220`；计划 `docs/plans/gcode-desktop-implementation.md:59-61`；`docs/goals/api-providers.md:73-93`。 | 需要用户提供相应 API Key、网络和可用额度；当前可先自主准备四类请求/日志/脱敏 harness，不把协议替身升级为真实通过。 |
| MCP OAuth | MCP 的 OAuth token path 已指向 GCode `DataDir`，但真实授权、刷新、脱敏日志和 server 组合尚未验证。 | 用标准 MCP OAuth server 验证授权、刷新、token 隔离、工具注册、拒绝和 Stop/Close 清理；不引入模型订阅登录。 | `docs/goals/session-runtime.md:98-104`、`docs/goals/api-providers.md:50,91-93`；这是 MCP 独立边界，不是四类模型 provider 的认证方式。 | 需要可用 MCP server/授权凭据；可先完成隔离 fixture 和记录模板。 |
| 当前包的原生对话/流式证据 | `output/acceptance/native-streaming/round-4/review.json` 为 `PASS_FOR_RECORDED_SCOPE`，但 `performance_scope=NOT_ACCEPTED`、`scroll_scope=INJECTED_ONLY`；AX 阻塞采样，不能证明持续性能、实际滚动效果、阅读位置恢复或三流同时运行期间完成全部操作。 | 在 root 当前 AX 诊断结束后，补当前包 `7bb…` 的非阻塞采样、流式增高、历史 prepend 锚点、实际 viewport 前后状态和长会话交互证据；保持 round-1/2/3 历史失败，不覆盖为成功。 | 设计 `docs/design/gcode-desktop.md:70,231-238`；计划 `docs/plans/gcode-desktop-implementation.md:51,63`；现有边界见 `docs/verification/acceptance-matrix.md:26,52,55,89`。 | root 正在 AX 诊断，本轮不重复打开窗口或运行流程；后续可由 root/desktop-ui/acceptance-audit 收口。 |
| 当前包的终端剩余交互 | Intel 包已通过 PTY load、cwd、合成输入、resize、shell exit 和 Close 清理；wheel/trackpad、selection/OS clipboard、物理键盘、中文 IME 和完整原生交互仍未形成全量 S3 证据。 | 区分可由 AX/合成事件复现的控件和终端状态，与需要设备行为的硬件惯性/IME/剪贴板路径；分别记录通过、未验证和设备限制。 | 设计 `docs/design/gcode-desktop.md:229-238`；计划 `docs/plans/gcode-desktop-implementation.md:53`；矩阵 P4.3/A14。 | 自动化原生事件和现有 Intel 环境可继续自主收口；真实硬件/跨平台结论需要设备，不能阻塞无关实现。 |
| Apple Silicon 与 Windows | 当前只有 macOS 14.8.7 Intel 的包、窗口、PTY 和资源证据；产品目标明确包含 Apple Silicon，随后 Windows。 | 分别在目标设备记录构建、原生运行、窗口生命周期、终端依赖、内存/延迟和未验证项；交叉编译只能作为准备。 | `PRODUCT.md:5-7`；设计 `docs/design/gcode-desktop.md:18,237`；计划 `docs/plans/gcode-desktop-implementation.md:64`；矩阵 A15/P5.4。 | 需要目标设备和 Windows/macOS 环境；无设备时保持未验证，同时推进代码和证据模板。 |
| 用户视觉确认 | T3 暗黑截图是最低质量标准，当前 visual-v3 技术复核仍为 candidate，用户尚未确认。 | 用户确认代表性样板方向后，再扩展 worktree、扩展配置和其他管理页面；若未确认，继续大规模视觉/页面扩展会重复返工。 | `PRODUCT.md:30-32`；设计 `docs/design/gcode-desktop.md:5,28,74,94-98`；`GOAL.md:34-36`。 | 只能由用户完成；不以代码测试、内部 review 或截图 hash 替代。 |

## 现在可自主执行的三个最高价值方向

1. **先收敛 session/appmodel 合同和恢复审计。** 保留已经通过测试的状态映射、generation 和取消屏障，补 snapshot/event schema/version 校验、事件保存失败可见性和 pending 失效审计；这不需要厂商凭据、目标设备或用户视觉确认，直接决定纠正、队列、重启和迟到回调能否被可靠验收。
2. **准备并实现真实 API/MCP 的验收 harness 和脱敏边界。** 四类 provider 先统一请求记录、临时项目动作、错误注入、取消和历史切换的采样格式；MCP OAuth 单独使用 token path 和 refresh fixture。代码和 fixture 可先自主完成，实际厂商请求只在用户提供 API Key/网络后执行，不能用“OAuth”名义要求订阅账号登录。
3. **完成不依赖视觉扩展的 runtime/工作区集成准备。** 优先实现长期 background specialist host 的执行归属、DataDir 隔离、跨 parent run 的 TaskOutput/TaskStop 和 Stop/Close 清理；同时把现有 worktree Service 的创建/绑定/合并/删除合同整理成桌面适配 seam 和临时 Git 验收脚本。视觉确认前不扩展新页面，确认后再接入 worktree 管理 UI。

## 暂不执行的动作

本轮不打开 GCode 窗口，不运行测试、打包、真实 API 或新的 streaming 流程，不触碰原包 `7bb817…`、用户凭据和原生输出目录。root 当前正在做 AX 诊断，应先保留其结果，再决定当前包的下一轮原生证据是否需要补采样。
