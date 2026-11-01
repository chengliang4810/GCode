# 工具折叠的运动与构建耗时调查

2026-10-30。只读比较 `D:/projects/RCode` 的
`e8399dc38ee669e60e8b87916a0bfd8f580270b6` 与当前 GCode 工具工作记录。
下文保留接入前调查和隔离诊断数据。用户随后批准了专项调整：
手动折叠/尾部跟随分离、可见行测量缓存和 140ms 原生折叠。
后续实现与验收记录见 [工具折叠调整](../tool-fold-motion.md)。

## 结论及证据强度

发现两个不同因素，不能将用户的“晕”直接等同于实际掉帧：

1. **已复现的二次位移**：停在会话底部、启用正常动效和 SmoothScrolling 时，
   200ms 折叠高度动画同时触发会话末尾的跟随弹簧。
   受控组件实验中，工具组下面的正文先向下偏移约 167.29 DIP 再返回，
   可测位移持续到约 783ms；收起时方向相反。详情开闭的正文往返约 62.93 DIP。
   仅将 `FollowAnimation` 设为 nil，仍保留高度折叠及 FollowEnd，
   正文往返变为 0 DIP，几何变化在约 200ms 内完成。
   这解释了可能的“整段被带着晃”的感觉，不需要发生慢帧也会出现。
2. **已测到的同步构建负担**：24 行虚拟视口建立前，
   `activityTextDetail` 会对全文每个展示行调用 `c.MeasureText`，计算水平滚动宽度。
   这不是仅 shape 可见 24 行。约 60KB / 10,000 短行的夹具连续五次打开，
   每次构建帧约 71–74ms，约 10,029 次文本 layout，并分配约 31MB。
   因此长输出另有真实卡顿风险；不能由此推断用户当时打开的内容同样长。

这些是 **Windows 上 MyGo ui.Tester 的组件/内存绘制** 数据，
不是当前用户窗口的 FPS、桌面合成器、显示器刷新或输入延迟数据。
未启动 RCode 做性能对照，也没有把两个不同渲染器的时长当成同机 A/B 性能结论。

## 当前 GCode

| 位置 | 实际处理 |
| --- | --- |
| `internal/desktop/transcript.go:73,136` | 工具组及每项都使用 `CollapsibleBase` |
| `third_party/mygo/ui/collapsible.go:38,64` | 200ms EaseOut；按进度改变外框裁剪高度 |
| `internal/desktop/chat.go:53-57` | FollowEnd 保持会话尾部；正常动效且 SmoothScrolling 开启时额外使用 `ui.Smooth` |
| `third_party/mygo/ui/motion.go:32-37` | Smooth 是默认 500ms、无 bounce 的弹簧；到精度阈值才停车 |
| `third_party/mygo/ui/list.go:1553-1616` | 内容高度持续变化时重定向跟随补偿；不仅响应新消息，也会响应折叠 |
| `internal/desktop/transcript_detail.go:83-99` | 缓存本次展开的行，但首次/重建时测量全部行宽，随后仅构建视口行 |
| `third_party/mygo/ui/runtime.go:821-846` | 未构建元素被 prune，其 ui.Local 资源也释放；收起后重新展开会重建详情缓存 |
| `third_party/mygo/internal/text/layout.go:680-686` | 文本布局缓存最多 4,096 条；10,000 独立短行超过此容量 |

已有的有效处理不应重复否定：

- 原生列表已有稳定 key、行高测量和滚动锚点。
- 减少动态效果可以直接到达折叠目标；动画结束后不会无休止重绘。
  现有 `TestTranscriptOfficialFoldHonorsReducedMotionAndParks` 本次重复三次通过。
- 用户滚动会停止列表跟随。问题是手动折叠与末尾跟随的组合策略，
  不是整个滚动系统都没有锚点。
- 正文/工具信息仍完整保留；优化不应通过截断数据、减少 24 行上限
  或丢弃横向内容实现。

## RCode 已核实的处理

| 处理 | 当前源码依据 |
| --- | --- |
| 手动工具组、思考、工具详情均为 140ms、`cubic-bezier(0,0,.58,1)` | `crates/ui/src/transcript.rs:116,8518-8540,8951-8979`；`motion.rs:357` |
| 高度裁剪；普通手动开闭没有额外 body 淡入/上移 | `transcript.rs:8841-8880` |
| 组箭头与高度共用 140ms 时钟；详情箭头直接换图标 | `transcript.rs:8600-8653,10245-10253` |
| 新流式行另用 360ms 入场、480ms 连接线及 4px/opacity；不是手动折叠参数 | `transcript.rs:117-120,8539-8630` |
| 回到历史会话清除入场 epochs 和折叠时钟，避免重挂载重播 | `transcript.rs:5213-5222` |
| 关闭动画期间保留内容，稳定收起后不构建隐藏内容 | `transcript.rs:8332,8851` |
| 思考列表只 shape 可见行；按 Unicode 展示宽度选代表行测宽 | `transcript.rs:9725-9790` |
| 已完成消息按 fingerprint 缓存；同数量行变化用 remeasure 保留高度 hint/锚点 | `transcript.rs:3352,5408-5420,5520` |
| 尾部跟随只有一个 after-layout spring driver；目标收缩时清除增长速度估计，停稳后停车 | `transcript.rs:303-324,4823,10836-10854` |
| 用户滚轮立即取消旧自动补偿，避免排队帧覆盖用户输入 | `transcript.rs:4109-4154` |
| reduced motion 直接到目标，不调度折叠后续帧；后台暂停只影响 UI 动效 | `crates/ui/src/motion.rs:1202`，折叠渲染分支 |

这里最有价值的是避免动画/滚动相互追逐、减少隐藏和不可见文本工作、
保留高度锚点、抑制重挂载重播；140ms 参数本身不能替代这些处理。

