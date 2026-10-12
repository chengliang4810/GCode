# /goal：GCode 首版验收审计

状态：初版矩阵已建立（2026-10-12）；持续审计中。

本目标只负责把设计基线和执行计划转成可复核的验收项，检查代码、行为测试和真实运行证据是否同时存在，并把阻断缺口反馈给 root。源码、配置和其他目标的文档不在本目标所有权内。

## 审计边界

- 设计依据：`docs/design/gcode-desktop.md`，尤其是第 1、2、3、6、7、8、10 节。
- 交付依据：`docs/plans/gcode-desktop-implementation.md` 的阶段 1—5 完成条件。
- 当前 appmodel 合同：`internal/appmodel/types.go`。
- 当前执行面：`internal/controller`、`internal/runtime`、`internal/desktop`、`internal/providers`、`internal/workspace`、`internal/worktrees`、`internal/execution`。
- 本审计允许运行只读构建和测试，不写入产品数据、用户凭据、真实项目或原 zero 数据目录。

## 证据等级

| 等级 | 含义 | 可支持的结论 |
| --- | --- | --- |
| S0 | 设计或源码静态存在 | 只能说明有类型、入口或实现意图，不能说明行为通过 |
| S1 | 针对实现的 Go 单测/协议替身/race 输出 | 只能说明自动化覆盖的行为；不能替代真实 API、原生窗口或目标设备 |
| S2 | 本地真实临时项目/真实 API/真实进程运行记录 | 可支持相应场景的运行结论；必须记录请求类型、目录、结果和脱敏日志 |
| S3 | 目标平台真实应用包、窗口、输入法、PTY 或设备证据 | 可支持原生和平台验收；离屏 `ui.NewTester`、交叉编译和截图文件本身不够 |

单项只有在其要求的最低等级证据齐全后才能标记“通过”。“已实现”“编译通过”“协议替身通过”“离屏截图通过”均不能代替设计第 10 节要求的 S2/S3 证据。

## 当前事实

1. `docs/source-origin.md` 已固定 zero 提交 `99721c762f37cd43ac511007a5f51d1846df959e`，并记录 clone、归档、全树比对和 MIT 许可；`docs/THIRD_PARTY_NOTICES.md`、`resources/licenses/` 和 `docs/goals/third-party-notices.md` 已登记 21 份许可证原文及当前依赖映射。`output/acceptance/stage-5/native-provenance/verification.txt` 进一步证明 Ghostty manifest 原始资源经 MyGo 实际 ad hoc 签名后与当前包内 dylib 逐字节一致，计划 1.1 已完成。
2. `cmd/gcode` 已存在，并通过 `internal/application.Service` 接入 controller/runtime/desktop；`docs/verification/stage-1.md` 记录 Go/mygo 构建、ad hoc 签名、真实窗口、设置页、托盘、关闭隐藏和同 PID 重开，`docs/verification/stage-4.md` 又记录了运行中取消/停止并退出、后台完成和同 PID 恢复。输入法、完整原生交互、设置持久化和多平台生命周期仍未验收。
3. `internal/desktop` 已有三栏视图、主题、会话消息、工具、权限、问题、队列、文件/Diff/终端标签和设置样板；`markdown.go` 已有常用 Markdown、Chroma、选择/复制和异步高亮。`docs/verification/stage-4-conversation.md` 已验证 `ui.ListState`/`ui.List` 的可视范围与 overscan、底部跟随、上滑保持、会话独立位置和历史 prepend 锚点；原生滚轮、触控板惯性、中文输入法和真实 controller 事件仍不能由离屏测试替代。
4. `internal/runtime` 已有进程内 Agent 组装、API Key 凭据读取、sandbox、MCP、Skills、Hooks、插件和 specialist/swarm 注册路径；`docs/verification/stage-5-extensions.md` 已从真实 `application.Service` 入口经过 controller/runtime/Chat Completions adapter 完成本地应用层 S2，覆盖真实子进程、allow/deny、结果历史、warning 脱敏、Stop/Close 取消和 MCP 清理；真实厂商 API/OAuth、原生扩展配置界面和目标设备仍待。
5. `internal/controller` 已有会话、纠正/队列、权限/提问等待、快照持久化、消息状态、运行配置快照和取消代际校验；controller 普通测试/定向 race，以及阶段 2/3 application 协议替身场景均有记录。仍不能把协议替身结果当作真实 API、原生窗口或完整边界验收。
6. `internal/appmodel` 已补入 `Message.RunID/State`、`QueuedMessage.State`、`Session.ActiveRun`、`RunConfig` 和 `Session.PauseReason`；四种 API format、运行中/取消中/等待权限/等待问题/失败/中断状态以及消息、队列、权限和问题载荷均有当前合同表达，权限/提问失效事实、取消失败细分和完整事件重放仍缺少可审计字段或行为证据。
7. `internal/execution` 仍是 Agent 托管 shell 进程面；`NativeTerminalFactory`、Darwin `syscall.Open` 非阻塞 PTY 补丁和应用包内 `libghostty-vt.dylib` 已接入。最新 `output/acceptance/stage-4/terminal/terminal-qa-unicode.log` 以及 patched/Unicode 测试已通过 terminal load、cwd、输入、CJK/emoji、resize、shell exit 0 和 Close 后 PID gone；滚动/复制、物理键盘和 GCode 原生窗口最终交互仍未单独验收。
8. 阶段 2/3 已在临时项目和本地协议替身下验证文件工具、重载历史、三个同目录 session、托管进程后代和独立终端隔离；当前没有真实厂商 API、Apple Silicon 或 Windows 的本轮证据。缺凭据和设备时状态必须保持“未验证”。
9. `internal/controller/controller.go` 当前已检查 `persistErr`，失败时回滚或写入状态保存失败并阻止继续报告成功；仍缺不可写目录下的 S2 重启和 UI 回归，不能把静态处理当作持久化验收。
10. runtime 在返回错误前使用 provider key 做脱敏，controller 在最终状态也调用 redaction；仍需用含 key 的 provider/网关错误验证 `controller.json`、session 历史和 UI 均不泄漏。
11. plugin/hook/skill diagnostics、skill duplicate、MCP skipped 和 specialist Load/manifest warning 已通过脱敏 warning 上报，MCP/specialist/swarm/执行进程有清理路径；`docs/verification/stage-5-extensions.md` 已记录本地应用层 S2 的扩展工具、授权边界、结果历史、错误、取消和 MCP stdio 退出，真实 API/OAuth、原生扩展界面和目标设备仍未闭环。
12. execution Agent 路径通过 `StartOwned` 传入 session 归属，阶段 3 已验证三个 session 与独立终端隔离；`internal/tools` 仍保留无 session 参数的 `ExecSessions`、`Stop*`、`Write*`、`Resize*` 接口，需继续确认桌面入口不会复用或收紧入口。

