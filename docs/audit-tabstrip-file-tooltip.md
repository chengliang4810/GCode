# 文件图标、详情提示与 GPU 混合读回 — R16

本阶段修复非选中文件图片的 `.78` 透明度，以及所有标签无条件使用标题提示的问题。
File 使用真实路径，Browser 使用当前已导航 URL；SideChat、Diff、Terminal、Subagent
等无原版详情的标签不生成标题提示。PR 详情仍待对齐，不能把 PR 列表标题当详情。
框架保持 `zeron-native-ui.6`，没有修改全局混合、颜色或 Tooltip API。

冻结原版 `shell.rs` 的 `right_surface_rows` 为 SideChat 等返回 `None`，仅在
`detail=Some` 时挂载详情提示。因此 R15 文档将提示差异仅保留为“停留未匹配”并不充分；
当前已由源码确认语义缺口并修复。R15 原件与原图保持历史身份。

文件图片 `.78` 与通用图标 `.7` 是不同规则。新增测试先复现四种主题/比例的文件
透明度失败，再验证 A→B→A 选择后活动图片为1、非活动为.78；几何、脏状态和关闭不变。
详情测试涵盖真实文件路径、实际 Browser URL、地址草稿、缺失文件及无详情的标签。
全量测试、tagged fixture 测试、Go vet、MyGo vet、lint 五门槛均通过。

## 计时与实际原生证据

独立 style 合同增加 `file-hover`，不改原36场景合同。两侧新增可选停留参数，默认0，
仅 hover 状态接受0–10秒；本轮请求1秒。计时起点是收到输入并提交悬停/几何后的
owner UI 线程样本，保留单调时间、实际时长和约16ms观测。原版每次观测核对 Win32
前台/owner、GetCursorPos、消息取出、GPUI位置和几何；内部 chip hover 仍为未知。
Go 使用真实输入观察与生产 chip Hovered，不能称为 GetCursorPos。Go 可容许同位置
重复消息导致的短暂稳定帧失效，结束必须重新 Ready；两侧检查范围没有假装完全相同。
轮询不能排除两个样本之间离开又返回，1秒成熟状态也不能证明350ms阈值。

本轮原版两次各2张完整 WGC PNG成功；Go首次在16ms观测到指针由220/61变成
213/43.5而拒绝，未生成PNG或成功manifest。该次通用错误文本写有“PNG retained”，
实际文件清单为零，以实际清单为准。Go后续两次各2张成功；实际鼠标由父agent在
已观察位置放置，工具历史说明与窗口消息证据分开，不伪称其为独立OS光标测量。

完整原图保留普通侧聊没有标题提示、文件有实际路径提示。文件提示的位置、字体、
背景等仍与原版不同。原版详情350ms、show-time指针+(1,1)及本轮自然尺寸翻边，
MyGo默认600ms、warm/keyboard行为和锚点上方定位；原版close提示另为500ms。
公开API不能以旧Bounds或不精确的测字代替本轮真实布局尺寸，本轮没有接入半成品
Tooltip helper。延迟、位置、翻边、样式、真实PR详情及完整视觉均保持待完成。

八张style原图的成功停留实际为1.0079848–1.0171901秒。完整PNG坐标(354,180)的
非活动文件图标内部点，dark两侧均为 `[164,172,181]`，light均为 `[129,141,159]`；
完整14DIP图标框仍有边缘/背景差异，单点一致不等于图标整体接受。

## 无窗口 GPU 控制实验

独立 Rust/D3D11 项目只使用已有锁定依赖，未启动窗口或 SwapChain，也没有改产品。
同一 Intel Arc B580、LUID `0x12a5a`、FL11_1，driver `32.0.101.9033` 的54组读回
覆盖3种shader编译参数、2组受控浅/深端点、3种混合方式和BGRA8/float RT，并非
两个完整生产主题场景。精确源字节、锁定
依赖图、完整 `ID3D11BlendState::GetDesc`、二进制与原始readback均封存。

浅色先清为RGBA `[243,243,245,255]`，BGRA8 straight输出 `[222,222,223,255]`，
premultiplied单源和dual-source均为 `[222,222,224,255]`。三种编译参数结果相同；
浅色RGBA32_FLOAT三种混合输出逐bit相同，未混合PS数值也实际读回。这支持本设备上
该浅色端点的定点RT/输出合并差异，不能归因于dual-source独有缺陷。深色float另有
1 ULP差异，未忽略。两个实际UI进程的adapter/feature level尚未绑定到此实验，不能
直接宣布全部截图根因已闭环，也不能据此调一个截图的蓝通道。

封存入口为 `output/native-blend-readback-r16-run-attempt02/seal.json`。
下一步先绑定实际应用适配器，再评估显式且可选的straight-alpha绘制路径；Tooltip
需要按本轮实际布局尺寸定位的可选框架能力。二者均须通用回归和独立原生对照。

## 构建与验收边界

阶段封存300项源码，相对R15为11项改变、13项新增、无删除；构建另绑定951项
应用/嵌入输入和52项发布资源。四目标包已构建并校验输入无漂移；Mac仅交叉构建，
没有Mac原生验收，安装器未执行。默认36场景、确切发布EXE交互和独立审阅的最终
结果见 `reference/validation/tabstrip-r16-result.json`。完整目标仍为active。

R16默认36张新Go原图已与保留的R14原版36张通过严格输入/捕获前后门禁配对，
SSIM仍仅作筛查；旧69项比较器防护未重跑，不改称R16测试。确切Windows发布
EXE `f5e4159f…5e0e`（DPI192）实际完成Files、README/layout打开与切换、文件详情
悬停、菜单Close tab和正常退出0，stderr为空。Computer Use返回JPEG原字节、UIA、
独立WGC及单独的工具历史说明均分开保存；即时UIA/关闭枚举的滞后另有settled观察。
