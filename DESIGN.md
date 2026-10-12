---
name: GCode
description: 面向本地代码仓库的原生中文 Agent 工作台视觉系统
colors:
  background-dark: "#090909"
  sidebar-dark: "#080808"
  surface-dark: "#0d0d0d"
  surface-alt-dark: "#181818"
  surface-raised-dark: "#191919"
  selection-dark: "#242424"
  border-dark: "rgba(255,255,255,0.085)"
  text-dark: "#e7e7e7"
  muted-dark: "#a0a0a0"
  accent-dark: "#5b9cff"
  accent-soft-dark: "rgba(91,156,255,0.15)"
  on-accent: "#ffffff"
  success-dark: "#3dbb8a"
  warning-dark: "#e7b86c"
  danger-dark: "#ed6b76"
  code-dark: "#111111"
  code-text-dark: "#d9d9d5"
  diff-add-dark: "rgba(52,211,153,0.13)"
  diff-delete-dark: "rgba(248,113,113,0.13)"
  background-light: "#f6f6f4"
  sidebar-light: "#ededeb"
  surface-light: "#fbfbf9"
  surface-alt-light: "#f0f0ee"
  surface-raised-light: "#e6e6e3"
  selection-light: "#dcdcd8"
  border-light: "rgba(25,25,22,0.13)"
  text-light: "#1c1c1a"
  muted-light: "#666660"
  accent-light: "#4b67c8"
  accent-soft-light: "rgba(75,103,200,0.12)"
  success-light: "#217d58"
  warning-light: "#94620e"
  danger-light: "#b33c43"
  code-light: "#f0f0ed"
  code-text-light: "#2b2b28"
  diff-add-light: "rgba(22,135,93,0.11)"
  diff-delete-light: "rgba(189,62,70,0.11)"
typography:
  body:
    fontFamily: "system-ui"
    fontSize: "15.5px"
    fontWeight: 400
    lineHeight: 1.55
  title:
    fontFamily: "system-ui"
    fontSize: "14.5px"
    fontWeight: 600
  section:
    fontFamily: "system-ui"
    fontSize: "16px"
    fontWeight: 650
  label:
    fontFamily: "system-ui"
    fontSize: "11px"
    fontWeight: 600
    letterSpacing: "0.6px"
  code:
    fontFamily: "SFMono-Regular, Menlo, Consolas, monospace"
    fontSize: "13px"
    lineHeight: 1.4
rounded:
  dot: "4px"
  small: "6px"
  code: "7px"
  control: "8px"
  row: "9px"
  panel: "10px"
  pill: "13px"
  message: "16px"
  composer: "18px"
spacing:
  unit: "4px"
  xs: "8px"
  sm: "12px"
  md: "16px"
  lg: "20px"
  xl: "24px"
  2xl: "28px"
  footer-safe: "40px"
components:
  primary-button:
    backgroundColor: "{colors.accent-dark}"
    textColor: "{colors.on-accent}"
    typography: "{typography.label}"
    rounded: "{rounded.control}"
    padding: "0 12px"
    height: "34px"
  icon-button:
    textColor: "{colors.text-dark}"
    rounded: "{rounded.control}"
    size: "32px"
  neutral-stop:
    backgroundColor: "{colors.surface-raised-dark}"
    textColor: "{colors.text-dark}"
    rounded: "{rounded.pill}"
    size: "32px"
  composer:
    backgroundColor: "{colors.surface-alt-dark}"
    rounded: "{rounded.composer}"
    padding: "10px 12px"
  user-message:
    backgroundColor: "{colors.surface-alt-dark}"
    rounded: "{rounded.message}"
    padding: "10px 14px"
  session-selected:
    backgroundColor: "{colors.selection-dark}"
    rounded: "{rounded.panel}"
    padding: "6px 8px"
    height: "48px"
  tool-trigger:
    rounded: "{rounded.control}"
    padding: "0 8px"
    height: "28px"
  select-control:
    backgroundColor: "transparent"
    textColor: "{colors.muted-dark}"
    rounded: "{rounded.control}"
    height: "30px"
    width: "170px"
  model-popover:
    backgroundColor: "{colors.surface-raised-dark}"
    rounded: "{rounded.panel}"
    minWidth: "220px"
    maxHeight: "260px"
    padding: "4px"
  composer-auxiliary:
    height: "28px"
    gap: "12px"
  input-field:
    backgroundColor: "{colors.surface-alt-dark}"
    textColor: "{colors.text-dark}"
    rounded: "{rounded.control}"
    height: "34px"
  code-block:
    backgroundColor: "{colors.code-dark}"
    textColor: "{colors.code-text-dark}"
    typography: "{typography.code}"
    rounded: "{rounded.code}"
    padding: "9px 12px"
  diff-surface:
    backgroundColor: "{colors.code-dark}"
    rounded: "{rounded.control}"
