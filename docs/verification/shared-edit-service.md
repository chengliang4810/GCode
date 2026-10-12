# 共享文件编辑竞争 Service S2 验证

验证日期：2026-10-12

## 结论

本地应用层 S2 通过。`TestServiceSharedEditIntegrationS2` 使用真实临时 Git
项目和真实文件，从 `application.Service` 进入 `controller.Controller`、
`runtime.Engine`、Chat Completions provider adapter，再调用生产内置
`read_file`、`edit_file` 和 `write_file`。HTTP `httptest.Server` 只替代模型
协议，没有 fake runner。

同文件竞争在 `edit_file` 和 `write_file` 两条路径均通过：两个会话先完成读取，
由 channel barrier 同时放行写入；每次恰有一个完整写入成功，另一个工具结果
明确报告 `the file changed on disk since you last read it`，最终磁盘内容等于
成功会话的完整内容。外部编辑场景在两条写入路径均拒绝陈旧内容，
`outside-editor\n` 保持不变。不同文件场景在相同 barrier 后两个写入均成功，
`a.txt`、`b.txt` 分别保留完整目标内容。每个场景还检查真实 `events.jsonl`
中的读写工具事件和工具结果，并检查 Git porcelain 报告对应修改。断言会解析每个
session 的工具 arguments，严格匹配 marker、路径、旧值/新值或完整 `content`、
call ID、状态和输出；每条历史事件都核对 `SessionID`。同时解析第 3 次 provider
请求体，核对 user marker、`read_file`/写工具参数、`tool_call_id` 和工具结果，
并拒绝混入另一 session 的 marker、路径或写入值。

## 确定性与边界

fixture 在收到每个会话携带 `read_file` 结果的下一次 provider 请求后登记
barrier；达到参与会话数才返回写入 tool call。barrier、provider 请求和等待
均有 10/20 秒期限，没有用 sleep 证明两个会话已经读完。外部编辑由 fixture
在 barrier 释放后直接对真实路径调用 `os.WriteFile`，因此与工具路径分离。provider
最终请求体在 fixture 中按 marker 保存，测试完成后结构化读取，避免只凭最终磁盘
内容判断 session 所有权。

测试使用固定的测试 API key，仅在 HTTP Authorization 校验中使用；日志不打印
请求体或 key。每个 `httptest.Server` 通过测试清理关闭，Service、临时文件和
临时 Git 项目也在测试结束时释放。

## 原始命令与证据

以下命令均在仓库根目录执行，原始输出位于
`output/acceptance/shared-edit-service/`：

```text
go test -mod=mod ./internal/application -run '^TestServiceSharedEditIntegrationS2$' -count=1 -timeout=180s -v
go test -mod=mod -race ./internal/application -run '^TestServiceSharedEditIntegrationS2$' -count=1 -timeout=300s -v
go vet -mod=mod ./internal/application
```

结果：普通 focused 和 race focused 均通过 6 个子场景；`go vet` 退出 0 且无
输出。严格归属断言的证据文件为 `focused-ownership-regular.txt`、
`focused-ownership-race.txt`、`vet.txt`。

## 结论范围

这是本机临时项目上的本地协议替身 S2，证明 GCode 内置读写之间的同文件陈旧
内容拒绝和不同路径写入行为。它不等同真实厂商 API 的 S3，也不覆盖 shell、
外部编辑器与内置工具同时写同一路径的完整组合、原生 UI 或其他平台。
