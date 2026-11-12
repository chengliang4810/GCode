# 桌面测试

2026-11-13：核心统计持久化后，`TestDesktopLocalConversationMetricsPanels`、
`TestDesktopLocalApplicationUsage` 的深浅主题均通过，统计面板与用量页代表截图已检查。
应用从独立真实核心的持久化数据重开，模型端点为本地 HTTP 夹具；不代表真实供应商或远程宿主验收。
证据：`output/durable-metrics-20261113/native/20261113-012455-212660129/evidence.json`。
设计与验收边界见 [统计持久化](metrics-persistence-2026-11-13.md)。

2026-11-11 MyGo v0.8.3：设备菜单、引用图标对齐、模型选择器、正文/预览链接、
Markdown 表格、设置持久化、面板开合和拖拽关闭共 8 组原生用例通过，
证据为 `output/mygo-upgrade-20261111/native-final/20261111-122519-2869211955/evidence.json`。
相关深浅主题截图已检查。引用布局使用私有核心重开的显示历史夹具；链接使用真实本机文件
和回环 HTTP 模型/网页。自动化不证明 OS 输入法、远程宿主或真实供应商验收。
原有引用发送用例仍有升级前可复现的技能展开问题，边界与对照证据见
[MyGo 升级记录](mygo-upgrade-2026-11-11.md)。

2026-11-11 链接读取错误清理：`TestDesktopLocalTranscriptLinks` 增加先读取二进制文件失败、
再打开有效中文 UTF-8 文本的流程，验证正文显示且旧编码错误消失、中央草稿保持。
深浅主题原生窗口均通过，截图已检查；同文件失败后重试另由 `ui.Tester` 组件回归覆盖。
证据为 `output/linked-file-error-20261111/native/20261111-111144-2295010584/evidence.json`。
本机核心读取真实临时文件，模型回复及网页使用本地 HTTP 夹具，不代表真实供应商验收。

2026-11-11 工具语义展示：`TestDesktopLocalSemanticToolLabels` 在私有本机核心中执行
后台命令、任务读取/停止、目标读取、目录、网页、技能、网页搜索、MCP 和失败委派，
关闭核心后由原生程序重开历史；深浅主题的名称、摘要及详情交互通过。
模型、网页、搜索与 MCP 均使用本地 HTTP 夹具，不代表真实供应商或远程宿主验收。
证据为 `output/tool-labels-20261111/native-final-verified/20261111-102244-3006991953/`，
规则见 [会话工具记录](tool-presentation.md)。
同轮原有 `TestDesktopLocalTranscriptActivity` 在思考预览无障碍文本断言处失败；
截图可见首行，按钮标签不包含预览正文，失败证据保留在该专项的 `native-final/`。

2026-11-05 审计修复后的全仓检查、设置保存、密钥显隐、链接、模型重试与侧聊验收
见 [核验后修复与决策](audit-repairs-2026-11-05.md)。`output/go.mod` 将生成证据隔离为
独立模块，保留其中的中文测试工作区，避免其成为主模块的 Go 导入路径。

桌面退出总时限按用户选择固定为 5 秒；`cmd/rcode/shutdown_test.go` 覆盖精确截止、
重复关闭不延长、正常清理撤销与真实阻塞子进程强退（含日志阻塞）。
当前原生正常关闭、重启及设置保留证据为
`output/shutdown-5s-native-complete/20261105-105234-2600549520/evidence.json`。

真实原生窗口的端到端测试统一使用 MyGo `plugins/automation`。
测试二进制通过 `internal/testenv.Main` 使用物理临时目录，保留业务存储对符号链接的
拒绝规则。POSIX 终端夹具使用 `/bin/sh`，macOS 使用系统支持的 `en_US.UTF-8`
语言环境，避免继承开发者启动脚本或不受支持的 Linux locale。真实终端测试在
提示符出现后才发送输入；这不替代 PTY 输出、中文、输入中断和断线续接断言。

Go 业务核心测试和 `ui.Tester` 组件测试继续保留；端到端测试覆盖真实启动、
跨页面操作及持久化流程，不重复每个组件的全部边界组合。

2026-11-05 目标/待办补齐验证新增工具状态详情、编辑标签关闭重开草稿、
重新载入和恢复执行 ID 隔离。最新 Windows 深浅主题记录为
`output/goal-todo-rcode-audit/20261105-043717-3872042836/evidence.json`；
对照清单与边界见 [RCode 目标/待办复核](rcode-goal-todo-audit.md)。

## 执行

Windows、Linux、macOS 桌面会话中使用同一条命令，仅需已有的 Go 工具链：

```sh
go run ./scripts/desktop-e2e
# 或 make e2e
go run ./scripts/desktop-e2e -run TestDesktopLocalSettingsPersistence
go run ./scripts/desktop-e2e -out-dir output/native-tests -timeout 4m
```

