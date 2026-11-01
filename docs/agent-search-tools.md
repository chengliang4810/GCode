# Agent 文件搜索

公开只读工具为 `Glob` 和 `Grep`。`Glob` 查文件名与路径，`Grep` 查文件内容。
两者都在进程内自行遍历目录树并匹配，不调用外部搜索程序：机器上没有 ripgrep
也不影响使用，Windows、Linux 服务及远程宿主都不再有额外依赖。
正则用 Go 的 `regexp`（与 ripgrep 默认同一类 RE2 引擎，不支持 POSIX 字符类，
也没有 PCRE2）。三种模型协议收到相同定义。旧小写名称和 `find` 工具没有兼容别名；
会话内查找快捷键的 ID `find` 不属于 Agent 工具。

实现参照原版 RCode（`crates/agent/src/tools/glob.rs`、`grep.rs`）：那边同样在
Rust 里遍历目录并用 `regex` crate 匹配，明确写着“so the tool works on a machine
with no search binary installed”。

## Glob

```json
{"pattern":"internal/**/*.go"}
```

```json
{"pattern":"src/**/*.{ts,tsx}","path":".","offset":100,"head_limit":100}
```

只有 `pattern` 必填，`path` 缺省为会话工作目录。相对模式相对于搜索目录；
绝对模式会从不含通配符的目录前缀选择根，再应用同一匹配算法。

| 模式 | 行为 |
| --- | --- |
| `*.go`、`go.mod` | 无目录分隔符，匹配任意深度的文件名 |
| `src/*.go` | 只匹配 `src` 直接子文件 |
| `src/**/*.go` | `**` 匹配零个或多个完整目录段 |
| `*.{go,ts}` | 花括号分支，最多展开 64 个模式 |
| `file?.[gt]s` | 单字符通配和字符类 |

