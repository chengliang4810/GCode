# 跨平台目标构建验证

日期：2026-10-12（Asia/Shanghai）

## 当前集成源码的复验

后台生命周期和 Markdown cache generation 修复合入后，root 已重新执行相同的两个交叉编译命令；本节记录当前源码，下面的 `20261012/` 记录保留为修复前历史。

当前证据位于 [20261012-integrated/manifest.json](../../output/acceptance/platform-build/20261012-integrated/manifest.json)。两个命令均 exit 0，生产清单为 `32e08f88522775e0cfb2568393ea93371b773d31222191f4e8533b2792f13a67`，832 个文件逐一核对未变；正式 Intel 应用包仍为 `db479d18...`，没有被交叉编译覆盖。

| 目标 | 文件类型 | 大小 | SHA-256 | 范围 |
| --- | --- | ---: | --- | --- |
| `darwin/arm64` | Mach-O 64-bit arm64 | 31,479,314 bytes | `1beeb005e4f703df98029bc238b4794d4cac30e600335018b12c05e4e4f5339e` | 编译通过；目标设备未运行，平台 bundle 未生成 |
| `windows/amd64` | PE32+ x86-64 | 33,629,184 bytes | `ab745f345f397ffaa2d5a8ea2bbfe5ffabf70bbe8071b3a83932a4ccf81ce406` | 编译通过；目标设备未运行，安装包未生成 |

原始命令、日志、退出结果和源码核对分别见同目录的 `commands.txt`、`*-build.log`、`results.json` 和 `source-verification.json`。此复验不证明目标设备的 UI、中文输入、终端资源或生产签名。

## 修复前历史记录

环境：Go `go1.27.2`，主机 `darwin/amd64`，`CGO_ENABLED=1`（仅记录主机默认值；两次构建显式使用 `CGO_ENABLED=0`）。本轮没有启动 Apple Silicon 或 Windows 产物。

## 结论

`darwin/arm64` 和 `windows/amd64` 的 Go 交叉编译均通过，生成了可识别的 Mach-O 和 PE32+ 可执行文件；冻结源码清单的 832 个 production files 在构建前后逐文件匹配。

这不是完整平台交付验收。直接 `go build` 没有生成 macOS `.app`、Windows 安装器或签名，也没有验证原生资源、目标设备启动、性能、中文输入、PTY、真实厂商 API 和完整 UI 交互。按照设计要求，交叉编译不能替代目标机器验收。

## 产品和构建入口证据

