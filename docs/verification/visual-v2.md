# 原生工作台视觉重做

日期：2026-10-12。状态：集中修复与最终原生 closure 检查已完成，独立结论为 `technical_status: passed`、`readiness: user_review`；用户视觉认可仍为 pending。

用户拒绝阶段 1 初始样板，指出“太简陋了，不够精致”，并提供 T3 暗黑截图作为最低质量标准。旧截图仅作为问题证据，不再代表获准方案。

参考文件：[用户截图](../design/references/t3-dark-user-reference.png)，原始尺寸 2360×1475。参考项目：[T3 Code](https://github.com/pingdotgg/t3code)、[Droppy Code](https://gitlab.com/droppyformac1/droppy-code)。源码研究需固定 SHA，按实际来源记录，不能把参考项目技术栈迁入产品界面。

重做方向见 [DESIGN.md](../../DESIGN.md)。实际样板与生产数据沿用同一 Go + mygo 视图；原生工作台、设置及窄窗截图和功能回归完成后补充本记录。

## 第一批原生检查

当前视图重做已涵盖中性近黑/浅色主题、三栏独立表头、低调会话选中、无外卡片的助手正文、统一底部输入容器、可展开工具详情、Diff 旧/新行号与语法颜色。Markdown 使用 Goldmark CommonMark/GFM 转换为原生渲染模型。

使用同一产品 `desktop.NewVisualSample` 视图编译原生 QA 宿主，独立应用身份 `GCode Visual QA 20261012`，不写生产会话数据。原生窗口 PID `75524`、CGWindow ID `21795`。源码指纹及 binary hash 见 `output/acceptance/visual-v2/round-1/build-manifest.json`，指纹为 `2ef417132566bbe5c877563a1c4c0154a99776dc8e7fe7f1b4ae108d2e5e83fa`。

- 暗黑：1440×900、1280×800、1024×700 DIP。
- 浅色：1440×900、1024×700 DIP。
- 设置：1440×900 暗黑/浅色；通过原生 AX 点击“设置”、主题 radio 和“返回工作台”进入。
- 图像：`output/acceptance/visual-v2/round-1/*-native.png`，由 macOS `screencapture -l 21795` 捕获，显示真实交通灯；另保存 mygo CapturePage 内容截图。七张原生图均已实际打开检查，SHA/尺寸清单在 `captures.json`。
- 原生输入：通过 AX 设置多行输入控件后读回中文和 emoji，文字逐字一致；见 `native-input.txt`。这是程序化输入证据，不能替代物理键盘和中文输入法候选窗口验收。
- QA 窗口已通过原生关闭按钮退出，进程 exit 0。

`go test ./internal/desktop -count=1`、对应 race 和 vet 已通过。全仓测试中发现的一次纠正顺序测试失败正在独立定位；这里不把一次重跑成功当作最终回归完成。

首次截图暴露的视觉与控件问题统一交由独立审阅形成修复清单，再集中修复并进行第二批检查。第一批图证明的是当前原生实现与状态，不证明已经达到用户 T3 底线。

本记录当前不证明视觉合格，也不证明完整产品、真实 API 或其他设备验收通过。

## 第二批原生检查

第一批独立审阅的八项修复已集中处理：统一三栏与原生标题栏的垂直基线；会话标题和路径/分支改为两行；窄窗收紧左栏并默认收起资源面板；工具摘要提取命令/路径；统一 Select 的表面；设置内容最大宽度 980 DIP、去掉重复外卡片并使用 54 DIP 独立 footer；明确浅色 Markdown 代码/选区颜色；修复短 Diff 折叠保持和长行换行。

Diff 长行第一次调整仍被原生截图证实存在截断，最终定位为横向滚动容器以无限宽测量 List。现在 List 和行使用动态可视宽度，代码允许收缩并换行，行高使用实际测量结果；hunk 元数据单行显示并提供完整 Tooltip。测试验证长行高度和尾部可达性，第二批截图使用最终修复后的源码。

使用 `output/acceptance/visual-v2/native-qa.py` 重建真实原生 QA 宿主，复用生产 `desktop.NewVisualSample` 视图，示例内容始终有明确标记。本批 PID 为 `91799`、CGWindow ID 为 `21810`，系统为 macOS 14.8.7 Intel，Go 为 `go1.27.2 darwin/amd64`。

| 场景 | 原生截图 |
| --- | --- |
| 暗黑 1440×900 DIP | `output/acceptance/visual-v2/round-2/dark-1440x900-native.png` |
| 暗黑 1280×800 DIP | `output/acceptance/visual-v2/round-2/dark-1280x800-native.png` |
| 暗黑 1024×700 DIP | `output/acceptance/visual-v2/round-2/dark-1024x700-native.png` |
| 浅色 1440×900 DIP | `output/acceptance/visual-v2/round-2/light-1440x900-native.png` |
| 浅色 1024×700 DIP | `output/acceptance/visual-v2/round-2/light-1024x700-native.png` |
| 暗黑设置 1440×900 DIP | `output/acceptance/visual-v2/round-2/settings-dark-native.png` |
| 浅色设置 1440×900 DIP | `output/acceptance/visual-v2/round-2/settings-light-native.png` |

七张图均由 macOS `screencapture` 捕获并实际打开检查；这是操作系统原生窗口截图，包含真实交通灯。指纹与清单位于本批 `build-manifest.json` 和 `captures.json`：

- 源码指纹：`356c2e9b00492573d92733120fa5d04cfccc4791a8235818d7fd5ca54812a57d`。
- QA binary SHA-256：`09a771af4e3df146063f3b8a275ce97fdcfd5f1d0340439483584f13037b3711`。
- `source_unchanged_after_capture: true`；原生交互结束后再次核对源码与七张截图 hash，全部一致。

原生 AX 检查通过：

1. 点击“设置”、浅色/深色 radio 和“返回工作台”，两张设置图中的保存按钮完整可见；原生控件清单为 `settings-ax.txt`。
2. 对多行输入控件写入中文/emoji 并逐字读回，记录在 `native-input.txt`。此项只证明 AX 输入，不替代物理键盘、中文输入法候选及 Enter/Escape 行为。
3. 短 Diff 初始展开，AXPress 后折叠；composer 中文输入触发重绘后保持折叠，随后恢复展开。可重放脚本为 `output/acceptance/visual-v2/check-diff-collapse.applescript`，证据为 `round-2/diff-collapse.txt`。
4. 通过原生 `AXCloseButton` 正常关闭 QA 窗口，驱动 exit 0，PID `91799` 与驱动 PID `91546` 均已退出。没有操作系统剪贴板变更，复制功能本轮未实测。

## 最终自动化回归与应用包

原始日志位于 `output/acceptance/visual-v2/regression/`。这些结果覆盖本轮 controller 取消发布竞态修复、来源提示裁剪及最终 Diff 修复，旧 `stage-5/final-20261012/` 只代表更早的源码状态。

| 命令 | 结果/证据 |
| --- | --- |
| `go test ./... -count=1` | 最终 closure 源码 exit 0，`go-test-closure.log`、`go-test-closure-result.json` |
| `go vet ./...` | 最终 closure 源码 exit 0，`go-vet-closure.log`、`go-vet-closure-result.json` |
| `make test-race` | exit 0，`test-race-confirmed.log`、`test-race-result.json` |
| `go test -race ./internal/desktop -count=1` | 最终 closure 源码 exit 0，`desktop-race-closure.log`、`desktop-race-closure-result.json` |
| `make fmt-check` | 最终 closure 源码 exit 0，`go-vet-closure-result.json` 中 `fmt_exit_code: 0` |
| `GOFLAGS=-buildvcs=false go tool mygo build --skip-dmg` | 最终 closure 源码 exit 0，`package-closure.log`、`package-closure-result.json` |
| `codesign --verify --deep --strict build/darwin-amd64/GCode.app` | 最终 closure 应用包 exit 0，`codesign-closure.log` |

当前包为 `build/darwin-amd64/GCode.app`，约 25.1 MB，使用本机 ad hoc 签名。本轮原生图来自同产品视图的 QA 宿主；重新构建的正式包尚未单独打开，不以签名验证代替正式包运行验收。

后续正式包已在 `docs/verification/native-product.md` 完成隔离身份的实际设置、三次启动/退出、恢复与采样。其最新源码/包指纹和日志位于 `output/acceptance/native-product/final/`、`regression/`；本节上述 visual closure 构建记录仍对应当时的源码，不用后续包替换原生视觉截图的身份或 hash。

controller 本轮还修复了 `Send`/`Stop` 在释放锁并发布快照后才 cancel 的竞态：持久化成功后在锁内 cancel，持久化失败恢复状态且不取消原 run。`Close` 先尝试持久化，再取消并等待资源收束；即使保存失败也清理运行资源。确定性 subscriber barrier 与保存失败测试已通过，具体记录见 `docs/goals/session-runtime.md`。

## 验收边界

第二批独立结论记录在 `docs/verification/visual-v2-review.md`，当时为 `disposition: fix`：前轮八项已解决，但 1280 的短 Diff 换行后视口高度不足、1024 品牌显示 `GC...` 仍存在。随后只针对这两项确定性布局缺陷完成 closure，见下节；没有继续主观打磨。`DESIGN.md` 和 `.impeccable/design.json` 均保持候选状态；旧样板被拒绝的事实与最终截图是不同记录。

本批完成的是当前设备的视觉候选、有限 AX 交互和自动化回归，不能据此宣称达到用户 T3 最低交付线。用户视觉评价之前不扩展其余管理页面。真实厂商 API、worktree 桌面入口、扩展管理、物理键盘/IME/滚动/复制、性能与 Apple Silicon/Windows 仍按各自验收项继续保持未闭环。

## 两项确定性缺陷的最终 closure

本次只修复第二批独立审阅指出的两项布局缺陷，没有新增主观打磨清单：

- 短 Diff（不超过 200 行）使用按实际 RichText 行高自然伸展的内容布局，交由资源面板统一纵向滚动。取消按 `renderedRows*22+4` 固定短文件视口；大 Diff 继续使用 List 虚拟化。`TestShortDiffWrapUsesActualContentHeight` 验证尾行完整落在下一文件之前；最终 1280 原生图中的 `return nil` 和闭合行完整可读。
- 1100 DIP 以下使用带 `GCode` 可访问标签的 G 品牌图标，保留新会话按钮与标题栏安全空间，不再显示 `GC...`。`TestNarrowSidebarKeepsBrandUnambiguous` 与最终暗黑/浅色 1024 原生图均已复核。

最终七张图位于 `output/acceptance/visual-v2/closure/`，名称与第二批相同，均由新编译的原生 QA 窗口捕获并逐张实际打开。最终展示应使用 closure 图，`round-1/` 与 `round-2/` 保留为不同源码状态的历史证据。

- PID：`99100`；CGWindow：`21818`。
- 源码指纹：`54bbd1062cf6642461f8c5b9696dbc3d847d5763abbf5c2c0b72960e968c70b3`。
- QA binary SHA-256：`5d6f241f7365a107594e75bc8979c65fef5cc906b92eca4fa092e258bdc0b9d9`，保存在本批目录，避免覆盖历史 binary。
- `build-manifest.json`：`source_unchanged_after_capture: true`、`native_qa_exit_code: 0`；`captures.json` 和 `closure-integrity.txt` 已确认当前视图源码、binary 与七张图 hash 一致。
- `native-input.txt` 和 `diff-collapse.txt` 再次通过；短 Diff 用户折叠→composer 重绘仍折叠→恢复展开。`close-action.txt` 记录原生 AXCloseButton，驱动 exit 0 且 PID 已退出；本轮仍未操作系统剪贴板。

独立 reviewer 只复核这两项功能修复及对应截图/hash，结论记录在 `docs/verification/visual-v2-review.md`：`technical_status: passed`、`readiness: user_review`、用户视觉认可 `pending`。当前可交给用户评价视觉基线；技术修复通过不等于用户已认可，也不代表完整产品或其他设备通过。
