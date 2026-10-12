# Application Service Worktree 验证

日期：2026-10-12  
环境：macOS 14.8.7，Darwin x86_64，Go 1.27.2，Git 2.39.5  
证据目录：`output/acceptance/worktree-service/`

## 结论

真实 `application.New` 入口的 worktree 主流程已通过定向普通测试和定向 race 测试。默认会话保持项目根目录且 `Worktree=false`；显式开启后，服务根据指定本地 `feature-base` 创建独立 `gcode/<session-id>` 分支，快照中的 `Cwd` 与真实 Git worktree、分支一致；源目录未提交文件留在源目录且没有被复制。

`Close` 后工作目录仍在，使用同一 data directory 重开服务可以恢复 session 的 `Cwd`、分支和 worktree 标记；重开后的显式 `RemoveWorktree` 成功，说明持久化 session 绑定能够继续完成 owner 操作。显式合并、删除已通过；脏 worktree 的删除/合并和脏源目录的合并均被拒绝，失败现场保留。worktree manager 现有测试另行覆盖其他 session 占用时的删除拒绝。

## 场景与证据

### 真实 Service 创建与重开

- 测试：`internal/application/worktree_integration_test.go:TestServiceWorktreeDefaultBranchAndRestart`
- 临时仓库：测试内执行真实 `git init -b main`、提交初始文件、创建 `feature-base` 提交并切回 `main`。
- 默认关闭：未设置 `Worktree` 的会话 `Cwd` 等于 canonical 项目根目录，`Worktree=false`，`Branch` 为空。
- 分支选择：`Service.ListBranches` 返回本地 `feature-base` 与 `main`；新会话传入 `BaseBranch=feature-base`。
- 隔离：真实 `git worktree add -b gcode/<session-id>` 建立独立目录；`git branch --show-current` 与快照 `Branch` 一致；feature-base 的已提交文件存在，源目录中同时存在的 tracked `README.md` 未提交修改和 untracked `source-uncommitted.txt` 都不进入 worktree，且仍保留在源目录。
- 持久化：`Service.Close` 后 worktree 目录仍存在；用同一 data directory 调用 `New`，快照恢复相同 session ID、`Cwd`、分支和 `Worktree=true`；随后通过 Service 显式删除恢复的 worktree。

### Service 操作与失败保留

- 测试：`internal/application/worktree_integration_test.go:TestServiceWorktreeOperationsProtectAndCloseExplicitly`
- 脏 worktree：创建 `pending.txt` 后 `RemoveWorktree` 返回 `worktrees.ErrSessionWorktreeDirty`，文件和目录仍在。
- 脏 worktree 合并：创建未提交 `feature.txt` 后 `MergeWorktree` 返回 `ErrSessionWorktreeDirty`，目录仍在；提交后继续重试。
- 脏源目录合并：源目录创建 `source-pending.txt` 后 `MergeWorktree` 被拒绝，worktree 仍在；清理源目录后重试成功。
- 显式完成：`MergeWorktree(..., "main")` 把 feature 文件合入源目录；`RemoveWorktree` 成功删除 worktree，session 保留并带有“worktree 已删除”状态。

### Manager 占用保护

- 测试：`internal/worktrees/manager_test.go:TestManagerCreatesBranchedWorktreeAndProtectsRemoval`
- 真实 manager 绑定 session 后，`Remove` 返回 `ErrSessionWorktreeInUse`；解绑后对脏 worktree 返回 `ErrSessionWorktreeDirty`；清理脏文件后删除成功。
- Service 公共 API 没有把同一 checkout 绑定给第二个 session 的入口，因此占用场景由 manager 的真实 Git 测试直接覆盖，未通过反射或测试专用 Service API 注入状态。

## 执行命令

```sh
go test ./internal/application -run 'TestServiceWorktree' -count=1 -v
go test ./internal/worktrees -count=1 -v
go test -race ./internal/application -run 'TestServiceWorktree' -count=1
go test -race ./internal/worktrees -count=1
```

四条命令均退出 0。定向应用测试实际运行两个真实 Git Service 场景；manager 测试覆盖分支 worktree、合并、占用/脏删除保护、symlink base 以及 Git 子进程取消安全性。详细命令摘要和运行结果保存在 `output/acceptance/worktree-service/`。

本轮没有把全量 `go test ./...` 当作 worktree 证据。一次额外的应用包全量运行在已有 HTTP fixture 的 `httptest.Server blocked in Close` 连接等待处超过 170 秒后中止；这不影响上面的四条定向命令，后续总回归由主任务统一执行并记录。

## 实现修复

真实 macOS 临时目录验证发现 `worktrees.Manager.Create` 返回了 lexical `/var/...` 路径，而 Git 真实 worktree 根目录解析为 `/private/var/...`，造成 session `Cwd` 与 Git 检查结果不一致。`internal/worktrees/manager.go` 在 `git worktree add` 成功后将返回路径 canonicalize，使快照、Git cwd 和 manager 绑定使用同一真实路径；现有 symlink base 测试与 Service 重开测试均通过。

## 未验收项

本次只验证应用层 Service、controller 已有 worktree 路径和 worktrees manager。桌面 UI 没有 worktree 创建、分支选择、合并、删除入口或原生交互验收；没有把 UI 功能标为通过。真实厂商 API、跨平台原生运行和用户视觉验收也不属于本目标。