---

# Design System: GCode

## Overview

**Creative North Star: "安静的原生工作台"**

GCode 的视觉候选围绕持续编码现场组织：内容面保持安静，正在运行的任务、需要授权的动作和文件变更拥有清楚但有限的强调。Go + mygo 原生控件承担输入、选择、发送、停止、设置和面板切换，界面不依赖 HTML、JavaScript 或 WebView。

三栏工作台使用近黑或低反差浅色表面，助手正文直接落在阅读面，用户消息使用低调圆角容器，输入区集中在底部单一表面。蓝色只承担操作和焦点，运行状态使用语义色，当前会话使用中性选中面。最新候选见 [visual-v4 原生暗色截图](output/acceptance/visual-v4/confirmed/dark-1440x900-native.png)，已复核深浅主题、窄窗及项目树展开、收起和对话切换。当前候选等待用户视觉评价，不表示 T3 像素一致或用户已确认。

**Key Characteristics:**

- 中文、原生、可操作的编码工作台。
- 中性表面分层，有限的蓝色操作强调。
- 直接助手正文、轻量用户消息和紧凑工具过程。
- 可拖动三栏、窄窗收起资源面板、固定设置 footer。

## Colors

颜色由 `theme.go` 的深浅主题 palette 直接提供。深色主题以近黑表面和浅色文字建立阅读层级，浅色主题保持相同语义映射和较低层级反差；透明边界只承担分割和必要的控件轮廓。

### Primary

- **蓝色操作色**：`accent-dark` 与 `accent-light` 用于链接、焦点、进行中状态、主要按钮和需要注意的提问面。

### Semantic

- **成功色**：`success-dark` 与 `success-light` 用于完成状态和新增 Diff。
- **警示色**：`warning-dark` 与 `warning-light` 用于等待、授权和示例数据提示。
- **危险色**：`danger-dark` 与 `danger-light` 用于失败、错误、停止反馈和删除内容。

### Neutral

- **工作台表面**：`background-*`、`sidebar-*`、`surface-*`、`surface-alt-*` 和 `surface-raised-*` 分别承担根背景、导航、阅读面、输入/悬停面和抬升控件面。
- **中性选中**：`selection-*` 只表示当前项目、会话或其他选中对象，不用蓝色铺满列表。
- **文字层级**：`text-*` 用于主要内容，`muted-*` 用于路径、时间、行号和辅助说明。
- **边界**：`border-*` 通过低不透明度的 1 DIP 线分隔栏、表头、表单和必要的原生控件。

### Code and Diff

- **代码面**：`code-*` 与 `code-text-*` 组成代码块、预览、终端和 Diff 的深色/浅色代码表面。
- **变更面**：`diff-add-*` 和 `diff-delete-*` 只覆盖新增/删除行，并与符号、行号和语法文字同时出现。

**The Neutral Selection Rule.** 当前对象使用中性选中面；蓝色保留给动作、焦点、链接和进行中状态。

## Typography

**Display Font:** 使用 `system-ui` 原生界面字体。

**Body Font:** 使用 `system-ui`，由 mygo 在目标平台映射到系统 UI 字体。

**Label/Mono Font:** 标签沿用系统 UI 字体；代码、路径、工具参数、Diff 行和终端使用 `SFMono-Regular, Menlo, Consolas, monospace`。

**Character:** 中文界面保持直接、紧凑和可扫描。正文与标题使用系统字体，等宽字体只出现在需要对齐或表达代码语义的内容中。

### Hierarchy

- **Body**（400，15.5 DIP，1.55 行高）：助手正文、主要说明和可选择的 Markdown 文本。
- **Title**（600，14.5 DIP）：会话表头标题和紧凑的工作台标题。
- **Section**（650，16 DIP）：设置页分组标题。
- **Label**（600，11 DIP，0.6 DIP 字距）：侧栏分组标签、状态标签和辅助导航。
- **Code**（400，13 DIP，1.4 行高）：代码块和 Diff 主体；工具详情和文件路径按内容密度使用更小的等宽尺寸。

