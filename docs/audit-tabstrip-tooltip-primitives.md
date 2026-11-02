# R17 提示状态、点定位与渲染设备取证（进行中）

R17 目标当时为 active；UI 优化自 2026-10-29 起暂停。最新完整封存索引仍是
`reference/validation/tabstrip-r16-result.json`；本文记录 R17 的候选和观察，
不替代该历史证据，也不表示发布、完整视觉或跨平台验收通过。

## 历史 R17 `.9` 源码（发布未接受，当前框架为 v0.7.4/`rcode-native-ui.22`）

此前按受审的27项差异接入MyGo `.9`：1,864文件、14补丁；该次采用的2931项正式输入，树为
`27e5b9c724175bd11d537765210727fe25722a3407989e7fdcae71fa4357e8d9`。首13补丁原字节和顺序保持；
第14补丁`mygo-v0.6.4-windows-glyph-placement.patch`的SHA为
`8a463bc728654abeb1ae3461687a5d8512a69e77f45284640aec5fc0b097a893`。采用记录为
`output/tabstrip-r17-quarter-v5-adopted/attempt-01/result.json`（3f4175e6…bb07c）。
该次采用的AX app961只将原生提示leaf包在checked `WithGlyphPlacement`中，leaf仍为
`0bcfb0cc…be51c`，未恢复generic Label或旧959项app。该接入不包含隔离换行候选。
随后另采用下文Cloud导航四文件，不将旧2931/961输入表或旧canonical包重标为含Cloud改动。
Cloud修复前的`.9` pin、五门槛与canonical四目标候选包构建/校验已通过；旧C4或V5诊断结果不重标。
最新封存发布索引与resume hub仍保留R16历史指纹，完整目标保持active。

新的正式门槛attempt01中，pin退出0，但标准`go test ./... -count=1`因发现
未隔离的output诊断副本而exit1；业务测试包通过不能把整条失败命令算成功。
Root新增`output/go.mod`共享module边界，原证据路径/字节、原runner与2931项
生产输入不变，标准根命令不作过滤。原失败与修正分别见
`output/tabstrip-r17-quarter-v5-formal-gates/attempt-01/result.json`、
`output/tabstrip-r17-root/v5-gate-discovery-correction.json`。attempt02的pin、full、
tagged、Go vet、MyGo vet均exit0，lint启动前因GOPROXYoff拒固定module deprecation
元数据解析而exit1。attempt03仅重跑lint，使用已有本机
`file:///C:/Users/chengliang/go/pkg/mod/cache/download`后exit0，无网络、工具版本或
生产源码变化；最终记录复用02的五项精确raw refs，共六命令均0、2931源未变。
见`output/tabstrip-r17-quarter-v5-formal-gates/attempt-03/result.json`
（831c2944ee20348513f0df369c018e0932136a2218a0af93bc565de6a81b151c）。
01/02原失败均保留，03不是全套重跑，不能称首次全绿。

canonical attempt01仅完成两条退出0的Go预检命令；runner误要求`go env GOENV`
输出字符串`off`，实际环境变量`GOENV=off`使Go1.27.3合法返回空值。Root已核
`EnvFile`/envcmd源码，属于runner验收假设错误，不是用户环境错误。该轮0次
canonical build、0包、2931源未变，原结果为
`output/tabstrip-r17-quarter-v5-canonical-delivery-attempt01/result.json`
（2ee71944…d9cc）。新attempt02只修该验收，环境仍为`off`，随后一次实际
canonical build完成windows/amd64、darwin/amd64、darwin/arm64、darwin/universal
四目标与checker校验，九命令均exit0、2931项源前后无漂移、231个输出文件。
不能把01 preflight算成实际package build，也不称首次全绿。最终记录为
`output/tabstrip-r17-quarter-v5-canonical-delivery-attempt02/final-result.json`
（8043c0a304287937181fdcdf7c5aba6d29e012b355028e5267ece3e99ced768d）；
postseal复核3242条artifact引用及生产源/备份、包输出匹配。新Windows候选包EXE为
`9c58004289a9da967b3e1b6e1baae03c031374b2f0c3aa6fc272677e0b8ff795`；
该构建记录没有运行其GUI、installer或Mac程序，没有重测unused gate，不代表
视觉或发布接受。新的完整包身份不借用旧C4、V5隔离夹具或R16发布身份。

后续已对这个确切`.9` Windows主包9c580042…f795完成独立限定native smoke：
Files→打开README→打开layout→重选README，实际detail Name=`README.md`、
close Name=`Close tab`，generic Label仍缺席；从右键原生截图菜单关闭layout后，
README编辑器、文件树和会话保持，Alt+F4退出0且stderr空。14份状态、15份原Sky
JPEG和初始client PNG已封，2931当前源及对应备份匹配。新记录为
`output/resume-validation/tabstrip-r17-quarter-v5-native/result.json`
（0d850ab4bebfd6c9a308ea015266321fb64e7028dbcbdda926c77842fbb7767c）。
实存client3200×2000 pixels、请求1600×1000，不能由比例推断fresh DPI。
该smoke不表示视觉、installer、Mac、读屏事件或精确350/500ms边界接受；旧c18
诊断40图仍保持自己的来源，新主包native没有借用或重标它们。


采用依据中的体积矩阵属于接入前V5 c18a…1165、继承`.8`的诊断树：10个after build
相对历史C4均raw不增长，9项delta0、Wasm counter−425B。它修复V4的Darwin+32/+16
门槛失败，但不证明metadata、CPU或runtime0；历史C4对`.6`的counter+2,048B仍待闭环。
首次default-six调用的索引设施错误及明确授权resume保留，没有改业务expected。
见`output/tabstrip-r17-quarter-v5-execution/attempt-01/final-result.json`。

V5+当前AX的独立组合为2825项审计输入，另有30份RawSVG测试补充；四项有限检查
全部exit0，gofmt只读无diff。实际EXE e0f85e5d…13619以自己的c18树/stamp完成
run17/18/19，40份完整PNG与V4AX run15/16逐字节相同。run17缺浅色AX采样，run19
补采后两主题实际名称均为`Tab02.txt`。相对冻结原版的dark/light全帧差异仍为
22,410/39,185像素，文字最大通道差1，不能据回归相同宣布视觉一致。封存
`output/tabstrip-r17-quarter-v5-ax-native-seal/attempt-01/result.json`为45d197f0…56dd；
独审`output/tabstrip-r17-quarter-v5-ax-native-independent-review/attempt-01/result.json`
为67e5db0c…9ca2，15,211检查无新增finding、保留1项规划P3。它们不是`.9`新EXE证据。
V5阶段的历史ledger R6为21次试验、206PNG（含保留失败部分原图）、17成功/4失败；旧18个
trial对象全部保持，见`output/tabstrip-r17-native-fixture-ledger-r6/`（9d20f728…1c22）。