## appmodel 合同缺口

以下是对设计边界的静态审计结果，供 session-runtime/root 在维护共享合同前处理。这里不直接改 `internal/appmodel`。

| 缺口 | 影响的设计规则 | 当前表现 | 建议责任 |
| --- | --- | --- | --- |
| 已提交消息状态字段刚补入，尚未完成全状态行为审计 | 所有已提交消息必须有持久化标识和明确状态；停止/失败/重启后队列暂停且可继续 | `QueuedMessage.State`、`Message.State/Interrupted` 已记录提交、排队、运行、完成、失败、撤回和中断等当前状态；仍需验证每个转换、保存失败和重启后的完整恢复 | session-runtime + root |
| run 归属和运行配置快照刚补入，历史交互状态仍不完整 | 迟到回调必须按 session/run 归属；运行中模型保持旧配置 | `Message.RunID`、`Session.RunID/ActiveRun`、`RunConfig` 和 controller generation 已能关联当前 run；权限/提问回答及请求失效事实仍主要在内存中 | session-runtime |
| `Session.ActiveRun` 已表达运行中的 provider/model/cwd/权限快照 | 模型修改对下一次任务生效，已运行任务保持原模型 | 当前快照可记录 `ProviderID`、`ProviderName`、`Format`、`BaseURL`、`Model` 等；设置变更、保存失败和重启重放边界待验证 | session-runtime + root |
| `RunState` 没有 completed/succeeded、paused、cancel-failed 或 explicit-stop 语义 | 正常完成、停止、失败、重启暂停、取消失败的 UI 和队列行为不同 | `Idle`、`Interrupted` 与 `Failed` 可表达部分结果，但 `QueuePaused` 与 `Error` 不能稳定表达原因和用户可继续动作 | session-runtime + desktop-ui |
| `Permission`/`AskRequest` 缺少请求状态、失效原因和回答持久化载荷 | 纠正必须使旧授权/提问失效，迟到回复不能影响新任务 | 内存 pending map 可拒绝旧代际回复，但快照清除后没有“已失效”事实可审计；问题回答未进入 appmodel 历史 | session-runtime |
| provider 合同使用 `Provider.Format`，凭据由独立 credential store 管理 | 四类 API Key、兼容网关和普通 Responses endpoint 需要明确路由与认证 | `APIFormat` 已能区分四类 wire 路由，API key 不进入 appmodel 快照；custom headers 不是首版硬要求，真实 API、网关错误和脱敏仍待验证 | api-providers + root |
| `Session` 没有创建时间、恢复原因或 worktree 标识/来源分支快照 | 重启后历史/状态/工作目录可恢复，worktree 绑定需可复核 | `PauseReason` 已记录当前暂停原因；仍只有 `UpdatedAt`、`Cwd`、`Worktree`、`Branch`，无法完整区分恢复原因，也难以在快照内审计 worktree 来源 | session-runtime + workspace-tools |
| `Snapshot` 没有 schema/version 或持久化错误状态 | 重启恢复必须可诊断，保存失败不能静默成功 | controller 文件有外层 version，且生产路径已有 `persistErr` 回滚/错误标记；UI 快照仍没有独立的保存失败/恢复告警字段，S2 不可写目录回归待补 | root + session-runtime |

