# GCode 产品综合审计报告（2026-11-05）

> 本页保留修复前的审计材料，不作为当前缺陷清单。2026-11-05 已按源码逐条复核并
> 完成不改变既定功能的修复，当前状态、误报纠正、验证结果和待决策的功能影响见
> [核验后修复与决策](audit-repairs-2026-11-05.md)。
> 特别注意：本文 architecture-2、architecture-4、concurrency-3 不符合核验时的代码；
> concurrency-5 本轮未复现；concurrency-4 的竞争风险被低估。

- 审计对象：GCode 原生 Go 桌面应用（MyGo UI + 同进程强类型业务核心）与无桌面服务 `cmd/gcode-server`；模型协议支持 OpenAI Chat Completions / OpenAI Responses / Anthropic Messages，供应商完全由用户自定义。项目处于开发阶段。
- 审计方式：安全、性能、UI 一致性、并发正确性、健壮性、架构与边界六个领域专项审计，逐条发现由独立复核员复核（详见第 2 节）。
- 发现合计：31 条（已确认 15 条：14 medium + 1 low；待人工复核 16 条：均为 low）。无 high 严重度发现。
- 报告撰写说明：本报告基于六份领域审计材料与逐条复核结论整理；撰写时在同一工作树复跑了 `go vet ./...`、`go build ./...` 与 `go vet ./internal/... ./cmd/...`（结果见附录 A），并抽查了部分关键行号（与复核员修正一致）。除本文件外未改动任何其他文件。

---

## 1. 执行摘要

**总体健康度：较好。** 六个领域均未发现 high 严重度问题。安全域未发现命令注入与路径穿越；gcode-server 全部端点都在常量时间 Bearer 校验之后且无 CORS 响应头，安全域审计据此未发现浏览器侧 CSRF/DNS-rebinding 缺口——跨域或被重绑页面发出的请求不携带 Bearer token、无法通过认证（本仓无 Host/Origin 类校验，防护完全依赖认证前置；该结论出自安全域静态审计，Host/Origin 防线的逐项证据未单独列示）。文件操作普遍经 `os.Root`/`IsLocal`/`EvalSymlinks` 约束（例外见 security-2：绝对路径链路仅 EvalSymlinks 后 `os.Open`、无范围限制），日志与错误文本对凭据有系统性红化。并发域未发现可证实的数据竞争或死锁（`-race` 因本机无 C 编译器未能运行，见第 2 节未覆盖项）。架构边界成立：`internal/desktop` 生产代码不导入 engine/remote/mock（`go list -deps` 证实），UI 经 `contracts.Service` 强类型接口同进程直调，`cmd/gcode-server` 无 MyGo/桌面依赖。持久化统一 0600 临时文件 + fsync + 原子替换。已有性能调优（`docs/memory-tuning.md`：启动后 FreeOSMemory、媒体 LRU、壁纸缩略图）本次未重复报告。

风险集中在三个主题：

1. **部署与凭据暴露面（安全）**：gcode-server 非回环监听默认明文 HTTP 承载 Bearer token、会话内容与 provider 明文 API key（security-1）；模型/网页内容中的 `file:`/绝对路径链接可一键把含全部明文 API key 的 `engine.json` 读入 UI（security-2）；key/token 明文落盘（security-3，low）。
2. **引擎关键路径上的重活（性能/并发）**：`e.mu` 是全部命令、快照、事件发布的唯一入口，而锁内承担了会话全量重序列化 + fsync（performance-1）、非 delta 事件的整份快照克隆×订阅者数（performance-2）、每条流式 token 的两次取锁 + 线性扫描（performance-3）；另有流式文本 `+=` 的 O(n²) 累加（performance-4）、锁内可阻塞的终端进程 IO（concurrency-1）、以及不属于锁问题但同处关键关停路径的窗口主线程同步 join 引擎全量关停（concurrency-2）。
3. **UI 规范债务与本地化缺口（UI）**：共享 `iconButton`/`textButton` 被业务代码逐处硬编码覆盖（ui-1、ui-3）、供应商表单文案未走 i18n（ui-2）、链接 tooltip 硬编码深色配色（ui-4）。goal/todo 新面板本身正确进入了共享定义；未接入共享定义的自绘/重复模式（ui-5~ui-7）属遗留迁移范围，仅作低优先级标注（两类区分标准见 4.3 导语）。

另有一条影响可用性的独立问题：modelcatalog 缓存文件损坏会让桌面端与 headless 服务整机拒绝启动而非回退内置种子数据（robustness-1）。

**最需要优先关注的 5 件事**（分组与第 5 节 P1 对应，个别组做了合并展示）：

1. 对 gcode-server 非回环监听强制 TLS（或至少启动日志明确告警明文传输风险），并将 `GET /v1/provider-key` 改为掩码返回（security-1；P1-1）；
2. 将聊天/文档链接目标限制在会话 CWD 与注册项目之内，或对 `file:`/绝对路径目标要求用户显式确认，并把凭据文件加入读取拒绝列表（security-2；P1-1）；
3. modelcatalog 缓存 decode 失败时回退嵌入 seed 并重写缓存，不让可再生缓存阻断启动（robustness-1；P1-2）；
4. 拆解引擎关键路径上的重活：`e.mu` 锁内的 fsync、快照克隆与流式扫描（performance-1/2/3），终端 IO 与窗口关停移出关键路径（concurrency-1/2），流式 `+=` 改 Builder（performance-4）（P1-3/4）；
5. 单批清理共享控件硬编码覆盖与 i18n 缺口（ui-1~4），并处理 contracts 裸字符串 Kind（architecture-1）（P1-5/6）。

---

## 2. 审计方法与覆盖范围

### 2.1 六个领域的覆盖范围

| 领域 | 覆盖范围摘要 | 检查方式与执行情况 |
| --- | --- | --- |
| 安全 | `cmd/gcode-server`、`internal/remote`、`internal/mcp`、`internal/processutil`、`internal/storage`、`internal/agent`、`internal/workbench`、`internal/repohost` 及 engine/model 的凭据处理、工具执行、认证边界、外部内容注入面与危险默认值 | 只读静态分析 + 全库 grep 交叉排查（监听地址、CORS 响应头、日志泄露点、文件权限位）；未运行任何测试或构建 |
| 性能 | 先读 `docs/memory-tuning.md` 确认已调优项避免重复报告；逐行通读 `internal/model` 流式解码、`internal/engine` 事件发布/持久化/运行循环、`internal/storage` 写盘、`internal/desktop` 事件应用、`internal/agent` 工具循环；goroutine 泄漏排查与正则热路径盘点 | 逐行通读 + 排查清单；未运行基准测试（见 2.4），个别量级数据来自复核时的合成微基准（附录 B） |
| UI 一致性 | 依据 `docs/ui-controls.md` 与 AGENTS.md「Shared UI controls」；共享外框调用点、共享控件覆盖、硬编码颜色（`ui.Hex`）、key/label 定位、i18n 键存在性、图标清单、三处 RoleTooltip 主题化程度对比 | `go test ./internal/desktop -run 'Test(SettingsControls\|InspectSettingsControls)' -count=1` 通过；`go test ./internal/desktop -run 'TestSettingsPage' -count=1` 通过；其余为代码层面检查 |
| 并发正确性 | `internal/engine`、`internal/agent`、`internal/model`、`internal/remote`、`internal/desktop`、`internal/processutil`、`internal/terminalhost` 的全部 goroutine 产生点与共享状态写者逐一交叉验证 | `go vet ./internal/...` 通过；`go test -count=1 ./internal/{agent,model,remote,terminalhost}/...` agent/model/remote 通过、terminalhost 一用例连续两次失败（见 concurrency-5）；`go test -count=1 ./internal/engine/...` 通过（78.5s）；`-race` 未能运行（见 2.4） |
| 健壮性 | 错误吞噬、panic 风险、资源泄漏、持久化完整性、超时与重试五个维度，通读 17+ 包核心实现 | `go vet ./internal/... ./cmd/...` 通过；`go build ./internal/... ./cmd/...` 通过（EXIT=0） |
| 架构与边界 | 传输边界（`go list -deps` 包依赖方向）、分层（engine/contracts 零 MyGo 依赖）、contracts 类型规范、死代码全仓核实、mock 引用链、6 份文档约 15 项关键声明核对 | `go list -deps`、grep、死代码扫描；`go vet ./internal/contracts ./internal/engine ./internal/desktop` 通过 |

观察项（不作为缺陷，两条性质不同，分列）：

- agent 文件/Shell 工具的整盘读写属编码代理的设计边界（定性说明，非缺陷）。
- 预留的 ToolGuard 扩展点在 `cmd/rcode` 与 `cmd/gcode-server` 两处入口均未启用——这是当前状态事实；属预期留白还是待启用，审计材料未给出判断，需团队确认。

### 2.2 逐条独立复核机制

每条发现由独立复核员对 evidence 逐行重放（读源码、必要时运行测试或临时实验），给出 verdict。正文一律按复核结论呈现：复核员对初报的**纠正**（行号、频率、定性）以「复核修正」标注，**补充**的缓解/加重因素以「复核补充」标注。

- **status=verified（已确认）**：复核成立。
- **status=unconfirmed（待人工复核）**：按流程未做逐条独立复核。本次 16 条 unconfirmed 全部为 low，复核员统一结论为「低严重度发现，按流程未做独立复核，建议人工抽查」。

