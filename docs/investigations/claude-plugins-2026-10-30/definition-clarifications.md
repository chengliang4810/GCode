# 插件定义核实补充（2026-10-30）

本页澄清初始静态盘点和实际安装发现中的四类失败。它不以放宽检查提高通过率，
也不把静态发现成功记为运行成功。以下网页是当日公开文档快照语义；本地参考源码
仅为交叉证据，不代替官方 Claude 当前二进制的执行验证。

| 情况 | 核实结论 | 调查分类 |
|---|---|---|
| 市场 entry.name 与 plugin.json.name 不同 | 明确允许不同，分别拥有安装身份和运行命名空间 | 两者不同本身不是无效插件 |
| 可选 argument-hint 为 YAML sequence | 文档未明确授权 list；它是辅助展示字段，本地参考会容忍。不能据此证明整个插件不可运行 | 辅助字段的类型/容忍差异；确切展示语义待官方运行验证 |
| Hook matcher 为 `*` | 官方明确与空值、未声明一样匹配全部 | 拿 `*` 硬编译正则并拒绝是实现错误 |
| 源码包含 symlink | 根据目标是否位于插件、同市场或市场外分别处理 | 必须逐条解析 target，不能把“存在链接”直接当插件无效 |

## 两种名称

官方 loading reference 明确 marketplace 插件有两种名称且可不同：
entry name 用于安装、启用、cache 路径；manifest name 用于组件 namespace 和名字冲突。
manifest reference 的 metadata precedence 对此再次明确。市场创建教程建议一致，是为了
避免用户按 manifest 名安装时找不到市场 entry；该建议不等于必须覆盖 manifest 名或拒绝安装。
大小写差异也不能仅凭相异就拒绝；仍需分别验证名称本身是否有效。

- https://code.claude.com/docs/en/plugins/loading#entry-name-and-manifest-name
- https://code.claude.com/docs/en/plugins-reference#metadata-precedence
- https://code.claude.com/docs/en/plugin-marketplaces#keep-the-entry-name-and-the-manifest-name-the-same

## argument-hint

官方 skills frontmatter 表把 `argument-hint` 描述为自动完成提示，给出字符串示例；
没有明确写它接受数组。`arguments`、`allowed-tools` 等字段则明确写出了 YAML list 支持，
因此不能把所有字段都当成正式支持 list。另一方面，文档说明即使 YAML 无法解析，技能 body
仍加载，只是不带 metadata。把一个可选 UI hint 的 sequence→string 解码错误升级为整个插件
失败，缺乏公开语义依据。可以容忍/忽略这项辅助 metadata 并报告问题。

本地只读参考 `D:/projects/claude-code/src/skills/loadSkillsDir.ts` 246–247 行使用 String 转换
argument-hint；JavaScript 数组会转成逗号连接文本。插件命令 loader 的
`src/utils/plugins/loadPluginCommands.ts` 263 行仅有编译期 string 断言。这些是本地参考实现
证据，不是已执行官方二进制的证据。

- https://code.claude.com/docs/en/skills#frontmatter-reference
- https://code.claude.com/docs/en/skills#skill-not-triggering

## Hook matcher

官方表把 `*`、空字符串、未声明分成 match-all；只含字母、数字、下划线、连字符、空格、
逗号和竖线的值走 exact/list；其余才走 JavaScript 正则，且默认不是全字符串锚定。
`FileChanged` 与 `StopFailure` 的 exact 字符集更窄。实现必须先区分这些语义，再编译正则。

- https://code.claude.com/docs/en/hooks#matcher-patterns

## Symlink 的三种处理

官方 hosting 文档明确缓存复制策略：

- 目标在插件自身目录内：在 cache 中保留为相对 symlink。
- 目标在同一市场的其他位置：解除链接，将目标内容复制到 cache 的该位置。
- 目标在市场外：出于安全跳过。

本地 path 插件和 command copy 来源，只保留插件自身内的链接，跳过其他链接。
loading reference 对声明的组件路径另行要求 containment：真正逃逸的组件路径拒绝加载该组件，
不应据此断言插件所有其他组件都会失败。某个仓库中的 symlink 是否可用，需要固定来源根、
插件根及每个最终 target 后判断；循环、断链、权限及 Windows 创建链接能力应单列证据。
本补充不授权不受约束的 follow-links，也未把未测试的平台行为当成成功。

- https://code.claude.com/docs/en/plugins/host-marketplace#share-files-within-a-marketplace-with-symlinks
- https://code.claude.com/docs/en/plugins/loading#paths-that-escape-the-plugin-directory

## 初始 skills 候选计数的补充

同日 manifest reference 还明确一个例外：市场 entry 的 source 是市场根、且列出特定 skills
子目录时，只加载这些子目录，不扫描默认 skills/。manifest 自己的 skills 才追加默认目录。
初始 `marketplace-audit.json` 对 `anthropics/skills` 的五条根来源 entry 采用了较宽的默认候选
扫描，其逐条 skills 数量应按这一规则复核；不据该候选数量声称组件实际已加载。
当前真实安装/组件发现证据优先于最初候选扫描。

- https://code.claude.com/docs/en/plugins-reference#how-entry-fields-combine-with-pluginjson
