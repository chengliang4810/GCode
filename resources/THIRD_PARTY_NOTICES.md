# GCode 第三方声明

本文件随 GCode 应用分发，登记 Go 依赖与原生终端资源的来源、固定版本、许可和版权归属。完整原始许可文本位于同目录的 [licenses/](licenses/)，未改写许可条款。核验日期：2026-10-12。

GCode 接管 zero 提交 `99721c762f37cd43ac511007a5f51d1846df959e`，随后独立维护。该源码采用 MIT 许可，版权为 Copyright (c) 2026 Gitlawb；完整许可见 [zero 源码许可](licenses/zero@99721c762f37cd43ac511007a5f51d1846df959e-LICENSE)。其原始文本 SHA-256 为 `67b7a613596a56e90712dd5a8450a4507ec3cb5a6db09425cf7a72a4b0ec89b7`。

## Go modules

以下条目覆盖当前应用的生产依赖、平台条件依赖以及已声明的测试或工具依赖。Go 标准库和操作系统框架不作为 Go module 单独登记。

| 模块与固定版本 | 构建范围 | 许可与版权归属 | 原始许可文本 |
|---|---|---|---|
| `github.com/Microsoft/go-winio v0.6.2` | Windows 条件图 | MIT；Copyright (c) 2015 Microsoft | [`github.com-Microsoft-go-winio@v0.6.2-LICENSE`](licenses/github.com-Microsoft-go-winio@v0.6.2-LICENSE) |
| `github.com/alecthomas/chroma/v2 v2.27.0` | 当前生产图 | MIT 文本；Copyright (C) 2017 Alec Thomas；同一原文还包含 Liberation Mono 字体的 SIL Open Font License 1.1 条款及其版权声明 | [`github.com-alecthomas-chroma-v2@v2.27.0-COPYING`](licenses/github.com-alecthomas-chroma-v2@v2.27.0-COPYING) |
| `github.com/aymanbagabas/go-udiff v0.4.1` | 当前生产图 | BSD 条款含 The Go Authors 版权；MIT 条款含 Copyright (c) 2023 Ayman Bagabas | [`github.com-aymanbagabas-go-udiff@v0.4.1-LICENSE-BSD`](licenses/github.com-aymanbagabas-go-udiff@v0.4.1-LICENSE-BSD)、[`github.com-aymanbagabas-go-udiff@v0.4.1-LICENSE-MIT`](licenses/github.com-aymanbagabas-go-udiff@v0.4.1-LICENSE-MIT) |
| `github.com/coder/websocket v1.8.15` | 已声明；当前生产图未出现 | Coder 2025 许可文本；原文件没有 SPDX 标题，完整条款以原文为准 | [`github.com-coder-websocket@v1.8.15-LICENSE.txt`](licenses/github.com-coder-websocket@v1.8.15-LICENSE.txt) |
| `github.com/dlclark/regexp2/v2 v2.5.0` | Chroma 间接依赖 | MIT；Copyright (c) Doug Clark | [`github.com-dlclark-regexp2-v2@v2.5.0-LICENSE`](licenses/github.com-dlclark-regexp2-v2@v2.5.0-LICENSE) |
| `github.com/ebitengine/purego v0.11.1` | MyGo 间接依赖 | Apache License 2.0 | [`github.com-ebitengine-purego@v0.11.1-LICENSE`](licenses/github.com-ebitengine-purego@v0.11.1-LICENSE) |
| `github.com/egoist/mygo v0.4.3-0.20261011132240-65154ea8777c` | 当前生产图；`go.mod` 通过 `replace` 使用 `third_party/mygo`；`ui` 和 terminal plugin | MIT；Copyright (c) 2026 MyGo contributors；仓内 fork 的 `LICENSE` 与登记原文一致 | [`github.com-egoist-mygo@v0.4.3-0.20261011132240-65154ea8777c-LICENSE`](licenses/github.com-egoist-mygo@v0.4.3-0.20261011132240-65154ea8777c-LICENSE) |
| `github.com/go-quicktest/qt v1.102.0` | 已声明间接依赖；当前生产图未出现 | MIT；Copyright (c) 2017 Canonical Ltd. | [`github.com-go-quicktest-qt@v1.102.0-LICENSE`](licenses/github.com-go-quicktest-qt@v1.102.0-LICENSE) |
| `github.com/go-text/typesetting v0.3.5` | MyGo 间接依赖 | Unlicense OR BSD-3-Clause；原文版权为 The go-text authors | [`github.com-go-text-typesetting@v0.3.5-LICENSE`](licenses/github.com-go-text-typesetting@v0.3.5-LICENSE) |
| `github.com/go-text/typesetting/harfbuzz`（随 `v0.3.5` 模块目录） | 模块缓存中的嵌套源码许可；当前 macOS 生产图未列出该 package | MIT；原文保留 B. KUGLER、Google、Ebrahim Byagowi、Facebook、Mozilla Foundation、Codethink、Nokia、Keith Stribley、Martin Hosken、SIL International、Chris Wilson、Behdad Esfahbod、David Turner、Red Hat 等版权行 | [`github.com-go-text-typesetting-harfbuzz@v0.3.5-LICENSE`](licenses/github.com-go-text-typesetting-harfbuzz@v0.3.5-LICENSE) |
| `github.com/ledongthuc/pdf v0.0.0-20250511090121-5959a4027728` | 当前生产图 | BSD-3-Clause；Copyright (c) 2009 The Go Authors | [`github.com-ledongthuc-pdf@v0.0.0-20250511090121-5959a4027728-LICENSE`](licenses/github.com-ledongthuc-pdf@v0.0.0-20250511090121-5959a4027728-LICENSE) |
| `github.com/rogpeppe/go-internal v1.15.0` | 已声明间接依赖；当前生产图未出现 | BSD-3-Clause；Copyright (c) 2018 The Go Authors | [`github.com-rogpeppe-go-internal@v1.15.0-LICENSE`](licenses/github.com-rogpeppe-go-internal@v1.15.0-LICENSE) |
| `github.com/yuin/goldmark/v2 v2.1.6` | 当前生产图；原生 Markdown AST 解析 | MIT；Copyright (c) 2019 Yusuke Inuzuka | [`goldmark-LICENSE.txt`](licenses/goldmark-LICENSE.txt) |
| `golang.org/x/image v0.46.0` | MyGo 间接依赖 | BSD-3-Clause；Copyright 2009 The Go Authors；另含 Google 的 Additional IP Rights Grant | [`golang.org-x-image@v0.46.0-LICENSE`](licenses/golang.org-x-image@v0.46.0-LICENSE)、[`golang.org-x-image@v0.46.0-PATENTS`](licenses/golang.org-x-image@v0.46.0-PATENTS) |
| `golang.org/x/sys v0.48.0` | 当前生产图及平台条件图 | BSD-3-Clause；Copyright 2009 The Go Authors；另含 Google 的 Additional IP Rights Grant | [`golang.org-x-sys@v0.48.0-LICENSE`](licenses/golang.org-x-sys@v0.48.0-LICENSE)、[`golang.org-x-sys@v0.48.0-PATENTS`](licenses/golang.org-x-sys@v0.48.0-PATENTS) |
| `mvdan.cc/sh/v3 v3.13.1` | 当前生产图 | BSD-3-Clause；Copyright (c) 2016, Daniel Martí | [`mvdan.cc-sh-v3@v3.13.1-LICENSE`](licenses/mvdan.cc-sh-v3@v3.13.1-LICENSE) |

