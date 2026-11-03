# Composer footer：源码审计与实施边界

2026-10-30 当前状态：权限入口、PR 入口和配置、项目动作及本地账户菜单已移除。
正文保留当时的源码审计与验收证据；当前实施范围见
[UI 残留清理](ui-residual-cleanup.md)，不再把这些旧入口列为待实现功能。

2026-11-04：GCode 的分支刷新时机和默认 checkout 语义已修正，见
[Git checkout 当前实现](thread-naming-worktrees.md)。
分支列表优先展示当前仓库上次成功加载的数据，同时立即在后台刷新；
刷新失败保留旧列表并显示错误与重试。下文“普通 render 只从 Idle 拉取”等描述
保留为冻结参考审计，不表示当前 GCode 会因已有 Ready 缓存而跳过刷新。

日期：2026-10-28。事实源固定为 Zeron v0.2.128
`ab189159f10cf8e63c0502be75a59bbc487e0eaf`，只读目录
`output/reference-sources/zeron`。原版 UI 文件名简写从 `crates/ui/src/` 解析；
`proto/`、`engine/`、`doc/`、`client/`、`harness/` 前缀从 `crates/` 解析，显式
`crates/...` 路径则从参考根解析。本项目路径相对当前 checkout。
初版是实施前源码审计；第5、6节保留当时基线，随后实现状态见第5节开头及
`contracts.md`。编译、单元测试和现有 picker 截图均不能证明 footer 视觉一致。

实现时继续使用现有 typed Service、状态化进程内 mock 和原生 MyGo 控件。不要为
footer 再建一套服务、账户缓存，或把持久 `ui.Element` / `ui.Context` 放进状态。

## 1. 槽位与内容是两个条件

`composer.rs:95` 定义 `SESSION_FOOTER_HEIGHT=24`。底槽高度依据
`composer.rs:12170–12243`，不能只依据“有无 Git”或源码前面的注释：

```text
newThreadChrome = dockFrame.selectors()，没有 frame 才走 route fallback
sessionChrome = 1 - newThreadChrome
bottomSlot = (selectedSpace.gitDetected || dockFrame存在) ? 1 : sessionChrome
height = 24 * bottomSlot
marginTop = -SPACE_SM * (1 - bottomSlot)
marginBottom = -SPACE_SM * bottomSlot
```

主壳 `shell.rs:11651–11659` 每次 render 都向 composer 设置 `DockFrame`；
侧聊 `shell/side_chats.rs:428–430` 设置 `DockFrame::settled(true)`。
`composer_dock.rs:69–86,187–195` 的稳定值为 hero：selectors=1/footer=0，
settled：selectors=0/footer=1。因此实际壳中的 hero 非 Git 项目仍有 **24px
空槽**，而不是高度零。无 DockFrame 的独立 composer 才走非 Git 动态高度分支。

| 稳定场景 | 底槽 | 左侧 / Git 内容 | 尾部内容 |
| --- | --- | --- | --- |
| 主 hero，Git | 24px | 可交互 checkout → ref，Cloud 隐藏 checkout | 没有 session PR、账户环、上下文环 |
| 主 hero，非 Git / No project | 24px（主壳已传 frame） | 空；上方 project/device 仍可存在 | 空 |
| 主 settled，有 Git session row | 24px | 只读 checkout-kind → session ref；满足关联才有 PR | 可选账户环 → 可选上下文环 |
| 主 settled，非 Git / project row 尚无 Git 信息 | 24px | `render_footer` 返回 None；不画 checkout/ref/PR | 两种环仍各自按数据条件决定，不能一起隐藏 |
| 侧聊 settled，Git 或非 Git | 24px | 不画 checkout/ref/PR | 仅本侧聊自己的可选上下文环；不画账户环 |
| 无 frame 的独立 hero，非 Git | 0 | 空 | 空 |
| 无 frame 的独立 composer 过渡 | `24 * sessionChrome`（Git 则固定24） | 由各自 opacity 决定 | 由 session opacity 决定 |

