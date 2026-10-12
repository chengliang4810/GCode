"""正式包真实 Service/controller 持续流式与同项目多会话原生验收驱动。

默认运行会复制正式 ``GCode.app``，只修改隔离副本的 bundle identity 并重新
ad-hoc 签名，然后让真实设置页保存一个仅监听 ``127.0.0.1`` 的 Chat
Completions SSE fixture。System Events 只操作这个 QA 进程自己的窗口；请求
记录只保留 session marker 和时序/计数，不保存 Authorization 或完整请求体。

``--self-test`` 只运行本地纯函数断言，不打开窗口，也不绑定 HTTP socket。真实
原生运行应在 root 提供当前正式包身份后串行执行。
"""

from __future__ import annotations

import argparse
import datetime as _datetime
import hashlib
import http.server
import json
import math
import os
import pathlib
import plistlib
import re
import shutil
import struct
import subprocess
import sys
import tempfile
import threading
import time
from typing import Any


ROOT = pathlib.Path(__file__).resolve().parents[3]
DEFAULT_BUNDLE = "build/darwin-amd64/GCode.app"
DEFAULT_OUTPUT = "output/acceptance/native-streaming/round-1"
DEFAULT_DURATION = 30.0
DEFAULT_HZ = 10.0
DEFAULT_SESSIONS = 3
LOCAL_FIXTURE_KEY = "gcode-native-streaming-local-fixture-key"
CHAT_PATH = "/v1/chat/completions"
MAX_AX_SECONDS = 12
MAX_WAIT_SECONDS = 40
MARKER_PREFIX = "NATIVE_STREAM_SESSION_"
RUN_SCOPED_EVENT_TYPES = frozenset(
    {
        "message",
        "tool_call",
        "permission",
        "permission_request",
        "permission_decision",
        "tool_result",
        "provider_usage",
        "error",
        "specialist_start",
        "specialist_stop",
    }
)


