# 2026-10-28 换机交接与进度

## 当前目标和状态

完全复刻 Zeron v0.2.128 的桌面界面、布局、视觉状态与交互。
源码事实源固定为 `ab189159f10cf8e63c0502be75a59bbc487e0eaf`，不自动追新。
Go/MyGo 原生界面，只有公开网页正文使用系统 WebView；业务数据全部由有真实
状态变化的进程内 Go mock 服务提供。保留 Zeron 品牌/英文，移除所有语音入口。

**当前为实现与验收进行中，未达到“完全一致”。** 用户因本机 Computer Use
和人工操作冲突，要求整理资料、提交推送后暂停目标，在另一台机器恢复。
新机器先完成下面的环境核对与基线检查，再明确恢复执行目标及桌面操作。

## 仓库与固定版本

| 项目 | 固定信息 |
| --- | --- |
| 工作仓库 | `https://github.com/chengliang4810/GCode.git` |
| 分支 | `codex/isolated-worktree`；不替换 main，不改其他本机 checkout |
| Go | 1.27.3；go.mod 已固定，允许 Go toolchain 自动下载 |
| 实际 MyGo 依赖 | v0.6.4 + `zeron-native-ui.2`，仓库内 `third_party/mygo` |
| MyGo 依赖源码 SHA | `fd197203233f09d5cede40fc69fad6e6bac8f6dc797e1770f1637e4687ab133e`，1,830 文件、5 个补丁 |
| 原版参考仓库 | `https://github.com/zeronsh/zeron.git`，上述 0.2.128 提交，只读 |
| 官方 MyGo 示例参考 | `https://github.com/egoist/mygo.git`，`43199d0b0fb95fd0b71e5a225d2807c04ebcc609` |
| 桌面目标 | Windows 11 22H2+ amd64；macOS 13+ amd64/arm64，分别实机验收 |

## 已实现与资料入口

| 范围 | 当前实现 | 逐项记录 |
| --- | --- | --- |
| 工程/数据 | 四层结构、typed DTO/命令/事件、可取消流、状态转换、固定场景、持久化、独立安装/数据身份 | `contracts.md`、`audit-agent-data.md` |
| 资源/主题 | 507 项原版素材、30 主题、210 公式/图表矢量；原生 Markdown/编辑器/终端/网页正文 | `reference-baseline.md`、`reference/baseline.json` |
| 应用壳/文件区 | 侧栏/聊天/工具/最右 Files，动态挤压、拖动、takeover、底部终端、标签与文件编辑 | `audit-workbench.md` |
| 侧栏 | 分组/排序/显示菜单、置顶拖动、归档、行内重命名、原生键盘选择和项目过滤 | `audit-sidebar.md` |
| 模型/输入区 | 紧凑/标准菜单、搜索、收藏、推理、provider 记忆与锁定、加载/重试；中央/侧聊共用 surface 与独立 model/access/Plan | `audit-model-picker.md`、`audit-composer.md` |
| Agent 输出 | 按 turn 分组的活动树、思考、读写编辑/命令、失败恢复、差异/子代理、独立最终回复；真实不同的会话轨迹 | `audit-transcript.md`、`audit-agent-data.md` |
| 命令/项目 | All/Threads/Commands/Files 四页、typed 文件/内容搜索、行列定位、新项目设备/目录选择、离线/重复错误 | `audit-command-palette.md` |
| 设置条件流程 | Providers/Devices/Cloud/Billing 四页、目标设备/账号/code 登录、更新、GitHub 仓库、计划/credits 的 mock 闭环 | `audit-account-settings.md` |

其他设置、Git/PR、浏览器和终端已有实现，完整覆盖以 `ui-parity.md` 及
`reference/scenes.json` 为准。清单“automated-regression-only”表示实现回归，
不能读成原生/视觉验收通过。

## 下一阶段优先项

1. 建立同尺寸/DPI/主题/时间/数据的独立参考截图，先解决公共几何、字体、
   材质和控件，再比较整窗。现有实现图不能当参考图或用于自己证明 SSIM。
2. 完成源码 composer compact/expanded/dock 与 footer/usage ring：准确常量、
   阈值和源码函数在 `audit-composer.md`。此次只保留已验证的共用 surface/
   独立控制器，未验证的 compact 试接线已撤掉，当前仍不是原版全部布局。
3. 主/侧 composer 根据所选 device 消费 `Integrations.Providers` 的
   Installed/Enabled，避免 remote 会话误用 local 开关；侧聊 working trailer 待接。
4. 原版搜索的独立文件名/内容返回时序、索引/错误状态、CRUD 后重查、长树
   虚拟化/多选/Git 装饰；新项目卷/Home 索引/补全/Cloud clone 等详见命令审计。
5. Providers 原版设备菜单、账号整行切换、更新调度、Cloud off 导航自动 enable
   等条件差异，详见账号设置审计。它们保持业务 mock，不调用真实认证或支付。
6. 完整原生菜单/动效、PR inline review、快捷键全集、背景 hold/resume 与
   系统壁纸色、各类状态与字体换行仍需逐项收敛。
7. Windows 原生 IME/剪贴板/窗口/DPI/网页弹层与性能门槛，以及 Mac Intel/ARM
   的实际执行、Retina、菜单、材质和输入验收均未完成。

## 新 Windows 机器开始

使用 Git、Go 和 PowerShell 7。在新目录检出工作分支：

