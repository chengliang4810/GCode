# 首版运行时边界

## 目标

补齐首版应用运行时在以下边界上的可复核证据，并在发现真实缺陷时做最小修复：

- 厂商错误包含固定凭据时，runtime、controller 快照和 session 历史均不泄漏凭据。
- 真实持久化路径失效时，Send、Stop、settings 的成功报告、内存回滚和旧任务取消语义保持一致。
- 持久化恢复后重启不会重复执行已提交任务。
- Close 失败时仍收束真实运行时资源。

## 所有权

本目标只允许修改：

- `internal/application/runtime_boundaries_test.go`
- 为修复上述边界所必需的 `internal/controller` 与 `internal/runtime` 文件
- `docs/verification/runtime-boundaries.md`
- `output/acceptance/runtime-boundaries/`

不修改 `internal/application/service.go`、worktrees 测试、desktop、cmd 或共享验收矩阵；不提交、不推送。

## 完成判据

- 使用真实 `application.Service`、本地 `httptest` 厂商替身、独立临时数据目录和真实文件完成最小高价值集成场景。
- 普通测试和必要的 race 定向测试通过；原始输出保存在 `output/acceptance/runtime-boundaries/`，不含密钥。
- 验证报告区分已证实行为、修复内容和仍缺项，并记录真实 shell 清理后的 PID 检查。
