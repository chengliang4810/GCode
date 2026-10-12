# /goal：四类 API Key Provider

状态：四类 API Key provider、config 和 provider catalog 已完成收敛；ChatGPT/Codex 订阅入口、provider OAuth 元数据及专用模型目录已移除，OAuth 仅供标准 MCP 使用。桌面无消费者的 doctor、providerhealth 和 providermodeldiscovery 已删除。协议替身测试已通过，真实厂商 API 尚未用凭据验证。

## 目标

在 GCode 内部提供四类可独立配置的 API Key provider：

- OpenAI Responses（通用 API Key）
- OpenAI Chat Completions
- Anthropic Messages
- Gemini Generate Content

所有实现都维持 `zeroruntime.Provider` 契约：
`StreamCompletion(context.Context, CompletionRequest) (<-chan StreamEvent, error)`。
事件必须保留流式文本、reasoning、工具调用及结果所需的 call id/参数片段、usage、finish reason、响应错误和 context cancellation 语义；历史重放不能丢失跨 provider 所需的 reasoning 签名或工具调用关联。

## 构造和配置合同

controller 的推荐构造入口是：

```go
providers.NewTurnSessionProvider(profile, providers.Options{})
```

该入口返回 `zeroruntime.TurnSessionProvider`，用于获得 provider 和能力投影；只需要一次请求时可调用：

```go
providers.New(profile, providers.Options{})
```

其中 `profile` 至少需要以下字段（实际类型由 root 负责的 config 包确定）：

| 字段 | 作用 |
| --- | --- |
| `Name` | 配置项显示/识别名称；不参与 provider kind 或模型 allowlist 映射 |
| `Provider` 或 `ProviderKind` | 内部 transport kind；可由 `APIFormat` 与 `BaseURL` 推导，不从 `Name` 推导 |
| `APIFormat` | 主 wire 路由鉴别值：`responses`、`chat-completions`、`anthropic`、`gemini` |
| `BaseURL` | 可选 endpoint 覆盖；空值使用官方默认值 |
| `APIKey`/凭据引用 | 普通 API Key；不得依赖订阅 OAuth |
| `Model` | 上游 API model id |
| `CustomHeaders`/认证字段 | 兼容网关的明确 header 覆盖 |

官方默认 base URL：

- OpenAI Responses/Chat：`https://api.openai.com/v1`
- Anthropic：`https://api.anthropic.com`
- Gemini：`https://generativelanguage.googleapis.com`

ChatGPT/Codex 订阅 catalog、专用模型目录、`newCodexProvider` 和 provider OAuth resolver 不属于 GCode 当前产品路径；通用 Responses 必须从 `APIFormat=responses` 路由到普通 `/responses` API Key endpoint。MCP server 的 OAuth 仍按 MCP 协议独立处理。

## 协议依据

- OpenAI Responses：SSE 事件包含 `response.output_text.delta`、reasoning delta、`response.function_call_arguments.delta/done`、完成、`response.incomplete` 和错误事件；请求使用普通 API Key Bearer。`store:false` 的 stateless tool loop 会从 `response.output_item.done` 保存完整 reasoning item，并在下一轮连同 `summary`、`encrypted_content`、`id` 和 `status` 原样回放；`response.incomplete` 会保留真实终止原因及 usage。
- OpenAI Chat Completions：流式 chunk 从 `choices[].delta.content` 和 `choices[].delta.tool_calls[].function.arguments` 累积；`finish_reason` 覆盖 `stop`、`length`、`tool_calls`、`content_filter`，usage 可由 `stream_options.include_usage` 获取。
- Anthropic Messages：`POST /v1/messages`，使用 `anthropic-version: 2023-06-01` 与 `x-api-key` 或 Bearer；SSE 解析 `message_start`、`content_block_*`、`message_delta`、`message_stop`，thinking/signature 原样保存以支持工具历史回放；tool result 保留 `is_error`，`model_context_window_exceeded` 映射为 length 截断。
- Gemini：`POST /v1beta/models/{model}:streamGenerateContent?alt=sse`，使用 `x-goog-api-key`；解析 candidate parts 的 text/functionCall、`thought:true` reasoning、`thoughtSignature` 和 `usageMetadata`，并在下一轮回放 Gemini thought block 与 function-call signature。

