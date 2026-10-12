# Service 授权与提问边界 S2

日期：2026-10-12

## 目标

补齐本地应用入口下的授权、Agent 提问和纠正取消证据。每个场景都从真实 `application.Service` 发起，经过 `controller.Controller`、`runtime.Engine` 和 Chat Completions 协议适配器；HTTP 只替代外部模型服务，工具、文件系统、session 历史和取消信号走生产实现。

## 所有权

本目标只允许修改：

- `internal/application/permission_question_integration_test.go`
- 为修复本目标明确暴露的 controller/runtime 缺陷所必需的文件；修改前须说明证据并协调内部所有权
- `docs/goals/permission-question-service.md`
- `docs/verification/permission-question-service.md`
- `output/acceptance/permission-question-service/`

不修改 desktop、workspace、worktrees、共享验收矩阵或原生窗口；不提交、不推送。

## 覆盖范围

- 受控 `exec_command` 使用 `sandbox_permissions=require_escalated` 触发真实权限等待；允许后写入实际项目文件，拒绝后文件不存在。
- `ask_user` 通过真实问题回调接收答案；答案进入下一次 provider 请求、工具结果、session 消息和 `events.jsonl`。
- 在 permission 等待期间发送纠正，旧 run 必须先取消完成，随后新 run 才能等待并执行。
- 在 ask_user 等待期间发送纠正，旧 `ReplyAsk` 必须失效，新问题的回答只能归属于新 run。
- 旧 `ReplyPermission`、旧 `ReplyAsk` 均被拒绝；两个 session 的 pending request 互相隔离，跨 session 回复不能执行工具。

## 完成判据

- 普通测试和 `-race` 定向测试均以 `-timeout=180s` 通过。
- HTTP fixture 的 handler 不因等待连接阻塞 `server.Close`；如使用等待通道，cleanup 必须释放。
- 测试输出和错误不包含 provider key。
- 验收报告区分本地 API 替身已验证的行为、真实厂商 API/原生 UI 未验证的边界。