## Nerd Fonts SymbolsOnly

GCode 原生 terminal 使用 Nerd Fonts v3.5.1 SymbolsOnly 的 `SymbolsNerdFontMono-Regular.ttf`。固定 release commit 为 `b894ea7803af6aade63d60a4381e006098ec9c4d`，release asset `NerdFontsSymbolsOnly.tar.xz` 的 SHA-256 为 `01172f37db8543edb102e5cb5c64101c9f4686630804d49b419aa07b23a69996`，目标字体 SHA-256 为 `fe471e538392f51910faab985fa8e192a39dd3426125edd15b71b3680df0e749`。SymbolsOnly 自带 MIT 文本和 `Copyright (c) 2014 Ryan L McIntyre`，见 [Nerd Fonts SymbolsOnly LICENSE](licenses/nerd-fonts/nerd-fonts-symbols-only-v3.5.1-LICENSE)。

SymbolsOnly 的字体文件还包含 Nerd Fonts glyph source README 列出的图标集合，不能把所有图标来源压缩声明为上述 MIT。固定版本、commit、blob、版权、NOTICE 检查和每个来源的原始许可文本见 [Nerd Fonts 来源清单](licenses/nerd-fonts/SOURCE_MANIFEST.md)。随应用资源携带的核心许可材料如下：

