# Nerd Fonts SymbolsOnly v3.5.1 来源与许可证清单

本目录保存 GCode 可能随原生 terminal 分发的 Nerd Fonts SymbolsOnly 许可和归属材料。内容按来源复制，未改写许可条款；`SOURCE_MANIFEST.md` 是 GCode 的核验索引，不是上游许可文本。

核验日期：2026-10-12（Asia/Shanghai）

## 固定 Nerd Fonts 资产

- Nerd Fonts release：`v3.5.1`，release commit `b894ea7803af6aade63d60a4381e006098ec9c4d`。
- SymbolsOnly release asset：`NerdFontsSymbolsOnly.tar.xz`，SHA-256：`01172f37db8543edb102e5cb5c64101c9f4686630804d49b419aa07b23a69996`。
- 目标字体：`SymbolsNerdFontMono-Regular.ttf`，2,610,012 bytes，SHA-256：`fe471e538392f51910faab985fa8e192a39dd3426125edd15b71b3680df0e749`。
- SymbolsOnly 自带的原始许可证是 MIT，版权行为 `Copyright (c) 2014 Ryan L McIntyre`。该许可证只说明 SymbolsOnly 基础字体；图标来源的许可证和归属按下表另行保留。
- release asset 中实际包含 `LICENSE`、`README.md`、Regular 字体、Mono 字体和 `10-nerd-font-symbols.conf`；v3.5.1 资产没有 `SymbolsNerdFontPropo-Regular.ttf`。

Nerd Fonts 本身说明其 SymbolsOnly 只携带基础字体的 MIT 文本，图标来源的作者、许可证和仓库通过 glyph source README 链接提供。因此本目录同时保存 v3.5.1 的 glyph source README、license audit、Nerd Fonts 仓内许可证副本以及固定上游许可证，避免只保留 MIT 文本造成归属缺失。

## 图标来源固定身份

“上游许可证 blob”是固定上游 commit 中对应许可证文件的 Git blob SHA；“Nerd Fonts blob”是 `ryanoasis/nerd-fonts@b894ea7803af6aade63d60a4381e006098ec9c4d` 中实际被使用的许可证文件 blob SHA。所有 `NOTICE` 均检查了固定 commit 的仓库树；`无` 表示没有独立的顶层或路径名为 NOTICE 的文件。

