# T3 参考下的原生工作台候选复核

日期：2026-10-12。结论：这是历史候选。用户已反馈布局和空间细节仍需调整，且左侧必须为“项目栏目 → 项目名称 → 多个对话”的树形结构；本候选未获视觉认可。后续实现与原生证据见 [visual-v4.md](visual-v4.md)。不能据此宣布达到 T3 交付标准或完整首版完成。

身份补充：本文九张截图记录采样时的原生样例源码与独立 QA binary。其后合入的 Markdown cache generation 和后台生命周期修复不由这些旧截图证明；当前正式包的构建、回归和 round-18/19 真实 Service 流式记录见 [包与回归身份](current-package.md)。工作台配色和布局仍以本候选接受评价，用户视觉认可保持 pending。

## 实际变化

- composer 主输入面只保留文本、轻量模型选择、独立停止和发送；纠正/队列、完全访问和实际分支位于下方辅助行。
- 模型选择使用 mygo `SelectBase`，无静止边框，弹层使用低反差表面；唯一 provider ID 负责选项绑定，可读名称只负责显示。同名、同模型服务的第二个选项已有真实 Tester 回归。
- 会话表头保留标题和 cwd；右栏使用单层文件/Diff/终端 Tabs 与真实 Git 统计，移除重复标题和分支。
- 变更计数在 raw 更新时解析并缓存，保留文件折叠、长行换行、短 Diff 自然高度和大 Diff 虚拟化。
- G 品牌采用柔和 accentSoft 表面，新会话加号采用中性按钮。

## 原生证据

最终目录为 [visual-v3/confirmed](../../output/acceptance/visual-v3/confirmed/)，使用明确标注的示例数据和当前生产 Go + mygo 视图。它是独立原生 QA 程序，不冒充正式包的真实编码任务。

root 已逐张打开九张实际窗口截图：深色 1440×900、1280×800、1024×700，浅色 1440×900、1024×700，设置深浅两张，模型弹层和中文/emoji 输入两张。输入区、分支辅助行、Tabs、Diff 汇总和系统窗口按钮在记录尺寸下可见；1024 DIP 默认收起右侧资源栏，正文宽度保持可用。

- [暗色工作台](../../output/acceptance/visual-v3/confirmed/dark-1440x900-native.png)
- [窄窗](../../output/acceptance/visual-v3/confirmed/dark-1024x700-native.png)
- [模型弹层](../../output/acceptance/visual-v3/confirmed/model-popup-native.png)
- [原生控件记录](../../output/acceptance/visual-v3/confirmed/control-checks.json)
- [构建身份](../../output/acceptance/visual-v3/confirmed/build-manifest.json)
- [截图 SHA 与 root 复核](../../output/acceptance/visual-v3/confirmed/review.json)

实际 AX 操作证明弹层可打开并通过 Escape 关闭，中文/emoji 多行输入逐字读回，空白输入禁用发送、有内容启用发送。鼠标 AXPress 打开后 Escape 的实际焦点为任务输入 TextArea，未宣称焦点回到选择器。样例未执行服务派发；模型选择派发和运行配置隔离由服务 stub 与 Tester 证明。

QA PID 2281，通过真实 AXCloseButton 关闭，父驱动 exit 0；root 核对 PID 已消失、九张 capture SHA 和 view 源码一致。首批 `round-1` 保留；随后只批量修复同名 provider 值绑定，再完成一次确认批次，没有追加主观打磨轮次。

## 正式包与完成边界

[visual-refinement-confirmed](../../output/acceptance/current-package-regression/visual-refinement-confirmed/) 全仓 test、desktop race、vet、fmt、package、codesign 均 exit 0。正式包身份见 [current-package.md](current-package.md)。原生候选与正式包采用相同当前 view 源码，但二者 binary 身份独立记录。

用户 T3 截图是最低视觉质量标准，以上仅为可评价候选。物理键盘、中文 IME、剪贴板、真实厂商 API、Apple Silicon 和 Windows 不由本记录证明。持续流式并发另见 [native-streaming.md](native-streaming.md)，不由示例样板推断。
