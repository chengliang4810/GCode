# 输入框命令与资源引用

主对话和侧对话共用原生 MyGo 富文本输入框：

- `/`：内置 Init、Goal、Compact、已启用的用户命令和技能。
- `@`：已启用的用户技能和 MCP 服务器。
- `$`：已启用的用户技能。

候选随光标处的查询过滤。上下键移动，Enter 或 Tab 插入，Esc 关闭；
修改查询后重新打开。鼠标选择与键盘使用同一条插入路径。
没有匹配项时保留空状态，Enter 不意外发送未完成的查询。
候选列表滚动跟随键盘选择，面板始终在输入框上方的全局浮层显示，
覆盖目标、待办列表和消息内容，不推动页面布局，也不受会话滚动区裁剪。
上方空间不足时缩短列表并滚动，不翻转到输入框下方。

插入结果是带图标和名称的原子引用，后面跟一个普通空格。
删除、选择、撤销及富文本复制保留资源身份，同名资源不会按显示名重新匹配。
剪贴板的纯文本使用名称；同进程富文本剪贴板保留稳定 ID。
消息的 Copy 操作复制原始标记，再粘回输入框可重建引用。
输入法预编辑不写入草稿，Enter 确认候选时不发送消息。
这些输入法断言来自 MyGo 事件路径测试，尚不代表 Windows 系统输入法实测。

草稿、消息和远程边界使用以下规范文本；本机 UI 与核心仍通过 Go 类型直接交互：

```text
[Init](gcode://command/init)
[文档](gcode://skill/documents)
[本机工具](gcode://mcp/local-tools)
```

名称只用于显示，核心按 ID 校验当前启用状态。选中技能会将正文加入本轮模型输入；
MCP 引用提示使用已配置的工具，不安装、启用或认证服务器，也不强制调用工具。
所有启用的 MCP 服务器仍沿用既有核心生命周期。
用户命令使用 `$ARGUMENTS`、`$0` 起的位置参数、`$ARGUMENTS[n]` 与声明式参数名展开；
参数支持引号分组；缺失位置保留原占位符，无接收占位符时追加 `ARGUMENTS: ...`。
排队消息分别保存原始输入和已展开的执行输入，
编辑队列和历史消息不会展示技能正文或丢失引用。
用户资源引用在接受发送时校验，队列保存原始标记及执行输入。插件和项目的 slash 调用
还会在实际运行 worker 中处理 frontmatter、输入准入与动态上下文，不能在 UI 线程执行脚本。

`/init` 请求 Agent 调查工作区并生成或更新 AGENTS.md；
`/goal` 使用已有目标生命周期，引用的技能/MCP 说明随目标执行保存；
`/compact` 使用已有上下文压缩功能。内置命令名不能用于自定义命令。
命令只在整条输入开头执行；`你好 Init Goal` 这样的混排只表示引用，
不会依次启动两个命令。

发送后使用同类图标和名称渲染用户消息，保留相邻 Markdown、链接及代码块。
混合公式、HTML、图片的段落交给既有 Markdown 渲染器，引用仍显示为链接文字。

本次参考 `D:/projects/ZCode` 的候选、原子标记和历史消息交互，
未引入其 React/Electron 编辑器。GCode 当前候选列表仍取用户技能、用户命令和 MCP，
没有把插件资源伪装成用户资源 ID。插件市场管理已另行接入宿主；
已安装插件的技能/命令用普通文本 `/插件名:资源名 参数` 或模型的
`Skill` 调用（`{"skill":"插件名:资源名","args":"参数"}`）进入运行层。
项目 `.claude/commands`、`.claude/skills` 也由实际项目目录加载。
候选框尚不列出全部插件和项目资源；没有文件/会话/白板或子 Agent 引用目录。
引用入口本身不会安装插件、启动 OAuth 或认证 MCP。

当前插件 slash/Skill 路径支持动态 `!` 命令上下文、context=fork 和技能 hooks；
这些在宿主执行，跨设备连接时使用宿主路径、依赖和凭据。allowed-tools 等高级元数据
仍需按实际权限语义验收；不把仅解析成功当成兼容通过。

验证入口：

```sh
go test ./internal/contracts ./internal/engine ./internal/desktop
go test ./internal/desktop -run 'Test(SettingsControls|InspectSettingsControls)'
go run ./scripts/desktop-e2e -run TestDesktopLocalPromptReferences
```

原生用例使用独立数据目录、真实本机 Go 核心和本地 HTTP 模型夹具，
覆盖深浅主题、900 宽窗口、候选选择、发送展开、历史渲染和独立进程重启。
它不请求外部供应商，不证明系统输入法或暂停中的视觉一致性验收。

2026-10-30 原生引用验证记录（对应当时用户资源范围，不证明新增插件候选/运行路径）：

- 上方固定方向与覆盖修正：
  `output/prompt-picker-placement/20261030-102836-1716803787/evidence.json`。
  原生目标/待办数据为预置显示夹具；验证全局浮层的遮挡和点击，不作为目标执行证据。
- 最终引用专项：`output/prompt-references-final/20261030-101729-3187226424/evidence.json`。
  深浅主题截图包含输入框中的“你好 Init Goal”，以及技能候选、发送消息和重启恢复。
- 现有 7 项原生用例回归：
  `output/prompt-references-regression/20261030-101133-2160316665/evidence.json`。
- 全仓测试、无 CGO 全仓测试、Go vet、MyGo vet 和框架来源检查通过；
  最后修改后的标签转义、正文保留、相邻引用、Goal 原文及资源校验专项再次通过。
- 原生窗口使用 accessibility 的 setValue/insertText/press 操作；
  键盘选择、撤销和输入法确认使用 `ui.Tester`。未将后台窗口操作声称为 OS 键盘验证。
