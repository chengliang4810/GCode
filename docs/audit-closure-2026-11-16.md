# nq7723skhoru 审计统一结论（2026-11-16）

已逐项核对原会话第一轮报告、后续自我纠正和补充，以及本会话两轮修复记录，按可独立判断的断言拆为 **83 项**。本页是这些已报告事项的统一当前状态入口；历史页面保留当时复现过程，不再作为当前待修清单。

核实基线为 `fa107dd` 及本轮选择性修改。共享工作树中其他任务的未提交图片、文件浏览器与 MyGo `.26` 改动不属于本轮提交和隔离验证版本；基线框架为锁定的 `.25`。没有推送、安装或运行真实供应商。

“已修复”包含先前提交与本轮修复，并不表示本轮新发现了这么多缺陷。“原判断过强/不成立”给出纠正依据；“契约已明确”以用户确认的受信宿主执行为准；“未复现”表示已检查所指路径但没有成立的失败证据，不编造复现。“平台/证据/门禁边界”是本次核实结论，不是承诺下一轮再调查的未分类问题。

## 远程与信任

| ID | 原报告断言 | 当前判定 | 核实结论 | 主要源码/回归入口 |
| --- | --- | --- | --- | --- |
| R01 | provider key 明文回传 | 契约已明确 | 认证持有者拥有宿主控制权；reveal 是显式动作，响应禁止缓存并记录审计，不能把授权读取当匿名泄露。 | `internal/remote/provider_keys.go` |
| R02 | 非回环 HTTP 无强制 TLS | 已修复 | 非回环默认要求 TLS；明确配置的受信反代/隧道例外独立处理。 | `cmd/gcode-server/main.go`；`internal/appconfig/connection.go` |
| R03 | Write/Edit 无目录边界 | 已修复 | 合法参数测试覆盖绝对越界、父目录和符号链接逃逸；不宣称限制 Bash 的 OS 权限。 | `internal/agent/tools.go`；`tools_test.go` |
| R04 | 链接可读任意宿主绝对路径 | 契约已明确 | 认证后的宿主文件操作属于受信控制能力；托管资源另有注册根及 os.Root 边界。 | `internal/engine/linked_files.go`；`attachments.go` |
| R05 | Bash 无 OS 沙箱/危险命令确认 | 契约已明确 | 用户明确采用受信宿主执行，不新增沙箱或通用危险命令分类器。 | `docs/execution-trust.md` |
| R06 | 子进程全量继承宿主环境 | 已修复 | 默认继承收敛为 OS/工具链所需环境，显式业务环境保留。 | `internal/processutil`；`internal/agent/shell.go` |
| R07 | 插件变量展开和敏感配置环境传参 | 契约已明确 | 是用户启用受信组件后的显式配置通道；同 UID 进程隔离不是当前产品契约。 | `internal/engine/plugin_resources.go`；`docs/hooks.md` |
| R08 | 插件默认启用/安装后执行 | 契约已明确 | 插件与 MCP 以宿主权限运行，不再将首次启动确认当作未修漏洞。 | `internal/plugins`；`docs/execution-trust.md` |
| R09 | MCP 可请求回环/私网 | 契约已明确 | 用户配置的本机和内网服务是合法用途；不能仅凭可达私网认定越权。 | `internal/mcp` |
| R10 | 图标服务 SSRF 断言 | 原判断过强 | 存在响应上限和同源引用/重定向限制；回环测试不等于真实云元数据攻击证据。 | `internal/providerlogo` |
| R11 | 远程目录选择可列任意路径 | 契约已明确 | 入口经 bearer 认证，目录选择是宿主控制能力，不是匿名遍历。 | `internal/remote/server.go`；`internal/engine/directories.go` |
| R12 | 错误包含内部路径 | 契约已明确 | 认证控制客户端可见宿主诊断信息；凭据泄露与路径可见须分别判断。 | `internal/remote/client.go`；`server.go` |
| R13 | Hook 搜索 Token 未脱敏 | 已修复 | 覆盖原文、URL/JSON 转义与全部 Hook 结果文本字段。 | `internal/engine/hook_runtime_redact_test.go` |
| R14 | 供应商/插件配置明文落盘 | 契约已明确 | 宿主私有配置是当前存储策略；系统密钥库是独立能力，未找到跨权限泄露实证。 | `internal/engine/session_storage.go`；`docs/execution-trust.md` |
| R15 | connection.json 读取跟随链接 | 已修复 | 读取复用 Store.Load 的常规文件和严格 JSON 校验。 | `internal/storage`；`internal/appconfig` |
| R16 | gitOutput 运行项目自定义 hooks/pager | 已修复 | 统一禁用 hooksPath、fsmonitor 与 pager，保留受信执行契约。 | `internal/engine/worktree.go`；`git_environment_test.go` |
| R17 | MCP stdio 写入无限制 | 已修复 | 与 HTTP 一样在写出前拒绝超出 24 MiB 的消息。 | `internal/mcp/client.go`；`write_limit_test.go` |
| R18 | 远程事件无续接 | 已修复 | Last-Event-ID、宿主 epoch、有限重放窗口和权威快照重建均有回环/核心回归；无跨重启持久事件日志。 | `internal/engine/event_resume_test.go`；`internal/remote/event_resume_test.go` |
| R19 | checkout 请求取消违反宿主拥有任务 | 原判断不成立 | 准备期尚未接受，可取消并清理；已接受任务归宿主，客户端断开不取消执行。 | `internal/engine/worktree.go`；`git_environment_test.go` |

