# GCode 桌面端执行计划

日期：2026-10-12

状态：执行中。当前 canonical 包为 `terminal-font-bundled`：executable `d9674cf38371f6ff805d81c9dd934c43e0a69eb9091896befcc24a0707349ff7`、code `ce4ca62b6b81668396404b96e3d96117073c7ea1845fdcc73bee064604812417`、production/test fingerprint `697811ee3c4fc978e76c9400aa22da01df701b82b71555cd5730fcb0321c1f14` / `7f29d17f9c809d7d2370c6c88d687d9d6799ee755c380768049438ac92654f78`，生产/测试文件 896/425，executable 26,857,584 bytes，embedded font `fe471e538392f51910faab985fa8e192a39dd3426125edd15b71b3680df0e749`，构建前后清单未变。全仓测试、受影响模块 race、vet、fmt、package 和 codesign 六项均 exit 0。用户已反馈 visual-v3 的布局和空间细节仍需调整，并明确左侧结构为“项目栏目 → 项目名称 → 多个对话”。已实现可折叠项目树及栏宽、标题、缩进、行高、共用阅读列调整；`visual-v4/confirmed` 保存 7 张实际原生截图，项目独立折叠、重新展开与跨项目对话切换 AX 检查通过，仍等待用户视觉评价。当前 canonical 的 workspace、固定 glyph 和 terminal 原生复核分别见 `output/acceptance/native-workspace/terminal-font-bundled/`, `output/acceptance/terminal-font/bundled-native/` 与 `output/acceptance/native-terminal/terminal-font-bundled/`；技术范围 review 已通过，完整终端的物理键盘/IME、clipboard、鼠标 wheel/selection、任意 Unicode、真实 API 和用户视觉认可仍 pending，P4.3/A14 继续部分。round-18/19、跨平台构建、旧项目树包和旧 terminal final 仍各自对应历史 integrated/旧包/旧源码，不倒填为新包验证。首版未通过。

产品行为以 [设计基线](../design/gcode-desktop.md) 为准。本文只管理实施顺序、改动边界和验收证据。已确认的产品决策不重复访谈；实现发现事实冲突时，记录影响并调整对应任务。

## 执行顺序

阶段 1 → 2 → 3 → 4 → 5。阶段 1 先得到原生样板，阶段 2 跑通第一条真实编码链路，然后逐步补齐首版范围。第一条链路通过不代表首版完成。

| 阶段 | 可交付结果 | 依赖 |
|---|---|---|
| 1. 接管与样板 | 可启动的 GCode 原生窗口、统一视觉组件、独立于 CLI/TUI 的内核组装 | 已确认设计、本地参考源码 |
| 2. 真实编码链路 | 一个 API 服务完成文件修改、命令、授权、停止和历史续接 | 阶段 1、可用测试 API 凭据 |
| 3. 并发与输入控制 | 同目录多个会话运行，纠正/队列及资源归属正确 | 阶段 2 的执行与存储基础 |
| 4. 完整工作台 | 文件/Diff/终端、外部编辑器、可选 worktree、完整设置及窗口生命周期 | 阶段 3 |
| 5. 首版验收 | 四类 API、扩展能力和平台验收记录、可启动应用包 | 阶段 4、对应 API 和目标设备 |

## 阶段 1：源码接管与原生样板

