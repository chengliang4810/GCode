# Pure Go PTY

Copied from `third_party/mygo/internal/pty` when the framework was pinned at
MyGo `v0.6.4+zeron-native-ui.9`. The current pinned framework is MyGo v0.7.3,
which no longer ships `internal/pty`; this copy remains app-owned. The MIT
license is retained in `LICENSE`. The original files remain untouched.

This package keeps the shared terminal host free of MyGo's window and UI
dependencies, including when compiled into `cmd/gcode-server`.

Local changes: attribution headers, removal of the unused `waitOnce` field,
and synchronized Windows process handle cleanup and termination.
