# MyGo v0.10.0 固定补丁

2026-11-23：从官方 v0.9.1 升级到 **v0.10.0**（提交 `ebde9399baf2734b3361856a53f4036859973d1d`），本地修订为 `rcode-native-ui.29`。上游输入由 Go 模块校验，`go.mod` 替换至 `./third_party/mygo`。

## 补丁核对与应用顺序

21 项补丁按源码锁顺序重放，适配新版弹层尺寸、触屏合成事件、Windows WebView2 状态和字形批处理。构建身份之后还有列表无障碍防护。

| 顺序 | 功能 | v0.10.0 处理 |
| --- | --- | --- |
| 1 | [动效偏好](../patches/mygo-v0.10.0-01-motion.patch) | 保留窗口级动效与滚轮缓动偏好。 |
| 2 | [Markdown 行高](../patches/mygo-v0.10.0-02-markdown-line-height.patch) | 保留精确 DIP 行高。 |
| 3 | [弹层贴窗](../patches/mygo-v0.10.0-03-snap-to-window.patch) | 保留 SnapToWindow 定位。 |
| 4 | [输入显示快照](../patches/mygo-v0.10.0-04-input-display-text.patch) | 保留未提交输入法文本的显示快照。 |
| 5 | [Windows UIA](../patches/mygo-v0.10.0-05-windows-uia-provider.patch) | 保留服务端 provider 选项及 COM 契约测试。 |
| 6 | [富文本 composer](../patches/mygo-v0.10.0-06-richtext-composer.patch) | 键盘与编辑命令改用上游 OnKey/OnCommand；删除旧 OnPaste 和旧键盘签名，保留显示/组合状态快照、内容边距与固定行高。补齐命令回调对快捷键粘贴的覆盖及回调前快照。 |
| 7 | [正文链接选择](../patches/mygo-v0.10.0-08-inline-link-selection.patch) | 保留拖拽越过阈值后的段落选择。 |
| 8 | [折叠动效](../patches/mygo-v0.10.0-09-collapsible-animation.patch) | 保留可配置折叠动画。 |
| 9 | [WebView 数据目录](../patches/mygo-v0.10.0-10-webview-data-directory.patch) | 保留 Windows WebView2 独立目录；按上游拆分结构接入 webview2_env.go，保留无 WebView 构建。 |
| 10 | [Windows 字形定位](../patches/mygo-v0.10.0-11-windows-glyph-placement.patch) | 保留 ClearType quarter 定位与缓存隔离，适配上游新增描边/阴影字形参数。 |
| 11 | [链接绘制](../patches/mygo-v0.10.0-12-rendering-parity.patch) | 保留点状下划线与字形绘制装饰。 |
| 12 | [Tooltip 基础能力](../patches/mygo-v0.10.0-13-tooltip-primitives.patch) | 保留点定位与输入事实；使用新版 markAbsolute 和布局缓存分类。 |
| 13 | [跨虚拟化选择](../patches/mygo-v0.10.0-15-selection-spans.patch) | 保留选区 span 快照。 |
| 14 | [分栏拖动](../patches/mygo-v0.10.0-18-split-drag.patch) | 保留 20 DIP 命中区与同帧尺寸写入，并合入上游越界回退距离保留逻辑；两类回归同时保留。 |
| 15 | [富文本裁切](../patches/mygo-v0.10.0-20-richtext-clip.patch) | 保留编辑区绘制裁切与像素回归。 |
| 16 | [行内代码字号](../patches/mygo-v0.10.0-21-inline-code-size.patch) | 保留行内代码与代码块独立字号。 |
| 17 | [差异折叠条](../patches/mygo-v0.10.0-22-diff-no-separators.patch) | 隐藏折叠条及其占位并保留隐藏上下文。 |
| 18 | [安装包命名](../patches/mygo-v0.10.0-23-macos-dmg-name.patch) | 保留 GCode 安装包架构命名。 |
| 19 | [生成 API](../patches/mygo-v0.10.0-17-api-gen.patch) | 以 v0.10.0 生成公开 API 包装。 |
| 20 | [构建身份](../patches/mygo-v0.10.0-19-patch-identity.patch) | 固定 v0.10.0 和 rcode-native-ui.29。 |
| 21 | [列表无障碍防护](../patches/mygo-v0.10.0-24-list-state-a11y-guard.patch) | 保护列表状态释放后的无障碍读取。 |

## 身份与重放

源码锁为 [`third_party/mygo.lock.json`](../third_party/mygo.lock.json)，
检查入口为 [`scripts/frameworkpin/pin.go`](../scripts/frameworkpin/pin.go)。
锁包含上游模块校验和、补丁顺序与各补丁 SHA-256、源码文件数和树指纹。
树指纹对 UTF-8 路径按字节排序，再逐项哈希 `路径 + NUL + 文件 SHA-256 十六进制 + LF`。

从校验过的官方 v0.10.0 模块源码开始，设置 `core.autocrlf=false`，按锁内顺序执行 `git apply --check` 与 `git apply`，
得到的文件集合及每个文件字节必须与当前 `third_party/mygo` 完全一致。
公开 API 包装在新增公开 API 的补丁之后生成。

```sh
go run ./scripts/check-framework.go
go tool mygo vet .
```

## 验证

上游模块校验成功。21 项补丁逐一通过 apply --check，重建的 3,049 个文件与当前源码逐字节一致。框架身份和 Windows 桌面设置控件回归通过。上游贡献筛选见 `reference/validation/` 下的阶段结果。

两项 Windows 字形测试失败，分别涉及变换裁切夹具和 DirectWrite 覆盖率对照。在当前机器用原 v0.9.1 源码复跑，两个测试同样失败。

Windows 原生 E2E 通过设置保存与重启恢复、模型选择器和 Markdown 表格。后两组覆盖深浅主题。证据在 `output/mygo-0100-e2e/`。应用全量测试中桌面包通过，整体存在失败，包括 Windows 符号链接权限和真实终端等待 shell 就绪超时。