Go 入口按当前系统构建独立的 `e2e` 程序：仅 Windows 使用 `.exe` 后缀和
`-H=windowsgui`，Linux/macOS 使用原生可执行文件，无需 PowerShell 或 Bash 包装。
入口验证框架源码与普通构建的插件隔离，然后运行
`go test -json -tags e2e ./internal/e2e`。
将二进制、构建日志、测试日志及 JSON 事件、完整无障碍树、PNG 和 `evidence.json` 保存在
`output/desktop-e2e/<本次运行>/`。每次运行使用新目录，保留失败记录。
测试窗口会实际显示，需要图形桌面；`--headless` 不属于这条路径。
Windows 下后台 Go 命令、Git 命令和测试子进程默认使用共享
`internal/processutil.HideConsole`，不再另开终端窗口；日志和退出码继续收集。
原生应用窗口保留显示，不需要额外参数。这里的“后台”仅指控制台子进程，
不把窗口隐藏或 `ui.Tester` 渲染计作真实可见窗口验收。
Linux/macOS 的普通子进程本身不创建终端，沿用原生启动行为。
应用自己的交互式终端仍走 ConPTY/PTY，不应用此设置。
Linux 需要可用的 `DISPLAY` 或 `WAYLAND_DISPLAY` 及 MyGo 的系统运行依赖。
Linux CI 可在配置好依赖的 Xvfb 环境中运行
`xvfb-run -a go run ./scripts/desktop-e2e`；macOS 需要已登录的图形会话。
纯 SSH、容器或无桌面服务器环境不能直接作为原生桌面验收环境。
没有匹配或只有跳过的用例会失败，不将“没有执行测试”记录为通过。

直接运行测试时必须设置 `RCODE_E2E_BINARY` 和 `RCODE_E2E_ARTIFACTS`，
分别指向测试程序和证据目录；缺少配置会失败，不会静默跳过。
统一 Go 入口会自动配置这两个环境变量，只传给子进程，不修改当前 shell。
Linux/macOS 支持相同入口和客户端，但尚未完成对应平台的实机验证。

## 规范

- 在 `cmd/rcode/automation_e2e.go` 中按 `e2e` build tag 导入插件。普通构建不导入；
  无桌面服务也不依赖插件。不要将测试 tag 加入发布构建配置。
- 使用稳定、唯一的控件 key；重新查询当前树，不跨帧保存数值节点 ID。
  控件新增 key 应在构造前设置，沿用共享组件，不能为测试修改外观。
- 优先检查控件公开的 action 和状态，通过插件执行操作，断言用户可见结果。
  需要键盘语义时使用 `input`，不能把 `act` 调用宣称为真实系统鼠标或输入法验证。
- 每次启动独立数据目录和 socket，仅管理测试创建的子进程。socket 使用用户目录下
  独立短目录，退出后删除；本机系统 TEMP 下曾出现 AF_UNIX 监听成功但连接失败，
  测试不依赖该目录。业务数据与证据保留在本次输出中。
- 所有连接、请求、启动及业务状态等待均设超时。轮询明确的控件或持久化结果；
  `idle` 只代表 UI 不再请求帧，不能代表后台任务完成。不得用固定等待代替完成条件。
- 失败时尽力保存最终树、截图及应用日志，再清理自有进程。
  不连接用户日常程序，不使用用户真实数据目录或凭据。
- 分别标明 mock、本机核心、远程宿主和真实模型供应商测试。mock 成功不能作为
  真实后端证据；本机持久化成功也不能作为模型请求或远程续接证据。

插件的 socket 仅连接外部测试驱动与测试进程。桌面 UI 与本地业务核心仍通过
强类型 Go 接口直接交互，不引入应用内部传输协议。

持久化断言通过 `persistence_fixture_test.go` 读取配置索引的 `records`，
按会话根登记找到 `sessions/<会话ID>/<SHA256>.json`，校验摘要后重建消息与目标等状态。
不能从 `engine.json.snapshot.messages` 读取正文，也不能向该字段注入夹具。
优先通过核心公开命令准备业务数据；不执行模型的表格、菜单位置和目标遮挡夹具，
仅在私有宿主停止后修改对应会话记录、重算摘要并提交索引，保留历史与未知字段。
模型元数据夹具位于运行根的 `cache/gcode/models-dev.json`。

`go test -tags e2e ./internal/e2e -run '^TestFixture'` 单独验证夹具读写、跨根解析与
真实核心重开，不要求原生测试程序或桌面会话；它不构成原生界面验收。
修改夹具后，受影响的原生用例仍需通过统一入口重新运行。

## 当前覆盖

macOS 设置标题栏侧栏动效修复的原生记录位于
`output/settings-titlebar-motion/20261105-164958-4280240792/`：深浅主题 900×600
连续开合侧栏的标题坐标采样、设置页导航和截图检查通过。避让空间随侧栏宽度连续补齐，
不再等动画结束才恢复；组件回归另按 16ms 推进验证中间位置与最终位置连续。
自动化截图仅记录应用绘制内容，不将系统红绿灯像素或完整视觉一致性记为已验收。

`TestDesktopLocalTodoWrite` 覆盖真实 TodoWrite、底部默认折叠、长列表前后折叠、
失败保留、完成隐藏、重启恢复与关闭状态；空列表边界另由核心及组件测试覆盖。
`TestDesktopLocalGoalTodoStack` 覆盖真实目标和待办的层叠几何、右侧编辑保存、
暂停/恢复/检查完成/清除及消息草稿保留。
二者均使用真实本机 Go 核心、私有 HTTP 模型夹具和独立数据目录，覆盖深浅主题及窄窗；
不计作真实供应商或远程宿主验收。面板参考来源与证据见 [todos.md](todos.md)。

