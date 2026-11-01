# 冻结官方插件业务探针

这些文件是待执行计划，不是通过记录。真实模型请求、组件触发、文件结果和账本由
`corerunner` 与主验收流程产生；本目录不包含供应商配置、凭据或预填的运行成功结果。

来源固定为 `anthropics/claude-plugins-official` 的
`f8c5e7a1dcb71b10cb74586224b5401f3de5db28`。该版本的市场快照和逐插件来源证据在
`docs/investigations/claude-plugins-2026-10-30/snapshots/`。
本次用 `git show` 阅读冻结组件全文，以 `git archive` 将三个插件解到独立临时目录，
再实际调用 `plugins.Load` 和 `acceptance.ComponentsForBundle`。
`discovered-components.json` 保存此次真实发现的名称、ID 与组件摘要；
不是根据文件名猜测 ID，也不是运行证据。临时源文件已清理，缓存工作树未修改。
第二批五个候选的精确组件 ID、digest 和来源直接取自完整账本
`output/plugin-acceptance/run-3847800893/ledger.json`，
并阅读该账本中冻结安装目录内的实际技能全文和相关模板/验证参考。

每个 JSON 的 `pluginId/sourceCommit` 及组件全集都与该发现结果一致：

| 插件 | 组件 | 业务结果 |
| --- | --- | --- |
| frontend-design | 1 个 skill | 实际生成自行车修理门店页面，验证三项服务及价格、地址、语义结构和响应式基本条件 |
| claude-md-management | 1 个 command、1 个 skill | 精确修改已批准的项目指令；另在独立工作区发现错误测试命令并生成逐文件质量报告 |
| code-simplifier | 1 个 agent | 实际子 Agent 简化运输费函数，保留 12 个边界/严格布尔行为，运行不可改写的 Node 验证器 |
| claude-code-setup | 1 个 skill | 只读分析实际脚本、格式配置和敏感文件边界，提出三项有项目依据的 Hook 推荐，原文件逐字不变 |
| math-olympiad | 1 个 skill | 走既有证明审查路径，由两个独立纯推理子 Agent 找出素数多项式错误证明的具体反例与逻辑漏洞 |
| skill-creator | 1 个 skill | 创建可复用费用 CSV 技能初稿、真实 Decimal 汇总脚本和待评审的测试提示，执行四种真实 CLI 数据用例 |
| playground | 1 个 skill | 生成单文件卡片设计工具，执行页面同一纯计算函数的四组输入，检查三种预设、样式输出、提示和控件绑定 |
| project-artifact | 1 个 skill | 走源中明确的 headless 本地保存路径，从非软件项目笔记生成状态页，对照表格、状态 JSON、负责人、依赖和转义 |

runner 为每个组件建立独立数据目录和工作目录，所有 fixture 路径都必须位于该目录内。
这些插件均没有声明 Hook、MCP、额外账号配置或依赖组件，所以 `allowHooks` 为空。
计划禁止联网、安装依赖、提交代码及 GitHub 操作；模型请求由主流程显式指定的供应商配置提供。
这些任务指令不构成 OS 网络或文件权限沙箱，实际宿主权限边界仍按核心实现执行。

每个探针通过 `allowCommands` 声明显式允许的完整 Bash 命令：
frontend-design 仅允许 `python verify_frontend.py`，
code-simplifier 仅允许 `node verify_shipping.mjs`；
skill-creator 仅允许 `python verify_skill.py`，
playground 仅允许 `node verify_playground.mjs`，
project-artifact 仅允许 `python verify_artifact.py`。
claude-md-management 的两个探针、claude-code-setup 和 math-olympiad 均为空，不允许 Bash。
不允许添加命令前后缀、重定向或其他 shell 操作。
命令实际成功执行后，其 Bash 工具输出还必须满足业务断言。

`verifierOutputs` 列出仅允许验证器生成的结果文件：
`frontend-verification.json`、`shipping-verification.json`、
`skill-verification.json`、`playground-verification.json` 和 `artifact-verification.json`。
模型工具不得读取、写入或编辑这些文件，runner 直接检查结果。
不可变 fixture 由与原始 fixture 文本完全相等的 `file.equals` 断言标识，
执行验证命令前必须逐字核对；验证脚本属于这一类。
任务目标中的 `shipping.mjs`、已批准修改的 `CLAUDE.md` 和审计报告占位文件则允许按任务修改。
这些字段需要配合 runner 的 ToolGuard 执行，不能仅凭计划中的限制文字认定保护已生效。

## 如何判断结果

