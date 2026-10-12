# Application Service Worktree 目标

## 目标

从真实 `application.New` 入口闭环验证 worktree 会话的创建、绑定、持久化、重开以及显式合并/删除行为。验证使用临时真实 Git 仓库，覆盖默认关闭、指定本地分支、源目录未提交内容隔离、真实 `cwd`/分支/session 绑定和失败保护。

本目标只覆盖应用层服务与 `internal/worktrees` 内核行为，不建设桌面 UI，也不把程序化 fixture 或已有 controller 单元测试当作真实 Service 验收证据。

## 所有权

- 负责文件：`internal/application/worktree_integration_test.go`、必要的 `internal/application/service.go`、必要的 `internal/worktrees/**`、本目标文档、`docs/verification/worktree-service.md`、`output/acceptance/worktree-service/**`。
- 不修改：`internal/application/service_test.go`、桌面、`cmd`、controller、runtime、共享验收矩阵及其他代理负责的测试。
- 如需 controller 改动，先向主代理说明并协调，不在本目标内直接改变 controller 契约。

## 完成判据

1. `application.New` 创建的服务默认不会为新会话启用 worktree。
2. 开启 worktree 并指定本地分支后，创建真实独立分支；源目录中的未提交文件不会出现在 worktree 中。
3. 快照中的会话 `Cwd`、`Branch`、`Worktree` 与真实 Git 状态和 session ID 绑定一致。
4. `Close` 后用同一数据目录重开，worktree 会话及 manager 占用绑定保留，且不会自动重跑或删除工作目录。
5. 通过 Service 显式 `MergeWorktree`/`RemoveWorktree` 完成合并和删除；合并/删除失败时 worktree 仍保留。
6. 脏 worktree、源目录脏状态和其他 session 占用时，删除/合并被拒绝并留下原现场。
7. 定向普通测试、定向 race 测试和真实 Git 检查通过；所有命令、环境、结果、未验收 UI 缺口记录在 `docs/verification/worktree-service.md` 与 `output/acceptance/worktree-service/`。

## 证据边界

证据必须来自临时真实 Git 仓库和真实 `application.New` 入口。fixture-only 测试、controller 内部直接调用、桌面截图或未运行的静态推断不能标记为 Service worktree 已验收。当前任务完成后仍需明确记录桌面 UI worktree 入口未验收。