| 用例 | 实际路径 |
| --- | --- |
| `TestDesktopLocalMessageRail` | 原生窗口 + 私有 `--local` 真实核心重开持久化历史夹具；深浅主题、中英文、1320/900 宽度；刻度几何、悬停预览、点击和键盘定位、当前刻度、单轮/窄区隐藏、会话切换与草稿保留；不请求模型。运行状态与证据见 `message-rail.md` |
| `TestDesktopLocalTranscriptScrollbarEdge` | 原生窗口 + 私有 `--local` 核心与持久化历史；深浅主题 1500×800，信息卡开关及右侧工作区打开时滚动视口贴对话面板右缘，右缘滚轮、返回最新与草稿保留；不请求模型 |
| `TestDesktopLocalProjectCreationWithIgnoredArtifacts` | 原生项目选择器添加实际 GCode checkout；独立进程、配置和 socket；验证登记成功、后台文件列表完成、未跟踪验收缓存被忽略及重启后项目仍可见；无模型调用。最新证据 `output/project-create-async-native/20261030-200639-2897047696/evidence.json` |
| `TestDesktopLocalTranscriptActivity` | 深浅主题原生窗口 + `--local` 真实核心；实际读取 80 行文件、持久化恢复、思考折叠预览、思考与输出最多 24 行可视高度及内部滚到底、外层位置不变、完整参数、提问去重与回答后续执行；模型为本地 HTTP 夹具 |
| `TestDesktopLocalQuestionComposer` | 独立原生窗口 + `--local` 真实核心；英文浅色 1320×880、中文深色 900×600；提问替换底部输入框、单选自动前进、分页返回修改、多选及自定义文字优先、整组答案落盘和模型续执行、普通草稿恢复、历史无交互选项；模型为本地 HTTP 夹具 |
| `TestDesktopLocalTranscriptFoldMotion` | 深浅主题原生窗口 + 真实本机核心；工具组、思考及读取详情开闭和快速反向点击的连续几何采样、24 行内部滚动、减少动态效果、草稿及核心重开保持；模型为本地 HTTP 夹具 |
| `TestDesktopScenarioSettings` | 原生窗口 + 确定性场景；设置导航、搜索输入、开关状态、外观页面 |
| `TestDesktopSettingsTitlebarSidebarMotion` | 原生窗口 + 私有 `--local` 核心；深浅主题 900×600，在设置页连续收起/展开侧栏，采样标题栏标题坐标，验证动画中始终避让平台窗口按钮并保存几何及截图；不请求模型，不代表视觉一致性验收 |
| `TestDesktopLocalSettingsPersistence` | 原生窗口 + `--local` 真实核心；修改设置、检查落盘、独立进程重启后验证 |
| `TestDesktopLocalStorageSwitchAndContinuation` | 原生窗口 + `--local` 真实核心；目录切换不复制、重启启用、旧上下文进入续聊请求且正文仍写旧根、新会话独立工作区与日志写新根；模型为本地 HTTP 夹具 |
| `TestDesktopLocalMarkdownTables` | 原生窗口 + 私有 `--local` 真实核心；预置两张表，深浅主题/中英文、900×600/1320×880，独立工具栏、可见区滚动、预览固定表头、横向滚动、展开宽度和输入框避让；无供应商请求；CSV 内容及文件写入另由组件回归验证 |
| `TestDesktopLocalSmartModelConfiguration` | 原生窗口 + `--local` 真实核心；900×600、中英文、深浅主题；默认值、快速修改 ID、缓存填充、七档推理勾选、手动覆盖落盘、重开保留及重置；查询不重写缓存、不请求模型 |
| `TestDesktopLocalModelIconActions` | 原生窗口 + `--local` 真实核心；模型行图标测试、编辑与删除，使用本地 HTTP 响应夹具并检查落盘 |
| `TestDesktopLocalModelPickerLayout` | 原生窗口 + `--local` 真实核心；900×600、中英文、深浅主题及 2/12 模型目录，验证新建页与带消息会话在不同输入高度下均与模型按钮保持 6 像素间隔、上方双列菜单、悬停供应商筛选模型且无重复名称提示、底部模型管理、大小目录搜索策略、选中后关闭、无紧凑开关或收藏、单模型编辑保存及独立思考强度落盘；不请求模型 |
| `TestDesktopLocalSameNameProviderSwitch` | 原生窗口 + `--local` 真实核心；中文深浅主题，两个本地 HTTP 供应商提供相同模型名，首家 503 耗尽重试后重开，直接查看脱敏原始 JSON 错误，记录当前模型、切换供应商后未悬停、悬停及移走指针时的模型行外观，指针点击切到第二家并实际收到成功回复，悬停确认供应商身份；再次重启后检索轨迹中的原始错误码、正文与脱敏凭据，不访问真实供应商 |
| `TestDesktopLocalModelRetry` | 原生窗口 + `--local` 真实核心；中文深浅主题 900×600，本地 HTTP 503/Retry-After 夹具；前两次普通加载、第 3 次显示 3/10、真实指针悬浮 HTTP 503 提示、成功/取消清除与逐次请求计量；不访问真实供应商 |
| `TestDesktopLocalProjectConversationReorder` | 原生窗口 + 私有 `--local` 真实核心；中文浅色 1320/深色 900 宽度；指针拖到项目内首位、末位，核对界面与落盘顺序，独立进程重启后顺序保留；不请求模型，不使用用户 profile |
| `TestDesktopLocalCustomSubagentSettings` | 原生窗口 + `--local` 真实核心；创建自定义子智能体、取消草稿、禁用、重启读回、确认删除；不请求模型 |
| `TestDesktopLocalPluginManagement` | 原生窗口 + `--local` 真实核心；私有临时 Git 市场、市场源添加/刷新、用户级安装、独立市场菜单、来源 tab 与管理弹窗、配置取消/保存/清除、真实 Git 更新、禁用、独立进程重启及卸载；不访问公开市场或模型 |
| `TestDesktopLocalSidebarFooter` | 原生窗口 + `--local` 真实核心；中英文/深浅主题、默认与拖宽至 400 DIP 的侧栏下验证底栏顺序、固定 8 DIP 间距及靠右排列；常驻检查更新、移除 Git/PR、手机弹窗开关、会话页返回设置、用量设置页导航/筛选；确认通用页已移除更新入口，关于页提供 GitHub/Star 按钮并能打开原生更新窗口；不访问供应商或更新源 |
| `TestDesktopLocalCallTrajectory` | 原生窗口 + `--local` 真实核心、本地 HTTP 模型与真实 Read 工具；重启读取请求/响应/工具结果/HTTP 错误，验证右侧 Tab、搜索与刷新。系统右键菜单由 `ui.Tester` 验证，原生通过生产路由打开；见 `call-trajectory.md` |
| `TestDesktopLocalApplicationUsage` | 原生窗口 + `--local` 真实核心；本地 HTTP 模型夹具实际请求两个模型、切换模型、分叉并重启，悬停对话与回复用量/用时入口打开面板，验证累计用量、缓存命中率、持久化计时、历史供应商/模型及窄窗几何，省略缓存字段的请求不稀释命中率，面板不显示计数覆盖；同时覆盖用量页去重、模型各占 50%、7/30 天、热力图与刷新；英文浅色和中文深色窄窗，不访问真实供应商，统计口径见 `conversation-metrics.md` |
| `TestDesktopLocalConversationMetricsPanels` | 原生窗口 + `--local` 真实核心；七次本地 HTTP 模型请求、六次真实 Read，重开后检查完整用量与用时面板、34.0% 缓存命中、实测总用时与面板内实测生成用时和“复制 → 用量 → 用时 → 日期”顺序、首字计时、面板边界与 Escape；真实指针悬停打开、移入保持、切换与移出关闭；确认计数覆盖行及速度口径说明已移除；中文深浅主题，不请求真实供应商 |
| `TestDesktopLocalConversationMetricsVisibility` | 原生窗口 + `--local` 真实核心；失败无用量时隐藏统计，首次有效数据出现后按已上报数值显示、不加 `≥` 或说明文字；上下文入口从首条用户消息起只显示固定宽度圆环，实际指针悬停打开完整容量/计数/百分比面板，移入保持且 Escape 关闭；深浅主题，本地 HTTP 夹具 |
| `TestDesktopLocalProjectNewConversation` | 原生窗口 + `--local` 真实核心；深浅主题、264/350px 侧栏，悬停项目行并通过指针事件点击独立新建按钮，核对画布项目和折叠隔离；悬停画布项目名显示圆形关闭按钮，指针点击后显示不使用项目并保留草稿，再选项目后发送，真实 Read 目标项目文件、核对持久化归属及工作目录，返回旧对话保留草稿；模型为本地 HTTP 夹具 |
| `TestDesktopLocalRepositoryRefsRefresh` | 原生窗口 + `--local` 真实核心与私有真实 Git 仓库；旧 Ready 缓存重开刷新、打开列表保留已有分支并异步显示外部新增分支、新建草稿刷新、刷新完成后再次外部切换分支仍能发送；核对会话实际分支、source context 和真实 Read 结果；深浅主题，模型为本地 HTTP 夹具；队列等待/加载/失败期间缓存显示、搜索、游标按名称保持及延后选择由组件回归确定性验证 |
| `TestDesktopLocalPromptReferences` | 原生窗口 + `--local` 真实核心；深浅主题窄窗的命令/技能候选、原子引用、展开发送、持久化及重启恢复；模型为本地 HTTP 夹具 |
| `TestDesktopPromptPickerOverGoalTodos` | 原生窗口 + `--local` 真实核心；使用预置的暂停目标和待办显示数据，验证菜单始终位于上方、覆盖重叠内容并可点击；不执行目标或请求模型 |
| `TestDesktopWindowCaptionControls` | 原生窗口 + 确定性场景；窗口隐藏系统按钮后自绘的最小化/最大化/关闭存在，28×28 且与左侧 Files、面板开关同中心线、4px 节奏、右缘 6px 内缩；按下最大化后中键切换为还原图形并可恢复。证据 `output/native-tests/20261101-023437-2195741834/` |
| `TestDesktopLocalSideChat` | 原生窗口 + `--local` 真实核心；900×600、浅色英文/深色中文；右侧空态创建子会话、父上下文进入模型请求而侧聊时间线空白、独立发送与父草稿/历史保护；模型为本地 HTTP 夹具 |
| `TestDesktopLocalTranscriptLinks` | 原生窗口 + 真实本机核心、真实临时文件与本地 HTTP 模型/网页；深浅主题下的空白 Browser、Markdown 默认预览、相对链接、Unicode 行列定位、PNG/SVG、网页导航及中央草稿/会话保持 |

