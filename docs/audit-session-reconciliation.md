# 会话选择与画布目标恢复（R11）

事实源固定为 Zeron v0.2.128 / `ab189159f10cf8e63c0502be75a59bbc487e0eaf`。
本机只读源码为 `output/reference-sources/zeron`。本轮修复的是会话数据与原生
View 的状态衔接；不据此声明完整视觉、输入或跨平台验收通过。

R10 已按实际会话 lookup 隐藏画布上的面板，但 `sessionID` 仍可能指向已删除
的会话。随后发送仍会携带这个 ID，不能把“看起来像画布”当作状态已经清理。
R11 接受快照后按当前已接受的 Sessions 校正选择；相同或更旧的来包不会替换
状态，但仍可修复当前状态本来就有的失效选择。已归档但仍存在的会话保留选择。
未知或历史已删除的 `chat/<id>` 导航落到画布。

冻结源码 `state.rs::select_chat/current_canvas_target/restore_canvas_target` 在
画布进入会话时记住项目、设备和显式无项目，在会话 A→B 时不覆盖。Go 现在
按同样边界保存目标，返回画布或删除所选会话时恢复草稿、项目及设备。项目
消失时先按记住的真实设备选项目，再按名称忽略大小写、ID 排序取全局首项。
显式无项目不会被自动选回项目；项目全部消失仍保留原设备。

原版 `state.rs` 的 `first_space_on_picked_device` 注释提到经过本机 fallback，
实际分支却是在指定设备没有项目时直接选全局首项，没有额外优先本机。
本实现遵循该分支，并有远程 Alpha、本机 Zulu 的反例回归。
同文件 `1508–1510` 的 healing 注释也警告全局 fallback 会改变画布目标，
但实际调用的 helper 仍允许该 fallback；本轮同样遵循代码而非这句注释。

Cloud 的逻辑目标和项目真实设备需区分。冻结 `pickers.rs:1152–1182` 只在项目
或有效设备改变时丢弃 Cloud checkout，不会静默恢复。Go 书签保留真实项目设备，
同项目、同设备可保留 Cloud 展示；跨项目、设备变化、删除项目后 fallback 到
其他授权仓库，均回到实际设备。项目本身以 Cloud 为设备时保留既有 Cloud 路径。

背景清理不改变设置页、设置 section 或当前 PR 内容。PR 目前使用共享工作区
scope，因此清理选择后暂缓切换可见 workspace，显式返回聊天或打开 Files、
History 等聊天 surface 时再同步。保留其他 scope 的编辑器缓冲区，避免把数据
选择清理扩大成删除尚未保存的工作区内容。

新建发送的关联也已修正。`Subscribe` 是容量 1 的最新完整快照流，瞬时
`Event.requestId` 可以被后续事件覆盖。`Run.requestId` 是本轮增加的可选 typed
字段，随完整快照及持久化保留。App 只采用本次 request、当前画布代次且不在
发送前已存在的 RunID；不再猜测“第一条新会话”。这同时防止重启后相同请求
字符串碰撞、侧聊/子会话误选、切页抢导航及新画布被旧结果覆盖。成功命令的
完整快照若已删除新建目标，也会结束 pending，避免输入永久禁用。

已删除所选会话的主草稿和待发送状态会清理。失败发送只恢复仍匹配的请求，
迟到的非空会话附件结果必须仍属于当前已接受 Sessions；它们不能重建删除的
会话草稿或替换当前画布。新画布代次也拒绝旧的空会话失败结果。

回归入口为 `app_session_test.go`、`attachments_test.go`、
`composer_footer_cloud_test.go`、`internal/mock/request_identity_test.go`。
覆盖两个完成顺序、快照合并、设置/PR/另一会话导航、历史 Run、画布代次、
创建后并发删除、项目/设备 fallback、Cloud 条件及持久化副本所有权。

最终源码、门禁、包、独立审阅和原生证据以
`reference/validation/session-r11-result.json` 为准。独立原生诊断 adapter 通过
typed mock 后端删除自有 `chat-0`，App.Attach 正常订阅后更新 UI；它与发布 EXE
有不同二进制身份，不能当成发布 EXE 的删除菜单验收，也不操作真实服务。

标签栏的 R11 几何与仍未完成的 scrollbar、fade、拖动及菜单视觉差异另见
`audit-workbench.md` 和 `output/tabstrip-r11-audit/result.json`。R8 成对图片分数、
R9 焦点诊断、R10 发布包原生观察均保留原身份；Mac、其他 DPI、IME、完整可访问性
和整体视觉一致性仍需独立验收，目标保持 active。
