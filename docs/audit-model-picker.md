# 模型选择器逐项审查

> 以下为旧版参考审查记录。2026-10-30 用户最终取消紧凑模型选择器，现已统一为
> 上方的供应商名称菜单和模型菜单，底部固定模型管理，选中模型或 Escape 直接关闭。
> 紧凑开关、首层选项页和返回逻辑已删除；此前的图标侧栏方案也已被替代。
> 收藏、收藏字段、数字直选快捷键及每行设置按钮均已移除；模型编辑统一从底部
> 「模型管理」进入。
> 思考强度独立选择，其他模型选项保留在列表底部。
> 本文旧宽度、横向供应商栏、紧凑/非紧凑模式和选中后保持打开等结论均为历史记录；
> 现行说明见 [user-settings.md](user-settings.md)。

事实源：Zeron v0.2.128 / `ab189159f10cf8e63c0502be75a59bbc487e0eaf`。
恢复机器的只读参考位于 `output/reference-sources/zeron` 和
`output/reference-sources/mygo`；下列绝对路径为原审计机器的位置。
已读取 `D:/projects/mygo/examples/effort-slider/effort.go`、gallery 的 List
示例及固定 v0.6.4 API。使用官方 PopoverBase、ButtonBase、TextInputBase、
List、SliderBase 和原生 Menu。Slider 只移植原版外观，键鼠、焦点和步长由官方控件负责。

原截图的居中大按钮/说明段落不符合原版。当前代码又独立发现了标题连接、
标准宽度错误、左缘锚定、默认输入框外观、选中项强调色、选中后错误关闭、
切供应商不切模型、快速搜索后 Enter 失效和窄窗口零宽按钮，均已修正。

## 源码依据

- `pickers/compact.rs`：宽 256、32px 行高/2px 间隔/7 行上限、48/36px 标题、28px 轨道、44×32px 拇指。
- `pickers.rs::render_harness_model_popover`：标准宽 304、40px 顶部和搜索行、按 provider 搜索。
- `popover.rs::menu_geometry` 与 `attach_overlay_end`：180px 翻转门槛、640px 上限、尾缘锚定。
- `pick_model` / `pick_harness` / `on_key_down`：标准菜单保持打开、provider 记忆、Escape 逐级返回、数字和收藏快捷键。
- `state.rs::harness_switchable`：运行和等待输入期间锁定 provider。
- `composer.rs::render`：模型组最多占输入框表面宽 45%，文字可缩；附件和发送保持可点击。

## 小项结果

“通过”仅表示列出的实现回归，不代表与 GPUI 的视觉对照通过。

| ID | 小项 | 证据 / 状态 |
| --- | --- | --- |
| M01 | Compact 256 / 标准 304 宽度 | 三尺寸实际矩形通过 |
| M02 | 尾缘对齐、上下翻转、不出窗口 | 三尺寸及面板组合实际点击通过 |
| M03 | effort/model 双行、单行列表、原品牌图标、左对齐 | 浅深色 PNG 检查通过 |
| M04 | 淡灰选中 wash、inset ring、白色 pill 拇指 | 实现 PNG 已检查，抗锯齿对照待验收 |
| M05 | Slider End 改 effort，切模型去除不支持的等级 | 控件回归通过 |
| M06 | 空格/大小写/provider 名搜索、空结果 | 实际 TextInput 回归通过 |
| M07 | 快速过滤后首次 Enter 选当前结果 | 回归通过；修复旧闭包/cursor=-1 问题 |
| M08 | 上下键、Enter、Cmd/Ctrl+数字直选 | 键盘回归通过 |
| M09 | Escape 先回 options，再关闭 | 原生快捷键路由回归通过 |
| M10 | 标准菜单选中后保持打开 | 实际点击通过 |
| M11 | 标准 provider 分页不混入别的 provider | 回归通过 |
| M12 | 各 provider 模型/effort 记忆及重建 view 恢复 | UI/持久化回归通过 |
| M13 | 收藏前置、取消、快捷键收藏、空收藏 | 回归通过 |
| M14 | 无结果时 Enter 不改变模型 | 回归通过 |
| M15 | 7,000 个长名模型虚拟化、末项搜索选择 | 原生 List 回归通过 |
| M16 | Working/awaitingInput 不换 provider | 状态回归通过 |
| M17 | 无 effort 模型不继承 High/Max | 归一化已修正 |
| M18 | 列表局部主题不改全窗玻璃背景 | 像素回归通过，修复 SetTheme 的 root fill 副作用 |
| M19 | 300px 聊天列下保留模型/附件/发送点击 | 窄窗实际点击及菜单边界通过 |
| M20 | Plan 在 access menu，启用后显示 chip suffix | 原生 Menu 操作通过 |
| M21 | 目录加载、错误/重试、无 agents | typed catalog 状态、2.4s 原版 ghost-label pulse、原生 Retry 闭环与无 agent 保留草稿回归通过；空目录按源码显示 skeleton，搜索/收藏空态另显原文 |
| M22 | 原版 scrollbar、fast mode、完整动效 | Fast 编码/按钮/快捷键已有新机实现回归；scrollbar、完整动效和配对视觉仍待验收 |
| M23 | 同状态 GPUI 截图、系统输入与监视器 DPI | R6配对8场景完整WGC原图，3项过0.995、5项未过；无视觉验收完成；系统输入及多DPI继续待验收 |
| M24 | 独立复核发现：空模型发送暗退回 Claude | 已修复；统一恢复 preferred provider 的有效模型，未解析时保留输入，retry 后恢复；使用真实持久化场景回归 |
| M25 | 独立复核发现：跨会话遗留 provider filter 与运行锁冲突 | 已修复；切会话关闭旧菜单，重新打开按当前 provider 定位；Claude→运行中 Codex 回归通过 |
| M26 | 全部 provider 禁用、新聊天和已有聊天区别 | 状态/显示已修复；新聊天显示 no agents，已有聊天仅保留仍安装的已提交 provider；按设备安装/启用过滤及缺失安装回归见 audit-composer-device.md |

