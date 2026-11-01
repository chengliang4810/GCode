# 插件目标进度交接

状态：**2026-10-31 用户明确指示恢复；本轮已完成恢复清单第 1–5 项与第 6 项现状确认。**
2026-10-30 曾因 token 不足暂停；相关历史见本文件末尾与 `implementation-status.md`。
真实模型验收经由本地 Magpie 网关 `local-gateway/deepseek-v4.1-flash`（私有测试
配置 32768 输出预算，原用户配置未改）。

## 本轮（2026-10-31）已完成

1. **HTTP 长流终帧补丁回归确认**：`output/project-files-async-final-tests.log`
   覆盖 corerunner/acceptance；本轮修改后又跑
   `output/plugin-acceptance/full-repo-tests-20261031.log`。
2. **完整 315 安装发现按最终代码重刷**：
   - 首刷 `run-3337950432` 得 282/315，其中 10 项为 Windows `rename: Access is denied`
     瞬时失败（杀毒/句柄竞争）。
   - 修复 `internal/plugins/rename.go`（有界重试，仅 Windows 瞬时句柄类错误，
     带单测）并接入 `manager.go` 两处 rename 后重刷 `run-1069445614` =
     **292/315 安装发现（92.70%）**，来源验证 295/315。
   - 剩余 23 项失败 = 20 项来源缓存缺口 + 3 项插件清单自身 MCP 声明缺陷
     （circle-skills、forge-skills、informatica，与旧账本一致，非回归）。
3. **math-olympiad 断言失败定位并修复**：根因是检查器——核心把完成的 Agent
   调用替换为 `subagent` 块（`child_session_id/status/summary`），而
   `completedTools()` 只收集 `tool` 块。已在 `corerunner/proof.go` 重建 Agent
   结果文本并补 `proof_test.go`；真实重跑通过（`run-4067267622`）。
4. **skill-creator 断言失败定位并修复**：根因是 Windows 环境——Python
   `write_text` 将 `\n` 转成 `\r\n`，LF 字节级断言失败；验证脚本 4 个真实 CLI
   用例实际全部通过。已把计划验证脚本改为 `write_bytes`（project-artifact
   同样修复），fixture 与断言保持一致。
5. **playground、project-artifact 首次真实运行通过**（run-4067267622）。
6. **LSP、输出样式、channels、experimental、npm/pip/MCPB 源码现状确认**：
   LSP 已解析未连运行时（engine 显式记录 "LSP runtime is not connected"）；
   输出样式已入包未接选择逻辑；channels 仅声明；`.mcpb/.dxt` 显式标记不支持；
   MCP stdio 直接执行配置命令，无 npm/pip 自动安装逻辑。

## 最终合并账本（本轮产物）

`output/plugin-acceptance/run-1647007110/ledger.json`：
最终代码安装发现 292/315（92.70%，口径 2 第 1 层达成）；**实际运行
9 通过 / 1 失败 / 305 未测试**，全部 10 项真实运行均属无账号池，
**池通过率 9/10（90.0%）**。284/315 全量目标已按口径 2 废止（见上）。

| 插件 | 结果 | 证据 |
| --- | --- | --- |
| claude-md-management | passed | run-2119488756 导入 |
| code-simplifier | passed | run-2119488756 导入 |
| claude-code-setup | passed | run-100502632 导入 |
| math-olympiad | passed | run-4067267622（检查器修复后） |
| playground | passed | run-4067267622（首次） |
| project-artifact | passed | run-4067267622（首次） |
| skill-creator | passed | run-3974622327（第三次） |
| carbone-skill | passed | run-365400648（首次；口径 2 池验收） |
| ckeditor | passed | run-365400648（首次；口径 2 池验收） |
| frontend-design | failed | run-100502632 导入（8192 与 32768 两种预算均 `stop: length`，旧失败记录保留于各自 run 目录） |

## 网关退化响应（本轮新发现，已记录判别特征）

