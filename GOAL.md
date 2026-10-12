# /goal：交付 GCode 原生编码 Agent 桌面端

状态：执行中。设计依据为 `docs/design/gcode-desktop.md`，任务顺序为 `docs/plans/gcode-desktop-implementation.md`。

根目标：接管 zero 的最新源码，在当前工程实现独立维护的 Go + mygo 原生桌面工作台，完成真实编码链路、多会话、纠正/队列、权限、历史、四类模型 API、文件/Diff/终端、可选 worktree 和扩展运行能力。UI 与内核通过进程内 Go 接口通信。

## 并行目标与所有权

| 代理目标 | 独占责任 | 交付 |
|---|---|---|
| source-intake | 临时 zero 快照、`docs/source-origin.md` | 来源、完整差异清单、可复制目录 |
| desktop-ui | `internal/desktop/` | 三栏原生界面、主题、对话、输入、权限、设置、资源面板视图 |
| session-runtime | `internal/runtime/`、`internal/controller/` | 内核组装、会话生命周期、队列、取消、历史、扩展 |
| api-providers | `internal/providers/` | 四类 API Key 协议、移除订阅专用入口、协议测试 |
| workspace-tools | `internal/workspace/`、`internal/worktrees/`、`internal/execution/`、内置文件/命令协调 | 文件、Git/Diff、worktree、资源归属和并发安全 |
| go-toolchain | Homebrew Go 安装、`docs/goals/go-toolchain.md` | 官方最新稳定版成为默认 Go，实际编译验证 |
| acceptance-audit | `docs/verification/acceptance-matrix.md` | 独立逐项审计与缺口报告 |
| root | 模块与入口、共享契约、源码导入裁剪、最终集成、验收记录 | 可构建可运行应用及逐项完成证据 |

各代理在 `docs/goals/<目标>.md` 记录专属 /goal、进展和验证。共享契约由 root 维护；修改其他代理所有权内的文件先协调。参考仓库只读，不提交或推送。

## 完成审计

- 设计和计划中的首版要求逐项有代码与相应范围的测试/运行证据。
- 原生窗口、真实 API、目标设备分别记录，离屏或模拟通过不能替代实测。
- 缺少凭据或设备时继续所有独立工作，保留未验证项；不能把部分实现标记为整体完成。
- 全部必要工作和验收满足后才能结束根目标。

## 当前进展（2026-10-12）

- 当前 canonical 包为 `terminal-font-bundled`：executable `d9674cf38371f6ff805d81c9dd934c43e0a69eb9091896befcc24a0707349ff7`、code `ce4ca62b6b81668396404b96e3d96117073c7ea1845fdcc73bee064604812417`、production/test fingerprint `697811ee3c4fc978e76c9400aa22da01df701b82b71555cd5730fcb0321c1f14` / `7f29d17f9c809d7d2370c6c88d687d9d6799ee755c380768049438ac92654f78`，生产/测试文件 896/425，executable 26,857,584 bytes，embedded font `fe471e538392f51910faab985fa8e192a39dd3426125edd15b71b3680df0e749`，构建前后清单未变。全仓测试、受影响模块 race、vet、fmt、package 和 codesign 六项均 exit 0；详情见 `docs/verification/current-package.md` 与 `output/acceptance/current-package-regression/terminal-font-bundled/{package-manifest.json,results.json}`。旧项目树包只保留在新目录的 `previous-project-tree-package/GCode.app`，不得归入当前身份。
- 当前 canonical 包已纳入四类 provider wire format fixture（responses、chat-completions、anthropic-messages、gemini-generate-content）以及四个 terminal lifecycle fixture 的 RED→GREEN 回归。workspace review 为固定范围通过；固定 glyph `U+E0A0`、`U+E0B0`、`U+F07C`、`U+F120` 已通过。完整终端 review 的 focus、GUI ASCII 输入、cwd、shell 编辑→预览/Diff、resize、键盘翻页、shell/menu exit 和清理已通过，root 已独立打开并复核全部 10 张截图；`results.json` 的 `scrollback: CAPTURED; VISUAL_REVIEW_REQUIRED` / `PASS_PENDING_VISUAL_REVIEW` 仅保留驱动原始字段。P4.3/A14 仍因任意 Unicode、物理键盘/IME、clipboard、鼠标 wheel/selection、真实 API 和用户视觉认可未完成而部分。
- 用户已反馈 visual-v3 的布局和空间细节仍需调整，并明确左侧结构为“项目栏目 → 项目名称 → 多个对话”。已实现可折叠项目树及栏宽、标题、缩进、行高、共用阅读列调整；`visual-v4/confirmed` 保存 7 张实际原生截图，项目独立折叠、重新展开与跨项目对话切换 AX 检查通过，仍等待用户视觉评价。 [visual-v4.md](docs/verification/visual-v4.md) 记录本次范围；首版仍未交付。

