# MCP 工具名称限制核实

核实日期：2026-10-30。以下区分公开协议/产品限制、只读参考源码、
GCode 实现与静态市场前缀风险。没有连接市场 MCP 服务或使用真实供应商账号。

## 公开主源

- Anthropic Messages 自定义 `tools[].name` 明确匹配
  `^[a-zA-Z0-9_-]{1,128}$`：
  https://platform.claude.com/docs/en/agents-and-tools/tool-use/define-tools
- Claude Code 的 MCP 文档明确：完整名称超过 128 时，该工具不发送给 Claude，
  同一服务器其他工具仍可用。名称包含 MCP 前缀、服务器/插件命名空间和工具名；
  用户可缩短服务器名或请服务器作者缩短工具名。文档没有承诺自动截断或 hash 别名：
  https://code.claude.com/docs/en/mcp#tools-with-names-longer-than-128-characters
  https://code.claude.com/docs/en/errors#mcp-tool-name-over-128-characters
- OpenAI Chat Completions `tools[].function.name` 明列最长 64 字符，
  字符集为字母、数字、下划线和连字符：
  https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create
- 最新 OpenAI Responses create 文档的 `tools` / Namespace / `tools` /
  Function / `name` 明列 `maxLength: 128` 和上述字符集。
  同页顶层 Function / `name` 只写 string 及用途，未另列长度。
  因此不能把 Chat 的 64 当作 Responses 已明确规定的统一上限，
  也不能将 namespace 子字段的 128 宣称为已经直接证明 flat 字段硬上限。
  GCode 当前选择 Responses 允许到 128 的本机边界，真实供应商接受度尚未实测：
  https://developers.openai.com/api/reference/resources/responses/methods/create
  https://developers.openai.com/api/reference/resources/responses/methods/create.md

## 本地参考源码的限定证据

只读参考 `D:/projects/claude-code` HEAD
`8d80e740f764be358e265eed4250606c78fca579`：

- `src/services/mcp/mcpStringUtils.ts:39-51` 直接连接规范化服务器前缀与工具名，
  没有截断、hash 或别名。
- `src/services/mcp/normalization.ts:9-24` 注释仍写旧的 `{1,64}`，
  但函数实际仅替换非法字符；没有执行 64 长度校验或裁剪。
  注释与当前官方 128 公开文档不一致，不能用此注释作为长度实现依据。
- `src/services/mcp/client.ts:1764-1786` 从真实 `tools/list` 结果调用上述命名函数，
  保留原始服务器/工具信息用于调用。这里只证明该参考路径不截断，
  不把它等同于当前官方发行二进制的完整错误处理。

## 官方 315 条静态风险

计算使用同目录 `marketplace-audit.json` 当时的完整官方 315 条快照，
固定 marketplace commit `f8c5e7a1dcb71b10cb74586224b5401f3de5db28`。
结果及每个静态服务器前缀见 `mcp-name-prefix-risk.json`：

- 170 条发现 MCP 组件文件；168 条解析出命名服务器，去重后 213 个
  entry/server 对。
- 两条文件未提供命名服务器：`shopify-ai-toolkit` 是空 `mcpServers`，
  `ui5-typescript-conversion` 是空对象。这不是服务不可运行的推断。
- 规范化完整前缀长度 21–75。3 个前缀已经超过 64：
  Informatica 的两个服务器为 75、74，BlackRock 的一个服务器为 70。
  因而这些前缀下任何非空工具名都会超过 64。
- 15 个服务器前缀在 64 边界下最多只剩 16 字符给工具名；
  117 个最多只剩 32 字符。128 边界下没有任何前缀已耗尽非空工具名预算。
- Informatica 两个静态 URL 为空，仍需配置；不能把命名空间放宽称为这两个服务器已可用。

这里统计的是前缀预算，不是真实工具清单。没有运行服务器、
没有调用 `tools/list`，不知道各服务器实际暴露多少工具及其名称；
不能据此计算“多少个实际工具超过 64/128”或插件运行率。

## 当前产品修正与验证

GCode 的通用 MCP 层保留完整命名空间，最大 128；
超过该值仅跳过单个工具，保留 typed `ToolDiagnostics` 并发通知。
普通运行汇总服务器发现诊断，再按本次协议筛选：
Chat 64，Messages / Responses 128；worker 系统上下文包含被排除工具及原因。
不截断、hash 或新增旧别名。

新增本地 loopback HTTP 证据覆盖三协议完整 engine → model adapter →
MCP `tools/call` → model 历史：97 字符完整工具名在 Messages/Responses
原样执行；Chat 排除它并执行同服务器短名工具；超过 128 的另一个工具
仅被单独排除，worker 收到诊断且会话继续。此证据不等于真实外部供应商验证。
