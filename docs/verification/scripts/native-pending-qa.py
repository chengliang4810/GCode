"""Run the native pending permission/ask_user acceptance fixture.

The driver is deliberately a small, macOS-only harness around the packaged
GCode application.  It clones the formal app bundle, changes only bundle
identity metadata, ad-hoc signs that clone, and starts a loopback-only
Chat Completions fixture.  The fixture drives the real runtime; System Events
is used only for the app's own window and controls.

``--self-test`` exercises the fixture protocol helpers without opening a
native window or binding an HTTP socket.  The default command is the real
native run and is intentionally left for the acceptance operator.
"""

from __future__ import annotations

import argparse
import hashlib
import http.server
import json
import os
import pathlib
import plistlib
import shutil
import struct
import subprocess
import sys
import tempfile
import threading
import time
import unittest
from typing import Any


ROOT = pathlib.Path(__file__).resolve().parents[3]
FIXTURE_CONFIG = ROOT / "output/acceptance/native-pending-ui/_fixture/scenarios.json"
LOCAL_FIXTURE_KEY = "gcode-native-pending-local-fixture-key"
DEFAULT_BUNDLE = "build/darwin-amd64/GCode.app"
DEFAULT_OUTPUT = "output/acceptance/native-pending-ui"
MAX_AX_SECONDS = 12
MAX_WAIT_SECONDS = 35