2026-10-30：存储最终原生证据为
`output/storage-native-final/20261030-182931-3147670004/evidence.json`。
存储切换、设置重启、真实 Git worktree、深浅主题侧聊与模型缓存共 5 项通过。
配置与正文分片、故障回滚、离线根与 Linux 检查边界见
[存储验收](storage-acceptance-2026-10-30.md)。

2026-10-30：工具折叠专项证据为
`output/fold-motion-native/20261030-175804-1229908973/evidence.json`。
正常动效两主题共 14 组操作的底部正文采样位移均为 0 DIP；
另覆盖减少动态效果和内部滚动。此轮仅隔离了引用已删除字段的旧
`features_test.go`，未覆盖生产代码，因此不是整个 E2E 包通过。
专项组件、共享控件边界、Go vet、MyGo vet、普通构建和源码锁在此轮实现时通过；
18:07 复核被随后终端存储改动的两个未定义方法阻断，全仓另有范围外失败。
完整条件、输入和前后对照见
[tool-fold-motion.md](tool-fold-motion.md)。

2026-10-30：插件管理原生证据为
`output/plugin-settings-final-native/20261030-191614-29241047/evidence.json`。
真实私有 Git 市场的安装、更新、配置、禁用、重启和卸载通过，
浅色正常窗口与深色 900×600 截图已检查。
业务状态通过落盘与可访问控件断言，截图前等待动画/绘制结束，避免像素仍处于上一帧。
前次长目录下 Git pack 路径失败保留在
`output/plugin-settings-final-native/20261030-191200-2379312586/evidence.json`；
修复每命令 Git 长路径设置和临时 fetch 仓库后，以上最新完整原生流程通过。
这不是公开市场逐插件运行或真实供应商验收。

