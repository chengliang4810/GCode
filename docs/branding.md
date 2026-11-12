# GCode 命名

2026-10-30 按用户最新要求，应用展示名称与生成文件统一使用 **GCode**。
现有内部命令、主题 ID、代码标识和资源文件名使用 `rcode`；
环境变量前缀使用 `RCODE_`；需要连字符的仓库式 slug 使用 `r-code`。

开发启动使用 `make run`：先构建 `.mygo/run/GCode`（Windows 为 `GCode.exe`），
再启动该程序，确保 macOS Dock 使用 GCode 名称。
直接 `go run ./cmd/rcode` 会以内部命令名 `rcode` 创建临时程序；
`App.SetName("GCode")` 不会改变这个临时文件名及其 Dock 名称。
安装名为 `GCode`，程序名为
`GCode.exe`，macOS 应用包为 `GCode.app`，应用标识为 `dev.gcode.rcode`。模块名 `gcode`、服务入口
`cmd/gcode-server` 和数据目录 `~/.gcode` 保持现有实现。主题为
`rcode-light` / `rcode-dark`，网页批注链接为
`rcode-annotation:`。不提供旧品牌字段或标识的兼容读取。

上游仍为只读 Zeron v0.2.128，提交
`ab189159f10cf8e63c0502be75a59bbc487e0eaf`。原仓库 URL、源码路径、版权和许可、
未经修改的参考代码及历史截图、日志和验收记录保留原名。资源文件可以重命名，
但导入清单中的 `source` 仍记录真实上游路径，原素材字节不变。
主题导出器先核实原始参考代码，再使用现有内部主题 ID 和 GCode 展示名称，
保留来源 URL。

更新归档名称由 MyGo 根据 `mygo.json` 的应用名生成，例如
`gcode-0.1.0-windows-amd64.tar.gz`；额外 ZIP 同样读取配置中的名称，
例如 `GCode-0.1.0-windows-amd64.zip`。GitHub 发布标题和附件筛选同步使用 GCode。
原先要求安装名必须与展示名不同、拒绝同名 `.app` 的副本隔离规则已移除。
Windows 安装位置为 `%LOCALAPPDATA%\Programs\GCode`，开始菜单快捷方式为 `GCode.lnk`。
更新签名密钥继续使用现有专用密钥，不重新生成。
历史 RCode/Zeron 截图、日志、验收产物与其文件名保留当时名称，
不能据此判断当前构建仍使用旧展示名。

应用图标使用用户选定的 GC 方案 C：深色圆角底、白色图形、不带 GCode 文字，
圆角外部透明。小尺寸使用经过光学校正的 GC 轮廓：放大有效图形、加宽留白，
避免 G/C 交接处在低分辨率下粘连。`go run ./scripts/app-icon` 从同一组矢量路径
直接生成 `internal/assets/branding/app-icon-small.svg` 和各尺寸 PNG，不从大图缩小。
Windows 窗口按实际 DPI 分别加载 16/20/24/32/40/48 等原生尺寸，并在跨 DPI
调整窗口时更新、关闭窗口时释放句柄；其他平台使用共享主图。
`mygo.json` 从 1024px 的 `internal/assets/branding/app-icon.png` 生成 Windows
可执行文件/安装程序图标和 macOS 应用包图标。桌面在 `App.Attach` 时显式设置
窗口图标，使普通 `go run` / `go build` 也能显示正确的 Windows 任务栏图标；
普通 `go build` 本身不生成 Windows PE 图标资源，交付程序必须使用 MyGo 打包。
`internal/assets/data/app-icon.png` 及原图标 SVG 是清单固定的上游原素材，
仅保留来源身份，不作为当前应用图标。
Windows 原生回归 `TestDesktopApplicationIcon` 已通过，直接查询测试窗口的小/大
图标句柄；证据位于 `output/gcode-icon-native/20261030-155653-2760135284/`。
Windows MyGo 安装包及签名更新包校验通过，产物位于 `output/gcode-logo-package/`。
当前本地便携程序已正常关闭并替换重启，五个会话保留，已提取确认运行窗口的
16/32 像素图标和 EXE 图标；记录为 `output/gcode-logo-integration/delivery.json`、
`live-icons.json` 和对应 PNG。本次不代表 macOS 实机或完整视觉一致性验收。

小尺寸版本的原生验收为
`output/gcode-icon-small/acceptance/20261030-170228-3098894524/evidence.json`，
直接读取系统窗口的 HICON 位图，确认 16/32px 像素与专用资源一致。
24px 已直接从矢量路径导出并检查；本机为 96 DPI，不将其算作 144 DPI 显示器验收。
新旧原生图标对比为 `output/gcode-icon-small/comparison.png`，运行程序已替换并正常
重启、五个会话保留，见 `output/gcode-icon-small/delivery/delivery.json`。
全仓 Go 测试、MyGo vet 和 Windows 打包/签名更新校验通过，
产物位于 `output/gcode-icon-small/package/`。

当前 MyGo 为 v0.8.3，本地补丁标识为 `rcode-native-ui.24`，源码树 SHA-256 记录于
`third_party/mygo.lock.json`，
共 2,356 文件、16 个补丁（行内链接选择、WebView 数据目录、文本选区、
代码块制表符与分栏拖拽等）。
桌面测试使用上游原生 `plugins/automation`，不再由本地
补丁引入。`third_party/mygo.lock.json` 与 `scripts/frameworkpin/pin.go` 同步固定
源码和补丁校验值。品牌替换没有重新授予历史视觉验收结论；原 UI 优化目标继续暂停。

2026-10-30 更名验证：全仓 Go 测试、Go vet、MyGo vet、发布工作流 actionlint 通过。
MyGo Automation 实窗确认应用窗口标题为 `GCode`，并回归底栏的设置、用量与更新入口；
证据为 `output/gcode-brand-native/20261030-093636-225111446/evidence.json`。
四目标签名包与内容校验通过，产物位于 `output/gcode-release/`；
Windows PE 的 ProductName/FileDescription 为 `GCode`，
InternalName/OriginalFilename 为 `GCode.exe`。
macOS 已验证 `.app` 名、Info.plist 和架构；本机是交叉构建，没有生成 DMG，
也不代表 macOS 实机安装或公证验收。

macOS 的窗口 `SetIcon` 不设置 Dock 图标。桌面 Attach 在 macOS 使用 `App.Dock.SetIcon` 加载 macOS 专用品牌 PNG，使普通开发启动也显示 GC Logo；发布包仍通过 `mygo.json` 生成应用包图标。

macOS 专用图标由 `go run ./scripts/app-icon` 从现有 GC 矢量路径生成：
`app-icon-macos.png` 为 1024×1024，黑色底板占 832×832，四边各留 96px 透明区域；
`app-icon-macos.icns` 包含 16/32/64/128/256/512/1024 原生分辨率图像。
开发版 Dock 使用专用 PNG，发布包通过 `macos.icon` 使用同源 ICNS。
通用 PNG 与 Windows 小尺寸资源保持原有尺寸。此比例是本次视觉调整值，不代表已完成 Dock 并排实机验收。

macOS 图标内部的 GC 图案按底板画布的 82% 独立矢量绘制并居中，增加图案与底板之间的内留白；832px 底板和 96px 外透明边距不变。
