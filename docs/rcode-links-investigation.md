# RCode 链接交互调查

2026-10-30，只读检查 `D:/projects/RCode`，版本
`e8399dc38ee669e60e8b87916a0bfd8f580270b6`，并与本 checkout 比较。
这份记录保留接入前的源码调查。用户随后批准接入，当前实现及验收边界见
[正文与文件预览链接](transcript-links.md)；不能把下表中的全部 RCode 行为
当作 GCode 已具备的能力。固定 Zeron 参考未修改。

## 已核实的历史调整

- `979e07ecc4c305a18d5c755cd05dd7b50fb2c4b8`，2026-10-26：
  简化正文链接样式并完善悬浮提示。
- `b0467f7d12943285e047071270c676def6910239`，2026-10-22：
  文件链接文法接受 Windows 盘符路径。
- `1f38ea7`，2026-10-22：记录 transcript 文件链接与 Windows 路径。
  浏览器路由和 Markdown 文件预览还有更早的实现，不能都归于样式提交。

## RCode 当前规则

| 内容 | 实际行为 | 源码入口 |
| --- | --- | --- |
| 正文链接 | 文件和网页均用主题色文字；没有箭头、文件图标、代码灰底或常驻下划线；hover 点状下划线 | `crates/ui/src/markdown/render.rs:1288` |
| 完整目标 | hover/键盘 focus 展示深色圆角提示，最大宽度 360；长内容可滚动；文件显示解析后的绝对路径及行列 | `markdown/link_destination.rs:28`、`markdown/link_interaction.rs:616` |
| 文件标签 | 显式 Markdown 链接保留作者标签；可解析的行内代码路径显示文件名，同名冲突补必要路径；复制保留原路径 | `markdown/inline_code_links.rs:245,308` |
| 长 URL | 按可用宽度省略显示，真实目的地址、复制地址及文本选择保留全文；表格复用同一逻辑 | `markdown/link_presentation.rs:117` |
| 文件普通点击 | 在所属会话的右侧原生文件页打开并选中；重复打开复用页签；会话 root 外绝对文件按来源宿主只读读取 | `shell.rs:3722,3881` |
| Markdown 文件 | `.md` / `.markdown` 大小写无关，默认 Preview；带行号切到 Code 后定位；预览当前缓冲区 | `files/document.rs:79`、`files/preview.rs:578` |
| 图片文件 | 在右侧原生图片页 fit 显示，支持面板内缩放和平移；文件图片点击不另开 lightbox，Markdown/附件图片可用 lightbox | `files/image_preview.rs:13`、`files/preview.rs:1175` |
| URL 普通点击 | 默认打开所属会话的右侧 Browser；配置可改为外部浏览器；显式内开/外开动作不受偏好影响 | `shell.rs:3536` |
| 点击与键盘 | 普通左键即可；拖动选择文字不激活；Tab 聚焦，Enter/Space 激活，Shift+F10 菜单；未发现 Ctrl 点击外开的分支 | `markdown/link_interaction.rs:223,653` |
| 文件菜单 | 内部打开；本机文件另有系统默认应用、显示目录；可解析的远程文件也可复制绝对路径 | `markdown/link_interaction.rs:354,471` |
| URL 菜单 | 内开、外开、复制地址、内部打开偏好开关 | `markdown/link_interaction.rs:515` |

聊天正文支持 POSIX、Windows 盘符、file URL 和工作区相对路径。相对路径
直接归属于发出消息的会话的第一个 root；不会按文件存在性搜索其他 checkout。
绝对路径可匹配已知 root，匹配不到则由来源宿主只读打开。
Windows 路径按盘符语法和大小写规则处理；正文拒绝 `.`/`..`、UNC、
drive-relative、query 等歧义路径。

行列支持 `:12`、`:12:5`、`#L12`、`#L12C5` 及行范围，范围定位首行。
列是从 1 开始的字符列，零行和逆序范围无效。
源码为 `workspace_links.rs:339,467,655`。

Markdown 文件预览中的相对路径基于当前文档目录，允许合法 `../` 和文档锚点；
这与聊天正文路径规则有意不同。HTTP(S) 使用会话 Browser 路由，
`mailto:` 交系统邮件程序；该预览不解析绝对文件路径。
源码为 `files/markdown_preview.rs:114,1035`。

模型生成的网页链接只接受显式 HTTP(S)，拒绝凭据、空白/控制字符、反斜杠
和损坏的 percent escape。拒绝后不能降级为系统 opener。
地址栏为用户补全 `https://` 的行为不适合直接用于模型链接。
源码为 `browser/model.rs:114`、`markdown/links.rs:41`。

## 接入前的 GCode 差异（历史）

1. `internal/desktop/transcript_theme.go` 未设置 Markdown `OnLink`。
   MyGo 的默认合法 URL 会交系统 opener，本地文件链接没有接入侧栏。
2. `internal/desktop/transcript_math.go` 和 `transcript_prompt.go` 还有独立
   `ui.Link` 入口，不能只修普通 Markdown 就声称统一。
3. `internal/workbench` 已有 `OpenBrowser`、`OpenFile`、`OpenFileAt`。
   后者的列是从 0 开始的 UTF-8 字节偏移，需要显式转换链接字符列。
4. 新文件文档默认 Code；预览切换仅识别大小写敏感 `.md`，
   `.MD` / `.markdown` 不具备同样入口。
5. 文件 Markdown 预览使用空 Options，没有相对文档导航或会话 Browser 接线。
6. 现有 MyGo 默认链接样式和菜单不是 RCode 的上述实现；不能通过启用默认
   OnLink 就推断样式、完整目标提示、长 URL 选择与复制都已经完成。

后续接入已统一普通 Markdown、数学混合段落、用户引用、文件预览的
路由及来源会话归属，并增加共享样式和菜单。本机使用现有强类型 Go 接口；
远程宿主的文件不能交给本机文件系统处理，也不需要复制 Rust 源码。

## 文档与代码不一致

RCode 的 `docs/transcript-file-links.md` 有两处描述需要按现有代码理解：

- 关于 root 顺序的概述容易让人理解为相对路径会依次回退到 parent/project；
  `first_root_owning` 对相对路径直接选择首个 root。扩大搜索用于绝对路径及内部 mention。
- 菜单段落称三个 system-level rows 仅本机显示，随后又说 Copy path 普遍可用。
  代码实际是两个仅本机系统动作，Copy path 对可解析的远程文件也显示。

接入前的 GCode 审计只声称工具文件 badge 可打开编辑器，没有声称正文链接已完成，
当时未发现对应的事实性文档夸大；接入后的审计已单独更新。上述 RCode 文件保持只读。