官方资料：

- [OpenAI Responses types](https://developers.openai.com/api/reference/typescript/resources/responses)
- [OpenAI function calling](https://developers.openai.com/api/docs/guides/function-calling)
- [OpenAI reasoning](https://developers.openai.com/api/docs/guides/reasoning)
- [OpenAI Responses streaming events](https://developers.openai.com/api/reference/resources/responses/streaming-events)
- [OpenAI Chat Completions](https://platform.openai.com/docs/api-reference/chat/create)
- [Anthropic Create a Message](https://platform.claude.com/docs/en/api/messages/create)
- [Anthropic streaming](https://platform.claude.com/docs/en/build-with-claude/streaming)
- [Anthropic stop reasons](https://platform.claude.com/docs/en/build-with-claude/handling-stop-reasons)
- [Anthropic tool use](https://platform.claude.com/docs/en/agents-and-tools/tool-use/handle-tool-calls)
- [Gemini Generate Content](https://ai.google.dev/api/generate-content)
- [Gemini thought signatures](https://ai.google.dev/gemini-api/docs/thought-signatures)

## 实施和验证

provider 层及其直接的发现、健康检查和 config 依赖已按以下边界完成：

1. 已迁移 zero provider 实现及 `providerio`，包 import 统一到 GCode module/`zeroruntime`。
2. 已将原 Responses 协议解析整理为通用 `openai.ResponsesProvider`，从 `APIFormat=responses` 构造普通 API Key `/responses` 请求；订阅身份、account header、OAuth refresh 和 Codex websocket 生产构造已从 provider 层移除。
3. `providers.New` 按 `APIFormat` 路由 `responses`、`chat-completions`、`anthropic`、`gemini`；Anthropic/Gemini 同时识别现有 catalog 的 `messages`/`generate-content` 输入并归一化到同一实现。
4. 已在 `httptest` 协议替身中覆盖四类请求路径与认证 header、流式文本、reasoning、Responses stateless reasoning item 回放、Gemini thought/signature 回放、Anthropic tool error/stop reason、tool call/result 历史、usage、完成原因、HTTP/SSE 错误和 context cancellation；定向 provider 包测试覆盖这些协议边界。
5. 替身测试只证明协议处理，不能代替真实 API 凭据验收。config 已移除 `OAuthLoginCandidates` 及订阅 OAuth/Codex 注释和测试。config/factory 都按四类 wire format 处理默认值：OpenAI/OpenAI-compatible 默认 `chat-completions`，Anthropic/Anthropic-compatible 默认 `anthropic`，Google 默认 `gemini`；只提供 `APIFormat` 时按官方或自定义 `BaseURL` 映射 transport kind。名称不会改变 transport kind，也不会把第三方自定义模型挡在 unrestricted provider 外，受限 catalog 的 allowlist 仍生效。桌面未消费的 doctor、providerhealth 和 providermodeldiscovery 随源码裁剪移除，不再保留对应入口或测试。

当前保留包的定向验证命令：

```text
go test -count=1 ./internal/config ./internal/providers/... ./internal/providermodelcatalog
```

当前边界：

- provider 生产路径只接受普通 API Key、显式自定义认证或无需认证的本地 provider；provider catalog 不再提供订阅账户或 provider OAuth 入口。
- MCP 的 `MCPOAuthConfig`、MCP merge、MCP discovery 和 identity-bound token store 独立保留。
- 协议替身和仓库定向测试覆盖已完成；真实厂商 API、真实凭据、账户配额和线上网络行为仍未验证。
- provider stream idle timeout 的环境变量为 `GCODE_STREAM_IDLE_TIMEOUT`，不提供旧名称兼容。