依据：`composer.rs:6929–6933,11613–11627,12073–12089,12174–12238`；
`pickers.rs:4321–4390,4426–4488`；`account_usage.rs:430–470`。
side 的 `harness=None` 禁止账户环，但其 AccountUsage 仍读取自己的
`AppState.context_usage`，不是借主会话的用量。

过渡时直接使用 dock 的 selectors/footer 值，不能让 hero chips 和 session
footer 同时静态挂出。`composer_dock.rs:198–217`：进入 session 时 selectors
在 0–0.25 阶段离开、footer 在 0.25–0.55 阶段进入；返回时 footer 先在
0–0.18 阶段离开，selectors 在 0.50–0.95 阶段进入。无 frame fallback 则用
`composer.rs:100–103` 的两段 opacity。完整动效可作为后续验收，稳定分支不能省略。

上方 destination 是另一行：`pickers.rs:4230–4316` 为 project → device，
`composer.rs:12119–12146` 控制其位置。独立 shell 24px status strip 也不是这个
footer，不能为了“去重”删除，见 `shell.rs:11826,12239`。

## 2. Checkout / ref：draft 有动作，session 不搬家

| 内容 | 数据与分支 | 动作 / 限制 | 原版证据 |
| --- | --- | --- | --- |
| Git 可用性 | selected space 的 `git_detected` | 非 Git 不尝试 ListRefs；不能由 branch 非空猜 Git | `pickers.rs:4325–4333,4430–4432,4493–4498,2217–2222` |
| Draft ref 行 | `RepoRef{name,current,worktree_path}`，不是 string 列表 | 320px ref popover；Loading/Idle skeleton、Error retry、空结果文案不同；全量搜索后最多展示300条，超限有 Showing 提示，键盘也限300 | `proto/src/entities.rs:430–443`；`pickers.rs:36,3784,4336–4343,4732–4870` |
| Draft checkout | Local / NewWorktree / Cloud；“Current worktree”是 Local 的一种解析结果 | checkout popover 224px，普通菜单只有 Current checkout/worktree 与 New worktree；Cloud 由 device 菜单选择 | `pickers.rs:112–143,3016–3031,4873–4914` |
| Cloud draft | Cloud repo 可达或 project 本身在 Cloud 才 `runs_on_cloud` | hero 底部只保留 ref；Cloud 消失时退回 Local 并清理旧 ref | `pickers.rs:2991–3013,2230–2243,4352–4362` |
| Ref 文案 | config.branch，否则显示当前/已解析 ref | 无 ref：`Select ref`；NewWorktree/Cloud：`From <ref>`；Local：裸名字 | `pickers.rs:3066–3094` |
| Session checkout-kind | Cloud device → `Cloud`；否则 `chat.cwd != space.path` → `Worktree`；否则 `Local checkout` | 纯只读标签，不打开 checkout 菜单；不要只用 WorktreeID 是否非空代替 cwd 比较 | `pickers.rs:4426–4452` |
| Session ref | `chat.branch`，缺失为 `No ref` | 只读、长标签尾部渐隐；与 checkout 靠左相邻，尾部 PR/rings 不跟着分支长度移动 | `pickers.rs:4453–4486,4190–4202` |
| 刚创建但 row 未同步 | selected chat ID 已有、chat row 尚无 | `render_footer` 临时保留 draft 内容，防止出现半空 locked footer | `pickers.rs:4398–4412,4491–4561` |

Refs 的请求属于项目/目标设备：本地/远端使用 `LIST_REFS(repoPath,
targetDeviceId?)`；Cloud 使用 `LIST_CLOUD_BRANCHES(repo,defaultBranch?)`。
key 包括项目或 Cloud repo/default branch。普通 render 只从 Idle 拉取，错误
等待显式 retry/reopen，不能每帧重新请求；force 刷新已有数据时保留旧 rows。
见 `pickers.rs:2212–2333`。项目/目标变更后的迟到响应必须丢弃；当前项目不能接收
上一项目的 ref rows。这是 Go 实施要求，不是声称所有原版异步分支已经逐项验收。

选择 ref 的实际语义见 `pickers.rs:2338–2423`：

