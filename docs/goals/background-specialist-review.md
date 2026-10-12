# background specialist staging 复审

状态：最新 staging 源码复审完成，2026-10-12（Asia/Shanghai）。复审对象为 `/tmp/gcode-background-specialist-stage`，重点核对最新的 `internal/specialist/runtime.go`、`internal/background/manager.go`、`internal/runtime/runtime.go` 及新增生命周期测试。本文只修改主 checkout 的审阅记录，没有修改 staging 生产代码、测试或其他文档。

## 结论

此前提出的生命周期 P1 已全部解除。最新实现已经覆盖：

- Engine 与惰性 specialist runtime 创建/关闭的原子所有权；
- child owner、cancel 登记和任务收束之间的窗口；
- child 完成和 metadata 持久化失败时的内存终态；
- 无响应 child 的 bounded close；
- 并发、重复 `Runtime.Close` 和 `Engine.Close` 的共享结果；
- 无 PID 任务在 metadata 持久化失败时仍然执行 cancel。

因此，**代码层面建议集成当前 staging**。最终集成仍应等待 root 的最后一轮 targeted test、application lifecycle 和 race 结果；本代理没有运行测试。

## P1 修复复核

| 审阅项 | 当前判断 | 证据 |
| --- | --- | --- |
| Engine 关闭与惰性 runtime 创建竞态 | 已修复 | `internal/runtime/runtime.go:750-787` 使用同一把 `engine.mu`：关闭先发布 `closed` 并取得 runtime，惰性创建也在锁内检查 `closed`。关闭后到达创建点会收到 `runtime is closed`。 |
| `StartTask` 与 cancel 登记窗口 | 已修复 | `internal/specialist/runtime.go:73-103` 在 `runtime.mu` 内完成关闭/重复检查、`manager.RegisterCancel`、任务发布和 `tasksDone` 建立；生产路径在 `internal/specialist/exec.go:512-530` 先创建 context，再调用该入口。 |
| child 完成时吞掉 `MarkExited` 持久化错误 | 已修复 | `internal/background/manager.go:257-286` 先发布内存 terminal 状态，再返回 metadata 错误；`internal/specialist/exec.go:566-621` 把错误写入输出事件并传入 accounting。 |
| child/provider 忽略 cancellation 导致关闭永久等待 | 已修复 | `internal/specialist/runtime.go:31,155-227` 使用固定 5 秒 bounded wait，超时列出仍运行的 task ID 并返回诊断错误；当前实现已移除废弃的 `WaitGroup`。 |
| PID0 任务 persistence error 导致 cancel 丢失 | 已修复 | `internal/background/manager.go:309-338` 先把任务写成内存 `killed`、删除 owner、解锁并调用 `cancel`，之后才持久化；持久化失败仍返回错误，但不会漏掉取消。 |
| 并发/重复 Close 结果不一致 | 已修复 | `Runtime.Close` 用 `closeDone/closeErr` 让并发调用等待首次关闭并共享同一结果；`internal/runtime/runtime.go:751-757` 让重复 `Engine.Close` 继续委托同一个 specialist runtime，因此也返回真实关闭结果。 |

## 关闭语义复核

最新 `Runtime.Close` 的顺序是：

1. 在锁内将 runtime 标记为 closed，保存 active task channel，并发布唯一的 `closeDone`；
2. 调用 `manager.KillRunning()`；
3. 最多等待 5 秒；
4. 若仍有 task，返回包含 task ID 的 timeout 错误；
5. 只有所有 task 收束后才清理 runtime 追踪的 prompt 文件；
6. 保存 `closeErr`、关闭 `closeDone`，让并发和后续调用得到相同结果。

这满足既有目标中“child 无法在 bounded wait 内收束时返回可诊断错误，不能无限阻塞 `Service.Close`”的要求。超时后仍未收束的 child 会由自身 defer 路径继续清理；关闭结果保留 timeout 和 task ID，任务内存状态也不会被迟到的 child completion 覆盖。

## 已有验证证据

已读取的 staging 输出记录：

