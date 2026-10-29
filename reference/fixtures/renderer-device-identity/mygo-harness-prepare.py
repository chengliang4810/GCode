"""Prepare an isolated R17 harness source snapshot, without invoking Go/native.

Adds only a new command and alternate module files to a bounded app copy.
Existing copied production source, ordinary module files and pin stay exact.
"""
import argparse
import ast
import hashlib
import importlib.util
import json
from pathlib import Path
import sys

HERE = Path(__file__).resolve().parent
sys.dont_write_bytecode = True
SPEC = importlib.util.spec_from_file_location("mygo_prepare_v2", HERE / "mygo-prepare-v2.py")
PREP = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(PREP)
ROOT = HERE.parents[2]
APP_TREES = ["internal", "cmd", "resources", "scripts/frameworkpin"]
APP_FILES = ["go.mod", "go.sum", "mygo.json", "AGENTS.md", "third_party/mygo.lock.json"]
RANGES = [(168, 195, "freshOutput"), (260, 272, "windowMetrics"), (423, 428, "validateCapturedState"),
          (430, 450, "measureNativeWindow"), (452, 517, "settledState"), (519, 533, "resultSnapshot-and-pause")]


def data_inventory(values):
    return sorted([dict(path=key, bytes=len(data), sha256=PREP.digest(data)) for key, data in values.items()], key=lambda row: row["path"])