## 模型协议

| ID | 原报告断言 | 当前判定 | 核实结论 | 主要源码/回归入口 |
| --- | --- | --- | --- | --- |
| P01 | Anthropic 同角色消息必然 400 | 原判断过强 | 相邻同角色已合并并有回归；原报告未经真实响应即宣称必然 400 的因果不成立。 | `internal/model/messages_test.go` |
| P02 | Responses 工具参数增量丢失 | 已修复 | 累计参数、缺失调用报错，completed.output 权威替换临时结果，空数组也有效。 | `internal/model/responses_test.go`；`stream_test.go` |
| P03 | Responses 最终文本前缀不符就失败 | 契约已明确 | 没有修订事件契约时失败封闭，避免静默拼出与已展示增量不一致的答案。 | `internal/model/stream_test.go：TestResponsesReconcilesFinalText` |
| P04 | 超大 SSE 被网络重试 | 已修复 | 单行及多行事件累计超限归 response_too_large；Gateway 只请求一次。 | `internal/model/stream_test.go`；`retry_test.go` |
| P05 | 无效工具调用无限重试 | 原判断不成立 | 允许模型纠正，但共享最多 10 次额外尝试的预算；不完整批次不执行。 | `internal/model/retry_test.go` |
| P06 | 流中断显示/history 不同即损坏 | 原判断不成立 | 显示保留诊断文本，执行历史只接收完整消息；重试替换未完成显示块。 | `internal/engine/run.go`；`internal/model/stream_test.go` |
| P07 | Chat 重复 tool index 静默覆盖 | 已修复 | 同一 index 出现另一调用 ID 时明确报错。 | `internal/model/chat_test.go：TestChatRejectsReusedToolCallIndexForAnotherCall` |
| P08 | 系统动态边界标记暴露给模型 | 已修复 | 三协议去除内部边界标记，保留缓存切分语义。 | `internal/model/chat_test.go：TestSystemPromptBoundaryIsNotSentToChatOrResponses` |
| P09 | 无 DONE 标记也应视为完成 | 原判断不成立 | 完整结束标记属于协议要求，不允许把截断批次当成功执行。 | `internal/model/stream_test.go：TestDecoderRejectsIncompleteStreamsAndCalls` |
| P10 | 0/0 usage 占位应一概忽略 | 原判断不成立 | 按协议字段存在性区分已知零/未知；Responses 占位另有专门规则，不能跨协议统一抹去。 | `internal/model/stream_test.go：TestDecodersUsagePresence`；`responses_test.go` |
| P11 | content_filter/未知 finish 诊断相同 | 已修复 | Chat/Responses 内容过滤独立归 content_filter 且不重试；未知 stop reason 保留具体值，不据诊断缺口推导错误执行。 | `internal/model/refusal_test.go` |
| P12 | refusal 内容丢失 | 已修复 | 保留 Chat refusal 增量和 Responses 流式/最终拒答；Messages refusal 正常结束并保留文本，混入工具调用时终止且不执行。 | `internal/model/refusal_test.go` |
| P13 | 纯图片消息生成空文本 part | 已修复 | Chat/Responses 不再生成空文本 part；三协议共同验证图片保留且没有空文本，不虚构曾收到供应商 400。 | `internal/model/refusal_test.go` |