`*`、`?` 不跨目录，`**` 必须独占目录段。字符类使用 Go `path.Match` 语义，
取反用 `[^a]`；模式大小写敏感，`/` 与 Windows `\` 都是目录分隔符，
不提供反斜杠转义。非法模式明确报错；不宣称覆盖任意 minimatch 扩展。

文件由进程内遍历得到：包含隐藏文件，尊重 `.gitignore` 与 `.ignore`（含嵌套目录中的
规则），显式排除 `.git`、`node_modules` 子目录，不跟随符号链接。
匹配后按文件修改时间由新到旧排序，相同时间按路径排序。
`offset` 先跳过匹配项，再由 `head_limit` 取页；默认 100，范围 0..2000。
0 取消页大小限制，扫描和输出预算仍然生效。

## Grep

```json
{"pattern":"func\\s+Search","type":"go"}
```

```json
{
  "pattern":"request.*failed",
  "glob":"internal/**/*.{go,ts}",
  "output_mode":"content",
  "-i":true,
  "-C":2,
  "-B":0,
  "offset":10,
  "head_limit":50
}
```

只有 `pattern` 必填，最长 4096 字节。`path` 可以是目录或单个普通文件，默认工作目录。
正则用 Go 的 `regexp` 语义；没有启用 PCRE2，也不支持 POSIX 字符类。

| 参数 | 语义 |
| --- | --- |
| `glob` | 相对搜索根的路径过滤，与 Glob 自有的路径匹配器同一套语义 |
| `type` | 内置语言表（go、ts、js、rust、py、java、c、cpp、css、html、json、yaml、md、sh、toml、sql、xml）按扩展名过滤，未知类型报错 |
| `output_mode` | 默认 `files_with_matches`，另有 `content`、`count` |
| `-i` | 忽略大小写，默认 false |
| `multiline` | 匹配可跨行，且 `.` 匹配换行、`^`/`$` 匹配行边界 |
| `context`、`-C` | 正文模式前后上下文；两者同时给出时 `-C` 优先 |
| `-B`、`-A` | 分别覆盖前、后上下文，允许 0，范围 0..100 |
| `-n` | 正文模式显示行号，默认 true |
| `offset`、`head_limit` | 先跳过再截取，默认每页 250，范围 0..2000 |

`glob` 与 `type` 同时提供时显式正向 glob 覆盖类型过滤，两者不是交集。旧 `case_insensitive/before_context/after_context/line_numbers`
不再接受；`context` 本身是当前 Claude 参数，不是旧字段别名。

- `files_with_matches`：每个匹配文件一行。
- `content`：匹配行 `path:line:content`，上下文行 `path-line-content`；
  重叠行去重，关闭 `-n` 后省略行号。分页单位包含上下文行。
- `count`：每个匹配文件 `path:matched-line-count`，计数为覆盖的唯一行数，
  末尾 `Total` 为全部匹配文件的行数总计。分页不改变单文件计数或总计，
  汇总行不计入下一页 offset。

Grep 尊重 `.gitignore` 与 `.ignore`，默认不包含隐藏文件，额外排除 `.git`、
`node_modules`；显式 glob 覆盖类型过滤。CRLF 行尾按行尾处理，不跟随符号链接。

## 预算和失败

工作目录内的结果用相对路径及 `/`，目录外用绝对路径，可直接传入 `Read.file_path`。
显式符号链接搜索根被拒绝；这些规则不是工作区或 OS 权限沙箱。

- 两个工具都有 15 秒超时，取消会终止遍历。
- Glob 最多接收 100000 个目录项；到达上限报错，不承诺枚举被 ignore 排除的条目数。
- Grep 的文件上限为 2 MiB；二进制（含 NUL）与超限文件被静默跳过。
  非 UTF-8 文件若匹配会计入跳过提示，显式超限文件也给出提示；没有完整的所有跳过文件清单。
  无匹配结果不能据此推断被忽略或跳过的文件没有命中。
- 输出约限 64000 字节，正文单行超过 4000 字节在 UTF-8 边界截断并注明。
  页大小或字节预算截断时返回继续使用的 offset；恰好最后一页不误报截断。
- 文件变化可能改变跨页位置，没有固定搜索快照。

非法输入、路径错误、进程失败、扫描预算与超时均返回工具错误。
文件写入等有副作用的操作仍与只读搜索维持执行屏障。

## 性能（本机实测）

2026-11-01，Windows 11 / i5-14600KF，语料为本仓库（约 4,380 个文件、82 MB，
含 `third_party/mygo`）；原生与 rg 各 4 轮、轮流先跑，取中位数（GNU grep 为数轮中位数，
测量时段更早、机器负载更低）：

| 操作 | GCode 原生 | rg 15.1.0 | 对比 |
| --- | ---: | ---: | --- |
| Glob `**/*.go`（全树） | 236 ms | 265 ms | 略快 |
| Glob `internal/**/*.go`（按前缀收窄） | 31 ms | 256 ms | 快 8 倍 |
| Grep files `settingsPageViewport` | 2.37 s | 2.28 s | 持平（慢 4%） |
| Grep content 同上 | 2.54 s | 2.24 s | 慢 14% |
| Grep files 高频命中（`e`） | 752 ms | 2.16 s | 快 2.9 倍 |

以上是同一次交错运行（每项 4 轮、原生与 rg 轮流先跑）的中位数，测量时机器有并发负载，
rg 自身也比空闲时慢 3–5 倍，所以同一行的比值比绝对值可靠。GNU grep 3.0 在更早的低负载
时段为 13.8 s（files）与 14.6 s（content），仍慢一个数量级。数值随机器与磁盘状态波动，
不作为跨机承诺。

已借鉴的加速手段（见 `glob.go`、`grep_walk.go`、`search.go`）：
Glob 按模式字面前缀收窄遍历根；Grep 页满即停（`count` 仍统计总数）；目录项自带的
mtime/size 直接用，不再逐文件 `os.Stat`；文件按大小一次读入、二进制只读头部；
**目录遍历 8 路并行、文件按路径排序后再访问**；读取与匹配按核数并行、按路径顺序回填；
`.gitignore`/`.ignore`/`.git/info/exclude` 连同父目录规则一起生效。

## 验证边界

2026-10-30 工具专项使用真实临时目录和 rg，覆盖大小写/上下文公开参数、ignore 文件、
mtime 排序、绝对模式、三种输出、分页、CRLF、多行与输出预算。
Engine 既有三协议夹具也已迁移到 `Glob → Grep` 契约；夹具不代表真实供应商或插件逐项可用率。
Windows 符号链接用例可能因宿主权限跳过，不将跳过视为实测成功。

2026-11-01 改为进程内实现后，同一套用例在**没有 ripgrep 的 PATH** 下全部通过
（`TestSearchToolsNeedNoExternalBinary` 另以空 PATH 断言一次），并新增嵌套 ignore 用例；
此前 Go 原生搜索实现的 2026-10-29 日志在 `output/search-tools-check/`。
这些仍不能证明 Linux 实机、race、真实供应商或原生 UI 已通过。
