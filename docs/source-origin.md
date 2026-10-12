# zero 来源与接管快照

本文固定 GCode 接管 zero 的一次性源码输入。GCode 后续独立维护，不建立 zero 上游同步机制。本次记录不表示产品源码已经导入或裁剪完成。

## 来源

| 项目 | 结果 |
|---|---|
| 上游仓库 | `https://github.com/Twigpine/zero` |
| 分支 | `main` |
| 复核时间 | 2026-10-11（Asia/Shanghai；GitHub API 响应 `2026-10-11T15:15:03.959Z`） |
| 采用提交 | `99721c762f37cd43ac511007a5f51d1846df959e` |
| 提交消息 | `fix(providers): send stable session headers to OpenCode (#1060)` |
| 提交时间 | `2026-09-15T23:58:40Z` |
| 提交树 SHA | `4f5cdf5e9fbd41dd8aa2e1904ecbce48043741da` |
| GitHub API 状态 | `main` 为默认分支；提交 `verification.verified=true`；仓库许可证字段为 MIT |
| 原调查线索 | `99721c762f37cd43ac511007a5f51d1846df959e`，本次实时复核一致 |

API 原始响应保存在 `/tmp/gcode-zero-source-intake-compare-99721c762f37cd43ac511007a5f51d1846df959e/github-commit-main.json` 和同目录的 `github-repository.json`。应以提交 SHA 而不是仓库当前可变分支名作为导入依据。

## 可复现下载

本次通过 Git clone 和 codeload 归档各下载一次，并交叉核验：

- clone 工作树：`/tmp/gcode-zero-source-intake-99721c762f37cd43ac511007a5f51d1846df959e`
- 归档文件：`/tmp/gcode-zero-source-intake-99721c762f37cd43ac511007a5f51d1846df959e.tar.gz`
- 归档 SHA-256：`c7edc03623180937538a222ff21530a6bf8807bbfd8ac79c8508add93b95bd2d`
- 归档大小：3,681,673 字节
- clone 验证：`git rev-parse HEAD` 等于采用提交；`git status --short --branch` 只有干净的 `main...origin/main`；`git fsck --full --no-progress` 无错误
- 归档验证：归档解包到 `/tmp/gcode-zero-archive-extract-99721c762f37cd43ac511007a5f51d1846df959e` 后有 1,539 个文件；与 clone 工作树的文件清单和 Git blob 内容哈希均无差异

归档下载地址为：

```text
https://codeload.github.com/Twigpine/zero/tar.gz/99721c762f37cd43ac511007a5f51d1846df959e
```

按固定提交重新获取的最小复核命令：

```sh
git clone --depth=1 --branch main https://github.com/Twigpine/zero.git /tmp/zero-main-99721c762f37cd43ac511007a5f51d1846df959e
git -C /tmp/zero-main-99721c762f37cd43ac511007a5f51d1846df959e rev-parse HEAD
git -C /tmp/zero-main-99721c762f37cd43ac511007a5f51d1846df959e fsck --full --no-progress
```

## 与本地 zero 的全树比对

本地输入为 `/Users/chengliang/code-repositories/zero`，确认没有 `.git` 元数据。比对基于远程 `git ls-tree -r HEAD` 和本地 `find` 的全文件清单，再对共同文件计算 Git blob SHA-1；同时核对可执行文件模式。原始清单、哈希和差异文件在 `/tmp/gcode-zero-source-intake-compare-99721c762f37cd43ac511007a5f51d1846df959e`。

| 项目 | 数量/结果 |
|---|---:|
| 远程提交树文件 | 1,539 |
| 本地文件 | 1,527 |
| 共同文件 | 1,526 |
| 共同文件内容差异 | 0 |
| 远程独有文件 | 13 |
| 本地独有文件 | 1（`.DS_Store`） |
| 远程非普通文件模式 | 3 个可执行文件；与本地同路径模式一致 |

远程独有的 13 个文件：

```text
.github/ISSUE_TEMPLATE/bug_report.yml
.github/ISSUE_TEMPLATE/config.yml
.github/ISSUE_TEMPLATE/feature_request.yml
.github/PULL_REQUEST_TEMPLATE.md
.github/workflows/ci.yml
.github/workflows/pr-auto-review.yml
.github/workflows/release-artifacts.yml
.github/workflows/release-please.yml
.github/workflows/zero-action-smoke.yml
CHANGELOG.md
LICENSE
README.md
README_ZH.md
```

