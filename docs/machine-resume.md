# 新机恢复记录 — 2026-10-28

> 桌面测试已接入 MyGo Automation，原子命令引用使用富文本输入框；本记录时源码修订为 `rcode-native-ui.12`。
> 当前框架见 `mygo-patches.md`（MyGo v0.7.3 + `rcode-native-ui.20`）。
> 入口、固定来源和验证边界见 [desktop-testing.md](desktop-testing.md)；
> 下文 R16/R17 的 `.9` 及更早指纹继续保留其历史证据身份。

> 2026-10-29 后端实现更新：普通启动已使用原生 Go 核心，默认数据目录为 `~/.gcode`。当前运行说明见 [backend-v1.md](backend-v1.md)；本页其余内容保留对应 UI 阶段的历史证据。

> 2026-10-29 更新：用户确认当前 UI 基本满意，已暂停继续优化；i18n、账户菜单
> 边界和新建侧边聊天按后续请求完成。现按用户要求创建本地 UI 基线提交，下一阶段
> 实现真实后端。以下 active 与 R16/R17 内容保留其历史阶段身份，不代表继续执行
> 原视觉目标。当前范围和验证入口见 [ui-checkpoint.md](ui-checkpoint.md)。

用户明确表示“已换机，恢复目标”。本会话未带入原目标对象，因此按
`goal-execution.md` 重建同一目标，状态为 **active**，恢复本机原生 UI 自动化。
完整 UI 一致性仍未验收，不能标为 complete。

最新封存为R16：`reference/validation/tabstrip-r16-result.json`，详见
`audit-tabstrip-file-tooltip.md`。文件图片`.78`和无详情标签提示已修；R16 框架为`.6`。
当时源码已接入尚未发布验收的 [R17 `.9`](audit-tabstrip-tooltip-primitives.md)：
MyGo1,864文件、14补丁、树`27e5b9c724175bd11d537765210727fe25722a3407989e7fdcae71fa4357e8d9`。
该次采用时2931项正式输入与受审提案一致；当时AX app961保留真实内容名称，只新增
已测的checked quarter tooltip leaf。其后Cloud四文件采用另记下文，不将旧输入表/包
重标为含该改动。早期V1–V4的成本失败和原图继续保留，不能当作V5失败或通过。
V5诊断树相对C4的10项raw不增长已实测，但不是runtime0；历史C4对R16的
counter+2,048B仍未解决。Cloud修复前的`.9` pin与五门槛、canonical四目标候选包构建/校验已通过。
最新封存发布阶段仍为R16；历史C4+AX候选包、V5隔离原生图和当前源码身份分开。
下文按阶段保留历史状态，完整视觉与跨平台验收仍未完成。

P0后续进展见`audit-tabstrip-tooltip-primitives.md`：隔离V4AX采集分支完成两次Go
构建、六项ABI/记录器测试，以及baseline/off/on三阶段两主题6张整PNG字节回归。
正常Release得到12 uploads/12 Globals/346 instances/67 issued draws，原始输入已独审；
它不是正式`.9` fixture、GPU完成或精确WGC生产帧证据。当前受控账本R7为
25试验/214PNG/20成功/5失败，首轮baseline图失败保留。GPU04的288项受控执行与
独立重算保留1级RGB及非零ULP差异，真实GUI根因仍待确定。
参考侧第三次default metadata保留其原requested-feature缺口；后续一次feature+Windows
filter metadata已退出0/helper退出2，959 packages/nodes、30members，锁5c7b128d…caf34
和4323raw输入不变。completion a1e9cf48…73f1e与独审1e4fe0d2…2a3b关闭原P1，详见
`audit-tabstrip-tooltip-primitives.md`。workspace feature union不是实际编译units，
metadata与首Job smoke（8aa94b81…8178）均不含参考项目编译；SDK/首build/ABI/native
仍须另据。仅prepared plan.graphScope“all workspace dependencies”措辞过宽新增P3；
README“workspace-wide”仍成立，不称官方Cargo文档/工具bug。旧19项差异和as-of不回写。

后续Job02 smoke两例通过（055e6710…3e68b）；首次参考Rust项目build已实际执行但Cargo
退出101，blake3 custom build/LNK1181失败，driver退出1且未重试。外层execution
f76336b9…6fbf3绑定原日志；该次original/raw、当时2931输入/7docs/current保护不变，
`actualWorkInputDrift=null`不等于空数组。后续工具诊断确认blake3不识别绝对CC路径；
仅改global CC=cl.exe、保留target CC绝对绑定的第二次cached build现已完成：Cargo退出0，
原source/raw、实际work/C5 raw子集、锁和旧记录均不变，独立新结果97f00838…68d21。
实际ml64/MSVC.asm及trace EXE f678bc89…c70ba已记录；自有Job后继按界限清理，
不声称后继全部自然退出。参考baseline、ABI和baseline/off/on原生整图验收仍待执行。
Cloud导航四文件已按02采用：red4场景失败→green九顶层及全仓overlay通过，见
[Cloud审计](audit-account-settings.md)和采用0b3a17e9…b090a。现已补正常native路径：
独立offline profile、2933输入的开发EXE69a43719…f0090，off rev2→provisioning rev4→
ready/cloud-1 rev5，同页不重复、设置搜索返回rev6；PID50440正常退出0。封存a5981b8b…63b1c
保留首张错窗拒收与后续激活恢复。live四项门禁通过；lint首败7dd9…保留，仅本地file
proxy重跑退出0/0 issues（8d0df8f4…3b6b1）。失败/取消/快速路由native、race/线程trace、
`failed_action`及异常Enable→Off差别仍保留；非新canonical包或Cloud完整视觉验收。

`.9`焦点source triage04174c1d…36c3之后，新9c58单次启动正常exit0，主标记/undo和side
挂载已观察；raw PS探针policy拒绝exit1、未COM。Sky+直接PS UIA同轮也不符合Windows
Computer Use guidance:237，因此路径停止而未绕策略。native封存822ccedc…16f61见同一
audit；四phase公开focus与popup矩阵仍pending，不新立产品bug或修workaround。旧R9
raw MTA/fresh STA只证明原R8包被测sequence的工具字段错显，不是本轮`.9`结果。

## 环境与基线

- 当前 checkout：`D:/projects/GCode`，分支 `codex/isolated-worktree`。
  开始时工作区干净，HEAD 为 `488ec8d2cecfc8a3bf2cc6034d84d60a14adf238`。
- Windows 11 Enterprise LTSC，amd64，10.0.26100；PowerShell 7.6.6。
  Go 自动获取并运行仓库要求的 1.27.3，MyGo 上游保持 v0.6.4。
