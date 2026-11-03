# 原生 Go 后端

2026-10-29 起，普通桌面启动接入真实 Go 核心。按用户确认，借鉴 Pi Agent 的模型循环、
工具结果、steering 与 follow-up 设计，用 Go 实现；没有嵌入官方 TypeScript Pi，
也没有 Pi RPC、Node 或 JavaScript 运行时。本机 UI 与核心同进程，直接调用强类型 Go
接口。历史 UI 视觉一致性优化仍暂停。
插件脚本或外部 MCP 可能另行要求 Node、Python 等宿主依赖，这不改变本机核心的 Go 调用边界。

## 启动与配置

```powershell
go run ./cmd/rcode
go run ./cmd/gcode-server
```

打包安装的桌面程序在一台电脑上只运行一个实例：再次启动会把命令行交给正在运行的
实例、聚焦其窗口后自己退出。开发构建（`go run`、普通 `go build`）不带打包元数据，
默认可以多开；确定性场景（`--scenario`）与无窗口渲染（`--headless`）不参与单实例。
`RCODE_ENFORCE_SINGLE_INSTANCE=1` 让开发构建也单实例；
`RCODE_ALLOW_MULTIPLE_INSTANCE=1` 让打包版允许再开一个。
单实例锁按用户区分（socket 位于用户临时目录），不同 Windows 账户互不影响。

桌面默认简体中文，语言设置可选择英文。设置 → 模型设置中新增自定义供应商，填写名称、
协议、Base URL、API Key 与模型 ID。Base URL 填服务地址，例如 `https://host`，
核心根据协议拼接 `/v1/chat/completions`、`/v1/responses` 或 `/v1/messages`；
填写末尾带 `/v1` 的地址时不重复添加。自定义网关路径前缀保留，
例如 `https://host/gateway` 与 `https://host/gateway/v1` 均请求 `/gateway/v1/...`。
模型列表使用同一规则请求 `/v1/models`，保存的 Base URL 保留用户输入。
不内置供应商品牌、账号登录或官方模型目录；也不自动猜测自定义模型的能力。

仅支持三种协议：

| 配置值 | 协议 |
| --- | --- |
| `openai-chat` | OpenAI Chat Completions |
| `openai-responses` | OpenAI Responses |
| `anthropic-messages` | Anthropic Messages |

每个模型直接以请求模型 ID 显示，不再保存独立显示名；配置上下文容量、最大输出、`inputTypes` 和 `reasoningLevels`。
新增模型默认上下文 200000、最大输出 12800；智能配置从宿主本地 models.dev 缓存填充草稿。
容量为 0 表示未知，不伪造上下文百分比，也不自动触发容量压缩。
思考档位为空表示没有可选项；非空时，新选择采用配置数组最后一项。
主会话和子 Agent 共用这份目录，显式失效档位会报错，不静默改用其他档位。
原 `reasoning` 布尔字段已移除，没有旧字段读取或数据迁移。
API 中还可分别设置每百万 token 的输入/输出单价，单价未配置时不推算价格。
公开快照不含 API Key；配置编辑留空保留现有 key。
供应商配置可先保存为空 Base URL、零模型的草稿，也可删除最后一个模型。
只有连接地址有效且有启用模型的供应商才进入可执行目录；调用边界再次校验，
待配置项不会自动回退到其他供应商。API Key 按服务需要填写，允许无鉴权本地服务。

模型调用已按“Agent → 统一模型接口与 Gateway → 三种独立协议适配器”组织。
Engine 启动任务时绑定用户配置，Agent 不解析协议或持有渠道凭据；
协议私有续接数据由模型层封装、持久化并校验目标，切换模型不再由 Engine 清除协议字段。
Peri 只作为分层设计参考，本实现由本仓库原有 Go 代码独立重构，没有复制 Peri 代码或增加依赖。
详细边界、重试与验证范围见 [模型适配说明](model-adapters.md)。

目标执行沿用普通 Agent 与模型适配器，通过 `/goal 目标` 启动，
自动在执行与完成检查之间续跑。支持暂停/恢复、子任务等待和可选 Token 预算，
详见 [目标执行](goals.md)。

## 模型适配边界