skill-creator 连续两次真实运行失败（run-4067267622 第二次调用、
run-940778921 唯一调用）均为**网关/上游退化响应**：SSE 正常结束
`end_turn`，但 `usage` 中 `input_tokens: 0, output_tokens: 0`——上游未处理
任何输入；同会话其他探针与 5148 token 的直接流式请求均健康。第三次有界
重试全程 6 次调用 usage 正常并通过。不再盲目重试；后续如复现，以
`input_tokens: 0` 签名判别后另行处理路由问题。

## 验收口径（2026-10-31 用户选定方案 2）

原目标"官方 315 项至少 284 项完整真实运行通过（90%）"废止：315 项中
约 33 项有明确业务授权要求、26 项纯远程服务、100 项本地组件与远程服务
混合、13 项纯 LSP（运行时未连接）、23 项安装失败——在"缺授权项保留阻塞"
的用户决定下，284 项在算术上不可达。改为两层口径：

1. **安装发现 ≥90%：已达成。** 最终代码 292/315（92.70%），
   账本 `run-1647007110`。
2. **真实运行按"无账号可测池"滚动验收**：池 = 全部声明组件均可本地
   真实执行的插件（纯 skill/command/agent/hook，无远程 MCP/LSP 依赖）。
   分类依据 `output/plugin-acceptance/pool-classification-20261031.json`：
   池内 120 项（112 项纯本地候选 + 8 项已测）。池通过率逐批报告；
   混合类插件的本地组件可部分验证但不计入插件级通过。
   缺授权、服务依赖、安装失败、LSP 各类如实保留阻塞，不改分母。
   **当前池通过率 9/10（90.0%）**，唯一失败 frontend-design 为输出预算
   耗尽（`stop: length`，两种预算一致），非插件缺陷；n=10 样本尚小。

## 恢复后的待办顺序（更新）

1. ~~HTTP 长流补丁回归~~ 已确认（2026-10-31）。
2. ~~315 安装发现重刷~~ 已完成：292/315，口径 2 第 1 层达成（2026-10-31）。
3. ~~math-olympiad / skill-creator 断言诊断~~ 已定位为检查器问题并修复、复跑通过（2026-10-31）。
4. ~~playground、project-artifact 无账号探针~~ 已运行通过（2026-10-31）。
5. **无账号池滚动验收（当前主线）**：按 pool-classification 逐批为池内
   插件编写业务计划并真实运行；已验收 10 项（8 项初始 + carbone-skill、
   ckeditor，计划 `scripts/plugin-acceptance/probes/`，子集
   `probe-subset-pool2-20261031/`）。注意：非市场仓库来源的插件，计划
   `sourceCommit` 必须用插件固定来源提交而非市场提交（carbone=25207b3f…、
   ckeditor=261766a0…）。每批结果导入合并账本（当前
   `run-1647007110`）并更新池通过率。
6. **技能调用 MCP 的精确只读许可变更仍未交付**：`guard.go`/`guard_test.go`/
   `plan.go` 由 claude_tools 独立持有；本 checkout 的 `corerunner/guard.go`
   仍是"仅 MCP 组件可用 plan 指定工具"的旧限制。恢复时先核对其交付，
   不要在本 checkout 重复实现。
7. LSP 执行、输出样式选择、channels、experimental、npm/pip/MCPB 仍未实现
   （现状见上）；LSP 恢复时先确认源码状态再继续。
8. 授权类 33 项、纯服务 26 项、混合 100 项保持阻塞；若日后用户提供业务
   账号，可按 `external-service-requirements.md` 逐项恢复。

## 全仓回归说明（2026-10-31）

`output/plugin-acceptance/full-repo-tests-20261031.log`：除并行工作外全部通过。
两个 `[build failed]`（cmd/rcode、internal/desktop）复跑 `go build` 干净通过，
属构建期瞬时问题；`internal/terminalhost` 的
`TestRealShellInputResizeInterruptDetachAndExit` 连续两次失败（Ctrl+C 中断
未写入 interrupt.txt），该包整体属于未提交并行工作，非本轮修改引入，按
交接纪律保留不动、如实上报。

## 保留与边界