- frontend-design 必须生成 `index.html`，再实际执行不可修改的 Python HTML 检查器。
  检查器读取真实服务 article 的 `data-service/data-price` 和可见文字，
  验证三种价格、营业日、地址、导航、viewport、移动媒体查询及 reduced-motion，
  将实际解析结果写为 `frontend-verification.json`。仅口头说完成不能满足断言。
  这不是浏览器截图、视觉品质或参考图一致性验收。
- claude-md-management 的 command 任务事先批准两项精确修改，要求实际使用 Edit，
  保留原指令和源文件；不依赖 runner 自动回答确认问题。
  skill 任务只生成质量报告，不允许修改任何 CLAUDE.md；
  必须指出 fixture 中 `npm test` 过时，并从实际 Makefile 找出两个 Go 命令。
  这分别验证执行修改和只读审计路径，不证明全部交互确认行为都已覆盖。
- code-simplifier 必须启动实际命名子 Agent。Node 检查器导入修改后的模块，
  验证非正数、整数、小数及 `express === true` 的原行为，并拒绝保留原有嵌套 else/
  冗余累加变量；真实计算结果写入 `shipping-verification.json`。
  验证器自身必须逐字保持不变。仅写报告或输出 PASS 不能满足断言。
- claude-code-setup 的技能明确只读，不要求它写报告或实施建议。
  必须真实读取项目文件，输出与已有脚本一致的格式化、lint 和 `.env` 保护建议；
  实际 Read 内容、推荐中的具体命令/事件/证据路径，以及全部原文件不变共同构成断言。
- math-olympiad 检查“有限取值为素数且一直为奇数，所以对所有正整数为素数”的错误证明。
  必须有实际 Agent 输出的两次 INCORRECT，以及具体 `1681`、`41` 和反例 `n=40`；
  不用 Python 代替技能要求的纯推理，不声称完成新题的完整多候选解题/PDF 工作流。
- skill-creator 使用源中允许的“先做初稿，不运行完整评估循环”范围。
  校验器实际执行生成脚本，验证带逗号的 CSV 分类、负数退款、空表、
  精确十进制 `0.10+0.20` 及非法金额退出行为，并检查技能元数据和至少两个测试提示。
  这不证明模型对照评估、人工反馈、benchmark viewer 或描述优化已运行。
- playground 的 Node 验证器执行 HTML 中同一脚本的实际 `cardModel` 函数；
  DOM 初始化通过 `typeof document` 分支避开，不注入假 DOM。
  它核对 CSS 值、非默认提示内容、预设和静态事件/复制绑定；
  浏览器渲染、控件实际交互和剪贴板权限仍需浏览器验收。
- project-artifact 的源明确允许无交互环境先保存 HTML，留待交互会话发布。
  校验器从真实 HTML 表格及嵌入状态读取三项工作流，对照负责人、状态、依赖和时间，
  并拒绝把源文本 `<Q4>` 解释为 HTML、把原始 `<` 放进状态 JSON 或遗留模板占位符。
  不调用 Artifact，不产生虚构发布 URL，不证明 claude.ai 登录、发布或跨版本刷新已通过。

宿主额外工具：frontend-design 计划需要 `python` 的 Python 3 标准库；
code-simplifier 计划需要带 `node:assert/strict` 的 Node.js；
claude-md-management 计划不运行 Go 编译器或外部账号命令。Shell 与搜索仍需要核心声明的
Git Bash/bash 和 rg。缺少这些依赖应记录阻断，不能改成模拟输出。

2026-10-30 已检查 JSON、精确组件覆盖和不可变 verifier 的语法；
独立构造的错误/原始输入会被 verifier 拒绝，独立构造的正确结果可通过。
这些检查只验证计划和检查器，没有调用模型、运行插件或增加通过数。
第二批三个新验证器也已实测：缺少目标产物被拒绝，独立构造的正确参考产物通过，
错误业务结果被拒绝，输出报告与断言逐字相同。
完整计划的组件覆盖和 commit 均与上述完整账本一致；
`go test ./scripts/plugin-acceptance/corerunner -count=1` 通过。
实际插件模型运行与通过数只能从随后产生的运行证据和完整账本读取。

## code-review 暂不提供无账号通过计划

已读冻结的 `plugins/code-review/commands/code-review.md` 全文。
它要求通过 gh 查询真实 PR 的关闭/草稿/既有评论状态、拉取差异、调用多种角色 Agent 并复核。
`--comment` 还会触发外部发布，源文件中的这段要求不提供发布授权。

当前没有为此探针指定 GitHub 凭据和真实待审 PR；runner 的独立环境也不继承用户账号。
因此不创建假 gh、不把本地补丁审阅改写成官方 PR 工作流通过，也不写带虚构 ID 的计划。
后续可在明确指定真实 PR、配置只读认证及禁止发布的范围后设计独立计划；
在完整工作流被实测前，这项仍留在官方市场分母中。