执行链为 `Agent → agent.Model → model.Gateway → 三个协议适配器`。
Engine 根据用户渠道与模型配置调用 `model.Client.Bind`，为本次任务绑定配置快照。
Agent 只接收统一的 `Request`，其中包含系统提示词、历史、工具定义和推理等级；
协议选择、Base URL、凭据和模型标识不进入 Agent 请求。Agent 的模型接口只有 `Stream`。
上下文容量作为执行参数传入，手动与自动压缩复用同一个已绑定接口。

`internal/model/chat.go`、`responses.go` 和 `messages.go` 分别负责三种协议的请求、
鉴权与事件解码；`client.go` 共用 HTTP、超时、取消和 HTTP 状态重试，
`stream.go` 共用 SSE framing、完整工具参数校验与统一停止原因。
遇到未知协议、非法 URL 或未配置的模型直接报错，不选择默认供应商或其他协议。
模型能力沿用用户配置，不根据模型名称猜测。

协议续接数据位于 `Message.Continuation`，只有适配层读取内部内容。它随执行历史
持久化，并在 worker/fork 克隆时复制底层数据。Responses 原始输出、Anthropic 独立
推理块及签名仅在渠道 ID、协议、地址、凭据与模型 ID 全部匹配时回放；切换目标时使用
统一的正文、图片和工具调用/结果，不把另一目标的签名或加密推理发送过去。
Engine 不再解释或清除协议字段。该内部结构直接更新，没有旧字段解析或数据迁移。

本次仅参考 Peri 的分层思路，代码由本项目独立以 Go 实现；没有复制 Peri/Claude Code
源码，也没有引入它们的依赖。三协议确定性测试与真实服务商验收仍分别记录。

## 配置入口与数据根

`Store.ConfigDirectory()` 是固定配置入口，默认 `~/.gcode`；显式 `--data-dir`
选择独立入口，重启仍需同一参数。`Store.Directory()` 是当前活动数据根，首次初始化时
可以与配置入口重合，之后不能把两者混用。项目源文件始终留在用户选择的实际目录。

设置 → 通用的目录选择登记或切换数据根，重启启用；它不复制或删除旧根内容，
也不把现有会话搬到新目录。每个会话保存 StorageRootID，历史从创建时绑定的根读取，
续聊产生的消息、运行记录及新附件也继续写入这个根。因此旧会话在 C 盘时，
切换目录不会释放其已有空间，也不能阻止继续使用该旧会话带来的增长。
旧盘离线会标明所属会话不可用，不能伪造空历史或自动回退到其他盘。
新目录登记要求符合根身份和路径检查；已登记根可再次选择，不要求把其已有数据清空。

配置入口的 location.json 保存根登记、当前根及待启用根；每个根有身份标记。
配置入口与已解析数据根持有各自锁。新创建数据根不得与配置入口或其他根相互包含，
不会覆盖用户已有身份不同的数据。缺失或身份变化的历史根不会被自动重建，
也不会因为打开历史对话而复制到当前根。

| 所属位置 | 路径 | 内容 |
| --- | --- | --- |
| 配置入口 | engine.json | 设置、供应商与小型会话索引，不保存全部会话正文 |
| 会话绑定根 | sessions/会话ID/内容摘要.json | 消息、执行历史、模型上下文、目标、运行、队列、会话改动及终端历史；索引提交后清理旧正文版本 |
| 配置入口 | connection.json、server.json | 远程配置及宿主认证配置 |
| 配置入口 | user-resources.json、skills/ | 用户技能、命令和私有 MCP 配置 |
| 配置入口 | plugins/registry.json | 市场来源、缓存目录和安装记录 |
| 会话绑定根；无会话操作用当前根 | plugins/data/、plugins/sessions/ | 插件持久数据及受管会话环境文件 |
| 配置入口 | plugin-configuration.json | 宿主私有插件配置 |
| 附件绑定根 | uploads/ | 托管附件，通过 root ID 解析 |
| 会话绑定根 | workspaces/会话短ID/ | 普通无项目对话独占的工作目录 |
| 工作树创建时的数据根 | worktrees/工作树短ID/ | 新创建的真实 Git 工作树 |
| 当前数据根 | cache/gcode/models-dev.json、cache/gcode/models-dev-state.json | 模型目录缓存及刷新状态 |
| 配置入口 | native/ | 原生窗口状态及偏好 |
| 当前数据根 | logs/、cache/gcode/、tmp/gcode/ | 应用日志、框架缓存和临时文件 |
| 当前数据根 | browser/ | Windows WebView2 Cookie、登录状态及缓存 |

