# /goal：工作区、worktree 与会话执行资源

## 范围

本目标负责 `internal/workspace`、`internal/worktrees`、`internal/execution`，以及内置文件写入协调工具。服务面向桌面会话提供目录/文件/Git/editor/worktree 接口，执行资源通过 session ID 做归属校验。

## 已完成

- workspace 服务：规范化工作区根和路径，拒绝绝对路径越界及 symlink 越界；支持文件列表、只读预览、二进制/截断标记、Git 状态和 Diff、配置 editor 启动。
- workspace 文件写入：同一路径使用短时锁，最终读取并校验预期版本后原子写入；不持有整个工作区锁。
- worktree 服务：从当前 HEAD 或指定本地分支创建独立分支 worktree；不复制源目录未提交内容，不提供自动清理；删除和合并前检查 session 占用与脏状态。
- execution 服务：每个进程记录所属 session；`StartOwned`、`ContinueOwned`、`SnapshotOwned`、`ListOwned`、`WriteInputOwned`、`ResizeInputOwned`、`StopOwned`、`StopAllOwned` 执行归属检查，输出有界并复用现有 PTY/transport。
- runtime 为每个 Agent session 创建独立的 execution process manager，并在 runtime 返回前停止该 session 的残留托管命令；用户交互终端不共用这组资源。
- 多会话真实链路验证：三个 session 共用同一个规范化临时项目 cwd，分别经 Service、Controller、Runtime、Provider 和 `exec_command` 启动命令；停止长命令 session A 后，A 及其后代进程被取消，短命令 session B/C 和独立用户终端均正常完成。测试使用本地 Chat Completions HTTP 协议替身，不调用厂商 API，详细记录见 `docs/verification/stage-3.md`。
- 内置文件工具：`FileTracker`、`CommitFile`、`WriteFile`、`EditFile`、`ApplyPatch` 共用 workspace 路径锁，陈旧输入在最终写入前拒绝。
- controller：可配置 `WorktreeManager`；新建 worktree session 使用 session ID 创建和绑定，持久化 session 加载时重新绑定，避免重启后误删仍被使用的 worktree。

## API 摘要

workspace：`New`、`ListFiles`、`ReadFilePreview`、`GitStatus`、`GitDiff`、`OpenEditor`、`ReadFile`、`WriteFile`。

worktrees：`NewManager`、`Create`、`Bind`、`Unbind`、`Users`、`Merge`、`Remove`。

execution：`NewProcessManager`、`Start`、`Continue`、`StartOwned`、`ContinueOwned`、`SnapshotOwned`、`ListOwned`、`WriteInputOwned`、`ResizeInputOwned`、`StopOwned`、`StopAllOwned`。

## 当前约束

- 文件协调只覆盖 GCode 内置读/写操作。任意 shell、外部 editor 和其他进程仍可同时修改目录；检测到变更时返回冲突。
- execution 只负责 Agent 托管命令；用户交互终端仍由 mygo 的真实 PTY 提供。
- workspace 和 worktrees 已按当前 `gcode` module 编译验证。

## 验证记录

- 已完成三个 session 的并发归属与取消验证：A 运行带子进程的长命令，B/C 运行短命令；三者使用同一个真实临时项目 cwd。停止 A 后，A 进入 `Interrupted` 且没有残留活动运行，A 的主进程和后代没有写出完成标记，B/C 均写出完成标记，独立用户终端也写出完成标记。
- 验证命令：`gofmt -w internal/application/concurrency_test.go`、`go test -mod=mod ./internal/application`、`go test -mod=mod -race ./internal/application`。
- 上述应用级并发测试覆盖真实 Service/Controller/Runtime/Provider/`exec_command` 和 POSIX shell；HTTP 服务仅为 Chat Completions 协议替身，因此不能替代真实厂商 API 验收。
- Git 子目录路径核查已修复：若打开的项目目录是 Git 仓库根目录下的子目录，workspace 服务先解析仓库根，再将 porcelain 的仓库相对路径过滤并转换为项目目录相对路径；外层改动不泄漏，嵌套项目内的已跟踪修改、未跟踪文件的 diff 和预览路径一致。真实临时 Git 用例位于 `internal/workspace/workspace_test.go`。
- 旧 worktree 的 `Prepare`、`Release`、`Clean` detached/自动清理路径及其专用测试已移除；当前生产入口只有 `internal/worktrees.Manager`，创建独立分支并保留 session 归属、脏状态和显式删除/合并检查。

## 阶段 5 集成支持回归

记录日期：2026-10-12。

- 品牌迁移相关断言已按当前 GCode 生产行为同步：modelregistry、peermsg、sessions、specialist 和 workspaceseed 的定向测试不再要求旧 Zero 展示文本；没有对 `IsZero` 或数学 zero 等正常语义做全仓替换。
- sandbox 的两个 legacy override 测试继续显式注入 `ZERO_SANDBOX_GRANTS_PATH` 和 `ZERO_WINDOWS_SANDBOX_HOME`，验证旧环境变量不会覆盖生产使用的 `GCODE_*` 变量；生产路径保持 GCode 命名。
- 通知配置和 `internal/notify` 已删除：当前桌面生产代码没有通知配置消费者，保留该字段只会继续解析、复制和校验一个无效入口。
- 本轮限定范围回归已通过：`internal/sandbox`、`internal/peermsg`、`internal/workspaceseed`、`internal/modelregistry`、`internal/sessions` 和 `internal/specialist` 均以 `go test -mod=mod ... -count=1` 通过。
- `internal/application` 的扩展测试仍有两项失败：`TestServiceExtensionsIntegration` 报告 `permission plugin_runtime_echo did not become visible`，`TestServiceExtensionChildCancellationOnStopAndClose/close` 未出现 `mcp.pid.done`。这两项属于 application/runtime 扩展链路，不属于本目标允许修改的范围；因此本记录不能据此宣称全仓回归通过，结果应交由 root/runtime 继续处理。
- 已通过工作区相关包测试：`go test -mod=mod ./internal/workspace ./internal/worktrees ./internal/execution ./internal/tools ./internal/controller`、`go test -mod=mod -race ./internal/workspace`、`go test -mod=mod -race ./internal/worktrees`。
