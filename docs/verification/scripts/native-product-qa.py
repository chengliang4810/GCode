"""Verify packaged native settings using an isolated bundle identity and fake keys."""

import argparse
import hashlib
import json
import os
import pathlib
import plistlib
import shutil
import statistics
import struct
import subprocess
import time


root = pathlib.Path(__file__).resolve().parents[3]
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--bundle", default="build/darwin-amd64/GCode.app")
parser.add_argument("--output", default="output/acceptance/native-product")
options = parser.parse_args()
source = root / options.bundle
out = root / options.output
out.mkdir(parents=True, exist_ok=True)
qa_name = "GCodePackageQA" + str(time.time_ns())
bundle = out / (qa_name + ".app")
data = pathlib.Path.home() / "Library/Application Support" / qa_name
fake_key = "qa-only-inert-api-key-no-live-endpoint"
project = out / "project"
project.mkdir(exist_ok=True)


def run(*args):
    return subprocess.check_output(args, text=True, cwd=root).strip()


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def code_digest(path):
    """Signing can change Mach-O metadata; compare actual __TEXT,__text bytes."""
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


def ax(app, body):
    script = 'tell application "System Events" to tell (first application process whose unix id is ' + str(app.pid) + ')\n' + body + '\nend tell'
    return run("osascript", "-e", script)


def wait_until(condition, label, timeout=15):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if condition():
            return
        time.sleep(0.05)
    raise AssertionError(label)


def state():
    return json.loads((data / "controller.json").read_text())["snapshot"]


def ready(app):
    if app.poll() is not None:
        raise AssertionError("application exited before native window was ready")
    try:
        return ax(app, 'return exists button "设置" of window 1') == "true"
    except subprocess.CalledProcessError:
        return False


def quit_app(app):
    names = ax(app, "return name of every menu bar item of menu bar 1").split(", ")
    assert len(names) >= 2, names
    menu = json.dumps(names[1], ensure_ascii=False)
    ax(app, 'click menu bar item ' + menu + ' of menu bar 1\nclick menu item "退出 GCode" of menu 1 of menu bar item ' + menu + ' of menu bar 1')
    assert app.wait(timeout=10) == 0


def capture(app, name):
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
    manifest.setdefault("captures", []).append({"file": name, "pid": app.pid, "window_id": int(window_id), "sha256": digest(path)})
    return int(window_id)