2026-10-30：链接接入最终原生证据位于
`output/transcript-links-committed/20261030-170830-1054252519/evidence.json`。
深浅主题均通过；空白浏览器、文件预览、实际代码定位与图片实窗截图已检查。
WebView 导航由当前节点、页面标题和本地页面服务器请求共同验证，
没有把宿主捕获中的 WebView 区域当作网页像素证据。
早期草稿失败来自测试未等待异步会话切换完成，即向旧页面复用的 input key 输入；
最终用例等目标会话标题和真实正文链接出现，再等发送动作可用以确认业务草稿提交，
所有打开链接后的草稿等值断言均保留。
`TestDesktopLocalTranscriptActivity` 的深浅主题同时复验通过，日志位于
`output/transcript-links-final/20261030-170605-1312626428/`；
该次整批运行因修正前的链接测试前置等待失败，不能把整批状态标为通过。
全仓 Go 测试、Go vet、MyGo vet、源码锁和共享控件检查通过；
Linux amd64 无桌面服务交叉编译通过。
路径、远程认证/文件事件、图片缓存并发发布、过期图片结果、快照事件顺序和复制
另有核心/远程/组件回归。Windows 相对符号链接逃逸用例因账户缺少创建权限跳过。
未请求真实外部供应商或运行远程实机 UI；复制精度及 MyGo 既存失败边界见
[transcript-links.md](transcript-links.md)。

2026-10-30：工具工作记录的最终原生证据为
`output/transcript-activity/20261030-162651-3743064682/evidence.json`，
深浅主题均通过，已检查实窗截图。组件回归另覆盖流式未换行预览、
手动展开跨完成事件保持、无卡片的失败提问错误、窄宽/深浅主题/字体、
完整长行、差异与输出同视口、Home/End 以及跨行复制。
首次运行的浅色等待失败来自测试把 `input.requested` 错当成落盘边界；
最终用例按原生问题卡片等待，回答完成后再核对落盘，没有放宽生产行为。
未请求真实外部供应商，未验证远程实机或系统输入法，不作为视觉一致性验收。

2026-10-30：侧聊上下文及入口专项的原生证据位于
`output/side-chat-context-native/20261030-155932-3289309110/evidence.json`。
两种主题/语言均验证了真实本地核心中的创建、持久化、继承上下文和独立发送，
实窗截图已检查。`+` 菜单连续多开及草稿隔离由组件测试覆盖；
三协议、运行中历史边界、重启和压缩后的重试约束由核心测试覆盖。
未请求真实外部供应商，未验证远程实机或系统输入法，不作为视觉一致性验收。

2026-10-30：会话 GFM 表格增加共享工具栏、Markdown 复制、CSV 导出、宽度展开和原生预览。
主区长表横向滚动条避让输入框；嵌套子会话禁用主区借位与固定滚动条。
`TestDesktopLocalMarkdownTables` 使用真实私有核心与预置展示回复，完成两表独立操作、
窄/宽窗口、深浅主题、中英文、固定预览表头、实际滚动及扩宽后滚动条边界验证。
原生证据为 `output/markdown-tables-verified/20261030-160912-951449189/evidence.json`。
组件证据目录 `output/markdown-tables/component` 另验证复制、CSV BOM/引号/公式前缀、
临时文件实际写入、流式追加及选择器与公式钩子共存；系统保存窗口本身未自动化验收。
实现和参考源码结论见 [Markdown 表格](markdown-tables.md)。

2026-10-30：智能配置改为始终自动执行，移除开关、加载文案及匹配提示。
同优先级的候选按供应商 ID、模型键排序，自动应用第一个；未匹配时保留表单值。
`TestLookupUsesLocalFileAndSelectsStableFirstMatch` 连续查询验证首选稳定且零网络请求。
原生用例使用两个不同上下文限制的同名缓存条目，验证自动填充第一项、没有开关和提示，
并保留手动覆盖、保存、重开及重置交互。深浅主题、中英文及模型选择器/行操作全部通过。
证据为 `output/model-auto-fill/20261030-153731-956382442/evidence.json`。
全仓 Go 测试、Go vet、MyGo vet 通过；旧“多匹配提示手动配置”和开关文档已更新。
本地更新记录为 `output/local-update-20261030-153915/result.json`，窗口可见且响应，
五个既有会话与本地目录缓存保留。

