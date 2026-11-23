# ZCode `/workflow` 复刻规格调查

调查日期：2026-11-19。目标：为 GCode 后续复刻 Dynamic Workflow 建立功能、布局、显示逻辑及验收清单。**本轮只更新参考代码和编写调查，不实现产品功能，不恢复已暂停的全局视觉优化。**

## 1. 基线与证据边界

- 参考仓库：`/Users/chengliang/code-repositories/ZCode`。
- 调查开始时：`main / 3.14.0 / 872ad960de7ec172591f7e1952f7849229f94521`，工作区干净。
- 本次确认的最新远端：`main / 3.18.2 / fec975905a2abd4c07e35c40d51fa097a2258ec9`，提交时间 `2026-11-13 16:21:37 +0800`。
- 更新已完成：通过用户提供的本机 `127.0.0.1:7890` 代理执行 `fetch origin main` 和 `merge --ff-only origin/main`，由旧 HEAD 快进 9 个提交至上述 SHA。旧 HEAD 是新 HEAD 的祖先；`HEAD...origin/main` 为 `0 / 0`；参考仓库工作区干净。代理仅用于本次命令，没有写入全局或仓库 Git 配置。
- 基线检查：更新后 `node scripts/check-workspace-freshness.mjs --no-fetch` 通过，ahead 0 / behind 0。此前直连网络下的超时不记作成功证据。
- 这是**静态源码调查**，不是实窗、真实供应商、远程宿主或像素一致性验收。没有安装 ZCode 依赖、启动其应用或运行其产品测试。关键新版行为先以固定 SHA 官方原始文件交叉检查，再以更新后的本地源码复核。
- GCode 现有 `internal/desktop/zz_accent_probe_test.go` 及调查期间出现的 `internal/model/failure.go`、`internal/model/retry_test.go` 改动不属于本次任务，均保留不动。

历史调查保留，不混用基线：

- [3.16.2 功能调查](zcode-workflow-2026-10-30.md)。
- [3.17.1 与 moejs / JSON 实验](dynamic-workflow-moejs-json-2026-10-30.md)。该文记录的是独立实验，不是已接入 GCode 的功能。

后文参考路径均相对于上述 ZCode 仓库；实施时应固定本页 SHA，不依赖会继续变化的 `main`。

## 2. 先明确复刻的是什么

`/workflow` 不是独立路由，也不是 n8n 式拖拽 DAG 编辑器。它是一个 **prompt 型内置命令**，引导主 Agent 加载 dynamic-workflows skill、先设计子 Agent 拓扑与上下文，再编写 TypeScript 并调用 `CreateWorkflow`。

最新版命令已从插件迁到 CLI 内置代码，插件被停用也不应让命令消失。用户或插件同名命令不能覆盖它。不能照旧本地 3.14 的插件入口实现。

完整闭环：

```text
对话输入 /workflow 或选择菜单入口
  → 主 Agent 设计、编写脚本
  → 草稿阶段线 / 编译反馈
  → 用户确认（可改模型、并发）
  → 后台运行、Actor 会话、问题升级、留白补全
  → 对话运行卡 + 状态胶囊 + 侧栏运行行
  → 运行详情 / 子会话 / 脚本步骤 / 产物
  → 保存定义 / 历史 / 再次运行
```

同时排除两套同名旧体系：旧 `Workflow` / `.workflow.js` 和 CLI `/expert` 不是本次目标。不能拿固定阶段模板或普通一次性 `Agent` 工具代替 Dynamic Workflow。

**关键心智模型：执行的是程序，图是程序的展示投影。** 条件、循环、并行、异常及动态补全由脚本控制；不能反过来把画面上的站点当成全部调度事实。

### 2.1 全局模式、有效能力与按需激活

这与单次 run 的模型/并发设置是两层不同配置，不能混为一个开关。

| 全局模式 | 新入口与模型工具 |
| --- | --- |
| disabled | 隐藏新启动入口与模型工具；已有历史运行仍可查看 |
| onDemand | 命令、中枢、技能及恢复入口开放；会话开始时不立即向模型注册工具，触发后激活 |
| alwaysOn | 从会话第一次模型请求起注册工作流工具 |

默认值计算为合法环境覆盖优先于合法服务端 mode，否则 disabled，得到 `offeredMode`；合法用户选择再覆盖该默认，得到有效 `mode`，`enabled = mode != disabled`。**offeredMode 是默认值，不是能力上限**：服务端默认关闭或请求失败，不阻止用户明确开启；但宿主根本未装配该功能的 resolver 是另一道真实能力边界。

设置位于“Agent 能力 → 行为与工具”。快照 ready 且含合法 offeredMode 即出现，即使默认关闭；加载中、读取失败、宿主不提供该字段则不显示。选项标注默认值，选回默认就删除显式用户选择。UI 读取宿主已合成的结果，不在不同页面重新算一遍。

`onDemand` 不只认手敲命令：首次 `/workflow`、Resume/Configure、中枢直接启动、恢复含工作流历史的会话均可激活；激活在该会话内单向保持，压缩上下文不撤销。disabled 同时过滤命令目录、菜单、技能与新会话工具，插件同名命令不能绕过；已有活跃会话不因用户改设置而强行回收已经取得的工具面。

模型工作流工具簇共十项：CreateWorkflow、AmendWorkflow、SaveWorkflow、ListSavedWorkflows、EvalWorkflowSnippet、ListWorkflowRuns、GetWorkflowRun、ResumeWorkflowRun、ResolveWorkflowQuestion、FillWorkflowHole。retune 是设置操作的执行分支，不是第十一个独立模型工具。

远程不是一概禁用：桌面管理的 SSH/WSL/Docker Host 接收用户选择，独立 server 保持自己的配置权威，移动远控跟随桌面宿主。参考通过 Host 同步策略；GCode 本机必须保留强类型直连，只有真正远端边界才使用传输适配器。

## 3. 必须一起复刻的界面

| 界面 | 布局 | 显示内容 / 核心交互 |
| --- | --- | --- |
| 输入与命令菜单 | 现有 composer 与命令候选 | 插入命令不等于立即启动；发送后才展开命令提示 |
| 工具草稿与编译反馈 | 对话工具行 | 名称、逐步揭示的阶段线、诊断与修订反馈；没有运行不能画成已启动 |
| 运行确认 | 权限弹窗 | 问句、名称、阶段数、修订关系、两项设置、保存来源/实参、时间线、折叠脚本、批准/拒绝/修改反馈 |
| 运行卡 | 对话轮尾横向卡 | 阶段站点、Agent 药丸、并行轨道、回边、问题/留白、设置、停止/恢复、产物 |
| 运行详情 | 右侧 tab，纵向阶段脊柱 | 状态/并发/用量、来源/设置改动、结果或错误、阶段与名册、问题、留白、产物 |
| 子 Agent | 右侧独立 tab | 对应 Actor 的真实只读会话；未创建会话时显示等待占位 |
| 脚本步骤 | 右侧独立 tab | 文件读取、搜索、Git、命令及普通步骤日志，不是源码编辑器 |
| 产物 | 运行卡、完成卡与独立 tab | 文件、Markdown、HTML 路由及 chart/table/metrics/board 等专用显示 |
| 状态与项目侧栏 | 现有任务摘要区域 | 后台 run 的阶段缩略线、运行数、未读/完成提醒 |
| 工作流中枢 | 自动化模块的工作流 tab | 全局和项目分组、保存定义卡、详情、历史、运行/修订/删除/作用域操作 |
| 启动与再次运行 | 参数弹窗或无参直启 | 目标项目、参数校验、预填、在途禁重复、失败留窗、接受后导航 |
| 运行目录 | 右侧目录 tab | 日志发现查询，运行中/已结束分组，历史 run 入口 |

这几类右侧 tab 共享同一个 run / 父会话事实，不能各自产生一套运行状态。

## 4. 对话显示顺序和卡片选择

### 4.1 工具行不等于运行卡

- 成功且关联到 run 的创建/修订工具行使用简短摘要，不在该行复制完整运行图。
- 编译失败、拒绝、未关联等结果保留相应工具反馈，不能统一改成“工作流完成”。
- 一轮末尾从直接启动元信息、创建/修订结果、恢复结果提取 run；同轮相同 runId 去重。
- 最新有效脚本的图需要吸收成功 Fill 的 display，不能永远使用种子脚本图。
- **仅就地改并发的 retune 只有设置行，不产生第二张运行卡或新 run。**
- 工作流中枢直接启动的首轮没有用户气泡，运行卡就是该启动轮的主体。
- 轮尾顺序：主正文 → 合条件的完成卡 → 运行卡 → 其他尾部内容。

### 4.2 完成卡不是所有终态的公共卡

完成卡只服务满足条件的工作流成功后台结果轮；失败、停止、问题/留白升级不伪造成功交付。卡片不折叠：主产物预览 → 其余交付索引 → 耗时、Token、子 Agent 数、进入阶段数四项统计。

未知统计显示 `—`，不能拿 0 代替。GCode 的耗时必须走共享 `i18n.Duration`、截断到整秒，不照搬参考中部分省略秒或小数显示。

### 4.3 运行卡示意

```text
┌ 工作流状态 / 名称                 问题 / 留白  设置  停止/恢复  打开 ┐
│ 阶段数 / Agent 数 / 模型摘要                                    │
│        回边 / 跳段弧 / 留白补全标题                              │
│  ● 计划 ───────── ● 并行实现 ═══════ ● 验证                      │
│  [规划 Agent]     [实现 Agent A]    [验证 Agent]                 │
│                   [实现 Agent B]                               │
│                   [还有 N 个]                                  │
│  详情已截断的说明（仅在事实确实被裁剪时）                         │
│  [产物 A] [产物 B] [产物 C] +N                                  │
└────────────────────────────────────────────────────────────────┘
```

默认展开；无折叠箭头，点击卡片空白或 Enter/Space 切换。子按钮不能冒泡触发折叠。收起只隐藏 Agent 药丸区，阶段线和产物仍保留。

缺活投影时降级为中性“已结束”摘要，不猜测 completed，不伪造状态灯、轨道或控制。存在 run 但没有发起图时，也不能凭空拼图。缺图与缺 run 是两种状态。

停止后显示“正在停止”并禁重复提交，等待核心状态到位；拒绝原因留在对应操作附近。不要靠固定延时把状态当作已停止。

## 5. 时间线：几何、顺序、滚动和名册

### 5.1 参考几何

`components/workflow-timeline/timeline-geometry.ts` 的参考值：站宽 168、站间距 24、步长 192；药丸高 32、间距 6；轨道行/站台行各 24；弧道间距 14；灯心距站左缘 17。

这些是复核参考布局的量尺，**不是单页覆写 GCode 共享控件高度/字体/颜色的授权**。原生布局先复用共享设计定义，业务页面不自建另一套按钮和输入框规格。

### 5.2 不变量

- 阶段按图中声明顺序，不因状态变化重新排序。
- 同 Actor 的头像与身份跨阶段稳定；按实例身份联接，不按可重复的人名联接。
- 并行分支画轨道带，跳段/循环画弧；不是允许用户任意拖拽连线的画布。
- 动态实例数和静态 step 数不同：`settled/observed` 是实际观察实例，不能写成静态定义总数。
- 投影有界不等于执行有界；被裁剪的 Agent 仍要进入统计，不能一离表人数就缩水。
- 横向 overflow 不露原生粗滚动条；边檐指示视外阶段，点击跳到相应站；底部细滚动轨支持拖动，方向键一次移动一站。
- 自动跟随只在运行焦点改变时执行，取最右运行站；不能每次事件都抢回用户滚动。草稿跟随新揭示站。
- 草稿逐站揭示和打字效果不是执行进度；减弱动态效果时取消非必要动画。

