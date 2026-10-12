# 正式包持续流式与同项目多会话原生验收

日期：2026-10-12。round-18-integrated-final-completion 和 round-19-integrated-final-confirmation 已在同一确认正式包、同一 driver 和同一三会话负载上连续达到严格 completion 规则的 `PASS_FOR_RECORDED_SCOPE`；round-17-parsed-prefix 仍是旧 tail-token 规则下的限定通过，round-15-selection-fixed 和 round-16-ax-role-tree 仍失败。两轮重复通过只确认当前记录范围的可复现性，不构成整体性能或所有原生交互通过。未统计 FPS/frame time，未证明滚动效果、阅读位置恢复或视觉交付，整体性能结论仍未通过。

## 当前状态

round-4 之前的驱动语法检查和纯函数自测已通过；round-4 使用新的 `output/acceptance/native-streaming/round-4/` 目录完成原生窗口、loopback fixture、AX 读回、controller snapshot、journal、截图和退出清理。round-4 的结果文件为 PASS，但 AX 读回是同步操作，测量样本不足以支持性能结论；合成滚轮也没有留下 viewport 或阅读位置对比。

round-5 驱动已加入只读诊断反馈循环。点击单独受 12 秒预算限制，点击完成后 tail-query 使用 18 秒严格外层预算，每次 last-row 查询以剩余预算限幅；它不修改生产源码，不增加任何预算，不改变 `last row` 查询和 marker/tail/foreign 断言。每个成功 AX attempt 只保存已知 fixture token 的 `own_marker_visible`、`foreign_markers`、`foreign_tail_tokens` 和 `text_bytes`，不再落盘原始 AX UI 文本。失败后、正常清理前会尝试通过当前 QA PID 保存 `captures/failure-window.png`，记录 PID、CGWindow 和 SHA-256；capture 异常写入独立 `diagnostic_errors`/`failure_window_capture`，不能覆盖 `primary_error` 或改变清理错误语义。目标是记录首个 AX 延迟的分段时序，不能把诊断读取错误写成生产故障或性能结论。

round-17 使用 parsed-prefix 版本的正式包执行，运行参数仍为三会话、30 秒、10 Hz；本轮没有加入 FPS、frame time 或滚动效果统计，也没有改变原有 running AX 的预算和退出语义。四次旧 running AX 样本均是一次 click+query：聚合耗时为 `739.832 ms`、`772.926 ms`、`675.421 ms`、`819.115 ms`，query 本身为 `399.2 ms`、`360.5 ms`、`297.7 ms`、`413.0 ms`。这只能证明旧 tail-token 规则下的四次原生读回成功，不能推出 final completion 已进入 viewport，也不能推出性能门槛、滚动交互或人工视觉认可。

round-18-integrated-final-completion 在当前确认正式包上执行了三会话、30 秒、10 Hz 的严格 completion 验收。正式包 executable SHA-256 为 `db479d18dbde05705cf7bfced02c8ab6164f650d1f4bf3e28e0d481a76eeb216`，`__TEXT,__text` 代码段 SHA-256 为 `40d66e826fe9117eaaee6be0cc4757831221d84c0463af621412348616ca8b8e`，832 个 production 文件的 source fingerprint 为 `32e08f88522775e0cfb2568393ea93371b773d31222191f4e8533b2792f13a67`，driver SHA-256 为 `3a2377dd9b5881841c3f88fea26a916350e9f785e406510aadba02db02c1419b`；隔离副本代码和 Resources 校验通过。

三条 SSE 各完成 300 个 content chunk，持续 30 秒、10 Hz，共 900 个 chunk，峰值并发为 3，均 `complete: true` 且未断开。三会话最终均为 `idle` 且 `active_run == nil`，独立 run ID、三份 journal 和 snapshot 归属均通过。四次 running AX 和三次 final completion AX 均各一次 attempt；running AX 聚合耗时为 `690.649–874.547 ms`，final completion AX 聚合耗时为 `905.878–934.383 ms`。final completion 均通过真实 `last row of table 1` static text 读回本 session 唯一 token，`contains_own_marker` 和 `contains_completion_token` 均为 true，foreign marker/completion token 列表均为空；`final_completion` 摘要为 `record_count: 3`、`expected_record_count: 3`、`assertion: PASS`。

正常退出码为 `0`，QA PID 已消失，fixture listener 已关闭，隔离凭据已删除，collector stop/join、源码/原包 unchanged 和 evidence secret scan 均通过。该轮 review 为 `PASS_FOR_RECORDED_SCOPE`，证据目录为 [`round-18-integrated-final-completion`](../../output/acceptance/native-streaming/round-18-integrated-final-completion/)，其中包含 [`review.json`](../../output/acceptance/native-streaming/round-18-integrated-final-completion/review.json)、[`results.json`](../../output/acceptance/native-streaming/round-18-integrated-final-completion/results.json)、[`manifest.json`](../../output/acceptance/native-streaming/round-18-integrated-final-completion/manifest.json) 和 [`measurement-samples.json`](../../output/acceptance/native-streaming/round-18-integrated-final-completion/measurement-samples.json)。streaming/final 截图已保存并由 root 打开，但 `visual_approval` 仍为 `NOT_CLAIMED`；real API 为 `NOT_TESTED`，FPS/frame time 为 `NOT_MEASURED`，scroll effect/reading position 为 `NOT_ASSERTED`。