- [x] **1.1 固定源码来源。** 导入时复核 zero 最新 main，下载到临时目录，与本地 zero 做全树清单和内容比对，记录采用提交、本地差异与裁剪清单；补齐 LICENSE/第三方声明。将选定快照纳入 GCode，保留已有设计文件，参考仓库保持只读。源码固定、比对、21 份原始许可和终端库下载到签名的字节级来源链均有证据。见 `docs/source-origin.md`、`docs/goals/source-intake.md` 与 `docs/goals/third-party-notices.md`。
- [x] **1.2 建立可重现构建。** Go 1.27.2、仓库内相对路径 mygo 和 `cmd/gcode` 原生入口已固定。最新 `terminal-font-bundled` 全仓测试、受影响模块 race、vet、fmt、package 和 codesign 均 exit 0，manifest 记录 executable `d9674cf3...`、code `ce4ca62b6...`、production/test fingerprint `697811ee...`/`7f29d17f...`、896/425 文件、embedded font `fe471e53...` 及构建前后清单未变。application provider fixture 的四个 wire format 子场景和 terminal lifecycle 的四个 RED→GREEN fixture 已随该包回归。当前 canonical workspace、固定 glyph 和 terminal 原生复核分别位于 `native-workspace/terminal-font-bundled`、`terminal-font/bundled-native` 与 `native-terminal/terminal-font-bundled`；固定 glyph 和终端截图的 root 技术范围 review 已通过，完整终端输入交互、真实 API、设备/IME 和用户视觉仍待。旧项目树包位于新目录的 `previous-project-tree-package/GCode.app`，旧 integrated 包、round-18/19 与旧 terminal final 分开记录。见 `docs/verification/visual-v4.md`、`docs/verification/current-package.md` 和 `docs/verification/terminal-font-bundled.md`。
- [ ] **1.3 完成代表性样板。** 在实际产品视图中实现三栏、共享主题/尺度、中文消息、代码块、工具状态、纠正/队列输入、Diff 样例和设置表单。未接运行时的样例状态明确标记；组件后续直接用于真实数据。`output/acceptance/visual-v4/confirmed/` 已完成项目树与布局调整的七张深浅主题/尺寸原生截图及独立折叠/展开/跨项目选择；visual-v3 的历史证据覆盖 popup AX、Escape、中文/emoji 读回、空稿发送禁用/有稿启用、AXCloseButton 退出 0 的技术检查；视觉记录仍为 candidate，用户视觉认可 pending，物理键盘/中文 IME 未验证。
- [ ] **1.4 迁出内核组装。** 从 zero `internal/cli/app.go` 迁出工具注册、sandbox、MCP、插件、Hooks 和子代理组装，放入新增 `internal/runtime`。当前 runtime/application 已承载共享 specialist runtime 与 background manager 生命周期，`run_in_background`、`TaskOutput`、`TaskStop`、跨 parent 查询/停止和 `Service.Close` 收束已由应用层测试及当前集成回归覆盖；仍需补齐完整桌面入口、真实厂商 API/OAuth、原生扩展配置与目标设备验收，因而本计划项保持未勾选。复用现有 `internal/agent`、`internal/sessions`、`internal/execution` 等包，避免把 Agent loop 写进视图。
- [x] **1.5 完成桌面裁剪。** CLI/TUI、npm 用户入口、模型订阅认证及废弃宿主入口已移除；`cmd/gcode` 的 343 个编译依赖不引入旧包，四类 provider 均 API Key，子代理由 runtime 启动；`internal/oauth` 仅保留 MCP 所需认证。来源提示裁剪、定向测试及构建已通过独立复核，见 `docs/verification/source-closure.md`；Windows helper 文案和全仓品牌字符串不是本项的功能结论。

**完成条件：** 原生样板可在当前机器启动；视觉检查有真实窗口证据；核心组装可通过 Go 接口调用，相关核心测试通过，`go build ./cmd/gcode` 成功；桌面依赖图不再引入废弃入口。来源记录写入 `docs/source-origin.md`。视觉反馈期间可以继续独立的内核工作。

## 阶段 2：第一条真实编码链路

- [ ] **2.1 项目与模型配置。** 打开本地目录、创建会话；复用 `internal/config` 和 `internal/credstore` 的适用能力，先接通现有 API Key Chat Completions。每次任务固定工作目录、模型和权限配置，凭据通过设置页录入并与日志隔离。
- [ ] **2.2 基础会话控制。** 建立 session/run 标识、运行 context、结构化事件、授权和 Agent 提问等待。后台通过 mygo 主线程更新 UI，迟到事件必须核对所属任务。首条链路即按会话管理资源，阶段 3 再验证并发边界。
- [ ] **2.3 执行与停止。** 接通读写文件、命令、流式文本和工具过程。权限请求可批准/拒绝；停止取消请求、托管进程和子代理，收束状态后允许继续对话。
- [ ] **2.4 持久化与续接。** 复用 `internal/sessions` 追加事件和重放能力，保存完整 tool call/result 关系。重启恢复历史并标记中断；用户主动发送新消息后继续上下文。

