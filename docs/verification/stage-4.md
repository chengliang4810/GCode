# 阶段 4：原生工作台与应用生命周期

记录日期：2026-10-12。环境为 macOS 14.8.7（23J520），Intel `darwin/amd64`，Go 1.27.2。当前仓库尚无 Git 提交，以下证据对应现场源码和隔离 QA 应用，不扩大为完整首版验收。

## 真实原生窗口生命周期

使用实际 `cmd/gcode` 构建独立应用身份，通过 linker 指定 `mygo.packageName` 和 `mygo.packageIdentifier`，分别写入 `GCodeLifecycleQA20261012C`、`GCodeLifecycleQA20261012D` 数据目录。生产 GCode 数据未用于本场景。模型端为本地 HTTP Chat Completions 替身；应用、provider、controller、runtime、POSIX 命令和原生窗口均为真实实现。

夹具：`output/acceptance/stage-4/_lifecyclefixture/main.go`。记录和截图目录：`output/acceptance/stage-4/lifecycle/`。

### 运行中退出确认和资源清理

QA C 构建命令：

```sh
go build -buildvcs=false \
  -ldflags '-X github.com/egoist/mygo.packageName=GCodeLifecycleQA20261012C -X github.com/egoist/mygo.packageIdentifier=app.gcode.desktop.lifecycleqa-c' \
  -o output/acceptance/stage-4/gcode-lifecycleqa-c ./cmd/gcode
```

从真实原生输入区发送任务，执行 `printf started > quit.started; sleep 300; printf done > quit.done`，随后通过原生菜单触发“退出 GCode”。

- 首次出现一张确认 sheet，按钮为“取消”“停止并退出”，说明会停止任务并保留历史、待执行消息和工作目录。OS 截图为 `quit-c-confirm-0.png`、`quit-c-confirm-1.png`。
- 点击“取消”后，应用 PID 38576、托管 shell PID 38604 和子进程 PID 38605 继续运行；会话仍为 `running`，`active_run` 保留。证据为 `cancel-preserves-processes.txt`、`cancel-preserves-run.json`。
- 再次触发退出并点击“停止并退出”，应用返回 exit 0，以上三个 PID 均消失，`quit.done` 不存在；provider HTTP context 收到取消。证据为 `quit-cleans-processes.txt`、`fixture-c.log`。
- 会话持久化为 `interrupted`，`active_run` 清除，队列暂停并记录“任务已停止，队列已暂停”。证据为 `quit-final-state.json`。
- 同目录的独立外部 shell 正常产生 `independent-c.done`，表明本次清理没有终止该外部进程。
- 重开 QA C 后，原生界面显示“已中断”，没有自动重新执行该命令；之后由用户动作发起了新的独立 QA 任务。

第一轮 QA B 的长请求夹具没有及时 flush HTTP headers，120 秒后触发真实 HTTP header timeout；其后退出发生于空闲状态。该轮不用于证明运行中确认退出。QA C 夹具增加 SSE keepalive 和 flush 后重新取得上述证据，未更改产品超时策略。

### 关闭窗口继续任务和同 PID 重开

QA D 使用同一真实产品入口及独立身份。驱动 `output/acceptance/stage-4/lifecycle/hide-qa.py` 在一个连续场景内完成输入、关闭、后台完成、重开和空闲退出，避免短命令在关闭前已完成。

- 真实命令开始后点击系统 `AXCloseButton`。应用 PID 50414 保持存活，CGWindow 清单只剩 layer 25 的托盘窗口 21648，主窗口已隐藏。
- 主窗口隐藏期间，12 秒命令完成并产生 `hide.done`。
- 再启动相同 binary，第二实例 exit 0；主窗口 21647 由原 PID 50414 恢复，会话状态为 `idle`、没有 `active_run`。
- 随后通过原生菜单空闲退出，应用 exit 0。

证据：`hide-qa.log`、`hide-reopen-evidence.json`、`hidden-completed-reopened-d.png`。截图使用 macOS `screencapture -l`，包含系统窗口按钮。驱动完整退出，没有保留该 QA 应用进程。

## 长会话原生 final（历史包身份）

`output/acceptance/native-conversation/final/{manifest.json,review.json,measurement-samples.json}` 是旧正式包上的限定原生记录，不能倒填为后续 pending 正式包。该 final 的 executable SHA-256 为 `597842904bb16beb7d3b3c511e3ef785885a7c32f6ecbf329c03b415018e34ec`，代码段 SHA-256 为 `0af5901132a5587d961d2bd147a2ecc3ac9cf785ea221aaad24ab2a8c8f6f22e`，`review.json` 为 `PASS_FOR_RECORDED_SCOPE`。

