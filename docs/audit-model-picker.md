# 模型选择器逐项审查

事实源：Zeron v0.2.128 / `ab189159f10cf8e63c0502be75a59bbc487e0eaf`。
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
| M22 | 原版 scrollbar、fast mode、完整动效 | 待继续核对 |
| M23 | 同状态 GPUI 截图、系统输入与监视器 DPI | 待原生验收 |
| M24 | 独立复核发现：空模型发送暗退回 Claude | 已修复；统一恢复 preferred provider 的有效模型，未解析时保留输入，retry 后恢复；使用真实持久化场景回归 |
| M25 | 独立复核发现：跨会话遗留 provider filter 与运行锁冲突 | 已修复；切会话关闭旧菜单，重新打开按当前 provider 定位；Claude→运行中 Codex 回归通过 |
| M26 | 全部 provider 禁用、新聊天和已有聊天区别 | 状态/显示已修复；新聊天显示 no agents，已有聊天保留已提交 provider；后端 toggle/retry 更新 catalog 状态 |

回归集中在 `internal/desktop/model_picker_test.go`。本轮修正前浅深色共 6 张，
修正后正常菜单与四种目录状态共 14 张，保存于
`output/model-audit/before`、`output/model-audit/after`。
manifest 为 `reference/validation/model-picker-audit.json`。
这些是实现图，没有替换 reference golden，也没有用裁剪/缩放/遮罩获取 SSIM。