**The Readable Mono Rule.** 等宽字体服务于代码和路径；普通中文说明不切换到等宽字体。

## Layout

工作台由两个原生 `ui.Split` 组成：左侧项目树，中间会话正文与 composer，右侧文件、Diff 或终端。1440 DIP 窗口的初始左栏为 256 DIP、中栏为 650 DIP、右栏约 532 DIP；分割线可拖动，右侧资源面板保持至少 290 DIP。消息列表和 composer 共用 748 DIP 最大阅读列，左右各留 24 DIP；助手正文最大宽度 700 DIP，用户消息最大宽度 580 DIP。表头使用项目名 / 对话标题的单行导航，目录路径通过 tooltip 展示。

窗口宽度小于 1180 DIP 时，资源面板默认收起，表头仍保留展开按钮。窄窗左栏宽度限制在 176 DIP 与窗口宽度的 19% 之间，并且不超过 220 DIP；显式展开资源面板时，中栏会根据右栏最小宽度重新约束。窗口表头至少 52 DIP，并在原生标题栏更高时跟随标题栏高度。

会话正文使用 16 DIP 上下、28 DIP 左右的内容内边距并保持独立滚动；composer 位于底部，外层留出 8/20/12/20 DIP 的边距，主输入面与下方辅助行相隔 5 DIP。设置页内容最大宽度为 980 DIP，滚动内容底部保留 40 DIP 安全间距，保存操作在独立的 54 DIP footer 中保持可见。间距优先沿用 4 DIP 基准及 8、12、16、20、24、28 DIP 的重复节奏。

## Elevation & Depth

静止工作台不使用显式阴影或渐变层；模型弹层使用 0/6/20 DIP、20% 黑色的轻量阴影。深度由背景、导航、阅读面、输入面和抬升面之间的细微色阶，加上低不透明度边界、悬停面和状态面建立。原生窗口标题栏和分割线承担结构定位；交互反馈通过表面变化和语义色表达。

### Named Rules

**The Flat by Default Rule.** 静止内容保持平面；只在悬停、选中、错误、授权和可操作状态使用表面变化或边界。

## Shapes

整体圆角基准为 8 DIP。紧凑图标按钮、分组触发器、选择框和输入框使用控制圆角；项目行、授权面和设置编辑面使用稍大的行或 panel 圆角；用户消息和 composer 分别使用更柔和的消息与 composer 圆角。状态徽章和停止按钮使用胶囊/圆形轮廓。

边界是 1 DIP 的低反差线，只放在表头、分割、表单字段、设置 footer 和需要明确轮廓的原生控件上。助手正文不再包裹外层卡片，Markdown 引用、工具详情、用户消息、设置编辑面和代码块才使用明确的局部容器。

## Components

### Buttons

- **Primary:** 主要提交动作使用 accent 背景、白色文字、34 DIP 高度、控制圆角和 0/12 DIP 内边距。
- **Icon:** 工具栏和表头动作使用 32 DIP 方形点击区域、控制圆角、线性图标和 Tooltip；悬停时切换到次级表面。
- **Compact:** 文字动作使用 32 DIP 高度、0/10 DIP 内边距和控制圆角；设置页主保存动作使用 primary 规格。
- **Circle:** 发送使用 accent 圆钮；运行中停止使用 neutral 圆钮和抬升表面，二者都保持 32 DIP 点击区域。

### Inputs / Fields

- **Composer:** TextArea 高度为 78 DIP，透明输入层放在单一底部表面中。主操作行只包含模型选择、独立停止和向上箭头发送；空白草稿禁用发送。下方 28 DIP 辅助行包含纠正/队列、完全访问和实际分支，保持操作层级与默认偏好语义。
- **Select:** composer 模型选择器使用 `ui.SelectBase`，触发器为 170×30 DIP，静止时无边框，悬停使用抬升表面。低反差弹层最小 220 DIP 宽、最大 260 DIP 高；选项以唯一 provider ID 绑定，显示可读名称，同名服务仍可独立选择。设置页 API 类型选择器为 34 DIP 高度，保留次级表面与必要边界。
- **Settings fields:** 设置表单文字输入和密码输入为 34 DIP 高度，标签使用辅助层级；API Key 的保存状态用成功/警示色文字表达。

