"""Probe the packaged native terminal through the real macOS window surface.

The input workspace manifest must be produced by ``native-workspace-qa.py``.
This driver reuses that run's isolated bundle identity and data directory. It
does not call a model endpoint and it deliberately does not read or write the
system clipboard. Screenshot scrollback evidence remains pending visual
review by the root agent.
"""

import argparse
import hashlib
import json
import os
import pathlib
import plistlib
import re
import shlex
import signal
import struct
import subprocess
import time


ROOT = pathlib.Path(__file__).resolve().parents[3]
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--workspace-manifest", required=True)
parser.add_argument("--glyph-probe-only", action="store_true", help="Capture fixed terminal glyphs and exit; skip workspace/resize scenarios")
parser.add_argument("--output", default="output/acceptance/native-terminal/round-1")
options = parser.parse_args()


def resolve_from_root(value):
    path = pathlib.Path(value).expanduser()
    return path if path.is_absolute() else ROOT / path


workspace_manifest_path = resolve_from_root(options.workspace_manifest).resolve()
out = resolve_from_root(options.output).resolve()
out.mkdir(parents=True, exist_ok=True)
workspace_manifest = json.loads(workspace_manifest_path.read_text(encoding="utf-8"))
isolated_bundle = pathlib.Path(workspace_manifest["isolated_bundle"]).resolve()
data_directory = pathlib.Path(workspace_manifest["data_directory"]).resolve()
source_bundle = pathlib.Path(workspace_manifest.get("original_bundle", "")).resolve()
isolated_exe = isolated_bundle / "Contents/MacOS/GCode"
source_exe = source_bundle / "Contents/MacOS/GCode"
fixture = out / "fixture"
fixture.mkdir(parents=True, exist_ok=True)


def write_json(name, value):
    (out / name).write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def code_digest(path):
    """Hash the executable's __TEXT,__text bytes, excluding signing metadata."""
    raw = path.read_bytes()
    header = struct.unpack_from("<IiiIIIII", raw)
    if header[0] != 0xFEEDFACF:
        raise AssertionError("expected Intel 64-bit Mach-O executable")
    position = 32
    for _ in range(header[4]):
        command, size = struct.unpack_from("<II", raw, position)
        if command == 0x19:
            segment = struct.unpack_from("<II16sQQQQiiII", raw, position)
            for index in range(segment[9]):
                section = struct.unpack_from("<16s16sQQIIIIIIII", raw, position + 72 + index * 80)
                if section[0].rstrip(b"\0") == b"__text" and section[1].rstrip(b"\0") == b"__TEXT":
                    return hashlib.sha256(raw[section[4] : section[4] + section[3]]).hexdigest()
        position += size
    raise AssertionError("Mach-O executable code section missing")


def resources(bundle):
    base = bundle / "Contents/Resources"
    return {str(path.relative_to(base)): digest(path) for path in sorted(base.rglob("*")) if path.is_file()}


def source_files():
    paths = [ROOT / name for name in ("go.mod", "go.sum", "mygo.json", "third_party/mygo/go.mod", "third_party/mygo/go.sum")]
    for directory in ("cmd", "internal", "third_party/mygo"):
        paths.extend(path for path in (ROOT / directory).rglob("*.go") if not path.name.endswith("_test.go"))
    for directory in ("internal/desktop/fonts", "resources"):
        paths.extend(path for path in (ROOT / directory).rglob("*") if path.is_file())
    return {str(path.relative_to(ROOT)): digest(path) for path in sorted(paths)}


def run(*args, cwd=ROOT, timeout=30):
    completed = subprocess.run(
        args,
        cwd=cwd,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=timeout,
        check=False,
    )
    if completed.returncode != 0:
        raise subprocess.CalledProcessError(completed.returncode, args, completed.stdout, completed.stderr)
    return completed.stdout.strip()


def ax(app, body, timeout=30):
    script = (
        'tell application "System Events" to tell (first application process whose unix id is '
        + str(app.pid)
        + ")\n"
        + body
        + "\nend tell"
    )
    return run("osascript", "-e", script, timeout=timeout)


def wait_until(condition, label, timeout=30):
    deadline = time.monotonic() + timeout
    last_error = None
    while time.monotonic() < deadline:
        try:
            if condition():
                return
        except (AssertionError, OSError, ValueError, subprocess.CalledProcessError, json.JSONDecodeError) as error:
            last_error = str(error)
        time.sleep(0.1)
    suffix = "; last error: " + last_error if last_error else ""
    raise AssertionError(label + suffix)


def exists(app, reference):
    return ax(app, "return exists " + reference) == "true"


def text_ref(value, scroll=3):
    return "static text " + json.dumps(value, ensure_ascii=False) + " of scroll area " + str(scroll) + " of window 1"