round-19-integrated-final-confirmation 使用与 round-18 相同的正式包、driver 和三会话 30 秒/10 Hz 负载复跑，review 为 `PASS_FOR_RECORDED_SCOPE`。三条 SSE 再次各完成 300 个 chunk，共 900 个 chunk，峰值并发为 3，三流均 complete 且未断开；最终三会话均为 `idle`、`active_run == nil`，三份 journal 的 run/session 归属通过。四次 running AX 读回各一次 attempt，click+query 聚合耗时为 `649.507 ms`、`649.917 ms`、`586.090 ms`、`722.974 ms`；三次 final completion AX 读回各一次 attempt，聚合耗时为 `917.057 ms`、`918.033 ms`、`880.689 ms`。三个 final record 均 PASS，均从真实 `last row of table 1` static text 读回本 session 唯一 completion token，own marker/token 可见且 foreign marker/completion token 为空，`record_count` 与 `expected_record_count` 均为 3。正常退出码 `0`、QA PID 消失、fixture listener 关闭、隔离凭据删除、collector stop/join、源码/原包 unchanged 和 secret scan 均通过。证据目录为 [`round-19-integrated-final-confirmation`](../../output/acceptance/native-streaming/round-19-integrated-final-confirmation/)，streaming 截图 SHA-256 为 `762a5ae41a5110b9e1c11b07512cc307fcde3a3b6cc83dd1dd2e6b0f60ec002f`，final 截图 SHA-256 为 `3226ace094b670688daf2f5e09d1f966ddf7e9186c8284b901bd9d6aaff5d639`；截图已由 root 打开但 `visual_approval` 仍为 `NOT_CLAIMED`，real API 为 `NOT_TESTED`，FPS/frame time 未测，实际滚动效果和阅读位置恢复未断言。
round-18 和 round-19 的 manifest 使用旧 driver SHA-256 `3a2377dd9b5881841c3f88fea26a916350e9f785e406510aadba02db02c1419b`，两轮均由 root 以 `3/3` final completion 结果独立核验。随后当前 driver 只增加成功路径的显式 final completion 汇总门槛，并增加“不完整/FAIL 汇总必须阻止成功”的 self-test；当前 driver SHA-256 为 `88a08fdcfb35cec5111b0d16938c117891c6ad0982e095803cfda48226ec2624`，`py_compile` 与 `--self-test` 均通过。该修正未用于 round-18/round-19 原生重跑，不改变两轮历史 artifact 的 driver 身份。

先执行静态检查：

```sh
python3 -m py_compile docs/verification/scripts/native-streaming-qa.py
python3 docs/verification/scripts/native-streaming-qa.py --self-test
```

在 root 核对当前 `build/darwin-amd64/GCode.app` 后，串行执行真实验收：

```sh
python3 docs/verification/scripts/native-streaming-qa.py \
  --bundle build/darwin-amd64/GCode.app \
  --output 'output/acceptance/native-streaming/<new-round>' \
  --duration 30 \
  --hz 10 \
  --sessions 3
```

每次重跑都必须把 `<new-round>` 替换为新的、尚不存在或为空的目录；不要复用已有的 `round-1`、`round-2`、`round-3` 或 `round-4`。

`--self-test` 不打开窗口、不读取生产数据、不绑定 HTTP socket。正式运行需要 macOS 辅助功能权限；root 应避免与其他原生驱动同时操控窗口。

## round-4 实际结果

正式包身份和隔离边界均通过：原包为 `build/darwin-amd64/GCode.app`，源可执行文件 SHA-256 为 `7bb8174524b12fec71a6b3bf5c653319f017394c7050b9f467e1091ebb5fd885`，Mach-O `__TEXT,__text` 代码段 SHA-256 为 `5e9391a92029c0eb3b3b8d3ae0007cbe376d1021c8a0d05e37e5d955e39da1d7`，生产源码规范化清单 SHA-256 为 `02d2ecc2f9ff000176aa644ba0570ed5643c44a05c545a227cf249b17707f27c`；隔离副本只改变 bundle identity 并 ad-hoc 签名，`bundle_code_and_resources_identical`、`source_files_unchanged_after_run` 和 `original_bundle_unchanged_after_run` 均为 `true`。

三个会话均属于 `project_87504ee64e7460ef`，使用同一个临时项目和同一个 cwd。三条 loopback Chat Completions SSE 请求各持续 30 秒并产生 300 个 chunk，`peak_concurrent` 为 3，均 `complete: true` 且 `disconnected: false`。controller 在流式期间观察到三个独立 `running`/`active_run`，最终三个会话均为 `idle` 且 `active_run` 为 `nil`；三个 journal 各有 3 个事件，包含一个 `user`、一个 `assistant` 和一个 `run_end`，run ID 与 session 归属一致。