def digest(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def code_digest(path: pathlib.Path) -> str:
    """Hash __TEXT,__text, which ad-hoc signing is not allowed to change."""
    raw = path.read_bytes()
    magic = struct.unpack_from("<I", raw)[0]
    if magic != 0xFEEDFACF:
        raise AssertionError("正式包必须是单架构 64-bit Mach-O")
    header = struct.unpack_from("<IiiIIIII", raw)
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
    raise AssertionError("Mach-O 缺少 __TEXT,__text")


def resource_digests(bundle: pathlib.Path) -> dict[str, str]:
    base = bundle / "Contents/Resources"
    return {
        str(path.relative_to(base)): digest(path)
        for path in sorted(base.rglob("*"))
        if path.is_file()
    }


def source_digests() -> dict[str, str]:
    paths = [ROOT / name for name in ("go.mod", "go.sum", "mygo.json", "third_party/mygo/go.mod", "third_party/mygo/go.sum")]
    for directory in ("cmd", "internal", "third_party/mygo"):
        paths.extend(path for path in (ROOT / directory).rglob("*.go") if not path.name.endswith("_test.go"))
    for directory in ("internal/desktop/fonts", "resources"):
        paths.extend(path for path in (ROOT / directory).rglob("*") if path.is_file())
    return {str(path.relative_to(ROOT)): digest(path) for path in sorted(paths)}


def json_string(value: str) -> str:
    return json.dumps(value, ensure_ascii=False)


def run_command(*args: str, cwd: pathlib.Path = ROOT, timeout: int = MAX_AX_SECONDS) -> str:
    result = subprocess.run(args, cwd=cwd, text=True, capture_output=True, timeout=timeout, check=True)
    return result.stdout.strip()


class AxError(RuntimeError):
    pass


def ax(app: subprocess.Popen[str], body: str, timeout: int = MAX_AX_SECONDS) -> str:
    """Run AppleScript against exactly this process, never the front window."""
    if app.poll() is not None:
        raise AxError(f"应用已退出，退出码={app.returncode}")
    script = (
        'tell application "System Events" to tell (first application process whose unix id is '
        + str(app.pid)
        + ")\n"
        + body
        + "\nend tell"
    )
    try:
        result = subprocess.run(
            ["osascript", "-e", script],
            cwd=ROOT,
            text=True,
            capture_output=True,
            timeout=timeout,
            check=True,
        )
    except subprocess.TimeoutExpired as error:
        raise AxError("osascript 超时") from error
    except subprocess.CalledProcessError as error:
        detail = (error.stderr or error.stdout or "").strip()
        raise AxError(f"osascript 失败: {detail[:240]}") from error
    return result.stdout.strip()


def wait_until(condition, label: str, timeout: float = MAX_WAIT_SECONDS) -> None:
    deadline = time.monotonic() + timeout
    last_error = ""
    while time.monotonic() < deadline:
        try:
            if condition():
                return
        except (AssertionError, AxError, FileNotFoundError, json.JSONDecodeError, OSError) as error:
            last_error = str(error)
        time.sleep(0.1)
    suffix = f"；最后错误：{last_error}" if last_error else ""
    raise AssertionError(f"{label}{suffix}")


def state(data_dir: pathlib.Path) -> dict[str, Any]:
    raw = json.loads((data_dir / "controller.json").read_text())
    return raw["snapshot"]


def session_from(snapshot: dict[str, Any], session_id: str) -> dict[str, Any]:
    for session in snapshot.get("sessions") or []:
        if session.get("id") == session_id:
            return session
    raise AssertionError(f"session {session_id!r} 不存在")


def newest_session(snapshot: dict[str, Any]) -> dict[str, Any]:
    sessions = snapshot.get("sessions") or []
    if not sessions:
        raise AssertionError("会话尚未创建")
    return sessions[-1]


def last_user_text(payload: dict[str, Any]) -> str:
    for message in reversed(payload.get("messages", [])):
        if message.get("role") != "user":
            continue
        content = message.get("content", "")
        if isinstance(content, str):
            return content
        return json.dumps(content, ensure_ascii=False)
    return ""


def content_text(value: Any) -> str:
    """Return only message content, without searching tool metadata."""
    if isinstance(value, str):
        return value
    if isinstance(value, list):
        parts: list[str] = []
        for item in value:
            if isinstance(item, str):
                parts.append(item)
            elif isinstance(item, dict):
                if isinstance(item.get("text"), str):
                    parts.append(item["text"])
                elif "content" in item:
                    parts.append(content_text(item["content"]))
        return "\n".join(part for part in parts if part)
    if value is None:
        return ""
    return json.dumps(value, ensure_ascii=False)


def assistant_function_call_ids(payload: dict[str, Any], function_name: str) -> list[str]:
    """Find exact assistant function call IDs for a function name."""
    call_ids: list[str] = []
    for message in payload.get("messages", []):
        if not isinstance(message, dict) or message.get("role") != "assistant":
            continue
        candidates: list[Any] = []
        tool_calls = message.get("tool_calls")
        if isinstance(tool_calls, list):
            candidates.extend(tool_calls)
        function_call = message.get("function_call")
        if isinstance(function_call, dict):
            candidates.append(function_call)
        direct_function = message.get("function")
        if isinstance(direct_function, dict):
            candidates.append(direct_function)
        for call in candidates:
            if not isinstance(call, dict):
                continue
            function = call.get("function") if isinstance(call.get("function"), dict) else call
            if function.get("name") != function_name:
                continue
            call_id = call.get("id") or call.get("call_id") or function.get("id") or function.get("call_id")
            if isinstance(call_id, str) and call_id and call_id not in call_ids:
                call_ids.append(call_id)
    return call_ids


def tool_result_contents(payload: dict[str, Any], call_id: str) -> list[str]:
    """Return content from tool messages linked to exactly one call ID."""
    contents: list[str] = []
    for message in payload.get("messages", []):
        if not isinstance(message, dict):
            continue
        if message.get("role") != "tool" or message.get("tool_call_id") != call_id:
            continue
        contents.append(content_text(message.get("content")))
    return contents


def tool_result_contains(payload: dict[str, Any], call_id: str, expected: str) -> bool:
    expected = expected.casefold()
    return any(expected in content.casefold() for content in tool_result_contents(payload, call_id))


def any_tool_result_contains(payload: dict[str, Any], expected: str) -> bool:
    expected = expected.casefold()
    for message in payload.get("messages", []):
        if not isinstance(message, dict) or message.get("role") != "tool":
            continue
        if expected in content_text(message.get("content")).casefold():
            return True
    return False


def sse_event(delta: dict[str, Any], finish: str) -> bytes:
    event = {"choices": [{"index": 0, "delta": delta, "finish_reason": finish}]}
    return ("data: " + json.dumps(event, ensure_ascii=False) + "\n\n").encode()


def sse_tool_call(call_id: str, name: str, arguments: dict[str, Any]) -> bytes:
    encoded = json.dumps(arguments, ensure_ascii=False, separators=(",", ":"))
    delta = {
        "tool_calls": [
            {
                "index": 0,
                "id": call_id,
                "type": "function",
                "function": {"name": name, "arguments": encoded},
            }
        ]
    }
    return sse_event(delta, "tool_calls") + b"data: [DONE]\n\n"


def sse_text(text: str) -> bytes:
    return sse_event({"content": text}, "stop") + b"data: [DONE]\n\n"


class FixtureState:
    """In-memory server state; only redacted booleans are written as evidence."""

    def __init__(self, scenario: str, project: pathlib.Path):
        self.scenario = scenario
        self.project = project
        self.lock = threading.Lock()
        self.records: list[dict[str, Any]] = []
        self.errors: list[str] = []
        self.step = 0

    def record_error(self, message: str) -> None:
        with self.lock:
            self.errors.append(message)

    def next_step(self, record: dict[str, Any]) -> int:
        with self.lock:
            self.step += 1
            record["step"] = self.step
            self.records.append(record)
            return self.step

    def validate_and_response(self, payload: dict[str, Any], auth: str) -> bytes:
        if auth != "Bearer " + LOCAL_FIXTURE_KEY:
            self.record_error("provider Authorization 不匹配本地 fixture")
            raise ValueError("invalid local fixture authorization")
        marker = last_user_text(payload)
        ask_user_call_ids = assistant_function_call_ids(payload, "ask_user")
        contains_production_answer = any(
            tool_result_contains(payload, call_id, "production") for call_id in ask_user_call_ids
        )
        record = {
            "scenario": self.scenario,
            "last_user_marker": marker[:120],
            "contains_production_answer": contains_production_answer,
            "contains_permission_denied": tool_result_contains(payload, "deny-call", "permission denied"),
            "contains_stale_answer": any_tool_result_contains(payload, "stale"),
            "message_count": len(payload.get("messages", [])),
        }
        step = self.next_step(record)
        if self.scenario == "permission-allow-deny":
            if step == 1:
                return sse_tool_call(
                    "allow-call",
                    "exec_command",
                    {
                        "cmd": "printf 'approved content\\n' > permission-allowed.txt",
                        "sandbox_permissions": "require_escalated",
                        "yield_time_ms": 1000,
                    },
                )
            if step == 2:
                return sse_tool_call(
                    "deny-call",
                    "exec_command",
                    {
                        "cmd": "printf 'must not be written\\n' > permission-denied.txt",
                        "sandbox_permissions": "require_escalated",
                        "yield_time_ms": 1000,
                    },
                )
            if step == 3:
                if not record["contains_permission_denied"]:
                    self.record_error("拒绝结果没有进入下一次 provider 请求")
                return sse_text("permission flow completed")
        elif self.scenario == "ask-history":
            if step == 1:
                return sse_tool_call(
                    "ask-history-call",
                    "ask_user",
                    {
                        "header": "部署环境",
                        "questions": [
                            {
                                "header": "环境",
                                "question": "选择要部署的环境。",
                                "options": ["staging", "production"],
                            }
                        ],
                    },
                )
            if step == 2:
                if not record["contains_production_answer"]:
                    self.record_error("ask_user 回答 production 没有进入下一次 provider 请求")
                return sse_text("已记录部署环境 production")
        elif self.scenario == "permission-correction":
            if step == 1:
                if "permission-correction-old" not in marker:
                    self.record_error("permission correction 首次 prompt marker 缺失")
                return sse_tool_call(
                    "old-permission-call",
                    "exec_command",
                    {
                        "cmd": "printf 'old\\n' > permission-old.txt",
                        "sandbox_permissions": "require_escalated",
                        "yield_time_ms": 1000,
                    },
                )
            if step == 2:
                if "permission-correction-new" not in marker:
                    self.record_error("permission correction replacement 未使用新 prompt")
                return sse_tool_call(
                    "new-permission-call",
                    "exec_command",
                    {
                        "cmd": "printf 'new\\n' > permission-new.txt",
                        "sandbox_permissions": "require_escalated",
                        "yield_time_ms": 1000,
                    },
                )
            if step == 3:
                if record["contains_stale_answer"]:
                    self.record_error("迟到 permission answer 进入 replacement provider 请求")
                return sse_text("permission replacement completed")
        elif self.scenario == "ask-correction":
            if step == 1:
                if "ask-correction-old" not in marker:
                    self.record_error("ask correction 首次 prompt marker 缺失")
                return sse_tool_call(
                    "old-ask-call",
                    "ask_user",
                    {"questions": [{"question": "ask-correction-old question", "options": ["old answer"]}]},
                )
            if step == 2:
                if "ask-correction-new" not in marker:
                    self.record_error("ask correction replacement 未使用新 prompt")
                return sse_tool_call(
                    "new-ask-call",
                    "ask_user",
                    {"questions": [{"question": "ask-correction-new question", "options": ["replacement answer"]}]},
                )
            if step == 3:
                if record["contains_stale_answer"]:
                    self.record_error("迟到 ask answer 进入 replacement provider 请求")
                return sse_text("ask-correction-new completed")
        self.record_error(f"{self.scenario} 收到意外 provider step {step}")
        raise ValueError("unexpected fixture step")


class FixtureHandler(http.server.BaseHTTPRequestHandler):
    server: "FixtureHTTPServer"

    def log_message(self, _format: str, *_args: Any) -> None:
        return

    def do_POST(self) -> None:  # noqa: N802 - BaseHTTPRequestHandler API
        fixture = self.server.fixture
        length = int(self.headers.get("Content-Length", "0"))
        raw = self.rfile.read(length)
        if self.path != "/v1/chat/completions":
            fixture.record_error(f"unexpected path {self.path}")
            self.send_error(http.server.HTTPStatus.NOT_FOUND)
            return
        try:
            payload = json.loads(raw)
            response = fixture.validate_and_response(payload, self.headers.get("Authorization", ""))
        except (ValueError, json.JSONDecodeError, TypeError) as error:
            self.send_error(http.server.HTTPStatus.BAD_REQUEST, "invalid local fixture request")
            fixture.record_error(str(error))
            return
        self.send_response(http.server.HTTPStatus.OK)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Content-Length", str(len(response)))
        self.end_headers()
        self.wfile.write(response)
        self.wfile.flush()


class FixtureHTTPServer(http.server.ThreadingHTTPServer):
    daemon_threads = True
    allow_reuse_address = True

    def __init__(self, fixture: FixtureState):
        super().__init__(("127.0.0.1", 0), FixtureHandler)
        self.fixture = fixture


def wait_state(data_dir: pathlib.Path, predicate, label: str, timeout: float = MAX_WAIT_SECONDS) -> dict[str, Any]:
    last: dict[str, Any] = {}

    def check() -> bool:
        nonlocal last
        last = state(data_dir)
        return bool(predicate(last))

    wait_until(check, label, timeout)
    return last


def exists_ax(app: subprocess.Popen[str], reference: str) -> bool:
    return ax(app, "return exists " + reference) == "true"


def ax_has_name(app: subprocess.Popen[str], name: str, role: str | None = None) -> bool:
    target_name = json_string(name)
    role_clause = "true"
    if role:
        short_role = {
            "AXButton": "button",
            "AXStaticText": "static text",
            "AXTextArea": "text area",
        }.get(role, role)
        role_clause = f"(nodeRole is {json_string(role)} or nodeRole is {json_string(short_role)})"
    script = f'''set targetName to {target_name}
set found to false
set allNodes to get entire contents of window 1
repeat with nodeRef in allNodes
 try
  set nodeElement to contents of nodeRef
  set nodeRole to role of nodeElement
  set nodeName to ""
  try
   set nodeName to name of nodeElement
  end try
  if nodeName is missing value or nodeName is "" then
   try
    set nodeName to value of nodeElement
   end try
  end if
  if nodeName is targetName and {role_clause} then
   set found to true
   exit repeat
  end if
 end try
end repeat
return found'''
    return ax(app, script) == "true"


def ax_press_name(app: subprocess.Popen[str], name: str, role: str = "AXButton") -> None:
    target_name = json_string(name)
    short_role = {"AXButton": "button"}.get(role, role)
    script = f'''set targetName to {target_name}
set pressed to false
set allNodes to get entire contents of window 1
repeat with nodeRef in allNodes
 try
  set nodeElement to contents of nodeRef
  set nodeName to name of nodeElement
  set nodeRole to role of nodeElement
  if nodeName is targetName and (nodeRole is {json_string(role)} or nodeRole is {json_string(short_role)}) then
   perform action "AXPress" of nodeElement
   set pressed to true
   exit repeat
  end if
 end try
end repeat
if pressed is false then error "AX control not found: " & targetName
return "pressed"'''
    ax(app, script)


def ensure_permission_card(app: subprocess.Popen[str]) -> None:
    wait_until(lambda: ax_has_name(app, "需要你的授权", "AXStaticText"), "授权卡片没有出现在原生窗口")


def ensure_question_card(app: subprocess.Popen[str]) -> None:
    wait_until(lambda: ax_has_name(app, "Agent 需要你的选择", "AXStaticText"), "提问卡片没有出现在原生窗口")


def option_label(options: list[dict[str, Any]], option_id: str) -> str:
    for option in options:
        if option.get("id") == option_id:
            label = str(option.get("label", "")).strip()
            if label:
                return label
    raise AssertionError(f"pending option {option_id!r} not found in controller options")


def question_option_label(options: list[dict[str, Any]], expected: str) -> str:
    for option in options:
        label = str(option.get("label", "")).strip()
        if label == expected:
            return label
    raise AssertionError(f"question option {expected!r} not found in controller snapshot")


def click_permission(app: subprocess.Popen[str], permission: dict[str, Any], choice: str) -> str:
    label = option_label(permission.get("options", []), choice)
    wait_until(lambda: ax_has_name(app, label, "AXButton"), f"授权按钮 {label} 没有出现")
    ax_press_name(app, label)
    return label


def click_question_option(app: subprocess.Popen[str], options: list[dict[str, Any]], expected: str) -> str:
    label = question_option_label(options, expected)
    wait_until(lambda: ax_has_name(app, label, "AXButton"), f"提问选项 {label} 没有出现")
    ax_press_name(app, label)
    return label


def send_prompt(app: subprocess.Popen[str], prompt: str) -> None:
    value = json_string(prompt)
    wait_until(lambda: exists_ax(app, 'text area 1 of window 1'), "任务输入框没有出现")
    ax(app, f"set value of text area 1 of window 1 to {value}\nclick button \"发送\" of window 1")


def configure_provider(app: subprocess.Popen[str], data_dir: pathlib.Path, project: pathlib.Path, base_url: str) -> str:
    ax(app, 'set frontmost to true\nclick button "设置" of window 1')
    ax(app, 'click button "添加服务" of scroll area 1 of window 1')
    wait_until(lambda: ax(app, "return count of text fields of scroll area 1 of window 1") == "4", "provider editor 没有出现")
    for field, value in (
        (1, "原生 pending fixture"),
        (2, base_url + "/v1"),
        (3, "native-pending-fixture-model"),
        (4, LOCAL_FIXTURE_KEY),
    ):
        ax(app, f"set value of text field {field} of scroll area 1 of window 1 to {json_string(value)}")
    ax(app, 'click button "保存服务" of window 1')
    saved = wait_state(
        data_dir,
        lambda snapshot: any(
            provider.get("name") == "原生 pending fixture"
            and provider.get("base_url") == base_url + "/v1"
            and provider.get("model") == "native-pending-fixture-model"
            for provider in snapshot.get("settings", {}).get("providers") or []
        ),
        "provider 保存后没有进入持久化 snapshot",
    )
    provider = next(provider for provider in saved["settings"]["providers"] if provider.get("name") == "原生 pending fixture")
    credential_file = data_dir / "credentials" / "credentials.enc"
    wait_until(lambda: credential_file.exists(), "本地 fixture credential 没有落入隔离凭据目录")
    ax(app, 'click radio button "深色" of radio group 1 of scroll area 1 of window 1')
    ax(app, 'click button "保存设置" of window 1')
    wait_state(
        data_dir,
        lambda snapshot: (
            snapshot.get("settings", {}).get("default_provider_id") == provider.get("id")
            and snapshot.get("settings", {}).get("theme") == "dark"
        ),
        "设置保存后 default provider 或深色主题没有持久化",
    )
    ax(app, 'click button "返回工作台" of window 1')
    wait_until(
        lambda: exists_ax(app, 'button "设置" of window 1') and not exists_ax(app, 'button "返回工作台" of window 1'),
        "返回工作台没有完成",
    )
    ax(app, f"set value of text field 1 of window 1 to {json_string(str(project))}")
    ax(app, 'click button "打开项目" of window 1')
    wait_state(data_dir, lambda snapshot: len(snapshot.get("projects") or []) == 1, "项目没有打开")
    ax(app, 'click button "新会话" of window 1')
    snapshot = wait_state(data_dir, lambda value: len(value.get("sessions") or []) == 1, "会话没有创建")
    return newest_session(snapshot)["id"]


def quit_app(app: subprocess.Popen[str]) -> bool:
    if app.poll() is not None:
        return False
    try:
        names = ax(app, "return name of every menu bar item of menu bar 1").split(", ")
        if len(names) >= 2:
            menu = json_string(names[1])
            ax(app, f'click menu bar item {menu} of menu bar 1\nclick menu item "退出 GCode" of menu 1 of menu bar item {menu} of menu bar 1')
            app.wait(timeout=10)
            return app.returncode == 0
    except (AxError, subprocess.TimeoutExpired, subprocess.CalledProcessError):
        pass
    terminate_process(app)
    return False


def terminate_process(process: subprocess.Popen[str]) -> None:
    if process.poll() is not None:
        return
    try:
        process.terminate()
        process.wait(timeout=8)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait(timeout=8)


def read_history(data_dir: pathlib.Path, session_id: str) -> str:
    path = data_dir / "sessions" / session_id / "events.jsonl"
    if not path.exists():
        return ""
    return path.read_text()


def first_pending(session: dict[str, Any], kind: str) -> dict[str, Any]:
    key = "permissions" if kind == "permission" else "questions"
    pending = session.get(key, [])
    if not isinstance(pending, list) or not pending or not isinstance(pending[0], dict):
        raise AssertionError(f"{kind} pending request 不存在")
    return pending[0]


def message_for_run(session: dict[str, Any], run_id: str, prompt: str) -> dict[str, Any] | None:
    for message in session.get("messages") or []:
        if not isinstance(message, dict):
            continue
        if (
            message.get("role") == "user"
            and message.get("run_id") == run_id
            and message.get("text") == prompt
        ):
            return message
    return None


def correction_snapshot_evidence(
    old_session: dict[str, Any],
    replacement_session: dict[str, Any],
    old_pending: dict[str, Any],
    new_pending: dict[str, Any],
    old_prompt: str,
    new_prompt: str,
) -> dict[str, Any]:
    old_run_id = old_session.get("run_id")
    new_run_id = replacement_session.get("run_id")
    old_pending_id = old_pending.get("id")
    new_pending_id = new_pending.get("id")
    old_pending_run_id = old_pending.get("run_id")
    new_pending_run_id = new_pending.get("run_id")
    old_message = message_for_run(replacement_session, str(old_run_id or ""), old_prompt)
    new_message = message_for_run(replacement_session, str(new_run_id or ""), new_prompt)
    checks = {
        "run_ids_present": bool(old_run_id and new_run_id),
        "run_ids_different": bool(old_run_id and new_run_id and old_run_id != new_run_id),
        "pending_ids_present": bool(old_pending_id and new_pending_id),
        "pending_ids_different": bool(old_pending_id and new_pending_id and old_pending_id != new_pending_id),
        "old_pending_belongs_to_old_run": bool(old_run_id and old_pending_run_id == old_run_id),
        "new_pending_belongs_to_new_run": bool(new_run_id and new_pending_run_id == new_run_id),
        "old_message_run_id": bool(old_message and old_message.get("run_id") == old_run_id),
        "old_message_interrupted_state": bool(old_message and old_message.get("state") == "interrupted"),
        "old_message_interrupted_flag": bool(old_message and old_message.get("interrupted") is True),
        "new_message_run_id": bool(new_message and new_message.get("run_id") == new_run_id),
    }
    failures = [name for name, passed in checks.items() if not passed]
    return {
        "old_run_id": old_run_id,
        "new_run_id": new_run_id,
        "old_pending_id": old_pending_id,
        "new_pending_id": new_pending_id,
        "old_pending_run_id": old_pending_run_id,
        "new_pending_run_id": new_pending_run_id,
        "old_message": {
            "id": old_message.get("id") if old_message else None,
            "run_id": old_message.get("run_id") if old_message else None,
            "state": old_message.get("state") if old_message else None,
            "interrupted": old_message.get("interrupted") if old_message else None,
        },
        "new_message": {
            "id": new_message.get("id") if new_message else None,
            "run_id": new_message.get("run_id") if new_message else None,
            "state": new_message.get("state") if new_message else None,
        },
        "snapshot_checks": checks,
        "snapshot_assertion": "PASS" if not failures else "FAIL",
        "snapshot_failures": failures,
    }


def event_order_evidence(
    history: str,
    old_run_id: str,
    new_run_id: str,
    old_pending_id: str,
    new_pending_id: str,
    kind: str,
) -> dict[str, Any]:
    evidence: dict[str, Any] = {
        "format": "GAP",
        "format_issues": [],
        "assertion": "GAP",
        "gaps": [],
        "sequence": {},
    }
    if not history.strip():
        evidence["gaps"].append("events.jsonl 不存在或为空")
        return evidence

    events: list[dict[str, Any]] = []
    run_payload_types = {
        "message",
        "tool_call",
        "permission",
        "permission_request",
        "permission_decision",
        "tool_result",
        "provider_usage",
        "error",
    }
    for line_number, line in enumerate(history.splitlines(), 1):
        if not line.strip():
            evidence["format_issues"].append(f"第 {line_number} 行为空")
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError as error:
            evidence["format_issues"].append(f"第 {line_number} 行 JSON 无效: {error.msg}")
            continue
        if not isinstance(event, dict):
            evidence["format_issues"].append(f"第 {line_number} 行不是对象")
            continue
        missing = [key for key in ("sequence", "type", "payload") if key not in event]
        if missing:
            evidence["format_issues"].append(f"第 {line_number} 行缺少字段: {','.join(missing)}")
            continue
        if not isinstance(event.get("sequence"), int) or not isinstance(event.get("type"), str):
            evidence["format_issues"].append(f"第 {line_number} 行 sequence/type 类型不正确")
            continue
        payload = event.get("payload")
        if not isinstance(payload, dict):
            evidence["format_issues"].append(f"第 {line_number} 行 payload 不是对象")
            continue
        if event["type"] in run_payload_types and not isinstance(payload.get("runId"), str):
            evidence["format_issues"].append(f"第 {line_number} 行 payload 缺少结构化 runId")
        event["_line"] = line_number
        events.append(event)
    if evidence["format_issues"]:
        evidence["gaps"].append("events.jsonl 结构字段不足，未宣称完整事件顺序")
        return evidence
    evidence["format"] = "PASS"

    def sequence_for(predicate) -> int | None:
        for event in events:
            if predicate(event):
                return event["sequence"]
        return None

    old_user = sequence_for(
        lambda event: event["type"] == "message"
        and event["payload"].get("runId") == old_run_id
        and event["payload"].get("role") == "user"
    )
    new_user = sequence_for(
        lambda event: event["type"] == "message"
        and event["payload"].get("runId") == new_run_id
        and event["payload"].get("role") == "user"
    )
    if kind == "permission":
        old_pending = sequence_for(
            lambda event: event["type"] == "permission_request"
            and event["payload"].get("runId") == old_run_id
            and event["payload"].get("id") == old_pending_id
        )
        new_pending = sequence_for(
            lambda event: event["type"] == "permission_request"
            and event["payload"].get("runId") == new_run_id
            and event["payload"].get("id") == new_pending_id
        )
    else:
        old_pending = sequence_for(
            lambda event: event["type"] == "message"
            and event["payload"].get("runId") == old_run_id
            and event["payload"].get("role") == "ask"
            and isinstance(event["payload"].get("request"), dict)
            and (
                event["payload"]["request"].get("id")
                or event["payload"]["request"].get("ID")
            )
            == old_pending_id
        )
        new_pending = sequence_for(
            lambda event: event["type"] == "message"
            and event["payload"].get("runId") == new_run_id
            and event["payload"].get("role") == "ask"
            and isinstance(event["payload"].get("request"), dict)
            and (
                event["payload"]["request"].get("id")
                or event["payload"]["request"].get("ID")
            )
            == new_pending_id
        )
    sequence = {"old_user": old_user, "old_pending": old_pending, "new_user": new_user, "new_pending": new_pending}
    evidence["sequence"] = sequence
    missing = [name for name, value in sequence.items() if value is None]
    if missing:
        evidence["gaps"].append("事件缺少结构化定位: " + ",".join(missing))
        return evidence
    if not (old_user < old_pending < new_user < new_pending):
        evidence["gaps"].append("old user/pending 与 new user/pending 的 sequence 顺序不成立")
        return evidence
    evidence["assertion"] = "PASS"
    return evidence


def assert_no_secret(root: pathlib.Path) -> None:
    for path in root.rglob("*"):
        if not path.is_file() or path.name.endswith(".app"):
            continue
        try:
            content = path.read_bytes()
        except OSError:
            continue
        if LOCAL_FIXTURE_KEY.encode() in content:
            raise AssertionError(f"fixture secret leaked to evidence: {path}")


def preserve_data_evidence(data_dir: pathlib.Path, run_dir: pathlib.Path, session_id: str | None) -> bool:
    copied = False
    controller = data_dir / "controller.json"
    if controller.exists():
        shutil.copyfile(controller, run_dir / "controller.json")
        copied = True
    if session_id:
        history = data_dir / "sessions" / session_id / "events.jsonl"
        if history.exists():
            shutil.copyfile(history, run_dir / "events.jsonl")
            copied = True
    return copied


def capture_sanitized_ax(app: subprocess.Popen[str], output: pathlib.Path, name: str) -> None:
    # AX output contains only labels/fixture commands and is useful for the
    # native audit.  It never includes the API key field because this is the
    # workbench view, not settings.
    (output / name).write_text(ax(app, "return entire contents of window 1"))


def capture_failure_pending_evidence(
    app: subprocess.Popen[str] | None,
    run_dir: pathlib.Path,
    session_id: str | None,
) -> dict[str, Any]:
    """Capture pending UI evidence only after proving the app is on workbench."""
    evidence: dict[str, Any] = {"status": "SKIP"}
    if app is None or session_id is None:
        evidence["reason"] = "session_id 或 app 不存在"
        return evidence
    if app.poll() is not None:
        evidence["reason"] = "app 已退出"
        return evidence
    try:
        if (
            exists_ax(app, 'button "返回工作台" of window 1')
            or exists_ax(app, 'button "保存设置" of window 1')
        ):
            evidence["reason"] = "仍在设置页面，跳过 AX/screenshot 以避免 API Key 进入证据"
            return evidence
        capture = capture_window(app, run_dir, "failure-pending.png")
        capture_sanitized_ax(app, run_dir, "failure-pending-ax.txt")
        evidence.update({"status": "CAPTURED", "ax": str(run_dir / "failure-pending-ax.txt"), "screenshot": capture})
        return evidence
    except (AxError, AssertionError, OSError, subprocess.TimeoutExpired, subprocess.CalledProcessError) as error:
        evidence.update({"status": "ERROR", "error": str(error)})
        return evidence


def capture_window(app: subprocess.Popen[str], output: pathlib.Path, name: str) -> dict[str, Any]:
    query = '''import CoreGraphics
let rows = CGWindowListCopyWindowInfo(.optionOnScreenOnly, kCGNullWindowID) as? [[String: Any]] ?? []
for row in rows {
 if (row[kCGWindowOwnerPID as String] as? Int ?? 0) == QA_PID_VALUE,
    (row[kCGWindowLayer as String] as? Int ?? -1) == 0 {
  print(row[kCGWindowNumber as String] ?? "")
  break
 }
}'''.replace("QA_PID_VALUE", str(app.pid))
    window_id = run_command("swift", "-e", query, timeout=30)
    if not window_id:
        raise AssertionError("当前 QA PID 没有可捕获的原生窗口")
    path = output / name
    subprocess.run(["screencapture", "-x", "-o", "-l", window_id, str(path)], check=True, timeout=30)
    return {"file": str(path), "pid": app.pid, "window_id": int(window_id), "sha256": digest(path)}


def run_scenario(scenario: str, source: pathlib.Path, output: pathlib.Path, config: dict[str, Any]) -> dict[str, Any]:
    fixture_root = output / "_fixture"
    fixture_root.mkdir(parents=True, exist_ok=True)
    scenario_config = next(item for item in config["scenarios"] if item["id"] == scenario)
    run_dir = pathlib.Path(tempfile.mkdtemp(prefix=scenario + "-", dir=fixture_root))
    bundle: pathlib.Path | None = None
    data_dir: pathlib.Path | None = None
    project = run_dir / "project"
    project.mkdir()
    server_fixture = FixtureState(scenario, project)
    server = FixtureHTTPServer(server_fixture)
    server_thread = threading.Thread(target=server.serve_forever, name="native-pending-fixture", daemon=True)
    app: subprocess.Popen[str] | None = None
    results: dict[str, Any] = {"scenario": scenario, "status": "FAIL", "checks": {}}
    source_exe = source / "Contents/MacOS/GCode"
    original = {"executable": digest(source_exe), "code": code_digest(source_exe), "resources": resource_digests(source)}
    qa_name = "GCodePendingQA" + str(time.time_ns())
    results.update(
        {
            "qa_identity": qa_name,
            "project": str(project),
            "data_directory": str(pathlib.Path.home() / "Library/Application Support" / qa_name),
            "source_executable_sha256": original["executable"],
            "source_code_sha256": original["code"],
            "source_resources": original["resources"],
            "captures": [],
        }
    )
    session_id: str | None = None
    try:
        server_thread.start()
        bundle = run_dir / (qa_name + ".app")
        data_dir = pathlib.Path.home() / "Library/Application Support" / qa_name
        shutil.copytree(source, bundle, symlinks=True)
        info_path = bundle / "Contents/Info.plist"
        info = plistlib.loads(info_path.read_bytes())
        info.update(
            CFBundleName=qa_name,
            CFBundleDisplayName=qa_name,
            CFBundleIdentifier="app.gcode.desktop.pendingqa." + qa_name.lower(),
        )
        info_path.write_bytes(plistlib.dumps(info))
        subprocess.run(["codesign", "--force", "--deep", "--sign", "-", str(bundle)], check=True, capture_output=True, timeout=30)
        subprocess.run(["codesign", "--verify", "--deep", "--strict", str(bundle)], check=True, capture_output=True, timeout=30)
        executable = bundle / "Contents/MacOS/GCode"
        if code_digest(executable) != original["code"] or resource_digests(bundle) != original["resources"]:
            raise AssertionError("正式包副本的代码或资源与正式包不一致")
        results.update({"isolated_bundle": str(bundle), "bundle_code_and_resources_identical": True})
        if data_dir.exists():
            raise AssertionError("隔离 Application Support 目录已存在")

        env = os.environ.copy()
        env["GCODE_CRED_STORAGE"] = "encrypted-file"
        for key in ("GCODE_VISUAL_SAMPLE", "GCODE_EDITOR", "VISUAL", "EDITOR"):
            env.pop(key, None)
        # The formal app has its own redaction boundary.  Do not create an
        # acceptance log at all: a process diagnostic must never become a
        # second place where a credential could be persisted.
        with open(os.devnull, "w") as null_log:
            app = subprocess.Popen([str(executable)], cwd=ROOT, env=env, stdout=null_log, stderr=null_log, text=True)
            wait_until(lambda: exists_ax(app, 'button "设置" of window 1'), "原生正式包窗口没有准备好")
            session_id = configure_provider(app, data_dir, project, f"http://127.0.0.1:{server.server_address[1]}")
            results["session_id"] = session_id
            prompt = scenario_config["initial_prompt"]
            send_prompt(app, prompt)

            if scenario == "permission-allow-deny":
                pending = wait_state(data_dir, lambda value: session_from(value, session_id).get("state") == "waiting_permission", "allow permission 没有等待")
                results.setdefault("captures", []).append(capture_window(app, run_dir, "permission-state-waiting.png"))
                capture_sanitized_ax(app, run_dir, "permission-state-waiting-ax.txt")
                ensure_permission_card(app)
                results.setdefault("captures", []).append(capture_window(app, run_dir, "permission-allow-waiting.png"))
                click_permission(app, session_from(pending, session_id)["permissions"][0], "allow")
                wait_until(lambda: (project / "permission-allowed.txt").read_text() == "approved content\n", "allow 没有写入真实 fixture 文件")
                pending = wait_state(data_dir, lambda value: session_from(value, session_id).get("state") == "waiting_permission", "deny permission 没有等待")
                ensure_permission_card(app)
                results.setdefault("captures", []).append(capture_window(app, run_dir, "permission-deny-waiting.png"))
                click_permission(app, session_from(pending, session_id)["permissions"][0], "deny")
                wait_state(data_dir, lambda value: session_from(value, session_id).get("state") == "idle", "allow/deny run 没有完成")
                if (project / "permission-denied.txt").exists():
                    raise AssertionError("deny 不应写入 fixture 文件")
                results["checks"].update({"allow_real_file": "PASS", "deny_no_file": "PASS"})
            elif scenario == "ask-history":
                pending = wait_state(data_dir, lambda value: session_from(value, session_id).get("state") == "waiting_question", "ask_user 没有等待")
                ensure_question_card(app)
                results.setdefault("captures", []).append(capture_window(app, run_dir, "ask-history-waiting.png"))
                question = session_from(pending, session_id)["questions"][0]["questions"][0]
                click_question_option(app, question["options"], "production")
                wait_state(data_dir, lambda value: session_from(value, session_id).get("state") == "idle", "ask_user run 没有完成")
                history = read_history(data_dir, session_id)
                if "ask_user_answers" not in history or "production" not in history:
                    raise AssertionError("ask_user 回答没有写入 history")
                results["checks"].update({"answer_next_provider": "PASS", "answer_history": "PASS"})
            elif scenario == "permission-correction":
                old = wait_state(data_dir, lambda value: session_from(value, session_id).get("state") == "waiting_permission", "旧 permission 没有等待")
                old_session = session_from(old, session_id)
                old_permission = first_pending(old_session, "permission")
                old_id = old_permission["id"]
                old_run_id = old_session.get("run_id")
                if not old_run_id or old_permission.get("run_id") != old_run_id:
                    raise AssertionError("旧 permission pending 没有结构化归属到旧 run")
                capture_sanitized_ax(app, run_dir, "old-permission-ax.txt")
                send_prompt(app, scenario_config["replacement_prompt"])
                replacement = wait_state(
                    data_dir,
                    lambda value: (
                        session_from(value, session_id).get("state") == "waiting_permission"
                        and (session_from(value, session_id).get("permissions") or [{}])[0].get("id") != old_id
                    ),
                    "纠正后 replacement permission 没有出现",
                )
                new_session = session_from(replacement, session_id)
                new_permission = first_pending(new_session, "permission")
                new_id = new_permission["id"]
                correction = correction_snapshot_evidence(
                    old_session,
                    new_session,
                    old_permission,
                    new_permission,
                    scenario_config["initial_prompt"],
                    scenario_config["replacement_prompt"],
                )
                results["correction"] = correction
                if correction["snapshot_assertion"] != "PASS":
                    raise AssertionError("纠正后的 run/pending/message 结构断言失败: " + ",".join(correction["snapshot_failures"]))
                history = read_history(data_dir, session_id)
                event_order = event_order_evidence(
                    history,
                    str(correction["old_run_id"]),
                    str(correction["new_run_id"]),
                    str(correction["old_pending_id"]),
                    str(correction["new_pending_id"]),
                    "permission",
                )
                results["correction"]["event_order"] = event_order
                if event_order["format"] == "PASS" and event_order["assertion"] != "PASS":
                    raise AssertionError("permission correction events.jsonl 顺序断言失败或存在结构缺口")
                if (project / "permission-old.txt").exists():
                    raise AssertionError("旧 permission run 在纠正后写入了文件")
                wait_until(lambda: ax_has_name(app, "已中断"), "旧 permission tool 没有显示已中断状态")
                capture_sanitized_ax(app, run_dir, "replacement-permission-ax.txt")
                results.setdefault("captures", []).append(capture_window(app, run_dir, "permission-replacement-waiting.png"))
                click_permission(app, new_session["permissions"][0], "allow")
                wait_until(lambda: (project / "permission-new.txt").read_text() == "new\n", "replacement permission 未写入新文件")
                wait_state(data_dir, lambda value: session_from(value, session_id).get("state") == "idle", "replacement permission 未完成")
                results["checks"].update(
                    {
                        "old_run_interrupted": "PASS",
                        "replacement_run": "PASS",
                        "event_order": "PASS" if event_order["assertion"] == "PASS" else "NOT_VERIFIED",
                        "late_old_permission_reply": "NOT_INJECTED",
                    }
                )
            elif scenario == "ask-correction":
                old = wait_state(data_dir, lambda value: session_from(value, session_id).get("state") == "waiting_question", "旧 ask_user 没有等待")
                old_session = session_from(old, session_id)
                old_request = first_pending(old_session, "question")
                old_id = old_request["id"]
                old_run_id = old_session.get("run_id")
                if not old_run_id or old_request.get("run_id") != old_run_id:
                    raise AssertionError("旧 ask_user pending 没有结构化归属到旧 run")
                capture_sanitized_ax(app, run_dir, "old-ask-ax.txt")
                send_prompt(app, scenario_config["replacement_prompt"])
                replacement = wait_state(
                    data_dir,
                    lambda value: (
                        session_from(value, session_id).get("state") == "waiting_question"
                        and (session_from(value, session_id).get("questions") or [{}])[0].get("id") != old_id
                    ),
                    "纠正后 replacement ask_user 没有出现",
                )
                new_session = session_from(replacement, session_id)
                new_request = first_pending(new_session, "question")
                new_id = new_request["id"]
                correction = correction_snapshot_evidence(
                    old_session,
                    new_session,
                    old_request,
                    new_request,
                    scenario_config["initial_prompt"],
                    scenario_config["replacement_prompt"],
                )
                results["correction"] = correction
                if correction["snapshot_assertion"] != "PASS":
                    raise AssertionError("纠正后的 run/pending/message 结构断言失败: " + ",".join(correction["snapshot_failures"]))
                history = read_history(data_dir, session_id)
                event_order = event_order_evidence(
                    history,
                    str(correction["old_run_id"]),
                    str(correction["new_run_id"]),
                    str(correction["old_pending_id"]),
                    str(correction["new_pending_id"]),
                    "question",
                )
                results["correction"]["event_order"] = event_order
                if event_order["format"] == "PASS" and event_order["assertion"] != "PASS":
                    raise AssertionError("ask_user correction events.jsonl 顺序断言失败或存在结构缺口")
                wait_until(lambda: ax_has_name(app, "已中断"), "旧 ask_user tool 没有显示已中断状态")
                capture_sanitized_ax(app, run_dir, "replacement-ask-ax.txt")
                results.setdefault("captures", []).append(capture_window(app, run_dir, "ask-replacement-waiting.png"))
                question = new_session["questions"][0]["questions"][0]
                click_question_option(app, question["options"], "replacement answer")
                wait_state(data_dir, lambda value: session_from(value, session_id).get("state") == "idle", "replacement ask_user 未完成")
                history = read_history(data_dir, session_id)
                results["checks"].update(
                    {
                        "old_run_interrupted": "PASS",
                        "replacement_run": "PASS",
                        "event_order": "PASS" if event_order["assertion"] == "PASS" else "NOT_VERIFIED",
                        "late_old_answer": "NOT_INJECTED",
                    }
                )
            else:
                raise AssertionError(f"未知 scenario: {scenario}")

            if server_fixture.errors:
                raise AssertionError("; ".join(server_fixture.errors))
            normal_exit = quit_app(app)
            results["normal_exit_requested"] = normal_exit
            results["app_exit_code"] = app.returncode
            if not normal_exit or app.returncode != 0:
                raise AssertionError(f"应用没有通过菜单正常退出，exit={app.returncode}")
            results["status"] = "PASS"
    except Exception as error:
        results["error"] = str(error)
        results["failure_pending_evidence"] = capture_failure_pending_evidence(app, run_dir, session_id)
        raise
    finally:
        if app is not None:
            if app.poll() is None:
                terminate_process(app)
                results["forced_process_cleanup"] = True
            results["app_exit_code"] = app.returncode
        with server_fixture.lock:
            results["provider_records"] = list(server_fixture.records)
            results["provider_errors"] = list(server_fixture.errors)
        server.shutdown()
        server.server_close()
        server_thread.join(timeout=5)
        results["server_thread_exited"] = not server_thread.is_alive()
        results["server_stopped"] = True
        if data_dir is not None and data_dir.exists():
            results["data_evidence_copied"] = preserve_data_evidence(data_dir, run_dir, session_id)
            credentials_dir = data_dir / "credentials"
            if credentials_dir.exists():
                shutil.rmtree(credentials_dir, ignore_errors=True)
                results["fixture_credentials_removed"] = not credentials_dir.exists()
            else:
                results["fixture_credentials_removed"] = True
            results["data_directory_retained"] = data_dir.exists()
        else:
            results["data_directory_retained"] = False
        results["qa_bundle_retained"] = bool(bundle and bundle.exists())
        results["qa_project_retained"] = project.exists()
        (fixture_root / f"{scenario}-result.json").write_text(json.dumps(results, ensure_ascii=False, indent=2))
    return results


class FixtureSelfTest(unittest.TestCase):
    def test_tool_sse_is_provider_compatible(self) -> None:
        payload = json.loads(sse_tool_call("id", "exec_command", {"cmd": "echo ok"}).split(b"\n", 1)[0].split(b": ", 1)[1])
        delta = payload["choices"][0]["delta"]["tool_calls"][0]
        self.assertEqual(delta["function"]["name"], "exec_command")
        self.assertEqual(json.loads(delta["function"]["arguments"])["cmd"], "echo ok")

    def test_last_user_text_uses_latest_user_message(self) -> None:
        payload = {"messages": [{"role": "user", "content": "old"}, {"role": "assistant", "content": "x"}, {"role": "user", "content": "new"}]}
        self.assertEqual(last_user_text(payload), "new")

    def test_fixture_records_answer_without_raw_secret(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            fixture = FixtureState("ask-history", pathlib.Path(temp))
            result = fixture.validate_and_response(
                {
                    "messages": [
                        {
                            "role": "assistant",
                            "tool_calls": [
                                {
                                    "id": "ask-history-call",
                                    "type": "function",
                                    "function": {
                                        "name": "ask_user",
                                        "arguments": '{"options":["production"]}',
                                    },
                                }
                            ],
                        },
                        {"role": "tool", "tool_call_id": "ask-history-call", "content": "production"},
                    ]
                },
                "Bearer " + LOCAL_FIXTURE_KEY,
            )
            self.assertIn(b"[DONE]", result)
            self.assertEqual(fixture.records[0]["contains_production_answer"], True)
            self.assertNotIn(LOCAL_FIXTURE_KEY, json.dumps(fixture.records))

    def test_fixture_does_not_treat_ask_option_as_answer(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            fixture = FixtureState("ask-history", pathlib.Path(temp))
            fixture.validate_and_response(
                {
                    "messages": [
                        {
                            "role": "assistant",
                            "tool_calls": [
                                {
                                    "id": "ask-history-call",
                                    "type": "function",
                                    "function": {
                                        "name": "ask_user",
                                        "arguments": '{"options":["production"]}',
                                    },
                                }
                            ],
                        }
                    ]
                },
                "Bearer " + LOCAL_FIXTURE_KEY,
            )
            self.assertEqual(fixture.records[0]["contains_production_answer"], False)


def load_config() -> dict[str, Any]:
    if not FIXTURE_CONFIG.exists():
        raise FileNotFoundError(f"fixture 配置不存在: {FIXTURE_CONFIG}")
    config = json.loads(FIXTURE_CONFIG.read_text())
    ids = [item.get("id") for item in config.get("scenarios", [])]
    required = {"permission-allow-deny", "ask-history", "permission-correction", "ask-correction"}
    if set(ids) != required:
        raise AssertionError(f"fixture scenario 集合不完整: {ids}")
    return config


def self_test() -> int:
    load_config()
    suite = unittest.defaultTestLoader.loadTestsFromTestCase(FixtureSelfTest)
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    return 0 if result.wasSuccessful() else 1


def reject_existing_output(output: pathlib.Path) -> None:
    """Refuse to overwrite any evidence from a previous native run."""
    if output.exists() and not output.is_dir():
        raise SystemExit(f"验收输出不是目录，拒绝覆盖: {output}")
    if not output.exists():
        return
    conflicts: list[str] = []
    for entry in output.iterdir():
        if entry.name != "_fixture":
            conflicts.append(str(entry))
    fixture_root = output / "_fixture"
    if fixture_root.exists() and not fixture_root.is_dir():
        conflicts.append(str(fixture_root))
    elif fixture_root.is_dir():
        for entry in fixture_root.iterdir():
            if entry.name != "scenarios.json":
                conflicts.append(str(entry))
    if conflicts:
        raise SystemExit(
            "验收输出已存在本次运行证据，拒绝覆盖；请使用新的 --output: " + ", ".join(sorted(conflicts))
        )


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bundle", default=DEFAULT_BUNDLE)
    parser.add_argument("--output", default=DEFAULT_OUTPUT)
    parser.add_argument("--scenario", action="append", choices=["permission-allow-deny", "ask-history", "permission-correction", "ask-correction"])
    parser.add_argument("--self-test", action="store_true", help="只运行本地 fixture 协议单测，不打开原生窗口")
    options = parser.parse_args()
    if options.self_test:
        return self_test()
    if sys.platform != "darwin":
        raise SystemExit("原生 pending 验收驱动必须在 macOS 上运行")
    source = (ROOT / options.bundle).resolve()
    if not source.is_dir():
        raise SystemExit(f"正式包不存在: {source}")
    output = (ROOT / options.output).resolve()
    reject_existing_output(output)
    output.mkdir(parents=True, exist_ok=True)
    config = load_config()
    scenarios = options.scenario or [item["id"] for item in config["scenarios"]]
    source_exe = source / "Contents/MacOS/GCode"
    source_executable_sha256 = digest(source_exe)
    source_code_sha256 = code_digest(source_exe)
    source_resources = resource_digests(source)
    manifest = {
        "driver_sha256": digest(pathlib.Path(__file__).resolve()),
        "original_bundle": str(source),
        "output_fixture": str(output / "_fixture"),
        "scenarios": scenarios,
        "source_executable_sha256": source_executable_sha256,
        "source_code_sha256": source_code_sha256,
        "source_resources": source_resources,
        "changes": "仅隔离副本 Info.plist bundle identity 与 ad-hoc signature",
        "real_api": "NOT TESTED",
        "clipboard": "NOT TOUCHED",
        "appearance": "dark; selected and saved through the native settings controls",
        "localhost_only": True,
        "secret_policy": "invalid fixture key is stored only in isolated encrypted credentials; plaintext never enters evidence and credentials are removed after exit",
        "source_files_at_start": source_digests(),
    }
    (output / "_fixture").mkdir(parents=True, exist_ok=True)
    (output / "_fixture" / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2))
    results = []
    try:
        for scenario in scenarios:
            results.append(run_scenario(scenario, source, output, config))
        source_unchanged = source_digests() == manifest["source_files_at_start"]
        original_bundle_unchanged = (
            digest(source_exe) == source_executable_sha256
            and code_digest(source_exe) == source_code_sha256
            and resource_digests(source) == source_resources
        )
        manifest["source_unchanged_after_run"] = source_unchanged
        manifest["original_bundle_unchanged_after_run"] = original_bundle_unchanged
        if not source_unchanged:
            raise AssertionError("原生验收期间源码发生变化")
        if not original_bundle_unchanged:
            raise AssertionError("原生验收期间正式包发生变化")
        assert_no_secret(output / "_fixture")
        summary = {"status": "PASS", "scenarios": results, "manifest": manifest}
    except Exception as error:
        manifest["source_unchanged_after_run"] = source_digests() == manifest["source_files_at_start"]
        manifest["original_bundle_unchanged_after_run"] = (
            digest(source_exe) == source_executable_sha256
            and code_digest(source_exe) == source_code_sha256
            and resource_digests(source) == source_resources
        )
        summary = {"status": "FAIL", "error": str(error), "scenarios": results, "manifest": manifest}
        raise
    finally:
        (output / "_fixture" / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2))
        (output / "_fixture" / "results.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2))
    print(json.dumps(summary, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