新增规划P3：准备器显式LF而冻结builder在Windows默认CRLF，导致预测manifest
34f99f9a…26db4与实际d143761a…13621不同。2825个完整行内容、顺序、key顺序及
源/编译备份原字节全同，19777处LF→CRLF准确解释19777B差额。实际EXE/stamp只
绑定实际d143；不能把旧预测改称实际使用。原plan/build均不改，追加比较见
`output/tabstrip-r17-quarter-v5-ax-manifest-serialization-review/attempt-01/`（d0fbd1bd…41ff）。

GPU source04的cfg(test)常量拼写错误exit101保留；source05仅修测试token后，
7/7纯Rust测试及locked/offline release build均exit0，见
`output/tabstrip-r17-text-leaf-gpu-build-attempt05/final-result.json`（e00afda0…7e3）。
随后独立bounded24 synthetic single-RTV native pilot已exit0并完成24/24项，
exact-texel-center sampled实际通过。48份raw共196608B，24份after逐word精确等于
CPU f32 q/255 RGB、alpha1，24份before均为正零；878份runtime文件已重hash，379
输入pins未变。最终封存为
`output/tabstrip-r17-text-leaf-gpu-runtime-analysis-attempt03/final-result.json`
（0ff95f6a07fcbe3b10156c26f58fb768616725d00c02be9692310b0dec966481）。
这不包含derived、gamma、MRT、blend、6048 full矩阵或actual UI，不能据此解释
GUI残差；截至该runtime03阶段actual上传words仍不可用，既有模型或该次sampled raw不替代actual UI证据。

## P0实际上传采集与GPU04：进行中

GPU04已完成288个source-parameter-derived case，含48个实际双RTV MRT、672份完整
readback，共2457600B。144组同profile flags对及144组Load/Sample对全相同。
共享brush后16组corrected仍精确相同，但全部24组BGRA8跨路径对仍最多差1级RGB；
暗色Hsla的1ULP差不足以单独解释这个受控leaf差异。CPU模型与GPU的非零ULP差异
保留，不据此确定FMA、OM、gamma或真实GUI根因。实际执行及独立重算分别见
`output/tabstrip-r17-text-leaf-gpu-runtime-analysis-attempt04/final-result.json`
（146b75cb…62a85）及
`output/tabstrip-r17-text-leaf-gpu-runtime04-independent-review/attempt-01/result.json`
（8e997422…1bfb9）。这不是6048全矩阵或实际GUI replay。

P0 Go采集分支基于隔离V4AX：baseline2824、trace2827项输入，继承`.8`，不是正式`.9`。
两次offline实际构建、六项typed ABI/epoch/flush/path测试通过；原汇总器误断言
`go version -m`首行格式的失败保留，仅修汇总解析，未重跑构建/测试。见
`output/tabstrip-r17-p0-go-builds-sealed-attempt02/result.json`（91576031…9006e）。
首次native baseline正常退出0，但暗色整图失败、缺浅色成熟Sky；其原图和独审保留。
暗色卡片/正文左2px，正文对应ROI相同，整卡对应仍有2个边缘像素差，不能当作
对齐后通过或已确定唯一根因。原合成hover快照也不能当作OS鼠标位置。

随后新输出中依次执行未插桩baseline、trace-off、trace-on；参数和旧15整图expected
不变，校准实际输入并提前完成每主题离开/进入。三轮均正常退出0，两主题共6张
完整PNG逐字节等于旧15；23份原Sky状态/JPEG和六个实际成熟`Tab02.txt`提示保留。
trace-on正常Release后，独立raw解析确认12 uploads、12 Globals、346 instances和
67 issued draws逐项关联；其中47 default、20 custom PS身份notCaptured、0 effect。
这是host提交值与issued调用，不是GPU完成、Present、精确WGC producer或完整shader
对象身份观察。复合记录
`output/tabstrip-r17-p0-go-native-seal/attempt-01/result.json`为80d5b432…0dca2，独审
`output/tabstrip-r17-p0-go-native-independent-review/result.json`为d802b2e1…4ebf9。
当前受控fixture ledger R7为25试验/214PNG/20成功/5失败；首轮exit0但图门槛失败计入
失败，原R6的21对象原值保留。主包JPEG与独立GPU案例均不计入该ledger。

实际kind5共243条、4组profile，与独立raw解析逐word吻合；其中45/90条profile的
Color、Radii、Inner前两项、Params透明度分别匹配leaf模型两个case的Go字面量。
见`output/tabstrip-r17-p0-go-host-leaf-field-comparison/attempt-01/result.json`
（8b421491…9cd04）。这不按坐标/调色板推断具体字符串、主题、帧，也没有补出atlas、
逐像素clip/fade、underlay、参考侧HSLA或GUI残差原因。

参考侧先后保留路径过长、缺archive两次metadata失败；仅私缓存补齐381个固定lock
校验通过的archive后，第三次offline metadata退出0，wrapper按门槛退出2、未编译。
下载77581593B，c3解包后2064257290B，不能把网络体积当磁盘成本。
该图1298 packages/nodes、30 workspace members；lock1308，不把锁条目数当编译单元。
见`output/tabstrip-r17-text-actual-input-capture-prepared/attempt-02/metadata-reference-trace-attempt03/seal.json`
（c7f90cb2…b14c7）。当时独审发现默认图的zeron-ui features为空，而未来命令显式请求
`compact-picker-fixture`、启用`gpui_platform/test-support`；该P1及默认图独审
`output/tabstrip-r17-rust-metadata-independent-review/attempt-01/result.final.json`
（472a2838…ae3db）保留其历史时点。`tabstrip-native`自动发现target未声明required-features，
不把命令选择冒称Cargo target声明。