round-4 共完成四次轻量 AX tail-token 读回：三个会话各一次，以及切回第一个会话的 `session-1-before-scroll` 一次。四次均包含当前会话 tail token，`foreign_marker_check` 和 `foreign_tail_token_check` 均为空；对应原始文本保存在 `session-1/2/3-streaming-ax.txt` 和 `session-1-before-scroll-ax.txt`。这些 raw AX 文件是历史证据，新 driver 不再生成。首次 AX 读回的聚合耗时为 `27686.087632 ms`，后续三次为 `1663.861772`、`760.924816`、`876.567313 ms`。该计时从会话切换开始，包含同步 `osascript` IPC、AX 查询和等待，因此只能作为样本。四次调用按顺序执行，不能声称四次都在三条流同时 `running` 时完成；根据 provider 完成时间和该顺序，合成滚轮很可能发生在流结束后。

合成滚轮证据只记录 `cghidEventTap` 事件已注入、table frame 和指针已恢复；没有滚动效果断言、滚动前后 viewport 对比、滚动后 AX 读回或阅读位置恢复断言。应用菜单退出码为 0，QA PID 消失，fixture server/thread 停止，隔离 `credentials/` 已删除。

round-4 只留下两次 `controller_poll` 和一次 process sample（RSS `66572 KiB`、CPU `17.8%`），因为同步 AX 读回占住了驱动的采样循环。该轮不支持持续轮询频率、AX 稳定延迟、吞吐、RSS/CPU 或整体性能达标结论。

证据链接：[`results.json`](../../output/acceptance/native-streaming/round-4/results.json)、[`manifest.json`](../../output/acceptance/native-streaming/round-4/manifest.json)、[`review.json`](../../output/acceptance/native-streaming/round-4/review.json)、[`measurement-samples.json`](../../output/acceptance/native-streaming/round-4/measurement-samples.json)、[`controller-final.json`](../../output/acceptance/native-streaming/round-4/controller-final.json)、[`provider-records.json`](../../output/acceptance/native-streaming/round-4/provider-records.json)、[`stream-chunks.json`](../../output/acceptance/native-streaming/round-4/stream-chunks.json)、[`streaming-window.png`](../../output/acceptance/native-streaming/round-4/captures/streaming-window.png)、[`final-window.png`](../../output/acceptance/native-streaming/round-4/captures/final-window.png)。review 状态为 `PASS_FOR_RECORDED_SCOPE`，`performance_scope` 为 `NOT_ACCEPTED`；两张截图均标记为 `visual_approval: NOT CLAIMED`，需要人工复核。

## round-5 至 round-8 结果

round-5 的三条 SSE 均完成 300 个 chunk，backend final snapshot 和 journal 归属证据通过，但第二个 session 的点击耗时为 `12005.453 ms`，tail-query 未进入；应用正常退出失败，最终被强制退出，exit code `-9`。该轮 review 为 `FAIL_NATIVE_INTERACTION_AND_NORMAL_EXIT`。profiler 只观察到主线程 CoreText 活动，存在 profiler overhead，未建立 Go 根因。

round-6 使用三会话且未启用 profiler。三条 SSE 均完成 300 个 chunk，峰值并发为 3，最终 snapshot、三个 journal、正常退出和清理均通过；第三个 session 的点击耗时为 `510.268 ms`，tail-query 聚合耗时为 `18518.624 ms`，8 次 attempt 后因严格外层 deadline 失败。前两个 session 的 AX 聚合耗时为 `847.376 ms` 和 `948.488 ms`。该轮 review 为 `FAIL_FOR_RECORDED_SCOPE`，失败点是原生 AX tail-query，不能写成 SSE、runtime、snapshot 或 journal 失败。

round-7-single 只验证一个 session：SSE 完成 300 个 chunk，峰值并发为 1，snapshot、journal、正常退出和清理均通过；`session-1-streaming` 与 `session-1-before-scroll` 两次 AX 聚合耗时分别为 `614.418 ms` 和 `684.725 ms`。该轮 review 为 `PASS_FOR_RECORDED_SCOPE`，只证明单会话记录范围，不能外推三会话并发或整体性能。

round-8-two 只验证两个 session：两条 SSE 各完成 300 个 chunk，峰值并发为 2，snapshot、两个 journal、正常退出和清理均通过；`session-1-streaming`、`session-2-streaming` 和 `session-1-before-scroll` 三次 AX 聚合耗时分别为 `2448.633 ms`、`3677.418 ms` 和 `821.170 ms`。该轮 review 为 `PASS_FOR_RECORDED_SCOPE`，只证明两会话记录范围，不能替代三会话验收或整体性能结论。

以上各轮都采用同一预算语义：session 点击单独受 12 秒单次 AX 上限限制；点击返回后，last-row tail-query 使用 18 秒严格外层预算，query timeout 按每次 attempt 的剩余时间限幅；晚于 deadline 返回的成功结果不接受。round-7-single 和 round-8-two 的成功不能改变三会话仍未通过、`performance_scope` 仍为 `NOT_ACCEPTED` 的结论。

