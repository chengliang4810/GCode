# 阶段 3：多会话并发与资源归属

记录日期：2026-10-12。真实厂商 API 尚未验收。

`internal/application/concurrency_test.go` 建立三个真实应用 session，并让它们共用同一个规范化的临时项目 cwd。测试使用本地 `httptest.Server` 提供 Chat Completions HTTP 协议替身，带固定认证 header 和 fixture session 标识，不调用任何厂商 API。每次请求仍经过真实 Service、Controller、Runtime、Provider，再由 `exec_command` 启动真实 POSIX shell 命令。新建 session 默认不开启 worktree，三个 session 直接绑定同一个项目目录。

测试场景如下：

1. A 执行 `printf started > a.started; (sleep 30; printf done > a.done) & (sleep 3; printf done > a.child.done) & wait`，用于保留主命令及后代进程。
2. B 执行 `printf started > b.started; sleep 2; printf done > b.done`。
3. C 执行 `printf started > c.started; sleep 2; printf done > c.done`。
4. 等待三个 started 标记出现，并确认 A 已进入保留命令的后续 provider 请求后，另外启动独立用户终端命令 `sleep 4; printf done > terminal.done`。
5. 调用 `Service.Stop(A)`，等待 A 进入 `Interrupted` 且 `ActiveRun == nil`，同时等待 B/C 和独立终端退出。

取得的结果：

- A 的 `a.done` 和后代的 `a.child.done` 均未生成，证明取消覆盖了 A 的托管命令及其后代进程。
- B/C 均生成各自的完成标记并恢复空闲，证明停止 A 没有取消其他 session 的命令。
- 独立用户终端生成 `terminal.done`，证明 Agent session 的托管进程资源与用户交互终端资源分离。
- 三个 session 均绑定同一个 canonical project cwd；协议替身检查每个请求的 fixture session 标识和 `Bearer fixture-concurrency-key`，错误请求会使测试失败。

验证命令均通过：

```sh
gofmt -w internal/application/concurrency_test.go
go test -mod=mod ./internal/application -count=1
go test -mod=mod -race ./internal/application -count=1
```

该阶段证明的是本地协议替身下的应用内核与真实 shell 集成行为，不能据此声称真实模型、真实厂商 API 或原生桌面界面已经完成端到端验收。

另有 Git 子目录路径核查：当打开的项目目录位于 Git 仓库根目录下时，`git -C <子目录> status --porcelain=v1` 仍返回仓库根相对路径。例如仓库包含 `src/base.txt`，从 `src` 执行状态命令仍返回 ` M src/base.txt`；但从 `src` 执行 `git diff HEAD -- src/base.txt` 无输出，只有 `git diff HEAD -- base.txt` 才能得到差异。workspace 服务现已先解析仓库根，将 porcelain 路径转换为项目目录相对路径并过滤外层改动，同时对未跟踪文件生成 `--no-index` diff；真实临时 Git 用例覆盖嵌套项目、外层已跟踪/未跟踪改动、嵌套修改、未跟踪内容和文件预览路径一致性。
