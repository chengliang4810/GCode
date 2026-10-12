# 阶段 5：扩展能力 S2 应用层集成验证

记录日期：2026-10-12。

## 结论

S2 本地应用层集成验证通过。测试从真实 `application.Service` 入口启动，完整经过：

```text
application.Service → controller.Controller → runtime.Engine → Chat Completions adapter
```

本地 HTTP Chat Completions fixture 只替代外部模型服务；MCP、插件、Hook、Skill 和 specialist 均通过生产代码加载或启动真实子进程。主 session 的工具结果、告警、子 session 父子关系和历史事件均已持久化。`Stop` 与 `Close` 都能取消 child provider 请求，`Close` 同时释放存活的 MCP stdio 进程。

这份证据覆盖本地应用边界，不等同于真实厂商 API、OAuth、原生窗口或目标设备验收。

## 环境与隔离

- 环境：macOS Darwin 23.6.0 x86_64，Go `go1.27.2 darwin/amd64`。
- 每个测试使用独立的 `t.TempDir()` 作为 `DataDir`、项目目录、日志目录和进程标记目录。
- provider 使用 `httptest.NewServer` 提供 `/v1/chat/completions`，检查 `Authorization: Bearer <fixture-key>`，并返回真实 Chat Completions SSE 数据。
- MCP、插件和 Hook 命令都启动当前测试二进制的 helper entrypoint，因此生产客户端会经过真实进程启动、stdio、工作目录和退出路径；helper 不访问生产数据目录，也不调用远程服务。

## 覆盖矩阵

| 能力 | 应用层配置或入口 | 真实验证 |
| --- | --- | --- |
| 用户 MCP | `<DataDir>/config.json` | `user-demo` stdio server，工具 `mcp_user_demo_echo`，完成 initialize、tools/list、tools/call |
| 项目 MCP | `<project>/.gcode/config.json` | `project-demo` stdio server，工具 `mcp_project_demo_echo` |
| 插件 | `<DataDir>/plugins/application-runtime-test/plugin.json` | 生产插件 runner 启动两次 helper，回收 stdout 并记录项目 cwd |
| Skill | `<DataDir>/skills/runtime-skill/SKILL.md` | `skill` 工具读取并返回 Skill body |
| 项目 Hook | `<project>/.gcode/hooks.json` | `afterTool` 生产 dispatcher 启动 helper，写入 JSONL 并校验 cwd |
| specialist | `<DataDir>/specialists/runtime-child.md` | `Task` 经过 specialist executor 创建 child session，再进入 runtime.Engine |
| malformed 配置 | 用户层 `hooks.json`、无效 plugin JSON、带未知 frontmatter key 的 specialist | 通过应用 warning 消息和事件暴露诊断，合法扩展仍可运行 |
| session store | `<DataDir>/sessions` | 主 session 的 tool result/event 与 child session 的历史、父子关系、cwd |

测试显式禁用默认 `exa` MCP，避免测试因默认配置发起外部网络请求。

## 真实触发链

`TestServiceExtensionsIntegration` 的本地 provider 按顺序返回 8 个 parent 请求和 1 个 child 请求：

1. 插件 `plugin_runtime_echo` 成功执行。
2. 插件再次执行并返回结果。插件 manifest 的静态权限仍为 `prompt`，但 macOS native sandbox 为该普通 shell 子进程提供了安全边界，因此两次调用均由 sandbox 自动放行；测试不把这两次调用解释为交互式 deny。
3. 用户 MCP 工具获得应用层 `allow`，真实 stdio server 返回结果。
4. 同一用户 MCP 工具获得应用层 `deny`，工具历史记录拒绝信息，server 日志不会出现第二次调用。
5. 项目 MCP 工具获得 `allow` 并返回结果。
6. `skill` 返回 Skill 文件正文。
7. `Task` 启动 `runtime-child`，child provider 返回 `application child final`。
8. parent provider 返回最终文本 `application extensions complete`。

首次 Chat Completions 请求会声明插件、两个 MCP 工具、`skill` 和 `Task`。项目 `afterTool` Hook 对工具调用产生至少五条 JSONL 记录，且每条记录的 cwd 都等于项目目录。插件日志包含两次真实执行及对应参数和 cwd。

## 持久化与脱敏

- 应用 snapshot 中保留成功、拒绝和 child 工具结果；Task 结果包含 child final answer。
- 主 session 的 `events.jsonl` 包含 warning message 和 tool result 事件。
- `sessions.Store.ListChildren` 返回一个 specialist child，父 session ID、项目 cwd 均正确；child 重放历史包含 `application child final`。
- malformed plugin、malformed Hook 和 specialist manifest warning 都进入主 session；provider key 不出现在 warning、snapshot 或序列化 JSON 中。
- `Close` 后通过 helper 写入的 PID 使用 `syscall.Signal(0)` 探测，确认两个 MCP stdio 进程已退出。测试不依赖 helper 在被终止后写入 `.done` 文件。

为保证 child warning 不破坏结果回填，`controller.onToolResult` 会在同一 run 中按 tool call ID 查找包含该调用的 assistant message，而不是只查看最后一条消息。child 加载 malformed specialist 时可能先追加 warning；结果仍必须回填原 Task tool call。

## Stop 与 Close 取消

`TestServiceExtensionChildCancellationOnStopAndClose` 覆盖两个场景：

- `Service.Stop`：child provider 请求开始后，Stop 传播 context cancellation；主 session 最终为 `Interrupted` 且无 active run。
- `Service.Close`：child provider 请求被取消；测试同时启动一个真实 MCP stdio server，确认 `Close` 完成并且该 MCP PID 已退出。

两种场景都使用本地 HTTP handler 的 `request.Context().Done()` 作为 child 请求取消证据，没有用 sleep 推断取消。

## 验证命令与结果

```sh
go test -mod=mod -vet=off ./internal/application \
  -run 'TestServiceExtensionsIntegration|TestServiceExtensionChildCancellationOnStopAndClose' \
  -count=1
```

结果：通过。

```sh
go test -mod=mod -vet=off -race ./internal/application -count=1
```

结果：通过。

## 证据边界与未覆盖项

- HTTP server 是本地确定性 fixture，验证生产 Chat Completions adapter 的 URL、Bearer key、SSE 解码和应用边界；没有验证 OpenAI 或其他厂商的真实 API、网络质量、模型决策、OAuth、账户限流或服务端错误格式。
- MCP 验证的是本地 stdio 协议和应用层授权，不包含远程 MCP OAuth 或真实第三方 server。
- 没有操作原生窗口，因此未验证桌面扩展配置界面、实时回调渲染、原生退出 UI 或桌面签名包。
- 本包通过不代表 `go test ./...`、`go vet ./...` 或阶段 5 其他平台/设备验收已经完成。
