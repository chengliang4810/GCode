# 插件格式与本地安装库

`internal/plugins` 独立实现 Claude 插件文件格式的读取和受管安装。该包不启动插件命令、
Hook、MCP 或 LSP 进程；业务宿主通过类型化 `Bundle` 使用已加载的定义。

## 入口

- `Load(root)`：读取本地插件目录。
- `LoadInstallation(installation)`：按照安装时冻结的来源与市场条目加载组件。
- `LoadWithManifest(root, entryManifest, source)`：允许市场条目补充组件声明，或在没有
  `.claude-plugin/plugin.json` 时提供 manifest。
- `MaterializePluginSource(ctx, sourceTree, pluginRelative, target)`：从完整市场源树
  复制插件；`SourceTree{Root, Links}` 的 `Root` 是源树目录，`Links` 可提供归档中的
  符号链接元数据，键为规范化的 slash 相对路径、值为原始目标。
  普通文件系统来源使用 `SourceTree{Root: root}`。`pluginRelative` 是树内的相对目录，市场根本身使用 `.`。
  调用方必须保留完整源树，不能先截取插件子目录后再传入。
- `ExportGitSource(ctx, source, target)`：拉取并核对固定提交，不执行 worktree checkout；
  将完整 Git archive 导出为普通文件与 `SourceTree.Links`，返回源树和实际 commit。
- `NewManager(root)`：打开受管插件目录。首次创建只登记两个默认市场，不联网。
- `Sources`、`Catalogs`、`Installed`：返回独立的内存快照，不等待网络刷新。
- `AddSource(ctx, location)`：识别 GitHub 仓库、Git URL、清单文件或目录，读取清单中的名称，
  校验成功后一次提交来源与目录；无效、重复或已取消的添加不发布部分登记。
- `PutSource`、`RemoveSource`、`Refresh`：维护市场来源和目录快照。
- `Install`、`SetEnabled`、`Update`、`Uninstall`：维护插件安装记录。
- `Bundles()`：返回宿主用户级已启用且依赖可用的插件，各项目使用同一组插件。

安装身份为 `plugin@marketplace`，同一身份只有一条用户级安装记录。
安装操作不接受作用域或项目路径；启停、更新、卸载与配置都直接按安装身份执行。
`Update` 使用已经刷新的目录，调用方先对来源执行 `Refresh` 才能发现远端更新。

默认来源：

| ID | 市场 | GitHub 仓库 |
| --- | --- | --- |
| `official` | `claude-plugins-official` | `anthropics/claude-plugins-official` |
| `community` | `claude-community` | `anthropics/claude-plugins-community` |

## 格式行为

技能目录 `skills/` 与 manifest `skills` 路径相加。市场根目录来源的 entry
若声明 `skills`，只加载该条目显式指定的目录；安装记录保留原来源以判定此规则。
市场条目的 Hook 按事件覆盖插件 manifest 中同事件的定义。
`commands`、`agents`、
`outputStyles` 一旦声明路径，就替换相应的默认目录扫描。路径重复按实际文件去重。
默认 Hook、MCP、LSP 文件与 manifest 声明合并；MCP/LSP 的同名条目由后声明者覆盖。

支持路径、路径数组、命令内联内容和命令元数据覆盖；Hook/MCP/LSP 支持文件与内联
对象及其数组。YAML frontmatter 使用 `go.yaml.in/yaml/v3`，支持多行文本、数组、
布尔值、嵌套 Hook，并保留原始字段供宿主读取 agents 等定义。

Hook 的 `args` 区分省略与空数组：省略选择 shell form，`[]` 选择 exec form。
`command`、`http`、`mcp_tool`、`prompt`、`agent` 的定义均保留。未知 Hook 字段
保留在 `Unknown`，同时记入 `Bundle.Unsupported`，不会静默丢弃。

市场支持 GitHub、Git、HTTP JSON、本地目录、本地 JSON 和内联 settings 来源。
插件来源支持市场内相对路径、GitHub、Git URL、git-subdir，以及仅供本地明确配置
使用的目录来源。Git 安装记录实际 commit；`sha` 固定到完整提交。GitHub、Git URL
和 Git 来源的 `path` 同样选择仓库内插件子目录；`git-subdir` 必须提供 `path`。
HTTP/JSON/内联市场没有可解析的本地插件仓库，必须使用独立插件来源。

