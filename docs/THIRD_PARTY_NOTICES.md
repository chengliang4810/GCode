# GCode 第三方声明

本文件为开发核验记录。随应用分发的可移植声明位于 [`resources/THIRD_PARTY_NOTICES.md`](../resources/THIRD_PARTY_NOTICES.md)，其许可链接使用应用资源目录内的 `licenses/`；本记录中的缓存路径和验收文件不随应用打包。MyGo 自动打包 `resources/` 目录，`mygo.json` 只额外携带根 `LICENSE`。

本文件登记当前 GCode 分发中实际构建依赖和终端原生资源的版本、许可、版权归属及原始许可文本位置。原始文本保存在 [`resources/licenses/`](../resources/licenses/)；文件内容按来源复制，未改写许可条款。

核验日期：2026-10-12（Asia/Shanghai）。Go module 缓存根目录：`/Users/chengliang/go/pkg/mod`。模块版本以仓库的 [`go.mod`](../go.mod) 和 `go.sum` 为准。

## 核验范围

生产依赖图通过以下命令核对：

```sh
GOCACHE=/tmp/gcode-go-cache-intake go list -deps -f '{{if .Module}}{{.Module.Path}} {{.Module.Version}}{{end}}' ./cmd/gcode
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
GOCACHE=/tmp/gcode-go-cache-intake \
go list -deps ./cmd/gcode
```

当前 macOS 入口图包含 Chroma、go-udiff、regexp2、purego、MyGo、go-text/typesetting、ledongthuc/pdf、x/image、x/sys 和 mvdan.cc/sh。MyGo 由 `go.mod` 的 `replace github.com/egoist/mygo => ./third_party/mygo` 解析到仓内 fork，声明版本仍为 `v0.4.3-0.20261011132240-65154ea8777c`。Windows 条件图额外使用 go-winio。`go.mod` 还声明了 coder/websocket、go-quicktest/qt 和 go-internal；截至本次核验，它们没有出现在当前 GCode 生产包图中，但仍保留其固定版本的原始许可文本，以覆盖现有模块声明。Go 标准库和系统框架不作为 Go module 另行登记。

## Go modules

