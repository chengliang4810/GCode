# 设置页保存服务反馈验证

日期：2026-10-12（Asia/Shanghai）。

## 代码验证

- `go test ./internal/desktop`：通过。
- `go test -race ./internal/desktop`：通过。
- 定向行为测试：保存成功后刷新 `HasAPIKey`、保存失败保留草稿、保存期间较新草稿不被覆盖、切换到其他服务后保持当前编辑器：通过。

## 行为边界

`internal/desktop/app.go` 的脏草稿路径只从同一 `Provider.ID` 同步 `HasAPIKey`。服务名称、Base URL、API 类型、模型、API Key 输入、主题、发送方式和选中服务仍由当前草稿持有。保存失败不会执行快照刷新。

## 原生验收衔接

正式包的原生重启闭环已经验证保存后持久化状态；本修复对应保存当下的 AX 断言“本机凭据存储中已有密钥”。正式包重建和该原生断言由 root 负责。本文件只记录 desktop 模型层的确定性行为，不重复视觉 QA。