1. 已有 session row 直接 return，不能移动会话。
2. Cloud 只记录将克隆的 branch。
3. 有 `worktree_path` 的 ref 自动选 Local 并复用该目录。
4. NewWorktree 或已 current 的 ref 只记录 draft pick。
5. Local 选择普通非 current ref，实际调用 `SWITCH_REF(repoPath,refName,
   targetDeviceId?)`；成功关闭并重拉 refs，失败保留菜单及错误，一次只允许一个 switch。

`pick_checkout` 从 NewWorktree 返回 Local 时，若选中的是非 current 且没有现成
worktree 的普通 ref，会清掉 draft branch，回到当前 checkout，见
`pickers.rs:2459–2477`。

首次 send 消费 **typed checkout plan**，不是消费显示字符串：
`CurrentCheckout{branch?}`、`ReuseWorktree{path,branch}`、
`NewWorktree{base?}`、`Cloud{branch?}`，定义/解析在
`pickers.rs:130–143,3034–3061`。新 worktree 是 send 时创建；refs 未加载时的
fallback 也有明确定义，不能把现有最后一个 worktree 冒充新建结果。
`composer.rs:9731–9818` 把 branch/cwd/create specification 带到新会话，随后才由
权威会话状态显示实际创建的 branch。

`NewWorktree{base=None}` 仍创建新 worktree：实际 consumer 在
`composer.rs:9758–9774` 将未指定 base 解析为 `HEAD`。`pickers.rs:137`
注释声称退回当前目录，与实际代码不符，实施按 consumer。selected chat ID
非空就不是新会话，即使 chat row 还没同步也不能再发 checkout plan，见
`composer.rs:9244–9247,9738`。

Go 本轮只将 footer 设为只读，并禁止后续 draft selection/send/queued send
迁移已有会话的 checkout；没有移除历史显式 `SessionUpdate.Branch/WorktreeID/DeviceID`
协议能力。该旧 API 仍有既有回归，不把本轮限制写成全协议禁止。

### 普通 mock 授权入口的 Cloud 闭环

`state.rs:4068–4080` 的 `cloud_repo_for` 要求 Cloud 已开启，且可达仓库
`full_name` 与项目的 GitHub 仓库名按 ASCII 大小写等价匹配；不能仅凭仓库 ID
或“有一个 Cloud 设备”推断可达。`pickers.rs:3230–3282` 只在满足该条件时
提供一条虚拟 Cloud 设备行，不列各会话独占的 Cloud 机器。Cloud-origin
项目始终运行在 Cloud 的例外仍按 `2998–3013` 保留，普通远端设备仍可选。

Go 初始项目/PR 使用 `zeronsh/fieldnotes`，旧 mock 的 GitHub 连接及选择仓库
动作却返回 `alex/fieldnotes`，使标准授权无法到达严格匹配的 Cloud 分支。
本轮仅将这两个确定性动作的样例名称统一为 `zeronsh/fieldnotes` /
`zeronsh/zeron`，保留 `fieldnotes` / `zeron` ID；不重写已有存档中的任意仓库。
前后端均校验 Cloud ready、仓库已授权及完整名称；设备菜单的展示和接受动作
使用同一条件。授权消失后同步草稿的设备与 checkout，避免出现 Cloud chip
配本地 checkout plan。每个新 Cloud host 仅复制其支持的 agent 能力，不借用
模板账户、用量、登录信息或账户错误。

`TestComposerFooterStandardGithubAuthorizationReachesCanonicalProject` 已先以旧
`alex/...` 名称复现失败，再通过标准 typed enable/connect/poll/authorize/choose
动作验证 Cloud refs、send 新机器及断开后的拒绝；
`TestComposerCloudStandardAuthorizationUIAndSend` 使用真实 Settings、device/ref
picker 和原生输入控件的 Tester 完成 Connect → Choose on GitHub → Cloud →
ref → Enter send，并验证新 host 的 cwd/source context、普通远端保留、
会话机器不出现在设备菜单，以及断开后回到项目设备。没有手塞正确仓库 DTO。
这是 typed/backend 与 headless 控件证据；未访问真实 GitHub、运行真实 Git，
也未在本次补丁中进行原生窗口或像素验收。

## 3. PR badge：关联的是会话 source context