## 核心与持久化

| ID | 原报告断言 | 当前判定 | 核实结论 | 主要源码/回归入口 |
| --- | --- | --- | --- | --- |
| C01 | async Hook 永久阻塞回合收尾 | 已修复 | 异步生命周期归宿主，不参与回合同步 join；有超时和关闭收尾覆盖。 | `internal/agent/hooks_test.go：TestAsyncHookDoesNotBlockTheTurnAndOutlivesIt` |
| C02 | 每订阅者重复构造完整投影 | 已修复 | 单次发布按作用域复用投影，再深拷贝给消费者，保留独立所有权。 | `internal/engine/engine.go`；`audit_regression_test.go` |
| C03 | 持状态锁编码并写持久化 | 已修复 | 编码及 I/O 在状态锁外；事务门保持串行，命令和最终收尾保留读取屏障。 | `internal/engine/persistence_commit_test.go` |
| C04 | 静默工具无法定时保存 | 已修复 | 独立定时刷新、退出等待及失败重试；间隔不代表崩溃丢失硬上限。 | `internal/engine/persistence_flush_test.go` |
| C05 | 自动压缩没有时间/进展边界 | 已修复 | 总截止 2 分钟、最多 16 fold、无进展停止；失败不发送已知超限正常请求。 | `internal/agent/agent.go`；`context_budget_test.go` |
| C06 | 中文 token 固定低估两倍 | 原判断过强 | 已改 UTF-8 字节估算，真实 usage 优先；没有特定 tokenizer 证据不能宣称固定比例。 | `internal/agent/context_budget.go` |
| C07 | 非分层手动压缩无法回滚 | 已修复 | 真实索引写入失败测试覆盖两种模式，恢复 context/history/memory。 | `internal/engine/compact_rollback_test.go` |
| C08 | Goal 索引省略导致重启丢失 | 原判断不成立 | Goal 在正文恢复，索引是元数据投影；重启暂停并保留预算。 | `internal/engine/goal_test.go：TestGoalRestartPausesAndSettlesToLastHeartbeat` |
| C09 | MCP 任意调用超时永久毁掉连接 | 原判断过强 | 请求等待取消与写出半帧后连接失败必须区分；不删除必要的协议失败隔离。 | `internal/mcp/client.go`；`client_test.go` |
| C10 | MCP Capabilities 无锁即竞争 | 原判断不成立 | 握手构造时写、发布后只读，返回值复制；未找到运行时更新路径。 | `internal/mcp/client.go` |
| C11 | history/context 浅拷贝必然回滚失败 | 未复现缺陷 | 所指就地覆盖发生在后台 CommitPreparedInput，事务门禁止其与命令并发；当前命令路径换 slice 或 append，未构造出回滚污染。 | `internal/engine/commands.go`；`run.go`；`persistence_commit_test.go` |
| C12 | Runs/ModelRequests 无上限即泄漏 | 核实为历史保留 | 记录随会话历史增长，删除/卸载有回收；未指定保留策略时不擅自裁剪用户历史。 | `internal/engine/session_views.go`；`model_requests_test.go` |
| C13 | mock 快照所有权不足 | 已修复 | 复用契约深拷贝，嵌套字段不再跨快照共享。 | `internal/mock/clone_test.go` |
| C14 | mock 带快照导致增量路径零覆盖 | 已补验证 | mock 快照与真实增量确有差异；保留确定性场景，并新增真实核心订阅到桌面合并的独立回归，不用 mock 证明真实增量。 | `internal/desktop/stream_subscription_test.go`；`internal/engine/event_resume_test.go` |
| C15 | 删除会话残留正文、Runs、Changes | 已修复 | 索引成功后清理拥有的正文与诊断，删除失败保留；未知文件和用户工作目录不递归删除。 | `internal/engine/session_tools_test.go`；`session_delete_test.go` |
| C16 | 文件操作/附件/提示词/删除持状态锁 I/O | 已修复 | 主要正文读写及清理在状态锁外执行；事务门保留，未将其描述为所有修改都可并行。 | `internal/engine/files_io_test.go`；`attachments.go`；`resources.go`；`commands.go` |
| C17 | 保存重复 Resolve 是已证实热点 | 原判断过强 | 重复校验存在，但已移出状态锁；缺少占比测量，不能宣称它比编码更慢。保持每次访问身份验证，不新增 TTL。 | `internal/storage/storage.go`；`internal/engine/session_storage.go` |
| C18 | rename 没有父目录同步 | 原判断不成立 | 非 Windows 的替换早已同步父目录，不重复增加热路径 fsync。 | `internal/storage/replace_other.go`；`root_json_other.go` |
| C19 | mkdir/unlink 缺父目录同步 | 已修复 | 拥有的会话/终端/工作区/运行目录新增和记录删除显式请求父目录同步；Windows 明确不承诺目录刷盘。 | `internal/storage/directory_test.go`；`internal/engine/session_storage.go` |
| C20 | 附件无上限/JSON 与批次成本 | 已修复 | 保留单件/数量/批次限制；二进制上传、读取前批次预检、按剩余额度读取及大小复核。 | `internal/attachmentio`；`internal/remote/resources_test.go` |
| C21 | 隐藏目录内容出现在普通搜索 | 已修复 | 按相对路径段过滤，覆盖 Windows 分隔符与显式 IncludeHidden。 | `internal/engine/files_test.go` |

