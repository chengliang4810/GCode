# TabStrip 滚动条与渐隐 — R12

冻结参考为 Zeron `ab189159f10cf8e63c0502be75a59bbc487e0eaf`。
本阶段补齐原生标签滚动容器的命中区域和不透明背景下的边缘渐隐。
框架、实现回归、发布包、原生观察和原版视觉验收分别记录。

## 源码事实与修复

原版 `surface_chrome.rs:245–277` 使用36px全高渐隐，左侧90°、右侧270°，
判断为 `X > 1` 与 `X < MaxX - 1`，颜色来自 `theme.surface`。
本实现映射为 `p.Shell`，保留R11的38px标题栏、4px top inset、112×24标签、
24px滚动加号、28px固定展开按钮和4px间隙。

MyGo默认水平滚动条的命中轨道覆盖标签底部约7DIP。R12新增每个滚动容器独立的
`Scrollbars(false)`，仅应用于TabStrip，移除该指示条及其命中区域。
普通滚动、键盘选择、焦点、End滚入、辅助功能及其他滚动区域继续使用原生行为。
框架细节、补丁重放、默认行为与体积测量见 [固定补丁](mygo-patches.md)。

原先在构建时捕获上一轮MaxX，会在仅宽度/标签数量改变且X仍为0时画错渐隐。
真实Tester调度中，首次溢出、关闭非活动末项、窗口增宽/缩窄和同窗父列增宽/缩窄
共6项失败；一秒逻辑时间后无待绘帧，错误仍存在，不能靠强制两次Frame掩盖。
修复在绘制回调读取稳定的 `ui.ScrollState` 纯数据，使其使用本轮已完成布局的范围；
不捕获Element/Context，不写状态，不新增计时器或持续请求下一帧。

原版路径读取上一帧flags，但尚未验证其原生调度是否同样滞后。本实现优先保证正常
单次绘制即得到正确稳定态，不将上述实现侧缺陷归咎于原版，也不宣称过渡逐帧一致。

## 验证范围

- 修复前后使用相同的底部 `y32` 坐标，1x/2x分别覆盖wheel→hover→标签/加号；
  原4项均失败，修复后通过，并核对焦点、active tab、菜单和展开按钮独立性。
- 36px渐隐回归覆盖light/dark、1x/2x、阈值、颜色、全高、窄容器裁剪、收窄/增宽、
  滚动和命中；单次正常绘制后的空闲收敛另有回归及修复前日志。
- 框架8类回归覆盖双轴、拖动取消、稳定Key、链式最终值、默认恢复、父子隔离、
  键盘/辅助功能、虚拟List与绘制边界；这些实现侧图像不是原版golden。

五项主线检查通过，263项源码相对R11仅8项变化。第一次fresh目录的Mac staging
重命名在Windows遇AccessDenied，失败日志及部分产物保留；第二个fresh目录的
四目标资源、架构、ZIP、checksum和框架标记校验通过。发布包未执行安装器。

确切Windows发布EXE（SHA `1e8d0dc3290b2a4f4d97095e45c59fb0c54542e8ad65736857281cbe351c2fe0`）
在DPI192/per-monitor下通过实际OS横向滚轮及窗口相对y32/y33点击，切换layout与
README diff，加号底边打开Files/Browser/Terminal/Diffs/History菜单，选择mock
History并展开/收回。实际exit0、stderr空、48份raw文件封存；UIA可能早于绘制帧，
稳定内容和原始截图分别核对。原生popup标签不由此UIA reader暴露，五项由原图核对。
该原生样点不代替最后1DIP/更多DPI的完整边界证明。阶段索引为
`reference/validation/scrollbars-r12-result.json`。

## 仍待完成

玻璃背景需要子树透明度遮罩；把不透明shell渐变盖在玻璃上会改变材质，因此本轮
只启用在实际不透明背景（包括不支持玻璃时的降级背景）。真实玻璃暂不画该渐隐。
原生还确认新History或takeover收回后活动标签可保持裁切，手动wheel才能完整揭示。
该自动滚入问题已留原图并进入R13诊断，不用既有键盘测试替代该路径的验收。
后续R13已修复其中菜单Chosen丢请求，见 [菜单滚入审计](audit-tabstrip-reveal.md)。
缩窄/收回与菜单不同，原版自动滚入要求未建立，未认定为已确认parity缺陷。
150ms拖动兄弟标签滑动、展开/滚入动画、168px自绘菜单及完整焦点/重复按压语义、
匹配原版的独立截图对照、更多DPI和Mac Intel/ARM实机验收继续待办。

R14 adds36 independent production-component native pairs and69 strict comparison
guards at actual2x DPI; see `audit-tabstrip-fixture.md`. R13 drawing and `.6`
framework stay unchanged. Full-window threshold scores do not accept fades,
colors, glass or the complete titlebar; the real glass subtree mask remains open.

## R15 follow-up

R15 changes application tab drawing through the existing shader API; the `.6`
framework tree and eleven patches are unchanged. Four package targets and a
separate exact Windows EXE tab interaction smoke pass.36 original/Go baseline
pairs plus four supplemental states do not establish full visual acceptance;
see `audit-tabstrip-style.md`. Earlier evidence above retains its phase identity.