root 已复核五张 capture：100/1000/5000 条历史末尾显示、5000 条上移后阅读位置、切换会话后 marker 恢复和指针位置恢复；AX 中文/emoji 输入逐字读回，合成滚轮能够移动实际历史，正常菜单退出 exit 0，源码与原始包未改变。三个规模场景共加载 6100 条历史，RSS 采样不能作为独立历史规模曲线。

该记录仍不覆盖持续 controller 流式、真实并发、原生历史 prepend 锚点、物理滚轮/触控板惯性、系统剪贴板、中文 IME、大 Diff/主题组合、真实厂商 API、其他平台或用户视觉认可，因此 P4.1/A13/A16 继续为部分。

## 正式包文件工作区与外部编辑器

`docs/verification/native-workspace.md` 与 `output/acceptance/native-workspace/final/` 记录历史 Intel 正式包的隔离工作区场景。该 final 的 source executable SHA-256 为 `f1269b5c8a132a898565c9e06ab4660b389163158e814ee29986ac1f69b3a7b8`，executable code SHA-256 为 `d792ee8083451dc9b1a027f9f973edfe6f8784bd9aac0591731ad56609ad4fb5`，不能倒填为后续 pending 包。文件列表包含已跟踪、未跟踪、子目录和二进制文件；进入 nested 目录、返回项目根目录、中文/emoji 预览和二进制提示均通过。真实 tracked/untracked Diff 与 `git diff` 一致。

产品的“编辑”入口打开唯一 QA 文件的 TextEdit 窗口；驱动通过精确 `AXFocusedWindow` 修改内容，发送原生 Command+S 后逐字校验磁盘和保存 SHA，只关闭该 QA 窗口而不退出 TextEdit。返回 GCode 刷新后，预览和 Diff 均显示保存后的内容，并与 `git-after-edit.txt` 和两张原生图一致。`results.json` 的 `files_preview_directory_binary`、`real_git_tracked_untracked_diff`、`external_textedit_saved`、`preview_and_diff_refresh` 和 `normal_exit` 均通过，manifest 记录 `source_unchanged=true`、`original_bundle_unchanged=true`、`qa_exit_code=0`。

结合阶段 2/3 的非 Git 项目与路径边界应用层 S2，P4.2 在当前 macOS 14.8.7 Intel 范围达到最低通过条件。该工作区证据本身不覆盖终端命令→Diff，终端收口见下节；它也不扩展到真实厂商 API、其他平台、物理键盘/IME、系统剪贴板或用户视觉验收。

## 原生终端与共享变更收口

`output/acceptance/native-terminal/final/{manifest.json,results.json,review.json}` 是历史 Intel 正式包和隔离 QA 项目的终端证据，source executable SHA-256 为 `f1269b5c8a132a898565c9e06ab4660b389163158e814ee29986ac1f69b3a7b8`，code SHA-256 为 `d792ee8083451dc9b1a027f9f973edfe6f8784bd9aac0591731ad56609ad4fb5`，资源逐项 hash 仍以原 manifest 为准，不能倒填为后续 pending 包。Terminal 通过 AX group 定位并获得焦点；合成原生键盘输入/Return 后，真实 shell 写出命令结果、cwd 和 PID，cwd 与同会话 QA 项目一致。shell 修改文件后，GCode 刷新预览和 Diff，唯一 `QA_SHELL_EDIT_...` 行作为新增第三行出现，和外部 TextEdit 修改共同保留。

窗口 1440×900 时 PTY 为 `stty size=51 68`，1280×800 时为 `45 49`。两种尺寸都执行了 Shift+PageUp/PageDown，`review.json` 由 root 复核旧序号出现并恢复到底部提示符；正常 shell `exit` 后 PID 消失，第二次启动中的 `sleep 300` 子进程随应用菜单退出清理。记录的五个 app/shell/child PID `30017`、`30989`、`30033`、`31018`、`31420` 均已消失，输入源恢复为 `com.sogou.inputmethod.sogou.pinyin`，源码和原始包未改变，且未操作剪贴板。

上述文件/外部编辑器与终端命令→Diff 共享变更闭环，使 A7 在当前 macOS 14.8.7 Intel 最小范围通过。P4.3/A14 仍为部分：wheel/trackpad、selection/OS clipboard、物理键盘和中文 IME 未验证；`results.json` 的 `scrollback` 仍标记为 `CAPTURED; VISUAL_REVIEW_REQUIRED`，真实 API、Apple Silicon/Windows 和用户视觉验收也不在本轮范围内。