def app_ready(app):
    if app.poll() is not None:
        raise AssertionError("QA application exited before the native window was ready")
    return exists(app, 'radio button "终端" of tab group 1 of window 1')


def window_geometry(app):
    value = ax(
        app,
        "set p to position of window 1\nset s to size of window 1\nreturn (item 1 of p as text) & tab & (item 2 of p as text) & tab & (item 1 of s as text) & tab & (item 2 of s as text)",
    )
    fields = value.split("\t")
    if len(fields) != 4:
        raise AssertionError("unexpected window geometry: " + value)
    return {"x": int(float(fields[0])), "y": int(float(fields[1])), "width": int(float(fields[2])), "height": int(float(fields[3]))}


def observe_entire_contents(app):
    return ax(app, "return entire contents of window 1")


def locate_terminal(app, ax_text):
    """Find the Terminal AX group by observed AX attributes, never by an index."""
    script = r'''
set tabCharacter to ASCII character 9
set candidates to entire contents of window 1
set output to ""
repeat with itemReference in candidates
    set candidate to contents of itemReference
    try
        set candidateName to name of candidate as text
    on error
        set candidateName to ""
    end try
    try
        set candidateDescription to description of candidate as text
    on error
        set candidateDescription to ""
    end try
    try
        set candidateRole to role of candidate as text
    on error
        set candidateRole to ""
    end try
    try
        set candidateTitle to value of attribute "AXTitle" of candidate as text
    on error
        set candidateTitle to ""
    end try
    try
        set candidateLabel to value of attribute "AXLabel" of candidate as text
    on error
        set candidateLabel to ""
    end try
    try
        set candidateAXDescription to value of attribute "AXDescription" of candidate as text
    on error
        set candidateAXDescription to ""
    end try
    if candidateRole contains "Group" or candidateRole is "AXGroup" or candidateRole is "group" then
        try
            set candidatePosition to position of candidate
            set candidateSize to size of candidate
            try
                set candidatePath to candidate as text
            on error
                set candidatePath to ""
            end try
            set output to output & candidateRole & tabCharacter & candidateName & tabCharacter & candidateDescription & tabCharacter & candidateTitle & tabCharacter & candidateLabel & tabCharacter & candidateAXDescription & tabCharacter & (item 1 of candidatePosition as text) & tabCharacter & (item 2 of candidatePosition as text) & tabCharacter & (item 1 of candidateSize as text) & tabCharacter & (item 2 of candidateSize as text) & tabCharacter & candidatePath & (ASCII character 10)
        end try
    end if
end repeat
return output
'''
    value = ax(app, script)
    groups = []
    for line in value.splitlines():
        fields = line.split("\t", 10)
        if len(fields) != 11:
            continue
        role, name, description, title, label, ax_description, x, y, width, height, path = fields
        try:
            groups.append(
                {
                    "role": role,
                    "name": name,
                    "description": description,
                    "ax_title": title,
                    "ax_label": label,
                    "ax_description": ax_description,
                    "x": int(float(x)),
                    "y": int(float(y)),
                    "width": int(float(width)),
                    "height": int(float(height)),
                    "path_from_candidate": path,
                }
            )
        except ValueError:
            continue
    if not groups:
        raise AssertionError("no AX group was returned")
    named = [
        group
        for group in groups
        if any(value == "Terminal" for value in (group["name"], group["description"], group["ax_title"], group["ax_label"], group["ax_description"]))
    ]
    if named:
        selected = named[0]
        selected["selection_reason"] = "AXTitle/AXLabel/AXDescription/name matched Terminal"
    else:
        # MyGo's group label can be present in AXDescription while System
        # Events' textual `entire contents` omits it. If a current build does
        # not expose any of the label attributes, use the largest observed
        # group only after retaining the full group inventory and AX tree.
        visible = [group for group in groups if group["width"] > 200 and group["height"] > 200]
        if not visible:
            raise AssertionError("Terminal AX group attributes were not exposed; groups=" + repr(groups))
        selected = max(visible, key=lambda group: group["width"] * group["height"])
        selected["selection_reason"] = "largest observed AXGroup fallback; visual review required"
    selected = dict(selected)
    selected["all_groups"] = groups
    selected["observed_tree_has_group_terminal_path"] = bool(re.search(r"(?:^|, )group[^,]*Terminal[^,]*", ax_text))
    return selected


def observed_terminal_path(ax_text, candidate_path):
    paths = re.findall(r"(?:^|, )(group[^,]*Terminal[^,]*)", ax_text)
    if paths:
        return max(paths, key=len)
    if candidate_path and candidate_path.lstrip().startswith("group "):
        return candidate_path
    observed_groups = re.findall(r"(?:^|, )(group \d+ of window[^,]*)", ax_text)
    if observed_groups:
        return observed_groups[-1]
    raise AssertionError("Terminal group path was not present in the observed AX tree")