随后一次新feature+Windows-filter metadata实际退出0，helper按门槛退出2：
959 packages/959 resolve nodes/30 workspace members；显式请求的UI feature和
`gpui_platform/test-support`均在实际图中，锁5c7b128d…caf34与4323项raw缓存输入
逐项保持，work无差异。completion为
`output/tabstrip-r17-text-actual-input-capture-prepared/attempt-02/feature-metadata-execution-attempt01/run/completion.json`
（a1e9cf48…73f1e）；独审
`output/tabstrip-r17-rust-metadata-independent-review/attempt-03-feature-graph/result.json`
（1e4fe0d2…2a3b）26项通过，原requested-feature图P1由此关闭。图仍带workspace feature
union，不能当成单个package/example的实际编译units或仅凭同一lock SHA批准build。
这两项记录不含参考Rust项目编译；首Job生命周期smoke失败独立保留在
`output/tabstrip-r17-p0-rust-build-driver-prepared/attempt-01/smoke-runs/lifecycle-attempt01/result.json`
（8aa94b81…8178），不是Cargo编译。首次项目编译、SDK/链接/借用/ABI/native须另据各自记录。

后续Job02 lifecycle smoke的自有Python正常退出与父子超时终止两例均按预期通过，见
`output/tabstrip-r17-p0-rust-build-driver-prepared/attempt-02/smoke-runs/lifecycle-attempt01/result.json`
（055e6710…3e68b）；它不是项目编译或安全沙箱证明。随后首次参考Rust项目build已实际
发生：Cargo PID53184退出101，日志为`blake3 v1.8.7` custom build失败及LNK1181找不到
输入`.o`；driver退出1、仅一次、未自动重试。外层
`output/tabstrip-r17-p0-rust-first-build-attempt01/execution.json`（f76336b9…6fbf3）绑定
actual build和原始stdout/stderr。该次执行前后original/raw inputs、当时2931正式输入、
7docs及current保护均不变；`actualWorkInputDrift`实际为null，不能写成空数组或据此断言
私有work无漂移。后续诊断确认blake3的CC完整字符串识别与cc实际工具选择不一致。
第二次cached build仅改global `CC=cl.exe`，target CC仍绝对绑定；实际ml64/MSVC `.asm`
路径成立，Cargo退出0，用时11m23s。终态
`output/tabstrip-r17-p0-rust-cached-resume-attempt02/build.json`（97f00838…68d21）绑定
trace EXE f678bc89…c70ba及source/raw、实际work3846、C5 raw子集、锁和旧记录零漂移。
primary退出0与有界自有Job helper清理分别记录，不声称全部后继自然退出。首次101/null
原件不改；参考baseline、运行时ABI、baseline/off/on原生整图和真实输入捕获仍待执行。

Cloud导航另已采用四文件修复：每次新visit先fresh refresh，Ready/available/Off再自动
Enable；无中间View离开再回的旧visit也被取消并替换。01生产+02回归为4场景失败、
NewConversation控制通过；02九顶层及全仓overlay测试退出0。采用证据
`output/tabstrip-r17-cloud-navigation-adopted/attempt-01/result.json`（0b3a17e9…b090a），
delta独审见`output/tabstrip-r17-cloud-navigation-independent-review/attempt-02/result.json`。
此前仅有headless证据，现已补下述native正常路径；失败/取消/快速路由及线程trace仍未验。
DTO仍无`failed_action`。异常Enable成功却返回Off
仅一次的typed防递归也与原版可再次Enable的行为不同，详见
[Cloud审计](audit-account-settings.md)。旧19项历史差异不因本次导航修复被抹去。

Cloud正常native路径现已封存：独立offline profile的主程序开发构建，2933项输入
（manifest02e9e2e6…04684），EXE69a43719…f0090、PID50440。实际设置栏导航使signed-in/off
rev2→provisioning rev4→ready/cloud-1 rev5；同页重复仍rev5，Devices出现Cloud，真实设置
搜索返回仅fresh refresh到rev6。正常AltF4退出0、stderr0、2933输入与原四窗口不变。
12份Sky状态/9张原图中首张错窗拒收，激活目标后恢复；不把AX指定窗口当成精确WGC
producer证明。见`output/tabstrip-r17-cloud-navigation-native/attempt-01/result.json`
（a5981b8b…63b1c）。这是实际worker正常路径，非完整Cloud视觉/失败取消矩阵、race或
OS线程跟踪，也不是新canonical包，不关闭P0及焦点的其他native待验项。
Cloud live门禁的pin/tagged/Go vet/MyGo vet四项退出0；lint首次因GOPROXYoff读取
module deprecation元数据失败（7dd9ccae…93e9）。仅lint用已有本地file proxy重跑退出0、
0 issues（8d0df8f4…3b6b1），分别见`output/tabstrip-r17-cloud-navigation-live-gates/`
下attempt-01与attempt-02-lint-local；保留首败，不称首次或整套重新全绿。

新增规划P3仅指冻结prepared `plan.json#/graphScope`的“packages remains all workspace
dependencies”过宽，精确源为
`output/tabstrip-r17-text-actual-input-capture-prepared/attempt-02/feature-metadata-prepared-attempt01/plan.json`
（7bc04662…ea587）；追加澄清见本次completion关联的`scope-clarification.json`。
README的“workspace-wide”在30个members范围仍成立，不列为错误，也不称Cargo官方文档或
工具有bug。原plan/helper不回写；R11最终独审
`output/tabstrip-r17-progress-p0-r11-independent-review/final-result.json`（48946720…81a）
所保留的19项既有差异和各阶段as-of仍保持，这条prepared措辞P3单列。

### `.9`焦点复验：诊断路径已停止

source triage `output/tabstrip-r17-focus-ax-source-triage/attempt-01/result.json`
（04174c1d…36c3）未建立当前产品focus缺陷。其后仅启动现有9c580042…f795主包一次，
主输入标记及undo、真实Agent卡挂载side已观察，正常exit0；但raw PowerShell探针被
Execution Policy拒绝、exit1且未加载脚本/执行COM。新封存为
`output/tabstrip-r17-focus-ax-source-triage/attempt-01/native-execution-attempt01/result.json`
（822ccedc…16f61）。Sky与直接PS UIA同轮混用也不符合Windows Computer Use
`docs/guidance.md:237`；该路径已停止，未绕policy或改用其他运行时/接口规避限制。
四phase main→side→main→side公开focus及model popup矩阵仍pending，不据工具字段新立
产品bug或增加workaround。技能/执行路径限制不是产品代码缺陷。旧R9 raw MTA与fresh
STA在原被测R8包的有限sequence证明工具字段错显，不证明缓存根因，也不把该历史观察
重标为本轮`.9`结果。

两项冻结注释与后续事实有差异：采集测试仍写“尚未编译/执行”，新记录已实际执行
六项；`gpu.Instance`注释写十三个float4，实际十四个/224B。保留冻结源码字节，在本节
与新证据中标清准备时点，未借此改动正式框架或宣称全目标完成。