2026-10-30：模型表单平铺并改为请求模型 ID，不再保存独立显示名。
上下文/输出初始值为 200000/12800；输入类型及七种推理等级采用共享勾选项，
无高级折叠和模型能力区，底部操作固定显示。当前编辑器打开不覆盖已保存值。
models.dev 查询使用本地文件索引，刷新循环归业务宿主所有；
`TestRefreshLoopRequestsOnceEveryThreeHours` 用 Go 虚拟时间验证两个三小时边界，
其间十次查询不增加请求；失败/无效 JSON 保留文件、条件请求、重启调度、
网络阻塞期间本地查询和取消关闭亦有回归，远程元数据接口验证真实核心与认证。
原生证据为 `output/smart-model-form-final/20261030-152712-1429382102/evidence.json`：
新表单深浅主题、四组模型选择器和模型行图标操作均通过；后者仅使用本地供应商响应夹具。
全仓测试、uifixture、Go vet、MyGo vet 通过，Linux 无桌面服务交叉编译通过。
真实 models.dev 下载复核记录在 `output/smart-model-form-final/live-cache/result.json`：
一次刷新请求成功，随后二十次本地查询不增加请求。先前一次网络下载超时保留了有效缓存，
后续显式复核成功；此记录不代表运行时会绕过三小时刷新间隔自动重试。
本地更新记录为 `output/local-update-20261030-153125/result.json`，
更新后窗口可见且响应，五个既有会话 ID 保留，真实用户目录已创建 `models-dev.json`；
旧程序和离线许可包均有保留/交付记录，没有复制用户配置或凭据到测试目录。
下方“保存显示名”的旧验收属于此前版本；当前相关用例改为编辑上下文并验证不改选中模型。

2026-10-30：当前模型菜单已统一为两列，删除紧凑开关、DTO 字段及首层/返回逻辑。
供应商名称可见、模型管理固定于左列底部，右列小目录不显示搜索框，大目录保留搜索。
组件回归覆盖 360 宽度及 280 高度的菜单夹具；桌面最小窗仍为 900×600。
最终原生证据为
`output/unified-model-picker-final/20261030-120312-1656522899/evidence.json`：
四组模型菜单场景、命令引用和目标/待办覆盖均通过；模型菜单使用真实本地核心，
不请求外部供应商。全仓 Go 测试、Go vet、MyGo vet 和 uifixture 回归通过。
本地便携程序已更新，正常显示并响应，四个既有会话保留；
更新与旧程序备份记录为 `output/local-update-20261030-120433/result.json`。
下列紧凑/非紧凑和顶部设置入口证据仅记录此前版本。

2026-10-30：修正新建页与已有会话的模型菜单距离不一致。
菜单垂直锚点统一为模型按钮，浮层底边与按钮上沿相隔 6 像素，
不再额外偏移到整个输入框顶部；两列顶部对齐和按空间向左/右展开保持不变。
`TestModelMenuTracksButtonAcrossComposerLayouts` 覆盖首页/会话输入框及空/多行草稿；
`TestDesktopLocalModelPickerLayout` 在私有数据目录中预置一条展示用消息，
先确认已有会话输入框确实比新建页矮，再比较两页的实际浮层间距。
四组原生场景覆盖中英文、深浅主题、2/12 模型目录，真实本地核心处理设置与选择，
消息记录仅用于定位场景，不代表完成真实供应商请求。
最终证据为 `output/model-button-anchor-final/20261030-145857-1257385802/evidence.json`，
各场景的 `single-model-position.png` 与 `conversation-model-position.png` 保存两页窗口。
实际测量新建页输入区高 76 像素、会话输入区高 47 像素，菜单间隔均为 6 像素。
全仓测试、Go vet、MyGo vet 和 uifixture 回归通过。
本地便携程序更新记录为 `output/local-update-20261030-150003/result.json`，
更新后窗口可见且响应，五个既有会话 ID 保持不变。

此前 `output/model-menu-anchor-final/20261030-121103-1965434049/evidence.json`
记录的整输入框顶部锚定方式已被替代；该方式会使高输入框中的菜单离按钮过远。
对应 `TestModelMenuClearsComposerAndAlignsCardTops` 已替换为上述按钮间距回归。
此前本地更新记录 `output/local-update-20261030-121319/result.json` 仅属于旧锚定版本。

2026-10-30：移除模型收藏、替换模型设置入口的最终证据位于
`output/model-settings-entry-final/20261030-113859-2131022241/evidence.json`。
四组原生模型场景覆盖深浅主题、两种模式、顶部模型设置入口、每行模型设置按钮、
打开正确模型并保存显示名，确认聊天选用模型不变，且不存在收藏或底部重复入口。
命令引用和目标/待办覆盖用例同批通过。全仓 Go 测试、Go vet、MyGo vet、
uifixture 回归通过，Linux 无桌面服务完成交叉编译；未请求外部模型供应商。
同时复现 Windows 读句柄阻止替换文件的问题，原子保存现仅对两类占用错误进行
500 毫秒内重试。Windows 回归验证释放句柄后成功，以及永久拒绝时原文件与临时文件处理。

