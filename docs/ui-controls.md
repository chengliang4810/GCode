# 统一控件

共享页面框负责外层布局，页面负责框内内容，共享组件负责外观，
业务核心与调用方负责状态和保存。
外部参考图不覆盖现有组件规范。不同页面的同类控件必须来自同一实现。

## 修改入口

| 内容 | 唯一维护位置 |
| --- | --- |
| 设置页最大宽度、居中、外层内边距、页面间距和标题规格 | `internal/design/settings.go` 的 `SettingsPage()` |
| 单行高度、图标按钮尺寸、圆角、字号、内部间距、开关/复选框/滑块尺寸 | `internal/design/controls.go` 的 `Controls()` |
| 主/次/普通/危险按钮及输入、选择、开关等状态和绘制 | `internal/desktop/settings_controls.go` |
| 主题语义颜色及 UI 字体映射 | `internal/design/themes.json` 与 `internal/design/design.go` |
| 玻璃壳、浮层毛玻璃与侧边栏 tone 的配方（tint、模糊半径、发丝线） | `internal/design/material.go`；绘制入口 `internal/desktop/frost.go` |
| 界面字号层级及等量像素偏移 | `internal/design/text.go` |
| 聊天侧栏导航、项目/对话行与分区尺寸 | `internal/design/controls.go` 的 `Sidebar()` |

所有设置页使用 `settings_page.go` 的共享视口、内容框和标题构造函数。
外层统一使用 1200 最大宽度、54 顶部内边距、40 水平内边距和 48 底部内边距；
标题在窄窗使用 24 字号，窗口宽度达到 1024 时使用 30 字号；
均按界面基准字号作等量像素增减，规则见 `typography.md`。
数值为逻辑像素，内容框在剩余主区中居中，窄窗自动收缩。
模型页的双栏属于框内布局，不拥有独立的页面宽度或大标题。
需要改变整体密度时只修改统一定义，不增加针对某一设置页的外框分支。

例如修改 `ButtonRadius` 会更新全部共享操作按钮；修改 `InputHeight` 会同时更新
设置文本框和选择器；修改 `SwitchWidth` / `SwitchHeight` 会同时更新普通设置、
集成设置、供应商与资源列表开关。开关绘制按统一尺寸缩放。
标题栏的图标槽位也读取 `IconButtonSize`，不另写一份 28 px 宽度。
窗口隐藏系统按钮时的自绘规则见文末「标题栏窗口按钮」。
排序手柄复用共享图标按钮；拖拽落点指示线使用主题强调色，线宽统一由
`ReorderIndicatorWidth` 定义。页面只处理带类型的拖放结果和保存命令。
侧栏、右侧工作区与 Files 停靠栏的开合使用 `internal/desktop/panel_tween.go`
的宽度补间（180–240ms，CSS `ease`，减少动效时立即落位），拖拽仍直接跟随
指针，边缘回弹由 `internal/desktop/panel_resize.go` 统一定义；页面不另写
开合动画。
资源列表的图标容器和信息标签使用 `controlResourceAvatar` / `controlResourceBadge`；
`ResourceIconSize`、`BadgePaddingX`、`BadgePaddingY` 仍在统一尺寸定义中维护。
图标按钮的点击区域保持逻辑像素尺寸，不因文字缩放挤出标题栏；
侧栏的更新圆形按钮和用量图标同样由 `settings_controls.go` 提供，
圆角读取 `Controls().RoundIconButtonRadius`，页面只配置 key、标签与回调。
文字按 UI 字号 token 作像素偏移，图标、间距、圆角和开关几何不随文字缩放。
按钮、输入框、选择器按当前文字行高保留最小高度，避免大字号裁切。
刻度滑块保留原生刻度所需的最小高度，小字号也不压缩刻度区域。

主按钮使用主题的 `Accent` / `AccentText`，悬停和按下使用对应语义色，
不使用黑白反色，也不硬编码紫色；默认主题为紫色，更换强调色后同步变化。

按钮宽度按内容或所在列决定，不要求不同文案拥有相同宽度。
多行编辑区的高度由内容布局决定；其字体、边框和内部间距仍由共享定义维护。

## 页面调用

```go
primaryButton(c.Key("save"), tr(c, "Save")).Disabled(saving).OnClick(save)
controlTextInput(c.Key("name"), &draft.Name).FillWidth().Label(tr(c, "Name"))
a.controlSwitch(c.Key("enabled"), &enabled, label, saving, func(value bool) {
    // 这里提交本业务的类型化命令。
})
```

选择器使用 `controlSelect`、`controlSelectPopup` 和 `controlSelectItem`；
原生菜单型选择器使用 `controlMenuButton`。三色预设预览使用
`controlEffortSwatch`，宽高由 `Controls().GradientSwatchWidth` /
`GradientSwatchHeight` 统一维护，下拉触发器和每一行共用同一实现。
资源列表使用 `controlRowButton`，
导航、预览、色板和选择卡片也有对应的共享变体，不能用普通操作按钮强行拼装。
密钥使用 `controlSecretInput`，保留输入焦点的显示按钮不会抢先触发失焦保存。
调用方根据草稿或已保存密钥决定显示按钮是否可用，并在点击后通过类型化接口
按需读取已保存密钥；隐藏、保存或离开编辑器时清除明文展示缓存。