| 模块与固定版本 | 构建范围 | 许可与版权归属 | 原始许可文本 |
|---|---|---|---|
| `github.com/Microsoft/go-winio v0.6.2` | Windows 条件图 | MIT；Copyright (c) 2015 Microsoft | [`github.com-Microsoft-go-winio@v0.6.2-LICENSE`](../resources/licenses/github.com-Microsoft-go-winio@v0.6.2-LICENSE) |
| `github.com/alecthomas/chroma/v2 v2.27.0` | 当前生产图 | MIT 文本；Copyright (C) 2017 Alec Thomas；同一原文还包含 Liberation Mono 字体的 SIL Open Font License 1.1 条款及其版权声明 | [`github.com-alecthomas-chroma-v2@v2.27.0-COPYING`](../resources/licenses/github.com-alecthomas-chroma-v2@v2.27.0-COPYING) |
| `github.com/aymanbagabas/go-udiff v0.4.1` | 当前生产图 | BSD 条款含 The Go Authors 版权；MIT 条款含 Copyright (c) 2023 Ayman Bagabas | [`github.com-aymanbagabas-go-udiff@v0.4.1-LICENSE-BSD`](../resources/licenses/github.com-aymanbagabas-go-udiff@v0.4.1-LICENSE-BSD)、[`github.com-aymanbagabas-go-udiff@v0.4.1-LICENSE-MIT`](../resources/licenses/github.com-aymanbagabas-go-udiff@v0.4.1-LICENSE-MIT) |
| `github.com/coder/websocket v1.8.15` | 已声明；当前生产图未出现 | Coder 2025 许可文本；原文件没有 SPDX 标题，完整条款以原文为准 | [`github.com-coder-websocket@v1.8.15-LICENSE.txt`](../resources/licenses/github.com-coder-websocket@v1.8.15-LICENSE.txt) |
| `github.com/dlclark/regexp2/v2 v2.5.0` | Chroma 间接依赖 | MIT；Copyright (c) Doug Clark | [`github.com-dlclark-regexp2-v2@v2.5.0-LICENSE`](../resources/licenses/github.com-dlclark-regexp2-v2@v2.5.0-LICENSE) |
| `github.com/ebitengine/purego v0.11.1` | MyGo 间接依赖 | Apache License 2.0 | [`github.com-ebitengine-purego@v0.11.1-LICENSE`](../resources/licenses/github.com-ebitengine-purego@v0.11.1-LICENSE) |
| `github.com/egoist/mygo v0.4.3-0.20261011132240-65154ea8777c` | 当前生产图；`go.mod` 通过 `replace` 使用 `third_party/mygo`；`ui` 和 terminal plugin | MIT；Copyright (c) 2026 MyGo contributors；仓内 fork 的 `LICENSE` 与登记原文一致 | [`github.com-egoist-mygo@v0.4.3-0.20261011132240-65154ea8777c-LICENSE`](../resources/licenses/github.com-egoist-mygo@v0.4.3-0.20261011132240-65154ea8777c-LICENSE) |
| `github.com/go-quicktest/qt v1.102.0` | 已声明间接依赖；当前生产图未出现 | MIT；Copyright (c) 2017 Canonical Ltd. | [`github.com-go-quicktest-qt@v1.102.0-LICENSE`](../resources/licenses/github.com-go-quicktest-qt@v1.102.0-LICENSE) |
| `github.com/go-text/typesetting v0.3.5` | MyGo 间接依赖 | Unlicense OR BSD-3-Clause；原文版权为 The go-text authors | [`github.com-go-text-typesetting@v0.3.5-LICENSE`](../resources/licenses/github.com-go-text-typesetting@v0.3.5-LICENSE) |
| `github.com/go-text/typesetting/harfbuzz`（随 `v0.3.5` 模块目录） | 模块缓存中的嵌套源码许可；当前 macOS 生产图未列出该 package | MIT；原文保留 B. KUGLER、Google、Ebrahim Byagowi、Facebook、Mozilla Foundation、Codethink、Nokia、Keith Stribley、Martin Hosken、SIL International、Chris Wilson、Behdad Esfahbod、David Turner、Red Hat 等版权行 | [`github.com-go-text-typesetting-harfbuzz@v0.3.5-LICENSE`](../resources/licenses/github.com-go-text-typesetting-harfbuzz@v0.3.5-LICENSE) |
| `github.com/ledongthuc/pdf v0.0.0-20250511090121-5959a4027728` | 当前生产图 | BSD-3-Clause；Copyright (c) 2009 The Go Authors | [`github.com-ledongthuc-pdf@v0.0.0-20250511090121-5959a4027728-LICENSE`](../resources/licenses/github.com-ledongthuc-pdf@v0.0.0-20250511090121-5959a4027728-LICENSE) |
| `github.com/rogpeppe/go-internal v1.15.0` | 已声明间接依赖；当前生产图未出现 | BSD-3-Clause；Copyright (c) 2018 The Go Authors | [`github.com-rogpeppe-go-internal@v1.15.0-LICENSE`](../resources/licenses/github.com-rogpeppe-go-internal@v1.15.0-LICENSE) |
| `github.com/yuin/goldmark/v2 v2.1.6` | 当前生产图；原生 Markdown AST 解析 | MIT；Copyright (c) 2019 Yusuke Inuzuka | [`goldmark-LICENSE.txt`](../resources/licenses/goldmark-LICENSE.txt) |
| `golang.org/x/image v0.46.0` | MyGo 间接依赖 | BSD-3-Clause；Copyright 2009 The Go Authors；另含 Google 的 Additional IP Rights Grant | [`golang.org-x-image@v0.46.0-LICENSE`](../resources/licenses/golang.org-x-image@v0.46.0-LICENSE)、[`golang.org-x-image@v0.46.0-PATENTS`](../resources/licenses/golang.org-x-image@v0.46.0-PATENTS) |
| `golang.org/x/sys v0.48.0` | 当前生产图及平台条件图 | BSD-3-Clause；Copyright 2009 The Go Authors；另含 Google 的 Additional IP Rights Grant | [`golang.org-x-sys@v0.48.0-LICENSE`](../resources/licenses/golang.org-x-sys@v0.48.0-LICENSE)、[`golang.org-x-sys@v0.48.0-PATENTS`](../resources/licenses/golang.org-x-sys@v0.48.0-PATENTS) |
| `mvdan.cc/sh/v3 v3.13.1` | 当前生产图 | BSD-3-Clause；Copyright (c) 2016, Daniel Martí | [`mvdan.cc-sh-v3@v3.13.1-LICENSE`](../resources/licenses/mvdan.cc-sh-v3@v3.13.1-LICENSE) |