## 界面与架构

| ID | 原报告断言 | 当前判定 | 核实结论 | 主要源码/回归入口 |
| --- | --- | --- | --- | --- |
| U01 | 全局指令 UI 线程读写远端路径 | 已修复 | 宿主类型化服务、远程认证适配器、后台加载与保存；UI 只读准备状态。 | `internal/engine/instructions_test.go`；`internal/desktop/settings_input_test.go` |
| U02 | 表格导出 UI 线程写盘 | 已修复 | 用户选择路径后在后台写入，Window.Update 回报结果。 | `internal/desktop/transcript_table.go` |
| U03 | 媒体缓存无界 | 原判断不成立 | 媒体缓存有 16 项 LRU；按条数而非解码字节计量，不等于无界。 | `internal/desktop/media.go` |
| U04 | 文件图标按文件名无限缓存 | 原判断不成立 | 缓存键是有限内置图标路径加主题，文件名只用于查表。 | `internal/workbench/file_icons.go` |
| U05 | artifact 全局缓存即泄漏 | 原判断不成立 | 缓存内置不可变 SVG，未发现随用户会话正文增长的入口。 | `internal/desktop/overlays.go` |
| U06 | 公式跨 goroutine 持 UI 元素 | 原判断过强 | 每 App 有界缓存及队列，worker 随 context 关闭；SVG/Invalidate 服务不等于持久 UI Element/Context。 | `internal/desktop/transcript_math.go` |
| U07 | 删除会话残留专属 UI map | 已修复 | 权威会话集合清理专属状态，保留新会话草稿、其他会话和混合用途全局键。 | `internal/desktop/session_cleanup_test.go` |
| U08 | 共享控件门禁只覆盖固定 control 名单 | 原判断部分不成立 | control 前缀已覆盖；跨包专用控件不在当前门禁。视觉一致性重构按用户要求暂停，不伪称已跨包验收。 | `internal/desktop/control_gate_test.go`；`docs/ui-controls.md` |
| U09 | Tokens/文件选择标题漏翻译 | 已修复 | 统一图表可见文本及提示，标题在 UI 线程准备翻译；中英文、深浅主题和宽度回归。 | `internal/desktop/settings_usage_charts_test.go`；`internal/i18n/zh-CN.json` |
| U10 | 文件树把文件名当未翻译文案 | 原判断不成立 | 文件名是业务数据；可见文本和可访问名均来自 File.Name，不翻译文件名。 | `internal/workbench/workbench.go` |
| U11 | 浏览器及命令页签缺稳定 key | 已修复 | 六个工具栏按钮按浏览器 ID 固定 key，命令页签固定索引 key。 | `internal/workbench/browser.go`；`internal/desktop/command_palette.go` |
| U12 | 非 Windows WebView profile 不跟随数据根 | 平台能力边界 | MyGo API 明确只支持 Windows；不是已证明的权限缺陷，不虚构跨平台能力。 | `third_party/mygo 的 SetWebViewDataDirectory` |
| U13 | 本机通过 IPC 绕行核心 | 原判断不成立 | 本机强类型同进程服务；远程序列化只在实际边界，测试 automation socket 不作为业务通道。 | `cmd/rcode`；`internal/desktop` |
| U14 | gcode-server 要求 MyGo/显示服务器 | 原判断不成立 | 依赖闭包无桌面 UI；Linux 无 CGO 服务构建单独验证。 | `cmd/gcode-server`；`go list -deps` |
| U15 | 模型供应商品牌绑定协议 | 原判断不成立 | 限定三协议，品牌、URL、凭据及模型由用户配置。 | `internal/model/client.go`；`internal/contracts` |

