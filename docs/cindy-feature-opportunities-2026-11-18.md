# Cindy 后续借鉴清单

依据本地源码及产品帮助检查，参考 HEAD：`c591eaa0feb2d6e45591effa6786b6fefe5c00d7`。下列是后续建议，不表示已经实现，也没有向外部服务发送消息。本轮实现范围见 [自动化与灵动岛](automations-and-island.md)。

| 优先级 | 功能 | 值得借鉴的具体行为 | GCode 的下一步 | 风险/边界 |
| --- | --- | --- | --- | --- |
| 高 | 自动化前置检查 | 无变化或条件不满足时跳过模型调用，并保留检查结果 | 在现有核心调度前增加有截止时间的检查，明确区分跳过和检查失败 | 命令执行权限、退出码和超时必须明确；不能把检查失败当成无事发生 |
| 高 | 项目自动化配置 | 仓库可分享自动化；文件变化先展示预览，确认具体内容后才导入 | 先支持显式导入/导出，再考虑项目文件入口 | 项目中的提示词/命令不能自动取得后台执行授权 |
| 高 | 每轮修改卡片 | 逐条回复对应文件改动，明确完整/部分捕获，支持有条件的撤销与重做 | 复用现有工具前后快照和变更记录，先实现精确的每轮审查入口 | 并发写同一工作目录、Shell 隐式写文件等情况必须标记不完整，不能猜测并覆盖他人改动 |
| 中 | 记忆的查看和编辑 | 用户可搜索、查看、修改或删除具体记忆，并能区分记忆与自定义指令 | GCode 已有记忆开关，下一步应优先提供可检查内容与作用域，而非继续堆积自动写入 | 项目和全局作用域隔离、凭据过滤；复用现有记忆模型 |
| 中 | 存储检查和清理预览 | 先扫描、展示分类和待删除内容，保护草稿/队列引用，清理前备份 | 先提供只读存储占用和一致性检查，再增加显式清理 | 不删除运行中会话、历史数据根或未提交 worktree；不默认自动清理 |
| 中 | 自动化模板 | 常用检查任务可从模板开始，保留完整可编辑提示词 | 提供少量内置模板，如工作日报、仓库检查；创建前允许审阅模型/项目/时间 | 模板不应暗含自动提交、推送或联系第三方 |
| 低 | 灵动岛增量体验 | 更多活动摘要、多显示器定位、有限提示音及紧凑状态细化 | 先验证当前浮窗在各平台的实际行为，再增加有明确需求的配置 | 避免复制角色系统、音效目录或大量展示配置；不重启已暂停的全局 UI 视觉改造 |

## 核查入口

- 前置检查：`packages/maker-scheduler/src/types.ts`、`apps/desktop/src/main/scheduler-host/__tests__/preRunHook.test.ts`、`apps/desktop/help-knowledge/scheduler.md`。
- 仓库自动化：`apps/desktop/src/main/scheduler-host/project-automation-loader.ts`，其中明确区分仓库预览与已确认导入。
- 每轮变更：`apps/desktop/src/shared/turnChangeSet.ts`、`apps/desktop/src/main/turn-change-set/store.ts`、`apps/desktop/help-knowledge/reviewing-changes.md`；类型中明确列出并发工作目录等无法安全撤销的原因。
- 记忆控制：`apps/desktop/help-knowledge/memory.md`、`apps/desktop/src/main/maker-host/memory-settings-store.ts`；GCode 当前入口为 `internal/desktop/settings_personalization.go`。
- 存储保护：`apps/desktop/help-knowledge/storage.md`、`docs/product-rules/archive-pauses-automations.md`。

推荐顺序为：自动化前置检查 → 仓库配置显式导入 → 每轮修改审查。前三项直接提高后台任务的成本可控性、可复用性和结果可检查性。机器人伙伴、社交渠道、语音、跨品牌 Harness 聚合暂不建议照搬：它们超出当前 GCode 的明确产品边界，且用户此前已要求移除 Bots。
