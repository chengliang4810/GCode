# /goal：正式应用包的原生设置与运行采样

状态：已完成本目标限定的正式包设置、恢复与 Intel 运行采样。责任为 root；完整 GCode 根目标仍执行中。

目标：从构建好的 GCode.app 建立仅替换应用标识并重新 ad hoc 签名的隔离副本，验证真实设置操作、重启持久化、多服务和默认发送方式；记录当前 Intel Mac 的启动、空闲 RSS 与原生界面操作耗时。

完成判据：

- 明确原包与隔离包的身份、资源和可执行代码指纹；生产 GCode 与原 zero 数据不读写。
- 通过原生 AX 操作保存主题、默认发送方式和两个模型服务，完整退出后重开，UI 和 controller.json 一致；单次发送方式选择不改全局设置。
- 固定假 key，使用隔离目录的 encrypted-file 凭据，不写用户 Keychain。
- 至少三次进程启动和实际原生操作的原始采样，说明 AX 调用与文件缓存的测量边界，不用人工门槛虚构性能通过。
- 正常退出、没有遗留 QA 进程，原包不变；物理键盘、IME、真实厂商 API 和其他设备分别保持未验证。

所有权：`docs/verification/scripts/native-product-qa.py`、`docs/verification/native-product.md`、`output/acceptance/native-product/`，不修改产品界面。

完成证据：`docs/verification/native-product.md` 与 `output/acceptance/native-product/final/`。修复启动恢复密钥标记和保存当下派生状态后，三次真实包启动/正常退出、双服务、主题/默认方式、即时提示和重开恢复均通过；对应最新源码全仓 test/vet/fmt、关键包 race、正式包构建和签名检查通过。两张设置图已实际打开检查，没有遗留 QA 进程。

此前失败轮次保留作夹具错误和产品缺陷证据；最终成功后不重复采样。本目标没有替代用户视觉认可、物理键盘/IME、真实 API 或其他设备验收。
