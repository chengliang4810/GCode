"""Capture the native project tree and layout in a bounded window matrix."""

import argparse
import hashlib
import json
import os
import pathlib
import subprocess
import time


root = pathlib.Path(__file__).resolve().parents[3]
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--output", default="output/acceptance/visual-v4/round-1")
options = parser.parse_args()
out = root / options.output
binary = out / "gcode-visualqa"
out.mkdir(parents=True, exist_ok=False)


def run(*args):
    return subprocess.check_output(args, cwd=root, text=True).strip()


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def sources():
    paths = [root / "go.mod", root / "go.sum", root / "cmd/gcode/main.go"]
    paths += sorted((root / "internal/desktop").glob("*.go"))
    for directory in ("internal/desktop/fonts", "resources"):
        paths += sorted(path for path in (root / directory).rglob("*") if path.is_file())
    paths += [root / "output/acceptance/visual-v2/_nativeqa/main.go", pathlib.Path(__file__).resolve()]
    return {str(path.relative_to(root)): digest(path) for path in paths}


def ax(body):
    script = 'tell application "System Events" to tell (first application process whose unix id is ' + str(app.pid) + ')\n' + body + '\nend tell'
    return run("osascript", "-e", script)


def wait_for(predicate, label, timeout=60):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if predicate():
            return
        time.sleep(0.2)
    raise AssertionError(label)


def window_id(pid):
    return run("swift", "-e", '''import CoreGraphics
let rows = CGWindowListCopyWindowInfo(.optionOnScreenOnly, kCGNullWindowID) as? [[String: Any]] ?? []
for row in rows {
    if (row[kCGWindowOwnerPID as String] as? Int ?? 0) == PID_VALUE,
       (row[kCGWindowLayer as String] as? Int ?? -1) == 0 {
        print(row[kCGWindowNumber as String] ?? "")
        break
    }
}'''.replace("PID_VALUE", str(pid)))


before = sources()
subprocess.run(["go", "build", "-buildvcs=false", "-o", str(binary), "./output/acceptance/visual-v2/_nativeqa"], cwd=root, check=True)
assert before == sources(), "source changed during QA build"
manifest = {
    "source_files": before,
    "source_fingerprint": hashlib.sha256(json.dumps(before, sort_keys=True).encode()).hexdigest(),
    "binary_sha256": digest(binary),
    "binary_path": str(binary.relative_to(root)),
    "go": run("go", "version"),
    "os": run("sw_vers"),
    "content": "明确标记的示例数据，复用生产 Go + mygo 视图",
}
(out / "build-manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2))

env = os.environ.copy()
env.update(GCODE_VISUAL_QA_OUTPUT=str(out), GCODE_VISUAL_QA_HOLD="1")
with (out / "native-qa.log").open("w") as log:
    app = subprocess.Popen([str(binary)], cwd=root, env=env, stdout=log, stderr=log)
    try:
        wait_for(lambda: (out / "light-1024x700-native.png").exists(), "native capture matrix did not finish")
        wait_for(lambda: "captures complete" in (out / "native-qa.log").read_text(), "QA window was not held")
        wid = window_id(app.pid)
        manifest.update(pid=app.pid, cgwindow_id=wid)
        ax("set frontmost to true")
        ax('click button "设置" of window 1')
        ax('click radio button "深色" of radio group 1 of scroll area 1 of window 1')
        ax('click button "返回工作台" of window 1')
        time.sleep(0.5)
        (out / "tree-expanded-ax.txt").write_text(ax("return entire contents of window 1"))

        def tree_element(label, press=False):
            action = 'perform action "AXPress" of itemRef' if press else ''
            return ax('set allItems to entire contents of window 1\n'
                      'repeat with itemRef in allItems\n'
                      'try\nif name of itemRef is ' + json.dumps(label, ensure_ascii=False) +
                      ' or description of itemRef is ' + json.dumps(label, ensure_ascii=False) +
                      ' then\n' + action + '\nreturn role of itemRef as text\nend if\nend try\nend repeat\nreturn "MISSING"')

        assert tree_element("会话 整理工作区文档") != "MISSING"
        disclosure = tree_element("项目 GCode", press=True)
        assert disclosure != "MISSING"
        time.sleep(0.4)
        assert tree_element("会话 整理工作区文档") == "MISSING"
        assert tree_element("会话 重排知识库索引") != "MISSING"
        (out / "tree-collapsed-ax.txt").write_text(ax("return entire contents of window 1"))
        subprocess.run(["screencapture", "-x", "-o", "-l", wid, str(out / "tree-collapsed-native.png")], check=True)
        assert tree_element("项目 GCode", press=True) != "MISSING"
        time.sleep(0.4)
        assert tree_element("会话 整理工作区文档") != "MISSING"
        subprocess.run(["screencapture", "-x", "-o", "-l", wid, str(out / "tree-expanded-native.png")], check=True)
        assert tree_element("会话 重排知识库索引", press=True) != "MISSING"
        time.sleep(0.4)
        # The conversation header now belongs to Atlas, rather than the first project.
        texts = ax("return value of every static text of window 1")
        assert "重排知识库索引" in texts and "Atlas Notes" in texts, texts
        (out / "tree-selected-ax.txt").write_text(ax("return entire contents of window 1"))
        manifest["tree_checks"] = {"project_role": disclosure,
                                  "collapse_only_own_children": "PASS",
                                  "reopen_children": "PASS",
                                  "select_other_project_conversation": "PASS",
                                  "sample_service_dispatch": "NOT_RUN",
                                  "physical_keyboard_ime": "NOT_RUN"}
        manifest["source_unchanged_after_capture"] = before == sources()
        assert manifest["source_unchanged_after_capture"], "view source changed during capture"
        (out / "build-manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2))
        captures = {p.name: {"sha256": digest(p), "bytes": p.stat().st_size} for p in sorted(out.glob("*-native.png"))}
        (out / "captures.json").write_text(json.dumps(captures, ensure_ascii=False, indent=2))
        print(json.dumps({"pid": app.pid, "cgwindow_id": wid, "captures": list(captures)}, ensure_ascii=False), flush=True)
        # Close this fresh QA window normally, then verify the exact PID exits.
        ax('perform action "AXPress" of (value of attribute "AXCloseButton" of window 1)')
        app.wait(timeout=10)
        assert app.returncode == 0, app.returncode
        manifest["native_qa_exit_code"] = app.returncode
        (out / "build-manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2))
    finally:
        if app.poll() is None:
            app.terminate()
            app.wait(timeout=10)
