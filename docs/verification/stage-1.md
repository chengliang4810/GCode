# 阶段 1：源码接管、入口与原生样板

记录日期：2026-10-12。整体仍在实现，以下只列实际取得的证据。

## 当前成果

- 来源为 zero `99721c762f37cd43ac511007a5f51d1846df959e`，全树核验、归档哈希与裁剪清单见 `docs/source-origin.md`。
- Go module 为 `gcode`，Go 1.27.2，mygo 固定基线为 `v0.4.3-0.20261011132240-65154ea8777c`，通过仓内 `third_party/mygo` fork 固定终端修复；构建无需机器外的绝对 replace 路径。已移除外部 zero module、Bubble Tea/Bubbles/Lip Gloss、npm 和旧 CLI/TUI 宿主。
- 新桌面入口 `cmd/gcode` 使用 `ui.View`，`internal/application` 直接适配 controller；后台快照通过 `Window.Update` 更新界面，没有 ACP 或 WebView 通信。
- 工作区适配器以已打开项目/会话的实际根目录限制浏览，处理 macOS 路径别名；文件路径相对于当前浏览目录。接通 Git Diff、外部编辑器和 mygo 原生终端工厂。
- 窗口关闭时隐藏窗口，保留控制器；显式退出等待控制器和终端清理完成，再退出原生事件循环。

## 构建与当前设备

环境：macOS 14.8.7（23J520），x86_64，Go 1.27.2 darwin/amd64。

执行 `GOFLAGS=-buildvcs=false go tool mygo build --skip-dmg` 成功。关闭 VCS 元数据采集是因为当前仓库尚无提交，不是提交或构建源快照的替代。产物为 `build/darwin-amd64/GCode.app`，初次包约 23.9 MB；采用本地 ad hoc 签名，`codesign --verify --deep --strict` 通过。

包内已包含 `Contents/Resources/libghostty-vt.dylib`。库存在与签名验证并不证明终端场景通过，终端实测另外记录。

## 原生窗口证据

执行应用包里的真实 GCode 程序并使用 macOS `screencapture -l` 针对该窗口截图，保留系统窗口按钮：

- `output/acceptance/stage-1/gcode-native-launch.png`：真实初始工作台。
- `output/acceptance/stage-1/gcode-native-settings.png`：通过系统 Accessibility 按钮打开真实设置页。

关闭系统 `AXCloseButton` 后：GCode PID 95867 仍运行，屏幕窗口清单只剩托盘。执行 `open build/darwin-amd64/GCode.app` 后窗口恢复，PID 仍为 95867。这证明当前空闲应用的隐藏和重开；运行中任务的持续执行与显式退出隔离仍须单独验证。

代表性样板驱动在实际 mygo 原生窗口中运行桌面产品视图，以 `Window.CapturePage` 保存浅/深色 1280×800 与 1024×700 图像：`native-{light,dark}-{尺寸}.png`。这些捕获不包含系统按钮，不能替代 OS 窗口截图。

2026-10-12 最后一轮捕获使用修复后的当前产品视图并通过 `screencapture -l` 保留系统按钮，生成 `native-os-{light,dark}-{1280x800,1024x700}.png`。窗口 PID 32134、CGWindow ID 21580，驱动正常退出。四张图均已人工查看：浅色代码标点完整可读，1280 窗口右面板填满其区域，输入区停止标签可见，1024 窗口默认收起右面板并保留展开入口；另有定向布局测试覆盖窄窗口手动展开。图内标注“示例数据·仅视觉验证”，不作为真实 Agent 执行证据。

已将浅色、深色原生样板呈现用户并异步请求视觉方向确认。在收到回复前不扩展其他管理页面；现有对话行为及独立内核工作继续。视觉方向、中文输入法和焦点的完整条目仍未完成。

## 尚未完成

- 用户确认已经呈现的代表性 OS 原生样板方向。
- 中文输入法组合/候选位置、Enter/Escape、复制粘贴、长会话阅读位置及物理键盘终端交互。
- 运行中关闭窗口、退出清理、多实例保护的完整场景。
- 裁剪后全量回归、扩展运行能力及 Apple Silicon/Windows 目标设备验收。