### 5.3 超过六个 Agent 的最新规则

**3.18.2 规则不同于旧本地 3.14：**

1. 阈值计算包含表内与表外 Agent；不只是当前可见药丸数。
2. 不超过六个直接显示；超过阈值显示五个固定位置加“还有 N 个”。
3. 固定位置优先级为 **asking → running → failed → 参与者原序补位**。不再是旧版 failed 优先，也不再要求 running 不超过三个才具名。
4. 同组保留参与者顺序，避免每次更新整列翻动。
5. 表外只有数量/结算事实，没有可打开的假药丸；未结算的表外实例计 pending，不猜成 done。
6. “还有 N 个”打开详情对应阶段；详情里再按状态组展示剩余名册，不在卡内长出无界列表。

实现时应把这些计算写成共享纯函数，卡片、确认、详情、缩略入口使用同一套规则。

## 6. 状态词、控制按钮和设置

### 6.1 状态不是一个布尔值

| run 状态 | 语义 | 原版视觉语义 | 控制原则 |
| --- | --- | --- | --- |
| pending | 已接受、尚未运行 | 中性空环 | 不能伪装执行中 |
| running | 运行中，含等待问题/留白 | warning 活动灯 | 可停止；是否可配置另看能力 |
| completed | 成功结算 | success | 不提供无意义的配置重放 |
| errored | 脚本/工作流错误 | destructive | 可按核心规则修订/恢复 |
| stopped | 用户/模型/供应商/中断/被替代 | 中性空环并说明原因 | 不等于失败；恢复只认 resumable |

`superseded` 是被修订后继替代的停止状态，不显示 Resume。不能看到 stopped 就自行算可恢复。

节点显示有 pending/running/done/failed 四态：executing/repairing/nudged 为 running；queued/dispatched/waiting 为 pending；结算取消按失败语义。聚合优先表达仍在运行，已结算与排队混合也不是“全部完成”。阶段还要结合进入事实、当前阶段及表外计数。

### 6.2 两项设置，以及新版最重要的行为变化

运行设置只有子 Agent 模型与最大并发数。打开弹层时冻结起点；只发送修改字段；全改回去、请求在途或模型不可用时禁 Apply。

- 模型 `null` 表示恢复会话模型；不因配置项缺席就偷偷换供应商。
- 并发默认值是起点，**不是硬天花板**。新版只有至少 1 的整数下限，可以高于默认；取消 run 自有界用 `null`。
- `running + 只改并发`：就地 retune，同 runId，不停止、不新开运行、不换详情 tab。
- 改模型或一般修订：停止前驱并创建后继，保留可重放前缀；接受后等后继投影出现再替换原 tab。
- pending 只有引擎控制已建立时才可能就地修改；UI 不预先承诺一定不停。
- pending/running/errored 可配置；stopped 排除 superseded；completed、缺投影或缺能力不配。
- 后果文案按**本次实际修改**决定，未修改时不先写“将停止当前运行”。

模型选择、并发起点、已学习的限流水位和 run 自有限制是不同事实。不能把 governor 学到的低水位误称为“本机最大并发”。

### 6.3 确认窗

顺序保持：问句与名称 → 修订关系 → 子代理模型/最大并发 → 保存来源/参数 → 阶段线与留白 → 脚本。

有图时脚本默认收起；没有可展示 step 时展开脚本。新请求重置展开状态。可调设置依宿主能力出现，模型不可用阻止批准。在途时控件与批准动作一起禁用。

允许一次、本会话允许、拒绝，以及附反馈要求调整是不同结果；拒绝反馈交回主 Agent，不伪装已运行。已保存定义不自动获得工具调用的永久信任；中枢用户亲自点击直接启动是另一个已经授权的入口。

## 7. 留白与动态补全不能遗漏

这是旧 3.14 调查不足以覆盖的重要部分。`hole<T>()` 暂停当前分支等待主 Agent 补代码，其他分支可以继续；`FillWorkflowHole` 扩展当前 run，不新建 run。

- 未补全留白是一座独立站：虚线空灯、虚线接轨；到达后呈 warning 等待态。
- 类型 T 是编译/模型契约，**不在用户界面显示类型签名**。
- 卡头显示待补全数量，与问题数量可同时存在；点击打开对应运行，不显示用户写代码的输入框。
- 尾留白有淡出的后续轨道，表示程序后半尚未写完。
- 补全带新 phase 时，新站插在原位置，保留留白标题和局部边框/提示；无新 phase 时保留原站并加补全标识。
- 嵌套补全需要父子关系，卡片和详情缩进一致。
- Fill 确认只突出该补全片段及邻接上下文，不把完整 run 重画成一次新启动。
- 编译错误不提交有效脚本、不推进留白；诊断指向草稿文件，允许修正后重交。
- 成功补全后持久化有效脚本、图、Schema 和事实，再唤醒等待；已有站点身份不得重编号。

## 8. 运行详情、问题与脚本步骤

### 8.1 纵向详情，不复制横向卡

```text
名称                         配置  恢复  停止  保存/再次运行
状态 / 最大并发/冷却
谱系与用量
启动来源/实参，或本次设置改动
结果/错误
│ ● 阶段 A                         已结算/观察数 / 轮次
│   [Agent 行]
│      升级问题 / 等待信息
│ ● 阶段 B
│   [五个固定 Agent]
│   [还有 N 个 → 分组名册]
│ ◌ 等待补全的留白
产物（默认展开）
```

阶段头可折叠；每个新 running 阶段自动打开，已经手动打开的不因焦点转移而关闭。折叠时仍保留状态、计数/轮次和头像或名册量条。

打开指定 phase 时展开对应阶段、滚到标题并短暂强调；不能持续追滚妨碍阅读。实例定位不能只靠同名阶段，要带阶段身份和进入时刻等稳定锚点。

顶部操作包含保存/再次运行，不能只复刻旧版的配置、停止、恢复。哪些条件可保存、保存后的运行目标和预填来源，必须以核心/journal 元数据为准。

### 8.2 问题不是直接向用户要答案

Actor 遇到问题向主 Agent escalate，等待 `ResolveWorkflowQuestion`；UI 展示问题、上下文、标识及已等待时间，不在 Actor 行下直接放用户答题表单。未能联接到具体 Actor 的问题保留在尾部，不丢失、不猜作者。

缺 askedAt 不编造“刚刚”。提问与留白等待不同，Resolve 不等于 Resume。

### 8.3 子会话和脚本日志

- 有 Actor 槽位但还没有 sessionId 时，打开等待页；会话创建后自动进入真实只读 transcript。
- 子会话与 run 的所有权和生命周期归核心；UI tab 不拥有业务 Agent。
- 脚本药丸打开执行日志并按阶段定位，不展示第二张时间线。
- Read 行不可展开；文件操作打开当前文件版本，不冒充执行时的内容快照。
- 命令收起时按需取尾部，展开时取完整有界内容；不是实时 stdout 直播。
- 日志按事件序号更新，独立保存展开状态；不能每个渲染帧重拉全部输出。

## 9. 产物、完成卡与保存后再次运行

产物不等于脚本 `return` 或内部 `report`。必须有独立元数据、内容、版本和发布顺序。

- 运行卡最多三个产物链接，再附静态 `+N`；不是点击后展开全部的按钮。
- 完成卡主交付优先显式 primary，唯一交付可自动作为主交付；其余保留索引。
- 详情失败/停止后仍显示已经发布的产物；不能随 run 失败清空。
- **运行详情**有 primary 时主预览配索引，无 primary 时使用瓦片画廊；**完成卡**无 primary 时只显示索引，不画假预览。完成卡索引最多六行，超出时五行加“还有 N 个”；通知清单确有截断且总数未知时用 `…`，不猜总数。
- 默认最新版本；用户查看旧版时新发布不强行跳版本；只能切到真实存在的历史版本。
- summary 用于实时轻量信息，journal 用于完整查询；不能靠表内投影的裁剪数据假造完整列表。
- 只有要展示预览时取内容，不为一排索引提前加载全部字节。
- HTML 仅在 MIME、来源、平台能力满足条件时走浏览器面板；缺路径查元数据，远端/Web 或失败回产物页。不能在普通 renderer 随意执行 HTML。
- 新版的保存/再次运行是完整闭环：再次运行应从日志取本次实参预填，忽略当前定义已删除的键，缺值用当前声明默认值；不能简单复用 composer 草稿。

### 9.1 保存与再次运行的完整交互

- 完成卡和 run 详情头使用同一控制器、同一保存事实缓存。同一个 run 不能一边显示“保存”、另一边显示“再次运行”。
- 详情的保存/再次运行动词仅在 run 为 completed 且能力允许时出现；静态、分享只读或无会话服务上下文不挂写操作。“已保存”是文件事实，使用中性芯片，不冒充运行成功状态，也不因操作能力关闭而消失。
- 未保存时弹层顺序为：顶部整行“让 ZCode 帮我提炼保存” → 分隔线 → 名称、项目/全局作用域、说明、落点预览 → 直接保存。草稿在每次打开时冻结，Esc 关闭即丢弃，背景事件更新不覆盖输入。
- 运行名只有本身符合文件名规则才预填名称；中文等显示名预填到说明而不是塞进必然报错的文件名。参考名称规则为 1–64 字符，仅字母、数字、`_`、`-`、`.`，不能全是点；最终仍由核心校验。
- 直接保存按 runId 读取**实际执行的有效脚本**，不经过模型回合；提炼保存则将已有输入及 runId 交回父会话，由主 Agent 抽取可变 args、说明和使用时机后调用 SaveWorkflow。二者不是同一个动作的两个文案。
- 本机默认项目作用域；远端 run 锁定项目档，不暴露宿主家目录的全局档。参考落点为项目 `.zcode/workflows/<name>.dwf.ts` 或本机 `~/.zcode/workflows/<name>.dwf.ts`，这是 ZCode 路径证据，不要求 GCode 沿用其产品目录名。
- 名称探测有 320ms 合并窗，区分目标作用域重名覆盖和另一作用域同名遮蔽。探测失败不假装保存失败，最终以核心结果为准；首次返回 target_exists 时展示覆盖警告并改变提交语义，不能静默覆盖。
- 保存成功后出现“已保存”芯片，动词变为“再次运行”，焦点回原操作位，并播一次无障碍通知；保存失败留在原表单。芯片能进入中枢时可点击，否则只显示文字。
- 再次运行不是 Resume：创建新会话及新 run，复用中枢启动器。项目无参可直启；有参或全局定义弹窗；全局默认本次 run 的本机项目，但仍让用户确认目标。实参从 journal 预填，不从对话草稿取。
- 已保存关联不持久写死 saved=true，而依次尝试：成功 SaveWorkflow 的当前有效候选 → 正文与实际脚本逐字相同的定义（项目优先，每个作用域最多扫描 50 份）→ 同名定义。删除某候选仍可能命中另一份；全部不命中才回“保存”。保存、删除、中枢刷新会使缓存失效，并丢弃失效前的在途回答；不是每帧扫描。
- 再次启动仍按 name+scope 读取当前文件，定义消失/损坏会返回错误，不偷偷使用旧 run 的脚本兜底。实参持久化属于 run journal，声明与默认值属于定义 metadata；直接保存表单只提交 description，不自动提炼 args，也不把本次实参全部变成定义默认值。
- completed 是 UI 提供入口的门，不是底层直接保存 API 的重复状态校验；参考 API 实际检查名称、run 记录及有效脚本、覆盖权限、分析与写入结果。
- 完成卡宽不足 480 时，保存/再次运行只保留图标，文字留在无障碍名称和提示；详情头保持文字。原版使用锚定弹层，GCode 默认模态表单规范与之存在交互差异，实施时必须明确取舍，不暗中宣称两者完全一致。

