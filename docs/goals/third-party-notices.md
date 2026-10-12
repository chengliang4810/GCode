# /goal：第三方声明与分发许可

状态：已完成当前 Go module、MyGo terminal 原生资源和固定 zero 接管许可证的材料收集；已用 MyGo 实际嵌套签名参数复现当前工作区 dylib，确认 manifest 原始 hash 与分发 hash 的差异来自 ad hoc 签名转换。

## 目标

为 GCode 建立可随分发包携带的第三方声明和原始许可文本，覆盖当前生产依赖图、Windows 条件依赖、现有 `go.mod` 声明以及 MyGo terminal 使用的原生资源。保留 zero 固定接管提交的 MIT 原文，不改写第三方许可条款。

## 交付物

- [`docs/THIRD_PARTY_NOTICES.md`](../THIRD_PARTY_NOTICES.md)：版本、构建范围、版权、许可类型、来源和原文映射。
- [`resources/licenses/`](../../resources/licenses/)：按模块、版本或固定提交命名的原始许可文件。
- [`docs/source-origin.md`](../source-origin.md)：zero 来源记录补充当前分发许可材料、验证命令和 MyGo/Ghostty 来源边界。

## 固定来源和覆盖范围

- Go module 缓存：`/Users/chengliang/go/pkg/mod`。
- 版本来源：仓库 [`go.mod`](../../go.mod)、[`go.sum`](../../go.sum) 和模块缓存目录。
- 生产图核验：

  ```sh
  GOCACHE=/tmp/gcode-go-cache-intake go list -deps -f '{{if .Module}}{{.Module.Path}} {{.Module.Version}}{{end}}' ./cmd/gcode
  GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  GOCACHE=/tmp/gcode-go-cache-intake \
  go list -deps ./cmd/gcode
  ```

- 当前 macOS 生产图的模块为 Chroma、go-udiff、regexp2、purego、MyGo、go-text/typesetting、ledongthuc/pdf、x/image、x/sys 和 mvdan.cc/sh；其中 MyGo 通过根 `go.mod` 的 `replace github.com/egoist/mygo => ./third_party/mygo` 使用仓内 fork。Windows 条件图增加 go-winio。
- `go.mod` 中当前生产图未出现的 coder/websocket、go-quicktest/qt 和 go-internal 也保留了固定版本原文，因为它们仍是现有模块声明的一部分。
- go-text/typesetting 的 `harfbuzz/LICENSE`、x/image/x/sys 的 `PATENTS`、Chroma `COPYING` 中的字体许可以及 MyGo terminal 主题许可均作为独立或合并原文保留。
- zero 固定提交为 `99721c762f37cd43ac511007a5f51d1846df959e`；根目录 `LICENSE` 和 `resources/licenses/zero@99721c762f37cd43ac511007a5f51d1846df959e-LICENSE` 内容一致。

## libghostty-vt 边界

当前生产图使用仓内 `third_party/mygo` fork，声明版本为 `github.com/egoist/mygo v0.4.3-0.20261011132240-65154ea8777c`。其 terminal manifest 指向 Ghostty 提交 `befcdfd2c3a1cb24d9ec886e93c95b2b5daa7028`，darwin-amd64 资源预期 SHA-256 为 `421e98d2683b39555209c59395655b4a217da0382e7669c92f6dff3949832664`。

当前工作区资源 `.mygo/dev/darwin-amd64/GCode Dev.app/Contents/Resources/libghostty-vt.dylib` 实测 SHA-256 为 `80144a339496a9f6035c21734b57e3936f0c5971fddb5d57a1dfa4663f17c096`，大小为 `2106320` 字节；二进制字符串显示 `library_version=0.1.0-dev`、`commit=null`、`dirty=null`。manifest 原始缓存文件 SHA-256 为 `421e98d2683b39555209c59395655b4a217da0382e7669c92f6dff3949832664`，未签名大小为 `2071848` 字节。对原始缓存副本执行 `codesign --force --sign - --preserve-metadata=entitlements` 后，副本 SHA-256 为 `80144a339496a9f6035c21734b57e3936f0c5971fddb5d57a1dfa4663f17c096`，与当前包内文件逐字节一致；完整证据保存在 [`output/acceptance/stage-5/native-provenance/verification.txt`](../../output/acceptance/stage-5/native-provenance/verification.txt)。因此两个 hash 的差异是 ad hoc 签名转换，不再视为未知来源。

本机没有 Ghostty checkout 缓存。固定提交的原始 `LICENSE` 通过该提交的上游原文核验后保存，并在声明文档中标明这一来源差异。

## 协作边界

本目标没有修改 `go.mod`、`mygo.json`、构建配置、产品源代码或其他代理负责的文件。后续接入已将根 `go.mod` 的 MyGo 生产图切换到 `third_party/mygo` 相对 fork；其 `LICENSE` 与缓存 MyGo 原文及 `resources/licenses/` 对应文件逐字一致，SHA-256 为 `f0b56f4ab53beb6e18c5e155219c2e2b6000b5475c0ae3ddb3af3894fd9349b4`，因此不重复登记同一份文本。若 fork 后续改变许可证，应按 fork 实际版本补充独立声明。

## 验证记录

已核验：

- 许可文件均与对应版本的 Go module 缓存、仓内 MyGo fork，或明确固定的 zero/Ghostty 提交原文逐字核验。
- zero 根 `LICENSE`、仓库根 `LICENSE` 和资源副本的 SHA-256 一致。
- MyGo terminal manifest 的 Ghostty 固定提交、发布 URL、原始资源 hash、实际嵌套签名参数、签名后 hash 和当前应用资源 hash 已通过字节级复现核验。
- 原始许可文件内容未添加说明文字、版权重写或条款改写。

分发/构建流程如果改变嵌套签名参数，应重新执行上述复现核验；当前工作区产物已经完成从 manifest 原始资源到包内 dylib 的字节级来源核验。