## 工程与文档

| ID | 原报告断言 | 当前判定 | 核实结论 | 主要源码/回归入口 |
| --- | --- | --- | --- | --- |
| E01 | pwsh 测试负载敏感/默认测试失败 | 已修复 | 等待按可观察结果和截止时间处理；Windows FIFO 测试按构建标签隔离，交叉编译验证。 | `internal/agent/background_test.go`；`internal/engine/files_pipe_unix_test.go` |
| E02 | CI race 必然超时 | 原判断过强 | 已拆核心普通回归与定向 race；本机耗时不能证明云端必然失败，不把未执行 Actions 写成通过。 | `.github/workflows/release.yml` |
| E03 | 原生 E2E 从不在 CI 执行 | 已修复 | 有独立 PR/手动原生 workflow 和隔离测试入口；只记录实际运行的场景。 | `.github/workflows/desktop-e2e.yml` |
| E04 | 格式/lint 配置等于已执行 | 核实为门禁边界 | CI 实际有 gofmt、go vet、MyGo vet；单独 golangci-lint 执行结果另记，不将配置文件当作通过证据。 | `scripts/lint.ps1`；`.github/workflows/release.yml` |
| E05 | install-smoke 无引用等于发布链失败 | 原判断过强 | 脚本入口存在与是否自动调用分别核实；未执行签名发布链不宣称成功或失败。 | `scripts/install-smoke.ps1`；`scripts/check-distribution.go` |
| E06 | 构建产物可被误提交 | 已修复 | .mygo、.tmp 和根二进制忽略规则已存在；本轮显式按文件暂存。 | `.gitignore` |
| E07 | 测试文件少即缺陷成立 | 原判断过强 | 测试按行为覆盖核对，不以文件名一一对应或 src:test 比例证明逻辑错误。 | `internal/contracts`；`internal/engine`；`cmd/gcode-server` |
| E08 | 提示词备份源码破坏整仓构建 | 已修复 | 源码快照曾改为 .go.txt；现已按清理要求删除备份目录，根模块不再解析混合 package。 | Git 历史提交 `88c9db8` |
| D01 | 多个文档当前框架指纹漂移 | 已修复 | 叙述文档统一引用 canonical lock，不把其他任务未提交的 .26 身份写进本轮；历史记录保留原身份。 | `third_party/mygo.lock.json`；`docs/mygo-patches.md` |
| D02 | UI parity 重复计数 | 原判断不成立 | JSON 有 58 个唯一 ID：48 included、5 macOS-only、5 excluded，与文档一致；原报告的 53 计数错误。 | `docs/ui-parity.md`；`reference/settings-index.json` |
| D03 | 更新入口/版本号过时 | 已核对 | 当前入口为关于，版本来自 mygo.json；历史版本保留时间语义。 | `docs/updates.md`；`mygo.json` |
| D04 | split-drag 补丁死链 | 已核对 | 当前引用的 v0.8.3-18 补丁存在；原 v0.7.4 路径不再当作当前入口。 | `docs/audit-panel-motion.md`；`patches` |
| D05 | 验收 manifest 等于可复核产物 | 证据边界已明确 | manifest 是路径/摘要索引，不能替代未随分支提交的原始产物。 | `docs/acceptance-report.md`；`reference/validation` |
| D06 | 历史 output/异机源码链接缺失 | 证据边界已明确 | 区分本机缺失产物与另一机器的外部源码引用；不补造运行证据，不以缺失证明历史未发生。 | Git 历史中的逐项核实记录；临时审计产物已清理 |
| D07 | 多轮报告互相矛盾/滚动列问题 | 已收口 | 本表覆盖原会话各轮结论和后续补充，重复旧报告已清理，可从 Git 历史追溯，当前状态统一由本表解释。 | `docs/audit-closure-2026-11-16.md` |

