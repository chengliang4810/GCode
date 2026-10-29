# 菜单选择后的标签滚入 — R13

R12原生检查发现：从滚动末端加号菜单新增History后，活动标签只露出右侧一段。
本阶段修复的是副本已存在的selection reveal请求丢失；尚未据此认定原版同一路径的
原生行为，也未宣称完整视觉一致。阶段索引为 `reference/validation/reveal-r13-result.json`。

## 原因和修复

生产菜单的 `Chosen()` 在TabStrip构建时同步处理。此时旧chips已构建，新History
才被加入并设置 `revealTab=true`；旧函数末尾又无条件清除该标志。MyGo消费菜单事件后
重建出新chip，但请求已经丢失。实际诊断中MaxX已从56增至172，X仍56，History只露
24DIP；直接在构建前调用同一个OpenSurface则完整显示112DIP，因此不是旧MaxX上限。

现在仅在TabStrip入口取出并清除已有请求，当前chips消费这个局部值。菜单在本轮
后段新产生的请求留给下一次构建。没有计时器、额外host invalidation、持续滚入、
worker或持久Element/Context；框架仍为`.6`，未新增API或补丁。

## 验证

320/360DIP×1x/2x×新增/已存在标签的8项真实生产菜单回归，修前全部失败、修后
全部完整显示112×24。测试确认菜单在构建中改变selection/count，经过多次构建，
并非绕过菜单直接调用。另6项direct/End对照通过；手动wheel移开后不会被持续拉回。
本包53项顶层测试、全仓五项门槛和默认 `git diff --check` 均通过。

263项构建/应用源码相对R12只有3项改变：两个标签文件及生成的license metadata。
后者只把306处CRLF改为LF；解析JSON等义，50份其他许可文件逐字节未变。
R12原始263项源码和10份文档已按封存hash备份，旧包/原生证据未重标身份。

四目标检查包的架构、资源、ZIP、checksum和框架标记通过。确切Windows EXE SHA为
`36ede6c6f3b4c26cf2b57ee3ad8d4334bdbdaebfe16719edce3180bcad84d67f`。
在DPI192/per-monitor的实际OS路径中，打开Changes及两个diff、横向滚到旧加号末端、
点击加号底边并选择History，新活动chip立即完整显示，空闲后仍正确，无需补救输入。
之后主动向左wheel，空闲后保留用户位置而正文仍为History。窗口实际Alt+F4退出0、
stderr空，34份raw文件独立封存。精确112DIP由Tester验证；原生样点和JPEG不是原版golden。

## 与缩窄、加号可见性的区别

宽窗口收回到窄栏后，活动标签也可能保持在可见区外，但它没有新增selection请求，
与上述菜单丢标志不同。冻结原版的right_tab_scroll、expand和稳定宽度路径没有建立
“resize必须自动滚入”的要求。该现象保留为交互预期待核对，R13没有擅自改写它。
滚入active chip也不要求plus始终可见；用户主动wheel移开的选择应保留。

R14已建立独立原版TabStrip fixture与36对实际2x DPI/主题/数据/scroll基线，见
`audit-tabstrip-fixture.md`；高整窗分数未作为接受。真实玻璃遮罩、过渡动画、自绘
菜单、完整视觉和Windows/Mac原生验收继续待办，整体目标保持active。

## R15 follow-up

R15 changes application tab drawing through the existing shader API; the `.6`
framework tree and eleven patches are unchanged. Four package targets and a
separate exact Windows EXE tab interaction smoke pass.36 original/Go baseline
pairs plus four supplemental states do not establish full visual acceptance;
see `audit-tabstrip-style.md`. Earlier evidence above retains its phase identity.