round-5 至 round-8 证据：[`round-5 review`](../../output/acceptance/native-streaming/round-5/review.json)、[`round-5 results`](../../output/acceptance/native-streaming/round-5/results.json)、[`round-6 review`](../../output/acceptance/native-streaming/round-6/review.json)、[`round-6 results`](../../output/acceptance/native-streaming/round-6/results.json)、[`round-7-single review`](../../output/acceptance/native-streaming/round-7-single/review.json)、[`round-7-single results`](../../output/acceptance/native-streaming/round-7-single/results.json)、[`round-8-two review`](../../output/acceptance/native-streaming/round-8-two/review.json)、[`round-8-two results`](../../output/acceptance/native-streaming/round-8-two/results.json)。各轮的 `manifest.json`、`measurement-samples.json`、`diagnostic-samples.json` 和截图均保留在对应目录；截图仍标记为 `visual_approval: NOT_CLAIMED`。

## round-13 与 round-14 结果

H1 最小排版修复后，round-13 的 session 1 AX tail-query 通过，session 2 的 tail-query 超过 18 秒严格外层预算；round-14 的 session 1、session 2 单次 query 通过，session 3 的 tail-query 超过 18 秒严格外层预算。成功的单次 query 约为 `0.3–0.7 s`，没有 timeout，说明排版瓶颈已经下降；仍不能据此断言 tail 内容始终进入可见原生 viewport。

两轮 backend 最终状态均为 `interrupted`，journal 均缺 assistant/run_end；正常退出码均为 `0`，collector 正常，且两轮都保留 streaming 截图。因此 round-13、round-14 都是三会话 FAIL，不能写成 SSE、runtime 或正常退出通过，也不能宣称三会话目标完成。

证据目录：[`round-13-layout-fixed`](../../output/acceptance/native-streaming/round-13-layout-fixed/)、[`round-14-fixed-frame-stats`](../../output/acceptance/native-streaming/round-14-fixed-frame-stats/)。新 driver 的 measurement evidence 只保留脱敏 ownership 字段和 `text_bytes`；历史 round-4 的 raw AX 文本继续作为历史证据描述。

## round-15 至 round-19 结果

round-15-selection-fixed 已观察到三个 session 同时 running，session 1 的 AX 读回通过，但 session 2 的 tail-query 在 18 秒严格预算内始终不可见，累计 29 次 attempt 后失败。三条 fixture 分别完成 263、251、239 个 chunk，因失败清理而断开；最终 session 均为 interrupted。failure-window 截图成功保存，正常退出码为 0，collector、fixture、QA PID 和凭据清理均通过。该轮不是三会话通过。

round-16-ax-role-tree 的 session 1 tail-query 同样耗尽 18 秒严格预算，最后一次 attempt 为 `osascript` timeout；session 1、session 2 的 fixture 各完成 300 个 chunk，session 3 完成 292 个后断开，最终 session 3 为 interrupted。失败后的 AX role-tree traversal 生成 `node_count: 823`、`status: DIAGNOSTIC_ONLY` 的诊断文件，不能把它当作 primary assertion 或生产根因。failure-window 截图成功保存，正常退出码为 0，collector 和清理通过。

round-17-parsed-prefix 是旧 tail-token 规则下的 `PASS_FOR_RECORDED_SCOPE`。root review 离线核对 832 个 production 文件、正式包与隔离副本代码边界；源可执行文件 SHA-256 为 `b02f442a7c05ec7c6a8db313b691a0a44d914513525c314827c041fe9fa23b4b`，`__TEXT,__text` SHA-256 为 `45f943c874bbabc248878cf99477f2213b310684aeab59621a8cf5dc12e2c408`。三条 SSE 各完成 300 个 chunk，独立 run/journal 终态为 idle，四次旧 AX 均一次 attempt 且 marker/tail ownership 通过，正常退出码为 0，QA PID、fixture 和凭据清理通过；该轮没有 `final_completion` record，因此不满足新的严格 completion 验收。

round-18-integrated-final-completion 在当前确认正式包上达到严格 completion 规则的 `PASS_FOR_RECORDED_SCOPE`。executable SHA-256 为 `db479d18dbde05705cf7bfced02c8ab6164f650d1f4bf3e28e0d481a76eeb216`，代码段 SHA-256 为 `40d66e826fe9117eaaee6be0cc4757831221d84c0463af621412348616ca8b8e`，source fingerprint SHA-256 为 `32e08f88522775e0cfb2568393ea93371b773d31222191f4e8533b2792f13a67`，driver SHA-256 为 `3a2377dd9b5881841c3f88fea26a916350e9f785e406510aadba02db02c1419b`，832 个 production 文件已核对，隔离副本代码和 Resources 校验通过。三条 SSE 各 300 个 chunk、30 秒、10 Hz，共 900 个 chunk，峰值并发为 3，三流均 complete 且未断开；最终 snapshot 三会话均 idle 且无 active run，三个 journal 的 session/run 归属均通过。

