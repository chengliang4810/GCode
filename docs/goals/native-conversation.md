# /goal：正式包长会话原生交互与采样

状态：本目标限定范围已完成。责任：root。完整 GCode 根目标保持执行中。

目标：在当前 Intel Mac 的正式包隔离副本中载入明确标注的 100、1000、5000 条历史消息夹具，记录实际原生窗口、已实现文本范围、RSS、会话切换及输入控件往返耗时，验证原生合成滚轮上移与会话独立阅读位置。复用产品视图，不修改视觉候选或增加管理页面。

完成判据：

- 代码段及 Resources 与原正式包一致，数据目录、项目和 bundle identity 唯一。
- 三种消息规模在实际窗口可打开、输入读回、切换，记录原始样本和测量方法。
- 合成原生滚轮改变历史阅读范围，切换会话后恢复同一会话阅读位置。
- 有期限的驱动、原生 AX/截图/hash 和正常退出证据，源包和生产源码保持不变。

边界：历史夹具不是真实厂商对话；合成滚轮不是物理触控板惯性。结果不单独证明流式 controller 事件、历史 prepend、系统剪贴板、IME、其他平台、真实 API 或整体性能达标。没有用户指定性能门槛时只报告实际样本，不虚构通过线。

并行分工：conversation-performance 负责组件基准；shared-edit-service 负责真实 Service 文件竞争；pending-ui-harness 负责授权/提问原生驱动准备。root 独占原生窗口操作和本目标驱动，避免并发操控窗口。

完成证据：`docs/verification/native-conversation.md` 和 `output/acceptance/native-conversation/final/{manifest.json,measurement-samples.json,review.json}`。新正式包通过三种规模的末尾显示、中文/emoji 输入、合成滚轮上移、会话阅读恢复和独立指针恢复读回。root 实际打开五张原生截图，核对 hash 和 PID 44724 退出；源码/原包不变。旧包 round-3 留作基线，不被覆盖。没有把本目标提升为持续流式/并发性能、用户视觉认可或整体首版通过。
