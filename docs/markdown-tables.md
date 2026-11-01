# Markdown 表格

会话消息里的标准 GFM 表格由现有 MyGo 解析器识别，自动替换为 Go/MyGo 增强表格。
无需额外工具协议、专用 JSON 或 WebView；用户消息和助手回复共用该逻辑。

## 参考与实现

只读检查本地 ZCode `3c5b227649371507ddc4eb46ac5133afe6db1cab`：
`packages/ui/src/components/ai-elements/message.tsx` 使用 Streamdown，
通过 table/thead/tbody/tr/th/td 组件映射接入 `markdown-table.tsx`。
该组件在原生 HTML table 外加工具栏、滚动视口和预览窗口，而不是改变模型输出格式。
复制和下载都从显示的单元格文本构建，列宽采用内容测量与最小/最大约束。
宽表格可借用会话空白；长表格的虚拟横向滚动条避开 composer，子智能体等嵌套容器禁用此固定行为。

GCode 的独立原生实现位于 `internal/desktop/transcript_table*.go`：

- `Options.Block` 截获解析后的 Table，按行和单元格生成不可变显示数据。
- 以解析器解析后的链接目的地构建单元格，保留行内代码、强调、链接和对齐，使用现有 Markdown 文本组件。
- 每列按单元格内容测量，最小 64、最大 448 逻辑像素；窄窗横向滚动，较宽窗口按内容比例分配剩余宽度。
- 流式表格的测量列宽只增不减，结束后重新按完整内容计算；每张表独立保存滚动、展开和预览状态。
- 使用主题颜色、圆角外框、行分隔与悬停状态，不添加竖向网格；工具栏使用共享图标按钮。
- 复制为 Markdown：复制显示文本，转义反斜杠和管道符，补齐表头分隔行；不复制行内样式标记和链接 URL。
- 下载 CSV：本机系统保存窗口选路径，后台写 UTF-8 BOM 与 CRLF；按 CSV 规则处理逗号、引号、换行，公式前缀作为文本导出。
- 预览为窗口内原生模态，表头留在滚动区域之外，正文支持横纵滚动，横向位置与表头同步。
- 主会话的宽表可展开到可用会话区域，并避让侧边会话信息；长表浮动滚动条停在输入框上方。
- 子会话/子智能体表格保持自身区域，不借用主会话宽度，不将浮动滚动条挂到主输入框。

共享横向滚动条实现与尺寸入口为 `settings_controls.go` / `design.Controls()`。
开启/关闭公式渲染不再移除整个 Markdown Block 钩子；表格和引用仍保留。
不改 MyGo 源码、模型消息、宿主会话数据或远程传输协议。

## 验证边界

组件回归验证 Markdown 复制往返、中文/引号/管道符 CSV、公式前缀、实际文件写入、
流式追加、宽度分配、嵌套限制、复制操作、预览开关和固定表头。
CSV 文件写入使用测试选择的临时路径；操作系统保存对话框尚未进行自动化验收。

原生 `TestDesktopLocalMarkdownTables` 使用私有真实核心和预置展示消息，无供应商请求：
深浅主题、中英文、900×600/1320×880、两个独立表格、可见区域滚动、预览、宽表拖动入口、
展开会话宽度与输入框避让。预置回复不是实模型流式供应商证据。
截图和树是实现验收证据，不宣称与参考逐像素一致。

## 2026-10-30 验证记录

`go test ./...`、表格与共享控件专项回归（含 `uifixture` 构建）、
`go vet ./internal/desktop ./internal/design`、`go tool mygo vet .` 均通过。
原生双主题证据位于
`output/markdown-tables-verified/20261030-160912-951449189/evidence.json`。

发布构建已更新本机 `output/model-picker-layout/GCode.exe`，可见窗口正常响应，
5 个会话及模型目录缓存保持不变。更新记录与旧程序备份位于
`output/local-update-20261030-161552/`；发布程序 SHA-256 为
`EA3A2FDAAAF08E3F7E4CF9209C07AC173FBBB925E5C130E7A6F707EF1F6EDF34`。
