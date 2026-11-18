# 长任务内存排查（2026-11-18）

## 结论与边界

修复了桌面会话订阅绕过核心有界通道、向 MyGo 主线程无限提交回调的问题。
这证明了一个可复现的内存积压路径，尚不能证明它是当前安装进程全部内存占用的来源。
没有重启、停止或替换正在运行长任务的 GCode，也没有修改其会话数据。

## 运行进程证据

检查对象为 `/Applications/GCode.app/Contents/MacOS/GCode`，PID 10022，版本 0.0.3-beta，已运行约 7 小时。
RSS 在观察中约 304～364 MiB，出现自行回落，不能据此判断持续泄漏。
01:45 的系统分布采样记录物理内存足迹约 351 MiB。
本地记录索引中有 58 个会话，5 个状态为 working，其中包括父任务和 4 个子 Agent。
索引只作任务规模参考，不等同于实时 Go 堆分析。
系统采样：`output/memory-dispatch-20261118/live-vmmap.txt`、`live-process.txt`。
未获取运行进程的 Go heap profile，无法把 VM_ALLOCATE 全部归因到具体 Go 对象。

## 原因与处理

MyGo `Window.Update` 异步返回；其 mainLoop 使用 `append` 保存待处理回调。
原桌面订阅接收一个核心事件就提交一个 UI 回调，没有等待 UI 应用事件。
因此 UI 忙碌时，订阅持续排空核心通道，将事件及完整快照转移到无界主线程队列。
核心自己的 256 容量通道与溢出时快照重建机制不能限制这条队列。

现在每条订阅最多提交一个尚未应用的回调，完成后才接收下一事件。
取消订阅会立即结束等待；已排队的回调检查取消状态，避免应用过期事件。
继续保留事件顺序、会话代际检查及核心已有的溢出重建语义。
仅修改桌面事件调度，不改变本机核心通信方式或共享控件外观。

## 可复现验证

基准 `BenchmarkSubscriptionDispatchStalledUI`：模拟 UI 停顿 30 ms，最多 512 个事件，每个事件含独立的 64 KiB 文本快照。
在同一机器、同一 Go 工具链执行 `-benchtime=1x -count=3 -benchmem`。
模拟时间窗口内处理量受调度影响，因此数值为区间，不能据此推断真实进程总内存的节省比例。

| 指标 | 修复前（3 次） | 修复后（3 次） |
| --- | --- | --- |
| 排入主线程的回调数 | 260 / 466 / 512 | 1 / 1 / 1 |
| 分配字节 | 18,103,456 / 32,362,600 / 35,476,744 | 145,696 / 145,680 / 144,752 |
| 分配次数 | 2322 / 3764 / 4127 | 39 / 38 / 33 |

回归测试先在原调度方式下失败，再在修复后通过：

- `TestSubscriptionDispatchBoundsPendingUIWork`：UI 未处理前不得继续排入回调，恢复后严格按顺序应用全部事件。
- `TestSubscriptionDispatchCancellationReleasesWorker`：UI 不消费时取消也能结束 worker，迟到回调不再应用状态。
- 真实核心流式订阅、会话订阅范围及共享设置控件检查通过，命令：
  `go test ./internal/desktop -run 'Test(SettingsControls|InspectSettingsControls|SubscriptionDispatch|RealCoreSubscription|.*SessionSubscription|.*SessionScope)' -count=1 -timeout=90s`

竞态检测通过：`go test -race ./internal/desktop -run '^TestSubscriptionDispatch' -count=1 -timeout=60s`。

## 待上线验证

当前运行进程没有应用修复。待长任务结束并安装新构建后，再采集空闲、长任务中、任务结束后的物理内存及 Go 堆趋势。
本轮不承诺把整个应用内存降低到特定数字，也没有使用强制 GC 或任意内存上限掩盖数据持有问题。
