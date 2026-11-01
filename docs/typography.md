# 界面字号

2026-10-30 起，GCode 的界面字号层级按本机只读参考
`D:/projects/ZCode`（HEAD `3c5b227649371507ddc4eb46ac5133afe6db1cab`）
的当前实现设置：

- `packages/ui/src/styles.css` 的 `--text-ui-*` 定义；
- `packages/ui/src/lib/uiFontSize.ts` 的 14px 默认值及 12–20px 范围；
- `packages/ui/src/SettingsPage.tsx` 的设置页标题；
- `packages/ui/src/components/ai-elements/message.tsx` 的正文、标题与行高；
- `packages/ui/src/lib/codePreviewSettings.ts` 的独立 12px 代码预览；
- `packages/ui/src/terminal/TerminalSession.tsx` 的独立 13px 终端默认值。

只读取数值、层级及行为，使用本项目 Go/MyGo 独立实现；没有复制参考源码。
字号调整不恢复此前暂停的完整视觉一致性目标。

| 角色 | 默认字号 | 与用户界面字号的关系 |
| --- | ---: | --- |
| 极弱元数据、计数、紧凑徽标 | 10px | -4px |
| 次要元数据、快捷键、时间 | 12px | -2px |
| 浮层说明 | 13px | -1px |
| 正文、导航、表单标签、操作按钮、输入及会话 | 14px | 基准 |
| 对话标题 h2 | 16px | +2px |
| 对话标题 h1、弹窗标题 | 18px | +4px |
| 窄窗设置标题 | 24px | +10px |
| 宽窗设置标题（窗口宽度 ≥1024） | 30px | +16px |

外观设置中的数值现在就是实际逻辑像素，不再将 16 换算为 13。
例如选择 16px 时，正文、辅助文字、一级会话标题和宽窗设置标题分别为
16、14、20 和 32px。已有设置值按当前含义直接使用，不提供旧换算或数据迁移。

统一定义位于 `internal/design/text.go`；`TextSize` 根据主题基准进行像素加减。
共享控件使用 `internal/design/controls.go`，设置标题使用
`internal/design/settings.go`，页面不得覆盖共享控件字号。
普通文字采用相对行高，聊天正文为 1.75 倍、输入为 1.625 倍。
会话的单行输入区保持 47px，通过 `(47 - 当前行高) / 2` 计算上下内边距；
不能保留固定 12px 内边距，否则 16px 及以上字号会在空输入区产生滚动条。
展开输入区仍随实际内容增高，并在 260px 上限后保留正常滚动。
图标、圆角、内部间距、开关和页面外框保持逻辑像素，只有文字控件按内容增高。
代码、文件、Diff 和终端采用各自保存的绝对字号，不随界面字号变化。
用户已选字体保留；此轮同步的是字号逻辑，不替换字体文件或系统字形渲染器。

参考文档 `docs/ui/ui-font-tokens.md` 只列出了最初的四档字号，
没有涵盖当前源码中的 caption、xl、24/30px 设置标题等新增档位。
本次以参考源码为准。GCode 旧文档中的 20px 设置标题、按 13px 基准比例缩放
及冻结参考字号属于此前实现，已由本文和 `ui-controls.md` 的当前规则替代；
历史截图与验收记录仍保留。

实现检查与原生窗口证据位于 `output/zcode-typography/`。
这些证据验证字号规则、布局和交互，不代表与 Electron 字形像素完全一致。

全仓 Go 测试（本机 `CGO_ENABLED=0`）、Go vet、MyGo vet 和普通桌面构建通过。
最终原生证据为
`output/zcode-typography/native-verified/20261030-154156-2760719344/evidence.json`：
设置交互、真实本地核心保存及重启、四组深浅主题模型菜单、六组深浅主题
12/14/20px 字号均通过。字号用例核对真实设置页标题高度及输入框高度，
并保存普通设置、模型页和首页截图；没有调用外部模型。
先前一次整组运行的供应商悬停提示超时记录仍保留在 `native-final/`，
该场景单独复测与最终整组复测均通过，没有为此修改业务或测试实现。

单行滚动条修复证据为
`output/composer-scrollbar/native/20261030-155008-1164874615/evidence.json`。
原生模型菜单场景改用 16/20px 字号，覆盖深浅主题；
`conversation-compact.png` 的输入区右侧有逐像素无滚动条断言。
组件回归覆盖 12/14/16/18/20px、空/英文/中文和 emoji 草稿、
长文本到上限后的滚动以及清空后恢复单行。