2026-11-01：模型行逐行设置按钮已移除，模型编辑统一从供应商菜单底部的
「模型管理」进入；上条 2026-10-30 证据只记录当时版本。

2026-10-30：此前紧凑/非紧凑交互修复的证据位于
`output/model-picker-modes-delivery/20261030-111347-2769742718/evidence.json`。
紧凑模式验证首层小面板、展开列表、返回按钮和选中返回；非紧凑模式验证直接列表及选中后
保持打开。四组原生场景继续覆盖中英文、深浅主题、收藏、供应商悬浮名称、独立思考强度
落盘和模型管理。组件测试另验证 Escape 的两级返回、侧聊隔离及模型选项保留；
全仓 Go 测试、Go vet、MyGo vet 和 uifixture 组件回归通过。
命令引用及目标/待办覆盖复验的证据位于
`output/model-picker-modes-verified/20261030-111159-192177897/evidence.json`。
目标/待办用例的命令目录改为在停机私有 profile 中一次性预置，仅验证展示和浮层；
真实命令编辑/引用仍由资源测试和 `TestDesktopLocalPromptReferences` 覆盖。

2026-10-30：此前模型选择器布局证据位于
`output/model-picker-layout-delivery/20261030-105541-1465636731/evidence.json`。
四组原生用例覆盖英文浅色、中文深色及紧凑开关两态；截图包含模型列表、供应商悬浮名称、
收藏列表和独立思考强度选择器，另检查收藏与思考档位的真实落盘以及模型管理入口。
组件回归补充侧聊配置隔离、数字快捷键移除、7,000 模型虚拟化与供应商滚动时固定入口。
本轮未请求外部模型，不作为供应商服务或像素一致性的验收。
命令引用与目标/待办覆盖复验通过，证据位于
`output/model-picker-layout-verified/20261030-105248-2913645993/evidence.json`。

2026-10-30：候选菜单方向与覆盖专项证据位于
`output/prompt-picker-placement/20261030-102836-1716803787/evidence.json`。
900×600 原生窗口的深浅主题已检查目标详情/待办列表被浮层遮挡，
首页和会话页候选均检查在输入框上方。组件回归另外覆盖侧对话、
更窄输入框、上方空间不足时的滚动高度，以及点击不穿透目标控件。

2026-10-30：命令与资源引用专项最终证据位于
`output/prompt-references-final/20261030-101729-3187226424/evidence.json`。
原生测试使用 accessibility 编辑和按下动作，键盘与输入法路径由组件测试覆盖；
未激活后台测试窗口或声称系统键盘/输入法验收。完整功能语义见
[prompt-references.md](prompt-references.md)。

2026-11-03：底栏反序与等距布局专项通过，证据位于
`output/sidebar-footer-reordered/20261103-175043-3098085740/evidence.json`。
英文浅色与中文深色窄窗验证检查更新、用量、手机、设置的顺序和间距，
Git/PR 入口已移除；手机空白弹窗可打开和关闭，底栏与设置页均可打开原生检查更新窗口。
实窗截图已检查，未访问更新源或执行下载安装。

2026-10-30：以下为旧版更新入口显隐的历史证据；2026-11-03 起入口常驻。该专项当时通过，证据位于
`output/updater-visibility-native/20261030-115505-4284844316/evidence.json`。
英文浅色与中文深色窄窗均验证未检测到新版本时底栏仅保留设置、PR 与用量，
并通过设置页打开、关闭原生更新窗口后再次确认底栏更新入口隐藏。
实窗截图已检查。组件测试覆盖发现新版本后显示、状态清除后隐藏及手动检查；
插件测试覆盖检查失败、无更新、跳过版本、稍后提醒与安装后等待重启的状态和通知。
全仓 Go 测试、Go vet、MyGo vet 及共享控件边界检查通过。
未访问真实更新源或执行真实下载安装。

2026-10-30：此前侧栏底部专项通过，证据位于
`output/sidebar-footer-native/20261030-092856-689907308/evidence.json`。
英文浅色与中文深色窄窗均运行真实本机核心，验证四个稳定 key、设置导航、
空 profile 的零用量以及原生更新窗口开关；PR 外链不在测试中访问。
非零用量累计、缓存不重复计入、分叉历史去重及 PR 链接选择由组件/核心测试验证。
前一轮同时通过现有场景设置和真实核心持久化回归，
证据位于 `output/sidebar-footer-native/20261030-092537-338226911/`。
实窗底栏、用量窗口和原生更新窗口截图已检查；这是功能/布局证据，
不代表系统输入法、真实更新下载安装或已暂停的完整视觉一致性验收。

截图来自 MyGo `CapturePage`，属于真实窗口内容捕获，不是系统合成截图。
系统输入法、托盘、系统上下文菜单、原生弹窗、嵌入 WebView 的像素和视觉一致性
仍需对应原生专项验证。接入插件没有恢复已暂停的 UI 优化目标。

2026-10-30：模型行图标专项通过，证据位于
`output/desktop-e2e/20261030-003104-255688902/evidence.json`。
模型行实窗截图已目视检查；模型测试使用本地 HTTP 夹具，不代表真实供应商验收。

2026-10-30：自定义子智能体原生流程通过，证据位于
`output/desktop-e2e/20261030-004500-14702289/evidence.json`。
创建、保存后返回列表、编辑取消、禁用落盘、独立进程重启读回以及确认删除均已验证；
表单、列表及删除后的实窗截图已目视检查。不请求外部模型，不代表视觉一致性验收。