## 10. 工作流中枢与参数弹窗

### 10.1 列表与详情

- 外框仍是自动化页（GCode 的自动化本来就是独立路由），“自动化 / 工作流”是模块切换，不另造独立应用，也不另占侧栏入口或设置页分节（见 §26）。
- 标题区有刷新、创建对话入口，显示有效模式（不是磁盘存储值或默认模式）。设置链接与 Agent 设置行同样依据合法 offeredMode；工作流 tab 整体则依据 enabled，关闭时回到自动化，而 Agent 设置行仍可用于开启。刷新只刷新列表/计数，不强刷策略缓存或重建活跃会话工具面。
- 全局组和项目组分开；当前项目有标记；远程项目要保留宿主身份。
- 最新版项目来源已改用统一项目目录：选择存在 primary root、未删除、未明确离线的项目，不再只枚举已打开 tab。保留 projectId / hostId；远端项目附宿主工作区身份，不能仅凭路径串判定同一个项目。
- 全局运行/移动目标只用本机项目候选，不能因为它是“全局”就允许任意远端宿主读本地定义。
- 合法定义计数、加载中、坏文件、真正为空分别显示；坏文件不是合法定义，也不是静默消失。
- 卡片展示名称、两行描述、最多三个参数与 `+N`、最近运行状态；整卡进详情，按钮不冒泡。
- 详情“定义 / 历史”切换；名称旁显示项目/全局归属，面包屑不再重复项目层。
- 详情脚本只读、可复制、通过对话修订；元信息、参数声明与脚本逻辑要分开。
- 最近产物从最近成功且确有产物的 run 选取，不拿最新失败 run 盖掉已有交付。
- 提升全局、移动到项目、复制路径、删除都是不同动作；不能统一实现成改一个 scope 字段。

原版存在行内元信息编辑；GCode 当前默认新增/编辑使用模态表单。此次只记录差异，不擅自修改供应商设置或推翻共享规范。实施这些编辑入口前需明确精确复刻是否包含该交互例外；详情展示本身不是新增/编辑表单。

### 10.2 启动规则

| 场景 | 行为 |
| --- | --- |
| 项目定义，无参数 | 直接建空会话并启动，不弹空表单 |
| 项目定义，有参数 | 参数弹窗，固定所属项目 |
| 全局定义 | 始终选择本机项目，必要时填写参数 |
| 再次运行 | 从该次运行的已持久化实参预填，仍按当前声明校验 |
| 启动中 | 禁重复提交及取消；不能同一帧点两次开两个会话 |
| 启动拒绝 | 解释错误，留在原弹窗；尽力回收失败产生的空会话，回收本身仍可能被拒绝或异常 |
| 启动接受 | 再关窗并导航；accepted 不等于已经 completed |

参数按声明顺序：string/number 单行、boolean 开关、json 多行；支持默认值、必填与说明。number 必须有限，json 必须可解析，未知参数拒绝，不自动强转。空 string/number/json 不传，boolean 明确传布尔值。

最新版长表单弹窗三段式：固定标题 → 可滚动目标/参数 → 固定说明、错误、取消/运行。参考宽 480，最大高受视口约束；不能让几十个参数把按钮挤出屏幕。校验失败按声明顺序滚动并聚焦首个错误字段，错误文案与字段一起进入可视区。

启动复用同一核心编译/运行入口，但不先发模型请求，不继承 composer 的待发输入与临时模型配置；目标是定义所属项目/用户选择项目，不是碰巧当前激活的项目。

## 11. 后端能力与 GCode 承接边界

沿用既有 **Go 核心 + moejs + JSON/JSONL** 路线，不因为参考用 Node/SQLite 就另起一套常驻 Node 服务或改数据库。保留 TypeScript 泛型/Schema、四图分析、稳定站点、重放、留白补全等能力，不默认降级为简化 DAG。

| 能力 | 必须保留的语义 |
| --- | --- |
| 编译 / 分析 / lowering | 类型和 Schema、站点身份、因果/控制/数据/交接图；display 是有界投影 |
| Actor | 持久会话、同 Actor ask FIFO、不同 Actor 并行、结构化结果修复与错误 |
| 调度 | 默认并发、run 自有限制、provider/model 公平限流、冷却、在途计数与 live retune |
| 工作区能力 | files/git/world.run 的真实宿主、路径归属、权限、输出模式与取消政策 |
| Resume | 同 runId 重建脚本 Runtime，按 journal 重放已完成事实，不是内存 VM 快照 |
| Amend | 后继 run、前驱结算与可重放工作导入、谱系及用量事实 |
| Fill | 同 run 扩展有效脚本，保留词法环境与旧站点，原子提交后唤醒 |
| 问题 / 通知 | Actor escalate、主 Agent Resolve、完成/停止/配额/留白通知，断线可重建 |
| 保存定义 | 项目/全局来源、参数契约、直接启动、历史与再次运行 |
| 产物 | 声明/发布/版本/顺序/内容完整性，以及断线读取 |

`moejs.CompileTS` 的类型擦除不等于 TypeScript 类型检查；普通 Agent 单次任务不等于可复用 Actor。JSON 结果缓存不等于可靠重放：Promise 的历史释放顺序也要记录，否则 `Promise.race` 可能改分支。

建议所有权：

```text
MyGo 页面 / 展示投影 / 草稿
       │ 强类型 Go 方法和事件（本机不经 IPC）
Go Workflow 核心
       ├─ 定义、运行、Actor、问题、预算、权限
       ├─ 类型/Schema、分析、lowering
       ├─ 调度与 provider/model 限流
       ├─ moejs owner goroutine + 后台宿主 I/O
       └─ JSON checkpoint / JSONL journal / 内容文件
真正的远端适配器 ── 同一个核心（Linux 无桌面可运行）
```

客户端断线不停止宿主已接受任务；UI worker 不持有长期 `ui.Element` / `ui.Context`。模型凭据不进入脚本、产物或展示日志。

停止 VM 不等于停止 Go I/O 或工具进程。参考最新版 `worldRun` 调用仍未显式传 run AbortSignal，这是静态风险，不是本轮已复现问题；GCode 需要明确自己的完整取消链。外部副作用后、日志提交前的崩溃也不能靠换 JSON/SQLite 自动获得 exactly-once。

### 11.1 参考实现问题与不照搬的边界

- `UI/project-workflow/useWorkflowRunSave.ts:143–158` 解析父会话或用户选择的目标后，构造启动器参数时丢弃 projectId / hostId；下游 `UI/settings/saved-workflows/useSavedWorkflowLauncher.ts:159–167` 却实际消费 projectId。**字段传递遗漏可静态确认，实际项目绑定影响未运行验证**。GCode 应保存正确的目标归属，不能将此遗漏当成复刻规格。
- 上述父会话目录解析位于启动器统一 try/catch 外，异常是否进入一致错误映射仍需运行验证。
- 参考存在旧 Host 能力探测与旧格式回退，本轮只将其作为读懂源码的背景；GCode 默认不做旧版本兼容，不照搬这些分支、字段别名或旧格式解析。
- 参考的秒以下耗时显示、原版行内编辑与锚定保存弹层，与 GCode 已有规则不完全相同。布局和业务显示逻辑以本规格为目标，控件外观继续服从共享定义；精确复刻中的交互冲突要显式确认，不能悄悄改规则后声称完全一致。

## 12. 实施分段与验收标准

分段只是施工顺序，不是删减范围，也不等于每段完成就可以宣称完整复刻。

1. **固定契约与证据集**：相同脚本/状态输入、关键窗口与主题截图、事件序列、图/实例联接规则。
2. **核心底座**：定义、类型/分析、Actor、调度、journal、取消与重放；先无 UI 做确定性测试。
3. **主链路**：命令、编译反馈、审批、运行卡、阶段线、状态摘要、详情和 Actor transcript。
4. **动态能力**：问题/留白、Fill、Resume、Amend、live retune、配额恢复及断线重建。
5. **交付与复用**：产物、完成卡、保存/再次运行、中枢、历史和长参数弹窗。
6. **一致性验收**：逐入口、逐状态、逐尺寸与主题比对；通过后才称完成。

### 必测矩阵

- 状态：pending/running/completed/errored/stopped，各停止原因、缺投影、缺图、拒绝控制。
- 图：零 step、单阶段、多阶段、分支带、循环弧、多阶段同时运行、长名称、无 phase marker。
- 规模：0/1/6/7/数百 Actor，表内/表外裁剪，问题/运行优先固定名额，计数不缩水。
- 动态：等待问题、等待留白、嵌套 Fill、有/无新 phase、补全编译错误、最新有效脚本联接。
- 控制：停止竞态、同 run 并发修改、模型修订产生后继、superseded 禁恢复、accepted 后投影延迟。
- 持久化：冷启动、事件续接、裁剪投影重建、Promise 释放顺序、损坏日志与未知副作用窗口。
- 产物：零/一/多件、显式 primary、旧版本阅读中发布新版、失败后已有交付、HTML 本机/远端路由。
- 中枢：项目/全局/远端身份、合法/坏定义/空态、目标归属、再次运行预填及定义变化。
- 表单：1/2/几十参数、无参直启、首错定位、必填/默认/非法数字/非法 JSON、启动失败回收。
- 展示：深浅主题、窄窗、长中文/英文、键盘、减弱动画、手动滚动不被抢、所有耗时整秒与等宽数字。

GCode UI 修改时必须运行共享组件边界检查及 `ui.Tester`；桌面 E2E 只经 `scripts/desktop-e2e` 和 MyGo automation，以稳定 key 和可观察业务状态等待。Mock、真实本机核心、远程宿主、真实供应商分别记录。截图或编译通过不能替代跨入口布局与显示逻辑验收。

## 13. 新版关键源码索引

以下位置已对应本地更新后的固定 SHA。目录缩写仅用于减少重复：`UI = packages/ui/src`，`CLI = apps/zcode-cli/packages/bootstrap/src`，`TL = UI/components/workflow-timeline`。