会话与工作树目录使用 12 位随机小写字母数字短 ID，创建目录使用排他预留，
不会随显示标题变化。普通无项目新对话不再共享 `workspaces/` 本身；
侧聊、fork 和子 Agent 继承父会话 CWD，保留共同访问工作文件的协作语义。
新建会话复用已有工作树时，其记录使用当前根，工作树本身保留实际路径。
自定义工作树目录选项及其字段已删除；归档或删除会话不会删除工作树或共享工作目录。
草稿附件在首次导入时绑定当时的数据根，首次发送沿用附件所属根，
不能因为后来切换活动目录而把已导入文件指向另一位置。

全局日志、缓存、临时文件和浏览器资料在重启后使用新的活动根，不搬走旧文件。
日志轮转及清理只处理应用拥有的日志和缓存/临时命名空间，不触及会话、
附件、工作区、工作树、插件持久数据或浏览器登录资料。配置入口中的用户技能和
插件安装不属于运行缓存，仍可能占用配置入口所在磁盘；
外部命令和 MCP 进程自行选择的输出位置也不受此设置统一管理。

完整会话状态先写到其所属根，再保存配置索引，不能通过切换活动根把同 ID 指向新建空目录。
JSON 仍使用临时文件与原子替换；Windows 有界重试不会先删除原文件。
凭据目前保存在宿主本地文件，不是系统凭据库加密存储。
开发阶段不做旧配置格式的兼容解析、双写或隐式历史迁移。

## Agent 与会话

下述公开契约于 2026-10-30 按 Claude 插件接入更新；后文带日期的旧验证记录保留当时名称与行为。

- 流式文本和推理按类型化增量事件更新原生界面，完整模型返回与工具结果用于后续调用。
- 工具包括 `Read`、`Write`、`Edit`、`Bash`、`Glob`、`Grep`、`AskUserQuestion`、
  `Skill`、`Agent`、`TaskOutput`、`TaskStop`；目录列表扩展名为 `ListDirectory`。
  Read/Write/Edit 接收 `file_path`，Edit 使用 `old_string/new_string/replace_all`，
  不保留旧小写名与旧字段别名。Read 支持 UTF-8 文本和 PNG/JPEG/GIF/WebP/BMP 图片，
  图像处理与三协议附件边界见 [Read 工具](read-tool.md)。
- `Bash` 在 Windows 明确使用 Git Bash；Unix 使用 bash。Windows 可通过
  `RCODE_GIT_BASH_PATH` 指定可执行文件，不把 Bash 脚本交给 PowerShell。
  超时单位为毫秒，默认 120000，最大 600000；不加载用户 profile 或 BASH_ENV。
  SessionStart Hook 收到受管 CLAUDE_ENV_FILE 后可向其写入 Bash 环境，后续命令显式加载
  该会话文件；不会使用用户任意 BASH_ENV。PowerShell 不加载这份 Bash 环境文件。
  动态上下文和 Hook 可明确选择 PowerShell，原生交互式终端的 shell 配置独立。
- `Bash.run_in_background` 启动宿主会话拥有的任务；模型回合结束或远程客户端断开
  不终止已接受的后台命令。`TaskOutput` 按 ID 读取并可限时等待，`TaskStop` 只停止
  本会话拥有的命令或子 Agent；宿主关闭清理自有进程。任务不是 OS 沙箱，
  仍使用宿主用户权限。`dangerouslyDisableSandbox=true` 目前明确报不支持。
- `Glob` 和 `Grep` 在进程内遍历目录并匹配，不要求宿主安装 `rg`。Glob 依修改时间降序，
  Grep 使用 Go 正则及 `-i/-n/-A/-B/-C/context`，省略 path 时使用工作目录。
  ignore、分页与预算见 [搜索工具](agent-search-tools.md)。
- Edit 默认要求唯一匹配，`replace_all=true` 替换全部；保留原文件换行。
  Write/Edit 产生真实前后差异；工具的结构化 Data 用于 Hook tool_response。
