# 正式应用包：原生设置、恢复与运行采样

日期：2026-10-12（Asia/Shanghai）。环境：MacBookPro16,1，macOS 14.8.7（23J520），Intel darwin/amd64。

状态：本记录的原生设置、即时保存反馈、退出重开与 Intel 运行采样已通过。完整首版和用户视觉认可仍未通过。

身份说明：本文 `final/` 和 `regression/` 对应优化前的正式包 `f1269b…a7b8`，保留原始结果。之后 `internal/desktop/app.go` 的绘制快照性能修复已重建正式包；当前包与回归见 [current-package.md](current-package.md)，新包长历史原生结果见 [native-conversation.md](native-conversation.md)。本文的三次设置/恢复采样没有在新包重复执行，不能把旧 manifest 称为当前包身份。

## 验证范围

驱动 `docs/verification/scripts/native-product-qa.py` 从 `build/darwin-amd64/GCode.app` 复制隔离应用身份，重新 ad hoc 签名，只修改 bundle 名称、显示名称、标识和签名。验证原包与隔离包的 Mach-O `__TEXT,__text` 指纹、全部 Resources 指纹一致，并在结束后复核原包未变。

隔离应用使用唯一的 Application Support 目录、固定无效假 key 和 `GCODE_CRED_STORAGE=encrypted-file`。端点固定为 `http://127.0.0.1:9/v1`；不发送编码任务，不调用真实厂商服务，不使用用户 Keychain。通过 macOS System Events AX 控件操作真实设置页，保存浅色、默认队列和两个服务，创建默认不启用 worktree 的会话，并检查单次选择“纠正”不改变全局默认。

预期完成三次独立启动和正常退出；重开后同时检查 UI 与 `controller.json`，逐个确认模型服务的已有密钥状态。启动耗时从进程创建到 AX“设置”控件可用，包含 AX 轮询开销；文件缓存可能已暖。设置页切换采样包含 `osascript` IPC，不能当作纯渲染帧延迟。空闲内存只记录进程 RSS，不代替全系统内存或峰值。

## 已发现并区分的问题

| 证据轮次 | 现象 | 判断与处理 |
| --- | --- | --- |
| 初次运行 `output/acceptance/native-product/` | 首次写入前读取不存在的 `controller.json` | 夹具错误。出厂默认从 AX 控件读取；保存后等待文件存在。 |
| `round-2/` | 异步保存服务后，等待条件遍历尚为 null 的 provider 列表而抛出 TypeError | 夹具错误。实际文件随后已保存首个服务；等待条件将 null 视为空列表。项目/会话等待同步修正。 |
| `round-3/` | 首次保存两个服务、主题/队列和默认会话成功；重开设置页却没有“本机凭据存储中已有密钥” | 产品缺陷。加密凭据文件仍存在；`Provider.HasAPIKey` 不序列化，controller 加载 settings 后没有从凭据库恢复该派生状态。修复由 runtime-boundaries 负责，并要求真实 Service 重开测试。 |
| `closure/` | 修复启动恢复后，原生驱动三次启动/退出、主题/队列、双服务及重开密钥状态全部通过；打开保存当下的截图发现当前编辑器仍显示“尚未配置密钥” | 后续确定性行为缺陷。dirty draft 阻止服务快照刷新派生密钥状态；保存成功的当下反馈仍滞后，不能因重启断言通过而忽略。由 provider-save-feedback 修复；驱动已补保存当下状态断言，随后只重验这条真实包设置链路。 |

驱动额外修正了凭据文件断言路径为 `data/credentials/credentials.enc`，并在切换 provider 后等待对应编辑器实际渲染。失败轮次保留，不能用于声明完整验收通过。每次异常均由 finally 尝试原生菜单退出，必要时终止隔离进程；现场进程检查未发现遗留 QA 应用。

## 最终结果

最终证据目录为 `output/acceptance/native-product/final/`。`regression/native-product-result.json` 记录驱动 exit 0，且从测试到构建、签名、原生驱动结束的源码指纹保持一致。最终成功后停止重复采样。

- 初始默认方式为“纠正”；AX 保存浅色和默认队列后，UI 与持久化文件一致。
- 通过真实设置页添加两项模型服务；每次保存完成的当下，当前编辑器即显示“本机凭据存储中已有密钥”。
- 两次完整退出重开后，两项服务/模型、浅色和默认队列均恢复；逐个选中服务，已有密钥状态正确。
- 新会话默认 `Worktree=false`、cwd 为隔离项目根目录；单次选择“纠正”不改变持久化的默认队列，重开输入区继续采用默认队列。
- `controller.json` 和三份应用日志均不含固定假 key，凭据文件存在且不含明文假 key；使用的是隔离 encrypted-file 后端，不以此声称生产 Keychain 已实测。
- 三次退出均为 exit 0；结束后的进程清单没有 QA 应用或驱动。原包和生产源码未变。

保存当下和重开后的两张真实窗口图均已打开检查，绿色已有密钥提示正确。图像、PID、CGWindow 和 hash 见 `final/manifest.json`：保存图 PID 15734 / window 21905；重开图 PID 15836 / window 21917。

| 启动序号 | PID | 创建进程至 AX 设置控件可用 | 空闲 RSS | 退出码 |
| --- | --- | --- | --- | --- |
| 1 | 15734 | 3082.56 ms | 59612 KiB（58.21 MiB） | 0 |
| 2 | 15836 | 695.52 ms | 57016 KiB（55.68 MiB） | 0 |
| 3 | 15892 | 914.29 ms | 57784 KiB（56.43 MiB） | 0 |

五次设置页 AX 往返分别为 249.89、273.74、261.72、274.78、248.95 ms，中位数 261.72 ms。它们包含脚本 IPC；只有三次启动和五次切换样本，不能据此宣称稳定冷启动或纯界面渲染性能达标。

## 当前源码回归与应用包

`output/acceptance/native-product/regression/` 已记录：

- `go test ./... -count=1 -timeout=180s`：exit 0。
- `go vet ./...`、`make fmt-check`：exit 0。
- execution/workspace/tools 完整 race：exit 0，见 `core-race.log`；controller/runtime、application/worktrees 的完整 race 复用 `output/acceptance/runtime-boundaries/` 的同源码通过记录；本次 desktop 保存反馈的普通/race 见 `docs/verification/provider-save-feedback.md`。
- `make package` 与 `codesign --verify --deep --strict build/darwin-amd64/GCode.app`：exit 0。

完整测试源码指纹为 `f8ca1546a09df0359d40bd873ab049b0ffe684db84f4a2ee0b829eabe35f8d75`，清单含 cmd/internal/mygo 的 Go 文件及模块文件。原包可执行文件 SHA-256 为 `f1269b5c8a132a898565c9e06ab4660b389163158e814ee29986ac1f69b3a7b8`；隔离包重新签名后完整文件 hash 不同，实际 `__TEXT,__text` 指纹与全部 Resources 均保持一致。manifest 另记录驱动期间生产源码、原包和资源未改变。

物理键盘、输入法组合、真实 API、Apple Silicon 和 Windows 均未验证。本记录不替代用户视觉评价。
