"""Build a bounded report from prior evidence; local inspection facts are frozen."""
from __future__ import annotations

import datetime
import json
import pathlib
import re
import tarfile
import urllib.parse

BASE = pathlib.Path(__file__).resolve().parent

# Each row is manually reviewed against this fixed public source location.
# These describe account-backed business operations, not offline skill usefulness.
SELECTION = [
    ("amplitude", "README.md", 72, "Amplitude 账户与 API 访问权限", "读取账户产品分析数据、创建图表和仪表盘"),
    ("apollo", "README.md", 81, "Apollo.io 账户、OAuth、相应权限及可能消耗的 credits", "检索和丰富账户下的联系人及销售数据"),
    ("appwrite", "README.md", 72, "Appwrite 托管 MCP 的浏览器 OAuth", "通过托管 MCP 管理 Appwrite 项目资源"),
    ("atlan", "README.md", 29, "Atlan OAuth 授权", "访问 Atlan 数据目录与元数据"),
    ("atlassian", "README.md", 115, "Atlassian 站点账户与 OAuth；另有 API token 路径", "读取和管理账户范围内的 Jira、Confluence 等资源"),
    ("clickhouse", "skills/setup/SKILL.md", 15, "ClickHouse Cloud 凭据与 OAuth", "查询并管理 ClickHouse Cloud 账户资源"),
    ("dropbox", "README.md", 30, "Dropbox OAuth 与相应 scopes", "查找、整理、分享账户内 Dropbox 内容"),
    ("fullstory", "README.md", 24, "Fullstory 账户、MCP beta/API 权限及 API key", "分析 Fullstory 会话和账户数据"),
    ("gc-ai", "README.md", 18, "GC AI 账户及 connector 登录", "读取 GC AI 项目与账户内法律研究资料"),
    ("grafana-cloud-mcp", "README.md", 11, "Grafana Cloud 账户与 OAuth 2.1", "访问 Grafana Cloud stack 和观测数据"),
    ("honeycomb", "commands/honeycomb-setup.md", 44, "Honeycomb 账户；OAuth 或 API key", "检索和分析 Honeycomb 观测数据"),
    ("hostinger", "README.md", 46, "Hostinger OAuth；或 hPanel API token", "管理 Hostinger 托管资源和部署"),
    ("incident-io", "README.md", 9, "incident.io workspace 的 OAuth", "查询和处理 workspace 内事故资源"),
    ("lovable", "README.md", 18, "Lovable OAuth 2.1 bearer token", "访问和操作 Lovable 账户项目"),
    ("lusha", "README.md", 45, "Lusha API 访问账户与 OAuth", "执行 Lusha 联系人及公司数据业务"),
    ("miro", "README.md", 14, "Miro MCP OAuth", "读取和修改 Miro board"),
    ("monday-crm", "README.md", 83, "monday 账户 OAuth；免费层也需要账户", "读取和创建 monday CRM 工作区与交易数据"),
    ("mongodb-atlas", "README.md", 3, "MongoDB Atlas 账户 OAuth", "通过托管 Atlas MCP 访问集群"),
    ("newrelic", "readme.md", 27, "New Relic 账户 OAuth", "查询账户内 New Relic 遥测与资源"),
    ("notion", "README.md", 83, "Notion OAuth workspace 授权", "读取和修改 Notion workspace 内容"),
    ("posthog", "README.md", 20, "PostHog OAuth；LLM analytics 另需项目 API key", "查询 PostHog 项目分析数据或上传启用后的会话遥测"),
    ("railway", "skills/use-railway/references/setup.md", 7, "Railway 登录/注册 OAuth；CLI 也要求登录", "在 Railway 部署和管理项目"),
    ("render", "README.md", 88, "Render OAuth；CLI 替代也需要 render login", "管理 Render 服务、部署、日志与数据库"),
    ("resend", "README.md", 47, "Resend 账户、业务 API key，邮件流程还可能需验证域名", "调用 Resend 发送/管理邮件及账户资源"),
    ("revenuecat", "README.md", 7, "RevenueCat 账户 OAuth", "访问 RevenueCat 项目及购买配置"),
    ("rootly", "README.md", 160, "Rootly OAuth；或 ROOTLY_API_TOKEN", "查询和管理 Rootly 事故数据"),
    ("sanity", "README.md", 44, "Sanity 已登录 CLI 身份；托管 connector 使用 OAuth", "访问 Sanity 项目、dataset 和远程内容"),
    ("slack", "README.md", 117, "Slack workspace OAuth，且需管理员批准 MCP integration", "读取和处理 Slack workspace 业务内容"),
    ("wingspan", "README.md", 42, "Wingspan 账户登录和 OAuth 同意", "读取 Wingspan 账户或执行需确认的账户操作"),
    ("zoominfo", "README.md", 26, "ZoomInfo 账户、API 产品权限与 OAuth", "检索和丰富 ZoomInfo 业务数据"),
    ("datadog", "README.md", 88, "Datadog OAuth；或 API key + application key", "读取 Datadog 观测数据和执行账户范围内操作"),
    ("sonatype-guide", "README.md", 17, "Sonatype Guide 账户与 API token", "调用 Sonatype Guide 的账户化依赖风险分析"),
    ("aws-agents-for-devsecops", "README.md", 49, "AWS 账户 SigV4 凭据；DevOps 单空间也可 bearer token", "运行 AWS DevOps/Security Agent 的云端调查"),
    ("pagerduty", "README.md", 44, "PAGERDUTY_API_KEY；本地 Git history 本身不需此 key", "读取 PagerDuty 服务和事故数据以完成服务化事故流程"),
    ("pinecone", "README.md", 49, "PINECONE_API_KEY", "查询、创建和操作 Pinecone 向量索引"),
]

