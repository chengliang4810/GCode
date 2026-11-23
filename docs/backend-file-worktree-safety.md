# 文件、工作树与目录浏览保护验收

## 本轮范围（2026-11-19）

只调整四项后端行为及必要的现有目录提示，不迁移已有工作树、不增加独立工作树目录设置，
不恢复暂停的 UI 视觉优化。本机桌面继续通过强类型 Go 接口直连核心；HTTP 仅用于真实远程边界。

### 1. 编辑器安全保存

- 同一父目录内排他创建临时文件，完整写入、设置原文件权限位、Sync、Close，最后替换目标。
- 保存前及提交前复核身份、权限、大小、修改时间与内容；父目录替换、链接改向、目标变化时拒绝提交。
- 路径限定在项目根内；末端符号链接、多硬链接、非普通文件及超过 16 MiB 的内容明确拒绝。
- 故障注入覆盖短写、写入、Chmod、Sync、Close、提交前与重命名失败，核对原文件与临时文件清理。
- 并发读者测试核对 Unix 上只读到完整旧版或完整新版；保留既有编辑器版本和加载状态语义。

限制：最终复核与替换不是跨进程 CAS，不能保证外部进程在两步之间写入时绝对不被覆盖。
保留的是权限位，不保证 owner、ACL、扩展属性；没有承诺目录项断电持久性。
Go 不保证非 Unix 平台 Rename 的原子性；Windows 仅做交叉编译，不作为实机原子保存证据。

### 2. 新工作树防嵌套

- 创建目录、分支前，解析数据根与实际源 checkout，按目录身份逐祖先检查目标分组目录。
- 目标与源 checkout 相同或位于其内部时拒绝；覆盖符号链接、子目录及大小写路径别名。
- 拒绝后不产生目录、分支、工作树登记或源文件变动。兄弟目录、外部目录仍允许。
- 已登记工作树的 current/reuse 不受新建布局检查影响，不搬家、不改路径。
- Git common-dir 可以独立于 checkout 存放，不能把其父目录误当成工作目录边界。

### 3. 有界目录浏览

- 目录与 Home/磁盘卷查询共用进程级 4 个 worker，调用方默认最多等待 5 秒。
- Home、路径解析、Open、ReadDir、Close、卷查询均在获准 worker 中运行，不占核心视图锁。
- 每批最多 128 条，总扫描最多 10,000 条，结果最多 500 项；返回子集按路径排序。
- 无法确认完整时返回类型化 `Truncated`，不把扫描上限后的未知路径误报为不存在。
- 桌面沿用现有提示区域，显示部分列表；保留选择现有项、导航和直接输入完整路径。
  中文、英文、深浅主题及 560/1320 宽度由组件测试覆盖，没有新增视觉规格。
- 真实远程边界用 HTTP 504 表示宿主查询超时、503 表示目录 worker 饱和；客户端恢复
  `context.DeadlineExceeded` / `contracts.ErrDirectoryBrowseBusy` 的 `errors.Is` 语义。
  映射限定目录两个端点，不把其他业务的 HTTP 错误误判为文件系统故障。

限制：操作系统中的永久阻塞无法强制取消。超时只结束调用方等待，真实 worker 返回前
一直占名额；满额立即报告繁忙，不会无限遗弃 worker。返回子集并非全目录排序后的前 500 项。
测试中的挂起来自可控依赖，不是实际断开的网络挂载或异常磁盘实验。

### 4. 每轮执行刷新 Git 来源

- 首轮、追问、排队实际启动、目标续跑和子智能体统一经过启动边界，在已有任务 worker
  中、模型和工具执行前，按实际 CWD 观测分支与 HEAD；Git context 预算为 3 秒。
- 成功后仅持久化该会话的分支和来源，发布类型化事件，不切分支、不改变 checkout 选择。
- 探测失败或部分失败保留旧值及旧观察时间；detached HEAD 和 unborn branch 按 Git
  明确的正常状态处理，不把任意命令失败当成它们。
- 取消、关闭、任务替换、会话/CWD/工作树变化及更新观察之后到达的旧结果都丢弃。
  持久化失败恢复旧观察，不取消已接受任务。
- 真实临时 Git + 本地 HTTP 模型 fixture 覆盖初轮、外部切分支并提交后的追问、排队实际
  启动以及重开持久化；确定性测试覆盖过期结果、失败与截止时间。

限制：观察不是跨进程 Git 事务；观察完成之后外部 Git 仍可能改变 checkout。

## 实际验证

以下通过记录来自本轮 macOS 本机，不等于全仓库或跨平台实机验收。

| 检查 | 结果及证据 |
| --- | --- |
| 四项核心竞态回归 | `go test -race ./internal/engine -run 'Test(FileSave\|RunSource\|Worktree\|GitRefs\|Director\|EngineDirectory\|Location)' -count=1 -timeout=5m`，通过（161.506s） |
| 启动边界周边回归 | `go test ./internal/engine -run 'Test(Queue\|Goal\|Subagent\|Naming\|EngineExecutes\|EngineCancel\|FileCommands\|ProjectFiles)' -count=1 -timeout=5m`，通过（279.528s） |
| 目录最终竞态回归 | 核心/mock/remote 的 `Test(Directory\|EngineDirectory\|RemoteDirectory\|WindowsVolume)`，`-race -count=3 -timeout=2m`，通过（6.693s / 1.353s / 7.363s）；含宿主超时、繁忙错误往返和其他路由隔离 |
| 契约、mock、remote 完整包 | `go test ./internal/contracts ./internal/mock ./internal/remote -count=1 -timeout=4m`，通过；含真实临时文件系统目录查询及协议 fixture |
| 桌面共享控件及项目选择器 | `Test(SettingsControls\|InspectSettingsControls\|ProjectPicker\|NewProject\|SidebarNewProject\|Catalog\|Translation\|Chinese)`，通过（20.915s）；含中文提示和深浅主题窄窗 |
| 本地化完整包 | `go test ./internal/i18n -count=1`，通过（0.388s） |
| 静态检查 | `go vet ./internal/engine ./internal/contracts ./internal/mock ./internal/remote ./internal/i18n`，通过 |
| 无桌面服务 | Linux amd64、`CGO_ENABLED=0` 的 `cmd/gcode-server` 交叉构建通过，不引入图形会话依赖 |
| Windows 引擎 | Windows amd64、`CGO_ENABLED=0` 的引擎测试二进制交叉编译通过，未运行 |
| 原生桌面工作树流程 | `go run ./scripts/desktop-e2e -run '^TestDesktopLocalNamingAndWorktree$' -timeout 4m`，1 项通过（6.33s） |

原生证据：`output/desktop-e2e/20261119-233432-3776206959/evidence.json`。
该流程使用独立原生窗口、数据目录、socket、临时真实 Git 仓库及本机真实核心；模型回复
来自本地 HTTP fixture。mock、协议 fixture 和原生流程的证据不得互相替代。

完整 engine 包的一次尝试在三分钟套件截止时尚未完成，当时运行到
`TestGoalSelectedCommandPosition`；本轮按影响范围的回归已通过，但不宣称完整核心套件通过，
也不据截止时正在运行的用例判断根因。之前八分钟完整套件未完成的历史记录仍保留。

未验收：真实模型供应商、外部远程机器或手机、Windows/Linux 实机、异常网络文件系统、
断电持久性、OS 输入法、WebView 像素与 UI 视觉一致性。交叉编译只证明构建，不证明运行行为。