| 图标集 | Nerd Fonts 版本 | 固定上游身份 | 许可证与必须保留的归属 | NOTICE | 本目录材料 |
| --- | --- | --- | --- | --- | --- |
| Codicons | `0.0.45` | `microsoft/vscode-codicons@dbb6555160650624bb1719d6264910b1cbe528f2`（tag `v0.0.45`）；`LICENSE` blob `35b538308f557d8eadbd38eb35269294cbea21c7`；`LICENSE-CODE` blob `3d8b93bc7987d14c848448c089e2ae15311380d7` | CC BY 4.0；Microsoft VS Code Codicons 及贡献者归属。完整上游 LICENSE 还包含 Git logo 的 CC BY 3.0 归属；保留 LICENSE-CODE 作为来源代码许可记录。 | 无 | `codicons-v0.0.45-LICENSE`、`codicons-v0.0.45-LICENSE-CODE`；v3.5.1 仓内副本 `nerd-fonts-codicons-v0.0.45-LICENSE.txt`（blob `085c3ca0c4920beedbf179f7987e705aec84115b`） |
| Devicons | `2.17.0` | `devicons/devicon@54cfe13ac10eaa1ef817a343ab0a9437eb3c2e08`（tag `v2.17.0`）；`LICENSE` blob `fcc9b6677248224a60d060ccc6851eb420fa003d` | MIT；`Copyright (c) 2015 konpa`。各产品名、logo 和品牌归其权利人，使用不表示背书；该限制和品牌说明来自固定 README。 | 无 | `devicon-v2.17.0-LICENSE`、`devicon-v2.17.0-README.md` |
| extraglyphs / Hack | Nerd Fonts 未给独立图标版本 | `source-foundry/Hack@b47d21f808fbf4784565feccde2213ef2af924b3`（tag `v3.003`）；`LICENSE.md` blob `08927e504fce744f928c584c6e4a07328f2e94ed` | Hack 部分 MIT；`Copyright (c) 2018 Source Foundry Authors`。同一文件还保留 DejaVu 公共领域说明和 Bitstream Vera License；Bitstream/Vera 是保留名称和商标。 | 无 | `hack-v3.003-LICENSE.md`；Nerd Fonts 对应副本 `nerd-fonts-hack-MIT-Bitstream-Vera-LICENSE.md` |
| Font Awesome | `6.5.1` | `FortAwesome/Font-Awesome@deeea78c52bfe00b6e251ffddccf5570d5fdb05e`（tag `6.5.1`）；`LICENSE.txt` blob `39e18e3d30767f5a321f87d338f911906aa88da9` | 图标 CC BY 4.0；字体 SIL OFL 1.1，Reserved Font Name `Font Awesome`；非字体/非图标代码 MIT。版权归属 Fonticons, Inc.；品牌图标仍是各自权利人的商标，不表示背书。 | 无 | `font-awesome-v6.5.1-LICENSE.txt`；v3.5.1 仓内副本 `nerd-fonts-font-awesome-v6.5.1-LICENSE.txt`（blob `cc557ece45db3719d26f8f6e0182d89651b76062`）。上游 tag 文本为 2023 版权行，Nerd Fonts 副本为 2022；两份均保留以对应各自固定来源。 |
| Font Awesome Extension | `0.0.3` | tag `v.0.0.3` commit `bbfbc8c7e21a206e24489b30ddc695995913169e`；该 tag 没有许可证文件。上游后来在 commit `09d80249058ee8018a45da30add4339289dbb466` 添加 `LICENCE`，blob `a2f736647822bec9215890f6a3bde699831f8ee5`。 | MIT；`Copyright (c) 2017 André Luiz Gava`。许可证文件晚于 tag，故同时记录 tag 身份和后续许可证提交，不把后续文件伪装成 tag 内文件。 | 无 | `font-awesome-extension-v0.0.3-LICENCE` |
| Font Logos | `1.3.0` | `lukas-w/font-logos@e92426e28ee5bbf4ca328b33e3a393b7a5045f01`（tag `v1.3.0`）；`LICENSE` blob `cf1ab25da0349f84a3fdd40032f0ce99db813b8b` | Unlicense；`package.json` 固定版本明确写 `license: "Unlicense"`。作者为 Lukas W；README 明确所有品牌图标是各自权利人的商标，只能用于代表对应公司或产品。不得改写为 MIT，也不能把 Unlicense 当作商标授权。 | 无 | `font-logos-v1.3.0-UNLICENSE`、`font-logos-v1.3.0-README.md` |
| MaterialDesign | Nerd Fonts 记录为 `2022-10-06` 快照 | `Templarian/MaterialDesign-Font@d72e30826e6797c1b3a2976e71d0d2f790c1a2eb`（该仓库在 2022-10-06 前的最新提交，2022-03-25）；`LICENSE` blob `382f8a138afaaefe9ca2cfa80d33317f7814d997` | Pictogrammers Free License：字体和图标按 Apache 2.0，非字体/非图标代码按 MIT；保留 Pictogrammers 归属及 Apache 2.0 原文。 | 无 | `material-design-font-2022-10-06-LICENSE`、`material-design-font-2022-10-06-README.md`、标准条款 `Apache-2.0.txt`。Nerd Fonts 对应许可证 blob 同为 `382f8a138afaaefe9ca2cfa80d33317f7814d997`。 |
| Octicons | `18.3.0` | `primer/octicons@680a888079911b37c7690f23aa222de2a499e3dd`（tag `v18.3.0`）；`LICENSE` blob `163074d5b43ac314f9db311945cb4c19fd0c3d44` | MIT；`Copyright (c) 2023 GitHub Inc.`。GitHub/Octocat 等 logo 受 GitHub 商标规则约束，使用不表示背书。 | 无 | `octicons-v18.3.0-LICENSE`；Nerd Fonts 对应副本相同 |
| Seti and original | `0.8.1` | `jesseweed/seti-ui@6c358eb9ce5cc20bccd72cc4d7179fd485cd8fe7`（tag `v0.8.1`）；`LICENSE.md` blob `0686d9e64771ecfe3d755fbc412be64a3a4b2a77` | MIT；`Copyright (c) 2014 Jesse Weed`。 | 无 | `seti-ui-v0.8.1-LICENSE.md`、`seti-ui-v0.8.1-README.md` |
| Pomicons | `1.001` | `gabrielelana/pomicons@d04478259e7be6ca44234661a2752975f977a4fe`（master，2022-01-22；仓库无对应 tag）；`LICENSE` blob `5dd70fa0e0bc67c638079585bdb8d56e0648ff87` | OFL 1.1；`Copyright (c) 2021, Gabriele Lana`；Reserved Font Name `Pomicons`。修改版不能继续使用该保留名称，字体本身仍按 OFL 分发。 | 无 | `pomicons-v1.001-LICENSE`、`pomicons-v1.001-README.md`；标准条款 `OFL-1.1.txt` |
| Powerline Extra | `1.200` | `ryanoasis/powerline-extra-symbols@ae05de7c51f6609479f4f1a4a0f6f65631731c1b`（master，2019-08-17；仓库无对应 tag）；`LICENSE` blob `c71d82753460a6c580966fd5f5a3da268ed5a974` | MIT；`Copyright (c) 2016 Ryan L McIntyre`。Nerd Fonts README 记录其对 glyph 做过修改并将版本标为 1.200。 | 无 | `powerline-extra-symbols-v1.200-LICENSE`、`powerline-extra-symbols-v1.200-README.md`；Nerd Fonts 对应副本相同 |
| Powerline Symbols | `1.000 (ca 2013)` | `powerline/powerline@429b52f2baf7c8e393ed41d76365fad7d0b06984`（tag `1.0`）；`LICENSE` blob `ad3cf61a4fbb77fe7ea5079985d60666e7962e8b` | 上游 LICENSE 是 MIT；`Copyright 2013 Kim Silkebækken and other contributors`。Nerd Fonts audit 将其写为 `Free License`，但固定上游文件明确是 MIT；声明按上游原文记录，并保留 Nerd Fonts 的来源差异。 | 无 | `powerline-v1.000-LICENSE`、`powerline-v1.000-README.rst`；v3.5.1 仓内副本 `nerd-fonts-powerline-symbols-v1.000-LICENSE.txt`（blob `23b2ab298f959223a99ce8f9a7f7fb50e6c9ed39`） |
| Power Symbols IEC | `Feb 2015` | `jloughry/Unicode@41bf268c900a42001696a09496e2468728fb4c7f`（2015-02-28）；`LICENSE.txt` blob `f2d63f66d2b770cd0bea2bd283fda802fe30eb2f` | MIT；`Copyright (c) 2013 Joe Loughry`。仓库 README 还记录 IEC/Unicode 符号的来源与版权讨论，随包保留该 README。 | 无 | `power-symbols-iec-2015-LICENSE.txt`、`power-symbols-iec-2015-README.md` |
| Weather Icons | `2.0.10 (1.100)` | `erikflowers/weather-icons@a85ba96d252d6729eb9a07aa2f0544049b291163`（tag `2.0.10`）；上游仓库没有独立 LICENSE/OFL 文件，也没有 NOTICE；README 固定 blob `0845532bf87cd4944f5a8c3002dad86a02238391` | README 说明字体按 OFL 1.1，代码按 MIT，文档按 CC BY 3.0；设计最初归 Lukas Bischoff，v1.1 之后图稿/代码归 Erik Flowers。Nerd Fonts 仓内 `OFL.txt` 使用占位版权行，不能替代实际归属；本目录同时保留该原文、FAQ 和标准 OFL 1.1 条款。 | 无 | `weather-icons-v2.0.10-README.md`、`nerd-fonts-weather-icons-v2.0.10-OFL.txt`、`nerd-fonts-weather-icons-OFL-FAQ.txt`、`OFL-1.1.txt` |

