# /goal：定位持续流式中的原生 AX 读回异常

状态：执行中。责任：root；本目标补足首版交互验收，不改变根目标的完整范围。

## 已确认事实

- 当前正式包 executable 为 `7bb8174524b12fec71a6b3bf5c653319f017394c7050b9f467e1091ebb5fd885`，生产源码和测试仍与最终回归一致。
- `native-streaming/round-4` 的三条 loopback 流、运行归属、最终历史和清理通过；首个 AX 聚合往返为 27686.087632 ms，另外三次约为 1664、761、877 ms。
- 同步 AX 操作占住测量循环，该轮只有两次 controller poll 和一次进程样本。不能把 driver PASS 当作持续交互性能通过。
- 原始证据没有 click/query/retry 分段时间，不能把异常归因到产品 UI、provider、runtime 或 System Events。
- [固定异常样本](../../output/acceptance/native-streaming/diagnosis-baseline/observed-outlier.json) 只用于对照；离线检查不能替代原生复现。

## 并行所有权

| 责任 | 所有权 | 产出 |
| --- | --- | --- |
| conversation_performance | streaming 驱动及其两份原有文档 | click/AX attempt 分段计时、状态时序、独立采样、严格预算、自测 |
| behavior_review | `ax-diagnosis-review.md` | 只读审阅反馈信号、现有 AX/profiling 能力、线程清理和凭据边界 |
| acceptance_refresh | `acceptance-gap-audit.md` | 原需求剩余缺口、OAuth 范围与可自主推进顺序 |
| root | 本文、原生串行执行及新增证据 review | 当前包身份核对、复现、最小化、原始样本分析及集成判断 |

## 诊断顺序

1. 修正驱动观测：AX 阻塞时仍采样 controller 和进程；分开记录点击、每次查询和等待，保留错误类型及调用前后流状态。保留现有归属断言，不增加超时。
2. 对真实截止时间模式做无窗口自测，拒绝超预算后到达的成功结果；自测必须使用实际等待路径，不能只测一个孤立计时公式。
3. 当前包保持不变，在新目录串行执行 round-5，记录正常退出/清理与生产 hash。30 秒 fixture 暂用于复现原异常的持续更新条件；在复现后再逐项减小输入和持续时间，不能先假定更小场景等价。
4. 得到可重复的具体失败信号后，再列出可证伪的根因假设，并一次改变一个变量验证。尚无证据时不修改生产代码。
5. 修复须在相同调用链回归；不把修正驱动预算、单次快速读回或最终 backend 正确代替界面响应问题的闭环。

## 完成条件与范围

异常的 click/query/retry 归属和持续更新时序有实际证据；最小复现能够报告相同失败；实际根因或明确的工具局限被验证，并复跑原三会话场景。root 逐项检查采样、run/消息/journal、截图、包身份、退出和凭据清理。

若没有可重复异常，只能报告本轮未复现及其样本，不能声明历史异常已修复。真实厂商 API、物理输入/IME/剪贴板、终端完整交互、用户视觉认可及其他平台仍由根验收矩阵独立约束。

## 2026-10-12 复现记录

- round-5：首个 click/query 共 1245 ms 通过，第二个 session click 在 12 秒上限失败，尚未进入 tail query；最终三条流及 journal 归属通过，但正常菜单退出失败，最终退出码 -9。独立 collector 108 样本无错误且 join 成功。root 核对832生产文件、正式包 hash、最终 snapshot/journal、凭据目录已删除和 QA PID 已消失，打开实际截图；详见 `round-5/review.json`。
- round-5 的 sample 实际落在第二次 click 阻塞期间，主线程忙于 CoreText 排版；Go 帧被 stripped，尚未定位 Go 调用或根因。ready 的 session-1 phase 标签是粗阶段标记，不作为精确归属。
- round-6：无 profiler，保留三会话、30秒、10Hz 的同规格输入；第三个 session tail query 在18秒预算内失败，包含12秒和约2.1秒的两次 AX timeout。三流各300 chunks、最终 snapshot/journal 通过；正常退出0、collector152样本无错误、凭据/PID清理通过。root 已复核并打开截图，详见 `round-6/review.json`。因此 profiling 不是异常存在的必要条件，但两轮具体失败阶段不同，不能称为完全一致的click复现。
- round-7-single：仅将 sessions 从3降为1，保持30秒和10Hz、文本模式、包与驱动不变，PASS。两个AX聚合为614/685ms，collector198样本、正常退出0。root 已独立核对最终snapshot/journal、832文件、正式包和实际清理并打开final截图。单轮通过不证明所有单会话性能或并发安全。
- round-8-two：在round-6规格基础上仅将sessions从3降为2，PASS，三次AX聚合2449/3677/821ms。正常退出与清理通过，root独立核对最终snapshot/journal、832文件和正式包；streaming截图已打开。
- round-9-three-short：恢复3会话，仅将duration从30降为10秒，PASS，四次AX聚合1332/1938/5920/675ms。正常退出与清理通过，root已独立核对并打开streaming截图；不把5.92秒读回当流畅性通过。
- round-10-three-mid：3会话、20秒、10Hz，PASS但首个AX聚合17402ms，包含重试，不能认定流畅性通过。正常退出、collector166样本、832文件和实际清理通过。root已独立核对并打开streaming截图。
- round-11-three-25：3会话、25秒、10Hz，首个AX query budget超时，聚合18373ms，FAIL。正常退出0；退出取消时第3条流242/250chunks，必须保留中断事实，不能写三流完成。root已核对源码/包/实际清理与已有snapshot/journal，未把缺少run_end的失败副本当完整后台PASS。

