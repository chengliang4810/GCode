# 供应商 Logo

在模型设置中点击供应商详情标题旁的图标，打开 Logo 选择框：

- 选择内置品牌后立即保存，供应商列表、模型选择菜单和当前模型名称同步使用所选 Logo。
- 点击“从网站获取”，根据当前填写的 Base URL 查找网站图标，获取成功后立即保存。
- 点击“使用默认图标”清除自定义 Logo，恢复通用供应商图标。

选择框仅显示 Logo，不显示品牌文字及介绍；悬停提示和可访问名称保留品牌名。
国内品牌优先排列：DeepSeek、通义千问、豆包、智谱、Kimi、MiniMax、
腾讯混元、文心一言、讯飞星火、百川。新增原始 SVG 来自固定提交的 Lobe Icons，
来源与文件哈希在 `internal/assets/brands/manifest.json`，MIT 文本随离线资源交付。

图标选择与供应商协议、名称、凭据和模型配置独立。同时保留原有 SVG：
OpenAI、Claude、Grok、Cursor、GitHub、Antigravity、Devin、Hermes、OpenCode、
Muse 和 Pi。品牌列表只提供图案，不构成固定供应商目录。

网站获取由真实 Go 业务宿主执行。Base URL 的路径不参与网站发现；
例如 `https://example.com/api/v1` 对应 `https://example.com/`。
优先读取首页声明的同源 `icon` / `apple-touch-icon` 链接，然后尝试
`/favicon.ico`、`/favicon.png`、`/apple-touch-icon.png` 和 `/favicon.svg`。
请求不附带模型 API Key 或 Cookie，不接受 URL 中的凭据或查询参数；
跨源图标链接和跨源重定向不会被跟随。API 域名没有网站图标时会显示失败提示，
保留原图标，用户仍可选择内置品牌。

请求总期限为 12 秒，每次 HTTP 请求最多 5 秒；图像最多 256 KiB，
栅格图像最多 1024×1024，保存前等比缩小至最长边 256 像素并转成 PNG。
ICO 使用固定版本的 Fyne image 解码器，并预先限制条目和分配大小。
SVG 仅接受自包含矢量图形，拒绝脚本、外部引用及不支持的元素。
获取期间显示进度并允许取消；取消、关闭或切换供应商后，过期结果不会覆盖当前选择。
失败与取消都不修改已保存图标及 API Key。

`model.Provider.Logo` 和公开供应商投影的 `Logo` 为同一字符串：
空值表示默认图标，内置值为资源 ID，网站图片为验证过的 PNG 或 SVG data URI。
下载内容随供应商配置保存和远程快照同步，重启及正常显示无需再次访问网站。
桌面本机通过类型化 `ProviderLogoService` 直接调用业务核心；
远程客户端仅在连接边界使用受宿主认证保护的 `POST /v1/provider-logo`。
Logo 获取及依赖不引入 MyGo 到 Linux 无桌面服务。

验证使用隔离本地网站与真实本机核心，不请求真实模型供应商。测试包含内置选择、
ICO/SVG/PNG、首页声明、失败、取消、外部引用、凭据隔离、保存重启和远程认证。
原生窗口证据与本轮全仓验证结果在 `output/provider-logo-final/`。

2026-11-03：原生最终证据为
`output/provider-logo-final/20261103-183529-2207168994/`。
深浅主题均完成内置选择、下载中取消、网站图标保存、失败保留、独立进程重启和
当前模型图标检查，已目视核对选择框与模型名称前的图片。核心/组件/远程专项、
HTTP 获取与远程接口竞态检查、共享控件门禁、相关 Go vet、
`go tool mygo vet ./internal/desktop`、离线许可资源校验及 Linux amd64
无桌面服务交叉构建通过；无桌面服务的 MyGo 依赖数为 0。
全仓 `CGO_ENABLED=0 go test ./... -timeout=2m` 未全部通过，
包含 Bash 临时目录、其他桌面交互/布局、Windows 卷名、引擎超时、
远程终端、终端宿主及壁纸读取失败；另有并行工作产生的
`output/panel-drag-close/validation` 示例编译失败。
详细日志为最终证据目录的 `full-suite.log`。未将专项验证记作全仓通过。

最终补验 `output/provider-logo-release-check/20261103-192943-2151682615/`
对当前源码重新构建，深浅主题均通过。取消用例确认网站图标请求已到达后，
通过弹窗关闭按钮取消并等待弹窗消失，再放行服务端响应，验证迟到结果被丢弃；
错误提示中的“取消”按钮亦在后续失败场景中验证。

2026-11-03 纯 Logo 与国内品牌追加验收：
`output/provider-logo-domestic/20261103-194307-3399499607/` 的深浅主题均通过。
原生树验证 21 个选项均有可访问名称、没有品牌文字及介绍段落，
选择 DeepSeek 后真实保存，网站获取/取消/失败/重启流程继续通过。
两种主题的图标网格已目视检查。全部品牌 SVG 解析、供应商组件、共享控件、
离线许可资源校验和 MyGo 桌面静态检查通过。
