# 工具折叠调整

2026-10-30，按用户批准的方向试调。仅调整工具工作记录的折叠和长详情构建，
不恢复已暂停的完整视觉一致性目标。RCode 只读参考，独立实现行为。
接入前证据见 [调查记录](investigations/2026-10-30-tool-fold-motion.md)。

## 当前行为

- 工具组和单项详情使用 140ms ease-out 原生折叠；快速反向点击从当前进度继续。
  减少动态效果时立即完成。
- 手动折叠期间，仅暂停所属会话面板的尾部跟随弹簧，仍保持 FollowEnd；
  动画结束恢复正常流式跟随。中央和侧聊各自持有状态。
- 详情最多显示 24 行，超出内容内部滚动，仍可横向滚动、选择和复制全文。
- 紧凑模式把中间正文折叠为“过程说明”，分组汇总显示“过程说明 N 条”；
  它表示模型的过程文字，不表示写文件或持久化记忆。英文使用 Progress update。
- 不再为首屏逐行调用原生文本测量。解析数据按会话、块和字体缓存，
  首次只精确测量代表行及可见行，滚动后增量测量。源文本和流式状态改变会失效，
  字体改变保留解析结果、重算几何。缓存上限为 64 项、估算占用 16MiB；
  超大项仅由当前详情持有。缓存不保存 Element 或 Context。

入口为 `internal/desktop/transcript_motion.go`、
`transcript_detail.go` 和 `transcript_detail_cache.go`。
MyGo 新增 `CollapsibleBaseWithAnimation`，默认 CollapsibleBase 的行为不变；
API 和独立补丁在 `rcode-native-ui.16` 引入。
当前 `.17` 另收录并行工作已有的 WebView 数据目录源码为第 22 项独立补丁，
未在此专项改写那六个文件的功能实现。

## 验证

同机受控组件实验，10,000 行、60KB 短行夹具：

| 项目 | 调整前 | 调整后 |
| --- | --- | --- |
| 首次展开构建 | 71.54ms | 2.94ms |
| 首次展开分配 | 约 31MB | 2,689KB |
| 首次精确测量行数 | 全文 | 26 行 |
| 底部正文最大往返位移，工具组 | 167.29 DIP | 0 DIP |

调整前包含 200ms 折叠及尾部弹簧，调整后为实际生产 140ms 折叠策略。
这些是 `ui.Tester` 构建和几何数据，不能换算为真实桌面帧率。
调整后的热构建有低于本机计时分辨率的读数，不代表零耗时。
输入和结果保存在 `output/fold-motion-investigation/`、
`output/fold-motion-adjusted/build-cost.json` 和 `positions.json`。
纯数据缓存 benchmark 另测得冷构建约 1.07–1.18ms，
热命中约 21.7–22.8ns、零分配，不代表整个 UI 帧。

Windows 原生专项 `TestDesktopLocalTranscriptFoldMotion` 通过：

- 真实本机核心、独立数据目录、实际 Read 文件及本地 HTTP 模型夹具；
  深浅主题，正常/减少动态效果。
- 工具组、思考、读取详情的展开/收起与快速反向点击；每约 20ms 采样，
  正常动效覆盖中间状态。两主题共 14 组操作测得正文位移均为 0 DIP。
- 24 行视口滚至第 99 行、中央草稿不变、核心重开后会话和消息保持。
- 主/侧面板隔离、字体/内容失效、缓存淘汰、完整数据、静止后停止绘制，
  另由组件回归覆盖。

原生证据：
`output/fold-motion-native/20261030-175804-1229908973/evidence.json`；
汇总为 `output/fold-motion-adjusted/native-motion-summary.json`。
浅色思考和深色读取详情的真实窗口截图已检查。
本地 HTTP 夹具不代表真实供应商验收。

此次选定原生用例运行时，用 Go overlay 只隔离了仍引用已删除
`CustomWorktreeLocation` / `WorktreePath` 字段的旧 `features_test.go`，
未覆盖生产代码。映射为 `output/fold-motion-adjusted/e2e-overlay.json`。
这次不能记为整个 E2E 包通过。

18:04 前，专项组件测试、共享控件边界、Go vet、MyGo vet、普通桌面构建及框架源码锁通过。
全仓测试尝试仍有 Agent 清理、工具目录、存储/会话迁移和 MCP 退出等范围外失败；
不将专项成功表述为全仓通过。框架还存在已单独复现的
`TestReadOnlyInput` accessibility ActionSetValue 失败，不归入本次通过项。
随后同工作区的终端存储改动加入 `loadStandaloneTerminals` /
`persistStandaloneTerminals` 调用，18:07 复核时方法尚未定义，导致桌面专项
重新编译阻断；框架源码锁仍通过。已构建的体验程序和上述原生专项保留其各自证据，
不能据此宣称最新整个工作区可编译。

## 体验窗口

普通非 e2e 构建：
`output/fold-motion-preview/GCode-preview.exe`。
已打开独立场景窗口，会话名为「工具折叠体验」，数据位于
`output/fold-motion-preview/profile/`。
包含长历史、100 行思考、读取详情和 10,000 行短输出，供手动开闭和连续反向点击。
这是确定性场景预览，真实核心证据为上面的原生专项；未修改用户现有窗口和设置。
进程记录与启动捕获为该目录的 `preview-process.json` 和 `preview-visible.png`。

此前注释将详情行高写死为 18px，现已改为与字体联动的描述；
旧审计中的 200ms 差异保留为历史并注明已被本次 140ms 实现取代。
桌面测试文档中的框架版本、文件数和补丁数同步到实际 `.17`。