| 职责 | 源码位置 |
| --- | --- |
| 命令提示与展开 | `CLI/builtin-workflow-command.ts:32`、`:64` |
| 官方展示 / 启动 / 通知规格 | `docs/dynamic-workflow/presentation.md`、`launch.md`、`transcript-and-notifications.md` |
| 成功工具行与运行卡联接 | `UI/v4/workflowRunCardJoin.ts`、`workflowTurnDigests.ts`、`ConversationWorkflowDigests.tsx` |
| 对话运行卡 | `TL/WorkflowRunDigest.tsx:82` |
| 时间线 / 几何 / 固定位名册 | `TL/WorkflowTimeline.tsx:132`、`timeline-geometry.ts:9`、`roster-model.ts:106` |
| 设置纯规则 / 后果文案 | `TL/workflowRunSettings.ts:55`、`:69`、`:153`、`:178` |
| 留白模型 / 灯 / 补全框交互 | `TL/timeline-holes.ts`、`WorkflowHoleParts.tsx`、`WorkflowTimelineFills.tsx:152` |
| 运行确认 | `UI/WorkflowPermissionBlock.tsx:121` |
| 运行详情 / 状态头 / 阶段 | `UI/app-shell/WorkflowRunSidePane.tsx:66`、`WorkflowRunSidePaneSections.tsx:76`、`WorkflowRunPhaseList.tsx:79` |
| 配置 ACK 与 tab 跟随 | `UI/app-shell/useWorkflowRunPaneSettings.ts:92` |
| Actor / 脚本 / 运行目录 | `UI/app-shell/WorkflowActorSessionSidePane.tsx:125`、`WorkflowWorkspaceSidePane.tsx:350`、`WorkflowRunDirectorySidePane.tsx:249` |
| 完成卡与索引纯规则 | `UI/v4/ConversationWorkflowCompletion.tsx:36`、`TL/WorkflowCompletionArtifacts.tsx:39` |
| 保存 / 再次运行共享控制器 | `UI/project-workflow/useWorkflowRunSave.ts:37`、`WorkflowRunSaveControls.tsx:65`、`WorkflowRunPaneSaveSlots.tsx:81` |
| 保存弹层 / 默认草稿 / 提炼提示 | `TL/WorkflowSavePopover.tsx:87`、`workflowRunSave.ts:44`、`:68` |
| 已保存关联与 journal 实参查询 | `CLI/zcode-protocol/saved-workflows-run.ts:159` |
| 中枢 / 详情 / 参数弹窗 | `UI/settings/saved-workflows/SavedWorkflowsSection.tsx:84`、`SavedWorkflowDetailView.tsx:92`、`SavedWorkflowLaunchDialog.tsx:99` |
| 启动与失败回收 | `UI/settings/saved-workflows/useSavedWorkflowLauncher.ts:113` |
| 项目选项及本机 / 宿主身份 | `UI/project-sidebar/useSavedWorkflowProjectOptions.ts:6` |
| 模式合成 / 设置 / 用户选择同步 | `packages/shared/src/dynamic-workflow-feature.ts:96`、`:116`、`UI/settings/DynamicWorkflowModeSetting.tsx:44`、`UI/hooks/useDynamicWorkflowAvailability.ts:45` |
| 按需激活 / 既有会话策略边界 | `apps/zcode-cli/packages/core/src/runtime/methods/dynamic-workflow-activation.ts:55`、`CLI/zcode-protocol/dynamic-workflow-policy.ts:7` |
| 就地改并发 | `CLI/app/dynamic-workflow-run-retune.ts:32` |
| 配额恢复 | `CLI/app/dynamic-workflow-quota-resume.ts:47` |
| 外部命令执行与取消链审查点 | `CLI/app/workflow-world-read.ts:315`、`:326` |

可复用的参考 fixture 位于 `packages/ui/test/fixtures/`：`workflow-digest-fixed.fixture.tsx`、`workflow-prelaunch.fixture.tsx`、`workflow-status.fixture.tsx`、`workflow-summary-hover.fixture.tsx` 等。它们可帮助建立固定输入的对照场景，但本轮没有运行这些 fixture；自行生成图不能代替真实窗口验收。

本轮文档校验：源码索引中 44 项路径存在且行号未越界，4 个列举 fixture 存在，Markdown 代码围栏配对通过；`git diff --no-index --check /dev/null docs/investigations/zcode-workflow-replication.md` 无空白诊断（新文件有差异，no-index 退出码为 1，不冒充退出码 0）。这里只证明引用与文档格式有效，不证明运行时或视觉一致性。

本页的界面示意是信息结构说明，不是从真实窗口截取的视觉证据。最终复刻仍需固定版本的实窗截图与逐状态交互验收。

## 14. 实施进度（GCode 侧）

本节记录复刻在 GCode 仓库里的实际落地情况。每一项都注明代码位置与验证方式；
没有验证的条目明确写「未验证」。

### 14.1 第 2 段：核心底座（已完成）

- `internal/workflow/`：纯规则包。`status.go`（五态、停止原因、可配置/可恢复判定）、
  `settings.go`（只改并发 = 同 run 就地 retune；改模型 = 后继 run；并发下限 1）、
  `geometry.go`（站宽 168、站距 24、步长 192、药丸高 32/间距 6、轨道行 24、站台行 24、
  弧道间距 14、弧基 26、灯心 17；名册阈值 6、固定位 5、优先级 asking → running →
  failed → 参与者原序）、`artifacts.go`（运行卡 3 件 + 静态 +N；完成卡索引 6 行、
  溢出为 5 行 + N）、`journal.go`（JSONL 事实日志）、`replay.go`（按日志重建运行）、
  `reply.go`（实例回复里的问题/留白/产物标记）。
- `internal/engine/workflow.go` 与 `workflow_runner.go`：运行的接受、阶段推进、
  并发执行、实例会话、问题/留白等待、产物发布、取消、修订后继、日志与重放。
- 验证：`go test ./internal/workflow/ ./internal/contracts/ ./internal/engine/ -run TestWorkflow`。

### 14.2 第 3 段：主链路显示层（本轮完成的部分）

| 内容 | 位置 | 说明 |
| --- | --- | --- |
| 运行投影 | `internal/workflow/projection.go` | 阶段按声明顺序、实例全量计数、名册有界、产物限三件、待答问题与等待留白计数 |
| 时间线投影 | `internal/workflow/timeline.go` | 站、轨道（按真实执行窗口重叠划分）、回边/跳段弧（区间图贪心着色） |
| 轮尾运行卡 | `internal/desktop/workflow_run.go` | 状态词 + 名称 + 阶段数/实例数/模型摘要、阶段线、实例药丸、产物条、停止/配置/打开详情 |
| 阶段线绘制 | `internal/desktop/workflow_timeline.go` | 轨道、站灯、回边与跳段弧、实例药丸与「还有 N 个」 |
| 运行详情 tab | `internal/desktop/workflow_run_pane.go` | 纵向阶段脊柱、实例行、升级问题、等待留白、产物与错误 |
| 设置弹窗 | `internal/desktop/workflow_settings.go` | 子代理模型 + 最大并发数；只发送实际改动的字段；后果文案按本次改动生成 |
| 工作台 surface | `internal/workbench/workflow.go` + `workbench.go` | `workflow` surface kind、`OpenWorkflow`、`RenderWorkflow` 渲染钩子 |
| 应用接线 | `internal/desktop/app.go` | `New` 与 `Attach` 两处都接上 `RenderWorkflow`；`workflow.changed` 事件经快照刷新 |
| 轮次锚点 | `internal/contracts/workflow.go`、`internal/engine/workflow.go`、`internal/workflow/replay.go` | 运行记录发起它的那条回复（`MessageID`），运行卡因此挂在发起它的那一轮轮尾，而不是会话末尾 |
| 阶段边 | 同上 | 运行携带声明的阶段边，跳段与回边是显示事实而不是界面猜测 |

验证：
- `go test ./internal/workflow/ -count=1`（投影与时间线纯规则）
- `go test ./internal/desktop/ -run TestWorkflow -count=1`（运行卡、详情面板、轮次锚点、设置改动判定；
  含深浅主题与 900/1320 两档宽度）
- `go test ./internal/desktop/ -run 'Test(SettingsControls|InspectSettingsControls)' -count=1`（共享控件边界）
- `go vet ./...` 无输出

### 14.3 本轮未做（明确留到后续分段）

- 模型工具簇（Create/Amend/Save/List/Eval/ListRuns/GetRun/Resume/ResolveQuestion/FillHole）
  与运行确认弹窗：GCode 尚无工作流脚本编译器，`internal/workflow` 目前接收的是已编译的
  `WorkflowProgram`，因此本轮只做「已有运行事实的显示」，没有凭空造一条启动入口。
- 完成卡、产物预览路由、保存/再次运行、工作流中枢与参数弹窗（第 5 段）。
- 断线重建、配额恢复、脚本步骤 tab、运行目录 tab（第 4 段）。
- 一致性验收（第 6 段）：尚未做逐入口、逐状态、逐尺寸与主题的实窗比对。

## 15. 本轮实施状态（第 1 至 3 段）

本节记录已经落到 GCode 的实现与证据，以及还没有做的部分。分段完成不等于复刻完成。

### 15.1 已实现

| 范围 | 位置 | 说明 |
| --- | --- | --- |
| 领域类型 | `internal/contracts/workflow.go` | 定义、程序、运行、阶段、实例、问题、留白、产物、参数、设置改动、运行确认与决定 |
| 脚本编译入口 | `internal/workflow/compile.go` | JSON 脚本文本 → 阶段程序；同一次调用返回诊断（error/warning），错误不阻止收集其余诊断 |
| 条件表达式 | `internal/workflow/condition.go` | `==`、`!=`、`&&`、`||`、`!`、括号；事实路径带点分段，裸词是字面量 |
| 阶段推进规则 | `internal/workflow/advance.go` | 声明顺序是默认走向；条件不成立就跳过该站；成立的回边回到前面的阶段并重置其后阶段 |
| 事实命名空间 | `internal/workflow/advance.go` | `run.*`、`phase.<id>.status/rounds`、`actor.<id>.status/result`、`hole.<id>`、`question.<id>`、`arg.<name>` |
| 模式规则 | `internal/workflow/mode.go` | `disabled` / `onDemand` / `alwaysOn`；按需模式需先激活，激活后一直有效 |
| 纯规则投影 | `internal/workflow/projection.go`、`timeline.go`、`geometry.go`、`status.go`、`settings.go`、`artifacts.go`、`replay.go` | 卡片、详情、时间线、名册、状态词、设置后果、产物索引与日志重放 |
| 运行执行 | `internal/engine/workflow.go`、`workflow_runner.go` | 阶段推进、实例并发、提问、留白、产物、停止、恢复、修订后继、日志 |
| 运行确认 | `internal/engine/workflow_confirm.go` | 脚本编译后先登记确认请求并等待决定；批准前没有运行，也没有运行卡 |
| 十个模型工具 | `internal/engine/workflow_tools.go`、`workflow_tool_helpers.go` | CreateWorkflow、AmendWorkflow、SaveWorkflow、ListSavedWorkflows、EvalWorkflowSnippet、ListWorkflowRuns、GetWorkflowRun、ResumeWorkflowRun、ResolveWorkflowQuestion、FillWorkflowHole |
| 工具可见性 | `internal/engine/session_tools.go` | 按会话模式装配；禁用模式不出现任何工作流工具；子智能体与实例会话拿不到这些工具 |
| 确认弹窗 | `internal/desktop/workflow_confirm.go` | 问句与名称、修订关系、两项设置、参数、诊断、折叠脚本、拒绝反馈、允许一次/本会话允许 |
| 运行卡与详情 | `internal/desktop/workflow_run.go`、`workflow_run_pane.go`、`workflow_timeline.go`、`workflow_settings.go` | 对话运行卡、右侧详情脊柱、阶段线、设置弹窗 |
| 全局模式设置 | `internal/desktop/settings.go` | 通用分区新增“工作流”一行，选项为关闭/按需/始终开启 |
| 命令入口 | `internal/engine/workflow_command.go`、`internal/desktop/composer_prompt.go` | `/workflow` 是保留字：命令目录插入的芯片与手敲的行首命令都算一次命令；命令先按全局模式打开本会话的能力，再把请求展开成给主 Agent 的设计提示 |
| 工具行 | `internal/contracts/activity.go`、`internal/engine/tool_presentation.go`、`internal/desktop/transcript_model.go`、`transcript_detail.go` | 十个工作流工具有自己的工具类型与短标题；工具行只留结论与名字，整段脚本不进这一行 |

### 15.2 本轮明确取舍

