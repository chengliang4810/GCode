"""Fetch frozen public Git objects for acceptance; never check out or execute plugins."""
from __future__ import annotations

import argparse
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import threading
from urllib.parse import urlparse


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--snapshots", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--marketplace-file", type=Path,
                        help="Also fetch explicitly pinned sources from the frozen marketplace")
    parser.add_argument("--workers", type=int, default=4)
    parser.add_argument("--timeout", type=int, default=120)
    args = parser.parse_args()
    if not 1 <= args.workers <= 8 or not 1 <= args.timeout <= 600:
        parser.error("workers must be 1..8; timeout must be 1..600 seconds")
    root = args.output.resolve()
    root.mkdir(parents=True, exist_ok=True)
    sources = set()
    for path in args.snapshots.glob("claude-plugins-official--*--source-evidence.json"):
        entry = json.loads(path.read_text(encoding="utf-8"))
        repo, commit = entry.get("repo", ""), entry.get("commit", "")
        if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repo):
            raise ValueError(f"invalid public GitHub repository in {path.name}")
        if not re.fullmatch(r"[0-9a-f]{40}", commit):
            raise ValueError(f"unfrozen source in {path.name}")
        sources.add((repo, commit))
    if args.marketplace_file:
        market = json.loads(args.marketplace_file.read_text(encoding="utf-8"))
        for entry in market["plugins"]:
            source = entry.get("source")
            if not isinstance(source, dict) or not source.get("sha"):
                continue
            commit = source["sha"]
            if not re.fullmatch(r"[0-9a-f]{40}", commit):
                raise ValueError("marketplace pin must be a complete lowercase commit SHA")
            repo = source.get("repo", "")
            if source.get("url"):
                parsed = urlparse(source["url"])
                if parsed.scheme != "https" or parsed.hostname != "github.com":
                    raise ValueError("cache fetch only accepts public HTTPS GitHub sources")
                repo = parsed.path.strip("/").removesuffix(".git")
            if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repo):
                raise ValueError("invalid pinned public GitHub repository")
            sources.add((repo, commit))
    env = dict(os.environ, GIT_TERMINAL_PROMPT="0", GIT_CONFIG_NOSYSTEM="1",
               GIT_CONFIG_GLOBAL=os.devnull, GIT_NO_REPLACE_OBJECTS="1")
    # Keep only network routing from the user's Git config. Do not inherit
    # credential helpers, rewrite rules or executable hooks.
    proxy_settings = []
    for key in ("http.proxy", "https.proxy"):
        result = subprocess.run(["git", "config", "--global", "--get", key],
                                capture_output=True, text=True)
        if result.returncode == 0 and result.stdout.strip():
            proxy_settings.append((key, result.stdout.strip()))
    env["GIT_CONFIG_COUNT"] = str(len(proxy_settings))
    for index, (key, value) in enumerate(proxy_settings):
        env[f"GIT_CONFIG_KEY_{index}"] = key
        env[f"GIT_CONFIG_VALUE_{index}"] = value
    gate = threading.Lock()
    results = []

    def save():
        ready = [row for row in results if row["status"] == "ready"]
        (root / "index.json.tmp").write_text(
            json.dumps({"sources": [{k: row[k] for k in ("repository", "commit", "root")}
                                    for row in ready]}, indent=2), encoding="utf-8")
        (root / "index.json.tmp").replace(root / "index.json")
        (root / "fetch-results.json").write_text(
            json.dumps(sorted(results, key=lambda row: row["repository"]), indent=2),
            encoding="utf-8")

    def fetch(source):
        repo, commit = source
        name = hashlib.sha256(f"{repo}@{commit}".encode()).hexdigest()[:24]
        target = root / name
        row = {"repository": repo, "commit": commit, "root": name, "status": "failed"}
        prefix = ["git", "--no-replace-objects", "-c", "core.hooksPath=",
                  "-c", "protocol.ext.allow=never"]

        def git(*parts, check=True):
            result = subprocess.run(prefix + list(parts), env=env, capture_output=True,
                                    text=True, timeout=args.timeout)
            if check and result.returncode:
                raise RuntimeError(result.stderr[-4096:])
            return result

        try:
            if not target.exists():
                git("init", "--bare", str(target))
            result = git("-C", str(target), "cat-file", "-t", commit, check=False)
            if result.returncode or result.stdout.strip() != "commit":
                git("-C", str(target), "fetch", "--depth=1", "--no-tags",
                    "--no-recurse-submodules", f"https://github.com/{repo}.git", commit)
            actual = git("-C", str(target), "rev-parse", "--verify", f"{commit}^{{commit}}")
            if actual.stdout.strip() != commit:
                raise ValueError("downloaded Git object does not match frozen commit")
            row["status"] = "ready"
        except (OSError, ValueError, RuntimeError, subprocess.TimeoutExpired) as error:
            row["error"] = str(error)[-4096:]
        with gate:
            results.append(row)
            save()
            print(f"{len(results)}/{len(sources)} {repo} {row['status']}", flush=True)

    with concurrent.futures.ThreadPoolExecutor(max_workers=args.workers) as pool:
        list(pool.map(fetch, sorted(sources)))
    failed = sum(row["status"] != "ready" for row in results)
    print(f"Ready {len(results)-failed}/{len(results)}. No plugin was executed.")
    if failed:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