## 本轮实现边界

- 状态锁外执行主要文件命令正文读写、附件存取、提示词资源准备和提交后的会话清理。独立事务门仍串行修改；已安装待提交状态时，快照继续等待完整提交或回滚。元数据身份验证和会话预留仍属于串行事务，不能据此宣称所有 I/O 或所有命令均不等待。
- 删除会话的正文和诊断清理在索引成功后进行，并在不持状态锁时执行；失败删除保留正文和诊断。文件系统清理仍为应用拥有条目的尽力回收，不是安全擦除。
- 非 Windows 的拥有目录新增/删除请求同步父目录；Windows 目录同步明确没有新增硬保证。测试不替代掉电实验。
- 拒答按协议作为有文本的正常结束保留；拒答与工具调用混杂时拒绝执行整个批次。没有自动更换模型、重新尝试拒答或改变宿主权限。
- 文案和 key 修改不改变布局、控件尺寸或视觉规范；跨包视觉一致性重构继续暂停。
- 提示词源码备份曾仅增加 `.txt` 后缀；后续清理已删除该备份，原内容可从 Git 历史追溯。叙述文档以 canonical lock 为当前框架身份入口，历史身份不重标。

## 验证结果与证据边界

以下是审计阶段的历史验证结果。原始输出曾归档于本机 `output/session-audit-closure-20261116/`，已按后续清理要求删除；可随提交复核的回归测试源码仍在对应代码包中。

- 存储改造版本：Engine 全量通过（679.721 秒），Agent、Model、MCP、Remote、Contracts、Storage、Mock 全量通过。
- 桌面及相关包全量通过：Desktop 250.119 秒、Workbench 2.458 秒、i18n 0.090 秒；其余 36 个包的测试或无测试包编译也全部通过。
- Engine/Storage 原有关键边界定向 race 通过（112.455 秒 / 1.308 秒）。
- 协议补充后重新运行 Model/Agent 全量 race，并对真实核心订阅到桌面增量、删除失败保留诊断、文件 I/O 快照和目录边界补跑独立导出 race；Model 1.857 秒、Agent 30.586 秒；新增 Desktop/Engine/Storage 定向 race 分别为 4.934 / 9.286 / 1.138 秒。Git 配置隔离和 MCP 超大请求写前拒绝也通过独立 race。
- 整仓构建、普通/e2e 标签 Go vet、MyGo vet、框架与分发检查、格式检查和 Windows 核心/存储交叉编译分别记录，不用交叉编译代替 Windows 原生运行。
- `scripts/lint.ps1` 已实际尝试，下载固定的 golangci-lint 依赖时连接 `proxy.golang.org` 超时，未得到 lint 诊断结果；不记录为通过。CI 当前自动门禁为 Go vet、MyGo vet、gofmt 等，未宣称该脚本已自动接入。

未执行真实模型供应商、公网 TLS/反向代理、Windows 原生窗口、GitHub Actions 云端作业、安装替换/签名公证链或掉电实验。已完成的是当前源码、确定性协议 fixture、本机真实核心、回环 HTTP/stdio 和组件验证；这些环境边界在此一次性列明。

协议字段核对使用 OpenAI 官方 Responses 流式事件文档、官方 Chat chunk 类型及 Anthropic 官方 stop reason 文档；实现验证使用本地合成拒答文本，没有向真实供应商发送请求。