- 只读工具可以并发，文件写入、命令、问题和 MCP 调用作为执行屏障。
  取消前台 shell 会终止该次自有进程树。
- 运行中发送的消息进入队列；普通消息在当前回答后继续，“作为引导发送”在模型步骤
  边界注入。删除队列项后不再执行。AskUserQuestion 接收 questions 数组、对象选项与
  multiSelect，经原生问题区逐问等待，回传 questions 和按问题文本索引的 answers。
- 主会话、空侧聊、复制历史的 fork 共用核心，各自拥有运行与取消状态。
- 内置子 Agent 为 general-purpose、Explore、Plan、claude。Agent 按类型启动真实子会话，
  前台等待，后台通知；Explore/Plan 为只读调查/规划类型。`sonnet/opus/haiku/inherit`
  统一映射到当前执行上下文模型，用户为子 Agent 类型配置的模型和思考档位优先，
  不是固定调用 Anthropic 模型。详见 [子 Agent](subagents.md)。
- `/compact` 手动压缩模型上下文；已配置容量时约 75% 的估算阈值可自动触发。
  完整执行历史保留，估算不当作供应商实际 token 数。压缩运行中暂不接受新消息。
- 重启后未完成运行标为中断，对没有记录结果的工具调用追加明确的中断结果；
  不自动重放可能已经发生的写入或命令。用户可以检查现状后重试。

自动读取用户根与项目根的 `AGENTS.md`。用户导入的技能与命令仍来自托管资源目录；
运行 worker 另加载当前项目 `.claude/skills`、`.claude/commands`、`.claude/agents` 和
已启用插件资源。插件技能/命令用 `/插件名:资源名` 或 `Skill` 的精确名称调用，
用户 `/skill:ID` 与现有用户命令继续按各自资源身份解析。
原生引用候选与运行目录的范围不同，见 [输入框引用](prompt-references.md)。
六个原生设置页的维护约定见 [用户级 Agent 设置](user-settings.md)。

## 插件与市场

默认登记官方 `anthropics/claude-plugins-official` 与社区
`anthropics/claude-plugins-community`，登记源不会自动安装其中全部插件。
市场清单、插件 manifest 和 Markdown 组件由 internal/plugins 解析；宿主后台处理
刷新、安装、更新、启停、卸载及配置，UI 与本机核心仍通过类型化 Go 接口交互。
插件仅支持用户级安装，同一宿主所有项目共用安装、启停和配置；
运行使用固定安装目录和配置快照，更新不会覆盖活动执行读取的旧安装版本。

运行层接入 skills、commands、agents、hooks 与 MCP。项目资源按名称覆盖插件资源，
应用用户技能/命令和显式子 Agent 配置再按名称或 ID 覆盖；命名空间降低插件间冲突。
命令支持 `$ARGUMENTS`、从 `$0` 开始的位置参数、`$ARGUMENTS[n]` 和声明式参数名；
动态命令在 worker 经过输入准入后执行，不在 UI 线程运行。
支持插件路径/数据目录/技能目录/会话变量及配置替换、context=fork 和技能附带 Hook。
技能 `allowed-tools` 已解析，但权限自动授予等完整 Claude 语义仍须专项验收；
其他 frontmatter 声明也不能仅凭被解析就计为执行兼容通过。

插件运行的 shell、网络、依赖、凭据和用户配置都来自实际宿主。LSP 运行层尚未连接，
output-style 选择尚需宿主支持，headersHelper 未执行；管理投影显示相应诊断。
Hook 的事件/JSON 解释、命令/HTTP/模型/MCP 执行分层已有代码，不能据此声称全部高级
Hook、异步通知和真实外部插件已完成组合验收。OAuth 浏览器授权 UI 也尚未接通。

官方市场冻结版本是 90% 可运行目标的唯一分母，社区不计入；
当前市场安装、确定性夹具或 manifest 扫描都不能替代逐插件真实运行结果，
未填写凭据、缺少依赖和未验证项不计为通过。

项目文件树读元数据、打开文件时才读正文；编辑器保存检查磁盘内容版本，防止覆盖外部
修改。搜索有数量/文件大小上限。用户附件按 MIME 和实际注册的 Go 图像解码器校验；
当前可识别 PNG、JPEG、GIF、WebP、BMP，并支持 UTF-8 文本。图片传给模型，
文本作为附件内容加入请求。不宣称支持历史 mock 展示的全部媒体格式。
上传入口保留原始图片，不执行 `Read` 的方向归一化、BMP 转 PNG、缩放和压缩；
能够导入也不代表目标模型原生接受该格式。Agent 主动读图的处理预算见上述 Read 说明。