- `output/acceptance/background-specialist/staged/final-background-runtime-race.txt`：background、specialist、runtime 相关包记录为 `ok`；
- `output/acceptance/background-specialist/staged/final-application-race.txt`：application 包记录为 `ok`；
- `output/acceptance/background-specialist/staged/p2-targeted-packages-final.txt`：background、runtime、specialist、application 均记录为 `ok`。

新增测试覆盖：

- `internal/specialist/runtime_test.go:86-185`：并发/重复 `Runtime.Close`、无响应 child 的 5 秒超时、task ID 诊断和 fixture 释放；
- `internal/runtime/background_lifecycle_test.go:47-107`：并发/重复 `Engine.Close` 与 specialist runtime 关闭结果共享；
- `internal/background/manager_test.go:439-472`：metadata 持久化失败时无 PID owner 仍收到 cancel，内存任务保持 `killed`；
- `internal/application/background_specialist_lifecycle_test.go:92-282`：跨 parent 的 `TaskOutput`/`TaskStop`，cooperative `Service.Close`，metadata 终态和重复 Close。

这些证据覆盖了本轮 P1 的失败路径和正常 application 主路径。新增的 DataDir 检查仍主要依赖实现证据：`internal/runtime/runtime.go:781-785` 使用 `<engine.dataDir>/background`，当前 application 生命周期测试没有建立两个不同 DataDir 的 Service 来验证互不可见。

`output/acceptance/background-specialist/staged/full-internal-v1.txt` 记录过 sandbox/tools 的失败，失败路径指向 staging 临时目录和沙箱基线漂移；它不是本轮 specialist/background 改动的 targeted 结果。主 checkout 后续已在修正 evidence 路径后完成 confirmed 全仓回归，结果见下文。

## 非阻塞风险

| 项目 | 判断 | 说明 |
| --- | --- | --- |
| 超时后的 child 与 prompt 文件 | 可接受但需保留诊断 | `Runtime.Close` 超时后立即返回错误，无法安全替仍运行 child 清理其 prompt；child 最终退出时仍走 defer 清理。任务 ID 和 timeout 错误保留了关闭证据。 |
| 持久化失败后的重启恢复 | 仍需定义 | 当前进程会把 terminal 状态发布到内存并报告 durability error，但磁盘可能仍是旧的 `running`；重启 reload 可能再次修正该任务。该问题不再影响当前进程的 `TaskOutput` 终态。 |
| DataDir 双实例隔离 | 实现正确，验证不足 | manager root 已绑定 Engine 的 DataDir，但缺少两个 Service 的 application 级互不可见断言。它是补充证据，不是当前 P1 阻塞。 |

## 最终判定

**此前 P1 已解除，13 个文件现已精确集成到主 checkout。** 主 checkout 的全仓测试、受影响包 race、vet、fmt、package 和 codesign confirmed 结果均为 exit 0；原生 round18 仍由 root 串行执行，本记录不扩展到视觉、real API 或其他平台验收。

本代理未运行测试、race、构建、打包、真实 provider、窗口或设备流程。


## 最终日志身份与 13 文件清单核对

本轮最终六份测试日志均已读取，结果如下：

- `p2-background-specialist-final.txt`：`internal/background`、`internal/specialist` 均为 `ok`；
- `p2-background-specialist-race-final.txt`：`internal/background`、`internal/specialist` 均为 `ok`；
- `p2-runtime-final.txt`：`internal/runtime` 为 `ok`；
- `p2-runtime-race-final.txt`：`internal/runtime` 为 `ok`；
- `p2-application-lifecycle-race-final.txt`：`internal/application` 为 `ok`；
- `p2-targeted-packages-final.txt`：`internal/background`、`internal/runtime`、`internal/specialist`、`internal/application` 均为 `ok`。

`staging-diff-p2-final.txt` 的集成清单精确包含 13 个文件，且 `removed_since_baseline` 为空：