复核流程说明（依审计材料可确认的范围）：

- **准入口径**：31 条中 15 条完成逐条独立复核——覆盖全部 14 条 medium 与 1 条 low（performance-5，因其初报的频率描述与严重程度需对照实现核实）。其余 16 条 low 未逐条复核。送审的具体规则（谁决定、按什么标准挑选）审计材料未记录，本报告不作推断；可确认的口径是「medium 全部复核，未复核者均为 low」。
- **否决条目处置**：审计材料未记录任何被复核员否决而删除的发现，本报告无法核实其是否存在；本报告仅呈现材料中实际出现的 31 条。
- **严重程度判级**：报告未附书面 rubric。各条目判词（「medium 恰当」「维持 low」）逐条来自领域审计与复核员，理由写在各条目的复核状态内（如 robustness-1 不到 high 的原因是原子写使半写概率受限），边界可逐条倒查。

### 2.3 静态检查

`go build ./...` 与 `go vet ./...` 在审计期间均为退出码 1，错误全部来自 `output/desktop-e2e` 下历史 e2e 运行产物目录（14 条 `malformed import path`），未报告任何源码级问题；范围限定 `./internal/... ./cmd/...` 时两者均通过、无输出。输出原文、撰写时复跑结果与成因说明见附录 A。

### 2.4 未覆盖项（明确声明）

- **完整 go test 套件未运行**：审计期间只运行了 2.1 表中列出的范围限定测试，未运行全量测试套件、desktop e2e 套件或 Makefile 全量目标。
- **`-race` 数据竞争检测未运行**：`CGO_ENABLED=1 go test -race ./internal/{agent,model,remote,terminalhost}/...` 因本机（Windows）PATH 无 C 编译器（cgo 不可用）无法执行；并发结论仅以 `go vet` 与普通 `go test` 佐证。
- **运行时性能剖析未做**：无 CPU/内存 profile、无真实负载基准；性能结论来自逐行代码阅读与少量合成微基准。
- **真实渲染像素/视觉一致性未评估**：UI 域仅做代码层面检查（控件来源、颜色硬编码、i18n 键），未做截图对比或像素级评估；按项目规范，单页截图或编译通过也不能代替跨页面组件一致性检查。
- **安全域为只读静态分析**：未运行任何测试或构建，未做动态验证（渗透测试、模糊测试）。
- **工作树并发变更**：审计期间工作树仍在被修改（`goal.edit` 从 `ExpectedObjective` 改为 `ExpectedRevision` 机制），行号以最终核验时的文件状态为准。

---

## 3. 风险矩阵

| 领域 | medium（已确认） | low（已确认） | low（待人工复核） | 合计 |
| --- | :---: | :---: | :---: | :---: |
| 安全 | 2 | 0 | 1 | 3 |
| 性能 | 4 | 1 | 3 | 8 |
| UI 一致性 | 4 | 0 | 3 | 7 |
| 并发正确性 | 2 | 0 | 3 | 5 |
| 健壮性 | 1 | 0 | 3 | 4 |
| 架构与边界 | 1 | 0 | 3 | 4 |
| **合计** | **14** | **1** | **16** | **31** |

说明：本次无 high 严重度发现；「待人工复核」条目均为 low，修复决策前应先完成人工复核（见第 5 节 P2）；medium/low 的判级依据说明见 2.2。

---

## 4. 分领域详细发现

### 4.1 安全（3 条）

安全域结论总体良好：未发现命令注入、路径穿越与 CORS/CSRF 缺口，日志与错误文本对凭据有系统性红化；剩余风险集中在部署传输（security-1）、诱导式本地读取（security-2）与凭据落盘形态（security-3）。ToolGuard 观察项见 2.1 末尾。

#### security-1 · gcode-server 非回环监听默认明文 HTTP（medium · 已确认）

- **位置**：`cmd/gcode-server/main.go:122`（另见 `internal/remote/server.go:141-146`、`internal/remote/provider_keys.go:17-30`、`internal/appconfig/connection.go:71-79`）
- **问题**：无桌面服务默认明文 HTTP 承载 Bearer token 与明文 provider API key，非回环监听无 TLS 强制或告警。
- **证据**：`main.go:49` 默认 `--listen 127.0.0.1:27655`；TLS 证书须成对提供（main.go:116-118），否则 `server.Serve(listener)`（main.go:119-123，122 行）以纯 HTTP 服务；启动日志（main.go:115）只打印地址，全代码不存在任何 loopback/明文风险告警。认证仅为 `Authorization: Bearer` 常量时间比较（`internal/remote/server.go:141-146`；token 由 main.go:73 `rand.Text()+rand.Text()` 强随机生成）。`GET /v1/provider-key` 原样返回供应商明文 key（`internal/remote/provider_keys.go:17-30`、`internal/engine/provider_keys.go:18-22`）。客户端侧 `internal/appconfig/connection.go:71-79` 仅校验 scheme 为 http/https，允许 `http://`，token 与会话内容经同一明文通道。AGENTS.md 明确要求桌面端和移动端连接该服务。
- **严重程度**：medium
- **复核状态**：已确认——复核成立。复核补充：缓解因素为默认回环监听开箱安全、非回环暴露需用户显式 `--listen`、`docs/backend-v1.md:249-252` 已文档化隧道/反代/TLS 用法并警告勿经未加密公网 HTTP 传 token（:303 注明公网/TLS 尚非已验证环境）；但非回环部署是产品明确支持的预期用法而代码零强制、零告警，且 `/v1/provider-key` 明文端点放大泄露影响，medium 恰当。
- **修复建议**：对非回环监听地址强制要求 TLS（或至少在启动日志明确告警明文传输风险）；`/v1/provider-key` 改为掩码返回、编辑密钥时单独确认。实施注意：`docs/backend-v1.md:249-252` 已文档化经隧道/反代/TLS 的部署方式，「硬失败」式强制会与之冲突，需为「经加密隧道访问本机端口」的场景提供显式豁免（如环境变量确认），并另行设计移动端/第三方客户端的证书信任路径；若暂不具备条件，至少先落地启动告警。

#### security-2 · `file:`/绝对路径聊天链接可一键读取本地凭据文件（medium · 已确认）

- **位置**：`internal/engine/linked_files.go:124`（另见 `internal/linktarget/target.go:84-99`、`internal/desktop/transcript_links.go:77-84`、`internal/workbench/links.go:86-135`）
- **问题**：模型/网页内容中的 `file:` 或绝对路径聊天链接可在 UI 一键读取任意本地文件（含保存全部明文 API key 的 `engine.json`）。
- **证据**：`linktarget.Parse` 非 preview 场景接受 `file:` 绝对路径（`internal/linktarget/target.go:84-99`，`target_test.go:20-21` 锁定该行为）；桌面转录/工作区点击链路 openTranscriptLink→OpenLink→openLinkTarget→ReadLinkedFile；ReadLinkedFile 对绝对路径仅 `filepath.EvalSymlinks` 后 `os.Open`（`internal/engine/linked_files.go:78-124`），无 CWD/项目范围限制（相对路径才有 IsLocal+`os.Root` 约束，linked_files.go:81-84、114-122）。`~/.gcode/engine.json` 保存含明文 `apiKey` 的全部 providers（`internal/engine/session_storage.go:274-275`、`internal/model/model.go:110`），一次点击即可将其读入文档视图。
- **严重程度**：medium
- **复核状态**：已确认——核心链路属实。复核修正：初报「作为 file.read 事件广播给已连接客户端」不成立——`file.read` 仅在 `file.ProjectID != ""` 时发布（`linked_files.go:188-209`，已抽查核实），项目外的 engine.json 不会发布该事件、不进 state.Files，内容只返回给点击的那个客户端 UI，实际泄漏面比初报小；且模型自带 Read 工具本就支持绝对路径任意读（`internal/agent/read.go:74-77`），项目外读取在 `linked_files_test.go:61-63` 是被测试锁定的有意设计。问题性质是「无确认的诱导式凭据暴露 + 缺少凭据拒绝列表」而非新增读取能力，medium 恰当。
- **修复建议**：将聊天/文档链接目标限制为会话 CWD 与注册项目路径之内，或对 `file:`/绝对路径目标要求用户显式确认；把凭据文件（`engine.json`、`connection.json`、`server.json`）加入读取拒绝列表。实施注意：项目外读取是 `linked_files_test.go:61-63` 锁定的有意设计，上述限制实质上调整该设计意图，应先在团队层面确认变更，再同步修改该测试。

#### security-3 · key/token 明文落盘（low · 待人工复核）

