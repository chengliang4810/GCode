# 插件逐项运行验收

`go run ./scripts/plugin-acceptance` 创建逐插件 JSON / CSV 账本。默认范围固定为
`anthropics/claude-plugins-official` 的 315 条市场记录：

- 市场 commit：`f8c5e7a1dcb71b10cb74586224b5401f3de5db28`
- 冻结清单 SHA-256：`1150acdc77c88c087e09fbec563eb6b052e4ee2044082ab9dc60698d5fb023e5`
- 90% 门槛：至少 **284 / 315** 条完整运行验收通过。

默认只生成账本，不联网、不执行插件、不读取用户的 `.gcode` 账户或配置。调查中的
静态 source-evidence 只记为 `snapshot_only`，安装、发现和运行均保持 `not_tested`。
`needs_auth`、`unavailable`、`blocked`、失败及未测试条目都留在 315 的分母中。
默认清单文件被修改、缺行或重复时，不能继续声称是这个冻结基线。

每次运行创建独立 `output/plugin-acceptance/run-*` 目录，保存 `ledger.json`、
`ledger.csv`、来源导出、组件清单、隔离安装、运行工作目录及日志。失败保留证据。
同名插件的其他版本、模型或平台证据不会增加本次分子；重复提交也不会重复计数。

## 验证来源、安装和组件发现

离线缓存是包含对应固定 commit 对象的 Git 仓库。驱动按 commit 导出插件源码，
忽略 checkout 中未提交的改动，不写源仓库，不执行仓库中的脚本，也禁止 partial clone
隐式联网补对象。每个仓库与提交在本次运行中只导出一次完整源码树，插件切分在
保留市场内共享文件上下文后进行。

```powershell
go run ./scripts/plugin-acceptance `
  --source-root D:\read-only-cache\claude-plugins-official `
  --cache D:\read-only-cache\plugins\index.json `
  --install-and-discover `
  --workers 4 `
  --timeout 2m
