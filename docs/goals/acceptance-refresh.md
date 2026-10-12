# /goal：验收矩阵刷新与共享编辑、原生交互证据登记

状态：worktree Service、runtime-boundaries、permission-question-service、shared-edit-service、native-workspace、native-terminal、native-conversation、旧 d0102… 包的 native pending presentation、native-product final 与当前源码自动化回归证据已登记；P3.2/A6 在当前最低范围通过，旧包 pending 四场景已通过记录范围但 P2.2/P2.3/P3.4/A8 仍部分；最终 `visual-refinement-confirmed` current-package 回归已通过，身份为 executable `7bb817…`、code `5e9391…`、source fingerprint `02d2ecc…`；`visual-v3/confirmed` 的 9 张截图及 AX 技术检查已由 root 复核，但仍为 candidate、用户视觉认可 pending；native-streaming round-1/2/3 的历史失败或部分失败保持不变，round-4 driver 为 `PASS`、review 为 `PASS_FOR_RECORDED_SCOPE`，但性能为 `NOT_ACCEPTED`、滚轮为 `INJECTED_ONLY`；P4.1/A13/A16、P4.3/A14、P5.3 仍部分，整体首版仍未通过。

日期：2026-10-12（Asia/Shanghai）

## 目标

根据当前 `docs/verification/worktree-service.md`、`docs/verification/runtime-boundaries.md`、`docs/verification/permission-question-service.md`、`docs/verification/shared-edit-service.md`、`docs/verification/native-workspace.md`、`docs/verification/native-product.md`、`output/acceptance/native-terminal/final/{manifest.json,results.json,review.json}`、`output/acceptance/native-conversation/final/{manifest.json,review.json,measurement-samples.json}`、`output/acceptance/native-pending-ui/presentation-confirmed/{review.json,captures.json}`、`output/acceptance/visual-v3/confirmed/{captures.json,control-checks.json,model-popup-ax.txt,native-input.txt,native-qa.log}`、`output/acceptance/native-streaming/round-{1,2,3,4}/`、最终 current-package 回归目录、真实 Service 集成测试和已有日志，刷新共享验收矩阵中 worktree、运行时边界、共享编辑、授权/提问/纠正、长会话、持续流式、视觉样板和最终原生工作区/终端条目，使证据等级与真实实现边界一致。当前结论是：应用层 `Service` 已有真实 Git worktree、permission/ask_user/纠正/跨 session 归属和共享文件编辑冲突能力，并通过定向及包级 S2 普通/race 验证；shared-edit-service S2 加上 native-workspace/native-terminal final，使 P3.2/A6 达到当前最低范围通过；旧 d0102… pending presentation final 的四场景在 macOS Intel 范围通过记录，`NOT_INJECTED` 只表示对应的迟到回复注入场景未执行，不代表任何迟到处理已被证明，journal 还缺显式 runner end acknowledgment；最终 `visual-refinement-confirmed` current-package 的全仓普通测试、构建、签名和 desktop race 均通过，包身份为 executable `7bb817…`、code `5e9391…`、source fingerprint `02d2ecc…`；`visual-v3/confirmed` 的 9 张截图与 AX 检查已由 root 复核，但仍是 candidate，用户视觉认可 pending；native-conversation final 是旧正式包上的限定原生长会话证据，native-streaming round-1/2/3 的失败或部分失败保持历史记录，round-4 driver 与独立 review 已在 loopback/受限 AX 范围通过但 `performance_scope=NOT_ACCEPTED`、`scroll_scope=INJECTED_ONLY`，P4.1/A13/A16 仍部分；P4.3/A14、P5.3 和整体首版仍保留真实 API、输入交互、完整状态审计、完整阶段闭环、持续性能和跨平台缺口。

## 所有权与边界