四次 running AX 和三次 final completion AX 均各一次 attempt。final completion 在 backend complete 和 session idle 之后执行，真实 last-row AX 均读回各自唯一 completion token；三个 final record 均 `PASS`，`record_count` 和 `expected_record_count` 均为 3，own marker/token 可见且 foreign marker/completion token 为空。正常退出码为 0，QA PID 消失，fixture listener 关闭，隔离凭据删除，collector、源码/原包 unchanged 和 evidence secret scan 均通过。该轮证据目录为 [`round-18-integrated-final-completion`](../../output/acceptance/native-streaming/round-18-integrated-final-completion/)，截图仍 `visual_approval: NOT_CLAIMED`，不能推出 FPS/frame time、实际滚动效果、阅读位置恢复、真实 API 或用户视觉认可。

round-19-integrated-final-confirmation 使用与 round-18 相同的正式包、driver 和三会话 30 秒/10 Hz 负载复跑，review 为 `PASS_FOR_RECORDED_SCOPE`。三条 SSE 再次各完成 300 个 chunk，共 900 个 chunk，峰值并发为 3，三流均 complete 且未断开；最终 snapshot 三会话均 idle 且无 active run，三个 journal 的 session/run 归属均通过。四次 running AX 和三次 final completion AX 均各一次 attempt；running AX 聚合耗时为 `649.507 ms`、`649.917 ms`、`586.090 ms`、`722.974 ms`，final completion AX 聚合耗时为 `917.057 ms`、`918.033 ms`、`880.689 ms`。三个 final record 均 `PASS`，`record_count` 与 `expected_record_count` 均为 3，own marker/token 可见且 foreign marker/completion token 为空。正常退出码为 0，QA PID 消失，fixture listener 关闭，隔离凭据删除，collector、源码/原包 unchanged 和 evidence secret scan 均通过。证据目录为 [`round-19-integrated-final-confirmation`](../../output/acceptance/native-streaming/round-19-integrated-final-confirmation/)，截图仍 `visual_approval: NOT_CLAIMED`，不能推出 FPS/frame time、实际滚动效果、阅读位置恢复、真实 API 或用户视觉认可。

round-17 证据：[`round-17 results`](../../output/acceptance/native-streaming/round-17-parsed-prefix/results.json)、[`round-17 manifest`](../../output/acceptance/native-streaming/round-17-parsed-prefix/manifest.json)、[`round-17 review`](../../output/acceptance/native-streaming/round-17-parsed-prefix/review.json)、[`round-17 measurement`](../../output/acceptance/native-streaming/round-17-parsed-prefix/measurement-samples.json)、[`round-17 streaming screenshot`](../../output/acceptance/native-streaming/round-17-parsed-prefix/captures/streaming-window.png)、[`round-17 final screenshot`](../../output/acceptance/native-streaming/round-17-parsed-prefix/captures/final-window.png)。截图的 `visual_approval` 仍为 `NOT_CLAIMED`，real API 仍为 `NOT_TESTED`；本轮旧规则 PASS 不能扩展为 final completion、FPS/frame-time、滚动效果、阅读位置恢复或视觉交付通过。

round-18 证据：[`round-18 results`](../../output/acceptance/native-streaming/round-18-integrated-final-completion/results.json)、[`round-18 manifest`](../../output/acceptance/native-streaming/round-18-integrated-final-completion/manifest.json)、[`round-18 review`](../../output/acceptance/native-streaming/round-18-integrated-final-completion/review.json)、[`round-18 measurement`](../../output/acceptance/native-streaming/round-18-integrated-final-completion/measurement-samples.json)、[`round-18 streaming screenshot`](../../output/acceptance/native-streaming/round-18-integrated-final-completion/captures/streaming-window.png)、[`round-18 final screenshot`](../../output/acceptance/native-streaming/round-18-integrated-final-completion/captures/final-window.png)。

round-19 证据：[`round-19 results`](../../output/acceptance/native-streaming/round-19-integrated-final-confirmation/results.json)、[`round-19 manifest`](../../output/acceptance/native-streaming/round-19-integrated-final-confirmation/manifest.json)、[`round-19 review`](../../output/acceptance/native-streaming/round-19-integrated-final-confirmation/review.json)、[`round-19 measurement`](../../output/acceptance/native-streaming/round-19-integrated-final-confirmation/measurement-samples.json)、[`round-19 streaming screenshot`](../../output/acceptance/native-streaming/round-19-integrated-final-confirmation/captures/streaming-window.png)、[`round-19 final screenshot`](../../output/acceptance/native-streaming/round-19-integrated-final-confirmation/captures/final-window.png)。

新的严格 completion 检查：三条 fixture 完成且三个 session 已 `idle` 后，驱动对每个 session 再次点击并调用真实 `last row of table 1` AX 读取。该检查沿用点击单独 12 秒上限、点击后 18 秒 outer deadline 和按剩余预算限幅的 subprocess timeout；它只接受 AX 返回正文中的 session 唯一 completion token。`measurement-samples.json` 单独写入 `kind: final_completion`，保存 completion token、attempt 时序、`text_bytes`、`contains_completion_token`、`foreign_completion_token_check` 和 marker ownership，不保存 AX 原始正文，也不以 controller snapshot、session state 或 row label 代替正文。旧 tail token 可见但 completion token 缺失时，检查必须失败；completion token 混入其他 session 也必须失败。

## 正式包隔离和凭据边界