```

`--source-root` 提供市场仓库，第三方插件的独立仓库通过缓存索引声明：

```json
{
  "sources": [
    {
      "repository": "Airtable/skills",
      "commit": "812ee67f1fd3d76fb45ff8df40afaa0448602ba8",
      "root": "D:/read-only-cache/airtable-skills"
    }
  ]
}
```

`root` 可以相对于 `index.json` 所在目录。缓存只有工作目录副本、没有固定 Git 对象，
或插件来源尚未冻结到完整 commit 时，记录 `blocked`，不会擅自访问网络补齐。
索引已经包含市场仓库时，无须再传 `--source-root`；同一仓库与提交指向不同根目录
会被拒绝。历史调查的 manifest 文本摘要只统一 CRLF/LF，Git 提交及安装目录摘要仍
绑定原始字节。导出接受 Git 的提交 comment PAX 元数据，不把元数据落盘。
普通文件落盘，符号链接以类型化 `SourceTree.Links` 表保留并写入同级
`*.links.json` 证据，不在源码导出缓存创建系统链接。无关仓库链接不消耗系统权限，
归档内容也不能通过链接重定向写入。最终插件物化保留内部相对链接、复制同市场共享
目标、跳过市场外目标；硬链接和非普通文件仍被拒绝。Windows 没有创建插件实际需要
的内部符号链接权限时明确阻塞，不用普通文件冒充链接验证。

来源通过校验后，驱动创建本次专用的目录适配市场，使用 `plugins.Manager.Install`
安装，再通过真实 `plugins.LoadInstallation` 发现组件。每个候选使用独立安装
registry，其他待测插件的 namespace 和启用状态不会污染该候选。这两个阶段分别记录
`native-plugin-manager` 和 `native-bundle-loader`。目录适配只把已经验证的源码导出
接到安装器，不改变冻结原始 entry 的指纹和原始来源身份。

发现范围包含每个 skill、command、agent、Hook handler、MCP server、LSP server、
output style、settings 项、依赖、用户配置项和 channel。每项都有稳定 ID 和定义
摘要，整个安装目录另有内容摘要。无法支持的执行组件保留阻塞原因。

来源、安装和发现通过时，**运行仍然是 `not_tested`**。例如一个含 skill 与 MCP 的
插件，只有 skill 实测通过，也不能把整个插件记为通过。

## 运行证据接口

可导入包位于 `gcode/scripts/plugin-acceptance/acceptance`。
`acceptance.RuntimeRunner` 接收：

- 插件 ID、市场/source commit、安装目录摘要、平台和模型。
- 已加载的类型化 `plugins.Bundle` 和完整待测组件。
- 本次独立的 workspace / evidence 目录与明确传入的私有测试账户。

命令已链接 `corerunner.InProcessRunner`，通过 `engine.Open` 和类型化
`contracts.Service` 调用真实 Go 业务核心。每项 probe 使用自己的 Store、Home、
CWD 和精确候选插件安装。模型请求使用明确指定的 Provider，不读取用户实际
`.gcode`，也不继承个人 API / 云凭据环境变量。运行期间串行切换独立 CLI 进程的
最小环境，完成后恢复；该方式不改变生产业务核心。
来源验证由 `--workers` 控制并发；实际模型 probe 顺序执行，每个插件真正开始时
才启动它的超时，避免等待环境租约消耗运行时限。
每个 probe 完成或失败后，先关闭核心和 Store，再通过受约束目录句柄移除准确的
临时 Home，保留 CWD 与脱敏证据；清理失败会使该 probe 失败。修复前旧运行的副本
受自动安全审查阻止清理，准确范围见
`docs/investigations/claude-plugins-2026-10-30/private-artifact-cleanup.md`，不能分享那些
完整 run 目录。

Runner 在真实调用边界收集事件和最终状态。skill/command 必须观察到精确
`skill.invoked`；agent 必须有对应类型的已完成子会话；Hook 必须匹配该 handler 的
完整 rule ID 和真实退出码；每个 MCP server 必须完成计划指定 tool 的实际调用。
模型能力还必须有真实 HTTP 模型响应，不能只由模型声称通过。
`model-http.json` 保留请求与响应摘要、最多 2 MiB 的脱敏原始响应及 SSE frame
接收时间、类型、结束原因和 token usage，用于区分网关响应与模型协议解析问题。
不记录认证请求头。每个 CWD 初始化独立 Git 仓库，避免 Git 向上识别实际项目。
这些隔离措施不是操作系统沙箱。
本地 `httptest` 的确定性模型服务仅验证这条真实核心调用路径，不是公开插件或实际
云模型运行证据。

执行实际账号验收时，必须显式传入：

```powershell
go run ./scripts/plugin-acceptance `
  --install-and-discover `
  --source-root D:\read-only-cache\claude-plugins-official `
  --cache D:\read-only-cache\plugins\index.json `
  --run `
  --provider-config D:\private-tests\provider.json `
  --probe-dir D:\private-tests\plugin-probes `
  --model test-provider/my-configured-model
```

私有测试文件使用业务核心原生 `model.Provider`，另设模型选择和推理档位：

```json
{
  "provider": {
    "id": "test-provider",
    "name": "Private acceptance account",
    "protocol": "openai-chat",
    "baseUrl": "https://your-configured-endpoint.example/v1",
    "apiKey": "private-test-key",
    "models": [{ "id": "my-configured-model" }]
  },
  "modelId": "test-provider/my-configured-model",
  "reasoning": "",
  "pluginConfigs": {}
}
```

协议采用项目当前的 `openai-chat`、`openai-responses` 或 `anthropic-messages`。
`pluginConfigs` 按完整插件 ID 提供明确测试配置，值为字符串。密钥和这些值不写账本，
实际事件及状态日志也会对明确传入的私有值脱敏。没有该文件参数时，不查找用户账户。

## 每插件业务 probe 计划

`--probe-dir` 内的文件名为 `<plugin>@<marketplace>.json`。计划必须绑定完整 source
commit，完整覆盖发现清单中的每个组件，且每项都有明确业务断言。示例：

```json
{
  "pluginId": "demo@fixture",
  "sourceCommit": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  "allowHooks": [],
  "probes": [
    {
      "id": "write-fixture-answer",
      "componentId": "skill:demo:run",
      "prompt": "/demo:run",
      "fixtures": [{ "path": "question.txt", "text": "Compute 6 * 7." }],
      "assertions": [
        { "kind": "file", "path": "result.txt", "equals": "42" }
      ]
    }
  ]
}
```

