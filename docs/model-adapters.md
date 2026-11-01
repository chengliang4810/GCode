# 模型调用边界

2026-10-29 起，本机桌面和无桌面服务使用同一条模型调用链：

```text
Engine 解析用户配置并绑定渠道/模型
                  │
Agent.Model.Stream（统一 Go 接口）
                  │
          model.Gateway
             ├─ chatAdapter       → /chat/completions
             ├─ responsesAdapter  → /responses
             └─ messagesAdapter   → /messages
                  │
        公共 HTTP 请求与 SSE 分帧
                  │
       各适配器解析 → Delta / Result
```

这是对本仓库原有 Go 实现的独立重构。Peri 仅用于分层思路参考，没有引入、翻译或
复制 Peri 源代码，也没有增加 Peri 运行时或依赖。项目的许可证文件不因本次调整而变更。

## 调用方

`internal/agent` 定义自己消费的 `Model` 接口，只有 `Stream` 一个方法。
Agent 的请求包含系统提示词、历史、工具定义和推理等级；不携带渠道协议、地址、
API Key 或具体模型配置。上下文容量由 Engine 单独传入，用于估算压缩触发条件。
手动和自动压缩均复用同一接口。

`internal/engine` 通过 `Client.Bind(provider, model)` 创建 `Gateway`，之后交给 Agent。
绑定时校验配置，冻结该次运行使用的协议、地址、凭据和模型。运行中的设置修改不改变
已接受任务的目标；后续运行重新绑定。供应商名称与协议无关，不按品牌或模型名称猜测能力，
不回退到另一供应商或官方地址。

模型思考能力使用显式 `ReasoningLevels` 数组，不再使用布尔值生成统一档位。
空数组不发送思考参数；新选模型未指定档位时取数组最后一项。
已有显式无效档位不会被悄悄改成默认；`none` 必须由模型配置明确列出。
Chat 发送 `reasoning_effort`，Responses 发送 `reasoning.effort`；
Messages 使用 `thinking.type=adaptive` 和 `output_config.effort`，显式 `none`
发送 `thinking.type=disabled`，不再把不同等级全部映射为同一个固定预算。
Messages 的 `minimal` 配置无效。实际模型接受哪些等级仍由用户配置声明，
本机校验及请求夹具不能证明远端服务支持所填能力。

本机这几层之间都是强类型 Go 调用。JSON 请求编码和 SSE 解析只发生在访问模型服务时，
不会给本机 UI 与业务核心增加 HTTP、RPC 或进程间桥接。

## 三种适配器

每个适配器负责自己的路径、鉴权、请求字段和流事件解析：

| 文件 | 协议与鉴权 | 主要转换 |
| --- | --- | --- |
| `internal/model/chat.go` | Chat Completions；Bearer | system 消息、function tools、tool_calls、tool 结果 |
| `internal/model/responses.go` | Responses；Bearer | instructions、input、function_call、function_call_output |
| `internal/model/messages.go` | Anthropic Messages；x-api-key | system、content blocks、tool_use、tool_result |

共同请求类型为 `model.Request`，流式展示增量为 `model.Delta`，完整结果为 `model.Result`。
工具调用统一为 ID、名称和 JSON 对象参数。停止原因归一为正常完成或需要工具；
无法支持的停止状态、输出截断和不完整工具调用明确失败，不伪装成正常完成。
一个批次的全部调用通过校验后才发布给 Agent，重复调用 ID 会使整个批次失败。

工具返回 `model.ToolOutput`，文字与图片分别进入统一历史；适配器负责各协议的图片位置。
Chat 在同批全部工具结果之后追加图片用户消息，Responses 和 Anthropic 将图片嵌入工具结果
的内容数组。图片不会降成普通 Base64 文本，也不会被转协议时丢弃。详见 [Read 工具](read-tool.md)。

公共 HTTP 层处理取消、HTTP 状态、有限重试和拒绝自动重定向；
公共 SSE 层只处理分帧和增量投递，不识别供应商事件字段。
三种协议各自产生完整结果前都必须收到相应的完成标记。
Responses 的最终正文须与已发增量前缀一致，缺少的后缀会补发；冲突则报错。

现有重试语义保留：HTTP 429/5xx 在响应流开始前最多重试两次；缺少完成事件时，
Agent 最多重试当前未提交的模型步骤两次，替换该步骤的临时展示，不重复之前已经完成的工具。
该机制没有改为 Peri 的部分输出续跑策略。

## 协议续接数据

`Message.Continuation` 是模型层拥有的不透明状态，Agent 和 Engine 只能保存、
传递或通过 `model.CloneMessages` 复制，不读取其中的供应商字段。
Responses 的原始输出项、Anthropic 的有序思考块和签名留在这里，
用于同一目标下的工具往返及重启续接；它们不是 UI 展示历史的替代物。

续接状态按渠道 ID、协议、Base URL、凭据和模型 ID 的摘要隔离，不将凭据写进续接对象。
目标相同时由原适配器读取；任一目标要素改变时，仅发送可移植的正文、图片、工具调用
和工具结果，不回放不匹配的私有状态。原历史仍保留，切回原目标后可继续使用原续接数据。
Engine 不再在切换模型时逐字段删除思考、签名或 Responses 输出。

压缩保留的近期消息仍携带它们原有的续接状态。替换为摘要的历史前缀不再作为原始协议
消息发送。没有增加旧字段别名、旧格式读写或历史数据迁移。

## 验证范围

本地确定性测试覆盖三协议的请求、鉴权、流解析、工具往返、错误、取消/重试及持久化。
新增测试覆盖模型接口注入、配置绑定、私有续接 JSON 往返、消息克隆所有权、
渠道/模型/地址/凭据/协议切换隔离，以及 Engine 重启和切换模型后再切回原目标。
这些夹具不代表本次已通过真实供应商的三协议实测。

2026-10-29 本轮执行结果：

- `go test -count=1 -p 1 ./...`、`go vet ./...`、`go build ./...` 通过。
- Windows/amd64 与 Linux/amd64 无桌面入口构建通过；Linux 依赖图不含 MyGo、
  `internal/desktop` 或 `internal/workbench`。本轮未在 Linux 上运行构建产物。
- 独立只读审阅完成，发现的停止语义、重复工具 ID、Responses 最终输出一致性问题
  已修复并添加回归。
- 首次与构建并行执行的全仓测试出现一次 MCP 子进程清理超时；该包单独复跑及随后
  串行全仓测试均通过。本轮未修改 MCP 清理实现。

串行测试输出保存在 `output/model-adapter-check/tests.txt`。本轮没有真实供应商三协议
调用证据，也没有运行 race 检查；验证结果不扩展为这两项已经通过。

后续子 Agent/profile 调整增加了逐模型档位、配置深复制和跨供应商子任务调用测试；
普通子 Agent 与主会话复用同一个 Gateway/适配器实现。详见 [子 Agent](subagents.md)。

`Usage.Known` 区分供应商明确返回的零与缺失计数。流中断/错误保留已经收到的计数下限；
Anthropic 输入量包含缓存读取与创建量，Chat/Responses 不重复加其缓存子集。
目标执行在统一模型接口外统计工作、子任务、压缩和检查请求，未知用量的预算处理见
[目标执行](goals.md)。
