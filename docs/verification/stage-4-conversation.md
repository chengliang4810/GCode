# 阶段 4：长会话与阅读位置验证

状态：已完成代码级验证，等待 root 的原生生命周期验收。

## 实现范围

- 对话区使用 mygo `ui.ListState` 和 `ui.List`，只构建当前可视范围及 overscan 行，不再每帧遍历所有消息。
- 每个会话维护独立的 `ListState`，以消息 ID 作为稳定 key。
- `FollowEnd` 负责首屏和流式追加时的底部跟随；用户滚轮上移后，追加消息和末行高度增长保持当前阅读位置。
- 历史消息插入到列表头部时，mygo 根据稳定 key 自动移动锚点，保持原先正在阅读的消息位置。
- 消息的 Markdown 分块和行内 span 按消息、会话和主题缓存；代码高亮继续使用已有异步 Chroma 缓存。

## 自动化验证

```text
go test ./internal/desktop -run 'TestConversation' -count=1 -v
go test -race -count=1 ./internal/desktop
go vet ./internal/desktop
```

覆盖项：

- 5000 条会话只构建可见范围，首屏直接定位到末尾。
- 流式更新最后一条消息的文本高度时仍保持在底部。
- 用户上滑后追加消息不会抢回阅读位置，也不会重新启用跟随。
- 切换会话后分别恢复两个会话自己的滚动位置。
- 头部 prepend 历史消息后，原阅读锚点按新增行数平移。

## 限制

原生窗口滚轮、触控板惯性、中文输入法和真实 controller 事件仍由 root 的原生生命周期验收负责；本阶段不修改 acceptance 矩阵。