本地独有的 `.DS_Store` 是操作系统元数据，不属于 zero 源码。结论是：本地 zero 的共同源码与该 main 提交逐文件一致，但本地不完整，缺少上述上游仓库文件；导入必须以固定快照为准，并显式决定哪些文档、许可证和维护文件进入 GCode。

## 许可证和第三方声明

### 上游项目许可证

- 快照根目录存在 `LICENSE`，内容为 MIT License，版权行是 `Copyright (c) 2026 Gitlawb`，文件大小 1,064 字节，Git blob 为 `008f8978c797abf439984442996fcc399fad0181`。
- `package.json` 的 `license` 为 `MIT`；`README.md` 和 `README_ZH.md` 均再次指向根目录 MIT 许可证。
- 本地 zero 缺少根目录 `LICENSE`，所以正式导入时必须从固定快照补入；不能依据本地目录的缺失判断 zero 无许可证。

### 第三方材料现状

快照没有根目录 `NOTICE`、`COPYING`、`THIRD_PARTY_NOTICES` 或单独的 Go 依赖许可证汇总文件。现有可核实材料是：

- `go.mod` / `go.sum` 固定 Go 依赖版本；源码中存在少量针对具体依赖的注释，例如 `internal/imageinput/pdf.go` 标注 `github.com/ledongthuc/pdf` 为 BSD。它们不能替代完整的依赖许可证清单。
- `package-lock.json` 保存 npm 包的 `license` 元数据，统计值为 `MIT=64`、`Apache-2.0=3`、`ISC=5`、`BSD-3-Clause=1`、`MPL-2.0=1`，根包记录为 `SEE LICENSE IN LICENSE`。这些元数据属于 zero 的 npm 发布链路。
- `docs/NPM_PACKAGING.md` 明确写出 `agent-browser` 为 Apache-2.0，并要求其随发布包携带 LICENSE；该依赖及 `tuistory` 被放入发布 payload/辅助目录。GCode 计划移除 npm 用户入口，因此是否仍保留这些 npm 依赖必须由裁剪后的实际依赖图决定，不能把 lockfile 元数据当作已完成的 GCode 第三方声明。

GCode 导入边界：保留上游 MIT `LICENSE`；对实际保留下来的 Go、原生 UI 和其他运行时依赖重新生成或补齐第三方声明。上述 zero 来源段是历史输入核查，不替代当前分发许可清单；当前材料见 `docs/THIRD_PARTY_NOTICES.md` 与 `resources/licenses/`。

## 上游维护文件的处理

快照根目录有 `AGENTS.md`、`CONTRIBUTING.md`、`CODE_OF_CONDUCT.md`、`.github` 工作流及 release/npm 维护脚本。它们属于上游项目维护流程。GCode 以本工程的 `AGENTS.md`/用户指令和设计基线为准，导入时不复制上游 `AGENTS.md` 来覆盖本项目指令，也不保留上游维护工作流作为 GCode 产品功能。是否保留面向用户的 Zero 文档由 root 按裁剪范围单独决定。

## 用户宿主裁剪清单

session-runtime 已明确确认内核组装已经迁出。静态检查显示 `internal/runtime`、`internal/controller`、`internal/desktop`、`internal/providers`、`internal/workspace`、`internal/worktrees` 和 `internal/execution` 没有导入旧 `internal/cli` 或 `internal/tui`；`cmd/gcode` 走 application/controller/runtime 路径。

已完成的独立裁剪：

- 删除 `internal/release/`、`internal/agenteval/`、`internal/perfbench/`、`internal/review/`、`internal/contextreport/`、`internal/execprofile/`、`internal/update/`、`internal/installtest/` 和 `internal/npmwrapper/`。这些包的依赖图仅通向已删除的旧 CLI、性能/评测/发布/审查命令或旧安装链路；`internal/trace` 的契约测试已改为本地解析 NDJSON，不再反向依赖 agenteval。
- 删除 `cmd/zero-release/`、`cmd/zero-perf-bench/`、`cmd/zero-pr-review/`、`cmd/zero-linux-sandbox/`、`cmd/zero-seccomp/`、`cmd/zero-windows-command-runner/` 和 `cmd/zero-windows-sandbox-setup/`。`internal/sandbox` 的 Windows runner/setup 后端保留，由 GCode 主入口通过 `__windows-command-runner` / `__windows-sandbox-setup` self-dispatch 调用。
- 删除 `bin/`、`packaging/`、npm/action/release 自动化入口、旧安装脚本、`scripts/sandbox-smoke.sh` 和上游旧 CLI 参考文档。当前 `docs/` 只保留 GCode 的设计、计划、目标、验收和来源记录。

