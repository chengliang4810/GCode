# Windows 文字测量审计（2026-10-28）

已确认的主要字宽差异来自 DirectWrite 的 Typography 默认行为，不是字号、字体文件、字重或 variable axes。父 agent 据此增加 Windows 专属 `design.ApplyReferenceTypography`，通过公开的 `FontFeatures("kern=0")` 匹配冻结原版默认 Typography；本审计没有修改产品或 vendor 源码，也没有启动原生窗口。R5 原生 WGC 和整图验收由主 agent 单独记录。

机器可读来源、命令及 23 项产物 SHA256 见 [windows-typography.json](../reference/validation/windows-typography.json)。测量脚本和完整数据保留在 `output/font-metrics-audit/`，不作为参考 golden。

## 来源与已排除因素

原版固定为 `ab189159f10cf8e63c0502be75a59bbc487e0eaf`；其 Cargo 固定 GPUI/ZUI 为 `b564a34413bb7ef1da04121c4a287758f94993d9`。本次只读核对当前 Cargo 缓存中的该 revision，源文件路径与完整哈希记在 JSON。

- `ui/src/popover.rs:910–919` 的 `menu_row` 使用 `ui_rems(13)`；`typography.rs:113–122` 默认 rem 为 16，因此实际字号为 13px。Go fixture 的 theme 为 13，`design.UISize(c, 13)` 同样为 13px。
- 16 个 bundled TTF 与冻结参考逐个 SHA256 相同。它们是独立静态字重，没有 `fvar` 轴；Geist 各字重的 units-per-em 为 1000，ascent/descent/gap 为 1005/295/0。完整 name、weight 和 metrics 见 `bundled-font-metadata.json`。
- 字体旧 family name 与 typographic family name 确有区别，但不能据此推断选错字体。无窗口 COM 测量覆盖旧 `FontCollection.GetFirstMatchingFont` 与现代 `FontSet.GetMatchingFonts`：72 对结果的实际 face 字重、glyph IDs、advances、offsets 和 line metrics 全部相同。400/500/600 都实选正确字重，排除了该假说。

## Typography 实测

GPUI `gpui_windows/src/direct_write.rs:446–452` 总是创建 Typography 对象，`:567` 即使 features 为空也调用 `SetTypography`。MyGo `internal/text/dwrite_windows.go:918–920` 在 features 为空时直接返回，保留 DirectWrite 未设置 Typography 的默认行为。

`directwrite_probe.py` 不创建 HWND，沿相同 bundled font 注册路径读取实际 DirectWrite 布局和 DrawGlyphRun 回调，共保存 144 组测量。13px、500 字重的结果如下：

| 文本 | 未 SetTypography | 空 Typography | 显式 kern=0 |
| --- | ---: | ---: | ---: |
| Context window | 98.943 | 101.907 | 101.907 |
| Standard | 55.835 | 57.135 | 57.135 |

这几组的 baseline 始终为 13.065，line height 为 16.9。单独切换 `SetPairKerning` 不改变上述结果，不能用该 property 的值排除 OpenType 默认特性的差异。

另以 `directwrite_features_probe.py` 完成 1,560 次布局，覆盖 Geist、Geist Mono、Segoe UI、Consolas，400/500/600、正常/斜体，以及 13 种词句和组合 marks。312 组空 Typography 与 `kern=0` 的完整 glyph IDs、advances、offsets、line metrics、layout metrics 全部相同，callback errors 为 0。

该设置也影响默认连字，并非只调整字距。例如 Geist 13px/400 的 `office` 在未设置 Typography 时为 5 个字形、32.721px；空 Typography 和 `kern=0` 都为 6 个字形、35.282px。显式开启 `liga/calt` 的对照确实产生差异，说明这些样本有检出能力。

此结论仅支持已测字体和默认特性的 Windows 映射。GPUI `direct_write.rs:1763–1814` 在非空 Features 时另外补入 `liga/clig/calt=1`；当前 helper 不能被解释为任意非空 Features 的通用映射，也没有建立 macOS 或所有文字脚本的等价性。

## 隔离 production popup 验证

用 Go `-overlay` 替换测试输入文件，挂载现有 production `modelTrigger/modelPopover`，只在该隔离测试 root 增加 feature 或 line-height；磁盘上的产品及原测试文件保持不变。三个实验分别为原 MyGo 默认、`kern=0`、`kern=0` 加 21px line-height，每个实验导出 dark/light compact/standard 四张 headless 图。

`popup-comparison.json` 保存原生参考及实验图片的哈希、局部 ROI 和 ink boxes。此处只是诊断探针，不改变或替代整张 WGC 的严格分数。以 dark compact 的客户端坐标为例，边界均为包含端点：

| 文字 | 独立原版 WGC ink box | MyGo 默认 | kern=0 |
| --- | --- | --- | --- |
| High | 22,371–50,383 | 22,371–49,383 | 22,371–50,383 |
| Context window | 22,467–122,475 | 22,468–118,476 | 22,468–121,476 |
| Standard | 172,467–226,475 | 174,468–227,476 | 173,468–227,476 |