## Nerd Fonts SymbolsOnly

GCode 原生 terminal 使用 Nerd Fonts v3.5.1 SymbolsOnly 的 `SymbolsNerdFontMono-Regular.ttf`。固定 release commit 为 `b894ea7803af6aade63d60a4381e006098ec9c4d`，release asset `NerdFontsSymbolsOnly.tar.xz` 的 SHA-256 为 `01172f37db8543edb102e5cb5c64101c9f4686630804d49b419aa07b23a69996`，目标字体 SHA-256 为 `fe471e538392f51910faab985fa8e192a39dd3426125edd15b71b3680df0e749`。SymbolsOnly 自带 MIT 文本和 `Copyright (c) 2014 Ryan L McIntyre`，见 [Nerd Fonts SymbolsOnly LICENSE](../resources/licenses/nerd-fonts/nerd-fonts-symbols-only-v3.5.1-LICENSE)。

SymbolsOnly 的字体文件还包含 Nerd Fonts glyph source README 列出的图标集合，不能把所有图标来源压缩声明为上述 MIT。固定版本、commit、blob、版权、NOTICE 检查和每个来源的原始许可文本见 [Nerd Fonts 来源清单](../resources/licenses/nerd-fonts/SOURCE_MANIFEST.md)。随应用资源携带的核心许可材料如下：

| 图标来源 | 固定版本 | 许可与归属 | 原始材料 |
| --- | --- | --- | --- |
| Codicons | `0.0.45` | CC BY 4.0；Microsoft/VS Code Codicons 及贡献者；完整上游文本另含 Git logo 的 CC BY 3.0 归属 | [LICENSE](../resources/licenses/nerd-fonts/codicons-v0.0.45-LICENSE)、[LICENSE-CODE](../resources/licenses/nerd-fonts/codicons-v0.0.45-LICENSE-CODE) |
| Devicons | `2.17.0` | MIT；`Copyright (c) 2015 konpa`；产品 logo 仍归各自权利人 | [LICENSE](../resources/licenses/nerd-fonts/devicon-v2.17.0-LICENSE)、[品牌说明](../resources/licenses/nerd-fonts/devicon-v2.17.0-README.md) |
| extraglyphs / Hack | `v3.003` | MIT；Source Foundry Authors；同时含 DejaVu 公共领域和 Bitstream Vera License/保留名称 | [LICENSE](../resources/licenses/nerd-fonts/hack-v3.003-LICENSE.md) |
| Font Awesome | `6.5.1` | 图标 CC BY 4.0；字体 OFL 1.1、Reserved Font Name `Font Awesome`；代码 MIT；Fonticons, Inc. 及品牌商标提示 | [LICENSE.txt](../resources/licenses/nerd-fonts/font-awesome-v6.5.1-LICENSE.txt) |
| Font Awesome Extension | `0.0.3` | MIT；`Copyright (c) 2017 André Luiz Gava` | [LICENCE](../resources/licenses/nerd-fonts/font-awesome-extension-v0.0.3-LICENCE) |
| Font Logos | `1.3.0` | Unlicense；Lukas W；品牌 logo 仍是各自权利人的商标 | [UNLICENSE](../resources/licenses/nerd-fonts/font-logos-v1.3.0-UNLICENSE)、[README](../resources/licenses/nerd-fonts/font-logos-v1.3.0-README.md) |
| MaterialDesign | `2022-10-06` 快照 | Pictogrammers Free License；字体和图标 Apache 2.0，代码 MIT；无独立 NOTICE | [LICENSE](../resources/licenses/nerd-fonts/material-design-font-2022-10-06-LICENSE)、[Apache-2.0](../resources/licenses/nerd-fonts/Apache-2.0.txt) |
| Octicons | `18.3.0` | MIT；`Copyright (c) 2023 GitHub Inc.`；GitHub logo 按商标规则使用 | [LICENSE](../resources/licenses/nerd-fonts/octicons-v18.3.0-LICENSE) |
| Seti and original | `0.8.1` | MIT；`Copyright (c) 2014 Jesse Weed` | [LICENSE](../resources/licenses/nerd-fonts/seti-ui-v0.8.1-LICENSE.md) |
| Pomicons | `1.001` | OFL 1.1；`Copyright (c) 2021, Gabriele Lana`；Reserved Font Name `Pomicons` | [LICENSE](../resources/licenses/nerd-fonts/pomicons-v1.001-LICENSE)、[OFL-1.1](../resources/licenses/nerd-fonts/OFL-1.1.txt) |
| Powerline Extra | `1.200` | MIT；`Copyright (c) 2016 Ryan L McIntyre`；Nerd Fonts 对 glyph 有修改 | [LICENSE](../resources/licenses/nerd-fonts/powerline-extra-symbols-v1.200-LICENSE) |
| Powerline Symbols | `1.000 (ca 2013)` | 上游 LICENSE 为 MIT；`Copyright 2013 Kim Silkebækken and other contributors`；Nerd audit 的 `Free License` 标记以此原文校正 | [LICENSE](../resources/licenses/nerd-fonts/powerline-v1.000-LICENSE) |
| Power Symbols IEC | `Feb 2015` | MIT；`Copyright (c) 2013 Joe Loughry` | [LICENSE](../resources/licenses/nerd-fonts/power-symbols-iec-2015-LICENSE.txt) |
| Weather Icons | `2.0.10 (1.100)` | 字体 OFL 1.1；设计归 Lukas Bischoff，v1.1 后图稿/代码归 Erik Flowers；上游没有独立 LICENSE/OFL 文件，Nerd Fonts OFL 副本含占位版权行 | [README](../resources/licenses/nerd-fonts/weather-icons-v2.0.10-README.md)、[Nerd OFL](../resources/licenses/nerd-fonts/nerd-fonts-weather-icons-v2.0.10-OFL.txt)、[OFL-FAQ](../resources/licenses/nerd-fonts/nerd-fonts-weather-icons-OFL-FAQ.txt)、[OFL-1.1](../resources/licenses/nerd-fonts/OFL-1.1.txt) |

