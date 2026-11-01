# 外部服务账号与凭据要求核实

核实日期：2026-10-30。基于官方 marketplace 固定 commit
`f8c5e7a1dcb71b10cb74586224b5401f3de5db28` 的 315 条和已固定的各插件源码。

**已逐项证实 35 个插件的指定核心外服业务流程需要独立业务账号或凭据；其中 32 项还取得匿名初始化 HTTP 401 + Bearer 挑战。**
这不是“35 个插件完全不可用”，也不是 315 条认证要求的完整普查。
技能正文、代码建议、本地辅助命令等可能无需连接账号，故整个插件必须凭据才能使用的数量仍未建立。
若要求必须有“所有功能都无法匿名/本地完成”的证据，不能将下面 35 项直接作为这个分母。

用户已有的模型 API key、模型订阅或 Magpie 模型插件登录，不授予 Slack、Notion、AWS、Stripe 等业务服务权限。

## 证据层级与操作范围

1. **源码硬要求**：每项都列固定 repository/commit、README/技能的明确账号或认证说明，
   并在结构化 JSON 列出实际 MCP 配置路径、凭据字段名、环境变量/user_config 模板名。
   `sensitive` 不等于 `required`；没有把敏感字段本身误写成 schema 必填。
2. **服务实测认证门槛**：对 121 个去重 HTTPS endpoint 仅发送匿名 MCP `initialize`，
   不带 Authorization、cookie、既有 session；不跟随重定向，不登录、不调用业务工具。
   103 返回 401，其中 96 带认证挑战；3 返回 403；12 返回 200；另有 302、307、404 各一项。
   403 单独不足以证明认证要求；200 只证明初始化接受，不能等同业务能力已匿名验收。
   仅在该次请求新建临时协议 session 时发送 DELETE 清理，不保存 session/cookie/挑战参数。
3. **推断与未知**：没有把远程 URL 声明、401 总数、无 key 字段、OAuth client ID
   单独当作整插件硬阻塞。没有测试真实登录、scopes、付费权益、额度、读写权限或业务成功率。

握手逐条证据见 [public-mcp-auth-probes.json](public-mcp-auth-probes.json)，
可复核脚本为 [probe_public_mcp_auth.py](probe_public_mcp_auth.py)。
该脚本仅做协议初始化和自身临时 session 回收，不执行插件代码或任何业务 tools/call。

## 逐项源码证据

下表的“必须”限于指定的账户业务能力。OAuth“不需要手填 API key”仍然需要真实账户同意，
CLI 已登录身份也仍是业务凭据；免费层与 sandbox 也不能自动等同无需账号。
所有行的本地状态相同：**在授权范围内未验证到可用的对应业务授权**，
并非断言整个机器或浏览器里绝对没有凭据。

