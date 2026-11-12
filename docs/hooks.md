# 钩子

Hook 的外部事件与 JSON 契约遵循 [Claude Code 公开规范](https://code.claude.com/docs/en/hooks)，
本次核对日期为 2026-10-30。内部使用 Go 结构体和回调；只有真正跨进程的命令、
跨网络的 HTTP 与 MCP 适配器才进行 JSON 编码。用户设置、插件与被激活技能的规则
在同一个宿主执行，不借桌面传输层绕行本机核心。

本次下载的官方 `hooks.md` 为 283,477 字节，SHA-256：
`B755D9C7C166EC77E36EE2388E3299DD149A9F160171B6E97E427BE439F942F9`。

## 规则与执行方式

`contracts.HookRule` 是 matcher 分组与单个 handler 合并后的类型化形态。
插件原始文件仍使用 Claude 的 `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[...]}]}}`，
资源加载器负责展开，不要求插件改成 GCode 的设置存储结构。

- `command`：`args` 缺失或 `null` 时执行 shell；存在数组时直接启动可执行文件，
  包括 `args: []` 的无参数执行。没有 `process` 类型。
- `http`：向 `url` POST 同一个事件对象。只接受 2xx JSON 响应；响应上限 1 MiB。
  `headers` 仅展开 `allowedEnvVars` 允许的环境变量，其余变量为空。
- `prompt`、`agent`：通过宿主注入的 `HookEvaluator` 使用实际对话模型执行。
  输出为 `{"ok":true}` 或 `{"ok":false,"reason":"..."}`。
  `$ARGUMENTS` 替换为事件 JSON；未写占位符时追加到提示尾部。
- `mcp_tool`：通过同一个类型化评估器调用已连接服务的 `server`/`tool`。
  `input` 的字符串递归展开 `${tool_input.command}`、`${session_id}` 等事件字段；
  保持 JSON 转义，不拼接成执行脚本。返回普通 Hook JSON。

没有注入对应的模型或 MCP 评估器时，执行记录明确报告缺少运行时。类型被识别、
规则能加载，不等于真实模型或服务已运行。测试中的注入评估器也不构成真实供应商证据。
公开事件与 handler 类型的支持矩阵在 `contracts.HookTypeSupported` 中统一维护，
不支持的组合跳过执行。

真实宿主已注入当前执行上下文绑定的模型与已连接的 MCP 工具。prompt 单次调用；
agent 使用真实工具循环，最多 50 次模型请求，并排除 Agent、AskUserQuestion、
计划模式和任务生命周期等递归或交互入口。`rule.Model` 不作为供应商模型 ID 发送；
用户为子 Agent 配置的模型已在进入该执行上下文前决定。
agent Hook 按实际绑定的模型协议筛选 MCP 完整工具名：Chat 最多 64 字符，
Messages/Responses 最多 128 字符。超长工具不改名，跳过原因与发现阶段诊断
写入评估上下文；手动压缩的模型请求也接收这些诊断。

Unix shell form 默认 `sh`，可用 `shell: "bash"`；Windows 优先 Git Bash，
没有 Git Bash 时使用 PowerShell。`shell: "bash"`/`"powershell"` 明确选择对应 shell。
`CLAUDE_CODE_GIT_BASH_PATH` 可指定 Git Bash；项目的 Bash 工具共用本机路径定位，
因此也识别 `RCODE_GIT_BASH_PATH`。exec form 在 Windows 不接受 `.cmd`/`.bat`。
脚本内容不进行 Bash 与 PowerShell 语法互译。
这是 Hook 自身的公开约定；名为 `Bash` 的业务工具仍要求真正的 Bash，
不能把两者的默认 shell 混为一谈。

通用环境包括 `CLAUDE_PROJECT_DIR`；插件规则另有 `CLAUDE_PLUGIN_ROOT`、
`CLAUDE_PLUGIN_DATA` 和 `CLAUDE_PLUGIN_OPTION_<KEY>`。当前推理配置可通过
`CLAUDE_EFFORT` 传入。`OTEL_*` 不传给子进程。`${CLAUDE_PLUGIN_ROOT}` 等路径
在启动前展开；用户配置仅允许在 exec form 使用 `${user_config.key}`，
shell form 使用对应环境变量，避免把配置值当作脚本。

默认 command/http/mcp 超时 600 秒；prompt 30 秒，agent 60 秒。
`UserPromptSubmit` 与模型切换默认 30 秒，`MessageDisplay` 默认 10 秒；
`SessionEnd` 默认 1.5 秒，显式超时最多 60 秒。`timeout` 可按规则指定秒数，支持小数秒，不截断为整数。
`async: true` 与 `asyncRewake` 同样受超时约束（未指定时按上表默认时长），
异步 Hook 在宿主上下文中运行，不阻塞当前回合，其结果在后续边界投递；
宿主关闭时后台 Hook 被取消并回收。
本机命令被取消时回收进程树。

## 输入与输出

命令从 stdin 读取一个 JSON 对象，末尾带换行。字段遵循公开 snake_case：

```json
{
  "hook_event_name": "PreToolUse",
  "session_id": "session",
  "transcript_path": "/host/session/transcript.jsonl",
  "cwd": "/host/project",
  "tool_name": "Bash",
  "tool_use_id": "call",
  "tool_input": {"command": "git status"}
}
```

事件按需携带 `prompt`、`prompt_id`、`permission_mode`、`agent_id`、`agent_type`、
`tool_response`、`error`、`last_assistant_message`、`stop_hook_active` 等。
`tool_response` 保留工具的结构化结果，不统一压成文本。内部 run ID、父会话 ID
和 UI 输入追踪 ID 不作为额外协议字段发送。

`PreToolUse` 的允许、拒绝、询问及参数改写：

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "ask",
    "permissionDecisionReason": "确认此次操作",
    "updatedInput": {"command": "git status"},
    "additionalContext": "使用改写后的参数"
  }
}
```

`permissionDecision` 为 `allow`、`deny`、`ask` 或 `defer`；`defer` 丢弃改写与附加
上下文。`updatedInput` 实际替换调用参数，并通知宿主更新审计和展示。默认允许的
工具只触发 `PreToolUse`，不会伪造一次 `PermissionRequest`。仅在确需权限决定时
触发后者；其输出使用 `hookSpecificOutput.decision.behavior = allow/deny`。
没有规则决定时交给宿主类型化权限回调，没有可用回调则阻止执行。

`UserPromptSubmit`、`Stop` 等采用顶层 `{"decision":"block","reason":"..."}`；
不同事件按公开矩阵解释，不把所有事件统一成 allow/deny。`continue:false` 与
`stopReason` 是停止处理信号，区别于 `Stop` 的 block（要求继续工作）。
`Stop`/`SubagentStop` 的 `additionalContext` 也会继续会话，与 block 共用
`stop_hook_active` 和连续最多八次的保护。
`PostToolUse` 可通过 `updatedToolOutput` 改写结果；内置工具按结果形状校验，
无效值保留原输出并记录诊断，MCP 结果允许服务自定形状。
`updatedMCPToolOutput` 只对 MCP 生效。当前 Read 改写支持文本结果，
尚未实现图片或其他 Read 变体的结果改写。观测类事件忽略控制字段。
普通文本 stdout 仅在 `SessionStart`/`UserPromptSubmit` 进入上下文；
`WorktreeCreate` 的命令 stdout 是路径。HTTP 只能返回 JSON。

退出码 2 按事件处理：工具执行前可阻止动作，工具执行后把 stderr 反馈给模型；
`PermissionRequest` 与观测事件不把退出码 2 当成结构化权限拒绝。
退出码 2 仍读取 JSON 字段，且保持事件对应的阻止效果；Elicitation 是例外，
会忽略同次输出的 `hookSpecificOutput`。只有真实进程退出才在 `hook.finished`
上记录 `ExitCode`，进程未启动、HTTP 与模型评估没有伪造的退出码。
其他退出码、超时、无效 JSON 默认记录诊断并继续；适用事件可以显式
`onFailure: "block"`。Stop、SubagentStop、TaskCompleted、TeammateIdle 不因
执行错误自动进入继续循环。宿主上下文取消及审计记录保存失败仍传播给调用方。

多个匹配 handler 并行执行，按注册顺序汇总上下文和参数改写。权限冲突优先级为
deny、ask、allow。prompt/agent 同时最多 12 个。matcher 为简单工具名时精确匹配，
支持 `|`/逗号列表；带正则字符时用 ECMAScript 正则执行，单次匹配限时 100 ms；
非法正则按公开行为退回子串匹配。没有 matcher 定义的事件忽略 matcher，
不会拿最终回复或用户提示内容做匹配。

`if` 用工具权限表达式进一步筛选工具事件，例如 `Bash(git push *)`。
命令匹配会检查复合命令与命令替换，保留引号文本边界。这里只用于筛选 Hook，
不是完整的命令沙箱或 shell 解析器。

## 生命周期与证据边界

`HookEvents()` 声明公开的 33 个事件：
SessionStart、Setup、InstructionsLoaded、UserPromptSubmit、UserPromptExpansion、
MessageDisplay、PreToolUse、PermissionRequest、PermissionDenied、PostToolUse、
PostToolUseFailure、PostToolBatch、Notification、SubagentStart、SubagentStop、
TaskCreated、TaskCompleted、Stop、StopFailure、TeammateIdle、ConfigChange、
CwdChanged、DirectoryAdded、FileChanged、WorktreeCreate、WorktreeRemove、
PreCompact、PostCompact、PreModelSwitch、PostModelSwitch、Elicitation、
ElicitationResult、SessionEnd。

声明和解析这些事件不代表产品已实现所有事件对应的能力；事件必须由实际发生该动作的
核心模块触发。不存在的团队、文件监听或 WebView 生命周期不能靠构造输入冒充真实运行。
Hook 单元测试、真实 Go 核心测试、原生桌面测试和真实供应商测试须分别记录。

当前核心在真实工具批次完成后触发一次 PostToolBatch，其 `tool_calls` 保存模型
实际收到的字符串或内容块；PostToolUse 则保存工具的结构化结果。模型 API 失败
触发 StopFailure，`error` 为公开错误类型，细节在 `error_details`。
MessageDisplay 采用公开的非交互模式：每个有文本的完整消息一次，
`index=0`、`final=true`、`delta` 为全文；结果只更新最终展示，不改模型历史。
尚不把它宣称为原生界面的流式逐行批次对齐。

自动和手动压缩都围绕真实摘要触发 PreCompact/PostCompact，并在成功后触发
`SessionStart` 的 `source=compact`。子 Agent 实际创建后、首次模型请求之前，
由父运行触发 SubagentStart 并注入上下文；后台委派等待该启动边界后才释放父规则。
子 Agent 在自己的停止边界触发 SubagentStop，反馈可让该子 Agent 继续；
公开 `session_id`/`transcript_path` 指向父会话，`agent_id`/
`agent_transcript_path` 指向子会话。

`async` 结果不能追溯阻止已经完成的操作；`additionalContext`/`systemMessage`
通过注入回调交给宿主在后续边界投递。`asyncRewake` 退出码 2 通过单独回调唤醒
会话。宿主需要持有 runner、在关闭时取消上下文并 `Wait()`。

`AddRules` 增加会话规则，`PushRules` 返回幂等清理函数供技能作用域使用。
`once` 只对来源为 `skill` 的规则生效；失败或阻止后的尝试不消耗 once，
成功后在该 runner 生命周期内不再执行。宿主要跨多次 run 维持同一 session 的
once 语义时，必须维持该生命周期或用 `WithHookOnceState` 注入共享状态，
不应把每次 run 当成新 session。`HookInput.EnvFile` 允许宿主仅为 SessionStart
等公开允许的事件注入 `CLAUDE_ENV_FILE`（另有 Setup、CwdChanged、FileChanged）；
通过 PowerShell 执行的 Hook 不注入此变量。文件内容导入后续工具环境属于宿主职责，
单独设置路径并不能证明环境已经生效。

外部 Hook 的 JSONL 会话投影绑定 `Session.StorageRootID`，
写入该 root 的 `sessions/<session>/hook-transcript.jsonl`。
配置目录或当前活动数据 root 的变化不会把它重定向到其他会话目录；
所属 root 无法解析时明确失败。插件敏感配置、供应商凭据和允许展开的 HTTP
环境凭据在日志、事件和模型附加上下文边界统一脱敏。

`updatedPermissions`、文件监听列表、技能重扫、初始自动用户消息、
elicitation 动作与终端控制序列尚未全部获得宿主动作实现；
执行记录明确标出未消费字段，不因能解析这些字段就算成功。

本次替换了旧文档中七事件、`process`、Windows 默认 PowerShell、
30 秒统一默认超时、camelCase stdin、顶层 allow/deny、任意 stdout 注入上下文、
所有工具必发 PermissionRequest 和执行错误中止运行等描述。这些旧约定不再保留。
