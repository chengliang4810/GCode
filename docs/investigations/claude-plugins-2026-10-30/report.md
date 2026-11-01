# GCode 插件管理与 Claude 插件市场调查

调查日期：2026-10-30，Asia/Shanghai。

同日后续决策：90% 验收只计算默认官方市场，社区市场继续提供但不计入分母。Claude 模型别名在当前阶段继承执行模型，用户对指定子智能体类型配置的模型优先。下文对应建议已据此更新。

本次只调查、运行已有专项测试并生成报告，没有实现插件功能、修改应用源码、安装或执行市场插件，也没有恢复已暂停的 UI 优化。报告依据当前工作树，而非仅依据 Git 已提交版本。

## 1. 结论

**可以将 Claude 公开插件格式作为 GCode 的插件契约，但当前项目还缺少插件运行层和市场管理层，不能通过增加市场页面就达到兼容。现在没有证据证明市场插件运行率达到 90%。**

现有 Go 核心已经提供真实技能导入、命令展开、MCP 工具调用、Hook 进程和子智能体执行，能够作为基础。不过这些功能采用 GCode 自身定义，部分与 Claude 定义存在直接冲突，需要更新现有实现及调用方，不能只导入文件或转换配置。

默认市场应包含用户要求的两个真实源：

- `anthropics/claude-plugins-official`，市场名 `claude-plugins-official`。
- `anthropics/claude-plugins-community`，市场名 `claude-community`。

另将 `anthropics/skills` 和 `obra/superpowers-marketplace` 作为可选来源；`anthropics/claude-code` 的市场如今主要重定向到官方插件仓库，默认再加一份会增加重复条目。

“与 Claude 完全一致”必须分开验收：文件/配置定义、加载与执行语义、插件实际任务效果。第三项还取决于宿主系统、模型能力、凭据、外部服务和依赖工具，前两项一致不能保证第三项。

## 2. 事实源与版本边界

### GCode

- 路径：`D:/projects/GCode`。
- HEAD：`e864970b8d3e745555530af54ddfdd66b0c3168b`。
- 开始调查时 `git status --porcelain` 有 415 条记录，包含大量真实后端新增文件；因此 HEAD 本身不能代表被调查代码。
- 审阅的关键文件 SHA-256、文档抓取时间和测试结果记录在 [source-evidence.json](source-evidence.json)。

### 本地 Claude 参考

- 路径：`D:/projects/claude-code`。
- origin：`claude-code-best/claude-code`，**不是** `anthropics/claude-code`。
- HEAD：`8d80e740f764be358e265eed4250606c78fca579`。
- `package.json:2–10` 自称反向工程项目，版本为 `2.8.4`；`README_EN.md:12`、`AGENTS.md:7` 也说明反编译重建及 stub/feature flag。
- 根目录未发现 LICENSE，`README_EN.md:211` 的研究用途声明不能作为可移植其代码的开放许可证据。建议只研究行为，按公开规范独立实现。
- 查询时 npm `@anthropic-ai/claude-code` 的 `latest` 为 **2.1.314**。不能将第三方重建仓库的 `2.8.4` 当成 Anthropic 官方产品版本或“更新的官方实现”。

### 公开规范

规范采用查询时 Anthropic 公开文档；它是持续更新的文档，不应假定与某个第三方仓库版本完全对应。已记录 10 篇页面的 URL、抓取时间、正文长度和 SHA-256。Python 抓取曾返回 403，随后 PowerShell 独立抓取全部返回 200，失败与成功路径均在证据文件中说明。

主要来源：

