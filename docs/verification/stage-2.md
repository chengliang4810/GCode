# 阶段 2：编码链路集成

记录日期：2026-10-12。真实厂商 API 尚未验收。

`internal/application/service_test.go` 建立本地 Chat Completions HTTP 协议替身，经应用适配器、真实 controller/runtime/provider 和内置工具完成：

1. 保存名为“测试服务”的自定义 API 服务与独立凭据。
2. 打开临时非 Git 项目并新建会话。
3. `read_file` 读取真实 `value.txt`。
4. `edit_file` 将实际文件从 `old` 改成 `new`。
5. `exec_command` 执行真实 `cat value.txt`。
6. 检查三个工具结果和实际文件。
7. 关闭控制器、重新加载相同应用数据，再发送下一轮。
8. 验证下一轮 HTTP 请求中保留三个 assistant tool call 与各自 tool result，检查配置快照不含测试密钥。

验证命令均通过：

```sh
go test -buildvcs=false ./internal/application ./cmd/gcode -count=1
go test -race -buildvcs=false ./internal/application -count=1
```

另有嵌套目录浏览、非 Git 目录与项目外文件拒绝测试，通过实际 workspace 服务验证。macOS `/var` 与 `/private/var` 别名已在适配器中规范化。

这些是本地协议替身的完整内核集成证据，不是厂商 API 或原生界面端到端验收。真实模型的读改命令、授权拒绝、长命令停止和重启续接仍待相应 API 凭据与场景运行。