- 初始 `check-framework.go` 核实 `.2` 的 1,830 文件及源码 SHA
  `fd197203233f09d5cede40fc69fad6e6bac8f6dc797e1770f1637e4687ab133e`。
  布局阶段加入第六个只读输入快照补丁后为 `zeron-native-ui.3`，1,831 文件，
  SHA `c6a86278ae261c75b6b195632d97d4d64218ed69189ec45554b85c5826b4d2a6`。
  独立副本反向回 `.2`、正向重放回 `.3` 的完整树指纹均通过。R7升级到
  `.4`（1,834文件/9补丁）；R8升为`.5`（1,835文件/10补丁），R12升为`.6`
  （1,836文件/11补丁），准确树指纹与
  重放证据见下方阶段记录及 `mygo-patches.md`。
- 冻结参考保存在当前 checkout 的忽略输出中：
  `output/reference-sources/zeron`（`ab189159f10cf8e63c0502be75a59bbc487e0eaf`）和
  `output/reference-sources/mygo`（`43199d0b0fb95fd0b71e5a225d2807c04ebcc609`）。
  两个参考只读，原版构建使用另行导出的 `output/reference-native-0.2.128`。
- 新机基线与修复后完整 `go test ./... -count=1`、Go vet、MyGo vet 已通过。
  固定 golangci-lint 2.15.0 检查通过，`0 issues`。
  原始命令输出保存在 `output/resume-validation/`，与交接机的历史日志分开。

## 本次实现

主/侧 composer 根据自己的设备查找 Provider 的 `Installed && Enabled`，不再
借用 local/global 开关。已有会话可保留已安装但禁用的当前 Provider；未安装或
缺少该设备元数据时，其可选行排除，选中标签保留。设备切换与陈旧菜单激活均有
回归，详见 `audit-composer-device.md`。独立审阅发现的 Installed 例外遗漏已修复。

侧聊的最后一条 transcript 现在挂载同一 working trailer，计时读取子会话的 Run；
独立的 Pending 产生 Sending/Queued 桥接，不借用主会话的计时。无消息时仍显示
进行状态；匹配 RequestID 的确认/失败清理桥接，旧确认不能清理新请求。已有
错误和草稿保留行为继续生效。

`working_trailer_test.go` 新增对主/子计时隔离、完成移除、空列表、发送/排队、
旧确认和失败草稿的 UI 回归。`transcript_side.go` 保留原生 List、独立滚动/键、
输入 Handle 和后端命令，不保存 ui.Element/Context，不引入后台状态写入。

共用输入 surface 现在按冻结源码选择 compact49、hero expanded120–304 和
settled dock expanded104–304。原生 textarea 自行计算 Unicode/换行高度，
22.75px 行高、260px 文本高度上限、150ms resize settle 及32px迟滞均有回归。
同一个 keyed 原生输入与 Handle 保留焦点、选择、撤销和 IME 状态。

框架第六补丁仅提供 `InputDisplayText()` 的只读快照，包括未提交 preedit，
不会把组合文字写入业务草稿。独立审阅加强测试后还修复了非空 IME Enter
误发送、CJK/emoji 高度估算、小字号提前触顶，以及普通文本复用旧 input key
读出遗留草稿的问题。API 生成、完整 MyGo UI 测试、锁和许可证清单已同步。
详见 `audit-composer-layout.md`。该布局阶段之后已接入 footer/context ring，见下方
R6/footer节；完整footer与动效仍待验收。

## 原生观察及其限度

Computer Use 在新机正常启动，未复现旧机持久 Escape 停止状态。证据在
`output/resume-validation/native/`：

- 交接源码构建 `zeron-resume.exe`（SHA-256
  `4dc187e41cb87346047f6336b87f27d46ea4664c579ff32943f264fabef4b806`）
  运行 default/light/chat-0，客户端 1320×880。显式聚焦后中文草稿显示；切换到
  chat-1 不带入该草稿，返回 chat-0 恢复；本机 mock 发送、回复并清空输入。
- 实施构建 `zeron-resume-updated.exe`（SHA-256
  `9970830c20b85c478f4db35b33ab219e2d88120e544b486ea4bed585b4319220`）
  运行 transcript/dark/chat-0，客户端 1600×1000。Agent 卡打开原生侧聊；
  原生编辑控件定向设置中文草稿、点击侧聊发送后出现
  `Activity status: showcase-agent`，完成后移除；主聊记录未增加该条消息。
  `native-source-hashes.json` 记录该构建源码。此构建早于随后 Installed 例外
  的窄范围修复；最终过滤规则由修复后测试另行验证，不能偷换二进制身份。
- WGC 返回的原始 data URL 为 **image/jpeg**，已保存为 `.jpg`，未经缩放。
  这些图片用于原生行为/布局观察，不是无损 PNG goldens，也没有据此报告 SSIM。
  真实中文输入通过不等于 Windows IME 候选/组合确认完整验收。
- 原生侧聊的指针点击后 UIA 焦点报告仍指向主 Message。随后显式激活窗口、点击
  侧输入的文本起点，截图显示侧 caret；实际键盘输入 `x` 后仅侧编辑器的值变为
  `x`，主输入为空，撤销也正确作用于侧草稿。UIA 的 focused_element 仍报主输入。
  实际按键路由正确，焦点上报链路的一致性尚待定位，不标通过。
- `.3` 布局构建 `zeron-final.exe`（此阶段 SHA-256
  `11278ba5e53886196a9fe13ef568bd122f00c53d9d692da34a7e10fc8676ae46`）
  运行 default/light，1320×880。实际键盘中文多行输入展开，Ctrl+Z 恢复空白
  compact；新会话 hero 高度、18行输入触顶及输入框内向上滚动均已观察。
  证据在 `output/resume-validation/composer-native-final/`，对应161项源码清单
  `final-source-hashes.json`。它早于接续的 compact panel 键盘修正，不能代表
  后续构建身份。观察图仍为原始 JPEG，Windows IME 候选窗口未验收。
- 当前集成 Windows 检查包 `packages/windows-amd64/Zeron.exe`（SHA-256
  `38e0127e24ca8954eda64b8bc7f3b49101d6bb968e3f6dbfac21d717c58ee6c1`）
  已真实启动，default/dark/chat-0，1320×880。End 将 panel 的 High 改为 Max，
  slider 的 UIA 数值2→3；Down 打开列表并高亮邻行 Sonnet4.6，Enter 选中它并
  回到 panel，Escape 关闭。证据在 `output/resume-validation/package-native/`。
  UIA focused_element 仍报主 Message，而实际按键正确作用于 popup，保持待定位。
  本次直接运行包内 exe，未执行安装器或声称安装/卸载验收。

## 独立原版观察