- **位置**：`internal/engine/session_storage.go:275`（另见 `internal/appconfig/connection.go:18-22,80`、`cmd/gcode-server/main.go:63-75`）
- **问题**：供应商 API key、远程连接 token、服务 token 均以明文 JSON 落盘，未接入 OS 凭据存储。
- **证据**：`engine.json` 持久化 `Providers: e.providers`（`internal/engine/session_storage.go:274-275`），`Provider.APIKey` 为明文字段（`internal/model/model.go:110`）；远程连接 token 明文存 `connection.json`；服务 token 明文存 `server.json`（`cmd/gcode-server/main.go:63-75`）。缓解因素：所有配置写入经 `os.CreateTemp` 0600 + fsync + 原子替换（`internal/storage/storage.go:556-579`），配置目录 0700（storage.go:85），Windows 依赖用户目录 ACL 继承；Snapshot 输出已剥离 key（`internal/engine/engine.go:323` `public.APIKey = ""`）。
- **严重程度**：low
- **复核状态**：待人工复核——低严重度发现，按流程未做独立复核（复核员统一结论），建议人工抽查。
- **修复建议**：接入 OS 级凭据存储（如 Windows DPAPI、macOS Keychain、libsecret）保存 key/token；若保留明文落盘，至少在文档中说明该威胁模型与机器本地攻击者的边界。

### 4.2 性能（8 条）

性能域未发现 goroutine 泄漏与未关闭的 HTTP body，transcript 渲染已有行级缓存与指纹复用；`docs/memory-tuning.md` 已调优项不重复报告。主要隐患集中在引擎全局锁内的持久化/克隆/扫描（performance-1/2/3）与流式累加（performance-4），其余为局部浪费。

#### performance-1 · persistLocked 在全局锁内全量序列化 + fsync（medium · 已确认）

- **位置**：`internal/engine/session_storage.go:180`
- **问题**：persistLocked 在全局引擎锁 `e.mu` 内对每个已加载会话组装 `sessionDisk` 并整体 `json.MarshalIndent`（含 fsync），工具循环每条结果触发一次，阻塞同锁上的所有事件与流式 token。其中 `History` 与 `Context` 是同一对话的两份全量消息数组，每次都完整编码——初报「重序列化两次」即指这一对全量拷贝；`Messages` 为另一份会话消息列表，随同一 MarshalIndent 一并编码（`session_storage.go:184-217` 已核实组装点）。
- **证据**：sessionDisk 组装 `Messages: e.state.Messages[...]`、`History: e.history[...]`、`Context: e.context[...]` 后整体 `json.MarshalIndent(disk, "", "  ")`（session_storage.go:184-217），未变更也要先付完整编码成本才能算 sha256（:221-224）；命中变更经 `WriteJSONRoot` fsync（`internal/storage/root_json.go:45`），每次还无条件重写 `engine.json`（session_storage.go:274-276，`storage.WriteJSON` 内部 MarshalIndent + `f.Sync()`，`internal/storage/storage.go:574`）；`writeSessionRecord` 触发 cleanSessionRecords 重读旧记录算哈希（session_storage.go:354-369）。调用频率：工具循环每条结果（`run.go:613-623` Record、`tool_loop.go:88-92`、`agent.go:303`）、每次模型尝试（`model_requests.go:48-66`）、goal 心跳每 15 秒。流式 token 的 Emit 回调同样需要这把锁（`run.go:652-656`、878）。
- **严重程度**：medium
- **复核状态**：已确认——复核成立。复核补充：合成微基准显示单次编码成本随会话长度显著增长（6MB 约 30ms、24MB 约 98ms，未含 fsync；两点间略低于线性外推，增长规律未基准化）；「随轮内工具数增加调用频率」为代码阅读结论（调用点见证据），该维度未做基准。复核修正：goal 心跳实际在 `goal.go:544-569`（初报 519 为 parseGoalVerification），`WriteJSON` 自身的 Sync 在 `storage.go:574`。不影响结论。
- **修复建议**：为会话记录引入脏标记，只序列化本轮确实变化的会话；History/Context 只编码一份复用哈希结果，或把记录写入移出 `e.mu`（先在锁内快照再异步落盘）；`engine.json` 加内容哈希跳过未变更重写。

#### performance-2 · 非 delta 事件整份快照克隆，且每订阅者再克隆（medium · 已确认）

- **位置**：`internal/engine/engine.go:363`
- **问题**：所有不带 Message 的事件（message.delta 豁免）都发布整份快照，且每个订阅者再克隆一次，全程在引擎锁内。
- **证据**：publishLocked 对 `event.Message == nil` 且非 message.delta 的事件执行 `event.Snapshot = &snapshot`（engine.go:363-366），随后对每个订阅者 `contracts.CloneSnapshot`（engine.go:373-376），慢订阅者恢复时第三次 snapshotLocked（engine.go:385-390）。snapshotLocked = CloneSnapshot(e.state) + 对所有会话所有消息跑 projectMessageTodos（`internal/engine/todo.go:64-73`），深克隆全部消息 Blocks、diff hunks/lines、Runs、Queue（`internal/contracts/clone.go:52-59,88-93`）。触发频率：`model.request.recorded` 每轮模型请求一次（`model_requests.go:64`）、goal 心跳每 15 秒、run.started/finished、file.read、queue.changed 等；远程 SSE 客户端对每个事件 json.Marshal 整份快照（`internal/remote/server.go:127`）。
- **严重程度**：medium
- **复核状态**：已确认——复核成立，medium 恰当（可扩展性债务而非正确性缺陷：成本随转录规模线性增长并在远程路径按事件次数放大；字符串字节刻意共享、流式 delta 已豁免、带 Message 的事件走更轻的按消息克隆，短会话影响有限）。复核修正：goal 心跳 ticker 在 `goal.go:548`、发布在 `goal.go:570`（初报 528 有偏）；初报标题曾写「所有不带 Message 的事件」而忽略 message.delta 豁免，正文已按复核结论修正。
- **修复建议**：把快照事件改为带变更范围的增量事件，或至少让所有订阅者共享同一份克隆结果、仅对慢消费者单独重建；goal 心跳等周期事件改发轻量增量。

#### performance-3 · 流式 token 每条两次取全局锁 + 两次线性扫描（medium · 已确认）

- **位置**：`internal/engine/model_requests.go:30`
- **问题**：每个流式内容 delta 串行两次获取同一把 `e.mu` 并做两次线性扫描，与 fsync/快照克隆同锁竞争。
- **证据**：meteredModel.Stream 的 project 回调对每个内容 delta 调 `m.updateRetry(nil)`（model_requests.go:30-32）；updateRetry 先 `m.core.mu.Lock()`（:73）再 `e.sessionIndex` 线性扫描全部会话（:76，`engine.go:394-401` 确认为线性扫描），之后才早退（:81-83）；emit 链继续到 applyAgentEventLocked 又一次拿 `e.mu` 并从 `messages[0]` 线性扫到位于列表末尾的流式消息（`run.go:830-839`）。persistLocked（含 fsync）或快照克隆持锁时，token 投递被整体阻塞。
- **严重程度**：medium
- **复核状态**：已确认——复核成立，medium 恰当（无正确性影响，单次开销微秒级，仅在高 delta 速率、大会话与频繁 persist 时可感知）。复核修正两点：message.delta 事件本身是轻量增量事件——发布在 `run.go:878`、该分支随后 `return nil`（run.go:880），不走 CloneBlocks（初报两处行号分别指发布行与返回行，本报告已统一表述），per-delta 成本比初报暗示的略低；persist 阻塞流式的根因是 `e.mu` 全局串行化本身，updateRetry 只是叠加成本。
- **修复建议**：updateRetry 先判断 `run.id != m.turnID` 再决定是否拿锁与做 sessionIndex——注意初报「先无锁判断」会引入 `session.ModelRetry` 读侧数据竞争，故须以此为前置条件；为 `*current` 建立 ID→index 缓存，避免 applyAgentEventLocked 全量扫描；每 delta 的锁粒度收敛到单一入口。

#### performance-4 · 流式文本/工具参数 `+=` 平方累加（medium · 已确认）

- **位置**：`internal/model/stream.go:53`（另见 `internal/engine/run.go:877`、`internal/model/chat.go:192-193`、`internal/desktop/app.go:460`、`internal/model/messages.go:203-207`）
- **问题**：流式文本与工具参数用 `+=` 累加，长回复 O(n²) 分配，且模型层与展示层各累加一份；全项目共五处平方累加点。
- **证据**：初报三处——streamState.add：`s.result.Message.Text += delta.Text`、Reasoning 同理（`internal/model/stream.go:53-54`）；展示层 `run.go:877` `message.Blocks[block].Text += part.text`；工具参数 `model/chat.go:192-193` `call.Name +=` / `call.Arguments +=`。历史最终经 Record 线性写入（`run.go:613-617`），三处副本中两处走平方路径。复核修正另发现两处：桌面 UI 层 `app.go:460`，Anthropic 适配器 `model/messages.go:203-207`（对文本和工具参数；Responses 适配器无此问题）。
- **严重程度**：medium
- **复核状态**：已确认——复核成立并扩大范围：初报三处之外另发现 `app.go:460` 与 `model/messages.go:203-207` 两处，合计五处（run.go 行号初报 876，实为 877，1 行漂移）。实测 100KB 回复的 `+=` 累计分配 27~212MB（strings.Builder 对照 0.5MB），「数十 MB 量级」判断准确，medium 恰当。
- **修复建议**：streamState 与展示块改用 `strings.Builder` 或 `[]byte` 累加，结束时再转字符串；tool-call 参数片在解码器内先拼 `[]byte`（覆盖 chat.go 与 Anthropic 适配器）。

#### performance-5 · tool/message 事件克隆整条消息 Blocks 三次（low · 已确认）