def app_bytes(root, selection=None):
    files = {}
    if selection is not None:
        for row in selection:
            relative = row["path"]
            PREP.require(relative in APP_FILES or any(relative.startswith(name+"/") for name in APP_TREES), "Selected app path is outside bounded roots")
            PREP.require(not relative.startswith("cmd/renderer-device-r17/"), "Baseline must exclude the diagnostic command")
            path = PREP.checked_path(root / relative)
            PREP.require(path.is_relative_to(root) and relative not in files, "Duplicate or escaped app path")
            data = path.read_bytes()
            PREP.require(len(data) == row["bytes"] and PREP.digest(data) == row["sha256"], "Selected baseline bytes differ")
            files[relative] = data
        return files
    for relative in APP_FILES:
        path = PREP.checked_path(root / relative)
        files[relative] = path.read_bytes()
    for relative in APP_TREES:
        base = PREP.checked_path(root / relative, directory=True)
        for row in PREP.inventory(base):
            path = base / row["path"]
            files[relative + "/" + row["path"]] = path.read_bytes()
    return files


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--app-source-root", required=True)
    parser.add_argument("--attempt-root", required=True)
    parser.add_argument("--expected-variant-tree", required=True, type=PREP.sha_arg)
    parser.add_argument("--expected-base-tree", required=True, type=PREP.sha_arg)
    parser.add_argument("--expected-base-count", required=True, type=int)
    parser.add_argument("--main-template", required=True)
    parser.add_argument("--expected-main-template-sha256", required=True, type=PREP.sha_arg)
    parser.add_argument("--app-baseline-manifest")
    parser.add_argument("--expected-app-baseline-manifest-sha256", type=PREP.sha_arg)
    parser.add_argument("--snapshot-name", default="app")
    parser.add_argument("--evidence-name", default="harness")
    args = parser.parse_args()
    for name, prefix in [(args.snapshot_name, "app"), (args.evidence_name, "harness")]:
        PREP.require(name.startswith(prefix) and all(c in "abcdefghijklmnopqrstuvwxyz0123456789-" for c in name), "Invalid fresh snapshot name")
    helper_bytes = Path(__file__).read_bytes()
    prepare_bytes = (HERE / "mygo-prepare-v2.py").read_bytes()
    source = PREP.checked_path(args.app_source_root, directory=True)
    attempt = PREP.checked_path(args.attempt_root, directory=True)
    PREP.require(attempt.parent == (ROOT / "output").resolve() and attempt.name.startswith("renderer-device-r17-mygo-attempt"), "Unexpected attempt root")
    mirror = PREP.checked_path(attempt / "_source/mygo", directory=True)
    proof_path = PREP.checked_path(attempt / "evidence/result.json")
    proof = json.loads(proof_path.read_bytes())
    PREP.require(proof["baseTreeSHA256"] == args.expected_base_tree and proof["baseFileCount"] == args.expected_base_count
                 and proof["mirrorTreeSHA256"] == args.expected_variant_tree and proof["originalBaseFilesByteIdentical"], "Wrong prepared variant")
    variant_before = PREP.inventory(mirror)
    PREP.require(PREP.tree_hash(variant_before) == args.expected_variant_tree and len(variant_before) == args.expected_base_count + 2, "Variant bytes changed")
    main_path = PREP.checked_path(args.main_template)
    main_bytes = main_path.read_bytes()
    PREP.require(PREP.digest(main_bytes) == args.expected_main_template_sha256, "Main template SHA differs")
    selection = None
    selection_identity = None
    PREP.require(bool(args.app_baseline_manifest) == bool(args.expected_app_baseline_manifest_sha256), "Baseline manifest path/SHA must be supplied together")
    if args.app_baseline_manifest:
        selected_path = PREP.checked_path(args.app_baseline_manifest)
        selected_bytes = selected_path.read_bytes()
        PREP.require(PREP.digest(selected_bytes) == args.expected_app_baseline_manifest_sha256, "Baseline manifest SHA differs")
        selection = json.loads(selected_bytes)
        selection_identity = dict(path=str(selected_path), sha256=PREP.digest(selected_bytes))
    original = app_bytes(source, selection)
    base_rows = data_inventory(original)
    app = attempt / "_source" / args.snapshot_name
    evidence = attempt / "evidence" / args.evidence_name
    PREP.require(not app.exists() and not evidence.exists(), "Harness snapshot cannot be overwritten")
    PREP.require(not any(key.startswith("cmd/renderer-device-r17/") for key in original), "New diagnostic command already exists")
    app.mkdir()
    evidence.mkdir()
    result = dict(schemaVersion=1, status="preparing-harness-source-only", goExecuted=False, gofmtExecuted=False,
                  buildExecuted=False, nativeExecuted=False, originalAppSource=str(source), appRoot=str(app),
                  baseTreeSHA256=args.expected_base_tree, baseFileCount=args.expected_base_count,
                  variantTreeSHA256=args.expected_variant_tree, helperSHA256=PREP.digest(Path(__file__).read_bytes()),
                  originalProductionBodiesChanged=False, formalRootWritten=False)
    try:
        for relative, data in original.items():
            target = app / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            with target.open("xb") as stream:
                stream.write(data)
        PREP.require(PREP.inventory(app) == base_rows, "App snapshot differs")
        copied = []
        cmd = app / "cmd/renderer-device-r17"
        cmd.mkdir()
        (cmd / "main_windows.go").write_bytes(main_bytes)
        for name in ["capture_windows.go", "metrics_windows.go"]:
            relative = "cmd/tabstrip-fixture/" + name
            data = original[relative]
            (cmd / name).write_bytes(data)
            copied.append(dict(source=relative, sourceSHA256=PREP.digest(data), target="cmd/renderer-device-r17/"+name,
                               delta="none: exact whole-file bytes, existing package main preserved"))
        chunks, extracts = [], []
        old_main = original["cmd/tabstrip-fixture/main.go"]
        old_lines = old_main.splitlines(keepends=True)
        for first, last, name in RANGES:
            data = b"".join(old_lines[first-1:last])
            PREP.require(name == "resultSnapshot-and-pause" or name.encode() in data, "Source extraction moved; review before changing ranges")
            chunks.append(data)
            extracts.append(dict(source="cmd/tabstrip-fixture/main.go", sourceSHA256=PREP.digest(old_main),
                                 firstLine=first, lastLine=last, name=name, excerptSHA256=PREP.digest(data),
                                 delta="none inside excerpt; only new file header/imports and inter-excerpt blank lines"))
        header = b'''//go:build windows && (amd64 || arm64) && uifixture && mygo_renderer_device_diag

package main

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "math"
    "os"
    "path/filepath"
    "reflect"
    "time"

    "gcode/internal/workbench"
    "github.com/egoist/mygo"
)

'''
        (cmd / "fixture_helpers_windows.go").write_bytes(header + b"\n".join(chunks))
        module = original["go.mod"]
        old_replace = b"replace github.com/egoist/mygo => ./third_party/mygo"
        PREP.require(module.count(old_replace) == 1, "Expected exact canonical replacement line")
        replacement = ('replace github.com/egoist/mygo => "' + mirror.as_posix() + '"').encode()
        alternate = module.replace(old_replace, replacement)
        diag = app / ".diagnostics"
        diag.mkdir()
        (diag / "renderer-device-r17.mod").write_bytes(alternate)
        (diag / "renderer-device-r17.sum").write_bytes(original["go.sum"])
        after = PREP.inventory(app)
        before_paths = set(original)
        originals_after = [row for row in after if row["path"] in before_paths]
        PREP.require(originals_after == base_rows, "Copied production file changed")
        additions = [row for row in after if row["path"] not in before_paths]
        want = {"cmd/renderer-device-r17/"+name for name in ["main_windows.go", "capture_windows.go", "metrics_windows.go", "fixture_helpers_windows.go"]}
        want |= {".diagnostics/renderer-device-r17.mod", ".diagnostics/renderer-device-r17.sum"}
        PREP.require({row["path"] for row in additions} == want, "Unexpected app additions")
        PREP.require(app_bytes(source, selection) == original, "App source changed during bounded snapshot; preserve this attempt and retry explicitly")
        PREP.require(PREP.inventory(mirror) == variant_before, "Variant changed during snapshot")
        PREP.require(main_path.read_bytes() == main_bytes and Path(__file__).read_bytes() == helper_bytes and (HERE / "mygo-prepare-v2.py").read_bytes() == prepare_bytes, "Preparation template/helper changed during snapshot")
        snapshots = evidence / "templates"
        snapshots.mkdir()
        for name, data in [(Path(__file__).name, helper_bytes), ("mygo-prepare-v2.py", prepare_bytes), (main_path.name, main_bytes)]:
            (snapshots / name).write_bytes(data)
        PREP.save(evidence / "app-base-files.json", base_rows)
        PREP.save(evidence / "app-files.json", after)
        PREP.save(evidence / "transport-provenance.json", dict(wholeFileCopies=copied, unchangedExcerpts=extracts, helperHeaderSHA256=PREP.digest(header)))
        manifest = dict(schemaVersion=1, kind="r17-mygo-diagnostic-source-variant",
                        app=dict(root=str(app), treeSHA256=PREP.tree_hash(after), fileCount=len(after)),
                        mygo=dict(root=str(mirror), treeSHA256=args.expected_variant_tree, fileCount=len(variant_before)),
                        baseTreeSHA256=args.expected_base_tree, baseFileCount=args.expected_base_count,
                        baseAcceptance="candidate02 explicitly authorized for diagnostics, not final acceptance",
                        boundedAppRoots=APP_TREES, boundedAppFiles=APP_FILES,
                        selectedBaselineManifest=selection_identity,
                        excludedTopLevel=[".git", "output", "third_party/mygo", "reference", ".agents", "tools", "docs"],
                        wholeFileCopies=copied, unchangedExcerpts=extracts, appAdditions=additions,
                        frameworkPreparationResultSHA256=PREP.digest(proof_path.read_bytes()),
                        helperSHA256=PREP.digest(Path(__file__).read_bytes()), mainTemplateSHA256=PREP.digest(main_bytes),
                        diagnosticBinary=True, canonicalRelease=False, oldR16BinaryIdentityProven=False,
                        capturedFrameProducerProven=False, runtimeSHAIsInheritedEmptyDevelopmentValue=True)
        manifest_path = evidence / "source-manifest.json"
        PREP.save(manifest_path, manifest)
        stamp = PREP.digest(manifest_path.read_bytes())
        build_manifest = dict(schemaVersion=1, diagnosticBinary=True, sourceManifestPath=str(manifest_path),
                              sourceManifestSHA256=stamp, binarySHA256="PENDING_ROOT_BUILD_SHA256",
                              mainPackage="gcode/cmd/renderer-device-r17", module="gcode", toolchain="go1.27.3", mygoReplacement=str(mirror))
        PREP.save(evidence / "build-manifest.pending.json", build_manifest)
        result.update(status="source-prepared-static-only-build-and-native-pending", appFileCount=len(after), appTreeSHA256=PREP.tree_hash(after),
                      copiedAppOriginalFileCount=len(base_rows), copiedAppOriginalsByteIdentical=True,
                      liveAppSourceUnchangedDuringSnapshot=True, frameworkVariantUnchanged=True, appAdditions=additions,
                      sourceManifest=str(manifest_path), sourceManifestSHA256=stamp,
                      canonicalGoModAndSumByteIdentical=True, canonicalPinAndLockByteIdentical=True,
                      boundedSnapshotIsStandaloneReleaseTree=False,
                      buildWorkingDirectory=str(app), buildTarget="./cmd/renderer-device-r17",
                      buildArguments=["build", "-mod=readonly", "-modfile=.diagnostics/renderer-device-r17.mod", "-trimpath",
                                      "-tags=uifixture,mygo_renderer_device_diag", "-ldflags=-X main.diagnosticSourceStamp="+stamp,
                                      "-o", str(attempt / "build/renderer-device-r17.exe"), "./cmd/renderer-device-r17"],
                      buildEnvironment=dict(GOWORK="off", CGO_ENABLED="0", GOOS="windows", GOARCH="amd64", GOTOOLCHAIN="local", GOFLAGS=""),
                      pending=["Go 1.27.3 toolchain owner verification", "gofmt (not run)", "Go compiler/type checking (not run)",
                               "actual resolved module graph proof", "root build/exe seal and finalized build-manifest", "native View/WGC evidence"],
                      evidenceScope="New harness binary and explicit diagnostic variant; not production release or old R16 binary identity")
        PREP.save(evidence / "result.json", result)
        print(evidence / "result.json")
        return 0
    except Exception as error:
        result.update(status="failed-preserve-evidence", error=f"{type(error).__name__}: {error}")
        PREP.save(evidence / "result.json", result)
        print(result["error"], file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