- **脚本源语言用 JSON**：GCode 尚未接入 moejs，`CompileTS` 只做类型擦除，且没有四图分析、journal、resume、amend、fill。编译入口收在 `workflow.CompileScript` 一个函数里，后续换成别的源语言不必改调用方。
- **批准之前不建运行**：确认请求存在核心状态里，批准后才创建运行。拒绝时不留下运行，也不留下运行卡，避免把没跑过的事画成跑过。
- **推进由声明顺序加边条件驱动**：脚本没有独立的跳转语句，条件只作用在边上。跳过与回边都写进日志，重放能还原。
- **回边有轮次上限**：同一阶段最多进入 `workflow.PhaseRoundLimit` 轮，回边不会让运行永远转圈。
- **实例会话不是主 Agent 回路**：实例升级的问题会记录在运行上并等待，主 Agent 用 ResolveWorkflowQuestion 回答；界面不替主 Agent 要答案。
- **命令只负责开口，不负责运行**：`/workflow` 展开成设计提示并激活会话，运行仍要走脚本编译与用户确认。命令关闭时直接拒绝，用户命令不能占用这个保留字。

### 15.3 验证证据

- `go test ./internal/workflow/ -count=1`：编译诊断、条件求值、推进规则、模式规则全部通过。
- `go test ./internal/engine/ -run 'TestWorkflow' -count=1`：工具可见性、坏脚本只给诊断、批准后运行、拒绝不留运行、保存与列出定义、实参校验通过。
- `go test ./internal/engine/ -run 'TestWorkflowCommand' -count=1`：命令展开提示、模型看到展开后的正文与工具目录、会话被激活、关闭模式拒绝且不触达模型、命令文本解析的多种写法。
- `go test ./internal/desktop/ -run 'TestWorkflow' -count=1`：运行卡、详情、设置、确认弹窗的 `ui.Tester` 断言通过；确认弹窗覆盖脚本默认折叠与展开。
- `go test ./internal/desktop/ -run 'TestComposerOffersTheWorkflowCommand|TestComposerKeepsTheWorkflowCommand' -count=1`：命令目录按全局模式给出或隐藏入口，内置名是保留字。
- `go test ./internal/desktop/ -run 'TestWorkflowToolRows|TestWorkflowToolNames' -count=1`：工作流工具行的标题、摘要与“脚本不进工具行”。
- `go test ./internal/engine/ -run 'TestWorkflowToolPresentation|TestToolPresentation' -count=1`：工具类型、参数保留与结果投影。
- `go test ./internal/i18n/ ./internal/design/ -count=1`：新增界面文案的中文目录补齐，界面文案无间隔号规则通过。
- `go build ./...` 与 `gofmt` 无差异。

这些是确定性单元与组件测试，只证明规则与界面结构成立，不证明真实模型回合、远程宿主与视觉一致性。

### 15.4 还没做

- 工作流中枢与启动/再次运行参数弹窗（全局组、项目组、定义/历史、实参预填、首错定位）。
- 完成卡、产物面板、保存定义与再次运行的完整交互（含名称探测与覆盖提示）。
- 运行详情里的 Actor 子会话面板、脚本步骤面板与运行目录 tab。
- 问题的对话展示与补全入口（后端链路已在，界面未接）。
- 桌面端到端验收与跨入口布局比对。

## 16. 第 5 段：留白站（已落地）

本段只做「留白在时间线与详情里长什么样」。补全的写入链路（`FillWorkflowHole`）
与等待语义在第 2 段已有，本段补上的是显示。

### 16.1 规则（`internal/workflow/timeline.go`）

- **留白是一座独立的站**，插在它所属的阶段之后；找不到所属阶段的留白接在最后，
  它仍然是这次运行的一部分，不能丢。
- 站的下标就是显示位置：插入留白后统一重排，灯、弧与药丸都按它画。
- **轨道不因留白而变**：留白继承所属阶段的轨道（`Track`）；阶段之间的重叠判定
  仍只看两个阶段的执行窗口。
- **弧只连阶段站**：插进留白不改变阶段之间的邻接。`next` 边的相邻判定按
  **阶段在 `run.Phases` 里的顺序号之差为 1**，不再按显示下标之差——后者会把
  中间插了留白的两段误判成不相邻，白白画一条弧。
- **尾巴留白**（末站是留白）分两态：`open` 表示脚本的结尾还没写下，轨道在它
  之后多跑 40px 并淡出；`filled` 表示已经补上，只留 40px 余地，不画残段。
- 留白状态只有两个字：`waiting`（等人补全）与 `filled`（已补上）。
  `contracts.WorkflowHoleWaiting` / `contracts.WorkflowHoleFilled` 是唯一出处，
  界面、引擎与重放共用，不再散落裸字符串。

### 16.2 显示（`internal/desktop/`）

- 轨道**按段画**：触到开着或等着补全的留白那一段是虚线——那里还不是代码。
- 留白的灯是**空的虚线圈**：等着补全时描警示色并带一圈常亮光晕（不搏动，
  那里没有东西在跑）；补全后是普通的成功灯。
- 留白站不画实例药丸（它没有实例），站名旁写「等待补全」（英文 `Waiting for fill`）；
  补全后留一枚笔标。
- 留白的**类型签名不出现在界面上**：它是编译契约，不是给用户读的。
- 卡头的留白芯片带笔图标且**可点**，点开的是这次运行；界面不提供写代码的输入框。
  问题芯片同样可点：两枚芯片都只是通往这次运行的入口。
- 详情页分两组：**等待补全的留白**（带「等待补全」状态词）与**已补全的留白**
  （笔标 + 补全进来的正文）。

### 16.3 顺带修掉的时间线布局问题

留白站要落在同一条轨道上，暴露出旧的时间线绘制把轨道行、站名与名册挤在一张
盒子里：站名被站灯压住、药丸盖住产物条、名册多的站把站名顶得高低不齐。改法：

- 轨道行只画轨道、灯和弧，不装内容；站名与名册落在它下面的站台行。
- 站台行顶端对齐（`AlignItems(ui.Start)`），站名因此在同一基线上。
- 末站是尾巴留白时，站台列宽出 40px，淡出残段才有地方画。

这属于「限定范围修复」：不引入新的视觉规格，只把已有的几何常量用对。

### 16.4 验证证据

- `go test ./internal/workflow/ -count=1`：留白插在所属阶段之后、无阶段的留白
  不丢、留白继承所属阶段的轨道、插了留白也不误画弧、尾巴留白两态、
  `HoleOpen` 只在等着补全时为真。
- `go test ./internal/desktop/ -run 'TestWorkflow' -count=1`：留白站名与「等待补全」
  出现在运行卡上；留白芯片可点并打开运行详情；详情页分出「已补全的留白」。
- `go test ./internal/i18n/ ./internal/contracts/ ./internal/design/ ./internal/workflow/ -count=1`：
  新增文案的中文目录补齐，格式与目录检查通过。
- `go test ./internal/desktop/ -run 'Test(SettingsControls|InspectSettingsControls)' -count=1`：
  共享控件边界未被破坏。
- `go test ./internal/desktop/ -count=1`（全量）与 `go test ./internal/engine/ -count=1`（全量）
  通过；`go vet` 无输出；`go build ./...` 与 `gofmt` 无差异。

渲染探针（`ui.Tester` 出图，人工看图后删除探针文件）确认：轨道分段虚线、
留白空心虚线灯、尾留白淡出残段、站名不再被灯压住、产物条不再被药丸盖住。

时间线横向滚动已补上：超出卡片宽度时出现共享细滚动条（`controlHorizontalScrollbar`），
阶段可拖动与用方向键逐站移动。**仍然存在**：ZCode 的边檐（`WorkflowTimelineLedge`，
折到视口一侧的站灯与 `+n`）尚未实现。

这些是确定性单元与组件测试，只证明规则与界面结构成立，不证明真实模型回合、
远程宿主与视觉一致性。

## 17. 第 6 段：保存、再次运行与恢复（已落地）

这一段补的是「跑完之后还能做什么」：一次运行跑完可以原样存成可复用的定义，
存过之后同一个位置变成「再次运行」；停下来的运行能接着跑。

### 17.1 规则层（`internal/workflow/save.go`）

- **名字就是文件名**：`ValidSavedName` 只接受 1 到 64 个字母、数字、`_`、`-`、`.`，
  且不能全是点。这条规则同时是路径穿越的防线，界面与核心两侧都不放松。
- **落点预览**：项目档 `.gcode/workflows/<name>.json`，全局档
  `~/.gcode/workflows/<name>.json`。用户要能一眼看出这份定义落在哪儿。
- **名字只在运行名本身合规则时预填**：运行名是模型用用户语言起的标签，
  塞进文件名框只会换来一次必然报错；它改为预填到「说明」——那正是这次运行
  在做什么的一句话。
- **直接保存读这次运行实际执行的脚本**（`SavedDefinition`），不经过模型回合，
  也不重新推导。程序结构由运行事实折回：阶段顺序、阶段内实例、阶段边。
- **再次运行不是 Resume**：`LaunchRequest` 新会话、新运行，只沿用定义身份与
  这次运行自己的实参（`Args`），来源记为 `rerun`。程序由核心从定义取，界面不传。
- **已保存的第三层推断**（`SavedByScript`）：运行没有定义身份时，按脚本逐字相同
  去找定义，项目档优先、每个作用域最多看 50 份。这是推断，不是全盘扫描。

### 17.2 界面（`internal/desktop/workflow_save.go`）

- **一个控制器两个面**：`workflowRunControls` 同时供对话运行卡与详情头使用，
  同一个 run 不会一个面写「保存」、另一个面写「再次运行」。
- 只有 `completed` 的运行才出现保存/再次运行：跑到一半的脚本还是一次性的。
- 未保存 →「保存」按钮打开模态表单：名称、作用域（本项目/本机）、说明、
  落点预览、覆盖提示。**草稿在弹窗打开那一刻冻住**，背景事件不覆盖用户正在敲的字。
- 保存成功 →「已保存」芯片 +「再次运行」。「已保存」是**文件事实**：中性配色，
  不冒充运行成功状态。
- 同名定义已存在时，表单明说这是**覆盖**，提交沿用原定义身份；不静默覆盖。
- 远端运行（`HostID` 非空）只给项目档：家目录在宿主机器上，本机中枢读不到它。
- 没有记录脚本的运行说清楚「没有可保存的内容」，保存按钮不可点。
- **恢复**（`Resume run`）只在核心说可以继续时出现（`workflow.CanResume`）：
  stopped 不等于可恢复，被修订后继替代的运行不给恢复。

### 17.3 明确取舍

- **保存用模态表单**，不是参考实现的锚定弹层。这是 `docs/ui-controls.md` 的
  既定规范（新增/编辑默认模态弹窗），此处照办。
- **再运行直接复用中枢启动器**：GCode 目前的启动路径允许定义本身带程序，
  所以「再次运行」只要发出 `workflow.launch` + `DefinitionID`，实参随请求带过去；
  参数弹窗与中枢列表在下一段。
- **不持久化 `saved=true`**：已保存是每次由定义事实推出来的，删掉定义就会退回
  「保存」。缓存只活在一次渲染里。

### 17.4 验证证据

- `go test ./internal/workflow/ -count=1`：名字规则、落点预览、草稿预填、
  保存内容取运行的脚本、按脚本找定义、再次运行的请求形状。
- `go test ./internal/desktop/ -run TestWorkflow -count=1`：跑完的卡给「保存」，
  存过的卡给「再次运行」与「已保存」芯片，卡片与详情两个面说法一致，
  可恢复的运行给「恢复」、被替代的不给，保存表单的四个字段与落点预览，
  不合规则的名字当场拒绝且表单不关，没有脚本的运行说清楚原因，
  同名定义报覆盖而另一个作用域的同名不算覆盖。