- 运行 ID 已统一贯穿 Controller/runtime/子代理；主动取消与真实错误已分开呈现，授权卡片显示解码命令并保留操作按钮。
- 历史 `integrated-streaming-background-confirmed` 包与回归：executable `db479d18dbde05705cf7bfced02c8ab6164f650d1f4bf3e28e0d481a76eeb216`、`__TEXT,__text` 代码段 `40d66e826fe9117eaaee6be0cc4757831221d84c0463af621412348616ca8b8e`、production source fingerprint `32e08f88522775e0cfb2568393ea93371b773d31222191f4e8533b2792f13a67`、test fingerprint `a4f6282568d45675c8a897febf1f08958d6ea84964ba05a474fa83e4bdb52b2f`；生产文件 832 个、测试文件 420 个，构建前后源码清单均未变化，executable 为 24,206,352 bytes。全仓测试、受影响包 race、vet、fmt-check、package 和 codesign 均 exit 0，完整记录见 [current-package.md](docs/verification/current-package.md)。该身份只属于历史 integrated 回归，不代表当前 canonical 包。
- 后台 specialist 已集成到当前应用层：`internal/background.Manager` 在 GCode DataDir 持久化任务元数据与输出并支持 running/completed/error/killed、PID 和进程内取消；`internal/specialist` 已接通 `run_in_background`、`TaskOutput`、`TaskStop`、resume、完成等待、关闭收束和 prompt 文件清理；`internal/runtime` 持有共享/懒加载 runtime 与 manager 生命周期，`Engine.Close` 等待后台任务并执行取消/清理；`internal/application` 已验证 parent A 创建、parent B 查询/停止以及 `Service.Close` 收束。`internal/application/background_isolation_integration_test.go` 的当前全仓普通/race 覆盖已证明两个 DataDir 实例相互隔离；持久化失败后的跨重启恢复仍缺。该证据来自协议替身、本地 fixture 和 application/runtime 测试，不等于真实厂商 API、真实 OAuth/MCP OAuth、原生配置窗口或目标设备验收。
- 首次集成全仓失败目录 `output/acceptance/current-package-regression/integrated-streaming-background-final/` 必须保留：失败原因是证据副本位于 Go 扫描路径，被当作不完整 package 编译，出现缺失符号。修复时将后台副本移到 `output/acceptance/background-specialist/integrated/_before/`，将 `markdown-before.go` 改为 `markdown-before.go.txt`，副本字节保持不变；随后 `integrated-streaming-background-confirmed` 全仓、race、vet、fmt、package 和 codesign 通过。
- 上次 `visual-refinement-confirmed` 完整回归仍是历史包：executable `7bb8174524b12fec71a6b3bf5c653319f017394c7050b9f467e1091ebb5fd885`、代码段 `5e9391a92029c0eb3b3b8d3ae0007cbe376d1021c8a0d05e37e5d955e39da1d7`、source fingerprint `02d2ecc2f9ff000176aa644ba0570ed5643c44a05c545a227cf249b17707f27c` 只代表当时源码。round-17 流式历史包仍为 executable `b02f442a7c05ec7c6a8db313b691a0a44d914513525c314827c041fe9fa23b4b`、代码段 `45f943c874bbabc248878cf99477f2213b310684aeab59621a8cf5dc12e2c408`，两者都不能替代当前集成包身份。
- `round-18-integrated-final-completion` 与 `round-19-integrated-final-confirmation` 已在当时 integrated 包上完成两轮限定原生复核，两个 root review 均为 `PASS_FOR_RECORDED_SCOPE`：每轮同项目三会话各 30 秒、10 Hz、300 chunks，四次 streaming AX 与三次 final completion 读回均一次 attempt，最终 idle、run/journal 归属、QA PID、fixture listener 和凭据清理通过。round-18 streaming query 为 332.626/368.654/354.817/435.930 ms、final completion query 为 605.753/574.635/594.629 ms；round-19 的 click+query 聚合为 streaming 649.507/649.917/586.090/722.974 ms、final completion 917.057/918.033/880.689 ms，对应纯 query 分别为 324.125/314.041/269.957/395.728 ms 与 610.005/599.025/572.647 ms。两轮使用的历史 driver SHA-256 为 `3a2377dd9b5881841c3f88fea26a916350e9f785e406510aadba02db02c1419b`；后续工作树 driver self-test SHA-256 为 `88a08fdcfb35cec5111b0d16938c117891c6ad0982e095803cfda48226ec2624`，未重新执行 native，不能把 round-18/19 归属于后续 driver。两轮仍不证明 FPS/frame time、真实滚动效果、阅读位置恢复、真实 API 或用户视觉认可；完整 native streaming 交互仍保持部分。
- 旧 d0102a 正式包的授权允许/拒绝、提问入历史、授权等待时纠正、提问等待时纠正四个原生场景已完成限定复核，不能倒填为后续 UI 修复包实测；迟到旧回复未注入，journal 没有显式 runner 结束确认，不扩大结论。
- 用户 T3 暗黑截图是最低视觉质量标准。最新 visual-v4 工作台候选保留 `candidate` / `user_visual_acceptance: pending`，不能由构建、功能测试或内部审阅代替用户视觉认可；visual-v3/confirmed 九张原生候选与 round-18/19 截图均保留各自包身份。
- [跨平台构建](output/acceptance/platform-build/20261012-integrated/manifest.json) 已实际通过 `darwin/arm64` 与 `windows/amd64` 的只读 module 交叉编译；两项均 exit 0，root 已复核 Mach-O/PE32+ 类型、产物 SHA、CPU 头和历史 integrated 生产源码指纹 `32e08f88522775e0cfb2568393ea93371b773d31222191f4e8533b2792f13a67`，832 个 production files 构建前后未变化。该指纹属于历史 integrated 源码，不属于当前 `terminal-font-bundled` 包；两项 `native_run=NOT_RUN`、`package=NOT_BUILT`，目标设备运行、完整 bundle/原生终端资源、平台签名和性能仍未验证。
- 可选 worktree 和扩展管理页面按既定顺序在视觉基线确认后推进。当前 canonical workspace/terminal review 已通过固定范围技术复核，root 已完成终端截图技术复核；固定 glyph 已通过，但物理输入交互和用户视觉认可仍 pending，P4.3/A14 继续部分。真实厂商 API、持续流式/并发原生性能、完整系统输入和终端交互，以及 Apple Silicon/Windows 原生运行与正式打包仍未完成，根目标保持执行中。