支持 `file`、`file-exists`、`file-absent`、`text`、`exit` 断言。`file`/`text` 必须给出
`equals`、`contains` 或 `regex`；文本来源可为 `assistant` 或指定 `toolName` 的 `tool`。
退出码必须是观察到的进程结果，缺少退出码不会被猜测为 0。所有 fixture 和文件断言
都限制在该项 probe 的私有 CWD，不接受绝对路径、逃逸或符号链接。

Runner 把测试所有者的策略接到核心 `ToolGuard`，在根、子 Agent 及动态命令的实际
工具执行前检查参数。文件工具只可写私有 CWD，插件来源只能读取；网络工具默认拒绝。
`allowCommands` 逐条列出允许的完整 Bash 命令，其他命令一律拒绝。与 fixture 原文
完全相等的文件断言表明该 fixture 不可修改，执行允许的校验命令前再次核实其字节。
待修改业务文件仍可修改。`verifierOutputs` 列出的报告禁止模型读写，只能由允许的
校验进程生成。上述策略不限制操作系统能力；直接 Hook 命令及 MCP 服务仍需要明确
审阅与对应测试账号授权。

MCP probe 还必须给出 `mcpTool`，它是该 server 的原始 tool 名称。Runner 根据真实
插件与 server 名称生成并核对完整工具命名空间，不会把另一个 server 的调用算作
该组件通过。Hook 的组件 ID 必须逐项列入 `allowHooks`；缺少授权或不完整计划时，
在发送任何模型任务、触发任何插件 Hook 之前阻塞整个插件。

当前 LSP、output styles、settings、依赖能力、配置值和 channels 尚未接入该 Runner，
这些组件保持 `blocked`。没有计划、缺账号、需人工许可、平台不符或断言失败也不会
从分母中移除。`--run` 未达到目标时以非零退出。私有工作目录不是操作系统级沙箱，
计划必须针对允许实际访问的测试账号及资源。

## 导入外部运行证据

```powershell
go run ./scripts/plugin-acceptance `
  --install-and-discover `
  --source-root D:\read-only-cache\claude-plugins-official `
  --cache D:\read-only-cache\plugins\index.json `
  --model test-provider/my-configured-model `
  --platform windows/amd64 `
  --runtime-evidence D:\test-results\plugin-runtime.json
```

证据采用 `acceptance.RuntimeEvidence`：schema version、run ID、插件 ID、两项 commit、
安装内容摘要、平台、模型和每项组件的 `Probe`。没有整插件 `passed` 输入字段。
一个 passing probe 必须同时满足：

1. 组件 ID / 组件摘要与本次实际发现完全一致，且全部组件均有结果。
2. 操作类型正确，例如 skill 是 `skill-invoke`，MCP 是 `mcp-tool-call`。
3. 指向有 SHA-256 的普通 JSONL 日志文件，文件内容与摘要一致。
4. 日志包含一条实际 `invoke` 和随后一条 `result`，两者完整绑定插件、source commit、
   市场 commit、安装摘要、组件摘要、模型、平台和 probe ID。
5. 日志时间在 probe 时间段内，没有执行错误，并有实际结构化输入与输出。

模型能力需要 prompt / messages、response / messages / artifact 和模型响应身份；
MCP 需要真实 `tools/call` 请求与结果；LSP 需要 `textDocument/*` 请求及结果；
Hook 需要事件输入及 stdout / response / result、exit code 或 transport。
仅 `{ "passed": true }`、清单解析事件或缺少组件的结果会被拒绝。

验证器保证证据结构、内容和身份绑定完整；导入者仍须提供真实执行边界生成的日志。
该机制不能单靠外部 JSON 证明未知执行器的真实性。

## 可重复验证

```powershell
go test ./scripts/plugin-acceptance/...
go test -tags integration ./scripts/plugin-acceptance/...
go vet ./scripts/plugin-acceptance/...
```

普通测试覆盖固定 315 分母 / 284 门槛、未测和鉴权缺口不剔除、重复版本 / 平台 / 模型、
证据摘要和组件缺失拒绝、失败账本保留及恶意归档路径。integration 测试只使用本地
临时 Git 仓库验证来源、原生安装和发现，并断言这些结果不能升级为运行通过。
Runner 的 integration 测试另使用本地 `httptest` 模型与 MCP 服务，经真实核心执行
技能、文件工具和 MCP 调用，并断言“模型声称写文件但没有文件”必须失败。