source_exe = source / "Contents/MacOS/GCode"
original = {"executable": digest(source_exe), "code": code_digest(source_exe), "resources": resources(source)}
shutil.copytree(source, bundle, symlinks=True)
info_path = bundle / "Contents/Info.plist"
info = plistlib.loads(info_path.read_bytes())
info.update(CFBundleName=qa_name, CFBundleDisplayName=qa_name, CFBundleIdentifier="app.gcode.desktop.packageqa." + qa_name.lower())
info_path.write_bytes(plistlib.dumps(info))
subprocess.run(["codesign", "--force", "--deep", "--sign", "-", str(bundle)], check=True, capture_output=True)
subprocess.run(["codesign", "--verify", "--deep", "--strict", str(bundle)], check=True, capture_output=True)
exe = bundle / "Contents/MacOS/GCode"
assert code_digest(exe) == original["code"]
assert resources(bundle) == original["resources"]
assert not data.exists(), "QA identity must start with an empty data directory"
manifest = {
    "original_bundle": str(source), "isolated_bundle": str(bundle), "qa_identity": qa_name,
    "data_directory": str(data), "original_executable_sha256": original["executable"],
    "isolated_executable_sha256": digest(exe), "executable_code_sha256": original["code"],
    "resource_sha256": original["resources"], "code_and_resources_identical": True,
    "changes": "Bundle identity metadata and ad hoc code signature only",
    "os": run("sw_vers"), "hardware": run("sysctl", "-n", "hw.model"),
    "source_files_at_start": source_files(),
    "go_build_info": run("go", "version", "-m", str(source_exe)),
    "measurement": "Process launch to ready AX settings control; warm filesystem cache possible. AX round trips include osascript IPC overhead.",
}
(out / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2))
env = os.environ.copy()
env["GCODE_CRED_STORAGE"] = "encrypted-file"
env.pop("GCODE_VISUAL_SAMPLE", None)
launches = []
switches = []
for index in range(3):
    with (out / ("application-" + str(index) + ".log")).open("w") as log:
        started = time.perf_counter()
        app = subprocess.Popen([str(exe)], cwd=root, env=env, stdout=log, stderr=log)
        try:
            wait_until(lambda: ready(app), "packaged native window did not become ready")
            elapsed = (time.perf_counter() - started) * 1000
            ax(app, "set frontmost to true")
            launches.append({"pid": app.pid, "ready_ms": round(elapsed, 2)})
            ax(app, 'click button "设置" of window 1')
            if index == 0:
                (out / "initial-settings-ax.txt").write_text(ax(app, "return entire contents of window 1"))
                assert ax(app, 'return value of radio button "纠正" of radio group 2 of scroll area 1 of window 1') == "1"
                ax(app, 'click radio button "浅色" of radio group 1 of scroll area 1 of window 1\nclick radio button "队列" of radio group 2 of scroll area 1 of window 1\nclick button "保存设置" of window 1')
                wait_until(lambda: (data / "controller.json").exists() and state()["settings"]["theme"] == "light" and state()["settings"]["default_send_mode"] == "queue", "settings did not persist")
                for label, model in [("隔离服务 A", "fixture-a"), ("隔离服务 B", "fixture-b")]:
                    ax(app, 'click button "添加服务" of scroll area 1 of window 1')
                    wait_until(lambda: ax(app, "return count of text fields of scroll area 1 of window 1") == "4", "provider editor did not appear")
                    for field, value in [(1, label), (2, "http://127.0.0.1:9/v1"), (3, model), (4, fake_key)]:
                        ax(app, 'set value of text field ' + str(field) + ' of scroll area 1 of window 1 to ' + json.dumps(value, ensure_ascii=False))
                    ax(app, 'click button "保存服务" of window 1')
                    wait_until(lambda: any(p["name"] == label for p in (state()["settings"]["providers"] or [])), "provider did not persist")
                    wait_until(lambda: ax(app, 'return exists static text "本机凭据存储中已有密钥" of scroll area 1 of window 1') == "true", "saved provider credential status missing")
                capture(app, "saved-settings-native.png")
            else:
                current = state()
                assert current["settings"]["theme"] == "light"
                assert current["settings"]["default_send_mode"] == "queue"
                assert sorted(p["name"] for p in current["settings"]["providers"]) == ["隔离服务 A", "隔离服务 B"]
                for provider in current["settings"]["providers"]:
                    ax(app, 'click button ' + json.dumps("模型服务 " + provider["name"], ensure_ascii=False) + ' of scroll area 1 of window 1')
                    wait_until(lambda: ax(app, 'return value of text field 1 of scroll area 1 of window 1') == provider["name"], "provider selection did not render")
                    wait_until(lambda: ax(app, 'return exists static text "本机凭据存储中已有密钥" of scroll area 1 of window 1') == "true", "restored provider credential status missing")
                assert ax(app, 'return value of radio button "浅色" of radio group 1 of scroll area 1 of window 1') == "1"
                assert ax(app, 'return value of radio button "队列" of radio group 2 of scroll area 1 of window 1') == "1"
                if index == 1:
                    manifest["restored_window_id"] = capture(app, "restored-settings-native.png")
                    (out / "restored-settings-ax.txt").write_text(ax(app, "return entire contents of window 1"))
            ax(app, 'click button "返回工作台" of window 1')
            if index == 0:
                (out / "initial-workbench-ax.txt").write_text(ax(app, "return entire contents of window 1"))
                ax(app, 'set value of text field 1 of window 1 to ' + json.dumps(str(project)) + '\nclick button "打开项目" of window 1')
                wait_until(lambda: len(state()["projects"] or []) == 1, "project did not open")
                ax(app, 'click button "新会话" of window 1')
                wait_until(lambda: len(state()["sessions"] or []) == 1, "session did not appear")
                wait_until(lambda: ax(app, 'return exists radio group 1 of window 1') == "true", "composer controls missing")
                assert ax(app, 'return value of radio button "队列" of radio group 1 of window 1') == "1"
                ax(app, 'click radio button "纠正" of radio group 1 of window 1')
                assert ax(app, 'return value of radio button "纠正" of radio group 1 of window 1') == "1"
                assert state()["settings"]["default_send_mode"] == "queue"
                manifest["per_message_choice_preserves_global_default"] = True
                assert state()["sessions"][0]["worktree"] is False
                assert state()["sessions"][0]["cwd"] == str(project)
            else:
                assert ax(app, 'return value of radio button "队列" of radio group 1 of window 1') == "1"
            if index == 2:
                for _ in range(5):
                    start = time.perf_counter()
                    ax(app, 'click button "设置" of window 1\nreturn exists radio group 1 of scroll area 1 of window 1')
                    switches.append(round((time.perf_counter() - start) * 1000, 2))
                    ax(app, 'click button "返回工作台" of window 1')
            time.sleep(0.3)
            launches[-1]["idle_rss_kib"] = int(run("ps", "-p", str(app.pid), "-o", "rss="))
            quit_app(app)
            launches[-1]["exit_code"] = app.returncode
        finally:
            if app.poll() is None:
                try:
                    quit_app(app)
                except Exception:
                    app.terminate()
                    app.wait(timeout=10)
            (out / "measurement-samples.json").write_text(json.dumps({"launches": launches, "settings_switch_round_trip_ms": switches}, ensure_ascii=False, indent=2))

assert fake_key not in (data / "controller.json").read_text()
credentials_file = data / "credentials" / "credentials.enc"
assert credentials_file.exists()
assert fake_key.encode() not in credentials_file.read_bytes()
assert digest(source_exe) == original["executable"]
assert resources(source) == original["resources"]
assert source_files() == manifest["source_files_at_start"]
shutil.copyfile(data / "controller.json", out / "restored-controller.json")
assert all(fake_key not in p.read_text() for p in out.glob("application-*.log"))
manifest.update(normal_exit=True, original_bundle_unchanged=True, encrypted_fake_keys=True, source_unchanged=True, fake_key_absent_from_application_logs=True)
(out / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2))
evidence = {"launches": launches, "settings_switch_round_trip_ms": switches, "settings_switch_median_ms": statistics.median(switches), "settings_restart": "PASS", "providers_restart": "PASS", "real_api": "NOT TESTED", "physical_keyboard_ime": "NOT TESTED"}
(out / "results.json").write_text(json.dumps(evidence, ensure_ascii=False, indent=2))
print(json.dumps(evidence, ensure_ascii=False, indent=2), flush=True)