PR 只出现在主 settled、Git、已有 session row 的 footer。数据为
`ChangeRequestSummary{provider,number,title,url,state,base_ref,head_ref}`，见
`proto/src/entities.rs:1055–1066`。不是按当前项目随便挑一个 PR，也不能仅用
`Session.PullRequestNumber` 在全局列表里猜关联。

`change_requests.rs:396–427` 要求 conversation-owned `source_context`：核对
snapshot.device_id 与 chat.device_id、snapshot.cwd 与 source.repo_root、
snapshot.branch 与 trim(source.branch)，以及非空的 source.checkout_id。
不额外核对 source.cwd、head_sha 或 observed_at；session 的 cwd 可以是 repo 子目录。
`source_context` 的真实字段见 `proto/src/entities.rs:183–215`：checkout ID、
repo root、cwd、branch、可选 head SHA、observed_at。旧 scalar branch 本身不足以
证明共享 checkout 没有被其他会话换分支。成功查询无 PR 的 `None` 是权威清空；
查询失败应保留上一成功 snapshot，见 `entities.rs:1240–1251`。

source context 不是首次 send 后永久冻结：`engine/src/doc_host.rs:5954–5975`
在每次 fresh harness dispatch（包括恢复/steer fallback）前，从本次 request.cwd
重新捕获该会话的 source context。其他会话切换共享 checkout 不会立即改写旧会话
快照；该会话下一次 fresh dispatch 才观察到变化。

Badge 从 state 得到 Open/success、Merged/code_text、Closed/danger；title 去掉
换行。Composer badge 高20、圆角6、font11，hover card 延迟350ms，见
`change_requests.rs:17–58,194–240`。点击 `open_on_device(url,chat.device_id)`，
按用户 PR destination 进入原生 PR、应用内浏览器或系统浏览器；非 github.com
不能误当成 native GitHub provider，见 `pull_request_detail.rs:113–125`。
本项目后续仍用 typed mock PR 内容，不连接真实 gh、GitHub 或外部业务服务。

## 4. 两种 ring 的数据与阈值不同

“Plan usage”是当前 harness 活跃账户的**额度窗口**，不是 Plan mode 开关、
`Account.Plan` 套餐字符串或 Cloud credits。“Context window”是当前会话 host
报告的**上下文占用快照**，不是消息累计计费 tokens。

| 项目 | 账户额度环 | 上下文环 |
| --- | --- | --- |
| 来源 | 当前目标设备 cache 中，当前 harness 第一个 active account 的 usage_windows | 当前 composer 自己的复制 chat snapshot `context_usage` |
| 出现条件 | `signs_in && reports_usage`，且 active account 存在且窗口非空；side 传 harness=None | window 已报告且 >0；包括 side；不依赖 Git 或账户环是否存在 |
| 比例 | 所有窗口各 clamp[0,1] 后取最大值 | `tokens / window`；tokens 允许未知，window 必须 >0 |
| 未知 | 没有 fraction 就整个环不画 | 已知容量但 tokens 未知：`—`/faint；真实0：`0%`；容量未知则不画环 |
| 警告/危险 | ≥80% / ≥95% | ≥75% / ≥90% |
| 超100% | 提前 clamp，因此文字最高100% | 文字保留真实比例，可120%；圆弧单独 clamp 满圈；remaining 最低0 |
| 点击 | 打开本目标、本 harness 的 accounts 卡；可切换账户 | 只读 Context window 卡，无清空/压缩动作 |

证据：`account_usage.rs:32–49,102–123,238–241,430–470`；
`settings/accounts.rs:40–41`；`context_usage.rs:9–29,91–95,110–165`；
`proto/src/agent.rs:1158–1169`。正常账户环弧色为 accent、文字 muted，警告/危险
分别用 warning/danger。上下文普通状态弧与文字均 muted。

共同几何：ring chip h24、左右padding6、gap5、radius6、font11；canvas16，
弧半径6、stroke1.8、从12点方向开始，track 为 text_faint 的0.25 opacity。
账户在前、context 在后，cluster gap4；PR 到 ring cluster 另外留4px，尾部
padding10。见 `context_usage.rs:33–95`、`account_usage.rs:465–470`、
`composer.rs:12227–12236`。不能用静态圆形图片代替随快照变化的进度。