- **位置**：`internal/engine/run.go:929`
- **问题**：每个 tool/message 事件在发布处、每订阅者、UI 端各克隆一次整条消息 Blocks。
- **证据**：applyAgentEventLocked 事件末尾 `changed := *message; changed.Blocks = contracts.CloneBlocks(message.Blocks)`（run.go:928-930）；publishLocked 内每订阅者再 CloneBlocks（`engine.go:377-381`）；桌面端 applyEvent 收到后又 CloneBlocks（`app.go:442`，初报 441 差一行）。CloneBlocks 对每块克隆结构体并重建 hunks/Lines/Pages 切片（`internal/contracts/clone.go:123-179`）。
- **严重程度**：low
- **复核状态**：已确认——三处克隆属实。复核修正：初报「每次流式更新」的频率描述有误——`tool.input.changed` 仅在 hook 返回 UpdatedInput 时触发（`hooks.go:553-558`，每次工具调用至多 2 次且需配置 hook）；逐 token 流式走 message.delta（`run.go:878`）不带 Message、完全不触发 Blocks 克隆。实际三重克隆仅发生在每工具调用约 2-4 个事件上（3 处克隆点 × 约 2 个事件 ≈ 6 份；事件数 2–4 时为 6–12 份）；合成基准显示重度形态单次克隆约 42µs/207KB，单工具调用约 6 份克隆 ~0.25ms，相对工具执行本身占比很小，维持 low。
- **修复建议**：事件只携带被修改的单块（BlockID + 新值）让 UI 打补丁；或发布处只克隆一次、订阅者共享（订阅者本地不可变），UI 端信任事件数据免克隆。可与 performance-2 的快照共享化一并处理。

#### performance-6 · 每条 message.delta 触发 workspace/bottom 全量对账（low · 待人工复核）

- **位置**：`internal/desktop/app.go:468`
- **问题**：applyEvent 对 message.delta 分支末尾固定调用 `a.workspace.Update(s); a.bottom.Update(s)`（无事件类型区分）；workspace.Update 遍历 tabs×Sessions 找页面标题、对每个打开文档内层遍历 snapshot.Files、再对账全部 Terminals（`internal/workbench/workbench.go:130-151`），项目文件数达数千、delta 高频时是渲染线程上每 token 的固定线性成本。
- **严重程度**：low
- **复核状态**：待人工复核——低严重度发现，按流程未做独立复核（复核员统一结论），建议人工抽查。
- **修复建议**：对 message.delta 跳过与对话无关的对账（tabs/documents/files/terminals），或 workbench.Update 记录上次处理的 Files/Terminals 版本号做短路。

#### performance-7 · hookWildcardMatches 每次匹配重新编译正则（low · 待人工复核）

- **位置**：`internal/agent/hook_condition.go:191`
- **问题**：每次调用执行 `strings.ReplaceAll` + `regexp.Compile`（hook_condition.go:190-192）；调用点在 hookConditionMatches 内按规则×Bash 子命令循环（:95-108），hookedTool.Execute 每次工具调用触发多轮匹配（`internal/agent/hooks.go:522-560,569`）。规则少时代价可控，但每次重复编译同一模式属纯浪费。
- **严重程度**：low
- **复核状态**：待人工复核——低严重度发现，按流程未做独立复核（复核员统一结论），建议人工抽查。
- **修复建议**：在 hook 规则加载/推送时预编译通配模式并缓存（如 rules 结构里带 `*regexp.Regexp`），匹配路径直接查缓存。

#### performance-8 · 每次模型请求尝试整体拷贝请求体（low · 待人工复核）

- **位置**：`internal/model/client.go:287`（已抽查核实：`strings.NewReader(string(body))`）
- **问题**：streamAttempt 中 `http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))`——body 只编码一次，但 `string(body)` 在每次尝试（含重试）都完整拷贝；长对话请求体可达数百 KB。`client.go:24-31` 已 Clone Transport 说明请求体大小是真实场景。
- **严重程度**：low
- **复核状态**：待人工复核——低严重度发现，按流程未做独立复核（复核员统一结论），建议人工抽查。
- **修复建议**：改为 `bytes.NewReader(body)`，零拷贝且语义相同。

### 4.3 UI 一致性（7 条）

依据 `docs/ui-controls.md` 与 AGENTS.md「Shared UI controls」：设置页共享外框（`settingsPageViewport`/`settingsPageContent`/`settingsPageHeading`）未发现任何绕过（唯一调用点 `internal/desktop/settings.go:118-127`）；goal/todo 新面板正确进入共享定义；门禁测试与 TestSettingsPage 通过。问题集中在共享控件被业务代码逐处硬编码覆盖与 i18n 缺口。注意：门禁测试 `TestSettingsControlsUseSharedDefinitions` 只扫描 `settings*.go` 文件（`internal/desktop/control_gate_test.go:19`），下述非设置页文件均处盲区。

豁免判定说明（两类遗留代码待遇不同的依据）：对**现有共享控件的视觉属性覆盖**（ui-1、ui-3）违反 AGENTS.md「业务页面……不得覆盖共享控件的 Height、Size、Padding、Radius、Border、Background、Font、Draw 等视觉属性」的明文条款，判 medium；**未接入共享定义的自绘控件或重复模式**（ui-5~ui-7）没有改变任何现有共享控件的行为，属遗留迁移范围、按团队约定暂缓迁移（具体的遗留豁免清单未见于本次审计材料，规范原文禁止新增逐页豁免名单），仅作 low 标注。区分依据是「是否改变了既有共享控件的行为」。

#### ui-1 · 共享 iconButton 被逐处 .Size() 硬编码覆盖（medium · 已确认）

- **位置**：`internal/desktop/chat.go:166`（另见 `annotation_chips.go:26`、`chat.go:314`、`overlays.go:116-117`、`project_picker.go:403`、`transcript_side.go:253`）
- **问题**：共享 iconButton 的尺寸被遗留代码逐处 `.Size()` 硬编码覆盖；同一批工作区的新增代码已正确使用共享 inlineIconButton 变体，说明正确用法就在手边、覆盖属历史欠账。
- **证据**：`chat.go:166` `.Size(20, 16)`；同类覆盖 `annotation_chips.go:26` `.Size(16, 16)`、`chat.go:314` `.Size(16, 16)`、`overlays.go:116-117` 两处 `.Size(24, 24)`、`project_picker.go:403` `.Size(24, 24)`、`transcript_side.go:253` `.Size(20, 20)`——16/20/24（共享定义另有 28）多种尺寸并存。共享 `inlineIconButton`（`internal/desktop/settings_controls.go:126-130`，24×24）已存在，本批新增的 goal/composer_todo 代码（复核修正：`goal.go:130-137`、`composer_todo.go:189`，初报 99-106/182 行号有偏）确实在正确使用它。违反「业务页面不得覆盖共享控件 Size、新增视觉变体必须进入共享定义」。
- **严重程度**：medium
- **复核状态**：已确认——7 处覆盖逐一确认存在，无多报无漏报；门禁测试照常通过（因扫描范围不含这些文件）。均为 2026-11-02 前提交的遗留代码而非本批新增（问题句已按此改写）；overlays/project_picker 的 24×24 覆盖与现有共享变体完全重复；其中两处可直接改用现成 inlineIconButton。
- **修复建议**：删除这些 Size 覆盖，改用 inlineIconButton；确需 16px/20px 变体时在共享定义中新增语义变体，而非逐页硬编码。

#### ui-2 · 供应商表单文案未走 i18n，中文界面中英混排（medium · 已确认）

- **位置**：`internal/desktop/settings_models.go:232`（另见 `internal/desktop/settings_usage_charts.go:292`）
- **问题**：自定义供应商表单部分文案未调用 tr()，同表单相邻字段已本地化，中文界面下出现中英混排。
- **证据**：`settings_models.go:232` `ui.Text(c, "Base URL")`，而 `internal/i18n/zh-CN.json:838` 已有译文「接口基础地址」；`settings_models.go:249` `ui.Text(c, "API Key")` 无键；`settings_usage_charts.go:292` `ui.Text(c, "Tokens")` 无键。同表单相邻字段 `settings_models.go:237` `tr(c, "API format")` 已本地化。
- **严重程度**：medium
- **复核状态**：已确认——复核成立。复核补充：`i18n.In` 对缺失键回退英文原文，因此仅包 tr() 不补 zh-CN.json 缺失键无效，修复须两步都做；遗漏范围略广：usage 视图 `:297`/`:303` 还内嵌未本地化的 "Tokens" 字样。
- **修复建议**：统一改走 tr() 并补充 zh-CN.json 缺失键，与同表单其余字段保持一致。

#### ui-3 · textButton 被逐处覆盖外观，绕过共享次级/危险变体（medium · 已确认）

