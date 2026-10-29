#!/usr/bin/env python3
"""Prepare only R17's explicitly supplied, isolated diagnostic manifests.

This uses the existing lock as data. It never invokes Cargo, resolves versions,
downloads dependencies, edits the source cache, or changes renderer source.
Cargo --offline --locked remains a separate, required validation step.
"""

import argparse
import collections
import copy
import difflib
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import sys
import tomllib


GIT_URL = "https://github.com/zeronsh/zui"
REVISION = "b564a34413bb7ef1da04121c4a287758f94993d9"
GIT_SOURCE = f"git+{GIT_URL}?rev={REVISION}#{REVISION}"
REFERENCE_COMMIT = "ab189159f10cf8e63c0502be75a59bbc487e0eaf"
FEATURE = "renderer-device-identity"
SOURCE_ROOT_MANIFEST_SHA = "8321ad8583475138fc1819ff4fb39d319ade62aa8b7235e4c789e1c63f8061fa"
SOURCE_WINDOWS_MANIFEST_SHA = "8a0dd8beaa4189bb174f8269cc8e24ec3b960644754fceb2fcb505ccd666cec0"
PATH_DEPENDENCIES = {"gpui", "collections", "gpui_util"}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def parse(data):
    return tomllib.loads(data.decode("utf-8-sig"))


def write_json(path, value):
    path.write_text(json.dumps(value, indent=2, ensure_ascii=False, allow_nan=False) + "\n", encoding="utf-8")


def contains(parent, child):
    return child == parent or parent in child.parents


def checked_output_path(value, task_root):
    raw = Path(os.path.abspath(value))
    for ancestor in (raw, *raw.parents):
        try:
            info = ancestor.lstat()
        except FileNotFoundError:
            continue
        require(not stat.S_ISLNK(info.st_mode), f"Symlink write path: {ancestor}")
        require(not (getattr(info, "st_file_attributes", 0) & 0x400), f"Reparse write path: {ancestor}")
    resolved = raw.resolve()
    output = (task_root / "output").resolve()
    require(resolved != output and contains(output, resolved), f"Write path must be beneath this checkout/output: {resolved}")
    for protected in ("reference-native", "reference-native-0.2.128", "reference-sources"):
        require(not contains(output / protected, resolved), f"Preserved output is not a diagnostic mirror: {resolved}")
    return resolved


def toml_value(value):
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, str):
        return json.dumps(value, ensure_ascii=False)
    if isinstance(value, int):
        return str(value)
    if isinstance(value, list):
        return "[" + ", ".join(toml_value(item) for item in value) + "]"
    if isinstance(value, dict):
        return "{ " + ", ".join(f"{toml_key(key)} = {toml_value(item)}" for key, item in value.items()) + " }"
    raise ValueError(f"Unsupported manifest value type: {type(value).__name__}")


def toml_key(value):
    return value if re.fullmatch(r"[A-Za-z0-9_-]+", value) else json.dumps(value, ensure_ascii=False)


def manifest_bytes(document):
    lines = ["# R17 isolated diagnostic package; inherited definitions expanded from the pinned source."]

    def table(path, values):
        if path:
            lines.extend(("", "[" + ".".join(toml_key(part) for part in path) + "]"))
        for key, value in values.items():
            if not isinstance(value, dict):
                lines.append(f"{toml_key(key)} = {toml_value(value)}")
        for key, value in values.items():
            if isinstance(value, dict):
                table((*path, key), value)

    table((), document)
    result = ("\n".join(lines) + "\n").encode("utf-8")
    require(parse(result) == document, "Generated standalone manifest failed semantic roundtrip")
    return result


def dependency_tables(document):
    for kind in ("dependencies", "dev-dependencies", "build-dependencies"):
        if kind in document:
            yield kind, document[kind]
    for target, values in document.get("target", {}).items():
        for kind in ("dependencies", "dev-dependencies", "build-dependencies"):
            if kind in values:
                yield f"target.{target}.{kind}", values[kind]