### 上下文快照不能从累计 InputTokens 推导

`ContextUsage` 是两个 `Option<u64>`：tokens / window。原版链路为：

```text
harness ContextUsage event
→ engine/src/sessions.rs:4446–4451
→ doc/src/schema.rs:418–443 的 meta.contextUsage 原子值
→ client/src/session/mod.rs:466,564,961 的 session snapshot
→ ui/src/state.rs:2015–2017
→ account_usage.rs:430 / context_usage.rs
```

Doc 更新中，未提供 tokens 沿用旧 tokens，window 只接受正值，否则沿用旧容量；
真实 tokens=0 必须更新。该事件允许在 Done 之后到达，不应重新打开一个 turn。
UI 切换/关闭只重置当前投影并保留该会话缓存，重新进入可恢复，见
`state.rs:4231–4275`。新 provider session（`request.resume=None`）才在
`engine/src/sessions.rs:3459–3460` 明确清除 host context；切换额度账户本身不清空它。

Codex `crates/harness/src/codex/normalize.rs:294–313` 取
`tokenUsage.last.totalTokens`，fallback 为 **last** input+output。
同文件 `1480–1502` 的回归把 thread total=900000 与 last=42000 分开，必须得到
42000。Claude `harness/src/claude/normalize.rs:1069–1086` 使用本次 assistant
input + cache-read + cache-creation；`910–919` 的 compact post_tokens 可以下降。
这些字段与累计 InputTokens / OutputTokens / Cost 不是一件事。

UI 不 sum 消息、不用模型 catalog limit 自动补成“host 已报告的容量”。Context
popover helper 虽有“两值已知 / 仅tokens / 仅capacity / 都未知”四种文本分支
（`context_usage.rs:119–147`），正常 footer 点击先受 has_window 过滤，因此只会
打开“两值已知”或“capacity已知、tokens未知”两类卡。

### 账户 cache、刷新与切换

`AccountsSnapshotCache` 与 Settings → Accounts 共用，key 是 target：None=本机，
远端为明确设备 ID。主 footer 从 session.device_id 推导 target，不能用用户上次
打开 Settings 时的设备。`account_usage.rs:102–133`、
`composer.rs:12184–12196`、`settings/accounts.rs:415–423`。

无 cache 时先普通 ListAgentAccounts，让已有持久 usage 立即出现，再强制探测；
首次track、5分钟poll及打开accounts卡均可发起强制刷新，强制请求至少间隔30秒，见
`account_usage.rs:27–29,138–186,247–255`。当前实现没有 hover 网络请求。

账户卡宽400，只列该 harness，保留 engine 顺序。姓名 email → display_name →
`Unknown account`；plan_label 有值才显示；active 显示 In use。账户 row 只有
`!active && switchable` 才可切换，乐观写共享 cache，失败还原 previous 并显示
底部红字，这一回滚分支特指 RPC 返回 Err；RPC 成功但响应反序列化失败时仅
`load(false)`，不设置该错误。卡内 meter 只画前两个窗口，但 ring 的 max 使用全部窗口；tooltip
可包括各 reset。见 `account_usage.rs:191–235,301–405`、
`settings/accounts.rs:258–266`。Footer 卡没有 Add/Remove/跳 Settings 的额外入口。

要区分原版当前行为与设计改良：list RPC/反序列化失败在 `account_usage.rs:161–168`
经 `.ok()` 丢弃，没有新的错误 skeleton；无 rows 时只有 heading。`self.error`
用于 switch 失败；空窗口的 usage_error 只在非 active account 的对应分支显示。
不能把 Settings 页面所有错误/操作控件原样搬进 footer，或把这些缺席状态写成
“原版已提供 retry”。

## 5. 实施状态与历史缺口