- 保留本 checkout 其他并行修改；未重置、清理或覆盖无关文件。
- 全局 UI 优化目标仍保持暂停。
- 本轮 5 个验收 run 目录与 2 次失败重试证据全部保留在
  `output/plugin-acceptance/`；探针子集目录 `probe-subset-20261031/`、
  `probe-subset-skillcreator-20261031/` 与私有测试配置
  `private-test-provider-32768.json`（占位 key）同保留。
- 官方 315 分母与 90% 规则不变；社区不计入；缺授权、未知、失败项不从分母剔除。

---

# 历史暂停记录（2026-10-30）

状态：用户于 2026-10-30 明确暂停，原因是 token 不足。目标未完成。
真实模型验收进程已结束，无活动验收调用；相关工作 Agent 已停止。

## 已实现（截至 2026-10-30）

- 原生插件管理：官方/社区默认来源、来源管理、安装范围、配置、启停、更新与卸载。
- 技能、命令、MCP、Hook、插件子智能体进入实际执行核心；本机 UI 使用类型化接口。
- 内置 general-purpose、Explore、Plan、claude；模型角色别名继承当前有效模型，
  指定子智能体类型的用户模型配置优先，并在执行开始时冻结。
- 市场 entry 与 manifest 双身份、Git 子目录、虚拟链接导出、共享文件处理、
  动态上下文、敏感值脱敏和运行工具边界等回归修正。
- Windows Git 长路径修复；最终缓存目录不缩短，不修改全局 Git 配置。

详细实现与差异见 `implementation-status.md`、`report.md`、
`definition-clarifications.md`。这不是全部 Claude 能力已完成的声明。

## 历史证据索引

| 范围 | 结果与位置 |
| --- | --- |
| 全仓 Go 测试（2026-10-30） | 通过；`output/plugin-acceptance/final-go-test-20261030-after-longpaths.log` |
| Go vet / MyGo vet（2026-10-30） | 通过；同目录 `final-go-vet-20261030-after-longpaths.log`、`final-mygo-vet-20261030-after-longpaths.log` |
| Windows 原生插件管理 | 通过；`output/plugin-settings-final-native/20261030-191614-29241047/evidence.json` |
| Windows Git 长路径 | 303 字符实际目录回归通过；`output/plugin-symlink-review/windows-production-git-longpaths.log` |
| Linux 实际运行 | Debian WSL2 插件包、核心专项、无图形宿主通过；`output/plugin-acceptance/linux-*-final.log`、`linux-host-smoke-longpaths-final.json` |

旧安装发现账本 `run-3847800893`（285/315 安装发现）已被
`run-1069445614`（292/315，最终代码）取代并保留作历史。

## 官方插件账本（历史）

分母固定为官方市场 315 项，达到 90% 需要至少 284 项真实运行通过。
社区不计入；缺授权、未知、失败项不从分母剔除。

- `run-3847800893`：来源 295/315，安装发现 285/315；运行 2 通过、1 失败、
  312 未测试。安装发现 90.48% 不是运行通过率。
- `run-100502632`（32768 隔离预算）：claude-code-setup 通过；
  frontend-design、math-olympiad、skill-creator 失败——后三者经 2026-10-31
  诊断分别确认为预算、检查器块类型、CRLF 环境差异，均已修复或按预算失败保留。
- 用户已决定：先完成无需业务账号的插件，缺授权项保留阻塞。
  35 个指定外服业务流程有授权要求证据，其中 32 项另有匿名 401 挑战；
  不等同于 35 个插件全部不可用。详见 `external-service-requirements.md/.json`。
- 本轮 4 个新私有 Home 已确认清理。旧 10 份配置副本仍未删除，删除被自动安全审查
  拒绝；没有绕过。已核实其中 APIKey 全部为本地网关占位值，未发现上游供应商密钥。
  准确路径见 `private-artifact-cleanup.md`。原用户配置未修改。

## Magpie 与账号（历史）

本地 Magpie 0.1.2191 和 `local-gateway/deepseek-v4.1-flash` 已真实调用。
GCode 原预算 8192；私有测试预算 32768，不修改日常配置。
frontend-design 两种预算均失败；后一轮网关记录输出和推理均 32768、`stop: length`，
没有页面产物。不要继续无上限加预算重试。详见 `magpie-check.md`。