def flatten_windows(document, source_workspace):
    result = copy.deepcopy(document)
    workspace = source_workspace["workspace"]
    require(result["package"]["name"] == "gpui_windows", "Unexpected copied package")
    for key, value in list(result["package"].items()):
        if isinstance(value, dict) and value.get("workspace") is True:
            require(value == {"workspace": True}, f"Unexpected package inheritance options: {key}")
            result["package"][key] = copy.deepcopy(workspace["package"][key])
    if result.get("lints", {}).get("workspace") is True:
        require(result["lints"] == {"workspace": True}, "Unexpected lint inheritance options")
        result["lints"] = copy.deepcopy(workspace["lints"])
    bindings = []
    for location, dependencies in dependency_tables(result):
        for name, declaration in list(dependencies.items()):
            if not isinstance(declaration, dict) or declaration.get("workspace") is not True:
                continue
            local = {key: value for key, value in declaration.items() if key != "workspace"}
            require(set(local) <= {"features", "optional", "default-features"}, f"Unsupported dependency inheritance options: {location}.{name}")
            base = copy.deepcopy(workspace["dependencies"][name])
            if isinstance(base, str):
                base = {"version": base}
            if "features" in local:
                base["features"] = list(dict.fromkeys(base.get("features", []) + local.pop("features")))
            base.update(local)
            if "path" in base:
                require(name in PATH_DEPENDENCIES, f"Unexpected inherited path dependency: {name}")
                require(base.pop("path") == f"crates/{name}", f"Unexpected inherited path for {name}")
                base.update({"git": GIT_URL, "rev": REVISION})
                bindings.append(name)
            dependencies[name] = base
    require(set(bindings) == PATH_DEPENDENCIES, "Expected all three same-identity GPUI bindings")
    require(FEATURE not in result["features"], "Diagnostic feature already exists; preserve the supplied mirror")
    result["features"][FEATURE] = []
    require("workspace" not in result, "Copied gpui_windows is already a workspace")
    result["workspace"] = {}

    def no_inheritance(value):
        if isinstance(value, dict):
            require(value.get("workspace") is not True, "Unexpanded workspace inheritance remains")
            for child in value.values():
                no_inheritance(child)
        elif isinstance(value, list):
            for child in value:
                no_inheritance(child)

    no_inheritance(result)
    return result, bindings


def newline(data):
    return "\r\n" if b"\r\n" in data else "\n"


def append_root_patch(data, windows_root):
    original = parse(data)
    require(GIT_URL not in original.get("patch", {}), "Zui patch already exists; preserve it")
    for name in ("gpui", "gpui_platform", "gpui_tokio"):
        dep = original["workspace"]["dependencies"][name]
        require(dep.get("git") == GIT_URL and dep.get("rev") == REVISION, f"Unexpected Zeron {name} identity")
    patch = {"gpui_windows": {"path": windows_root.as_posix()}}
    expected = copy.deepcopy(original)
    expected.setdefault("patch", {})[GIT_URL] = patch
    end = newline(data)
    text = f'{end}# R17 diagnostic mirror only; original Git package replaced by the isolated package.{end}[patch.{toml_key(GIT_URL)}]{end}gpui_windows = {toml_value(patch["gpui_windows"])}{end}'
    result = data + (b"" if data.endswith(b"\n") else end.encode()) + text.encode("utf-8")
    require(parse(result) == expected, "Root manifest has an unapproved semantic delta")
    return result