2026-10-28 后续已接入 `composer_footer_git.go`、`composer_footer_usage.go`：
新会话 typed refs/checkout plan、已有会话只读 checkout/ref、source-key PR、
主聊账户/上下文环及侧聊自己的上下文环。24px槽与Git内容的条件分别处理。
`contracts/context_usage.go` 与 `mock/context_usage.go` 提供独立host占用快照、
unknown/0/压缩下降/超capacity、provider/target/cwd恢复隔离与持久化；不再从
Message.Usage累加。账户复用Settings同一store，整target首次/5分钟刷新、
30秒强制节流、按login group乐观切换、连续失败回滚均有行为回归。
DisplayName已补到DTO。原生fixture与冻结参考配对仍单独记录，以下表格是
**实施前发现**，不能继续当成当前代码事实。

| 已有代码 | 能复用的部分 | 不能据此声称已完成的部分 |
| --- | --- | --- |
| `internal/desktop/chat.go:202–214` | 24px footer 占位 | 所有路由都显示 editable Checkout/Branch Select；未按 Git/hero/session 切换、无 PR/rings |
| `internal/desktop/transcript_side.go:224–226` | 侧聊24px空槽，与共享 surface 分开 | 可选 side context ring 仍未实现 |
| `internal/contracts/types.go:63–78,90–113` | Project/device/path/branch/worktrees；Session 身份与 Branch/WorktreeID | 无 GitDetected、refs resource/current tag、session cwd/source context、权威 PR checkout 关联 |
| `types.go:205–211,241–247` | 每条消息 Usage 的 Input/Output/Cached/Cost 及 ContextTokens/ContextLimit；catalog ContextLimit | 两个 context int 无法区分未知和0，且不在 session snapshot；模型容量不是 host context 快照 |
| `internal/contracts/integrations.go:62–77`；`workspace.go:99–102` | ProviderAccount 的 TargetID/ProviderID/InUse/Switchable/ReportsUsage/UsageState/Error/windows；UsedPercent 转 fraction 时除100 | 不使用全局 Account.Usage/Account.Plan 代替目标 provider 数据；DisplayName 缺失；ResetsAt 零值需明确未报告约定 |
| `internal/mock/integration_actions.go:137–165,167–202` | provider.accounts.refresh / provider.account.switch；按 target/provider/account 校验，已有异步状态与登录分组 | footer 自身未连接；保留 Settings 共用状态，不新增第二个账户 store |
| `internal/contracts/workspace.go:51–65` | PR number/title/state/repo/base/head 和现有详情数据 | 缺 provider-neutral URL 与 checkout/source-context 关联；Session.PullRequestNumber 不能代替权威匹配 |
| `internal/desktop/app.go:645–660` | send 已走 typed command | `checkout != Current checkout` 时取项目最后一个已有 worktree；这不是 NewWorktree plan |
| `internal/mock/actions.go:413–420,462–480` | mock 确有会话状态和 ContextTokens 字段 | 可更新已有 session 的 branch/worktree；context 按前值+本次Input并min到limit，不覆盖压缩下降/未知/超100%语义 |

`internal/mock/agent_scenes.go:184` 的 context=input+output+2400 是固定场景数据，
可以作为明确的 mock 观测值，但不能把这个公式提升为通用上下文计量算法。
`settings_providers.go:341–405` 已有账户 meter/切换命令，可复用纯数据与配色函数，
不必把带额外操作按钮的整个 Settings row 复刻进 footer。

## 6. 原 typed 扩展设计与验证入口

以下保留初版语义设计，现已扩展既有 Snapshot/Command/Event，未引入真实
RPC/CLI/provider连接。实际字段以 `internal/contracts` 和 `contracts.md` 为准。

| 最小增量 | 字段 / 身份 | 服务与 mock 的责任 |
| --- | --- | --- |
| 项目 Git/ref resource | Project.GitDetected；`RepoRef{Name,Current,WorktreePath?}`；target+project key；LoadStatus/Error/Rows | 明确非 Git、loading、empty、error/retry、stale refresh；普通 render 不发请求风暴 |
| Draft checkout plan | typed Kind：Current/ReuseWorktree/NewWorktree/Cloud，搭配 branch/base 可选值、reuse path/ID | 仅新会话 send 消费；真正创建新的 mock Worktree，发布实际 cwd/branch/ID；不能借最后一个现有项 |
| Session source context | CheckoutID/RepoRoot/CWD/Branch，必要时 HeadSHA/ObservedAt | 每次fresh host dispatch从本会话cwd捕获；已有会话footer不允许迁移；其他会话不直接改写本会话snapshot |
| Checkout PR snapshot | target/checkout/repoRoot/branch key；可空 Summary(provider,number,title,url,state,base,head) | successful None 清空；error 保留前一成功值；换 source key 不借旧 PR |
| Session ContextUsage | 两个可空非负计数 Tokens/Window，按 session key 保存 | host-style 合并/复制；新provider会话/明确reset才清空；UI切换/关闭只重置投影，真正删除session才移除key；保留0、capacity-only、压缩后下降及超capacity值 |
| Provider account usage | 优先复用现有 ProviderAccount 与 UsageWindow | current composer 的 resolved ProviderID+TargetID 选 InUse；复用现有refresh/switch；不足时仅补 DisplayName/可选reset约定 |