- 本轮只修改 `docs/verification/acceptance-matrix.md`、`docs/verification/stage-4.md`、`docs/plans/gcode-desktop-implementation.md` 和本文件。
- 不修改产品代码、测试、原生窗口、用户数据或凭据；不提交、不推送。
- 不把 Service/Manager 证据扩大为桌面 UI 或完整首版验收。
- 不把单个平台 final 证据扩大到真实 API、原生 IME/物理键盘、Apple Silicon/Windows 或用户视觉认可。
- permission-question-service 已通过真实 `application.Service` focused 普通/race 与 `internal/application` 全包普通/race，但只支持应用层 S2 结论，不替代真实 API或完整状态审计；原生 pending presentation 由独立旧 d0102… 正式包 final 登记。
- shared-edit-service 已通过真实 `application.Service` 的同文件冲突、外部编辑陈旧拒绝和不同文件并行；结合历史 native-workspace/native-terminal final 的共享展示，P3.2/A6 只提升到当前最低范围通过，不替代真实厂商 API或同路径 shell/外部编辑器组合。
- 旧 d0102… pending presentation final 使用 executable `d0102a642e6c73f6a41b76070dce74e1a6de0fcfaaef9d1738a9a96fe805666a`、code `78cf0dc07360914dfee73bd9c7582a400fd800291f51b29f4f0254771a656876`、production source manifest `7a21a151252adc8572086ac380370a806f3ec7b224f5bf35e851fd55acaaba73`；四个场景为 `PASS_FOR_RECORDED_SCOPE`，但 `NOT_INJECTED` 只表示对应的迟到回复注入场景未执行，不代表任何迟到处理已被证明，journal 也没有显式 runner end acknowledgment。其后生产源码和 UI 已继续变更，d57597…/8c82…/060f… 仅形成中间包回归，未重跑这些 pending 场景。
- native-conversation final、native-workspace final、native-terminal final 各自保留原始历史包身份；其中长会话覆盖限定原生交互，不能把 6100 条同时加载的 RSS 当成独立历史规模曲线。
- native-workspace final 已通过其历史 Intel 正式包的文件/目录/二进制/中文 emoji/真实 Git Diff/TextEdit 保存刷新链路；它支持 P4.2 当前 macOS Intel 最低范围，A7 的终端命令→Diff 证据由 native-terminal final 单独登记，不扩展管理页面，也不倒填为后续 pending 包。
- native-terminal final 已通过其历史 Intel 正式包 Terminal AX focus、命令/cwd、PTY resize、键盘翻页、shell/app cleanup 和 shell 修改→预览/Diff 刷新；它支持 A7 当前 macOS Intel 最小范围通过，但 P4.3/A14 的 wheel/trackpad、selection/OS clipboard、物理键盘和中文 IME 仍属未验证边界，也不倒填为后续 pending 包。
- provider-save-feedback 已通过 desktop 普通/race 与正式包 final 断言；final 证据支持 P1.2、P2.1、P4.5、A9 的当前 macOS 范围，P5.4 只到 Intel 采样，因此保持部分。

## 已核验事实

### Service 与真实 Git

- `internal/application.Service` 暴露 `ListBranches`、`NewSession`、`MergeWorktree`、`RemoveWorktree`；`NewSession` 在 `Worktree=true` 时通过 controller 和 `worktrees.Manager` 创建并绑定 worktree。
- `TestServiceWorktreeDefaultBranchAndRestart` 使用临时真实 Git 仓库，验证默认会话仍使用项目根目录；指定 `feature-base` 后创建 `gcode/<session-id>` 分支，快照 `Cwd/Branch/Worktree` 与真实 Git 一致。
- 源目录 tracked 修改和 untracked 文件均不搬运到新 worktree，并继续留在源目录；`Close` 后目录保留，同一 data directory 重开可恢复 session 绑定、路径、分支和 worktree 标记。
- `TestServiceWorktreeOperationsProtectAndCloseExplicitly` 验证脏 worktree 的删除/合并拒绝、脏源目录阻止合并、失败现场保留，以及清理后显式合并和删除成功。
- `internal/worktrees/manager_test.go` 继续提供其他 session 占用保护证据；Service 公共 API 没有把同一 checkout 注入第二个 session 的入口，因此该事实没有被扩大为 Service 场景。

### 授权、提问与纠正 S2