## 原生授权、提问与纠正 final（旧 d0102… 包）

旧正式包的回归位于 `output/acceptance/current-package-regression/pending-presentation-final/`，原生 pending 证据位于 `output/acceptance/native-pending-ui/presentation-confirmed/{review.json,captures.json}`。包身份为 executable SHA-256 `d0102a642e6c73f6a41b76070dce74e1a6de0fcfaaef9d1738a9a96fe805666a`、代码段 SHA-256 `78cf0dc07360914dfee73bd9c7582a400fd800291f51b29f4f0254771a656876`，生产源码清单 SHA-256 `7a21a151252adc8572086ac380370a806f3ec7b224f5bf35e851fd55acaaba73`。`review.json` 的总状态为 `PASS_FOR_RECORDED_SCOPE`，不是完整阶段 4 或首版通过声明。该场景之后生产源码和 UI 已继续修改，d57597…/8c82…/060f… 仅形成中间回归包，任何上述 pending 结果都不能倒填为后续包实测。

四个场景均通过记录范围：

- `permission-allow-deny`：allow 真正写入本地文件，deny 不产生文件。
- `ask-history`：回答进入下一次 provider 请求，并进入历史。
- `permission-correction`：旧 run 中断、新 run 启动，snapshot 和 journal event order 通过；迟到旧 permission reply 为 `NOT_INJECTED`。
- `ask-correction`：旧 run 中断、新 run 启动，snapshot 和 journal event order 通过；迟到旧 answer 为 `NOT_INJECTED`。

六个 capture（五个唯一 hash）均由 root 复核，四个 QA PID 均消失；fixture 凭据已清理，数据证据已复制，源码和原始包未改变，四个场景均正常退出。`NOT_INJECTED` 只表示对应的迟到回复注入场景未执行，不代表任何迟到处理已被证明；两个 correction 场景的 journal 均为 `explicit_runner_end_acknowledgment: NOT_EXPLICIT_IN_JOURNAL`。

证据范围是 macOS 14.8.7 Intel、AX 控件、本地 loopback provider 和真实本地文件。真实厂商 API、物理键盘/IME、系统剪贴板、Apple Silicon、Windows、完整状态审计和用户视觉认可仍未覆盖；因此 P2.2/P2.3/P3.4/A8 与 P5.3 仍保持部分。

## 当前包回归基线（最终包，已确认）

`output/acceptance/current-package-regression/visual-refinement-confirmed/` 记录最终 current-package：executable SHA-256 `7bb8174524b12fec71a6b3bf5c653319f017394c7050b9f467e1091ebb5fd885`、代码段 SHA-256 `5e9391a92029c0eb3b3b8d3ae0007cbe376d1021c8a0d05e37e5d955e39da1d7`、source fingerprint `02d2ecc2f9ff000176aa644ba0570ed5643c44a05c545a227cf249b17707f27c`。该目录的全仓普通 `go test ./... -count=1 -timeout=180s`、`go vet ./...`、`make fmt-check`、`make package`、codesign、`go test -race ./internal/desktop -count=1 -timeout=180s` 和 fingerprint 校验均 exit 0，最终包身份已冻结。此前 d57597…/8c82…/060f… 仅保留在 `visual-refinement-final/` 作为中间包历史记录。

`output/acceptance/visual-v3/confirmed/` 已保存 9 张原生截图；root 已打开复核。`control-checks.json` 记录模型 popup 真实 AX 打开、Escape 关闭后焦点回到输入区、空稿 send disabled、有稿 send enabled、中文/emoji AX 读回和 QA AXCloseButton 正常退出 0；`captures.json` 保存截图 hash，`model-popup-ax.txt`、`native-input.txt` 和 `native-qa.log` 提供交互记录。视觉证据仍标记为 candidate，用户视觉认可保持 pending；`physical_input_ime=NOT_TESTED`，不扩展为物理键盘或中文 IME 通过。streaming round-4 driver 与独立 review 已记录为 `PASS_FOR_RECORDED_SCOPE`，但 performance 为 `NOT_ACCEPTED`、scroll 为 `INJECTED_ONLY`，仍受 loopback、轻量 AX 和合成事件范围限制，不能从视觉复核或最终包回归推定完整 streaming 原生验收通过。

## 持续流式原生 round-1—4

`output/acceptance/native-streaming/round-1/` 和 `round-2/` 保留历史 driver `FAIL`：round-1 未形成三路并发且正常退出失败，round-2 出现 marker 串入其他 session 且正常退出失败。round-3 的 `review.json` 状态为 `PARTIAL_DRIVER_AX_TIMEOUT`、`overall_driver_result=FAIL`：三条独立 session/run 均完成 30 秒/10 Hz、每条 300 chunks、peak=3，provider/controller、最终 idle/run_end 和退出清理通过；首个 session 的 AX 读回超时，完整 AX 轮询与合成滚轮未完成。

