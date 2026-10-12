# 原生 pending permission / ask_user 验收记录

状态：`PASS_FOR_RECORDED_SCOPE`。最新四场景证据位于 `output/acceptance/native-pending-ui/presentation-confirmed/`，历史失败轮次保留在同级 `round-*` 目录，不被覆盖。

驱动为 [`scripts/native-pending-qa.py`](scripts/native-pending-qa.py)，只接受本地临时 Chat Completions SSE fixture。运行顺序必须先通过 Python 语法和 fixture 单测，再运行默认四场景原生验收：

```sh
python3 -m py_compile docs/verification/scripts/native-pending-qa.py
python3 docs/verification/scripts/native-pending-qa.py --self-test
python3 docs/verification/scripts/native-pending-qa.py \
  --bundle build/darwin-amd64/GCode.app \
  --output output/acceptance/native-pending-ui
```

## 验收断言

| S3 断言 | 证据来源 | 状态 |
| --- | --- | --- |
| `require_escalated` 的 `allow` 通过原生卡片后真实写入 fixture 文件 | `presentation-confirmed/_fixture/permission-allow-deny-result.json`、临时项目检查 | PASS |
| `deny` 不写入文件，拒绝结果进入下一次模型请求 | provider 记录中的对应 `deny-call` tool result 与项目文件检查 | PASS |
| `ask_user` 的 `production` 选择进入下一次请求并持久化 history | `ask-history-result.json`、provider 记录、session `events.jsonl` | PASS |
| permission/ask_user 等待时发送纠正，旧 run 先 interrupted、replacement request ID 变化 | 两个 correction result、controller snapshot、old/replacement AX、结构化 `events.jsonl` 顺序 | PASS |
| 旧 permission/ask_user pending 不在 replacement UI 中，replacement 归属正确，旧 run 已取消 | old/replacement AX、snapshot、history、provider 记录 | PASS；journal 未提供显式 runner end acknowledgment |
| 迟到旧 request ID 的直接回放 | `permission_question_integration_test.go` S2 普通/race 覆盖；当前原生驱动不注入旧 request ID | NOT_INJECTED |
| 所有请求只发往 `127.0.0.1`；凭据不落盘到 evidence；源包代码和资源保持一致 | `presentation-confirmed/_fixture/manifest.json`、`results.json`、`review.json` | PASS_FOR_RECORDED_SCOPE |
| 四场景原生 pending 卡片截图可见并与 captures hash 相符 | `captures.json`、`review.json`、各场景 PNG/AX | PASS_FOR_RECORDED_SCOPE |

## 证据规则

- `presentation-confirmed/_fixture/results.json` 记录四场景 `PASS`、脱敏的 provider 布尔检查、结构化 correction 断言和 `NOT_INJECTED` 边界，不保存 API Key 或完整请求体；每个场景的 `*-result.json` 以及 controller/history 证据保留在场景目录。
- `presentation-confirmed/_fixture/manifest.json` 记录 driver/executable/code/Resources hash、源代码 hash、隔离策略、loopback 限制和剪贴板未触碰声明；本次 driver SHA-256 为 `7e63f3baf7833cf9b13378cd8d1e78f82e7c85f6adec82e729b0e49ec40c96c0`，正式包 executable/code SHA-256 分别为 `d0102a642e6c73f6a41b76070dce74e1a6de0fcfaaef9d1738a9a96fe805666a` 和 `78cf0dc07360914dfee73bd9c7582a400fd800291f51b29f4f0254771a656876`；fixture key 只存在于隔离 encrypted-file 凭据中，明文不进入证据，退出后移除 `credentials/`。
- `review.json` 的结论为 `PASS_FOR_RECORDED_SCOPE`；六张 capture 记录对应五个唯一图像 hash（permission allow/state 两张图相同），均已由 root 复核，QA PID 均已确认退出。
- native driver 只清理运行中的 app、HTTP server、osascript 和 PID；QA bundle、项目、截图、AX、controller/history 证据保留在场景目录，隔离 Application Support 也保留，但 app 退出后移除 `credentials/` 并记录该操作。
- round-1 至 round-5 的失败输出和日志继续保留，不能被最终 evidence 覆盖；其中 round-1 是驱动处理 `projects=null` 的问题，round-2 至 round-5 是驱动 AX 遍历方式的问题。另有 `presentation-final` 的历史驱动失败，review 已注明其发生在项目/会话创建前的 theme AX 路径，不据此宣称产品 runtime 失败。

## 限制

当前产品的 Service/controller 通过进程内 Go 接口工作，没有外部控制端点。驱动因此以真实 AX 操作触发允许、拒绝和回答；纠正场景用 controller 持久化 snapshot/history、provider 记录、old/replacement AX 和结构化 `events.jsonl` 断言确认旧 pending 被清除、旧消息中断以及 replacement 归属正确。当前原生驱动不伪造或注入旧 request ID 的迟到回复，结果标记为 `NOT_INJECTED`；更底层的迟到 request ID 直接回放仍由 `internal/application/permission_question_integration_test.go` 的 focused 普通/race 测试覆盖。journal 没有显式旧 runner end acknowledgment，因此不能把当前事件顺序断言扩展成完整 runner 取消生命周期证明。

`review.json` 还明确列出本记录范围之外的项目：真实厂商 API、显式迟到回复注入、显式 runner 取消确认、长 permission 滚动（仅 Tester 证据）、物理键盘或 IME、剪贴板、Apple Silicon、Windows 和用户视觉认可。
