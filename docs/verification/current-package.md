# 包与回归身份

日期：2026-10-12（Asia/Shanghai）。环境：Go 1.27.2 darwin/amd64，macOS 14.8.7 / 23J520。当前包自动化回归通过；固定 glyph、工作区和终端技术范围 review 已完成，但物理输入等完整交互及用户视觉认可仍 pending，首版未交付。

## 当前 canonical 包

当前 build/darwin-amd64/GCode.app 对应 [terminal-font-bundled](../../output/acceptance/current-package-regression/terminal-font-bundled/)。该身份包含内嵌终端字体、四类 provider 协议 fixture 覆盖和四个终端生命周期 fixture；旧项目树包只作为历史副本保留。

| 身份 | 值 |
| --- | --- |
| executable SHA-256 | d9674cf38371f6ff805d81c9dd934c43e0a69eb9091896befcc24a0707349ff7 |
| __TEXT,__text SHA-256 | ce4ca62b6b81668396404b96e3d96117073c7ea1845fdcc73bee064604812417 |
| production source fingerprint | 697811ee3c4fc978e76c9400aa22da01df701b82b71555cd5730fcb0321c1f14 |
| test fingerprint | 7f29d17f9c809d7d2370c6c88d687d9d6799ee755c380768049438ac92654f78 |
| 生产 / 测试文件 | 896 / 425 |
| executable bytes | 26,857,584 |
| embedded terminal font SHA-256 | fe471e538392f51910faab985fa8e192a39dd3426125edd15b71b3680df0e749 |
| 构建前后源码 / 测试未变 | true / true |

896 个 production files 包含 Go 源码、内嵌字体和 resources；425 个测试文件属于这个包的 test fingerprint。逐文件 SHA、资源、字体和构建身份见 [source-fingerprint.json](../../output/acceptance/current-package-regression/terminal-font-bundled/source-fingerprint.json)、[test-fingerprint.json](../../output/acceptance/current-package-regression/terminal-font-bundled/test-fingerprint.json) 和 [package-manifest.json](../../output/acceptance/current-package-regression/terminal-font-bundled/package-manifest.json)。

## 自动化回归

[results.json](../../output/acceptance/current-package-regression/terminal-font-bundled/results.json) 六项均 exit 0：

| 验证 | 命令 | 耗时 |
| --- | --- | ---: |
| 全仓测试 | go test ./... -count=1 -timeout=180s | 102.097 s |
| 受影响模块 race | go test -race ./internal/desktop ./internal/application -count=1 -timeout=180s | 75.384 s |
| 静态检查 | go vet ./... | 1.035 s |
| 格式检查 | make fmt-check | 0.206 s |
| 应用打包 | make package | 4.502 s |
| 签名验证 | codesign --verify --deep --strict build/darwin-amd64/GCode.app | 0.116 s |

本包的 application 协议 fixture 在 internal/application/provider_formats_integration_test.go 覆盖四个 wire format 子场景：responses、chat-completions、anthropic-messages、gemini-generate-content。TestServiceProviderFormatsFlow 验证工具调用、历史续接和切换模型；同一 fixture 还覆盖取消和 HTTP error，凭据不进入请求正文、会话快照或磁盘。四个终端生命周期 fixture 从 RED 修正到 GREEN：

- TestEnsureTerminalClosesLateOpenAfterCloseWithDroppedDispatch
- TestEnsureTerminalClosesQueuedOpenWhenDispatchIsDiscarded
- TestEnsureTerminalDoesNotOpenAfterClose
- TestEnsureTerminalDoesNotLetStaleDirectoryOpenReplaceCurrentTerminal

这些测试与 TestBundledTerminalFontCoversPromptSymbols 均包含在上方 425 个测试文件和六项回归中。

## 当前包工作区与终端原生复核

当前包的隔离副本已重新执行工作区、固定 glyph 和终端场景。技术范围 review 均完成，但它们不代替用户视觉认可：