## 历史 C4 与早期候选

此前正式候选 MyGo `.8` C4：1,847 文件、13 补丁，树指纹
`a8088f650809d12f13ccb768e915b659b66a45aed1a56aad8eff642e2fbf178a`。
在 C3 UI 基底上仅加入已测的 CLI 最终提交重试及修订标记，实际源码与正反重放见
`output/framework-package-rename-r17-candidate01/`。
新夹具四为 `output/tabstrip-r17-go-fixture/attempt-04/`：959输入，EXE
`502e87c8408657643bc64c2976cbb4d3436458cd9962522eae64deb595ee0dcf`；
R5 五项全仓检查全部通过。新夹具双主题原生复跑退出0，原始 PNG 与候选三逐字节相等，
封存于 `output/tabstrip-r17-c4-native-seal/attempt-01/`，不沿用旧二进制身份。
四目标构建与项目校验器均通过；完整2911项源码前后无漂移，四种程序/终端库架构、
226个磁盘payload、225个ZIP entry及其权限/哈希经独立复核。
实际结果见 `output/mygo-rename-r17-c4-canonical-delivery-attempt01/`，独立审阅见
`output/mygo-rename-r17-c4-canonical-independent-review/attempt-01/`。

此前 MyGo `.7` C3：1,841 文件、12 补丁，树指纹
`512725b6283c673d4e95063df2744728d0603153110307c1bb76bf5bf4997ee9`。
`output/framework-point-r17-candidate03/` 保存实际源码、补丁、pin 校验及正反重放。
此前应用候选三为 `output/tabstrip-r17-go-fixture/attempt-03/`：959 输入，EXE
`2462e5303e64901d05cf4a7b44d4394b7e131db16f5be8cb1e65c005e39b7462`。
新增短文本叶节点逻辑宽度取整；换行、超宽文本和 RTL 窗口保留原路径。
四文件变更与已测候选逐字节一致，独立复核见
`output/tabstrip-r17-leaf-ceil-adoption-independent-review/attempt-01/`。
新的全仓普通/fixture 测试、Go vet、MyGo vet、lint 五门槛全部通过，原日志及
959项源码前后核验见 `output/tabstrip-r17-root/final-r4-checks.json`，不沿用候选二身份。

此前应用候选二为 `output/tabstrip-r17-go-fixture/attempt-02/`：957 输入，EXE
`2ab81bbb67aaa8c4795c89b3b512c19fb443362fd74c8c9a34f1f8f002a97809`，
包含边缘夹具判稳与字体修正。候选一的953输入、旧EXE和五门槛结果单独保留；
不追溯改写为当前候选的证据。
候选二的全仓普通/fixture测试、Go vet、MyGo vet、lint五门槛均通过，
原日志与源码前后核验见 `output/tabstrip-r17-root/final-r3-checks.json`。
候选二的36张原生基线在 `output/tabstrip-go-r17-2x-baseline-attempt08/`，与冻结原版
完整帧配对比较完成。该比较器另有71项防护和271项来源链独立复核，
见 `output/tabstrip-r17-c3-baseline-guard-independent-review/attempt-01/`。
这些是受控夹具与来源检查，SSIM仅作筛查，不表示整窗或交互验收。
候选三重新完成36张原生基线，位于
`output/tabstrip-go-r17-2x-baseline-attempt11/`，全部 PNG 与候选二逐字节相等。
这提供到已比较原图的传递证据，并不是再次运行原版或新一轮 Go parity 比较器。
959项源码、40张新原图及 R4 五门槛的联合封存见
`output/tabstrip-r17-leaf-ceil-native-seal/attempt-01/`；全部13次夹具试验（含4次失败）
的扩展清单见 `output/tabstrip-r17-native-fixture-ledger-r2/`。

## 源码契约和实现

冻结 Zeron 提交保持 `ab189159f10cf8e63c0502be75a59bbc487e0eaf`。
详情等待350ms，关闭按钮等待500ms；两者独立，状态跟随可视槽位，任务捕获旧文本、
旧 source bounds，实际显示时捕获鼠标位置。File 使用实际路径，Browser 使用当前 URL，
具体 PR 使用 typed Number/Title；无详情的标签和 PR 列表不制造标题详情。

`AttachAtPoint` 使用本轮自然尺寸和约束：超过右/下边界则分别翻到点左/上，负原点归零，
不提前把长提示压窄。详情卡最大380DIP、字体10.5DIP、padding6/9、radius6、border1，
无阴影；父容器移动和窗口变更会使点定位重新计算。

输入观察按需订阅，首个订阅之前没有历史；不插入会替换祖先路由的 `HandleInput`。
`Handle.OwnsPrimaryPress` 读取 committed identity，取消代次在事件入口、任何可能重入的
鼠标移动/回调之前采样。真实任意 Up、MyGo 的 Blur、已知无主键的 Move 和隐藏节点
能使该 API 的旧归属失效；它不是完整输入日志，也不宣称 GPUI 的通用冒泡等价。
Windows 客户区 Move 另保留本条消息的按钮事实；未知平台/入口保留明确的未知状态。
该事实通过内部负数 tag 传递原始五按钮位，仅按需观察者解码，不伪造 Up。
`Context.DeviceScale()` 读取本帧实际绘制比例，不查询默认显示器或保存 UI context。

应用捕获的 source 槽位 pending 与框架归属分开：原版没有“Blur 就清 pending”的规则。
原先以该假设编写的探索失败已撤回，不能计作修复证据。实际 unheld Move 的处理顺序为
先让 tooltip 看到旧 pending、取消旧任务，再清 pending；本次 Move 不立即重开350ms任务。
完整合并事件序列、prompt/drag、跨平台和纯 MouseExit 原生路径仍待验证。

## 原生观察