round-4 的 driver 和 `review.json` 已由 root 独立复核，review 状态为 `PASS_FOR_RECORDED_SCOPE`、`driver_result=PASS`：三条真实 loopback 流均为 30 秒、10 Hz、每条 300 chunks（合计 900 chunks）、peak=3；三条 session/run 的 marker/token 归属、`user`/`assistant`/`run_end` 顺序、AX 尾部读回、最终历史和清理均通过。三个 session 最终均为 idle 且 `active_run == nil`，菜单退出 0、PID 消失、隔离凭据目录不存在，当前包 code/resources/source 身份与回归 manifest 一致。合成滚轮范围仅为 `INJECTED_ONLY`，指针恢复，但没有 viewport 前后效果、scroll 后 AX 读回或运行状态时间戳断言，因此不证明实际滚动效果、阅读位置恢复或物理滚轮/触控板行为。

round-4 的四次 AX 往返分别约为 `27686.087632ms`、`1663.861772ms`、`760.924816ms`、`876.567313ms`；首个 AX 约 27.686 秒并同步阻塞周期采样，只留下 2 个 `controller_poll` 和 1 个 process sample（`66572 KiB / 17.8%`）。review 将 `performance_scope` 标为 `NOT_ACCEPTED`，所以该轮只证明 driver-scope 的流式归属、收束和受限原生操作，不证明持续流式交互性能、响应阈值，也不能证明所有 AX/scroll 操作都发生在三条流同时 running 期间。用户视觉认可仍 pending；真实厂商 API、物理键盘/IME、系统剪贴板、完整终端交互、Apple Silicon 和 Windows 仍未验证。

## 其他阶段 4 证据

- 历史 Intel 正式包的双服务、主题/默认发送方式、即时保存提示、三次退出重开和运行采样：`docs/verification/native-product.md`。最终包的全仓普通测试、desktop race、应用包构建/签名回归已通过，但没有重新执行这些原生生命周期场景；原生设置 UI 和持久化文件结论继续绑定其原始包身份。d57597… 中间包记录保留在 `visual-refinement-final/`。
- 非 Git 目录和嵌套仓库路径的应用层证据：`docs/verification/stage-2.md`、`docs/verification/stage-3.md`、`docs/goals/workspace-tools.md`；上面的正式包工作区和 native-terminal final 已补齐当前 Intel 的真实文件/Git/Diff/外部编辑器/终端命令共享变更链路，A7 当前 Intel 最小范围通过。
- 长会话虚拟化、底部跟随、上滑保持、历史锚点：`docs/verification/stage-4-conversation.md` 与上面的 `native-conversation/final`。代码级测试和限定旧包原生交互均有记录；持续 controller 流式、真实并发、原生历史 prepend 锚点和独立性能曲线仍待。
- 真实 PTY、中文/emoji、resize、shell 退出、关闭后 PID 释放：`docs/goals/terminal-runtime.md` 与 `output/acceptance/native-terminal/final/{results.json,review.json}`。终端 wheel/trackpad、selection/OS clipboard、物理键盘、中文 IME 与完整窗口交互仍待。

## 尚未完成

- 用户确认代表性原生样板方向；worktree 与扩展管理页面仍属后续范围，本轮不宣称完成。
- worktree 创建、分支选择、显式合并/删除的桌面入口及窗口验收。
- 原生 Markdown 完整能力、streaming 的完整 AX/滚轮交互、真实并发长会话、原生历史 prepend 锚点和输入法组合/候选位置。
- 终端 wheel/trackpad、selection/OS clipboard、物理键盘、中文 IME 的真实窗口交互；A7 当前 Intel 的终端命令→Diff 记录已收口。
- pending correction 的真实迟到回复注入/runner end acknowledgment、完整状态审计；当前 `NOT_INJECTED` 只表示对应注入场景未执行。
- 真实厂商 API、Apple Silicon、Windows 分别验收；不能从当前 Intel 设置/恢复及构建回归推定这些结果。
- streaming round-4 driver/review 已在 loopback/受限 AX 范围达到 `PASS_FOR_RECORDED_SCOPE`，但 performance 为 `NOT_ACCEPTED`、scroll 为 `INJECTED_ONLY`；用户视觉认可、完整 AX/滚轮行为和物理键盘/中文 IME 仍未完成。
