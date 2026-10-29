# Footer 原版文字与盒子测量

[footer-text-bounds.json](../reference/validation/footer-text-bounds.json) 保存原始记录、命令、二进制和源码哈希。本次只新增 ignored 诊断 examples，直接引用既有 `FooterFixture` 和生产 `AccountUsage/context_usage`；未修改 Go 产品、原版组件、renderer、现有 footer/compact harness 或原图。9 个生产文件与只读冻结 clone 哈希相同，既有 Cargo manifests/lock 和两份 compact fixtures 也与前一次 footer build 的 provenance 相同。

通过公开 `App::register_inspector_element<DivInspectorState>`、`Inspector::render_inspector_states` 读取选中 Div 的实际 bounds、content size、显式 style refinement。祖先业务 ID、source location 和 phase 共同标识元素。原组件 SharedString 没有公开的 TextLayout/字形 baseline 全局查询，因此以下盒子为实测，baseline 另列为推导。`base_style` 只表示本节点的显式设置，不冒充完整继承后的样式。

诊断请求窗口 `WindowBounds::Windowed` 的位置明确为 `(40,40)`、大小 `1320×560`，未指定 display ID，没有使用默认 center。Inspector 占 30rem，实测 rem=16，原生产 root 实际为 `840×560`。本次 PID 1852 正常自退出 0，释放 native slot；**实际 scale=2**，不能与旧 R2 的 dpi96 原生截图强行配对。它没有生成截图。

随后无窗口、未 activate 的 GPUI display probe（PID 3904，自退出 0）实测仅一屏：primary `DisplayId(281612635)`，logical `2048×1280`、work area `2048×1232`，公开 Debug 显示 physical `4096×2560`、scale=2。没有可选的 1x 屏。未改 OS 显示设置或任何旧 metadata。

## Account card 实际 DIP 边界

| 元素 | 实际 x,y,w,h | 字体 / 行高来源 |
| --- | --- | --- |
| card | 359,385,400,121 | `popover.rs:330` |
| heading | 364,390,390,26 | 10px / 500，phi pixel snap 为 16；`popover.rs:970` |
| account row 0 | 364,418,390,49 | `popover.rs:911` |
| name 0 | 372,424,174,20 | 12.5px / 500，phi snap 为 20；`account_usage.rs:366` |
| metadata line | 372,445,174,16 | 12px / leading16，margin-top1；`settings/widgets.rs:600` |
| Pro | 372,445,20,16 | 继承 metadata 字体；`account_usage.rs:333` |
| dot | 400,445,3,16 | 继承 metadata 字体；`settings/widgets.rs:617` |
| In use | 411,445,34,16 | 继承 metadata 字体；`account_usage.rs:340` |
| account row 1 | 364,469,390,32 | 无 metadata 行 |
| name 1 | 372,475,364,20 | 12.5px / 500，phi snap 为 20 |

card 高度分解为 border+padding10、heading26、两处 gap 共4、row0=49、row1=32，总计121。第一行文字栈是 name20 + meta margin1 + meta16，再加上下 padding12；第二行只有 name20 + padding12。空 metadata 没有额外行或 margin。name 的 174/364px 是 flex 分配宽度，不能写成实际自然字宽。

原生 `shape_line` 测得 name0/1 的自然 advance 为 114.737503 / 96.112488，ceil 为115 /97。meta 的 Pro/dot/In use 分别为19.223999/2.412000/33.084000，ceil 后为20/3/34；两个 gap_x=8，总 content width=73，与 metadata line 的实际 content size `(73,16)` 相符。

heading 使用 `tracked_upper` 的 U+200A hair spaces。该 tracked string 的实际 shape_line ascent 为10.791016、descent2.95；本探针没有导出每个 shaped run 的字体，不能把这组数据直接替换成假定的纯 Geist ascent。

## Context card 与 chip

| 场景 | card 实际 x,y,w,h | details 实际 x,y,w,h | details 文字 content size |
| --- | --- | --- | --- |
| known | 644,424,172,82 | 649,457,162,44 | 146,38 |
| zero | 640,424,176,82 | 645,457,166,44 | 150,38 |
| over | 637,424,179,82 | 642,457,169,44 | 153,38 |

heading 都为26高；details 显式字体12px、leading19，两行38px，另含 bottom padding6。最长文字行的自然 advance 分别为145.595993、149.568008、152.028015，ceil 为146、150、153；加 details 横向 padding16 和 card border+padding10，得到 card 宽172、176、179。

| chip label | 原生 advance | ceil 后 label width | 实际 chip width | 实际 cluster width |
| --- | ---: | ---: | ---: | ---: |
| account 90% | 22.638000 | 23 | 56 | — |
| known context 21% | 19.855000 | 20 | 53 | 113 |
| zero context 0% | 16.115000 | 17 | 50 | 110 |
| over context 120% | 27.147999 | 28 | 61 | 121 |

chip 显式字体11px，phi snap 行高18，ring16、gap5、左右 padding共12，所以实际 chip width 减33，与独立 `shape_line` 的 ceil 结果相同。这是从实际 chip border box 与源码 flow geometry 推出的 label 分配宽，未冒充 SharedString 直接 bounds。open chip 带 deferred popover 子元素，`DivInspectorState.content_size` 会受该子元素影响，不能拿它当 label 宽；closed known 的 account/context content size 为44×18 /41×18，分别等于 ring16+gap5+label23/20。

实际字形 baseline 尚未直接导出。JSON 单独标注按实测文字子盒、匹配的原生 ascent/descent/leading 和 source pixel snap 推导的 baseline，例如本次2x的 name0 为438.5、meta 为457.5 DIP；这些值不能当作旧1x截图的实测 baseline，也不构成完整像素验收。

## 复跑

```powershell
$env:CARGO_TARGET_DIR = 'D:/projects/GCode/output/reference-native/target'
cargo build --offline --locked -j 2 --manifest-path output/reference-native-0.2.128/Cargo.toml -p zeron-ui --example footer-bounds-audit --features compact-picker-fixture
# 原生运行前协调 slot；使用新的输出目录，保留 run-01。
output/reference-native/target/debug/examples/footer-bounds-audit.exe output/footer-text-bounds-audit/run-new

cargo build --offline --locked -j 2 --manifest-path output/reference-native-0.2.128/Cargo.toml -p zeron-ui --example footer-display-probe --features compact-picker-fixture
output/reference-native/target/debug/examples/footer-display-probe.exe output/footer-text-bounds-audit/gpui-displays-new.json
```

规范诊断源位于 `output/footer-text-bounds-audit/_source/`；导出的两个 examples 为相同字节副本。现有 manifest 未为新 example 增加条目，复跑时必须显式保留 `compact-picker-fixture` feature。原生 business click 经既有 fixture 的 public `click_position` 和公开 mouse down/up dispatch，点击时先关闭 inspector，避免其 picking 截断业务事件。该路径仍不代表 OS 鼠标、激活或真实外部账户验收。