普通文件悬停：`output/tabstrip-go-r17-2x-file-hover-attempt01/` 两张原始 WGC 图，
进程正常退出。与 R16 独立原版图比较，两端提示外框均为 WGC坐标(444,182)、
136×62物理像素，即68×31DIP；底色和边框直线段的颜色一致。
文字区域仍有差异：原版可见 ink 顶部206、Go204物理像素。
后续冻结源码与无窗口 DirectWrite 控制证明 detached tooltip 使用系统 UI 字体，
本机 SPI 返回 Microsoft YaHei UI，而 Go 旧卡继承 Geist；控制准确复现原图 ink 范围。
候选二显式选择源字体，并按源公式吸附行高，没有文字坐标补偿。
新原生结果单独记录，不能以 CPU 控制覆盖旧图。完整原图保留，统计没有裁切、缩放、
遮罩或排除区域后宣称验收。
详见 `output/tabstrip-r17-native-file-review/`。
新候选普通提示双主题已正常退出并生成成功清单，在
`output/tabstrip-go-r17-2x-file-hover-attempt06/`；前一轮 `attempt05/`
因前后 pointer 从(220,61)变为(573,513.5)、hover与判稳改变而拒收，不作为通过图。
新原图中两端 ink support 均为(464,206,95,15)物理像素，但原版731个非底色
像素、Go750个，仍有同位置 alpha/颜色残差。原版短卡136×62、Go135×62物理像素，
详见 `output/tabstrip-r17-native-file-review-attempt02/`，不把相同 ink 范围称为完全相同。

候选三在 `output/tabstrip-go-r17-2x-file-hover-attempt09/` 完成新的双主题原生采集，
进程退出0。完整原图统计见 `output/tabstrip-r17-native-file-review-attempt03/`：
两端短卡均为(444,182,136,62)物理像素，修正了上一候选少1px的外框。
ink support 仍同为(464,206,95,15)，非底色像素仍是原版731、Go750；
共同文字区域暗色381、浅色260个像素不同，不能据此称文字渲染完全一致。

边缘候选一的两次试验保留在 `output/tabstrip-go-r17-2x-file-hover-edge-attempt02/`
和 `attempt03/`。后者保留暗色原图，长提示位于约(199,482.0213)、380×47.978714DIP，
但浅色未完成，不能当两主题成功。失败来自提示诊断的自然成熟被错误计入标签栏几何
判稳；`output/tabstrip-r17-edge-fixture/attempt-02/` 有先失败后通过的 CPU 证据。
修复后新二进制和 Go 原生复跑单独记录。新原版 R17 参考程序的两主题边缘原图已取得，
在 `output/tabstrip-reference-r17-2x-file-hover-edge-attempt02/`，独立审阅95项通过。
Go 新候选双主题也正常完成，在 `output/tabstrip-go-r17-2x-file-hover-edge-attempt07/`。
候选三的边缘场景再次完成双主题并退出0，保留于
`output/tabstrip-go-r17-2x-file-hover-edge-attempt10/`，完整统计在
`output/tabstrip-r17-native-file-review-attempt03/edge-result.json`。
Source 每主题60次实际样本，覆盖1005.6449/1001.743ms；实际最大采样间隔18.6098ms。
Rust 在 WGC 后重采 native activity/input，没有完整 post-WGC geometry snapshot。
1秒成熟停留不证明350ms的原生显示阈值。
候选二仍能直接观察到两端长路径断行不同；冻结 GPUI 会在加 padding/border 前
对 text leaf 逻辑宽度 ceil。候选三已修正可容纳单行叶节点的取整，长路径换行仍待修正。
独立源码审阅与原图统计继续定位，
不以卡片宽度常量、手写换行或截图变换拟合。
机制已独立确认：GPUI 先对完整未换行文本塑形，再自行选择 glyph 断点；
连字符、下划线和点算 word，单斜杠不创建候选。MyGo 正常 Windows 文本由
DirectWrite WRAP 返回 line metrics；不能将 WholeWord 或 Character 选项先当等价。
详情见 `output/tabstrip-r17-wrap-review/`。两端边缘卡框实际均为(400,1022,760,96)px，
两行 ink 的顶部也一致，但第一行文字宽708/640px、第二行205/273px，断行仍不同。

## 确切 Windows 程序与打包状态

本次已校验的 C4 Windows EXE 为
`c4e5b652ab1cf5c641ae6cdac302654566bda50be480ae0d6ac7ad1ce5251dce`。
它在 transcript mock 下实际完成 Files、README/layout.go打开/切换、详情与关闭提示、
原生菜单 Close tab，退出0且 stderr 空；13次状态、14份原始 Sky JPEG 和3200×2000
初始client PNG保存在 `output/resume-validation/tabstrip-r17-c4-native/`。
第一次保存器误用旧目录被 `wx` 写入保护拒绝；独立保存器随后记录全程，
旧C3证据所引用的原件已全部重新hash，字节未变。没有运行安装器或操作真实服务。
以下旧C3运行和四次失败保留各自源码/二进制身份。

已完成原生冒烟的 C3（字体修正、叶节点取整前）Windows EXE 为
`f12f4596674c6e9b45358929e6643b5ed75fc316a2b0311a14189e5f2e5c8e31`。
进程内 transcript mock 下完成 Files、README/layout.go 打开与切换、两种成熟提示、
菜单 Close tab，正常退出0且 stderr 空。13次 Sky state、14份原始 JPEG 和独立初始
client PNG 保留于 `output/resume-validation/tabstrip-r17-c3-native/`。
即时 UIA 有滞后，另留 settled 观察；工具提示的 AX name 仍是通用标签，
没有把它计为可访问性内容验收，也没有操作真实账号、provider或外部消息。

四次 canonical 四目标打包均失败：前三次在 Windows 包完成、darwin/amd64
编译完成后，CLI 最终目录 rename 返回 Access denied；第四次脱离原 PowerShell
包装和 resume-validation 父路径，Windows、两种 Mac 单架构输出已保留，
universal 两个切片也编译合并完成，但最终 universal staging rename 仍返回同类错误。
因此原包装脚本和父路径并不是该失败类别的必要条件，尚未识别占用方或证明同一根因。
第四次的实际日志、2,902项源码前后核验及169个部分产物核验见
`output/mygo-rename-r17-canonical-delivery-attempt01/`。
原 result 中 `windowsPayloadEqualsPriorNativeBoundHash=false` 是未执行检查的默认值，
不是已测出 hash 不同；`postfailure-result.json` 核实其仍与上述 Windows EXE 相等。
受控 Windows 子文件句柄可复现 error5，关闭自有句柄后恢复；这只证明可能机制。
有限重试随后通过独立审阅并作为第13补丁进入正式 C4 候选；该受控重试证据本身
不代表四目标验证。其后新 C4 四目标构建与 Windows 原生运行见上文，各自保留独立身份。
它只重试 build/dev 共用 `replacePath` 的最后 `src→dst`；最多10次、900ms计划等待，
系统调用和调度耗时另计。旧目标暂存 `.old` 的时长可能延长；rollback仍是既有
best-effort行为，不承诺其他进程同时阻止回滚时一定恢复。原失败占用方尚未识别。
真实自有句柄的先失败后通过、持续10次拒绝及受控旧目标恢复均已测；
普通 CLI suite 仍有 `TestLanguagePackaging` 失败，C3同样失败，独立符号链接探针返回1314。
后续 integration 精确排除 installer/signing/该既有用例后，其余98项顶层通过、11项自然跳过；
没有称普通 suite 全绿、修改系统权限、运行安装器或发布。完整包与独立审阅见
`output/mygo-rename-r17-bounded-retry-final/` 和
`output/mygo-rename-r17-tested-packet-independent-review/attempt-01/`。
R2封存前的13次夹具试验见 `output/tabstrip-r17-native-fixture-ledger-r2/`，失败图与成功清单分开。

