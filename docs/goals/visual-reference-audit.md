# /goal：T3 暗黑参考与 GCode closure 视觉差距审计

状态：只读视觉审计已完成；用户视觉认可仍为 `pending`；不把候选图写成视觉交付通过。

日期：2026-10-12（Asia/Shanghai）

## 目标

以用户提供的 T3 暗黑截图作为最低视觉质量标准，对照当前 GCode closure 暗黑 1440×900 原生截图，登记最多五项可以从图像、产品事实和设计规则追溯的精致度、层级、密度或一致性差距。审计只记录当前候选与参考之间的可见差异，不扩展 T3 的业务范围，也不替用户做最终视觉认可。

## 所有权与边界

- 本轮只修改 `docs/goals/visual-reference-audit.md` 和 `docs/verification/visual-reference-audit.md`。
- 已读取 `DESIGN.md`、`PRODUCT.md`、`docs/verification/visual-v2.md` 和 `docs/verification/visual-v2-review.md`。
- 已实际打开并对照：`docs/design/references/t3-dark-user-reference.png`（2360×1475）和 `output/acceptance/visual-v2/closure/dark-1440x900-native.png`（2880×1800）。两张图均为 16:10，审计按布局比例、层级和表面关系比较，不按物理像素逐点复制。
- 本轮不运行测试、不启动原生窗口、不重新截图、不修改生产代码、UI、矩阵或其他文档，不提交、不推送。
- `docs/verification/visual-v2.md` 与 `docs/verification/visual-v2-review.md` 的技术检查结果仍保留为 `technical_status: passed` / `readiness: user_review`；这不等于用户已经认可候选。

## 审计结果

五项具体差距及证据登记在 `docs/verification/visual-reference-audit.md`。它们是当前候选的审计发现，不是已经授权的实现任务；任何后续视觉改动仍需结合用户评价和产品约束单独决定。