COUNTEREXAMPLES = [
    ("microsoft-docs", "README.md", 16, "README 明确不需要 API key、登录或注册；匿名 initialize 200。", "https://learn.microsoft.com/api/mcp"),
    ("sumup", "README.md", 16, "README 明确技能可不连接账户；账户工具才需登录。匿名 connector 401不能据此阻塞整个插件。", "https://mcp.sumup.com/mcp"),
    ("confidence", ".mcp.json", None, "同包 docs endpoint 匿名 initialize 200；flags endpoint 401。是混合能力，不计整体认证硬阻塞。", "https://mcp.confidence.dev/mcp/docs"),
    ("mapbox", ".mcp.json", None, "同包 docs endpoint 匿名 initialize 200；其他 account endpoints 401。不能把它们合并成全包不可用。", "https://mcp-docs.mapbox.com/mcp"),
    ("aws-agents", "plugins/aws-agents/skills/agents-build/references/local-vs-deployed.md", 25, "源码区分本地无认证运行与云端强制认证；Knowledge endpoint 匿名 initialize 200。", "https://knowledge-mcp.global.api.aws"),
    ("forge-skills", ".mcp.json", None, "两个声明的 endpoint 均匿名 initialize 200；此证据仅表示握手接受，不代表业务 tools/call 已验证。", "https://mcp.atlassian.com/v1/forge/mcp"),
    ("qt-development-skills", ".mcp.json", None, "Qt docs endpoint 匿名 initialize 200；没有据此认定需要第三方账户。", "https://qt-docs-mcp.qt.io/mcp"),
]


def read_source(entry: dict, cache: pathlib.Path, relative: str) -> list[str]:
    archive = cache / (
        entry["source_repo"].replace("/", "--") + "--" + entry["source_commit"] + ".tar.gz"
    )
    prefix = entry.get("source_path", ".").strip("./")
    target = (prefix + "/" if prefix else "") + relative
    with tarfile.open(archive, "r:gz") as tar:
        for member in tar:
            repo_relative = member.name.split("/", 1)[-1]
            if repo_relative == target and member.isfile() and member.size < 2 * 1024 * 1024:
                return tar.extractfile(member).read().decode("utf-8").splitlines()
    raise FileNotFoundError(f"{entry['name']}: {target}")


def server_configs(entry: dict) -> dict:
    servers = {}
    for detail in entry.get("mcp_details", []):
        if isinstance(detail, dict):
            servers.update({
                k: v for k, v in detail.get("mcpServers", detail).items()
                if isinstance(v, dict)
            })
    return servers