## Nerd Fonts 固定来源文件

以下文件直接来自 `ryanoasis/nerd-fonts@b894ea7803af6aade63d60a4381e006098ec9c4d`，并保留其 Git blob 身份：

| Nerd Fonts 路径 | blob SHA |
| --- | --- |
| `patched-fonts/NerdFontsSymbolsOnly/LICENSE` | `06eb073d6b545f7f505919a52e6af903f913d4b8` |
| `patched-fonts/NerdFontsSymbolsOnly/README.md` | `3cde95d0707e6b3a75cc82fa0c6acadc8b6b94ca` |
| `src/glyphs/README.md` | `836b62ea31c7e73b703550f9f240b8d8fc8a3895` |
| `license-audit.md` | `a47f213a45767238af87d4b722cc875cd400ea43` |
| `src/glyphs/codicons/LICENSE.txt` | `085c3ca0c4920beedbf179f7987e705aec84115b` |
| `src/glyphs/font-awesome/LICENSE.txt` | `cc557ece45db3719d26f8f6e0182d89651b76062` |
| `src/glyphs/materialdesign/LICENSE` | `382f8a138afaaefe9ca2cfa80d33317f7814d997` |
| `src/glyphs/octicons/LICENSE` | `163074d5b43ac314f9db311945cb4c19fd0c3d44` |
| `src/glyphs/pomicons/LICENSE` | `5dd70fa0e0bc67c638079585bdb8d56e0648ff87` |
| `src/glyphs/powerline-extra/LICENSE` | `c71d82753460a6c580966fd5f5a3da268ed5a974` |
| `src/glyphs/powerline-symbols/LICENSE.txt` | `23b2ab298f959223a99ce8f9a7f7fb50e6c9ed39` |
| `src/glyphs/weather-icons/OFL.txt` | `fd295ff85e40801883186c705e94f9e504d9f96c` |
| `src/glyphs/weather-icons/OFL-FAQ.txt` | `e058b7906f5184a4811843f59c369c4b1e33b17e` |
| `patched-fonts/Hack/LICENSE.md` | `08927e504fce744f928c584c6e4a07328f2e94ed` |