这些缺口不是要求预建通用总线或兼容迁移层；目标是让当前首版需要验收的事实在单一当前合同中可表达。弃用字段应直接删除或重塑，不以兼容回退掩盖缺失状态。

## 首轮阻断项

- **B1：原生生命周期 C/D 已通过，但原生交互仍未闭环。** `docs/verification/stage-4.md` 已证明运行中关闭后任务继续并完成、同 PID 恢复窗口和 session，以及显式“停止并退出”只清理托管资源并保留中断 session、队列和工作目录；中文输入法、滚轮/触控板、物理键盘、视觉和设置持久化仍未验收。
- **B2：controller/application 已有部分 S1/S2 协议替身证据。** 普通测试和定向 race 覆盖纠正/FIFO/取消失败/旧回复/重启/跨 session 停止，阶段 2/3 覆盖文件工具、历史重载和三个 session；真实 API、真实厂商错误、取消 acknowledgment、授权/提问全链路和 S2 边界仍未闭合。
- **B3：appmodel 基础消息状态和运行配置快照已补入，但请求失效事实、取消失败细分、回答历史、schema/version 和完整事件重放仍无法完整审计。** 这部分不能再以“完全缺少消息状态”表述作为依据。
- **B4：真实 API、原生交互和目标平台门槛仍未通过。** provider/application 的外部服务仍是本地协议替身；PTY 的 shell/cwd/input/Unicode/resize/exit/close 主要路径已有最新记录，长会话代码级虚拟化/锚点也已通过，但滚轮/复制、物理键盘、IME 和 GCode 原生窗口最终交互尚缺，Apple Silicon/Windows 没有记录。
- **B5：worktree 内核已有实现和测试，但桌面产品入口缺失。** 新 Manager/controller 已覆盖独立分支、源目录未提交内容不搬运、Bind/重启、脏/占用删除和合并；旧 `Prepare/Clean`/detached 入口已经删除，但 desktop 没有创建、分支、合并、删除 UI。
- **B6：扩展本地应用层 S2 已通过，外部边界仍未闭合。** `docs/verification/stage-5-extensions.md` 已覆盖 plugin/hook/skill/MCP/specialist 的真实子进程、controller 授权、结果历史、错误/warning 脱敏、取消和 MCP 退出清理；真实厂商 API/OAuth、原生扩展配置界面、目标设备、不可写目录和含密钥错误的独立 S2 验证仍待。
- **B7：当前自动化全量回归已通过，阶段场景证据仍未闭环。** 最新 `output/acceptance/visual-v2/regression/` 记录全仓 test/vet、关键包 race、desktop race、fmt-check、应用包构建与签名验证全部 exit 0，覆盖本轮取消发布竞态、来源提示裁剪和最终 Diff 修复。旧 `stage-5/final-20261012/` 仅代表此前源码；阶段 2—4 完整场景复核和真实设备验收仍未闭环。

## 2026-10-12 设计/实现核对

本轮只读核对设计第 10 节、阶段 4—5 实现和已有验证记录，结论如下：