def add_ui_dependency(data):
    original = parse(data)
    require(original["package"]["name"] == "zeron-ui", "Unexpected UI package")
    require(all("gpui_windows" not in entries for _, entries in dependency_tables(original)), "UI already directly depends on gpui_windows")
    declaration = {"git": GIT_URL, "rev": REVISION, "default-features": False, "features": [FEATURE]}
    expected = copy.deepcopy(original)
    expected.setdefault("target", {}).setdefault("cfg(windows)", {}).setdefault("dev-dependencies", {})["gpui_windows"] = declaration
    end = newline(data)
    line = "gpui_windows = " + toml_value(declaration) + end
    text = data.decode("utf-8-sig")
    header = re.compile(r"(?m)^\[target\.(['\"])cfg\(windows\)\1\.dev-dependencies\][ \t]*(?:#[^\r\n]*)?\r?\n")
    matches = list(header.finditer(text))
    require(len(matches) <= 1, "Duplicate Windows dev-dependency table")
    if matches:
        at = matches[0].end()
        text = text[:at] + line + text[at:]
    else:
        require("dev-dependencies" not in original.get("target", {}).get("cfg(windows)", {}), "Unsupported existing Windows dev-dependency header spelling")
        text += ("" if text.endswith("\n") else end) + end + "[target.'cfg(windows)'.dev-dependencies]" + end + line
    result = (b"\xef\xbb\xbf" if data.startswith(b"\xef\xbb\xbf") else b"") + text.encode("utf-8")
    require(parse(result) == expected, "UI manifest has an unapproved semantic delta")
    return result


def prepare_lock(data):
    original = parse(data)
    packages = original["package"]
    windows = [p for p in packages if p["name"] == "gpui_windows"]
    ui = [p for p in packages if p["name"] == "zeron-ui"]
    require(len(windows) == len(ui) == 1, "Expected unique gpui_windows and zeron-ui lock packages")
    require(windows[0]["version"] == "0.1.0" and windows[0].get("source") == GIT_SOURCE, "Unexpected locked gpui_windows source/version")
    require("checksum" not in windows[0], "Unexpected checksum on Git gpui_windows")
    require("source" not in ui[0], "Expected a local zeron-ui lock package")
    require("gpui_windows" not in ui[0]["dependencies"], "UI lock edge already exists")
    gpui = [p for p in packages if p["name"] == "gpui"]
    require(len(gpui) == 1 and gpui[0].get("source") == GIT_SOURCE, "GPUI identity is already duplicated or unexpected")
    text = data.decode("utf-8-sig")
    blocks = list(re.finditer(r"(?ms)^\[\[package\]\]\r?\n.*?(?=^\[\[package\]\]|\Z)", text))
    changes = []
    for match in blocks:
        block = match.group()
        name = parse(block.encode())["package"][0]["name"]
        if name == "gpui_windows":
            block, count = re.subn(r'(?m)^source = ' + re.escape(toml_value(GIT_SOURCE)) + r'\r?\n', "", block)
            require(count == 1, "Could not remove exactly one locked Git source line")
        elif name == "zeron-ui":
            lines = block.splitlines(keepends=True)
            starts = [i for i, value in enumerate(lines) if value.rstrip("\r\n") == "dependencies = ["]
            require(len(starts) == 1, "Expected a multiline UI lock dependency array")
            start = starts[0] + 1
            stop = next(i for i in range(start, len(lines)) if lines[i].rstrip("\r\n") == "]")
            entries = [json.loads(value.strip().removesuffix(",")) for value in lines[start:stop]]
            require(entries == ui[0]["dependencies"] and entries == sorted(entries), "Unexpected UI lock dependency ordering/layout")
            at = next((i for i, value in enumerate(entries) if value > "gpui_windows"), len(entries))
            lines.insert(start + at, ' "gpui_windows",' + newline(data))
            block = "".join(lines)
        else:
            continue
        changes.append((match.start(), match.end(), block))
    require(len(changes) == 2, "Expected exactly two lock package blocks to change")
    for start, stop, replacement in reversed(changes):
        text = text[:start] + replacement + text[stop:]
    result = (b"\xef\xbb\xbf" if data.startswith(b"\xef\xbb\xbf") else b"") + text.encode("utf-8")
    expected = copy.deepcopy(original)
    for package in expected["package"]:
        if package["name"] == "gpui_windows":
            del package["source"]
        elif package["name"] == "zeron-ui":
            package["dependencies"] = sorted(package["dependencies"] + ["gpui_windows"])
    actual = parse(result)
    require(actual == expected, "Lock has an unapproved semantic delta")
    registry = lambda doc: collections.Counter((p["name"], p["version"], p["source"], p.get("checksum")) for p in doc["package"] if p.get("source", "").startswith("registry+"))
    require(registry(original) == registry(actual), "Registry package/version/checksum multiset changed")
    version_set = lambda doc: collections.Counter((p["name"], p["version"], p.get("checksum")) for p in doc["package"])
    require(version_set(original) == version_set(actual), "A package version/checksum/count changed")
    return result, {"packageCount": len(packages), "registryPackageCount": sum(registry(original).values()), "registryVersionChecksumMultisetUnchanged": True,
                    "allPackageVersionChecksumMultisetUnchanged": True, "onlySourceIdentityMigration": "gpui_windows Git -> local path", "onlyAddedNamedDependencyEdge": "zeron-ui -> gpui_windows",
                    "allOtherLockFieldsAndEdgesUnchanged": True, "singleGpuiIdentity": GIT_SOURCE}


