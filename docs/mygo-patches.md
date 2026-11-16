# MyGo v0.9.1 固定补丁

2026-11-16：从官方 v0.8.3 升级到 **v0.9.1**（提交 `13a0b3e7b3e161454889ff8e1bbf16ab6ffbbe0d`），
本地修订为 `rcode-native-ui.27`。使用经 Go 模块校验的上游源码，
`go.mod` 仅替换至 `./third_party/mygo`。旧补丁文件已删除，可从 Git 历史追溯。

## 补丁核对与应用顺序

现有 18 项补丁均逐项与新上游核对。上游只吸收了富文本 composer 补丁中的键盘与命令回调部分，
因此该补丁缩减但未整体删除。其余仍需保留；不为废弃接口增加兼容层。

| 顺序 | 功能 | v0.9.1 处理 |
| --- | --- | --- |
| 1 | [动效偏好](../patches/mygo-v0.9.1-01-motion.patch) | 保留窗口级动效与滚轮缓动偏好。 |
| 2 | [Markdown 行高](../patches/mygo-v0.9.1-02-markdown-line-height.patch) | 保留精确 DIP 行高。 |
| 3 | [弹层贴窗](../patches/mygo-v0.9.1-03-snap-to-window.patch) | 保留 SnapToWindow 定位。 |
| 4 | [输入显示快照](../patches/mygo-v0.9.1-04-input-display-text.patch) | 保留未提交输入法文本的显示快照。 |
| 5 | [Windows UIA](../patches/mygo-v0.9.1-05-windows-uia-provider.patch) | 保留服务端 provider 选项及 COM 契约测试。 |
| 6 | [富文本 composer](../patches/mygo-v0.9.1-06-richtext-composer.patch) | 键盘与编辑命令改用上游 OnKey/OnCommand；删除旧 OnPaste 和旧键盘签名，保留显示/组合状态快照、内容边距与固定行高。补齐命令回调对快捷键粘贴的覆盖及回调前快照。 |
| 7 | [正文链接选择](../patches/mygo-v0.9.1-08-inline-link-selection.patch) | 保留拖拽越过阈值后的段落选择。 |
| 8 | [折叠动效](../patches/mygo-v0.9.1-09-collapsible-animation.patch) | 保留可配置折叠动画。 |
| 9 | [WebView 数据目录](../patches/mygo-v0.9.1-10-webview-data-directory.patch) | 保留 Windows WebView2 独立目录；按上游拆分结构接入 webview2_env.go，保留无 WebView 构建。 |
| 10 | [Windows 字形定位](../patches/mygo-v0.9.1-11-windows-glyph-placement.patch) | 保留 ClearType quarter 定位与缓存隔离，适配上游新增描边/阴影字形参数。 |
| 11 | [链接绘制](../patches/mygo-v0.9.1-12-rendering-parity.patch) | 保留点状下划线与字形绘制装饰。 |
| 12 | [Tooltip 基础能力](../patches/mygo-v0.9.1-13-tooltip-primitives.patch) | 保留点定位与输入事实；使用新版 markAbsolute 和布局缓存分类。 |
| 13 | [跨虚拟化选择](../patches/mygo-v0.9.1-15-selection-spans.patch) | 保留选区 span 快照。 |
| 14 | [分栏拖动](../patches/mygo-v0.9.1-18-split-drag.patch) | 保留 20 DIP 命中区与同帧尺寸写入，并合入上游越界回退距离保留逻辑；两类回归同时保留。 |
| 15 | [富文本裁切](../patches/mygo-v0.9.1-20-richtext-clip.patch) | 保留编辑区绘制裁切与像素回归。 |
| 16 | [行内代码字号](../patches/mygo-v0.9.1-21-inline-code-size.patch) | 保留行内代码与代码块独立字号。 |
| 17 | [生成 API](../patches/mygo-v0.9.1-17-api-gen.patch) | 基于 v0.9.1 重新生成最终 API 包装，仅生成一次。 |
| 18 | [构建身份](../patches/mygo-v0.9.1-19-patch-identity.patch) | 直接固定 v0.9.1 / rcode-native-ui.27，删除中间旧版本身份。 |

## 身份与重放

源码锁为 [`third_party/mygo.lock.json`](../third_party/mygo.lock.json)，
检查入口为 [`scripts/frameworkpin/pin.go`](../scripts/frameworkpin/pin.go)。
锁包含上游模块校验和、补丁顺序与各补丁 SHA-256、源码文件数和树指纹。
树指纹对 UTF-8 路径按字节排序，再逐项哈希 `路径 + NUL + 文件 SHA-256 十六进制 + LF`。

从校验过的官方 v0.9.1 模块源码开始，按锁内顺序执行 `git apply --check` 与 `git apply`，
得到的文件集合及每个文件字节必须与当前 `third_party/mygo` 完全一致。
公开 API 包装在其他功能补丁之后生成，构建身份最后应用。

```sh
go run ./scripts/check-framework.go
go tool mygo vet .
```

## 应用迁移范围

阅读新版 `ui/doc.go` 和 `docs/ui/migration.md`，预览整个仓库的 `migrate-ui`，
仅对应用 `internal` 运行写入迁移，避免改写上游示例和工具自身。
迁移把即时构造容器的 key 放到 `Context.Key`，不改变控件视觉规格。
存储元素 key 的剩余提示按构造点人工核对，不把展示投影改为持久元素。

SQLite 上游移除的旧动态库变量在 GCode 应用层没有调用；不新增旧路径回退。
Ghostty 原生库清单与许可证原文经逐字比较保持不变。

验证结果见 [升级验证](mygo-upgrade-2026-11-16.md)，实际安装状态见交付记录；编译、组件回归、原生窗口、
Windows 交叉编译分别记录，不以交叉编译代替 Windows 原生验收。
