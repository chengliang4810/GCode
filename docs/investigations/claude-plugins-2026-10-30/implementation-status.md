# 插件功能实施与验收状态

更新时间：2026-10-30。调查基线、公开资料和参考代码差异见同目录 `report.md`。
本文记录实施证据，不替换冻结市场清单，也不把开发测试写成公开插件通过。

**目标已按用户要求暂停。** 本文中的执行中事项不再继续；
最新结果、未测补丁和恢复待办以同目录 `PAUSED.md` 为准。

## 已接入的业务路径

- 原生 MyGo 插件页通过类型化核心命令维护来源、目录、安装范围、配置、启停、
  更新和卸载。本机不经过 HTTP/stdio bridge；远程宿主复用同一业务核心。
- 内置官方、社区两条来源。网络和 Git 操作在核心工作线程执行，界面接收状态。
  安装发布不可变版本目录，运行持有启动时快照。
- Claude Markdown/YAML、插件 manifest、市场条目、依赖、命名空间和作用域进入
  统一资源加载器。个人、项目和插件技能/命令使用 `Skill` 与 `/name` 运行入口。
- Hook、动态上下文、stdio/HTTP/SSE MCP 和插件子智能体已接实际执行路径。
  MCP 连接通过协议发现选择当前支持的协议版本。
- 内置 `general-purpose`、`Explore`、`Plan`、`claude`。
  `sonnet/opus/haiku/inherit` 继承有效模型；指定类型的用户模型与推理配置优先，
  不新增角色映射设置。Explore/Plan 保持只读工具边界。

完整格式、路径、工具和未支持组件见 `docs/plugins-format.md`、
`docs/plugins.md`、`docs/hooks.md`、`internal/mcp/README.md`。
声明了事件或保留了字段，不代表相应宿主能力已经完成。

## 已验证的路径

原生窗口管理流程最新重建复验通过证据：
`output/plugin-settings-final-native/20261030-191614-29241047/evidence.json`。
它使用独立应用、数据目录与临时 Git 市场，实际添加/刷新来源、local 安装、
配置取消/保存/清除、更新到 1.1.0、禁用、重启与卸载。
此用例不访问公开市场，不计入官方插件通过数。

核心集成测试实际运行临时插件的技能、Hook 子进程、stdio MCP、配置过的子智能体、
参数改写及文件写入。真实 loopback modern MCP 测试覆盖生产连接与调用路径。
确定性模型服务用于验证协议和状态，不充当外部模型验收。

`go test ./... -count=1 -p 2 -timeout 180s` 已通过完整仓库复验，
日志为 `output/plugin-acceptance/final-go-test-20261030.log`；
`go vet ./...` 和 `go tool mygo vet .` 也通过。
随后真实原生复验暴露了 Windows 深目录下 Git pack 长路径错误，
记录为 `output/plugin-settings-final-native/20261030-191200-2379312586/evidence.json`。
失败证据保留；修复后重新构建并复验原生流程已通过，见上面的最新记录。
生产 Git 命令统一使用每命令长路径设置与隐藏控制台，仅 fetch 工作仓库放独占系统
临时目录，最终安装目录不缩短。真实 Windows 303 字符目录的导出、刷新、安装和加载
回归通过，日志为 `output/plugin-symlink-review/windows-production-git-longpaths.log`。
Linux/amd64 服务与核心测试二进制已交叉构建，并在 Debian WSL2 中实际执行：
技能/Hook/MCP/配置子智能体、fork、敏感输出和 ToolGuard 专项通过，
日志为 `output/plugin-acceptance/linux-core-tests.log`。
真实无图形服务进程还通过了认证、默认两市场来源、远程来源添加/刷新、
实际安装/禁用/卸载；未认证请求实际返回 401。
证据为 `output/plugin-acceptance/linux-host-smoke.json` 及同名 `.log`。
这是 WSL2 Linux 运行证据，不等同于其他 Linux 发行版或真实外部供应商验证。
最新代码再次交叉构建并实际执行的记录为
`linux-plugins-final.log`、`linux-engine-final.log` 和 `linux-host-smoke-final.json`
（均位于 `output/plugin-acceptance/`）：插件全包、执行核心专项及无图形宿主通过。
Git 长路径修复后再构建的 Linux Git 专项及无图形宿主也通过，
记录为 `linux-git-longpaths-final.log` 和 `linux-host-smoke-longpaths-final.json`。

独立审阅发现后补充了以下实际回归：

- 动态输出和结构化结果的敏感值脱敏，包含默认值与数组成员。
- MCP 配置值单次展开，保留值内的 `$`；PowerShell 配置通过环境值引用。
- 普通技能重复调用幂等；fork 局部规则与禁用工具在子运行生效，返回后释放。
- 动态上下文遵守当前运行的工具限制，不能绕过父技能或子智能体限制。
- 项目资源覆盖个人同名资源，普通 `/name` 可以调用个人技能。
- 会话后台任务保留至显式停止、会话删除或宿主关闭；完成结果有界保留。
- 小数秒 Hook 超时、实际 SubagentStart/SubagentStop 和手动压缩生命周期。