固定源码只读 clone 不变；仅导出 fixture 的未使用语音依赖启用其既有
`load-dynamic` 功能，绕过 `ort-sys` 原生运行时下载停滞。新增依赖仅为
libloading0.9.0，既有版本未升级；生产 UI/renderer 不变，无语音验收。
改动可由 `scripts/reference-build.ps1 -DynamicVoiceRuntime` 重放，仍使用
Cargo `--locked`。fresh archive 上 forward/reverse 检查及规范化文本哈希
匹配通过；Windows Git 行尾转换导致原始文件字节哈希与首次手动构建导出不同，
已另记。来源记录为 `reference/validation/reference-dynamic-voice-runtime.json`。

capture adapter 已编译，原生运行取得8张深/浅色×宽/窄屏的 panel/list 图，
在 `output/resume-validation/reference-native-observed/observations.json`
逐张记录 ready 元数据、窗口、客户端尺寸、scale、二进制 SHA 和 JPEG SHA。
图像直接来自独立原版 GPUI 窗口；没有裁剪、缩放或以实现图替代。

随后原版 fixture 在键盘阶段断言 `High != XHigh` 并退出，未产生 PASS。
原因是冻结的 `compact-picker-fixture.rs` 仍假定 Model→Fast→Effort 的
`down/down/end` 焦点路线；同提交生产 `pickers/compact.rs:771–830` 已用
Up/Down 打开邻模型列表、F 切 fast、Left/Right/Home/End 调 effort。
按生产代码接续副本键盘修正，不能改 fixture 断言后冒充原版测试通过。
完整键盘原生验收与无损像素对照均保持 pending。

副本共用面板已补齐生产按键与 Fast 按钮，含 Cursor option effort、主/侧归属、
provider 设备可用性和运行锁回归。另补兼容的 `ModelOption.ChoiceLabels`：
显示 `Standard`/`1M tokens`，保存仍为 `standard`/`1m`。旧字符串 choice 数据继续
可用，snapshot 深拷贝包含新 map。详情见 `audit-model-picker.md` 和 `contracts.md`。

## 无损参考与受控实现 fixture

新增 `scripts/reference-capture-lossless.ps1`：仅导出 fixture 增加已锁定的
windows-capture1.5.0 开发依赖，在自身进程用确切 HWND 捕获 WGC 原始帧并直接
PNG 编码，不改生产 renderer/组件、不经 JPEG、不裁剪或缩放，不请求隐私例外。
196个生产 UI 源文件与固定只读 clone 的逐项哈希相同，clone 仍 clean。

成功目录 `output/reference-lossless/capture-layout-20261028-02` 有8张原版 PNG，
签名、像素尺寸、字节数和 SHA 均已核实；scale1，宽版842×592、窄版362×592，
对应 client840×560 /360×560。全窗原样包含非客户区。第一次因从 async 更新
调用只允许 render 阶段的 request_animation_frame 而失败，后改公开 refresh
与正常平台帧等待；失败日志保留，第二次成功退出0。layout-only 保留原矩阵
Enter/Escape 导航与 list 状态断言，明确跳过已知过时的完整键盘断言，不写其 PASS。
构建/运行及路径、换行回放证据见 `reference/validation/lossless-result.json`。

实现侧新增 `uifixture` tag 隔离命令，复用真实 production
`modelTrigger → modelPopover`，不复制面板绘制；使用同4模型/选项/主题/尺寸。
`output/captures/model-picker-fixture-native-20261028-r2` 同样取得8张 PNG，但
它们是原生 `Window.CapturePage()` 的 client840×560 /360×560 内容。
初次 client0×0 的启动读取没有被算成功；显式 Show、按实际已渲染状态有界等待后
重跑通过。详见 `audit-compact-picker-fixture.md`。

原样严格比较已实际拒绝842×592与840×560的边界不一致，没有生成 SSIM。
直接看两张原图还可见 dark popup 底色、effort 轨道材质、option 行边框及
popover 位置差异，尚待收敛。详见 `reference/validation/compact-fixture-comparison.json`。
下一步统一捕获边界和 anchor，再比较完整场景，不能裁剪/缩放/遮罩消除差异。

## 已完成配对的完整窗口比较

保留原版默认 frame 的两次无边框试验没有得到等价客户区边界，已恢复原窗口配置；
失败试验与原始备份各自保留在 `reference/validation/frame-boundary-trials.json`
和 `lossless-frame-boundary-baseline.json`。副本改为自身进程内的独立 WGC bridge，
只捕获调用方自己的 HWND，不枚举其他窗口。两侧原始全窗 PNG 已匹配到
842/362×592、client840/360×560、scale1、client 相对原始窗口偏移(8,31)。

原版新增实际 appearance/page/effort/Fast 元数据，没有改生产组件；新输出为
`output/reference-lossless/capture-native-state-20261028-01`。副本 R3 输出为
`output/captures/model-picker-fixture-wgc-20261028-r3`，完整 committed geometry
见 `audit-compact-picker-fixture.md`。面板位置为(8,356,256,134)，列表为
(8,264,256,226)。实际触发按钮高度32，已纠正从字形目测为24的旧估计。

`scripts/compare-compact-fixtures.ps1` 先核对8场景、磁盘 PNG/hash/尺寸、实际
typed 状态、同框边界和无变换元数据，再用已有 parity 工具比较完整原图。
独立审阅补齐目录尾斜杠保护与 client 在 frame 内的相对位置校验。R3 的
`output/compact-comparison-20261028-r3-reviewed/comparison.json` 给出
SSIM0.971695047–0.991387733，8项均未达0.995。没有裁剪/缩放/遮罩/配准。
窄图与宽图的背景权重不同，分数不能代替实际控件差异检查。
负向预检在独立拷贝上实跑：尾斜杠输入下的输出子目录被拒绝，同尺寸但 client
偏移被改动的 sidecar 也在创建输出前被拒绝；原始输入不改。证据位于
`output/compact-preflight-review-20261028-01/guard-checks.json`。新增 typed 状态
元数据后的8张原版图与前一组逐字节相同，记录为
`output/reference-lossless/typed-state-image-equivalence.json`。

生产 popup 已按冻结源码修正底色、shadow、slider 轨道/拇指、扁平 option 行及
窗口安全边缘定位。配对又定位到 trigger open 底色、overlay 继承字重与列表状态
差异，继续局部修复。原生标题栏还有色差。源码显示两框架默认的重定向/合成设置
不同，但当前捕获未记录 foreground HWND 或 DWM 属性，不能确定单一原因或宣称
active 条件相同，不擅自调整全局 palette。
R3桥所用路径普通且全新；其后独立审阅修复的断链/重解析路径拒绝属于下一工具
版本，不能把 R3 图片冒充新 DLL 的输出。

