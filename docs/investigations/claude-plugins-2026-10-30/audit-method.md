# Claude 插件市场逐项静态盘点（2026-10-30）

## 结论与边界

这是公开仓库固定 commit 的只读静态调查，不是安装验收。没有安装插件或依赖，没有执行插件命令、hook、MCP、LSP，没有调用外部业务服务。`runtime` 全部为 `not_tested`。不能由本结果得出“90% 运行可用”。

完整记录 5 个市场的 **2,625 个市场条目**：按名称去重为 2,444 个；按 GitHub 仓库与路径去重为 2,380 个；按已声明/解析 commit 加路径去重为 2,533 个。这三种都不是无争议的“独立插件总数”，因为同名可能不同实现，同一仓库同一路径不同 commit 也可能内容不同。

四个较小市场共尝试源码检查 342 条，成功读取 337 条（328 个不同 repo/commit/path），5 条因下载预算未完成。官方社区市场 2,283 条全部完成目录元数据审查，其中 23 条恰好与已检查来源的固定 commit 和路径相同，复用该来源证据；其余 2,260 条只审查目录字段，**尚未读取插件仓库，不能声称已核实可用**。因此全清单共有 360 条有来源证据、2,260 条 catalog only、5 条 source unverified；运行实测 0 条。

## 固定清单

| 市场 | 来源 | 固定 commit | 条目 | 有源码证据 |
|---|---|---|---:|---:|
| claude-plugins-official | anthropics/claude-plugins-official | f8c5e7a1dcb71b10cb74586224b5401f3de5db28 | 315 | 310 |
| claude-community | anthropics/claude-plugins-community | 06ec7b6a0a82bdc4bec44bd82d4368c42f20d7c1 | 2,283 | 23（证据复用） |
| claude-code-plugins | anthropics/claude-code | b3f291d131924810e2b7713ededc144843a229d5 | 12 | 12 |
| anthropic-agent-skills | anthropics/skills | ca45fc496a3436251f191354091d557da31c15b5 | 5 | 5 |
| superpowers-marketplace | obra/superpowers-marketplace | 87b5911c829706f0665aab18462c40cd47822835 | 10 | 10 |

每个清单均取自该 commit 的 `.claude-plugin/marketplace.json`；原始 JSON 保存在 `snapshots/*--marketplace.json`。`anthropics/claude-code` 这 12 条现在全部通过 `git-subdir` 指向官方插件仓库，因此不宜在 UI 当成另一套不重复的 12 个新插件。它们使用的 `main` 在解析时已移到 `95380b3cd58f7fbf575ebda0fe291224ba028317`，与最初市场快照 commit 不同；逐项输出分别保存了 marketplace commit 和实际 source commit，未假设分支不变。

## 逐项文件

- `marketplace-audit.json`：所有 2,625 条的原始市场 entry、固定来源、结构检查状态、组件路径、依赖文字线索、hook/MCP/LSP 配置及证据指针。
- `marketplace-audit.csv`：同样完整行数，便于筛选 unknown、catalog only、MCP、hook 等。
- `snapshots/*--source-evidence.json`：337 条成功深扫的紧凑证据；保存插件路径树、manifest/组件 JSON、Markdown frontmatter 或短文本片段与原 archive SHA-256，不存可执行安装结果。
- `audit_marketplaces.py`：纯 Python 标准库只读扫描器。运行需要 Git 与网络；不会运行下载源码。大型 tar 包只写临时目录，不解包到工作区。

## 静态判断的含义

`static_pass` / `static_pass_with_runtime_requirements` 只表示来源可读取、发现的 JSON 可解析、组件位置可发现；**不是完整 Claude schema 验证，更不验证 hook 输出行为、模型遵循、权限、MCP 协议/鉴权、LSP 或平台可用性**。后者的 runtime requirements 名称只提示后续检查工作，不保证文字提及的依赖一定在执行路径上。

manifest 可以省略；`strict:false` 的市场内联定义按有效配置处理。对陌生字段不报非法，保留 `userConfig`、`dependencies`、`monitors`、`channels`、`settings` 等原始内容，以免用旧版规范错误拒绝新插件。当前公开规范的 skills 自定义路径追加默认 `skills/`；commands、agents 显式路径替换其默认目录发现。本次按这一规则计算候选组件。尚未验证完整的 strict 合并冲突、schema 约束、跨文件 include、符号链接和运行时动态生成配置。

识别相对目录、GitHub URL、GitHub `owner/repo` 简写与 `git-subdir`。实际市场还出现 `source:"url"` 同时带 `path`：官方 `atomic-agents` 与 `zilliz`。本次按指定子目录读取以避免误报空插件，并在风险字段保留这一组合；宿主实现必须明确核实支持语义，不能只处理 git-subdir 的 path。

GitHub API 在本次网络出口返回 403 rate limit exceeded，所以来源固定使用 `git ls-remote`，清单用 raw.githubusercontent.com，源码用 codeload.github.com。最多 5 个并发，单个请求超时 35 秒、Git 40 秒；源码成员只读 UTF-8 文本，每文件上限 512 KiB、每仓文本预算 32 MiB。初探压缩包预算为 40 MiB；最终预算收紧为 25 MiB，原已缓存的两份 25–40 MiB 源也从最终已核实集排除。下载限额是调查资源边界，不代表插件坏了。

## 范围内组件与兼容风险