| 插件 ID | 固定源码证据 | 需要的业务授权 | 匿名初始化 |
| --- | --- | --- | --- |
| `amplitude@claude-plugins-official` | [amplitude/mcp-marketplace@c4bcb5feff54 · README.md:72](https://github.com/amplitude/mcp-marketplace/blob/c4bcb5feff54cf3332290ee0071a2def2b1f149d/plugins/amplitude/README.md#L72) | Amplitude 账户与 API 访问权限 | 401 + Bearer |
| `apollo@claude-plugins-official` | [apolloio/apollo-mcp-plugin@bd0513c961f0 · README.md:81](https://github.com/apolloio/apollo-mcp-plugin/blob/bd0513c961f02105492fcff0d7d29c9cccf1ed23/README.md#L81) | Apollo.io 账户、OAuth、相应权限及可能消耗的 credits | 401 + Bearer |
| `appwrite@claude-plugins-official` | [appwrite/claude-plugin@ab3c90b37c95 · README.md:72](https://github.com/appwrite/claude-plugin/blob/ab3c90b37c95b7068f0c064dc562cf21958e8e19/README.md#L72) | Appwrite 托管 MCP 的浏览器 OAuth | 401 + Bearer |
| `atlan@claude-plugins-official` | [atlanhq/agent-toolkit@17b7ae3b4852 · README.md:29](https://github.com/atlanhq/agent-toolkit/blob/17b7ae3b4852a24a9271a55eb339dea0b38f1f60/README.md#L29) | Atlan OAuth 授权 | 401 + Bearer |
| `atlassian@claude-plugins-official` | [atlassian/atlassian-mcp-server@b74220d41142 · README.md:115](https://github.com/atlassian/atlassian-mcp-server/blob/b74220d41142fae52b4f680c805b2ab81050991a/README.md#L115) | Atlassian 站点账户与 OAuth；另有 API token 路径 | 401 + Bearer |
| `clickhouse@claude-plugins-official` | [ClickHouse/clickhouse-claude-code-plugin@652664c8924c · skills/setup/SKILL.md:15](https://github.com/ClickHouse/clickhouse-claude-code-plugin/blob/652664c8924c5a9454d9a7a5f6191944b4adfb11/skills/setup/SKILL.md#L15) | ClickHouse Cloud 凭据与 OAuth | 401 + Bearer |
| `dropbox@claude-plugins-official` | [dropbox/dropbox-ai-plugins@60cea151b269 · README.md:30](https://github.com/dropbox/dropbox-ai-plugins/blob/60cea151b269e9ab47266279d80839c471e0d8c6/claude/README.md#L30) | Dropbox OAuth 与相应 scopes | 401 + Bearer |
| `fullstory@claude-plugins-official` | [fullstorydev/fullstory-skills@ca872d97f5c3 · README.md:24](https://github.com/fullstorydev/fullstory-skills/blob/ca872d97f5c35dc41ee2eecfc6e392a5895ac171/README.md#L24) | Fullstory 账户、MCP beta/API 权限及 API key | 401 + Bearer |
| `gc-ai@claude-plugins-official` | [GC-AI-Inc/gc-ai-claude-plugin@efda779972c7 · README.md:18](https://github.com/GC-AI-Inc/gc-ai-claude-plugin/blob/efda779972c7b7162e79234e164bc2f359a8a518/README.md#L18) | GC AI 账户及 connector 登录 | 401 + Bearer |
| `grafana-cloud-mcp@claude-plugins-official` | [grafana/ai-marketplace@6c28ba1961d5 · README.md:11](https://github.com/grafana/ai-marketplace/blob/6c28ba1961d502d82301cbd64d2322e6e960477c/plugins/grafana-cloud-mcp/README.md#L11) | Grafana Cloud 账户与 OAuth 2.1 | 401 + Bearer |
| `honeycomb@claude-plugins-official` | [honeycombio/agent-skill@6bb09279fc3f · commands/honeycomb-setup.md:44](https://github.com/honeycombio/agent-skill/blob/6bb09279fc3fb69abf113228be939dbb929e34f1/honeycomb/commands/honeycomb-setup.md#L44) | Honeycomb 账户；OAuth 或 API key | 401 + Bearer |
| `hostinger@claude-plugins-official` | [hostinger/claude-plugin@569880c60681 · README.md:46](https://github.com/hostinger/claude-plugin/blob/569880c60681a7068b3ca8ca84e2fe7ab6cfa7ff/README.md#L46) | Hostinger OAuth；或 hPanel API token | 401 + Bearer |
| `incident-io@claude-plugins-official` | [incident-io/skills@00f7d1dc8089 · README.md:9](https://github.com/incident-io/skills/blob/00f7d1dc80892deb1656f1cf577dd073a4a2de10/plugins/incident-io/README.md#L9) | incident.io workspace 的 OAuth | 401 + Bearer |
| `lovable@claude-plugins-official` | [lovablelabs/mcp@0336e6db8026 · README.md:18](https://github.com/lovablelabs/mcp/blob/0336e6db8026b0f02cb89d1451cc48ea3f469791/README.md#L18) | Lovable OAuth 2.1 bearer token | 401 + Bearer |
| `lusha@claude-plugins-official` | [lusha-oss/lusha-mcp-plugin@c3017a3d6831 · README.md:45](https://github.com/lusha-oss/lusha-mcp-plugin/blob/c3017a3d6831e444b781044337f0394e76b12d89/README.md#L45) | Lusha API 访问账户与 OAuth | 401 + Bearer |
| `miro@claude-plugins-official` | [miroapp/miro-ai@b6408e1bdfe0 · README.md:14](https://github.com/miroapp/miro-ai/blob/b6408e1bdfe0c842f209d40b52e92caf118a4d39/claude-plugins/miro/README.md#L14) | Miro MCP OAuth | 401 + Bearer |
| `monday-crm@claude-plugins-official` | [mondaycom/mcp@8fdc0b4af07a · README.md:83](https://github.com/mondaycom/mcp/blob/8fdc0b4af07a4b7e1059ca3f8320d64968737b8e/plugins/monday-crm/README.md#L83) | monday 账户 OAuth；免费层也需要账户 | 401 + Bearer |
| `mongodb-atlas@claude-plugins-official` | [mongodb/agent-skills@c370a63b2707 · README.md:3](https://github.com/mongodb/agent-skills/blob/c370a63b27075008a92134686b24af3817edeb00/plugins/mongodb-atlas/README.md#L3) | MongoDB Atlas 账户 OAuth | 401 + Bearer |
| `newrelic@claude-plugins-official` | [newrelic/claude-code-plugin@f8e5f8b62139 · readme.md:27](https://github.com/newrelic/claude-code-plugin/blob/f8e5f8b62139072a22153ec92de1c0cc1af2f56a/readme.md#L27) | New Relic 账户 OAuth | 401 + Bearer |
| `notion@claude-plugins-official` | [makenotion/claude-code-notion-plugin@9847f2aa1a15 · README.md:83](https://github.com/makenotion/claude-code-notion-plugin/blob/9847f2aa1a15f25df35ed1fb7b4557dbb60cd651/README.md#L83) | Notion OAuth workspace 授权 | 401 + Bearer |
| `posthog@claude-plugins-official` | [PostHog/ai-plugin@e0fbf5e742fb · README.md:20](https://github.com/PostHog/ai-plugin/blob/e0fbf5e742fb4242c4ea509b395b2e343642f0b7/README.md#L20) | PostHog OAuth；LLM analytics 另需项目 API key | 401 + Bearer |
| `railway@claude-plugins-official` | [railwayapp/railway-skills@a100f93e482c · skills/use-railway/references/setup.md:7](https://github.com/railwayapp/railway-skills/blob/a100f93e482c5153774fed74d0b3fb8f8ca63342/plugins/railway/skills/use-railway/references/setup.md#L7) | Railway 登录/注册 OAuth；CLI 也要求登录 | 401 + Bearer |
| `render@claude-plugins-official` | [render-oss/render-plugin-claude-code@d7ecf826c597 · README.md:88](https://github.com/render-oss/render-plugin-claude-code/blob/d7ecf826c5971eb7dfdb5aed79fa838a8d67ff46/README.md#L88) | Render OAuth；CLI 替代也需要 render login | 401 + Bearer |
| `resend@claude-plugins-official` | [resend/resend-skills@fd14937dba5a · README.md:47](https://github.com/resend/resend-skills/blob/fd14937dba5a99de5dba902808f70fa575513cc5/README.md#L47) | Resend 账户、业务 API key，邮件流程还可能需验证域名 | 401 + Bearer |
| `revenuecat@claude-plugins-official` | [RevenueCat/rc-claude-code-plugin@ccfc038185ec · README.md:7](https://github.com/RevenueCat/rc-claude-code-plugin/blob/ccfc038185ec457b2ac9077f3bbf1ba947a44dbb/revenuecat/README.md#L7) | RevenueCat 账户 OAuth | 401 + Bearer |
| `rootly@claude-plugins-official` | [Rootly-AI-Labs/rootly-claude-plugin@65832aa6ff7a · README.md:160](https://github.com/Rootly-AI-Labs/rootly-claude-plugin/blob/65832aa6ff7a7b39c6bd64899a7a64646e3948ed/README.md#L160) | Rootly OAuth；或 ROOTLY_API_TOKEN | 401 + Bearer |
| `sanity@claude-plugins-official` | [sanity-io/agent-toolkit@88d6cdfa7cb0 · README.md:44](https://github.com/sanity-io/agent-toolkit/blob/88d6cdfa7cb06c99edd5f376efa4dac21ae3f877/README.md#L44) | Sanity 已登录 CLI 身份；托管 connector 使用 OAuth | 401 + Bearer |
| `slack@claude-plugins-official` | [slackapi/slack-mcp-plugin@ad6be8809667 · README.md:117](https://github.com/slackapi/slack-mcp-plugin/blob/ad6be88096678812c448b0279390b4d25bce0164/README.md#L117) | Slack workspace OAuth，且需管理员批准 MCP integration | 401 + Bearer |
| `wingspan@claude-plugins-official` | [wingspanHQ/mcp-claude-plugin@7f99cb3d1789 · README.md:42](https://github.com/wingspanHQ/mcp-claude-plugin/blob/7f99cb3d1789a91390fa7abf1571328736b5e0a6/README.md#L42) | Wingspan 账户登录和 OAuth 同意 | 401 + Bearer |
| `zoominfo@claude-plugins-official` | [Zoominfo/zoominfo-mcp-plugin@31a1ec506078 · README.md:26](https://github.com/Zoominfo/zoominfo-mcp-plugin/blob/31a1ec506078e0aaf4e14e098e39865f32777d8a/README.md#L26) | ZoomInfo 账户、API 产品权限与 OAuth | 401 + Bearer |
| `datadog@claude-plugins-official` | [datadog-labs/claude-code-plugin@195f570c002b · README.md:88](https://github.com/datadog-labs/claude-code-plugin/blob/195f570c002beab3d7bd342739034d0019aa4863/README.md#L88) | Datadog OAuth；或 API key + application key | 未探测此配置（非静态公开 URL/本地 stdio） |
| `sonatype-guide@claude-plugins-official` | [sonatype/sonatype-guide-claude-plugin@1dae73980f59 · README.md:17](https://github.com/sonatype/sonatype-guide-claude-plugin/blob/1dae73980f591d3196f5532ac72186513563d028/README.md#L17) | Sonatype Guide 账户与 API token | 401 + Bearer |
| `aws-agents-for-devsecops@claude-plugins-official` | [aws/agent-toolkit-for-aws@7bde20faede4 · README.md:49](https://github.com/aws/agent-toolkit-for-aws/blob/7bde20faede49dd5764c02b54f5e306849f7e3ca/plugins/aws-agents-for-devsecops/README.md#L49) | AWS 账户 SigV4 凭据；DevOps 单空间也可 bearer token | 未探测此配置（非静态公开 URL/本地 stdio） |
| `pagerduty@claude-plugins-official` | [PagerDuty/claude-code-plugins@761cba75bd50 · README.md:44](https://github.com/PagerDuty/claude-code-plugins/blob/761cba75bd50fd561405c3b173ecf36084432089/README.md#L44) | PAGERDUTY_API_KEY；本地 Git history 本身不需此 key | 401 + Bearer |
| `pinecone@claude-plugins-official` | [pinecone-io/pinecone-claude-code-plugin@c383d38b5cc3 · README.md:49](https://github.com/pinecone-io/pinecone-claude-code-plugin/blob/c383d38b5cc3c5ec219f2e68026e47ffbf46524a/README.md#L49) | PINECONE_API_KEY | 未探测此配置（非静态公开 URL/本地 stdio） |

每项完整 commit、指定业务操作、字段/配置路径、探测时间与缺失项保存在
[external-service-requirements.json](external-service-requirements.json)。
没有执行这些业务操作，也没有因发现账号要求而尝试读取其他位置的凭据。

特别注意：PagerDuty 的本地 Git history 不需 API token；Appwrite 的语言技能、
incident.io 的本地架构辅助、Sanity 的 schema/代码说明等可以有独立价值。
表中只把各自的账户业务能力列为待授权，不将这些离线能力计为失败。
Grafana Cloud 的 README 提供另一个 self-hosted 插件作为选择，这不是同一 Cloud connector 的匿名替代。
Hostinger、Honeycomb、Datadog、Rootly、Render、Sanity、Railway 的 API/CLI/OAuth
替代路径仍需对应服务的业务身份，不能用模型授权代替。

## 不计整插件硬阻塞的反例与混合能力

- `microsoft-docs@claude-plugins-official`：README 明确不需要 API key、登录或注册；匿名 initialize 200。 [固定源码](https://github.com/MicrosoftDocs/mcp/blob/f8ffde185dfd232dbf5d187c22ce299eadc3d583/README.md#L16)
- `sumup@claude-plugins-official`：README 明确技能可不连接账户；账户工具才需登录。匿名 connector 401不能据此阻塞整个插件。 [固定源码](https://github.com/sumup/sumup-skills/blob/6922c7052ae9d0be834f27361b785bd80e0829cc/README.md#L16)
- `confidence@claude-plugins-official`：同包 docs endpoint 匿名 initialize 200；flags endpoint 401。是混合能力，不计整体认证硬阻塞。 [固定源码](https://github.com/spotify/confidence-ai-plugins/blob/f416826fde819978ab90f1d7ae27513667aa5d0f/.mcp.json)
- `mapbox@claude-plugins-official`：同包 docs endpoint 匿名 initialize 200；其他 account endpoints 401。不能把它们合并成全包不可用。 [固定源码](https://github.com/mapbox/mapbox-agent-skills/blob/f71cd72b8d8937b06db440538bd65559eb792c49/.mcp.json)
- `aws-agents@claude-plugins-official`：源码区分本地无认证运行与云端强制认证；Knowledge endpoint 匿名 initialize 200。 [固定源码](https://github.com/aws/agent-toolkit-for-aws/blob/0d6167ad2e6dfb858098c4162fe74d04c975532f/plugins/aws-agents/skills/agents-build/references/local-vs-deployed.md#L25)
- `forge-skills@claude-plugins-official`：两个声明的 endpoint 均匿名 initialize 200；此证据仅表示握手接受，不代表业务 tools/call 已验证。 [固定源码](https://github.com/atlassian/forge-skills/blob/4f04ae02879435f23808a4798da28f75d3ab74c3/.mcp.json)
- `qt-development-skills@claude-plugins-official`：Qt docs endpoint 匿名 initialize 200；没有据此认定需要第三方账户。 [固定源码](https://github.com/TheQtCompanyRnD/agent-skills/blob/a784c48445e34f126760f17ba0600a25f2d19968/.mcp.json)

此外，Adobe、BlackRock 和 Val Town 的 403 不直接计为缺账号；Vanguard 的 302、
Mintlify 的 307、Zocks 的 404 也不能充当认证结论。Context7、Exa 等即使本次返回 401，
本报告也没有只凭此状态推定其所有能力必须第三方账号。

## 本地授权定向检查

只读检查已授权的 `C:/Users/chengliang/.gcode/engine.json` 与
`C:/Users/chengliang/.config/magpie/` 下 `providers.json`、`plugins.json`、
`plugin-providers.json`、`plugin-auth.json`、`logins.json`、`settings.json`、`served.json`。
没有用新 Store 打开旧快照；没有打开 routing、浏览器凭据库或其他账户文件。
检查只保留字段类型、字段存在与非空布尔，不复制 key、token、账号标识或配置原值。

| 范围 | 可核实结果 |
| --- | --- |
| GCode 模型 provider key 字段非空 | `true`，只证明模型授权材料存在 |
| GCode snapshot 中命名 MCP 业务 server 定义存在 | `false`（字段为空/None） |
| Magpie 已检查结构是模型 provider/account/auth schema | `true` |
| 已检查结构存在命名 MCP 业务授权定义 | `false` |
| 已验证这些业务服务授权可用 | `false` |
| 浏览器、其他凭据库、routing、无关账号文件被检查 | `false` |

这些结果不能证明凭据不存在于任何其他位置，也不能证明存在的模型 key 当前有效。
对上表业务流程，当前缺的是对应服务授权及 scopes/产品权限验收；有些还需要
Fullstory beta、Slack 管理员同意、ZoomInfo API entitlement、Apollo credits、
Resend 域名验证等，不能仅补一个字符串就承诺业务成功。

## 可复核边界

35 项是独立固定源码证据支持的业务授权依赖下限，32 项叠加了真实匿名认证挑战；
**整插件零凭据不可用数量未知，完整的 315 条业务认证依赖数量也未宣称已穷尽。**
该调查没有改变生产代码，不建立插件失败率，也不降低此前对真实模型/账号业务验收的要求。
