# JSON 保存重复计算优化

运行中 GCode 的线程采样反复出现 `persistLocked → json.MarshalIndent`、
轨迹记录编码和 JSON 校验。此次只消除已经生成合法 JSON 后的重复处理，
不调整保存时机、磁盘格式、大小限制、截断内容、原子替换及同步刷盘。

- 轨迹复用检查大小时的编码结果；超过限制后重新编码截断后的记录。
- `WriteJSONRoot` 在 `MarshalIndent` 成功后直接调用已有的 `WriteBytesRoot`。
- 私有 `writeSessionRecord` 仅接收 `persistLocked` 直接编码的结果，复用同一写入入口。
- 公共 `WriteJSONBytesRoot` 继续拒绝非法 JSON，外部字节入口的校验不变。

`TestTrajectoryEncodedContentAndTruncation` 对普通中文记录和超限记录逐字节核对
磁盘内容，并检查调用方的请求与响应没有被修改。现有存储测试还覆盖编码失败不覆盖旧文件、
无效 JSON、路径逃逸、原子替换和会话持久化。

## 性能证据

同一 checkout、Go 1.27.3、macOS amd64、Intel i7-9750H，
`BenchmarkWriteTrajectory` 使用 512 KiB 的请求上下文，真实写入独立临时目录，
包含同步刷盘。前后各运行 6 次，每次 `-benchtime=200ms`，benchstat 对比：

| 指标 | 修改前 | 修改后 | 变化 |
| --- | --- | --- | --- |
| 每次耗时 | 69.21 ms | 55.40 ms | -19.95%，p=0.002 |
| 每次分配字节 | 2.568 MiB | 1.672 MiB | -34.90%，p=0.002 |

完整输出在 `output/runtime-optimization-20261112/benchstat.txt`；
基线在 `before.txt`，修改后在 `after-complete.txt`。同机还有用户任务，
耗时受系统负载与磁盘影响；结果证明该保存路径的收益，不代表整应用 CPU 降幅，
分配字节数也不是常驻内存。

最初一次后测被其他并行修改中的 WebFetch 编译错误打断；重测期间使用只修正该
依赖接收器名称的临时 Go overlay，未覆盖工作区文件。该依赖随后已在工作区修正。
功能验收使用不带 overlay 的当前 checkout。

## 功能验收

- Storage 全包通过。
- Engine 定向回归通过：轨迹编码与保留、会话保存与重开、提交失败保留原记录、
  离线历史根、路径约束、脏会话跳过、复合提交回滚及真实回环协议工具循环。
- `TestDesktopLocalTranscriptLinks` 深浅主题原生 E2E 通过，使用独立数据目录、
  本机真实核心与临时文件、回环模型和网页夹具；不代表真实供应商验证。
- 相关全包检查中的 Engine、Desktop 均超过 180 秒总时限，不能声明全包通过；
  日志保存在 `output/runtime-optimization-20261112/tests.txt`。

此次只提交源码与回归，不替换或重启用户正在运行的应用。
