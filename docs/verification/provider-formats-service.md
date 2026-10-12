# application.Service 四类 Provider 格式验证

## 范围和入口

验证入口是
[`internal/application/provider_formats_integration_test.go`](../../internal/application/provider_formats_integration_test.go)：

```text
TestServiceProviderFormatsFlow
TestServiceProviderFormatsCancellation
TestServiceProviderFormatsHTTPError
```

三个测试均参数化覆盖四类 wire format：

| GCode 格式 | 协议入口 | 本地 fixture 期望路径 |
| --- | --- | --- |
| `responses` | OpenAI Responses | `/v1/responses` |
| `chat-completions` | OpenAI Chat Completions | `/v1/chat/completions` |
| `anthropic` | Anthropic Messages | `/v1/messages` |
| `gemini` | Gemini Generate Content | `/v1beta/models/{model}:streamGenerateContent?alt=sse` |

测试从真实 `application.Service` 开始，经过
`controller.Controller → runtime.Engine → provider factory → provider adapter`，
再回到 session snapshot 和持久化状态。只有模型服务 HTTP 端点由本地
`httptest.Server` 替代；项目目录、文件工具、session、controller、runtime、凭据
存储和四个 provider adapter 都使用生产实现。

## 流程断言

`TestServiceProviderFormatsFlow` 对每种格式使用两个 provider，并按以下顺序运行：

1. 发送首轮 prompt。fixture 返回对应协议的流式 `read_file` tool call；生产
   runtime 执行真实 `read_file`，测试检查工具参数、成功状态和真实临时文件内容。
2. 发送续接 prompt。测试检查第二个请求携带之前的 tool call 和真实 tool result，
   session 收到 `fixture continuation complete`。
3. 调用 `Service.SetProvider` 切换第二个 model，再发送新 prompt，检查 session 的
   `ProviderID` 和第四个 wire request 的 model，session 收到
   `fixture model switch complete`。
4. 对四类请求逐项检查 endpoint、Gemini query、认证 header、model、prompt marker、
   请求数量和格式对应的历史结构：Responses 的 `function_call`/
   `function_call_output`、Chat 的 assistant `tool_calls`/`tool`、Anthropic 的
   `tool_use`/`tool_result`、Gemini 的 `functionCall`/`functionResponse`。

fixture 对每个请求检查 POST 方法，并只针对已知认证 header 集合拒绝非预期 header
携带 fixture key；不会把预期的 `Authorization`、`x-api-key` 或 `x-goog-api-key`
误判为泄露。每个请求体也拒绝出现 fixture API key；测试完成后递归扫描 data directory，确认 key 没有进入 session/controller 等持久化文件。

## 取消和错误断言

`TestServiceProviderFormatsCancellation` 让 fixture 阻塞在
`request.Context().Done()`，调用 `Service.Stop` 后等待 HTTP handler 观察到 context
取消，再检查 session 回到 `Interrupted` 且没有 session error。该断言覆盖应用层到
provider HTTP request 的取消传播，不用固定 sleep 推断取消已经发生。

`TestServiceProviderFormatsHTTPError` 让 fixture 返回 HTTP 400，并检查四个 adapter
都能把错误传回 session 的 `Failed` 状态。错误文本不能包含 fixture key；测试还会
把完整 `appmodel.Snapshot` 序列化后扫描 key，并递归扫描 data directory。fixture
会保留请求校验错误，避免把错误响应误判为 adapter 行为通过。

## 证据边界和当前缺口

本测试使用 loopback `httptest.Server`，只能证明 GCode 应用链路、请求映射、流式
解析、工具历史、取消和错误归属在本地协议替身上的行为。它不能证明真实厂商的
鉴权、模型可用性、限流、网络、TLS、厂商 schema 校验或真实 API 版本兼容性。
真实厂商 API、真实 credentials 和真实网络仍未验证，不能把本测试称为厂商 API
验收。

## 运行记录

首轮普通运行记录在
`output/acceptance/provider-formats-service/first-regular.log`。四个 Flow 子场景都
超时，但失败原因相同：session 已经回到 `idle`，真实 `read_file` 工具结果已成功
写入 `Message.Tools[].Output`，而等待谓词错误地在 assistant 的 `Message.Text` 中
查找文件内容。修正等待谓词后，Responses 子场景继续暴露了请求断言的索引错误：
`range` 的索引从 0 开始，却用 `step == 4` 判断第四个请求的 secondary model。
现已改为 `step := index + 1`，并让等待 helper 在非预期 `Failed` 状态立即输出
state、error 和 messages。

最终普通运行记录为
`output/acceptance/provider-formats-service/review-regular.log`，四类格式全部通过，
耗时 8.501 秒。最终 race 运行记录为
`output/acceptance/provider-formats-service/review-race.log`，四类格式全部通过，
耗时 10.665 秒；两次运行均包含 POST/auth-header、完整 snapshot 脱敏和最终的
Failed 状态即时诊断断言。Gemini loopback
请求在这两次 application focused 运行中实际
进入 adapter 并通过 endpoint、`alt=sse`、`x-goog-api-key`、流式 function call、
历史续接、切换 model、取消和 HTTP 400 断言；此前对 Gemini 会在本链路被 resolver
阻断的判断没有被本测试运行结果证实。配置 resolver 的其他入口仍不属于本测试范围。

上述记录使用本地 loopback fixture，不能替代真实厂商 API、真实 credentials、真实
网络、TLS、限流、模型可用性或厂商 schema 的验收。`go vet` 尚未在本子任务中运行。
