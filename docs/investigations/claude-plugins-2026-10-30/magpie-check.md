# 本地 Magpie 与测试模型检查

检查日期：2026-10-30。只读检查既有配置和服务，没有重启、更新或修改 Magpie，
也没有修改用户 GCode profile。配置中的密钥、账号标识和请求正文未写入本报告。

## 实际服务

- `http://127.0.0.1:3425/api/hello` 返回 Magpie，版本 `0.1.2191`。
- 本机 GCode 配置为 `local-gateway/deepseek-v4.1-flash`，协议为
  Anthropic Messages，Base URL 为 `http://127.0.0.1:3425/v1`。
  当前适配器最终请求 `/v1/messages`，实际调用已经成功。
- 授权读取 `/v1/models` 得到 17 个条目。其中路由
  `group/deepseek-v4.1-flash` 声明 1000000 上下文、128000 输出上限、
  工具调用与推理能力。这是网关的能力声明，不能替代对应上限的实际运行验证。
- 本机 Magpie 的 DeepSeek 路由包含多个模型来源；最近插件验收请求实际由
  `dawanfan/deepseek-v4.1-flash` 响应，记录为
  `/v1/messages → /v1/chat/completions`。因此记录的是经过协议转换的本地路由，
  不能声称直接调用了 DeepSeek 官方接口。
- Magpie 配置的 `modelOutputs` 对该来源声明 128000；
  GCode 的该模型 `MaxOutput` 为 0。

GCode 冻结的是执行时选定的供应商、模型 ID 和配置。Magpie 路由仍可在同一逻辑模型
背后选择不同来源；因此 `sonnet/opus/haiku` 继承当前逻辑模型并不保证每次调用都落到
相同上游账号或相同实现。这属于用户现有网关路由的行为，验收需要记录实际响应与路由，
不能仅从别名映射正确推断质量相同。

## 8192 截断原因

`internal/model/messages.go` 在未配置输出预算时发送 `max_tokens: 8192`。
此前 `frontend-design` 失败的实际请求沿用这一默认值。
对应 Magpie 用量记录为 HTTP 200、输出 8192、推理 8192、`stop: length`；
验收侧 SSE 为 `stop_reason: max_tokens`，业务产物未完成。

这能够解释本次预算耗尽，不能据此推断 Magpie 本身只能输出 8192，
也不能把 HTTP 200 记作插件通过。隔离复测将显式设置 32768 输出预算，
保留默认预算下的失败证据，并记录复测条件；用户的日常配置不变。

## 账号与凭据边界

Magpie 官方集成文档说明其本地网关接收应用标识用的 key，
本机占位 key 与上游供应商 key 是两类值。官方参考：
<https://usemagpie.ai/docs/integrate> 和 <https://usemagpie.ai/docs/start>。

本次只读逐一核查了旧验收遗留的 10 份 `engine.json`：
每份仅有一个非空 APIKey，全部是已知的本地网关占位值；
未在这些文件中发现上游供应商 APIKey。此前“账户副本”的表述不能解释为
已经复制或泄露厂商密钥。文件位置与删除被拒记录见
`private-artifact-cleanup.md`，旧文件保持未修改。

模型订阅、路由和模型 API 已经具备真实调用证据。GitHub、Notion、Slack 等
插件所需的业务服务账号、授权和数据访问仍需逐项验证；
Magpie 的模型调用成功本身不是这些业务服务已授权的证据。