| 图标来源 | 固定版本 | 许可与归属 | 原始材料 |
| --- | --- | --- | --- |
| Codicons | `0.0.45` | CC BY 4.0；Microsoft/VS Code Codicons 及贡献者；完整上游文本另含 Git logo 的 CC BY 3.0 归属 | [LICENSE](licenses/nerd-fonts/codicons-v0.0.45-LICENSE)、[LICENSE-CODE](licenses/nerd-fonts/codicons-v0.0.45-LICENSE-CODE) |
| Devicons | `2.17.0` | MIT；`Copyright (c) 2015 konpa`；产品 logo 仍归各自权利人 | [LICENSE](licenses/nerd-fonts/devicon-v2.17.0-LICENSE)、[品牌说明](licenses/nerd-fonts/devicon-v2.17.0-README.md) |
| extraglyphs / Hack | `v3.003` | MIT；Source Foundry Authors；同时含 DejaVu 公共领域和 Bitstream Vera License/保留名称 | [LICENSE](licenses/nerd-fonts/hack-v3.003-LICENSE.md) |
| Font Awesome | `6.5.1` | 图标 CC BY 4.0；字体 OFL 1.1、Reserved Font Name `Font Awesome`；代码 MIT；Fonticons, Inc. 及品牌商标提示 | [LICENSE.txt](licenses/nerd-fonts/font-awesome-v6.5.1-LICENSE.txt) |
| Font Awesome Extension | `0.0.3` | MIT；`Copyright (c) 2017 André Luiz Gava` | [LICENCE](licenses/nerd-fonts/font-awesome-extension-v0.0.3-LICENCE) |
| Font Logos | `1.3.0` | Unlicense；Lukas W；品牌 logo 仍是各自权利人的商标 | [UNLICENSE](licenses/nerd-fonts/font-logos-v1.3.0-UNLICENSE)、[README](licenses/nerd-fonts/font-logos-v1.3.0-README.md) |
| MaterialDesign | `2022-10-06` 快照 | Pictogrammers Free License；字体和图标 Apache 2.0，代码 MIT；无独立 NOTICE | [LICENSE](licenses/nerd-fonts/material-design-font-2022-10-06-LICENSE)、[Apache-2.0](licenses/nerd-fonts/Apache-2.0.txt) |
| Octicons | `18.3.0` | MIT；`Copyright (c) 2023 GitHub Inc.`；GitHub logo 按商标规则使用 | [LICENSE](licenses/nerd-fonts/octicons-v18.3.0-LICENSE) |
| Seti and original | `0.8.1` | MIT；`Copyright (c) 2014 Jesse Weed` | [LICENSE](licenses/nerd-fonts/seti-ui-v0.8.1-LICENSE.md) |
| Pomicons | `1.001` | OFL 1.1；`Copyright (c) 2021, Gabriele Lana`；Reserved Font Name `Pomicons` | [LICENSE](licenses/nerd-fonts/pomicons-v1.001-LICENSE)、[OFL-1.1](licenses/nerd-fonts/OFL-1.1.txt) |
| Powerline Extra | `1.200` | MIT；`Copyright (c) 2016 Ryan L McIntyre`；Nerd Fonts 对 glyph 有修改 | [LICENSE](licenses/nerd-fonts/powerline-extra-symbols-v1.200-LICENSE) |
| Powerline Symbols | `1.000 (ca 2013)` | 上游 LICENSE 为 MIT；`Copyright 2013 Kim Silkebækken and other contributors`；Nerd audit 的 `Free License` 标记以此原文校正 | [LICENSE](licenses/nerd-fonts/powerline-v1.000-LICENSE) |
| Power Symbols IEC | `Feb 2015` | MIT；`Copyright (c) 2013 Joe Loughry` | [LICENSE](licenses/nerd-fonts/power-symbols-iec-2015-LICENSE.txt) |
| Weather Icons | `2.0.10 (1.100)` | 字体 OFL 1.1；设计归 Lukas Bischoff，v1.1 后图稿/代码归 Erik Flowers；上游没有独立 LICENSE/OFL 文件，Nerd Fonts OFL 副本含占位版权行 | [README](licenses/nerd-fonts/weather-icons-v2.0.10-README.md)、[Nerd OFL](licenses/nerd-fonts/nerd-fonts-weather-icons-v2.0.10-OFL.txt)、[OFL-FAQ](licenses/nerd-fonts/nerd-fonts-weather-icons-OFL-FAQ.txt)、[OFL-1.1](licenses/nerd-fonts/OFL-1.1.txt) |