- `docs/verification/permission-question-service.md` 记录真实 `application.Service` 进入 controller/runtime/Chat Completions adapter 的 S2：`exec_command` 的 permission allow/deny 产生对应文件结果，`ask_user` 答案进入下一次 provider 请求并保留工具历史与 `events.jsonl`。
- permission 等待和 ask_user 等待中的旧 run 收到 correction 后先完成取消，旧 reply 被拒绝，新 run 才能继续；两个 session 同时等待 permission 时，跨 session request ID 被拒绝，各自批准后分别完成对应文件。
- `output/acceptance/permission-question-service/focused-{regular,race}.txt` 的定向普通/race 与 `output/acceptance/permission-question-service/application-{regular,race}.txt` 的 `internal/application` 全包普通/race 均通过，测试结束前运行均 idle，fixture 正常关闭且输出无 key。该证据不包含生产代码变更。
- 这些证据支持 P2.2、P2.3、P3.4、A8 的具体应用层 S2 判据；旧 d0102… 原生 pending final 另补四个 Intel presentation 场景，但真实厂商 API、完整状态审计、runner 忽略取消和完整原生恢复仍未闭环。

### 共享文件编辑 S2

`docs/verification/shared-edit-service.md` 的 `TestServiceSharedEditIntegrationS2` 使用真实临时 Git 项目和真实文件，从 `application.Service` 进入 controller、runtime、Chat Completions adapter、`agent.Run` 以及生产 `read_file`、`edit_file`、`write_file`。focused 普通/race 与 ownership 普通/race 均通过，`go vet` 退出 0。

同文件竞争在 `edit_file` 和 `write_file` 两条路径都做到一方完整写入成功、另一方明确收到 `the file changed on disk since you last read it`；外部编辑造成的陈旧写入被拒绝，不同文件在同一 barrier 后均成功。测试还严格核对 `events.jsonl`、Git porcelain、每个 session 的工具参数/结果和第 3 次 provider 请求归属。结合历史 native-workspace/native-terminal final 的外部编辑器和 shell 修改→预览/Diff 共享展示，P3.2/A6 达到当前最低范围通过。

这是本地协议替身 S2；HTTP 只替代模型协议，不覆盖真实厂商 API、shell/外部编辑器同时写同一路径的完整组合、原生输入或其他平台。

### 原生授权、提问与纠正 final（旧 d0102… 包）

旧正式包的回归目录是 `output/acceptance/current-package-regression/pending-presentation-final/`，原生 captures/review 是 `output/acceptance/native-pending-ui/presentation-confirmed/{captures.json,review.json}`。包身份为 executable `d0102a642e6c73f6a41b76070dce74e1a6de0fcfaaef9d1738a9a96fe805666a`、code `78cf0dc07360914dfee73bd9c7582a400fd800291f51b29f4f0254771a656876`、production source manifest `7a21a151252adc8572086ac380370a806f3ec7b224f5bf35e851fd55acaaba73`；review 状态是 `PASS_FOR_RECORDED_SCOPE`。该场景之后生产源码和 UI 已继续修改，d57597…/8c82…/060f… 只形成中间包回归，未在后续包上重跑这些 pending 场景。

- permission allow/deny：allow 真正写入文件，deny 不写入文件。
- ask-history：回答进入下一次 provider 请求，并进入历史。
- permission-correction：旧 run 中断、新 run 启动，snapshot 和事件顺序通过；迟到旧 permission reply 为 `NOT_INJECTED`。
- ask-correction：旧 run 中断、新 run 启动，snapshot 和事件顺序通过；迟到旧 answer 为 `NOT_INJECTED`。

六个 capture（五个唯一 hash）由 root 复核，四个 QA PID 消失，fixture 凭据清理，源码与原始包未改变，场景正常退出。`NOT_INJECTED` 只表示对应的迟到回复注入场景未执行，不代表任何迟到处理已被证明；两个 correction journal 均缺 `explicit_runner_end_acknowledgment`。证据仅覆盖 macOS 14.8.7 Intel、AX、本地 loopback provider 和真实本地文件，真实 API、物理键盘/IME、剪贴板、Apple Silicon/Windows、完整状态审计和用户视觉认可仍缺。

### 长会话原生 final

`output/acceptance/native-conversation/final/{manifest.json,review.json,measurement-samples.json}` 是旧正式包的限定原生证据：executable `597842904bb16beb7d3b3c511e3ef785885a7c32f6ecbf329c03b415018e34ec`、code `0af5901132a5587d961d2bd147a2ecc3ac9cf785ea221aaad24ab2a8c8f6f22e`。root 复核 100/1000/5000 条历史末尾、6100 条同时加载、中文/emoji AX 输入、合成滚轮、切换会话后的 marker/指针恢复和正常退出；RSS 不能作为独立历史规模曲线。持续 controller 流式、真实并发、原生历史 prepend 锚点、物理滚轮/触控板、剪贴板/IME、大 Diff/主题组合和真实 API 仍待，因此 P4.1/A13/A16 保持部分。

