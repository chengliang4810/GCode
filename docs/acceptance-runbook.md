# Desktop acceptance runbook

Reference: Zeron 0.2.128, commit
`ab189159f10cf8e63c0502be75a59bbc487e0eaf`. Replica package version: 0.1.0.
Each result is recorded separately as compiled, offscreen-tested, native-tested
and visually accepted. The current report is not a visual acceptance pass.

## Reproduce the build and source checks

From this independent checkout, using Go 1.27.3:

```powershell
./scripts/reference-verify.ps1 -SourceRoot D:\zeron
go run ./scripts/check-framework.go
go test ./...
go vet ./...
go tool mygo vet .
./scripts/lint.ps1
./scripts/build.ps1 -Targets windows/amd64,darwin/amd64,darwin/arm64,darwin/universal
```

Use `scripts/lint.ps1 -ToolPath <absolute path>` for an already built
golangci-lint 2.15.0 when the module proxy is unavailable. A Windows race run
requires a compatible C compiler. `scripts/race.ps1 -Compiler <gcc.exe>` sets
CGO/CC/PATH temporarily and restores them. The four principal packages passed
with portable w64devkit2.10.1/GCC16.2.0; evidence is in reference/validation.

## Fixed scenes and implementation captures

```powershell
go run ./cmd/zeron --scenario showcase --appearance dark --dump-scene reference/fixtures/showcase.json
./scripts/capture-implementation.ps1 -AllThemes -BoundarySizes
./build/windows-amd64/Zeron.exe --scenario questions --route chat/chat-0 --appearance light
./build/windows-amd64/Zeron.exe --scenario transcript --route chat/chat-0 --appearance light
./build/windows-amd64/Zeron.exe --scenario live --route chat/chat-0 --appearance dark
```

The fixture includes the source commit and the full versioned Go contract.
`output/captures/matrix/evidence.json` records dimensions, scale, theme, route
and each image hash. It classifies them as implementation-only captures.
The unmodified GPUI fixture build and executable hash are recorded separately
in `reference/native-build.json`.

Native goldens still require the original and replica to receive equivalent
scene data and UI preferences. The stock sidebar fixture uses 1100×800 and a
310px sidebar; those defaults cannot silently serve as the 1320×880 / 256px
acceptance case. Historical source screenshots also cannot serve as new
Windows goldens.

## Windows native interaction pass

Launch the packaged executable from its extracted directory, keeping the
adjacent native terminal library. Record Windows build, GPU, CPU, display,
client size, DPI, appearance, scene, locale and input method.

1. Create a session, choose a model and effort, type Chinese composition,
   select a candidate with Enter, paste multiline text and an image, drag files,
   send, stop, retry, queue, edit and remove queued messages.
2. Exercise questions and permissions, then switch sessions during output and
   during an attachment import. Check draft ownership, unread state, archive,
   restore, pin, group collapse, search and keyboard focus.
3. Open files, edit/save/close, trigger read-only and version-conflict states,
   rename/delete/create, drag a file into the composer, and check Changes.
4. Open diff/history/PR, inspect checks and comments, add a comment, and follow
   the link back to its session. Record inline review states still missing.
5. Open/close terminal tabs, type, resize, select/copy, search and scroll ANSI
   output. Its connection is in-process; it must not create a real shell.
6. Navigate public websites in the browser, check back/forward/reload/failure,
   select an annotation, hover/remove its chip and send it. Open menus and
   dialogs over the page and verify stacking, focus and no click-through.
7. Change font/size/theme/material, import a theme, choose/adjust a background,
   change shortcuts and restart with normal persistence. Exercise all thirty
   themes and the 900×600 / 1320×880 / 1600×1000 boundaries at every listed DPI.
8. Repeat pane/session open/close thirty times; record goroutine/subscription,
   terminal and native-window counts. Measure input/drag/scroll/startup targets
   on the recorded hardware. Do not substitute unit timing for OS latency.

Use actual OS window screenshots for compositor materials and the native web
page. `Window.CapturePage` omits that independently composed page layer and
cannot establish its displayed pixels. Do not turn that omission into a mask
or a visual pass.

## Paired image review

Supply independently captured, equal-sized reference and implementation PNGs
to `go run ./cmd/parity --help`, using its documented flags. Save the original
pair, side-by-side, overlay, difference and metrics. Review the full window,
then text baselines/wrapping, icons, inputs, menus and materials. The SSIM
thresholds are screening gates; visible differences still fail.

## macOS checks

Copy each architecture ZIP to its corresponding Mac and extract while retaining
executable permissions. Run the same scenes and native pass, adding traffic
lights, application menus, Cmd shortcuts, Retina, IME, vibrancy, WKWebView and
Appshots. Record Intel and ARM runtime results separately. Universal binary
verification does not replace either real-machine result. Signing, notarization
and DMG production are performed in the macOS distribution environment.