## MCP

默认 handshake 提供 `2025-11-25`，接受客户端列明的其他 handshake 修订。
底层支持 stdio、Streamable HTTP 和 HTTP+SSE；显式 modern/auto 连接能力由 MCP 包提供，
应用普通配置不把协议日期或服务商品牌猜测为新协议。细节见 `internal/mcp/README.md`。
用户级配置保存在 user-resources.json，插件 MCP 来自实际启用 bundle；
没有把任意项目 .mcp.json 自动当成用户全局配置。

工具公开名是 `mcp__服务器__工具`，插件服务器带 plugin_插件名_服务器命名空间。
无效字符正规化，冲突或超过当前模型侧 64 字符限制会报错，不截断或增加旧名别名。
文字和校验后的图像进入三协议结果；结构化结果保留原始内容块与扩展。
stdio MCP 是真正外部工具边界，不是 UI 与本机 Go 核心的桥接。

包级 API 提供 resources、resource templates、prompts 和通知流，
不代表原生 UI 已有完整资源浏览/订阅入口。OAuth 底层已有发现、PKCE、回调校验、
令牌交换/刷新和显式客户端注册 API；宿主仍需接浏览器、回调监听及受保护的凭据保存，
当前没有完成插件 OAuth 登录 UI 或外部账号验收。headersHelper 不执行。
未声明 sampling、roots、elicitation 等客户端能力，不返回伪造成功。

用户配置连接失败会如实报错；某个插件 MCP 连接失败会记录插件诊断并不注册该工具，
不能将其余会话仍可运行算作这个插件通过。凭据和注入 token 在边界脱敏。

## Linux 无桌面与远程连接

```sh
go build -o gcode-server ./cmd/gcode-server
./gcode-server --show-token
./gcode-server --listen 127.0.0.1:27655
```

`--show-token` 显式输出该配置入口 `server.json` 中的认证 token；
切换运行数据根不会更换 token。服务运行时不同时打开同一配置入口。
默认监听回环地址。跨设备可用安全隧道/反向代理，或直接指定
`--listen 0.0.0.0:27655 --tls-cert cert.pem --tls-key key.pem`。
不要通过未加密公网 HTTP 传递宿主认证 token。

桌面设置 → 设备填写地址与 token，保存后重启桌面生效。或：

```powershell
go run ./cmd/rcode --connect https://host:27655 --token-file C:\private\host-token.txt
go run ./cmd/rcode --local
```

连接配置属于客户端自己的固定配置入口。模型凭据、项目、工具进程与运行状态属于远程宿主。
关闭客户端、取消事件订阅或网络断开不会取消宿主已接受的任务。
重连先取完整快照，再继续事件流；未实现持久化事件日志回放。
远程修改数据目录指向宿主，重启宿主后生效；原生“选择目录”按钮在远程模式禁用。

| API | 用途 |
| --- | --- |
| `GET /v1/snapshot` | 完整状态 |
| `POST /v1/commands` | 强类型命令的远程 JSON 表示 |
| `GET /v1/events` | SSE 快照及增量事件 |
| `POST /v1/directories` | 宿主目录选择数据 |
| `POST /v1/directory-locations` | 宿主的可选位置（Home 与磁盘卷） |
| `POST /v1/search` | 宿主项目搜索 |
| `POST /v1/attachments` | 上传附件字节，不接受任意宿主路径 |
| `GET /v1/resources?ref=...` | 读取托管附件 |
| `GET /v1/terminals/{id}/stream` | 终端原始输出字节流，断开只分离视图 |
| `POST /v1/terminals/{id}/input` | 终端原始输入字节，不创建临时视图 |

全部 API 要求 `Authorization: Bearer <token>`。移动客户端可以复用这套接口，
当前仓库没有新增 Android/iOS 客户端，也没有完成手机实机验收。
桌面 `--headless` 仍只是 MyGo 渲染测试，不能替代 `gcode-server`。

## 验证与范围