| 来源 | 用途 |
| --- | --- |
| [Plugin reference](https://code.claude.com/docs/en/plugins-reference) | manifest、默认目录与自定义组件路径行为 |
| [Marketplace reference](https://code.claude.com/docs/en/plugins/marketplace-reference) | 市场清单、source、strict、依赖和版本 |
| [Anthropic marketplaces](https://code.claude.com/docs/en/plugins/anthropic-marketplaces) | 官方与社区源身份 |
| [Plugin loading](https://code.claude.com/docs/en/plugins/loading) | 安装范围、缓存、启停和重载 |
| [Plugin components](https://code.claude.com/docs/en/plugins/components) | commands、agents、MCP、LSP 等组件 |
| [Plugin dependencies](https://code.claude.com/docs/en/plugins/dependencies) | 依赖解析及缺失行为 |
| [Skills](https://code.claude.com/docs/en/skills) | frontmatter、调用控制、参数和动态上下文 |
| [Hooks](https://code.claude.com/docs/en/hooks) | 事件、匹配、输入输出和决定语义 |
| [Subagents](https://code.claude.com/docs/en/sub-agents) | agents 文件及执行限制 |
| [MCP](https://code.claude.com/docs/en/mcp) | 传输、认证、工具名和用户输入 |

## 3. GCode 当前实现

| 范围 | 当前代码与证据 | 与目标的差距 |
| --- | --- | --- |
| 插件与市场管理 | `internal/desktop/settings.go:23` 只有模型、子智能体、MCP、技能、命令、Hook；`internal/contracts/user_resources.go:10` 只有三类用户资源 | 未发现 marketplace/plugin manifest 加载器、源登记、安装记录、更新卸载及依赖解析 |
| 技能 | `internal/engine/user_resources.go:234` 管理导入/编辑，`resources.go:13` 提供摘要，`resources.go:95` 显式调用 | 当前资源来自用户目录；没有 Claude 项目/插件发现、命名空间或完整 frontmatter 执行语义 |
| 技能元数据 | `user_resources.go:447` 按文本行提取 `name`/`description` | 不是 YAML 解析器；不能正确承载多行值、数组、嵌套 hooks、allowed-tools、context 等定义 |
| 命令 | `resources.go:34,95` 支持 `$ARGUMENTS` 和从 `$1` 开始的位置参数 | 不识别 `$0`、`$ARGUMENTS[N]`、插件命名空间，也不执行 Claude 动态上下文 |
| Hook | `internal/contracts/hooks.go:12` 有七事件，`internal/agent/hooks.go:126` 真正执行进程 | 用户级扁平规则；没有插件 hooks.json、完整事件集、分组 matcher、Claude 输入输出语义 |
| MCP | `internal/mcp/client.go:26,63`、`internal/engine/mcp.go:11` | stdio/Streamable HTTP 工具子集；缺少插件配置发现、SSE/OAuth、用户配置展开及完整能力 |
| MCP 生命周期 | `engine/mcp.go:22` 顺序连接所有启用服务器，任一失败中止本次运行 | 一个需要登录或离线的插件服务器会阻断整个运行；尚无按服务器隔离状态和恢复流程 |
| MCP 结果 | `mcp/client.go:347` 仅保留 text 与 structuredContent | 图片、资源等非文本内容未接入现有工具结果模型 |
| Agents | `internal/contracts/subagents.go:12`、`internal/engine/subagents.go:86` | 有真实子任务生命周期，但没有插件 agents Markdown 加载与字段投影 |
| 本地/远程 | `internal/engine`、`internal/contracts`、`internal/remote` 已分层 | 插件执行及安装应归实际宿主；不能在远程客户端复制一套业务执行器 |

当前文档 `docs/backend-v1.md:138–159`、`docs/user-settings.md:44,89–92,160–171`、`docs/hooks.md` 对“用户级、自身协议、没有市场、没有 SSE/OAuth”的描述与本次检查到的代码一致。它们不是 Claude 兼容声明。

## 4. 必须解决的兼容问题

### 4.1 文件格式与加载

应原生识别 `.claude-plugin/plugin.json` 和 `.claude-plugin/marketplace.json`，保留 `plugin@marketplace` 身份、原始组件文件和目录结构。manifest 可以省略，市场可提供 inline 定义；不能把“缺少 plugin.json”一概判为坏插件。

自定义路径的规则不能凭直觉实现：当前公开文档规定 commands/agents/outputStyles 替换默认目录，skills 增加到默认目录，hooks/MCP/LSP 合并。`strict`、组件冲突、manifest 与市场条目名称冲突需要按规范分别处理。

市场 source 也不只是 GitHub 整仓克隆。当前清单已实际出现相对目录、外部 Git URL、`git-subdir`、分支/ref/SHA、子目录及 inline 配置。应先按真实清单建立解析样本，再实现公开规范允许的其他来源；未实现的类型要明示，不得当成空插件安装成功。

### 4.2 Skills 与 commands

当前 Claude 将命令和技能关联到共同的调用语义，但依然支持 `commands/`。GCode 应将两者加载为有来源、命名空间和执行元数据的资源，保留 `/plugin:skill` 身份；不能把插件技能平铺成当前只允许字母数字等字符的 50 位资源 ID。

特别是：

- GCode 的 `$1` 是第一个参数；Claude 当前 `$0` 是第一个、`$1` 是第二个，还支持 `$ARGUMENTS[N]`。现有展开会把 `$ARGUMENTS[0]` 错误地变为“全部参数 + `[0]`”。
- `disable-model-invocation`、`user-invocable`、`allowed-tools`、`context: fork`、`agent`、`model`、`effort` 和技能级 hooks 影响调用权限与执行，不只是显示元数据。
- `${CLAUDE_PLUGIN_ROOT}`、`${CLAUDE_PLUGIN_DATA}`、`${CLAUDE_SKILL_DIR}`、`${CLAUDE_SESSION_ID}`、用户配置与动态 shell 上下文必须在正确边界展开，敏感值不能直接注入模型。
- 插件的相对支持文件要跟随真实安装路径，更新或卸载不能破坏正在执行任务所用的版本。

当前模型只能得到启用技能摘要并按需 read；没有正式的 Claude Skill 调用及其工具权限边界。仅把正文附加到提示词不足以声称一致。

### 4.3 Hooks

当前官方事件表有 **33 种**事件，GCode 有七种，而且同名事件也未必语义相同。

具体阻塞：

| 项目 | GCode | Claude 插件要求 |
| --- | --- | --- |
| 输入字段 | `hookEventName`、`sessionId`、`toolName`、`toolInput` | `hook_event_name`、`session_id`、`tool_name`、`tool_input` 等事件契约 |
| 输入工具名 | `read/write/edit/bash` | 常见插件匹配 `Read/Write/Edit/Bash`，还依赖其工具参数结构 |
| 输出 | 严格仅接受 decision/reason/additionalContext，decision 为 allow/deny | 事件特定 `hookSpecificOutput`、permissionDecision、updatedInput、continue 等 |
| matcher | SessionStart 匹配 source，Prompt/Stop 还匹配正文 | 应遵循每个事件的官方 matcher 语义，不支持 matcher 的事件不能套通用正则 |
| PermissionRequest | 每次工具执行前的规则检查 | Claude 的权限决策事件，不等同于每次调用工具 |
| Shell | Windows 用 PowerShell 执行 command | Claude 有显式 shell 配置；Windows 默认 Git Bash、未安装时用 PowerShell，Unix shell form 用 sh -c；脚本不会自动翻译 |
| Hook 类型 | command/process | 当前公开定义为 command/http/mcp_tool/prompt/agent；直接进程使用 command+args，process 是 GCode 自身定义 |
| 错误和异步 | 非零错误可能终止运行，异步结束前等待且错误影响运行结果 | 需逐事件对齐阻塞/非阻塞退出码、超时、async、重唤醒与上下文交付 |

证据：`internal/agent/hooks.go:24,126,196,257`。尤其解析器调用 `DisallowUnknownFields`，现成 Claude hook 返回 `hookSpecificOutput` 时会直接失败。把字段名简单改成 snake_case 也不足以解决其余差异。

### 4.4 MCP、工具名与模型选择

GCode 当前工具名为 `mcp_<server>_<tool>`；Claude 使用 `mcp__...__...` 结构及插件服务器命名空间。插件中的 allowed-tools 与 Hook matcher 因而也需要相同的可观察名称。

当前握手只接受 `2025-06-18`，不能把它称为“最新完整 MCP”。市场插件还会需要 SSE、OAuth、环境变量/用户配置、资源、提示和 elicitation。应按实际插件需求实现当前多服务器互通，并保留失败原因、认证状态与取消行为。

Claude 的 `sonnet/opus/haiku/inherit` 等模型选择与 GCode 用户自定义三协议模型没有天然一一对应关系。用户已选择当前阶段将这些标记虚拟映射到继承模型，不增加角色映射设置。执行顺序为：指定子智能体类型的用户模型及推理配置优先，否则继承调用方的实际模型及推理配置。主对话直接执行插件时继承主对话；已选定其他模型的子智能体内部执行技能时保持子智能体的执行模型，不再切回主对话模型。运行开始后冻结实际选择，不把别名作为真实模型 ID 发给供应商。

这是明确的 GCode 模型路由策略，不代表 Sonnet/Opus/Haiku 的能力、速度和成本等价。兼容率需针对实际选择的模型验证；后续开放角色映射时只需替换模型解析策略，不必在当前数据里创建虚假供应商模型。

### 4.5 内置子智能体

当前 GCode 只有 `general-purpose` 与小写 `explore`，还需要对齐 Claude 的可观察类型及实际能力，不能只改显示名称。优先补齐 `general-purpose`、`Explore`、`Plan` 的发现、类型选择、工具限制、独立上下文和生命周期；当前公开文档另列 `claude` 通用类型及 statusline-setup/claude-code-guide 等专用辅助类型，需按 GCode 实际能力逐项处理，不能注册空壳冒充支持。Explore/Plan 的只读边界不能因统一模型而放宽。

本地参考 `built-in/exploreAgent.ts:78` 默认给外部用户选择 haiku；当前公开 sub-agents 文档描述 Explore 通常继承主对话模型，另有 Fable/接入方式条件。这是新增核实的参考与公开资料差异，按公开资料处理。

### 4.6 “等那一套”的范围

市场并不只有 skills/commands/MCP/hooks。实际目标还涉及 agents、LSP、output styles、依赖、userConfig、插件 settings，最新公开资料还有更高阶扩展。

无需为尚未确定的扩展先建立庞大框架，但市场必须识别能力要求：未实现的组件应显示明确阻塞，不能在安装时丢弃后把插件标成可用。调用 `claude` CLI 的插件也要单列；让 GCode 再启动 Claude 并不能证明 GCode 已具备其原生能力。

## 5. 本地参考的实现路径与局限

可借鉴的分层：

| 参考代码 | 可研究的行为 |
| --- | --- |
| `src/utils/plugins/pluginLoader.ts:3076` | 会话目录、市场插件、依赖及 settings 汇总 |
| `marketplaceManager.ts:132,219` | settings 安装意图与实际市场缓存状态分离 |
| `installedPluginsManager.ts:397` | 用户/项目/local 安装记录与 projectPath |
| `loadPluginCommands.ts:228,339` | frontmatter、命名空间、变量与调用 |
| `mcpPluginIntegration.ts:137,343` | MCP 配置合并及插件服务器命名空间 |
| `loadPluginAgents.ts:92,153` | agent 字段与插件 agent 权限限制 |
| `src/utils/hooks.ts:603,1475` | Hook 决定、信任、权限及事件边界 |

**本次发现的文档/声明与代码差异全部列在下面；不能照抄参考来“获得兼容”。**

1. `AGENTS.md:84` 称版本为 `2.1.888`；`package.json:3` 与 `scripts/defines.ts:16–20` 实际使用 `2.8.4`。
2. `schemas.ts:434–516` 多处把自定义组件路径描述为“补充默认目录”；`pluginLoader.ts:1381–1384` 会在配置 commands/agents/skills/outputStyles 时跳过默认目录。当前官方规则则是 skills 增加、commands/agents/outputStyles 替换：参考的 skills 加载行为与公开规则冲突，其余几项是 schema 注释过时。
3. `schemas.ts:1247,1284` 称 strict=true 必须存在 plugin.json；`pluginLoader.ts:2454–2462` 明确无论 strict 如何，缺失 manifest 时使用市场条目。当前公开规范允许 manifest 可选，不能沿用这条旧 schema 注释。
4. `docs/extensibility/skills.mdx:83–90` 展示 context/agent/hooks；普通技能 `src/skills/loadSkillsDir.ts:259–261` 确实解析，但插件 `loadPluginCommands.ts:300–322` 未将这些字段接入。`packages/builtin-tools/src/tools/SkillTool/SkillTool.ts:625` 才依据 context=fork 分支执行，因此不能认为插件路径已经支持。
5. `schemas.ts:975–979` 声明 npm 市场来源；`marketplaceManager.ts:1618–1620` 直接报未实现。
6. `schemas.ts:1091–1108` 声明 pip 插件；`pluginLoader.ts:961` 直接报 Python package plugins 尚不支持。这是参考内部声明与执行的差异，不代表应把 pip 视为当前官方通用要求。
7. `schemas.ts:773–788` 接受 restartOnCrash/shutdownTimeout；`src/services/lsp/LSPServerInstance.ts:94–103` 却拒绝它们。
8. `src/utils/argumentSubstitution.ts:123–133` 对缺失的位置参数替换为空串；当前公开 Skills 规范要求缺失的位置占位符保留原文。两者都从 `$0` 开始索引，但缺位语义仍不一致。
9. `packages/builtin-tools/src/tools/AgentTool/built-in/exploreAgent.ts:78` 给外部用户默认 haiku；当前公开 sub-agents 文档对 Explore 说明通常使用主对话模型，并对 Fable/接入方式另设条件。本项目按公开资料及用户明确指定的模型继承策略实现，不沿用参考的固定 haiku 路由。

插件 agent 的 hooks/mcpServers/permissionMode 被参考加载器主动忽略，这一限制也出现在官方 subagents 文档中，应视为权限边界，不是随意补上所有字段就更兼容。

## 6. 市场逐项盘点

已冻结五个市场清单，并为 **2,625 个市场条目**生成逐条 JSON/CSV。条目数按市场身份统计，不代表有 2,625 个互不重复的插件。

| 市场仓库 | 市场名称 | 条目数 | 清单 commit |
| --- | --- | ---: | --- |
| anthropics/claude-plugins-official | claude-plugins-official | 315 | `f8c5e7a1dcb71b10cb74586224b5401f3de5db28` |
| anthropics/claude-plugins-community | claude-community | 2,283 | `06ec7b6a0a82bdc4bec44bd82d4368c42f20d7c1` |
| anthropics/skills | anthropic-agent-skills | 5 | `ca45fc496a3436251f191354091d557da31c15b5` |
| obra/superpowers-marketplace | superpowers-marketplace | 10 | `87b5911c829706f0665aab18462c40cd47822835` |
| anthropics/claude-code | claude-code-plugins | 12 | `b3f291d131924810e2b7713ededc144843a229d5` |

清单中的外部 source 有自己的固定 source_commit；不能把清单 SHA 当成所有外部插件代码的 SHA。源漂移时应重新测试相应版本。

附件：

- [marketplace-audit.csv](marketplace-audit.csv)：每个市场条目的来源、固定版本、组件、依赖提示、静态状态与运行状态。
- [marketplace-audit.json](marketplace-audit.json)：完整条目和配置证据索引。
- [audit-method.md](audit-method.md)：扫描方法、覆盖范围与限制。
- `snapshots/`：五份固定市场清单与已读取插件的紧凑组件证据。

**这是逐条登记与分层静态核查，不是每个插件都经过真实运行。** `catalog_only` 只说明读到了清单，`source_unverified` 表示来源读取受限，`static_pass` 也只表示本扫描未发现结构问题。全部条目的 runtime 都是 `not_tested`。不能把未知项算作失败，也不能算作可用。

静态源码覆盖、异常项和依赖统计见本报告末尾的“核查结果补充”；以最终 audit JSON 为可重算数据源。

## 7. 90% 应怎样验收

建议保留两个指标，并且都公开分母：

1. **官方市场实际可用率**：用户已确定仅以默认官方市场为验收范围。按本次冻结的 315 个市场身份，需要至少 284 项通过，才达到 ≥90%。社区市场继续支持，但不计入该指标；不能再临时剔除官方市场中需要登录、付费、Claude CLI 或当前不支持组件的条目来提高通过率。
2. **明确环境下的兼容率**：例如 Windows 桌面或 Linux 无桌面宿主、指定模型/版本与依赖。允许提供“已满足依赖的插件集”指标，但名称和排除清单必须公开，不能替代整体指标。

同一个源码在多个市场重复出现，身份指标可以分别列出，但应同时给出按源码+版本去重的覆盖结果，避免重复热门技能抬高通过率。混合插件中只运行一个 skill，不能代表其 Hook/MCP/agent 都通过。

每个插件至少记录：

| 阶段 | 通过条件与证据 |
| --- | --- |
| 来源与安装 | 固定 commit/ref，来源可访问、路径与组件可加载，依赖闭包完整 |
| 定义 | manifest/frontmatter/hooks/MCP 等解析及合并行为符合冻结规范 |
| 执行 | 每个声明组件实际被触发；Hook 决定及参数修改有效，MCP 完成连接/列工具/真实只读调用，技能/命令达到预定断言 |
| 生命周期 | 启停、更新、卸载、重启、任务取消、进程回收及运行中版本归属正确 |
| 宿主边界 | 本地桌面和远程服务分别执行；客户端断线不终止已经接受的任务 |
| 可复现 | 平台、模型、依赖、凭据是否齐备、日志、配置版本、结果及失败原因可追溯 |

OAuth 插件没有用户完成认证时只能是“待认证”；依赖付费服务而没有账号时是“未验证”；网络、源删除和上游版本变化也要单列。这些因素会使“长期所有插件 90%”成为持续维护目标，而不是一次编译即可保证的性质。

本次没有安装/运行插件，**已实测市场插件数为 0**。这表示运行证据缺失，不表示判定全部插件不可用。

## 8. 建议实施顺序

### 第一阶段：冻结定义与统一执行语义

- 以此次官方文档和真实市场样本建立可复现契约；实现 YAML/frontmatter、manifest、目录发现、source/strict/命名空间。
- 更新技能、命令、Hook 和工具可观察名称及参数结构；统一所有调用方、测试、文档，删除被取代字段。
- 完整实现实际市场使用的 Hook 边界、变量、权限决定、shell 与异步；先用受控进程验证，再跑现成插件。
- 建立 Claude 工具/模型角色对 GCode 核心的明确映射，不能只改提示词或添加字符串别名假装能力存在。

用户这次明确要求 Claude 定义对齐，属于目标契约变更；不需要同时保留旧 GCode Hook JSON 或旧命令参数语义。正常多协议 MCP 与供应商自定义能力继续属于当前业务需求。

### 第二阶段：宿主端插件与市场管理

建议新增职责清晰的 Go 模块：插件包/清单解析、市场来源与拉取、安装仓库与依赖、运行资源投影。通过现有 typed contracts 接入 Engine；桌面本地直接调用 Go，真正远程才使用现有远程适配器。

管理流程至少包括：源增删改/刷新、检索与详情、安装范围、启停、更新/卸载、版本和安装状态、依赖/认证/错误状态、本地目录试装。区分“市场已收录、已安装、启用、组件可加载、真实验证通过”。

配置、缓存、更新及 MCP/Hook 进程归执行宿主；Windows 客户端连接 Linux 服务时，应下载和执行 Linux 可用版本，并在服务上处理依赖。不能由客户端的 Windows PATH 或本地安装结果推断远程可用。

更新采用固定版本与原子切换；运行引用旧版本时保留它直到任务结束。来源权限、路径越界、符号链接、恶意 archive、凭据、依赖循环和重复身份需要安装层处理。它们是实际执行外部代码的产品边界，不是市场文案问题。

### 第三阶段：真实插件回归，再补市场 UI

- 先以官方冻结清单逐条建立真实验收状态，优先补会影响最多官方条目的运行能力；社区另外记录支持状态，不影响官方 90% 指标。
- 按纯技能、带动态命令、Hook、stdio MCP、OAuth/HTTP/SSE MCP、agents、LSP 选择覆盖组，随后扩展到逐插件真实验收。
- `plugins/automation` 验证原生桌面安装管理交互，Go 集成测试验证核心；Linux 无桌面宿主另验。
- UI 使用现有 SettingsPage、共享控件和主题；呈现来源、版本及必要的阻塞原因，避免堆叠不必要标签。此阶段不恢复全局视觉优化。

这里的主要成本是执行语义与跨环境验收，市场列表和安装按钮只占其中一部分。OAuth、Windows 脚本环境、模型角色映射和高级插件组件都可能成为影响 90% 的主变量。

## 9. 本次已有验证

实际运行：

```powershell
go test ./internal/contracts ./internal/agent ./internal/engine ./internal/mcp -run 'Test(User|MCP|Hooks|Hook|RunTriggers|Prompt|QueueKeepsReference|GoalRejectsInvalid|GoalPreservesInline|StreamableHTTP|StdioMCP)' -count=1 -timeout 4m
```

四包通过，覆盖已有资源、Hook、stdio/Streamable HTTP MCP 等隔离测试。它们不包含真实市场插件、真实 OAuth、真实供应商、跨系统运行或桌面市场页面验收。

本次没有改动原有应用和参考仓库。新增交付物仅位于本调查目录。

## 10. 核查结果补充

最终扫描针对官方、skills、obra 和 claude-code 四源共 342 个条目尝试读取源码；成功 337 个，另外 5 个受下载预算限制。社区源对同源码、同版本、同路径的 23 个条目复用源码证据，其余 2,260 个仅完成清单检查。总计 **360 条市场记录有源码检查证据，真实运行数仍为 0**。

| 状态 | 条目数 | 含义 |
| --- | ---: | --- |
| static_pass_with_runtime_requirements | 354 | 本扫描未发现结构错误，但发现组件、环境或依赖文本线索；不证明满足真实运行条件 |
| static_pass | 5 | 本扫描未发现结构问题或相关线索；不等于完整 schema 验证通过 |
| static_findings | 1 | 没有发现本扫描支持的插件组件，需要进一步人工判断 |
| source_unverified | 5 | 受下载大小预算限制；不能据此判断源或插件坏了 |
| catalog_only | 2,260 | 只有清单登记和字段检查，未读取源代码 |
| 合计 | 2,625 | 运行状态全部为 not_tested |

官方 315 个条目中读取源码 310 个，5 个未知为 desktop-commander、hyperframes、salesforce-development、sap-mdk-server、zscaler。未知原因是调查下载预算，不是可用性测试失败。

obra 市场的 private-journal-mcp 没有发现标准 Claude 插件组件。它可能主要交付 MCP server，需要核实其插件接线方式，不能直接宣布该 MCP 服务不可运行。

直接读取源码的 337 个条目中，组件出现条目数如下；同一条目可包含多个组件：

| 组件 | 条目数 |
| --- | ---: |
| skills | 268 |
| commands | 61 |
| agents | 49 |
| hooks | 71 |
| MCP | 172 |
| LSP | 14 |
| settings | 1 |

其中 24 个条目的 Hook matcher 明确涉及 Bash/Edit/Write，100 个条目出现 CLAUDE 环境变量文本。Hook 配置已出现 SessionEnd、UserPromptExpansion、CwdChanged、Elicitation、FileChanged、Compact 和子智能体等事件，证实扩展七事件是实际市场需求。

扫描还发现 Bash、Python、Node、凭据、Claude CLI 等文本线索。**这些是文本匹配，不是已确认的必需依赖**；README 的安装示例、开发说明和可选路径都会产生误报，因此不能用这些计数预测 90% 成功率。

五源共有 2,444 个不同插件名称、2,380 个不同 source repo/path 组合；这些也不是严格的功能去重结果。正式验收仍需按具体源码版本和行为建立身份。
