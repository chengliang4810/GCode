# 模型列表剩余差异审计

2026-10-28。本组件审计支线从 R6 四张原始 model-list WGC 配对图定位局部差异，再修改生产搜索、头部和列表绘制。该支线未启动原生窗口、重建可执行文件或修改框架；框架新增输入/贴窗能力与主线新采集另见 `audit-native-popup-style.md`。参考源码、旧截图与R6指标保持不变。机器证据见 `reference/validation/model-list-residual.json`。

## 原图定位

`output/model-list-residual-audit/analyze_pixels.py` 只读完整 PNG，独立重算原有 8×8 luminance SSIM，与比较器结果误差小于 1e-10。语义区域按原网格 tile 的中心归属，仅用于解释损失，不裁剪、缩放、配准或改变验收分数。

| 场景 | R6 全图 SSIM |
| --- | ---: |
| dark wide | 0.9935963171 |
| dark narrow | 0.9852484213 |
| light wide | 0.9932884938 |
| light narrow | 0.9845484851 |

搜索行占四图 SSIM 损失约 47–49%，列表正文约 28%，头部约 9–10%。非客户区虽有大量不同像素，所占 SSIM 损失仅约 0.07–0.26%；不能据此把列表问题归因于标题栏或修改全局主题。窄图相同控件误差被背景稀释得更少，分数更低。

以下是原始 wide 图的平坦区域采样值（完整窗口坐标在 JSON）：

| 区域 | dark 原版 / R6 | light 原版 / R6 |
| --- | --- | --- |
| popup 本体 | (39,39,42) / 相同 | (255,255,255) / 相同 |
| viewed provider tab | (56,56,59) / (54,54,57) | (235,235,235) / (239,239,239) |
| 未选列表底色 | (43,43,46) / (39,39,42) | (250,250,250) / (255,255,255) |
| 当前模型填充 | (64,64,67) / (61,61,63) | (237,237,237) / (242,242,242) |
| 搜索分隔线 | (56,56,59) / (53,53,55) | (227,227,227) / (236,236,236) |

## 冻结源码与修复

事实源固定为 Zeron `ab189159f10cf8e63c0502be75a59bbc487e0eaf`。

- `pickers.rs:1091–1094` 创建普通 `ComposerInput`，未使用 `with_text_metrics`。`composer.rs:110–111,2227–2235,5572–5575,5639` 使搜索文字实际为随 UI rem 缩放的 14px、空占位为 faint；外层 `pickers.rs:5222` 的 13px 被内部样式覆盖。保留原生 `TextInputBase` 的绑定、焦点、输入、过滤及上下键/Enter 路径，修正字号与占位颜色。
- `composer.rs:5640` 虽声明 rem 行高，实际自定义 `ComposerTextElement::request_layout`（5166–5195）调用 `layout_text`；后者在 4455–4456 读取字体大小后直接令 `line_height = px(configured_line_height)`，4720–4738 据此测量高度。因此搜索保留绝对 22.75 DIP，不能把这一行机械改为 `UISize(22.75)`。
- `composer.rs:5295–5308` 的 caret 为 2 DIP。使用新公开 `CaretWidth(2)`，保留框架原生 blink 和 IME 路径；不额外绘制 caret。
- `pickers.rs:5201–5228` 搜索图标为 14px/Muted、分隔线为 `hairline(.08)`；`theme.rs:1692–1718` 的 light hairline alpha 再乘 1.35。改用同一局部色值。
- `compact.rs:414–550` 头部按钮 32px、图标 14px、圆角 7px，back 使用 Muted、viewed tab 使用纯 ink(.08)。保留原生按钮，限定 compact 头部助手，不改通用 iconButton。
- `pickers.rs:5304–5312` 的 list host 是 ink(.02)，行保持透明；`compact.rs:219–225` 的 selected wash 在该底色上合成，其他 active 行为 ink(.05)。修复列表底色、selected/active 合成，保留原生虚拟 List。
- `compact.rs:238–241` 的 provider 图标使用 14px/Muted，品牌 tint 仍由既有图标助手保留。
- `theme.rs:1842–1852` 的 selected inset ring 沿原始 7px 圆角边界向内绘制。MyGo `Painter.Stroke` 自身在边界内绘制，移除额外的 0.5px inset，并使 List wrapper 同为 7px 圆角。