上述 trigger open/hover、overlay medium 字重与键盘高亮星标已按源码修复。
原生搜索控件的焦点、caret 与重开后的输入有控件回归；保留其真实闪烁，不为
比较图硬绘光标。最终 R4 使用新桥和重建 fixture（SHA
`9d8855f93a1a2a8b9f1d040a10279cfe3c9b5d0f32e384fcef8e06180b031563`），
输出 `output/captures/model-picker-fixture-wgc-20261028-r4` 的8图，进程退出0。
bridge SHA为`6c10000695b6e54163bd3c89e311a98461d2c5c08300c63c0b9b979e477805b1`。
磁盘签名、SHA、帧/客户区、DPI和 typed 状态均独立复核，7项直接运行源码指纹
见 `output/fixtures/model-picker-fixture-r4-build.json`。

`output/compact-comparison-20261028-r4-reviewed/comparison.json` 的8项
SSIM 提升到0.976711056–0.993399479，仍全部未过0.995。原始配对图再次检查了
dark/narrow panel 与 light/narrow list。仍可见 option 文字宽度/基线、列表字形与
小幅材质差异；非客户区未证明 active/DWM 状态一致。继续字体度量定位，不能
以修复后更接近为由宣称视觉验收完成。比较脚本也实跑了输入 junction 别名的
物理路径保护，拒绝向实际输入目录写子目录，记录在负向预检同目录。

### Windows 默认 Typography 对齐（R5）

独立无窗口 DirectWrite 测量排除了缺字体、字重选择和 variable axes：16个字体
哈希相同，72对旧/现代 family 匹配结果一致。GPUI始终 SetTypography（即使空），
MyGo原本在无feature时跳过，两者默认特性不同。以13px/500为例，Context window
字宽为101.907对98.943。1,560次布局中的312组完整 glyph 对照确认，公开
`FontFeatures("kern=0")` 可匹配已测的原版默认 Typography，也会影响默认连字。
来源、全部产物哈希、命令和限制见 `audit-windows-typography.md` 与
`reference/validation/windows-typography.json`。

`design.ApplyReferenceTypography` 现仅在 Windows 的真实 App.View 和同一fixture
root调用；不改字体文件、字号、vendor或其他平台。Windows回归核对实际独立
advance及子文本显式开启kern的覆盖。非空Features的完整映射、所有文字脚本及
Mac Typography仍不由该helper保证。仍有约1px字形位置差异；源码支持intrinsic
ceil与绝对边界取整的解释，但原版native text bounds尚未单独导出，不盲移文字。

R5新fixture SHA为`3f4673e8b563b95c0f788428482c6262dba96b0a2771cc6d8a1664f90095462d`，
8项直接运行源码在 `output/fixtures/model-picker-fixture-r5-build.json`。唯一一组
原生8图退出0，原始PNG/sidecar/状态已逐项复核。完整比较在
`output/compact-comparison-20261028-r5/comparison.json`：

| 场景 | 宽屏 SSIM | 窄屏 SSIM |
| --- | ---: | ---: |
| dark panel | 0.995681626 | 0.990053699 |
| dark model list | 0.993596317 | 0.985248421 |
| light panel | 0.995021923 | 0.988542910 |
| light model list | 0.993288494 | 0.984548485 |

只有两项宽屏panel达到0.995数值筛查，六项仍未达到；没有场景被宣布视觉验收
完成。原始全图再次检查了dark/narrow panel与light/narrow list差异，未裁剪、
缩放、配准或遮罩。源/工具/图片及完整阶段结果为
`reference/validation/popup-r5-result.json`。R3、R4和原先边界不匹配阶段均保留。

最新208项应用/构建/fixture源码冻结后，全量与tagged测试、Go vet、MyGo vet通过，
包构建后串行lint为0 issues。诊断Go源码最初被`./...`误扫，随后原字节移到
`_source`，确认go list不再包含诊断包，再完整重跑通过，失败日志没有删。
四架构新检查包位于 `output/resume-validation/popup-r5-packages`，全部资源、
原生库、架构、ZIP及checksum检查通过。新Windows包分别以default/dark1320×880、
transcript/light1600×1000实际启动、自捕获client PNG、自退出0，证据在
`output/resume-validation/popup-r5-native-boot/evidence.json`。这是启动/绘制证明，
不包含新的键鼠序列、安装器或Mac原生验收。208项源码在包和native运行后无漂移。

Footer源码审计见 `audit-composer-footer.md`：实际壳总传DockFrame，hero
非Git也保留24px空槽；已有session ref只读；侧聊只接自己的context。以下接续
已实现 typed DTO/mock、Git 与两种 usage；累计计费 tokens 不代替上下文占用。

### Option 文字与 footer（R6 / footer R3）

R6 用原版实际 Inspector bounds 验证 option 的 phi 行高、intrinsic ceil 和
整数边界取整，修正仅作用于 Windows 1x 的已测范围。8组完整 WGC 比较中3组
超过0.995，5组仍不足；没有视觉验收完成。列表残差、分数滚动、其他 DPI 和
字形 overhang 不能由该局部修正保证。证据为
`reference/validation/text-bounds-implementation-r6.json`。

主 footer 已接入 typed checkout/ref/PR、独立 context 占用、设备账户卡；侧聊
只读取自己 session 的 context。新 worktree 在首次发送才分配，已有 session
footer 只读。Cloud 可通过正常 mock 设置入口连接、选择授权仓库、发送并得到
独占设备。账户与 Settings 共用状态，支持30秒刷新去抖、5分钟 poll、乐观切换
和失败回滚；卡内前2窗口与环取全部窗口 max 保持原版区别。异步作用域、迟到
结果、深拷贝、持久化、占用下降/超容量及主侧隔离均有回归。

原版新增独立 footer harness 复用冻结 production AccountUsage/context，公开
GPUI mouse down/up 打开卡片；Go fixture 复用实际 footer，DTO 控制弹出状态。
两者都不是 OS 输入验收。旧1x阶段保留；显示器后来实测为200%，未由脚本修改
系统设置。最新配对是原版 `output/footer-reference-capture-20261028-r3-2x`
与实现 `output/footer-implementation-r3`，均为 logical840×560、client1680×1120、
完整WGC1684×1180，窗口内客户区偏移[13,58]。不能把1x图缩放充当2x图。

`output/footer-comparison-r3-2x-reviewed/comparison.json` 的28组完整原图 SSIM
为0.998987004–0.999950231，全部超过0.995，无裁剪、缩放、配准或遮罩。
空白背景占比高，分数仅用于组件筛查。账户卡121px与 context 卡/label 宽度
另由原版实际2x Inspector bounds 交叉核对，见 `audit-footer-text-bounds.md`。
严格比较工具独立审阅及33项测试通过：31个负例均在建立输出/进入parity前拒绝，
真实1x和2x正例可达比较阶段。完整 footer、Git/PR 菜单、动效、其他DPI与平台
仍须验收，阶段记录为 `reference/validation/footer-result.json`。