CC BY 来源需要归属、许可文本或链接、修改说明并避免暗示背书；Apache 来源需要保留版权/归属、许可和显著修改说明；OFL 来源需要保留完整 OFL 与 Reserved Font Name 约束；开源许可证不授予 Font Logos、Devicons、Font Awesome 或 Octicons 品牌商标授权。

## MyGo terminal 与 Ghostty 原生资源

当前生产图通过 `go.mod` 的 `replace github.com/egoist/mygo => ./third_party/mygo` 使用仓内 MyGo fork，声明版本为 `v0.4.3-0.20261011132240-65154ea8777c`。其 `plugins/terminal/mygo-plugin.json` 将 `libghostty-vt` 的源代码指向 Ghostty 提交 `befcdfd2c3a1cb24d9ec886e93c95b2b5daa7028`，并将 darwin-amd64 发布资源固定为：

```text
https://github.com/egoist/mygo/releases/download/libghostty-vt-befcdfd2c3a1/libghostty-vt-darwin-amd64.dylib
manifest SHA-256: 421e98d2683b39555209c59395655b4a217da0382e7669c92f6dff3949832664
source: https://github.com/ghostty-org/ghostty/tree/befcdfd2c3a1cb24d9ec886e93c95b2b5daa7028
```

这是 MyGo 清单声明的未签名原始资源来源。当前工作区应用资源
`.mygo/dev/darwin-amd64/GCode Dev.app/Contents/Resources/libghostty-vt.dylib`
的实测信息是：文件大小 `2106320` 字节，SHA-256 为
`80144a339496a9f6035c21734b57e3936f0c5971fddb5d57a1dfa4663f17c096`。
该值与清单预期值不同是因为清单资源原始文件未签名，而应用包内文件经过 MyGo 的嵌套 ad hoc 签名。已将原始缓存文件
`/Users/chengliang/Library/Caches/mygo/natives/421e98d2683b39555209c59395655b4a217da0382e7669c92f6dff3949832664/libghostty-vt.dylib`
复制到 [`output/acceptance/stage-5/native-provenance/`](../output/acceptance/stage-5/native-provenance/)，并按
`third_party/mygo/cmd/mygo/resources.go` 的 `nestedCodesignArgs("-", "", false)` 实际参数执行
`codesign --force --sign - --preserve-metadata=entitlements`。签名副本的 SHA-256 为
`80144a339496a9f6035c21734b57e3936f0c5971fddb5d57a1dfa4663f17c096`，与当前应用包文件逐字节一致；两者均为 ad hoc 签名，CDHash 均为
`3471fbc84b9632ab6f7605c4eeaee8e48d7c634e`。二进制内可读元数据显示 `library_version` 为
`0.1.0-dev`、`commit` 为 `null`、`dirty` 为 `null`。因此 hash 差异是原始资源到签名后分发文件的预期转换，当前包内 dylib 已有从 manifest 原始资源到分发文件的字节级复现证据；完整命令、尺寸和 hash 见 [`verification.txt`](../output/acceptance/stage-5/native-provenance/verification.txt)。