- **位置**：`internal/desktop/model_catalog.go:30`（另见 `project_picker.go:52,373,383`、`sidebar_actions.go:125`）
- **问题**：textButton 被逐处覆盖 Border/Padding/TextColor，绕过已有的共享 secondaryButton/dangerButton 变体。
- **证据**：`model_catalog.go:30` `textButton(c, tr(c, "Retry")).Border(1, a.palette.Border)` 在 plain 按钮上手绘次级外观；`project_picker.go:52`、`373`、`383` 三处 `.Padding(4, 6)` 覆盖共享 ButtonPaddingX=8；`sidebar_actions.go:125` `.TextColor(a.palette.Danger)` 自绘危险按钮。共享变体均在用：`settings_models_fetch.go:181` 同类 Retry 按钮就用 secondaryButton，`settings.go:1068` 用 dangerButton。
- **严重程度**：medium
- **复核状态**：已确认——5 处覆盖逐一核实，全仓 grep 无多报无漏报；这 5 处全部处于门禁测试扫描盲区（control_gate_test.go:19 只扫 settings*.go），已运行门禁测试确认照常通过。
- **修复建议**：Retry/Delete 换用 secondaryButton/dangerButton 共享变体；面包屑紧凑样式确属新外观时进入共享定义。

#### ui-4 · 链接 tooltip 硬编码深色配色与固定字号（medium · 已确认）

- **位置**：`internal/linkui/link.go:214`（另见 `internal/workbench/tabs_tooltip.go` 的主题化实现对照）
- **问题**：链接 tooltip 硬编码深色配色与 12px 字号，浅色主题下也恒为深色，与标签页 tooltip 的主题化实现不一致；字号不随用户界面字号偏好缩放。
- **证据**：`link.go:214-215` `Background(ui.Hex("#242428")).TextColor(ui.Hex("#fafafa")).FontSize(12).FixedLineHeight(18)`（复核修正：FontSize(12) 在 215 行），浅色主题下不变；全仓库该色值仅此一处。同为 RoleTooltip 的 `internal/workbench/tabs_tooltip.go` 使用 `p.Border`、`p.Input.Over(p.Background)`、`p.Muted` 与 `design.TextSize(c, design.TextCaption)`（复核修正：p.Border 在 216 行、TextSize 在 210 行），随主题与界面字号变化。该 tooltip 覆盖对话与 markdown 文档的全部链接。
- **严重程度**：medium
- **复核状态**：已确认——问题真实存在，medium 恰当；`link_test.go:225-265` 已有深浅主题子测试但未断言颜色，修复时可复用该夹具在浅色主题下验证。
- **修复建议**：改为语义主题色与字号 token；如确需固定深色 tooltip 变体，进共享定义并在深浅主题验证。

#### ui-5 · 黑白 ink 颜色模式四处重复硬编码（low · 待人工复核）

- **位置**：`internal/desktop/model_picker.go:803`
- **问题**：`transcript.go:545-550` 已有共享 `activityInk`（#000000/#ffffff 按暗色分支），但 `composer_footer_usage.go:377-379` 与 `567-569`、`model_picker.go:803-805`（#1a1a1a/#ebebeb）各自重写同模式；`working_trailer.go:132` 另硬编码三个装饰色 #B6D3EF/#EDB185/#F888A0。
- **严重程度**：low
- **复核状态**：待人工复核——低严重度发现，按流程未做独立复核（复核员统一结论），建议人工抽查。
- **修复建议**：收敛为单一 ink helper 或主题 token，避免新增区域漏掉深浅分支。

#### ui-6 · goal/todo 相邻面板头行几何不同源（low · 待人工复核）

- **位置**：`internal/desktop/goal.go:90`
- **问题**：同一 AttachedPanels 栈内 goal 头行用 `Padding(0, s.HeaderPadding).Gap(s.Gap)`（HeaderPadding=10、Gap=6），todo 头行 controlAttachedToggle（`internal/desktop/settings_controls.go:136-137`）用 `Padding(0, s.RowPaddingX)`（8）与 `Gap(8)`；`composer_todo.go:147-148` 中两面板上下相邻堆叠，同一表面两种行距。
- **严重程度**：low
- **复核状态**：待人工复核——低严重度发现，按流程未做独立复核（复核员统一结论），建议人工抽查。
- **修复建议**：统一 AttachedPanels 的 header 内边距/间距 token，或让 goal 头行复用共享变体。

#### ui-7 · menuChoice 与共享菜单行控件两套实现并存（low · 待人工复核）

- **位置**：`internal/desktop/widgets.go:60`
- **问题**：widgets.go:59-67 `ui.ButtonBase(c).FillWidth().Padding(7, 10).Radius(6)` 自绘菜单行，无 MinHeight 与 FontSize token；controlPromptChoice（`settings_controls.go:450-456`）经 controlRowButton 使用共享 ButtonHeight/ButtonPaddingX/FontSize。`chat.go:294/297`、`session_info.go:76`、`model_picker.go:853` 菜单行字号行高不同源。与 ui-1/ui-3 的区别：本条是未接入共享定义的自绘控件，未改变任何现有共享控件的行为（见 4.3 导语豁免判定说明）。
- **严重程度**：low
- **复核状态**：待人工复核——低严重度发现，按流程未做独立复核（复核员统一结论），建议人工抽查。
- **修复建议**：后续迁移时让 menuChoice 复用 controlPromptChoice，或在共享定义中合并为同一菜单行变体。

### 4.4 并发正确性（5 条）

未发现可证实的数据竞争或死锁：单一 `e.mu` + 回调身份校验（run.afterStop 两端均在锁内、modelChecks/naming/projectScans 以指针身份作废）、worker→window.Update 提交纪律、业务包零 UI import（AGENTS.md 边界成立）、子进程组终止（Unix 进程组 SIGKILL / Windows `taskkill /T`）均成立。验证缺口：`-race` 因本机无 C 编译器未能运行（见 2.4）。

#### concurrency-1 · 引擎全局锁内执行可阻塞的终端进程 IO（medium · 已确认）

- **位置**：`internal/engine/terminal.go:111`
- **问题**：terminalCommandLocked 在 executeCommand 持有 `e.mu` 的整个命令期间执行 `host.Write`/`host.Resize`/`host.Clear` 及 engineTerminalStream.Resize（terminal.go:196-221）；一旦宿主控制台停止消费输入（缓冲写满）该写调用阻塞，而 `e.mu` 是 Snapshot、Subscribe、OpenTerminal 等全部入口的唯一锁，全部会话与 gcode-server 全部客户端随之停摆。
- **证据**：`internal/engine/commands.go:139` 进入 terminalCommandLocked（复核修正：`e.mu.Lock()` 实际在 commands.go:40、defer 解锁在 :44，初报 26 行是 Execute 的日志 defer）；`host.Write([]byte(command.Text))`（terminal.go:111）、`host.Resize`（:118）、`host.Clear`（:127）；Session.Write 落到阻塞管道写（`internal/terminalhost/session.go:192-199` → `internal/terminalhost/pty/pty_windows.go:144-150`），输入管道默认缓冲创建（pty_windows.go:55）；Linux 侧 pty master 无 O_NONBLOCK 打开（`pty_linux.go:15`），命令允许最多 64KiB 输入（terminal.go:108-109）。
- **严重程度**：medium
- **复核状态**：已确认——问题成立，medium 恰当。复核补充触发频率低于初报描述：桌面终端输入走 terminalhost.Stream.Write（`session.go:292-300`）、远端输入走 WriteTerminal（`internal/remote/terminal.go:62`），均不持引擎锁；持锁的 `host.Write` 只经公共命令 API `terminal.input` 到达。桌面/远端 resize 虽经命令路径（`internal/workbench/terminal.go:253`、`internal/remote/terminal.go:131`），但其系统调用（TIOCSWINSZ ioctl、ResizePseudoConsole、2 字节信号写）极少无限期阻塞。因此这是真实可触发的全局冻结隐患而非日常故障，维持 medium。
- **修复建议**：锁内仅解析出 host 指针并校验存在性，释放 `e.mu` 后再调用 host.Write/Resize/Clear（WriteTerminal 模式 `terminal.go:176-188` 已示范），或为这些系统调用加超时/后台化。

#### concurrency-2 · 窗口主线程同步 join 引擎全量关停（medium · 已确认）

- **位置**：`internal/desktop/app.go:365`
- **问题**：窗口关闭在主线程同步 join 引擎全量关停，terminalhost Kill 后等待无界，最坏情况主线程长时间停顿甚至关停永久挂起。
- **证据**：mygo 在窗口主线程 WM_DESTROY 处理中同步触发 OnClosed → `win.OnClosed(func() { a.Close() })`（复核修正：实际在 `app.go:342`）→ `_ = a.service.Close()`（复核修正：实际 `app.go:366`）→ Engine.Close 串行 `host.Close()` 每个终端（`internal/engine/engine.go:287-292`，终端数量上限 32，定义于 `internal/engine/terminal.go:17` `if len(e.terminals) >= 32`），再 `e.wg.Wait()` 等全部 run/终端 watcher/模型检查/插件操作结束。terminalhost.Close 结构为 1 秒宽限后 Kill、随后 `<-s.done` 无超时等待（`internal/terminalhost/session.go:229-239`，依赖 `pty_windows.go:126-137` 的 WaitForSingleObject(INFINITE)）；若 done 永不关闭，Engine 的终端 watcher（`terminal.go:69` host.Result()）同样挂起使 `e.wg.Wait()` 永不返回，进程残留且 `engine.go:296` 的 persistLocked 不执行。
- **严重程度**：medium
- **复核状态**：已确认——问题成立且 medium 恰当；复核指出修复建议两点缺一不可。
- **修复建议**：将 service.Close() 移出主线程（后台 goroutine + 退出码回传），并给 terminalhost.Close 的最终 `<-s.done` 等待加上限（如 2s 后放弃 join，让进程随 ConPTY 句柄释放自然终止）。

