# 原生终端 CoreText glyph 覆盖探针

日期：2026-10-12

范围：只读取 CoreText/AppKit 字体注册表和字体 glyph 结果，固定探针码点，不启动 shell、不读取 shell 配置、环境变量或 prompt 字节，不启动 native 窗口，不运行 Go 测试或构建。

探针脚本：[terminal-font-coverage-qa.swift](scripts/terminal-font-coverage-qa.swift)

## 方法

脚本对每个字体和固定码点执行四层检查：

1. `CTFontGetGlyphsForCharacters`：`glyph == 0` 记为 `direct=RED`，非零记为 `direct=GREEN`。
2. `CTFontCreateForStringWithLanguage` 选择当前字体及其显式 cascade 的实际 fallback font，再对该 font 查询 glyph。`resolvedGlyph`、`resolvedFamily` 和 `resolved=GREEN/RED` 反映 CoreText fallback resolver 是否找到可绘制字体；`.LastResort` 视为未解决。
3. 同时用该字体创建 `CTLine` 并读取实际 `CTRun` 的 font family，输出为 `ctLineFamily`。它保留排版器路径的独立观测，不混入 `resolved` 判定。
4. `CTFontCopyDefaultCascadeListForLanguages`（`en-US`）只记录 cascade 列表数量和第一个直接覆盖该码点的候选 family，字段为 `cascadeCount` 和 `cascadeHit`。

固定码点为：`U+0041`（ASCII A）、`U+4E2D`（CJK 中）、`U+E0A0`、`U+E0B0`（Powerline 常用分隔符）、`U+F07C`、`U+F120`（FontAwesome/Nerd Font 常见私有区候选）。脚本选择系统 monospace、Menlo，以及字体注册表中已存在的 `Meslo LG S for Powerline` 和 `Source Code Pro for Powerline`。

## 一次实际运行

运行命令：

```sh
swiftc docs/verification/scripts/terminal-font-coverage-qa.swift \
  -framework AppKit -framework CoreText \
  -o /tmp/terminal-font-coverage-qa \
  && /tmp/terminal-font-coverage-qa
```

运行环境为 macOS `14.8.7 (Build 23J520)`、`x86_64`。命令退出码为 `0`，表示探针本身的基线断言通过；每个码点仍独立输出 `direct=RED/GREEN` 和 `resolved=RED/GREEN`，因此缺字结果不会被总体退出码隐藏。

这里的 `RESULT=GREEN` 只表示“探针完成，ASCII/CJK 基线和已发现的 Powerline 样本符合探针断言”。它不表示系统 monospace 已经修复缺字；同一份输出中系统 monospace 的四个目标私有区码点仍明确为 `resolved=RED`。需要把“缺字症状存在”作为失败条件时，使用 `--require-pua`：

```sh
/tmp/terminal-font-coverage-qa --require-pua
```

本机结果为 `exit=1`，末尾为：

```text
GREEN summary=system-and-Powerline-probes-complete
RED require-pua=system monospace-missing-target-pua
RESULT=RED
PROBE_EXIT_CODE=1
```

这才是当前症状的红判定；默认模式的绿色不能写成缺字修复通过。指定 `--cascade-family` 后，`--require-pua` 改为检查显式 cascade case，允许 primary system monospace 的直接 glyph 仍为零，只要求 fallback resolver 找到非 `.LastResort` 的目标字体。

代表性原始输出如下：