1. Markdown 基础能力已经落地：代码围栏、标题、列表、引用、粗斜体、行内代码、Chroma 高亮、选择和复制均可在 `internal/desktop/markdown.go` 找到；`docs/verification/stage-4-conversation.md` 已证明消息区使用 `ui.ListState`/`ui.List` 做可视范围和 overscan，并通过底部跟随、上滑保持、独立会话位置和历史 prepend 锚点测试。原生滚轮/触控板/IME/controller 事件仍待，因此 A13/P4.1 仍是部分。
2. 文件预览/Diff 已形成 `workspace`→`application.Workspace`→右面板链路，覆盖当前 cwd、只读/二进制/截断预览、Git status/Diff、非 Git 目录和外部编辑器；路径边界与嵌套 Git 有 S1/S2 本地临时项目证据，真实窗口切换仍待。
3. Worktree Manager/controller 的独立分支、源目录未提交内容不搬运、Bind/重启、脏/占用删除保护和合并/删除均有实现与测试；旧 `Prepare/Clean`/detached 入口已删除，但 desktop `Service`、新会话和设置页仍没有 worktree 操作入口。
4. 扩展注册路径不是完全静默：plugin/hook diagnostics、skill duplicate、MCP skipped 和 specialist Load/manifest warning 会上报脱敏 warning，MCP、specialist、swarm 和 execution 有 defer 清理；`docs/verification/stage-5-extensions.md` 已补入真实 application/controller/runtime 链路的本地应用层 S2，仍需真实 API/OAuth、原生配置界面、桌面回调和目标设备场景。
5. 终端工厂、Darwin 非阻塞 PTY 补丁和包内 `libghostty-vt.dylib` 已存在；最新 terminal QA 已通过 shell/cwd/input/CJK/resize/exit/close 主要路径，mygo patched/Unicode 测试也通过。滚动/复制、物理键盘和 GCode 原生窗口交互仍待，因此 P4.3/A14 保持部分。
6. 原生生命周期 C/D 已有真实证据：`docs/verification/stage-4.md` 记录了运行中窗口关闭后后台完成、同 PID 重开、取消确认继续运行和显式退出清理托管资源并保留 session；这不覆盖 IME、滚轮/触控板、物理键盘、视觉 v2、设置持久化或多平台。
7. 许可证材料和 Ghostty 资源来源已闭环：21 份许可证有原文映射，`native-provenance/verification.txt` 证明 manifest 原始下载经实际 ad hoc 签名后与当前包内 dylib 逐字节一致，计划 1.1 已勾选。后续只需在签名参数或构建链改变时重做 provenance 复核。
8. `docs/verification/visual-v2.md` 已登记最终 closure 七张暗黑/浅色/宽窄/设置原生图，当前源码、binary 与截图 hash 一致；AX 中文/emoji 输入读回、短 Diff 折叠跨重绘保持与正常关闭 exit 0 均通过。独立复核确认短 Diff 自然高度与窄窗品牌两项缺陷均 resolved，technical passed、readiness user_review；设计文档/sidecar 仍为候选，用户视觉评价和物理键盘/IME/滚轮/复制仍待。
9. `docs/verification/source-closure.md` 已完成独立 P1.5 复核：桌面编译依赖不引入 CLI/TUI/订阅 provider OAuth，四类 factory 均 API Key，标准 OAuth 仅为 MCP 保留；旧 CLI 提示已清理，计划 1.5 可勾选。P5.1 仍缺四类真实 API S2。

## 后续审计顺序

1. 使用 `output/acceptance/visual-v2/regression/` 作为当前源码的自动化回归与应用包构建证据；继续登记阶段 2—4 定向场景。旧 `stage-5/final-20261012/` 和更早的失败日志分别作为历史源码结果保存。
2. controller/session-runtime 补齐 runner 忽略取消、真实流式/长命令/权限/问题、迟到回调、不可写持久化目录和含密钥错误脱敏场景；现有 `persistErr`/redaction 静态处理需要 S2 证据。
3. 第二批视觉图已齐，先取得用户视觉评价；视觉认可后扩展 worktree 创建/分支/合并/删除和其余管理页面。独立补齐外部 editor/Diff、长会话滚轮/触控板/IME/焦点/标题栏等原生行为证据，避免重复主观打磨和截图循环。
4. 终端主要 PTY 路径已经通过；继续补滚动/复制、物理键盘和 GCode 原生窗口交互记录，并在签名参数变化时复核 manifest 原始 dylib 到分发包的 provenance。
5. api-providers/root 逐类记录四种真实 API 请求，覆盖文本、工具调用、取消、错误、历史上下文和下一轮模型切换；httptest 只能标 S1。
6. 在本地应用层 S2 记录基础上，审计 MCP、Skill、Hook、插件、specialist/swarm 子代理的真实 API/OAuth、原生配置界面、目标设备、含密钥错误和不可写目录清理。
7. 最后在 macOS Intel 当前机器闭环应用包、最终全量回归和性能采样，再分别记录 Apple Silicon/Windows 的构建、原生运行、终端资源和未验证项；许可证声明材料已完成，计划 1.1 保持已勾选。

## 当前结论

截至 2026-10-12，GCode 已有可启动入口、macOS Intel 生命周期 C/D、主要 PTY shell、代码级长会话虚拟化/阅读锚点、协议替身应用编码链路和三个 session 资源归属证据；文件/Diff、worktree 内核、来源裁剪/许可证、本地应用层扩展 S2、视觉 v2 第二批原生候选及当前自动化回归也已落地。首版仍未通过：用户视觉评价、原生 IME/物理键盘/滚轮复制、worktree 桌面入口、真实厂商 API/MCP OAuth、阶段 2—4 完整场景复核、性能与 Apple Silicon/Windows 仍未闭环。