Context 的最小数据形状可为：

```go
// 当前数据形状；不改变现有计费 Usage 的含义。
type ContextUsage struct {
    Tokens *uint64 `json:"tokens,omitempty"`
    Window *uint64 `json:"window,omitempty"`
}
// Snapshot 增加 map[SessionID]ContextUsage；继续利用现有
// Event.Snapshot / SessionID / Revision 发布，不必先新增另一条事件总线。
```

单次 report 中 nil 表示“这次未报告该字段”，按原版 merge 规则保留旧值；删除
session scope/显式 reset 才是清空整个快照。不要把 nil 直接赋成0，也不要在 footer
本地从 Message.Usage 再维护一个相互矛盾的累计器。若后续增量事件脱离完整
Snapshot，需用明确的 report/reset tagged payload 区分“未更新”与“已清空”。

UI projection 只消费该 composer 的 session（侧聊用 side session ID）和 resolved
provider/device。保持现有稳定 input/Handle；为 footer popup、list selection
维护普通数据与唯一 key。异步请求使用现有服务和 generation/revision 拒绝过期
结果，不把 UI Context 带到工作线程。账户切换只改变指定 target/provider 的账户
状态，不改侧聊选中的 model/access/Plan，也不迁移现有 session 的 checkout。
扩展 Snapshot 的 map/可空指针时，按现有 `internal/mock/clone.go` 的隔离惯例深拷贝，
不能让 UI 改写返回快照而污染服务内部状态。

### 状态化 mock 的最小闭环

1. 新建 Git draft：ListRefs Loading → Ready，选普通非current ref可有成功/失败；
   失败保留旧branch和菜单。选已存在worktree只复用；NewWorktree 首次send才分配
   新ID/path/branch；ack 前后 footer内容不跳空。
2. 已有session：展示只读local/worktree/cloud+ref；拒绝从footer迁移。PR使用
   source key 发布匹配、权威none、失败保留、迟到错key四种状态。
3. Context：容量未知隐藏 → 仅容量显示`—` → tokens=0显示0% → 75%/90%阈值 →
   >100%文字与满环 → compact后占用下降。累计InputTokens可以持续上升而context
   下降，两组数据分别发布；Done后context更新不新开turn。
4. Account：本机/远端、provider A/B、active/inactive、0/多于2窗口、无上报、
   loading/stale、switch成功/失败回滚、reset未知。ring取所有窗口max，卡内只画
   前两个；Settings与footer观察同一状态，30秒去抖/5分钟poll使用可控时钟。
5. 主/侧隔离：不同model/device/context并排，侧聊仅context；切换主路由或关闭
   side tab 后旧更新不能写到另一composer。Git/nonGit不影响可用ring的语义。

以上pure projection/typed mock/异步错误分支已落实为回归。同条件28组组件WGC
及独立Inspector几何证据见第8节；完整原生点击和整个footer仍须分别验收，
不能拿组件图或 compact-picker 八图代替。

## 7. 已发现的文档 / 注释与代码差异

- `docs/audit-composer.md` 将“Git-only session footer”和“条件 hero slot”概括在一起。
  Git 条件只控制 checkout/ref/PR 内容；实际主壳始终传 DockFrame，因此 hero
  非Git也保留24px槽。本文第1节按真实分支补足该例外；主任务随后已更正旧审计表。