- `go test ./internal/desktop/ -count=1`（全量）与 `go test ./internal/engine/ -count=1`
  （全量）通过；`go build ./...`、`gofmt -l internal/` 无差异。

这些是确定性单元与组件测试，只证明规则与界面结构成立，不证明真实模型回合、
远程宿主与视觉一致性。

## 18. 工作流中枢与启动（已落地）

这一段补上的是「已保存的定义在哪里被看见、怎么被启动」。定义与运行事实仍然
全部来自核心；中枢只做分组、展示和一次启动请求。

### 18.1 规则（`internal/workflow/hub.go`）

- **按作用域分组**：项目定义归项目组，全局定义归「本机」组。当前项目的组排
  最前，其余按项目顺序；远端项目保留宿主身份（`ProjectRef.Remote`），路径不作
  为身份。定义所属项目已不在快照里的，单列一组并写明「项目已不存在」——定义
  还在，不能静默消失。
- **组内按最近修改**（`UpdatedAt`）倒序，同时间按名字，顺序不依赖 map 迭代。
- **历史是定义自己的运行**：`DefinitionRuns` 只按 `DefinitionID` 归集，最新在前，
  可截断；`DefinitionLastRun` 取最新一次。
- **最近产物取「最近一次成功且真有产物」的运行**（`DefinitionArtifacts`）：
  更新的失败运行不能盖掉早先已经交付的东西。
- **卡片参数行有界**：`ArgumentPreview` 最多三个名字 + 剩余数量。
- **启动值的规则集中在 `LaunchArgs`**：未知参数拒绝；number 必须能解析且有限；
  json 必须能解析；空 string/number/json 不发送；boolean 一律显式发送 true/false；
  缺值先退到声明默认值，仍为空且必填才报错。错误按声明顺序报第一个，并带上
  参数名（`LaunchIssue{Name, Kind}`），文案由界面负责。
- **再次运行预填**（`LaunchPrefill`）：用那次运行实际用过的实参；定义里已经删掉的
  键忽略，缺的退回声明默认值。

### 18.2 界面（`internal/desktop/settings_workflows.go`、`workflow_launch.go`）

- 中枢是自动化页里的「工作流」模块，用共享控件：搜索框、新建入口、
  按组列表、每行「名称 + 两行说明 + 参数行 + 最近运行状态」。整行进详情，
  右侧「运行工作流」按钮不冒泡进整行。
- 侧栏不给工作流单开一条路：侧栏点「自动化」，页内切到「工作流」模块就是中枢
  （见 §26）。
- 模式行显示**当前生效的模式**（关闭 / 按需 / 始终开启），并给一个通往通用设置
  的入口；关闭时中枢仍可管理定义，只是会话里不出现工作流工具。
- 详情页分「定义 / 历史」：定义为元信息（阶段数、运行次数、更新时间）、参数声明
  表、**只读脚本**（等宽、可选中、不提供编辑）；历史列出这个定义自己的运行，
  点一行进该运行的详情。
- 不同动作分开：**复制路径**（落点预览，走剪贴板）、**提升到本机**（另存一份
  全局定义，项目那一份原地不动）、**删除**（先问一句）。
- 启动表单（`workflow_launch.go`）：
  - 项目定义且没有参数 → **不弹空表单**，直接起。
  - 全局定义 → 始终先选**本机项目**（远端项目不是候选）；没有本机项目时写明原因。
  - 项目定义 → 项目固定显示，不可改。
  - 参数按声明顺序：string/number 单行、boolean 开关、json 多行；带说明、必填星号
    与默认值。
  - 校验失败留在表单里，按声明顺序滚到第一个出错字段并聚焦它，错误文案紧跟字段。
  - 启动是两步：先 `session.create`（新会话），再在那个会话里 `workflow.launch`；
    两次快照之间不丢待办（`syncWorkflowLaunch`），运行一出现就选中该会话并打开
    运行详情 tab。被拒绝时把理由写回表单，并**尽力收回**刚建的空会话。
  - 直接起的那一类不画表单：点下去就在等运行，不会闪一张空弹窗。
  - 提交中禁用取消与再次提交；「accepted」不等于「completed」，界面只等运行出现。
  - 产物链接打开的是**交付它的那一次运行**，不是最近的那一次。

### 18.3 明确取舍

- **不重写定义编辑器**：定义由对话里的主代理写，中枢只读、只删、只另存。
  这与共享规范一致：新增/编辑走弹层，而中枢本身不做编辑。
- **提升不是改 scope**：另存一份，避免把项目那份悄悄搬走；这也让「复制路径」
  和「提升」保持两个不同动作。
- **刷新入口没有伪造**：定义随快照推送，中枢不需要一个假的刷新按钮；搜索是
  纯前端过滤，空结果会说明「是搜索把它藏起来了」。
- **历史的截断**：中枢最多列 50 条，完整查询仍在核心与运行详情。

### 18.4 验证证据

- `go test ./internal/workflow/ -count=1`：分组（当前项目在前、远端身份、项目已消失
  仍列出）、组内最近修改倒序、历史归集与截断、最近产物的取法、参数行有界、
  再次运行预填、`LaunchArgs` 的五类拒绝与默认值。
- `go test ./internal/desktop/ -run 'TestWorkflowHub|TestWorkflowLaunch' -count=1`：
  中枢列出各组与「从未运行」、空库与空搜索两种空态、详情把脚本与元信息分开、
  历史只含本定义的运行、无参数直接起、全局定义必须选本机项目、参数表单排出
  声明顺序、校验失败留在表单、再次运行预填、两步启动（会话先出现）、
  拒绝回到表单、删除先问、字段在帧之间是同一个指针且真实输入能留住、
  提升另存不改项目那一份、侧栏没有工作流入口而中枢在自动化页的模块里、直接起不闪空表单、
  产物归属到交付它的那次运行。
- `go test ./internal/desktop/ -count=1`（全量）、`go test ./internal/workflow/
  ./internal/i18n/ ./internal/contracts/ ./internal/design/ -count=1` 通过；
  `go build ./...`、`gofmt -l internal/`、`go vet` 无输出。
- 渲染探针（`ui.Tester` 出图，人工看图后删除探针）确认中枢列表、定义详情与
  启动表单的三段式布局成立。

这些是确定性单元与组件测试，只证明规则与界面结构成立，不证明真实模型回合、
远程宿主与视觉一致性；中枢启动链路与真实核心的联调尚未做端到端验收。

## 19. 原生端到端验收（中枢启动链路）

### 19.1 怎么跑

原生端到端测试统一走 `scripts/desktop-e2e`（它负责构建原生二进制、设置
`RCODE_E2E_BINARY` / `RCODE_E2E_ARTIFACTS`，并把证据留在
`output/desktop-e2e/<本次运行>/`）：

```sh
go run ./scripts/desktop-e2e                                   # 全量
go run ./scripts/desktop-e2e -run '^TestDesktopLocalWorkflowHub$' -timeout 10m
go run ./scripts/desktop-e2e -run TestDesktopLocalSettingsPersistence -out-dir output/native-tests
```

条件：macOS/Windows/Linux 的**图形桌面会话**（纯 SSH、容器、无桌面服务器不能作为
原生验收环境；Linux CI 用 `xvfb-run -a`）。测试窗口会真的显示出来。直接
`go test -tags=e2e ./internal/e2e` 会因为缺环境变量而失败，那不是通过。

证据目录里有：`evidence.json`（本次运行的结论）、`tests.jsonl` / `tests.log`、
每个 `capture` 点的 PNG 与完整无障碍树 `*-tree.json`、原生应用日志 `app.log`。
`visualParity` 字段恒为 `not-assessed`：自动化不证明视觉一致性。

### 19.2 本轮新增的用例

三条用例，每条都跑深浅主题两遍，都用真实本机核心 + 本地 HTTP 模型夹具：

**`TestDesktopLocalWorkflowHub`（`internal/e2e/workflow_test.go`）** —— 中枢那条路：

1. 夹具阶段先在真实核心里保存一份项目工作流定义（并建一个项目、存一个本地供应商）；
2. 关掉核心，用同一个数据目录重启原生应用（证明定义真的落盘，不是内存里的假象）；
3. 侧栏进自动化页，再切到「工作流」模块，断言已保存的定义列在它所属的项目组里；
4. 进详情页，断言**只读脚本**在页面上；
5. 从详情页「运行工作流」启动，断言落盘的运行**真的完成**且挂在新会话上；
6. 运行详情页对这个跑完的运行给「再次运行」，点了之后出现第二次运行。

**`TestDesktopLocalWorkflowConfirmationAndRunCard`（`workflow_card_test.go`）** ——
模型发起、用户批准那条路：

1. 主代理这一轮调用 `CreateWorkflow`（六阶段的脚本），确认窗出现；
2. 点「允许一次」，运行开始，**运行卡出现在发起它的那一轮末尾**；
3. 断言卡上确实有阶段线，且阶段线溢出时出现共享细滚动轨
   （`workflow-timeline-scrollbar:*` 只在内容真超宽时画）；
4. 断言落盘的运行完成，并且**锚定在那一轮**（`MessageID` 非空）。

**`TestDesktopLocalWorkflowHoleShowsOnTheCard`（`workflow_hole_test.go`）** ——
留白那条路：

1. 实例会话在模型夹具里回一处留白标记；
2. 断言留白真的记进了运行（标题就是实例写的那句）；
3. 断言卡上出现带笔的「1 处留白」芯片（深色主题按中文断言）；
4. 点芯片打开运行详情，断言详情里写着这一处留白的标题。

### 19.3 端到端跑出来的一个真问题

第一遍跑这条用例就红了：中枢里空的。原因不在界面，而在**持久化**：
`persistedSnapshot` 里没有 `workflowDefinitions` / `workflowRuns`，所以已保存的定义
和运行历史在关掉应用后就没了——中枢在重启后是空的，保存与「再次运行」也就无从谈起。

修法（`internal/engine/persistence_index.go`）：把两者纳入写盘快照，写盘前深拷贝
运行里的嵌套切片（阶段、实例、留白、产物等，执行线程会就地改写它们），读回时还原。
启动时仍在跑的运行仍由 `recoverWorkflowRunsLocked` 标成中断并可恢复。

新增引擎用例 `TestWorkflowDefinitionsAndRunsSurviveRestart`：保存定义、跑完一次、
关掉核心、用同一目录重开，定义与运行历史都在，已结算的运行仍是已结算。

### 19.4 验证证据

- `go run ./scripts/desktop-e2e -run '^TestDesktopLocalWorkflow' -timeout 15m`：
  三条用例、深浅主题共六遍全部通过；证据
  `output/desktop-e2e/20261121-093730-2529714639/evidence.json`
  （`passedTests` 里三条都在，`visualParity: not-assessed`、`realProvider: not-tested`）。
  截图与无障碍树同目录，已人工看过中枢列表、定义详情、运行卡、留白芯片四张。
- `go test ./internal/engine/ -run TestWorkflowDefinitionsAndRunsSurviveRestart -count=1`：
  重启后定义与运行历史都在。
- `go test ./internal/engine/ -count=1`（全量）、`go test ./internal/desktop/ -count=1`
  （全量）、`go test ./internal/workflow/ ./internal/i18n/ ./internal/contracts/ -count=1`
  通过；`go build ./...`、`gofmt -l internal/`、`go vet` 无输出。
