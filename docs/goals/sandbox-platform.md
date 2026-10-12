# /goal：收敛 GCode sandbox/platform

状态：代码收敛完成；本机定向验证完成；真实 Windows/macOS 设备验收待执行。

## 范围

本目标只负责 `internal/sandbox/**` 以及本记录，不修改 `cmd/gcode`、`cmd/zero-windows-*` 或 OAuth/MCP 的其他所有权代码。sandbox 的私有路径、环境变量、helper 名称和平台后端统一使用 GCode 标识；已经废弃的 Zero 私有输入不再作为回退路径读取。

## GCode 私有路径和环境名

- 工作区元数据目录是 `.gcode`，项目配置是 `.gcode/config.json`。
- Linux 和通用 XDG 凭据根是 `$XDG_CONFIG_HOME/gcode`；未设置时使用 `$HOME/.config/gcode`。
- macOS 凭据 deny baseline 同时覆盖 `~/Library/Application Support/GCode` 和 `~/Library/Application Support/GCode Dev`。开发应用和生产应用的数据根都必须受到保护；`~/Library/Application Support/zero` 不在规则中。
- sandbox grant 使用 `GCODE_SANDBOX_GRANTS_PATH`，Windows sandbox 状态使用 `GCODE_WINDOWS_SANDBOX_HOME`。
- sandbox 运行标记和网络标记是 `GCODE_SANDBOXED`、`GCODE_SANDBOX_BACKEND`、`GCODE_SANDBOX_NETWORK`；OAuth/MCP 文件覆盖是 `GCODE_OAUTH_TOKENS_PATH`、`GCODE_MCP_OAUTH_TOKENS_PATH`，存储后端选择是 `GCODE_OAUTH_STORAGE`。
- Linux helper 是 `gcode-linux-sandbox`。Windows helper 是 `gcode-windows-command-runner.exe` 和 `gcode-windows-sandbox-setup.exe`。

旧的 `.zero`、`$XDG_CONFIG_HOME/zero`、`ZERO_*` 环境变量、`zero-linux-sandbox` 和 `zero-windows-*` helper 名称不会被 sandbox 读取或生成。旧凭据不会因为本次 sandbox 收敛被迁移或删除。

## Windows helper 和入口契约

Windows helper 解析顺序固定为：运行文件相邻的独立 helper、PATH 中的 helper、当前 GCode 可执行文件的 self-dispatch。self-dispatch 使用 `os.Executable()`，并把下列隐藏参数放在 helper 参数之前：

```go
const (
	WindowsCommandRunnerSubcommand = "__windows-command-runner"
	WindowsSandboxSetupSubcommand  = "__windows-sandbox-setup"
)
```

root `cmd/gcode` 入口应在正常 UI 初始化前保留最小分派：

```go
switch args[0] {
case sandbox.WindowsCommandRunnerSubcommand:
	return sandbox.RunWindowsSandboxCommandRunner(args[1:], os.Stderr)
case sandbox.WindowsSandboxSetupSubcommand:
	return sandbox.RunWindowsSandboxSetup(args[1:], os.Stderr)
}
```

当前实际入口已按该契约接入；本目标不再修改 root 入口。`GCODE_WINDOWS_COMMAND_RUNNER_EXE` 和 `GCODE_WINDOWS_SANDBOX_SETUP_EXE` 只用于真实 smoke 或显式 helper 覆盖，不改变 self-dispatch 契约。

## 平台边界

- macOS 使用 native Seatbelt，读取规则按当前 profile 生成，GCode 凭据目录使用目录级 deny，并为插件、specialist、command 子目录保留 carve-out。
- Windows 使用 restricted-token 文件系统写 jail，并通过 WFP 基础设施处理网络 deny。当前 restricted-token 两个 runner level 都不接受非空 `DenyRead`，因为窄 SID token 无法同时加载普通系统程序并维持写边界；代码会在启动前拒绝该组合。
- Linux sandbox core 保留 bwrap/Landlock 的编译和测试能力，用于 runtime 共享与后续平台工作；当前没有 Linux 桌面发布入口。

## 验证

已执行或应保持通过：

```sh
GOTOOLCHAIN=local go test ./internal/sandbox
GOTOOLCHAIN=local go test ./internal/agent
GOOS=windows GOARCH=amd64 go test -c -o /tmp/gcode-sandbox-windows.test.exe ./internal/sandbox
GOOS=linux GOARCH=amd64 go test -c -o /tmp/gcode-sandbox-linux.test ./internal/sandbox
```

测试覆盖了 GCode 路径、旧 Zero 环境不兼容、macOS 两个 Application Support 数据目录的 deny baseline、Windows helper self-dispatch 解析和 Linux helper 编译。交叉编译只证明目标包可编译，不能替代目标设备上的行为验证。

## 未完成的设备验收

当前工作机是 macOS；本目标没有真实 Windows restricted-token/WFP、Windows 打包 helper、macOS Seatbelt native smoke 或 packaged GCode Dev/GCode 双数据目录读写测试的证据。发布前需要在对应设备上重新启动原生应用，验证 helper 解析、sandbox 实际拒绝读写、网络策略和应用数据目录；Linux 只需保持共享 core 的编译/测试结果，不能据此宣称 Linux 桌面发布支持。