- 原版 `composer.rs:12170–12172` 的“Non-Git sessions grow ... from zero”注释，
  必须与 `12174` 的 `dock_frame.is_some()` 分支一起读，不能当无条件布局规则。
- 原版 `pickers.rs:4745–4747` 注释称 existing-session pick 会切换checkout，
  但 `pick_ref:2338–2343` 明确禁止已有session，`render_footer:4426–4488`也只读。
  实施遵循现有代码中的 guard，而不是这条旧注释。
- 原版 `pickers.rs:137` 对 NewWorktree 无 base 的回退注释与
  `composer.rs:9758–9774` 的 `HEAD` 新建 consumer 不一致，按后者实施。
- 本文初稿将 draft 顺序写成 ref→checkout，并将 source context 描述成首次
  send 绑定；`pickers.rs:4491–4561` 实为 checkout→ref，doc_host 则在每次
  fresh dispatch 重新捕获。上文已更正，不将旧审计概括当成行为依据。
- 原版 `account_usage.rs:25–26` 注释称 hover 会探测usage；实际trigger
  `276–297` 只有点击开/关，网络刷新在首次track、打开Accounts和poll。不可为
  “遵循注释”额外引入每次hover请求。

本文没有把上述源码/文档差异当成已获准的vendor修复，也没有据此更改生产行为。

- 早期PR概括额外要求cwd/head SHA/observedAt；冻结生产匹配实际上只比较
  device、非空checkout ID、repo root、trim后的branch，现按这些字段实施。
- 本文第5、6节初稿及 `contracts.md` 的字段清单属于实施前状态，曾遗漏新的
  typed context、checkout/ref/PR、账户DisplayName和ChoiceLabels；现已更新，
  旧缺陷表显式标为历史，不把“未实现”当当前结论。
- 原mock新授权仓库使用alex/fieldnotes，而Project/PR使用zeronsh/fieldnotes；
  它们不是同一canonical repo，导致正常Cloud入口不可用。已统一新授权fixture
  的repo身份并保留原有ID；正常设置→连接→选择→发送路径已有控件/后端回归。

## 8. 当前实施与证据

阶段索引：`reference/validation/footer-result.json`。实现分别在
`composer_footer_git.go`、`composer_footer_usage.go` 和 typed/mock 同名文件；
不是另一套账户缓存或真实Git/账号服务。Git draft有 checkout→ref，已有session
只读；侧聊仅自己的context。账户首次track/打开/5分钟poll，以及同组乐观切换
失败回滚均复用Settings的Snapshot状态。上下文报告独立于累计计费，支持未知、
仅容量、零、超容量和compaction下降，随provider/target/cwd的host恢复作用域。

最新原版28图为 `output/footer-reference-capture-20261028-r3-2x`，实现为
`output/footer-implementation-r3`。原版用公开事件打开production卡；实现fixture
用DTO控制展开，明确不当OS输入验收。实际2x/DPI192、logical840×560、完整WGC
1684×1180；旧1x采样独立保留。严格完整原图比较
`output/footer-comparison-r3-2x-reviewed/comparison.json` 为
0.998987004–0.999950231，28/28超过0.995，但没有宣布视觉验收场景完成。
空白区域会抬高整图SSIM，不能忽略局部残差。原版实际card121px、行49/32px、
context卡和intrinsic label宽度另由Inspector核对，见 `audit-footer-text-bounds.md`。

比较工具检查原始PNG/hash、实际DPI/client/frame/内部偏移、typed DTO、完整账户
group/window/reset字段、生产fraction和公开popup来源。独立33项测试包含31个
负例，全部在创建输出/执行parity前拒绝，以及真实1x/2x正例控制；不能将截停
preflight测试说成33次完整parity。原图无裁剪/缩放/配准/遮罩。

Windows检查包原生观察已覆盖主context卡、账户卡与主侧独立读数。用户输入检测
后截图与目标应用不一致，随即停止操作；侧聊context卡点击、新worktree/ref
发送、PR跳转与完整动效尚未实机验收。仅清理了路径确认的本次测试进程，
不计为graceful exit0。Mac双架构、Windows多DPI/IME/剪贴板/浏览器与性能仍待。
