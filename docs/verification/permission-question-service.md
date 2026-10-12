# Service 授权与提问边界验收

日期：2026-10-12

## 结论

本地 application 层 S2 验收通过。测试从真实 `application.Service` 进入，经过 `controller.Controller`、`runtime.Engine` 和真实 Chat Completions adapter；`httptest.Server` 只替代外部模型服务。工具执行、文件结果、交互等待、session store 和取消顺序均由生产代码完成，没有 fake Runner。

这份证据只覆盖本地协议替身和应用内核，不扩大为真实厂商 API、原生窗口或目标设备验收。

## 已证实行为

- `exec_command` 携带 `sandbox_permissions=require_escalated` 时会经过真实 permission request。回复 `allow` 后项目目录实际生成 `permission-allowed.txt`；回复 `deny` 后 `permission-denied.txt` 不存在，工具历史保留拒绝结果。
- `ask_user` 的真实问题进入 session snapshot；回复 `production` 后，下一次 provider 请求包含该答案，`ask_user` 工具历史和 `events.jsonl` 均保留问题答案，最终回复也能使用该答案。
- permission 等待中的旧 run 收到 correction 后先完成取消，旧文件没有写入；新 run 才出现新的 permission request。旧 `ReplyPermission` 被拒绝，新回复能完成新文件写入。
- ask_user 等待中的旧 run 收到 correction 后先完成取消，旧 `ReplyAsk` 被拒绝；新问题的回答只推进新 run。
- 两个 session 同时等待 permission 时，各自只接受本 session 的 request ID；跨 session 的 `ReplyPermission` 被拒绝，分别批准后两个文件均由对应 session 完成。
- HTTP fixture 校验请求路径和 Bearer 认证，但错误消息不回显密钥。测试结束前所有运行均已 idle，`server.Close` 正常返回；验收输出未包含 fixture key。

## 覆盖测试

| 测试 | 主要证据 |
| --- | --- |
| `TestServicePermissionIntegrationWritesOnlyAfterTheRealApproval` | 真实 `exec_command` 授权 allow/deny、实际文件结果、工具历史和 provider 上下文 |
| `TestServiceAskUserIntegrationThreadsAnswersThroughContextAndHistory` | 真实 ask_user 问题、回复入下一次请求、工具历史和 `events.jsonl` |
| `TestServicePendingInteractionCancellationAndSessionOwnership/permission correction rejects stale reply` | permission 等待纠正、旧回复失效、新 run 顺序 |
| `TestServicePendingInteractionCancellationAndSessionOwnership/ask correction rejects stale reply` | ask_user 等待纠正、旧回复失效、新 run 顺序 |
| `TestServicePendingInteractionCancellationAndSessionOwnership/sessions own pending permissions` | 两个 session 的请求归属隔离和实际文件结果 |

## 验证命令与结果

定向普通测试：

```text
go test -mod=mod -vet=off ./internal/application -run 'TestServicePermissionIntegrationWritesOnlyAfterTheRealApproval|TestServiceAskUserIntegrationThreadsAnswersThroughContextAndHistory|TestServicePendingInteractionCancellationAndSessionOwnership' -count=1 -timeout=180s
ok   gcode/internal/application  6.561s
```

定向 race 测试：

```text
go test -mod=mod -vet=off -race ./internal/application -run 'TestServicePermissionIntegrationWritesOnlyAfterTheRealApproval|TestServiceAskUserIntegrationThreadsAnswersThroughContextAndHistory|TestServicePendingInteractionCancellationAndSessionOwnership' -count=1 -timeout=180s
ok   gcode/internal/application  8.085s
```

完整 `internal/application` 普通和 race 测试也通过：

- `output/acceptance/permission-question-service/application-regular.txt`
- `output/acceptance/permission-question-service/application-race.txt`
- `output/acceptance/permission-question-service/focused-regular.txt`
- `output/acceptance/permission-question-service/focused-race.txt`

原始输出均只含 `go test` 的通过行，没有 provider key。

## 实施范围

本轮只新增 `internal/application/permission_question_integration_test.go`、目标文档和验收输出；没有发现需要修改 controller/runtime 的生产缺陷，因此没有生产代码变更。

## 未验证与风险边界

- 未调用真实厂商 API，未验证厂商模型是否会按预期生成这些 tool call，也未验证网络质量、限流、OAuth 或厂商特定错误格式。
- 未启动 native UI；UI 对 pending permission/question 的展示、原生输入和关闭窗口行为仍未验证。
- 未做 Android、Windows 或其他目标设备验收。
- 本包通过不代表整个仓库的 `go test ./...`、`go vet ./...` 或发布包验收已完成。
