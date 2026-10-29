# 标签底色、通用图标与脏标记 — R15

本文保留R15历史判断与证据。R16已修文件`.78`和无详情标签提示，并以冻结源码
纠正“提示差异仅为停留未匹配”的不充分解释；GPU控制复现浅色混合差异，实际UI
适配器仍待绑定。最新记录见 `audit-tabstrip-file-tooltip.md`，不改写旧原图/报告。

本阶段修复三个冻结源码已确认的差异：标签底色的浮点 wash、未选中通用图标的
`.7` 透明度，以及文件脏标记由5改为6 DIP。框架仍为 `.6`，没有新增框架补丁。
阶段索引为 `reference/validation/tabstrip-r15-result.json`；完整视觉仍未接受。

`tabs_style.go` 使用现有 `ui/shader` 单次圆角填充，保留原版中性端点 `.92/.10`
和选中/悬停 alpha `.10/.06`，在前景之前绘制。缓存 shader 与四组只读 uniform；
未修改全局 `sourceInk`，没有额外命中区或背景拍平。玻璃与透明背景只在 CPU
绘制回归中验证，不能替代原生玻璃合成和遮罩验收。SVG 图标的 `.7` 仍受框架
字节 alpha 量化限制；文件图片、品牌、favicon、spinner 不套用通用图标规则。

287项阶段源码已保存实际字节，相对R14为9项改变、7项新增，无删除。候选构建
另绑定947项应用/嵌入输入，发布打包还核验52项资源；构建前后无漂移。
全量测试、uifixture测试、Go vet、MyGo vet、lint均通过。Windows与macOS
amd64/arm64/universal包内容校验通过，Mac交叉包不等于Mac实机通过。

## 保留原图的实际对照

R15 Go候选 `0330a562…76427` 在实际192 DPI下捕获36张完整WGC PNG，与保留的
R14原版02原图比较；参考输入、几何和比较器字节均未变。比较器沿用已封存的
69项防护结果，本阶段没有把旧防护说成新跑。独立重算36个整窗SSIM及144组
R14/R15区域统计一致；SSIM仍仅用于筛查，空白宿主会主导整窗分数。

dark/400/6/start 的活动底色两侧同为 `#232323`；宿主差异像素从12,934降到
2,849，平均RGB绝对通道差从0.821935降到0.120389。light同场景平均差从
0.920554降到0.185164，但活动底色仍为原版 `#dededf`、Go `#dedee0`。
24个非空场景差异像素下降，12个空场景不变；默认caption仍有20,772个像素、
最多1级通道差异。文字、图标与圆角覆盖没有据此接受。

浅色残余的只读调查在 `output/tabstrip-r15-light-residual-preflight`。
源码未发现自定义shader显式中间字节材质，不能把差异直接归因于该环节。
straight/premultiplied混合和硬件ROP精度是未证实候选；下一步需同设备GPU
readback、实际数值与适配器记录，不能用截图反调RGB通道。

## 补充状态和真实发布程序

独立的style合同增加每种2主题的 inactive-hover / dirty-file，保留原36合同字节。
原版通过真实 FilesSurface、FileDocument加载、原Editor替换与Change订阅触发
dirty；仅可变构建导出中的Shell/preview末尾有受feature guard保护的桥，生产方法体
和冻结clone未改。15份依赖保护和两处精确前缀校验通过，第一次Rust编译失败保留。

两侧各4张补充原图均捕获成功。原版捕获前后核对实际Win32前台、GetCursorPos、
native消息取出和GPUI位置；其内部chip hover没有公开getter，`actualHovered=null`。
图像可观察到悬停底色和close响应，脏文件两侧均显示6 DIP圆点。Go浅色悬停原图
另有Tab02 tooltip，原版该次无；停留时间尚未统一，保持待核，不裁剪隐藏或宣称
整窗状态完全匹配。新补充还确认既有文件图标缺口：未选中原版文件图片另有`.78`
透明度，Go文件图片当前缺少此分支；后续按文件规则修，不能套通用图标的`.7`。
字体与图标边缘仍须继续核对。

Go原生01–03均拒绝，02/03各留下1张未形成成功manifest的PNG。03诊断明确显示
指针从10/10变成430/271.5，窗口仍在前台、几何仍稳定，不能归因为失焦。
04在实际指针移到已观察空白区后36场景退出0；输入工具历史说明与独立窗口消息
观察分别记录。fixture另修复“新收到的指针覆在旧稳定帧”可能提前放行的问题，
其回归测试通过；没有把03失败误当该修复的实机因果证明。

发布EXE `0819ce3f…06bbc` 的真实Windows操作另存
`output/resume-validation/tabstrip-r15-release-native`：打开Files、README与layout.go，
重新选择README，经原生菜单关闭layout.go，稳定后仅余README，再Alt+F4退出0，
stderr为空，DPI192。即时UIA树有提交滞后，已另取settled观察；首个关闭后枚举仍有
窗口，后续独立枚举为零，原件均保留。此为确切发布包的交互冒烟，不是原版整窗验收。
启动JSON的sourceManifest路径笔误保留原件，由
`output/tabstrip-r15-root/release-source-identity-correction.json`连接实际构建输入。

## 文档与验收边界

R14“候选尚未实施”和“最后发布包R13”已更新为历史阶段描述。旧R11资料的
“exact settled wash endpoints”不符合原有字节Color精度，原报告保留，当前说明
限定为近似终点；R15的浮点输入也不能直接保证GPU最终像素一致。
原版surface fallback注释与实际全局fallback的差异仍按R11已有记录遵循代码。

原生玻璃、不同DPI、resize/reveal语义、完整Windows输入法/剪贴板/网页叠放/
可访问性/性能，以及Mac Intel与ARM实机验收仍待完成。目标保持active。
