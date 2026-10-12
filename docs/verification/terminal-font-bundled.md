# 内置终端字体与退出清理验证

日期：2026-10-12（Asia/Shanghai）。四个固定私有区码点的原生缺字已闭环；完整终端、整体视觉和首版验收仍未通过。

## 实现与原因

旧包使用系统 monospace。真实 PTY 输出的 UTF-8 字节正确，但 `U+E0A0`、`U+F07C`、`U+F120` 显示问号方框；`U+E0B0` 的三角形可见。旧包截图、字节和清理审计保留在 [baseline-native](../../output/acceptance/terminal-font/baseline-native/review.json)。

GCode 现在嵌入固定 Nerd Fonts SymbolsOnly v3.5.1 的 `SymbolsNerdFontMono-Regular.ttf`，通过 MyGo 的 `ui.RegisterFont` 注册应用字体。字体 SHA-256 为 `fe471e538392f51910faab985fa8e192a39dd3426125edd15b71b3680df0e749`，没有安装系统字体。完整许可、归属及固定来源记录随包携带，见 [SOURCE_MANIFEST.md](../../resources/licenses/nerd-fonts/SOURCE_MANIFEST.md)。

CoreText fallback resolver 找到字体不能证明实际排版使用了它：系统 monospace 后置 cascade 的 resolver 为绿，但 CTLine 仍选 `.LastResort`。产品采用符号字体作为 primary；实际 typesetter 对普通 ASCII 回退到 Menlo、中文回退到 PingFang SC，PUA 使用内置字体。探针原始脚本和输出见 [terminal-font-coverage.md](terminal-font-coverage.md)。终端会把符号缩放到单个 cell，资源覆盖单测只检查真实内置 TTF，不能替代原生截图。

同时修复了异步终端创建的退出竞态。请求 owner 在 UI 队列接管前持有资源；关闭 App 或切换目录会取消请求并清理尚未安装的终端，迟到结果自行关闭。Native factory 在创建前后检查取消状态。四个回归测试先红后绿，见 [terminal-lifecycle](../../output/acceptance/terminal-lifecycle/)。

## 当前包身份与回归

当前包为 `build/darwin-amd64/GCode.app`，executable SHA-256 `d9674cf38371f6ff805d81c9dd934c43e0a69eb9091896befcc24a0707349ff7`，code SHA-256 `ce4ca62b6b81668396404b96e3d96117073c7ea1845fdcc73bee064604812417`。

[terminal-font-bundled](../../output/acceptance/current-package-regression/terminal-font-bundled/package-manifest.json) 记录 896 个生产/嵌入资产/资源文件、425 个应用测试文件，构建前后清单不变。全仓测试、桌面与应用层 race、vet、fmt、package、codesign 全部通过。之前的项目树包完整保留于同目录 `previous-project-tree-package/GCode.app`，不改写历史证据。

## 实际原生验证

- [workspace review](../../output/acceptance/native-workspace/terminal-font-bundled/review.json)：新包隔离副本的目录/二进制/中文预览、真实 tracked/untracked Git Diff、TextEdit 保存刷新。4 张图已打开并核对 SHA，PID 46585 已消失。
- [glyph review](../../output/acceptance/terminal-font/bundled-native/review.json)：四个固定 PUA、`ABC abc 0123`、`中文终端` 均实际可见，无缺字方框；输出 UTF-8 等值。3 张图已打开并核对 SHA，PID 47565/47587 已消失。
- [terminal review](../../output/acceptance/native-terminal/terminal-font-bundled/review.json)：焦点、合成 GUI ASCII 输入、cwd、shell 编辑反映到预览/Diff、resize、Shift+PageUp/PageDown、shell exit 和带长子进程的菜单退出清理。10 张图已打开并核对 SHA，5 个 app/shell/child PID 已消失，搜狗输入源已恢复且独立读回。

| 窗口 | stty rows × cols | bottom → pageup → pagedown 的可见 marker |
| --- | --- | --- |
| 1440 × 900 | 61 × 63 | 131..180 → 071..131 → 131..180 |
| 1280 × 800 | 54 × 43 | 154..180 → 101..154 → 154..180 |

原包、资源和驱动记录的生产源码清单保持一致。驱动机械状态仍保留 `PASS_PENDING_VISUAL_REVIEW`，root 独立的范围复核另记 `review.json`，没有覆盖原记录。

这些结果只覆盖记录中的固定字符及交互。任意 Unicode、物理键盘/中文 IME、滚轮、选择与剪贴板、真实厂商 API、其他目标设备和用户 T3 视觉认可仍待验证；P4.3/A14 保持部分，首版仍未交付。
