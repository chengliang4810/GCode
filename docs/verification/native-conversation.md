# 正式包长会话原生交互与采样

日期：2026-10-12。环境：macOS 14.8.7 Intel，MacBookPro16,1。责任：root。

## 当前结论

性能修复后的正式包已完成 `final/` 原生复核。100、1000、5000 条夹具都显示末尾消息，九次中文/emoji 输入逐字读回；合成系统滚轮上移后，切走再回来保留相同阅读 marker。指针恢复已独立读回，正常菜单退出为 exit 0，PID `44724` 消失。五张截图已实际打开，hash 一致，源码和原正式包不变。本记录只通过这些限定行为，不代表完整 A13/A16 或首版交付。

本节所用性能修复包 executable SHA 为 `597842904bb16beb7b3d3c511e3ef785885a7c32f6ecbf329c03b415018e34ec`，代码段 SHA 为 `0af5901132a5587d961d2bd147a2ecc3ac9cf785ea221aaad24ab2a8c8f6f22e`。隔离身份为 `GCodeConversationQA1791750957266859000`，驱动 SHA 为 `029631c5a0c91a97f88fce605cfadc3a2458bdf6981647e598bcf246e27868be`。该包当时的生产源码相对旧包仅 `internal/desktop/app.go` 变化，go.mod/go.sum 未变；对应全仓 test/vet/fmt、package 和签名结果见 `output/acceptance/current-package-regression/`。后续运行归属与授权/中断呈现修复产生了新的包，最新身份见 [current-package.md](current-package.md)；本节结果不倒填成新包的长历史运行记录。

新包启动到 AX ready 为 3246.57 ms。原始样本见 `output/acceptance/native-conversation/final/measurement-samples.json`：

| 当前会话消息数 | 切换到末尾 AX 读回 | 输入往返中位数 | 当时 RSS | AX 历史 marker 数 |
| --- | ---: | ---: | ---: | ---: |
| 5000 | 470.08 ms | 265.62 ms | 69672 KiB | 8 |
| 1000 | 460.01 ms | 248.23 ms | 69956 KiB | 8 |
| 100 | 642.06 ms | 310.97 ms | 71652 KiB | 8 |

三个会话共 6100 条历史仍同时加载；这些 RSS 不是独立历史规模的内存曲线。AX 耗时包含 IPC/等待，后台负载未隔离，因此不能凭单轮新旧原生样本宣称延迟改善。滚动前底部 marker 为 `4992–4999`，滚动及切回为 `4987–4996`；截图实际内容从 `4995–4999` 移至 `4990–4994`。指针读回前后均为 `[1201.1124267578125, 338.3453369140625]`。机械 manifest 保留 `PASS_PENDING_CAPTURE_REVIEW`，root 的 `review.json` 为 `PASS_FOR_RECORDED_SCOPE`。

重放命令如下；目录已有记录，不可覆盖：

```sh
python3 docs/verification/scripts/native-conversation-qa.py --output output/acceptance/native-conversation/final
```

## 优化前正式包基线

性能优化前的正式包已完成一轮限定原生验证。通过产品自己的持久化加载路径载入三份明确标注的历史夹具（100、1000、5000 条，共 6100 条消息），三个会话均能显示末尾消息、输入中文/emoji 并逐字读回；合成系统滚轮能上移历史，切换会话后恢复原阅读范围。五张原生截图均已实际打开复核，hash 一致，正常菜单退出后 QA PID 消失。

此轮是旧正式包的基线，保留其独立身份和原始结果；新源码的原生结果以上述 `final/` 为准。

## 可重放驱动与身份

驱动：[native-conversation-qa.py](scripts/native-conversation-qa.py)。命令：

```sh
python3 docs/verification/scripts/native-conversation-qa.py --output output/acceptance/native-conversation/round-3
```

该目录已经存在；重放必须选择新的输出目录，脚本拒绝覆盖已有 manifest。源包为 `build/darwin-amd64/GCode.app`，隔离副本只修改 bundle identity 元数据和 ad hoc 签名；实际代码段及全部 Resources 与原包一致。