2026-10-30 已迁移核心工具和既有三协议/Hook/子 Agent fixture；工具专项、agent 与 remote
测试已有通过记录。插件组合、完整桌面、真实外部服务和官方市场逐插件可用率仍须分别验收。
以下旧检查与日期记录是对应版本的证据，不是本轮所有变更已通过的声明；搜索工具已在
2026-11-01 改为进程内实现，运行期不再新增 ripgrep 依赖。

Windows 自动测试覆盖三协议流/工具往返、真实文件写入与恢复、取消、排队删除、
并发只读工具屏障、压缩、MCP stdio/HTTP、远程断线继续执行、认证、key 脱敏、
存储切换/冲突与原生设置控件。

Linux amd64 测试二进制已在 WSL Debian 实际运行，移除 DISPLAY、
WAYLAND_DISPLAY、DBUS_SESSION_BUS_ADDRESS，Agent、storage、model、engine、
MCP、remote 六包通过。独立服务 CLI 也已验证默认 HOME/.gcode、认证 HTTP/SSE、
SIGTERM 正常关闭和重启后配置/会话保留。Linux amd64/arm64 交叉编译通过，
服务依赖图无 MyGo、desktop、workbench 或 design。arm64 尚未实机运行。

公网/TLS 证书、macOS、手机均未作为已验证环境。基础回归使用本地协议夹具；
真实模型验证单独记录在下方。会话命名模型、Git refs 与 worktree 已接入真实核心，
见 [会话命名与工作树](thread-naming-worktrees.md)。PR 与项目 actions 入口、契约及 mock
实现已移除。云账号/计费、主题导入等原 UI 原型能力尚未接入真实核心；不返回模拟成功。
不是 Pi 功能全集、Durable 工作流或整套 UI 视觉验收。

本轮证据保存在 `output/backend-v1-*` 与 `output/backend-v1/`。

最终检查：`go test ./...`、`go vet ./...`、`go tool mygo vet .`、
`golangci-lint`、MyGo 源码 pin、`git diff --check` 均通过。没有新增第三方运行依赖。
当前环境没有运行 Go race 检查所需的 C 编译器，未将 `-race` 列为通过项。

Windows 原生窗口实际验证了中文默认界面、自定义模型、工具写入与活动摘要、目录提交后
重启启用新根目录；磁盘上的实际文件与完整执行历史已核对。该路径使用本地 HTTP 模型
协议夹具，不能当作真实供应商调用或全部 Pi 功能验收。界面自动化被用户停止后未继续
操作窗口。最后的模型流解析、托管工作区路径调整与界面目录说明小改由自动测试验证，
未重新做原生截图。最后构建在 `output/backend-v1/Zeron-final.exe`；Linux 服务与测试日志
同目录和 `output/backend-v1-linux-*.txt` 中。

## 真实模型验证：2026-10-29

已使用用户指定的本地模型服务实际调用，未用 mock 或协议夹具代替：

| 配置 | 实测值 |
| --- | --- |
| Base URL | `http://127.0.0.1:3425/v1` |
| 协议 | `openai-chat` |
| 请求模型 ID | `deepseek-v4.1-flash` |
| 模型目录返回的 ID | `group/deepseek-v4.1-flash`；上述短 ID 实测也可调用 |

直接流式请求返回 HTTP 200、`GCODE_LIVE_OK`，耗时约 5.9 秒。
生产 Go 核心的工具闭环约 12.6 秒：模型实际调用 `write` 创建临时目录中的
`gcode-live-proof.txt`，再调用 `read` 确认，文件内容为 `GCODE_REAL_MODEL_OK` 加换行，
最终回复 `GCODE_LIVE_OK`。服务累计报告输入 3,979 tokens、输出 113 tokens。
测试通过真实供应商配置与 `message.send` 进入核心，没有注入模型客户端或伪造工具结果。
测试使用隔离临时数据目录，不修改桌面日常供应商配置。

`internal/engine/live_integration_test.go` 提供显式启用的测试。
通过环境变量 `GCODE_LIVE_BASE_URL`、`GCODE_LIVE_API_KEY`、`GCODE_LIVE_MODEL` 配置，
然后运行：

```powershell
go test ./internal/engine -run '^TestLiveModelToolRoundTrip$' -count=1 -v -timeout 150s
```