## 真实模型证据

用户授权使用既有 `local-gateway/deepseek-v4.1-flash`、`max` 配置。
只读导出用于独立测试核心，未改原用户配置。私有配置不纳入仓库、报告和账本。

临时 `provider-smoke` 插件第一次运行只返回单字 `I`，没有调用工具，正确记为失败：
`output/plugin-acceptance/run-2933970541/`。
第二次实际完成技能事件、6 次 HTTP 模型往返与工具执行，
`answer.txt` 的实际内容为 `51`：
`output/plugin-acceptance/run-654232493/`。
保留两次证据；尚不能由一次失败推断网关或解析器存在确定性故障。
这些是验收驱动的真实模型证据，**不是官方插件通过数**。

官方真实业务运行已经验证了 `claude-md-management` 的命令修改和技能审计，
以及 `code-simplifier` 的实际子智能体与 12 项 Node 行为保持检查。
`frontend-design` 复测实际触发技能后达到模型输出上限；不完整的工具调用未执行，
因此保持失败。当前测试沿用 `max` 推理；用户未显式配置输出上限时，
本机模型适配器默认单次输出预算为 8192。没有增大预算后冒充原配置通过。
这说明虚拟角色映射能够复用模型，但不会自动获得 Claude 的能力或输出预算。
具体通过数以完成身份校验后的完整 315 项账本为准。

用户随后指定检查本地 Magpie；真实服务、路由和截断证据见 `magpie-check.md`。
已明确区分网关声明的 128000 输出能力与 GCode 请求中的 8192 默认预算。
32768 隔离预算复测正在进行；不修改日常配置，不覆盖原预算失败记录。

## 官方市场验收

分母固定为官方市场 commit
`f8c5e7a1dcb71b10cb74586224b5401f3de5db28` 的 **315** 项，
达到 90% 需要至少 **284** 项完整真实运行通过。社区不计入分母。

已下载并核实 236 个来源仓库与固定提交组合，包含源码快照中的 231 个组合及
官方清单另行固定的 5 个提交。缓存仅包含公开 Git 对象，没有执行插件脚本。
缓存索引和下载记录：
`output/plugin-acceptance/source-cache/frozen/{index.json,fetch-results.json}`。

修复 Git 子目录与来源物化前的完整账本为
`output/plugin-acceptance/run-3847800893/ledger.json`：
来源验证 295/315，安装发现 285/315，实际运行 2 通过、1 失败、312 未测试。
安装发现的 90.48% 不等于用户要求的 90% 运行可用。
修复后的完整安装统计与隔离预算运行会写入新账本，并保留旧记录。

当前共有 8 个官方候选、9 个组件的明确业务计划：
frontend-design、claude-md-management、code-simplifier、claude-code-setup、
math-olympiad、skill-creator、playground、project-artifact。
计划读取冻结实际源码、绑定准确组件身份，并验证实际文件、计算结果或执行事件。
计划文件不是通过记录，局部流程的通过也不能证明计划中明确排除的外部发布或浏览器行为。

静态快照中有 105 项声明远程 MCP 服务。声明本身不证明需要认证或能够连接；
真实服务账号、授权和业务数据须逐项验证。模型可调用不能替代第三方服务授权。
未知、认证缺失、失败及不支持组件仍留在 315 的分母中。
当前不能声称达到 90%，也不会通过剔除这些项目提高数字。

旧验收运行中曾保留含测试模型配置副本的私有 Home。自动安全审查拒绝了对
10 份配置副本的删除，理由仅为 `blocked by policy`；未换工具绕过。
随后逐一核实其 APIKey 全部为本地网关占位值，未发现上游供应商密钥，
不能将此事描述为厂商密钥泄露。原始用户配置未改动，路径及处理状态另记录在
`private-artifact-cleanup.md`。新验收器关闭其核心和 Store 后清理本次拥有的
私有 Home，并将清理失败作为运行错误；脱敏证据与业务文件独立保留。

## 文档与代码差异处理

旧 `docs/user-settings.md` 中“仅用户资源”“技能正文用 read 加载”
“一基参数索引”“没有动态 shell”“七个 Hook 事件”“process 类型”等描述，
已按当前实现更新。历史日期下的测试结果保留为历史证据。

公开格式还区分市场根 entry 的显式 skills 与 manifest 的追加 skills；
entry Hook 按事件覆盖。安装元数据显式保存来源，加载时据此判断，
不根据目录里碰巧存在市场文件来猜测。其他参考文档/代码的逐项差异仍见调查报告。

## 2026-10-31 恢复轮：安装发现 292/315，运行通过 7/315

用户明确指示恢复后，按暂停清单完成六项，全部证据保留在
`output/plugin-acceptance/`：

1. **安装发现重刷与 Windows rename 修复。** 首刷 `run-3337950432` 得 282/315，
   10 项失败为 `rename: Access is denied`——杀毒或系统对刚关闭的源/目标目录
   短暂持句柄。新增 `internal/plugins/rename.go`（仅 Windows、仅
   ERROR_ACCESS_DENIED/SHARING_VIOLATION/LOCK_VIOLATION 的有界重试，注入式
   单测），接入 `manager.go` 的 `publish` 与 `save` 两处；重刷
   `run-1069445614` = **292/315 安装发现**。剩余失败为 20 项来源缓存缺口与
   3 项插件清单自身缺陷（circle-skills、forge-skills、informatica 的远程 MCP
   声明缺 command/URL，与旧账本一致，非回归）。