以下分母均为 **337 个直接成功读取的市场条目**，含同实现跨市场重复，不能外推 2,625 条总体：

| 发现组件 | 条目数 |
|---|---:|
| skills | 268 |
| commands | 61 |
| agents | 49 |
| hooks | 71 |
| MCP | 172 |
| LSP | 14 |
| settings | 1 |

组件数是 Markdown 条目/配置位置计数，MCP 配置内可能有多个 server，因此不能把一份 `.mcp.json` 说成只有一个 server。

100/337 条（29.7%）的源码/文档/测试里出现 `$CLAUDE_*` 变量线索。89 条提到 `CLAUDE_PLUGIN_ROOT`；还涉及 `CLAUDE_PLUGIN_DATA`、`CLAUDE_PROJECT_DIR`、`CLAUDE_ENV_FILE`、`CLAUDE_SKILL_DIR` 等。文字提及不等于运行必需，也可能只是教程。

71 条发现 hook 配置；其中 69 条出现 command handler、2 条出现 prompt handler；有 24/71 条（33.8%；占深扫条目 7.1%）的实际 hook matcher 包含 `Bash`、`Edit`、`Write` 之一。25 条的 hooks/ 脚本含 `tool_name` 读取；脚本文本中 Bash/Edit/Write 大写名分别出现于 26/23/24 条。GCode 的小写工具名若原样投入 hook matcher 与输入 JSON，会漏匹配；需要完整的 Claude 语义事件边界与输入/输出映射，而不是简单加载 JSON。

实际 hook 配置里不止传统 4–5 个事件，还出现 `PermissionRequest`、`PostToolUseFailure`、`StopFailure`、`UserPromptExpansion`、`FileChanged`、`ConfigChange`、`CwdChanged`、`Elicitation`、`ElicitationResult`、`InstructionsLoaded`、`PermissionDenied`、`PostCompact`、`TaskCreated`、`TaskCompleted`、`PostToolBatch` 等。出现于仓库不自动证明它们都已是稳定公开规范；应分别对照当前官方文档及实际产品支持。

依赖文字线索包括 bash 277 条、node/npm/npx 239 条、python 182 条、git 158 条、jq 141 条、gh 73 条、uv/uvx 50 条；150 条提到 claude CLI 调用模式，239 条提到 key/OAuth/token，199 条提到付费/订阅/额度。**这些是全文启发式命中，包含 README、教程、示例和测试，不能等同真正硬依赖或必须付费。** Windows 上 shell、命令名、路径及安装方式仍须另行验证。

## 明确异常与代表性证据

- `superpowers-marketplace:private-journal-mcp` 固定到 `obra/private-journal-mcp@016953f24a73dabb8a68569aecf4ebd89c74d70b`。其来源可访问，但没有发现 manifest、默认 skills/commands/agents/hooks、`.mcp.json` 或 `.lsp.json`，市场条目也没有 MCP 内联配置。它是 MCP 项目不等于这个 entry 能直接作为 Claude 插件安装使用；保留 `static_findings`，未执行安装，不做运行必坏的结论。
- 官方 `desktop-commander`、`hyperframes`、`sap-mdk-server` 超初探 40 MiB 预算；`salesforce-development` 约 29.7 MiB、`zscaler` 约 32.0 MiB，超过最终 25 MiB 预算。五者均为 `source_unverified`，并非 5 个坏插件。
- 官方 `security-guidance` 有 `Bash`、`Edit|Write|MultiEdit|NotebookEdit` matcher 与 Python 执行，具体见对应 source-evidence JSON；Claude Code 镜像市场所指的另一个 commit 有不同配置，证明不能仅按名称去重复或猜测版本相同。
- `ralph-loop` 包含 Stop hook、bash/jq 脚本与会话状态变量；只做 slash command 展开无法实现其循环控制语义。
- `superpowers` 包含 15 个默认 skill 和 SessionStart hook；市场源与官方源所选 commit 不同，版本需要独立追踪。
- 官方 `context7` 是 MCP 类案例，来源 JSON 能读不代表网络、认证和 tool 调用已经可用。
- `anthropics/skills` 5 条均使用仓库根路径、`strict:false` 和内联 skills 数组。按当前公开的“附加默认 skills”语义，会发现仓库默认 skills 集，不应把 manifest 缺失误判为坏插件；实际加载与隔离行为尚未执行验证。

## 默认源建议与 90% 验收

推荐内置 `anthropics/claude-plugins-official` 与 `anthropics/claude-plugins-community` 两个不同层级的来源：前者为主要发现入口，后者标为社区来源；内置市场条目不等于默认安装或自动执行所有插件。`anthropics/skills` 可作为官方技能集补充入口；`obra/superpowers-marketplace` 是规模适中的独立社区补充。`anthropics/claude-code` 当前作为引用入口即可，避免重复放大库存。供应链来源标识、可执行权限、网络/凭据要求和实测等级应独立呈现。

若“90%”覆盖全部 2,625 个市场条目，最低应有 2,363 个通过定义明确的运行验收。若按 2,444 个独立名称则至少 2,200 个；这不是可随意互换的口径。需要先冻结市场版本、来源 commit、去重策略、Windows/Linux 范围、支持的组件范围和外部凭据条件，再逐项执行隔离安装/发现/最小业务动作/卸载测试。当前运行实测为 0，不能承诺已达标。