`PlaceholderColor` 与 `CaretWidth` 是本轮框架 owner 提供的 opt-in API；模型调用不修改全局 Theme。原有 List 作用域 Theme 的保存/恢复方式保留，未向应用状态添加 Element 或 Context。

## 验证与边界

新增 `model_picker_list_visual_test.go` 覆盖 dark/light 实际 popup 盒、source 平坦色、选中 ring、Faint 占位、2 DIP caret 绘制，以及原生输入过滤后 Enter 选择。Windows 1x 下，排除 caret 列的占位 ink 与原版都为 client `(44,319)–(147,328)`；13px 旧字号会使右侧 ink 提前约 8px。

自定义 UI 字号 13/16/20 的测试按生产映射 `theme.FontSize=13*UIFontSize/16`，检查搜索行/原生 caret 高度仍为 22.75，并通过输入 `MMMM` 后的实际 caret 位移验证 14px 字体的 rem 缩放。这里没有把 MyGo theme 基准字号误当作 source 的 UI rem 设置值。

最终检查：

```powershell
go test ./internal/desktop -run 'Test(Model|Compact|SideCompact|SideModel)' -count=1
go test -tags uifixture ./internal/desktop -run 'Test(ModelPickerFixture|Model|Compact|SideCompact|SideModel)' -count=1
go vet ./internal/desktop
go tool mygo vet ./internal/desktop
```

分别通过 7.644s、7.746s，两个 vet 均退出 0。独立只读复核也未发现本轮局部改动的实质缺陷。中途新 `SnapToWindow` 冷开定位回归由主 agent 修复，未放宽既有 geometry/trigger 断言。

Headless 输出仅用于局部回归，不能代替原生 WGC 验收。旧 R6 四张列表图均未达到 .995；随后R7/R8已在真实2x统一构建采集，最新阶段为 `reference/validation/popup-r8-result.json`，不能用旧1x图重标DPI。R8四张列表分数与R7相同，仅dark wide达到.995，仍为0场视觉验收。搜索 SVG 仍观察到约 1px 纵向 ink 差异，favorite SVG 约 1px 横纵差异，属于待进一步原生布局/栅格证据的问题，未猜测平移。caret blink 时相仍保留原生行为；新宽度 API 不改变 IME candidate 的矩形宽度，此范围未做实际 IME 验收。

R9c进一步通过独立原版Inspector adapter量测实际2x、UIFont13/16/20：两个整数
row原点349/383的文字wrapper分别高16/19.5/24.5DIP，默认UIFont16的inner为
`(43,355,47,19.5)`、`(43,389,87,19.5)`。三次exit0，旧panic/exit101保留，
见 `output/model-picker-2x-text-audit/r9c/actual-bounds-evidence.json`。
默认字号的绝对边界snap可以解释R7正文1物理px偏差，但UIFont13是统一上移
0.5DIP的反例。glyph baseline仍由wrapper/shape/source公式推算，尚未直接导出；
分数祖先位置、fallback、attribution未测，因此尚未据此修改生产排版。

后续R9无窗口实验在18组实际MyGo原生Text中确认：仅设phi行高与普通Text的整列
像素相同，没有补上绝对边界snap。当前公开Bounds来自已提交帧，Draw只允许绘制，
因此不能用上一帧Handle或Draw写布局修补。局部框架opt-in设计尚未实施，见
`output/model-picker-2x-text-audit/r9-design/result.json` 和 `review-addendum.json`。
原版先snap layout bounds、再分别snap累计element offset，不能把最终总Y一次
snap当作等价；字体metrics下限亦须保留。分数scroll原版实测仍待补，预测公式与
自生成诊断图均不作为视觉验收。