## 本目录文件校验

下表用于确认产品资源中的原始文本没有被构建步骤静默替换。

| 文件 | bytes | SHA-256 |
| --- | ---: | --- |
| `Apache-2.0.txt` | 11357 | `b40930bbcf80744c86c46a12bc9da056641d722716c378f5659b9e555ef833e1` |
| `OFL-1.1.txt` | 4069 | `f19c674290e5dc79f02e8efe16139ab56a06a7128876f5b1579ffd0db5bc897e` |
| `codicons-v0.0.45-LICENSE` | 19242 | `af5e030844efddbc7ab00dcfea8b019703753d4d9f5172d727c533a492aec665` |
| `codicons-v0.0.45-LICENSE-CODE` | 1162 | `9906940f61b1f0b533fa7d99baf55178b2808fbe113ea51dfbfad8572ccd5f2b` |
| `devicon-v2.17.0-LICENSE` | 1072 | `121194741d4a915b9f5890fdd6dd95121f9b1f816517c792358d72d7c838d664` |
| `devicon-v2.17.0-README.md` | 13109 | `2c2eaf4ecefc9b20c36250793b34613638311e098ce20029ad7eb5d39ed3a37d` |
| `font-awesome-extension-v0.0.3-LICENCE` | 1073 | `b47b15d771c4507a75ff65beca3a78168d6af4cca9b45177a1aa817139c64ba5` |
| `font-awesome-v6.5.1-LICENSE.txt` | 7427 | `0aa8f86525273b2efa4f40f4272a188e187704252170e979dc06879adf68d43c` |
| `font-logos-v1.3.0-README.md` | 22485 | `b7c98abe0c74e0804e5852fa508ee2bc51b44a4aee45a8384204938a705fabf8` |
| `font-logos-v1.3.0-UNLICENSE` | 1210 | `88d9b4eb60579c191ec391ca04c16130572d7eedc4a86daa58bf28c6e14c9bcd` |
| `hack-v3.003-LICENSE.md` | 3744 | `1f61bb7c790c59b4b0ecdf304628b94e42ae4c8020094a8c3da73381ab212623` |
| `material-design-font-2022-10-06-LICENSE` | 992 | `3bae6305bcaa6d10219b999f65c342034c2d40b1c0e8567fa08605e73ccba829` |
| `material-design-font-2022-10-06-README.md` | 1678 | `6f180a0af2a3a4a69ed777d3de2fbef2de12eb7b5f0b26ecbe03f1f97e452f46` |
| `nerd-fonts-codicons-v0.0.45-LICENSE.txt` | 19047 | `d6239afa918961b465b07bf7411cbe34ff6685854f58553db7966f4881a0211f` |
| `nerd-fonts-font-awesome-v6.5.1-LICENSE.txt` | 7427 | `de784a808496d49f5f80dd9d1d6e9a51cff712cfbc339e7c25155631ae3cdb5d` |
| `nerd-fonts-hack-MIT-Bitstream-Vera-LICENSE.md` | 3744 | `1f61bb7c790c59b4b0ecdf304628b94e42ae4c8020094a8c3da73381ab212623` |
| `nerd-fonts-materialdesign-2022-10-06-LICENSE` | 992 | `3bae6305bcaa6d10219b999f65c342034c2d40b1c0e8567fa08605e73ccba829` |
| `nerd-fonts-octicons-v18.3.0-LICENSE` | 1068 | `4f886642f9e8d99eeadd484acdac011c3fde8a338faf52f5ca1408c13b9a9b63` |
| `nerd-fonts-pomicons-v1.001-LICENSE` | 4395 | `cb4797a9745683f287c28833db596e9d68ec43082d59359b3a49c504f0f610ce` |
| `nerd-fonts-powerline-extra-v1.200-LICENSE` | 1072 | `544307a58e6826c24145b2af1c3578b53e2e21ae0e6c67ab4c6abd544304d26a` |
| `nerd-fonts-powerline-symbols-v1.000-LICENSE.txt` | 1132 | `46d5355e4734468135d8e4a30d0916292cc4df2b3ec30aa68c866f82497933be` |
| `nerd-fonts-symbols-only-v3.5.1-LICENSE` | 1082 | `84a7a98c82140fb12c37fe42b93805baa16024cb3e5acc599b7ffe612c55d847` |
| `nerd-fonts-symbols-only-v3.5.1-README.md` | 3890 | `ad2ab32b8e88e894f6a364c18774a0b6bc8dcb50f587d3ddb35090c33102ef5d` |
| `nerd-fonts-v3.5.1-glyph-sources-README.md` | 2355 | `b3382c86b928820ff4135bc4c2cf0da5d871b3c2262b72711a00b21f2daf534a` |
| `nerd-fonts-v3.5.1-license-audit.md` | 8470 | `c2a3f634fa1e5001912f4f19a42fa86fdef5654691b70000f936089325aa404f` |
| `nerd-fonts-weather-icons-OFL-FAQ.txt` | 59071 | `8acd621722e71dc89de0087a264497b8964d240eedc53677f6344ab20ce36fd7` |
| `nerd-fonts-weather-icons-v2.0.10-OFL.txt` | 4692 | `69cdedc8a4f57a30d68e4f045af8b9cce0ec3b1a6e7eec1ad41737c894f26790` |
| `octicons-v18.3.0-LICENSE` | 1068 | `4f886642f9e8d99eeadd484acdac011c3fde8a338faf52f5ca1408c13b9a9b63` |
| `pomicons-v1.001-LICENSE` | 4395 | `cb4797a9745683f287c28833db596e9d68ec43082d59359b3a49c504f0f610ce` |
| `pomicons-v1.001-README.md` | 3223 | `173aa8d6488298c75f4eaf4b0d644f664cf4ea76acfbfa1bdb432afaa8a91a31` |
| `power-symbols-iec-2015-LICENSE.txt` | 1078 | `11aef1cdf2d58d822e6857e2574870ccc59d4900052bf5c098e319d750e7ac3d` |
| `power-symbols-iec-2015-README.md` | 61262 | `294ff992b4c6dd75274449b07c1c350385ebd0cc64567ff838e5d7be6fd9b174` |
| `powerline-extra-symbols-v1.200-LICENSE` | 1072 | `544307a58e6826c24145b2af1c3578b53e2e21ae0e6c67ab4c6abd544304d26a` |
| `powerline-extra-symbols-v1.200-README.md` | 3003 | `fd13032d108d9b7dd0856dba79d0f7a976e45ae5474af13977c074b03197c013` |
| `powerline-v1.000-LICENSE` | 1131 | `f5652bcff0ca84bd977c7980a5abd9c0ead8e5969c1b53b646eefed6811c9493` |
| `powerline-v1.000-README.rst` | 4199 | `d7ab9b690bab8a5a700a6d086a23b129f858c706ff80a5f9505ca142eed50eef` |
| `seti-ui-v0.8.1-LICENSE.md` | 1054 | `d4d2d888c6a0a220a7b0e2d6013cf37539b0e05ef937002b3b33bdb364575b88` |
| `seti-ui-v0.8.1-README.md` | 1466 | `45e28549f6399a68ca2a34dde513125140ceb03a2075485c0c23fae61c98f8f5` |
| `weather-icons-v2.0.10-README.md` | 2965 | `1854ba6df3083a336c6e951ed0e2c61bbf862cf4930b7861592331ce8d041159` |

## 分发注意

- MIT、CC BY 4.0、Apache 2.0、OFL 1.1 和 Unlicense 的原文分别按上述文件随资源携带；没有把全部 glyph source 错写成 SymbolsOnly 的 MIT。
- OFL 字体可以随软件捆绑，但字体本身必须保留版权、许可证和 Reserved Font Name 约束；不得单独销售字体或用保留名称宣传修改版。
- CC BY 来源需要归属、许可证链接或原文、修改说明，并避免暗示作者或项目背书。
- Apache 来源需要保留版权/归属、Apache 原文以及显著修改说明；本次固定源树没有独立 NOTICE 文件。
- Font Logos、Devicons、Font Awesome、Octicons 等包含品牌 logo；开源许可证不等于第三方商标授权。