CC BY 来源需要归属、许可文本或链接、修改说明并避免暗示背书；Apache 来源需要保留版权/归属、许可和显著修改说明；OFL 来源需要保留完整 OFL 与 Reserved Font Name 约束；开源许可证不授予 Font Logos、Devicons、Font Awesome 或 Octicons 品牌商标授权。

## MyGo fork

GCode 使用仓内 MyGo fork，固定源码基线为 `65154ea8777cf199dc6d6bcdbda4c206830a90ba`，声明版本为 `v0.4.3-0.20261011132240-65154ea8777c`。fork 保留原 MIT 许可，修改包括 Darwin PTY master 的阻塞打开方式和 Darwin shell 的 UTF-8 locale 规范化；这些改动未改变许可条款。MyGo 根许可 SHA-256 为 `f0b56f4ab53beb6e18c5e155219c2e2b6000b5475c0ae3ddb3af3894fd9349b4`。

## Ghostty 与终端主题

MyGo terminal 使用 Ghostty `libghostty-vt`，固定源代码提交为 `befcdfd2c3a1cb24d9ec886e93c95b2b5daa7028`。源码见 [Ghostty 固定提交](https://github.com/ghostty-org/ghostty/tree/befcdfd2c3a1cb24d9ec886e93c95b2b5daa7028)。该源码采用 MIT 许可，版权为 Copyright (c) 2024 Mitchell Hashimoto, Ghostty contributors；完整原文见 [Ghostty 许可](licenses/github.com-ghostty-org-ghostty@befcdfd2c3a1cb24d9ec886e93c95b2b5daa7028-LICENSE)。

Darwin amd64 原始库由 [MyGo 固定发布资源](https://github.com/egoist/mygo/releases/download/libghostty-vt-befcdfd2c3a1/libghostty-vt-darwin-amd64.dylib) 提供。manifest 固定未签名原始文件 SHA-256 为 `421e98d2683b39555209c59395655b4a217da0382e7669c92f6dff3949832664`，文件大小为 2,071,848 字节。本地 ad hoc 签名后文件大小为 2,106,320 字节，SHA-256 为 `80144a339496a9f6035c21734b57e3936f0c5971fddb5d57a1dfa4663f17c096`，CDHash 为 `3471fbc84b9632ab6f7605c4eeaee8e48d7c634e`。原始文件按 MyGo 构建参数签名后已与当前分发库逐字节比对一致；重新使用其他签名身份会改变分发文件 hash。

终端主题来自 iTerm2-Color-Schemes，版权为 Copyright (c) 2011 to Present Mark Badolato。各单独主题仍归其作者所有；完整原文见 [终端主题许可](licenses/github.com-egoist-mygo@v0.4.3-0.20261011132240-65154ea8777c-terminal-themes-LICENSE)。

应用资源仅携带当前依赖与资源对应的许可材料，没有包含开发者机器的缓存路径或内部验收记录。

