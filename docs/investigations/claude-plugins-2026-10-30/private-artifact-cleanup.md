# 验收私有账户副本清理记录

2026-10-30 的真实模型验收通过明确传入的私有测试账户运行，没有修改用户原始
GCode profile，也没有修改或删除最初导出的私有 provider 配置文件。

修复前的 Runner 使用独立 Home/Store，但业务核心会将测试账户复制到该 Store 的
`engine.json`。以下 **10 份本任务生成的副本仍存在**。不要分享其所在的完整旧 run
目录；需要交付证据时，只选择经过审阅的脱敏日志、账本及 CWD 业务产物。

## 尚未清理的准确文件

```text
D:\projects\GCode\output\plugin-acceptance\run-2119488756\runtime\0dce42b03b0bea664345\probe-1060054246\home\.gcode\engine.json
D:\projects\GCode\output\plugin-acceptance\run-2119488756\runtime\0dce42b03b0bea664345\probe-3004666495\home\.gcode\engine.json
D:\projects\GCode\output\plugin-acceptance\run-2119488756\runtime\4d57bd70aefdd2e08785\probe-3152859547\home\.gcode\engine.json
D:\projects\GCode\output\plugin-acceptance\run-2119488756\runtime\9925dc81a451baeb9eed\probe-4200624587\home\.gcode\engine.json
D:\projects\GCode\output\plugin-acceptance\run-289796149\runtime\0dce42b03b0bea664345\probe-2009393734\home\.gcode\engine.json
D:\projects\GCode\output\plugin-acceptance\run-289796149\runtime\0dce42b03b0bea664345\probe-2675998932\home\.gcode\engine.json
D:\projects\GCode\output\plugin-acceptance\run-289796149\runtime\4d57bd70aefdd2e08785\probe-1802820046\home\.gcode\engine.json
D:\projects\GCode\output\plugin-acceptance\run-289796149\runtime\9925dc81a451baeb9eed\probe-3340614425\home\.gcode\engine.json
D:\projects\GCode\output\plugin-acceptance\run-2933970541\runtime\7b0c8c627e9a19212248\probe-3118139135\home\.gcode\engine.json
D:\projects\GCode\output\plugin-acceptance\run-654232493\runtime\7b0c8c627e9a19212248\probe-1153922933\home\.gcode\engine.json
```

只读核查确认：这些路径位于本任务的 `output/plugin-acceptance/run-*/runtime/*/probe-*`
之内，与各 run 的账本目录一致，且核查时路径祖先没有符号链接或 junction。
文件包含复制的测试账户值；本文不记录该值。

随后结合本地 Magpie 检查逐一核实：上述 10 份文件每份仅有一个非空 APIKey，
全部是已知的本地网关占位值，没有发现上游供应商 APIKey。这里的“账户副本”
不能解释为厂商密钥泄露；旧文件仍保持未修改。服务与预算证据见
`magpie-check.md`。

## 自动安全审查结果

递归删除这些准确 owned Home 的操作被自动安全审查拒绝。随后将操作缩小到上述
10 个 `engine.json`，通过 PowerShell `Remove-Item -LiteralPath` 逐一明确提供绝对
路径，仅使用 `-Force -ErrorAction Stop`、不使用递归参数，仍被拒绝。

工具返回的理由仅为：

```text
rejected: blocked by policy
```

这些删除均没有执行。已停止旧目录清理，没有更换 shell、语言、ACL 或其他路径
绕过拒绝。用户原始 profile、原始私有配置和这些旧副本均没有被该清理动作修改。

## 新 Runner 的清理边界

新实现从创建 owned Home 时即注册清理。每次 probe 返回前先关闭核心及 Store，
释放后台任务和 Windows 文件句柄，再校验准确的 `probeRoot/home` 路径、真实目录及
无链接祖先，通过绑定 `probeRoot` 的 `os.Root.RemoveAll("home")` 移除临时 Home。

成功与失败路径均清理。关闭或删除失败会合并到 probe 错误，使该 probe 不能被记为
通过。CWD 的实际业务文件、脱敏 snapshot/events/model HTTP 诊断及 trace 保留。
确定性本地 HTTP 集成测试验证了成功和失败后 Home 均不存在。

`run-2119488756` 在该修复前已经编译并启动，因此仍属于上面的待清理旧 run；
不能以新实现的测试通过，推断这些旧文件已被删除。
