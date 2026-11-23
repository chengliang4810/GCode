# MyGo 补丁上游审查

2026-11-23。核对了 egoist/mygo 上 chengliang4810 的 18 个历史 PR、仓库已有审查记录，以及部分候选在上游 main 的实现。GitHub 状态为 Closed 的 PR 也可能通过其他合并提交进入上游，须检查维护者留言和代码。

## 通用候选

| 补丁 | 初步结论 | 提交前证据 |
| --- | --- | --- |
| 24 列表无障碍空状态 | 最高优先级。上游 main 的 `accessTree` 和 `accessDetails` 仍可通过列表帧访问 `f.s`，`ListState.cursor` 仍直接读接收者。按列表、无障碍、panic 关键词检索未找到同类 PR。 | 在干净的上游 main 用最小应用或 Tester 复现。独立回归同时验证无选择状态的列表语义，以及有选择列表的焦点跟随。 |
| 21 Markdown 行内代码字号 | 通用功能候选。上游 main 仍用同一 `CodeSize` 控制行内代码和代码块。同账号既有的 Markdown 外观 PR 涉及行高、链接权重和锚点。 | 写清两种字号的独立需求及默认行为。准备最小示例，继续按配置名和行为检索。 |
| 22 Diff 折叠分隔条 | 功能候选。差异预览需要保留有限上下文并省掉交互分隔条时有价值，现有补丁覆盖并排和统一布局。 | 核对上游 main 的 Diff 公开选项及键盘、无障碍访问，避免复制已有配置。 |
| 08、15 正文及虚拟列表选择 | 可能是通用缺陷。分别验证拖选链接与选区端点滚出虚拟列表两个独立问题。 | 上游新增段落三击及拖选，先在主分支验证同一手势，再查重。 |
| 10 WebView2 数据目录、18 分栏拖动 | 需拆分后再判断。应用级 profile 路径与当前环境变量方案功能有交集。分栏越界回退已由 [#1475](https://github.com/egoist/mygo/pull/1475) 解决。 | WebView2 先证明公开 API 的独立用例。分栏只验证同帧更新缺陷，命中区尺寸留在本地评估。 |
| 04、06、11、13 输入快照、富文本、字形和 Tooltip | 属于较大的扩展。GCode 依赖其中的行为，按单一缺陷重新评估。 | 优先找独立复现。Tooltip 的输入快照和悬停方案须核对维护者已有的输入轮询取舍。 |
| 17、19、23 生成代码、构建身份和打包命名 | 本地交付补丁。 | 继续随 GCode 管理。 |

## 已提交、关闭或有上游替代

- 01 动效偏好 [#965](https://github.com/egoist/mygo/pull/965)，02 精确 DIP 行高 [#968](https://github.com/egoist/mygo/pull/968)，03 SnapToWindow [#966](https://github.com/egoist/mygo/pull/966)，09 折叠动效 [#959](https://github.com/egoist/mygo/pull/959)。按既有维护者决定保留本地，不重复提交。
- 05 Windows UIA 的生产修复已合入 [#1007](https://github.com/egoist/mygo/pull/1007)，本地范围为契约测试与注释。
- 12 点状下划线 [#962](https://github.com/egoist/mygo/pull/962) 和旧 Instant 状态 [#963](https://github.com/egoist/mygo/pull/963) 在关闭留言中均指向 [#969](https://github.com/egoist/mygo/pull/969) 的合入。禁止重复提交这些能力。
- 20 富文本视口裁切已在发布标签之后通过 [#2031](https://github.com/egoist/mygo/pull/2031) 进入上游 main。当前锁定标签本地保留；下一次包含 #2031 的版本升级时，用本地回归验证替换。
- 旧 OnHover [#964](https://github.com/egoist/mygo/pull/964) 和 OnFilesDrop [#956](https://github.com/egoist/mygo/pull/956) 已按维护者建议改成轮询。其他历史记录见 [既有 PR 审查](investigations/pr-triage-2026-10-31.md)。

初次筛选完成后，在上游 main 上做了独立回归。

后续提交了 [列表空状态修复 #2228](https://github.com/egoist/mygo/pull/2228) 和 [Markdown 行内代码字号 #2231](https://github.com/egoist/mygo/pull/2231)。字号 PR 在未指定 InlineCodeSize 时沿用 CodeSize，Markdown 全包测试通过。Diff 补丁继续留在本地。