驱动在启动前记录正式包可执行文件 SHA-256、Mach-O `__TEXT,__text` 代码段 SHA-256、全部 Resources hash 以及生产源码规范化清单 SHA-256。它复制正式包到唯一 `GCodeStreamingQA<timestamp>.app`，只更新 `CFBundleName`、`CFBundleDisplayName`、`CFBundleIdentifier`，再执行 ad-hoc 签名和严格验证。运行后再次核对原始正式包和生产源码；任何变化都使结果失败。

应用使用唯一 bundle identity 产生隔离的 `~/Library/Application Support/<qa identity>`。provider 必须通过真实设置页保存，地址为 `http://127.0.0.1:<port>/v1`，协议为 Chat Completions，凭据使用 `GCODE_CRED_STORAGE=encrypted-file`。fixture key 只在设置页输入和 loopback 请求校验时存在，manifest、controller、provider-records、chunk samples、AX attempt ownership 字段和错误记录均不得保存它；应用退出后只删除隔离 `credentials/`，保留其余数据和复制的 evidence。

## Loopback SSE fixture

HTTP server 只绑定 `127.0.0.1` 的随机端口，只接受 `/v1/chat/completions`。它从最后一个 user prompt 提取唯一 `NATIVE_STREAM_SESSION_<n>_<qa identity>` marker，然后按每秒 10 个 chunk、默认 30 秒持续写入 SSE：

- 每个请求使用独立 marker，内容持续增长，包含中文、emoji `🧪`、Markdown 列表、普通段落、fenced Go code 和 table。
- 最后一个既有 content chunk 额外附带当前 marker 派生的唯一 `stream_completion_token`；它不减少正文、不增加 chunk、不改变 30 秒时长或 10 Hz 速率。finish event 和 `[DONE]` 仍按原有顺序发送。
- 每个请求结束时写 `finish_reason=stop` 和 `data: [DONE]`；客户端断开或驱动清理时记录 disconnected 并停止写入。
- `provider-records.json` 只保留 marker、请求序号、开始/首 chunk/末 chunk/完成时间、chunk 数、字节数、完成状态和并发峰值，不保存 Authorization、请求体或响应全文。
- `stream-chunks.json` 保存每个 chunk 的 marker、请求序号、chunk 序号、时间和字节数，用于重放首 token、每个 chunk 和 finish 时序；同样不保存内容正文或凭据。

三条请求的 `peak_concurrent` 必须达到会话数，每条流必须完成并达到默认期望 chunk 数的 70% 以上。这个比例只防止 fixture 提前结束，不是产品吞吐或延迟门槛。

## 原生操作顺序和断言

