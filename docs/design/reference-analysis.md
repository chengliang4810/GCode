# T3 与 Droppy 视觉参考调查

核验日期：2026-10-12。只读研究参考源码；GCode 保持 Go + mygo。

## 视觉权威

用户提供的 [T3 暗黑截图](references/t3-dark-user-reference.png) 是最低视觉质量标准。截图的三栏约为 15% / 45% / 40%，以正文和 Diff 为视觉主体，助手回复没有外层卡片，选中会话采用中性灰，输入区统一为底部浮动容器。尺寸和原生窗口按钮按实际 DIP 与可用宽度适配。

## 固定源码

| 项目 | 本次核验 SHA | 当前调查位置 |
|---|---|---|
| T3 Code | `f23d0084b6e023ffb1af9e332d8666affc99d542` | `/private/tmp/gcode-t3-reference-20261012` |
| Droppy Code | `180c89d483a22ad6c87b4053e71c2c2a2171cb04` | `/private/tmp/gcode-droppy-reference-20261012` |

临时 checkout 用于调查，不是构建依赖。GCode 不依赖这些本机绝对路径。

## T3 已核对规则

[index.css 固定源码](https://github.com/pingdotgg/t3code/blob/f23d0084b6e023ffb1af9e332d8666affc99d542/apps/web/src/index.css) 将控件圆角、侧栏 inset、浮动内容 inset 和 composer 阴影集中定义。字体是平台 UI sans 和标准代码等宽字体。默认暗色主题使用中性近黑背景与较浅的文字，边界和次级区域只作细微层级区分。GCode 将这些规则转化为 Go 语义颜色与共享组件，不引入 CSS 或 WebView。

## Droppy 已核对规则

- [Chrome.swift](https://gitlab.com/droppyformac1/droppy-code/-/blob/180c89d483a22ad6c87b4053e71c2c2a2171cb04/DroppyCode/UI/Chrome/Chrome.swift)：32pt 圆形控制、28pt capsule 内容高度，hover 约主文字 6%、selected 约 12%，系统动态文字颜色与细边线。
- [SidebarView.swift](https://gitlab.com/droppyformac1/droppy-code/-/blob/180c89d483a22ad6c87b4053e71c2c2a2171cb04/DroppyCode/App/Views/Sidebar/SidebarView.swift)：搜索与列表分区明确，详细会话行 48pt，标题约 13pt，主要动作按需出现。
- [TimelineRows.swift](https://gitlab.com/droppyformac1/droppy-code/-/blob/180c89d483a22ad6c87b4053e71c2c2a2171cb04/DroppyCode/App/Views/Chat/TimelineRows.swift)：助手正文直接排版，用户消息采用轻量容器；连续内容块约 20pt 垂直节奏。
- [ComposerView.swift](https://gitlab.com/droppyformac1/droppy-code/-/blob/180c89d483a22ad6c87b4053e71c2c2a2171cb04/DroppyCode/App/Views/Composer/ComposerView.swift)：输入和尾部操作构成单一表面，圆角 22pt，输入系统字体 14pt；空草稿发送禁用，运行中有停止。
- [DiffInspector.swift](https://gitlab.com/droppyformac1/droppy-code/-/blob/180c89d483a22ad6c87b4053e71c2c2a2171cb04/DroppyCode/App/Views/Diff/DiffInspector.swift)：文件层次、增删统计、独立旧/新行号、超过 200 行按需展开；原生 popover 是它的产品选择，GCode 保留已确认的右侧面板和窄窗收起行为。

Droppy 当前 [LICENSE](https://gitlab.com/droppyformac1/droppy-code/-/blob/180c89d483a22ad6c87b4053e71c2c2a2171cb04/LICENSE) 为 AGPLv3 及附加条款，历史 MIT 信息已经过时。本次借鉴视觉组织和交互约定，没有复制其实现或品牌资产。

## 实施约束

不搬入订阅登录、CLI Agent 控制、Hydra 或参考产品的其他功能。原生标准控件、实际事件和清楚数据归属优先；精致程度通过当前 GCode 的原生截图和交互验收确认，不能由参考仓库研究本身证明。