预算解释：18秒是 click 完成后的 tail-query budget；click 单独有12秒上限。不能宣称整个 read_session_ax 总耗时限定在18秒。

## 最小化边界与可证伪假设

当前最小已观察失败工作负载是3会话、25秒、10Hz；2会话30秒和3会话20秒不触发18秒deadline，但后者首个读回17.4秒。这是受时序影响的负载反馈，尚未证明精确毫秒边界。为保持用户要求的中文、emoji、列表、代码和表格真实路径，保留原始代表性payload；不把删减到纯ASCII或无窗口单位测试作为最终验收替身。反馈已有实际原生FAIL，后续在该窗口调用链定位并复跑原30秒输入。

root已在测试前向用户列出排名：

1. H1：流式消息反复整段排版。如果成立，同文本的分块稳定布局与缓存会减少主线程排版成本；trace会显示增长消息反复测量。
2. H2：更新未合并造成main queue积压。如果成立，只合并controller视图刷新而保留全部runtime事件会显著降低队列数量和ping延迟。
3. H3：会话切换后的缓存/scroll过期。如果成立，列表与显示内容的ID/版本可观察到旧值，切换后不再增长的固定消息也可能读错或迟迟不可见。
4. H4：AX导出大文本阻塞。如果成立，独立main-thread ping与真实UI操作迅速，但AX query明显变慢；源码/采样应落在accessibility出口而非普通绘制。

当前仍不把CoreText采样单独当作H1已证实。所有生产和driver代码仍冻结；后台specialist仅在独立/tmp源副本实现。

## H1 布局测量与修复

- 历史正式 binary 自身的 Go pclntab 可经 `go tool addr2line` 读取，偏移映射已保存在 `round-5/go-frame-symbolication.json`。原树是普通 SurfaceFrame/runFrame → layoutList/heightAt → intrinsic/leafWidths → text.System.Layout → CoreText；未显示 AX getter 为采样主路径，H4 降级为次级可能。
- 复用 mygo 现有 `MYGO_FRAME_STATS`，没有添加生产 instrumentation。round-12b 三会话25秒10Hz通过，但仅作诊断：303条帧记录，最大 layout 254.2ms，build 最大20.6ms。最大总帧2520.7ms是首帧GPU present占2491.5ms，不能归入文本排版。临时 raw 日志已删除，仅保留 frame 字段。
- 首次 round-12 因 wrapper 预建输出目录而在启动前被拒绝；该目录不是实际原生验收。wrapper 修正后使用新的 round-12b，未覆盖失败记录。
- 新增 `BenchmarkConversationGrowingMessagePending`，使用真实列表和平台 CoreText 布局链，模拟后台 parse completion 仍等待 UI queue 的单条累计消息。before 30/100/300段为34.2/102.5/270.3ms，after为3.46/2.12/4.94ms。300段分配量28MB降到287KB；这是同机单次样本，不是跨设备性能保证。
- 最小生产修复仅将 Markdown 尚未 ready 的原始文本按行构建 RichText，空行保留15.5×1.55 DIP高度，复用平台已有布局缓存。source 和消息复制仍完整；解析和最终 Markdown 渲染语义未变，没有增大 AX timeout 或减少会话/chunk/输出。
- desktop 普通测试与新正式 package 均 exit0。正在无 instrumentation 的 round-13 复跑原三会话30秒10Hz。生产冻结在该修复开始时解除；driver未改，后台 specialist仍只在独立/tmp stage修改。