### 持续流式原生 round-1—4

`output/acceptance/native-streaming/round-1/` 和 `round-2/` 保留历史 driver `FAIL`：round-1 未形成三路并发且正常退出失败，round-2 出现 marker 串入其他 session 且正常退出失败。round-3 的 `review.json` 状态为 `PARTIAL_DRIVER_AX_TIMEOUT`、`overall_driver_result=FAIL`：三条独立 session/run 均完成 30 秒/10 Hz、每条 300 chunks、peak=3，provider/controller、最终 idle/run_end 和退出清理通过；首个 session 的 AX 读回超时，完整 AX 轮询与合成滚轮未完成。

`output/acceptance/native-streaming/round-4/review.json` 的状态为 `PASS_FOR_RECORDED_SCOPE`、`driver_result=PASS`：三条真实 loopback 流均为 30 秒、10 Hz、每条 300 chunks（合计 900 chunks）、peak=3；三条 session/run 的 marker/token 归属、`user`/`assistant`/`run_end` 顺序、AX 尾部读回、最终历史和清理均通过。三个 session 最终均为 idle 且 `active_run == nil`，菜单退出 0、PID 消失、隔离凭据目录不存在，当前包 code/resources/source 身份与回归 manifest 一致。`captures/streaming-window.png` 和 `captures/final-window.png` 也已由 root 复核，但 review 明确不声明用户视觉认可。

round-4 的 `performance_scope` 为 `NOT_ACCEPTED`：四次 AX 往返约为 `27686.087632ms`、`1663.861772ms`、`760.924816ms`、`876.567313ms`，首个 AX 约 27.686 秒并同步阻塞周期采样，只留下 2 个 `controller_poll` 和 1 个 process sample（`66572 KiB / 17.8%`）；因此不能证明持续流式交互性能、响应阈值、并发任务内存/延迟，也不能证明所有 AX/scroll 操作都发生在三条流同时 running 期间。`scroll_scope` 为 `INJECTED_ONLY`，虽指针恢复，但没有 viewport 前后效果、scroll 后 AX 读回或运行状态时间戳断言，不证明实际滚动效果、阅读位置恢复或物理滚轮/触控板行为。真实厂商 API、物理键盘/IME、剪贴板、完整终端交互、Apple Silicon、Windows 和用户视觉认可仍未验证。

### 可重放日志

以下四条 worktree 定向命令均退出 0：

```sh
go test ./internal/application -run 'TestServiceWorktree' -count=1 -v
go test ./internal/worktrees -count=1 -v
go test -race ./internal/application -run 'TestServiceWorktree' -count=1
go test -race ./internal/worktrees -count=1
```

原始摘要位于 `output/acceptance/worktree-service/regular.log`、`race.log` 和 `git-checks.txt`。这些证据使用临时真实 Git 仓库和真实 `application.New`，可支持相应 Service 应用层 S2 结论。

此前额外执行 `go test ./internal/application ./internal/worktrees -count=1` 时，HTTP fixture 曾在 `httptest.Server blocked in Close after 5 seconds, waiting for connections` 连接等待处超过 170 秒后中止。本轮复现后由 runtime-boundaries 修复 fixture 的释放信号；没有独立堆栈证据把初次挂起归因于 worktree 实现。当前全包普通和 race 已通过，原始输出为：

- `output/acceptance/runtime-boundaries/application-worktrees-regular.txt`：application 20.589s，worktrees 2.643s。
- `output/acceptance/runtime-boundaries/application-worktrees-race.txt`：application 33.705s，worktrees 3.744s。

本轮复现的 fixture 修复消除了当前挂起；最终 `visual-refinement-confirmed` current-package 目录明确记录了全仓普通 `go test ./...`、go vet、fmt-check、package、codesign、`go test -race ./internal/desktop` 和 fingerprint 结果，均 exit 0；native-product 与 core-race 仍保留各自原始证据范围。真实 API、跨平台设备和用户视觉认可仍不在这些回归范围内。

## Runtime 边界更新

