# /goal：更新机器默认 Go 工具链

状态：已完成。

## 目标与边界

把本机由 Homebrew 管理的默认 Go 从 `1.26.5` 更新到官方当前最新稳定版。仅操作 Homebrew 的 `go` formula 和本文件，不修改 GCode 的 `go.mod`、源码或其他软件；不使用项目内自动下载的 toolchain 作为本机安装的替代证明。

## 事实与决策

2026-10-11（Asia/Shanghai）通过 Go 官方实时下载索引核实：

```text
version: go1.27.2
stable: true
filename: go1.27.2.darwin-amd64.tar.gz
sha256: 587b59182488b23aa6e5fc25110405a3e0e5b38ed2f5b2f46ed13c32aee356fe
size: 71695284
```

来源：[Go 官方下载索引](https://go.dev/dl/?mode=json)。本机为 Intel `darwin/amd64`，因此目标版本和架构匹配。

Homebrew 元数据更新后已识别 `go` stable `1.27.2`，所以继续沿用现有 Homebrew 管理方式执行：

```sh
HOMEBREW_NO_AUTO_UPDATE=1 \
HTTPS_PROXY=http://127.0.0.1:7890 \
HTTP_PROXY=http://127.0.0.1:7890 \
/usr/local/bin/brew upgrade go
```

Homebrew 因本机 macOS 14 配置没有可用 bottle，从源码构建完成：`/usr/local/Cellar/go/1.27.2`，耗时 4 分 34 秒；随后移除旧的 `/usr/local/Cellar/go/1.26.5`。升级命令只选择了 `go` formula，未升级其他用户 formula。此前首次 Homebrew 查询触发了 Homebrew 自身的自动更新，并刷新了内部 Portable Ruby；这是 Homebrew 运行所需的内部组件变化，已在本记录中明确保留。

## 验收证据

Homebrew 当前状态：

```text
go 1.27.2
/usr/local/bin/go -> ../Cellar/go/1.27.2/bin/go
/usr/local/opt/go -> ../Cellar/go/1.27.2
```

新 zsh 中执行：

```sh
zsh -lc 'type -a go; GOTOOLCHAIN=local go version; GOTOOLCHAIN=local go env GOROOT GOARCH GOOS'
```

结果：

```text
go is /usr/local/bin/go
go version go1.27.2 darwin/amd64
/usr/local/Cellar/go/1.27.2/libexec
amd64
darwin
```

使用临时程序验证本机编译器和运行时，而不是 `GOTOOLCHAIN=auto` 下载的版本：

```sh
GOTOOLCHAIN=local go build -o /tmp/go-toolchain-smoke/smoke /tmp/go-toolchain-smoke/main.go
/tmp/go-toolchain-smoke/smoke
```

结果为：

```text
go-smoke sum=6
```

临时验证文件已删除。项目源码、`go.mod` 和 `go.sum` 未因本目标修改。