`kern=0` 解释并缩小了主要字宽差异。单独改为 21px line-height 没有改变 option 行的上述纵向边界，因此不能把 1px 差异直接归因于行高值。

剩余位置有具体的源码线索：GPUI `elements/text.rs:765` 对 intrinsic text width 取 ceil；57.135px 的 value 会占 58px，足以使其尾端对齐的位置比 Go 的分数宽度向左约 0.865px。GPUI 默认行高为 phi，经 pixel snap 得 21px，随后 `taffy.rs:350–381` 对绝对文字边界作 half-toward-zero rounding。26px 行中 21px 文字的原始 top 为 461.5，源码算法得到 461；结合相同 font metrics，预测最终 baseline 为 476。MyGo 保留分数 top，预测 baseline 为 477，与当前 ink 残差一致。

这里的绝对文字 bounds 是按冻结布局算法推导，尚未从 GPUI 原生控件单独导出。它支持后续核对 intrinsic ceil 和绝对边界 snapping，不支持直接加全局 `Top(-1)`。没有实施字号、baseline 或 renderer 位移补偿。

## 复跑与范围

诊断 Go 文件最初放在 output 目录顶层，被 `go test ./...` 扫描；已原字节移至 `_source/`，移动前验证绝对源/目标均在当前 checkout，移动后 SHA256 相同。`go list ./...` 实际成功，12 个产品包，未包含诊断目录。Go 通配会忽略 `_source`，显式文件运行和 overlay 仍可复跑。

```powershell
python output/font-metrics-audit/directwrite_probe.py
python output/font-metrics-audit/directwrite_features_probe.py
go run output/font-metrics-audit/_source/mygo-measure.go

$env:ZERON_MODEL_FONT_FEATURES = 'kern=0'
$env:ZERON_MODEL_FONT_LINE_HEIGHT = ''
$env:ZERON_MODEL_SURFACE_EVIDENCE = 'D:/projects/GCode/output/font-metrics-audit/popup-kern0'
go test -overlay output/font-metrics-audit/overlay.json ./internal/desktop -run TestModelPopupSourceGeometryAndMaterial -count=1
go list ./...
```

默认实验需清空 `ZERON_MODEL_FONT_FEATURES`；21px 实验另设 `ZERON_MODEL_FONT_LINE_HEIGHT=21`，各写入独立目录。72 组 MyGo 测量、144 组 COM 测量和 1,560 次特性探针都是 Windows DirectWrite 的可复跑证据，不能代替原生截图、键盘/辅助功能、其他平台或真实 provider 验收。

## 后续：公开 inspector 导出的实际文字子盒

[text-bounds.json](../reference/validation/text-bounds.json) 记录另一个独立诊断 example 的来源、命令、二进制和运行哈希。该 example 引用既有导出的 UI 库，只新增 test-only example；Pickers、Compact、Popover、typography 四个组件文件逐个哈希与只读冻结 clone 相同，没有修改生产 renderer。它不是未改原版截图，也没有生成供 parity 比较的新截图。

公开 API `App::register_inspector_element<DivInspectorState>` 与 `Inspector::render_inspector_states` 可以观察当前 inspector 选中 Div 的真实 `bounds` 和直接子元素 `content_size`。由原有 `model-setting` ID 的祖先路径和 `div()` 的 source location 识别 label/value，鼠标坐标仅用于选择目标，不参与边界输出。Inspector 占用 30rem，因此本次诊断 viewport 为 1320×560、实际 rem=16，保留原 fixture 的 840px 根内容宽。

2026-10-28 运行 PID 21336，正常自退出 0 并释放 native slot。实际 scale=1，所得数据如下：

| 生产元素 | 实际 bounds（x,y,w,h） | 直接文字子盒 content size |
| --- | --- | --- |
| option row，`popover.rs:911` | 13,459,246,26 | 246,26（所有直接子元素合计） |
| label wrapper，`pickers.rs:6190` | 21,461,140,21 | 140,21 |
| value wrapper，`pickers.rs:6197` | 171,461,58,21 | 58,21 |

同一原生 GPUI `shape_line` 公共 API 测得 Standard advance=57.135002、Context window advance=101.906998、ascent=13.065、descent=3.835、pixel-snapped phi line height=21。实际 value 宽 58 和 wrapper top=461，直接证实此前 intrinsic ceil 与绝对边界取整的预测。label 的 140px 是 flex 分配后的宽，不能写成其自然字宽。文字子盒也不等于墨迹 bbox。

边界仍须明确：现有 SharedString 没有 ElementId/source location，其 TextLayout 属于 frame 的 RequestLayoutState；没有公开 API 能按祖先 ID 直接取出它。`StyledText::layout()` 只适用于调用方自己构造并持有的文字，`VisualTestContext::debug_bounds` 需要原组件预先设置 debug_selector。故原生 wrapper top 已从推导升级为实测，最终 glyph baseline=476 仍是由实测 top 和 line metrics 计算，尚未直接导出。Inspector picking 会截断普通鼠标业务处理，不能用于证明 hover 外观。后续产品适配须按实际行位置、字号与 DPI 做回归，不能把本次 13px/scale1 的结果直接变成全局位移。

