# GCode × desktop-agent-ui 规范审计报告（2026-11-18）

> 本轮只做审计，不改代码。所有发现均基于当前工作树静态证据；报告区分「已独立复核」
> 与「子审计结论待实机确认」，第 6 节列明证据边界。

- 审计对象：GCode 原生 Go/MyGo 桌面应用（`internal/desktop` 119 个非测试文件约 55.9k 行，
  `internal/design` 令牌 1704 行），当前 HEAD `5080c5e`。
- **基线口径**：审计基于**含未提交改动的工作树**（`internal/workbench/tabs.go`、
  `agent_island.go`、`conversation_metrics.go`、`transcript_side.go` 等 12 个文件有进行中的修改，
  与本次审计无关，未被改动）。因此本文行号对应当前工作树；这些改动提交后，涉及文件的
  行号会小幅偏移，复现时以文件名 + 代码片段为准。
- 规范来源：[`Ludi-Herry/desktop-agent-ui`](https://github.com/Ludi-Herry/desktop-agent-ui)
  （`SKILL.md` 铁律 + `references/{open-claude-manual,desktop-pitfalls,agent-interaction,components-motion,acceptance}.md`），
  克隆于 `/tmp/desktop-agent-ui`。
- 审计方式：7 个只读子审计并行（文案 / 排版 / 几何 / 颜色 / 交互 / 状态 / 动效无障碍），
  每份结论的关键 `file:line` 由主审逐条打开源码复核，未采信未验证断言；复核中修正了
  子审计两处口径（阴影调用点数量、`TextXS` 引用数）。
- 发现合计：**52 条编号条目**（P1 23 · P2 27 · P3 2），另有若干低优先补充项在各小节末尾
  以文字列出（未编号，均为 P3 级，未计入统计）。同时确认 11 项硬门禁**已通过**（第 5 节）。

---

## 1. 执行摘要

**总体判定：基础工程质量好，规范符合度不足，且存在两处「界面说的和系统做的不是一回事」。**

正面侧证据明确：卡片一律纯平、阴影只出现在浮层；窗口无边框拖拽区由引擎按命中链最内可交互
元素判定，按钮上拖动不会误拖窗口；macOS 交通灯避让、最小尺寸 900×600 的逐级降级、
多尺寸图标、悬停浮层滚动条（不占布局槽位）均已落地；减弱动效是**应用内偏好 + 系统设置
双层守卫**，设计正确；工具进程中断链从 cancel 到 Unix 进程组 SIGKILL / Windows
`taskkill /T /F` 全链贯通；断线重连走权威快照而非重放副作用。

问题集中在四个主题：

1. **状态与控制权不真实（最影响信任）**：运行期标题「Coding / Pondering / Scheming」是
   `sessionID + 秒数` 哈希出来的装饰词，与 Agent 真实当前步骤无关；「停止」只有单一按钮态，
   没有「请求已发出 / 停止中 / 已停止」的分段，中断后 transcript 不留断点；审批卡是
   fixture 专属死 UI——生产引擎从不产生 `permission` 块，卡片按钮发出的 `request.answer`
   带着 permission ID 落到只登记 question ID 的通道上，必然报错。
2. **两个「静默失败」断点**：Enter 发送绕过 `canSend`，六种不可发送状态下按 Enter 什么
   都不发生、也不提示；加载态在会话详情未命中时立刻画 Spinner，没有延迟揭示，快切会话会闪。
3. **硬门禁级度量违规**：`TextXS=10` 并在 13 处直接用于用户可见文字，`TextSize` 线性平移
   无下限（字号调到 12 时该档变 8px）；代码/终端字号下限 8px；中点「·」82 行遍布 38 个文件；
   `.Balance()` 全仓 0 次；浮层面板 `Clip()` 与阴影同层导致阴影被裁，并另有一段自绘模糊
   伪阴影绕过引擎。
4. **强调色与令牌纪律倒置**：全界面唯一的主操作（发送键）用 `palette.Text/Background`，
   而强调色洒在状态点、Spinner、文本高亮等约 28 处次级元素上；灵动岛自建 6 个硬编码深色
   hex 且无浅色分支；`ApplyAccent` 只重算 Selection/Caret/Glyph，`themes.json` 的
   `active`/`diffHunk` 烘焙成紫色，换强调色后按下态仍紫。

**最该先做的 5 件事**

1. 修掉审批卡死链：要么引擎真正产出 permission 事件并接通回答通道，要么删掉该卡片，
   不要保留「点了必然报错」的界面（STATE-1）。
2. 运行期标题接回真实事件，装饰词降级或删除（STATE-2）。
3. 补齐停止三段语义与中断断点渲染（STATE-3）。
4. Enter 路径检查 `canSend` 并给出明确失败态（INT-1）。
5. 字号硬门禁一次性收口：`TextXS` 抬到 12、`TextSize` 加下限、代码/终端下限改 12
   （TYPE-1/2/3），同步改已经固化了 8px 偏移的 `text_test.go`。

---

## 2. 规范语义映射（Web/CSS → 原生 MyGo）

规范原文以 CSS 与 DOM 为前提，逐条映射后才有判定意义。**不适用项不计入缺陷数。**

| 规范项 | 原生映射 | 本轮判定 |
| --- | --- | --- |
| 40px 贴底跟随阈值 | `ui.ListState.FollowEnd` / `AtEnd`（`third_party/mygo/ui/list.go`） | 可映射，实现偏差见 INT-5 |
| `prefers-reduced-motion` | `platform.Preferences.ReduceMotion`（darwin/windows/linux 后端各一处） | 可映射，双层守卫已实现 |
| `data-reduce-motion` / 应用内开关 | `Appearance.MotionPreference` + `ui.MotionPreferences` | 可映射，两处调用点绕过，见 A11Y-3/4 |
| `data-hold`（延迟揭示防闪） | 需应用自建首次请求计时 | 缺失，见 A11Y-2 |
| `tabular-nums` | `FontFeatures("tnum")` | 可映射，覆盖不足见 TYPE-5 |
| `text-wrap: balance` | `ui.Text(...).Balance()`（`api_gen.go:3961`） | 可映射，0 次使用 TYPE-7 |
| `overflow:hidden` 削顶 | 固定 `Height()` + `Clip()` | 可映射，见 GEO-2/3 |
| 18%/82% 吸附 + Escape 复位 | 像素极限折叠 + 全局 Esc | 部分可映射，见 INT-6/7 |
| 500ms 组字时间戳守卫 | 原生层按键扣留 + 应用层布尔组字态 | 部分可映射，见 INT-2/3 |
| 0.5px hairline | 原生自绘，1 DIP 为最小可绘单位 | **不适用**（改判为「边框线宽档位收敛」，GEO-5） |
| CSS 变量 / 逻辑属性 / `scrollbar-gutter` | 引擎自绘滚动条、无布局占位 | **不适用** |
| `clip-audit.js` CSS 阴影裁切脚本 | 脚本针对 CSS `box-shadow`，本仓为引擎自绘 | **不适用**，改为人工读 `Shadow`+`Clip` 同层链（GEO-1） |

---

## 3. 逐维度发现

严重度定义：**P1** = 违反硬门禁或造成用户误判/静默失败；**P2** = 明显偏离规范、可感知质量损失；
**P3** = 一致性债务或低风险。`✔` = 主审已打开源码复核；`◐` = 子审计结论，需实机确认。

### 3.1 文案与本地化（COPY）

| ID | 严重度 | 位置 | 问题 | 复核 |
| --- | --- | --- | --- | --- |
| COPY-1 | P1 | 全仓 82 行 / 38 文件非测试命中 | 中点「·」用作分隔符，规范铁律 1 要求一律移除 | ✔ |
| COPY-2 | P1 | `i18n/zh-CN.json:59,471,498` ↔ `:2,3,269,863` | 同一概念译名分裂：`Providers` 一处「服务商」、其余「供应商」 | ✔ |
| COPY-3 | P1 | `i18n/zh-CN.json:301` ↔ `:843-845` ↔ `:943` | 同一概念三种写法：`token` / `Token` / `词元` | ✔ |
| COPY-4 | P1 | `settings_about.go:16-34` | 两段 slogan 副标题 + 6 张「大字标题 + 说明」营销卡（“Choose any provider”/“Three model protocols…”），违反「禁营销副标题」铁律 | ✔ |
| COPY-5 | P1 | `composer_footer_usage.go:531`、`settings_providers.go:360` | 硬编码英文 `"Unknown account"` 未走 `tr()`，中文界面露英文 | ✔ |
| COPY-6 | P2 | `composer_footer_usage.go:433-449`（`trackedHeading`）、`session_info.go:81` | eyebrow：标签转全大写并逐字插 `\u200a` 加宽；`"Agents"` 配 `LetterSpacing(.8)`（全仓唯一一处字距） | ✔ |
| COPY-7 | P2 | `i18n/zh-CN.json:157,271,556,684,847,1093,1441` 等 15 行 | 中点写死进译文，且存在 `" · resets "`、`" · Last seen "` 这类不可译片段键 | ✔ |
| COPY-8 | P2 | `agent/todo.go:280`、`contracts/activity.go:264`、`engine/tool_presentation.go:61,71`、`mcp/catalog.go:160,163` | 后端拼好带「·」的摘要直接渲染进界面（`service · operation`、`N completed · N in progress`） | ✔ |
| COPY-9 | P3 | `workbench/annotate.js:242` | HTML 内硬编码 `Shift-click to add · Esc to finish`，既未 i18n 又含中点 | ✔ |

补充：`settings_billing.go:22,30`、`settings_devices.go:45` 把整句塞进 `settingsRow` 标题位、
描述位留空；`settings.go:644`、`menubar.go:322`、`settings_about.go:44` 标题与描述语义重复。
另有 49 行 `*_test.go` 与 fixture 把中点写进断言，改文案必须同步（`conversation_metrics_test.go:132`、
`settings_usage_charts_test.go:83/84/90` 等）。

### 3.2 排版与数字（TYPE）

| ID | 严重度 | 位置 | 问题 | 复核 |
| --- | --- | --- | --- | --- |
| TYPE-1 | P1 | `design/text.go:8` + 13 处非测试调用 | `TextXS=10` 低于 12px 硬门禁；调用点：`sidebar_view.go:130`、`composer_footer_usage.go:434`、`settings_billing.go:122`、`command_palette.go:75`、`session_info.go:72,73,81`、`composer_footer_git.go:589`、`settings_controls.go:576,747,750`、`sidebar_list.go:116`、`file_search.go:178` | ✔ |
| TYPE-2 | P1 | `design/text.go:18-24` | `TextSize` 为纯线性平移 `size + base - UIFontSize`，无下限：base=12 时 `TextXS` 变 8、`TextSmall` 变 10、`TextCaption` 变 11；该偏移已被 `design/text_test.go` 固化为断言 | ✔ |
| TYPE-3 | P1 | `internal/mock/actions.go:957-958` | `CodeFontSize`/`TerminalFontSize` 下限 8（`FontSize` 下限才是 12）→ 代码与终端可渲染 8px | ✔ |
| TYPE-4 | P1 | `agent_island.go:398` | 硬编码 `FontSize(11)`，不经 `TextSize` 缩放，违反令牌单一来源 | ✔ |
| TYPE-5 | P2 | `composer_footer_usage.go:390,578`、`menubar.go:174,194`、`trajectory.go:199`、`session_info.go:72-73`、`sidebar_session.go:139` 等 | 动态数值（百分比、active/unread、token 三元组、diff 计数、相对时间）未声明 `FontFeatures("tnum")`；全包该 API 仅 16 处 | ✔ |
| TYPE-6 | P2 | 默认行高 | 未设 `LineHeight` 的多行文案落回字体度量 ≈1.2（<1.5 要求）：`trajectory.go:145`、`settings.go:333,801`、`settings_usage.go:135`；已有 1.75（markdown）/1.625（输入框）/1.5（侧栏）三档并存 | ◐ |
| TYPE-7 | P3 | 全仓 | `.Balance()` 0 次使用（API 存在），中文长标题无防孤字 | ✔ |

附：单页字号档位失控——会话页同时出现 18/16/14/12/10 五种（`transcript_theme.go:23`、
`transcript.go:697`、`sidebar_view.go:130`、`session_info.go:72`），超出「≤4 档」；静态阶梯
8 档（10/12/13/14/16/18/24/30）之外还有游离值 11/23/32。`titlebar.go:154` 会话标题
`SingleLine()` 截断且无 `Tooltip`，全名不可恢复（`workbench/browser.go:276` 有正确做法）。

### 3.3 几何、裁切与阴影（GEO）

| ID | 严重度 | 位置 | 问题 | 复核 |
| --- | --- | --- | --- | --- |
| GEO-1 | P1 | `model_picker.go:286,288` + `:473` | Popover 面板链 `Clip()` 与 `Shadow(0,8,24,-3,…)` 同层 → 阴影被裁；`:473` 另用 `Background(#000).Alpha(.16).Blur(2)` 自绘模糊底板，源码注释明确承认是「MyGo 阴影会裁掉投射盒」的绕过写法。规范要求先找真实裁切边，而非加伪阴影 | ✔ |
| GEO-2 | P1 | `composer_footer_git.go:433,444` | `Height(20)` 内放 `TextBase=14` 且字号随用户设置增长（至 20px），大字号必然削顶 | ✔ |
| GEO-3 | P2 | `transcript.go:565`（Height 22）、`composer_footer_usage.go:372`（Height 24）、`command_palette.go:421`（Height 26） | 同类固定行高：内文用 `TextSmall(12)`，随用户字号升到 20 时实际字号达 18，行高仅余 4–8px，最大档贴边/削顶。GEO-2 的 `Height(20)` 是最紧的一处 | ◐ |
| GEO-4 | P2 | 浮层阴影 8 个调用点 / 5 组参数 | `(0,8,24,-3,.12)`、`(0,10,30,-3,.1)+(0,4,12,-4,.1)`、`(0,1,20,0,glow)` 等各写一份，同一浮层语言参数不一 | ✔ |
| GEO-5 | P2 | 全局 | 圆角 10/12/16 三档混用；同心圆角不成立——`design/agent_island.go:18-19` 外圆角 24、卡片内边距 14 → 内层应 `24−14=10`，实际 `IslandCardRadius=12`；`command_palette.go:404` 面板 16、`command_palette.go:421` 行 6（按内边距应 8–10）；`design/controls.go` 的 `PopupRadius 10` + `PopupPadding 5` → `OptionRadius` 应 5 而写 6；另存 5/7/9 各一处；圆形按钮 `Size/2` 不统一（`composer_surface.go:231` 是 `Size(11,11).Radius(3)`，`model_picker.go:307` 28 方用 7） | ✔ |
| GEO-6 | P2 | `design/controls.go` | 间距非 4 倍数普遍：`ButtonHeight 28.8`、`Gap 6`、行高 34、compact 49、actions 42、inset 7.5；`Gap/Padding` 非 4 倍数占比约 25% | ✔ |
| GEO-7 | P2 | `material.go:110-112`、`model_picker.go:800-802`、`tabs.go:178-180`、`command_palette.go:86-88`、`chat.go:145-147` | 「source wash」同一逻辑 4 次硬编码（同色不同 alpha）；页面自绘卡片 surface 绕过 `settingsCard` | ◐ |

`desktop` 非测试硬编码色调用共 61 处（`ui.Hex(` / `ui.RGB(` / `ui.RGBA(`），其中
`widgets.go:22,25` 的 Claude/Muse 品牌色属合理例外，应升为 `design.BrandMark` 并注明豁免。
**正面**：全仓 `Shadow` 仅 8 个调用点且全部在浮层，卡片纯平；未发现彩色竖条卡片。

### 3.4 颜色与强调色纪律（COLOR）

| ID | 严重度 | 位置 | 问题 | 复核 |
| --- | --- | --- | --- | --- |
| COLOR-1 | P1 | `composer_surface.go:228` | 全界面唯一主操作（发送键）用 `palette.Text` 作底、`palette.Background` 作字，完全不用强调色 | ✔ |
| COLOR-2 | 保留现状 | `sidebar_session.go:122,129`、`sidebar_view.go:239`、`settings_controls.go:543` | 运行/未读状态点与 Spinner 用 Accent，挤占唯一强调位。**决策（2026-11-18，用户）：不改**——运行态与未读需要一眼可见，颜色优先于「唯一强调色」纪律；主操作改用 Accent 后与状态点共存 | ✔ |
| COLOR-3 | P1 | `agent_island.go:285-292` | `islandTheme()` 取 `ui.DarkTheme()` 后硬编码 6 个值（`#101010/#202020/#282828/#242424/#e8e8e8/#858585`），无浅色分支、不随 palette/accent | ✔ |
| COLOR-4 | P1 | `design/accent.go:41-46` + `themes.json:21,40` | `ApplyAccent` 只重算 `Selection/Caret/Glyph`，`active`/`diffHunk` 烘焙成紫色 → 换 accent 后按下态与 diff 选区仍紫 | ✔ |
| COLOR-5 | P2 | `settings_providers.go:268` vs `settings.go:782` | 同为 warning 语义，一处 `Danger` 一处 `Warning` | ◐ |
| COLOR-6 | P2 | `settings_usage_charts.go:46` | 把 Accent/Success/Warning/Danger 当图表分类序列色，语义色被降级为装饰 | ◐ |
| COLOR-7 | P2 | `composer_footer_usage.go:497,502` | 分隔符用 `Muted.Alpha(.3)`，估算对比度 ≈1.5:1，几乎不可见（且本身是「·」） | ◐ |
| COLOR-8 | P2 | `sidebar_view.go:130` 等 10px + `Faint` 组合 | `#797981` 白底估算 ≈4.3:1，低于 4.5:1 AA | ◐ |

第七项需渲染取样确认，见第 6 节。强调色落点分类：合规（主操作/令牌派生）约 6 处，
疑似噪声约 28 处，比例倒置。次级文本灰阶 9~10 级（`Muted`、`Muted.Alpha(.3/.6/.7)`、
`TextMuted.Alpha(.65/.7/.75)`、`Faint`、`Text.Alpha(.9)`），scrim 6 种 alpha
（`.35/.4/.45/.55/.8` 等，散落 `overlays.go:15,36`、`transcript_table.go:286`、
`settings.go:795`、`media.go:115`）；`dangerMuted/warningMuted/successMuted/cursor`
与 `accent.wash/activity` 共 6 个主题键从未进入 `Tokens`，成为死键并逼出 `.Alpha()` 堆叠；
`material.go:100,102` 的 `ComposerBorder` 硬编码冷灰 `#bdc7d1/#435260`，与暖中性灰基调相悖。

### 3.5 交互与输入法（INT）

| ID | 严重度 | 位置 | 问题 | 复核 |
| --- | --- | --- | --- | --- |
| INT-1 | P1 | `composer_prompt.go:313-320` + `app.go:1260-1262` | Enter 相当分支直接 `p.onSend(s.raw)`，不检查 `p.canSend`；`app.send` 在 `pendingNew`/footer git busy/附件导入中/无模型/无会话且无 agents 五种情况下静默 `return false` → 无禁用态、无提示、草稿留在原处，用户以为已发送 | ✔ |
| INT-2 | P1 | 全仓仅 `richtext/editor_state.go:7` 布尔 | 无「组字结束 500ms 时间戳」守卫：原生层（`darwin/surface.go:1381-1401`、`windows/surface.go:1164-1168`）会扣留组字中的按键，但 commit 后补发的 Return 到达时 `Composing=false`，直接被当发送 → CJK 选词误发 | ◐ |
| INT-3 | P2 | `composer_question.go:191-201` | `input.Composing()` 在构建期取值，按键时用的是上一帧快照 | ◐ |
| INT-4 | P2 | `composer_prompt.go:306-309` | `stopWithEscape` 下 Esc 无条件调 `onStop()`，不判断是否正在运行 → 空闲时也 dispatch 一次 `message.stop` | ✔ |
| INT-5 | P2 | `third_party/mygo/ui/list.go:697-702` | 跟随判定为 `y >= top-0.5`（完全贴底），规范要求 ≤40px 容差；滚轮差 1px 即冻结跟随，流式输出不再贴底。属框架侧改动 | ✔ |
| INT-6 | P2 | `app.go:1086-1093,1200-1211`、`split.go:207-264` | 分栏按像素极限折叠，无 18%/82% 吸附；无拖拽态 Escape 复位与 `previousRatio` 回滚；越界即卸载 `ui.Split`，拖拽中丢失 press 归属 | ◐ |
| INT-7 | P2 | `composer_surface.go:228-242` | 发送/停止主按钮无 Hover/Active 反馈（固定底色），发送中只 `Disabled` 无 Loading；共享构造器 `settings_controls.go:36-77` 已有 Hover/Pressed 却未使用 | ✔ |
| INT-8 | P2 | `composer_surface.go:179`、`composer_question.go:163` | `FocusRing(false)` 后无自绘替代 → Tab 到输入区/问题面板无可见焦点 | ◐ |

补充（P3）：`chat.go:118-122`「回到底部」是列表下方占位的行内按钮，出现会改变视口高度、本身
可触发一次跳动；Composer 超 260px 后仍是 richtext 默认 10px 悬停滚动条，未做胶囊内局部隐藏；
`settings.go:488-492` 文案按平台显示 Ctrl+Enter，实现只认 `ui.Cmd`（Super）→ Windows 下无效；
`settings_dialogs.go:27,33-41` 弹窗只有 `MaxHeight` 无稳定高，错误文案插入会改变面板高度。

### 3.6 状态真实性与控制权（STATE）

| ID | 严重度 | 位置 | 问题 | 复核 |
| --- | --- | --- | --- | --- |
| STATE-1 | P1 | ~~`transcript_blocks.go:54-71`~~ → 已删除（见 §8） | 审批卡是「Allow once / Always allow / Deny」三按钮，但：(a) 全仓无生产代码构造 `Kind:"permission"` 块，仅 `internal/mock`；(b) 按钮发 `request.answer{ID: p.ID}`，而引擎侧 `request.answer` 只认 question 通道（`run.answers` 仅登记 question ID，`engine/run.go:485`）→ 必然返回 `question is no longer waiting for an answer`。真实审批只走 hook 的 AskUserQuestion。界面能力与后端不一致，且是点了必报错的死链 | ✔ |
| STATE-2 | P1 | `working_trailer.go:27,29-33,76` | 运行期标题由 `sessionID + 秒数/7` 哈希从 21 个装饰词里挑（Coding/Pondering/Scheming/Conjuring…），与真实当前步骤无关 | ✔ |
| STATE-3 | P1 | `chat.go:227-233,255`、`engine/run.go:780-786,813-817` | 停止只有单一按钮态：无「请求已发出 / 停止中 / 已停止」分段，`LastOutcome` 有 interrupted 但无 stopping；取消后 transcript 不追加断点块，工具被标 interrupted 而 `transcript.go:481-489` 不渲染该状态 | ✔ |
| STATE-4 | P1 | `engine/commands.go:186-203`（无 desktop 调用点） | 引擎有 `message.retry`（复用已完成工具结果、`closeInterruptedCalls` 兜底），UI 无任何入口；`transcript_test.go:371-375` 还断言界面不出现 Retry | ✔ |
| STATE-5 | P2 | `engine/diff.go:83-85,115-129` | Diff/变更只跟踪 Write/Edit，Bash 重定向与 patch 类写入无 Change 记录 → 「哪个工具改了哪个文件」不完整 | ◐ |
| STATE-6 | P2 | `sidebar_rows.go:80`、`sidebar_view.go:223-241` | 折叠项目组只聚合 Working，等待授权与出错会话在折叠组无徽章 → 后台会话卡在授权上用户看不到 | ◐ |
| STATE-7 | P2 | `agent_island.go:82-88`、`transcript_hooks.go:3-13` | 浮岛把 interrupted 归入 completed 视觉；hook 未知/空状态默认渲染 Completed（缺数据时谎报完成） | ◐ |

补充（P2/P3）：`contracts/workspace.go:27-39` 的 Change 无时间戳，生产 Run 不写
FinishReason/Summary（仅 fixture 写），审计只能回答「改了什么」不能回答「何时、为何结束」；
草稿与附件只存内存 map（`app.go:208`、`session_state.go:163-194`），重启即丢。

### 3.7 动效与无障碍（A11Y）

| ID | 严重度 | 位置 | 问题 | 复核 |
| --- | --- | --- | --- | --- |
| A11Y-1 | 不实施 | 全仓（`Announce` 0 命中） | 无任何状态实时区域：运行→完成→失败、toast、授权请求均不播报。**决策（2026-11-18，用户）：目标用户全部视力正常，屏幕阅读器播报不实施**；`Role()`/`Announce()` 对视力用户零收益，且会把耗时与重试计数读得极吵 | ✔ |
| A11Y-2 | P1 | `chat.go:90-96` + `session_subscription.go:100-108` | 会话详情未命中即立刻画 Spinner，无 `data-hold` 等价延迟揭示（应 ≥150ms），缓存/本地快切会闪 | ✔ |
| A11Y-3 | P2 | `workbench/tabs.go:182-186` | 只读 `c.Preferences().ReduceMotion`，忽略应用内 `MotionPreference` → 应用开关对标签页 hover 动画无效（`motion.go:28-43` 已有 `reduceMotion(c)` 统一入口未复用） | ✔ |
| A11Y-4 | P2 | `settings_controls.go:913-919` | 手写守卫只查 `ReduceMotion`/`system`，漏 `PauseInBackground` → 后台窗口仍动 | ✔ |
| A11Y-5 | P2 | `model_catalog.go:25-26,151-163` | 骨架屏为 5 行 × 8% 相位差的 wave（密集列表规范要求 lockstep 齐步）、周期 2400ms（规范 2.0s）、60fps 全量重建 | ◐ |
| A11Y-6 | P2 | `composer_question.go:162-165`、`settings_controls.go:438-448` | 聚焦容器 `FocusRing(false)` 且无自绘替代环，键盘焦点不可见 | ◐ |

补充（P3）：`transcript.go:196`、`working_trailer.go:104`、`model_catalog.go:163` 的扫光/呼吸
仅由 `PauseInBackground` 收闸，默认关闭时最小化仍以 30–60fps 重绘（`effortLive()` 已给正确范式）；
`app.go:985,1108` toast 3s 常驻、无淡入、无 pass-through；`working_trailer.go:131-157` 3×3 字形
用 9 个独立 Box + Opacity、15fps 全量重建。

**动效正面结论**：全仓无「无意义漂移/闪烁」装饰动画，用途基本成立——面板列宽 tween
180→240ms、分栏回弹 220ms 单次脉冲、Activity 折叠 140ms、扫光 3400ms、开关 180ms、
toast 计时；`working_trailer.go:128` 明确标注 display-only。

---

## 4. 问题聚类（按修复根因，而非按维度）

| 根因 | 涉及 ID | 一次修完的收益 |
| --- | --- | --- |
| **状态如实** | STATE-1..4, STATE-7, A11Y-2 | 「界面说的和系统做的」一致，消除死链与伪状态 |
| **字号/度量门禁** | TYPE-1..4, GEO-2/3/6 | 一次性消掉 12px 与削顶两类硬门禁违规 |
| **裁切与阴影** | GEO-1, GEO-4, COLOR-3 | 浮层阴影不再被裁、不再需要伪阴影，浮层语言统一 |
| **令牌收敛** | COLOR-1/2/4/5/6/8, GEO-7, A11Y-4 | 强调色回到主操作、灰阶 3 级、守卫走单一入口 |
| **分隔符与文案键** | COPY-1/3/7/8 | 改一次 `zh-CN.json` 消掉大半中点与译名分裂 |
| **交互断点** | INT-1/4/5/7 | 消除静默失败与假按钮 |

---

## 5. 已验证通过的硬门禁

以下 11 项均由主审读源码确认（未运行应用，属静态成立）：

1. 卡片纯平，`Shadow` 仅 8 个调用点且全在浮层（对照 GEO-1 的伪阴影例外）。
2. 减弱动效**双层守卫**（应用内 `MotionPreference` + 系统 `ReduceMotion`）实现正确（`motion.go:9-25`）。
3. 窗口无边框拖拽区按命中链最内可交互元素判定（`mygo/ui/input.go:557,607`），按钮上拖动不误拖窗口。
4. macOS 交通灯避让（`titlebar.go:67-79`、`cmd/rcode/main.go:314` `TrafficLightPosition(14,14)`）。
5. 最小尺寸 900×600 + `layout.go:43-88` 逐级自适应隐藏。
6. 多尺寸图标 16→256 + `.icns`，`assets_test.go` 守护。
7. 滚动条悬停浮层绘制、不占布局槽位（`mygo/ui/paint.go:1457-1468`）；全局隐藏仅 1 处且有自绘替代。
8. 工具进程中断链贯通：cancel → `CommandContext` → Unix 进程组 SIGKILL / Windows `taskkill /T /F`，含 `WaitDelay`。
9. 发送去重靠 `requestID`（`engine/run.go:43-53`），重试幂等有 `closeInterruptedCalls` 兜底。
10. 断线重连拉权威快照、不重放副作用（`session_subscription.go:162-190`、`remote/server.go:105-156`）。
11. 无假进度条：容量条是真实用量，计时来自 `run.StartedAt` 与真实事件。

---

## 6. 证据边界与待验收项

**本轮只做静态源码审计**：未构建、未运行应用、未跑任何测试或 e2e，无截图与像素取样。
所有行号对应当前工作树 HEAD `5080c5e`。

必须实机确认才能定级的项：

| 项 | 为何需实机 | 建议验证方式 |
| --- | --- | --- |
| GEO-1 阴影实际裁切幅度 | 静态只能看出 `Clip()` 与 `Shadow` 同层，实际裁掉多少取决于面板内边距 | 真窗截图 + 逐浮层放大比对；`clip-audit.js` 对本仓不适用 |
| GEO-2/3 削顶像素量 | 需在字号设置 12/16/20 三档下实测行高与裁剪 | 三档字号各截一次 composer footer 与 transcript |
| COLOR-7/8 对比度 | 数字由主题 hex 按 WCAG 相对亮度**估算**（≈1.5:1、≈4.3:1），未取样 | 取实际合成色再算，或截图取样 |
| INT-2 CJK 误发 | 属框架键路径推断，依赖平台 IME 补发行为 | macOS/Windows 真机拼音选词后按 Enter 复现 |
| A11Y-1/A11Y-6 屏幕阅读器与 Tab 顺序 | 静态只能确认 `Announce` 0 命中与 `FocusRing(false)` | VoiceOver/NVDA 实测；逐控件 Tab 走查 |
| A11Y-5 骨架渲染 | 只核了周期/相位参数，未看 105° 扫光真实渲染 | 真窗录屏比对 2.0s 周期与 lockstep |
| STATE-1 审批链路 | 已由代码证据闭环（无生产块构造 + `answers` 只登记 question ID），仍建议跑一次真实高风险写入确认无审批 | 真实供应商 + Bash 写入会话 |

仓内 49 行 `*_test.go` 与 fixture 把中点写死（`COPY-1` 的连带影响），改文案时必须同步断言，
否则「代码改了、测试还锁着 ·」。

---

## 7. 附录：复核用命令与口径

```sh
# 中点（非测试 / 文件数 / 测试行）
grep -rn "·" internal cmd --include=*.go | grep -v "_test.go" | wc -l      # 82
grep -rln "·" internal cmd --include=*.go | grep -v "_test.go" | wc -l     # 38
grep -rn "·" internal --include=*_test.go | wc -l                          # 49

# 10px 令牌调用点
grep -rn "TextXS" internal cmd --include=*.go | grep -v "_test.go" | grep -v "design/text.go" | wc -l   # 13

# 阴影调用点（全部在浮层）
grep -rn "\.Shadow(" internal --include=*.go | grep -v _test        # 8

# 桌面端硬编码色
grep -rn "ui\.Hex(\|ui\.RGBA\?(" internal/desktop --include=*.go | grep -v _test | wc -l  # 61

# 无障碍播报与防孤字（均为 0 命中）
grep -rn "Announce" internal cmd --include=*.go
grep -rn "\.Balance()" internal --include=*.go
```

口径说明：`TextXS` 的「13 处」按非测试引用点计；D 子审计给出的「约 39 个强调色落色点 /
噪声约 28」为人工分类计数，存在 ±2 的判定弹性（图标与状态点的归类边界），不作为硬指标。

---

## 8. 变更记录：删除审批卡（2026-11-18）

STATE-1 经用户确认后**采用删除方案**：审批卡只存在于 fixture，且按钮回答必然报错，
保留它等于保留一条点不动的假通道。真实审批已经在 hook 的 AskUserQuestion 里成立，
不需要第二种负载类型。

删除前确认的影响面（提问卡不受影响的三条独立路径）：

| 路径 | 实现 | 与 permission 的关系 |
| --- | --- | --- |
| 历史问答记录 | `transcript_blocks.go` 的 `case "question"` | 独立 case，未被改动 |
| 实时问答表单 | `composer_question.go`（选项选择、分页、Enter 提交） | 只读 `Block.Question` |
| 回答通道 | `composer_question.go:112` → `Command.QuestionAnswers` → `engine/commands.go` | 引擎本就只实现 question 通道 |
| 共享结构 | `contracts.Block` | 只删 `Permission` 字段，`Question` 保持不变 |

实际改动：

- `internal/contracts/types.go`：删除 `Block.Permission` 字段与 `Permission` 结构体。
- `internal/contracts/clone.go`：删除对应深拷贝分支。
- `internal/contracts/activity.go`：`TurnActivity` 的 kind 列表移除 `"permission"`。
- `internal/desktop/transcript_blocks.go`：删除 `case "permission"` 渲染分支。
- `internal/mock`：删除审批 fixture（`showcase` 场景、`permissions` 场景）、
  发送含 "permission" 文本时的演示分支、取消/归档/分叉里的审批结算逻辑、
  `permissionDenied` 运行终态与 `permission.denied` 事件。
- `cmd/rcode/paths.go`：场景白名单移除 `permissions`。
- 测试：`TestQuestionAndPermissionAnswers` → `TestQuestionAnswers`（只保留提问卡断言）；
  `transcript_model_test.go` 行序断言按删除后 4 行校正；
  `TestStoppedRequestsCannotResumeAndDenyDoesNotRun` → `TestStoppedRequestsCannotResume`；
  `TestContextUsagePermissionContinuationKeepsProviderSession` 改写为
  问题续跑版本（`TestContextUsageQuestionContinuationKeepsProviderSession`）；
  showcase kind 列表与场景列表移除 `permission(s)`。
- 文档：`docs/ui-fixture-contracts.md` 删除 permission 行与权限结算语句，新增一段说明
  「工具批准不是 transcript 块，走问题卡」；`docs/ui-parity.md`、`docs/acceptance-report.md`、
  `docs/acceptance-runbook.md` 同步措辞。

行为变化（有意为之）：fixture 里以 "permission" 字样触发审批的演示场景不复存在；
mock 不再模拟「拒绝命令后插一条 won’t run 消息」的结算。真实审批仍由
`engine/hook_runtime.go:125-161` 的问题卡完成；拒绝后由 hook 返回 deny 给工具调用。

---

## 9. 第四批决策记录（2026-11-18）

### 9.1 令牌表：采用 A 案

`themes.json` 结构实测：19 个主题族、30 个变体，每变体 27 个 `colors` 键 + 8 个 `accent` 键，全部 30/30 齐全。其中 6 个键从未进入 `Tokens`，另有 1 个令牌被填充但无人读取。

- **补进 `Tokens`**：`DangerMuted`、`WarningMuted`、`SuccessMuted`（JSON 键已存在，只改
  `design.go` 的 `Tokens` 结构与 `Palette()` 映射两处，30 个主题文件不动）。
  用途：替换 `Danger.Alpha(.1)`（`settings_controls.go:59`）这类语义含糊、深浅对比不可控的写法。
- **删除**：`colors.cursor`（光标实际走 `accent.caret`）、`accent.wash`（被 `Selection` 覆盖）、
  `accent.activity`（无消费点）；同步删 `design.go:68,72` 的 `Wash`/`Activity` 字段。
- **删除**：`Tokens.DiffHunk`。它被 `Palette()` 填充但 desktop 与 workbench 全无引用；
  并且 diff 存在双口径——`workbench/workbench.go:983-984` 用 `DiffAdd/DiffDelete`，
  `transcript.go:670` 用 `Success/Danger`，同一改动两处不同色。统一为 `DiffAdd/DiffDelete`。

### 9.2 COLOR-2：保留现状（不改）

运行态 Spinner 与未读点继续使用 Accent。理由：侧栏状态的「一眼可见」优先级高于「全界面单一强调色」；
主发送键改用 Accent 后，两者共存不构成歧义（主操作在输入区，状态点在列表区）。
该条从缺陷清单降为**已决策的产品取舍**，不在第四批执行。相关规范条款在本仓按此覆盖。

### 9.3 `active` / `diffHunk` 的色相重算方式

只换色相、保留主题既有 alpha：`Active`/`DiffHunk` 的 RGB 取 `ApplyAccent` 校正后的 `primary`，
A 取原主题值（`active` 亮/暗为 `1a`/`2e`，`diffHunk` 为 `12`/`14`）。这样不改 `themes.json`、
不引入新键，30 主题 × 7 强调色自动一致。`DiffHunk` 按 9.1 删除后，此规则只作用于 `Active`。

---

## 10. 批次决策记录（2026-11-18，用户确认）

| 编号 | 议题 | 决策 | 落地范围 |
| --- | --- | --- | --- |
| D1 | `TextXS` 抬到 12 后层级塌陷 | 删除该档，13 处调用点靠字重与灰阶分层 | 第二批 |
| D2 | UI 字号下限 12 时档位被压平 | 全部夹到 ≥12，字重补偿层级；不抬高用户可调下限 | 第二批 |
| D3 | 代码/终端字号 8px 是否受 12px 门禁 | **豁免**：两者是用户内容渲染，不是界面文案；设置项控件本身仍 ≥12 | 第二批（门禁加豁免） |
| D12 | 流式跟随阈值 0.5px → 40px | **不采纳**：40px 是 Web 滚动容器建议，原生列表硬搬会让用户拽不住；保持精确贴底 | 记为规范不采纳（INT-5 关闭） |
| D13 | Enter 不可发送时的反馈 | 输入框下方一行说明 + 按钮保持禁用态，不用 toast、不抖动 | 第六批 |
| D15 | 设计门禁测试 | **建**：先落字号类门禁（design 层 + 源码扫描），中点与对比度门禁随第五批补齐 | 第二批起 |
| D4 | `model_picker` 浮层阴影被裁 | 阴影移到外层 wrapper，保持浮层层次语言 | 第三批 |
| D5 | 灵动岛浅色分支 | **豁免**：常驻悬浮窗在浅壁纸上会消失；改为从 palette 派生并标注豁免 | 第三批 |
| D6 | `DiffHunk` 删除后 diff 上下文行底色 | 不需要，增删色已足够 | 第四批 |
| D7 | 图表系列色改中性阶后的区分度 | 接受，靠标签与排序补偿，保证相邻亮度交替 | 第四批 |
| D8 | `token`/`Token`/`词元` | 统一 `Token` | 第五批 |
| D9 | `Providers` 译名 | 统一「供应商」 | 第五批 |
| D10 | `settings_about.go` 营销卡 | 删卡，留一行能力说明 | 第五批 |
| D11 | 后端摘要结构化返回 | 做：结构化字段由 UI 拼装，中点的根在拼装层 | 第五批 |
| D14 | Windows Ctrl+Enter 文案与实现不一致 | 修实现（文案已承诺） | 第六批 |
| D16 | 剩余 P2 范围 | P1 全做，P2 只做用户可见项 | 按批推进 |
| D17 | hook 批准卡升级（影响范围/会话级允许） | 下轮单独做，属功能决策 | 待排期 |

---

## 11. 执行进度（2026-11-18）

### 11.1 已完成并复核

| 条目 | 结果 | 独立复核 |
| --- | --- | --- |
| STATE-1 审批卡死链 | 删除 UI、契约字段、mock fixture 与 `permissions` 场景（§8） | ✔ build/vet/测试通过 |
| COLOR-4 `active` 不随强调色 | `ApplyAccent` 末尾 `accentedHue(primary, t.Active)`：只换色相、保留主题 alpha（亮 `1a`/暗 `2e`） | ✔ 30 变体 × 7 预设矩阵测试存在并通过 |
| 9.1 令牌 A 案 | `Tokens` 增 `DangerMuted/WarningMuted/SuccessMuted`，删 `DiffHunk`；`themes.json` 30 变体删 `cursor`/`wash`/`activity`（colors 26 键、accent 6 键，逐变体一致） | ✔ 解析校验 + `go test ./internal/design` |

matrix 测试实测：210 个「变体 × 预设」组合中当前无需 `ensureContrast` 校正，
校正路径由单独用例钉住；变异验证（临时去掉 `accentedHue`）会立即失败，断言有效。


### 11.1.1 第二批字号部分（已完成并复核）

- `internal/design/text.go`：删除 `TextXS`(10px)，新增 `MinTextSize = 12`；
  `TextSize` 委托可测纯函数 `textSize(base, size) = max(size+base-UIFontSize, MinTextSize)`。
- 13 处 `design.TextXS` 调用点改为 `TextSmall` + 灰阶/字重分层；`sidebar_view.go:130`、
  `file_search.go:178` 由 `Faint` 升到 `Muted`（对比度 ≈4.3:1 → ≈6:1）；
  `session_info.go:81` 去掉 eyebrow 的 `LetterSpacing(.8)`。
- `agent_island.go:398` 硬编码 `FontSize(11)` 改为令牌。
- 固定高度削顶：`composer_footer_git.go:433/444`（Height 20）、
  `composer_footer_usage.go:372`（24）、`command_palette.go:421`（26）改为随字号计算，
  与 `settings_controls.go:643` 既有写法一致。
- D15 门禁（`internal/design/gates_test.go`，仅用 `go/parser` 扫描源码，不 import desktop）：
  字号常量 ≥12、`TextSize` 全档位 ≥12、desktop/workbench 非测试代码无 <12 字面量
  （豁免 `CodeFontSize`/`TerminalFontSize`）、全仓无 `TextXS` 引用。
  独立复核：4 条测试全部 PASS。
- 文档：`docs/typography.md` 的角色表删除 10px 行并补充 12px 下限与层级补偿说明。


### 11.1.2 第一批引擎侧：停止三段语义（已完成并复核）

- `internal/engine/commands.go`：`message.stop` 在 cancel 之前把该 run 置
  `RunStatusStopping` 并预置 `FinishReasonUserStop`，随即 `publishLocked(state.changed)`，
  订阅者能看到停止请求；`goal.pause` 分支不变。新增 `automationOwnsRunLocked` 护栏：
  自动化运行会把自己的记录状态拷贝过去，瞬时态不能污染它，因此自动化持有的 run 不进 `stopping`。
- `internal/engine/run.go`：收尾时先读预置的 `FinishReason`，再按既有分支写终态
  （`interrupted`+`userStop` / `interrupted`+`canceled` / `failed`+`error` / `completed`+`stop`）；
  建 run 改用 `RunStatusRunning` 常量。
- `internal/contracts/types.go`：追加 run status 与 finish reason 常量（未改结构体与 JSON 标签）。
- `docs/contracts.md`：新增「运行状态与结束原因」一节，含 `stopping` 是瞬时态、
  客户端不得假设一定观察到、`LastOutcome` 不新增取值。
- `internal/engine/engine_test.go` / `subagents_test.go`：`waitIdle`、`runsFinished`
  原本把「非 running」当终态，会把 `stopping` 误判为已结束，一并改为把 `stopping` 视作活跃。
- 新测试 `internal/engine/stop_semantics_test.go`：三条用例覆盖 用户停止 / 其它取消 / 正常完成，
  独立复核 `go test ./internal/engine -run TestStop` 全 PASS。

**由复核发现的遗漏（已另派修复）**：

1. **重启归一化漏 `stopping`**：`session_recovery.go:43` 与 `persistence_index.go:68` 只把
   `running` 归一为 `interrupted`。若 `stopping` 被持久化后宿主重启，它会永久停留，
   UI 会一直显示「停止中」。属必须修的真实缺陷。
2. **mock 词汇与引擎不一致**：mock 写 `userStopped`/`archived`/`toolFailed`，
   而 UI 与引擎用 `userStop`/`canceled`/`error`。桌面测试全跑在 mock 上，
   不统一则「用户停止」的 UI 判定在测试里永远不成立。


### 11.1.3 停止语义的两处收尾缺陷（已完成并复核）

复核第一批引擎侧改动时发现并修复：

1. **重启归一化漏 `stopping`**：`internal/engine/session_recovery.go` 与
   `persistence_index.go` 原本只把 `running` 归一为 `interrupted`+`engineRestarted`；
   `stopping` 落盘后宿主重启会永久停留，UI 会一直显示「停止中」且该 run 永无终态。
   两处条件已扩展为 `running || stopping`，并用 contracts 常量替换裸字符串。
   新测试 `internal/engine/recovery_stopping_test.go` 覆盖两条恢复路径；
   变异验证（把条件回退成只判 `running`）两个测试均失败，证明断言有效。
2. **mock 词汇与引擎不一致**：mock 原写 `userStopped`/`archived`/`toolFailed`，
   UI 与引擎用 `userStop`/`canceled`/`error`。已映射为 contracts 常量
   （`archived→FinishReasonCanceled`、`toolFailed→FinishReasonError`、`userStopped→FinishReasonUserStop`），
   mock 专有的 `answered`/`forked` 保留。

遗留（记录在案，未改）：`internal/mock/server.go` 的 `applicationClosed`/`sessionDeleted`
与 `agent_scenes.go` 的裸 `"stop"` 仍非引擎词汇；mock 的 `awaitingInput` 是 mock 独有的 run 状态，
引擎 `contracts.Run` 无此取值。这些都不影响 UI 读取用户停止语义。


### 11.1.4 运行期标题接回真实事件（STATE-2，部分交付后由主审收尾）

`internal/desktop/working_trailer.go` 已删除 `activityWords` 哈希取词，改为：

- `a.phaseLabel(c, sessionID)`：取该会话最新 run 的 `TurnActivity`，按最后一个块派生标签——
  工具按 `ToolCall.Category()` 分派（exec→`Running %s`、readFile→`Reading %s`、
  edit/write/patch→`Editing %d files`（去重路径计数）、search/glob→`Searching %s`、
  其他→`Reviewing changes`）；reasoning→`Thinking`；其余→`Waiting for the model`。
- `runStatus == stopping` 时标签直接为 `Stopping…`。
- `latestRun` / `sessionRunStopping` / `sessionRunInFlight` 三个 helper 供停止按钮与重试入口复用。
- 耗时仍走 `i18n.Duration`，只有真实 run 存在时才显示。

交付时该文件残留 `hash/fnv` 未用导入导致 `internal/desktop` 无法编译，已由主审修复（删除导入）。
原从事该工作的子 Agent 因超出时间预算中断，加载态延迟揭示、重试入口、停止按钮标签三项
已转交后续 Agent 收尾。


### 11.1.5 第一批 UI 收尾：加载态、重试入口、停止标签（已完成并复核）

- **A11Y-2 加载态延迟揭示**：`app.go` 新增 `sessionDetailsLoadingSince`；
  `session_subscription.go` 引入 `sessionDetailsRevealDelay = 150ms` 与
  `sessionDetailsRevealFade = 120ms`，`sessionDetailsAreLoading` 改为「已标记且已等满 150ms」，
  另加 `sessionDetailsLoadingPending`（未满 150ms）与两个共享渲染 helper
  （`sessionDetailsLoadingPlaceholder` 负责安排一次重绘、`sessionDetailsLoadingOpacity` 做 120ms 淡入，
  复用既有的 `activityRevealProgress`/`activityRowRevealEase`/`activityMotionFrame`，减弱动效时直接不透明）。
  所有清理点收敛到 `sessionDetailsLoaded(id)`，避免两个 map 泄漏。
  `chat.go` 与 `transcript_side.go` 使用同一判定：未满阈值只留占位 spacer，满阈值才出 Spinner/文案。
- **STATE-4 重试入口**：`transcript.go` 新增 `replyRetryable`（要求 assistant 消息、最新 run 的
  `ID` 等于消息 `TurnID`（无 TurnID 时退回按会话 `LastOutcome`）、且 `Status` 为
  `failed`/`interrupted`），脚注插入 `Retry response` 图标按钮，
  dispatch `message.retry`，`sessionRunInFlight` 时禁用。既有「completed 无 Retry」断言保持通过。
- **停止标签**：`chat.go` 在 `sessionRunStopping` 为真时把 `stopLabel` 换成 `Stopping…`，
  模式仍为 `composerStop`，未改 `composer_surface.go`（该文件正被并行会话修改）。

**连带的几何断言更新（第二批字号改动的必然后果）**：
`TestFooterUsagePopoversSnapOnlyAtActualWindowOverflow` 的 accounts 卡片期望高度由 128 改为 131、
底边贴齐的 `wantY` 由 378 改为 375（等价位置）。原因：`footerHeading` 字号由 `TextXS`(10) 抬到
`TextSmall`(12)，其固定行高由 17 变为 19，卡片是内容自适应高度而非间距令牌。
该改动在测试里加了注释说明来源，未回退字号。

**验证**：`go build ./...`、`go vet ./...` 通过；`go test ./internal/desktop -count=1` 全绿（222s）。


### 11.1.6 第三批浮层阴影（GEO-1/GEO-4，部分交付后由主审收尾）

子 Agent 在派发网络中断前完成：

- 新建 `internal/design/overlay.go`：`OverlayShadow(element)` 与五个常量
  （`0 / 8 / 24 / -3 / .12`），注释写明「浮层唯一阴影，页面卡片保持纯平」、
  「承载阴影的 wrapper 不得裁切，裁切留在内层内容容器」。
- `model_picker.go`：推理弹层与模型卡片都改为「外层 wrapper 承载阴影、内层 card 保留
  圆角/padding/Clip」，`:473` 的自绘 `Background(#000).Alpha(.16).Blur(2)` 伪阴影已删除
  （全文件已无 `Blur(2)`/`Alpha(.16)` 命中）。`:625` 的模型卡片同样改为 wrapper。
- 推理滑块 plate 的 4 处 `Shadow`（glow/press/drop/静态）保留：它们是刻意的内阴影层次，
  不是浮层投影，未纳入统一入口。

主审收尾：

- `composer_footer_usage.go:423` 与 `composer_footer_git.go:462` 的双层硬编码阴影
  （`(0,10,30,-3,.1)+(0,4,12,-4,.1)`）改为 `design.OverlayShadow`，
  `internal/desktop` 非测试代码里除滑块 plate 外已无裸 `Shadow` 调用。
- 验证：`go build ./internal/desktop`、`go test ./internal/desktop -run
  'TestFooterUsage|TestFooterGit|TestModelPicker|TestReasoning'` 通过。

**待真窗确认**：阴影是否仍被 popover 宿主层裁切（静态只能确认「承载阴影的元素不再裁切」），
以及统一为单层阴影后浮层的层次观感是否与原双层方案一致。


**后续修正（用户反馈）**：运行外部命令时阶段标题曾显示**完整命令行**，多行命令会把状态行撑成多行。
`toolPhaseLabel` 的 `ToolExec` 分支改为优先使用工具调用的 `description`（引擎在
`internal/engine/tool_presentation.go:84-86` 已把它写入 `ToolCall.Summary`），
回退顺序为 `description → command → 工具名`；新增 `phaseTarget()` 折叠换行/连续空白并截断到
60 个字符加省略号，`Reading`/`Searching` 同样适用；状态文本改为 `SingleLine` +
`MinWidth(0)` + `Shrink(1)`，超长时省略而不是换行挤压布局。
测试 `TestToolPhaseLabelPrefersTheRunDescription` 断言标题取 `description` 且完整命令行不外泄。


### 11.1.7 第五批文案：i18n 键层、渲染层与门禁（已完成并复核）

**键层（`internal/i18n/zh-CN.json`）**：15 条含中点词条全部重写（键与译文同改，因为 `tr()`
未命中时直接显示键）；删除 3 个不可译片段键并改为整句模板——
`" · resets "` → `Of %s, resets on %s`、`"Never expire · "` → `Never expires: %s`、
`" · Last seen "` → `Last seen %s`；同时清掉 `Of `/` a month`/`Renews `/`Ends `/`Buy for `/
`" credits"`/`Checks · %d`/`SIDE CHATS` 等死键与片段键。译名统一：「服务商」→「供应商」（3 处）、
`token`/`词元` → `Token`（5 处译文）。校验：1459 键、无重复、含中点的键与值为空列表。

**键名统一（D8 延伸）**：16 个 model-token 键由 `tokens`/`token` 改为 `Token`
（如 `Tokens used: %d` → `Token used: %d`、`Input tokens` → `Input Token`），
10 个 Go 文件与 2 个测试同步；认证语义的 `token`（`API token file`、`Remote API token`）
保留原文与「令牌」译文，因为那是凭据而非用量单位。

**渲染层**：24 个 desktop/workbench 文件里的分隔中点全部移除——
同行元素拆成 `ui.Row(...).Gap(6/8)` 的多段 `ui.Text`（`conversation_metrics`、
`settings_controls` 的 `reading` 签名改成 `[]string`、`automations` 的摘要改为
`[]string` + 逐段渲染、`settings_plugins` 新增 `pluginSummaryParts`）；
纯字符串上下文（tooltip、无障碍 `Label`、窗口标题、`strings.Join`）统一改 `", "`，
两个应用标题改 `GCode: `；markdown 表格的文本型分隔改为 `│`。

**后端摘要**：`contracts.ActivitySummary.Label()`、`agent/todo.go` 的进度文本、
`mcp/catalog.go` 的服务名、`engine/tool_presentation.go` 的 MCP/search 摘要
由 ` · ` 改为 `, ` 或 `: `；两个 fixture（`mock/agent_scenes.go`、`model_picker_fixture.go`）同步。

**D15 第二步门禁**：新增 `internal/design/dots_test.go` 的
`TestInterfaceCopyHasNoSeparatorDots`，扫描除 `_test.go`、`internal/design`、
`output`、`third_party` 之外的全部 Go 源码与 `zh-CN.json`，出现 `·` 即失败。
用探针（临时在 `sidebar_list.go` 加一行含中点的注释）验证过确实会报错并定位到行号，探针已删除。


### 11.1.8 剩余 P2（用户确认后开工，进行中）

- **COLOR-6 图表系列色**（D7）：`settings_usage_charts.go` 的 `usageChartColor` 原先返回
  `[Accent, Success, Warning, Danger, Muted, AccentStrong]`——语义色被当装饰用，且
  `Accent` 与 `AccentStrong` 同色相。改为「单强调 + 中性阶」：Accent 全强度起步，
  强调色按 `.68/.44/.26` 递减透明度穿插 `Muted`/`Faint`，相邻系列在色相与权重上都不同。
  新测试 `TestUsageChartSeriesUseOneAccentAndNeutralSteps` 断言：6 个系列互不相同、
  不含 Success/Warning/Danger、第一个系列是 Accent。
- **COLOR-5 / muted 令牌落地 / GEO-5 同心圆角 / INT-8 焦点环**：已派子 Agent 处理
  （`settings_providers.go`、`settings_controls.go`、`composer_question.go`、
  `command_palette.go`、`design/controls.go`、`design/agent_island.go`）。
  同心圆角推导：岛面板外圆角 24 − 内边距 14 = 10（现写 12）；弹层 10 − 内边距 5 = 5（现写 6）。


### 11.1.9 剩余 P2 完成（COLOR-5 / muted 令牌 / GEO-5 / INT-8）

- **COLOR-5**：`settings_providers.go` 的可恢复提示由 `Danger` 改 `Warning`（保留的
  `:392` 一处 Danger 是配额 ≥95% 的实际失败态）；同页用量条语义叠加改为
  `≥95% DangerMuted`、`≥80% WarningMuted`，不再用 `color.Alpha(...)` 无保障叠加。
- **muted 令牌落地**：`controlButton` 是自由函数且被多个页面调用，拿不到 `App.palette`，
  因此在 `design/controls.go` 新增 `SemanticTonesFor(theme)`：按主题暴露的
  (danger, warning, success) 三元组在 30 变体目录里反查 muted 三元组；
  目录外主题（灵动岛自建主题）回落基色，不臆造色阶。危险按钮 hover 改为
  `DangerMuted.Alpha(.1)`，仍是 10% 淡底。
- **GEO-5 同心圆角**：岛卡片 12 → 10（外层 24 − 内边距 14，令牌写成推导式）；
  下拉选项 6 → 5（弹层 10 − 内边距 5）；命令面板页签 6 → 8、页脚键位 chip 5 → 8、
  结果行 10 → 8（16 − 容器内边距，取最近的 4 倍数），并新增具名常量写清推导。
- **INT-8 焦点环**：`composer_question.go` 的问题面板与 `settings_controls.go` 的横向滚动条
  都在 `FocusRing(false)` 之后补了 `DrawOver` 自绘环（2px 主题 Focus 色，
  可见性走 `FocusVisible()`/当帧请求焦点），未新增色值、未改字号高度。
  面板必须用 `DrawOver`：霜面在 `Draw` 里画填充与发丝线，环要在其后才不被覆盖。

新测试：`TestSharedRadiiStayConcentric`、`TestSemanticTonesResolveTheThemesMutedRoles`、
`TestCommandPaletteRowRadiiStayConcentricWithThePanel`、`TestDangerButtonResolvesTheThemesMutedTone`、
`TestQuestionPanelDrawsItsOwnFocusRing`、`TestHorizontalScrollbarRingsItsThumb`；
两个像素级用例都做了反向验证（移走自绘环即失败）。

**因文案改动同步的断言（e2e 与桌面）**：`provider_errors_test`（`provider-1, same-name`）、
`model_retry_test`（`… 3/10, HTTP 503`）、`conversation_metrics_test`（`Metrics fixture, model`）、
`menubar_test`（窗口标题前缀 `GCode: `）、`tool_labels_test`（`files: 读取文本`）、
`usage_test`（`Local fixture, …`）、`transcript_tools_test`、`transcript_detail_test`、
`agent/tool_loop_test`、`engine/tool_presentation_test`。`go vet -tags=e2e ./internal/e2e` 通过。


### 11.1.10 第六批交互断点（INT-1 / D13 / INT-4 / D14）

- **INT-1**：新增 `chat.go` 的 `sendBlockReason()`，按 `send()` 的拒绝顺序返回原因
  （命名中 → 附件导入中 → 保存中 → 未选模型 → 无会话），`composerView` 里原来的多条件
  `canSend` 表达式替换为 `reason == "" && composerHasContent(...)`，
  **按钮禁用、Enter 判定、行内提示共用同一判定**，不会再出现「按钮亮但 Enter 无反应」。
- **D13**：`promptEditor` 新增 `blockedReason`，`onSend` 返回 false 时记录、成功时清空；
  `sendBlockedHint()` 只在「当前原因 == 上次被拒原因」时返回文案，原因消失自动收起。
  渲染在胶囊下方 footer 之上，`TextSmall + Muted`，节点非空才挂载，不占常驻高度，
  无 toast、无动画、无定时器。
- **INT-4**：Esc 分支补 `p.running`，空闲按 Esc 不再 dispatch `message.stop`。
- **D14**：抽出纯函数 `modEnterAccepted(mods, goos)`（darwin→`ui.Super`，其它→`ui.Ctrl`），
  使平台判定可测；设置页文案未改。
- 测试 `composer_send_guard_test.go` 五条：阻断原因表驱动、Enter 保持草稿并显示提示、
  修正后发送且提示消失、空闲/运行中 Esc 行为、跨平台修饰键。独立复核全 PASS。

### 11.1.11 状态真实性剩余 P2（STATE-6 / STATE-7 / A11Y-5）

- **STATE-6**：`sidebar_rows.go` 新增枚举 `sidebarActivity`（`None < Working < Errored < Awaiting`）
  与 `foldedActivity()`，折叠项目行用 `max()` 聚合所有未归档会话，
  优先级 **等待授权 > 出错 > 运行中**，同组只出一个徽章；未读计数不并入。
  `sidebar_view.go` 抽 `sidebarProjectActivity`，与会话行 `sidebarStatusGlyph` 同款色点/图标/Spinner，
  无障碍 `Label` 为「项目名, 状态」，旧徽章随状态变化消失。
- **STATE-7a**：`agent_island.go` 的 `run.Status == "interrupted"` 现在得到独立 `phase`
  与 `Detail`，排序落在 running 之后、completed 之前，紧凑行用 `TextMuted` 中性色，
  不给 Stop 而给 Dismiss，也不触发「完成」式自动展开；error 仍优先于 interrupted。
- **STATE-7b**：`transcript_hooks.go` 只在状态明确为 `completed` 时显示 `Completed`，
  缺失/未知值显示中性占位 `"—"`（`"Unknown"` 无词条，且本批禁止改 i18n）。
- **STATE-3 尾项**：`transcript.go` 的 `activityItem` 为 `interrupted` 工具行补次级标签
  `tr(c, "Interrupted")`（`Muted`、沿用该行字号），整行配色不变。
- **A11Y-5**：`model_catalog.go` 骨架改为锁步 `modelSkeletonPeriod = 2s`、30fps、
  减弱动效时固定 `.55` 不呼吸。
- 新测试：`TestAgentIslandInterruptedIsNotACompletion`、`TestTranscriptHookUnknownStatusIsNotCompleted`、
  `TestInterruptedToolRowLabelsItsState`、折叠组状态聚合优先级与生命周期用例。

### 11.1.12 引擎全量测试

`go test ./internal/engine -count=1 -timeout 40m` 通过（551.8s）。此前两轮 600s 超时
是套件本身耗时而非挂起，被点名超时的 `TestMCPTestCannotOverwriteEditedOrDeletedConfiguration`
单独运行 1.8s 通过。


### 11.1.13 排版收尾（TYPE-5 / TYPE-7 / A11Y-3）

- **TYPE-5 tabular-nums**：新增 11 处——footer 用量摘要与账户百分比、用量表百分比列、
  菜单栏 `%d active, %d unread` / `%d turns, %d tools` / 30 天 Token 读数、
  轨迹的三元组读数、应用用量指标、侧栏行尾相对时间、diff 统计的 `+n`/`-n`。
  明确判为「不加」并给出理由的有：整句说明文字、混合标签的描述行、
  固定宽度的静态标签、单行 hover 卡片文本。
- **TYPE-7 防孤字**：`design/settings.go` 新增 `SettingsPageTokens.Heading(c, text, label)`，
  宽窄断点选号、固定行高、边距与 `.Balance()` 集中在这一个入口；
  `settings_page.go` 的 `settingsPageHeading` 收敛为一行委托（该文件原不在清单内，
  不改它 Balance 无法落到共享入口——改动仅 6 行 → 1 行，无其它影响）。
  标题只有固定 `LineHeight()`、没有 `SingleLine()`，Balance 只改换行位置、
  不改行数与高度，已用测试固定该不变性。空态大标题没有共享入口（现有空态都是
  `TextBase` 小字，唯一大字号空态属 hero composer），本项跳过、不强行改。
- **A11Y-3 动效守卫**：`internal/workbench` 新增 `State.reduceMotion(c)`，
  逐条对齐 `desktop/motion.go` 的语义（ReduceMotion → 后台暂停 → MotionPreference →
  系统偏好），数据取自自身快照与窗口，**不 import `internal/desktop`**；
  `TabStrip` 每帧算一次并传入按钮构造，替换原先只读 `c.Preferences().ReduceMotion` 的判定。
- 新测试：`TestTabStripMotionGuardFollowsAppearanceSettings`（9 个子用例覆盖
  system/reduce/on/Reduced/reduce/full/off/无窗口暂停）、
  `TestSettingsPageHeadingFollowsTheFrameTokens`（窄窗/断点前/断点/宽窗 4 例）。
- 已知缺口（记录在案）：`settings_usage_charts.go` 的图表总额与按模型读数、
  `chat.go` 的 `%d matches` 仍缺 tnum；MyGo Tester 未公开读取 font-feature-settings 的接口，
  因此 tnum 目前只有代码复核与几何回归作为证据，没有脆弱断言。


### 11.1.14 收尾验证与一次环境事故

- 最终验证：`go build ./...`、`go vet ./...` 通过；
  `go test ./internal/desktop`（171s）、`./internal/design`、`./internal/workbench`、
  `./internal/contracts`、`./internal/mcp`、`./internal/agent` 全绿；
  `./internal/engine` 定向回归（停止语义、重启归一化、工具展示、问答）通过。
- **事故**：验证初期出现 `ld: write() failed, errno=28`（磁盘满，剩余 2.6 GiB），
  六个包的测试二进制全部链接失败。清理 `go clean -cache`（构建缓存 26 GiB）后恢复，
  剩余空间回到 29 GiB，重跑全部通过。工作树未受影响。
  仓库根目录另有 4 个未跟踪的构建产物（`app-icon`、`desktop-e2e`、`gcode-server`、`parity`，
  合计约 39 MB）与 6.4 GiB 的 `output/`，本次未删除。


### 11.1.15 A11Y-1 不实施（产品约束）

用户确认目标用户全部视力正常，屏幕阅读器（VoiceOver/NVDA）的状态播报不实施。
因此本轮**不做** live region 与 `c.Announce` 改造，审计条目 A11Y-1 由 P1 缺陷降为「不实施」。

仍然保留的两项键盘无障碍能力（服务视力正常但只用键盘的用户）：

- INT-8：关掉焦点框的容器自绘可见焦点环（问题面板、横向滚动条）。
- 共享控件的 `FocusVisible` 态。

若将来面向辅助技术用户，这条可按第 6 节的落点表恢复：状态行 `RoleStatus`（不含耗时与重试计数）、
发送受阻提示 `RoleAlert`、toast 走 `c.Announce`、后台完成维持系统通知不重复播报。


### 11.1.16 COLOR-8 收尾：Faint 只作装饰墨色

量化全部 30 个主题后确认：`Faint` 对背景的对比度**有 21 个低于 4.5:1**，最低
`atom-one-dark` 只有 2.32:1；`Muted` 在 30 个主题全部达标（最低 4.76:1）。
结论：`Faint` 不适合承载文字，只用于装饰（分隔线、占位底色、shimmer 起点）。

已将以下**文字**从 `Faint` 提升为 `Muted`：侧栏相对时间、消息时间戳、
todo 计数与明细、运行耗时。`composer_footer_git.go` 的 “Showing 300 of N refs”
与 `settings.go` 的「No matching fonts」两处留给几何批次同文件一起改。

同时补齐两处多行说明文案的行高：`trajectory.go` 的截断提示、`settings_usage.go`
的未知模型说明，统一 `.LineHeight(1.5)`（TYPE-6 的可控部分；全局默认行高不动）。


### 11.1.17 D17 批准卡升级（影响范围披露 + 会话级允许）

- **结构化披露**：`hook_runtime.go` 的 `hookPermissionScopeOf()` 从 `tool_input` 解析
  command / file_path / path / notebook_path / url，相对路径按 CWD 解析成绝对路径，
  写入类工具 `os.Stat` 判断是否覆盖已有文件；披露文本按工具类别给出「命令原文 / 目标路径 /
  可逆性 / 是否覆盖」，未知工具退化为 pessimistic 文案（`may not be reversible`）。
  主问题文本保持不变，回答解析键不受影响。
- **会话级允许**：新增第三个选项 `Always allow in this session`，键为
  `ToolName + 目标（绝对路径/URL → 命令首行 → 空）`；只记在引擎内存，不落盘、不跨重启。
  会话删除/归档时通过 `hookPermissionSessionUsable()` 与每次请求前的 `retain()` 失效。
- **结构性修正（主审）**：交付版本把允许记忆放在包级单例 `hookSessionApprovals`（因为
  `engine.go` 不在子 Agent 的允许清单），这会让同进程的第二个 Engine（测试、第二数据目录）
  读到他人的授权。已改为 `Engine.hookApprovals` 字段（`engine.go:135`），注入点
  `hooks.go:43` 与清理点 `:276` 同步，测试改为读 `core.hookApprovals`。
- 测试：披露 5 类、Allow 不记忆、Always allow 只静默同 scope、Deny 不记录、
  跨会话隔离与删除失效、无会话/无记忆时 fail-closed、scope 键退化、真实引擎端到端
  （一次批准后第二次写入不再提问且文件内容正确）。

**已知边界（无法可靠判定「同类请求」）**：opaque/MCP 工具退化为按工具名放行（会覆盖该工具在会话内
的所有调用）；Bash 只看命令首行且不解析重定向/子命令/变量；忽略工作目录差异；
脱敏后的 tool_input 会让「仅在被脱敏片段不同」的命令折叠为同一 scope；路径只做字符串级
`Join/Clean`，不解析符号链接；覆盖检测存在 stat 与执行之间的竞态（仅影响文案）；授权不随时间/次数衰减。


### 11.1.18 STATE-5 变更追踪补齐（首选方案：有界工作树快照）

- `ToolExec`/`ToolApplyPatch` 在 `tool.start` 采一次工作树基线、`tool.end` 再扫一次，差异即本次写入，
  复用抽出的 `lineDiff` 产出 `Hunks`；`Write`/`Edit` 行为与 ID 不变。
- **硬边界**：深度 8 层、每次快照 512 文件、单文件 1 MiB、内容预算 4 MiB；
  跳过 `.git`/`.cache`/`.venv`/`__pycache__`/`node_modules`/`output`/`dist`/`build`/`target`，
  不跟随符号链接。超限不静默：`log.Printf` 记录「本次变更追踪不完整」，内容超限时仍产出带
  `Status` 的 `Change` 但无行级 diff。
- **去重**：同一 turn 内同路径后写覆盖先写（`run.edits` 保留键记录路径 → Change.ID），
  `Write` 与 `Bash` 混写同一路径合并为一条；跨 turn 不去重。
- **归属**：`ToolCall.Diff` 只挂 `Write`/`Edit`/`ApplyPatch`；`Bash` 的变更只进 `state.Changes`，
  避免把整条命令当成一次文件编辑。另补上 `toolKind` 缺失的 `apply_patch`/`applyPatch`/`patch` 映射。
- 实测开销：本仓库根（约 1.9 万文件）每次 Bash 调用前后各扫一次约 26–32 ms，上限先生效，
  与仓库规模基本无关。
- 测试 7 条（含 `-race`）：shell 写入/新建/删除、Write/Edit 行为不变、同 turn 去重、
  扫描上限、截断基线不虚构 Change、真实 Bash 重定向、`apply_patch` 挂 Diff。

**仍无法追踪**：MCP 写工具；子代理内部工具调用；深于 8 层或在排除目录内的写入；
内容超限/非 UTF-8/读取失败的文件（只有状态没有行级 diff）；并发 Bash 调用之间快照会互相包含
（同 turn 去重后最终内容仍正确）；工具执行期间外部进程的写入会按时间窗被归到该工具；
`output/` 内写入永不追踪。

### 11.2 待办（由批次内其它文件负责，勿重复实现）

- `settings_controls.go:59` 的 `Danger.Alpha(.1)` 可在其文件 owner 收工后改用 `DangerMuted`。
- `transcript.go:670` 的 diff 着色仍用 `Success/Danger`，与 `workbench.go:983-984` 的
  `DiffAdd/DiffDelete` 不一致；该文件由第一批 Agent 负责，统一改动排在它收工后。
- 其余浮层阴影调用点（`composer_footer_usage.go:423`、`composer_footer_git.go:462`）等待共享入口落地后统一。

### 11.3 并行冲突记录

本轮同时存在第三方会话改动（非本任务）：`contracts/interrupted_turn.go`、
`engine/interrupted_turn.go`、`desktop/composer_history.go`、`workbench/file_changes.go` 等，
并修改了 `engine/commands.go`、`engine/run.go`、`contracts/command.go`、`composer_surface.go`。
本任务的引擎侧改动（`message.stop` 置 `stopping` + `FinishReason`）与上述改动落在不同区域，
当前可共存；`Run.Status` 的 `interrupted` 取值保持原样，未破坏「编辑最后一条已停止消息」所依赖的判定。

---

## 12. 发现 → 处置对照

状态取值：**已修**（本轮代码变更）｜**不实施**（产品决策）｜**不采纳**（规范条款本仓不适用）｜
**待真机**（静态无法定级，需截图或真窗）｜**待做**（已识别、未排期）。

| ID | 结论 | 状态 | 落点 |
| --- | --- | --- | --- |
| COPY-1 中点分隔 | 82 行全部移除，改布局分栏或逗号 | 已修 | e6a9126、c4b2b19 |
| COPY-2 「服务商/供应商」 | 统一「供应商」 | 已修 | c4b2b19 |
| COPY-3 token/词元/Token | 键名与译文统一 `Token`（认证 token 保留「令牌」） | 已修 | c4b2b19 |
| COPY-4 About 营销卡 | 6 张卡改三行能力陈述 | 已修 | e6a9126 |
| COPY-5 硬编码英文 | `Unknown account` 等走 `tr()` | 已修 | c4b2b19 |
| COPY-6 eyebrow | 去掉全大写与 `\u200a` 加宽 | 已修 | e6a9126 |
| COPY-7 片段键 | 3 个不可译片段键改整句模板 | 已修 | c4b2b19 |
| COPY-8 后端拼装摘要 | `·` 改 `,` / `:` | 已修 | 71a6c0a |
| COPY-9 annotate.js | 提示文案去中点 | 已修 | e6a9126 |
| TYPE-1 10px 档 | 删除，13 处改 12px + 灰阶/字重 | 已修 | f3f0506、c4b2b19 |
| TYPE-2 字号无下限 | 夹到 `MinTextSize=12` | 已修 | f3f0506 |
| TYPE-3 代码/终端 8px | **豁免**（用户内容渲染，非界面文案） | 不采纳 | — |
| TYPE-4 硬编码 11px | 改令牌 | 已修 | c4b2b19 |
| TYPE-5 tnum | 补 11 处 + 图表读数、`%d matches`；少数整句说明判为不加 | 已修 | c4b2b19、本轮 |
| TYPE-6 默认行高 ≈1.2 | 多行说明补 1.5（全局默认不动） | 已修 | c4b2b19 |
| TYPE-7 `.Balance()` | 设置页标题共享入口启用 | 已修 | f3f0506 |
| GEO-1 阴影被裁 + 伪阴影 | wrapper 承载阴影、删自绘模糊 | 已修 | f3f0506 |
| GEO-2/3 固定高度削顶 | 四处行高与活动文件徽章都改为随字号计算 | 已修 | c4b2b19、本轮 |
| GEO-4 阴影参数不一 | 统一 `OverlayShadow` | 已修 | f3f0506 |
| GEO-5 圆角不统一 | 同心推导 + 令牌化 + 圆形按钮区分 | 已修 | f3f0506、e6a9126 |
| GEO-6 间距 4 倍数 | 令牌层对齐（按钮 32、Tile 8、开关 44×28）；页面级 Gap 判为内容间距不改 | 已修 | f3f0506 |
| GEO-7 source wash 四处 | 收敛到 `design.Wash` | 已修 | e6a9126 |
| COLOR-1 发送键不用强调色 | 发送键改用 `Accent`/`AccentOn`；**用户确认强调色可同时用于主操作与状态点** | 已修 | 本轮 |
| COLOR-2 状态点用 Accent | **不改**（运行态需一眼可见） | 不实施 | §9.2 |
| COLOR-3 浮岛恒深色 | **豁免**（常驻悬浮窗需深底） | 不实施 | §10 D5 |
| COLOR-4 `active` 不随强调色 | 只换色相保留主题 alpha + 210 组矩阵测试 | 已修 | f3f0506 |
| COLOR-5 warning 语义 | 统一 `Warning`/`WarningMuted` | 已修 | c4b2b19 |
| COLOR-6 语义色当序列色 | 单强调 + 中性阶 + 断言 | 已修 | c4b2b19 |
| COLOR-7 不可见分隔符 | 分隔符本身已删 | 已修 | c4b2b19 |
| COLOR-8 10px+Faint 对比度 | Faint 不再承载文字（21/30 主题不达标，已量化） | 已修 | c4b2b19、e6a9126 |
| INT-1 Enter 绕过 canSend | 单一 `sendBlockReason` + 行内提示 | 已修 | c4b2b19 |
| INT-2 CJK 500ms 守卫 | **用户确认本机拼音下未复现误发**；不引入丢弃窗口（会误杀正常发送） | 不实施 | 本轮 |
| INT-3 组字快照过期 | 与 INT-2 同源，随其不实施 | 不实施 | — |
| INT-4 Esc 无条件停止 | 加 `running` 条件 | 已修 | c4b2b19 |
| INT-5 跟随阈值 40px | **不采纳**（原生列表会「拽不住」） | 不采纳 | §10 D12 |
| INT-6 分栏吸附与 Escape 复位 | 吸附**不采纳**（用户决定）；Escape 还原已实现（拖拽中按 Esc 恢复四栏持久化尺寸，拖拽结束即失效） | 部分已修 | 本轮 |
| INT-7 主按钮缺三态 | 发送键补 Hover/Pressed，发送中显示转圈；停止键保持中性墨色 | 已修 | 本轮 |
| INT-8 焦点环不可见 | `DrawOver` 自绘环（问题面板、滚动条） | 已修 | c4b2b19 |
| STATE-1 审批卡死链 | 删除 UI、字段与 fixture | 已修 | 71a6c0a |
| STATE-2 装饰阶段词 | 按真实事件派生，命令类优先 description | 已修 | c4b2b19 |
| STATE-3 停止无分段 | stopping → interrupted + 断点标签 | 已修 | 71a6c0a、c4b2b19 |
| STATE-4 无重试入口 | 失败/中断回复脚注提供重试 | 已修 | c4b2b19 |
| STATE-5 变更追踪不全 | 有界工作树快照（深度 8/512 文件/1 MiB） | 已修 | 71a6c0a |
| STATE-6 折叠组无徽章 | 聚合 等待授权 > 出错 > 运行中 | 已修 | c4b2b19 |
| STATE-7 中断显示为完成 | 浮岛独立中性阶段；hook 未知态显示 `—` | 已修 | c4b2b19 |
| A11Y-1 状态播报 | **不实施**（目标用户全部视力正常） | 不实施 | §11.1.15 |
| A11Y-2 加载态闪现 | 150ms 延迟 + 120ms 淡入 | 已修 | c4b2b19 |
| A11Y-3 标签页守卫绕过 | `workbench` 自有 `reduceMotion`，不反向依赖 | 已修 | c4b2b19 |
| A11Y-4 开关守卫漏后台 | 走统一入口 | 已修 | c4b2b19 |
| A11Y-5 骨架 wave | 锁步 2s、30fps | 已修 | c4b2b19 |
| A11Y-6 焦点环 | 同 INT-8 | 已修 | c4b2b19 |

**仍待做**：COLOR-1/INT-7（发送键用强调色并补 Hover/Pressed，需与「状态点保留 Accent」一起评审）、
INT-6（分栏吸附与 Escape 复位）、INT-2/3（CJK 组字守卫，需真机复现后决定）、
GEO-3 的 `transcript.go` 徽章固定高度、以及第 6 节列出的全部真机验收项。

## 13. 提交对照

| 提交 | 范围 |
| --- | --- |
| `f3f0506` | 设计令牌：字号下限、几何网格、同心圆角、浮层阴影入口、门禁测试 |
| `71a6c0a` | 引擎：停止三段语义、重启归一化、变更追踪、审批影响范围与会话级允许、mock 词汇、契约文档 |
| `c4b2b19` | 桌面与工作台：阶段词、加载态、重试、停止标签、发送守卫、键盘、状态徽章、inverted eyebrow 之外的文案/排版/对比度 |
| `e6a9126` | 收尾：关于页能力清单、标题去 eyebrow、Wash 单一入口、批注脚本提示 |

说明：工作树里同时存在并行会话的两个功能（编辑最后一条已停止消息、输入历史、文件变更），
它们与本次改动共享 `chat.go`、`app.go`、`composer_prompt.go`、`composer_surface.go` 等文件，
**无法按文件拆分**，因此随 `c4b2b19` 一并提交，并在该提交正文中注明。

## 14. 最后一轮：发送键、分栏 Escape 与三处补齐

- **COLOR-1 / INT-7（用户确认「允许强调色同时用于主操作与状态点」）**：
  `composerActionInk(a, mode)` 返回动作控件的底色与字色——发送态取 `Accent`/`AccentOn`，
  停止态保持 `Text`/`Background`，于是强调色在全界面只标「一个动作 + 运行指示」，
  停止不被误读为主操作。发送键补 `Hovered`/`Pressed`（都走 `AccentStrong`），
  发送中（`pendingSendInFlight`）在按钮内显示 12px 转圈，而不是只把按钮变灰。
  测试：`TestSendButtonCarriesTheAccentAndStopsWithoutIt`、`TestPendingSendSpinsTheActionControl`。
- **INT-6 部分实施（用户决定：不吸附、要 Escape 还原）**：
  `paneDragOrigin` 在分隔条开始移动的当帧记录四栏的持久化尺寸；
  拖拽中按 Escape → `cancelPaneDrag` 把四栏尺寸写回并安排保存；
  拖拽结束（下一帧没有任何分隔条移动）→ `endPaneDragFrame` 清掉记录，
  所以 Escape 只还原「正在进行的那次拖拽」，不会把很久以前的尺寸翻出来。
  吸附按决定不实现。测试三条：还原、无拖拽时不动作、拖拽结束后过期。
  这也顺带修掉了「拖过界会在拖拽中途卸载分隔条」造成的无法回退问题。
- **三处补齐**：活动文件徽章高度改为 `max(22, TextBase×1.5)`（内文是 `TextBase`、
  上下内边距为 0，所以不额外加 padding——子 Agent 核对了 `Padding` 的四参顺序后修正了我的初稿）；
  图表总额与按模型读数补 `tnum`；`%d matches` 补 `tnum`。
- **INT-2 / INT-3 不实施**：用户在本机拼音输入下未复现「选词补发的 Enter 触发发送」，
  故不引入 500ms 丢弃窗口——在未复现的情况下加守卫会误杀「选词后立刻按 Enter」的合法发送。

## 15. 真机验收（2026-11-19）

用 `go run ./scripts/desktop-e2e` 驱动真实原生窗口，逐项核对第 6 节的待验收项。

### 15.1 通过的项

| 验收项 | 方式 | 结果 |
| --- | --- | --- |
| 三档字号无削顶（GEO-2/3） | `TestDesktopLocalTypography` 在 12/14/20px × 深浅六组，断言设置页标题高度 = (字号+10)×4/3 | 6/6 PASS；20px 下设置页标题、说明段落、开关与选择框均无裁切 |
| 浮层阴影未被裁切（GEO-1） | `TestDesktopLocalModelPickerLayout` 截图 | 推理弹层、模型列表、供应商子菜单四周均有完整柔和投影，不再被卡片边缘切断；自绘伪阴影已不存在 |
| 强调色发送键 | 同上截图 | 发送键为强调色圆形按钮，停止键仍为中性墨色 |
| 提问卡（删审批卡的回归） | `TestDesktopLocalQuestionComposer` 深浅 | PASS，问题卡与回答通道正常 |
| 模型重试（阶段词与重连） | `TestDesktopLocalModelRetry` 深浅 | PASS |
| 工具语义标签 | `TestDesktopLocalSemanticToolLabels` 深浅 | PASS |
| 活动折叠动效 | `TestDesktopLocalTranscriptFoldMotion` 深浅 | PASS |

### 15.2 验收中发现并修掉的问题

**完成的思考行朗读的是块 ID 而不是内容**（既有缺陷，非本轮引入）：
`transcript.go` 的活动行无障碍标签对 reasoning 块固定用 `block.ID`，
屏幕阅读器读到的是 `Thought process: block-d6brl64w…` 这种无意义串，
而不是思考的第一行。既有 e2e `TestDesktopLocalTranscriptActivity` 正是为此而写，
它在基线提交 `5080c5e` 上**同样失败**（light/dark 都失败，基线已复现）。
现改为：**仍在流式输出的思考块用块 ID**（标签每 token 变化会造成朗读抖动），
**已完成的思考块用首行预览**；`transcript_test.go` 的点击目标随之更新。
修完 `TestDesktopLocalTranscriptActivity` 深浅均 PASS。

### 15.3 判定为环境抖动而非回归

`TestDesktopLocalPanelDragToClose` 在机器并发负载下失败两次（一次「Files explorer 宽度为 0」、
一次「窗口里找不到 titlebar-files」），在空闲状态下**连跑两次全部通过**；
基线提交上同一用例通过。对照结论：属资源竞争导致的启动竞态，非本轮改动引入。

### 15.4 仍未验收的项

- **对比度取样**：Faint 已不再承载文字（21/30 主题低于 AA 的问题从源头消除），
  但「实际合成色的对比度」仍需逐主题取样，当前只有主题色值的静态推算。
- **骨架屏观感**：模型目录加载骨架已改为 2.0s 锁步 30fps，但需要一次真实的加载过程截图；
  `TestDesktopLocalModelPickerLayout` 的模型列表场景未覆盖加载态。
- **CJK 组字**：用户确认本机拼音下未复现误发，500ms 守卫按决定不实施。