def focus_snapshot(app):
    """Record process AXFocusedUIElement/AXFocusedWindow, not window AXFocused."""
    script = r'''
set processFrontmost to frontmost as text
try
    set focusedWindowReference to value of attribute "AXFocusedWindow"
    set focusedWindow to name of focusedWindowReference as text
on error
    set focusedWindow to ""
end try
try
    set focusedElementReference to value of attribute "AXFocusedUIElement"
    set focusedElement to description of focusedElementReference as text
on error
    set focusedElement to ""
end try
set terminalFocus to "unavailable"
set candidates to entire contents of window 1
repeat with itemReference in candidates
    set candidate to contents of itemReference
    try
        if (name of candidate as text) is "Terminal" or (description of candidate as text) is "Terminal" then
            try
                set terminalFocus to focused of candidate as text
            end try
            exit repeat
        end if
    end try
end repeat
return processFrontmost & tab & focusedWindow & tab & focusedElement & tab & terminalFocus
'''.replace("PID_VALUE", str(app.pid))
    fields = ax(app, script).split("\t", 3)
    if len(fields) != 4:
        raise AssertionError("unexpected focus record")
    return {"process_frontmost": fields[0], "ax_focused_window": fields[1], "ax_focused_ui_element": fields[2], "terminal_focused": fields[3]}