### Navigation

侧栏使用导航表面和 48 DIP 原生表头，按“项目栏目 → 项目名称 → 多个对话”组织。项目行高 32 DIP，使用原生 disclosure 独立展开/收起，行尾加号在该项目内新建对话。子对话行高 48 DIP，缩进 16 DIP 并沿细引导线排列；只显示标题、分支、更新时间、状态点和队列数量，不重复项目名或目录。项目间距 12 DIP、同项目对话间距 2 DIP，选中对话使用中性选中面；快照刷新保留展开状态，新建或选择对话自动展开所属项目。窄窗保留侧栏并约束其宽度；低于 1100 DIP 时隐藏文字品牌，仅保留带 `GCode` 可访问标签的 G 品牌图标。资源面板改为表头按钮控制。

### Conversation

助手消息直接排版在正文面，用户消息右对齐并使用轻量圆角容器。工具触发器高度为 28 DIP，默认折叠，错误工具强制展开；触发器显示人类可读的命令或路径摘要，展开面板才显示原始参数、输出和错误。授权和提问使用带边界的状态面，队列和错误分别固定在 composer 上方。

授权卡片显示完整解码的命令及显式工作目录，命令区最多 180 DIP 并可独立滚动，保留批准/拒绝按钮。主动停止或纠正导致的取消显示中性的“已中断”；原始取消诊断仍可在工具详情展开查看，普通失败继续使用错误状态并默认展开。取消不是新的助手正文。

### Code and Diff

Markdown 代码块使用代码面、代码文字、7 DIP 圆角、复制动作和水平滚动；选区颜色跟随当前主题的 selection。资源栏主表头保留文件/Diff/终端 Tabs 和关闭入口；Diff 在同一行展示实际文件数和累计增删，不再重复资源/项目 Diff/分支标题。非 Git 不伪造统计，干净 Git 显示 0 个文件。增删计数在 raw 变化时解析并缓存，主表头与文件行复用。Diff 按文件和 hunk 分组，文件触发器高度为 32 DIP，代码区支持水平滚动，旧行号、新行号和变更标记分别占用稳定列宽，代码文字保持 13 DIP。`<=200` 行的文件使用自然内容高度逐行渲染，外层统一纵向滚动；`>200` 行的文件保留固定高度的 `ui.List` 虚拟化视口。动态 `rowWidth` 同时约束 List 和 Row；普通 Diff 行使用 `RichText.MinWidth(0)` 自然换行并按实际高度测量，hunk/meta 行保持单行并通过 Tooltip 提供完整内容。200 行以内的文件首次默认展开，用户明确折叠后的状态按路径持久保留。

### Settings

设置页使用最大 980 DIP 的阅读列，外观、发送方式和模型服务按纵向分组排列。模型服务内部采用左侧服务列表与右侧编辑面；底部 footer 固定显示保存设置、保存服务和删除动作，避免操作区被滚动内容截断。

## Do's and Don'ts

### Do:

- **Do** 保持 Go + mygo 原生渲染和真实事件链，所有输入、选择、发送、停止、复制、主题和面板动作都使用真实控件。
- **Do** 使用两套实际 palette 的语义映射；新增视觉 token 必须先在当前原生源码中有稳定使用点。
- **Do** 保持助手正文无外层大卡片、用户消息轻量容器、工具紧凑折叠和 composer 单一底部表面。
- **Do** 在窄窗复核资源面板收起、侧栏 clamp、正文可读宽度和原生标题栏间距。
- **Do** 让新增/删除、成功/失败、等待/授权状态同时有颜色、文字或符号依据，避免只依赖色彩。

### Don't:

- **Don't** 引入 HTML、JavaScript、WebView、CSS 产品界面或静态截图来替代原生控件。
- **Don't** 把用户评价完成前的候选记录写成 T3 像素一致、视觉交付通过或用户已认可。
- **Don't** 用大面积蓝色、重复外层卡片、全局大标题栏或助手外层卡片削弱内容阅读宽度。
- **Don't** 把原始工具 JSON 放在默认触发器中；默认显示短命令/路径，原始参数留在展开内容和复制动作中。
- **Don't** 删除纠正模式下停止与发送的双按钮语义；停止当前运行和提交下一条指令是两个独立动作。