1. 启动隔离 app，按 PID 等待设置按钮可访问；记录启动到 AX ready 的耗时。
2. AX 打开设置，添加 loopback provider，保存服务和深色主题，再返回工作台。通过持久化 `controller.json` 确认 provider、默认 provider 和凭据文件进入隔离目录。
3. 打开一个临时项目，连续创建三个同名会话。驱动从 controller snapshot 记录 session ID、项目 ID、工作目录，并用同名 AX button 的 occurrence/index 选择具体会话，避免标题相同时误选。
4. 依次选中三个会话，输入唯一 prompt marker 并点击真实“发送”。发送经过真实 desktop controller 和 Service；fixture 记录三条连接后，驱动轮询 controller snapshot，要求三个 session 都是 `running`、`active_run` 非空、run ID 独立，文本没有其他 marker 或 foreign tail token。
5. 在已经观察到三个 session 同时 `running`/`active_run` 后，按顺序切换三个会话并通过 AX table 读回各自专属 stream tail token、中文和 emoji；保存每次旧 running AX 读回耗时、`text_bytes` 和已知 token ownership 字段。每个成功 attempt 记录 `own_marker_visible`、`foreign_markers`、`foreign_tail_tokens`；这些列表只来自本轮已知 fixture marker/tail token，不保存原始 AX UI 文本。首个 prompt marker 只在第一 chunk 中出现，列表虚拟化导致它不在当前 AX viewport 时不判为丢失；完整 prompt marker 仍由 controller snapshot 和 journal 检查。AX 读回是同步调用，实际运行中可能跨过某条流的结束时间，不能把“开始读取时已同时 running”写成“四次读取都在同时 streaming”。对当前会话 table frame 执行合成 `cghidEventTap` 滚轮 helper，并记录指针恢复、frame 和 helper 输出；除非另有 viewport 对比，不得把它写成滚动效果或阅读位置恢复通过。它是 synthetic/native event，不能写成物理滚轮或触控板证据。
6. 当三条请求已经同时运行且窗口仍在工作台时，调用基于真实 QA PID 的 CoreGraphics `CGWindow` 探针，使用 `screencapture -l <windowID>` 保存持续流式窗口截图；流结束且最终 snapshot 通过后再保存最终会话窗口截图。每张图记录 PID、CGWindow ID、路径和 SHA-256，并确认没有在设置/API key 页截图。失败后、collector stop/join 和 quit_app 前也尝试调用同一 capture 保存 `captures/failure-window.png`；capture 异常放入独立 diagnostic 字段，不覆盖 `primary_error`，也不加入 cleanup errors。截图仅提供给人复核，驱动不把截图或 AX 文本自动判为视觉通过。
7. round-5 在三条任务发送前启动独立后台 collector，约每 200 ms 记录 controller 状态/run ID/fixture 请求完成数与活动数，约每 1 s 记录 QA PID 的 RSS KiB 和 CPU 百分比；collector 在退出时明确 stop/join，采样错误保留在 `diagnostic-samples.json`，不写入产品日志。AX 读回记录会话点击耗时以及每次 attempt 的起止时间、耗时、实际 `effective_timeout_ms`、成功状态、脱敏错误类型、tail 可见性、`text_bytes` 和 ownership 字段，并在每次 attempt 前后保存目标 controller/fixture 状态；每次 last-row 查询的 subprocess timeout 都限幅到外层剩余预算，预算耗尽后不再发起 AX 查询。测量文件必须说明 AX 包含点击、IPC、原生等待和 AX 查询；RSS/CPU 无隔离负载和无性能门槛。严格外层预算不能接受在 deadline 之后才返回的成功条件；同步 AX 调用不得暂停后台 collector。
8. 等待三条 SSE 完成，再等待三个 session 回到 `idle` 且 `active_run == nil`。最终 snapshot 中每个会话要有自己的 user/assistant run ID、完整 marker 和中文/emoji/Markdown 内容，并且不含其他 marker。只有 backend complete 与 idle 都成立后，才开始 final completion AX 检查。
9. 逐 session 点击并通过真实 last-row AX 读取唯一 final completion token。该检查沿用点击单独 12 秒上限、点击后 18 秒严格 outer deadline 和按剩余时间限幅的 query timeout；必须从 AX 返回的正文判断 token，不能用 controller snapshot、session state 或 row label 代替。独立 `final_completion` record 记录 `contains_completion_token`、`foreign_completion_token_check`、`foreign_marker_check`、attempt 时序和 `text_bytes`，不保存原始正文。
10. 读取三个 `sessions/<session_id>/events.jsonl`。逐行解析 JSON，检查 `sessionId`、递增 sequence、结构化 `payload.runId`、所有运行事件的 run ID 只属于该 session，并确认 `message:user`、`message:assistant`、`message:run_end` 存在。`run_end` 不要求不存在的 `content` 字段；未知但结构合法的非运行事件可以没有 run ID。journal 归属必须与最终 controller snapshot 一致。
11. 通过菜单退出。只有 exit code 0、PID 消失、fixture server/thread 停止、隔离 credentials 删除且 final completion assertion 通过后，结果才可写为 `PASS`；源码、原正式包、凭据或 fixture 断言失败均为 `FAIL`。

## 输出文件和原始采样

成功或失败的独立 output 目录包含：

- `manifest.json`：QA identity、bundle/source/code/Resources hash、loopback 地址、duration/hz/session 数、PID、隔离路径、退出和清理状态、边界声明。
- `results.json`：server stream、snapshot ownership、journal ownership、正常退出、fixture 错误和失败原因。它不复制完整 controller 或请求体。
- `provider-records.json`、`stream-chunks.json`：脱敏请求级和 chunk 级时序。
- `controller-final.json`、`sessions/session-<n>-<id>/{final-session.json,events.jsonl}`：最终 controller/journal 原始证据；失败轮次至少保留 `controller-last.json` 和已产生的 journal。
- `measurement-samples.json`：`ax_readback`、`final_completion`、`send` 样本，保存 AX 延迟/`text_bytes` 和发送时序；旧 running AX attempt 记录实际 `effective_timeout_ms`、tail 可见性、`own_marker_visible`、`foreign_markers` 与 `foreign_tail_tokens`。完成后独立 `final_completion` record 记录 completion token、实际 `effective_timeout_ms`、`contains_completion_token`、`foreign_marker_check` 与 `foreign_completion_token_check`。这些 ownership 字段只来自已知 fixture token，新 driver 不写入原始 AX UI 文本；round-5 的后台 `controller_poll`、`process` 样本位于独立的 `diagnostic-samples.json`；round-4 历史目录仍有 2 个 `controller_poll`、1 个 `process` 和 4 个 `ax_readback`，不能用于持续性能结论。
- `diagnostic-samples.json`：round-5 后台 collector 的 controller/process 样本、collector 错误和只读观测元数据；它是专属诊断证据，不是产品日志，也不把读取错误自动升级为生产断言。
- `manifest.json` 的 `diagnostic_errors` 与 `results.json` 的 `checks.failure_window_capture`：失败后窗口截图的独立诊断结果。成功时 `manifest.visual_evidence.failure` 包含 `captures/failure-window.png`、当前 QA PID、CGWindow 元数据和 SHA-256；应用/helper 已退出或 capture 异常时记录 `status: ERROR` 和错误类型，保持 `primary_error` 原值。
- `ready.json`：非敏感 QA handle，保存 PID、QA identity、隔离 data directory、阶段和更新时间；`ax_readback` 阶段用于 root 对同一 PID 做系统 sample，清理后写入 `process_alive` 和退出码，避免把旧 PID 当作仍在运行。
- 历史 round-4 的 `session-<n>-streaming-ax.txt`：工作台状态下的原始 AX table 文本，仅作为历史证据保留；新 driver 只保存脱敏 ownership 字段和 `text_bytes`，设置页出现时不抓取 API Key 字段。
- `captures/streaming-window.png`、`captures/final-window.png`：真实 QA PID 对应 CGWindow 的窗口截图；manifest 记录 CGWindow 元数据和 SHA-256，截图只供人复核视觉结果。
- `captures/failure-window.png`：失败后、正常清理前尝试保存的真实 QA PID 窗口截图；它只提供人工复核材料，不改变主失败原因。
- `synthetic-scroll.swift`、`cgwindow-list.swift` 和编译出的 helper：分别记录合成事件与 PID→CGWindow 绑定实现，不改变物理输入或视觉通过结论。

