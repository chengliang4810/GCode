# Service 共享文件编辑竞争 S2

## 目标

补齐 A6/P3.2 在本地应用层的真实多会话证据：两个共享同一工作目录的
`application.Service` 会话先读取同一版本，再通过真实 Chat Completions
协议替身同时请求写入。写入经过 `controller.Controller`、`runtime.Engine`、
provider adapter 和内置文件工具，不能用 fake runner 绕过 Agent 内核。

## 判据

- 同一文件的旧内容竞争：双方都在写入前完成 `read_file`，并由确定性 barrier
  放行写入；最终至多一方成功，另一方返回“文件自上次读取后已改变”的可见工具
  错误；磁盘内容必须是完整的某一方结果，不能是静默覆盖或部分内容。
- 外部编辑竞争：会话完成读取后，由测试 fixture 以会话外的真实文件写入替换
  内容；随后内置 `edit_file` 或 `write_file` 必须拒绝陈旧写入，外部内容保持完整。
- 不同文件并发：两个会话在同一 barrier 后分别写入不同文件；两个工具结果均
  成功，两个文件均保留完整目标内容。
- 覆盖现有 `edit_file`（replace）和 `write_file`（`overwrite: true`）两条真实
  写入路径；不复制工具内部实现来断言结果。
- 每个 session 都必须严格匹配自己的 marker、路径、旧值/新值或完整 `content`、
  工具 call ID 和结果状态；`events.jsonl` 逐条解析并核对 `SessionID`、工具事件
  与 call ID。读取的第 3 次 provider 请求体，逐条核对 user marker、工具参数、
  `tool_call_id` 和工具结果，且不得出现另一 session 的 marker、路径或写入值。

## 边界

测试只使用临时真实文件、临时 Git 仓库和本地 `httptest.Server`。HTTP 服务只
替代模型协议，不代表真实厂商 API（S3）；凭据为测试值且不得进入输出。barrier
使用 channel/同步计数并设置有限等待，禁止用 `sleep` 作为并发成立的证明。
文件协调只证明 GCode 内置读写工具之间的边界；shell、外部编辑器和真实厂商
服务仍属于独立验收范围。

## 交付证据

- `internal/application/shared_edit_integration_test.go`：Service 端到端测试、严格
  session/marker/path/value 归属断言及有期限的 provider fixture/barrier。
- `docs/verification/shared-edit-service.md`：环境、原始命令、结果和限制。
- `output/acceptance/shared-edit-service/`：普通测试、race 测试和 vet 的原始
  脱敏日志。