**完成条件：** 在专用临时 Git 项目，用真实模型完成“读取文件 → 修改函数 → 运行测试 → 总结”，然后停止一个长命令、拒绝一次授权、退出重开并续接。记录请求类型、变更、命令结果及脱敏日志。协议测试替身用于自动化回归，真实 API 验收单独记录。

## 阶段 3：多会话、纠正与队列

- [ ] **3.1 资源归属。** 改造 `internal/tools/exec_command.go` 和 `internal/execution/process_manager.go`，使进程启动、查询、输入和终止校验所属会话；子代理和待授权请求跟随父任务取消。切换会话只切换显示。
- [x] **3.2 共享目录编辑（当前最低范围）。** `docs/verification/shared-edit-service.md` 与 `output/acceptance/shared-edit-service/{focused-regular,focused-race,focused-ownership-regular,focused-ownership-race,vet}.txt` 已从真实 `application.Service` 贯通 controller/runtime/Chat Completions adapter/agent.Run，验证 `edit_file`、`write_file` 同文件一方成功一方明确冲突，外部编辑造成陈旧写入拒绝，不同文件可并行；`events.jsonl`、Git 和 provider 请求归属均有严格断言。当前 canonical 包的 `native-workspace/terminal-font-bundled` 与 `native-terminal/terminal-font-bundled` review 均为 `PASS_FOR_RECORDED_SCOPE`，结合历史 native-workspace/native-terminal final 的 shell/外部编辑→预览/Diff 共享展示，当前最低范围完成。HTTP 只替代模型协议，真实厂商 API、shell/外部编辑器同时写同一路径的组合和其他平台仍属范围外。
- [ ] **3.3 纠正/队列控制。** 实现持久化消息标识、纠正优先、FIFO 队列、撤回和暂停/继续。设置页默认“纠正”，持久化用户选择；取消完成后才能启动同会话下一轮，取消失败保留消息并显示错误。
- [ ] **3.4 验证竞态。** 覆盖流式输出、长命令、授权等待、Agent 提问、已有队列和连续纠正；注入迟到回调、取消失败、退出恢复。历史 integrated 回归已把 background specialist 的跨 parent 查询/停止、in-process cancel、完成等待和 `Service.Close` 收束纳入全仓/受影响包 race；round-18/19 也各完成三路 30 秒/10 Hz/300 chunks 与受限 AX 读回。仍需真实 runner/API、迟到回复注入、FPS/frame time、完整原生滚动/阅读位置和目标设备证据。

**完成条件：** 同目录至少三个主会话同时运行，停止一个不影响其他会话；同路径旧内容竞争不会双方静默成功；默认方式重启保留；停止/失败/重启暂停队列，正常结束自动续接；已提交消息不丢失、不重复执行、旧授权失效。

## 阶段 4：完整原生工作台