已完成的宿主删除：

| 范围 | 处理 |
|---|---|
| `cmd/zero/` | 删除旧 zero CLI 用户入口；不改 root 新增的 `cmd/gcode/` |
| `internal/cli/`、`internal/tui/` | 删除 CLI/TUI 宿主及其专用测试 |

第二轮宿主审计（2026-10-12）又删除了只服务旧用户入口、远程宿主或旧维护流程的包：

- `internal/acp/`、`internal/agentinit/`、`internal/browser/`、`internal/cron/`、`internal/daemon/`（含 `remote/`）、`internal/dictation/`、`internal/observability/`、`internal/privatedir/`、`internal/provideroauth/`、`internal/provideronboarding/`、`internal/reltime/`、`internal/repoinfo/`、`internal/search/`、`internal/selfverify/`、`internal/terminalpet/`、`internal/usage/`、`internal/usercommands/`、`internal/workspacetrust/`、`internal/zerocommands/` 和 `internal/zerogit/`。
- provider 输入裁剪随后删除了未进入当前产品入口的 `internal/aimlapi/`、`internal/doctor/`、`internal/providerhealth/`、`internal/providermodeldiscovery/` 和 `internal/notify/`。`internal/providercatalog/` 仍保留 AIMLAPI 的通用 API key 描述，但不再提供 partner header、充值 onboarding 或推荐标记；`internal/errhint/` 仅保留 agent reconnect 使用的 `HasStatusCode`，分类和 CLI/TUI 文案提示已删除；`internal/specmode/` 仍由 agent 的模式合同和测试覆盖，`internal/testutil/` 仍被保留包的测试共享。
- `internal/swarm` 的注释和测试说明已移除对已删除 daemon 包的引用；repomap 测试样例改用仍存在的搜索文件。

以上表格和独立裁剪均已完成；`Makefile`、`README*`、`go.mod` 和 `.gitignore` 由 root 维护，本目标不修改。

明确保留并重新检查引用：

- `internal/mcp`、`internal/skills`、`internal/hooks`、`internal/plugins`、`internal/specialist`、`internal/swarm`、`internal/sandbox`、`internal/providers` 和 `internal/tools`。它们属于桌面 runtime 能力，不能因原先由 CLI/TUI 组装就删除。
- `internal/oauth` 目前仍被 MCP OAuth 路径引用。模型订阅专用认证由 api-providers 目标另行裁剪；MCP 标准协议所需认证不能一并移除。
- `internal/tools/localcontrol` 仍被内置本地工具直接引用，不属于可随 CLI 入口删除的孤立包。
- `internal/sandbox` 的 Windows runner/setup 后端暂留；root 负责从主入口接入 self-dispatch、GCode 命名和打包路径。

删除前后已用 `go list -deps` 和受影响包测试确认没有宿主入口引用；删除动作不保留兼容 wrapper 或回退路径。Windows sandbox 仅保留 runtime 后端，独立旧 helper 命令入口已删除。

验证记录（2026-10-12）：`GOCACHE=/tmp/gcode-go-cache-intake go list ./...` 通过，保留 runtime/controller/application/desktop/provider/workspace/worktrees/execution 依赖图不含已删除旧宿主包。受影响包定向测试中，`cmd/gcode`、application、controller、runtime、desktop、providers、workspace、worktrees、execution、trace、repomap、workspaceindex、workspaceseed 和 tools 均通过；sandbox 仅剩 GCode 配置目录重命名相关的 `TestCredentialDeniesMatchTokenStoreFallbacks` 失败。全量 `go test ./...` 的编译阶段通过，但被 imageinput 本机剪贴板 helper 缺失及 plugins/skills 中断恢复测试失败阻断；`go vet ./...` 被并行 desktop toolbar 参数和 sandbox 测试变量问题阻断。这些未通过项不产生旧 CLI/TUI 或已删除发布包引用。

## 导入结论

