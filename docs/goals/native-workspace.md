# /goal：正式包原生工作区交互

状态：本目标限定范围已完成。责任：root。完整 GCode 根目标仍执行中，用户视觉认可仍 pending。

目标：在当前 Intel Mac 的正式 GCode.app 隔离身份副本中，使用唯一 QA Git 项目验证文件浏览、子目录、二进制提示、真实 Diff、TextEdit 编辑保存和终端输入/resize/退出清理。使用已存在的产品界面，不新增管理页面，不改视觉候选。

完成判据：

- 正式包与隔离副本的实际可执行代码和 Resources 一致；源码、原正式包和用户数据不变。
- 文件与 Diff 来自真实临时 Git 仓库，TextEdit 只操作唯一 QA 文件；刷新后内容一致。
- 通过 GCode 原生窗口输入终端命令，记录 cwd、shell PID、真实尺寸和退出/应用退出后的资源清理。
- 保存原生 AX、截图、Git 状态及可重放驱动；按实际结果区分通过与未验证。
- 不访问真实模型凭据、不发 API 请求、不输出用户剪贴板或其他编辑器文档。

所有权：`docs/verification/scripts/native-workspace-qa.py`、`docs/verification/native-workspace.md`、`output/acceptance/native-workspace/`。根目标、用户视觉认可、物理键盘/IME 和其他设备仍分别验收。

完成证据：`native-workspace/final/` 通过文件、Git 与 TextEdit 链路；`native-terminal/final/` 通过实际 AX Terminal 焦点、原生合成键输入/cwd、1440/1280 窗口到 PTY resize、Shift+PageUp/PageDown、shell exit、应用菜单退出清理 shell/child PID，以及 shell 修改同一文件后的预览/Diff 刷新。root 已实际打开检查滚动与 Diff 图、核对截图 hash 和全部记录 PID 消失；源包和源码未变，原输入法恢复。

本目标不宣称 wheel/触控板、系统剪贴板、物理键盘、中文 IME、真实 API 或其他设备通过。没有继续主观视觉打磨或扩展管理页面。