## 失败和未验证范围

历史失败轮次必须保留，不能被后续轮次覆盖：

- round-1：`results.json` 为 `FAIL`，fixture 峰值并发为 2；第 1、2 条请求分别只记录 13、12 个 chunk 后断开，第 3 条记录 209 个 chunk，QA 应用未通过正常退出流程并触发清理失败。
- round-2：`results.json` 为 `FAIL`，峰值并发为 1；第 2 个 marker 被错误写入另一个 session（`session_f5cbc56d0456f0a8`），随后正常退出清理也失败。该轮保留了重复会话标题导致 AX action 误选的失败证据。
- round-3：三条 backend 流均各完成 300 个 chunk，且 `shared_running` 断言通过；失败发生在切回第一个 session 的 AX tail-token 读回，`osascript` 超时。该轮说明原生 AX 读取路径失败，不能解读为生产 runtime 或 SSE fixture 失败。
- round-5：三条 SSE 和 backend final snapshot/journal 证据完成，但第二个 session 的点击耗时约 12 秒，tail-query 未进入；菜单退出未完成，QA 进程最终以 `-9` 强制退出。该轮 profiler 只提供带 overhead 的主线程观察，不能建立 Go 根因。
- round-6：三条 SSE、最终 snapshot、journal、正常退出和清理均通过，但第三个 session 的 tail-query 在点击完成后超过 18 秒严格预算；该轮为 `FAIL_FOR_RECORDED_SCOPE`。这不是 SSE、runtime、snapshot 或 journal 失败。
- round-13：H1 最小排版修复后，session 1 AX tail-query 通过，session 2 tail-query 超过 18 秒严格预算；backend 最终为 `interrupted`，journal 缺 assistant/run_end，正常退出码为 0，collector 和 streaming 截图正常。该轮不能标记为三会话通过。
- round-14：session 1、session 2 单次 AX query 约 `0.3–0.7 s` 通过，session 3 tail-query 超过 18 秒严格预算；backend 最终为 `interrupted`，journal 缺 assistant/run_end，正常退出码为 0，collector 和 streaming 截图正常。该轮不能标记为三会话通过。
- round-15-selection-fixed：三个 session 同时 running，session 1 AX 通过，session 2 tail-query 18 秒预算耗尽；三条 fixture 分别为 263、251、239 个 chunk，最终 session 均为 `interrupted`。failure-window、正常退出和清理证据完整，但该轮不能标记为三会话通过。
- round-16-ax-role-tree：session 1 tail-query 18 秒预算耗尽，最后一次 attempt 为 `osascript` timeout；session 1/2 fixture 各 300 个 chunk，session 3 为 292 个并断开，失败后的 role-tree 只作 `DIAGNOSTIC_ONLY` 证据。该轮不能标记为三会话通过。
- round-17-parsed-prefix：旧 tail-token 规则下三条 fixture 各 300 个 chunk，独立 run/journal 终态 idle，四次旧 AX 各一次 attempt 通过，正常退出码为 0，清理通过，结果为 `PASS_FOR_RECORDED_SCOPE`。该轮没有 `final_completion` record，不能作为新严格验收通过；也未统计 FPS/frame time，截图仍 `visual_approval: NOT_CLAIMED`，不能宣称滚动效果、阅读位置恢复或视觉交付通过。

请求 marker 不唯一、三条连接没有并发、任何 session 出现 foreign marker、run ID 重用、流提前断开、journal 结构缺失、正常退出失败、凭据未删除或源码/原包 hash 变化，都不能标记为通过。失败轮次保留在自己的目录，不能覆盖后续成功证据。

本记录使用 loopback Chat Completions 替身，不是真实厂商 API；不宣称真实网络、真实模型、API 限流、整体性能、持续 AX 延迟、GPU/frame time、物理键盘或中文 IME、系统剪贴板、物理滚轮/触控板、滚动效果或阅读位置恢复、Apple Silicon、Windows、终端交互或用户视觉认可已通过。AX 读回中文和 emoji 只证明当前原生文本可读回；截图只证明已保存与 QA PID/CGWindow 绑定，必须由人单独复核界面。失败截图 capture 的错误属于诊断证据，不能覆盖 primary error，也不能被解释为产品故障本身。
