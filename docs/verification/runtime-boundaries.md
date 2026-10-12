# 运行时边界验收

日期：2026-10-12

## 已证实行为

- 厂商返回包含固定凭据的错误后，`application.Service` 快照、`controller.json`、session `events.jsonl` 和临时数据目录均不包含该凭据；session 仍保留可诊断的 provider/auth 失败证据。
- `Send` 在 `controller.json` 不可写时返回错误、回滚内存中的消息和队列，且不会触达厂商。恢复持久化路径后只执行一次；关闭并重开 Service 不会重放已完成任务，显式的新 Send 才产生下一次请求。
- `Stop` 在持久化失败时保留旧任务为 `Running`、不暂停队列，也不取消活动运行；恢复路径后 Stop 才将 session 收束为 `Interrupted` 并清除 `ActiveRun`。
- `SaveSettings` 在持久化失败时恢复原设置，恢复路径后保存成功。
- provider 凭据仍存放在凭据存储中，controller 启动从存储恢复 `Provider.HasAPIKey` 派生标记。有密钥和无密钥 provider 在 `Close`→`New` 后分别恢复为 `true` 和 `false`；`controller.json` 不保存该标记，也不保存密钥明文。
- `Close` 在 `controller.json` 不可写时仍取消活动运行并清理真实 shell。验收记录了 shell PID：关闭前存活，关闭后退出，`done` 标记未生成。

## 实施修复与夹具调整

- `internal/controller/controller.go` 的 `load` 在恢复 settings 后重新查询凭据存储并填充 `HasAPIKey`，避免重启后的设置界面丢失已配置状态。
- 运行时边界测试使用真实 `application.Service`、本地 `httptest` provider、独立临时数据目录和真实文件故障（将 `controller.json` 替换为目录）。
- Stop 测试等待 provider 请求上下文或测试释放信号，不把 HTTP 连接关闭时序误判为运行取消失败。
- Close 测试在服务关闭后释放无响应头的 HTTP 替身，避免测试替身自身阻塞 `httptest.Server.Close`；真实 shell 的 PID 和完成标记仍作为产品资源收束证据。

## 验证命令

普通定向测试和 race 定向测试均通过，原始输出见：

- `output/acceptance/runtime-boundaries/focused-regular.txt`
- `output/acceptance/runtime-boundaries/focused-race.txt`
- `output/acceptance/runtime-boundaries/controller-runtime-regular.txt`
- `output/acceptance/runtime-boundaries/controller-runtime-race.txt`

覆盖的定向测试：

```text
TestServiceRuntimeBoundaryRedactsCredentialFromSnapshotsAndHistory
TestServiceSendPersistenceFailureRollsBackAndRestartDoesNotDuplicate
TestServiceStopPersistenceFailureRetainsRunUntilSuccessfulStop
TestServiceSettingsPersistenceFailureRollsBackAndRecovers
TestServiceRestoresProviderCredentialPresenceAfterRestart
TestServiceClosePersistenceFailureConvergesRealShellResource
```

`internal/application` 与 `internal/worktrees` 的完整普通和 race 测试均通过，原始输出见：

- `output/acceptance/runtime-boundaries/application-worktrees-regular.txt`
- `output/acceptance/runtime-boundaries/application-worktrees-race.txt`

此前的无超时全包运行曾在 `TestServiceClosePersistenceFailureConvergesRealShellResource` 的 `httptest.Server.Close` 处等待活动连接；堆栈显示阻塞点是测试 handler 等待未返回响应的请求，而非 worktree 实现。释放信号修正后，同一完整命令普通测试约 21 秒通过，race 测试约 34 秒通过，均无 race 报告。

以上具体测试名来自本轮复现。最初 `output/acceptance/worktree-service/full-package-note.txt` 只有连接等待的说明，没有独立的测试名或堆栈，因此不能断言两次挂起为同一原因；当前完整普通/race 通过表明该全包挂起已不再是现状阻断。

## 仍缺项

- 本轮只验证当前 application/controller/runtime 边界和 application/worktrees 包；未将其外推为整个仓库 `go test ./...` 已通过。
- Close 测试的 HTTP 替身在关闭后显式释放，验收重点是持久化失败时真实 shell 资源收束；它不把 `httptest` 连接关闭时序作为独立产品契约。
