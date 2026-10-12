# 项目树与空间布局原生复核

日期：2026-10-12。用户要求左侧结构为“项目栏目 → 项目名称 → 多个对话”，并继续调整布局和空间细节。已实现并完成限定原生复核；视觉候选尚未获用户认可，首版仍未交付。

## 变化

- 移除独立全局会话列表。项目下面只出现属于该项目的对话，不重复项目名或目录路径。
- 项目使用 mygo 原生 disclosure，可独立展开/收起；快照刷新保留当前窗口内的展开状态，选择或新建对话展开所属项目。
- 项目行加号按被点击的项目新建对话。项目目录通过 tooltip 可查看。
- 1440 DIP 窗口的初始栏宽为 256 / 650 / 约 532 DIP。左栏项目行 32 DIP、子对话行 48 DIP、层级缩进 16 DIP；组间距 12 DIP、同组行间距 2 DIP。
- 标题栏高 48 DIP，改为单行“项目名 / 对话标题”。消息列表和输入区共用最大 748 DIP 阅读列，左右各 24 DIP，消息间距 16 DIP；窄窗右栏默认收起，侧栏最多 220 DIP。

## 证据与边界

[确认目录](../../output/acceptance/visual-v4/confirmed/) 保存 7 张实际原生窗口截图：深色 1440×900、1280×800、1024×700，浅色 1440×900、1024×700，项目折叠和重新展开。截图使用明确标记的示例数据，复用生产 Go + mygo 视图，未执行真实厂商 API。

- [暗色工作台](../../output/acceptance/visual-v4/confirmed/dark-1440x900-native.png)
- [窄窗](../../output/acceptance/visual-v4/confirmed/dark-1024x700-native.png)
- [项目收起](../../output/acceptance/visual-v4/confirmed/tree-collapsed-native.png)
- [源码、binary、PID 与控件记录](../../output/acceptance/visual-v4/confirmed/build-manifest.json)
- [截图 SHA](../../output/acceptance/visual-v4/confirmed/captures.json)
- [root 复核](../../output/acceptance/visual-v4/confirmed/review.json)

真实 AX 查询得到项目节点为 `AXDisclosureTriangle`；AXPress 收起 GCode 只隐藏其自身对话，Atlas 对话仍可见；重新展开恢复 GCode 对话；点击 Atlas 子对话后表头读取到 Atlas Notes 与对应标题。QA PID 14346，经 AXCloseButton 正常退出 0，root 已确认 PID 消失，截图 SHA 与采样源码均一致。

Tester 覆盖子对话在项目下的顺序和缩进、独立折叠、刷新后保持折叠、Enter 重新展开、跨项目切换 cwd、项目行加号的 NewSession ProjectID。原生样例不连接 service；新建任务派发由 service stub 证明。

首批 [round-1](../../output/acceptance/visual-v4/round-1/) 检查后只批量调整阅读列宽度，再完成本次确认批次。历史 visual-v3 已收到用户要求继续修改的反馈，不作为获认可的视觉基线。

本次不证明物理键盘、中文 IME、系统剪贴板、真实 API、持续流式性能或 Apple Silicon/Windows 实机。历史 round-18/19 仍属于旧 integrated 包，不能倒填为本次新包流式实测。

## 正式包

[project-tree-layout-confirmed](../../output/acceptance/current-package-regression/project-tree-layout-confirmed/) 的全仓测试、受影响模块 race、vet、fmt、package 和 codesign 六项均 exit 0。原生 QA binary 与正式包身份独立，采用相同当前桌面视图源码。新包详情见 [current-package.md](current-package.md)。
