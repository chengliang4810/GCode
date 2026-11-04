# 消息导航条

2026-11-03：按用户截图补齐旧 RCode 的 `MessageRail`（对话大纲）。
本次参考为只读的 `D:/projects/RCode/crates/ui/src/rail.rs`，
检视时 HEAD 为 `e8399dc38ee669e60e8b87916a0bfd8f580270b6`。这是用户明确要求的
单项补齐；整体视觉一致性目标继续暂停。

主对话区左侧居中显示用户消息刻度；当前阅读位置的刻度加深，悬停或键盘聚焦时
加长。悬停预览显示该提问和它对应的回复开头，点击或按 Enter / Space 定位到
该用户消息所在的列表行。导航沿用当前 MyGo 虚拟列表的 `ScrollTo` 即时定位。
“返回最新消息”继续恢复跟尾，输入草稿不参与导航状态。

只在对话列表扣除会话信息卡留白后的可用宽度至少 768 DIP 且至少有两条用户消息时显示。
侧栏、工作区和会话信息卡占位都参与判断；侧聊不显示。每个刻度的命中区为
26×10 DIP，标记为 12×2 DIP，悬停扩为 20×2 DIP，间隔 3 DIP，左距 16 DIP。
长会话按连续区间均匀压缩，最多 12 个刻度，并按短视口高度继续减少。预览展示
该区间覆盖的提问数量；当前区间代表正在阅读的消息，其余区间代表区间首条消息。

行目标和预览在现有 `transcriptCache` 更新时重建；普通滚动、悬停不重新投影
整段会话。回复预览止于下一条用户消息，避免把后一个问题的回答显示在前一个
问题上。文本按 Unicode 字符截断并折叠空白；只有附件时显示文件名。

几何统一在 `internal/design/controls.go` 的 `MessageRail()`；刻度及预览外观统一
在 `internal/desktop/settings_controls.go`，使用当前主题语义色和界面字号。
页面只提供投影、当前位置、稳定 key 与跳转回调。

验证入口：

```sh
go test ./internal/desktop -run 'Test(MessageRail|SettingsControls|InspectSettingsControls)'
go run ./scripts/desktop-e2e -run TestDesktopLocalMessageRail
```

组件回归覆盖深浅主题、768 DIP 边界、单轮隐藏、137 轮压缩、预览、鼠标和键盘
定位、跟尾恢复、草稿及缓存失效。原生用例通过 MyGo Automation 打开独立的本机
核心与私有持久化历史夹具，覆盖深浅主题、中英文、窄窗口与会话切换。
这些检查不包含真实模型供应商调用或参考图像素一致性验收。

2026-11-04：主对话的滚动视口改为铺满当前对话面板，会话信息卡的占位只施加于
正文、查找/通知区域和输入框，不再把整个滚动容器向左挤。
滚动条保留 MyGo 标准的 3 DIP 边缘间距；打开右侧工作区后沿对话面板分界显示。
消息导航条的宽度判断仍扣除信息卡留白，不能因滚动视口变宽而覆盖正文。
`TestTranscriptScrollbarTracksConversationPaneEdge` 验证深浅主题与 900/1500/1800
窗口下的视口边缘、正文列位置和右缘滚轮输入。
`TestDesktopLocalTranscriptScrollbarEdge` 使用独立私有核心、持久化历史和原生窗口，
验证信息卡显示/隐藏、工作区打开、右缘滚轮及草稿保留；不请求模型供应商。
分栏线沿用现有 20 DIP 拖拽命中区，工作区打开时滚轮测试在其内侧的正文区域进行，
不把分栏拖拽命中区的行为算作滚动条拖拽验收。
此轮组件专项、导航条原生回归、Go vet 和 MyGo vet 通过。
滚动条原生最终证据为
`output/transcript-scrollbar-final-native/20261104-165143-3350953351/evidence.json`。
完整桌面套件仍有两项旧用例失败（已移除的 Apply 与已迁走的 General 更新按钮），
见 `output/transcript-scrollbar-desktop-tests.log`；不声明全套桌面测试通过。

2026-11-03 当前实现验证通过：

- `go test ./internal/desktop ./internal/design ./internal/i18n -count=1 -timeout 2m`；
  最后的稳定 key 调整后，导航条、共享控件及跟尾相关回归再次通过。
- `go tool mygo vet ./internal`；维护代码范围的 Go vet 也通过。
- Windows MyGo Automation 的浅色英文、深色中文及窄窗三个子场景通过；
  实际注入鼠标点击和 Enter 事件，验证跳转、高亮与返回最新消息。
  最终证据为 `output/message-rail-native/20261103-134451-2651879597/evidence.json`，
  原生截图与无障碍树保存在同目录各子场景的 `rail/` 内。
- 完整外壳的组件测试还覆盖 1320 → 900 → 1320 窗口变更，导航条随实际对话区
  缩窄隐藏、放宽恢复。

维护代码范围的 `CGO_ENABLED=0 go test ./internal/... ./cmd/... ./scripts/...`
中，业务与桌面包通过；发布资源包 `scripts/licenseresources` 的两个用例报告现有
`license checksum mismatch: README.md`，本次没有修改 README 或许可证清单。
根目录的 `./...` 还会扫到历史 `output/desktop-e2e/` 中文路径下的 Go 源码夹具，
产生非法 import path；因此 MyGo 静态检查从 `./internal` 运行，没有删除历史证据。