- [native-workspace/terminal-font-bundled/review.json](../../output/acceptance/native-workspace/terminal-font-bundled/review.json)：打开项目、新建会话、目录/二进制/中文预览、真实 tracked/untracked Git Diff、TextEdit 保存及刷新。4 张截图已实际打开并核对 SHA，PID 46585 已消失，source 和 bundle unchanged。
- [terminal-font/bundled-native/review.json](../../output/acceptance/terminal-font/bundled-native/review.json)：真实打包终端加载内嵌字体；U+E0A0、U+E0B0、U+F07C、U+F120 均可见且无缺字框，ASCII、中文和 UTF-8 精确输出通过。该记录状态为 PASS_FOR_FIXED_GLYPHS_AND_RECORDED_SCOPE，范围不包括任意 Unicode、物理键盘或中文 IME，用户视觉为 USER_PENDING。
- [native-terminal/terminal-font-bundled/review.json](../../output/acceptance/native-terminal/terminal-font-bundled/review.json)：10 张截图、focus/GUI ASCII 输入、cwd、shell 修改→预览/Diff、两种窗口 resize、键盘翻页、shell exit、菜单退出和长子进程清理均由 root 复核，状态为 PASS_FOR_RECORDED_SCOPE；5 个 app/shell/child PID 48772、48794、49915、49944、50265 均已消失，输入源恢复并独立读回为 com.sogou.inputmethod.sogou.pinyin。

终端复核的窗口/PTY 与 scrollback marker 为：1440×900 对应 stty size=61 63，底部→PageUp→PageDown 为 131..180→071..131→131..180；1280×800 对应 54 43，为 154..180→101..154→154..180。[results.json](../../output/acceptance/native-terminal/terminal-font-bundled/results.json) 保留驱动生成的 scrollback: CAPTURED; VISUAL_REVIEW_REQUIRED 和整体 PASS_PENDING_VISUAL_REVIEW 字段；root 已独立打开并复核全部 10 张截图，终端技术范围 review 已闭环。物理键盘/IME、clipboard、鼠标 wheel/selection、任意 Unicode、真实 API 仍未测试，因此完整 P4.3/A14 继续为部分。

## 历史包与证据边界

旧项目树包完整副本保留在 [previous-project-tree-package/GCode.app](../../output/acceptance/current-package-regression/terminal-font-bundled/previous-project-tree-package/GCode.app)，其身份为 executable 00b310de56c927242e8a0cf95bb31807cc251dbf0e0b3fa91b2d29e4c7e965e5、code 19ba9bb09ed4c91335896ec399ab11ffb6776553cfb457f76d9243662b6f1e44、source 12718290f02ee141239bed0b1b8cd431ef0670fbe6d473131b37f07db3d1ef63、test 2b80a01449cc0e86712441d51e55e9e8c04e56902e8698256851b317b8d1cbe5、832/422 文件。旧包结果不能归入当前 terminal-font-bundled 身份。

旧项目树包目录内的 integrated 备份仍在 [previous-integrated-package/GCode.app](../../output/acceptance/current-package-regression/project-tree-layout-confirmed/previous-integrated-package/GCode.app)。该历史 integrated 身份为 executable db479d18dbde05705cf7bfced02c8ab6164f650d1f4bf3e28e0d481a76eeb216、code 40d66e826fe9117eaaee6be0cc4757831221d84c0463af621412348616ca8b8e、source 32e08f88522775e0cfb2568393ea93371b773d31222191f4e8533b2792f13a67、test a4f6282568d45675c8a897febf1f08958d6ea84964ba05a474fa83e4bdb52b2f、832/420 文件；该身份只属于历史 integrated 回归。

round-18/19 均在该旧 integrated 包运行三会话、30 秒、10 Hz、每会话 300 chunks；3/3 final completion AX 和清理通过，两轮 review 均为 PASS_FOR_RECORDED_SCOPE。这些历史流式结果不代表当前新包实测，也不证明 FPS、真实滚动效果或阅读位置恢复。

首次后台集成失败目录 [integrated-streaming-background-final](../../output/acceptance/current-package-regression/integrated-streaming-background-final/) 保留：失败来自证据 .go 副本被当作残缺 package 扫描。副本移入 _before 或改为 .go.txt 后，confirmed 回归通过；13 个生产文件的集成与备份见 [integration-manifest.json](../../output/acceptance/background-specialist/integrated/integration-manifest.json)。

旧证据保留各自身份，不能倒填为当前包验证。当前新包未完成真实厂商 API、物理键盘/中文 IME/剪贴板、完整终端输入交互、持续原生流式性能或 Apple Silicon/Windows 实机验收；用户提供的 T3 截图仍是最低视觉标准，构建和内部复核不能代替用户认可。
