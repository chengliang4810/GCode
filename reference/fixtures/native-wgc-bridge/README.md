# Fixture-only in-process WGC bridge

This standalone Rust `cdylib` is used only by the Windows `uifixture` command.
It does not link the product, MyGo, GPUI or a renderer. The Go adapter uses the
standard Windows loader and needs no CGO or production Go dependency.

```powershell
./scripts/build-native-wgc-bridge.ps1
go build -tags uifixture -o output/model-picker-fixture.exe ./cmd/model-picker-fixture
# Native runs must be serialized with other fixture/evidence windows.
./output/model-picker-fixture.exe --out-dir output/my-fresh-wgc-run --capture-mode wgc --capture-bridge D:/projects/GCode/output/native-wgc-bridge/zeron_fixture_wgc.dll
```

`captureNativeWindow(ctx, hwnd, bridgePath, outputPath)` is the Go adapter
contract. The main fixture reads its own HWND in a UI callback, then invokes
the function in its existing background worker. `bridgePath` and `outputPath`
are absolute paths. The DLL exports ABI version 1, defined in `bridge.h`.
Its length-delimited UTF-8 path is copied into Rust-owned data. It rejects
NUL, invalid UTF-8, oversized buffers, existing PNG/metadata paths and any HWND
whose owner is not `GetCurrentProcessId()`. It never enumerates windows or
displays. The Go byte buffers stay alive until the synchronous call returns.
New-file checks inspect directory entries without following dangling links,
and every output ancestor must be a plain directory without a reparse point.

The locked `windows-capture =1.5.0` library owns its MTA/DispatcherQueue thread.
The Go caller does not initialize COM, manipulate a frame or receive any WinRT
object. Capture settings use `WithoutCursor` and system defaults for the
border, secondary windows, dirty regions and update interval. No permission
request, border suppression, privacy override or system-setting change is
performed. If Windows requires authorization, leave the run pending.

The complete BGRA8 WGC frame is saved directly by
`Frame::save_as_image(ImageFormat::Png)`, without crop, resize, masking or a
JPEG intermediate. `outputPath + ".capture.json"` records raw frame and PNG
sizes, signature/size verification, SHA-256, HWND/PID, complete window/client
geometry, client screen origin, DPI/scale and capture policies. The Go fixture
reads the same PNG bytes and performs its own PNG/hash validation. A complete
WGC window frame includes nonclient pixels and may omit invisible Windows
resize margins; it is not the same boundary as `Window.CapturePage()`.

Success is returned only after explicit WGC stop and thread join. A guard also
stops during bridge-owned unwinding. Both the C entry and our frame callback
contain bridge panics with `catch_unwind`; profiles explicitly use
`panic="unwind"`. This does not claim to repair upstream panic paths: the
locked library itself contains `expect` calls before its user callback and
the WinRT ABI does not catch all upstream panics.

The maximum 15-second setting bounds the first-frame wait only. Upstream
initialization and shutdown/join have no hard deadline. Context cancellation
is checked before and after the native call and shortens the frame wait when
a deadline exists; it never terminates a thread or unloads the DLL. The DLL
remains loaded for the fixture process lifetime because WinRT registrations,
callbacks and encoder workers may otherwise reference unloaded code.

The build script uses an isolated lock and target under this checkout, rejects
reparse write paths, runs non-capturing contract tests, and records source,
lock, toolchain and DLL hashes in `reference/validation/native-wgc-bridge.json`.
It does not run a native fixture or capture any window. Compilation and the
contract tests establish transport readiness, not visual or keyboard parity.

## Native title-bar comparison limits

The 2026-10-28 R3 full-window comparison has matching WGC/client dimensions
and matching large client background areas, while the native title bars differ
in tint. This is an unresolved comparison finding. It does not establish a
product theme defect or equal activation state. Whole-window comparison scores
are screening results; no scene has passed visual acceptance.

The frozen GPUI dependency is `b564a34413bb7ef1da04121c4a287758f94993d9`.
These are source configuration facts, rather than runtime attribute readings:

| Area | Frozen GPUI | MyGo `zeron-native-ui.3` |
| --- | --- | --- |
| Background policy | `crates/gpui/src/window.rs:1447` always applies the requested background; the default is Opaque (`platform.rs:2097–2114,2200–2210`). `gpui_windows/src/window.rs:876–880,1590–1637` applies `SetWindowCompositionAttribute` with `WCA_ACCENT_POLICY=0x13`, accent state `0`, flags `2`, gradient `0`. | `third_party/mygo/internal/windows/window.go:210–213` applies the window theme, but calls `SetVibrancy` only for a nonempty option. No `WCA_ACCENT_POLICY` call exists in this backend. |
| DWM backdrop type | DWM attribute 38 is written only by the explicit Mica/MicaAlt branches (`gpui_windows/src/window.rs:886–893,1561–1588`). The Opaque branch uses the accent-policy API instead. | `window.go:1043–1070` maps an explicit `SetVibrancy("")` call to `DWMSBT_NONE=1`; constructor absence skips that call, so absent Vibrancy is not evidence of an explicit attribute-38 NONE setting. |
| Redirection | With `GPUI_DISABLE_DIRECT_COMPOSITION` unset, `gpui_windows/src/platform.rs:172–173` selects DirectComposition and `window.rs:487–489` adds `WS_EX_NOREDIRECTIONBITMAP`. | With absent Vibrancy, `window.go:146` leaves `noRedirect=false`; `window.go:238–263` starts from `WS_OVERLAPPEDWINDOW|WS_CLIPCHILDREN` and adds the no-redirection flag only when requested. |
| OS light/dark mode | `gpui_windows/src/window.rs:499,547` obtains system appearance and configures DWM dark mode; `util.rs:134–149,160–170` derives it from `UISettings` foreground color and writes attribute 20. | `third_party/mygo/internal/windows/system.go:461–487` derives ThemeSystem from `AppsUseLightTheme` and writes attribute 20. It does not specify a caption color. |
| Activation | The original fixture calls application `cx.activate(true)`, whose Windows platform implementation is empty (`gpui_windows/src/platform.rs:481`). The separate window activation function (`window.rs:773–822`) requests active/focus/foreground, which is not itself a captured observation. | `window.go:798–809` calls `ShowWindow(SW_SHOW)` and `SetForegroundWindow`, without checking their return values. `IsFocused` (`window.go:852–857`) can read the actual foreground equality, but is not recorded in these captures. |

The current reference and bridge metadata contain HWND/PID, client/window
geometry, DPI/scale and pixels. They do **not** contain foreground HWND,
activation state, actual style/ex-style bits, `DWMWA_SYSTEMBACKDROP_TYPE`,
immersive dark mode or caption-color attribute values. Therefore neither
matching geometry nor calls to `Show`/`activate` prove equal native frame state.
The source differences above are possible explanations, not a measured cause
of the tint. A future focused investigation can add read-only diagnostic
metadata and recapture both fixtures. This record does not change the frozen
DLL, framework, renderer, product theme, or any Windows setting.