`docs/verification/runtime-boundaries.md` 与 `output/acceptance/runtime-boundaries/focused-{regular,race}.txt` 记录六项真实 `application.Service` 场景均通过：

- 固定凭据不会进入 Service 快照、`controller.json`、session `events.jsonl` 或临时数据目录。
- `Send` 在持久化失败时回滚，恢复后只执行一次，重开不重复执行已完成任务。
- `Stop` 在持久化失败时保留原运行，恢复后才收束为中断。
- `SaveSettings` 在持久化失败时回滚，恢复路径后保存成功。
- provider `HasAPIKey` 从凭据存储恢复，重开后有密钥/无密钥状态分别正确。
- `Close` 即使持久化失败仍取消活动运行并清理真实 shell，PID 消失且完成标记不生成。

这些是本地 HTTP fixture、真实 Service、临时数据目录和真实 shell 的应用层 S2 证据；`provider-save-feedback.md` 与 native-product final 已补充当前 macOS 范围的桌面保存反馈和正式包回归。整体仍不替代真实厂商 API、完整桌面交互或目标设备验收。

## Native-workspace final 边界

`docs/verification/native-workspace.md` 与 `output/acceptance/native-workspace/final/{results.json,manifest.json}` 记录当前 macOS 14.8.7 Intel 正式包的真实工作区链路：文件列表包含 tracked/untracked、子目录和二进制；nested 切换、中文/emoji 预览、真实 tracked/untracked Diff 与 `git diff` 一致；产品“编辑”打开唯一 TextEdit，AX 修改后由原生 Command+S 保存，随后只关闭该窗口并刷新 GCode 预览和 Diff。`results.json` 的文件/目录/二进制、Git Diff、TextEdit 保存、预览/Diff 刷新和正常退出均为 PASS，manifest 记录原包与源码未改变。

结合阶段 2/3 非 Git 项目和路径边界的应用层 S2，P4.2 在当前 Intel 范围通过，计划 4.2 已勾选；结合下节 native-terminal final，A7 也在当前 Intel 最小范围通过。该 native-workspace final 本身不覆盖终端命令→Diff、真实厂商 API、物理键盘/IME、系统剪贴板、其他平台或用户视觉认可。

## Native-terminal final 边界

`output/acceptance/native-terminal/final/{manifest.json,results.json,review.json}` 记录当前 macOS 14.8.7 Intel 正式包的终端收口。Terminal 通过 AX group 定位并获得焦点；合成原生键盘输入/Return 后，真实 shell 写出命令、cwd 和 PID，cwd 与同会话 QA 项目一致。shell 修改文件后，预览和真实 Git Diff 均读到唯一 `QA_SHELL_EDIT_...` 新增行。

1440×900 对应 `stty size=51 68`，1280×800 对应 `45 49`；两种尺寸均执行 Shift+PageUp/PageDown，`review.json` 复核旧序号出现并恢复底部提示符。正常 shell `exit` 后 PID 消失，第二次启动中的 `sleep 300` 子进程随应用菜单退出清理；记录的五个 app/shell/child PID `30017`、`30989`、`30033`、`31018`、`31420` 均已消失。输入源恢复为 `com.sogou.inputmethod.sogou.pinyin`，源码和原始包未改变，剪贴板未操作。

上述文件/外部编辑器与终端命令→Diff 共享变更闭环支持 A7 在当前 macOS 14.8.7 Intel 最小范围通过。`results.json` 仍将 scrollback 标为 `CAPTURED; VISUAL_REVIEW_REQUIRED`；P4.3/A14 仍为部分，wheel/trackpad、selection/OS clipboard、物理键盘、中文 IME、真实 API、Apple Silicon/Windows 和用户视觉验收未验证。

## Native-product final 边界

`output/acceptance/native-product/final/results.json` 记录正式隔离包三次启动退出均为 0，主题、默认队列、双服务、重开和凭据状态通过；`final/manifest.json` 记录 `per_message_choice_preserves_global_default=true`、加密假 key 和应用日志无 key。`docs/verification/provider-save-feedback.md` 记录保存当下 dirty draft 密钥反馈的 desktop 普通/race 通过，正式包 final 断言也通过。当前证据支持 P1.2、P2.1、P4.5、A9 在 macOS 14.8.7 Intel 范围的最低判据；P5.4 仅有 Intel 采样，Apple Silicon/Windows 保持缺口。