- `go vet -tags=e2e ./internal/e2e/` 通过（不开窗口即可校验 e2e 编译）。

### 19.5 本轮补的另外两条原生用例

**`TestDesktopLocalWorkflowFillHoleContinuesTheRun`（`workflow_fill_test.go`）** ——
留白补全之后运行继续：实例提出留白 → 卡上出现留白芯片 → 用户催一句，主代理在
下一轮调用 `FillWorkflowHole`（夹具从落盘状态里读出 run id 与 hole id）→ 断言
留白变成 `filled`、正文就是补进去的那段、**同一个运行**继续跑到完成。

**`TestDesktopLocalWorkflowQuestionShowsAndIsAnswered`（`workflow_question_test.go`）** ——
问题的来回：实例把问题升级给主代理 → 卡上出现待答芯片（**不是让用户直接答题的
表单**）→ 点芯片打开运行详情，详情里写着问题 → 用户催一句，主代理调用
`ResolveWorkflowQuestion` 作答 → 断言答案真的送达，运行继续到完成。

模型夹具按**请求体内容**分流（实例会话 / 工具结果续接 / 主代理新回合），因此不依赖
请求到达的先后。

### 19.6 整套原生套件的回归

73 条原生用例按四批跑完（`A–F`、`G–M`、`N–S`、`T–W` 加其余）：

- 一次**真实回归**被这套回归抓到：`TestDesktopLocalAutomationsAndIsland` 断言岛在
  展开后是 440×168，而早前的岛改版把行高从 112 改成 48（组件测试已经跟上，共享
  几何 `design.IslandGeometry` 是 `56+count*48`，展开后 440×104）。原生断言还写着
  旧数字。改成直接问共享几何，不再写死一个会过期的值。
- 两条**早就存在的时序 flake**：`TestDesktopLocalModelRetry`（悬停提示没等到）与
  `TestDesktopLocalTranscriptFoldMotion`（长转录滚到底）。它们单独跑都过；后者在
  **干净的 HEAD 工作树上也一样挂**（只是这次轮到另一个主题），因此不是本轮改动
  带来的。本轮如实记下，没有把它们算成通过。

### 19.7 这一路的证据边界

这些链路用的都是本地 HTTP 模型夹具，**不代表真实供应商或远程宿主验收**；截图也
不证明视觉一致性。原生套件里的历史用例仍有一条时序 flake 未修（见 §19.6）。

## 20. 完成卡（已落地）

§4.2 剩下的最后一块显示。

### 20.1 规则（`internal/workflow/completion.go`）

- **只服务成功且有交付的运行**：`Status == completed` 且至少发布过一件产物。
  失败、停止、还在跑、问题或留白升级都没有完成卡——没交付就不画交付。
- **主产物**：显式 primary 优先，唯一交付自动成为主产物，多件又没 primary 时
  **不伪造预览**，只画索引。
- **索引有界**：最多六行；超了就是五行加「还有 N 个」（`CompletionOverflowRows`）。
- **四项统计都取自事实**：耗时 = 结束减开始（缺结束时间就是未知）；进入阶段数
  **只数真的进过的阶段**（`EnteredAt` 非 nil），不是声明数；子代理数 = 运行创建的
  实例数；Token 来自运行自己那些实例会话的模型请求，一次可用的计数都没有就是未知。
- 未知一律留空给界面显示 `—`，不拿 0 冒充。

### 20.2 界面（`internal/desktop/workflow_completion.go`）

- 轮尾顺序按 §4.1：主正文 → **完成卡** → 运行卡 → 其他尾部内容。完成卡排在运行卡
  之前，因为读者先要知道「交付了什么」，再看「哪次运行交付的」。
- 卡不折叠：标题行（状态词 + 名称 + 已交付件数）→ 主产物预览（等宽、可选中、
  最多 8 行）→ 其余交付索引（点一件打开该产物）→ 四项统计。
- 耗时走共享 `i18n.Duration`，整秒；数值用等宽数字（`tnum`）。
- Token 从运行实例会话的模型请求汇总（`session.ParentRunID == run.ID` 且
  `Usage.Known`）；没有任何可用计数时显示 `—`。

### 20.3 与参考实现的差别

- 参考实现的完成卡属于「后台结果轮」。GCode 目前没有独立的后台结果轮，运行卡挂在
  发起它的那一轮末尾，因此完成卡也落在同一轮、运行卡之前。这是同一轮尾的排序，
  不是新造一种消息。
- 参考实现的耗时有时省略秒或显示小数；GCode 一律整秒，走共享格式化。

### 20.4 验证证据

- `go test ./internal/workflow/ -run TestCompletion -count=1`：只认成功且有交付的运行；
  主产物与索引的取舍；索引 6/5+N；进入阶段与子代理计数；未知保持未知。
- `go test ./internal/desktop/ -run TestWorkflowCompletion -count=1`：卡上的预览、
  索引与四项统计；失败/停止/无交付都不画卡；未知显示 `—`（不出现 0）；无 primary
  时只画索引；完成卡确实排在运行卡上方。
- `go test ./internal/desktop/ -count=1` 全量通过；`go vet ./...`、`gofmt` 无输出。
- 原生：`TestDesktopLocalWorkflowConfirmationAndRunCard` 里实例发布一件产物后，
  轮尾先出现完成卡（标题、主产物正文、四项统计），深浅主题都通过；
  证据 `output/desktop-e2e/20261123-013652-28639628/evidence.json`。

## 21. 两个复现问题的修复

### 21.1 长转录折叠用例的时序 flake

`TestDesktopLocalTranscriptFoldMotion` 在干净 HEAD 上也失败（失败的主题每次不同）。
根因：详情是一个虚拟列表，要用滚轮把它滚到底才能挂上最后一行；测试只在一个高度上
发一次滚轮，若那一刻视口顶部正被悬停控件压着，滚轮就落到别处，列表不动。

修法（只改测试，不改产品）：先等布局落定，再在视口内**上中下三处**各发一次滚轮
（滚轮路由按指针下的命中链选取目标）；没到底就再来一次。连跑六次全过。

### 21.2 无障碍树读取时的崩溃（产品缺陷，已修）

原生跑工作流用例时应用整个 SIGSEGV：

```
panic: runtime error: invalid memory address or nil pointer dereference
github.com/egoist/mygo/ui.(*ListState).cursor (list.go:893)
github.com/egoist/mygo/ui.(*engine).accessDetails (access.go:1008)
```

推断的原因：某个列表帧的 `rowsOf` 还在，但它的 `ListState` 已经不在了；
无障碍访问在现代码里读它，于是空指针崩掉整个应用——辅助技术或自动化读树都会踩到。

修法（框架内，最小面）：
- `(*ListState).cursor` 对空 receiver 返回 nil（唯一收口，所有调用点安全）；
- `accessDetails` 与焦点路径在 `f.s == nil` 时跳过列表的选择态；
- `listInput` 同样先判空。

框架改动按仓库约定登记为补丁：新增
`patches/mygo-v0.9.1-24-list-state-a11y-guard.patch`，写进 `scripts/frameworkpin/pin.go`
的补丁链与 `third_party/mygo.lock.json`，指纹随之更新
（`d8cc306d…`, 2852 files）。`go run ./scripts/check-framework.go` 通过。

### 21.3 验证证据

- `go run ./scripts/desktop-e2e -run '^TestDesktopLocalWorkflow' -timeout 9m`：连跑两次，
  每次五条、深浅主题共十遍全过；其中含曾崩过的 `…ConfirmationAndRunCard`。
- `go run ./scripts/desktop-e2e -run '^TestDesktopLocalTranscriptFoldMotion$'`：连跑四次全过。
- `go run ./scripts/check-framework.go`：`Verified MyGo v0.9.1 + rcode-native-ui.28`。
- `cd third_party/mygo && go test ./ui/ -run 'TestList|TestAccess' -count=1` 通过。
- `go build ./...`、`go vet ./...`、`gofmt -l` 无输出；`./internal/desktop/` 全量通过。

## 22. 时间线边檐（已落地）

§5.2 要求的「横向 overflow 不露原生粗滚动条；边檐指示视外阶段，点击跳到相应站」。

### 22.1 规则（`internal/workflow/ledge.go`）

纯几何，和参考实现同一套尺寸：灯 10、间距 16、内边距 8、淡出 40、最多三枚灯、
`+n` 占 26、残段最短 6。

- **折叠是展到不动点**：边檐越宽，压在它底下的站越多，所以要反复算到集合不再变化。
  站算在视口哪一侧，用的是它的灯心减去滚动量。
- **镜头正飞向的站不折**（`keep`）：平滑滚动在飞的那几百毫秒里，它按陈旧的滚动量
  算在视野外，可它正要进来——檐上闪一枚灯是误报。
- **檐上最多三枚灯，靠内容的一端优先**：左檐留末三枚，右檐留前三枚，其余进 `+n`。
- **残段用槽不用灯**：右檐从那一站的槽尾量起，否则轨道会横穿站名。

### 22.2 界面（`internal/desktop/workflow_timeline.go`）

- 有站折出去时，阶段线上方多出一条边檐带：左边折的站在左、右边折的站在右，
  每枚灯是自己那一站的颜色（留白仍是警示色，不另立图例），超出的显示 `+N`。
- 每枚灯是一枚按钮，无障碍名是「滚动到 <站名>」；选中后把那一站滚回视口靠这一侧的
  位置（`ScrollTo` + `ui.Smooth`）。
- 放得下时整条带不出现，也不占高度。

### 22.3 与参考实现的差别

参考实现的边檐**压在视口边缘**，并给轨道与药丸带各自 40px 渐隐遮罩。GCode 这一版
把边檐放在阶段线自己的带里（在轨道行上方），不叠在滚动层上——它同样回答「外面还有
几站、都是哪几站、点一下回到哪」，但不做遮罩渐隐。这是显示位置的取舍，不是能力缺失；
真正的遮罩需要给滚动层开口子，留到统一视觉那一轮。

### 22.4 验证证据

- `go test ./internal/workflow/ -run 'TestLedge|TestFold' -count=1`：边檐宽度（0/1/3/5 枚）、
  视口内不折、往回滚时左侧折叠、镜头目标不折、右侧折叠、视口没量到不折、
  三枚灯与靠内容一端、残段用槽、没有开着站时不画残段。
- `go test ./internal/desktop/ -run TestWorkflowTimelineLedge -count=1`：放不下时折到檐上
  并可点回滚、放得下时不出现、留白站在檐上仍是警示色。
- `go test ./internal/desktop/ -count=1` 全量、`./internal/workflow/ ./internal/i18n/
  ./internal/contracts/ ./internal/design/` 全通过；`go build ./...`、`gofmt` 无输出。
- 原生：`go run ./scripts/desktop-e2e -run '^TestDesktopLocalWorkflow' -timeout 9m`
  五条用例、深浅主题共十遍通过；证据
  `output/desktop-e2e/20261123-031937-3969543141/evidence.json`。
- 渲染探针（出图后删除）确认：六阶段卡片在窄宽度下右侧出现三枚绿灯加 `+1`。

## 23. 现在的缺口与理由

复刻的显示层到这轮为止已经齐了：运行卡、时间线（含留白站、横向滚动、边檐）、
详情脊柱、问题与留白、脚本步骤（见 §24）、完成卡、中枢与启动、保存与再次运行。

最后一个语义待定项已在 §25 落地：**运行目录**没有照参考实现列文件，而是改成
「这条对话的工作流运行目录」（运行历史的列表页）。理由与取舍见 §25.1。