```powershell
git clone --branch codex/isolated-worktree --single-branch https://github.com/chengliang4810/GCode.git GCode
cd GCode
git status --short
go env GOVERSION
go run ./scripts/check-framework.go
go test ./...
go vet ./...
go tool mygo vet .
```

确认 Go 显示 `go1.27.3`。无须把实际依赖改成官方示例仓库的 HEAD；
实际 UI 依赖已经包含在此仓库。浏览器需要正常的系统 WebView2 运行时。

准备外部只读参考（默认路径沿用原机器，也可传其他空目录）：

```powershell
./scripts/bootstrap-references.ps1 -ZeronRoot D:/zeron -MygoExamplesRoot D:/projects/mygo
./scripts/reference-verify.ps1 -SourceRoot D:/zeron
```

先直接从当前源码运行固定的新场景，避免历史 profile 混入旧模板：

```powershell
go run ./cmd/zeron --scenario default --route chat/chat-0 --appearance light
go run ./cmd/zeron --scenario transcript --route chat/chat-0 --appearance light
go run ./cmd/zeron --scenario live --route chat/chat-0 --appearance dark
```

构建检查包时使用新的输出目录，防止 Windows 覆盖已有 Universal 目录失败：

```powershell
./scripts/build.ps1 -Targets windows/amd64 -OutDir build/machine-check
./scripts/review-ui.ps1 -ReleaseDir build/machine-check/windows-amd64
```

普通运行 `go run ./cmd/zeron` 使用 OS 配置目录的 `gcode-zeron-ui/state.json`。
固定场景不传 `--data-dir` 时使用内存业务数据和独立 native profile。原版 Zeron
的数据目录和副本各自独立；迁移源码不会自动带入个人账号、正常 profile 或凭据。

Windows race 检查需要可用 x64 GCC：

```powershell
./scripts/race.ps1 -Compiler C:/path/to/gcc.exe
```

旧机器用官方 w64devkit 2.10.1/GCC 16.2.0；下载来源/校验记录在验证资料中。
lint 脚本固定 golangci-lint 2.15.0，可首次联网获取或通过 `-ToolPath` 指定它。

## Mac 开始

从相同分支检出，安装 Go 1.27.3；运行 `go test ./...`、`go vet ./...`、
`go tool mygo vet .`。`sh scripts/build-macos.sh` 在首次干净的 build 目录生成
Intel、ARM、Universal 包。已有产物应先保留到别处；不要混入旧的 app bundle。
需要参考源码时把仓库 clone 到实际路径，向支持 `SourceRoot` 的脚本显式传入。
签名/公证/DMG 与两个架构的实机验收分开记录。

## 证据与参考截图桥接

- `docs/evidence/`：本轮命令/项目、设置条件状态的实现图、矩形和 manifest。
- Git 精选跟踪的 `output/` 子目录：模型、侧栏、workbench、transcript、
  79 张全主题/边界矩阵、24 张设置图、验证与安装自截日志；不包含编译缓存。
- `reference/validation/`：历史构建/测试/来源 hash 与当前交接验证记录。
- `reference/reported/`：用户给出的模型菜单和 Agent 输出参考，保留补充证据身份。
- `scripts/reference-capture-bridge.ps1`：从只读原版官方 compact fixture 生成
  独立 capture adapter，仅改截图传输与测试配置，不改生产 UI/renderer。

原版 Windows 的 `render_to_image` 未实现。官方 fixture 的进程退出码可能仍为 0，
必须检查 stderr/result.txt 与实际 PNG，不能据退出码认定截图成功。桥接例程已编译，
但第一次等待 WGC 截图时工具再次报告 Escape 停止，**没有取得该参考图**。
`.ready.json` 仅是场景就绪记录，不是验收通过证据。新机须通过 Computer Use
选中唯一 fixture 窗口采集、保存原图后才写对应 `.captured` 确认文件。
每次重放使用全新空输出目录，避免旧 `.ready.json` 或 `.captured` 被当成此次结果。
构建原版 fixture 另需参考仓库 rust-toolchain 指定的 Rust/Cargo、MSVC C++
工具链和 Windows SDK。它们不是运行 Go 副本的前置条件。先用
`./scripts/reference-build.ps1 -SourceRoot D:/zeron -Example compact-picker-fixture -Features compact-picker-fixture`
生成隔离源码，再运行 bridge 脚本生成 adapter 并按其输出构建重放；完整步骤见
`scripts/reference-build.ps1`、`scripts/reference-capture-bridge.ps1`。

`build/ui-audit` 属于上一轮检查包，不包含这轮所有新增源码，本次不把它作为
当前版本交付。新机器从已提交源码重新构建；不要以旧包和旧 screenshot hash
推断当前源码表现。当前交接门槛见 `reference/validation/handoff-result.json`。

## 给新会话的恢复指令

> 读取 AGENTS.md、HANDOFF.md、docs/machine-handoff.md 和 docs/goal-execution.md。
> 我已换到专门的验收机器，请恢复“与 Zeron v0.2.128 桌面 UI/交互完全一致”目标；
> 若新会话没有原目标，则按这些边界建立同一执行目标。先核对分支/提交和固定
> 参考，跑当前基线，再从剩余差异继续实施。优先用 MyGo 官方组件，不新增 mock
> 标识，不改旧机器或其他 checkout。源码/构建/离屏/原生/用户验收分别记录，
> 没有真正完成全部门槛时不得把目标标为 complete。