def click_terminal_and_record_focus(app, terminal):
    x = terminal["x"] + max(terminal["width"] // 2, 1)
    y = terminal["y"] + max(terminal["height"] // 2, 1)
    ax(app, "set frontmost to true\nclick at {" + str(x) + ", " + str(y) + "}")
    focus = {}

    def focused():
        nonlocal focus
        focus = focus_snapshot(app)
        return focus["process_frontmost"] == "true" and focus["ax_focused_window"] == "GCode" and focus["ax_focused_ui_element"] == "Terminal"

    wait_until(focused, "GCode process did not become the AX focused window")
    return {"click": {"x": x, "y": y}, **focus}


def type_command(app, command):
    if any(ord(character) > 127 for character in command):
        raise AssertionError("native terminal command must be ASCII")
    input_source("ascii")
    ax(app, "keystroke " + json.dumps(command) + "\nkey code 36")


def input_source(mode, identifier=""):
    # Synthetic keystrokes still pass through the user's current IME. Keep
    # the fixture deterministic and restore the original source in finally.
    script = '''import Carbon
import Foundation
func sourceID(_ source: TISInputSource) -> String {
 let p = TISGetInputSourceProperty(source, kTISPropertyInputSourceID)!
 return Unmanaged<CFString>.fromOpaque(p).takeUnretainedValue() as String
}
let mode = MODE_VALUE
let requestedID = IDENTIFIER_VALUE
if mode == "current" {
 print(sourceID(TISCopyCurrentKeyboardInputSource().takeRetainedValue()))
} else {
 let source: TISInputSource
 if mode == "ascii" {
  source = TISCopyCurrentASCIICapableKeyboardInputSource().takeRetainedValue()
 } else {
  let filter = [kTISPropertyInputSourceID as String: requestedID] as CFDictionary
  let list = TISCreateInputSourceList(filter, false).takeRetainedValue()
  guard CFArrayGetCount(list) == 1 else { fatalError("input source unavailable") }
  source = Unmanaged<TISInputSource>.fromOpaque(CFArrayGetValueAtIndex(list, 0)).takeUnretainedValue()
 }
 guard TISSelectInputSource(source) == noErr else { fatalError("input source selection failed") }
 Thread.sleep(forTimeInterval: 0.1)
 let actual = sourceID(TISCopyCurrentKeyboardInputSource().takeRetainedValue())
 guard actual == sourceID(source) else { fatalError("input source readback mismatch") }
 print(actual)
}'''.replace("MODE_VALUE", json.dumps(mode)).replace("IDENTIFIER_VALUE", json.dumps(identifier))
    return run("swift", "-e", script)


def capture(app, name, manifest):
    if any(item.get("file") == name for item in manifest.get("captures", [])):
        raise AssertionError("duplicate native screenshot registration: " + name)
    query = '''import CoreGraphics
let rows = CGWindowListCopyWindowInfo(.optionOnScreenOnly, kCGNullWindowID) as? [[String: Any]] ?? []
for row in rows {
 if (row[kCGWindowOwnerPID as String] as? Int ?? 0) == PID_VALUE,
    (row[kCGWindowLayer as String] as? Int ?? -1) == 0 {
  print(row[kCGWindowNumber as String] ?? "")
  break
 }
}'''.replace("PID_VALUE", str(app.pid))
    window_id = run("swift", "-e", query, timeout=30)
    if not window_id:
        raise AssertionError("native CGWindow missing")
    path = out / name
    subprocess.run(["screencapture", "-x", "-o", "-l", window_id, str(path)], check=True, timeout=30)
    item = {"file": name, "pid": app.pid, "window_id": int(window_id), "sha256": digest(path)}
    manifest.setdefault("captures", []).append(item)
    return item


def set_size_and_capture(app, width, height, manifest):
    ax(app, "set frontmost to true\nset size of window 1 to {" + str(width) + ", " + str(height) + "}")
    geometry = {}
    wait_until(
        lambda: (geometry.update(window_geometry(app)) or True) and geometry["width"] == width and geometry["height"] == height,
        "native window did not reach " + str(width) + "x" + str(height),
    )
    manifest.setdefault("window_sizes", []).append({"requested": {"width": width, "height": height}, "actual": geometry.copy()})
    return geometry


def quote(path):
    return shlex.quote(str(path))


def read_pid(path):
    value = path.read_text(encoding="utf-8").strip()
    if not value.isdigit():
        raise AssertionError("invalid PID marker in " + str(path) + ": " + value)
    return int(value)


def pid_present(pid):
    completed = subprocess.run(["ps", "-p", str(pid), "-o", "pid="], text=True, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=10, check=False)
    return bool(completed.stdout.strip())


def ps_record(pid):
    completed = subprocess.run(
        ["ps", "-p", str(pid), "-o", "pid=,ppid=,pgid=,stat=,command="],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        timeout=10,
        check=False,
    )
    return completed.stdout.strip()


def terminate_pid(pid):
    if not pid_present(pid):
        return
    for sig in (signal.SIGTERM, signal.SIGKILL):
        try:
            os.kill(pid, sig)
        except ProcessLookupError:
            return
        try:
            wait_until(lambda: not pid_present(pid), "PID " + str(pid) + " did not exit", timeout=5)
            return
        except AssertionError:
            continue


def quit_app(app):
    if app is None or app.poll() is not None:
        return
    names = [name.strip() for name in ax(app, "return name of every menu bar item of menu bar 1").split(", ") if name.strip()]
    if len(names) < 2:
        raise AssertionError("application menu bar is unavailable: " + repr(names))
    menu = json.dumps(names[1], ensure_ascii=False)
    ax(
        app,
        "click menu bar item "
        + menu
        + " of menu bar 1\nclick menu item \"退出 GCode\" of menu 1 of menu bar item "
        + menu
        + " of menu bar 1",
    )
    if app.wait(timeout=15) != 0:
        raise AssertionError("application menu exit returned " + str(app.returncode))


def launch(name, env, manifest):
    log_path = out / name
    log = log_path.open("w", encoding="utf-8")
    app = subprocess.Popen([str(isolated_exe)], cwd=ROOT, env=env, stdout=log, stderr=log)
    manifest.setdefault("applications", []).append({"pid": app.pid, "log": name})
    return app, log


def fixture_text(path):
    return path.read_text(encoding="utf-8").strip()


def validate_identity():
    for path, label in ((source_exe, "source executable"), (isolated_exe, "isolated executable")):
        if not path.is_file():
            raise AssertionError(label + " missing: " + str(path))
    if not data_directory.is_dir():
        raise AssertionError("workspace data directory missing: " + str(data_directory))
    expected_resources = workspace_manifest.get("resource_sha256")
    if expected_resources and resources(isolated_bundle) != expected_resources:
        raise AssertionError("isolated bundle resources differ from workspace manifest")
    if expected_resources and resources(source_bundle) != expected_resources:
        raise AssertionError("source bundle resources differ from workspace manifest")
    expected_code = workspace_manifest.get("executable_code_sha256")
    if expected_code and code_digest(isolated_exe) != expected_code:
        raise AssertionError("isolated executable code differs from workspace manifest")
    if expected_code and code_digest(source_exe) != expected_code:
        raise AssertionError("source executable code differs from workspace manifest")
    expected_executable = workspace_manifest.get("original_executable_sha256")
    if expected_executable and digest(source_exe) != expected_executable:
        raise AssertionError("source executable hash differs from workspace manifest")
    expected_source = workspace_manifest.get("source_files_at_start")
    if expected_source and source_files() != expected_source:
        raise AssertionError("source files differ from workspace manifest")
    return {
        "source_executable_sha256": digest(source_exe),
        "isolated_executable_sha256": digest(isolated_exe),
        "source_executable_code_sha256": code_digest(source_exe),
        "isolated_executable_code_sha256": code_digest(isolated_exe),
        "resource_sha256": resources(isolated_bundle),
        "source_files_match_workspace_manifest": True,
    }


def main():
    marker = {
        "cwd": fixture / "shell-cwd.txt",
        "pid": fixture / "shell.pid",
        "ready": fixture / "shell-ready.txt",
        "shell_edit_ready": fixture / "shell-edit-ready.txt",
        "resize": fixture / "resize.txt",
        "resize_ready": fixture / "resize-ready.txt",
        "scroll_ready": fixture / "scroll-ready.txt",
        "long_shell_pid": fixture / "long-shell.pid",
        "long_child_pid": fixture / "long-child.pid",
    }
    for path in marker.values():
        path.unlink(missing_ok=True)

    manifest = {
        "status": "RUNNING",
        "workspace_manifest": str(workspace_manifest_path),
        "source_bundle": str(source_bundle),
        "isolated_bundle": str(isolated_bundle),
        "data_directory": str(data_directory),
        "fixture_directory": str(fixture),
        "real_api": "NOT TESTED",
        "physical_keyboard_ime": "NOT TESTED",
        "clipboard": "NOT TOUCHED",
        "network": "NOT USED",
        "captures": [],
        "marker_paths": {key: str(path) for key, path in marker.items()},
    }
    results = {
        "real_api": "NOT TESTED",
        "physical_keyboard_ime": "NOT TESTED",
        "clipboard": "NOT TOUCHED",
        "terminal_ax": "NOT RUN",
        "focus": "NOT RUN",
        "keyboard": "NOT RUN",
        "cwd": "NOT RUN",
        "resize": "NOT RUN",
        "scrollback": "NOT RUN",
        "shell_exit": "NOT RUN",
        "menu_exit_cleanup": "NOT RUN",
        "shell_edit_external_refresh": "NOT RUN",
    }
    write_json("manifest.json", manifest)
    write_json("results.json", results)

    apps = []
    logs = []
    shell_pids = []
    child_pids = []
    original_input_source = input_source("current")
    manifest["input_source_before"] = original_input_source
    phase = "identity"
    try:
        if not workspace_manifest_path.is_file():
            raise AssertionError("workspace manifest missing: " + str(workspace_manifest_path))
        if not isolated_bundle.is_dir() or not source_bundle.is_dir():
            raise AssertionError("workspace manifest bundle paths are unavailable")

        identity = validate_identity()
        manifest["identity"] = identity
        env = os.environ.copy()
        env["GCODE_CRED_STORAGE"] = "encrypted-file"
        for key in ("GCODE_VISUAL_SAMPLE", "GCODE_EDITOR", "VISUAL", "EDITOR"):
            env.pop(key, None)

        phase = "first-launch"
        app, log = launch("application.log", env, manifest)
        apps.append(app)
        logs.append(log)
        manifest["first_launch_pid"] = app.pid
        wait_until(lambda: app_ready(app), "native terminal workbench did not become ready")
        ax(app, "set frontmost to true")
        ax(app, 'click radio button "终端" of tab group 1 of window 1')

        phase = "terminal-ax"
        first_ax = observe_entire_contents(app)
        (out / "terminal-ax-entire-first.txt").write_text(first_ax + "\n", encoding="utf-8")
        terminal = None
        final_ax = first_ax

        def terminal_ready():
            nonlocal terminal, final_ax
            final_ax = observe_entire_contents(app)
            try:
                terminal = locate_terminal(app, final_ax)
                return True
            except (AssertionError, subprocess.CalledProcessError):
                return False

        wait_until(terminal_ready, "Terminal AX group did not appear")
        observed_path = observed_terminal_path(final_ax, terminal["path_from_candidate"])
        terminal["observed_path"] = observed_path
        terminal["entire_contents_file"] = "terminal-ax-entire.txt"
        (out / "terminal-ax-entire.txt").write_text(final_ax + "\n", encoding="utf-8")
        write_json("terminal-ax.json", terminal)
        manifest["terminal_ax"] = terminal
        results["terminal_ax"] = "PASS"

        phase = "focus-and-keyboard"
        manifest["first_focus"] = click_terminal_and_record_focus(app, terminal)
        write_json("focus.json", manifest["first_focus"])
        results["focus"] = "PASS"
        capture(app, "terminal-before-input-native.png", manifest)
        command = (
            "printf '%s\\n' \"$PWD\" > "
            + quote(marker["cwd"])
            + "; printf '%s\\n' \"$$\" > "
            + quote(marker["pid"])
            + "; printf 'NATIVE_READY\\n' > "
            + quote(marker["ready"])
        )
        type_command(app, command)
        capture(app, "terminal-after-input-native.png", manifest)
        wait_until(lambda: marker["ready"].is_file() and marker["pid"].is_file(), "native GUI keyboard marker did not arrive")
        shell_pid = read_pid(marker["pid"])
        shell_pids.append(shell_pid)
        cwd_value = fixture_text(marker["cwd"])
        expected_cwd = pathlib.Path(workspace_manifest.get("project", cwd_value)).resolve()
        if pathlib.Path(cwd_value).resolve() != expected_cwd:
            raise AssertionError("native terminal cwd mismatch: " + cwd_value + " != " + str(expected_cwd))
        manifest["first_shell"] = {"pid": shell_pid, "ps": ps_record(shell_pid), "cwd": cwd_value}
        results["keyboard"] = "PASS"
        results["cwd"] = "PASS"

        if options.glyph_probe_only:
            phase = "glyph-probe"
            ready = fixture / "glyph-ready.txt"
            output = fixture / "glyph-output.txt"
            ready.unlink(missing_ok=True)
            codepoints = (0xE0A0, 0xE0B0, 0xF07C, 0xF120)
            text = "\nGCODE_FONT_PROBE\nASCII=ABC abc 0123\nCJK=中文终端\n" + "".join(f"U_{point:04X}={chr(point)}\n" for point in codepoints)
            escaped = "".join(f"\\{byte:03o}" for byte in text.encode("utf-8"))
            type_command(app, "printf '" + escaped + "' | tee " + quote(output) + "; printf 'FONT_READY\\n' > " + quote(ready))
            wait_until(lambda: ready.is_file(), "font probe shell marker did not arrive")
            if output.read_text(encoding="utf-8") != text:
                raise AssertionError("font probe UTF-8 output differs from fixed input")
            capture(app, "terminal-glyph-probe-native.png", manifest)
            manifest["glyph_probe"] = {"codepoints": [f"U+{point:04X}" for point in codepoints], "output_file": str(output), "output_sha256": digest(output), "utf8_exact": True, "visual_review": "REQUIRED"}
            results["glyph_probe"] = "CAPTURED; VISUAL_REVIEW_REQUIRED"
            quit_app(app)
            wait_until(lambda: not pid_present(shell_pid), "glyph probe shell survived application exit")
            manifest["first_application_exit_code"] = app.returncode
            manifest["first_shell"]["gone_after_menu_exit"] = True
            results["menu_exit_cleanup"] = "PASS"
            results["result"] = "PASS_PENDING_VISUAL_REVIEW"
            manifest["status"] = "PASS_PENDING_VISUAL_REVIEW"
            return

        phase = "shell-edit-refresh"
        textedit = workspace_manifest.get("textedit") or {}
        qa_file = pathlib.Path(textedit.get("qa_file", "")).resolve()
        if not qa_file.is_file():
            raise AssertionError("workspace manifest textedit.qa_file is unavailable: " + str(qa_file))
        qa_identity = workspace_manifest.get("qa_identity", isolated_bundle.stem)
        shell_edit_marker = "QA_SHELL_EDIT_" + str(qa_identity)
        type_command(
            app,
            "printf '" + shell_edit_marker + "\\n' >> " + quote(qa_file) + "; printf 'SHELL_EDIT_READY\\n' > " + quote(marker["shell_edit_ready"]),
        )
        wait_until(lambda: marker["shell_edit_ready"].is_file(), "native shell edit marker did not arrive")
        if shell_edit_marker not in qa_file.read_text(encoding="utf-8"):
            raise AssertionError("native shell edit did not reach the workspace file")
        file_name = qa_file.name
        file_button = 'button "打开文件 ' + file_name + '" of scroll area 2 of window 1'
        ax(app, 'click radio button "文件" of tab group 1 of window 1')
        wait_until(lambda: exists(app, 'button "刷新工作区" of window 1'), "workspace file tab did not render")
        ax(app, 'click button "刷新工作区" of window 1')
        wait_until(lambda: exists(app, file_button), "shell-edited file did not appear in workspace file list")
        ax(app, "click " + file_button)
        full_file_text = qa_file.read_text(encoding="utf-8")
        wait_until(lambda: exists(app, text_ref(full_file_text, 3)), "workspace preview did not read back the shell edit")
        ax(app, 'click radio button "Diff" of tab group 1 of window 1')
        diff_marker = 'static text ' + json.dumps(shell_edit_marker, ensure_ascii=False) + ' of scroll area 1 of scroll area 2 of window 1'
        wait_until(lambda: exists(app, diff_marker), "Git Diff did not expose the shell edit marker")
        capture(app, "shell-edit-diff-native.png", manifest)
        manifest["shell_edit"] = {"file": str(qa_file), "marker": shell_edit_marker, "preview_readback": True, "diff_readback": True, "capture": "shell-edit-diff-native.png"}
        results["shell_edit_external_refresh"] = "PASS"
        ax(app, 'click radio button "终端" of tab group 1 of window 1')
        final_ax = observe_entire_contents(app)
        terminal = locate_terminal(app, final_ax)
        terminal["observed_path"] = observed_terminal_path(final_ax, terminal["path_from_candidate"])
        manifest["terminal_ax_after_file_refresh"] = terminal

        phase = "resize-and-scroll"
        scroll_command = (
            "i=1; while [ $i -le 180 ]; do printf 'GCODE_SCROLL_MARKER_%03d\\n' \"$i\"; i=$((i+1)); done; printf 'SCROLL_READY\\n' > "
            + quote(marker["scroll_ready"])
        )
        type_command(app, scroll_command)
        wait_until(lambda: marker["scroll_ready"].is_file(), "scrollback fixture did not finish")
        resize_records = []
        for width, height in ((1440, 900), (1280, 800)):
            geometry = set_size_and_capture(app, width, height, manifest)
            click_terminal_and_record_focus(app, terminal)
            size_label = str(width) + "x" + str(height)
            type_command(
                app,
                "printf 'SIZE_" + size_label + " %s\\n' \"$(stty size)\" >> " + quote(marker["resize"]) + "; printf 'SIZE_READY_" + size_label + "\\n' > " + quote(marker["resize_ready"]),
            )
            wait_until(lambda: marker["resize_ready"].is_file(), "stty marker did not arrive for " + size_label)
            text = marker["resize"].read_text(encoding="utf-8")
            match = re.search(r"SIZE_" + re.escape(size_label) + r"\s+(\d+)\s+(\d+)", text)
            if not match:
                raise AssertionError("stty size marker missing for " + size_label)
            resize_records.append({"requested_window": {"width": width, "height": height}, "actual_window": geometry, "stty_rows": int(match.group(1)), "stty_cols": int(match.group(2))})
            capture(app, "terminal-" + size_label + "-bottom-native.png", manifest)
            click_terminal_and_record_focus(app, terminal)
            ax(app, "key code 116 using {shift down}")
            time.sleep(0.5)
            capture(app, "terminal-" + size_label + "-pageup-native.png", manifest)
            ax(app, "key code 121 using {shift down}")
            time.sleep(0.5)
            capture(app, "terminal-" + size_label + "-pagedown-native.png", manifest)
            marker["resize_ready"].unlink(missing_ok=True)
        manifest["resize"] = resize_records
        manifest["scrollback"] = {"sequence": "GCODE_SCROLL_MARKER_001..180", "key_events": ["Shift+PageUp", "Shift+PageDown"], "visual_review": "REQUIRED"}
        results["resize"] = "PASS"
        results["scrollback"] = "CAPTURED; VISUAL_REVIEW_REQUIRED"

        phase = "first-shell-exit"
        click_terminal_and_record_focus(app, terminal)
        type_command(app, "exit")
        wait_until(lambda: not pid_present(shell_pid), "interactive shell PID did not disappear after exit")
        manifest["first_shell"]["exit_pid_record"] = {"pid_before": shell_pid, "pid_after": ps_record(shell_pid), "gone": True}
        results["shell_exit"] = "PASS"
        quit_app(app)
        manifest["first_application_exit_code"] = app.returncode

        phase = "restart-long-sleep"
        app, log = launch("application-restart.log", env, manifest)
        apps.append(app)
        logs.append(log)
        manifest["restart_pid"] = app.pid
        wait_until(lambda: app_ready(app), "restarted native workbench did not become ready")
        ax(app, "set frontmost to true")
        ax(app, 'click radio button "终端" of tab group 1 of window 1')
        restart_ax = observe_entire_contents(app)
        (out / "terminal-restart-ax-entire.txt").write_text(restart_ax + "\n", encoding="utf-8")
        restart_terminal = None

        def restart_terminal_ready():
            nonlocal restart_ax, restart_terminal
            restart_ax = observe_entire_contents(app)
            try:
                restart_terminal = locate_terminal(app, restart_ax)
                return True
            except (AssertionError, subprocess.CalledProcessError):
                return False

        wait_until(restart_terminal_ready, "restarted Terminal AX group did not appear")
        (out / "terminal-restart-ax-entire.txt").write_text(restart_ax + "\n", encoding="utf-8")
        restart_path = observed_terminal_path(restart_ax, restart_terminal["path_from_candidate"])
        restart_terminal["observed_path"] = restart_path
        manifest["restart_terminal_ax"] = restart_terminal
        write_json("terminal-restart-ax.json", restart_terminal)
        click_terminal_and_record_focus(app, restart_terminal)
        long_command = (
            "( sleep 300 ) & child=$!; printf '%s\\n' \"$$\" > "
            + quote(marker["long_shell_pid"])
            + "; printf '%s\\n' \"$child\" > "
            + quote(marker["long_child_pid"])
            + "; wait"
        )
        type_command(app, long_command)
        wait_until(lambda: marker["long_shell_pid"].is_file() and marker["long_child_pid"].is_file(), "long sleep PID markers did not arrive")
        long_shell_pid = read_pid(marker["long_shell_pid"])
        long_child_pid = read_pid(marker["long_child_pid"])
        shell_pids.append(long_shell_pid)
        child_pids.append(long_child_pid)
        manifest["long_sleep"] = {"shell_pid": long_shell_pid, "child_pid": long_child_pid, "shell_ps_before_exit": ps_record(long_shell_pid), "child_ps_before_exit": ps_record(long_child_pid)}
        capture(app, "terminal-long-sleep-native.png", manifest)

        phase = "menu-exit-cleanup"
        quit_app(app)
        wait_until(lambda: not pid_present(long_shell_pid), "long-running terminal shell PID did not disappear after application exit")
        wait_until(lambda: not pid_present(long_child_pid), "long-running terminal child PID did not disappear after application exit")
        manifest["long_sleep"].update(shell_ps_after_exit=ps_record(long_shell_pid), child_ps_after_exit=ps_record(long_child_pid), shell_gone=True, child_gone=True)
        results["menu_exit_cleanup"] = "PASS"
        results["result"] = "PASS_PENDING_VISUAL_REVIEW"
        manifest["status"] = "PASS_PENDING_VISUAL_REVIEW"
    except Exception as error:
        manifest["status"] = "FAIL"
        manifest["failure"] = {"phase": phase, "error": str(error), "type": type(error).__name__}
        results["result"] = "FAIL"
        results["failure"] = manifest["failure"]
        if apps and apps[-1].poll() is None:
            try:
                capture(apps[-1], "failure-native.png", manifest)
            except Exception as capture_error:
                manifest["failure_capture_error"] = str(capture_error)
        raise
    finally:
        for app in reversed(apps):
            if app.poll() is None:
                try:
                    quit_app(app)
                except Exception as error:
                    manifest.setdefault("cleanup_errors", []).append({"pid": app.pid, "error": str(error)})
                    try:
                        app.terminate()
                        app.wait(timeout=10)
                    except Exception:
                        try:
                            app.kill()
                            app.wait(timeout=10)
                        except Exception:
                            pass
        for pid in reversed(child_pids + shell_pids):
            if pid_present(pid):
                manifest.setdefault("cleanup_pids", []).append({"pid": pid, "ps_before_cleanup": ps_record(pid)})
                terminate_pid(pid)
        for log in logs:
            log.close()
        try:
            manifest["input_source_after"] = input_source("select", original_input_source)
            manifest["input_source_restored"] = manifest["input_source_after"] == original_input_source
        except Exception as error:
            manifest.setdefault("cleanup_errors", []).append({"phase": "input-source-restore", "error": str(error)})
            manifest["input_source_restored"] = False
            results["result"] = "FAIL"
        try:
            if "identity" not in manifest:
                manifest["source_integrity_final"] = {"status": "NOT RUN; identity validation failed"}
                manifest["original_bundle_unchanged"] = False
                manifest["source_unchanged"] = False
            else:
                manifest["source_integrity_final"] = {
                    "source_executable_sha256": digest(source_exe),
                    "isolated_executable_sha256": digest(isolated_exe),
                    "source_resources_match": resources(source_bundle) == manifest["identity"]["resource_sha256"],
                    "isolated_resources_match": resources(isolated_bundle) == manifest["identity"]["resource_sha256"],
                    "source_files_match_workspace_manifest": source_files() == workspace_manifest.get("source_files_at_start"),
                }
                manifest["original_bundle_unchanged"] = digest(source_exe) == manifest["identity"]["source_executable_sha256"] and manifest["source_integrity_final"]["source_resources_match"]
                manifest["source_unchanged"] = manifest["source_integrity_final"]["source_files_match_workspace_manifest"]
        except Exception as error:
            manifest.setdefault("cleanup_errors", []).append({"phase": "final-integrity", "error": str(error)})
            manifest["original_bundle_unchanged"] = False
            manifest["source_unchanged"] = False
        write_json("manifest.json", manifest)
        write_json("results.json", results)
        (out / "resize.txt").write_text(marker["resize"].read_text(encoding="utf-8") if marker["resize"].exists() else "", encoding="utf-8")


try:
    main()
except Exception as error:
    print(json.dumps({"result": "FAIL", "error": str(error)}, ensure_ascii=False, indent=2), flush=True)
    raise
else:
    print(json.dumps(json.loads((out / "results.json").read_text(encoding="utf-8")), ensure_ascii=False, indent=2), flush=True)