回归集中在 `internal/desktop/model_picker_test.go`。本轮修正前浅深色共 6 张，
修正后正常菜单与四种目录状态共 14 张，保存于
`output/model-audit/before`、`output/model-audit/after`。
manifest 为 `reference/validation/model-picker-audit.json`。
这些是实现图，没有替换 reference golden，也没有用裁剪/缩放/遮罩获取 SSIM。

## 新机独立原版观察

原版 native capture adapter 已取得8张深/浅色、宽/窄屏的 panel/list 原生
JPEG，元数据及逐图 SHA 见
`output/resume-validation/reference-native-observed/observations.json`。
这补充了独立原版观察来源；JPEG 和未匹配数据的整窗图不能作为无损像素验收。

同提交的 `compact-picker-fixture.rs` 与生产按键代码不一致：fixture 的
`down/down/end` 假定 Model→Fast→Effort 焦点顺序，但生产
`pickers/compact.rs:771–830` 的 Up/Down 会打开邻模型列表，Enter/Space 打开
当前模型列表，F 切 fast，Tab 循环 provider，Left/Right/Home/End 直接改变
effort。新机运行在该旧断言得到 High 而非 XHigh 并退出，没有产生 PASS。
副本按冻结生产代码补齐行为，fixture 失败单独保留，不能被读作副本键盘验收。

生产语义已接入共用面板：Up/Down/Enter/Space、无需聚焦 slider 的 effort 键、
Tab/ShiftTab provider 记忆、Fast 的5类编码、Cursor option effort 都有真实控件
回归。主/侧配置归属、设备可用项、运行锁及 typed settings/session 命令另有断言。
Fast 选项仅在 compact 中转为右侧按钮；standard 保留选项行，其余 option 可继续选择。

原合同将 option choice 简化为字符串，曾无法区分 `standard` ID 与 `Standard`
标签。现保留 `Choices`/`DefaultChoice` 的业务 ID，并新增可选 `ChoiceLabels`；
当前按钮、菜单、Cursor effort 标题按标签显示，保存仍为 `standard`/`1m` 等 ID。
缺失标签回退原 ID，旧 JSON 不变；snapshot 对标签 map 作深拷贝。对应新回归位于
`model_picker_keyboard_test.go`、`contracts/types_test.go` 和 `mock/clone_test.go`。
这些测试不是 Windows 原生候选窗口或像素对照验收。

此前阶段已补8张真正的原版无损 WGC PNG，以及复用 production modelPopover 的8张
实现 native client PNG，来源分别见 `reference/validation/lossless-result.json`
和 `audit-compact-picker-fixture.md`。全窗/客户区边界不同，严格原图比较已拒绝
842×592与840×560，没有 SSIM。原图观察可见 popup 底色、effort 轨道材质、
option 行边框和 anchor 位置差异；早期表中的实现 PNG 检查不能覆盖这些差异。
当前状态见 `reference/validation/compact-fixture-comparison.json`。

接续已统一到完整 WGC 边界，保留每张原始 PNG，无裁剪/缩放/配准/遮罩。
`output/compact-comparison-20261028-r3-reviewed/comparison.json` 的8项
SSIM 为0.971695047–0.991387733，均未达0.995；几何已对齐，差异仍需修复。
源码核对确认 trigger 打开底色、overlay 从 trigger 继承500字重，以及键盘高亮
行的非收藏星标曾被遗漏。它们属于生产显示差异，不是替换参考图来消除差异。
搜索 caret 时相、非客户区合成/active 状态尚未完全核实，不据分数宣布一致。
上述 trigger、500字重、键盘 active 星标已修复并在 R4 原生图中复采。
`output/compact-comparison-20261028-r4-reviewed/comparison.json` 的8项为
0.976711056–0.993399479，仍未达0.995。搜索真实 focus/caret、重复开关后输入
及高亮行移行/收藏点击有回归；没有通过硬绘 caret 或整体位移消除残差。

最终集成 Windows 包也已实测 End→Max、Down→邻行、Enter→选中并回 panel、
Escape→关闭。UIA focused_element 仍报主 Message，而键盘实际正确作用于 panel；
无障碍焦点报告一致性继续待定位。证据在 `output/resume-validation/package-native`。