#### concurrency-3 · remote.Client.Subscribe 依赖消费方取消 ctx（low · 待人工复核）

- **位置**：`internal/remote/client.go:151`
- **问题**：client.go:151-156 投递协程在 events 缓冲（256，client.go:127）满且消费方不取消传入 ctx 时永久阻塞；159-178 的断线重连循环（每 2s 重试）同样只由 streamCtx 终止。当前唯一调用方 desktop App 以 a.ctx 订阅并在 Close 时 cancel（`internal/desktop/app.go:297`、361），契约成立但属隐式契约。
- **严重程度**：low
- **复核状态**：待人工复核——低严重度发现，按流程未做独立复核（复核员统一结论），建议人工抽查。
- **修复建议**：在文档或类型上显式声明 Subscribe 的取消契约，或在 Client.Close 时主动断开所有活跃订阅（记录活跃 cancel 并统一撤销）。

#### concurrency-4 · run 启动窗口内无锁读共享 e.history（low · 待人工复核）

- **位置**：`internal/engine/run.go:576`
- **问题**：run.go:576 在未持 `e.mu` 时读取共享 map/切片；逐点核查 goal.set（`goal.go:78`）、goal.resume（`goal.go:164`）、message.retry（`commands.go:422`）、compact（`compact.go:24`）、session.delete（`commands.go:488`）、send 排队路径（`run.go:121-135`）等全部潜在并发写者，均在写 history 前检查 `e.runs[id]`，当前无可证明的并发写者；但该不变量散布在 6+ 个调用点，任何新增写入路径漏检即构成真实数据竞争。
- **严重程度**：low
- **复核状态**：待人工复核——低严重度发现，按流程未出具逐条复核结论（复核员统一结论为建议人工抽查）；但其证据收集阶段已逐点核查全部潜在并发写者并确认当前无可证明的并发写者，事实部分证据强度接近复核，待人工确认的主要是「锁内快照」修复方案的采纳。
- **修复建议**：将该读改为在 `e.mu` 内快照（如 `e.mu.Lock(); hist := model.CloneMessages(e.history[session.ID]); e.mu.Unlock()`），使该段代码不再依赖外部不变量。

#### concurrency-5 · 终端 Ctrl+C 中断传播用例在本机连续失败（low · 待人工复核）

- **位置**：`internal/terminalhost/session_test.go:89`
- **问题**：`go test -count=1 ./internal/terminalhost/...` 连续两次运行，TestRealShellInputResizeInterruptDetachAndExit 均失败：向运行 `Start-Sleep -Seconds 30` 的真实 PowerShell 写入 Ctrl+C（0x03）后，waitProof 在 20s 内未等到 interrupt.txt，History 尾部仍显示 PSReadLine 渲染的 Start-Sleep 行——即 0x03 未中断前台命令。普通输入/resize/detach 用例通过，说明字节通路可用，仅中断信号未生效。`docs/model-retry.md:99-103` 记录该测试在早前提交 ac26b06 上同样失败，属既有环境依赖失败项（本机 Windows 11 build 26100 + PowerShell 7，恰为 `pty_windows.go:170-173` 注释点名 ConPTY 信号行为变化的版本）。
- **严重程度**：low
- **复核状态**：待人工复核——失败事实已复现两次（连续两次运行均失败）并有 `docs/model-retry.md:99-103` 的历史记录佐证，「平台敏感用例失败」这一状态本身是明确的；按流程未做逐条复核的是根因分析与修复方案（ConPTY 信号路径与 CI 策略）。
- **修复建议**：在 Windows 11 24H2 上复核 ConPTY 输入侧对 0x03 的处理（必要时改用 ConPTY signal 通道或 ENABLE_PROCESSED_INPUT 相关路径），并在 CI 矩阵中把该用例标记为平台敏感而非普适门槛。

### 4.5 健壮性（4 条）

整体质量较高：storage 原子写 + 哈希校验 + 目录互斥锁；terminalhost/MCP/engine 的进程与 goroutine 均有 WaitDelay/ctx 绑定/ticker.Stop；HTTP 请求均被 ctx 超时或 LimitReader 约束；未发现未守卫的类型断言、切片越界或泄漏的句柄。本段全称结论的背书方式为逐行通读与全库 grep（非逐项比对清单，方法见附录 B），条目级反例以本节发现条目为准。

#### robustness-1 · modelcatalog 缓存损坏导致整机拒绝启动（medium · 已确认）

- **位置**：`internal/modelcatalog/catalog.go:111`
- **问题**：modelcatalog 缓存文件损坏会让整个应用（桌面端与 headless 服务）拒绝启动，而不是回退到内置种子数据。
- **证据**：Open 中 `c.data, err = decode(payload)` 失败即 `return nil, fmt.Errorf("read model catalog cache: %w", err)`（catalog.go:111-113，已抽查核实错误文本在 :113）；`models-dev-state.json` 解析失败同样直接返回错误（catalog.go:120-127，该文件只存 CheckedAt/ETag/LastModified，复核修正：cacheState 实际定义在 67-71）；engine.Open 原样上抛（`internal/engine/engine.go:183-186`），两个入口整体退出：`cmd/rcode/main.go:147` log.Fatal、`cmd/gcode-server/main.go:84-87` return err。同文件对 `len(payload)==0` 已有回退到嵌入 seed 的先例（catalog.go:105-110），损坏场景未同样处理。
- **严重程度**：medium
- **复核状态**：已确认——复核成立并实际运行临时实验验证（截断 JSON、空对象、state 损坏三种场景全部使 Open 失败，实验文件已删除）。不到 high 的原因：storage.WriteJSON 为临时文件 + rename 原子写，半写概率低，触发限于磁盘坏块/位翻转/误编辑。
- **修复建议**：缓存 decode 失败时回退到嵌入的 models-dev.json 并重写缓存，损坏的 state 文件直接丢弃重建；不要让 Open 因可再生的缓存失败。实施注意：嵌入 seed 可能落后于上次成功缓存的目录，离线场景下用户将拿到旧目录且无感知，建议回退时写运行日志并在 UI 提示「模型目录缓存损坏，已回退内置数据，联网后可刷新」。

#### robustness-2 · MCP stdio 服务器 stderr 被 io.Discard 丢弃（low · 待人工复核）

- **位置**：`internal/mcp/client.go:211`
- **问题**：`command.Stderr = io.Discard` 丢弃 stdio MCP 服务器全部 stderr；连接失败时用户只能看到 sanitize 后的协议错误（client.go:575 sanitizeError），无从得知是命令不存在、依赖缺失还是服务器崩溃。对比 agent 的 shell/hook 工具保留了有界 stderr（`internal/agent/hook_execute.go:199-201` boundedOutput）。
- **严重程度**：low
- **复核状态**：待人工复核——低严重度发现，按流程未做独立复核（复核员统一结论），建议人工抽查。
- **修复建议**：将 stderr 捕获到有界环形缓冲，脱敏后附在连接错误信息里或写入运行日志。

#### robustness-3 · writeJSON 原子替换未对父目录 fsync（low · 待人工复核）

- **位置**：`internal/storage/storage.go:574`
- **问题**：`errors.Join(f.Sync(), f.Close())` 后直接 `replaceFile(temp, path)`（storage.go:574-578），缺少目录 fsync。Linux（产品明确要求支持无桌面 Linux 服务）掉电后 rename 可能不持久：不产生半写文件，但 `engine.json`/`location.json` 会回退旧版本。结合 persistLocked 两段写入顺序（先会话记录、后 engine.json，`internal/engine/session_storage.go:226-277`），回退表现为会话记录哈希与索引不符，该对话被标记 StorageError。
- **严重程度**：low
- **复核状态**：待人工复核——低严重度发现，按流程未做独立复核（复核员统一结论），建议人工抽查。
- **修复建议**：非 Windows 平台在 rename 成功后打开父目录执行一次 fsync，保证替换目录项持久。

#### robustness-4 · remote SSE 重连固定间隔、无退避（low · 待人工复核）

- **位置**：`internal/remote/client.go:165`
- **问题**：断线后固定等待 1 秒（client.go:165）、重连失败循环固定 2 秒（client.go:176），无上限、无指数退避与抖动。远端宕机或过载时每个客户端持续以固定频率重连，服务端恢复瞬间所有客户端同时以同一节奏重连，在最需要抑制的故障放大时刻不提供任何衰减。
- **严重程度**：low
- **复核状态**：待人工复核——低严重度发现，按流程未做独立复核（复核员统一结论），建议人工抽查。
- **修复建议**：改为带抖动的指数退避并设置上限，重连成功后重置。

### 4.6 架构与边界（4 条）

五项检查全部完成：(1) 传输边界——desktop 生产代码不导入 engine/remote/mock（`go list -deps ./internal/desktop` 证实），UI 经 `contracts.Service` 强类型接口同进程直调，`cmd/gcode-server` 无 MyGo/桌面依赖，未发现绕行；(2) 分层——engine/contracts 零 MyGo 依赖，App 状态仅为展示投影与草稿；(3) contracts——未发现 any 滥用或重复类型定义（architecture-1 除外）；(4) 死代码——architecture-2/3；(5) 文档——核对 `docs/contracts.md`、`backend-v1.md`、`model-adapters.md`、`goals.md`、`todos.md`、`session-storage.md` 约 15 项关键声明，仅 goal.edit 一处不符（architecture-4）。

