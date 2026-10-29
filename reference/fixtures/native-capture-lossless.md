# Frozen Windows fixture PNG transport

`scripts/reference-capture-lossless.ps1` generates a separate
`compact-picker-windows-lossless` example in the existing reference export.
The fixed read-only clone, production GPUI renderer/components, JPEG adapter,
JPEG provenance and its eight images are not edited.

```powershell
./scripts/reference-capture-lossless.ps1 -SourceRoot output/reference-sources/zeron
./scripts/reference-capture-lossless.ps1 -SourceRoot output/reference-sources/zeron -Run -Surface opaque
```

The second command opens the fixture and captures its eight original matrix
scenes. Run it serially with other native UI evidence tools: window activation
can change popup/focus states. A supplied `OutputDirectory` must be a new path
inside this checkout, outside the read-only clone, with no reparse ancestors.
The default output is a fresh `output/reference-lossless/capture-*` directory.
The same write-path checks cover generated examples, manifests, snapshots,
logs, metadata and the Cargo target; a SourceRoot alias is resolved before
checking the read-only boundary.

The adapter uses the existing locked `windows-capture =1.5.0` library directly,
inside the fixture process. It obtains the exact GPUI HWND, verifies that its
owner is the current process, and requests no other window or display. The
library owns its MTA/dispatcher thread. The first complete BGRA8 WGC frame is
saved with `Frame::save_as_image(ImageFormat::Png)`. It is not resized, cropped
or converted from JPEG. Native row-padding removal is part of the library's
PNG save operation and does not remove image pixels.

Cursor capture is disabled; capture-border, secondary-window, update-interval
and dirty-region policies retain their system defaults. The code invokes no
permission dialog, access request, border-suppression option or system-setting
change. Unsupported/denied capture remains pending with an error record. If
Windows nevertheless presents an authorization surface, leave it pending;
do not operate privacy settings for this test.

Each `*.png.capture.json` records HWND/PID, complete window bounds, client bounds
and screen origin, GPUI logical bounds/viewport, scale, raw WGC dimensions,
PNG dimensions, byte count and SHA-256. It also reads the existing frozen
`fixture_compact_state` getter to record actual appearance, panel/list page,
effective/explicit reasoning, Fast, and keyboard-focused model/favorite state.
Selected-model/context defaults still depend on the frozen catalog; this
metadata does not claim that every private picker field was exported.
The PNG is the complete HWND capture,
including the native/nonclient extent selected by WGC. It is **not** equivalent
to the implementation's client-only `CapturePage` output; these images alone
do not establish a pixel-comparison result.

The default route exits after the frozen panel/list layout matrix and records
`layout-captured`. Enter/Escape scene navigation and the original model-list
state assertion remain. Full keyboard acceptance and visual parity remain
pending. `-IncludeKeyboardAssertions` retains the original frozen keyboard
sequence and assertions, including its known route mismatch; it does not
correct them or manufacture a pass. A zero process exit code alone is not
success: the PowerShell wrapper requires the declared result record, capture
count and every PNG hash.

`output/reference-lossless/transport-source/before` preserves the previous
dynamic-voice/JPEG build's root manifest, lock, UI manifest and voice manifest.
`after` holds this transport's stage. The dependency patch adds only a Windows
development dependency and the already-locked package to `zeron-ui`'s lock
entry; package versions do not change. Separate metadata is written to
`reference/native/lossless-capture-adapter.json`, with the template/generated
adapter hashes, before/after hashes, patch hash, build command, tool versions,
binary hash and runtime status. Historical JPEG build hashes keep their
original build-stage meaning.

The 2026-10-28 run saved all eight original PNG scenes under
`output/reference-lossless/capture-layout-20261028-02`. Each signature, IHDR
dimension, byte count and SHA-256 was independently checked against its capture
record. At scale 1 the complete WGC frames are 842×592 (wide) and 362×592
(narrow); their clients are 840×560 and 360×560. `GetWindowRect` also includes
invisible resize margins, yielding 856×599 and 376×599; the library's original
WGC frames omit those margins without any file transformation. Results,
per-image hashes and fresh CRLF/mixed-ending dependency replay checks are in
`reference/validation/lossless-result.json`. The failed first runtime remains
separately recorded. Keyboard and pixel-parity acceptance remain pending.

The subsequent state-metadata run is
`output/reference-lossless/capture-native-state-20261028-01`. All eight PNGs
are byte-identical to the prior successful set, as verified in
`output/reference-lossless/typed-state-image-equivalence.json`; the metadata
addition did not alter the production view. The original window frame remains
in use. Two frame-only trials and their rollback are recorded separately in
`reference/validation/frame-boundary-trials.json`.

The implementation's optional in-process WGC bridge now supplies an equivalent
full-window boundary. `scripts/compare-compact-fixtures.ps1` validates the
original PNGs, typed scene state and same client/frame geometry before invoking
the existing parity command. It saves complete-image diff/overlay/pairs, a
script snapshot and hashes into a fresh output directory. Exit 1 with a report
means the pixel threshold was not met; an exception means preflight or tool
execution failed. No input is modified or transformed. The comparison does
not verify active-window/DWM state, keyboard, timing, accessibility or other
platforms. The current report is linked from
`reference/validation/compact-fixture-comparison.json`.
