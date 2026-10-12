# /goal：GCode 原生终端 PTY 闭环

状态：macOS Intel 当前环境的终端闭环已通过；分发资源来源和其他平台仍未验证。

## 目标与边界

验证 GCode 当前 macOS Intel 原生终端从 `terminal.Load`、shell 启动、输入、工作目录、resize、正常退出到 `Close` 后进程释放的真实闭环，并修复已经定位的 Darwin PTY master 状态问题。本记录不把 Apple Silicon、Windows 或最终分发资源来源的结论扩大到当前证据之外。

环境：2026-10-12，macOS 14.8.7，`darwin/amd64`，Go 1.27.2。

## 失败证据与根因

上游 MyGo 版本为 `v0.4.3-0.20261011132240-65154ea8777c`，参考提交为 `65154ea8777cf199dc6d6bcdbda4c206830a90ba`。未修补的 module cache 使用 `os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)` 打开 Darwin PTY master。实际结果是：

- [mygo-pty-baseline.log](../../output/acceptance/stage-4/terminal/mygo-pty-baseline.log)：`TestStart`、`TestResizeAndInput` 各超时 10 秒。
- [mygo-terminal-baseline.log](../../output/acceptance/stage-4/terminal/mygo-terminal-baseline.log)：shell prompt、paste、close 等场景超时；`TestExit` 和视图尺寸场景以 exit code `129` 结束。

在 macOS 上，`os.OpenFile` 会使该 master 进入 Go runtime/kqueue 轮询路径，读取可能返回 `EAGAIN`。`plugins/terminal/terminal.go` 的 `read` 对非 nil read error 结束读取；随后终端把连接视为结束并关闭 PTY，shell 收到 `SIGHUP`，最终表现为 129。修补前后的同一组测试结果已经复现实证这条因果链，而不是只依据源码推断。

## 最小修复

将 MyGo 复制到仓内受控 fork：[`third_party/mygo`](../../third_party/mygo)，并在根 [`go.mod`](../../go.mod) 使用相对替换：

```go
replace github.com/egoist/mygo => ./third_party/mygo
```

仅修改 [`pty_darwin.go`](../../third_party/mygo/plugins/terminal/internal/pty/pty_darwin.go)：使用 `syscall.Open` 获得文件描述符，再用 `os.NewFile` 构造 `*os.File`，保留 Darwin PTY master 的阻塞状态。没有修改上游仓库、module cache 或 GCode 的 `internal/desktop` 终端调用路径。

修补 fork 的定向测试：

```sh
cd third_party/mygo
GOTOOLCHAIN=local go test -count=1 \
  ./plugins/terminal/internal/pty ./plugins/terminal
```

结果：[mygo-pty-patched.log](../../output/acceptance/stage-4/terminal/mygo-pty-patched.log) 和 [mygo-terminal-patched.log](../../output/acceptance/stage-4/terminal/mygo-terminal-patched.log) 均通过；后者耗时约 4.7 秒。

## 真实 GCode 终端验收

驱动为 [`terminal-qa.go`](../../output/acceptance/stage-4/terminal/terminal-qa.go)，通过根 module 运行并显式指定当前打包 GCode.app 内的 `libghostty-vt.dylib`，关闭 cache fallback：

```sh
GOTOOLCHAIN=local \
MYGO_GHOSTTY_VT="$PWD/build/darwin-amd64/GCode.app/Contents/Resources/libghostty-vt.dylib" \
go run ./output/acceptance/stage-4/terminal/terminal-qa.go \
	-log "$PWD/output/acceptance/stage-4/terminal/terminal-qa-unicode.log"
```

日志：[terminal-qa-unicode.log](../../output/acceptance/stage-4/terminal/terminal-qa-unicode.log)。结果为：

```text
terminal_load=ok
shell_cwd=pass
shell_input=pass
shell_unicode=pass
resize_1=pass cols=80 rows=24
resize_2=pass cols=120 rows=40
resize_3=pass cols=40 rows=10
resize_4=pass cols=100 rows=30
shell_exit_done=true exit_code=0
close_done=true elapsed=2.001s
close_case_pid_after=gone
result=PASS
```

该验收使用临时真实目录，验证了 shell cwd、ASCII 输入和输出 marker、四组 PTY resize、shell 正常退出、`Done` 关闭以及 `Close` 后 shell PID 释放。Unicode 链路的单独验收和修复见下一节。

## UTF-8 输入、VT cell 和复制验收

进一步的低层验收区分了 PTY、Ghostty VT 和 shell line editor：

