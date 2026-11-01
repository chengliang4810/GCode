本目录是 Dynamic Workflow 技术调查用的独立 Go 模块，不是产品实现。

使用相邻目录 `D:/projects/moejs` 的实际源码，不修改 GCode 主模块依赖。
基线为 moejs `b9fa0469d1df0900d7a7a92fb43994a6e65e9319`。

运行：

```powershell
$env:CGO_ENABLED = '0'
go test -v ./...
```

验证范围：

- TypeScript 控制流、Promise 并行调用真正的 Go 异步宿主函数。
- JSON 节点记录落盘、重新创建 JS Runtime 后复用已完成结果。
- 显式输入身份和脚本哈希，拒绝错误缓存命中。
- 演示只缓存结果会改变 Promise.race 的历史完成顺序，证明还需重放调度。
- 验证 CompileTS 类型擦除不做类型检查。
- 验证调用点 direct eval 可捕获词法闭包；不表示已实现 TS hole 编译与权限。
- 验证无限脚本可由上下文中断。

宿主结果来自确定性 Go 测试函数，不是真实 Agent/供应商。这里的 site ID
由脚本显式提供，没有编译器站点注入、泛型 Schema、静态图、Actor FIFO、
完整恢复、amend/fill、权限、产物、UI 或远程接入。

JSON 写入使用临时文件、Sync、Rename；未进行断电/文件系统故障注入，
不能据此声称生产持久化或副作用 exactly-once 已完成。
`commitSequence` 仅记录结果落盘顺序，不是完整的 JS Promise 释放顺序；
缓存命中也返回 Promise，避免用返回类型变化制造重放反例。
未完成记录通过宿主返回错误模拟，不是进程崩溃；循环显式传不同 ID，
没有验证同一静态站点 ordinal；本实验也没有验证编译取消或确定性 API 封闭。