- 隔离身份：`GCodeConversationQA1791750210616827000`。
- 应用 PID：`39403`，已退出。
- 原正式可执行文件 SHA：`f1269b5c8a132a898565c9e06ab4660b389163158e814ee29986ac1f69b3a7b8`。
- 原始/隔离代码段 SHA：`d792ee8083451dc9b1a027f9f973edfe6f8784bd9aac0591731ad56609ad4fb5`。
- `source_files_unchanged: true`，`original_bundle_unchanged: true`。
- 未配置模型服务、未访问 API 或用户凭据、未操作系统剪贴板、未启用视觉样板模式。

夹具直接写入唯一隔离数据目录的当前 `controller.json`，再由真实应用启动读取。它验证产品历史加载和原生视图，不代表真实模型生成了这些历史，也没有为该夹具伪造 sessions journal。

## 原始采样

`output/acceptance/native-conversation/round-3/measurement-samples.json` 保存每次原始读回耗时。应用启动到 AX“设置”按钮 ready 为 3299.29 ms。

| 当前会话消息数 | 切换到末尾 AX 读回 | 三次输入往返中位数 | 当时 RSS | AX 实现的历史 marker 数量 |
| --- | ---: | ---: | ---: | ---: |
| 5000 | 460.98 ms | 389.27 ms | 73292 KiB | 8 |
| 1000 | 476.81 ms | 250.35 ms | 73864 KiB | 8 |
| 100 | 504.36 ms | 249.76 ms | 76652 KiB | 8 |

三个会话始终在同一进程中载入全部 6100 条历史；RSS 是切换阅读位置后的进程采样，不能据此画出每种历史长度的独立内存增长曲线。耗时包含 osascript/AX IPC、异步渲染等待，文件系统可能热缓存；并行代理测试与系统后台负载没有隔离。没有用户指定性能门槛，本记录不虚构性能通过线，也不把 AX 往返说成纯帧时间或物理按键延迟。

## 阅读位置证据

三种规模的 AX 都只实现底部 8 个消息 marker：分别为 92–99、992–999、4992–4999。五张截图显示实际末尾消息、代码块和固定输入区。

合成滚轮在实际 AX table frame `[545,166,590,681]` 中心注入。使用系统 `cghidEventTap`，先移动指针并使 QA 窗口处于前台；实际原生图从底部 `4995–4999` 上移到 `4990–4994`，末尾 marker 不再出现在实现范围中。切到 100 条会话再切回，AX marker 集合保持 `4987–4996`，两张原生图的阅读内容一致。

`review.json` 为 root 的 `PASS_FOR_RECORDED_SCOPE`；机械 manifest 继续保留 `PASS_PENDING_CAPTURE_REVIEW`。两者不改变用户视觉验收状态。该历史版本只发送了指针恢复事件，没有独立读回指针位置；后续驱动已加入恢复位置的读回断言，不能倒填旧轮次证据。

## 驱动失败历史

- `round-1`：AppleScript 将 `entire contents` AX references 列表强转为 text，导致 `-1700`。错误截图 helper 的普通 `PID` 占位符还误替换了 `kCGWindowOwnerPID` 常量。均为驱动问题，不是产品历史加载失败。最终改为直接返回列表和独特的占位符。
- `round-2`：三种历史、输入读回及 RSS 采样成功，但直接 `CGEvent.postToPid` 没有改变阅读 marker。没有独立证据证明该事件已由 AppKit surface 接收，不能归为产品滚轮故障。改用系统 HID 路径并实际观察 marker 后，round-3 上移成功。
- 失败轮次 QA 应用均已退出，保留其 manifest 和实物记录，不用失败轮次提高通过状态。

## 仍未验证

持续并发任务、真实 controller 流式事件、消息 prepend 的原生锚点、物理滚轮/触控板惯性、选择/系统剪贴板、物理键盘/中文 IME、长行/大 Diff/主题切换组合场景、真实厂商 API、Apple Silicon/Windows。A13/A16/P4.1/P5.4 仍须按各自完整范围验收。