Windows 包实际观察到主聊账户卡、context卡、只读Git/PR显示，以及工作区侧聊
独立的4% context（主聊21%）。尝试继续点击时 Computer Use 检测到用户输入，
随后截图与目标窗口不一致，已停止键鼠，只按路径核对后清理自己创建的测试进程。
侧聊卡点击和新checkout/PR跳转未完成，未声称自行退出0。该原生观察包与最终
lint修复后的包分别记录，不混用二进制身份；见 `output/footer-native-ui/evidence.json`。

本阶段首次 lint 的6项检查发现已保留日志并修复。最终测试、lint与四架构包
重新按冻结源码验证，详见阶段JSON；Mac仍只有构建/资源检查，没有实机验收。
最终构建第一次在ARM目录rename遇到Windows Access denied；原失败目录/日志保留，
ACL正常且磁盘充足，未确定外部锁持有者。改用全新目录后四目标与完整包校验
通过，最新路径为 `output/resume-validation/footer-final-packages-r2`。
WGC fixture 的截图属于各自已记录二进制；后续 fixture 窗口位置固定及 Git
错误文案/静态检查修正不追溯改写原图，直接渲染源码哈希另行核对。

### R7：实际 2× 窗口与输入样式

R7阶段 MyGo 为 `zeron-native-ui.4`，1,834 文件、9 个有序补丁，完整树指纹
`eb9a7c79a121eaf1fb8ac0da0cd2b3956de04df67d6a3079571130b7a0be7f5e`。
从固定 v0.6.4 模块归档重放9补丁与当前树逐字节相同；反向撤销新增3补丁
精确还原 `.3`。新增能力为可选 SnapToWindow、PlaceholderColor、CaretWidth；
旧 AttachTo 默认行为不变。完整 UI 测试、生成一致性和体积/依赖检查另行记录。

源码核对纠正了两种看似合理但错误的推断：GPUI 的8px贴窗只在实际越界后
应用，窗口内0–7px位置不移动；ComposerInput renderer 的22.75行高是绝对DIP，
不能按外层 ui_rems 声明随界面字号缩放。冷开320/above、compact默认向上、
搜索14px/Faint、2DIP原生caret和列表样式也按实际代码修正，详见
`audit-native-popup-style.md` 与 `audit-model-list-residual.md`。

原生fixture现绑定真实Window，避免未绑定时1x fallback冒充产品2x分支。
原版独立8图在 `output/reference-lossless/capture-native-state-20261028-2x-01`，
实现8图在 `output/captures/model-picker-fixture-wgc-20261028-r7-2x`。二者均为
实际2x/DPI192、完整WGC、未变换；实现 nativeWindowAttached=true、
oneXOptionText=false。这些字段不能通过双侧改metadata代替实测。

| R7 complete PNG SSIM | wide | narrow |
| --- | --- | --- |
| dark panel | 0.996676105 | 0.992292948 |
| dark model list | 0.995044299 | 0.988087192 |
| light panel | 0.994310028 | 0.986806768 |
| light model list | 0.994854164 | 0.988500020 |

2/8达到0.995筛查，0场视觉验收。R6的1x与本次2x属于不同实测域，不能直接
用通过数宣称改善或退步。原图可见滑条材质、阴影、文字/图标和搜索caret时相
残差。新页脚实现 `output/footer-implementation-r4-2x` 与原版R3实际2x配对，
28组为0.998796753–0.999755061，均达门槛；整窗空白与非客户区状态仍影响分数。
账户卡/context卡全图已人工查看，但组件筛查不能代替完整footer和输入验收。

比较器guard分别55项（2正例/53负例）与36项（2正例/34负例）全通过；负例在
建比较输出/进入parity前拒绝，正例截获到go build，不执行真实Go/parity/native。
caller-local替身与marker恢复已验证，另独立核对2x的16张PNG/sidecar/IHDR/哈希。

冻结256项源码后，全量/tagged测试、Go vet、MyGo vet与lint0 issues通过，四目标
检查包已重建并验证资源/架构/ZIP/checksum/`.4`标记，位于
`output/resume-validation/popup-r7-packages`。源码在构建及36张实现原生自捕获
后无漂移；fixture进程退出0，无OS键鼠。随后同批R7 Windows包分别启动default/
dark与transcript/light，原生client自捕获、退出0、stderr为空，证据为
`output/resume-validation/popup-r7-native-boot/evidence.json`。它只验证启动/路由/
绘制，不是完整WGC、参考配对或新输入验收。Mac仍待实机；fixture、当前包与
旧包观察分开标识。准确文件哈希、范围与待办见
`reference/validation/popup-r7-result.json`。目标继续active。

R7包的后续原生输入补充见 `reference/validation/native-input-r7-result.json`：
从transcript真实Agent卡打开工作区侧聊，侧卡8,400/200,000、主卡42,000/200,000
分别点击核实；侧编辑/发送只进入进程内mock，完成后侧卡13,200/200,000（7%），
主聊仍21%。原生侧caret/文字路由正确而UIA焦点仍报告主Message，不能宣布
无障碍焦点一致。三个可见测试窗口通过Alt+F4关闭并确认不再存在；没有记录
这些交互窗口的退出码，不能称exit0。第一隐藏启动无可选窗口，按自身路径
核对后终止；其日志与此前退出0的自捕获启动分开保留。

此次也复现了两个应用问题：workspace TabStrip覆盖仍然存在的header控件，以及
Toggle right pane的primary Menu拦截OnClick。随后R8已完成分槽布局与secondary
ContextMenu修复，并修正accent plate的CSS inset边界差异。R7图、分数、源码及
包的身份保持历史原值，后续源码不能沿用这些结果冒充新的验证。

## 先前阶段的集成检查包

包含模型按键/ChoiceLabels 的完整 Go 测试、Go vet、MyGo vet、固定框架校验通过。
构建会临时写删 syso；一次并发 lint 读文件发生竞争，构建后串行重跑为0 issues。
日志为 `tests-integrated.log`、`go-vet-integrated.log`、`mygo-vet-integrated.log`、
`lint-integrated-after-package.log`。完整 MyGo UI suite 另按 `.3` 运行通过。

Windows amd64、Mac Intel、Mac ARM、Universal 包均已构建并验证架构、原生终端
库、资源、ZIP、校验和与 `.3` 精确标记，输出在 `output/resume-validation/packages`。
首轮 Mac 原生库下载 TCP 超时，改从同一 GitHub 官方 release asset API 获取，
两个库逐字节 SHA 与固定插件清单一致后进入依赖缓存；重跑构建和校验均通过。
这不代表 Mac 原生运行、签名、公证或安装验收。

本阶段172项应用/构建源码指纹在 `integrated-source-hashes.json`，构建和原生
操作后逐项复核无漂移。此前161项清单及其二进制/JPEG仍按各自阶段保留。