def digest(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def code_digest(path: pathlib.Path) -> str:
    """Hash ``__TEXT,__text``; ad-hoc signing must not change this section."""
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
    paths = [
        ROOT / name
        for name in ("go.mod", "go.sum", "mygo.json", "third_party/mygo/go.mod", "third_party/mygo/go.sum")
    ]
    for directory in ("cmd", "internal", "third_party/mygo"):
        paths.extend(path for path in (ROOT / directory).rglob("*.go") if not path.name.endswith("_test.go"))
    for directory in ("internal/desktop/fonts", "resources"):
        paths.extend(path for path in (ROOT / directory).rglob("*") if path.is_file())
    return {str(path.relative_to(ROOT)): digest(path) for path in sorted(paths)}


def utc_now() -> str:
    return _datetime.datetime.now(_datetime.timezone.utc).isoformat(timespec="milliseconds")


def json_string(value: str) -> str:
    return json.dumps(value, ensure_ascii=False)


def run_command(*args: str, cwd: pathlib.Path = ROOT, timeout: int = 30) -> str:
    result = subprocess.run(args, cwd=cwd, text=True, capture_output=True, timeout=timeout, check=True)
    return result.stdout.strip()


class AxError(RuntimeError):
    pass


class WaitDeadlineExceeded(AssertionError):
    """The condition did not complete within its strict outer budget."""


def redacted_error_type(error: BaseException) -> str:
    """Return a non-sensitive diagnostic classification for an exception."""
    if isinstance(error, WaitDeadlineExceeded):
        return "deadline_exceeded"
    if isinstance(error, subprocess.TimeoutExpired):
        return "subprocess_timeout"
    if isinstance(error, AxError):
        return "ax_timeout" if "超时" in str(error) else "ax_error"
    if isinstance(error, AssertionError):
        return "assertion_error"
    return type(error).__name__


def ax(app: subprocess.Popen[str], body: str, timeout: float = MAX_AX_SECONDS) -> str:
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


def wait_until(
    condition,
    label: str,
    timeout: float = MAX_WAIT_SECONDS,
    interval: float = 0.1,
    *,
    clock=time.monotonic,
    sleeper=time.sleep,
    deadline: float | None = None,
) -> None:
    """Wait with a strict outer deadline, including the condition call itself."""
    if deadline is None:
        deadline = clock() + timeout
    last_error = ""
    while True:
        remaining = deadline - clock()
        if remaining <= 0:
            break
        try:
            if condition():
                if clock() <= deadline:
                    return
                last_error = "条件在外层 deadline 后才成功"
                break
        except (AssertionError, AxError, FileNotFoundError, json.JSONDecodeError, OSError, subprocess.SubprocessError) as error:
            last_error = str(error)
        remaining = deadline - clock()
        if remaining <= 0:
            break
        sleeper(min(interval, remaining))
    suffix = f"；最后错误：{last_error}" if last_error else ""
    raise WaitDeadlineExceeded(f"{label} 超过严格外层 deadline{suffix}")


def read_snapshot(data_dir: pathlib.Path) -> dict[str, Any]:
    raw = json.loads((data_dir / "controller.json").read_text())
    snapshot = raw.get("snapshot")
    if not isinstance(snapshot, dict):
        raise AssertionError("controller.json 缺少 snapshot")
    return snapshot


def session_by_id(snapshot: dict[str, Any], session_id: str) -> dict[str, Any]:
    for session in snapshot.get("sessions") or []:
        if isinstance(session, dict) and session.get("id") == session_id:
            return session
    raise AssertionError(f"session {session_id!r} 不存在")


def wait_snapshot(data_dir: pathlib.Path, predicate, label: str, timeout: float = MAX_WAIT_SECONDS) -> dict[str, Any]:
    last: dict[str, Any] = {}

    def check() -> bool:
        nonlocal last
        last = read_snapshot(data_dir)
        return bool(predicate(last))

    wait_until(check, label, timeout)
    return last


def content_text(value: Any) -> str:
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


def last_user_text(payload: dict[str, Any]) -> str:
    for message in reversed(payload.get("messages") or []):
        if not isinstance(message, dict) or message.get("role") != "user":
            continue
        return content_text(message.get("content"))
    return ""


def extract_marker(text: str, allowed: list[str] | None = None) -> str:
    if allowed is not None:
        matches = [marker for marker in allowed if marker in text]
    else:
        matches = re.findall(r"NATIVE_STREAM_SESSION_[0-9]+_[A-Za-z0-9_-]+", text)
    if len(matches) != 1:
        raise AssertionError("请求缺少唯一 session marker")
    return matches[0]


def stream_tail_token(marker: str) -> str:
    """Return a session-specific viewport token that does not repeat the prompt marker."""
    return "NATIVE_STREAM_TAIL_" + hashlib.sha256(marker.encode("utf-8")).hexdigest()[:16]


def stream_completion_token(marker: str) -> str:
    """Return a session-specific token emitted only in the final content chunk."""
    return "NATIVE_STREAM_COMPLETE_" + hashlib.sha256((marker + ":complete").encode("utf-8")).hexdigest()[:16]


def session_message_texts(session: dict[str, Any]) -> list[str]:
    return [
        str(message.get("text", ""))
        for message in session.get("messages") or []
        if isinstance(message, dict)
    ]


def session_messages(session: dict[str, Any], role: str | None = None) -> list[dict[str, Any]]:
    result = []
    for message in session.get("messages") or []:
        if not isinstance(message, dict):
            continue
        if role is None or message.get("role") == role:
            result.append(message)
    return result


def validate_snapshot_ownership(
    snapshot: dict[str, Any],
    marker_by_session: dict[str, str],
    *,
    require_running: bool = False,
    require_complete: bool = False,
) -> dict[str, Any]:
    """Validate session/run/text ownership without relying on AX labels."""
    checks: dict[str, Any] = {}
    failures: list[str] = []
    sessions = {session.get("id"): session for session in snapshot.get("sessions") or [] if isinstance(session, dict)}
    run_ids: dict[str, str] = {}
    for session_id, marker in marker_by_session.items():
        session = sessions.get(session_id)
        if session is None:
            failures.append(f"missing session {session_id}")
            continue
        texts = session_message_texts(session)
        joined = "\n".join(texts)
        foreign = [other for other in marker_by_session.values() if other != marker and other in joined]
        foreign_tail_tokens = [
            stream_tail_token(other)
            for other in marker_by_session.values()
            if other != marker and stream_tail_token(other) in joined
        ]
        user_messages = session_messages(session, "user")
        assistant_messages = session_messages(session, "assistant")
        user_marker = any(marker in str(message.get("text", "")) for message in user_messages)
        assistant_text = "\n".join(str(message.get("text", "")) for message in assistant_messages)
        ids = {
            str(message.get("run_id"))
            for message in session_messages(session)
            if str(message.get("run_id", "")).strip()
        }
        active_run = session.get("active_run")
        session_run = str(session.get("run_id", ""))
        if session_run:
            run_ids[session_id] = session_run
        elif len(ids) == 1:
            run_ids[session_id] = next(iter(ids))
        checks[session_id] = {
            "state": session.get("state"),
            "user_marker": user_marker,
            "assistant_marker": marker in assistant_text,
            "foreign_markers": foreign,
            "foreign_tail_tokens": foreign_tail_tokens,
            "message_run_ids": sorted(ids),
            "active_run": bool(active_run),
        }
        if not user_marker:
            failures.append(f"{session_id} user marker missing")
        if foreign:
            failures.append(f"{session_id} contains foreign marker")
        if foreign_tail_tokens:
            failures.append(f"{session_id} contains foreign tail token")
        if len(ids) > 1:
            failures.append(f"{session_id} contains multiple message run ids")
        if require_running and (session.get("state") != "running" or not active_run or not session_run):
            failures.append(f"{session_id} is not running with an active run")
        if require_complete:
            if session.get("state") != "idle" or active_run is not None:
                failures.append(f"{session_id} is not idle without active_run")
            if not marker in assistant_text:
                failures.append(f"{session_id} assistant marker missing")
            for required in ("中文", "🧪", "```", "|"):
                if required not in assistant_text:
                    failures.append(f"{session_id} assistant response lacks {required!r}")
        if require_running and session_run:
            run_ids[session_id] = session_run
    if len(set(run_ids.values())) != len(run_ids):
        failures.append("session run ids are not independent")
    checks["run_ids"] = run_ids
    checks["session_count"] = len(sessions)
    checks["assertion"] = "PASS" if not failures else "FAIL"
    checks["failures"] = failures
    return checks


def validate_journal(
    history: str,
    session_id: str,
    marker: str,
    expected_run_id: str,
    all_markers: list[str],
) -> dict[str, Any]:
    """Check JSONL structure and that events stay in one session/run."""
    failures: list[str] = []
    roles: set[str] = set()
    event_count = 0
    previous_sequence = 0
    seen_run_ids: set[str] = set()
    for line_number, line in enumerate(history.splitlines(), 1):
        if not line.strip():
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            failures.append(f"line {line_number} is not JSON")
            continue
        if not isinstance(event, dict) or not isinstance(event.get("payload"), dict):
            failures.append(f"line {line_number} lacks object payload")
            continue
        event_count += 1
        sequence = event.get("sequence")
        if not isinstance(sequence, int) or sequence <= previous_sequence:
            failures.append(f"line {line_number} sequence is not increasing")
        if isinstance(sequence, int):
            previous_sequence = sequence
        if event.get("sessionId") != session_id:
            failures.append(f"line {line_number} has foreign session id")
        event_type = event.get("type")
        if not isinstance(event_type, str) or not event_type:
            failures.append(f"line {line_number} lacks event type")
        payload = event["payload"]
        run_id = payload.get("runId")
        has_run_id = isinstance(run_id, str) and bool(run_id)
        if event_type in RUN_SCOPED_EVENT_TYPES and not has_run_id:
            failures.append(f"line {line_number} {event_type} lacks runId")
        if has_run_id:
            seen_run_ids.add(run_id)
            if run_id != expected_run_id:
                failures.append(f"line {line_number} has foreign run id")
        encoded = json.dumps(payload, ensure_ascii=False)
        for other in all_markers:
            if other != marker and other in encoded:
                failures.append(f"line {line_number} contains foreign marker")
        role = payload.get("role") if event_type == "message" else None
        if isinstance(role, str):
            roles.add(role)
            if role in ("user", "assistant") and marker not in content_text(payload.get("content")):
                failures.append(f"line {line_number} {role} content lacks own marker")
    for required in ("user", "assistant", "run_end"):
        if required not in roles:
            failures.append(f"journal lacks role {required}")
    if event_count == 0:
        failures.append("journal is empty")
    return {
        "session_id": session_id,
        "event_count": event_count,
        "run_ids": sorted(seen_run_ids),
        "roles": sorted(roles),
        "assertion": "PASS" if not failures else "FAIL",
        "failures": failures,
    }


def sse_event(delta: dict[str, Any], finish: str | None = None) -> bytes:
    event = {"choices": [{"index": 0, "delta": delta, "finish_reason": finish}]}
    return ("data: " + json.dumps(event, ensure_ascii=False, separators=(",", ":")) + "\n\n").encode("utf-8")


def sse_done() -> bytes:
    return b"data: [DONE]\n\n"


def parse_sse_payloads(raw: bytes) -> list[str]:
    payloads: list[str] = []
    current: list[str] = []
    for line in raw.decode("utf-8").splitlines():
        if line.startswith("data:"):
            current.append(line[5:].lstrip())
        elif not line.strip() and current:
            payloads.append("\n".join(current))
            current = []
    if current:
        payloads.append("\n".join(current))
    return payloads


STREAM_BLOCKS = (
    "\n\n第 {index} 个增量：中文流式内容持续增长，emoji 🧪 保持完整。",
    "\n\n- 列表项 {index}：三个会话各自拥有独立 marker。\n- AX 读回只检查当前会话的内容。",
    "\n\n这一段是持续输出的普通段落，用于观察 controller 快照与原生视图的同步。",
    "\n\n```go\nfunc streamChunk(index int) string {{\n    return fmt.Sprintf(\"chunk-%d\", index)\n}}\n```",
    "\n\n| 序号 | 内容 |\n| ---: | --- |\n| {index} | 中文与 emoji 🧪 |",
)


def stream_fragment(marker: str, index: int) -> str:
    if index == 0:
        return f"流式开始：{marker}。\n\n"
    block = STREAM_BLOCKS[(index - 1) % len(STREAM_BLOCKS)].format(index=index)
    return block + f"\n\n持续输出尾部令牌：{stream_tail_token(marker)}"


def validate_ax_readback(
    value: str,
    marker: str,
    tail_token: str,
    all_markers: list[str],
    all_tail_tokens: list[str],
) -> dict[str, Any]:
    """Validate the current AX viewport without requiring the first streamed line."""
    foreign_markers = [other for other in all_markers if other != marker and other in value]
    foreign_tail_tokens = [other for other in all_tail_tokens if other != tail_token and other in value]
    if tail_token not in value:
        raise AssertionError(f"AX 没有读回当前 session 的 stream tail token: {tail_token}")
    if foreign_markers:
        raise AssertionError("AX 读回了其他 session marker: " + ", ".join(foreign_markers))
    if foreign_tail_tokens:
        raise AssertionError("AX 读回了其他 session tail token: " + ", ".join(foreign_tail_tokens))
    return {
        "prompt_marker_visible": marker in value,
        "tail_token_visible": tail_token in value,
        "foreign_markers": foreign_markers,
        "foreign_tail_tokens": foreign_tail_tokens,
    }


def validate_ax_completion_readback(
    value: str,
    marker: str,
    completion_token: str,
    all_markers: list[str],
    all_completion_tokens: list[str],
) -> dict[str, Any]:
    """Validate the final completion token from the real last-row AX value."""
    foreign_markers = [other for other in all_markers if other != marker and other in value]
    foreign_completion_tokens = [
        other for other in all_completion_tokens if other != completion_token and other in value
    ]
    if completion_token not in value:
        raise AssertionError(f"AX 没有读回当前 session 的 final completion token: {completion_token}")
    if foreign_markers:
        raise AssertionError("AX 读回了其他 session marker: " + ", ".join(foreign_markers))
    if foreign_completion_tokens:
        raise AssertionError("AX 读回了其他 session completion token: " + ", ".join(foreign_completion_tokens))
    return {
        "prompt_marker_visible": marker in value,
        "completion_token_visible": completion_token in value,
        "foreign_markers": foreign_markers,
        "foreign_completion_tokens": foreign_completion_tokens,
    }


def summarize_final_completion(
    records: list[dict[str, Any]],
    expected_record_count: int,
) -> dict[str, Any]:
    """Expose final-token status without copying AX text or controller rows."""
    return {
        "assertion": (
            "PASS"
            if len(records) == expected_record_count
            and expected_record_count > 0
            and all(record.get("status") == "PASS" for record in records)
            else "FAIL"
        ),
        "record_count": len(records),
        "expected_record_count": expected_record_count,
        "records": [
            {
                "sample": record.get("sample"),
                "session_marker": record.get("session_marker"),
                "status": record.get("status"),
                "attempt_count": len(record.get("attempts") or []),
                "contains_own_marker": record.get("contains_own_marker"),
                "contains_completion_token": record.get("contains_completion_token"),
                "foreign_marker_check": record.get("foreign_marker_check"),
                "foreign_completion_token_check": record.get("foreign_completion_token_check"),
                "error_type": record.get("error_type"),
            }
            for record in records
        ],
    }


def require_final_completion_pass(check: dict[str, Any]) -> None:
    """Keep the successful native path gated by the complete final-token check."""
    if check.get("assertion") != "PASS":
        raise AssertionError("最终 final completion 断言失败")


def redact_record(record: dict[str, Any]) -> dict[str, Any]:
    """Copy only safe fixture timing fields; never persist headers or body."""
    allowed = (
        "request_index",
        "session_marker",
        "started_at",
        "first_chunk_at",
        "last_chunk_at",
        "completed_at",
        "chunk_count",
        "response_bytes",
        "complete",
        "disconnected",
        "concurrent_at_start",
        "peak_concurrent",
        "duration_seconds",
    )
    return {key: record.get(key) for key in allowed}


def validate_server_records(
    records: list[dict[str, Any]],
    markers: list[str],
    duration: float,
    hz: float,
) -> dict[str, Any]:
    failures: list[str] = []
    by_marker: dict[str, dict[str, Any]] = {}
    expected_chunks = max(1, int(math.floor(duration * hz * 0.70)))
    for record in records:
        marker = record.get("session_marker")
        if marker in by_marker:
            failures.append(f"duplicate request marker {marker}")
        by_marker[str(marker)] = record
        if marker not in markers:
            failures.append("unknown request marker")
        if not record.get("complete"):
            failures.append(f"{marker} stream did not complete")
        if record.get("chunk_count", 0) < expected_chunks:
            failures.append(f"{marker} has too few chunks")
        if not record.get("first_chunk_at") or not record.get("last_chunk_at"):
            failures.append(f"{marker} lacks first/last chunk timestamp")
        if int(record.get("response_bytes", 0)) <= 0:
            failures.append(f"{marker} has no response bytes")
    missing = [marker for marker in markers if marker not in by_marker]
    failures.extend(f"missing request {marker}" for marker in missing)
    peak = max((int(record.get("peak_concurrent", 0)) for record in records), default=0)
    if peak < len(markers):
        failures.append(f"SSE peak concurrency {peak} < {len(markers)}")
    safe = [redact_record(record) for record in records]
    return {
        "records": safe,
        "expected_chunks_at_least": expected_chunks,
        "peak_concurrent": peak,
        "assertion": "PASS" if not failures else "FAIL",
        "failures": failures,
    }


class StreamFixture:
    """A loopback-only continuous SSE fixture with redacted in-memory records."""

    def __init__(self, markers: list[str], duration: float, hz: float):
        self.markers = tuple(markers)
        self.duration = duration
        self.hz = hz
        self.lock = threading.Lock()
        self.records: list[dict[str, Any]] = []
        self.chunk_events: list[dict[str, Any]] = []
        self.errors: list[str] = []
        self.active = 0
        self.peak = 0
        self.stop_event = threading.Event()

    def error(self, message: str) -> None:
        with self.lock:
            self.errors.append(message)

    def begin(self, marker: str, authorization: str) -> int:
        if authorization != "Bearer " + LOCAL_FIXTURE_KEY:
            self.error("provider authorization rejected")
            raise ValueError("invalid fixture authorization")
        if marker not in self.markers:
            self.error("provider request marker is unknown")
            raise ValueError("unknown fixture marker")
        with self.lock:
            self.active += 1
            self.peak = max(self.peak, self.active)
            record = {
                "request_index": len(self.records) + 1,
                "session_marker": marker,
                "started_at": utc_now(),
                "first_chunk_at": None,
                "last_chunk_at": None,
                "completed_at": None,
                "chunk_count": 0,
                "response_bytes": 0,
                "complete": False,
                "disconnected": False,
                "concurrent_at_start": self.active > 1,
                "peak_concurrent": self.peak,
                "duration_seconds": self.duration,
            }
            self.records.append(record)
            return int(record["request_index"])

    def chunk(self, request_index: int, size: int) -> None:
        with self.lock:
            record = self.records[request_index - 1]
            now = utc_now()
            if record["first_chunk_at"] is None:
                record["first_chunk_at"] = now
            record["last_chunk_at"] = now
            record["chunk_count"] += 1
            record["response_bytes"] += size
            self.chunk_events.append(
                {
                    "request_index": request_index,
                    "session_marker": record["session_marker"],
                    "chunk_index": record["chunk_count"],
                    "timestamp": now,
                    "bytes": size,
                }
            )

    def finish(self, request_index: int, *, complete: bool, disconnected: bool) -> None:
        with self.lock:
            record = self.records[request_index - 1]
            record["completed_at"] = utc_now()
            record["complete"] = complete
            record["disconnected"] = disconnected
            record["peak_concurrent"] = max(int(record["peak_concurrent"]), self.peak)
            self.active = max(0, self.active - 1)

    def snapshot(self) -> tuple[list[dict[str, Any]], list[str], int]:
        with self.lock:
            return ([dict(record) for record in self.records], list(self.errors), self.peak)

    def diagnostic_status(self) -> dict[str, Any]:
        with self.lock:
            completed = sum(1 for record in self.records if record.get("complete"))
            return {
                "request_count": len(self.records),
                "active_request_count": self.active,
                "completed_request_count": completed,
                "all_complete": len(self.records) == len(self.markers) and completed == len(self.markers),
                "peak_concurrent": self.peak,
                "error_count": len(self.errors),
            }

    def chunk_snapshot(self) -> list[dict[str, Any]]:
        with self.lock:
            return [dict(event) for event in self.chunk_events]

    def all_complete(self) -> bool:
        with self.lock:
            return len(self.records) == len(self.markers) and all(record["complete"] for record in self.records)

    def stop(self) -> None:
        self.stop_event.set()


class FixtureHandler(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    server: "FixtureHTTPServer"

    def log_message(self, _format: str, *_args: Any) -> None:
        return

    def do_POST(self) -> None:  # noqa: N802 - BaseHTTPRequestHandler API
        fixture = self.server.fixture
        if self.path != CHAT_PATH:
            fixture.error("unexpected provider path")
            self.send_error(http.HTTPStatus.NOT_FOUND)
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
            if length <= 0 or length > 16 * 1024 * 1024:
                raise ValueError("invalid request length")
            body = json.loads(self.rfile.read(length))
            marker = extract_marker(last_user_text(body), list(fixture.markers))
            request_index = fixture.begin(marker, self.headers.get("Authorization", ""))
        except (AssertionError, ValueError, TypeError, json.JSONDecodeError) as error:
            fixture.error("invalid provider request")
            self.send_error(http.HTTPStatus.BAD_REQUEST, str(error))
            return

        self.close_connection = True
        self.send_response(http.HTTPStatus.OK)
        self.send_header("Content-Type", "text/event-stream; charset=utf-8")
        self.send_header("Cache-Control", "no-cache, no-transform")
        self.send_header("Connection", "close")
        self.send_header("X-Accel-Buffering", "no")
        self.end_headers()
        interval = 1.0 / fixture.hz
        total_chunks = max(1, int(math.ceil(fixture.duration * fixture.hz)))
        started = time.monotonic()
        disconnected = False
        complete = False
        try:
            for index in range(total_chunks):
                target = started + index * interval
                delay = target - time.monotonic()
                if delay > 0 and fixture.stop_event.wait(delay):
                    break
                if fixture.stop_event.is_set():
                    break
                fragment = stream_fragment(marker, index)
                if index == total_chunks - 1:
                    # Keep the completion marker in the existing final content
                    # chunk. This changes neither chunk count nor stream timing.
                    fragment += f"\n\n流式完成令牌：{stream_completion_token(marker)}"
                payload = sse_event({"content": fragment})
                self.wfile.write(payload)
                self.wfile.flush()
                fixture.chunk(request_index, len(payload))
            else:
                terminal = sse_event({}, "stop") + sse_done()
                self.wfile.write(terminal)
                self.wfile.flush()
                complete = True
        except (BrokenPipeError, ConnectionResetError, OSError):
            disconnected = True
        finally:
            fixture.finish(request_index, complete=complete, disconnected=disconnected)


class FixtureHTTPServer(http.server.ThreadingHTTPServer):
    daemon_threads = True
    allow_reuse_address = True

    def __init__(self, fixture: StreamFixture):
        super().__init__(("127.0.0.1", 0), FixtureHandler)
        self.fixture = fixture


def exists_ax(app: subprocess.Popen[str], reference: str) -> bool:
    return ax(app, "return exists " + reference) == "true"


def send_prompt(app: subprocess.Popen[str], prompt: str) -> None:
    wait_until(lambda: exists_ax(app, 'text area 1 of window 1'), "任务输入框没有出现")
    value = json_string(prompt)
    ax(app, f"set value of text area 1 of window 1 to {value}\nclick button \"发送\" of window 1")


def configure_provider(app: subprocess.Popen[str], data_dir: pathlib.Path, project: pathlib.Path, base_url: str) -> tuple[str, str]:
    ax(app, 'set frontmost to true\nclick button "设置" of window 1')
    ax(app, 'click button "添加服务" of scroll area 1 of window 1')
    wait_until(lambda: ax(app, "return count of text fields of scroll area 1 of window 1") == "4", "provider editor 没有出现")
    for field, value in (
        (1, "原生 streaming fixture"),
        (2, base_url + "/v1"),
        (3, "native-streaming-fixture-model"),
        (4, LOCAL_FIXTURE_KEY),
    ):
        ax(app, f"set value of text field {field} of scroll area 1 of window 1 to {json_string(value)}")
    ax(app, 'click button "保存服务" of window 1')
    saved = wait_snapshot(
        data_dir,
        lambda snapshot: any(
            provider.get("name") == "原生 streaming fixture"
            and provider.get("base_url") == base_url + "/v1"
            and provider.get("model") == "native-streaming-fixture-model"
            and provider.get("format") == "chat-completions"
            for provider in snapshot.get("settings", {}).get("providers") or []
        ),
        "provider 保存后没有进入持久化 snapshot",
    )
    provider = next(
        provider for provider in saved["settings"]["providers"] if provider.get("name") == "原生 streaming fixture"
    )
    credential_file = data_dir / "credentials" / "credentials.enc"
    wait_until(lambda: credential_file.exists(), "本地 fixture credential 没有落入隔离凭据目录")
    ax(app, 'click radio button "深色" of radio group 1 of scroll area 1 of window 1')
    ax(app, 'click button "保存设置" of window 1')
    wait_snapshot(
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
    project_snapshot = wait_snapshot(data_dir, lambda snapshot: len(snapshot.get("projects") or []) == 1, "项目没有打开")
    return str(provider["id"]), str(project_snapshot["projects"][0]["id"])


def create_sessions(app: subprocess.Popen[str], data_dir: pathlib.Path, project_id: str, count: int) -> list[dict[str, Any]]:
    for expected in range(1, count + 1):
        ax(app, 'click button "新会话" of window 1')
        wait_snapshot(
            data_dir,
            lambda snapshot, expected=expected: sum(1 for session in snapshot.get("sessions") or [] if session.get("project_id") == project_id)
            == expected,
            f"第 {expected} 个会话没有创建",
        )
    snapshot = wait_snapshot(
        data_dir,
        lambda value: sum(1 for session in value.get("sessions") or [] if session.get("project_id") == project_id) == count,
        "同项目会话数量不正确",
    )
    sessions = [session for session in snapshot.get("sessions") or [] if session.get("project_id") == project_id]
    if len(sessions) != count:
        raise AssertionError("同项目会话快照数量不正确")
    cwds = {session.get("cwd") for session in sessions}
    if len(cwds) != 1:
        raise AssertionError("三会话没有共享同一项目工作目录")
    return sessions


def session_button_label(session: dict[str, Any]) -> str:
    title = str(session.get("title") or "未命名会话")
    return "会话 " + title


def click_session(app: subprocess.Popen[str], session: dict[str, Any], session_index: int) -> None:
    """Press the session row by its sidebar index, then verify AX name/description.

    Session rows are rendered in ``data.Sessions`` order inside sidebar scroll area
    1. Searching ``entire contents`` by a duplicate name can resolve every row to
    the first accessibility element, so this intentionally uses the concrete
    ``button <index> of scroll area 1 of window 1`` specifier.
    """
    if session_index <= 0:
        raise AssertionError(f"session sidebar index 必须为正数: {session_index}")
    expected = session_button_label(session)
    script = f'''set expectedLabel to {json_string(expected)}
set actualName to ""
set actualDescription to ""
try
 set actualName to (name of button {session_index} of scroll area 1 of window 1) as text
end try
try
 set actualDescription to (description of button {session_index} of scroll area 1 of window 1) as text
end try
if actualName is not expectedLabel and actualDescription is not expectedLabel then
 error "session sidebar index {session_index} mismatch: name=" & actualName & ", description=" & actualDescription & ", expected=" & expectedLabel
end if
click button {session_index} of scroll area 1 of window 1
return actualName & tab & actualDescription'''
    observed = ax(app, script)
    if expected not in observed.split("\t"):
        raise AssertionError(f"session sidebar index {session_index} AX name/description 未匹配: {observed!r}")


def wait_empty_session_ax(
    app: subprocess.Popen[str],
    sent_markers: list[str],
    session_id: str,
) -> str:
    """Wait until a newly selected empty session is visible in the AX table.

    The click is delivered before mygo consumes it in the next frame. An empty
    conversation has the two empty-state texts directly under the table and no
    message row. Rejecting every marker already sent by earlier sessions and
    every AX row makes the readback prove that selection was consumed before
    the next prompt is written.
    """
    value = ""

    def selected_empty() -> bool:
        nonlocal value
        value = read_ax_table(app)
        if any(marker in value for marker in sent_markers):
            return False
        return "of row " not in value

    wait_until(selected_empty, f"AX 没有读回空 session {session_id} 的会话内容", timeout=18, interval=0.2)
    return value


def wait_session_marker(
    data_dir: pathlib.Path,
    sessions: list[dict[str, Any]],
    target_session_id: str,
    marker: str,
) -> dict[str, Any]:
    """Wait for the sent marker in its target session and reject foreign ownership."""
    session_ids = [str(session["id"]) for session in sessions]

    def check(snapshot: dict[str, Any]) -> bool:
        target = session_by_id(snapshot, target_session_id)
        foreign = [
            session_id
            for session_id in session_ids
            if session_id != target_session_id and marker in "\n".join(session_message_texts(session_by_id(snapshot, session_id)))
        ]
        if foreign:
            raise RuntimeError(f"marker {marker} 被错误写入其他 session: {', '.join(foreign)}")
        return any(marker in str(message.get("text", "")) for message in session_messages(target, "user"))

    return wait_snapshot(data_dir, check, f"发送后目标 session {target_session_id} 没有 marker {marker}")


def read_ax_table(app: subprocess.Popen[str]) -> str:
    return ax(app, "return entire contents of table 1 of window 1")


def read_ax_last_message_row(app: subprocess.Popen[str], timeout: float = MAX_AX_SECONDS) -> str:
    """Read only direct static-text values from the last message row.

    Serializing ``entire contents`` of a long streamed conversation makes
    System Events materialize every nested AX object specifier, which can
    exceed the driver timeout. The tail token is rendered as direct static
    text in the latest message row, so this keeps the AX assertion small while
    still reading the native UI value rather than controller state.
    """
    return ax(
        app,
        "return (value of every static text of last row of table 1 of window 1) as text",
        timeout=timeout,
    )


def ax_diagnostic_context(
    data_dir: pathlib.Path,
    fixture: StreamFixture,
    session_id: str,
) -> dict[str, Any]:
    """Capture bounded controller/fixture state around one AX operation."""
    context: dict[str, Any] = {"observed_at": utc_now()}
    try:
        target = session_by_id(read_snapshot(data_dir), session_id)
        context["controller"] = {
            "state": target.get("state"),
            "run_id": target.get("run_id"),
            "active_run": target.get("active_run") is not None,
        }
    except Exception as error:
        context["controller"] = {"error_type": redacted_error_type(error)}
    try:
        context["fixture"] = fixture.diagnostic_status()
    except Exception as error:
        context["fixture"] = {"error_type": redacted_error_type(error)}
    return context


def read_session_ax(
    app: subprocess.Popen[str],
    session: dict[str, Any],
    marker: str,
    session_index: int,
    data_dir: pathlib.Path,
    fixture: StreamFixture,
    output: pathlib.Path,
    samples: list[dict[str, Any]],
    name: str,
    all_markers: list[str],
    *,
    clock=time.monotonic,
    sleeper=time.sleep,
) -> str:
    started = time.perf_counter()
    session_id = str(session["id"])
    value = ""
    tail_token = stream_tail_token(marker)
    all_tail_tokens = [stream_tail_token(other) for other in all_markers]
    record: dict[str, Any] = {
        "kind": "ax_readback",
        "source": "foreground",
        "session_marker": marker,
        "stream_tail_token": tail_token,
        "sample": name,
        "status": "RUNNING",
        "outer_budget_ms": 18000.0,
        "attempts": [],
    }

    record["click_before"] = ax_diagnostic_context(data_dir, fixture, session_id)
    click_started = time.perf_counter()
    record["click_started_at"] = utc_now()
    try:
        click_session(app, session, session_index)
        record["click_success"] = True
    except Exception as error:
        record["click_success"] = False
        record["click_error_type"] = redacted_error_type(error)
        record["status"] = "FAIL"
        record["error_type"] = record["click_error_type"]
        record["latency_ms"] = (time.perf_counter() - started) * 1000
        samples.append(record)
        raise
    finally:
        record["click_ended_at"] = utc_now()
        record["click_ms"] = (time.perf_counter() - click_started) * 1000
        record["click_after"] = ax_diagnostic_context(data_dir, fixture, session_id)

    attempt_number = 0

    def has_tail_token() -> bool:
        nonlocal attempt_number, value
        attempt_number += 1
        attempt: dict[str, Any] = {
            "attempt": attempt_number,
            "success": False,
            "tail_visible": False,
            "text_bytes": 0,
            "effective_timeout_ms": 0.0,
        }
        attempt["before"] = ax_diagnostic_context(data_dir, fixture, session_id)
        attempt_started = time.perf_counter()
        attempt["started_at"] = utc_now()
        value = ""
        try:
            remaining = query_deadline - clock()
            attempt["remaining_before_query_ms"] = max(0.0, remaining * 1000)
            if remaining <= 0:
                attempt["error_type"] = "deadline_exceeded"
                attempt["query_skipped"] = True
                raise WaitDeadlineExceeded(f"AX 查询 {tail_token} 没有剩余 outer deadline")
            effective_timeout = min(float(MAX_AX_SECONDS), remaining)
            attempt["effective_timeout_ms"] = effective_timeout * 1000
            attempt["query_skipped"] = False
            value = read_ax_last_message_row(app, timeout=effective_timeout)
            attempt["success"] = True
            attempt["tail_visible"] = tail_token in value
            attempt["text_bytes"] = len(value.encode("utf-8"))
            # Persist only ownership facts derived from the known fixture
            # tokens. The AX text itself stays in memory for this assertion
            # and is never written to evidence.
            attempt["own_marker_visible"] = marker in value
            attempt["foreign_markers"] = [
                other for other in all_markers if other != marker and other in value
            ]
            attempt["foreign_tail_tokens"] = [
                other for other in all_tail_tokens if other != tail_token and other in value
            ]
            return bool(attempt["tail_visible"])
        except Exception as error:
            attempt["error_type"] = redacted_error_type(error)
            raise
        finally:
            attempt["ended_at"] = utc_now()
            attempt["duration_ms"] = (time.perf_counter() - attempt_started) * 1000
            attempt["after"] = ax_diagnostic_context(data_dir, fixture, session_id)
            record["attempts"].append(attempt)

    record["wait_started_at"] = utc_now()
    query_deadline = clock() + 18.0
    try:
        wait_until(
            has_tail_token,
            f"AX 没有读回 {tail_token}",
            timeout=18,
            interval=0.2,
            clock=clock,
            sleeper=sleeper,
            deadline=query_deadline,
        )
        record["wait_ended_at"] = utc_now()
        viewport = validate_ax_readback(value, marker, tail_token, all_markers, all_tail_tokens)
        record.update(
            {
                "status": "PASS",
                "text_bytes": len(value.encode("utf-8")),
                "contains_own_marker": viewport["prompt_marker_visible"],
                "contains_tail_token": viewport["tail_token_visible"],
                "foreign_marker_check": viewport["foreign_markers"],
                "foreign_tail_token_check": viewport["foreign_tail_tokens"],
            }
        )
        return value
    except Exception as error:
        record["status"] = "FAIL"
        record["error_type"] = redacted_error_type(error)
        raise
    finally:
        record.setdefault("wait_ended_at", utc_now())
        record["latency_ms"] = (time.perf_counter() - started) * 1000
        samples.append(record)


def read_session_completion_ax(
    app: subprocess.Popen[str],
    session: dict[str, Any],
    marker: str,
    session_index: int,
    data_dir: pathlib.Path,
    fixture: StreamFixture,
    output: pathlib.Path,
    samples: list[dict[str, Any]],
    name: str,
    all_markers: list[str],
    *,
    clock=time.monotonic,
    sleeper=time.sleep,
) -> dict[str, Any]:
    """Read the final completion token from the native last message row."""
    started = time.perf_counter()
    session_id = str(session["id"])
    value = ""
    completion_token = stream_completion_token(marker)
    all_completion_tokens = [stream_completion_token(other) for other in all_markers]
    record: dict[str, Any] = {
        "kind": "final_completion",
        "source": "foreground",
        "session_marker": marker,
        "completion_token": completion_token,
        "sample": name,
        "status": "RUNNING",
        "outer_budget_ms": 18000.0,
        "attempts": [],
    }

    record["click_before"] = ax_diagnostic_context(data_dir, fixture, session_id)
    click_started = time.perf_counter()
    record["click_started_at"] = utc_now()
    try:
        click_session(app, session, session_index)
        record["click_success"] = True
    except Exception as error:
        record["click_success"] = False
        record["click_error_type"] = redacted_error_type(error)
        record["status"] = "FAIL"
        record["error_type"] = record["click_error_type"]
        record["latency_ms"] = (time.perf_counter() - started) * 1000
        samples.append(record)
        raise
    finally:
        record["click_ended_at"] = utc_now()
        record["click_ms"] = (time.perf_counter() - click_started) * 1000
        record["click_after"] = ax_diagnostic_context(data_dir, fixture, session_id)

    attempt_number = 0

    def has_completion_token() -> bool:
        nonlocal attempt_number, value
        attempt_number += 1
        attempt: dict[str, Any] = {
            "attempt": attempt_number,
            "success": False,
            "completion_visible": False,
            "text_bytes": 0,
            "effective_timeout_ms": 0.0,
        }
        attempt["before"] = ax_diagnostic_context(data_dir, fixture, session_id)
        attempt_started = time.perf_counter()
        attempt["started_at"] = utc_now()
        value = ""
        try:
            remaining = query_deadline - clock()
            attempt["remaining_before_query_ms"] = max(0.0, remaining * 1000)
            if remaining <= 0:
                attempt["error_type"] = "deadline_exceeded"
                attempt["query_skipped"] = True
                raise WaitDeadlineExceeded(
                    f"AX 查询 {completion_token} 没有剩余 outer deadline"
                )
            effective_timeout = min(float(MAX_AX_SECONDS), remaining)
            attempt["effective_timeout_ms"] = effective_timeout * 1000
            attempt["query_skipped"] = False
            value = read_ax_last_message_row(app, timeout=effective_timeout)
            attempt["success"] = True
            attempt["completion_visible"] = completion_token in value
            attempt["text_bytes"] = len(value.encode("utf-8"))
            # Keep only ownership facts for known fixture tokens. The AX value
            # remains in memory and is never written to evidence.
            attempt["own_marker_visible"] = marker in value
            attempt["foreign_markers"] = [
                other for other in all_markers if other != marker and other in value
            ]
            attempt["own_completion_visible"] = completion_token in value
            attempt["foreign_completion_tokens"] = [
                other
                for other in all_completion_tokens
                if other != completion_token and other in value
            ]
            return bool(attempt["completion_visible"])
        except Exception as error:
            attempt["error_type"] = redacted_error_type(error)
            raise
        finally:
            attempt["ended_at"] = utc_now()
            attempt["duration_ms"] = (time.perf_counter() - attempt_started) * 1000
            attempt["after"] = ax_diagnostic_context(data_dir, fixture, session_id)
            record["attempts"].append(attempt)

    record["wait_started_at"] = utc_now()
    query_deadline = clock() + 18.0
    try:
        wait_until(
            has_completion_token,
            f"AX 没有读回 {completion_token}",
            timeout=18,
            interval=0.2,
            clock=clock,
            sleeper=sleeper,
            deadline=query_deadline,
        )
        record["wait_ended_at"] = utc_now()
        completion = validate_ax_completion_readback(
            value,
            marker,
            completion_token,
            all_markers,
            all_completion_tokens,
        )
        record.update(
            {
                "status": "PASS",
                "text_bytes": len(value.encode("utf-8")),
                "contains_own_marker": completion["prompt_marker_visible"],
                "contains_completion_token": completion["completion_token_visible"],
                "foreign_marker_check": completion["foreign_markers"],
                "foreign_completion_token_check": completion["foreign_completion_tokens"],
            }
        )
        return record
    except Exception as error:
        record["status"] = "FAIL"
        record["error_type"] = redacted_error_type(error)
        raise
    finally:
        record.setdefault("wait_ended_at", utc_now())
        record["latency_ms"] = (time.perf_counter() - started) * 1000
        samples.append(record)


class DiagnosticCollector:
    """Collect controller/process samples independently of synchronous AX calls."""

    def __init__(
        self,
        app: subprocess.Popen[str],
        data_dir: pathlib.Path,
        fixture: StreamFixture,
        sessions: list[dict[str, Any]],
    ):
        self.app = app
        self.data_dir = data_dir
        self.fixture = fixture
        self.sessions = sessions
        self.controller_interval = 0.2
        self.process_interval = 1.0
        self.stop_event = threading.Event()
        self.lock = threading.Lock()
        self.samples: list[dict[str, Any]] = []
        self.errors: list[dict[str, Any]] = []
        self.started = False
        self.joined = False
        self.thread = threading.Thread(target=self._run, name="native-streaming-diagnostics", daemon=True)

    def start(self) -> None:
        self.thread.start()
        self.started = True

    def _append(self, sample: dict[str, Any]) -> None:
        with self.lock:
            self.samples.append(sample)

    def _record_error(self, source: str, error: BaseException) -> None:
        record = {
            "kind": "collector_error",
            "source": source,
            "timestamp": utc_now(),
            "error_type": redacted_error_type(error),
        }
        with self.lock:
            self.errors.append(record)
            self.samples.append(record)

    def _controller_sample(self) -> None:
        started = time.perf_counter()
        try:
            snapshot = read_snapshot(self.data_dir)
            fixture_status = self.fixture.diagnostic_status()
            self._append(
                {
                    "kind": "controller_poll",
                    "source": "background",
                    "timestamp": utc_now(),
                    "latency_ms": (time.perf_counter() - started) * 1000,
                    "request_count": fixture_status["request_count"],
                    "active_request_count": fixture_status["active_request_count"],
                    "completed_request_count": fixture_status["completed_request_count"],
                    "fixture_all_complete": fixture_status["all_complete"],
                    "peak_concurrent": fixture_status["peak_concurrent"],
                    "states": {
                        str(session["id"]): session_by_id(snapshot, str(session["id"])).get("state")
                        for session in self.sessions
                    },
                    "run_ids": {
                        str(session["id"]): session_by_id(snapshot, str(session["id"])).get("run_id")
                        for session in self.sessions
                    },
                }
            )
        except Exception as error:
            self._record_error("controller", error)

    def _process_sample(self) -> None:
        if self.app.poll() is not None:
            return
        try:
            raw = run_command("ps", "-o", "rss=,pcpu=", "-p", str(self.app.pid), timeout=5)
            fields = raw.split()
            if len(fields) < 2:
                raise ValueError("ps sample missing fields")
            self._append(
                {
                    "kind": "process",
                    "source": "background",
                    "phase": "streaming",
                    "timestamp": utc_now(),
                    "rss_kib": int(fields[0]),
                    "cpu_percent": float(fields[1]),
                }
            )
        except Exception as error:
            self._record_error("process", error)

    def _run(self) -> None:
        next_controller = time.monotonic()
        next_process = next_controller
        try:
            while not self.stop_event.is_set():
                now = time.monotonic()
                if now >= next_controller:
                    self._controller_sample()
                    next_controller += self.controller_interval
                    if next_controller <= time.monotonic():
                        next_controller = time.monotonic() + self.controller_interval
                if now >= next_process:
                    self._process_sample()
                    next_process += self.process_interval
                    if next_process <= time.monotonic():
                        next_process = time.monotonic() + self.process_interval
                wait_for = max(0.0, min(next_controller, next_process) - time.monotonic())
                self.stop_event.wait(wait_for)
        except Exception as error:
            self._record_error("thread", error)

    def stop_join(self, timeout: float = 8.0) -> bool:
        if not self.started:
            self.joined = True
            return True
        self.stop_event.set()
        self.thread.join(timeout=timeout)
        self.joined = not self.thread.is_alive()
        if not self.joined:
            self._record_error("thread_join", RuntimeError("diagnostic collector join timeout"))
        return self.joined

    def snapshot(self) -> dict[str, Any]:
        with self.lock:
            samples = [dict(sample) for sample in self.samples]
            errors = [dict(error) for error in self.errors]
        return {
            "status": "NOT_STARTED" if not self.started else ("PASS" if not errors and self.joined else "ERRORS_RECORDED"),
            "observation_only": True,
            "controller_interval_seconds": self.controller_interval,
            "process_interval_seconds": self.process_interval,
            "joined": self.joined,
            "sample_count": len(samples),
            "controller_poll_count": sum(1 for sample in samples if sample.get("kind") == "controller_poll"),
            "process_sample_count": sum(1 for sample in samples if sample.get("kind") == "process"),
            "error_count": len(errors),
            "errors": errors,
            "samples": samples,
        }


def write_json(path: pathlib.Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")


def write_ready(
    path: pathlib.Path,
    qa_identity: str,
    data_dir: pathlib.Path,
    app: subprocess.Popen[str] | None,
    phase: str,
    **extra: Any,
) -> None:
    payload: dict[str, Any] = {
        "qa_identity": qa_identity,
        "pid": app.pid if app is not None else None,
        "data_directory": str(data_dir),
        "phase": phase,
        "updated_at": utc_now(),
        "process_alive": app is not None and app.poll() is None,
        "exit_code": app.returncode if app is not None and app.poll() is not None else None,
    }
    payload.update(extra)
    write_json(path, payload)


def compile_scroll_helper(output: pathlib.Path) -> pathlib.Path:
    source = output / "synthetic-scroll.swift"
    source.write_text(
        '''import CoreGraphics
import Foundation
let pid = Int32(CommandLine.arguments[1])!
let x = Double(CommandLine.arguments[2])!, y = Double(CommandLine.arguments[3])!
let delta = Int32(CommandLine.arguments[4])!
guard let original = CGEvent(source: nil)?.location else { fatalError("cursor unavailable") }
func move(_ point: CGPoint) {
    guard let event = CGEvent(mouseEventSource: nil, mouseType: .mouseMoved, mouseCursorPosition: point, mouseButton: .left) else { fatalError("pointer event unavailable") }
    event.post(tap: .cghidEventTap)
}
defer { move(original) }
move(CGPoint(x: x, y: y))
Thread.sleep(forTimeInterval: 0.15)
guard let event = CGEvent(scrollWheelEvent2Source: nil, units: .pixel, wheelCount: 1, wheel1: delta, wheel2: 0, wheel3: 0) else { fatalError("scroll event unavailable") }
event.location = CGPoint(x: x, y: y)
event.post(tap: .cghidEventTap)
Thread.sleep(forTimeInterval: 0.15)
move(original)
Thread.sleep(forTimeInterval: 0.15)
guard let after = CGEvent(source: nil)?.location else { fatalError("cursor readback unavailable") }
let restored = abs(after.x - original.x) < 1 && abs(after.y - original.y) < 1
let record: [String: Any] = ["pid": pid, "original": [original.x, original.y], "after": [after.x, after.y], "restored": restored]
print(String(data: try! JSONSerialization.data(withJSONObject: record), encoding: .utf8)!)
if !restored { exit(1) }
'''
    )
    executable = output / "synthetic-scroll"
    run_command("swiftc", str(source), "-o", str(executable), timeout=60)
    return executable


def compile_cgwindow_helper(output: pathlib.Path) -> pathlib.Path:
    """Compile a small CoreGraphics probe used to bind screenshots to this PID."""
    source = output / "cgwindow-list.swift"
    source.write_text(
        '''import CoreGraphics
import Foundation

let pid = Int32(CommandLine.arguments[1])!
let windows = CGWindowListCopyWindowInfo([.optionOnScreenOnly, .excludeDesktopElements], kCGNullWindowID) as? [[String: Any]] ?? []
var matches: [[String: Any]] = []
for window in windows {
    guard let ownerPID = (window[kCGWindowOwnerPID as String] as? NSNumber)?.int32Value,
          ownerPID == pid,
          let windowID = (window[kCGWindowNumber as String] as? NSNumber)?.intValue,
          windowID > 0 else { continue }
    let ownerName = (window[kCGWindowOwnerName as String] as? String) ?? ""
    let name = (window[kCGWindowName as String] as? String) ?? ""
    let layer = (window[kCGWindowLayer as String] as? NSNumber)?.intValue ?? 0
    matches.append(["window_id": windowID, "owner_pid": Int(ownerPID), "owner_name": ownerName, "name": name, "layer": layer])
}
let data = try JSONSerialization.data(withJSONObject: matches, options: [])
print(String(data: data, encoding: .utf8)!)
'''
    )
    executable = output / "cgwindow-list"
    run_command("swiftc", str(source), "-o", str(executable), timeout=60)
    return executable


def assert_workbench_for_capture(app: subprocess.Popen[str]) -> None:
    """Ensure visual evidence is taken from the workbench, never the API-key editor."""
    if not exists_ax(app, 'button "设置" of window 1') or exists_ax(app, 'button "返回工作台" of window 1'):
        raise AssertionError("截图时原生窗口不在工作台，拒绝把设置/API key 页当作视觉证据")


def capture_window(
    helper: pathlib.Path,
    app: subprocess.Popen[str],
    output: pathlib.Path,
    phase: str,
    controller_state: dict[str, Any] | None = None,
) -> dict[str, Any]:
    """Capture the on-screen CGWindow owned by the QA PID and retain its hash."""
    if app.poll() is not None:
        raise AssertionError(f"截图前 QA 应用已退出，退出码={app.returncode}")
    assert_workbench_for_capture(app)
    windows = json.loads(run_command(str(helper), str(app.pid), timeout=15))
    candidates = [
        window
        for window in windows
        if int(window.get("owner_pid", -1)) == app.pid and int(window.get("window_id", 0)) > 0
    ]
    candidates.sort(key=lambda window: (int(window.get("layer", 0)) != 0, not bool(window.get("name"))))
    if not candidates:
        raise AssertionError(f"CGWindow 没有找到 QA PID {app.pid} 的可截图窗口")
    window = candidates[0]
    capture_root = output / "captures"
    capture_root.mkdir(parents=True, exist_ok=True)
    image = capture_root / f"{phase}-window.png"
    run_command("screencapture", "-x", "-l", str(int(window["window_id"])), str(image), timeout=30)
    if not image.is_file() or image.stat().st_size <= 0:
        raise AssertionError(f"screencapture 没有生成 {image}")
    record: dict[str, Any] = {
        "phase": phase,
        "path": str(image),
        "sha256": digest(image),
        "bytes": image.stat().st_size,
        "pid": app.pid,
        "cgwindow": window,
        "visual_approval": "NOT CLAIMED; image is retained for human review",
    }
    if controller_state is not None:
        record["controller_state"] = controller_state
    return record


def capture_failure_window(
    helper: pathlib.Path | None,
    app: subprocess.Popen[str] | None,
    output: pathlib.Path,
    manifest: dict[str, Any],
    summary: dict[str, Any],
    primary_error: str | None,
) -> None:
    """Retain best-effort failure-window evidence without changing the failure."""
    if primary_error is None:
        return

    failure: dict[str, Any] = {
        "phase": "failure",
        "status": "ERROR",
        "primary_error": primary_error,
    }
    try:
        if helper is None:
            raise RuntimeError("CGWindow helper 未编译")
        if app is None:
            raise RuntimeError("QA 应用未启动")
        if app.poll() is not None:
            raise RuntimeError(f"截图前 QA 应用已退出，退出码={app.returncode}")
        failure = capture_window(
            helper,
            app,
            output,
            "failure",
            {"primary_error": primary_error},
        )
        failure["status"] = "PASS"
    except Exception as error:
        failure.update(
            {
                "status": "ERROR",
                "error_type": redacted_error_type(error),
                "error": str(error)[:240],
            }
        )
        diagnostic_error = {
            "kind": "failure_window_capture",
            "error_type": failure["error_type"],
            "error": failure["error"],
        }
        manifest.setdefault("diagnostic_errors", []).append(diagnostic_error)

    manifest.setdefault("visual_evidence", {})["failure"] = failure
    summary.setdefault("checks", {})["failure_window_capture"] = failure


def quit_app(app: subprocess.Popen[str]) -> bool:
    if app.poll() is not None:
        return app.returncode == 0
    try:
        # Locate the actual application menu by the Quit item. The first menu
        # bar item is normally the app menu, but its position must not be
        # guessed from a returned name list.
        ax(
            app,
            '''set quitLabel to "退出 GCode"
set found to false
repeat with menuBarItemRef in menu bar items of menu bar 1
 set menuBarItem to contents of menuBarItemRef
 try
  if exists menu item quitLabel of menu 1 of menuBarItem then
   click menu item quitLabel of menu 1 of menuBarItem
   set found to true
   exit repeat
  end if
 end try
end repeat
if found is false then error "application Quit menu item not found"
return "requested"''',
        )
        wait_until(
            lambda: app.poll() is not None
            or exists_ax(app, 'button "停止并退出" of sheet 1 of window 1'),
            "退出菜单没有产生退出或停止确认",
            timeout=8,
        )
        if app.poll() is None:
            ax(app, 'click button "停止并退出" of sheet 1 of window 1')
        app.wait(timeout=20)
        return app.returncode == 0
    except (AxError, subprocess.TimeoutExpired, subprocess.SubprocessError, AssertionError):
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


def remove_credentials(data_dir: pathlib.Path) -> bool:
    credentials = data_dir / "credentials"
    if not credentials.exists():
        return True
    shutil.rmtree(credentials)
    return not credentials.exists()


def assert_no_secret(root: pathlib.Path) -> None:
    for path in root.rglob("*"):
        if not path.is_file() or ".app" in path.parts:
            continue
        try:
            content = path.read_bytes()
        except OSError:
            continue
        if LOCAL_FIXTURE_KEY.encode("utf-8") in content:
            raise AssertionError(f"fixture secret leaked to evidence: {path}")


def reject_existing_output(output: pathlib.Path) -> None:
    if output.exists() and any(output.iterdir()):
        raise AssertionError(f"拒绝覆盖已有 evidence 目录: {output}")


def self_test() -> int:
    global capture_window, click_session, ax_diagnostic_context
    markers = [f"{MARKER_PREFIX}{index}_selftest" for index in range(1, 4)]
    fragments = [stream_fragment(markers[0], index) for index in range(len(STREAM_BLOCKS) + 1)]
    raw = b"".join([sse_event({"content": fragment}) for fragment in fragments] + [sse_event({}, "stop"), sse_done()])
    payloads = parse_sse_payloads(raw)
    assert len(payloads) == len(STREAM_BLOCKS) + 3
    contents = [json.loads(payload)["choices"][0]["delta"]["content"] for payload in payloads[:-2]]
    assert contents[0] == f"流式开始：{markers[0]}。\n\n"
    assert markers[0] in contents[0]
    assert all(markers[0] not in content for content in contents[1:])
    tail_token = stream_tail_token(markers[0])
    joined_stream = "".join(contents)
    completion_tokens = [stream_completion_token(marker) for marker in markers]
    assert len(set(completion_tokens)) == len(completion_tokens)
    assert completion_tokens[0] not in joined_stream
    final_fragment = (
        stream_fragment(markers[0], len(STREAM_BLOCKS) + 1)
        + "\n\n流式完成令牌："
        + completion_tokens[0]
    )
    assert completion_tokens[0] in final_fragment
    assert all(token not in final_fragment for token in completion_tokens[1:])
    assert tail_token not in contents[0]
    assert all(tail_token in content for content in contents[1:])
    for required in ("中文", "🧪", "列表项", "普通段落", "```go", "func streamChunk", "| 序号 |", "持续输出尾部令牌"):
        assert required in joined_stream, required
    assert 'fmt.Sprintf("chunk-%d", index)' in joined_stream
    assert "{" in joined_stream and "}" in joined_stream
    assert payloads[-1] == "[DONE]"

    # A virtualized viewport may hide the first prompt line. The current-session
    # tail token is the AX assertion; importing another session's token fails.
    virtualized_ax = "当前可见的流尾\n" + tail_token
    viewport = validate_ax_readback(
        virtualized_ax,
        markers[0],
        tail_token,
        markers,
        [stream_tail_token(marker) for marker in markers],
    )
    assert viewport["prompt_marker_visible"] is False
    assert viewport["tail_token_visible"] is True
    try:
        validate_ax_readback(
            virtualized_ax + "\n" + stream_tail_token(markers[1]),
            markers[0],
            tail_token,
            markers,
            [stream_tail_token(marker) for marker in markers],
        )
    except AssertionError as error:
        assert "tail token" in str(error)
    else:
        raise AssertionError("AX 混入其他 session tail token 时没有失败")

    completion_view = markers[0] + "\n" + tail_token
    try:
        validate_ax_completion_readback(
            completion_view,
            markers[0],
            completion_tokens[0],
            markers,
            completion_tokens,
        )
    except AssertionError as error:
        assert "completion token" in str(error)
    else:
        raise AssertionError("旧 tail 可见但 final completion token 缺失时错误返回 PASS")
    completion_view += "\n" + completion_tokens[0]
    completion_result = validate_ax_completion_readback(
        completion_view,
        markers[0],
        completion_tokens[0],
        markers,
        completion_tokens,
    )
    assert completion_result["completion_token_visible"] is True
    assert completion_result["foreign_completion_tokens"] == []

    prompt = {"messages": [{"role": "user", "content": "请处理 " + markers[1]}]}
    assert extract_marker(last_user_text(prompt), markers) == markers[1]
    redacted = redact_record(
        {
            "request_index": 1,
            "session_marker": markers[0],
            "authorization": LOCAL_FIXTURE_KEY,
            "request_body": LOCAL_FIXTURE_KEY,
            "chunk_count": 20,
        }
    )
    encoded_redacted = json.dumps(redacted, ensure_ascii=False)
    assert LOCAL_FIXTURE_KEY not in encoded_redacted
    assert "authorization" not in redacted and "request_body" not in redacted

    run_ids = ["run-a", "run-b", "run-c"]
    snapshot = {
        "sessions": [
            {
                "id": f"s{index}",
                "state": "idle",
                "run_id": "",
                "active_run": None,
                "messages": [
                    {"role": "user", "run_id": run_ids[index - 1], "text": markers[index - 1]},
                    {
                        "role": "assistant",
                        "run_id": run_ids[index - 1],
                        "text": markers[index - 1] + " 中文 🧪\n```go\npass\n```\n| x |",
                    },
                ],
            }
            for index in range(1, 4)
        ]
    }
    ownership = validate_snapshot_ownership(
        snapshot,
        {f"s{index}": markers[index - 1] for index in range(1, 4)},
        require_complete=True,
    )
    assert ownership["assertion"] == "PASS", ownership

    journal = "\n".join(
        json.dumps(
            {
                "sessionId": "s1",
                "sequence": sequence,
                "type": "message",
                "payload": ({"runId": "run-a", "role": role, "content": markers[0]} if role != "run_end" else {"runId": "run-a", "role": role}),
            },
            ensure_ascii=False,
        )
        for sequence, role in enumerate(("user", "assistant", "run_end"), 1)
    )
    journal += "\n" + json.dumps(
        {
            "sessionId": "s1",
            "sequence": 4,
            "type": "session_checkpoint",
            "payload": {"checkpoint": "structurally valid without a run"},
        },
        ensure_ascii=False,
    )
    journal_result = validate_journal(journal, "s1", markers[0], "run-a", markers)
    assert journal_result["assertion"] == "PASS", journal_result
    foreign_run_journal = journal.replace('"runId": "run-a", "role": "assistant"', '"runId": "run-b", "role": "assistant"')
    assert validate_journal(foreign_run_journal, "s1", markers[0], "run-a", markers)["assertion"] == "FAIL"

    records = [
        {
            "request_index": index,
            "session_marker": marker,
            "started_at": "2026-10-12T00:00:00Z",
            "first_chunk_at": "2026-10-12T00:00:00Z",
            "last_chunk_at": "2026-10-12T00:00:02Z",
            "completed_at": "2026-10-12T00:00:02Z",
            "chunk_count": 20,
            "response_bytes": 1000,
            "complete": True,
            "disconnected": False,
            "concurrent_at_start": index > 1,
            "peak_concurrent": 3,
            "duration_seconds": 2.0,
        }
        for index, marker in enumerate(markers, 1)
    ]
    server_result = validate_server_records(records, markers, 2.0, 10.0)
    assert server_result["assertion"] == "PASS", server_result

    class ControlledClock:
        def __init__(self):
            self.value = 0.0

        def now(self) -> float:
            return self.value

        def sleep(self, seconds: float) -> None:
            self.value += seconds

    overdue_clock = ControlledClock()

    def overdue_success() -> bool:
        overdue_clock.value += 3.0
        return True

    try:
        wait_until(
            overdue_success,
            "严格 deadline 成功条件",
            timeout=2.0,
            interval=0.1,
            clock=overdue_clock.now,
            sleeper=overdue_clock.sleep,
        )
    except WaitDeadlineExceeded:
        pass
    else:
        raise AssertionError("条件在外层 deadline 后成功时错误返回 PASS")

    timeout_clock = ControlledClock()

    def overdue_timeout() -> bool:
        timeout_clock.value += 3.0
        raise AxError("osascript 超时")

    try:
        wait_until(
            overdue_timeout,
            "严格 deadline AX timeout",
            timeout=2.0,
            interval=0.1,
            clock=timeout_clock.now,
            sleeper=timeout_clock.sleep,
        )
    except WaitDeadlineExceeded:
        pass
    else:
        raise AssertionError("AX timeout 消耗外层预算后错误返回 PASS")

    class FakeAXApp:
        pid = 12345
        returncode = None

        def poll(self) -> None:
            return None

    original_subprocess_run = subprocess.run
    ax_shape_calls: list[dict[str, Any]] = []
    try:
        def fake_ax_run(args, **kwargs):
            ax_shape_calls.append({"args": args, "kwargs": kwargs})
            return subprocess.CompletedProcess(args, 0, stdout="tail-value\n", stderr="")

        subprocess.run = fake_ax_run
        assert read_ax_last_message_row(FakeAXApp(), timeout=0.375) == "tail-value"
    finally:
        subprocess.run = original_subprocess_run
    assert len(ax_shape_calls) == 1
    assert ax_shape_calls[0]["kwargs"]["timeout"] == 0.375
    assert ax_shape_calls[0]["args"][0] == "osascript"
    assert "last row of table 1 of window 1" in ax_shape_calls[0]["args"][2]

    budget_clock = ControlledClock()
    budget_calls: list[dict[str, Any]] = []
    original_click_session = click_session
    original_ax_context = ax_diagnostic_context
    try:
        click_session = lambda *args: None
        ax_diagnostic_context = lambda *args: {"controller": {}, "fixture": {}}

        def fake_budget_run(args, **kwargs):
            budget_calls.append({"args": args, "kwargs": kwargs})
            if len(budget_calls) == 1:
                # The first query consumes most of the 18-second outer budget.
                budget_clock.value += 15.0
                output = "not-the-tail"
            else:
                output = markers[0] + "\n" + stream_tail_token(markers[0])
            return subprocess.CompletedProcess(args, 0, stdout=output + "\n", stderr="")

        subprocess.run = fake_budget_run
        with tempfile.TemporaryDirectory() as temporary:
            budget_samples: list[dict[str, Any]] = []
            value = read_session_ax(
                FakeAXApp(),
                {"id": "s1"},
                markers[0],
                1,
                pathlib.Path(temporary),
                object(),
                pathlib.Path(temporary),
                budget_samples,
                "self-test-budget",
                markers,
                clock=budget_clock.now,
                sleeper=budget_clock.sleep,
            )
        assert stream_tail_token(markers[0]) in value
        assert len(budget_calls) == 2, budget_calls
        first_timeout = budget_calls[0]["kwargs"]["timeout"]
        second_timeout = budget_calls[1]["kwargs"]["timeout"]
        assert first_timeout == float(MAX_AX_SECONDS), first_timeout
        assert 2.7 < second_timeout < 2.9, second_timeout
        attempts = budget_samples[0]["attempts"]
        assert attempts[-1]["success"] is True
        assert attempts[-1]["own_marker_visible"] is True
        assert attempts[-1]["foreign_markers"] == []
        assert attempts[-1]["foreign_tail_tokens"] == []
        assert "value" not in attempts[-1]
        assert not (pathlib.Path(temporary) / "self-test-budget-ax.txt").exists()
        assert attempts[0]["effective_timeout_ms"] == float(MAX_AX_SECONDS) * 1000
        assert 2700 < attempts[1]["effective_timeout_ms"] < 2900
        assert all("last row of table 1 of window 1" in call["args"][2] for call in budget_calls)
    finally:
        subprocess.run = original_subprocess_run
        click_session = original_click_session
        ax_diagnostic_context = original_ax_context

    completion_clock = ControlledClock()
    completion_calls: list[dict[str, Any]] = []
    try:
        click_session = lambda *args: None
        ax_diagnostic_context = lambda *args: {"controller": {}, "fixture": {}}

        def fake_completion_run(args, **kwargs):
            completion_calls.append({"args": args, "kwargs": kwargs})
            if len(completion_calls) == 1:
                # The old tail is visible, but the final completion token is not.
                completion_clock.value += 15.0
                output = markers[0] + "\n" + stream_tail_token(markers[0])
            else:
                output = markers[0] + "\n" + completion_tokens[0]
            return subprocess.CompletedProcess(args, 0, stdout=output + "\n", stderr="")

        subprocess.run = fake_completion_run
        with tempfile.TemporaryDirectory() as temporary:
            completion_samples: list[dict[str, Any]] = []
            completion_record = read_session_completion_ax(
                FakeAXApp(),
                {"id": "s1"},
                markers[0],
                1,
                pathlib.Path(temporary),
                object(),
                pathlib.Path(temporary),
                completion_samples,
                "self-test-final-completion",
                markers,
                clock=completion_clock.now,
                sleeper=completion_clock.sleep,
            )
        assert completion_record["status"] == "PASS"
        assert completion_record["contains_completion_token"] is True
        assert completion_record["foreign_completion_token_check"] == []
        assert len(completion_calls) == 2, completion_calls
        assert completion_calls[0]["kwargs"]["timeout"] == float(MAX_AX_SECONDS)
        assert 2.7 < completion_calls[1]["kwargs"]["timeout"] < 2.9
        completion_attempts = completion_samples[0]["attempts"]
        assert completion_attempts[0]["completion_visible"] is False
        assert completion_attempts[-1]["own_completion_visible"] is True
        assert completion_attempts[-1]["foreign_completion_tokens"] == []
    finally:
        subprocess.run = original_subprocess_run
        click_session = original_click_session
        ax_diagnostic_context = original_ax_context

    completion_summary = summarize_final_completion([completion_record], 1)
    assert completion_summary["assertion"] == "PASS", completion_summary
    incomplete_summary = summarize_final_completion([completion_record], 3)
    assert incomplete_summary["assertion"] == "FAIL"
    try:
        require_final_completion_pass(incomplete_summary)
    except AssertionError:
        pass
    else:
        raise AssertionError("不完整 final completion 汇总错误地允许成功")
    failed_summary = summarize_final_completion(
        [{"status": "FAIL", "attempts": []}],
        1,
    )
    try:
        require_final_completion_pass(failed_summary)
    except AssertionError:
        pass
    else:
        raise AssertionError("FAIL final completion 汇总错误地允许成功")

    exhausted_clock = ControlledClock()
    exhausted_calls: list[dict[str, Any]] = []
    try:
        click_session = lambda *args: None
        ax_diagnostic_context = lambda *args: {"controller": {}, "fixture": {}}

        def fake_exhausted_run(args, **kwargs):
            exhausted_calls.append({"args": args, "kwargs": kwargs})
            exhausted_clock.value += 18.5
            return subprocess.CompletedProcess(args, 0, stdout="not-the-tail\n", stderr="")

        subprocess.run = fake_exhausted_run
        with tempfile.TemporaryDirectory() as temporary:
            exhausted_samples: list[dict[str, Any]] = []
            try:
                read_session_ax(
                    FakeAXApp(),
                    {"id": "s1"},
                    markers[0],
                    1,
                    pathlib.Path(temporary),
                    object(),
                    pathlib.Path(temporary),
                    exhausted_samples,
                    "self-test-exhausted-budget",
                    markers,
                    clock=exhausted_clock.now,
                    sleeper=exhausted_clock.sleep,
                )
            except WaitDeadlineExceeded:
                pass
            else:
                raise AssertionError("AX 剩余预算耗尽后错误返回 PASS")
        assert len(exhausted_calls) == 1, exhausted_calls
        assert len(exhausted_samples[0]["attempts"]) == 1, exhausted_samples
    finally:
        subprocess.run = original_subprocess_run
        click_session = original_click_session
        ax_diagnostic_context = original_ax_context

    original_capture_window = capture_window
    try:
        def raising_capture(*_args, **_kwargs):
            raise AssertionError("synthetic capture failure")

        capture_window = raising_capture
        with tempfile.TemporaryDirectory() as temporary:
            test_manifest: dict[str, Any] = {"visual_evidence": {}, "diagnostic_errors": []}
            test_summary: dict[str, Any] = {"status": "FAIL", "checks": {}, "error": "PRIMARY"}
            capture_failure_window(
                pathlib.Path("/tmp/helper"),
                FakeAXApp(),
                pathlib.Path(temporary),
                test_manifest,
                test_summary,
                "PRIMARY",
            )
            assert test_summary["error"] == "PRIMARY"
            assert test_summary["checks"]["failure_window_capture"]["status"] == "ERROR"
            assert test_manifest["visual_evidence"]["failure"]["status"] == "ERROR"
            assert test_manifest["diagnostic_errors"][0]["kind"] == "failure_window_capture"
    finally:
        capture_window = original_capture_window

    print(
        json.dumps(
            {
                "status": "PASS",
                "checks": [
                    "sse",
                    "marker",
                    "tail-token",
                    "ax-virtualization",
                    "redaction",
                    "snapshot",
                    "journal",
                    "concurrency",
                    "strict-deadline",
                    "diagnostic-clock",
                    "remaining-query-budget",
                    "ax-attempt-ownership",
                    "failure-capture-primary-error",
                    "completion-token-uniqueness",
                    "final-completion-required",
                    "final-completion-query-budget",
                ],
            },
            ensure_ascii=False,
        )
    )
    return 0


def run_native(options: argparse.Namespace) -> dict[str, Any]:
    if sys.platform != "darwin":
        raise SystemExit("原生 streaming 验收驱动必须在 macOS 上运行")
    source = (ROOT / options.bundle).resolve()
    if not source.is_dir():
        raise SystemExit(f"正式包不存在: {source}")
    output = (ROOT / options.output).resolve()
    reject_existing_output(output)
    output.mkdir(parents=True, exist_ok=True)
    ready_path = output / "ready.json"
    source_exe = source / "Contents/MacOS/GCode"
    original = {
        "executable_sha256": digest(source_exe),
        "code_sha256": code_digest(source_exe),
        "resources": resource_digests(source),
    }
    source_before = source_digests()
    qa_name = "GCodeStreamingQA" + str(time.time_ns())
    data_dir = pathlib.Path.home() / "Library/Application Support" / qa_name
    project = output / ("project-" + qa_name)
    project.mkdir()
    (project / "fixture-readme.txt").write_text("明确标注的 loopback streaming QA project。\n")
    markers = [f"{MARKER_PREFIX}{index}_{qa_name}" for index in range(1, options.sessions + 1)]
    fixture = StreamFixture(markers, options.duration, options.hz)
    server = FixtureHTTPServer(fixture)
    server_thread = threading.Thread(target=server.serve_forever, name="native-streaming-fixture", daemon=True)
    server_started = False
    app: subprocess.Popen[str] | None = None
    bundle: pathlib.Path | None = None
    cgwindow_helper: pathlib.Path | None = None
    sessions: list[dict[str, Any]] = []
    samples: list[dict[str, Any]] = []
    collector: DiagnosticCollector | None = None
    summary: dict[str, Any] = {"status": "FAIL", "checks": {}, "error": None}
    manifest: dict[str, Any] = {
        "status": "RUNNING",
        "qa_identity": qa_name,
        "data_directory": str(data_dir),
        "project": str(project),
        "original_bundle": str(source),
        "driver_sha256": digest(pathlib.Path(__file__).resolve()),
        "source_executable_sha256": original["executable_sha256"],
        "source_code_sha256": original["code_sha256"],
        "source_resources": original["resources"],
        "duration_seconds": options.duration,
        "chunk_hz": options.hz,
        "session_count": options.sessions,
        "session_markers": markers,
        "loopback_only": True,
        "fixture_endpoint": CHAT_PATH,
        "real_api": "NOT TESTED",
        "clipboard": "NOT TOUCHED",
        "physical_input_ime": "NOT TESTED",
        "synthetic_scroll": "cghidEventTap synthetic/native event only; physical wheel/trackpad NOT TESTED",
        "visual_evidence": {},
        "diagnostic_errors": [],
        "measurement": "AX round trips include osascript IPC and wait time; RSS/CPU are process samples without a performance threshold",
        "diagnostic_collector": {
            "sample_file": "diagnostic-samples.json",
            "controller_interval_seconds": 0.2,
            "process_interval_seconds": 1.0,
            "observation_only": True,
        },
        "ready_file": "ready.json",
        "changes": "只复制正式包；隔离副本仅修改 Info.plist identity 并 ad-hoc 签名",
        "source_files_at_start": source_before,
    }
    write_json(output / "manifest.json", manifest)
    result_records: list[dict[str, Any]] = []
    cleanup_errors: list[str] = []
    postcheck_errors: list[str] = []
    primary_error: str | None = None

    def add_failure(message: str) -> None:
        summary["status"] = "FAIL"
        existing = summary.get("error")
        summary["error"] = f"{existing}；{message}" if existing else message
        manifest["status"] = "FAIL"
        manifest["error"] = summary["error"]

    try:
        server_thread.start()
        server_started = True
        bundle = output / (qa_name + ".app")
        if data_dir.exists():
            raise AssertionError("隔离 Application Support 目录已存在")
        shutil.copytree(source, bundle, symlinks=True)
        info_path = bundle / "Contents/Info.plist"
        info = plistlib.loads(info_path.read_bytes())
        info.update(
            CFBundleName=qa_name,
            CFBundleDisplayName=qa_name,
            CFBundleIdentifier="app.gcode.desktop.streamingqa." + qa_name.lower(),
        )
        info_path.write_bytes(plistlib.dumps(info))
        run_command("codesign", "--force", "--deep", "--sign", "-", str(bundle), timeout=60)
        run_command("codesign", "--verify", "--deep", "--strict", str(bundle), timeout=60)
        executable = bundle / "Contents/MacOS/GCode"
        if code_digest(executable) != original["code_sha256"] or resource_digests(bundle) != original["resources"]:
            raise AssertionError("正式包副本的代码或资源与正式包不一致")
        manifest.update(
            {
                "isolated_bundle": str(bundle),
                "isolated_executable_sha256": digest(executable),
                "bundle_code_and_resources_identical": True,
                "loopback_address": f"127.0.0.1:{server.server_address[1]}",
            }
        )
        scroll_executable = compile_scroll_helper(output)
        cgwindow_helper = compile_cgwindow_helper(output)
        env = os.environ.copy()
        env["GCODE_CRED_STORAGE"] = "encrypted-file"
        for key in ("GCODE_VISUAL_SAMPLE", "GCODE_EDITOR", "VISUAL", "EDITOR"):
            env.pop(key, None)
        launch_started = time.perf_counter()
        with open(os.devnull, "w") as null_log:
            app = subprocess.Popen([str(executable)], cwd=ROOT, env=env, stdout=null_log, stderr=null_log, text=True)
        manifest["pid"] = app.pid
        write_ready(ready_path, qa_name, data_dir, app, "app_started")
        wait_until(lambda: exists_ax(app, 'button "设置" of window 1'), "原生正式包窗口没有准备好")
        manifest["launch_ready_ms"] = (time.perf_counter() - launch_started) * 1000
        _provider_id, project_id = configure_provider(app, data_dir, project, f"http://127.0.0.1:{server.server_address[1]}")
        sessions = create_sessions(app, data_dir, project_id, options.sessions)
        marker_by_session = {session["id"]: markers[index] for index, session in enumerate(sessions)}
        session_index_by_id = {str(session["id"]): index for index, session in enumerate(sessions, 1)}
        manifest["session_ids"] = [session["id"] for session in sessions]
        manifest["project_id"] = project_id
        manifest["session_project_ids"] = [session.get("project_id") for session in sessions]

        collector = DiagnosticCollector(app, data_dir, fixture, sessions)
        collector.start()
        write_ready(ready_path, qa_name, data_dir, app, "ready_before_send")
        send_started = time.perf_counter()
        for index, session in enumerate(sessions):
            marker = markers[index]
            prompt = (
                f"持续输出原生流式验收内容，必须保留 marker {marker}，包含中文、emoji 🧪、列表、段落、"
                "fenced code 和 Markdown table；请持续输出直到服务端结束。"
            )
            clicked_at = time.perf_counter()
            session_id = str(session["id"])
            click_session(app, session, session_index_by_id[session_id])
            wait_empty_session_ax(app, markers[:index], session_id)
            send_prompt(app, prompt)
            wait_session_marker(data_dir, sessions, session_id, marker)
            samples.append(
                {
                    "kind": "send",
                    "session_marker": marker,
                    "elapsed_from_send_start_ms": (time.perf_counter() - send_started) * 1000,
                    "select_and_send_ms": (time.perf_counter() - clicked_at) * 1000,
                }
            )

        initial_deadline = time.monotonic() + max(20.0, options.duration + 15.0)
        ax_done = False
        ready_ax_marked = False
        shared_running_verified = False
        while time.monotonic() < initial_deadline:
            snapshot = read_snapshot(data_dir)
            records, fixture_errors, peak = fixture.snapshot()
            if fixture_errors:
                raise AssertionError("loopback fixture error: " + "; ".join(fixture_errors))
            if len(records) == options.sessions:
                # All sessions must be observed running together once. Once that
                # evidence exists, a normal early completion in one session must
                # not invalidate ownership checks for the remaining streams.
                running_ownership = validate_snapshot_ownership(snapshot, marker_by_session, require_running=True)
                if running_ownership["assertion"] == "PASS" and not shared_running_verified:
                    shared_running_verified = True
                    summary["checks"]["shared_running"] = running_ownership
                ownership = validate_snapshot_ownership(snapshot, marker_by_session, require_running=False)
                if ownership["assertion"] != "PASS":
                    raise AssertionError("持续流式期间 session/run 归属失败: " + "; ".join(ownership["failures"]))
                if shared_running_verified and "streaming" not in manifest["visual_evidence"]:
                    manifest["visual_evidence"]["streaming"] = capture_window(
                        cgwindow_helper,
                        app,
                        output,
                        "streaming",
                        {
                            "session_states": ownership,
                            "fixture_request_count": len(records),
                            "fixture_peak_concurrent": peak,
                        },
                    )
                if shared_running_verified and not ax_done:
                    if not ready_ax_marked:
                        write_ready(
                            ready_path,
                            qa_name,
                            data_dir,
                            app,
                            "ax_readback",
                            sample="session-1-streaming",
                            sample_started_at=utc_now(),
                        )
                        ready_ax_marked = True
                    for index, session in enumerate(sessions):
                        marker = markers[index]
                        value = read_session_ax(
                            app,
                            session,
                            marker,
                            session_index_by_id[str(session["id"])],
                            data_dir,
                            fixture,
                            output,
                            samples,
                            f"session-{index + 1}-streaming",
                            markers,
                        )
                        for other in markers:
                            if other != marker and other in value:
                                raise AssertionError("AX 切换后读到了其他 session marker")
                    read_session_ax(
                        app,
                        sessions[0],
                        markers[0],
                        session_index_by_id[str(sessions[0]["id"])],
                        data_dir,
                        fixture,
                        output,
                        samples,
                        "session-1-before-scroll",
                        markers,
                    )
                    geometry = [
                        int(value.strip())
                        for value in ax(app, "return (position of table 1 of window 1) & (size of table 1 of window 1)").split(",")
                    ]
                    if len(geometry) != 4 or geometry[2] <= 0 or geometry[3] <= 0:
                        raise AssertionError("无法取得原生对话 table frame")
                    pointer = json.loads(
                        run_command(
                            str(scroll_executable),
                            str(app.pid),
                            str(geometry[0] + geometry[2] // 2),
                            str(geometry[1] + geometry[3] // 2),
                            "700",
                            timeout=20,
                        )
                    )
                    if not pointer.get("restored"):
                        raise AssertionError("合成滚轮 helper 未恢复指针位置")
                    manifest["synthetic_scroll"] = {
                        "status": "INJECTED",
                        "delivery": "cghidEventTap after native cursor move; cursor restored",
                        "table_frame": geometry,
                        "pointer": pointer,
                        "physical_wheel_trackpad": "NOT_TESTED",
                    }
                    ax_done = True
                if fixture.all_complete():
                    break
            time.sleep(0.15)
        if len(fixture.snapshot()[0]) != options.sessions:
            raise AssertionError("三条 SSE 请求没有全部建立")
        if not shared_running_verified:
            raise AssertionError("没有观察到三个 session 同时处于 running/active_run")
        if not fixture.all_complete():
            wait_until(fixture.all_complete, "SSE 流没有按时结束", timeout=max(20.0, options.duration + 20.0), interval=0.2)

        final_snapshot = wait_snapshot(
            data_dir,
            lambda snapshot: all(
                session_by_id(snapshot, session["id"]).get("state") == "idle"
                and session_by_id(snapshot, session["id"]).get("active_run") is None
                for session in sessions
            ),
            "三会话没有正常回到 idle",
            timeout=max(25.0, options.duration + 25.0),
        )
        final_ownership = validate_snapshot_ownership(
            final_snapshot,
            marker_by_session,
            require_complete=True,
        )
        summary["checks"]["snapshot_ownership"] = final_ownership
        if final_ownership["assertion"] != "PASS":
            raise AssertionError("最终 snapshot 归属断言失败: " + "; ".join(final_ownership["failures"]))
        final_completion_records: list[dict[str, Any]] = []
        try:
            for index, session in enumerate(sessions):
                final_completion_records.append(
                    read_session_completion_ax(
                        app,
                        session,
                        markers[index],
                        session_index_by_id[str(session["id"])],
                        data_dir,
                        fixture,
                        output,
                        samples,
                        f"session-{index + 1}-final-completion",
                        markers,
                    )
                )
        except Exception:
            final_completion_records = [
                sample for sample in samples if sample.get("kind") == "final_completion"
            ]
            summary["checks"]["final_completion"] = summarize_final_completion(
                final_completion_records,
                len(sessions),
            )
            raise
        summary["checks"]["final_completion"] = summarize_final_completion(
            final_completion_records,
            len(sessions),
        )
        require_final_completion_pass(summary["checks"]["final_completion"])
        manifest["visual_evidence"]["final"] = capture_window(
            cgwindow_helper,
            app,
            output,
            "final",
            {"session_states": final_ownership},
        )
        summary["checks"]["visual_evidence"] = manifest["visual_evidence"]
        raw_controller = data_dir / "controller.json"
        if raw_controller.exists():
            shutil.copyfile(raw_controller, output / "controller-final.json")
        journal_results: list[dict[str, Any]] = []
        evidence_root = output / "sessions"
        for index, session in enumerate(sessions):
            session_id = session["id"]
            session_final = session_by_id(final_snapshot, session_id)
            run_id = str(next((message.get("run_id") for message in session_messages(session_final, "user") if message.get("run_id")), ""))
            history_path = data_dir / "sessions" / session_id / "events.jsonl"
            history = history_path.read_text() if history_path.exists() else ""
            result = validate_journal(history, session_id, marker_by_session[session_id], run_id, markers)
            journal_results.append(result)
            target = evidence_root / f"session-{index + 1}-{session_id}"
            target.mkdir(parents=True, exist_ok=True)
            if history_path.exists():
                shutil.copyfile(history_path, target / "events.jsonl")
            write_json(target / "final-session.json", session_final)
            if result["assertion"] != "PASS":
                raise AssertionError(f"session {session_id} journal 归属断言失败: {'; '.join(result['failures'])}")
        summary["checks"]["journals"] = journal_results
        records, fixture_errors, _peak = fixture.snapshot()
        server_result = validate_server_records(records, markers, options.duration, options.hz)
        summary["checks"]["server_streams"] = server_result
        if fixture_errors:
            raise AssertionError("loopback fixture error: " + "; ".join(fixture_errors))
        if server_result["assertion"] != "PASS":
            raise AssertionError("SSE 并发/持续性断言失败: " + "; ".join(server_result["failures"]))
        result_records = server_result["records"]
        write_json(output / "provider-records.json", result_records)
        write_json(output / "stream-chunks.json", fixture.chunk_snapshot())
        write_json(output / "measurement-samples.json", samples)
        if app is None or not quit_app(app):
            raise AssertionError("应用没有通过菜单正常退出")
        manifest["exit_code"] = app.returncode
        summary["checks"]["normal_exit"] = app.returncode == 0 and app.poll() is not None
        if not summary["checks"]["normal_exit"]:
            raise AssertionError("应用退出码不是 0")
        manifest["status"] = "PASS"
        summary["status"] = "PASS"
    except Exception as error:
        primary_error = type(error).__name__ + ": " + str(error)
        summary["status"] = "FAIL"
        summary["error"] = primary_error
        manifest["status"] = "FAIL"
        manifest["error"] = summary["error"]
    finally:
        capture_failure_window(
            cgwindow_helper,
            app,
            output,
            manifest,
            summary,
            primary_error,
        )
        diagnostic_snapshot: dict[str, Any] = {
            "status": "NOT_STARTED",
            "observation_only": True,
            "joined": True,
            "sample_count": 0,
            "controller_poll_count": 0,
            "process_sample_count": 0,
            "error_count": 0,
            "errors": [],
            "samples": [],
        }
        if collector is not None:
            try:
                if not collector.stop_join():
                    cleanup_errors.append("diagnostic collector 未能 stop/join")
                diagnostic_snapshot = collector.snapshot()
            except Exception as error:
                cleanup_errors.append(f"diagnostic collector 清理异常: {type(error).__name__}: {error}")
                diagnostic_snapshot["status"] = "ERRORS_RECORDED"
        if app is not None and app.poll() is None:
            try:
                if not quit_app(app):
                    cleanup_errors.append("QA 应用未通过正常退出流程")
            except Exception as error:
                cleanup_errors.append(f"QA 应用退出清理异常: {type(error).__name__}: {error}")
        manifest["exit_code"] = app.returncode if app is not None and app.poll() is not None else None
        fixture.stop()
        if server_started and server_thread.is_alive():
            try:
                server.shutdown()
            except Exception as error:
                cleanup_errors.append(f"fixture server shutdown 异常: {type(error).__name__}: {error}")
        try:
            server.server_close()
        except Exception as error:
            cleanup_errors.append(f"fixture server close 异常: {type(error).__name__}: {error}")
        if server_started:
            try:
                server_thread.join(timeout=8)
            except RuntimeError as error:
                cleanup_errors.append(f"fixture thread join 异常: {error}")
        manifest["server_thread_stopped"] = not server_thread.is_alive()
        try:
            manifest["fixture_credentials_removed"] = remove_credentials(data_dir)
        except OSError as error:
            manifest["fixture_credentials_removed"] = False
            cleanup_errors.append(f"隔离凭据清理异常: {error}")
        manifest["qa_process_exited"] = app is None or app.poll() is not None
        manifest["primary_error"] = primary_error
        try:
            if (data_dir / "controller.json").exists() and not (output / "controller-final.json").exists():
                shutil.copyfile(data_dir / "controller.json", output / "controller-last.json")
        except OSError as error:
            cleanup_errors.append(f"controller 失败证据复制异常: {error}")
        for index, session in enumerate(sessions):
            session_id = session.get("id")
            if not session_id:
                continue
            history_path = data_dir / "sessions" / str(session_id) / "events.jsonl"
            if not history_path.exists():
                continue
            try:
                target = output / "sessions" / f"session-{index + 1}-{session_id}"
                target.mkdir(parents=True, exist_ok=True)
                if not (target / "events.jsonl").exists():
                    shutil.copyfile(history_path, target / "events.jsonl")
            except OSError as error:
                cleanup_errors.append(f"session {session_id} 失败证据复制异常: {error}")
        try:
            manifest["source_files_unchanged_after_run"] = source_digests() == source_before
        except (OSError, AssertionError) as error:
            manifest["source_files_unchanged_after_run"] = False
            postcheck_errors.append(f"源码 hash 清理核对异常: {error}")
        try:
            manifest["original_bundle_unchanged_after_run"] = (
                digest(source_exe) == original["executable_sha256"]
                and code_digest(source_exe) == original["code_sha256"]
                and resource_digests(source) == original["resources"]
            )
        except (OSError, AssertionError) as error:
            manifest["original_bundle_unchanged_after_run"] = False
            postcheck_errors.append(f"正式包 hash 清理核对异常: {error}")
        final_records, fixture_errors, peak = fixture.snapshot()
        if not result_records:
            result_records = [redact_record(record) for record in final_records]
        summary["provider_records"] = result_records
        summary["provider_errors"] = fixture_errors
        summary["peak_concurrent"] = peak
        summary["checks"]["diagnostic_collector"] = {
            "status": diagnostic_snapshot.get("status"),
            "observation_only": True,
            "joined": diagnostic_snapshot.get("joined"),
            "sample_count": diagnostic_snapshot.get("sample_count"),
            "controller_poll_count": diagnostic_snapshot.get("controller_poll_count"),
            "process_sample_count": diagnostic_snapshot.get("process_sample_count"),
            "error_count": diagnostic_snapshot.get("error_count"),
            "errors": diagnostic_snapshot.get("errors", []),
        }
        manifest["diagnostic_collector"].update(
            {
                "status": diagnostic_snapshot.get("status"),
                "joined": diagnostic_snapshot.get("joined"),
                "sample_count": diagnostic_snapshot.get("sample_count"),
                "controller_poll_count": diagnostic_snapshot.get("controller_poll_count"),
                "process_sample_count": diagnostic_snapshot.get("process_sample_count"),
                "error_count": diagnostic_snapshot.get("error_count"),
                "errors": diagnostic_snapshot.get("errors", []),
            }
        )
        try:
            write_ready(
                ready_path,
                qa_name,
                data_dir,
                app,
                "stopped",
                fixture_stopped=manifest.get("server_thread_stopped", False),
                credentials_removed=manifest.get("fixture_credentials_removed", False),
                collector_joined=diagnostic_snapshot.get("joined", False),
                cleanup_errors=list(cleanup_errors),
            )
        except Exception as error:
            cleanup_errors.append(f"ready 状态写入异常: {type(error).__name__}: {error}")
        manifest["cleanup_errors"] = cleanup_errors
        manifest["postcheck_errors"] = postcheck_errors
        summary["checks"]["cleanup"] = {
            "server_thread_stopped": manifest["server_thread_stopped"],
            "fixture_credentials_removed": manifest["fixture_credentials_removed"],
            "qa_process_exited": manifest["qa_process_exited"],
            "errors": cleanup_errors,
        }
        if (
            not manifest["server_thread_stopped"]
            or not manifest["fixture_credentials_removed"]
            or not manifest["qa_process_exited"]
            or cleanup_errors
        ):
            add_failure("清理失败: " + "; ".join(cleanup_errors or ["fixture server/thread、QA PID 或隔离凭据状态不满足"]))
        if postcheck_errors:
            add_failure("后置核对失败: " + "; ".join(postcheck_errors))
        write_json(output / "provider-records.json", result_records)
        write_json(output / "stream-chunks.json", fixture.chunk_snapshot())
        write_json(output / "measurement-samples.json", samples)
        write_json(output / "diagnostic-samples.json", diagnostic_snapshot)
        if manifest["source_files_unchanged_after_run"] is False or manifest["original_bundle_unchanged_after_run"] is False:
            add_failure("源码或原始正式包在验收期间发生变化")
        try:
            assert_no_secret(output)
            manifest["evidence_secret_scan"] = "PASS"
        except AssertionError as error:
            add_failure(str(error))
            manifest["evidence_secret_scan"] = "FAIL"
        write_json(output / "manifest.json", manifest)
        write_json(output / "results.json", summary)
    return summary


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bundle", default=DEFAULT_BUNDLE)
    parser.add_argument("--output", default=DEFAULT_OUTPUT)
    parser.add_argument("--duration", type=float, default=DEFAULT_DURATION)
    parser.add_argument("--hz", type=float, default=DEFAULT_HZ)
    parser.add_argument("--sessions", type=int, default=DEFAULT_SESSIONS)
    parser.add_argument("--self-test", action="store_true", help="只运行纯函数自测，不打开窗口、不绑定 HTTP socket")
    options = parser.parse_args()
    if options.self_test:
        return self_test()
    if options.duration <= 0 or options.hz <= 0 or options.sessions <= 0:
        raise SystemExit("--duration、--hz、--sessions 必须为正数")
    summary = run_native(options)
    print(json.dumps(summary, ensure_ascii=False, indent=2))
    return 0 if summary.get("status") == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
