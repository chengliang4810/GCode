"""Measure the packaged native conversation using isolated, labeled history fixtures."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import plistlib
import re
import shutil
import statistics
import struct
import subprocess
import time


ROOT = Path(__file__).resolve().parents[3]


def run(*args, timeout=30):
    return subprocess.check_output(args, cwd=ROOT, text=True, stderr=subprocess.PIPE, timeout=timeout).strip()


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def code_digest(path):
    raw = path.read_bytes()
    header = struct.unpack_from("<IiiIIIII", raw)
    if header[0] != 0xFEEDFACF:
        raise AssertionError("Expected Intel Mach-O")
    pos = 32
    for _ in range(header[4]):
        command, size = struct.unpack_from("<II", raw, pos)
        if command == 0x19:
            segment = struct.unpack_from("<II16sQQQQiiII", raw, pos)
            for index in range(segment[9]):
                section = struct.unpack_from("<16s16sQQIIIIIIII", raw, pos + 72 + index * 80)
                if section[0].rstrip(b"\0") == b"__text" and section[1].rstrip(b"\0") == b"__TEXT":
                    return hashlib.sha256(raw[section[4]:section[4] + section[3]]).hexdigest()
        pos += size
    raise AssertionError("Executable code section missing")


def resources(bundle):
    base = bundle / "Contents/Resources"
    return {str(p.relative_to(base)): digest(p) for p in sorted(base.rglob("*")) if p.is_file()}


def sources():
    paths = [ROOT / p for p in ("go.mod", "go.sum", "mygo.json")]
    for name in ("cmd", "internal", "third_party/mygo"):
        paths.extend(p for p in (ROOT / name).rglob("*.go") if not p.name.endswith("_test.go"))
    for directory in ("internal/desktop/fonts", "resources"):
        paths.extend(p for p in (ROOT / directory).rglob("*") if p.is_file())
    return {str(p.relative_to(ROOT)): digest(p) for p in sorted(paths)}


def quote(value):
    return json.dumps(value, ensure_ascii=False)


def seed(data, project):
    now = "2026-10-12T00:00:00+08:00"
    sessions = []
    for count in (5000, 1000, 100):
        messages = []
        for index in range(count):
            marker = f"QA_HISTORY_{count}_{index:05d}"
            text = f"{marker} 明确标注的长会话验收夹具，包含中文与 emoji 🧪。"
            if index % 4 == 1:
                text += "\n\n检查保存状态和测试结果。\n\n```go\nfunc verify() error {\n    return nil\n}\n```"
            messages.append({"id": marker, "role": "user" if index % 2 == 0 else "assistant",
                             "text": text, "state": "completed", "created_at": now})
        sessions.append({"id": f"qa-history-{count}", "project_id": "qa-project", "title": f"验收夹具 {count} 条消息",
                         "cwd": str(project), "worktree": False, "full_access": False, "state": "idle",
                         "messages": messages, "queue": [], "updated_at": now})
    snapshot = {"projects": [{"id": "qa-project", "name": "长会话验收夹具", "path": str(project)}],
                "sessions": sessions, "settings": {"providers": [], "default_send_mode": "correction", "theme": "dark"}}
    data.mkdir(parents=True)
    state = data / "controller.json"
    state.write_text(json.dumps({"version": 1, "snapshot": snapshot}, ensure_ascii=False))
    state.chmod(0o600)
    return {"history_sizes": [100, 1000, 5000], "fixture": "Synthetic persisted history through the production loader",
            "controller_seed_sha256": digest(state), "messages_total": sum(len(s["messages"]) for s in sessions)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bundle", default="build/darwin-amd64/GCode.app")
    parser.add_argument("--output", default="output/acceptance/native-conversation/round-1")
    args = parser.parse_args()
    out = (ROOT / args.output).resolve()
    out.mkdir(parents=True, exist_ok=True)
    if (out / "manifest.json").exists():
        raise AssertionError("Use a fresh output directory")
    qa_name = "GCodeConversationQA" + str(time.time_ns())
    data = Path.home() / "Library/Application Support" / qa_name
    bundle = out / (qa_name + ".app")
    source = (ROOT / args.bundle).resolve()
    project = out / ("project-" + qa_name)
    project.mkdir()
    (project / "fixture.txt").write_text("Clearly labeled synthetic history QA project.\n")
    app = None
    manifest = {"status": "RUNNING", "qa_identity": qa_name, "data_directory": str(data),
                "source_bundle": str(source), "isolated_bundle": str(bundle), "project": str(project),
                "driver_sha256": digest(Path(__file__).resolve()),
                "captures": [], "clipboard": "NOT TOUCHED", "real_api": "NOT TESTED",
                "physical_input_ime": "NOT TESTED", "measurement": "AX round trips include osascript IPC; warm cache possible; no performance threshold supplied"}
    samples = []
    source_before = sources()
    original = None

    def write():
        (out / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2))
        (out / "measurement-samples.json").write_text(json.dumps(samples, ensure_ascii=False, indent=2))

    def ax(body):
        return run("osascript", "-e", 'tell application "System Events" to tell (first application process whose unix id is '
                   + str(app.pid) + ")\n" + body + "\nend tell")

    def wait(condition, label, timeout=30):
        deadline = time.monotonic() + timeout
        last = ""
        while time.monotonic() < deadline:
            if app.poll() is not None:
                raise AssertionError("App exited while waiting for " + label)
            try:
                if condition():
                    return
            except (subprocess.CalledProcessError, FileNotFoundError) as error:
                last = str(error)
            time.sleep(0.1)
        raise AssertionError(label + "; " + last)

    def texts(name):
        value = ax("return entire contents of table 1 of window 1")
        (out / (name + "-ax.txt")).write_text(value)
        return sorted(set(re.findall(r"QA_HISTORY_\d+_\d{5}", value)))

    def capture(name):
        swift = '''import CoreGraphics
let rows = CGWindowListCopyWindowInfo(.optionOnScreenOnly, kCGNullWindowID) as? [[String: Any]] ?? []
for row in rows { if (row[kCGWindowOwnerPID as String] as? Int ?? 0) == QA_PID_VALUE,
 (row[kCGWindowLayer as String] as? Int ?? -1) == 0 { print(row[kCGWindowNumber as String] ?? ""); break } }'''
        window = run("swift", "-e", swift.replace("QA_PID_VALUE", str(app.pid)))
        if not window:
            raise AssertionError("QA native window not found")
        path = out / (name + "-native.png")
        run("screencapture", "-x", "-o", "-l", window, str(path))
        manifest["captures"].append({"file": path.name, "pid": app.pid, "window_id": int(window), "sha256": digest(path)})

    def rss():
        return int(run("ps", "-o", "rss=", "-p", str(app.pid)))

    def quit_app():
        names = ax("return name of every menu bar item of menu bar 1").split(", ")
        menu = quote(names[1])
        ax('click menu bar item ' + menu + ' of menu bar 1\nclick menu item "退出 GCode" of menu 1 of menu bar item ' + menu + ' of menu bar 1')
        if app.wait(timeout=15) != 0:
            raise AssertionError("QA application quit failed")

    write()
    try:
        source_exe = source / "Contents/MacOS/GCode"
        original = {"executable_sha256": digest(source_exe), "code_sha256": code_digest(source_exe), "resources": resources(source)}
        shutil.copytree(source, bundle, symlinks=True)
        info_path = bundle / "Contents/Info.plist"
        info = plistlib.loads(info_path.read_bytes())
        info.update(CFBundleName=qa_name, CFBundleDisplayName=qa_name, CFBundleIdentifier="app.gcode.desktop.conversationqa." + qa_name.lower())
        info_path.write_bytes(plistlib.dumps(info))
        run("codesign", "--force", "--deep", "--sign", "-", str(bundle))
        run("codesign", "--verify", "--deep", "--strict", str(bundle))
        exe = bundle / "Contents/MacOS/GCode"
        assert code_digest(exe) == original["code_sha256"] and resources(bundle) == original["resources"]
        manifest.update(identity=original, isolated_executable_sha256=digest(exe), source_files_at_start=source_before,
                        os=run("sw_vers"), hardware=run("sysctl", "-n", "hw.model"), fixture=seed(data, project))
        swift_path = out / "scroll.swift"
        swift_path.write_text('''import CoreGraphics
import Foundation
let pid = Int32(CommandLine.arguments[1])!
let x = Double(CommandLine.arguments[2])!, y = Double(CommandLine.arguments[3])!
let delta = Int32(CommandLine.arguments[4])!
guard let original = CGEvent(source: nil)?.location else { fatalError("cursor unavailable") }
func move(_ point: CGPoint) {
    guard let e = CGEvent(mouseEventSource: nil, mouseType: .mouseMoved, mouseCursorPosition: point, mouseButton: .left) else { fatalError("pointer event unavailable") }
    e.post(tap: .cghidEventTap)
}
defer { move(original) }
move(CGPoint(x: x, y: y))
Thread.sleep(forTimeInterval: 0.15)
guard let e = CGEvent(scrollWheelEvent2Source: nil, units: .pixel, wheelCount: 1, wheel1: delta, wheel2: 0, wheel3: 0) else { fatalError("scroll event unavailable") }
e.location = CGPoint(x: x, y: y)
e.post(tap: .cghidEventTap)
Thread.sleep(forTimeInterval: 0.15)
move(original)
Thread.sleep(forTimeInterval: 0.15)
guard let after = CGEvent(source: nil)?.location else { fatalError("cursor readback unavailable") }
let restored = abs(after.x - original.x) < 1 && abs(after.y - original.y) < 1
let record: [String: Any] = ["pid": pid, "original": [original.x, original.y], "after": [after.x, after.y], "restored": restored]
print(String(data: try! JSONSerialization.data(withJSONObject: record), encoding: .utf8)!)
if !restored { exit(1) }
''')
        scroll_exe = out / "native-scroll"
        run("swiftc", str(swift_path), "-o", str(scroll_exe))
        env = os.environ.copy()
        env["GCODE_CRED_STORAGE"] = "encrypted-file"
        env.pop("GCODE_VISUAL_SAMPLE", None)
        with (out / "application.log").open("w") as log:
            started = time.perf_counter()
            app = subprocess.Popen([str(exe)], cwd=ROOT, env=env, stdout=log, stderr=log)
            manifest["pid"] = app.pid
            write()
            wait(lambda: ax('return exists button "设置" of window 1') == "true", "native ready")
            manifest["launch_ready_ms"] = (time.perf_counter() - started) * 1000
            ax("set frontmost to true\nset size of window 1 to {1440, 900}")
            for count in (5000, 1000, 100):
                started = time.perf_counter()
                ax('click button ' + quote(f"会话 验收夹具 {count} 条消息") + ' of scroll area 1 of window 1')
                markers = []
                def has_latest():
                    nonlocal markers
                    markers = texts(f"history-{count}-bottom")
                    return f"QA_HISTORY_{count}_{count-1:05d}" in markers
                wait(has_latest, "latest message for " + str(count))
                switch_ms = (time.perf_counter() - started) * 1000
                input_ms = []
                for iteration in range(3):
                    value = f"原生输入读回 {count} / {iteration} 🧪"
                    started = time.perf_counter()
                    assert ax('set value of text area 1 of window 1 to ' + quote(value) + '\nreturn value of text area 1 of window 1') == value
                    input_ms.append((time.perf_counter() - started) * 1000)
                ax('set value of text area 1 of window 1 to ""')
                samples.append({"messages": count, "switch_and_latest_ax_ms": switch_ms, "input_roundtrip_ms": input_ms,
                                "input_median_ms": statistics.median(input_ms), "rss_kib": rss(), "realized_history_markers": markers,
                                "markers_count": len(markers)})
                capture(f"history-{count}-bottom")
                write()
            ax('click button "会话 验收夹具 5000 条消息" of scroll area 1 of window 1')
            wait(lambda: "QA_HISTORY_5000_04999" in texts("scroll-before"), "5000 bottom")
            geometry = [int(v.strip()) for v in ax("return (position of table 1 of window 1) & (size of table 1 of window 1)").split(",")]
            x, y, width, height = geometry
            ax("set frontmost to true")
            assert ax('return name of value of attribute "AXFocusedWindow"') == "GCode"
            pointer = json.loads(run(str(scroll_exe), str(app.pid), str(x + width // 2), str(y + height // 2), "700"))
            assert pointer["restored"]
            wait(lambda: "QA_HISTORY_5000_04999" not in texts("scroll-after"), "synthetic wheel moved history away from end")
            before = texts("scroll-after")
            capture("history-5000-scrolled")
            ax('click button "会话 验收夹具 100 条消息" of scroll area 1 of window 1')
            wait(lambda: "QA_HISTORY_100_00099" in texts("switch-away"), "100 session after scroll")
            ax('click button "会话 验收夹具 5000 条消息" of scroll area 1 of window 1')
            wait(lambda: texts("switch-back") == before, "per-session reading position restored")
            capture("history-5000-restored")
            manifest["reading_position"] = {"synthetic_wheel": True, "delivery": "cghidEventTap after native cursor move; cursor restored",
                                            "pointer": pointer, "table_frame": geometry, "before_switch": before,
                                            "after_switch": texts("switch-back"), "restored": True}
            quit_app()
            manifest["exit_code"] = app.returncode
            manifest["status"] = "PASS_PENDING_CAPTURE_REVIEW"
    except Exception as error:
        manifest.update(status="FAIL", error=type(error).__name__ + ": " + str(error))
        if app is not None and app.poll() is None:
            try:
                capture("failure")
                (out / "failure-ax.txt").write_text(ax("return entire contents of window 1"))
            except Exception as capture_error:
                manifest["failure_capture_error"] = str(capture_error)
        raise
    finally:
        if app is not None and app.poll() is None:
            try:
                quit_app()
            except Exception:
                app.terminate()
                try:
                    app.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    app.kill()
                    app.wait(timeout=10)
        manifest["qa_process_exited"] = app is None or app.poll() is not None
        manifest["source_files_unchanged"] = sources() == source_before
        if original:
            manifest["original_bundle_unchanged"] = digest(source / "Contents/MacOS/GCode") == original["executable_sha256"] and resources(source) == original["resources"]
        write()


if __name__ == "__main__":
    main()
