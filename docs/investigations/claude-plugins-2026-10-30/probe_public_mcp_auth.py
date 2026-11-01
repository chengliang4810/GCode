"""Read-only anonymous initialize probes; never calls business tools or logs credentials."""
from __future__ import annotations

import concurrent.futures
import datetime
import json
import pathlib
import urllib.error
import urllib.parse
import urllib.request

BASE = pathlib.Path(__file__).resolve().parent


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def probe(url: str) -> dict:
    result = {
        "url": url,
        "probed_at_utc": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "authorization_sent": False,
        "business_tool_called": False,
    }
    body = json.dumps({
        "jsonrpc": "2.0", "id": 1, "method": "initialize",
        "params": {
            "protocolVersion": "2025-11-25",
            "capabilities": {},
            "clientInfo": {"name": "gcode-public-auth-evidence", "version": "1"},
        },
    }).encode()
    request = urllib.request.Request(url, data=body, headers={
        "Accept": "application/json, text/event-stream",
        "Content-Type": "application/json",
        "User-Agent": "GCode-public-auth-evidence/1",
    })
    opener = urllib.request.build_opener(NoRedirect)
    try:
        response = opener.open(request, timeout=10)
    except urllib.error.HTTPError as exc:
        response = exc
    except Exception as exc:
        result["transport_error_type"] = type(exc).__name__
        return result
    with response:
        result["http_status"] = response.status
        challenge = response.headers.get("WWW-Authenticate", "")
        result["www_authenticate_present"] = bool(challenge)
        result["auth_scheme"] = challenge.split(None, 1)[0] if challenge else None
        # Read a bounded diagnostic prefix only. Do not persist response bodies,
        # challenge parameters, cookies or session identifiers.
        try:
            raw = response.read(4096)
        except Exception as exc:
            raw = b""
            result["diagnostic_read_error_type"] = type(exc).__name__
        text = raw.decode("utf-8", errors="replace").lower()
        result["response_auth_language"] = any(word in text for word in (
            "unauthoriz", "authentication required", "access token", "api key",
            "api_key", "missing token", "invalid token", "not authenticated",
        ))
        result["possible_edge_challenge"] = any(word in text for word in (
            "cloudflare", "captcha", "cf-chl-", "just a moment",
        ))
        session = response.headers.get("Mcp-Session-Id")
        if session and 200 <= response.status < 300:
            # End only the ephemeral session this anonymous probe just opened.
            cleanup = urllib.request.Request(url, method="DELETE", headers={
                "Mcp-Session-Id": session,
                "MCP-Protocol-Version": "2025-11-25",
            })
            try:
                with opener.open(cleanup, timeout=5) as ended:
                    result["session_cleanup_http_status"] = ended.status
            except urllib.error.HTTPError as exc:
                result["session_cleanup_http_status"] = exc.code
            except Exception as exc:
                result["session_cleanup_error_type"] = type(exc).__name__
    return result


def main():
    source = json.loads((BASE / "marketplace-audit.json").read_text(encoding="utf-8"))
    urls: dict[str, list[dict]] = {}
    for entry in source["plugins"]:
        if entry["marketplace"] != "claude-plugins-official":
            continue
        for detail in entry.get("mcp_details", []):
            if not isinstance(detail, dict):
                continue
            for server, config in detail.get("mcpServers", detail).items():
                if not isinstance(config, dict):
                    continue
                url = config.get("url", "")
                if not isinstance(url, str):
                    continue
                parsed = urllib.parse.urlparse(url)
                if (parsed.scheme != "https" or not parsed.hostname or parsed.username
                        or any(marker in url for marker in ("${", "{", "}", "<", ">"))):
                    continue
                ref = {"plugin": entry["name"], "server": server}
                urls.setdefault(url, [])
                if ref not in urls[url]:
                    urls[url].append(ref)
    results = []
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
        pending = {pool.submit(probe, url): refs for url, refs in urls.items()}
        for future in concurrent.futures.as_completed(pending):
            record = future.result()
            record["entries"] = pending[future]
            results.append(record)
    report = {
        "method": "Anonymous initialize only. No account, Authorization, cookies, "
                  "business tool calls, login or data creation. Redirects refused. "
                  "HTTP 401 proves an anonymous authentication gate, not that "
                  "every plugin feature requires authentication. HTTP 403 alone "
                  "is not treated as an authentication requirement.",
        "source_marketplace_commit": "f8c5e7a1dcb71b10cb74586224b5401f3de5db28",
        "results": sorted(results, key=lambda row: row["url"]),
    }
    output = BASE / "public-mcp-auth-probes.json"
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({
        "endpoints": len(results),
        "http_401": sum(row.get("http_status") == 401 for row in results),
        "http_403": sum(row.get("http_status") == 403 for row in results),
        "transport_errors": sum("transport_error_type" in row for row in results),
        "output": str(output),
    }))


if __name__ == "__main__":
    main()
