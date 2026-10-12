# GCode

使用 Go 和 mygo 原生 UI 的编码 Agent 桌面工作台。UI、会话控制器和 Agent 内核在同一 Go 进程内，通过 Go 接口与事件连接。

项目正在实现首版。当前代码与验收范围分别记录在 `docs/design/gcode-desktop.md`、`docs/plans/gcode-desktop-implementation.md` 和 `docs/verification/`；编译、协议替身测试、原生窗口和真实模型调用分别验收。

## 本地开发

需要 Go 1.27.2 或更高版本。当前使用的 mygo 源码和本地修复已纳入 `third_party/mygo`，`go.mod` 使用仓内相对路径，不依赖原电脑上的 mygo 项目目录；不需要 npm 或前端构建工具。

```sh
make dev
```

直接构建桌面可执行文件：

```sh
make build
```

构建包含终端依赖的 macOS 应用包：

```sh
make package
```

`mygo build` 负责打包终端所需的 libghostty-vt。直接运行裸二进制不能代替应用包的终端加载验收。当前先验证 macOS；Apple Silicon 与 Windows 的原生运行结果需分别记录。

## 换电脑继续开发

源码仓库为私有的 [chengliang4810/GCode](https://github.com/chengliang4810/GCode)，默认分支为 `main`。新电脑先安装 Go 1.27.2 或更高版本、Git 和 GitHub CLI；macOS 还需要 `make` 等命令行工具，可通过 Xcode Command Line Tools 安装。macOS 最低版本为 13.0。

```sh
gh auth login
gh repo clone chengliang4810/GCode
cd GCode
go mod download
make package
open "build/darwin-$(go env GOARCH)/GCode.app"
```

以上打包和启动命令用于 macOS，会按新电脑的 CPU 架构生成应用包。首次构建需要联网下载 Go 依赖和终端原生库。修改代码时使用 `make dev` 启动开发应用；`make build` 只构建裸可执行文件。

仓库包含源码、内置字体、mygo 修复、设计与执行计划、验证记录及许可材料。构建产物、原生截图和运行日志位于被忽略的 `build/`、`.mygo/`、`output/`，不会通过 Git 同步。验证文档中的旧机器证据路径是历史记录，换机后需重新产生相应证据。

API Key、应用设置和会话历史存放在用户数据目录中，不随源码上传。新电脑首次启动后需在设置中重新配置 API 服务和 API Key；已有项目需要重新选择新电脑上的目录。macOS Intel 的现有运行验证不代表 Apple Silicon 或 Windows 已完成实机验收，Linux 不在当前产品范围内。

首次配置 OpenAI 官方服务时，Base URL 填写 `https://api.openai.com/v1`；输入模型和 API Key 后点击“保存服务”。“保存设置”保存通用设置，不会代替“保存服务”写入 API Key。

## 使用

从“项目 → 打开项目”选择本地目录，在设置中添加 API 服务、协议、地址、模型和 API Key，然后新建会话并发送任务。模型接入包括 Responses、Chat Completions、Anthropic Messages 和 Gemini，均通过独立协议适配器对接。

“纠正”会停止当前任务并等待取消完成，再发送新消息；“队列”在当前任务正常结束后依次发送。默认方式可在设置中选择。同一项目可同时运行多个会话，worktree 是可选的独立工作目录。

关闭窗口会保留后台任务，可通过 Dock 或托盘重新显示。显式退出会停止应用管理的任务并保留历史和工作目录。

开发应用使用 `GCode Dev` 数据目录，生产应用使用 `GCode`。macOS 位于 `~/Library/Application Support/`，Windows 位于用户应用数据目录。配置和会话快照不存储 API Key；凭据使用独立存储。

展示代表性 UI 样板（明确标记示例数据）：

```sh
GCODE_VISUAL_SAMPLE=1 make dev
```

## 验证

```sh
make test
make vet
make test-race
```

真实厂商 API、中文输入法、PTY 和目标设备验收需要相应环境，不能用模拟 HTTP 或离屏视图测试代替。

## 来源与许可

Agent 内核一次性接管自 zero，之后独立维护。固定提交、导入核验和裁剪记录见 `docs/source-origin.md`；保留原始 MIT 许可证。第三方声明位于 `docs/THIRD_PARTY_NOTICES.md`，原始许可位于 `resources/licenses/`，均随应用包分发。UI 框架依赖 mygo，视觉规范参考 motrix-mygo。