Ghostty 固定提交的 MIT 原始许可文本已单独保存，版权行是 `Copyright (c) 2024 Mitchell Hashimoto, Ghostty contributors`：[`github.com-ghostty-org-ghostty@befcdfd2c3a1cb24d9ec886e93c95b2b5daa7028-LICENSE`](../resources/licenses/github.com-ghostty-org-ghostty@befcdfd2c3a1cb24d9ec886e93c95b2b5daa7028-LICENSE)。本机没有对应的 Ghostty checkout 缓存；固定提交许可证来自上游原文，当前 dylib 的来源核验则由 MyGo manifest 原始 hash、实际签名参数和字节级复现共同支持。

MyGo terminal 还把 iTerm2-Color-Schemes 主题许可文本嵌入仓内 fork 的生成源码；该原文已保存为 [`github.com-egoist-mygo@v0.4.3-0.20261011132240-65154ea8777c-terminal-themes-LICENSE`](../resources/licenses/github.com-egoist-mygo@v0.4.3-0.20261011132240-65154ea8777c-terminal-themes-LICENSE)，版权行是 `Copyright (c) 2011 to Present Mark Badolato`。该条款说明各单独主题仍归其作者所有。

## 接管源码许可证

GCode 接管的 zero 固定提交为 `99721c762f37cd43ac511007a5f51d1846df959e`。仓库根目录 [`LICENSE`](../LICENSE) 与固定快照根目录的 MIT 原文一致，版权行是 `Copyright (c) 2026 Gitlawb`，SHA-256 为 `67b7a613596a56e90712dd5a8450a4507ec3cb5a6db09425cf7a72a4b0ec89b7`。同一原文另存为 [`zero@99721c762f37cd43ac511007a5f51d1846df959e-LICENSE`](../resources/licenses/zero@99721c762f37cd43ac511007a5f51d1846df959e-LICENSE)，便于分发材料独立携带。

## 分发边界

- 本目录的原始文本只覆盖当前 Go module 声明、当前生产依赖图及 MyGo terminal 资源；未把没有进入当前图的其他缓存 module 或系统组件冒充为 GCode 依赖。
- `resources/licenses/` 中的文件名包含模块、版本或固定提交；文档表格保留每个文件对应的缓存或固定来源。
- 当前生产图已通过根 `go.mod` 的 `replace github.com/egoist/mygo => ./third_party/mygo` 使用仓内相对 fork；声明版本仍为 `v0.4.3-0.20261011132240-65154ea8777c`。`third_party/mygo/LICENSE` 与 module cache 中的 MyGo `LICENSE` 以及本目录对应原文逐字一致（SHA-256：`f0b56f4ab53beb6e18c5e155219c2e2b6000b5475c0ae3ddb3af3894fd9349b4`），因此不再为这份相同文本建立另一套声明。若 fork 后续改变许可证，需按 fork 实际版本补充独立条目。
- 仓内 fork 的生产源码相对对应 module cache 已核验到 Darwin PTY master 打开方式修改：`plugins/terminal/internal/pty/pty_darwin.go` 使用 `syscall.Open` 和 `os.NewFile` 保持阻塞模式；该修改没有新增许可文件或改变 MyGo 根许可证。