```text
FONT label="system monospace" family=".AppleSystemUIFontMonospaced" postscript=".AppleSystemUIFontMonospaced-Regular"
  direct=RED resolved=RED U+E0A0 Powerline E0A0 glyph=0 resolvedGlyph=4 resolvedFamily=".LastResort" ctLineFamily=".LastResort"
  direct=RED resolved=RED U+E0B0 Powerline E0B0 glyph=0 resolvedGlyph=4 resolvedFamily=".LastResort" ctLineFamily=".LastResort"
  direct=RED resolved=RED U+F07C FontAwesome F07C glyph=0 resolvedGlyph=4 resolvedFamily=".LastResort" ctLineFamily=".LastResort"
  direct=RED resolved=RED U+F120 FontAwesome F120 glyph=0 resolvedGlyph=4 resolvedFamily=".LastResort" ctLineFamily=".LastResort"

FONT label="system monospace + explicit cascade Symbols Nerd Font Mono" family=".AppleSystemUIFontMonospaced" postscript=".AppleSystemUIFontMonospaced-Regular"
  direct=RED resolved=GREEN U+E0A0 Powerline E0A0 glyph=0 resolvedGlyph=28 resolvedFamily="Symbols Nerd Font Mono" ctLineFamily=".LastResort"
  direct=RED resolved=GREEN U+E0B0 Powerline E0B0 glyph=0 resolvedGlyph=32 resolvedFamily="Symbols Nerd Font Mono" ctLineFamily=".LastResort"
  direct=RED resolved=GREEN U+F07C FontAwesome F07C glyph=0 resolvedGlyph=2645 resolvedFamily="Symbols Nerd Font Mono" ctLineFamily=".LastResort"
  direct=RED resolved=GREEN U+F120 FontAwesome F120 glyph=0 resolvedGlyph=2809 resolvedFamily="Symbols Nerd Font Mono" ctLineFamily=".LastResort"

FONT label="installed Powerline Meslo LG S for Powerline" family="Meslo LG S for Powerline"
  direct=GREEN resolved=GREEN U+E0A0 Powerline E0A0 glyph=2678 resolvedGlyph=2678 resolvedFamily="Meslo LG S for Powerline" ctLineFamily="Meslo LG S for Powerline"
  direct=GREEN resolved=GREEN U+E0B0 Powerline E0B0 glyph=2681 resolvedGlyph=2681 resolvedFamily="Meslo LG S for Powerline" ctLineFamily="Meslo LG S for Powerline"
```

完整输出由脚本按固定顺序打印；本记录只保留能判定结论的行。系统 monospace 和 Menlo 的 `U+4E2D` 直接 glyph 为零，但 `resolvedFamily` 为 PingFang SC，所以这是正常的 CJK fallback。相反，基础系统 monospace 的四个目标私有区码点都解析到 `.LastResort`，因此症状判定为红。

显式 cascade case 的 `resolvedFamily` 已解析到 `Symbols Nerd Font Mono`，四个目标 glyph 均非零；这使 `--require-pua` 从 baseline 的红变为绿。该 case 的 `ctLineFamily` 仍为 `.LastResort`，所以记录保留了 CoreText fallback resolver 与 CTLine/CTTypesetter 排版器的差异，不能把 resolver 级绿色直接表述为 native 窗口截图已修复。

系统字体的部分 `cascadeHit` 会显示 `STIX Two Math`，但对应的 `resolvedFamily` 仍是 `.LastResort`。因此 `cascadeHit` 只能说明默认列表中存在候选描述，不能替代实际 fallback resolver 结果；本结论采用 `resolvedGlyph`、`resolvedFamily` 和 `resolved` 判断覆盖。

两个原始日志：

- baseline 红判定：[baseline-system-monospace-require-pua.log](../../output/acceptance/terminal-font/coverage/baseline-system-monospace-require-pua.log)
- v3.5.1 进程注册 + 显式 cascade 绿判定：[v3.5.1-symbols-nerd-font-mono-process-cascade-require-pua.log](../../output/acceptance/terminal-font/coverage/v3.5.1-symbols-nerd-font-mono-process-cascade-require-pua.log)

## 显式 cascade 和进程字体注册

脚本支持把一个实际 family 放到系统 monospace 后面，直接验证 MyGo 使用的显式 cascade 路径：

```sh
/tmp/terminal-font-coverage-qa --cascade-family 'Meslo LG S for Powerline'
```

输出会额外出现 `FONT label="system monospace + explicit cascade Meslo LG S for Powerline"`。该 case 中 `U+E0A0`/`U+E0B0` 的 `resolved` 为 `GREEN`，`resolvedFamily` 为 `Meslo LG S for Powerline`；`U+F07C`/`U+F120` 仍为 `RED`，因为该字体并不覆盖 FontAwesome 私有区。这证明显式 cascade resolver 可以解决它实际覆盖的 codepoint，但不会凭空补齐其他区间。

`--font-file <TTF>` 在探针进程内调用 `CTFontManagerRegisterFontsForURL(..., .process, ...)`，然后可与 `--cascade-family <actualfamily>` 联用：

```sh
/tmp/terminal-font-coverage-qa \
  --font-file /path/to/SymbolsNerdFontMono-Regular.ttf \
  --cascade-family 'Symbols Nerd Font Mono'
```