## R6：按已测边界实现局部 option 文字布局

[text-bounds-implementation-r6.json](../reference/validation/text-bounds-implementation-r6.json) 单独记录本阶段源码、命令、原生进程、二进制、headless 产物和严格 WGC 比较哈希；前述 R5 与原生 inspector 的产物保留。此轮仅修改 `model_picker.go` 的 option 文字和专属测试，没有改全局主题、vendor、原版组件或 renderer。

采用的源规则分别是 GPUI `style.rs::TextStyle::default` 的 phi（`geometry.rs:3711` 为 1.618034）、`elements/text.rs:636` 的行高 pixel snap、`:765` 的 intrinsic width ceil，以及 `taffy.rs:350–381` 的绝对边界 rounding。`window.rs:2642` 经 `util.rs:130` 使用 half-toward-zero。Windows 1x 下，当行起点为整 DIP、居中文字的绝对起点非负时，行内 `floor((rowHeight-snappedLineHeight)/2)` 与该绝对规则等价；它不是固定 `Top(-1)`。13px 时得到 21px 行盒和 compact 行内 top=2；字体大于行高时可以为负 inset，例如 20px 得 32px 行盒、26px 行内 top=-3，仍保留原来的 26px 行高。

两个原生 `ui.Text` 使用 `FixedLineHeight`、明确行盒高度和 `AlignSelf(Start)`；行本身仍 `AlignItems(Center)`，所以 12px chevron 不上移。value 用当前主题字体、实际 UI 字号、继承的 500 字重及 Windows root 的 `kern=0` 测量，再取 ceil 并上限 100；label 继续 Grow 到剩余空间。原生 ButtonBase/Menu、typed choice ID 和侧聊 session ownership 保留。headless 实际默认矩形与原版 inspector 的 label `(21,461,140,21)`、value `(171,461,58,21)` 相同。

适配 gate 为 Windows 且已 Attach 窗口所属 display 的 `ScaleFactor==1`；非 Windows、非 1x 继续原布局。MyGo Context 没有本帧 DPI 或布局后回调，`LayoutBounds` 是上帧值，因此这里没有用其反馈，也没有保存 Element/Context 或在 Draw 写状态。未 Attach 的测试/fixture 仅在 1x 验证；这不建立任意 DPI 的结论。小数滚动、分数 parent transform 和负的 offscreen 文字起点仍在取整等价域之外。原版 `.truncate()` 对任意字形 overhang 的硬裁切也尚未逐项核验，当前保留 MyGo 原生 SingleLine ellipsis。

新增回归覆盖实际 option 字号 12–20px、26/30px 两种行高，每组连续移动到五个正整数坐标并回移，包含大字行盒超过 compact 行高的情况；文字布局不依赖额外 geometry redraw。侧聊实际标签得到 140px flex remainder，选择 `1M tokens` 仍保存 `1m`，长值 cap=100 后 label remainder=98，主会话模型不变。Go focused suite（7.744s）、`go vet ./internal/desktop`、`go tool mygo vet ./internal/desktop` 均过，另一次独立只读 review 和新 tests 重跑也通过。

直接 `go build -tags uifixture` 没有生成 syso，产物 `output/text-bounds-audit/implementation-r6/model-picker-fixture.exe` 的 SHA256 为 `c1c3539724a6696159fb1cb5f247c90f8fe2c0e2ee6bd9a301e527ee5f48682f`。该二进制也包含主 agent 当时新增的 footer fixture CLI 路径，不能按 R5 binary hash 解释。2026-10-28 原生 PID 9060 采完 8 张 WGC 图并正常退出 0，随后释放 native slot。每张 metadata 均为 dpi=96、scale=1，使用既有 R4 bridge。

`output/compact-comparison-20261028-r6/comparison.json` 对同一独立原版完整窗口 PNG 比较，不裁切、不重采样、不遮罩。SSIM 如下：

| 场景 | R5 | R6 | ≥ .995 |
| --- | ---: | ---: | --- |
| panel dark wide | .995682 | .997981 | 是 |
| panel dark narrow | .990054 | .995353 | 是 |
| panel light wide | .995022 | .997439 | 是 |
| panel light narrow | .988543 | .994112 | 否 |
| list dark wide | .993596 | .993596 | 否 |
| list dark narrow | .985248 | .985248 | 否 |
| list light wide | .993288 | .993288 | 否 |
| list light narrow | .984548 | .984548 | 否 |

只有 3/8 达阈值，脚本 exit 1 来自阈值失败，metadata/传输 guards 均已通过。dark wide 的独立局部 ink 观测采用完整图片坐标：value 两端均为 `(173,498)–(227,506)`；label 在阈值 80 时两端均为 `(23,498)–(123,506)`，阈值 120 时原版右端仍 123、实现为 122，说明仍有细小抗锯齿强度差。该 ROI 仅诊断，不替代或改变完整窗口分数。标题栏色差、list 与其余像素差异仍待独立定位，本轮没有据此修改全局主题或移动字号/baseline。