1. `internal/application/background_specialist_lifecycle_test.go`（已集成）
2. `internal/background/manager.go`
3. `internal/background/manager_test.go`
4. `internal/runtime/background_lifecycle_test.go`（已集成）
5. `internal/runtime/runtime.go`
6. `internal/specialist/accounting.go`
7. `internal/specialist/exec.go`
8. `internal/specialist/lifecycle_test.go`
9. `internal/specialist/runtime.go`
10. `internal/specialist/runtime_test.go`
11. `internal/specialist/stop_tool.go`
12. `internal/specialist/task_tool.go`
13. `internal/specialist/task_tool_test.go`

此前“两个测试文件在主目录不存在”是集成前快照。当前 `output/acceptance/background-specialist/integrated/integration-manifest.json` 已记录 13 个文件的 after hash，逐文件核对主 checkout 后 13 个 hash 全部匹配，manifest 状态为 `INTEGRATED_MAIN_REGRESSION_PASS`。`_before` 保存了 11 个原文件，manifest 记录 21 个 desktop 文件未被覆盖。最新 `main-drift-p2-final.txt` 的 desktop 漂移属于集成前无关变更，不涉及这 13 个文件。



## 主 checkout 最终回归与交付结果

当前主 checkout 的确认目录为 `output/acceptance/current-package-regression/integrated-streaming-background-confirmed/`。已读取真实 JSON 与日志，`results.json` 的六个步骤全部 `exit_code: 0`：

- `go test ./... -count=1 -timeout=180s`：126.146 秒；
- 受影响包 race（desktop/background/specialist/runtime/application）：62.242 秒；
- `go vet ./...`：1.510 秒；
- `make fmt-check`：0.246 秒；
- `make package`：4.238 秒；
- `codesign --verify --deep --strict build/darwin-amd64/GCode.app`：0.141 秒。

对应的 `test-all.log` 包含 57 个 package 结果且没有 `FAIL`，`race-affected.log` 的 5 个受影响包均为 `ok`。vet、fmt 和 codesign 日志为空是成功命令没有标准输出；成功结论以 `results.json` 的 exit code 为准。package 日志显示 darwin/amd64 的 `GCode.app` 已编译并签名。

`package-manifest.json` 进一步确认 832 个生产源文件和 420 个测试文件的 fingerprint 在构建后未变化，`source_unchanged_after_build` 与 `test_files_unchanged_after_build` 均为 true；生成包位于 `build/darwin-amd64/GCode.app`，executable size 为 24,206,352 bytes。manifest 的当前状态为 `INTEGRATED_MAIN_REGRESSION_PASS`。

第一轮 `integrated-streaming-background-final/test-all.log` 的失败来自把 `output/acceptance/background-specialist/integrated/before/` 和 `output/acceptance/native-streaming/diagnostic-tools/` 下的证据副本留在 Go 扫描路径中，Go 将这些不完整副本当作 package 并报缺少符号；失败项不在主 checkout 生产包。将证据目录改为 `_before` 后，confirmed 目录的全仓测试和后续检查均已 exit 0，因此第一轮失败不构成生产失败。

原生 round18 仍由 root 串行执行。本结果只闭合本功能的源码、测试、race、vet、格式、打包和签名边界，不宣称用户视觉、真实 API、设备或其他平台验收已完成。


## DataDir 双实例隔离定向验收记录

- 测试：`TestServiceBackgroundSpecialistIsolationAcrossDataDirs`。
- 命令：`go test ./internal/application -run '^TestServiceBackgroundSpecialistIsolationAcrossDataDirs$' -count=1`。
- 结果：成功，`internal/application` 输出 `ok`，耗时 5.861 秒。
- 日志：`/Users/chengliang/Documents/ChatGPT/GCode/output/acceptance/background-specialist/isolation-main/test.log`。
- 证据范围：两个真实 `application.Service` 使用不同 DataDir 启动独立 background specialist；双方 metadata、`TaskOutput`、`TaskStop` 互不可见，各自任务仍可读取；关闭一个 Service 只取消其自身任务，另一个 Service 继续运行并可读取自己的任务，随后正常关闭。