- [mygo-pty-unicode.log](../../output/acceptance/stage-4/terminal/mygo-pty-unicode.log) 中，强制 `LANG=en_US.UTF-8`、`LC_CTYPE=en_US.UTF-8` 后，PTY 原始输出保留了完整 UTF-8 字节；PTY 没有把中文或 emoji 改成连字符。
- [mygo-vt-unicode.log](../../output/acceptance/stage-4/terminal/mygo-vt-unicode.log) 通过直接 `Terminal.Write` 验证 `Text()`、`RowText`、UTF-8 有效性、CJK/emoji 的 `WideChar` 与 `SpacerTail`、emoji/组合字符的 grapheme cell，以及 `SelectAll`/`SelectionText` 的完整 UTF-8 结果。CJK 和 emoji 的 cell/ABI 读取正常。
- [mygo-terminal-unicode.log](../../output/acceptance/stage-4/terminal/mygo-terminal-unicode.log) 通过真实 `/bin/sh -i` PTY 输入 literal UTF-8 `printf` 命令，shell 回显和输出均完整保留中文、emoji、ZWJ family 和组合重音；同一组 MyGo `View` 测试通过 `selectAll`/`copy` 验证中文选区复制，并在 `Resize(40, 6)` 后验证 Unicode 文本和后续输出未丢失。
- [terminal-qa-unicode.log](../../output/acceptance/stage-4/terminal/terminal-qa-unicode.log) 在当前打包 GCode.app 资源上通过 `shell_unicode=pass`，并与 cwd、输入、resize、退出和 PID 释放一起得到 `result=PASS`。

原先看到的 `--------` 不是 `Text()` 解码、VT cell ABI 或 PTY 字节损坏。当前 macOS 进程环境带有 `LC_ALL=C.UTF-8`/`LC_CTYPE=C.UTF-8`；macOS 将这个 locale 映射为 `US-ASCII`，Bash/sh 的交互 line editor 因此把 literal UTF-8 输入当成 meta-key 字节，输出回车、退格、连字符和提示状态。ASCII shell `printf` 使用八进制转义时，输出路径本来就能正确显示 UTF-8，这也是此前错误归因到 `Text()` 的原因。

修复位于受控 fork 的 [`terminal.go`](../../third_party/mygo/plugins/terminal/terminal.go)：在 Darwin 上仅针对 `C.UTF-8`/`C.UTF8` 的 `LANG`、`LC_ALL`、`LC_CTYPE`，将 locale 设为系统可用的 `en_US.UTF-8`；明确的其他 locale 保持不变。该修复让 shell 输入、PTY 回显、Ghostty VT cell、`Text()` 和选区复制形成同一条可验证链路。

## 独立的原生资源问题

当前打包资源实测：

```text
build/darwin-amd64/GCode.app/Contents/Resources/libghostty-vt.dylib
sha256=80144a339496a9f6035c21734b57e3936f0c5971fddb5d57a1dfa4663f17c096
```

MyGo terminal manifest 为 Darwin amd64 资源声明的 hash 为：

```text
sha256=421e98d2683b39555209c59395655b4a217da0382e7669c92f6dff3949832664
```

这个 manifest hash 对应的是未签名的原始 cache 文件，大小为 `2071848` 字节。MyGo 实际对应用内嵌资源执行的 ad hoc 签名命令是：

```text
codesign --force --sign - --preserve-metadata=entitlements
```

签名后文件大小为 `2106320` 字节，SHA-256 为：

```text
sha256=80144a339496a9f6035c21734b57e3936f0c5971fddb5d57a1dfa4663f17c096
```

该签名复现结果与当前打包文件逐字节一致，CDHash 均为 `3471fbc84b9632ab6f7605c4eeaee8e48d7c634e`。因此日志中的 `manifest_hash_match=false` 是原始资源到签名后分发文件的预期 hash 变化，不是构建/分发断链；从 manifest 原始资源到当前包内 dylib 的来源已经闭环。完整命令、尺寸、hash 和 `cmp` 证据见 [`verification.txt`](../../output/acceptance/stage-5/native-provenance/verification.txt)，相关声明见 [`THIRD_PARTY_NOTICES.md`](../THIRD_PARTY_NOTICES.md)。

## 平台范围

本记录只支持当前 macOS 14.8.7 Intel `darwin/amd64` 的真实 PTY 结论。没有 Windows 或 Apple Silicon 的构建、原生运行和终端证据；这些平台保持“未验证”。

早期共享工作区曾因 worktrees 和 desktop 的并行改动出现编译错误：`internal/worktrees/manager.go` 缺少 `GitRunner`、`gitOutput` 等定义，`internal/desktop/app_test.go` 使用了当时 `ui.Color` 不存在的 `RGBA` 方法。该记录属于旧时点证据，不代表当前状态；当前最终全仓回归结果应以 root 最新验证为准。本目标的 MyGo 定向测试和根模块真实终端驱动均已通过。