注册结果会打印 `font-file-registration=GREEN scope=process`。本次固定资源结果为：文件大小 `2,610,012` bytes，SHA-256 为 `fe471e538392f51910faab985fa8e192a39dd3426125edd15b71b3680df0e749`，CoreText 注册后确认 actual family 为 `Symbols Nerd Font Mono`。四个目标码点的 `resolvedFamily` 都是该 family，`resolvedGlyph` 分别为 `28`、`32`、`2645`、`2809`，进程注册 + 显式 cascade 的 `--require-pua` 结果为 `exit=0`。`.process` 作用域只对当前探针进程生效，不写入系统字体注册，也不把用户安装字体当作产品资源。

## 源码 API 核对

- 当前原生终端只传入工作目录和 scrollback：[internal/desktop/terminal_native.go:12-14](../../internal/desktop/terminal_native.go#L12-L14)，没有设置 `terminal.Options.Font`。
- 空 `Family` 会归一为 `monospace`，默认大小为 13、weight 为 400：[third_party/mygo/plugins/terminal/font.go:49-64](../../third_party/mygo/plugins/terminal/font.go#L49-L64)。Darwin backend 对 `monospace` 调用 `NSFont.monospacedSystemFontOfSize:weight:`，失败才回退到 Menlo：[third_party/mygo/internal/text/coretext_darwin.go:522-553](../../third_party/mygo/internal/text/coretext_darwin.go#L522-L553)。探针实际得到的系统 family 为 `.AppleSystemUIFontMonospaced`。
- `terminal.Font.Family` 支持按顺序指定字体；终端运行中也能调用 `SetFont`：[third_party/mygo/docs/plugins/terminal.md:160-177](../../third_party/mygo/docs/plugins/terminal.md#L160-L177)、[third_party/mygo/docs/plugins/terminal.md:203-204](../../third_party/mygo/docs/plugins/terminal.md#L203-L204)、[third_party/mygo/plugins/terminal/terminal.go:519-527](../../third_party/mygo/plugins/terminal/terminal.go#L519-L527)。
- 应用内字体可通过 `ui.RegisterFont(data, family)` 注册：[third_party/mygo/ui/window.go:34-37](../../third_party/mygo/ui/window.go#L34-L37)。插件文档给出的可分发流程是用 `go:embed` 读取应用携带的 TTF/OTF，再注册同一 family：[third_party/mygo/docs/plugins/terminal.md:173-187](../../third_party/mygo/docs/plugins/terminal.md#L173-L187)。
- CoreText backend 会为字体族列表中后续显式 family 构造 cascade：[third_party/mygo/internal/text/coretext_darwin.go:644-671](../../third_party/mygo/internal/text/coretext_darwin.go#L644-L671)。这条路径要求产品明确指定目标 family；用户本机安装的字体不会成为可分发产品资源。

## 判断和最低正确修复路径

探针把 baseline 原因收敛到字体覆盖层：当前系统 monospace/Menlo 对 Powerline 私有区为直接 glyph 0，fallback resolver 和 CTLine 观察均落到 `.LastResort`；已安装的 Powerline 字体对 `U+E0A0` 和 `U+E0B0` 为非零 glyph，并由 `CTRun` 实际选中。固定 v3.5.1 资源在显式 cascade resolver 中覆盖四个目标码点，但其 `ctLineFamily` 仍需由产品原生路径继续核验。ASCII 和 CJK 通过正常字体及 fallback 工作，说明 baseline 问题不是 PTY 字节、VT parser、cell 宽度或通用 CoreText 绘制管线故障。

最低正确产品路径是：

1. 根据实际 prompt 使用的全部 codepoint 选择覆盖它们的字体资源。Powerline 字体覆盖 `U+E0A0`/`U+E0B0`，但本探针显示它们不覆盖 `U+F07C`/`U+F120`；若 prompt 使用后两者，需要覆盖这些精确 codepoint 的 Nerd Font 或其他字体。
2. 将字体文件作为应用资源分发，通过 `ui.RegisterFont` 注册固定 family。
3. 创建终端时显式设置 `terminal.Options{Font: terminal.Font{Family: "<bundled-family>"}}`；若终端已经运行，可用 `SetFont` 更新。

短期本机验证可以指定已安装的 `Meslo LG S for Powerline` 或 `Source Code Pro for Powerline`，但这不能作为产品修复，因为用户字体目录不是应用资源。若产品不需要图标 prompt，则将 prompt 限制为 ASCII 是更小的修复面。

## Symbols primary 的 descriptor/CTTypesetter 复核

为核对最小接入候选 `Family: "GCode Terminal Symbols, monospace"`，另保存了一份只读探针。它复现 MyGo Darwin backend 的注册 descriptor 路径：从字体数据调用 `CTFontManagerCreateFontDescriptorsFromData`，取 descriptor，再调用 `CTFontCreateWithFontDescriptor` 创建 primary `CTFont`。MyGo 会把 descriptor 同时放入字体自身 family 和 `ui.RegisterFont` 提供的 alias；因此探针以 `GCode Terminal Symbols` 作为 alias 记录，直接用该 descriptor 生成同一个 primary font。对应源码是 [`coretext_darwin.go:1272-1317`](../../third_party/mygo/internal/text/coretext_darwin.go#L1272-L1317) 和 [`coretext_darwin.go:755-778`](../../third_party/mygo/internal/text/coretext_darwin.go#L755-L778)。

精确命令保存在 [`v3.5.1-symbols-nerd-font-mono-primary-descriptor-probe.command.txt`](../../output/acceptance/terminal-font/coverage/v3.5.1-symbols-nerd-font-mono-primary-descriptor-probe.command.txt)，探针脚本保存在 [`v3.5.1-symbols-nerd-font-mono-primary-descriptor-probe.swift`](../../output/acceptance/terminal-font/coverage/v3.5.1-symbols-nerd-font-mono-primary-descriptor-probe.swift)，原始 stdout 保存在 [`v3.5.1-symbols-nerd-font-mono-primary-descriptor-probe.log`](../../output/acceptance/terminal-font/coverage/v3.5.1-symbols-nerd-font-mono-primary-descriptor-probe.log)。该命令只编译并运行 Swift/CoreText 探针，没有启动窗口、shell、Go 构建或 Go 测试。

固定资源的原始结果为：`descriptor-count=1`，实际 primary family 为 `Symbols Nerd Font Mono`，PostScript 名为 `SymbolsNFM`，字体大小为 `13.000000`，ascent/descent/leading 为 `10.397461/2.602539/0.000000`。四个目标 PUA 都在 primary font 中直接有 glyph，并由实际 `CTTypesetterCreateWithAttributedString` / `CTTypesetterCreateLine` run 使用该 family：

```text
U+E0A0 directGlyph=28  resolvedGlyph=28   resolvedFamily=Symbols Nerd Font Mono
U+E0B0 directGlyph=32  resolvedGlyph=32   resolvedFamily=Symbols Nerd Font Mono
U+F07C directGlyph=2645 resolvedGlyph=2645 resolvedFamily=Symbols Nerd Font Mono
U+F120 directGlyph=2809 resolvedGlyph=2809 resolvedFamily=Symbols Nerd Font Mono
```

同一 primary font 对 ASCII 和 CJK 仍能正常 fallback：`ASCII A` 的 typesetter run 是 `Menlo`，glyph 36，advance `7.826660`；`CJK 中` 的 run 是 `PingFang SC`，glyph 1278，advance `13.000000`。混合字符串的实际 run 顺序为 `Menlo → PingFang SC → Menlo → Symbols Nerd Font Mono → Menlo → Symbols Nerd Font Mono → Menlo`，所以将 symbols family 放在 primary 不会让 ASCII 或 CJK 变成 `.LastResort`。

该资源的 advance 也记录了一个布局限制：ASCII printable 的最大 advance 为 `7.826660`，而四个 PUA glyph 的 advance 都为 `13.000000`。终端 cell 宽度由 ASCII printable 的 shaping 结果计算，[`paint.go:61-79`](../../third_party/mygo/plugins/terminal/paint.go#L61-L79)；glyph 超过 cell 时，`shapeRun` 会按比例缩小再居中，[`paint.go:323-369`](../../third_party/mygo/plugins/terminal/paint.go#L323-L369)。因此 primary descriptor 路径已经证明 PUA 能被实际 typesetter 选中，但 symbols-only 字体的图标可能被缩小到 ASCII cell 内；这需要原生窗口验证视觉尺寸，不能仅凭本探针声称最终 UI 已通过。

本复核的 `RESULT=GREEN` 只表示 descriptor primary、ASCII/CJK fallback、混合 run 和 advance 断言完成；它不替代启动应用后的 native screenshot 或终端交互验收。

## 产品原生结果（2026-10-12）

固定字体已作为 primary 接入新 canonical 包。真实终端的四个 PUA、ASCII 和中文均可见，三张图/hash/PTY UTF-8/PID 清理已由 root 复核；完整交互另有十张原生截图与限定范围通过。见 [terminal-font-bundled.md](terminal-font-bundled.md)。本节补充产品证据，不改变前文 cascade resolver GREEN、CTLine RED 的原始探针结果。