## 当前全仓回归

`output/acceptance/current-package-regression/visual-refinement-confirmed/` 记录最终 current-package executable `7bb8174524b12fec71a6b3bf5c653319f017394c7050b9f467e1091ebb5fd885`、code `5e9391a92029c0eb3b3b8d3ae0007cbe376d1021c8a0d05e37e5d955e39da1d7`、source fingerprint `02d2ecc2f9ff000176aa644ba0570ed5643c44a05c545a227cf249b17707f27c` 的 `go test ./... -count=1 -timeout=180s`、`go vet ./...`、`make fmt-check`、`make package`、codesign、`go test -race ./internal/desktop -count=1 -timeout=180s` 和 fingerprint 校验，均 exit 0，最终包身份已冻结。`output/acceptance/current-package-regression/visual-refinement-final/` 仅保留 d57597… 中间包回归记录。`output/acceptance/current-package-regression/pending-presentation-final/` 及 `native-pending-ui/presentation-confirmed/` 只属于旧 d0102… 包，后续源码/UI 已变更，未在 d575 或最终包上重跑 pending presentation。`output/acceptance/native-product/regression/` 是此前正式包回归，不能覆盖最终包身份。`runtime-boundaries` 日志复用 controller/runtime/application/worktrees race，`output/acceptance/permission-question-service/application-{regular,race}.txt` 补充 `internal/application` 全包普通/race，`provider-save-feedback.md` 记录 desktop 普通/race。自动化命令与这些 S2 证据均不覆盖真实厂商 API、全部阶段 2—4 原生场景、Apple Silicon/Windows、物理输入、剪贴板、streaming 和用户视觉认可，因此 P5.3 仍为部分。

## 矩阵结论

