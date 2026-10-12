# /goal：Diff 与资源面板表头收紧

状态：表头实现、统计缓存、focused 验证、最终全仓回归和限定原生截图复核完成。用户视觉认可 pending。

## 目标

按 T3 暗黑参考和当前 GCode DESIGN 收紧右侧资源面板的首屏信息层级：移除顶栏可见的冗余“资源”标题；保留文件、Diff、终端 tabs 以及收起入口；移除 Diff 内容内重复的“项目 Diff + 分支”第二层表头；将真实 Git 变更文件数和新增/删除行汇总放在主表头剩余空间。窄面板需要继续显示 tabs 和关闭入口，摘要允许优先压缩文件数文字。

## 修改

- `internal/desktop/right_panel.go`
  - 删除资源面板顶栏可见的“资源”标题。
  - 保留 `ui.Tabs` 的“文件 / Diff / 终端”、`资源面板` 无障碍标签和“收起资源面板”按钮。
  - 为 tabs 设置可收缩的零最小宽度，给窄面板的 tabs、摘要和关闭入口保留布局空间。
  - Diff tab 调用主表头摘要渲染。
- `internal/desktop/diff.go`
  - 删除 Diff 内容区的“项目 Diff + 分支”第二层表头，因此 Diff 文件列表更早进入视野。
  - 新增真实 Git 摘要：变更文件数、累计 `+` 新增行和 `−` 删除行；非 Git 项目不伪造摘要，干净 Git 项目显示 `0 个文件`。
  - `cachedDiff` 在新 raw 解析时一次计算并保存 `added/deleted`，文件行和主表头复用缓存计数；raw 刷新时按路径和新内容替换缓存，保证统计更新。
  - 没有改变 Diff 解析、折叠、长行换行、短行自然高度或大 Diff 虚拟化。
- `internal/desktop/app_test.go`
  - 更新视觉样例断言，确认重复表头消失且样例真实摘要为 `2 个文件 / +12 / −1`。
  - 增加窄面板 tabs/关闭入口断言。
  - 增加非 Git 与干净 Git 状态的摘要/空状态断言。
  - 增加同路径 raw 刷新后的摘要和缓存计数断言。

## focused 验证

普通测试：

```text
go test ./internal/desktop -run 'TestVisualSampleRendersLightDarkAndNarrow|TestShortDiffCollapsePersistsAcrossFrames|TestDiffLongLinesWrapWithinViewport|TestShortDiffWrapUsesActualContentHeight|TestDiffHeaderSummaryPreservesGitStates|TestDiffHeaderSummaryRefreshesChangedRaw|TestNarrowExpandedResourcePanelFitsAvailableWidth|TestNarrowWindowCollapsesResourcePanelUntilExplicitlyOpened' -count=1
ok   gcode/internal/desktop  0.415s
```

Race 测试：

```text
go test -race ./internal/desktop -run 'TestVisualSampleRendersLightDarkAndNarrow|TestShortDiffCollapsePersistsAcrossFrames|TestDiffLongLinesWrapWithinViewport|TestShortDiffWrapUsesActualContentHeight|TestDiffHeaderSummaryPreservesGitStates|TestDiffHeaderSummaryRefreshesChangedRaw|TestNarrowExpandedResourcePanelFitsAvailableWidth|TestNarrowWindowCollapsesResourcePanelUntilExplicitlyOpened' -count=1
ok   gcode/internal/desktop  2.876s
```

本轮未打包、未启动原生窗口、未运行全仓回归、未提交。后续缓存修正也只运行上述 focused regular/race 测试。

root 最终回归与包身份见 [current-package.md](../verification/current-package.md)；实际深浅主题及窄窗候选见 [visual-v3.md](../verification/visual-v3.md)。原生样例汇总为 2 个文件、+12/−1；不能冒充真实任务产生的 Diff。