不设置 Base URL 时跳过，不让普通单元测试意外消耗模型额度。
可选 `GCODE_LIVE_REPORT` 指定脱敏报告输出路径；报告不包含 API Key。
本次记录为 `output/backend-v1/live-deepseek-test.txt`、
`output/backend-v1/live-deepseek-report.json`。

## 2026-10-29 文件面板闪退修复

实际桌面日志记录 `panic: ui: Handle bound to two controls in one build`，
堆栈定位到 `FilesView` 的文件搜索框。工具区打开 Files 标签、同时显示右侧 Files
dock 时，两处视图共用了搜索输入框与目录树的 `ui.Handle`，导致重复绑定。

同一会话的文件与搜索结果继续共享；workspace 与 dock 分别拥有输入框/目录树句柄和
搜索列表滚动状态。文件操作弹窗只在发起操作的面板中绘制，避免重复弹窗。
没有通过移除任一文件面板或吞掉 panic 绕过问题。

桌面回归测试先复现相同 panic，修复后覆盖双面板同时显示、搜索、Escape 焦点、
键盘打开文件、反复隐藏/恢复、单份弹窗。全量测试、go vet、MyGo vet、lint 与
diff 检查通过。两个生产文件浏览器组件也已在真实 Windows 原生窗口同时绘制，
成功捕获画面并正常退出；该组件验证不等于整个应用的鼠标交互验收。

## 2026-10-29 真实交互式终端

生产核心已支持 `terminal.create/input/resize/clear/close`。终端使用会话的真实工作目录；
没有会话时使用项目目录或数据根下的 `workspaces`。Windows 使用 PowerShell 的 ConPTY，
Linux 使用系统交互式 shell 的 PTY。显示、VT 解析、输入法与键盘处理复用现有 MyGo
terminal 插件。插件所需 `ghostty-vt.dll` 已随本次桌面测试构建放在可执行文件旁，并核对
固定 SHA-256。

工作目录由 shell 提示符显示，界面不另设路径栏或查找、清屏按钮。

宿主通过 `contracts.TerminalService` 提供原始字节流，桌面直接传给插件的 `Options.Conn`，
不经过本机 HTTP、JSON、JavaScript 或子进程业务桥接。移除视图只分离流，关闭标签明确结束
该终端；菜单新建终端创建独立进程。服务退出会结束其终端，重启不会自动执行旧命令。

Linux 无桌面核心不引入 MyGo。由于 MyGo 的 PTY 包是 `internal`，将其固定版本的纯 Go
系统实现保留 MIT 授权后复用到 `internal/terminalhost/pty`，来源与少量调整见该目录 README；
没有修改框架源码或增加第三方模块。

每个进程最多保留最近 8 MiB 原始输出，视图有 4 MiB 待消费缓冲；消费者过慢会被分离，
可重新连接。历史保留是有限字节回放，超出范围会截断，尚非任意全屏程序的完整终端屏幕镜像。
运行中历史仅在内存中，正常退出后保存最后输出；宿主异常终止不保证恢复终端内容或进程。
远程客户端通过认证的二进制 HTTP 流连接，断开不结束宿主进程。

已验证真实 Windows PowerShell 的中文输入、文件写入、工作目录、缩放、Ctrl+C、
视图分离后继续执行、重新连接和退出码。真实 HTTP 客户端也验证同一宿主进程的断线续用、
缩放、清屏与关闭。上述终端和远程终端测试在 WSL Debian 无 DISPLAY/WAYLAND_DISPLAY
环境下通过；Linux amd64/arm64 服务构建通过，服务依赖图无 MyGo。macOS 和 arm64 实机
尚未验证。

生产桌面组件通过真实核心创建终端并执行 PowerShell，生成实际文件，原生截图显示
`GCODE_NATIVE_PTY_OK` 和中文输出。验证使用隔离数据目录，没有注入 mock 服务。
截图为 `output/desktop-live/terminal-native.png`；原生程序、终端、远程和 Linux 日志均在
同目录的 `terminal-*` 文件中。本次是程序驱动的原生验证，未恢复外部鼠标键盘自动化。

原始 panic、失败复现、通过后的测试日志及原生截图位于
`output/desktop-live/files-crash-*`。修复版使用现有 `~/.gcode` 配置和会话重新启动；
不清空用户数据，也不替换真实模型配置。