- [ ] **4.1 对话与文档展示。** 完成原生 Markdown、代码高亮、选择/复制、长会话虚拟化和阅读锚点。代码级验证和 `output/acceptance/native-conversation/final/` 的旧正式包限定原生记录已覆盖 100/1000/5000 条历史末尾、CJK/emoji 输入、合成滚轮及会话位置恢复，但该 final 的 executable/code 为旧包 `597842904bb16beb7d3b3c511e3ef785885a7c32f6ecbf329c03b415018e34ec`/`0af5901132a5587d961d2bd147a2ecc3ac9cf785ea221aaad24ab2a8c8f6f22e`。历史 integrated 包的 round-18/19 review 均为 `PASS_FOR_RECORDED_SCOPE`：每轮三路 30 秒/10 Hz/300 chunks，共 900 chunks，四次 streaming AX 与三次 final completion AX 均一次 attempt，最终 session idle；两轮均未测 FPS/frame time、真实滚动效果、阅读位置恢复、物理滚轮/触控板、剪贴板、IME、大 Diff/主题组合和真实 API。round-15/16 失败证据继续保留，因此不能勾选 4.1。
- [x] **4.2 文件与项目 Diff（当前 macOS Intel）。** 工作区服务及右面板已读取当前会话实际目录，展示已跟踪/未跟踪文件、子目录、二进制提示、中文/emoji 预览和 Git 当前状态。当前 canonical `output/acceptance/native-workspace/terminal-font-bundled/{manifest.json,review.json}` review 为 `PASS_FOR_RECORDED_SCOPE`：4 张原生截图由 root 打开复核，PID `46585` 已消失，source/bundle unchanged，production files `896` 且源码未变；其余记录覆盖已跟踪/未跟踪/子目录/二进制、nested 切换、中文/emoji 预览、真实 Git Diff、TextEdit 保存和刷新预览/Diff。旧项目树包及历史 `native-workspace/final` 身份继续保留，不能替代当前包。P4.2 及 A7 的当前 macOS 14.8.7 Intel 最小闭环已通过，其他平台和完整系统输入交互继续单独验收。
- [ ] **4.3 真实终端。** 使用 mygo terminal/PTY，验证 cwd、键盘输入、resize、滚动、复制、shell 退出及释放。当前 canonical `output/acceptance/native-terminal/terminal-font-bundled/{manifest.json,results.json,review.json}` review 为 `PASS_FOR_RECORDED_SCOPE`：10 张原生截图由 root 打开复核，5 个 app/shell/child PID `48772`、`48794`、`49915`、`49944`、`50265` 均已消失，输入源恢复并独立读回为 `com.sogou.inputmethod.sogou.pinyin`；1440×900 对应 `stty size=61 63`、1280×800 对应 `54 43`，翻页 marker 分别为 `131..180→071..131→131..180`、`154..180→101..154→154..180`，shell 修改→预览/Diff 也通过。`output/acceptance/terminal-font/bundled-native/review.json` 已证明 U+E0A0/U+E0B0/U+F07C/U+F120 固定 glyph 无缺字框；`results.json` 保留机械字段 `scrollback: CAPTURED; VISUAL_REVIEW_REQUIRED` / `PASS_PENDING_VISUAL_REVIEW`，但 root 已独立打开并复核全部 10 张截图，技术范围 review 已闭环。wheel/trackpad、selection/OS clipboard、物理键盘、中文 IME、任意 Unicode、真实 API、Apple Silicon/Windows 和用户视觉认可仍未验证，因此不能勾选 4.3。
- [ ] **4.4 可选 worktree。** 改造 `internal/worktrees`，从当前 HEAD 或指定本地分支建立独立分支；展示真实 cwd，移除自动清理。完成主动合并/删除流程，检查使用中的会话和未保留修改；在临时仓库验证。
- [x] **4.5 设置与应用生命周期（已有 macOS Intel 记录）。** 多个服务/模型、发送偏好和主题设置已通过历史正式包即时保存与三次退出重开；窗口隐藏继续任务、同 PID 重开、显式退出确认、资源清理和历史保留已有原生生命周期证据。历史 `integrated-streaming-background-confirmed` 包的自动化回归、package 和 codesign 已通过；这些历史窗口场景没有在本轮后台 specialist 集成后重跑，仍以原始正式包身份引用，不能扩大为当前 canonical 包所有生命周期场景。见 `docs/verification/stage-4.md` 和 `docs/verification/native-product.md`。其他平台及 IME/完整工作台交互继续按对应条目验收。

**完成条件：** 在真实项目完成对话、文件预览、Diff、终端及外部编辑器切换；非 Git 目录可使用 Agent；worktree 不搬运源目录未提交内容，重启后仍保留；深浅主题、窄窗口、长中文内容与关闭重开均有原生验证记录。

## 阶段 5：协议、扩展与平台验收

