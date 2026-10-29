# 原生弹层和输入样式核对

事实源为固定 Zeron `ab189159f10cf8e63c0502be75a59bbc487e0eaf`，及其
`zui` / GPUI `b564a34413bb7ef1da04121c4a287758f94993d9`。原版源码只读。
本阶段接续 footer/R6 的剩余差异，不把测试或组件截图当作完整视觉验收。

## 贴窗规则

`gpui/src/elements/anchored.rs::prepaint` 的 limits 是整个 viewport：先处理右/下
越界，再处理负的左/上坐标。`SnapToWindowWithMargin(8)` 只在实际越界时留8px；
距边缘0–7px但仍在窗口内的坐标保持原值。超大盒最终左/上对齐，仍可超出右/下。
这不是 SwitchAnchor，也不是无条件夹到8px内边距。

`popover.rs:784–817` 的 account/context 卡使用 BottomRight anchor，并在卡外
包6px bottom padding。MyGo 相应 gap 属于 margin box；不能只按可见border box
贴窗，否则靠窗口下沿时少算6px。此前应用通过center attachment加固定偏移补偿
MyGo的flip/4px规则，既不能处理真实auto height，也会错误移动未越界的近边坐标。

框架现在提供 opt-in `Element.SnapToWindow(margin)`，保留未启用的 AttachTo 行为。
定位使用本轮实际大小，在子树布局前求出最终 margin box 位置；没有读取旧panel
高度、持久Element或在Draw里修改布局。应用的model、checkout/ref、account/context
菜单接入该能力。PR的框架Tooltip定位与完整动画仍需单独核对，未随菜单一起宣布完成。

`pickers.rs:3840–3905` 还规定未测量时先用320px/above；compact card 除新聊天
下方至少180px的 preferBelow 情况外始终向上，standard才使用自适应翻转。
新 `composer_popover.go` 记录普通Rect/height，变化时请求后续帧；首次冷开与resize
不再依赖 MyGo 的自动翻转掩盖零bounds。已有冷开geometry/trigger wash回归通过。

Footer应用回归覆盖普通、0/3/4px未越界、1px越界、过窄/过矮、resize和重开。
`Tester.Find` 返回可见裁剪交集；不能把被裁掉的标签区域当成panel被压窄。
框架另用committed Handle bounds验证完整盒。初次误判的失败日志保留。
分数坐标下GPUI额外offset.round、所有DPI和原生窗口行为仍待对照；generic Snap
API没有擅自对全部MyGo坐标取整。

## 输入字段

模型搜索的 source ComposerInput 覆盖wrapper的13px，默认实际字号14。
原图与DirectWrite量测同时证实旧13px字宽约少8px。placeholder为Faint，非Muted。
框架新增paint-only `PlaceholderColor`，显式透明与unset分开，默认仍TextMuted；
应用只给source使用此颜色的输入设置Faint，不更改全局theme或绘制假placeholder。
列表其余明确差异与证据见 `audit-model-list-residual.md`。

`composer.rs:5295–5309` 的原生caret宽为2DIP。MyGo此前默认约1DIP，设备像素
宽为 `max(round(scale),1)`，不能误称所有DPI下固定1device pixel。新增opt-in
`CaretWidth(2)`保留native blink/编辑/IME流程；主侧共用surface和模型搜索接入。
它只改变paint；IME候选矩形和全部原生输入矩阵仍待验收。

`composer.rs:5640` 的外层Div声明ui_rems行高，但自定义输入renderer在4455–4456
实际直接取 `px(configured_line_height)`。main/side均用默认22.75，未调用唯一
override入口with_text_metrics。此前surface随UIFont缩放行高属于实施错误；现保留
字号缩放而固定22.75DIP，UIFont13/16/20的dock/hero四行高度回归通过。不得仅凭
外层style推断实际renderer的行距。

## 取证边界

原生fixture现在绑定其真实Window以选择实际display分支，不启动App.Attach订阅。
此前未绑定Window的fixture使用1x测试fallback；这不能证明产品2x文字分支。
新compact元数据记录nativeWindowAttached和oneXOptionText，实际2x必须为true/false。
旧1x证据保持原字节，不重标为2x。

原版新8图位于 `output/reference-lossless/capture-native-state-20261028-2x-01`，
使用已有冻结binary `cd460f145549aad6d6686b2a6b6b8008936d713cd676d4fff5aa9771152833ba`。
进程退出0，declared layout-captured，8份metadata/PNG hash已核对，全部actual2x；
wide完整WGC1684×1180、narrow724×1180，没有OS键鼠、裁剪、缩放或系统设置修改。
实现8图的原图/sidecar哈希、actual2x及真实Window分支均已核对；完整比较为
`output/compact-comparison-20261028-r7-2x/comparison.json`，2/8达到0.995，
0场视觉验收。新的28张footer实现与原版实际2x比较均达数值门槛，但大面积
空白会抬高整窗分数，且非客户区按钮颜色仍不同，active/DWM状态未独立锁定。
R6的1x通过数不可直接与R7的2x比较成受控改善。框架锁、九补丁精确重放、
256项源码冻结、测试与四目标检查包记录在
`reference/validation/popup-r7-result.json`；没有新的OS键鼠或Mac原生验收。

## R8 accent border-box 修正

上述R7证据保持历史身份。GPUI的accent inset以含border的外框为界，MyGo的CSS
absolute inset以padding box为界；直接沿用inset0留下外侧紫色线和内侧白线。
应用仅将modelAccentPlate无border overlay每边外扩1DIP，覆盖同一原版外框，
保留颜色、gradient、shadow及Slider真实行为。普通Box仍可能进入hit链，但无
handler/interactive标记，不截断祖先Slider；不得描述成完全不存在于命中列表。

1x/2x整数/分数框绘制与真实Slider四档点击、拖动、焦点/键盘测试通过，证据在
`output/model-picker-r8-accent/result.json`。R8八图严格完整窗口比较3/8达到门槛，
四个panel分数改善，四个list分数未变。此处不推断尚未改变的文字/阴影已修复。
当前完整阶段为 `reference/validation/popup-r8-result.json`。