def main():
    audit = json.loads((BASE / "marketplace-audit.json").read_text(encoding="utf-8"))
    probes = json.loads((BASE / "public-mcp-auth-probes.json").read_text(encoding="utf-8"))
    by_url = {record["url"]: record for record in probes["results"]}
    official = {
        row["name"]: row for row in audit["plugins"]
        if row["marketplace"] == "claude-plugins-official"
    }
    cache = pathlib.Path(audit["cache_directory"])
    records = []
    for name, path, line, requirement, operation in SELECTION:
        entry = official[name]
        lines = read_source(entry, cache, path)
        if not 1 <= line <= len(lines):
            raise ValueError(f"source line is unavailable: {name}:{path}:{line}")
        relative = entry.get("source_path", ".").strip("./")
        repo_path = (relative + "/" if relative else "") + path
        # Short evidence excerpt, not a README copy.
        excerpt = " ".join(lines[line - 1].strip().split()[:16])
        configs = server_configs(entry)
        variables = set()
        configuration_fields = []
        matched_probes = []
        for server, config in configs.items():
            for category in ("env", "headers"):
                for key, value in config.get(category, {}).items():
                    if not isinstance(value, str):
                        continue
                    for variable in re.findall(r"\$\{([^}]+)\}", value):
                        variables.add(variable)
                    if any(part in key.lower() for part in ("auth", "token", "secret", "key", "pass")):
                        configuration_fields.append({"server": server, "category": category, "field": key})
            if config.get("oauth"):
                configuration_fields.append({"server": server, "category": "oauth", "field": "oauth"})
            probe = by_url.get(config.get("url", ""))
            if probe:
                matched_probes.append({
                    key: probe[key] for key in [
                        "url", "probed_at_utc", "http_status", "auth_scheme",
                        "www_authenticate_present", "possible_edge_challenge",
                    ] if key in probe
                })
        challenge_401 = any(
            p.get("http_status") == 401 and p.get("www_authenticate_present")
            and not p.get("possible_edge_challenge") for p in matched_probes
        )
        anonymous_init_200 = any(p.get("http_status") == 200 for p in matched_probes)
        records.append({
            "plugin_id": name + "@claude-plugins-official",
            "entry_name": name,
            "manifest_name": (entry.get("manifest") or {}).get("name", name),
            "repository": entry["source_repo"],
            "commit": entry["source_commit"],
            "plugin_source_path": entry.get("source_path", "."),
            "business_operation_requiring_authorization": operation,
            "required_business_credential_or_account": requirement,
            "source_hard_requirement_evidence": {
                "path_relative_to_plugin": path,
                "path_relative_to_repository": repo_path,
                "line": line,
                "short_excerpt": excerpt,
                "url": "https://github.com/" + entry["source_repo"] + "/blob/"
                       + entry["source_commit"] + "/" + urllib.parse.quote(repo_path, safe="/") + f"#L{line}",
            },
            "configuration_paths": entry.get("components", {}).get("mcp", []),
            "user_config_fields": [
                {
                    "key": key,
                    "type": field.get("type"),
                    "sensitive": field.get("sensitive", False),
                    "required": field.get("required", False),
                    "default_declared": "default" in field,
                    "definition_path": ".claude-plugin/plugin.json#userConfig." + key,
                }
                for key, field in (entry.get("manifest") or {}).get("userConfig", {}).items()
                if isinstance(field, dict)
            ],
            "credential_field_names": configuration_fields,
            "configuration_variable_names": sorted(variables),
            "anonymous_handshake_evidence": matched_probes,
            "anonymous_http_401_with_challenge_observed": challenge_401,
            "anonymous_initialize_200_observed": anonymous_init_200,
            "inference_only": False,
            "counted_as_core_service_operation_credential_dependency": True,
            "whole_plugin_unusable_without_credentials": "not_established",
            "anonymous_replacement_for_same_account_business_operation_found": False,
            "offline_or_other_plugin_features_may_remain_usable": True,
            "local_verified_business_authorization_available": False,
            "missing_or_unverified_prerequisites": [
                requirement,
                "授权范围内本地配置未发现可验证的对应业务授权；浏览器及其他凭据库未检查",
                "未登录、未执行任何服务业务工具；账户 scopes、产品权益、额度及数据访问尚未验收",
            ],
            "runtime_business_operation": "not_tested",
        })
    local = {
        "evidence_as_of_date": "2026-10-30",
        "frozen_inspection_not_revalidated_by_builder": True,
        "inspection_scope": [
            "C:/Users/chengliang/.gcode/engine.json",
            "C:/Users/chengliang/.config/magpie/providers.json",
            "C:/Users/chengliang/.config/magpie/plugins.json",
            "C:/Users/chengliang/.config/magpie/plugin-providers.json",
            "C:/Users/chengliang/.config/magpie/plugin-auth.json",
            "C:/Users/chengliang/.config/magpie/logins.json",
            "C:/Users/chengliang/.config/magpie/settings.json",
            "C:/Users/chengliang/.config/magpie/served.json",
        ],
        "gcode_model_api_key_field_nonempty": True,
        "gcode_snapshot_named_mcp_business_servers_present": False,
        "magpie_checked_schema_is_model_provider_account_auth": True,
        "magpie_checked_schema_has_named_mcp_business_auth_definition": False,
        "business_credentials_verified_available": False,
        "credential_values_or_account_identifiers_copied": False,
        "browser_credentials_or_unrelated_account_files_examined": False,
        "routing_directory_examined": False,
        "limit": "No verified business authorization in these scoped files is not "
                 "proof that no credentials exist elsewhere. Model-provider "
                 "authorization does not authorize a third-party business service.",
    }
    report = {
        "generated_at_utc": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "marketplace_entry_count": len(official),
        "marketplace_commit": "f8c5e7a1dcb71b10cb74586224b5401f3de5db28",
        "assessment_scope": "35 individually source-proven credential requirements "
                            "for named account-backed service operations. Not a "
                            "complete census of all credential dependencies in 315 "
                            "entries and not a count of wholly unusable plugins.",
        "source_proven_core_service_dependencies_lower_bound": len(records),
        "selected_entries_with_anonymous_401_auth_challenge": sum(
            r["anonymous_http_401_with_challenge_observed"] for r in records
        ),
        "selected_entries_with_anonymous_initialize_200": sum(
            r["anonymous_initialize_200_observed"] for r in records
        ),
        "whole_plugin_credential_block_count": None,
        "whole_plugin_credential_block_count_reason": "Not established; offline "
            "skills, local tools and alternative capabilities must not be treated "
            "as unavailable merely because the account connector is gated.",
        "local_authorization_inspection": local,
        "plugins": records,
        "counterexamples_and_mixed_capabilities": [
            {
                "plugin_id": name + "@claude-plugins-official",
                "repository": official[name]["source_repo"],
                "commit": official[name]["source_commit"],
                "path": path,
                "line": line,
                "reason": reason,
                "anonymous_probe": {
                    key: by_url[url].get(key) for key in
                    ["url", "http_status", "auth_scheme", "probed_at_utc"]
                },
                "counted_as_whole_plugin_credential_blocker": False,
            }
            for name, path, line, reason, url in COUNTEREXAMPLES
        ],
        "probe_summary": {
            "endpoints": len(probes["results"]),
            "http_401": sum(p.get("http_status") == 401 for p in probes["results"]),
            "http_401_with_auth_challenge": sum(p.get("http_status") == 401 and p.get("www_authenticate_present") for p in probes["results"]),
            "http_403": sum(p.get("http_status") == 403 for p in probes["results"]),
            "http_200": sum(p.get("http_status") == 200 for p in probes["results"]),
            "redirect_or_not_found": sum(p.get("http_status") in [302, 307, 404] for p in probes["results"]),
            "business_calls": 0,
            "login_flows": 0,
        },
    }
    (BASE / "external-service-requirements.json").write_text(
        json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    print(json.dumps({
        "selected_source_proven": len(records),
        "selected_401_with_challenge": report["selected_entries_with_anonymous_401_auth_challenge"],
        "selected_initialize_200": report["selected_entries_with_anonymous_initialize_200"],
    }))


if __name__ == "__main__":
    main()