def run(args):
    task_root = Path(__file__).resolve().parents[3]
    mirror = checked_output_path(args.mirror_root, task_root)
    windows = checked_output_path(args.gpui_windows_root, task_root)
    evidence = checked_output_path(args.evidence_dir, task_root)
    source = Path(args.gpui_source).resolve(strict=True)
    require(mirror.is_dir() and windows.is_dir(), "Both supplied new mirrors must already exist")
    require(not evidence.exists(), "Evidence directory must be new; preserve previous attempts")
    for left, right in ((mirror, windows), (mirror, evidence), (windows, evidence)):
        require(not contains(left, right) and not contains(right, left), "Mirrors/evidence must be disjoint; GPUI must be a sibling outside the Zeron workspace")
    for directory in (mirror, windows):
        require(not (directory / ".git").exists(), f"Do not modify a Git checkout: {directory}")
        require(not contains(source, directory) and not contains(directory, source), "Source cache and writable mirror overlap")
    stamp = mirror / ".source-commit"
    require(stamp.is_file() and stamp.read_text(encoding="utf-8").strip() == REFERENCE_COMMIT, "New Zeron mirror must retain its frozen origin stamp")
    source_root = (source / "Cargo.toml").read_bytes()
    source_windows = (source / "crates/gpui_windows/Cargo.toml").read_bytes()
    require(sha256(source_root) == SOURCE_ROOT_MANIFEST_SHA, "Pinned GPUI workspace manifest differs; preserve it")
    require(sha256(source_windows) == SOURCE_WINDOWS_MANIFEST_SHA, "Pinned gpui_windows manifest differs; preserve it")
    targets = {"gpui_windows__Cargo.toml": windows / "Cargo.toml", "zeron__Cargo.toml": mirror / "Cargo.toml",
               "zeron__crates__ui__Cargo.toml": mirror / "crates/ui/Cargo.toml", "zeron__Cargo.lock": mirror / "Cargo.lock"}
    for path in targets.values():
        require(checked_output_path(str(path), task_root) == path and path.is_file(), f"Missing/unsafe target file: {path}")
    before = {name: path.read_bytes() for name, path in targets.items()}
    require(before["gpui_windows__Cargo.toml"] == source_windows, "Copied Windows manifest must be byte-identical before flattening")
    evidence.mkdir(parents=True, exist_ok=False)
    (evidence / "before").mkdir()
    for name, data in before.items():
        (evidence / "before" / name).write_bytes(data)
    try:
        flattened, bindings = flatten_windows(parse(source_windows), parse(source_root))
        new_lock, lock_checks = prepare_lock(before["zeron__Cargo.lock"])
        after = {"gpui_windows__Cargo.toml": manifest_bytes(flattened), "zeron__Cargo.toml": append_root_patch(before["zeron__Cargo.toml"], windows),
                 "zeron__crates__ui__Cargo.toml": add_ui_dependency(before["zeron__crates__ui__Cargo.toml"]), "zeron__Cargo.lock": new_lock}
        (evidence / "after").mkdir()
        records = []
        for name, path in targets.items():
            data = after[name]
            (evidence / "after" / name).write_bytes(data)
            diff = "".join(difflib.unified_diff(before[name].decode("utf-8-sig").splitlines(keepends=True), data.decode("utf-8-sig").splitlines(keepends=True),
                                             fromfile=f"before/{name}", tofile=f"after/{name}"))
            (evidence / (name + ".diff")).write_text(diff, encoding="utf-8", newline="")
            records.append({"path": str(path), "beforeSHA256": sha256(before[name]), "afterSHA256": sha256(data), "beforeBytes": len(before[name]), "afterBytes": len(data), "fullDiff": name + ".diff"})
        result = {"schemaVersion": 1, "stage": "R17", "status": "validated-plan-before-write", "helperSHA256": sha256(Path(__file__).read_bytes()),
                  "mirrorRoot": str(mirror), "gpuiWindowsRoot": str(windows), "gpuiSourceReadOnly": str(source), "referenceCommit": REFERENCE_COMMIT,
                  "pinnedGpuiRevision": REVISION, "gpuiWorkspaceManifestSHA256": sha256(source_root), "gpuiWindowsOriginalManifestSHA256": sha256(source_windows),
                  "allowedDelta": {"gpuiWindowsManifest": "Expand package/lints/workspace dependencies exactly; bind three path dependencies to original exact Git revision; add empty renderer-device-identity feature and standalone empty workspace",
                                   "zeronRootManifest": "Only add original Git source patch for gpui_windows -> supplied sibling local package; no workspace exclude needed",
                                   "zeronUiManifest": "Only add Windows dev-dependency gpui_windows at original exact Git revision, default-features=false and diagnostic feature",
                                   "zeronLock": "Only remove gpui_windows Git source and add sorted zeron-ui -> gpui_windows edge; preserve every version/checksum and all other lock data"},
                  "sameIdentityGitBindings": sorted(bindings), "lockValidation": lock_checks, "files": records,
                  "cargoExecuted": False, "versionResolutionExecuted": False, "nativeExecuted": False, "goExecuted": False,
                  "pendingValidation": "Separate Cargo --offline --locked must succeed. On failure preserve evidence and report; do not relax the lock or resolve new versions.",
                  "protectedScope": "No writes to previous exports, Git checkouts, global Cargo cache, vendor, reference source, or renderer source"}
        write_json(evidence / "result.json", result)
        for name, path in targets.items():
            require(path.read_bytes() == before[name], f"Concurrent target edit detected; no overwrite: {path}")
        for name, path in targets.items():
            path.write_bytes(after[name])
        for name, path in targets.items():
            require(path.read_bytes() == after[name], f"Post-write verification failed: {path}")
        require((source / "Cargo.toml").read_bytes() == source_root and (source / "crates/gpui_windows/Cargo.toml").read_bytes() == source_windows,
                "Read-only source manifest changed during preparation")
        result["status"] = "prepared-manual-lock-awaiting-cargo-offline-locked"
        write_json(evidence / "result.json", result)
        print(evidence / "result.json")
    except Exception as error:
        write_json(evidence / "failure.json", {"stage": "R17", "status": "failed-preserve-all-evidence", "error": f"{type(error).__name__}: {error}",
                                               "currentTargetSHA256": {name: sha256(path.read_bytes()) if path.is_file() else None for name, path in targets.items()},
                                               "cargoExecuted": False, "nativeExecuted": False, "goExecuted": False})
        raise


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--mirror-root", required=True)
    parser.add_argument("--gpui-windows-root", required=True)
    parser.add_argument("--gpui-source", required=True)
    parser.add_argument("--evidence-dir", required=True)
    args = parser.parse_args()
    try:
        run(args)
    except Exception as error:
        print(f"R17 manifest preparation failed: {type(error).__name__}: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
