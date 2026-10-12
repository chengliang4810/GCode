# 桌面来源裁剪复核

日期：2026-10-12。状态：P1.5 通过，P5.1 仍为部分。复核范围为生产入口、编译依赖和订阅认证裁剪。

## 当前生产入口与依赖

`cmd/` 只保留 `cmd/gcode/main.go`。入口处理 Windows sandbox 的两个隐藏 self-dispatch 参数后进入 `application.New`；生产调用链为 `cmd/gcode → application.Service → runtime → providers.New`。

`go list -deps ./cmd/gcode` 成功，共 343 个依赖，未引入 `internal/cli`、`internal/tui`、`internal/doctor`、`internal/providerhealth`、`internal/provideroauth`、`internal/provideronboarding`、`internal/aimlapi`、`internal/providermodeldiscovery`、`internal/notify` 或旧 `cmd/zero-*`。遗留空目录没有 Go 文件，不构成编译包。

provider factory 仅按四类 APIFormat 路由：Responses、Chat Completions、Anthropic、Gemini，均为 API Key 认证。订阅 provider 的 `newCodexProvider`、`OAuthLoginCandidates`、`providerOAuth`、`ProviderOAuth`、`CodexProvider` 已不存在。

`internal/oauth` 的生产导入点仅为 MCP 的 `oauth.go`、`oauth_store.go` 和 `oauth_protected_resource.go`，保留标准 MCP OAuth 的 discovery、PKCE、token exchange、refresh 和 store。不能将其当作模型订阅认证删除。

## 已核验命令

以下命令由独立只读来源审计运行并确认成功；本轮全仓回归另见 `docs/verification/visual-v2.md`。

```sh
go list -deps ./cmd/gcode
go build -buildvcs=false ./cmd/gcode
go test -count=1 ./internal/providers/... ./internal/config ./internal/mcp ./internal/oauth
```

生产 Go 文件（排除测试）中 `zero setup`、`zero auth openrouter`、`gcode mcp oauth login`、`mcp oauth login`、`zero doctor`、`zero mcp`、`zero config`、`zero exec` 提示均已清理；providerio、MCP 和 resolver 现在提示通过模型服务/API Key 设置解决，不指向已移除的用户 CLI。

## 独立保留的缺口

- Windows sandbox helper 文案仍有 `gcode sandbox setup`，主入口实际接收 `__windows-sandbox-setup`。这是当前平台 helper 的文案一致性问题，纳入 Windows 实机验收；不属于旧 CLI/TUI 或订阅认证依赖。
- 部分品牌注释/错误仍有 `zero` 字样，不能据此声称已完成全仓品牌字符串清洁；P1.5 按入口与依赖功能判断通过。
- 四类 provider endpoint/认证头均有 S1 协议替身测试。真实厂商 API 的工具、取消、错误、历史和模型切换 S2 脱敏记录仍缺，P5.1 不通过完整验收。