## 接续

捕获边界已统一；继续修复已配对定位的 compact popup 视觉差异并重新采集。
79张布局阶段的实现矩阵已重新生成在 `output/resume-validation/implementation-matrix`，
覆盖主题/路由/场景/视口/scale；它们来自 MyGo Tester，不能作为原版或原生合成验收。
Composer 完整 footer 验收、动画/材质、Windows DPI/IME/剪贴板/网页叠放、性能及
Mac 双架构实机门槛仍按原目标保留。JPEG 观察图不能冒充无损参考。

旧交接中“等待用户恢复”的条件现已满足。`HANDOFF.md`、`goal-execution.md` 和
`machine-handoff.md` 已注明恢复；composer/account 审计中旧的设备过滤待办与侧聊
trailer 待办已更新。旧审计统一写 expanded120 下限和空侧 footer 的陈述
分别遗漏 settled dock104 分支、条件 context ring，现按源码明确更正。
其余历史构建/截图仍只证明各自记录的源码与条件。

### R8：描边、标题栏与 provider 声明

本轮封存记录为 `reference/validation/popup-r8-result.json`。当前 MyGo
`zeron-native-ui.5`，1,835文件、10补丁，完整树
`282c9e97b03e120d3960f9ee38ebf5f9407075b100f99c2d03ece74c37545e00`。
第10补丁正向/反向/再次正向均精确匹配 `.4`/`.5`；counter/hello的最终检查包
体积差均为0。ProviderOptions从错误的ClientSide标记改为ServerSide加COM线程标记；
AccessNode.Bounds只纠正文档为未裁剪border box，没有改变裁剪行为。

accent overlay按原版border box覆盖，消除CSS padding-box inset留下的双描边；
slider指针、拖动、焦点、键盘回归通过。标题栏改为实际互不重叠的槽，takeover
隐藏session info，primary点击pane与secondary菜单独立；原生新包已复测按钮、
展开/收回、信息卡和空白标题栏拖窗。详见 `audit-titlebar-uia.md`。

全量/tagged Go测试、Go vet、MyGo vet、固定v2.15.0 lint和MyGo Windows/UI测试
均通过。初次Go门禁扫描到Git忽略的output内诊断备份，失败日志保留；原字节移入
`_source`后完整重跑通过。260项源码在构建、截图、原生操作后逐项核对未漂移。
四目标资源/架构/ZIP/checksum/`.5`校验包在
`output/resume-validation/popup-r8-packages`；Mac仍只有构建证据。

实际2x的新compact8图和footer28图分别在
`output/captures/model-picker-fixture-wgc-20261028-r8-2x`、
`output/footer-implementation-r5-2x`，原PNG、sidecar、IHDR、真实Window分支和哈希
全部核对，fixture退出0且未用OS键鼠。与同一独立原版2x配对，compact为
0.988087192–0.997084323，3/8达到0.995，列表分数与R7相同；footer仍28/28
达到门槛。0场视觉验收，完整整窗比较没有裁剪/缩放/掩码，blank scaffold和原生
非客户区状态仍限制数值解释；55/36比较器guard沿用未变更的既有证据。

本批Windows包的原生输入记录在
`output/resume-validation/popup-r8-native-input/evidence.json`：真实Agent卡进入
工作区侧聊，受控文字和Ctrl+Z只作用于侧编辑器；本轮没有发送消息。新包经选中
自身窗口Alt+F4关闭，记录实际exit0、stderr空。UIA仍报告主Message而非侧编辑器，
所以provider声明修正不能宣布焦点异常闭环。尚未完成其他DPI、IME/剪贴板/网页、
材质/动画/性能、全路由和Mac实机验收，目标保持active。

并行的原版文字量测只进入独立adapter，UIFont13/16/20实际2x三次均exit0，
源码及实际bounds见 `output/model-picker-2x-text-audit/r9c/actual-bounds-evidence.json`。
早先adapter panic及三个exit101记录保留。它证明当前两个整数row位置的几何，
没有直接导出glyph baseline，也未覆盖分数祖先位置；不能据此全局上移文字。

### R9：独立读取公开 UIA 焦点

后续仍用R8同一封存Windows包，重新挂载主/侧编辑器，独立MTA进程直接调用
跨进程 `IUIAutomation.GetFocusedElement`，在其前后读取两编辑器Current焦点及完整
RuntimeId。侧→主→侧三阶段共9轮稳定样本、18次GetFocusedElement成功，36次
Current焦点读数均跟随真实输入。独立fresh managed UIA（STA）也返回侧编辑器。
同期原工具仍报告主Message；受控文字和Ctrl+Z只作用于侧，进程实际退出0。

这将已测序列的错显收窄到原工具读取/缓存/映射路径，不能继续仅凭它认定应用
公开焦点错误。具体工具机制、FocusChanged事件、屏幕阅读器、popup/IME/全键盘
矩阵仍未证明；两诊断进程的DPI awareness不同，矩形未用于几何验收。
R7/R8原始观察与未决结论按历史保留，补充证据在
`reference/validation/native-focus-r9-result.json`。本诊断未修改产品或框架；R10
画布显示条件修正属于后续源码，不能沿用R8包声称已做新原生验收。

### R10：有效会话画布与工作区往返

当前封存阶段为 `reference/validation/canvas-r10-result.json`。实际selected-session
lookup同时控制标题栏尾组和right/Files的effective fit；空ID或已不在snapshot中的ID
都不会让右侧占宽或takeover遮住新输入区，但面板意图未被清空。新建会话还明确
关闭canvas bottom terminal，保留之后Ctrl+J打开终端的功能。

260项源码相对R8仅app/titlebar/session_state及titlebar测试四文件改变；MyGo仍`.5`。
全量/tagged测试、Go vet、MyGo vet、v2.15.0 lint全部通过，四目标检查包在
`output/resume-validation/canvas-r10-packages`。Windows EXE为
`cccb9bd2ba30e5ce485a4bd0504978f7523b9f9b8eec23da3ad7d2e58f28c224`。
构建及原生操作后逐项核对260项未漂移，临时syso已清理。

App.View回归验证真实原生editor脏缓冲区、tab、会话draft、面板/expanded状态在
会话→canvas→会话往返中保留，恢复后expand和pane按钮仍可点击。Windows新包
随后以OS键鼠重复该路径：Ctrl+J→Ctrl+N→Ctrl+J、选会话、Files/layout.go、
未保存注释与主draft、expand→新画布→回原会话、collapse/关tools。主draft与
注释仍在，Files保留；37份raw UIA/JPEG/client/launch/exit文件已hash绑定，实际
exit0且stderr空。未保存文件或发送消息，也未使用用户数据目录/真实服务。

