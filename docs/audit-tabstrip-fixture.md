# 原版标签栏组件对照 — R14

R14 建立冻结原版生产组件与 Go 生产 `State.TabStrip` 的独立原生对照。
本阶段先保留 R13 绘图基线，不把已从源码发现的颜色、图标透明度和脏标记尺寸
差异提前修掉。此为R14历史记录；R15已实施这三项修正，详见 `audit-tabstrip-style.md`。
完整视觉一致、系统输入和跨平台验收仍未完成。

共同参数位于 `reference/fixtures/tabstrip-contract.json`：840×560DIP 客户区，
默认原生窗口、空标题、系统窗口外观；组件宿主在客户区(40,40)，高38DIP，
宽400/220DIP；dark/light，0/2/6 个空闲子会话，首个活动，滚动到各自实测最大
范围的0/0.5/1。共36场景。两框架均请求窗口位置40/40DIP，但实际窗口边界的
定义不同；保留实测屏幕原点，完整 PNG 不做注册或移动。

原版使用冻结的 `Shell::render_right_tab_strip`、原始 chip/plus/fade 和原始
展开按钮 helper。外壳沿用生产标题栏的已核对边距，属于声明过的组件测试宿主，
不是整套 Shell 标题栏。数据通过真实 SideChat 入口建立，config=None、无引擎、
无工作中消息，不调用外部服务。可变构建导出仅在 Shell 末尾追加 feature guard，
实际生产方法体不变；9份生产源码和旧 fixture 的保护记录在独立构建 provenance。
原有 Cargo.toml/lock 的 capture 依赖调整继续保留，因此不把导出 stamp 当作整树
等同冻结仓库的证明。

Go fixture 通过 typed 进程内服务建立同一组 parent/side 会话，调用实际 TabStrip。
观测入口只绑定持久 Handle；普通构建使用空实现，不保存 Element/Context。
几何来自已提交布局，稳定计数区分同一 engine frame 的多次构建；先完成选择，再
设置实际最大范围的请求比例，停止主动滚入。观测钩子的离屏绘图不变性另有检查。

实测几何与请求数据分别核验。原版空 strip 的 ScrollHandle 实测为零宽零高；
Go 的非零空 viewport 保留为产品布局差异。chip/viewport/maxX/字体/颜色不通过
改 metadata 或手工补矩形消差。首个原生 Go 候选在等待合成指针状态时退出1，
stderr 为 context deadline exceeded；该次无可用截图，记录保留在
`output/tabstrip-r14-go-run-01`，不计作视觉结果。02/03同样未通过指针门槛；
04明确因窗口不在实际前台而拒绝发送消息。05复用04冻结二进制，在启动时仅激活
一次窗口，布局提交后再发自有子窗口消息，36场景通过并退出0。原生前后台查询与
消息属于fixture证据，不把这些失败标成生产绘图缺陷。

`scripts/compare-tabstrip-fixtures.ps1` 在评分前核验36个完整场景、实际 DPI、
已独立观察的 Windows 非客户区尺寸、PNG 签名/尺寸/SHA、manifest/sidecar 身份、
父子会话、活动项、滚动比例、稳定布局、合成指针及原生窗口身份。完整原字节先
复制到新输出并核对 SHA，parity 只读取这些快照；构建期间原输入发生变化不能
悄悄改变评分图像。无裁剪、缩放、遮罩、JPEG 中转或实现图替换原版。

整窗空白会主导 SSIM；0.995只用于筛查。达到该数值不表示标签栏一致，仍须看
实际控件像素和布局。fixture 的合成窗口消息与 GPUI dispatch 均不是实际 OS 输入，
两帧布局稳定也不等于 GPU 呈现完成的独立硬件证明。

初轮两侧36张实际2× DPI无损捕获均退出0，51项guard全部通过。原始整窗SSIM为
0.9996599881029895至0.9997935357654425；独立审计重算与工具一致，未据此接受。
`output/tabstrip-comparison-r14-baseline-01`保存原字节快照与当时脚本，后续升级
live脚本不把这51项回归移作新版本结果。原始坐标范围的像素计数另存
`output/tabstrip-r14-pixel-diagnostics`，只读完整PNG，不生成局部图或通过阈值。
例如dark/400/6/start的活动底色主色，原版为#232323，Go为#262626；标题栏区域
也存在大量差异，不能用整窗空白掩盖。

初轮发现原版 `window.is_window_active()`只是GPUI缓存，Win32激活状态未经实测；
固定Windows `Platform::activate`还是空实现。原版标题按钮灰而Go为黑，先保留
为原生条件差异，不能直接认定是哪套绘图出错。第二轮已补捕获前后原始
foreground HWND/owner PID；原版另核实owner UI线程/线程active。Go的所有NativeHandle
和原生状态读取也在UI线程，捕获后另存nativeAfter。

最终使用reference02二进制与Go attempt05二进制，分别由reference run02和Go run07
捕获36张图并退出0；原版stderr仅含声明的启动激活等待提示。Go run06保留为
指针末坐标不符的失败，active/messagesSent均真，不能归为失焦。run07比run06
增加2秒最小呈现间隔，随后仍逐项检查实际状态；不是同启动参数重试，也不以
等待时间替代就绪证明。所有旧图、exe、分数与失败目录保留。

`output/tabstrip-comparison-r14-baseline-02`与69项guard绑定当前08c2f4e4比较器。
包含严格bool/owner线程类型、路径alias/ancestor、图像变化与输入漂移的正负例。
五门槛全部通过。280项源码相对R13有17项新增、仅两个既有工作台文件改变，
均为观测绑定/空实现；R13生产绘图未改，框架`.6`完整树未变。R14没有新发布包。

第二轮整窗SSIM为0.9998630230588416至0.9999965707212943；独立重算吻合。
两轮仪器/活动条件不同，分数变化不能写成产品改进。最终原版标题按钮也为黑色，
但caption仍有20,772个像素、最多1级通道差异；实际屏幕原点不同，DWM位置/渐变/
栅格原因尚未确定。宿主区域仍有112至14,642个像素不同，底色和inactive glyph差异
明确保留。当前只覆盖无悬停空闲SideChat，未覆盖dirty文件dot、玻璃和完整原生输入。
阶段封存在 `reference/validation/tabstrip-r14-result.json`，整体目标仍active。

旧资料还存在一项精度表述不符：`output/tabstrip-r11-audit/result.json:100`把
按钮wash终点称为“exact settled”。现有`ui.Color`的RGB与alpha均为字节，不能据此
宣称等同原版浮点终点。旧报告原件保留；`output/tabstrip-r15-style-preflight`
已核对量化路径；该候选在R14封存时尚未实施。R15已实施并做CPU/原版原生对照，
仍有浅色1级和边缘等差异，不能把浮点输入视为最终像素精确一致。