## 测量

环境：Windows amd64，Intel Core i5-14600KF（14 核 / 20 逻辑处理器），
Go 1.27.3。组件窗口 760×600，正常动效，9 个虚拟列表项，末项使用真实
`assistantTranscript` 与工具折叠组件；几何时钟按 60Hz 推进。
“60Hz”是实验采样时钟，不是测得的实际 FPS。

| 模式/操作 | 正文往返距离 DIP | 最后可测几何变化 |
| --- | ---: | ---: |
| 现有 Smooth + 工具组展开 | 167.29 | 783ms |
| 现有 Smooth + 详情展开 | 62.93 | 700ms |
| 保留高度动画，关闭跟随 glide + 工具组展开 | 0 | 200ms |
| 保留高度动画，关闭跟随 glide + 详情展开 | 0 | 183ms |
| 减少动效 | 点击处理时直接到目标 | 无后续运动 |

可测几何变化阈值为相邻采样差 0.05 DIP；Smooth 在 1,000ms 处仍可能请求帧，
但其剩余位移已很小。不能把 783ms 误写成弹簧精确停止时间。

数据来源：

- `output/fold-motion-investigation/positions.json`
- `output/fold-motion-investigation/build-cost-budget.json`
- `output/fold-motion-investigation/budget-frame-cost.log`
- `output/fold-motion-investigation/budget-cpu.prof`
- `output/fold-motion-investigation/budget-cpu-detail.txt`
- `output/fold-motion-investigation/parking.log`

构造输出全部在 64,000 字符以内，使用已完成工具展示块，没有执行工具或请求模型：

| 行数/字节 | 第一次点击耗时 | 后四次点击耗时 | 观察 |
| --- | ---: | ---: | --- |
| 80 / 2,480 | 5.32ms | 0.52–1.03ms | 短内容没有复现明显构建阻塞 |
| 2,000 / 62,000 | 31.95ms | 1.52–2.62ms | 原生字体布局缓存热后明显下降 |
| 10,000 / 60,000 | 71.54ms | 72.97–74.50ms | 超过文本布局缓存容量，反复展开仍重测 |

`MYGO_FRAME_STATS=16.7` 将慢帧分解到 build/layout/paint/present。
10,000 行帧的主要时间在 build；profile 中
`activityTextDetail` 的 `MeasureText` 调用行累计 1.03s，完整采样为 1.36s。
这是该夹具的归因证据，不是整个应用 CPU 占比。
另有较长行的 1MB 压力实验约 225–241ms；它超出上述字节预算，
不用于代表普通工具输出。

### 构建隔离与复现边界

调查时其他工作正在修改 hooks/plugins，完整工作区出现暂时编译不一致：
`ToolOutput.Data` 调用与类型不符，以及无关设置测试调用不存在的 `Tester.Render`。
只在 `output/fold-motion-investigation/overlay.json` 的 Go overlay 中隔离这两处，
没有修改它们的工作区文件。测试没有执行 hooks 或插件业务；
不能把诊断通过说成完整应用或这些模块通过。
之后插件模块继续修改，已成功编译的诊断二进制用于重复 profile。

诊断代码：`output/fold-motion-investigation/transcript_motion_probe_test.go`。
最后的受限字节夹具二进制 SHA-256：
`b81edd2150e902ae656917cd7365c506323650028a413a01d6d6d7fda3291483`。
构建和 profile 命令使用 `CGO_ENABLED=0`；
Windows 原生文本布局仍调用 DirectWrite，不代表字体工作都是纯 Go。

## 建议的修复次序

1. 将手动折叠与流式新增内容的滚动策略分开，防止高度动画持续重定向跟随弹簧；
   以点击位置和可读内容的锚点为验收对象，覆盖底部、离底、长详情、窄窗、快速反向点击。
   保留新消息正常跟随、用户滚动优先和减少动效。
2. 去掉打开详情前逐行原生 shape 的宽度扫描。
   可按 RCode 的方向做低成本宽度估计/代表行与可见行测量，并缓存与内容/字体相关的
   纯数据；不能保存 ui.Element/Context，也不能牺牲水平滚动或复制完整性。
3. 在上述问题修复后，将本类手动工具折叠统一为 140ms EaseOut，避免全局改变
   设置面板、弹窗和其他 Collapsible。不要为了跟参考而增加新的位移/淡入动效。
4. 用独立原生进程收集修复前后帧日志，再由用户在实际会话体验确认。
   本轮受控实验已经定位可处理因素，但没有替用户完成主观舒适度验收。

## 注释与代码的偏差

- RCode `FoldState.epoch` 注释仍称 200ms，实际工具折叠为 140ms。
- `FOLD_TWEEN_WINDOW` 注释引用 RESIZE 的 200ms；
  窗口常量实际是 400ms，用于内容保留/防重播，不是可见折叠时长。
- `RCODE_MOTION_SCALE` 注释称影响所有 catalog timeline，
  但工具折叠手工计算 elapsed/TOOL_FOLD.total，未乘 speed_scale；
  该变量不能用来减慢当前工具/思考折叠。
- GCode `transcript_thought.go:20` 仍写固定 18px，现行详情行高为字体大小×1.5：
  默认 14px 思考文本为 21px，工具代码默认 12px 为 18px。
  因而同为 24 行，思考视口的像素高度也不能直接按 RCode 的固定 18px 推断。

RCode 的 `docs/research/mugen-pretext.md` 是研究对象说明，
其中 200ms 与缓存设计不应当作当前 RCode 工具折叠规范。
GCode `audit-transcript.md` 的 TR-40 已明确保留 140ms/200ms 差异，
本轮未发现它将两者声称为一致。