R8 compact/footer图保持原二进制身份，本轮未重新采集或给新EXE套用旧SSIM。
滚动/选区、多tab次序、源会话terminal=true恢复、全部视觉及Mac实机仍未验收。
snapshot层清理dangling selected ID尚是独立待办；本轮仅修正其显示分支。
R9文字snap设计和fractional-scroll实测仍待后续，未实施新框架API。

### R11：会话数据衔接、创建关联与标签栏

R10 的显示 guard 不能清理发送目标。R11 从已接受 Sessions 校正失效选择，
恢复画布原项目/设备/草稿，保留 Settings section 与当前 PR surface；离开 PR
到聊天/Files/History 等入口时同步 workspace。历史失效路由也回到画布。
只清理所删会话主草稿/待发送状态，保留其他工作区脏缓冲区；迟到发送失败和
附件结果不得重建删除的草稿。项目删除 fallback、显式无项目、归档仍存在、
旧/equal快照、Cloud 同项目同设备与跨目标的区别均有回归。

容量1的订阅会合并瞬时ACK。新增可选 `Run.requestId`，完整快照和持久化保留
创建关联，App同时约束当前draft代次和发送前已有RunID。新会话不再采用任意
第一条新ID；切页时成功完成不会永久pending或抢走当前页。详见
`audit-session-reconciliation.md` 与独立 `implementation-review.json`。

标签栏按源码改为expand28×28、plus24×24，plus在可滚动chips末尾，只有expand
固定；支持展开/折叠图标、150ms hover和有效reduced-motion；非Git隐藏对应
菜单项，Git能力使用精确项目的typed GitDetected。原文档的24/24、plus固定和
非Git禁用已标历史。默认scrollbar仍与chips底部重叠约7DIP，36px fade、拖动/
reveal动画和168px菜单样式未完成，不能把本轮尺寸修正当作完整一致。

最终262项源码相对R10改变16项，框架仍为`.5`，源码manifest为
`output/resume-validation/session-r11-final-source-hashes.json`。最终r4全量、
tagged、Go/MyGo vet及固定v2.15.0 lint均通过，四目标包在`session-r11-packages`；
Windows EXE SHA为`371843aa3fdd6a44e80ca00f45cef53039ea6432437e715c565506b863e190c4`。
构建和原生操作后262项源码未漂移。

两类Windows原生观察分别封存：发布EXE实际输入/Enter新建mock会话、Changes、
加号菜单、展开/折叠；独立adapter实际选择/两份草稿/工具展开，再通过typed后端
删除自有chat-0，由App.Attach正常订阅回到画布并恢复原草稿，实际Enter创建
session-1001，后台user block/Run.requestId吻合。两窗口查询DPI192、per-monitor，
实际exit0、stderr空；发布18份、adapter26份raw文件独立hash。adapter首次被
Hidden启动参数隐藏而没有可操作窗口，自有进程停止exit-1保留失败记录；第二次
同binary可见运行完成，未把失败重标通过。adapter不能冒充发布包的删除菜单验收。

R8成对图片分数未用于R11；R9文字API尚未实施。完整视觉、其他DPI、IME、
剪贴板/浏览器/性能、FocusChanged/读屏及Mac Intel/ARM实机仍待完成，目标active。

### R12：标签滚动条命中、渐隐与空闲刷新

封存为 `reference/validation/scrollbars-r12-result.json`。MyGo升为`.6`，
1,836文件/11补丁，完整树`cefb5a5c8694f538ac4caeecb080d2fa96adbaf15c6c12e234e1a4bd06cce914`。
上游11patch重放、撤销11回`.5`、再次正向回`.6`精确一致；生成文件幂等，完整
ui/markdown测试和vet通过。只对TabStrip隐藏indicator及hitbox；wheel、键盘、
辅助功能和其他滚动容器的默认行为保留。

不透明背景36px全高shell渐隐已接入；pure paint读取本轮ScrollState，修复6种
MaxX变化而无下一帧请求造成的空闲错误，8项idle回归及51项工作区测试通过。
冻结原版的上一帧flags路径不等于确认原版有同样缺陷；过渡逐帧一致仍待验证。
新完整263项源码相对R11仅8项变化，五项主线门槛全部通过。

5目标×2示例before/after共20次生产构建全部0：counter在Windows/Linux不增，
Mac amd64/arm64分别+16/+32字节，wasm+101字节；hello五目标均不增。
nm/dumpdep和wasm逐函数检查归因为原有可达函数条件分支及少量类型数据，无新重依赖。
不能称全平台零成本；原Windows+512候选保留为已解决记录。

第一次fresh输出在Mac amd64 staging重命名遇WindowsAccessDenied；失败日志和
部分Windows产物保留。源码未改，第二个fresh目录
`output/resume-validation/scrollbars-r12-packages-r2`的四目标架构、资源、ZIP、
checksum与`.6`标记全部通过。最终Windows EXE SHA为
`1e8d0dc3290b2a4f4d97095e45c59fb0c54542e8ad65736857281cbe351c2fe0`。

该EXE在DPI192/per-monitor下由实际OS点击/横向滚轮打开Changes和两diff，
窗口相对y32/y33点击切换layout/README内容，加号底边打开五项原生菜单并选择
mockHistory，展开/收回正常。PID30872/HWND97785678，实际Alt+F4退出0、stderr空，
48份raw文件封存。原生样点证明本次底边操作；最后1DIP和1x/2x完整矩阵仍是Tester证据，
不是全原生边界验收。JPEG和客户端PNG没有当作独立原版golden。

原生还发现History新插入或收回takeover后可保持裁切，手动wheel能滚入；该问题保留
截图并进入R13诊断，不能由既有键盘测试推成自动滚入全通过。玻璃alpha遮罩、拖动/
滚入过渡、自绘菜单、独立参考配对和完整跨平台验收仍待完成。细节见
`audit-tabstrip-scrollbars.md`，目标继续active，未提交/推送。

### R13：菜单滚入请求与生成索引换行（本机2026-10-29）

菜单Chosen在旧chips构建之后同步增加/选择History，原函数末尾清理revealTab使
下一次重建丢请求。修复只在入口消费已存在请求，保留本轮新请求。8个真实菜单
case先失败后通过，6个direct/End对照通过，manualwheel不sticky，本包53项测试通过。
原MaxX已正确增大，先前“旧MaxX限制”只是候选，现已由数据排除。

263项源码相对R12仅3项变化；框架`.6`完整树未变。生成licensemanifest仅306个
CRLF→LF、JSON等义、50份其他许可文件不变，默认diffcheck由R12的2恢复为0。
R12原始263项源码和10份文档逐字节备份，旧证据未覆盖。主线五门槛及四目标
`output/resume-validation/reveal-r13-packages`资源/架构/ZIP/checksum/stamp全部通过。