## 实际设备和边界

新的原版诊断 EXE 完成36场景、72次截图前后查询，正常退出：Arc B580、
LUID `0x12a5a`、FL11.1、flags `0x20`、driver32.0.101.9033，RT/swapchain格式87、sample1。
独立复核见 `output/renderer-device-r17-native-review-attempt03/result.json`。
查询来自此新 GUI 的实际 renderer，不创建设备代替查询；不能追溯绑定旧 R16 EXE，
相同地址快照也不证明中间绝无设备替换。
Go/MyGo 新诊断也完成36场景、72次截图前后查询：同一 B580/LUID/driver，
实际 FL11.0、flags0x20，swapchain alphaMode0；原版为 FL11.1、alphaMode1。
Go 查询在 UI view 内执行，不能证明该查询就是 WGC 对应的 present producer。
启动自动激活由原始 foreground 数据支持，本轮短时窗口没有 root Sky 观察。
审阅见 `output/renderer-device-r17-mygo-native-review-attempt01/` 和
`output/renderer-device-r17-mygo-native-crosscheck-attempt02/`。
另建独立无窗口 R17 控制，严格各请求 FL11.1/11.0，设备实际 level/out-param/flags
及 context IUnknown 均交叉核实。两组54case与旧R16控制的完整 case 对象一致：
该受控 clear/shader/blend/format 组合没有因 feature level 改变输出，不能外推为
全部 GUI 绘制等价。debug DXBC 中3份 SPDB 变化，执行 token 与签名相同，
不宣称所有 shader blob 全同。详见 `output/native-blend-readback-r17-analysis-attempt01/`。

Win32 window rect 与 WGC 可见 frame 范围不同；PNG尺寸与 WGC frame 一致，
不能要求它等于完整 GetWindowRect，也不能据此推导绝对屏幕像素原点。

无窗口 COM 的单变量矩阵进一步核实：同phase ClearType154组原始纹理相同，
仅把Go的sixth/half-up位置策略改为源quarter提取策略后，44组shape与32组tie
case的bounds/bytes均与源参数模型相同；仅换mode无改善。支撑集合准确重现
原图731/750、交集728、源独有3、Go独有22；phase候选使集合恢复731并等于源。
996份raw及766组比较已独立复算。它证明受控coverage差异，不能外推透明/灰度/
所有字号等价，也不是实际GUI renderer的拦截。见
`output/tabstrip-r17-tooltip-phase-com-analysis-attempt01/`。

随后已完成唯一 quarter 隔离候选的真实 Windows 原生配对（run13），不是正式
C4树：vendor 1,859文件/`52f7b7746950047d30c2aa1b54856c4cf820131db3b8314b16edac418d435e96`，
app959项，实际EXE `a5023a6813e21b3b48cbb7f79b171163844bb8278e49fa232185a8eb59699a5e`。
两主题卡片均为136×62，文字非零像素集合均从750恢复为731并与原版集合完全相同。
共同文字ROI的不同像素暗268、浅89，最大RGB通道差均为1；原C4对应381/260、最大36/37。
全帧不同像素仍完整记录为22,410/39,185，卡片区域332/138，不能把文字支持集合相同
当作整体视觉验收。新原图、2,818项实际源码与备份、输入及独立复算见
`output/tabstrip-r17-quarter-native-analysis/attempt-01/` 和
`output/tabstrip-r17-quarter-native-independent-review/attempt-01/final-result.json`。
Root保留三次Sky观察及原JPEG；浅色成熟提示未额外观察成功，成熟WGC前后输入门槛
由原始sidecar证明，不声称每个边界都由Root连续见证。

该候选的默认六帧与C4逐字节相同，但counter另增1,536B、node/textStyle各增8B，
仍未达到框架未用功能不增长规则，未进入正式源码。dumpdep只有quarter名称的
共享arginfo元数据引用，不能据此称quarter实现体被默认程序保留。独立审阅另留一项
Windows/386测试条件与实现build tag范围不一致的P3；没有执行该平台测试。
现有Draw/content过渡入口不能直接替换标准文字绘制且保留全部语义，下一种接入
仍需实测成本，不能承诺零成本。源码审阅见
`output/tabstrip-r17-quarter-source-extension-review/attempt-01/`。

另一个app-only AX候选删除了覆盖实际内容的通用Label。实际run14的Windows UIA
分别读到`Tab02.txt`和`Close tab`，两张完整WGC PNG均与C4 run12逐字节相同，
进程正常退出0。该运行属于attempt-01的精确EXE，不能重标为后续测试修正版。
attempt-01独立审阅发现遗漏迁移一处`uifixture`旧Label查找；attempt-02已复现四场景
失败再修正为通过，保留可见裁剪与committed bounds的独立断言，生产字节没有变化。
历史AX接入的独立增量审阅未发现新问题，当时仅采用这7个app文件，框架仍为原C4；
quarter当时没有混入。采用记录见 `output/tabstrip-r17-ax-adoption/attempt-01/`，该阶段
R6五门槛均通过，961项夹具源码及2,913项完整构建输入分别封存。
新四目标包及独立分发验证通过，Windows EXE为
`ed8aa81bcbb4f8680e0c2e96846eb682c22a04a980ba7a9ea919eebdc6b6db7e`。
该新包实际完成Files、两文件打开/切换、两种真实AX名称、右键Close tab和正常退出0；
13次状态、15份原JPEG及3200×2000初始client PNG已独立核对。
见 `output/tabstrip-r17-c4-ax-canonical-delivery-attempt01/` 与
`output/resume-validation/tabstrip-r17-c4-ax-native/`，旧C4包仍属于修复前源码。
Mac实机、屏幕阅读器事件、完整视觉及发布接受没有因此通过。原候选、发现和原生证据分别见
`output/tabstrip-r17-tooltip-ax-name-candidate/`、
`output/tabstrip-r17-tooltip-ax-name-independent-review/` 与
`output/tabstrip-r17-ax-native-seal/attempt-01/`。