2. **Agent 子代理块检查器修复。** math-olympiad 三条 `toolName: "Agent"`
   文本断言失败的根本原因：核心在子代理完成时把通用 Agent 调用块替换为
   `subagent` 块（`subagentResult` 的 `child_session_id/status/summary` 文本），
   而 `completedTools()` 只收集 `tool` 块。`corerunner/proof.go` 现按核心实际
   交付格式重建完成的 Agent 结果，跳过运行中的子代理；`proof_test.go` 覆盖。
   真实重跑 `run-4067267622` 通过。
3. **Windows CRLF 检查器环境修复。** skill-creator 的 `skill-verification.json`
   字节级断言失败：Python `write_text` 在 Windows 把 `\n` 写成 `\r\n`，
   验证脚本 4 个真实 CLI 用例实际全部通过、内容语义正确。计划验证脚本改为
   `write_bytes`（fixture 与断言同步），project-artifact 同样预防性修复。
4. **playground、project-artifact 首次真实运行通过**（run-4067267622）。
5. **skill-creator 两次网关退化响应。** run-4067267622 第二次调用与
   run-940778921 唯一调用均为 SSE `end_turn` 但 `usage` 全 0（上游未处理输入）
   的退化响应；同会话其他探针与 5148 token 直接请求健康。第三次有界重试
   `run-3974622327` 全程 6 次调用 usage 正常并通过。判别签名
   `input_tokens: 0` 已记录，不再盲目重试。
6. **边界能力现状确认。** LSP 已解析未连运行时（engine 显式
   "LSP runtime is not connected"）；输出样式入包未接选择；channels 仅声明；
   `.mcpb/.dxt` 显式不支持；MCP stdio 直接执行配置命令、无 npm/pip 自动安装。

**最终合并账本 `run-873998936`**：最终代码安装发现 292/315，实际运行
**7 通过 / 1 失败 / 307 未测试**（2.22%，距 284 目标差 277）。通过：
claude-md-management、code-simplifier（run-2119488756 导入）、
claude-code-setup（run-100502632 导入）、math-olympiad、playground、
project-artifact（run-4067267622）、skill-creator（run-3974622327）。
失败：frontend-design（run-100502632 导入，32768 预算 `stop: length`；
8192 预算失败保留于 run-2119488756，每插件唯一证据按导入约束只入一条，
两份原始 run 均在盘）。

技能调用 MCP 的精确只读许可变更（`guard.go`/`guard_test.go`/`plan.go`）
仍由 claude_tools 独立持有，本 checkout 未交付，未在本轮重复实现。

全仓测试 `output/plugin-acceptance/full-repo-tests-20261031.log`：本轮修改
涉及包全部通过；cmd/rcode 与 internal/desktop 的两个 `[build failed]` 复跑
`go build` 干净通过（构建期瞬时问题）；`internal/terminalhost` 的
`TestRealShellInputResizeInterruptDetachAndExit` 连续失败属未提交并行工作，
非本轮引入，保留并如实上报。

## 2026-10-31 口径 2：安装发现达标 292/315，无账号池 9/10（90%）

用户确认 284/315 全量运行目标在"缺授权项保留阻塞"前提下算术不可达，
选定方案 2：安装发现 ≥90% 为主指标（已达成），真实运行改为"无账号可测池"
滚动验收。分类 `output/plugin-acceptance/pool-classification-20261031.json`：
112 项纯本地可测、100 项本地+服务混合、26 项纯远程服务、33 项授权阻塞、
23 项安装失败（20 来源缺口 + 3 清单缺陷）、13 项纯 LSP、8 项已测。

按新口径完成首批池扩容：carbone-skill（Carbone 模板语法授权，断言锚定
技能文档的循环 `[i+1]`、`:mul`/`:formatN`、`:showBegin/:showEnd` 并拒绝
Handlebars/Jinja/企业语法）与 ckeditor（GPL Classic 集成，断言单包模型、
`ClassicEditor.create`、CSS、GPL key、插件与工具栏匹配、无 premium 导入）。
两份计划验证脚本先在合成产物上自测字节精确，再真实运行一次通过
（run-365400648）。经验：非市场仓库来源的插件，计划 sourceCommit 必须用
插件固定来源提交（carbone=25207b3f…、ckeditor=261766a0…），用市场提交会
被计划校验拒绝为 blocked。

最终合并账本 `run-1647007110`：安装发现 292/315（92.70%）；运行
**9 通过 / 1 失败 / 305 未测试**，全部 10 项属无账号池，**池通过率
9/10（90.0%）**；唯一失败 frontend-design 为两种输出预算下 `stop: length`
（预算耗尽），非插件缺陷。n=10 样本尚小，池验收按批滚动，每批导入合并
账本并更新通过率。