R5先对齐 Windows 默认 Typography；R6再依据原版实际 Inspector bounds 修正
option 的 intrinsic ceil 与整数位置行高。修正限 Windows1x 已测域，不扩展为
分数滚动、其他DPI或任意字形overhang保证。原版 row/label/value 的实际bounds
与实现测试相符；字号12–20的18种字体/compact配置，各测5个整数位置。
`output/compact-comparison-20261028-r6/comparison.json` 中 dark宽/窄panel及light宽
panel达到0.995，light窄panel为0.994112133；四个model list仍不足。原图完整、
未变换，未宣称视觉验收。证据与限制见
`reference/validation/text-bounds-implementation-r6.json` 和
`audit-windows-typography.md`。R3–R5数字仅代表各自历史阶段。

R7按实际source renderer改为搜索14px/22.75DIP/Faint和2DIP原生caret，列表header、
active行、半径与图标按源码修正。菜单贴窗现在只在实际越界时退8px；冷开与
compact方向规则不再依赖默认AttachTo翻转。详见 `audit-native-popup-style.md`
和 `audit-model-list-residual.md`。fixture绑定真实Window，因此本次实际2x走
oneXOptionText=false，不用1x fallback代替产品分支。原版新8图独立捕获、双方
全窗无变换，`output/compact-comparison-20261028-r7-2x/comparison.json` 中仅
dark宽panel/list达到0.995，全部范围0.986806768–0.996676105，0场视觉验收。
旧1x结果保留；DPI不同，不能直接用两阶段通过数评价改善。滑条/阴影、文字与
caret时相等残差和原生输入仍待继续，最新证据为
`reference/validation/popup-r7-result.json`（R7历史阶段）。

R8只修正局部accent overlay的border-box坐标，保留Slider真实交互。实际2x
重新配对后四个panel分数上升，四个list分数未变，总计3/8达到0.995，范围
0.988087192–0.997084323，仍为0场视觉验收。完整证据和当前`.5`框架/260源码/
新包身份见 `reference/validation/popup-r8-result.json`。R9c原版实际列表wrapper
量测进一步说明统一上移0.5DIP会误伤其他字号，详见 `audit-model-list-residual.md`。

## 思考强度选择器的动效（R9d）

迁移只带了参考实现的静态骨架：轨道、填充的三色斜坡、档位点和滑块盘面都在，
但驱动它们的运动层没有对应物。填充宽度与滑块位置直接取自档位值，换档瞬移；
填充是固定的三色渐变，没有参考实现里流过填充的光带；滑块盘面下的呼吸光晕
与按下阴影被写成固定值——`fast` 分支的 `glow=.325` 与 `.775` 恰是参考实现
`breath=0.5` 的静止帧——而该分支在唯一调用点恒为 `false`，于是连静止帧的
光晕也没有。参考实现中 `intensity` 恒为 1，光晕与流光始终在画。

本轮按参考实现 `crates/ui/src/pickers/compact.rs` 补齐三组运动，实现在
`internal/desktop/effort_motion.go`，状态由 `composerControls` 按 composer 各存一份：

- 位置过渡：换档时填充前端与滑块沿参考的 `motion::RESIZE`（200ms，
  `cubic-bezier(0,0,0.58,1)`）滑到新档位，中途再换目标从当前值重新起算；
  拖拽时填充跟随指针移动，选中档位仍按 `Step(1)` 吸附，松手后从指针位置
  缓动到该档位。
- 按压过渡：滑块按下阴影按 0→1 缓动，偏移与浓度同时变化。
- 常驻流光：填充内两道不同速度的慢波、拇指前的辉光聚集、速度条纹、满档
  粒子与周期性闪电共用一个相位；底板辉光 `0.2+0.25*breath` 与滑块光晕
  `0.55+0.45*breath` 随之呼吸。

减少动效、后台暂停、窗口失焦或固定帧（fixture）时不推进相位，过渡直接落位。

本轮证据：`internal/desktop/effort_motion_test.go` 覆盖过渡的起始落位、中途
换目标不跳变、静止帧立即落位、跳变重定根、档位与拖拽共享的嵌入几何，以及
相位只在活跃时推进；`TestReasoningSliderHandleTravelsTheRail` 在真实渲染中
扫描盘面位置，断言它沿轨道移动而不是瞬移；`TestEffortFlowMovesTheLightWithThePhase`
比较两个相位的填充像素，确认光带确实在动。深浅主题各导出多帧人工查看，
流光、条纹、粒子与闪电的出现位置逐帧不同。

待验收，未做视觉验收：本轮没有原版 WGC 逐帧对照，上述段落里既有的 SSIM
数字全部属于本轮之前的阶段。原版流光是 4 DIP 条带逐段渐变，本实现用 32 段
渐变近似；条纹与粒子的分布按源码公式重写，未与原版逐像素比对。减少动效下
保留本实现既有的三色斜坡，未采用参考实现冻结相位后的流光外观。这些是待
验收项，不作为已通过的视觉对等结论。