## 2026-10-29 重启后消息被跳过修复

原桌面发送编号来自内存计数器，例如 `primary::1`；重启后计数器重新从 1 开始，
与磁盘中的旧 Run 请求编号重复。真实核心按请求编号去重，因而新发送被当作旧请求，
没有启动模型，也没有产生可供界面显示的新回复。已有会话和重建侧聊输入区同样会冲突。

主聊天与侧聊现在为每次新的发送生成随机请求编号，移除原计数器。后端的同请求去重
保持有效，没有通过重放旧命令、清空历史或重置用户配置绕过问题。

回归使用真实核心和同一临时数据目录，关闭并重建桌面 App、核心和存储后再次发送：
新画布、已有会话、侧聊三个用例修复前都复现旧 Run 数量不变，修复后都产生新 Run
及模型回复。原有“旧编号碰撞”夹具改为验证新请求避开旧编号及忽略历史 ACK。

使用用户配置的 `deepseek-v4.1-flash` 实际完成两次启动后的输入框发送，
每次均收到并在 UI Tester 的渲染文本中找到 `GCODE_CHAT_RESTART_OK`。
该检查通过真实供应商配置、原生输入框和生产核心执行；使用隔离数据目录，
不修改日常聊天或供应商配置。它不等同于真实窗口的外部鼠标键盘自动化。
真实模型日志为 `output/desktop-live/chat-restart-live.txt`；
复现和回归日志为同目录的 `chat-restart-before.txt`、`chat-restart-after.txt`。

## 2026-10-29 Peri shell 参考与会话异常

只读核对 `D:/projects/peri` 的 `d7ee444e`：`BashTool::name` 固定返回 `Bash`，
`shell_command` 在 Windows 构建 PowerShell 命令，Unix 构建 `bash -c`。
GCode 按当前小写工具命名惯例统一为 `bash`，删除额外的 `powershell` 入口，
并在工具定义、系统提示中声明宿主操作系统、真实工作目录、路径和 shell 语法。
执行器切换不翻译命令脚本。Peri 的后台任务提升和日志文件管理不作为本次新增能力。

实际会话 `session-HP3YZVKQL2FLPUBT37Q64M5WFU` 中，SVG 已写入并通过 XML 校验，
预览 PNG 也已生成。中间有三次失败：可选渲染依赖不存在、PowerShell 收到 `/c/...`
风格路径、首次浏览器截图没有落盘。最后一次工具成功后，模型只返回 `Let`，
在协议完成事件前结束流；原记录未包含可用于进一步归因的上游原始日志。

对这类未完成的模型响应最多重试两次，重试仅限当前模型步骤。
每次清除该步骤的半截展示，再使用已保存的上下文继续请求；完成并验证响应前不执行新工具，
不重跑此前完成的工具。取消立即结束等待，重试仍失败时保留明确失败状态及已完成的工具结果。
三种模型协议均覆盖断流恢复与工具不重跑的回归，另测重试次数上限和取消。
本机制不将不完整工具调用当作成功，也不自动重放历史用户任务。

新生成的核心标识使用小写随机部分；覆盖新会话、侧聊和 fork 的实际创建路径。
模型返回的 tool-call ID 保持原样，用于协议内准确配对。

验证：全量测试、Go vet、MyGo vet、lint 通过；Windows 实际执行 PowerShell，
WSL Debian 无桌面环境实际执行 Bash，覆盖中文/空格目录、非零退出码和取消。
Linux amd64/arm64 服务构建通过，服务依赖图无 MyGo。

本轮真实供应商检查单独记录：纯文本流式请求约 10.4 秒收到 `GCODE_STREAM_PROBE_OK`，
但两次完整工具验证均在 120 秒内未完成。进一步直连探测的原始 SSE 仅有空 assistant
起始块和 keepalive，缺少正文、工具调用及结束标记；约 15.7 秒后流结束。
因此本轮真实模型工具闭环未通过，不能把本机 PowerShell/Bash 执行通过替代它。
没有修改供应商配置或把该空流当成成功。证据为 `output/desktop-live/peri-shell-live*.txt`、
`peri-shell-stream-probe.txt`、`peri-shell-tool-probe.txt` 和 `peri-shell-probe-raw.sse`。
