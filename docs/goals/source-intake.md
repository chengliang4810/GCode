# /goal：固定 zero 源码接管输入

状态：来源核验和两轮用户宿主裁剪已完成；等待 root 处理 Windows sandbox helper 的 GCode 命名、打包和 smoke 脚本。

## 目标

复核 `https://github.com/Twigpine/zero` 的 `main` 最新提交，把可复现的源码快照下载到临时目录，与本地 `/Users/chengliang/code-repositories/zero` 做全树比对，并记录许可证、第三方声明、导入边界和证据路径。该目标不修改 GCode 产品源码、`go.mod` 或参考仓库，不提交、不推送。

## 边界

- 只读检查本地 zero；本地目录没有 Git 元数据。
- 下载的 GitHub 快照和比对清单保存在 `/tmp`，由本目标独占管理。
- 不把上游 `AGENTS.md`、`CONTRIBUTING.md`、`.github` 工作流或其他上游维护流程当作 GCode 指令导入。
- 仅按“用户宿主裁剪”清单移除已确认废弃的 zero 入口；不复制或修改 root、runtime、provider、workspace、execution 和 desktop 所有权范围内的产品源码。
- 不建立 zero 上游后续同步机制；本次只固定一个来源提交。

## 已完成

1. 通过 GitHub API 在 2026-10-11（Asia/Shanghai）复核 `main`，最新 SHA 为 `99721c762f37cd43ac511007a5f51d1846df959e`，默认分支为 `main`。提交签名状态为 API 返回的 `verified: true`。
2. 以 `git clone --depth=1 --branch main` 下载源码，并通过 `git fsck --full --no-progress`；快照工作树状态干净，`HEAD` 与 API SHA 一致。
3. 另下载同一 SHA 的 GitHub codeload tarball。归档 SHA-256 为 `c7edc03623180937538a222ff21530a6bf8807bbfd8ac79c8508add93b95bd2d`，大小为 3,681,673 字节；解包后 1,539 个文件与 clone 工作树的文件清单和内容哈希完全一致。
4. 本地 zero 全树共 1,527 个文件（含本地 `.DS_Store`）；远程提交树共 1,539 个文件。共同文件 1,526 个，Git blob 内容差异为 0；远程多出的 13 个文件和本地独有的 `.DS_Store` 已记录在 `docs/source-origin.md` 及比对目录。
5. 复核根目录 `LICENSE`、`package.json`、`go.mod`、`package-lock.json`、`docs/NPM_PACKAGING.md` 和所有名称含 license/notice/copying/third-party/attribution 的文件。远程有 MIT 根许可证，没有独立 `NOTICE`、`COPYING` 或 `THIRD_PARTY_NOTICES` 汇总文件；npm lockfile 有依赖许可证字段，npm 文档明确记录 `agent-browser` 为 Apache-2.0 并要求发布包带随附 LICENSE。

## 用户宿主裁剪（已完成）

session-runtime 已确认 `internal/runtime` 和 `internal/controller` 可以独立组装；当前静态依赖检查确认桌面/runtime/controller/provider/workspace/execution 包不再导入旧宿主。已删除旧 CLI/TUI、发布、npm、评测和遗留维护入口，保留 runtime 仍需要的 MCP、Skills、Hooks、插件、specialist/subagent、sandbox 和 provider 包。

已完成且不依赖 CLI/TUI 组装确认的裁剪：

- 删除旧发布、评测、性能基准、PR review、上下文报告及其仅供旧命令使用的包：`internal/release/`、`internal/agenteval/`、`internal/perfbench/`、`internal/review/`、`internal/contextreport/`、`internal/execprofile/`、`internal/update/`、`internal/installtest/` 和 `internal/npmwrapper/`。
- 删除对应命令 `cmd/zero-release/`、`cmd/zero-perf-bench/`、`cmd/zero-pr-review/`、`cmd/zero-linux-sandbox/`、`cmd/zero-seccomp/`、`cmd/zero-windows-command-runner/` 和 `cmd/zero-windows-sandbox-setup/`；Windows sandbox 由 GCode 主入口的内部 self-dispatch 承担，`internal/sandbox` 的后端能力保留。
- 删除 npm、旧分发和上游自动化入口：`bin/`、`packaging/`、`package.json`、`package-lock.json`、`action.yml`、旧安装/npm/action 脚本、release-please/Coderabbit 配置，以及 `CONTRIBUTING.md`、`SUPPORT.md`、`SECURITY.md`。
- 删除旧 CLI/TUI 参考文档和 `scripts/sandbox-smoke.sh`，仅保留 GCode 的 `design/`、`plans/`、`goals/`、`verification/` 和 `source-origin.md` 文档。
- 第二轮审计删除仅服务旧宿主、订阅登录、远程 daemon 或旧维护入口的包：`internal/acp/`、`internal/agentinit/`、`internal/browser/`、`internal/cron/`、`internal/daemon/`（含 `remote/`）、`internal/dictation/`、`internal/observability/`、`internal/privatedir/`、`internal/provideroauth/`、`internal/provideronboarding/`、`internal/reltime/`、`internal/repoinfo/`、`internal/search/`、`internal/selfverify/`、`internal/terminalpet/`、`internal/usage/`、`internal/usercommands/`、`internal/workspacetrust/`、`internal/zerocommands/` 和 `internal/zerogit/`。
- provider 输入裁剪又删除未进入当前产品入口的 `internal/aimlapi/`（邮箱登录、余额、充值和 partner session）、`internal/doctor/`、`internal/providerhealth/`、`internal/providermodeldiscovery/` 与 `internal/notify/`；`internal/providercatalog/` 仍保留 AIMLAPI 的通用 API key 描述，但不再注入归因 header、提供充值 onboarding 或标记为推荐 provider。`internal/errhint/` 仅保留 agent reconnect 使用的 `HasStatusCode`，删除无生产消费者的分类和 CLI/TUI 文案提示。