- [ ] **5.1 四类 API。** 在 provider factory 接通通用 API Key Responses，移除订阅身份假设；完成 Chat Completions、Anthropic Messages、Gemini 验证。每类覆盖流式文本、工具调用、取消、错误、历史上下文和下次任务切换模型。
- [ ] **5.2 扩展运行能力。** 应用层已集成 MCP、Skill、Hook、插件、specialist 和 background specialist：`Task` 的 `run_in_background`、`TaskOutput`、`TaskStop`、共享/懒加载 runtime、跨 parent 查询/停止、in-process cancel、完成等待、prompt 文件清理和 `Service.Close` 收束均有本地 fixture/application/runtime 证据，历史 integrated 包全仓与受影响包 race 通过。`internal/application/background_isolation_integration_test.go` 的当前全仓普通/race 已验证两个 DataDir 实例隔离；持久化失败后的跨重启恢复仍缺。仍需分别完成真实四类厂商 API/OAuth、原生扩展配置/回调和目标设备场景；这些自动化记录不能提前勾选本项。
- [ ] **5.3 完整回归。** 当前 canonical `terminal-font-bundled` 的全仓测试、受影响模块 race、vet、fmt、package 和 codesign 均 exit 0，production/test 为 896/425，executable/code/source/test 为 `d9674cf3...`/`ce4ca62b6...`/`697811ee...`/`7f29d17f...`，embedded font 为 `fe471e53...`，构建前后清单未变；六项耗时分别为 102.097s、75.384s、1.035s、0.206s、4.502s、0.116s，完整记录见 `output/acceptance/current-package-regression/terminal-font-bundled/{package-manifest.json,results.json}`。application 四个 provider wire format fixture、terminal 四个 lifecycle RED→GREEN fixture、固定 glyph review 已纳入该身份。旧项目树包保留在 `terminal-font-bundled/previous-project-tree-package/GCode.app`；历史 `integrated-streaming-background-confirmed` 的 executable `db479d18...`、code `40d66e82...`、source/test `32e08f88...`/`a4f62825...`、832/420 文件及旧回归仍按历史身份记录，不能归入当前包。完整阶段 2—4 原生闭环、终端输入交互、真实 API、跨平台原生运行、物理输入、FPS/frame time、长会话/并发和用户视觉仍未验证，因此不能勾选 5.3。
- [ ] **5.4 应用包与设备。** 历史 integrated 源码已在 `output/acceptance/platform-build/20261012-integrated/` 记录 `darwin/arm64` 与 `windows/amd64` 的只读 module 交叉编译：两项 exit 0，分别生成 Mach-O arm64 与 PE32+ x86-64，production source fingerprint 为历史 integrated 源码 `32e08f88522775e0cfb2568393ea93371b773d31222191f4e8533b2792f13a67`，832 个 production files 构建前后未变化。两项仍为 `native_run=NOT_RUN`、`package=NOT_BUILT`；后续需分别在 macOS Intel、Apple Silicon、Windows 记录正式应用包、原生运行、系统版本、终端依赖加载、签名以及启动/内存/交互延迟。当前 canonical Intel workspace/terminal 窗口证据已单独记录，其他目标设备保持未验证状态直到实际测试。

**完成条件：** 设计基线第 10 节验收项逐项有结果；macOS 个人可用交付和随后 Windows 交付分别报告。真实凭据或设备缺失时列明未验证项并继续独立工作，不以模拟、离屏截图或交叉编译替代实测。

## 验证记录与交付规则

- 每阶段在 `docs/verification/stage-N.md` 记录实际代码版本、环境、命令、结果和未验证项；原始日志/截图存入 `output/acceptance/stage-N/`，脱敏并按需忽略生成文件。
- 完成判断同时要求可运行成果和上述场景证据。样板完成、编译通过、自动化通过、真实 API 通过、目标平台通过分别记录。
- 当前计划没有日历工期估算；阶段 1 完成后根据真实集成结果评估剩余工作，保持后续任务顺序和完成条件明确。
- 用户要求 Git 提交时按功能分组，使用中英双语描述；提交、推送状态单独记录。