1. 可复制输入已经固定到 `99721c762f37cd43ac511007a5f51d1846df959e`，并有 clone、归档、API 响应和全树比对证据。
2. 本地 zero 不是完整快照：共同文件内容完全一致，但缺少 13 个远程文件；其中根 `LICENSE`、README、CHANGELOG 和 GitHub 维护文件是主要差异。
3. root 可以从 clone 目录导入，再按设计移除 CLI/TUI、订阅认证、npm 入口和上游维护流程；此处不修改产品源代码，不追踪后续 zero 更新。

## GCode 当前分发许可记录（2026-10-12）

当前 GCode 的第三方声明和原始许可文本集中在 [`docs/THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md) 与 [`resources/licenses/`](../resources/licenses/)；此处补充来源链路，避免把上游 zero 的历史核查段与当前分发材料混为一谈。

- 当前生产图由 `GOCACHE=/tmp/gcode-go-cache-intake go list -deps ./cmd/gcode` 核验，Windows 条件依赖由 `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 GOCACHE=/tmp/gcode-go-cache-intake go list -deps ./cmd/gcode` 补查。版本来自仓库 `go.mod`/`go.sum`；MyGo 通过 `go.mod` 的 `replace github.com/egoist/mygo => ./third_party/mygo` 使用仓内 fork，其余模块来自 `/Users/chengliang/go/pkg/mod` 缓存。
- `resources/licenses/` 保留了生产图所需模块、Windows 条件模块、现有 `go.mod` 声明中的固定版本，以及 MyGo terminal 所携带的原生资源许可。原文未重新排版或改写。
- 仓库根目录 `LICENSE` 与 zero 固定提交 `99721c762f37cd43ac511007a5f51d1846df959e` 的 MIT 原文一致；资源目录另保存带固定提交标识的 zero MIT 原文，SHA-256 为 `67b7a613596a56e90712dd5a8450a4507ec3cb5a6db09425cf7a72a4b0ec89b7`。
- MyGo `v0.4.3-0.20261011132240-65154ea8777c` 的 terminal manifest 指向 Ghostty `befcdfd2c3a1cb24d9ec886e93c95b2b5daa7028`，manifest 为 darwin-amd64 资源声明的原始 hash 是 `421e98d2683b39555209c59395655b4a217da0382e7669c92f6dff3949832664`。当前工作区 `.mygo/dev/darwin-amd64/GCode Dev.app/Contents/Resources/libghostty-vt.dylib` 实测 SHA-256 是 `80144a339496a9f6035c21734b57e3936f0c5971fddb5d57a1dfa4663f17c096`，二进制元数据为 `library_version=0.1.0-dev`、`commit=null`、`dirty=null`。将原始缓存文件复制到 [`output/acceptance/stage-5/native-provenance/`](../output/acceptance/stage-5/native-provenance/) 后，按 `third_party/mygo/cmd/mygo/resources.go` 的实际参数执行 `codesign --force --sign - --preserve-metadata=entitlements`，签名副本得到同一 `80144…` hash，并与当前包内 dylib 逐字节一致；两者 ad hoc CDHash 均为 `3471fbc84b9632ab6f7605c4eeaee8e48d7c634e`。
- 本机没有 Ghostty 源码 checkout 缓存；固定提交的 MIT `LICENSE` 原文已单独保存，并在第三方声明中标明其上游固定来源。原始资源 hash 与签名后分发 hash 的差异由 MyGo 嵌套签名实际复现解释，完整记录见 [`verification.txt`](../output/acceptance/stage-5/native-provenance/verification.txt)。
- 当前生产图已通过根 `go.mod` 的 `replace github.com/egoist/mygo => ./third_party/mygo` 使用相对 fork；其 `LICENSE` 与 module cache 和 `resources/licenses/` 中的 MyGo 原文逐字一致，SHA-256 为 `f0b56f4ab53beb6e18c5e155219c2e2b6000b5475c0ae3ddb3af3894fd9349b4`，因此不重复登记同一份文本。仓内 fork 的生产源码另有 `plugins/terminal/internal/pty/pty_darwin.go` Darwin PTY 阻塞模式修改，不新增许可文件。若 fork 后续改变许可证，须按 fork 实际版本补充独立声明。

本段的复核命令、模块清单、原文文件映射和 dylib 字节级来源复核见 `docs/THIRD_PARTY_NOTICES.md`、`docs/goals/third-party-notices.md` 与 `output/acceptance/stage-5/native-provenance/verification.txt`。