| 证据 | 事实 |
|---|---|
| [PRODUCT.md](../../PRODUCT.md#L5-L7) | 产品为 Go + mygo 原生桌面；分别验收 macOS Intel 与 Apple Silicon，随后 Windows；不包含 Linux。 |
| [docs/design/gcode-desktop.md](../design/gcode-desktop.md#L13-L19) | 交付平台决策是 macOS Intel/Apple Silicon 后 Windows，Linux 不纳入产品范围。 |
| [docs/design/gcode-desktop.md](../design/gcode-desktop.md#L229-L238) | 必须分别报告各平台编译、原生运行和未验证项；交叉编译不能替代目标机器验收。 |
| [Makefile](../../Makefile#L5-L13) | 普通入口是 `go build`；正式打包入口是 `go tool mygo build --skip-dmg`。 |
| [mygo.json](../../mygo.json#L1-L10) | 应用入口是 `./cmd/gcode`，输出目录为 `build`，macOS 最低系统版本为 `13.0`。 |
| [mygo build.go](../../third_party/mygo/cmd/mygo/build.go#L25-L43) | 完整 mygo build 会处理平台包；macOS 可生成签名 `.app`/磁盘映像，Windows 可生成安装器，并支持跨机编译。 |
| [mygo natives.go](../../third_party/mygo/cmd/mygo/natives.go#L54-L115) | 完整打包会扫描依赖的 `mygo-plugin.json` 并获取原生库；本轮未执行该流程。 |

## 交叉编译记录

两次命令均使用 `GOFLAGS=-mod=readonly`，没有修改 `go.mod` 或 `go.sum`；`-buildvcs=false` 避免构建时写入 VCS 元数据。原始命令和退出码见 [commands.txt](../../output/acceptance/platform-build/20261012/commands.txt)，stderr 日志见同目录的两个 `*-build.log`（均为空）。

| 目标 | 结果 | 文件类型 | 大小 | SHA-256 | 运行/打包 |
|---|---|---|---:|---|---|
| `darwin/arm64` | PASS，exit `0` | `Mach-O 64-bit executable arm64` | `31,444,562` bytes | `9f885af2d576d1718d951dad3ab38c2fcedb3c44f88501d29ad3865cec73412b` | `native_run=NOT_RUN`；`package=NOT_BUILT` |
| `windows/amd64` | PASS，exit `0` | `PE32+ executable (console) x86-64, for MS Windows` | `33,597,952` bytes | `9511e2c5d0a0ad3a622507551689129370572e0039e7d0fe474330002187bd62` | `native_run=NOT_RUN`；`package=NOT_BUILT` |

产物路径为：

- [GCode-darwin-arm64](../../output/acceptance/platform-build/20261012/GCode-darwin-arm64)
- [GCode-windows-amd64.exe](../../output/acceptance/platform-build/20261012/GCode-windows-amd64.exe)

机器可读的同一份记录见 [manifest.json](../../output/acceptance/platform-build/20261012/manifest.json) 和 [results.json](../../output/acceptance/platform-build/20261012/results.json)。

## 源码身份核对

本轮使用既有冻结清单 [source-fingerprint.json](../../output/acceptance/current-package-regression/visual-refinement-confirmed/source-fingerprint.json) 做逐文件核对：

| 项目 | 结果 |
|---|---|
| production files | `832` |
| 清单 SHA-256 | `02d2ecc2f9ff000176aa644ba0570ed5643c44a05c545a227cf249b17707f27c` |
| 构建后 `missing_files` | `[]` |
| 构建后 `changed_files` | `[]` |
| `per_file_match` | `true` |
| `go.mod` SHA-256 | `1ee0c069767ee85f5b78dcafb282f04819ed85e8899a32439872aeefe8ae2803` |
| `go.sum` SHA-256 | `f8f7b674f26aeaa57d0df64fffaf7722fe5f22a2d44ef9629bd8d72b52a1390f` |

详细 JSON 结果见 [source-verification.json](../../output/acceptance/platform-build/20261012/source-verification.json)。这项核对证明构建过程没有改动冻结清单覆盖的源码；它不证明产物已经在目标平台运行。

## 未执行项与限制

- **Windows ARM64**：未执行。设计文档只规定 Windows，没有给出 Windows CPU 架构；本轮以 `windows/amd64` 作为 Windows 构建目标。
- **完整 mygo 打包**：未执行。Makefile 的 `package` 入口会进入 mygo 的平台打包流程；该流程会处理 bundle、资源、签名/安装器，并可能获取依赖原生库。本轮只需要验证 Go 交叉编译，因此没有触发下载或 SDK 准备。
- **macOS `.app` 和签名**：未生成、未验证。当前结果是独立 Mach-O 文件，不是应用 bundle。
- **Windows 安装器**：未生成、未验证。当前结果是独立 PE 可执行文件。
- **原生资源**：未验证，包括 mygo 插件声明的 native libraries、实际 app bundle 资源和运行时加载路径。
- **目标设备运行**：未执行。没有在 Apple Silicon Mac 或 Windows 设备上启动产物。
- **性能和产品场景**：未执行。没有验证启动、空闲/长会话/并发内存、UI 交互、中文输入、PTY、剪贴板、真实厂商 API 或系统安装行为。
- **测试套件**：未执行全仓测试；本轮结论只覆盖上述两个 `go build` 命令和源码清单核对。

因此，本记录的最终状态是：两个指定目标的交叉编译 **PASS**；原生运行、打包、签名、设备验收和性能验证均为 **NOT_RUN**。任何平台发布或兼容性结论都必须在相应目标设备和正式打包流程完成后另行记录。
