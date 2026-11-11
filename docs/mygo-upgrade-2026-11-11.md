# MyGo v0.8.3 升级与组件收敛

2026-11-11：依赖从 `v0.7.4 + rcode-native-ui.23` 升至官方
`v0.8.3 + rcode-native-ui.24`。官方提交为
`5add3817c74bb7505d0b473bc5fb28abe2201b99`，本地补丁从 19 项缩至 16 项。
来源与升级前判断见 [上游审计](mygo-upstream-audit-2026-11-11.md)；
当前补丁哈希、源码树指纹和保留能力见 [固定补丁](mygo-patches.md)。

## 实际改动

- 删除补丁 07、14、16。05 只保留尚未由上游提供的契约说明和测试；
  其余补丁重放到官方源码，重新生成 API 包装、发布身份与源码锁。
- 用共享 `design.MarkdownCodeBlock` 按语言继承原生 `TabSize`，
  替代框架内的空格展开与显示列回源算法。Go/未知语言 4，JS/TS/JSON 2；
  源码、局部选择、整段复制与“复制代码”均保留原始 tab。
- 标准 Markdown 渲染的 HTML 混排、图片和脚注链接接入 `Options.LinkMenu`，
  与自建文件链接共用来源会话、远程宿主和导航菜单策略。
- 设备选择器改用受控 `PopoverMenuButton`，由框架负责菜单、关闭及焦点恢复。
- 公式与引用图标使用最终文字布局的 `DrawObjects`，删除第二次 shaping。
  引用图标按上游内联图标示例分配基线上下空间；word joiner 保持图标与引用首字同一行。
- 公式和引用段落共用对话选区，右键菜单使用 `ContextMenu`。
  复制时将公式对象映射回公式源文，并去除图标与 joiner，完整 Markdown 复制仍保留源文。
- 模型子菜单的锚点偏移使用 margin，与新版锚点定位语义一致。
  链接遵循上游键盘约定：Enter 激活，Space 滚动。

保留双列模型选择器、可移入并滚动的地址提示卡、表格导出/预览及数学渲染缓存。
`LinkIcon` 会给 GCode 引用增加链接交互，原生 `InlineMath` 在外层共享选区中的
源文复制也需要额外处理，因此没有为采用接口而改变这些行为。
本轮收益是补丁维护与重复排版减少；没有测量 CPU/GPU、内存或模型性能变化。

## 验证

框架从官方快照按 16 项补丁重新应用后，与固定源码逐文件、逐字节一致。
源码锁、框架锁测试、框架 UI/Markdown/RichText 测试及 Windows amd64 交叉编译通过，
命令与结果记录在 `output/mygo-upgrade-20261111/framework-verification.md`。

产品侧最终检查与原生证据在 `output/mygo-upgrade-20261111/`：

- 全部主模块包最终分组通过：`engine-tests.log`（完整核心，431.470 秒）、
  `desktop-full-final.log`（完整桌面包，117.922 秒）和 `other-packages-final.log`
  （其余所有包）。首次 `CGO_ENABLED=0 go test ./... -timeout 5m` 的核心包总时限不足，
  桌面与许可清单适配失败已修复；原始记录保留为 `full-tests.log`。
- `go-vet-final.log`、`mygo-vet-final.log` 与 `check-framework-final.log` 通过。
  普通桌面构建、Windows amd64 交叉构建及 Linux amd64 服务构建通过；
  `server-deps-final.log` 中没有 MyGo 或 automation 依赖。
- 原生 8 组用例通过：设备菜单、引用布局、模型选择器、正文/预览链接、表格、
  设置持久化、面板开合和拖拽关闭。证据为
  `native-final/20261111-122519-2869211955/evidence.json`，相关深浅主题截图已检查。
  模型选择器覆盖 2/12 模型目录，引用布局覆盖中英文和换行。

组件测试还覆盖原生 tab 的语言宽度与复制、HTML/图片/脚注菜单、
公式对象绘制、引用不可点击/不可聚焦、对象源文复制和共享控件边界。
过程中失败的日志保留，不将重跑成功覆盖为首次通过。

引用图标回归的升级前快照来自 `fe336ef`。同一像素位置测试在旧版的 8 个
中英文/字号/主题组合均失败，图标中心偏高约 5–8 DIP；修复后 8 个组合均通过，
图标与相邻文字墨迹中心差不超过 1.5 DIP。该测试验证具体对齐问题，
不代表全应用视觉一致性验收。

原有 `TestDesktopLocalPromptReferences` 的发送阶段在升级前快照的深浅主题均失败：
发送命令后，模型请求未包含测试预期的技能正文。该问题仍需单独修复，
本轮没有改动核心的命令/技能展开逻辑，也没有降低此用例的断言。
对照记录为 `baseline-native.log`；修复后的引用布局由独立的显示历史夹具验证，
不能把该布局用例称为命令/技能执行验收。

没有推送、安装或替换用户应用。原有未跟踪 `.mygo/`、`.tmp/`、`gcode` 保留。

Windows 只做交叉编译，未做该平台原生运行；Linux 服务验证不构成 Linux 桌面验收。
原生 automation 不证明真实 OS 输入法、远程宿主、真实供应商或完整视觉一致性。
