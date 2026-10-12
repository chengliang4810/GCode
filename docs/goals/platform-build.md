# /goal：跨平台目标构建与验收边界

日期：2026-10-12（Asia/Shanghai）

状态：交叉编译范围已完成；目标设备运行、原生打包和发布验收仍未完成。

更新：后台生命周期和 Markdown cache generation 修复集成后，root 在 [20261012-integrated/](../../output/acceptance/platform-build/20261012-integrated/) 重新完成 `darwin/arm64` 和 `windows/amd64` 两个交叉编译，均 exit 0；832 个生产文件对应当前清单 `32e08f88522775e0cfb2568393ea93371b773d31222191f4e8533b2792f13a67`，逐文件未变，正式 Intel bundle 未被覆盖。两份新产物只证明可编译，仍未在目标设备运行或完成平台打包。以下 `20261012/` 命令和 `02d2ecc...` 清单继续作为修复前历史证据；当前产物身份见 [跨平台复验](../verification/platform-build.md)。

## 目标

为 GCode 当前源码生成可供后续目标设备验收的跨平台构建产物，并把构建结果、源码身份和未验证范围记录成可复核证据。产品平台边界来自 [PRODUCT.md](../../PRODUCT.md) 和 [GCode 原生桌面 Agent 设计](../design/gcode-desktop.md)：macOS 分别验收 Intel 与 Apple Silicon，随后验收 Windows；Linux 不属于产品目标。

本轮在 macOS Intel 主机上执行两个不需要目标机运行的 Go 交叉编译：

- `darwin/arm64`，代表 Apple Silicon 构建；
- `windows/amd64`，作为本轮 Windows 构建目标。

Windows 设计只规定 Windows，没有规定 CPU 架构，因此本轮不扩展执行 `windows/arm64`。macOS Intel 的原生运行和包验收仍按既有目标单独记录，不由本轮 Apple Silicon 交叉编译结果代替。

## 实施边界

- 入口固定为 `./cmd/gcode`，使用 `go build -buildvcs=false`。
- 使用 `CGO_ENABLED=0` 和 `GOFLAGS=-mod=readonly`，避免引入目标机原生依赖或修改 module 文件。
- 只写入本目标的文档和 `output/acceptance/platform-build/20261012/` 证据目录。
- 不运行任何交叉编译产物，不执行全仓测试，不执行完整 `mygo build`，不下载原生 SDK，不修改 `build/darwin-amd64/GCode.app` 或生产源码。
- 交叉编译只证明 Go 源码在目标 `GOOS/GOARCH` 下能够生成可识别的可执行文件；它不能证明目标设备启动、UI、PTY、性能、原生资源加载或签名结果。

## 构建命令

```sh
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 GOFLAGS=-mod=readonly \
  go build -buildvcs=false \
  -o output/acceptance/platform-build/20261012/GCode-darwin-arm64 \
  ./cmd/gcode

GOOS=windows GOARCH=amd64 CGO_ENABLED=0 GOFLAGS=-mod=readonly \
  go build -buildvcs=false \
  -o output/acceptance/platform-build/20261012/GCode-windows-amd64.exe \
  ./cmd/gcode
```

两条命令均以 exit code `0` 完成。命令原文保存在 [commands.txt](../../output/acceptance/platform-build/20261012/commands.txt)，构建结果和限制保存在 [manifest.json](../../output/acceptance/platform-build/20261012/manifest.json) 与 [results.json](../../output/acceptance/platform-build/20261012/results.json)。

## 完成判据

| 判据 | 本轮结果 |
|---|---|
| `darwin/arm64` 交叉编译 | 已通过；产物和 SHA-256 已记录 |
| `windows/amd64` 交叉编译 | 已通过；产物和 SHA-256 已记录 |
| 当前生产源码身份保持不变 | 已通过；冻结清单中的 832 个 production files 逐文件匹配 |
| `windows/arm64` | 未执行；设计未规定该架构 |
| macOS `.app`、Windows 安装器、签名和原生资源 | 未执行 |
| Apple Silicon、Windows 目标设备运行 | 未执行 |
| 性能、中文输入、PTY、真实厂商 API 和完整交互 | 未执行 |

只有交叉编译相关判据已完成。后续若要声明平台交付完成，必须在相应目标设备上补做原生运行和产品设计要求的场景验收，并分别报告通过项与未验证项。

## 所有权与证据

本目标新增或维护的文件只有：

- `docs/goals/platform-build.md`；
- `docs/verification/platform-build.md`；
- `output/acceptance/platform-build/20261012/` 下的构建日志、命令、清单、结果和两个可执行文件。

源码身份依据 [source-fingerprint.json](../../output/acceptance/current-package-regression/visual-refinement-confirmed/source-fingerprint.json)。构建完成后的核对结果见 [source-verification.json](../../output/acceptance/platform-build/20261012/source-verification.json)。