`strict:false` 且插件自身有 manifest 时，市场不能再提供组件定义。未提供插件
manifest 时允许市场条目提供定义；`strict` 不用来强制要求 manifest 文件存在。

## 文件与事务边界

市场刷新及插件安装先完成临时目录准备、格式校验，再发布不可变目录和原子替换的
registry。失败保留旧安装记录和版本；更新和卸载不删除旧版本目录，以保证运行中
会话仍持有可读取的路径。受管目录的垃圾回收需要与会话引用管理共同完成，当前库
不自动清理退役版本。

插件安装按链接目标处理符号链接：

- 插件目录内部的目标在缓存中保留为相对符号链接。
- 市场内、插件目录外的目标解引用为普通文件或目录，复制到缓存对应位置。
- 市场外的目标跳过；独立本地目录复制仅保留插件内部目标，其余链接跳过。

加载缓存只允许插件根目录内可解析的链接；循环、悬空链接、指向 `.git` 或逃逸的链接
均拒绝。普通用户资源目录和独立 `ParseMarkdown` 入口的链接限制保持原状。
Windows 缺少创建链接的权限时明确失败，不把链接降级为普通文件或宣称已验证通过。
归档导出可只保存普通文件与 `SourceTree.Links`，不在源缓存创建 OS 链接；物化时只处理
选中插件及其共享依赖，无关仓库链接不触发解析或创建。选中插件内部需要保留的链接仍
必须在最终插件缓存创建真实 OS 链接，因此此边界仍会明确报告 Windows 权限不足。
生产 Git 市场刷新和独立 Git 插件均使用上述 exporter；`Catalog.Links` 随私有 registry
冻结，刷新后的市场安装及宿主重启后继续使用同一份链接表。Git 子目录来源从完整树选择
插件，不会因仓库其他位置的链接提前执行 OS-link checkout 而失败。
符号链接行为已在 Debian/WSL 的真实文件系统上验证；本机 Windows 相关用例因缺少权限
明确跳过，不能据此推断 Windows 链接可用。

拒绝路径逃逸、非普通文件、把目标复制到源目录内部、超过 512 MiB 的复制、
超过 100,000 项或 128 层的树以及超过 64 跳的链接链。定义文件另受读取大小限制。
Git 归档和虚拟链接的元数据另受 16 MiB 限制；归档拒绝硬链接、设备、重复路径及链接父级写入。
源文件读取及目标写入通过受约束的目录句柄进行，不跟随链接读取市场外的内容。
Git 和 HTTP 操作使用调用方 context，具有独立超时，Git 不询问
交互式凭据，也不运行 Git hooks。

`Bundle.Clone` 和 Manager 快照执行 Go 内存复制，不经过本机 UI 传输协议。
`registry.json` 仅是包自己的持久化边界。

## 插件依赖

`dependencies` 支持名称、`name@marketplace` 和带 `name`、`marketplace`、`version`
的对象。安装和更新会准备完整依赖图，环、缺失插件、版本冲突或其中任一安装失败
都不会发布部分安装记录。已经满足约束且启用的依赖会复用。

跨市场自动安装由根市场的 `allowCrossMarketplaceDependenciesOn` 控制；用户级
已经显式启用的目标依赖不要求该列表。Git 依赖约束通过远程 `<name>--v<version>`
标签选择最高满足版本，仅拉取该提交，不克隆完整历史。git-subdir 保留该提交的完整
源树供链接解析，然后只物化指定插件及其引用的同源共享资源；不能在解析前丢弃兄弟目录。
当前支持 exact/partial、比较运算符、`^`、`~`、`x/*` 和 `||`；未实现的范围表达式
返回明确错误。默认排除未显式请求的预发布版本。

禁用或卸载操作若新增启用插件的依赖缺口会被拒绝。`Installed().Issues`、
`DependencyIssues()` 给出缺失、禁用、版本不满足和循环原因；依赖有问题的插件
不会进入 `Bundles()` 的可执行清单。

## 明确保留的边界

包保留 `userConfig`、channels、settings 和不识别的 manifest 字段；
敏感配置存储、Hook执行、MCP连接、LSP进程均由业务宿主实现。
MCPB/DXT、npm/pip 安装器不在本包实现范围，遇到 MCPB/DXT 定义会报告
`Unsupported`；不会把它们计入已可执行的插件。

本地临时市场、HTTP延迟响应及临时 Git 仓库测试证明文件格式和管理事务；
这些测试不代表公开市场中的全部插件、外部服务凭据或真实模型任务已经验证通过。