- P4.4：保持“部分”。Service/Manager/controller 的真实 Git worktree 行为已具备 S2 定向证据；桌面 UI 创建、分支选择、合并、删除入口仍缺。
- A17：保持“部分”。源目录隔离和 session/Git 绑定已由 Service S2 证明，桌面创建/绑定流程仍未验收。
- A18：保持“部分”。Service 关闭/重开保留及重开后显式删除已通过，Manager/controller 另有占用和脏状态保护；桌面入口仍缺。
- appmodel `cwd/worktree` 合同：`Cwd/Branch/Worktree` 的当前 Service 行为已经有证据，但来源分支、恢复原因和占用快照仍未进入合同；不借此提升为“通过”。
- P3.2/A6：shared-edit-service S2 已证明同文件冲突、外部编辑陈旧拒绝和不同文件并行，结合历史 workspace/terminal final 的共享展示，当前最低范围为“通过”；HTTP 只替代模型协议。
- P2.2/P2.3/P3.4/A8：permission-question-service 的真实 Service focused 普通/race、纠正失效、跨 session request ownership 和 `internal/application` 全包普通/race 已补入；旧 d0102… pending presentation 又补 allow/deny、ask-history 和两个 correction 的原生记录，但这些项仍保持“部分”，`NOT_INJECTED` 只表示对应迟到回复注入场景未执行，不代表迟到处理已被证明，journal 也缺显式 runner end acknowledgment，真实 API、长链路状态审计和完整原生恢复仍缺。
- P4.2：native-workspace final 加上阶段 2/3 非 Git 与路径边界 S2，当前 macOS 14.8.7 Intel 最低范围通过；计划 4.2 已勾选。
- A7：当前 macOS 14.8.7 Intel 最小范围“通过”。native-workspace 与 native-terminal final 已证明 TextEdit/终端命令共享修改后预览和 Diff 正确刷新，终端新增行与真实 `git diff` 一致；其他平台和完整系统输入交互由独立条目约束。
- P4.1/A13/A16：`native-conversation/final` 的旧正式包限定原生记录已补入 100/1000/5000 条末尾、6100 条同时加载、CJK/emoji、合成滚轮、会话 marker/指针恢复和 AX 采样；round-4 driver/review 又证明三路 loopback 流、session/run 归属、受限 AX 读回和清理，但 `performance_scope=NOT_ACCEPTED`，AX 阻塞只留下 2 个 controller_poll 与 1 个 process sample，`scroll_scope=INJECTED_ONLY` 且无滚动效果断言，因此持续流式性能、真实并发内存/延迟、原生历史 prepend 锚点和完整滚轮行为仍缺，保持“部分”。
- P4.3/A14：保持“部分”。native-terminal final 已补齐 Terminal AX focus、命令/cwd、PTY resize、键盘翻页、shell/app cleanup 和 shell 修改→预览/Diff；wheel/trackpad、selection/OS clipboard、物理键盘、中文 IME 仍缺。
- P5.3：保持“部分”。最终 `current-package-regression/visual-refinement-confirmed` 的 executable `7bb817…`、code `5e9391…`、source fingerprint `02d2ecc…` 的全仓普通测试、`go vet`、fmt-check、package、codesign、`go test -race ./internal/desktop` 和 fingerprint 均通过；round-4 driver/review 在 loopback/受限 AX 范围为 `PASS_FOR_RECORDED_SCOPE`，但性能为 `NOT_ACCEPTED`、滚轮为 `INJECTED_ONLY`。旧 d0102… pending presentation 未在后续源码/UI变更后重跑；全部阶段 2—4 原生场景尚未闭环，真实 API、跨平台、物理输入、长会话真实并发、持续性能和完整状态审计仍阻断本项。
- A10：`output/acceptance/visual-v3/confirmed/` 的 9 张截图已由 root 打开复核；模型 popup AX open/Escape close、中文/emoji 读回、空稿 send disabled、有稿 send enabled 和 QA AXCloseButton 正常退出 0 均有记录。视觉证据仍为 candidate，用户视觉认可 pending；`physical_input_ime=NOT_TESTED`，旧 `output/acceptance/visual-v2/closure/dark-1440x900-native.png` 与 `docs/verification/visual-v2.md` 只作历史技术证据。
- 运行时边界：六项真实 Service focused regular/race、application/worktrees 包级普通/race 和最终 `visual-refinement-confirmed` 全仓回归已通过；此前 HTTP fixture 挂起已由本轮复现的释放信号修复消除。最终回归仍不扩大为真实厂商 API、跨平台设备或用户视觉验收。
- P1.2、P2.1、P4.5、A9：按最低判据在当前 macOS 14.8.7 Intel 范围通过；P5.4：Intel 采样通过，Apple Silicon/Windows 缺证，保持部分。
- 总判定：首版保持未通过。最终 `visual-refinement-confirmed` current-package 回归已通过，身份为 executable `7bb817…`、code `5e9391…`、source fingerprint `02d2ecc…`；`visual-v3/confirmed` 的 9 张截图与 AX 交互技术检查已由 root 复核，但仍为 candidate，用户视觉认可 pending。round-4 streaming driver/review 仅在 loopback/受限 AX 范围达到 `PASS_FOR_RECORDED_SCOPE`，性能为 `NOT_ACCEPTED`、滚轮为 `INJECTED_ONLY`；round-1/2/3 历史失败或部分失败保持记录。物理键盘/中文 IME 未验证，旧 visual-v2 截图不能等同用户认可；真实 API、跨平台、完整阶段 2—4 原生闭环、持续性能、长会话/并发性能和其他未闭环条目仍阻止首版整体通过。

## 范围外待办

`docs/goals/native-product.md` 的正式包 final 与 regression、`docs/verification/native-workspace.md` 的历史 Intel 文件工作区链路、`output/acceptance/native-terminal/final/` 的历史终端收口、`output/acceptance/native-conversation/final/` 的限定长会话原生记录、`output/acceptance/native-streaming/round-{1,2,3,4}/` 的持续流式记录、旧 d0102… 包的 `output/acceptance/native-pending-ui/presentation-confirmed/`、`output/acceptance/visual-v3/confirmed/` 的技术视觉证据和 `docs/verification/permission-question-service.md` 的应用层 S2 已有证据；后续仍需完成真实厂商 API、迟到回复注入/runner end acknowledgment、完整状态审计、持续性能阈值、Apple Silicon/Windows、用户视觉认可和原生 IME/物理键盘/wheel/trackpad/selection/clipboard 边界。桌面 worktree 入口、P4.3/A14 剩余终端交互、P5.3 全阶段原生闭环、完整流式 AX/滚轮效果、真实并发长会话和用户视觉认可仍是首版缺口。