最终Windows EXE `36ede6c6f3b4c26cf2b57ee3ad8d4334bdbdaebfe16719edce3180bcad84d67f`
在DPI192/per-monitor下实际从旧plus末端菜单选History，立即及idle都完整显示
活动chip，无补救输入；随后手动wheel移开，idle不强制拉回，History正文仍在。
PID31144/HWND584059688，实际exit0、stderr空，34份raw文件在独立目录。

缩窄/收回的裁切与菜单请求流失不同，原版没有已证明的resize自动滚入要求；
R13保留其语义。plus可见性也不等于activechip揭示。原版匹配fixture与完整
平台验收继续待做，详见 `audit-tabstrip-reveal.md` 与R13阶段索引。目标active。

## R14 — 生产TabStrip独立原生基线

阶段索引 `reference/validation/tabstrip-r14-result.json`。Go生产绘图沿用R13，只加
三个Handle观测绑定与普通构建空实现；280项来源相对R13新增17项、仅两项既有
工作台文件改变。`.6`/1,836文件/11补丁完整树不变，五门槛及默认diff-check通过。
R13源码和文档原字节均备份；R14没有新发布包，不能移用R13 EXE身份。

原版/Go真实组件各36场景，840×560DIP、400/220宽、0/2/6空闲SideChat、
dark/light、各自实测最大滚动比例0/0.5/1，实际DPI192。先后采集且各自正常退出。
初轮原版只有GPUIcached active，Windows App.activate为空实现，未证明Win32前台；
该轮原图/分数保留。最终reference02/run02与Go attempt05/run07保存捕获前后实际
foreground HWND/PID，原版另核实owner UI线程；Go raw native/State都在UI线程读取。
Go07比失败06新增2秒最小呈现间隔，之后仍有实际就绪检查，不以等待时间替代证据。

69项guard通过，包含类型、owner线程、前后身份、图像完整性、alias/ancestor路径
和原输入漂移后的快照隔离；3正控拦截go build，没有运行parity，66负例在预检拒绝。
真实36对另运行完整原PNG比较，SSIM0.9998630230588416..0.9999965707212943，
独立重算一致。整窗空白主导分数；host仍有112..14642像素差异。caption按钮变黑
但背景仍有20772个最多1级通道差异，实际screenorigin不同，原因未确定。

选中/hover wash、inactive generic glyph、6DIP dirty dot等源码差异保留待R15修正；
空闲SideChat不替代dirty文件、玻璃、hover、实际OS输入或Mac验收。见
`audit-tabstrip-fixture.md`。目标active，未提交/推送，不标complete。

## R15 标签样式与原生对照

287项源码已封实际字节（9改7增），五门槛全过；36对沿用原版基线重新对照，
另有4组hover/dirty补充。dark底色已同#232323，浅色蓝通道仍差1；四目标发布包
校验和确切Windows EXE标签交互退出0，完整验收未接受。见 `audit-tabstrip-style.md`。

## R17 V5：已接入源码与隔离原生证据

正式接入记录为 `output/tabstrip-r17-quarter-v5-adopted/attempt-01/result.json`；
首13补丁保持，第14补丁为Windows glyph placement。接入前c18a…1165诊断树
使用自己的EXE完成run17/18/19：40份完整PNG与独立V4AX run15/16全字节相同。
run17缺浅色AX采样，run19补采后两主题均为`Tab02.txt`；没有把缺采项写成通过。
相对冻结原版，dark/light全帧仍差22,410/39,185像素，文字最大通道差1，未视觉接受。
原生封存45d197f0…56dd及独审67e5db0c…9ca2均保留其c18诊断身份，不是`.9`新包。
新ledger R6为21试验/206PNG/17成功/4失败，旧18个trial对象全部保持。见
`output/tabstrip-r17-quarter-v5-ax-native-seal/attempt-01/`、
`output/tabstrip-r17-quarter-v5-ax-native-independent-review/attempt-01/` 与
`output/tabstrip-r17-native-fixture-ledger-r6/`。

新`.9`全仓门槛attempt01的pin通过，但标准全量测试发现output中的未隔离诊断
副本而exit1，业务包通过不替代整条命令失败。已加`output/go.mod`共享module
边界，不改既有证据路径/字节与2931项生产输入，也不按包过滤。attempt02的
pin/full/tagged/Go vet/MyGo vet均exit0，但lint启动前被GOPROXYoff的固定module
deprecation元数据解析拒绝；attempt03只重跑lint，使用既有本机file代理缓存后exit0，
没有网络、工具版本或源码变化。最终记录复用02五项精确raw refs，六命令均0，
不把03说成全套重跑，也不称首次全绿。最终门槛记录为
`output/tabstrip-r17-quarter-v5-formal-gates/attempt-03/result.json`（831c2944…151c）；
原失败和修正见`output/tabstrip-r17-quarter-v5-formal-gates/attempt-01/`与
`output/tabstrip-r17-root/v5-gate-discovery-correction.json`。

canonical attempt01只做两条exit0的Go预检；runner把`GOENV=off`时合法的空输出
误判，0次实际构建/0包/2931源未变。根因是验收假设，不是用户环境错误；原结果
见`output/tabstrip-r17-quarter-v5-canonical-delivery-attempt01/result.json`。新02只修
该验收且仍保持环境变量off，随后完成一次canonical build与四目标校验，九命令
均exit0、2931源未变；不把01预检称为实际构建。新Windows候选包EXE为
`9c58004289a9da967b3e1b6e1baae03c031374b2f0c3aa6fc272677e0b8ff795`，
构建记录未运行其GUI或installer，也未执行Mac原生。最终封存为
`output/tabstrip-r17-quarter-v5-canonical-delivery-attempt02/final-result.json`
（8043c0a3…768d），原release索引仍R16，不据包校验宣布视觉或发布接受。


### `.9`确切主包的限定 Windows smoke

同一canonical Windows EXE `9c580042…f795`已在独立新记录中实际运行，不再只有
build-only证据：Files→README→layout→重选README，实际detail Name为`README.md`、
close Name为`Close tab`；右键原生截图菜单关闭layout后，README编辑器、文件树与
会话保持，最后Alt+F4正常exit0、stderr0。14状态与15份原Sky JPEG、初始client PNG
单独封存；2931当前源及对应备份稳定。新seal为
`output/resume-validation/tabstrip-r17-quarter-v5-native/result.json`
（0d850ab4bebfd6c9a308ea015266321fb64e7028dbcbdda926c77842fbb7767c）。
observed client为3200×2000 pixels，requested为1600×1000，不能据比例推断fresh DPI。
这是限定Windows行为smoke，不是视觉对照、精确350/500ms、installer、Mac或读屏
事件接受，也不把旧c18诊断夹具40图重标为本次主包。
