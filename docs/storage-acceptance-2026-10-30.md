# 存储重构验收

本轮范围是运行数据存储；Dynamic Workflow 调查与方案不等同于 Workflow 已实现。
配置入口、会话绑定、多根解析、工作区及保留策略见 [会话存储](session-storage.md)。
测试均使用独立目录和自有进程，没有迁移、清理个人 `~/.gcode` 或停止用户应用。

## 行为与证据

- 切换只登记根身份，重启启用；配置入口不搬迁，不复制旧数据。
- 历史会话及续聊、执行与目标记录、工具改动、附件继续使用所属根。
- 独立无项目会话使用随机 12 位小写字母/数字目录，原子占位防碰撞。
  侧聊和 fork 共享原工作区，子 Agent 使用父根；删除会话不递归删除共享工作文件。
- worktree 统一位于创建时运行根的 `worktrees/<短ID>`，移除自定义位置开关和字段；
  复用已有 Git worktree 保留真实路径。
- 插件设置和安装包保留配置入口；运行数据和会话环境文件在会话绑定根。
- 对话正文先写不可变摘要文件，再提交固定索引。失败保留原提交，成功仅收集同会话
  的旧正文，不累计完整历史副本。离线历史根不阻塞在线根的命令或退出保存。
- 不可用根、身份变化和正文损坏明确报错；重试恢复正文并修复中断状态。
  目录逐层拒绝链接，防止不同会话别名及交叉清理。
- 独立终端输出单独分片；写失败保留“不完整”标记，重启不会冒充空或完整历史。
- 日志、原生缓存/临时、下载、Windows WebView2 和子进程临时目录已接入。
  远程宿主与远程桌面本机各自拥有自己的运行根。

Windows 原生最终测试通过 5 项，使用 MyGo automation、真实进程内核心、临时 Git
仓库和本地 HTTP 模型夹具：

`output/storage-native-final/20261030-182931-3147670004/evidence.json`

用例为 `TestDesktopLocalSettingsPersistence`、
`TestDesktopLocalNamingAndWorktree`、
`TestDesktopLocalStorageSwitchAndContinuation`、
`TestDesktopLocalSideChat`、`TestDesktopLocalSmartModelConfiguration`，
后两项含深浅主题。目录选择、重启、旧上下文进入续聊
请求、旧根实际写入、新根独立工作区与日志、真实 Git worktree 均有业务断言。
待切换页和历史续聊截图已检查。首轮按钮缺少稳定 key，修正后通过；首轮失败证据保留。
这些结果不代表真实外部供应商、macOS/Linux 原生窗口或视觉对齐验收。

`go test ./... -p 2 -timeout 3m`、`go vet ./...`、`go tool mygo vet .`、
共享组件门禁、框架源码完整指纹检查均通过；CGO_ENABLED 为 0。
MyGo 为 v0.6.4 + rcode-native-ui.17，验证 1892 文件。
后续目录安全小修有对应定向复验。Linux 无桌面入口交叉编译通过，依赖列表没有 MyGo。
WSL Debian 实际运行引擎存储与终端回归，以及真实 HTTP 宿主认证/断线续接和附件测试。

Go 回归包括多根重启、身份/摘要校验、历史附件、错误回滚、索引提交失败、
晚加载后失败不丢历史、正文与配置分离、版本收集、离线根最终保存隔离、
终端输出保存失败重启、共享控件边界及子进程环境。
Windows 无 symlink 创建权限的分支明确跳过；Linux WSL 补跑相同引擎测试，
检查外部和同根内部链接，不以交叉编译冒充运行结果。

## 边界

- 切换不释放旧盘空间；旧会话原位续聊仍会增加旧根数据，这是明确保留的行为。
- 安全清理仅覆盖活动根的应用日志和指定缓存/临时命名空间。
  会话、工作文件、下载、浏览器及可能被后台进程持有的 `tmp/processes` 不自动清理。
- TEMP/TMP/TMPDIR 是子进程默认值；用户显式路径、profile、MCP env 和外部工具自管
  缓存可以覆盖。该设置不是操作系统沙箱。
- macOS/Linux WebView 网站数据仍由平台默认仓库管理；Linux 的字体等框架缓存、
  Windows 有 32 MiB 上限的 shader cache 仍存在系统缓存路径。
- 自动更新最终替换需同文件系统，安装旁暂存仍保留；正常路径清理，崩溃残留
  不在运行根清理范围。
- WSL 的 Git 为 2.30.2，不支持现有核心要求的 `worktree list --porcelain -z`；
  未把该环境记作 Linux 完整 Git worktree 验收。Windows 真实 Git 创建、复用、
  重启与失败清理已通过；Linux 链接防护通过纯目录句柄回归验证。
- 开发阶段存储格式已更新，不添加旧格式迁移、别名或双格式读写；遇到旧格式明确失败，
  不覆盖原数据。

## 文档与代码校正

- 当前后端说明从单文件完整状态改为固定配置索引与会话正文分片；
  初稿 `sessions/<ID>/state.json` 已按最终内容摘要实现修正。
- “目录切换复制/重定位旧数据”的说明改为无复制、历史原位续聊。
- 删除当前设置和契约中的自定义 worktree 位置说明；Git ref 的真实 WorktreePath
  仍属当前业务数据，保留。
- 模型目录缓存说明及 E2E 夹具从根目录文件改为 `cache/gcode`。
- 原生待切换文案、插件运行目录说明、MyGo 补丁数量从 21 改为 22。
- `ui-fixture-contracts.md` 中旧资源引用、`state.json`、自定义 worktree 字段以及
  `machine-handoff.md` 内旧 UI 状态路径均已明确标为历史夹具/验收证据，保留原文；
  它们不描述生产核心。`desktop-testing.md` 过去创建根目录模型缓存的记录也是当时
  已发生的历史证据，不改写为本轮新路径。
