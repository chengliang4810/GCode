"""Exercise the packaged native workspace on a unique, isolated Git fixture."""

import argparse
import hashlib
import json
import os
import pathlib
import plistlib
import shutil
import struct
import subprocess
import time


root = pathlib.Path(__file__).resolve().parents[3]
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--bundle", default="build/darwin-amd64/GCode.app")
parser.add_argument("--output", default="output/acceptance/native-workspace/round-1")
options = parser.parse_args()
source = root / options.bundle
out = root / options.output
out.mkdir(parents=True, exist_ok=True)
qa_name = "GCodeWorkspaceQA" + str(time.time_ns())
bundle = out / (qa_name + ".app")
data = pathlib.Path.home() / "Library/Application Support" / qa_name
project = out / ("project-" + qa_name)
project.mkdir()
project = project.resolve()
results = {"identity": qa_name, "real_api": "NOT TESTED", "physical_keyboard_ime": "NOT TESTED", "clipboard": "NOT TOUCHED"}
manifest = {"project": str(project), "data_directory": str(data), "isolated_bundle": str(bundle), "captures": []}


def run(*args, cwd=root):
    return subprocess.check_output(args, text=True, cwd=cwd, timeout=30).strip()


def write_json(name, value):
    (out / name).write_text(json.dumps(value, ensure_ascii=False, indent=2))


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def code_digest(path):
    raw = path.read_bytes()
    header = struct.unpack_from("<IiiIIIII", raw)
    assert header[0] == 0xFEEDFACF, "expected Intel 64-bit Mach-O"
    position = 32
    for _ in range(header[4]):
        command, size = struct.unpack_from("<II", raw, position)
        if command == 0x19:
            segment = struct.unpack_from("<II16sQQQQiiII", raw, position)
            for index in range(segment[9]):
                section = struct.unpack_from("<16s16sQQIIIIIIII", raw, position + 72 + index * 80)
                if section[0].rstrip(b"\0") == b"__text" and section[1].rstrip(b"\0") == b"__TEXT":
                    return hashlib.sha256(raw[section[4]:section[4] + section[3]]).hexdigest()
        position += size
    raise AssertionError("Mach-O executable code section missing")


def resources(path):
    base = path / "Contents/Resources"
    return {str(p.relative_to(base)): digest(p) for p in sorted(base.rglob("*")) if p.is_file()}


def source_files():
    paths = [root / name for name in ("go.mod", "go.sum", "mygo.json", "third_party/mygo/go.mod", "third_party/mygo/go.sum")]
    for directory in ("cmd", "internal", "third_party/mygo"):
        paths.extend(p for p in (root / directory).rglob("*.go") if not p.name.endswith("_test.go"))
    for directory in ("internal/desktop/fonts", "resources"):
        paths.extend(p for p in (root / directory).rglob("*") if p.is_file())
    return {str(p.relative_to(root)): digest(p) for p in sorted(paths)}


def ax(body):
    script = 'tell application "System Events" to tell (first application process whose unix id is ' + str(app.pid) + ')\n' + body + '\nend tell'
    return run("osascript", "-e", script)


def wait_until(condition, label, timeout=15):
    deadline = time.monotonic() + timeout
    last_error = None
    while time.monotonic() < deadline:
        if app.poll() is not None:
            raise AssertionError("QA application exited before " + label)
        try:
            if condition():
                return
        except (subprocess.CalledProcessError, FileNotFoundError, json.JSONDecodeError) as error:
            last_error = str(error)
        time.sleep(0.1)
    raise AssertionError(label + ("; last error: " + last_error if last_error else ""))


def exists(reference):
    return ax("return exists " + reference) == "true"


def state():
    return json.loads((data / "controller.json").read_text())["snapshot"]


def checkpoint(name):
    (out / (name + "-ax.txt")).write_text(ax("return entire contents of window 1"))


def capture(name):
    query = '''import CoreGraphics
let rows = CGWindowListCopyWindowInfo(.optionOnScreenOnly, kCGNullWindowID) as? [[String: Any]] ?? []
for row in rows {
 if (row[kCGWindowOwnerPID as String] as? Int ?? 0) == PID_VALUE,
    (row[kCGWindowLayer as String] as? Int ?? -1) == 0 {
  print(row[kCGWindowNumber as String] ?? "")
  break
 }
}'''.replace("PID_VALUE", str(app.pid))
    window_id = run("swift", "-e", query)
    assert window_id, "native CGWindow missing"
    path = out / name
    subprocess.run(["screencapture", "-x", "-o", "-l", window_id, str(path)], check=True)
    manifest["captures"].append({"file": name, "pid": app.pid, "window_id": int(window_id), "sha256": digest(path)})


def quit_app():
    names = ax("return name of every menu bar item of menu bar 1").split(", ")
    menu = json.dumps(names[1], ensure_ascii=False)
    ax('click menu bar item ' + menu + ' of menu bar 1\nclick menu item "退出 GCode" of menu 1 of menu bar item ' + menu + ' of menu bar 1')
    assert app.wait(timeout=10) == 0


