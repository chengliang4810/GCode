# 壁纸取色与数学字体

2026-10-30，设置页的“匹配壁纸颜色”和“数学字体”已接入真实桌面消息。

## 系统壁纸

客户端开启取色后，在后台查询当前操作系统的壁纸，每分钟检查一次路径、
修改时间与大小；未变化的文件不重复解码。设置变化、深浅模式变化和窗口关闭
会取消旧任务，旧任务不能覆盖新设置。渲染期间没有壁纸文件或系统设置 IO。

Windows 使用 `SPI_GETDESKWALLPAPER`；macOS 使用系统自带 `osascript`
读取当前桌面图片；Linux 根据 `XDG_CURRENT_DESKTOP` / `DESKTOP_SESSION`
选择 GNOME、Cinnamon、KDE Plasma 或 XFCE 的当前配置。已识别的桌面不读取
另一桌面遗留的壁纸。桌面身份不可得时才顺序探测本地已有的图片。
GNOME 深色图片缺失时使用同一桌面的普通图片。

取色读取 PNG、JPEG、GIF、BMP、WebP。输入上限为 64 MiB / 6400 万像素，
在不超过约 1.6 万个采样像素中选取加权色簇，而不是将互补颜色平均成灰色。
主题使用该颜色生成对比度受约束的强调色、选择色与轻微表面色调；
关闭设置即恢复所选主题及强调色。无法发现或读取图片时保留所选主题，
设置行显示实际错误。动态壁纸、macOS HEIC 和未支持的 Linux 桌面可能无法取色。
多显示器使用系统提供的主桌面路径；KDE 选择第一个存在的本地图片，
并未实现按 RCode 所在显示器分别取色。

当前 Windows 实机读取验证取得本机壁纸与颜色；Linux 桌面选择、
缺失文件和取消行为有隔离测试，但未在 Linux/macOS 图形会话实测。

## 原生公式

真实主会话、侧会话及用户消息的 Markdown 共用数学设置：
支持 `$…$`、`\(…\)` 行内公式，`$$…$$`、`\[…\]` 显示公式，
以及已闭合的 `math` / `latex` 代码块。“Render math”关闭时恢复源码，
“Math size”按实际字形几何缩放。“Math font”切换六个真实字体文件，
不再切换固定示例 SVG。

实现固定依赖 `github.com/go-latex/latex`
`v0.0.0-20250304174226-2790903426af` 的 AST、符号映射和字体后端，
由应用的 Go 盒布局处理上下标、分式、根式、常用运算符/函数、
希腊字母、粗体/花体/双线体、部分重音及上划线，输出数学字体的矢量轮廓。
MyGo 通过原生 SVG 渲染这些字形，没有 WebView、JavaScript、
外部 TeX 程序或在线服务。字体解析、公式排版与 SVG 解析在单个可取消
后台 worker 中运行；UI 只读取不可变缓存。缓存数量和估算字节均有上限。

这不是完整 LaTeX：矩阵/对齐环境、宏定义、宏包、复杂伸缩括号以及
未支持的命令显示原始 TeX。普通左右括号不会按内容自动伸缩。
带 HTML、脚注或图片的混合段落由 MyGo 原生 Markdown 保留内容，
其中行内公式保持其原生源码显示；表格中的行内公式也保持源码。
行内公式使用原生文本对象槽位，保留周围文字、粗体和链接；
整段“Copy”菜单复制原始 Markdown。直接选择包含公式槽位的部分文本时，
MyGo 的对象槽位仍可能复制为对象替代字符，而不是原始 TeX。
流式未闭合的 math 代码块保留代码显示。
最终设备像素任一边达到原生 SVG 4096 限制前会显示源码，避免空白图片。

六个未修改的 OTF 从冻结上游
`Zeron@ab189159f10cf8e63c0502be75a59bbc487e0eaf/crates/zatex/fonts/`
导入，运行时按需解析。独立的计算包 `internal/mathrender` 不依赖 MyGo；
其来源与哈希在 `internal/assets/manifest.json` 的 `runtimeMathFonts` 中，
许可原文随 `resources/licenses/fonts/` 离线分发：

| 选项 | 文件 | SHA-256 |
| --- | --- | --- |
| Fira Math | FiraMath-Regular.otf | `2028cbd3dd4d8c0cf1608520eb4759956a83a67931d7b6d8e7c313520186e35b` |
| STIX Two | STIXTwoMath-Regular.otf | `f2076b9f1676438439dd41e23676f5ab99056e83d6b8f8c27841591ef2ccfa72` |
| Computer Modern | NewCMMath-Book.otf | `2ea09ebc9167b1e1a66f31390dc917f2d4004ecfca72d51b28010e4ad6becd95` |
| Libertinus | LibertinusMath-Regular.otf | `ded714b0d3808af527438ed7a85f16933133f66a12104c4a83db4cc248888011` |
| Palatino | texgyrepagella-math.otf | `1f9e010f60e947d0e925910009b2ea85ad54edc7cefb106f8cdefb9ffd1d5f2f` |
| Euler | Euler-Math.otf | `d6faf2b5f507aab84990b5fb144675a57dbd6b2b3c216947b99fb1dc9b140529` |

`TestMathFontChangesActualMessageGlyphsAndSize` 对真实 Markdown 输入验证
六字体的渲染差异、缩放、深浅主题和窄宽度。实现渲染图在
`output/residual-features/math/`。这些是当前原生控件渲染证据，
不代表参考 UI 的像素验收，也不代表真实模型服务的输出验收。

许可证文档此前称数学字体仅用于离线 SVG、运行时不带 OTF，
现已按实际实现同步；MyGo 修订号也已与当前 lock 对齐。
