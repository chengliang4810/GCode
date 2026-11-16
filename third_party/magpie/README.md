# magpie 脱敏引擎

来源：本机 `/Users/chengliang/code-repositories/magpie`，
上游仓库 `github.com/yetone/magpie`，固定提交
`8608ce0243fd79d49d2e7e5a8593644b5ff7caef`。

导入时间：2026-11-16。源码采用 MIT 许可，完整许可见 `LICENSE`。
这里只包含 `redact`、`lazyre`、`pii` 和 `rosetta` 及相关回归测试；
未引入原项目网关、GUI、订阅、供应商目录或插件体系。

GCode 适配：

- Go import 路径改为当前模块；占位符密钥由核心选择持久化位置。
- `redact/stream_text.go` 在类型化流输出上处理占位符跨片段还原，
  不把 UI 与本机核心连接改为 HTTP。
- 修正 `MaskJSON` 按工具参数名识别秘密时绕过 Actions、Allow 和 Seen 的分支。
- 本地模型下载使用系统环境代理，保留固定版本与 SHA-256 校验；
  缓存位于系统用户缓存目录的 `GCode/privacy-models`，拒绝公共临时目录兜底、
  不可信父目录和链接文件。下载与解压通过 `os.Root` 约束文件打开边界。
- Privacy Filter 子进程沿用 GCode 的隐藏控制台规则。
- 移除旧 helper 版本回退、旧架构下载目录回退、旧推理缓存格式、
  自动升级与历史模型选择兼容逻辑。只读取当前固定模型/运行时版本。
- 测试夹具随测试一起导入；不导入原网关或真实模型评测环境。
  需要真实模型的 Guard 用例仅在显式设置 `RCODE_PRIVACY_TEST_MODEL_DIR` 时运行。

AstrLink Guard 和 Privacy Filter 模型由各自发布方以 Apache-2.0 提供。
仓库不包含两者的模型权重；下载的运行时压缩包中保留其自带许可。
Guard 的 CoreML 程序结构模板与运行实现随上游导入，实际权重从固定
模型文件生成，不包含在模板中。模型文件的固定版本、大小和校验值见
`pii/astrlink.go` 与 `pii/pii.go`。
