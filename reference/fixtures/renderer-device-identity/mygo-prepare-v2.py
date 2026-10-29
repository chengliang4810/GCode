"""Prepare an explicitly identified MyGo base plus two opt-in diagnostic files.

Pure Python only. No Go/gofmt/Cargo/native execution. The historical helper and
attempt01 evidence remain unchanged; no old source/tree/count defaults exist.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import stat
import sys

ROOT = Path(__file__).resolve().parents[3]
TAG = "windows && (amd64 || arm64) && mygo_renderer_device_diag"
ADDITIONS = {"ui/renderer_device_identity_windows.go": "ui_template",
             "internal/gpu/d3d11/renderer_device_identity_windows.go": "d3d11_template"}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def save(path, value):
    with path.open("x", encoding="utf-8", newline="\n") as stream:
        json.dump(value, stream, indent=2, ensure_ascii=False, allow_nan=False)
        stream.write("\n")


def regular(path):
    info = path.lstat()
    require(not stat.S_ISLNK(info.st_mode) and not (getattr(info, "st_file_attributes", 0) & 0x400), f"Reparse point: {path}")
    return info


def checked_path(value, directory=False):
    path = Path(os.path.abspath(value))
    for ancestor in (path, *path.parents):
        if ancestor.exists() or ancestor.is_symlink():
            regular(ancestor)
    info = regular(path)
    require(stat.S_ISDIR(info.st_mode) if directory else stat.S_ISREG(info.st_mode), f"Wrong input type: {path}")
    return path.resolve()


def inventory(root):
    regular(root)
    rows = []
    for path in sorted(root.rglob("*")):
        info = regular(path)
        if stat.S_ISDIR(info.st_mode):
            continue
        require(stat.S_ISREG(info.st_mode), f"Nonregular source: {path}")
        data = path.read_bytes()
        rows.append({"path": path.relative_to(root).as_posix(), "bytes": len(data), "sha256": digest(data)})
    return sorted(rows, key=lambda row: row["path"])


def tree_hash(rows):
    value = hashlib.sha256()
    for row in sorted(rows, key=lambda row: row["path"]):
        value.update((row["path"] + "\0" + row["sha256"] + "\n").encode("utf-8"))
    return value.hexdigest()


def new_output(value, source):
    raw = Path(os.path.abspath(value))
    for ancestor in (raw, *raw.parents):
        if ancestor.exists() or ancestor.is_symlink():
            regular(ancestor)
    out = raw.resolve()
    require(out.parent == (ROOT / "output").resolve(), "Attempt must be directly beneath checkout/output")
    require(out.name.startswith("renderer-device-r17-mygo-attempt"), "Use a new renderer-device-r17-mygo-attempt* directory")
    require(not out.exists(), "Previous attempts cannot be overwritten")
    require(not out.is_relative_to(source) and not source.is_relative_to(out), "Input/output must be disjoint")
    return out


def sha_arg(value):
    if len(value) != 64 or any(c not in "0123456789abcdef" for c in value):
        raise argparse.ArgumentTypeError("Expected 64 lowercase SHA256 hex characters")
    return value


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ["source-root", "patch-manifest", "patch-root", "expected-revision", "ui-template", "d3d11-template", "out-dir"]:
        parser.add_argument("--" + name, required=True)
    for name in ["expected-tree", "expected-patch-manifest-sha256", "expected-ui-template-sha256", "expected-d3d11-template-sha256"]:
        parser.add_argument("--" + name, required=True, type=sha_arg)
    parser.add_argument("--expected-count", required=True, type=int)
    args = parser.parse_args()
    require(args.expected_count > 0, "Expected count must be positive")
    source = checked_path(args.source_root, directory=True)
    patch_root = checked_path(args.patch_root, directory=True)
    manifest_path = checked_path(args.patch_manifest)
    out = new_output(args.out_dir, source)
    before = inventory(source)
    require(len(before) == args.expected_count and tree_hash(before) == args.expected_tree, "Explicit source identity differs")
    manifest_bytes = manifest_path.read_bytes()
    require(digest(manifest_bytes) == args.expected_patch_manifest_sha256, "Explicit manifest SHA differs")
    lock = json.loads(manifest_bytes)
    require(lock["schemaVersion"] == 1 and lock["module"] == "github.com/egoist/mygo" and lock["upstreamVersion"] == "v0.6.4"
            and lock["upstreamSum"] == "h1:VzCqZKgvXcym5cSYKoC6IkTgcVeGZquPiui/a3oE36o="
            and lock["revision"] == args.expected_revision and lock["treeSha256"] == args.expected_tree
            and lock["fileCount"] == args.expected_count, "Manifest does not identify the explicitly selected base")
    require(isinstance(lock["patches"], list) and len(lock["patches"]) > 0, "Missing ordered patches")
    patch_inputs, seen = [], set()
    for patch in lock["patches"]:
        relative = Path(patch["path"])
        require(not relative.is_absolute() and ".." not in relative.parts and ":" not in patch["path"], "Patch escapes patch-root")
        path = checked_path(patch_root / relative)
        require(path.is_relative_to(patch_root) and patch["path"] not in seen, "Duplicate/escaped patch path")
        seen.add(patch["path"])
        data = path.read_bytes()
        require(digest(data) == patch["sha256"], f"Patch differs: {path}")
        patch_inputs.append((path, data, dict(patch)))
    templates = {}
    for relative, option in ADDITIONS.items():
        path = checked_path(getattr(args, option))
        data = path.read_bytes()
        require(digest(data) == getattr(args, "expected_" + option + "_sha256"), f"Template differs: {path}")
        require(data.startswith(("//go:build " + TAG + "\n\n").encode()), f"Missing exact build constraint: {path}")
        require(not (source / relative).exists(), f"Addition exists in base: {relative}")
        templates[relative] = (path, data)
    helper_bytes = Path(__file__).read_bytes()
    out.mkdir()
    evidence = out / "evidence"
    evidence.mkdir()
    result = dict(schemaVersion=2, stage="R17", status="preparing-explicit-base-diagnostic-variant",
                  baseSource=str(source), baseRevision=args.expected_revision, baseTreeSHA256=args.expected_tree,
                  baseFileCount=args.expected_count, basePatchCount=len(patch_inputs),
                  baseAcceptance="Explicit candidate; preparation does not grant final acceptance",
                  patchManifest=str(manifest_path), patchManifestSHA256=digest(manifest_bytes),
                  helperSHA256=digest(helper_bytes), arguments=vars(args),
                  goExecuted=False, gofmtExecuted=False, buildExecuted=False, nativeExecuted=False)
    try:
        save(evidence / "base-before.json", before)
        (evidence / "mygo.lock.input.json").write_bytes(manifest_bytes)
        snapshots = evidence / "templates"
        snapshots.mkdir()
        (snapshots / "mygo-prepare-v2.py").write_bytes(helper_bytes)
        for _, data, patch in patch_inputs:
            target = evidence / "patches" / patch["path"]
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(data)
        mirror = out / "_source/mygo"
        mirror.parent.mkdir()
        shutil.copytree(source, mirror)
        require(inventory(mirror) == before, "Full copy differs")
        additions = []
        for relative, (path, data) in templates.items():
            with (mirror / relative).open("xb") as stream:
                stream.write(data)
            (snapshots / path.name).write_bytes(data)
            additions.append(dict(path=relative, template=str(path), bytes=len(data), sha256=digest(data), buildConstraint=TAG))
        after = inventory(mirror)
        require([row for row in after if row["path"] not in ADDITIONS] == before, "An original base file changed/disappeared")
        require(len(after) == args.expected_count + 2 and {row["path"] for row in after} - {row["path"] for row in before} == set(ADDITIONS), "Expected exactly two additions")
        require(inventory(source) == before and manifest_path.read_bytes() == manifest_bytes, "Base/manifest changed during copy")
        require(all(path.read_bytes() == data for path, data, _ in patch_inputs), "Patch input changed")
        require(all(path.read_bytes() == data for path, data in templates.values()), "Template changed")
        require(Path(__file__).read_bytes() == helper_bytes, "Helper changed")
        save(evidence / "base-after.json", inventory(source))
        save(evidence / "diagnostic-mirror-files.json", after)
        result.update(status="source-prepared-static-only-not-formatted-or-compiled", mirrorRoot=str(mirror), mirrorFileCount=len(after),
                      mirrorTreeSHA256=tree_hash(after), additions=additions, originalBaseFilesByteIdentical=True,
                      originalFilesChanged=[], originalFilesRemoved=[], onlyAddedPaths=list(ADDITIONS),
                      baseSourceBeforeAfterIdentical=True, manifestAndPatchInputsUnchanged=True, manifestsAndGoSumsUnchanged=True,
                      parentVendorWritten=False, originalMethodsChanged=False, appFixtureOrHarnessAdded=False,
                      publicAPI="ui.RendererDeviceIdentity(c *ui.Context) (map[string]any, error); current View/UI thread only",
                      lifetime="Retain only plain owned data; never Context/Element. COM references remain local.",
                      readiness="First View may precede GPU creation; retry a later current View, never a stale map.",
                      evidenceScope="New diagnostic variant; not canonical release, old R16 identity, PNG producer proof or visual parity",
                      pending=["gofmt", "Go parsing/compilation", "module resolution", "harness integration", "actual native identity/WGC"])
        save(evidence / "result.json", result)
        print(evidence / "result.json")
        return 0
    except Exception as error:
        result.update(status="failed-preserve-evidence", error=f"{type(error).__name__}: {error}")
        save(evidence / "result.json", result)
        print(result["error"], file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