## 插件来源和固定补丁

2026-11-02 已整体升级到官方 MyGo v0.7.4（发布提交 `9bd34e1eb9`，MIT 许可沿用框架
原始 LICENSE）。上游已收录本仓库提交的 13 个 PR 能力（含 `PlaceholderColor`、
`CaretWidth`、`Scrollbars(false)`、`DottedUnderline`、`Context.Instant`、
`markdown.Options.Anchor`、`Theme.LinkWeight`、`Notification.ShowContext`、
`updater.AvailableVersion`、文本客户端辅助功能与包重命名重试），相应本地补丁删除。
`plugins/automation` 与原生 UI 接口由上游原生提供；独立安装名由上游
`windows.displayName`/`windows.installDirName` 与 slug 文件命名覆盖。
应用命名完全跟随 `mygo.json` 的 `name`（GCode 为 ASCII，上游默认值即可覆盖）。

2026-11-02 时框架固定为 `rcode-native-ui.23`；该阶段源码锁、补丁 SHA 和构建身份由
`go run ./scripts/check-framework.go` 检查。当前树为 2,356 文件、16 个补丁条目，
SHA-256 为
`e32652f3fe5699ea3c1e40a8fe4b832e96f1b9e6824f70bf7768bd0cc8760926`。
补丁清单见 [mygo-patches.md](mygo-patches.md)；下述 `.9`/`.10` 重放指纹是
v0.6.4 时期引入第 15 项补丁时的历史记录。
第 15 补丁反向还原后应得到原 `.9` 的 1,864 文件和
`b2c4fc2817725ad69b915e53e764b4af08971ed327d933004c773f6fbf44be43`；
重新应用后应得到 `.10` 的 1,882 文件和
`ff51360b711932fdb7451a87a6d94635671e9b0c79672c352ff3be5ce72564b2`。

## 验证记录

2026-11-04 项目菜单显示逻辑补验：同一原生用例通过深浅主题的选择器整体左边缘对齐、
默认向上展开、打开面板后悬停不显示快捷清除，以及未选项目时隐藏“不使用项目”操作。
组件测试另检查上方空间不足时向下翻转、右侧空间不足时缩窄、关闭面板后恢复悬停清除；
列表按可用空间保留完整行。当前证据位于
`output/project-menu-behavior-final/20261104-022545-4200045221/`。
共享控件门禁、Go vet、MyGo vet 和普通桌面构建通过。

2026-11-04 项目选择菜单：`TestDesktopLocalProjectSelectionMenu` 使用独立本机核心、
18 个临时项目和独立数据目录，通过浅色与深色（20px 字号）原生窗口检查。
覆盖项目列表滚动、固定底部操作、中文搜索、选中勾号、保留草稿的切换/清除，
以及从菜单打开新建项目。原生截图与无障碍树位于
`output/project-selection-menu-final/20261104-020916-1072064248/`。
组件回归额外覆盖窄窗口、键盘选择、空搜索结果和重新打开时重置搜索；
共享控件门禁、Go vet、MyGo vet 与普通桌面构建通过。
这些是本机选择菜单证据，不扩展为远程宿主或真实模型调用验收。

2026-10-30 控制台弹窗修复：Go 入口、源码检查、Git 夹具和真实核心 Git 命令
统一使用无控制台启动。Windows 系统级测试检查直接与嵌套子进程的
`GetConsoleWindow()==0`，同时验证输出、stderr、失败退出码及既有进程属性。
完整 7 项原生用例通过，证据位于
`output/desktop-e2e/20261030-101825-2592111653/evidence.json`；
全仓 Go 测试及相关 Go vet 通过。Linux amd64、macOS arm64 的入口与服务交叉编译通过。
本次还修正了自动化动作的等待条件：异步输入后等待按钮公开可用操作，再提交点击，
避免发送按钮尚未启用时误报失败。

Windows 实窗用例两项通过，最新完整记录：
`output/desktop-e2e/20261030-000830-3515763048/evidence.json`，
由统一 Go 入口执行，测试执行耗时 2.263 秒，不包含构建，也没有与旧流程进行性能对照。
设置搜索后的 PNG 已目视检查；真实本地核心用例检查落盘文件，并由新进程读回。
应用日志均为空，测试创建的窗口和 profile socket 目录正常退出、清理。
早期失败尝试的系统 TEMP socket 目录清理被自动审批拒绝，未重试删除；
这些目录不参与当前测试运行。

全仓 `go test ./...`、Go vet、MyGo vet、共享控件门禁、完整 MyGo `ui` 测试、
插件协议测试及源码指纹检查通过。第 15 补丁反向/正向重放指纹一致。
普通桌面依赖不包含插件，无桌面服务依赖不包含 MyGo；Windows 发布 tag 构建通过。
Linux amd64、macOS amd64/arm64 的 `e2e` 程序仅验证了交叉编译，尚未运行实机测试。
统一 Go 入口另通过 Linux amd64/arm64、macOS amd64/arm64 交叉编译；
Windows 下包含空格的输出路径与单用例筛选执行成功，无匹配用例时返回非零退出码。
入口的结果解析、平台参数、环境隔离及输出目录边界单元测试通过；
目录符号链接用例因当前 Windows 账户缺少创建符号链接权限而跳过。