冻结 GPUI 有两处注释与代码不一致：`Window::gpu_specs` 称 Windows 返回 None，
实际 Windows 实现及本次查询可返回设备；`GpuSpecs` 说明仅写 Vulkan，Windows 实际用 D3D。
以实现和实测为准，冻结参考不作修改。
另有两处坐标/缓存注释与实现不符：GPUI text缓存注释说width匹配或都为None，
实际新width=None也可复用旧wrap缓存；MyGo公开 `Glyph.X` 注释说advance-left，
实际 `Shape` 复制含drawing offset的内部X。当前按实现分析，未为修注释改变行为。
GPUI wrapped cache漏max_lines是另一个实现风险，不能把它混称为注释差异或照抄。
准确源码及行号见 `output/tabstrip-r17-wrap-optin-source-audit/`。

## 历史 C2–V4 成本与未闭环边界

未用功能体积 C2 比`.6`多：counter-native 2,560B、hello 1,024B。
独立试验后 C3 改用原始按钮 tag，并加入本帧缩放 API；该 C3 实测增量为
counter-native 2,048B、hello 0。`output/framework-point-r17-size-candidate03/`
保存两例实际二进制、源码、命令和 dumpdep；不将该C3候选标为满足框架体积规则。
历史 C4另行实测仍为counter+2,048B、hello0，完整树前后均为a808…/1,847文件，
见 `output/framework-unused-c4-baseline-r17/`。
quarter第二版仅交换内部size/weight字段顺序，实际textStyle144B、node1784B，
恢复到C4大小；counter较C4仍增512B（较第一版减少1,024B），hello仍逐字节相同。
它未满足不增长要求，也未运行第二版GUI；第一版原图不重标为第二版。
独立字段偏移推导与实际封存分别见 `output/tabstrip-r17-quarter-padding-review/attempt-01/`
及 `output/tabstrip-r17-cleartype-quarter-candidate/attempt-02/final-result.json`。
第三版仅将未发布 setter 改为 checked 包级函数；实际类型区减少48B，但counter
文件大小没有下降，仍较C4多512B。第二、三版独立差异审阅未发现新增runtime问题，
也没有把第一版run13原图重标为它们的原生结果。见
`output/tabstrip-r17-quarter-v2-v3-independent-review/attempt-01/`。
现有PE的完整类型图将第二版544B类型区增量分解为：命名callback217B、内部enum142B、
System方法项58B、setter48B、新字段描述及名字84B、旧Equal值减少8B，以及未分类布局3B。
默认代码extent净增96B，不能因text raw section未跨边界而称代码没有增长。第四版
已隔离实测per-run同签名回调和uint8 cache载体：counter9,682,432B，与C4文件大小
相同但SHA不同；hello9,785,344B，与C4字节相同。类型数据仍增144B、PC数据增88B，
不是运行时零成本。风险回归、76组控制、默认六帧及三项vet通过，执行包独立审阅
未发现新增问题；C4对`.6`历史counter+2,048B仍未解决。第四版尚未正式采用，
归因和入口契约分别见
`output/tabstrip-r17-quarter-residual512-review/attempt-01/` 与
`output/tabstrip-r17-quarter-run-callback-review/attempt-01/`。
第四版执行及独立审阅见 `output/tabstrip-r17-cleartype-quarter-candidate/attempt-04/`
和 `output/tabstrip-r17-quarter-v4-executed-independent-review/attempt-01/`。

Root另将第四版1,863项vendor与当前AX961项app组成2,824输入隔离树，app只新增
一处checked leaf wrapper，没有恢复generic Label。组合的全量、tagged工作台/夹具
及vet通过；首次全量因漏拷贝30份raw SVG测试数据失败已保留，补齐原始数据再通过，
不能称首次全绿。该组合EXE `0a505683...` 实际run15两主题正常退出，实际UIA名称
均为`Tab02.txt`，完整PNG与旧quarter run13逐字节相同；run16的36个默认原生场景
均与历史C3+leaf run11完整PNG逐字节相同。这些是新组合自己的证据，不重标旧图，
也不是新正式发布包或完整原版验收。见
`output/tabstrip-r17-quarter-v4-ax-native-seal/attempt-01/`、
`output/tabstrip-r17-quarter-v4-ax-native-independent-review/attempt-01/` 及
`output/tabstrip-r17-quarter-v4-ax-baseline-seal/attempt-01/`。
截至历史R8快照的夹具总账为18次试验、166张PNG（含既有失败的部分原图）、14成功/4失败，
旧总账原字节保留，新总账完整沿用前16次试验对象；见
`output/tabstrip-r17-native-fixture-ledger-r5/`。

## 历史 R7 诊断、未采用路径和记录差异

剩余文字颜色已另行离线核对：partial coverage内有双向±1，不能统一加减字色。
实际执行的逐步f32参数模型中，浅色两条RGBA路径bits相同；暗色source RGB比Go
低1 ULP（每通道2^-24），这不是PNG的1个8bit等级，也不是实际GUI绑定words观测。
它不能解释全部两主题残差。61份prepared原件及其副本保留，独立审阅通过；见
`output/tabstrip-r17-text-color-cpu-model-attempt01/` 和
`output/tabstrip-r17-text-color-cpu-independent-review/attempt-01/`。
旧54个恒量blend控制没有测试glyph纹理/gamma或真正GPUI SRC1_COLOR文本blend，
适用范围仍按原记录保留。后续分阶段GPU leaf控制V1已完成离线编译/link，exit0，
EXE尚未启动；独立预审保留整份inputs未绑定可信身份、构建路径祖先reparse检查
两项阻断，不能运行该版来作GPU结论。V2只修这两项，已离线编译/link并通过
独立静态复审；合法输入校验和24项synthetic pilot已获执行授权，实际执行证据
须另行封存，不能从build-only报告推断。现有EXE固定读取build02/work，不能为
manifest负控改旧inputs；该负控须另建独立同源构建目录。原source-only阶段字节
不回写为built；两版都不是生产ABI重放。V2构建与复审分别见
`output/tabstrip-r17-text-leaf-gpu-build-attempt02/` 和
`output/tabstrip-r17-text-leaf-gpu-source02-independent-review/attempt-01/`。
V1构建与预审见 `output/tabstrip-r17-text-leaf-gpu-build-attempt01/` 和
`output/tabstrip-r17-text-leaf-gpu-independent-preflight/attempt-01/`。

