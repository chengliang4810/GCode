# 原生 pending permission / ask_user 验收目标

## 目标

补齐当前正式 `GCode.app` 在原生窗口中的 pending 交互证据，覆盖 A8、P2.2、P2.3、P3.4 的 S3 缺口。验收驱动使用真实 `application.Service`、`controller`、`runtime` 和 Chat Completions 流式协议替身；协议替身只监听 `127.0.0.1`，不调用厂商 API。

## 场景

| 场景 | 原生动作 | 结果证据 |
| --- | --- | --- |
| `permission-allow-deny` | 发送任务，依次在真实授权卡片点击 `allow`、`deny` | allow 实际写入项目文件；deny 文件不存在；拒绝结果进入下一次 provider 请求 |
| `ask-history` | 发送任务，在真实提问卡片选择 `production` | 下一次 provider 请求包含回答；session history 含 `ask_user_answers` 与回答 |
| `permission-correction` | 旧 permission 等待期间发送纠正，新 permission 出现后允许 | 旧 run 标为 `interrupted`、旧文件未写入、新 run 写入新文件、request ID 更换 |
| `ask-correction` | 旧 ask_user 等待期间发送纠正，新问题出现后回答 | 旧 run 标为 `interrupted`、新问题 request ID 更换、replacement 归属正确；旧 request ID 迟到回复由 S2 测试覆盖，原生驱动标记 `NOT_INJECTED` |

## 所有权和边界

- 驱动：`docs/verification/scripts/native-pending-qa.py`。
- 无生产源码修改；正式包副本只修改 `Info.plist` 身份字段并重新 ad-hoc 签名。
- 每个场景使用独立临时项目、独立 bundle identity 和独立 Application Support 目录。
- fixture API Key 是固定无效的本地测试凭据；设置页会把它写入唯一隔离目录的 encrypted-file 凭据存储，脚本不会读取用户凭据、生产 Keychain、生产数据或真实 API，明文凭据不会进入证据，退出清理时移除隔离 `credentials/`。
- 不使用剪贴板；AX 脚本按目标 PID 绑定本场景窗口，所有等待和子进程调用有期限；`finally` 清理运行中的 app、HTTP server、osascript 和 PID，保留 QA bundle、项目、截图、AX、controller/history 和隔离数据目录作为证据，并移除隔离数据中的 fixture `credentials/`。

## 执行顺序

先做静态检查和本地 fixture 单测：

```sh
python3 -m py_compile docs/verification/scripts/native-pending-qa.py
python3 docs/verification/scripts/native-pending-qa.py --self-test
```

然后由 root 在 macOS 上串行执行默认正式包驱动：

```sh
python3 docs/verification/scripts/native-pending-qa.py \
  --bundle build/darwin-amd64/GCode.app \
  --output output/acceptance/native-pending-ui
```

原生运行前需要已构建的当前正式包和 macOS 辅助功能权限。`--self-test` 不打开窗口、不绑定 HTTP 端口；脚本本身不应由代理在验收前启动原生 app。

## 当前状态

最新记录范围已完成：`output/acceptance/native-pending-ui/presentation-confirmed/` 的 `review.json` 结论为 `PASS_FOR_RECORDED_SCOPE`，四个场景均为 `PASS`，且每个场景均通过菜单正常退出（exit code 0）。本次证据对应的正式包 executable SHA-256 为 `d0102a642e6c73f6a41b76070dce74e1a6de0fcfaaef9d1738a9a96fe805666a`，`__TEXT,__text` code SHA-256 为 `78cf0dc07360914dfee73bd9c7582a400fd800291f51b29f4f0254771a656876`，驱动 SHA-256 为 `7e63f3baf7833cf9b13378cd8d1e78f82e7c85f6adec82e729b0e49ec40c96c0`；manifest 同时确认源码和原始正式包在验收后保持不变。

最终场景证据包括：

- `permission-allow-deny`：真实 allow 写入项目文件，deny 未写入文件，下一次 provider 请求的对应 `deny-call` tool result 含 permission denied。
- `ask-history`：原生提问卡片选择 `production`，回答进入下一次 provider 请求并写入 session history。
- `permission-correction`、`ask-correction`：旧消息为 `interrupted` 且旧 pending 已被 replacement 清除；controller snapshot 中 old/new run、pending request ID 和归属一致，`events.jsonl` 的结构化顺序断言通过。

纠正场景的 `late_old_*` 仍为 `NOT_INJECTED`。原生驱动没有伪造或注入旧 request ID 的迟到回复；该行为继续由 `internal/application/permission_question_integration_test.go` 的 Service S2 普通/race 回放覆盖。`review.json` 明确记录 journal 没有显式的旧 runner end acknowledgment，因此这里不把 user → pending → replacement user → pending 的事件顺序描述成完整的旧 runner 结束证明。

历史失败轮次和原始日志保留在 `output/acceptance/native-pending-ui/round-*`，不得用最终 PASS 覆盖。round-1 的 `projects=null` 是驱动对持久化 snapshot 的处理问题，round-2 至 round-5 的授权卡片失败来自 AX `entire contents` 遍历方式；这些轮次不作为产品运行时失败结论。长授权内容滚动只有 Tester 证据，未纳入本驱动的完整原生断言。

本记录范围不包含真实厂商 API、迟到旧回复注入、显式 runner 取消确认、物理键盘或 IME、剪贴板、Apple Silicon、Windows 和用户视觉认可。
