# 软件更新与发布

GCode 使用仓库内 MyGo v0.7.3 的 `plugins/updater/native`。
更新窗口为原生 UI，不依赖 WebView；真实桌面普通启动注册插件，
确定性 `--scenario`、截图用 `--headless` 和 `cmd/gcode-server` 不注册。

侧栏底部右侧的圆形向上箭头仅在插件检测到新版本后显示。
未检查、已是最新版本、检查失败或跳过该版本时隐藏；选择稍后提醒或下载安装后
尚未重启时仍显示。**设置 → 通用 → 软件更新 → 检查更新…** 和原生应用菜单
始终保留手动检查入口。三处调用同一原生更新插件，不增加账户逻辑或额外轮询。
可见性通过当前固定补丁的 `AvailableVersion()` 与 `OnChange` 同步，
结果在主线程更新 UI 展示投影，不写入远程宿主或持久化陈旧版本标记。
后台按插件默认策略在启动约 10 秒后检查，此后每 24 小时检查；
默认不自动下载安装。更新窗口可以跳过版本、稍后提醒、下载并提示重启，
用户也可在其中开启自动下载。重启始终由用户决定；本机任务运行在桌面进程内，
应完成任务后再重启。远程宿主的任务不依赖客户端连接。
插件偏好位于当前桌面原生数据目录的 `updater.json`，不会发给远程宿主。

## 更新源

`mygo.json` 固定 `updates.github = "chengliang4810/GCode"`，
版本标签使用 `v<version>`。各平台分别读取正式 latest Release 的
`update-windows-amd64.json`、`update-darwin-amd64.json`、
`update-darwin-arm64.json` 或 `update-darwin-universal.json`。
草稿及 GitHub prerelease 不进入这个正式更新通道。

2026-10-29 核实仓库仍为私有；用户选择将现有仓库公开作为分发方案。
本轮不修改仓库可见性。在分发可更新版本前，仓库必须已公开，且正式 Release
必须包含对应平台清单和下载文件。当前未给客户端嵌入 GitHub token，
也没有私有仓库下载分支；私有状态下检查会失败。

## 更新签名

本次已生成专用 Ed25519 密钥对，公钥已写入 `mygo.json`。
私钥在本机用户目录 `~/.rcode/update-keys/mygo-update.key`，
公钥副本在同目录的 `mygo-update.pub`。Windows 私钥及目录只授予当前用户访问。
密钥不在 Git 仓库内，`.gitignore` 也排除了 `mygo-update.key`。

备份私钥到安全位置；发布后不能随意重新生成或更换，否则已安装的应用不会接受新签名。
换机时恢复同一私钥，而非再运行 keygen。
`MYGO_UPDATER_PRIVATE_KEY` 为 MyGo 上游要求的构建变量，内容是私钥本身。
本项目脚本也支持 `RCODE_UPDATE_PRIVATE_KEY_FILE`，其值为私钥文件路径。
两者均未设置时，脚本读取上述本机默认路径。
凭据不写入程序和更新清单。

同一私钥已配置为 `chengliang4810/GCode` 的 Actions secret
`MYGO_UPDATER_PRIVATE_KEY`。不要放入 GitHub Variables、源码、日志或 Release 附件。

## 构建

本机构建 Windows：

```powershell
./scripts/build.ps1 -Targets windows/amd64
```

在 macOS 构建三个 macOS 目标：

```sh
sh ./scripts/build-macos.sh
```

发布构建会先检查签名私钥是否与公钥匹配，缺少或不匹配就中止。
MyGo 生成安装包、签名 `.tar.gz` 更新包、每平台 `update-*.json`，
以及可生成时的增量 `.delta`。第一次发布没有历史版本，缺少增量包是正常情况。
构建后运行既有包内容校验，再核实清单版本、说明、日期、下载地址、
文件大小与每个新下载文件的 Ed25519 签名。普通 ZIP 不再重复包含更新包。

更新签名用于验证下载文件，不等同于 Windows 代码签名或 Apple 签名/公证。
现有脚本的 macOS 构建仍跳过公证，其他机器的系统信任验收需要另行完成。

## 发布流程

1. 更新 `mygo.json` 的 `version`，并在 `CHANGELOG.md` 添加对应 `## <version>` 段落。
2. 提交待发布源码并推送对应 `v<version>` 标签。
3. `.github/workflows/release.yml` 在 Windows/macOS runner 上执行 race 测试、
   构建与签名校验；所有目标通过后统一上传到 GitHub **草稿 Release**。
   手动触发也必须选择与配置版本相符的标签。
4. 检查四个平台的更新包、清单和安装包，完成实际安装更新验证。
5. 确认仓库公开后，手动发布草稿并设为 latest。流程不会自动公开仓库或发布草稿。

工作流只覆盖桌面发布；不部署或更新 Linux 无桌面宿主。
本地构建也不会上传文件、创建标签或发布 Release。
开发运行 `go run ./cmd/rcode` 不携带发布构建元数据，手动检查会显示无法自更新。

## 验证范围

实现检查与真实分发验收分别记录。原生设置入口测试覆盖中文/英文、
深浅主题和 900/1320 窄宽窗口；发布校验测试覆盖文件被篡改、
签名/密钥错误、清单缺失、版本/地址/大小错误及未签名增量。
在仓库公开并发布至少两个签名版本后，仍需验证真实 GitHub 下载、
安装替换、重启及用户数据保留。跨平台安装更新不能用构建通过代替。

2026-10-30 本机验证：全仓 `CGO_ENABLED=0 go test ./...`、Go vet、
MyGo vet、共享控件门禁、更新插件及原生窗口测试、MyGo updater 核心测试通过。
`actionlint` 通过；GitHub Actions 工作流尚未推送执行。
该次 `.10` 固定源码的 Windows amd64 与三个 macOS 目标签名构建、
包内容及更新签名校验通过，产物位于 `output/updater-release/`。
Windows 实窗使用独立 profile 点击更新入口，原生窗口确实请求了
`chengliang4810/GCode` 的正式更新清单并返回 404；
私有仓库且未发布正式更新时，这属于预期失败，不能记作更新下载安装成功。
实窗截图和日志位于 `output/updater-smoke-20261030/`。
macOS 仅完成交叉构建，原生安装、DMG、公证及实际跨版本升级仍待验收。
