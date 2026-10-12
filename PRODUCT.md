# GCode 产品事实

<!-- impeccable:product-schema 1 -->

## Platform

Go + mygo 原生桌面，macOS 优先，分别验收 Intel 与 Apple Silicon，随后 Windows。没有 iOS、Android 或 Linux 产品目标。

## Users

首版面向用户本人在本地真实代码仓库中持续编码，配合外部编辑器工作。

## Product Purpose

统一管理项目、编码会话、模型服务、工具过程、文件变更和终端。完整首版范围和行为以 [已确认设计](docs/design/gcode-desktop.md) 为准。

## Operating Context

中文三栏桌面工作台。同目录允许多个主会话并行；worktree 默认关闭，可选开启。关闭窗口继续任务，显式退出停止资源。文件预览只读，手动编辑在外部编辑器完成。

## Capabilities and Constraints

- 将 zero 当前源码一次性接管，删除 CLI/TUI 专用宿主和模型订阅登录；独立维护。
- 应用界面使用 Go + mygo，不使用 HTML、JavaScript、WebView 或 ACP 作为 UI 内部桥接。
- 通过 API Key 对接 Responses、Chat Completions、Anthropic Messages、Gemini；每会话独立选择模型。
- 默认受控权限，可单会话完全访问。保留 MCP、Skills、Hooks、插件、子代理和标准 MCP OAuth。
- 默认发送方式为“纠正”：停止完成后发送。另有 FIFO“队列”，默认方式在设置中持久化。
- 重启保留历史和队列，不自动重跑中断任务。

## Brand Commitments

名称 GCode；中文、直接、清楚的界面语言。2026-10-12 用户明确拒绝初始样板；其 [T3 暗黑截图](docs/design/references/t3-dark-user-reference.png) 是最低视觉质量标准，T3/Droppy 的样式和布局是新参考。技术栈不随参考项目改变。

## Evidence on Hand

已有运行时测试、原生生命周期证据及终端 PTY 证据；详见 docs/verification。视觉样例必须注明示例，不能替代真实编码验收。真实厂商 API、Apple Silicon 与 Windows 实机尚未全量验收。

## Product Principles

先交付端到端可用产品，再扩展管理能力。保持 UI、会话控制、Agent 运行时和厂商协议职责分离。采用现有依赖和成熟库，移除废弃路径，不建设向后兼容层。用户提供的实际界面反馈优先于内部自评。
