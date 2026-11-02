# 换机交接入口

请先阅读 [换机与进度说明](docs/machine-handoff.md)，再阅读
[执行目标](docs/goal-execution.md) 和 [验收报告](docs/acceptance-report.md)。

当前工作分支为 `codex/isolated-worktree`。2026-10-29 用户确认当前 UI 基本满意，
暂停继续优化，并要求创建本地提交，下一阶段实现真实后端。完整视觉一致性仍未
验收；不得将阶段冻结写成完全一致。见 [UI 基线](docs/ui-checkpoint.md) 和
[恢复记录](docs/machine-resume.md)。

最新封存为R16：文件图片`.78`和无详情标签的错误标题提示已修；五项检查与四目标
发布包校验通过。GPU控制实验复现浅色1级差，但实际UI适配器尚未绑定。
见[阶段证据](reference/validation/tabstrip-r16-result.json)与[对照说明](docs/audit-tabstrip-file-tooltip.md)。
完整视觉仍未接受，Tooltip延迟/位置/样式待对齐。上述为历史R16结果；当前源码
使用MyGo v0.7.4（修订 `rcode-native-ui.23`，2,050文件/19补丁），以
`third_party/mygo.lock.json` 为准；`.9`（1,864文件/14补丁）是更早的R17历史指纹。
菜单请求流失已修；缩窄/收回后的行为另待原版交互核对。
此前 R8 独立原图组件分数保留原身份，完整视觉与 Mac 实机验收仍待完成。

本机路径、正在运行的进程、缓存和旧检查包都不能作为新机器的运行前提。
源码、固定依赖、资源、审查资料和重放脚本随分支保存。