def text_ref(value, scroll=1):
    return "static text " + json.dumps(value, ensure_ascii=False) + " of scroll area " + str(scroll) + " of window 1"


tracked = project / (qa_name + "-edit.txt")
before = "QA original\n中文文件预览 🧪\n"
changed = "QA tracked change\n中文文件预览 🧪\n"
edited = "QA edited by TextEdit\n中文外部编辑已保存 🧪\n"
tracked.write_text(before)
(project / "nested").mkdir()
(project / "nested/child.txt").write_text("QA nested child\n")
(project / "binary.bin").write_bytes(b"QA\x00\xff\x00binary")
run("git", "init", "-b", "main", cwd=project)
run("git", "add", ".", cwd=project)
run("git", "-c", "user.name=GCode QA", "-c", "user.email=qa@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "QA fixture baseline", cwd=project)
tracked.write_text(changed)
(project / "untracked.txt").write_text("QA untracked\n")
(out / "git-before.txt").write_text(run("git", "status", "--short", cwd=project) + "\n" + run("git", "diff", cwd=project))

source_exe = source / "Contents/MacOS/GCode"
original = {"executable": digest(source_exe), "code": code_digest(source_exe), "resources": resources(source)}
shutil.copytree(source, bundle, symlinks=True)
info_path = bundle / "Contents/Info.plist"
info = plistlib.loads(info_path.read_bytes())
info.update(CFBundleName=qa_name, CFBundleDisplayName=qa_name, CFBundleIdentifier="app.gcode.desktop.workspaceqa." + qa_name.lower())
info_path.write_bytes(plistlib.dumps(info))
subprocess.run(["codesign", "--force", "--deep", "--sign", "-", str(bundle)], check=True, capture_output=True)
subprocess.run(["codesign", "--verify", "--deep", "--strict", str(bundle)], check=True, capture_output=True)
exe = bundle / "Contents/MacOS/GCode"
assert code_digest(exe) == original["code"] and resources(bundle) == original["resources"]
assert not data.exists()
manifest.update(qa_identity=qa_name, driver_sha256=digest(pathlib.Path(__file__)), original_bundle=str(source), original_executable_sha256=original["executable"], isolated_executable_sha256=digest(exe), executable_code_sha256=original["code"], resource_sha256=original["resources"], code_and_resources_identical=True, source_files_at_start=source_files(), os=run("sw_vers"), hardware=run("sysctl", "-n", "hw.model"), go_build_info=run("go", "version", "-m", str(source_exe)))
write_json("manifest.json", manifest)
env = os.environ.copy()
env["GCODE_CRED_STORAGE"] = "encrypted-file"
for name in ("GCODE_VISUAL_SAMPLE", "GCODE_EDITOR", "VISUAL", "EDITOR"):
    env.pop(name, None)

with (out / "application.log").open("w") as log:
    app = subprocess.Popen([str(exe)], cwd=root, env=env, stdout=log, stderr=log)
    manifest["pid"] = app.pid
    try:
        wait_until(lambda: exists('button "设置" of window 1'), "native window ready")
        ax("set frontmost to true")
        ax('set size of window 1 to {1440, 900}\nclick button "设置" of window 1')
        ax('click radio button "深色" of radio group 1 of scroll area 1 of window 1\nclick button "保存设置" of window 1\nclick button "返回工作台" of window 1')
        ax('set value of text field 1 of scroll area 1 of window 1 to ' + json.dumps(str(project)) + '\nclick button "打开项目" of scroll area 1 of window 1')
        wait_until(lambda: len(state()["projects"] or []) == 1, "project opened")
        ax('click button "新会话" of window 1')
        wait_until(lambda: len(state()["sessions"] or []) == 1, "session created")
        checkpoint("created-session")
        wait_until(lambda: exists('button "打开文件 ' + tracked.name + '" of scroll area 2 of window 1'), "workspace files loaded")
        checkpoint("initial-files")
        for filename in (tracked.name, "untracked.txt", "nested", "binary.bin"):
            assert exists('button "打开文件 ' + filename + '" of scroll area 2 of window 1')
        ax('click button "打开文件 nested" of scroll area 2 of window 1')
        wait_until(lambda: exists('button "打开文件 child.txt" of scroll area 2 of window 1'), "nested directory loaded")
        ax('click button "打开文件 child.txt" of scroll area 2 of window 1')
        wait_until(lambda: exists(text_ref("QA nested child\n", 3)), "nested preview loaded")
        checkpoint("nested-preview")
        ax('click button "返回上级目录" of window 1')
        wait_until(lambda: exists('button "打开文件 binary.bin" of scroll area 2 of window 1'), "returned to project root")
        ax('click button "打开文件 binary.bin" of scroll area 2 of window 1')
        wait_until(lambda: exists('static text "二进制文件不提供预览" of window 1'), "binary preview message")
        checkpoint("binary-preview")
        ax('click button "打开文件 ' + tracked.name + '" of scroll area 2 of window 1')
        wait_until(lambda: exists(text_ref(changed, 3)), "tracked preview loaded")
        results["files_preview_directory_binary"] = "PASS"
        ax('click radio button "Diff" of tab group 1 of window 1')
        wait_until(lambda: exists('static text "QA tracked change" of scroll area 1 of scroll area 2 of window 1'), "real Git Diff loaded")
        assert exists('static text "QA original" of scroll area 1 of scroll area 2 of window 1')
        assert exists('static text "QA untracked" of scroll area 2 of scroll area 2 of window 1')
        results["real_git_tracked_untracked_diff"] = "PASS"
        checkpoint("tracked-diff")
        capture("tracked-diff-native.png")
        ax('click radio button "文件" of tab group 1 of window 1')
        wait_until(lambda: exists('button "编辑" of window 1'), "file preview restored")
        ax('click button "编辑" of window 1')
        target = json.dumps(str(tracked), ensure_ascii=False)
        title = json.dumps(tracked.name)
        editor_ax_prefix = 'tell application "System Events" to tell process "TextEdit"\n'
        wait_until(lambda: run("osascript", "-e", editor_ax_prefix + 'return exists window ' + title + '\nend tell') == "true", "TextEdit opened exact QA window")
        (out / "textedit-qa-ax.txt").write_text(run("osascript", "-e", editor_ax_prefix + 'return entire contents of window ' + title + '\nend tell'))
        editor_field = 'text area 1 of scroll area 1 of window ' + title
        readback = run("osascript", "-e", editor_ax_prefix + 'set value of ' + editor_field + ' to ' + json.dumps(edited, ensure_ascii=False) + '\nreturn value of ' + editor_field + '\nend tell')
        assert readback == edited.strip()
        run("osascript", "-e", editor_ax_prefix + 'set frontmost to true\nperform action "AXRaise" of window ' + title + '\nend tell')
        wait_until(lambda: run("osascript", "-e", editor_ax_prefix + 'return name of (value of attribute "AXFocusedWindow")\nend tell') == tracked.name, "exact QA TextEdit window focused")
        run("osascript", "-e", editor_ax_prefix + 'if name of (value of attribute "AXFocusedWindow") is not ' + title + ' then error "unexpected focused editor window"\nkeystroke "s" using command down\nend tell')
        wait_until(lambda: tracked.read_text() == edited, "TextEdit save reached disk")
        assert tracked.read_text() == edited
        results["external_textedit_saved"] = "PASS"
        manifest["textedit"] = {"qa_file": str(tracked), "before_sha256": hashlib.sha256(changed.encode()).hexdigest(), "saved_sha256": digest(tracked), "pid": int(run("osascript", "-e", editor_ax_prefix + 'return unix id\nend tell'))}
        run("osascript", "-e", editor_ax_prefix + 'click (first button of window ' + title + ' whose subrole is "AXCloseButton")\nend tell')
        wait_until(lambda: run("osascript", "-e", editor_ax_prefix + 'return exists window ' + title + '\nend tell') == "false", "QA TextEdit window closed")
        ax('set frontmost to true\nclick button "刷新工作区" of window 1')
        wait_until(lambda: exists(text_ref(edited, 3)), "preview refreshed after TextEdit save")
        checkpoint("edited-preview")
        capture("edited-preview-native.png")
        ax('click radio button "Diff" of tab group 1 of window 1')
        wait_until(lambda: exists('static text "QA edited by TextEdit" of scroll area 1 of scroll area 2 of window 1'), "edited Git Diff loaded")
        checkpoint("edited-diff")
        capture("edited-diff-native.png")
        results["preview_and_diff_refresh"] = "PASS"
        (out / "git-after-edit.txt").write_text(run("git", "status", "--short", cwd=project) + "\n" + run("git", "diff", cwd=project))
        ax('click radio button "终端" of tab group 1 of window 1')
        time.sleep(0.5)
        checkpoint("terminal-initial")
        capture("terminal-initial-native.png")
        results["terminal"] = "PREPARED; follow-up native probe required"
        quit_app()
        results["normal_exit"] = app.returncode == 0
    except Exception as error:
        results["error"] = str(error)
        try:
            checkpoint("failure")
            capture("failure-native.png")
        except Exception:
            pass
        raise
    finally:
        if app.poll() is None:
            try:
                quit_app()
            except Exception:
                app.terminate()
                app.wait(timeout=10)
        manifest.update(original_bundle_unchanged=digest(source_exe) == original["executable"] and resources(source) == original["resources"], source_unchanged=source_files() == manifest["source_files_at_start"], qa_exit_code=app.returncode)
        write_json("manifest.json", manifest)
        write_json("results.json", results)
assert manifest["original_bundle_unchanged"] and manifest["source_unchanged"]
print(json.dumps(results, ensure_ascii=False, indent=2), flush=True)