允许设置 label、key、Disabled、OnClick、OnChange、Bind、Grow、Width、
FillWidth、Margin 等内容/行为/布局属性。禁止在页面覆写控件外观；
需要新外观时，在共享组件中增加有语义的变体，更新同类调用方及验收。
共享控件只产生 UI 事件，不自动调用 `settings.update`、`provider.save` 等业务命令。
共享页面框的宽度、对齐、外边距和标题外观也受门禁保护，不能使用上述布局接口
给某一页覆盖外框。内容卡片、双栏、编辑区在共享外框内仍可按业务布局。

## 门禁与验证

`control_gate_test.go` 自动扫描全部非测试 `settings*.go`，仅共享定义文件豁免。
它拒绝直接创建 MyGo 原始表单控件以及对共享控件的局部外观覆盖，
识别调用链、局部变量/函数别名、选择器 Trigger / Popup / Item。
普通容器、文本及对话框的布局不受影响，不保存历史违规基线。
门禁在普通 `go test ./...` 中运行；`make controls` 可单独运行。

门禁约束设置页。其他桌面区域已有的专用编辑器、会话导航、工具栏等不在本次
逐页迁移范围；它们调用的共享文字/图标按钮会采用统一定义。
这不代表整个桌面的每一种定制交互都已迁移或已经过视觉验收。

会话表格的浮动横向滚动条使用共享 `controlHorizontalScrollbar`，尺寸在 `Controls()` 的
`ScrollbarHeight`、`ScrollbarMinThumb` 统一定义，保留原生滑块键盘、拖动和可访问范围。

聊天侧栏使用共享 `controlSidebarRow`、`controlSidebarProjectRow`、
`controlSidebarConversationRow`、`controlSidebarHeading`、`controlSidebarMoreRow`
和 `controlSidebarEmptyRow`。项目子行的缩进也由 `Sidebar()` 统一维护。
列表与分区的业务回调由侧栏提供；布局依据与保留的 GCode 语义见 `sidebar-layout.md`。

模型列表只显示一行请求模型 ID，不再提供独立显示名称。
模型输入类型和推理等级共用 `controlCheckboxOption` 边框勾选项；文本输入固定勾选。
保留拖拽排序手柄，不显示模型启停开关或上下移动箭头。
测试、编辑和删除使用共享图标按钮，保留悬停提示与可访问名称。

验证同时涵盖状态交互、普通设置与供应商/资源开关几何一致性、深浅主题、
窄窗口与现有标题栏的大字体边界。渲染证据属于实现检查，不是参考图像素验收。

2026-10-29：全仓测试通过；最后的颜色与尺寸调整后，desktop/design 回归、
Go vet、MyGo vet 和桌面构建再次通过。控件样例、设置页和模型页实现渲染位于
`output/shared-controls/`；默认主题和蓝色强调色下，两处添加按钮均有真实渲染颜色断言。
更新的原生桌面已使用现有用户目录打开模型设置，窗口正常响应、启动错误日志为空，
实窗截图为 `output/shared-controls/native-model-settings.png`。

后续外层边距修正移除了模型页单独的 1720 最大宽度、32 顶部内边距及 30 字号标题。
同窗口下所有设置 section 的内容框与标题几何会与 General 比较，覆盖深浅主题、
900/1320/2048 宽度以及隐藏侧栏；门禁同时阻止页面框样式覆盖。
当前页面框实现证据位于 `output/settings-page-frame/`，其中 General 与 Model settings
使用相同的窗口和布局条件。旧的模型页宽屏截图只作为历史记录。
此轮全仓测试、Go vet、MyGo vet 和桌面构建通过。原生窗口已更新并检查，
启动错误日志为空；当前实窗截图为 `output/settings-page-frame/native-model-settings.png`。

## 标题栏窗口按钮

Windows 窗口通过 `mygo.WindowOptions.HideWindowButtons` 隐藏系统标题栏按钮，
由标题栏在 `internal/desktop/titlebar.go` 的 `captionControls` 自绘最小化、
最大化/还原和关闭。图形是 `internal/assets/glyphs/` 中按 24 单位画布、1.75 线宽
绘制的 GCode 自有资源，不依赖系统私用区字符或可选图标字体，也不进入
`internal/assets/manifest.json` 记录的冻结上游清单。

三个按钮与左侧相邻图标共用同一实现：`IconButtonSize` 28×28、`IconSize` 16、
圆角 `ButtonRadius`、4 px 间距、4 px 顶部内边距和同一条中心线；关闭按钮的
红底白图标悬停/按下属于共享变体 `buttonCaptionClose`，页面不得另行绘制。
最大化按钮带 `MaximizeButton()` 标记，保留 Windows 11 的 Snap Layouts。
最大化状态变化通过 `OnMaximize`/`OnUnmaximize` 触发重绘，图形随之切换。
右侧让位宽度由 `titlebarCaptionWidth()` 与 `titlebarEdgeInset` 推导，
不硬编码 108；全屏时平台本身不显示控件，标题栏同时收回这段宽度。

验证：`go test ./internal/desktop -run 'TestTitlebar'` 覆盖尺寸、中心线、节奏、
边缘内缩、悬停配色、深浅主题与全屏/最大化状态；原生实窗检查见
`TestDesktopWindowCaptionControls`，证据位于
`output/native-tests/20261101-023437-2195741834/`。
