"""Copy the sealed .7 bytes and add exactly two opt-in diagnostic Go files.

This helper never invokes Go, gofmt, Cargo or a native program. The copied
framework keeps its original go.mod/go.sum and every original source byte.
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
TEMPLATES = Path(__file__).resolve().parent
SOURCE = ROOT / "output/framework-point-r17/_source/mygo-after"
EXPECTED_TREE = "8a61910bf99d813f9e82bd408d343bead33dbd8a22b0524634644eef191c0321"
EXPECTED_COUNT = 1840
TAG = "windows && (amd64 || arm64) && mygo_renderer_device_diag"
ADDITIONS = {
    "ui/renderer_device_identity_windows.go": "mygo-ui-device-identity_windows.go.txt",
    "internal/gpu/d3d11/renderer_device_identity_windows.go": "mygo-d3d11-device-identity_windows.go.txt",
}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def save(path, value):
    path.write_text(json.dumps(value, indent=2, ensure_ascii=False, allow_nan=False) + "\n", encoding="utf-8")


def regular(path):
    info = path.lstat()
    require(not stat.S_ISLNK(info.st_mode) and not (getattr(info, "st_file_attributes", 0) & 0x400), f"Reparse point is not allowed: {path}")
    return info


def inventory(root):
    regular(root)
    rows = []
    for path in sorted(root.rglob("*")):
        info = regular(path)
        if stat.S_ISDIR(info.st_mode):
            continue
        require(stat.S_ISREG(info.st_mode), f"Nonregular source file: {path}")
        data = path.read_bytes()
        rows.append({"path": path.relative_to(root).as_posix(), "bytes": len(data), "sha256": digest(data)})
    return sorted(rows, key=lambda row: row["path"])


def tree_hash(rows):
    # Exact scripts/frameworkpin.TreeHash algorithm: sorted relative UTF-8 path,
    # NUL, lowercase content digest, newline. It is not a raw-content hash.
    value = hashlib.sha256()
    for row in sorted(rows, key=lambda row: row["path"]):
        value.update((row["path"] + "\0" + row["sha256"] + "\n").encode("utf-8"))
    return value.hexdigest()


def new_output(value):
    raw = Path(os.path.abspath(value))
    for ancestor in (raw, *raw.parents):
        if ancestor.exists() or ancestor.is_symlink():
            regular(ancestor)
    out = raw.resolve()
    require(out.parent == (ROOT / "output").resolve(), "Attempt must be directly beneath this checkout/output")
    require(out.name.startswith("renderer-device-r17-mygo-attempt"), "Use a new renderer-device-r17-mygo-attempt* directory")
    require(not out.exists(), "Preserve previous attempts; output must be new")
    return out


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out-dir", required=True)
    args = parser.parse_args()
    out = new_output(args.out_dir)
    before = inventory(SOURCE)
    require(len(before) == EXPECTED_COUNT and tree_hash(before) == EXPECTED_TREE, "Canonical actual-after .7 source identity differs")
    lock_path = ROOT / "third_party/mygo.lock.json"
    lock_bytes = lock_path.read_bytes()
    lock = json.loads(lock_bytes)
    require(lock["revision"] == "zeron-native-ui.7" and lock["treeSha256"] == EXPECTED_TREE and lock["fileCount"] == EXPECTED_COUNT and len(lock["patches"]) == 12,
            "Canonical patch lock is not the approved .7/1840/12 baseline")
    patch_rows = []
    for patch in lock["patches"]:
        path = ROOT / patch["path"]
        require(digest(path.read_bytes()) == patch["sha256"], f"Canonical patch differs: {path}")
        patch_rows.append({"path": patch["path"], "sha256": patch["sha256"]})
    template_bytes = {path: (TEMPLATES / template).read_bytes() for path, template in ADDITIONS.items()}
    for path, data in template_bytes.items():
        require(data.startswith(("//go:build " + TAG + "\n\n").encode()), f"Missing exact opt-in build constraint: {path}")
        require(not (SOURCE / path).exists(), f"New diagnostic path already exists in canonical source: {path}")
    out.mkdir()
    evidence = out / "evidence"
    evidence.mkdir()
    result = {"schemaVersion": 1, "stage": "R17", "status": "preparing-opt-in-mygo-mirror", "canonicalSource": str(SOURCE),
              "canonicalRevision": "zeron-native-ui.7", "canonicalTreeSHA256": EXPECTED_TREE, "canonicalFileCount": EXPECTED_COUNT,
              "canonicalPatchCount": 12, "canonicalLockSHA256": digest(lock_bytes), "canonicalPatches": patch_rows,
              "helperSHA256": digest(Path(__file__).read_bytes()), "goExecuted": False, "gofmtExecuted": False, "buildExecuted": False, "nativeExecuted": False}
    save(evidence / "canonical-before.json", before)
    (evidence / "mygo.lock.before.json").write_bytes(lock_bytes)
    try:
        mirror = out / "_source/mygo"
        mirror.parent.mkdir()
        shutil.copytree(SOURCE, mirror)
        require(inventory(mirror) == before, "Full mirror copy differs from the original 1840 files")
        additions = []
        snapshots = evidence / "templates"
        snapshots.mkdir()
        for relative, template in ADDITIONS.items():
            data = template_bytes[relative]
            target = mirror / relative
            with target.open("xb") as stream:
                stream.write(data)
            (snapshots / template).write_bytes(data)
            additions.append({"path": relative, "template": template, "bytes": len(data), "sha256": digest(data), "buildConstraint": TAG})
        (snapshots / "mygo-prepare.py").write_bytes(Path(__file__).read_bytes())
        after = inventory(mirror)
        originals = [row for row in after if row["path"] not in ADDITIONS]
        require(originals == before, "An original canonical file was changed/removed")
        require(len(after) == EXPECTED_COUNT + 2 and {row["path"] for row in after} - {row["path"] for row in before} == set(ADDITIONS),
                "The mirror must add exactly the two approved Go files")
        require(inventory(SOURCE) == before and lock_path.read_bytes() == lock_bytes, "Read-only canonical input changed during copy")
        save(evidence / "canonical-after.json", inventory(SOURCE))
        save(evidence / "diagnostic-mirror-files.json", after)
        result.update(status="source-prepared-static-only-not-formatted-or-compiled", mirrorRoot=str(mirror), mirrorFileCount=len(after),
                      mirrorTreeSHA256=tree_hash(after), additions=additions, original1840FilesByteIdentical=True,
                      onlyAddedPaths=list(ADDITIONS), originalFilesChanged=[], originalFilesRemoved=[],
                      canonicalSourceBeforeAfterIdentical=True, canonicalLockUnchanged=True, manifestsAndGoSumsUnchanged=True,
                      parentVendorWritten=False, originalMethodsChanged=False, appFixtureOrHarnessAdded=False,
                      moduleBoundary="The complete mirror retains its original go.mod; no diagnostic .go files are placed in the parent repository package tree",
                      publicAPI="ui.RendererDeviceIdentity(c *ui.Context) (map[string]any, error), current Content view/UI thread only",
                      readiness="The first View may run before GPU creation. A missing/fallback/closed renderer returns failed-or-pending without creating a substitute. Retry only from a later current View.",
                      lifetime="The returned plain map may be retained. Never retain Context/Element or run this query on a worker. COM AddRef/Release references are local to the query.",
                      identityScope="hwnd is the actual renderer surface child; ownerWindowHWND is the live top-level window from its existing host. Address tokens are snapshots, not continuity proofs.",
                      unavailableFields=["separately stored selected adapter: Renderer stores none", "GPUI-style overlay plane: not represented by this renderer"],
                      pending=["gofmt", "Go parsing/compilation with mygo_renderer_device_diag", "root-owned fixture/harness integration", "real owner-thread renderer query", "actual LUID/feature-level/driver output", "native evidence and visual acceptance"],
                      evidenceScope="New diagnostic mirror of approved .7 only; not a production renderer change, default binary-size claim, old-binary identity proof or visual parity result")
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