其余都已有确定证据：单测、组件测试（`ui.Tester`）、原生端到端（深浅主题）各就各位，
逐条记在 §14–§22。

## 24. 脚本步骤（已落地）

§8.3 的「脚本药丸打开执行日志并按阶段定位」：日志本来就在，缺的是一条只读的读法。

### 24.1 能力边界（`contracts.WorkflowJournalService`）

日志读写归核心，界面只读。仓库里已有若干这种「宿主可选能力」接口
（`ResourceService`、`ApplicationUsageService`、`ModelMetadataService`），所以这里
照同一副样子加，而不是把 `Service` 主接口撑大：

```go
type WorkflowJournalService interface {
    WorkflowJournal(ctx context.Context, runID string) (WorkflowJournal, error)
}
```

返回的是 `contracts.WorkflowJournal{Records, Damaged}`：记录带序号、种类、时刻与
载荷；`Damaged` 数出读不出来的行——一条坏行不能把周围的事实一起藏掉。

**为什么不在 `contracts` 里直接用 `workflow.JournalRecord`**：`internal/workflow`
依赖 `contracts`，反过来引用会成环。因此契约层有自己的同形记录，跨越边界时做一次
字段映射（不是序列化）。

实现：
- **引擎**（`internal/engine/workflow_journal.go`）：复用
  `workflow.JournalPath` + `workflow.ReadJournal`。**运行必须是这台引擎认识的**——
  运行身份会拼成文件路径，未知身份不能变成路径；空身份同样拒绝。
- **mock**：确定性宿主没有日志，返回空日志（不编造步骤）。
- **远程**（`internal/remote/workflow_journal.go`）：`GET /v1/workflow-journal?runId=`，
  客户端实现同一个接口；宿主不支持时 501。

### 24.2 规则（`internal/workflow/steps.go`）

- 按日志顺序把事件折成**按阶段分组**的步骤：阶段组保持它第一次出现的次序，
  运行自己的事实（开始、结算、停止、设置变更、产物）单独一组。
- 每组一步一行：序号、种类、一眼能读的那条事实（名字、标题、答案、原因），
  不把整段载荷倒出来。
- 载荷坏掉的行仍然成为一步，只是没有细节——不猜内容。

### 24.3 界面（`internal/desktop/workflow_steps.go`）

- 运行详情里多一块「步骤」：刷新按钮 + 每组一个可折叠表头（组名 + 步数）+ 步骤行。
- **读一次就够**：打开详情时读一次；`workflow.changed` 事件到了才再读一次
  （按事件序号更新，不每帧重拉）。每个渲染帧都不会触发读取。
- 组独立保存折叠状态；宿主没有这个能力时说「这个宿主不提供运行步骤」，
  读取失败说清原因，都不伪装成「这次运行没有步骤」；坏行数如实显示。

### 24.4 验证证据

- `go test ./internal/workflow/ -run TestBuildSteps -count=1`：按阶段分组与首次出现次序、
  各类事件的细节文案、坏载荷仍成一步、空日志。
- `go test ./internal/engine/ -run TestWorkflowJournalReadsOneRunsOwnLog -count=1`：真实运行
  写下的日志能读回、含 run.started/phase.entered/run.settled、未知与空运行身份被拒。
- `go test ./internal/desktop/ -run TestWorkflowStepsPanel -count=1`：按阶段分组、只读一次
  （三帧仍是一次）、点刷新再读一次、`workflow.changed` 触发一次重读而别人的事件不触发、
  组独立折叠、缺能力与读失败各自说清楚、坏行计数。
- `go test ./internal/desktop/ -count=1` 全量、`./internal/workflow/ ./internal/contracts/
  ./internal/mock/ ./internal/remote/ ./internal/i18n/` 全通过、`./internal/engine/ -run
  TestWorkflow` 通过；`go build ./...`、`go vet ./...`、`gofmt` 无输出。
- 原生：`TestDesktopLocalWorkflowHub` 里断言运行详情出现「步骤」与「运行开始」，
  截图确认一次真实运行留下六步、按阶段分成两组（「这次运行」2 步、「plan」4 步）；
  证据 `output/desktop-e2e/20261123-034338-3605803682/evidence.json`。

## 25. 运行目录（已落地）

参考实现（`WorkflowWorkspaceSidePane` 一族）的「运行目录」列出**这一次运行涉及的工作
目录与文件**。GCode 里没有对应的事实：运行没有自己的工作目录，工作目录归会话与项目，
文件改动由 git/diff 视图负责。照抄会造出一份和会话、工作区都对不上的第三次事实。

### 25.1 取舍

三个候选：

| 方案 | 内容 | 判断 |
| --- | --- | --- |
| A 不做 | 没有这个入口 | 用户看不到「这条对话跑过哪些工作流」，运行只能从对话里翻 |
| B 照参考列文件 | 从工具调用推断运行碰过的目录与文件 | 推不出可信清单：宿主的读取、脚本、子代理各自不同，推断结果会漏也会错，且与会话工作区重复 |
| **C 改成运行目录**（已选） | 这条对话所有工作流运行的列表页：还在跑的、已结束的，点一行进那一次 | 用已有的权威事实（运行归会话），不新增推断；补上「从一次运行看到同类」的缺口 |

选择理由：**只展示核心已经拥有的事实**。运行条目对会话是一对多，这份列表本来就在
`state.WorkflowRuns` 里，界面只是按会话取一次、排一次序，不需要新状态、新命令或新持久化。

### 25.2 规则（`internal/workflow/directory.go`）

- `BuildRunDirectory(runs, sessionID, limit)` 只取 `SessionID` 相同的运行；会话身份为空
  直接空页（不把「全部运行」当成「这条对话的运行」）。
- 分两桶：`Running`（pending、running，还在动）与 `Ended`（其余，状态词自己说明怎么结束）。
- **排序只在这里做一次**：`UpdatedAt` 倒序，其次 `StartedAt`，最后 `ID` 兜底——两处各排
  一次会让行在任一侧重读时跳动。
- 行里带：名称、状态（被取代的运行改说「已被取代」而不是藏在「已停止」后面）、
  阶段数、产物数、并发数、起止时间。
- `RunDirectoryLimit = 64`：取满就 `Truncated`，页面直说「更早的运行没有列在这里」，
  不用过滤后的行数假装「就这些」。行数上限只有这一处，页脚计数与列表不会打架。

### 25.3 界面（`internal/desktop/workflow_directory.go`）

- 工作台新增 surface `workflow-runs`：标题按会话标题命名（「<会话> 的运行」），
  副标题是运行总数（等宽数字）。
- 两组（「运行中」「已结束」）各是一个带表头的块，行是共享行按钮：状态词着色、名称、
  阶段数、耗时（`i18n.Duration` 整秒）。整行可点，点开那一次运行的详情。
- 空对话说「这条对话还没有跑过工作流」，不画空表、不画空表头。
- 入口在运行详情头部：**「这条对话的所有运行」**（图标 checklist），会话为空时禁用。
  这样形成闭环：对话卡 → 运行详情 → 全部运行 → 另一次运行的详情。
- 全部使用共享控件与语义色，没有新的视觉规格。

### 25.4 验证证据

- `go test ./internal/workflow/ -run TestBuildRunDirectory -count=1`：两桶划分与「最新在前」、
  只取本会话、页上限与 `Truncated`、空会话身份、行内事实（停止原因/产物数/阶段数/并发）。
- `go test ./internal/desktop/ -run 'TestWorkflowRunDirectory|TestWorkflowRunPaneOpensTheDirectory'`：
  列表按会话标题命名并分两桶且「运行中」在上、点行进详情、详情里的入口开目录、
  空对话说清楚且不出空表头。
- `go test ./internal/desktop/ -count=1` 全量 281s 通过；`./internal/workflow/ ./internal/i18n/
  ./internal/workbench/` 通过；`go build ./...`、`go vet ./...`、`gofmt -l internal/` 无输出。
- 原生：`TestDesktopLocalWorkflowHub` 追加目录闭环——运行详情点「这条对话的所有运行」→
  目录 tab 出现该运行的行 → 点行回到同一运行详情；深浅主题各一遍通过，
  证据 `output/desktop-e2e/20261123-042931-2816286804/evidence.json`，截图
  `hub-runs-directory.png`（标题「Runs of New conversation」、副标题「1 runs」、
  「Ended」组一行「Completed Native-review 1 phases 0s」）。

### 25.5 全量原生套件回归

分批跑完本机可执行的全部 71 条原生用例（`internal/e2e` 共 73 条，其中
`TestDesktopApplicationIcon`、`TestDesktopWindowCaptionControls` 是 `_windows_test.go`，
macOS 不编译），全部通过：

| 批次 | `-run` | 条数 | 证据目录 |
| --- | --- | --- | --- |
| 1 | `^TestDesktop(ApplicationIcon|Local[A-C])` | 9 | `20261123-043614-2929176165` |
| 2 | `^TestDesktopLocal[D-H]` | 7 | `20261123-043806-3640396151` |
| 3 | `^TestDesktopLocal[I-M]` | 10 | `20261123-044036-2763380966` |
| 4 | `^TestDesktopLocal[N-S]` | 27 | `20261123-044218-4293376980` |
| 5 | `^TestDesktop(Local[T-Z]|PromptPicker|Scenario|SettingsTitlebar|Unconfigured|WindowCaption)` | 18 | `20261123-044547-3772165025` |

证据里 `visualParity` 恒为 `not-assessed`、`realProvider` 恒为 `not-tested`：
自动化不证明视觉一致性，也没打过真实供应商。

## 26. 入口纠正（已落地）

### 26.1 问题

参考实现里工作流中枢是设置页里的一个模块（§10.1：「外框仍是共享设置页，
『自动化 / 工作流』是模块切换，不另造独立应用」）。复刻时把它落成了
「侧栏一条『工作流』入口 + 设置页一个『Workflows』分节」，两处都属于原项目
没有的额外入口：原项目侧栏没有工作流项，设置页也没有独立的工作流分节。

### 26.2 现在的做法

- 删掉侧栏的「工作流」入口（`internal/desktop/sidebar_view.go`）。侧栏只留
  「自动化」，工作流不再占一条路。
- 删掉设置页的「Workflows」分节（`internal/desktop/settings.go`：分节列表、
  分节图标、分节渲染三处）。设置页不再有中枢页面。
- 中枢改挂在自动化页内做模块切换（`internal/desktop/automations.go`）：
  页面顶部一行「自动化 / 工作流」，选「工作流」时主体换成中枢
  （`a.workflowHubView`）。侧栏点一次「自动化」，页内再切模块。
- General 分节里的工作流**模式**开关（关 / 按需 / 始终开）保留：那是功能
  模式，不是中枢页面。

### 26.3 与参考实现的差别

- 外框不同：参考实现在共享设置页里做模块切换，这里在自动化页里做。原因是
  GCode 的自动化本来就是独立路由（`route == "automations"`），工作流跟着它
  走比塞回设置页更一致；「不另造独立应用」这条仍然守住。
- 模式开关仍留在 General 分节：参考实现把模式放在标题区可见，这里保留原位，
  中枢里有一行「模式: … 」加一个「打开设置」链接指过去。

### 26.4 验证证据

- `go test ./internal/desktop/ -run TestWorkflowHub -count=1`：中枢用例改在自动化
  页的工作流模块里渲染后全过；新用例断言侧栏没有「Workflows」、自动化页点
  「Workflows」切到模块、模块里列出项目组与本机组定义。
- `internal/e2e/workflow_test.go` 的原生路径改为「侧栏 → 自动化 → 工作流模块」。