root 后续处理：

- root 在主入口增加 Windows sandbox 的 `__windows-command-runner` / `__windows-sandbox-setup` self-dispatch，并负责 GCode 命名及打包；`Makefile`、`README.md`、`go.mod` 和 `.gitignore` 由 root 维护，不在本目标内修改。

静态依赖结果：上述删除包均没有被 `cmd/gcode`、runtime、controller、desktop、provider、workspace 或 execution 的生产依赖图使用；`internal/specmode` 仍由 agent 模式合同和测试覆盖，`internal/testutil` 仍被保留包测试共享。保留的 runtime 仍通过 `internal/oauth` 使用 MCP/provider OAuth，不能把所有含 `oauth` 的包一并删除；`internal/tools/localcontrol` 仍被工具包使用，也不能因其过去由 CLI 组装而删除。`internal/sandbox` 的 Windows runner/setup 后端保留，root 将从主入口提供 self-dispatch。

验证结果（2026-10-12）：

- `GOCACHE=/tmp/gcode-go-cache-intake go list ./...` 通过；保留的 runtime/controller/application/desktop/provider/workspace/worktrees/execution 依赖图不含已删除旧宿主包。
- 受影响包及 GCode 入口定向测试大部分通过，包括 `cmd/gcode`、application、controller、runtime、desktop、providers、workspace、worktrees、execution、trace、repomap、workspaceindex、workspaceseed 和 tools。`internal/sandbox` 仍有 `TestCredentialDeniesMatchTokenStoreFallbacks` 失败，事实是测试期待旧 `.config/zero` 路径而当前 GCode 配置路径为 `.config/gcode`；该重命名由 go-toolchain/root 继续处理。
- `go test ./...` 已完成全量编译，但测试命令以失败退出：`internal/imageinput` 需要本机 `pngpaste` 或 PyObjC，`internal/plugins` 与 `internal/skills` 的中断恢复测试出现已有临时目录/事务状态失败，不能归因于本轮删除；本轮改动涉及的 trace/repomap/testrunner/verify 测试通过。
- `go vet ./...` 目前受并行 desktop 改动中的 `toolbar` 参数不匹配和 sandbox 测试未使用变量阻断，属于其他目标文件，不是裁剪引用错误。

## 证据

- Git clone 快照：`/tmp/gcode-zero-source-intake-99721c762f37cd43ac511007a5f51d1846df959e`
- 原始 codeload 归档：`/tmp/gcode-zero-source-intake-99721c762f37cd43ac511007a5f51d1846df959e.tar.gz`
- 全树及内容清单：`/tmp/gcode-zero-source-intake-compare-99721c762f37cd43ac511007a5f51d1846df959e`
- 来源与导入边界：`docs/source-origin.md`
- API 原始响应：比对目录中的 `github-commit-main.json` 和 `github-repository.json`

## 验证命令

```sh
git -C /tmp/gcode-zero-source-intake-99721c762f37cd43ac511007a5f51d1846df959e rev-parse HEAD
git -C /tmp/gcode-zero-source-intake-99721c762f37cd43ac511007a5f51d1846df959e fsck --full --no-progress
/usr/bin/openssl dgst -sha256 /tmp/gcode-zero-source-intake-99721c762f37cd43ac511007a5f51d1846df959e.tar.gz
```

当前结果是：`HEAD=99721c762f37cd43ac511007a5f51d1846df959e`、`fsck` 无错误、归档哈希为上述值。

## 交接

root 可直接从上述 clone 目录导入需要的文件；导入时保留 MIT `LICENSE`，按产品范围重新审查 Go/npm 依赖的许可证材料，并跳过上游维护指令和发布工作流。该快照代表一次性接管输入，之后不追踪 zero 的上游变化。