#### architecture-1 · 判别 Kind/Delivery 为裸字符串散布多包（medium · 已确认）

- **位置**：`internal/contracts/types.go:469`（另见 `types.go:160` Block.Kind、`types.go:477` SendRequest.Delivery）
- **问题**：命令/事件/块的判别 Kind 与 Delivery 等边界值是裸字符串，字面量散布在 contracts、engine、desktop、mock 等多包且无中央常量，新增命令或改拼写时编译期无保护，只有运行时 Validate/switch 落空。
- **证据**：`types.go:469` Event.Kind、`types.go:160` Block.Kind、`types.go:477` SendRequest.Delivery 均为 string；分发与消费靠各包各自写字面量：`internal/engine/commands.go:132` `case "goal.set", "goal.edit", "goal.pause"…`、`internal/desktop/app.go:434` `event.Kind == "message.delta"`、`app.go:1135` `cmd.Send.Delivery = "steer"`、`internal/engine/commands.go:516` `Delivery = "steer"`、`internal/mock/actions.go:673` publishLocked("message.delta",…)。同仓已有类型化先例：`types.go:13-20` SessionStatus、`internal/contracts/activity.go:8-23` ToolKind、`internal/contracts/goal.go:11-19` GoalStatus；Validate（`contracts/command.go:82` 起）也全靠字面量兜底。
- **严重程度**：medium
- **复核状态**：已确认——发现成立，medium 恰当（架构一致性/可维护性问题而非功能缺陷）。复核补充：字面量实际散布比初报的四处更广——`internal/remote` 非测试文件（`terminal.go:131` 等）亦然。
- **修复建议**：参照 SessionStatus/ToolKind 的做法，在 contracts 内为 Command.Kind、Event.Kind、Block.Kind、Delivery 定义类型化常量并替换 engine/desktop/mock/remote 中的字面量。

#### architecture-2 · agent.WithHookHTTPClient 死选项（low · 待人工复核）

- **位置**：`internal/agent/hooks.go:165`
- **问题**：`agent.WithHookHTTPClient` 选项及 HookRunner.http 字段为死代码：全仓 grep 仅命中定义 hooks.go:165-170 与写入 hooks.go:168、声明 hooks.go:208、初始化 hooks.go:220，内外无任何读取点；同组其余 WithHook* 选项均有外部引用，唯独此选项为零。
- **严重程度**：low
- **复核状态**：待人工复核——低严重度发现，按流程未做独立复核（复核员统一结论），建议人工抽查。
- **修复建议**：删除 WithHookHTTPClient 与 HookRunner.http 字段及 NewHookRunner 中的空 Client 分配；确有预留需求则在章程层面说明。

#### architecture-3 · contracts.BrowserAnnotation 死别名（low · 待人工复核）

- **位置**：`internal/contracts/annotation.go:29`
- **问题**：`type BrowserAnnotation = Annotation` 无任何引用；全仓 grep（排除 third_party/reference）仅命中定义行与 annotation.go:13 的注释，engine/desktop/mock/remote/测试均未使用。
- **严重程度**：low
- **复核状态**：待人工复核——低严重度发现，按流程未做独立复核（复核员统一结论），建议人工抽查。
- **修复建议**：删除该别名，保留第 13 行注释中对参考协议的溯源即可。

#### architecture-4 · 文档漂移：goal.edit「原内容校验」与实现不符（low · 待人工复核）

- **位置**：`docs/goals.md:14`
- **问题**：`docs/goals.md:14`「按原目标 ID 和原内容校验冲突」、`docs/todos.md:52`「保存使用带原目标 ID 和原内容的 goal.edit 类型化命令」，但 `internal/contracts/goal.go:28-31` `GoalEditInput{Objective string; ExpectedRevision uint64}` 不含原内容字段，`internal/engine/goal.go:127-128` 仅校验 `goal.Revision != command.GoalEdit.ExpectedRevision`；命令 ID 为 goal ID（`internal/desktop/goal_editor.go:67`），无原内容参与校验。
- **严重程度**：low
- **复核状态**：待人工复核——低严重度发现，按流程未做独立复核（复核员统一结论），建议人工抽查。（背景：审计期间该机制正从 ExpectedObjective 迁移到 ExpectedRevision，文档未同步。）
- **修复建议**：将两处文档改为「按目标 ID 与 ExpectedRevision 乐观并发校验」，与 GoalEditInput 实际字段一致。

---

## 5. 修复优先级建议

判定规则：verified 且 high 为 P0；其余按严重程度与复核状态排列。

**P0：无。** 本次审计未发现 high 严重度问题。

**P1（14 条 verified medium，全部列出，建议按主题与依赖排序）：**

1. **凭据与传输暴露**（security-1、security-2）：用户可见的部署安全面，改动集中在 gcode-server 入口与链接目标校验，收益/成本比最高。先做 TLS 强制或启动告警、provider-key 掩码返回；再做链接目标范围限制与凭据文件拒绝列表（注意两处「实施注意」：隧道/反代部署兼容、linked_files_test 锁定的有意设计变更）。
2. **启动阻断**（robustness-1）：modelcatalog 缓存回退 seed，改动小、消除单点可用性风险（注意回退时的日志/UI 提示）。
3. **引擎关键路径重活**（concurrency-1、concurrency-2、performance-1、performance-2、performance-3）：锁内 IO、锁内持久化/克隆/扫描同源于 `e.mu` 全局串行化，建议合并设计一次改造——锁内只解析指针不做 IO（concurrency-1）、持久化脏标记并移出锁（performance-1）、快照共享/增量事件（performance-2）、流式索引缓存（performance-3）；concurrency-2 的主线程同步关停不属于锁问题，但同属关停路径改造，一并设计。
4. **流式累加 O(n²)**（performance-4）：`strings.Builder`/`[]byte` 机械替换，覆盖五处平方累加点（model/stream.go、run.go、model/chat.go、app.go:460、Anthropic 适配器），收益确定。
5. **UI 共享控件与本地化**（ui-1、ui-2、ui-3、ui-4）：单批清理，同时把门禁测试扫描范围扩展到非 settings 文件，防止复发。
6. **contracts 类型化常量**（architecture-1）：参照 SessionStatus/ToolKind 为 Command.Kind、Event.Kind、Block.Kind、Delivery 定义类型化常量并替换 engine/desktop/mock/remote 字面量；工作量可估，可随任一相关改动渐进落地。

**P2（1 条 verified low + 16 条 unconfirmed low）：**

- 对 16 条「待人工复核」先做人工复核（成本远低于误修复成本），确认后按实际频率与影响重估——performance-5 的复核已表明初报频率可能高估（其自身已确认为 low，可与 P1 第 3 项的快照共享化一并处理）。
- 可直接清理项：architecture-2/3（死代码删除）、architecture-4（文档改为与 GoalEditInput 一致）。
- 平台敏感项：concurrency-5 建议在 CI 矩阵中标记为平台敏感而非普适门槛。
- 低成本机械项（以下修法为候选方案，须先通过上述人工复核，非已确认结论）：performance-8（`bytes.NewReader`）、robustness-4（指数退避）、robustness-2（stderr 有界捕获）、robustness-3（目录 fsync）、performance-6/7、ui-5/6/7、security-3、concurrency-3/4。

---

## 6. 附录

### 附录 A：静态检查输出摘要

**审计期间结果（由审计执行方提供）：** `go build ./...` 退出码 1；`go vet ./...` 退出码 1；错误均为 `output/desktop-e2e` 下历史 e2e 运行产物目录中的非 ASCII 导入路径（14 条 `malformed import path`）。范围限定复核：`go vet ./internal/...` 通过（exit 0 无输出）；`go vet ./internal/... ./cmd/...` 通过、无输出；`go build ./internal/... ./cmd/...` 通过（EXIT=0）。

**撰写时复跑结果（本报告撰写时在同一工作树执行，与审计期间一致）：**

```
$ go vet ./... ; echo EXIT=$?        → EXIT=1（14 条 malformed import path，无其他输出）
$ go build ./... ; echo EXIT=$?      → EXIT=1（同样 14 条，无其他输出）
$ go vet ./internal/... ./cmd/...    → EXIT=0（无输出）
```

完整错误清单（`go vet ./...` 与 `go build ./...` 输出相同，逐行原文）：