隔离换行第二版完成实际DWrite 12组合同测试，但未通过两项接入门槛。
短LF文本自然宽度47.302494经卡片padding加减后成为47.3024902，少1 ULP，
strict > 将末glyph拆出第三行；三种scale均复现。原版整体ceil位于UI叶
测量返回层，不能改raw glyph/Line.Width或放宽比较来掩盖失败。未启用功能的
counter另增1,536B，hello逐字节相同；框架尚未采用，也未改旧回归expected。
独立LF审查见 `output/tabstrip-r17-wrap-lf-integration-review/attempt-01/`，
实际候选见 `output/tabstrip-r17-wrap-candidate/attempt-02/`。

提示卡扣除文字诊断ROI后，暗64、浅49个不同像素全部落在12×12像素角方区。
这只是坐标分类，透明角会露出底层背景，不能据此归因为半径或外圆角AA。
完整原图差异没有遮罩或消除，见
`output/tabstrip-r17-card-corner-residual-attempt01/final-result.json`。

冻结shader的“Hsla to linear RGBA”注释还存在术语歧义：本输入是sRGB字节经HSL
再逆变换，函数没有传递函数转换；若linear指linear-light，该用词与运算链不一致。
不能据此认定gamma或残差bug，也没有修改冻结参考。限定证据见CPU报告中的
`terminology-review.json`。
隔离checked-facade候选只减少512B，仍较`.6`多1,536B；未正式采用，不能计为门槛通过。
README、补丁页和交接页原先仍把`.6`/11补丁称为当前代码，现已区分 R16封存与R17候选。
补丁页还曾把 C3 指纹标为 C2，验收页的11补丁清单未明确限定为 R16，现已修正。
R2进度封存后的独立审阅又发现：补丁页的重建命令仍只列11步，遗漏第12个
tooltip-primitives 补丁；当前流程已补齐。R2及其文档字节快照保留当时状态，
不以当前修正回写旧封存或将其独立审阅称为没有发现。

原生计时、文字宽度和换行残差、体积规则、R17发布接受、玻璃、完整场景矩阵及
Mac Intel/ARM 实机仍待完成。交叉编译、CPU回归和单个 Windows 夹具不能替代这些验收。

## 历史 R8 增量

历史R8封存时仍保持正式C4 `.8`/13补丁与当时AX源码。第四版quarter的16次跨平台构建均成功，
counter在Darwin amd64/arm64实际增32/16B、Linux 0、Wasm减598B；四个hello均字节相同。
四次Wasm BuildInfo读取静默exit1，原runner整体exit1保留，不称全部检查成功。
Mac增量准确落在Mach-O重定位流/字符串表，不能用Windows raw0外推。第五版仅准备
三态cache mode，尚未正式采用；它必须有自己后续执行的测试、体积和原生身份。
见 `output/tabstrip-r17-quarter-v4-cross-unused-size/attempt-01/`、
`output/tabstrip-r17-quarter-v4-cross-unused-attribution/attempt-01/`。

换行第三版只让CR/LF详情回原native Text，旧三个LF expected在普通及tagged回归中
均通过；五条有限验证命令退出0，框架与第二版原字节一致，未重新测size。因此第二版
counter+1,536B仍是未采用门槛，完整source LF自然测量也未实现。见
`output/tabstrip-r17-wrap-candidate/attempt-03/execution/attempt-01/`。

GPU build02的实际runtime01合法验证通过，但pilot初始化depth描述符检查失败，0/24
cases、无Draw/readback；此前20次HLSL编译已经成功，并有12份X4000警告原blob。
第三版只改两个disabled depth请求字段为默认ALL/LESS并补请求日志，严格描述符校验
保留；build03及独审完成。后续runtime02是另一个绑定其精确EXE的24项单RTV计划，
实际执行结果须按它自己的新记录读取，不从build-only报告推断。既存P2是
IndependentBlend=false时RT1字段ignored，回读RT1 mask不能证明MRT1有效mask；
runtime01未到该guard，驱动是否误拒须看后续独立运行；full/MRT前须另修有效状态与双readback证据。
见 `output/tabstrip-r17-text-leaf-gpu-runtime-analysis-attempt01/` 和
`output/tabstrip-r17-text-leaf-gpu-build-attempt03/`。

F1 whole-input真实负控另用独立同源构建：先封源/EXE，再合法验证0，owned inputs
只翻一bit后验证1，stderr准确报告expected6945…和actual977b…，输出哨兵未创建。
变更保留不恢复，旧source02/build02/cache完整集合原字节不变。它不是旧build02 EXE
的负控。见 `output/tabstrip-r17-text-leaf-input-auth-negative/attempt-01/`。

新增两处注释差异：MyGo gpu.go称13个float4，实际Instance为14个、静态224B，
正式vendor未为改注释破坏pin；V4 RasterGlyph称全部unsupported先于cache lookup，
实际backend/AA/color在quarter placement miss后才查、失败在写入与glyph lookup前
回native。V5只纠正该注释并保持旧顺序，未把现有cache失效假设定性为新bug。
实际上传词的设计分别采GPUI HSLA+CB与MyGo RGBA Instance+CB；GPUI post-VS RGBA、
exact WGC producer仍notCaptured，source-only设计不算观察结果。见
`output/tabstrip-r17-text-actual-input-capture-design/attempt-01/`。

## 其余历史文档与记录差异（原件保留）

隔离wrap V1的`Layout.Width`注释称longest line，却未限定opt-in source measure；
该P3及V1原件保留，V2只修注释后另行执行，不能把V1审阅改成零finding。
R6旧文档曾把已bind compile/link的GPU01称source-only，后续R7只修live新快照；
旧R6文档与非零审阅仍保留。R7的`actualDWriteOrSizeGatePassed=false`又与独立
DWrite成功字段矛盾；R8移除OR字段，分列DWrite=true、size=false、总体=false，
旧R7 P3不回写。

GPU03 launcher旧key `build02ExistedAtCheck`实际检查build03是否存在，addendum
已限定其意义，不能由false推断旧build02不存在。旧label与记录保留，见
`output/tabstrip-r17-text-leaf-gpu-build-attempt03/legacy-label-addendum.json`。
上述差异与本次planned-LF/actual-CRLF预测P3均属于各自明确范围，不说明整个
历史证据失败，也不能被当前源码或新通过结果追溯抹除。