```
malformed import path "gcode/output/desktop-e2e/20261101-135040-3790720148/TestDesktopLocalTranscriptLinks-dark-1243226562/workspace/示例": invalid char '示'
malformed import path "gcode/output/desktop-e2e/20261101-135040-3790720148/TestDesktopLocalTranscriptLinks-light-3805426353/workspace/示例": invalid char '示'
malformed import path "gcode/output/desktop-e2e/20261101-151807-2987448678/TestDesktopLocalTranscriptLinks-dark-2177882481/workspace/示例": invalid char '示'
malformed import path "gcode/output/desktop-e2e/20261101-151807-2987448678/TestDesktopLocalTranscriptLinks-light-3909455090/workspace/示例": invalid char '示'
malformed import path "gcode/output/desktop-e2e/20261101-173322-3584585105/TestDesktopLocalTranscriptLinks-dark-3410484137/workspace/示例": invalid char '示'
malformed import path "gcode/output/desktop-e2e/20261101-173322-3584585105/TestDesktopLocalTranscriptLinks-light-3848056770/workspace/示例": invalid char '示'
malformed import path "gcode/output/desktop-e2e/20261101-185848-4201073726/TestDesktopLocalTranscriptLinks-dark-1929027036/workspace/示例": invalid char '示'
malformed import path "gcode/output/desktop-e2e/20261101-185848-4201073726/TestDesktopLocalTranscriptLinks-light-2780791187/workspace/示例": invalid char '示'
malformed import path "gcode/output/desktop-e2e/20261101-190312-3045573035/TestDesktopLocalTranscriptLinks-dark-1852335252/workspace/示例": invalid char '示'
malformed import path "gcode/output/desktop-e2e/20261101-190312-3045573035/TestDesktopLocalTranscriptLinks-light-2835587107/workspace/示例": invalid char '示'
malformed import path "gcode/output/desktop-e2e/20261101-192452-3441950967/TestDesktopLocalTranscriptLinks-dark-2856801679/workspace/示例": invalid char '示'
malformed import path "gcode/output/desktop-e2e/20261101-192452-3441950967/TestDesktopLocalTranscriptLinks-light-723240388/workspace/示例": invalid char '示'
malformed import path "gcode/output/desktop-e2e/20261104-171043-2681042965/TestDesktopLocalTranscriptLinks-dark-3782143996/workspace/示例": invalid char '示'
malformed import path "gcode/output/desktop-e2e/20261104-171043-2681042965/TestDesktopLocalTranscriptLinks-light-346714484/workspace/示例": invalid char '示'
```

说明：`output/desktop-e2e` 是 `go run ./scripts/desktop-e2e` 的运行产物落盘目录（本机现存 42 个时间戳目录，最早为 `20261029-235256-c6de310e`，即该失败至少自 2026-10-29 的产物落盘起存在；审计材料未记录更早状态，本报告无法核实其首次出现时间）。其中 e2e 测试工作区含中文目录名（如 `workspace/示例/`），Go 工具链全仓扫描时将其当作导入路径解析失败。未单列为 finding 的理由：失败源于构建产物目录而非源码，属构建卫生问题而非产品缺陷；处理出路是清理历史产物或将 e2e 产物移出 module（使 `output/` 落在 Go 工具链扫描范围之外），是否执行由团队决定。

**撰写时行号抽查（只读 grep，与复核员修正一致）：** `internal/engine/commands.go:40` `e.mu.Lock()`；`cmd/gcode-server/main.go:120` ServeTLS / `:122` server.Serve；`internal/desktop/app.go:342` `win.OnClosed(func() { a.Close() })` / `:366` `_ = a.service.Close()`；`internal/modelcatalog/catalog.go:113` `read model catalog cache`；`internal/engine/linked_files.go:188` `if err == nil && file.ProjectID != ""`；`internal/model/client.go:287` `strings.NewReader(string(body))`。本次修订新增核实：`internal/engine/run.go:878` message.delta 发布 / `:880` 该分支 `return nil`；`internal/engine/session_storage.go:184-217` sessionDisk 组装与整体 MarshalIndent；`internal/engine/terminal.go:17` 终端数量上限 32；`internal/remote/server.go` 仅 Bearer 认证、无 Host/Origin 校验（grep 证实）。

### 附录 B：审计方法说明

- **独立复核机制**：每条发现由独立复核员逐行重放 evidence（读源码、必要时运行测试或临时实验）并给出 verdict。verified 表示复核成立（含对行号/频率/严重程度的修正，正文按复核结论呈现并标注）；unconfirmed 表示按流程未做逐条独立复核——本次 16 条 unconfirmed 全部为 low，复核员统一结论为「低严重度发现，按流程未做独立复核，建议人工抽查」。核准入口径、否决条目处置与判级依据的说明见 2.2。
- **安全域方法**：只读代码审计，覆盖监听地址默认值、TLS 分支、server.json token 生成/存储/日志、remote 全部路由（commands/directories/events/terminals/provider-key/attachments/search/trajectory/logo/metadata）、storage 0700/0600/原子替换/符号链接/重叠根校验、engine 全部路径与命令处理、agent shell（BASH_ENV/ENV 剥离、Windows bash 探测）、mcp（stdio 启动、重定向禁转发、PKCE/state/issuer 校验、SSE 同源约束）、模型凭据红化与错误边界、workbench/repohost/linktarget/providerlogo 注入面；全库 grep `net.Listen`/`ListenAndServe`、`Access-Control`/CORS、`log.Print`、`os.WriteFile`/`OpenFile` 权限位。未运行任何测试或构建。
- **性能域方法**：先读 `docs/memory-tuning.md` 确认已调优项（启动后 FreeOSMemory、媒体 LRU、壁纸缩略图）避免重复报告；逐行通读 model 流式解码与请求路径、engine 事件发布/持久化/运行循环、storage 写盘、desktop 事件应用与 workbench 更新、agent 工具循环与 hook 匹配；goroutine 泄漏排查覆盖 goal 心跳、存储维护、壁纸、SSE 心跳、terminalhost 全文；正则盘点 grep 全库 `regexp.MustCompile`/`Compile` 并核实热路径（hook_condition、grep_walk、mcp/catalog）。未运行基准测试；个别量级数据来自复核时的合成微基准（6MB/24MB 会话编码耗时、100KB 回复 `+=` 累计分配 27~212MB、重度形态单次克隆 42µs/207KB）。
- **UI 一致性域方法**：阅读 `docs/ui-controls.md` 与 AGENTS.md「Shared UI controls」；运行门禁测试 `go test ./internal/desktop -run 'Test(SettingsControls|InspectSettingsControls)' -count=1` → ok 与 `go test ./internal/desktop -run 'TestSettingsPage' -count=1` → ok；grep `settingsPageViewport`/`settingsPageContent`/`settingsPageHeading` 与 MaxWidth 全部使用点；grep `ui.Hex` 扫描硬编码颜色并逐处读上下文；grep 共享按钮构造器链式视觉覆盖与原生 MyGo 构造器调用点；核对新增 todo/goal 全部文案键在 `internal/i18n/zh-CN.json` 的存在性；核对图标名存在于 `internal/assets/manifest.json` 或 glyphs（goal.svg 符合 24 画布 1.75 线宽 currentColor 约定）；对比三处 RoleTooltip（transcript_rail.go、linkui/link.go、workbench/tabs_tooltip.go）主题化程度。
- **并发正确性域方法**：通读 engine（run/queue/commands/compact/subagents/naming/provider_checks/terminal/plugins）、agent（agent/tool_loop/background/hooks/hook_execute/shell/process_*）、model（client/stream/trajectory）、remote（server/client/terminal）、desktop（app/media/attachments/notifications 等 goroutine 站点）、processutil 与 terminalhost 的全部 goroutine 产生点与共享状态访问，grep 逐一核对写者后交叉验证。验证执行：`go vet ./internal/...` 通过；`CGO_ENABLED=1 go test -race ./internal/{agent,model,remote,terminalhost}/...` 未能运行（cgo 需要 gcc，本机 PATH 无 C 编译器，race 检查如实记为未执行）；`go test -count=1 ./internal/{agent,model,remote,terminalhost}/...` agent/model/remote 通过、terminalhost 的 TestRealShellInputResizeInterruptDetachAndExit 失败（复跑一次仍失败，连续两次，25.9s 超时，interrupt.txt 未出现）；`go test -count=1 ./internal/engine/...` 通过（78.5s）；`go test -count=1 -run TestCloseUnblocksReaderAndCanceledAttachment ./internal/terminalhost/` 通过。
- **健壮性域方法**：五维度（错误吞噬、panic 风险、资源泄漏、持久化完整性、超时与重试）通读 17+ 包核心实现；运行 `go vet ./internal/... ./cmd/...` 与 `go build ./internal/... ./cmd/...`（均通过、无输出）；全库 grep 排查无 ok 类型断言、裸切片索引、ticker 未 Stop、http.Client 无界、resp.Body 未 Close、exec.Command 未 Wait、panic 调用点及 recover 边界。注意：审计期间工作树仍在被并发修改（goal.edit 从 ExpectedObjective 改为 ExpectedRevision），引用行号以最终核验时文件状态为准。
- **架构与边界域方法**：`go list -deps` 核实包依赖方向（desktop 不含 engine/remote/mock；engine/contracts 不含 mygo/desktop）；grep desktop/workbench 非 uifixture 代码中 `os/exec`、websocket、jsonrpc、json.Marshal 往返（仅 fixture 与展示格式化命中）；阅读装配与事件循环；扫描 any/`interface{}`、字面量 Kind 散布及导出符号引用；`go vet ./internal/contracts ./internal/engine ./internal/desktop` 通过；导出符号死代码全仓核实（BrowserAnnotation、WithHookHTTPClient）；mock 引用链核查（`cmd/rcode --scenario` 门控、gcode-server 不导入 mock、fixture 均 `go:build uifixture`、automation 插件 `go:build e2e`）；文档核对 6 份 docs 约 15 项关键声明，仅 goal.edit 一处与实现不符。
