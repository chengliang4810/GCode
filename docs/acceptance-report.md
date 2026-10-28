# Acceptance report — 2026-10-28

Overall result: **in progress / not visually accepted**.

Reference: Zeron `ab189159f10cf8e63c0502be75a59bbc487e0eaf`.
Build target: Go1.27.3 / MyGo0.6.4 + pinned `zeron-native-ui.2`, Windows11 amd64;
macOS13+ amd64/arm64. The reference is Zeron 0.2.128; the independent replica
package version is 0.1.0.
This report distinguishes implemented behavior, automated checks, native
observations and comparison with the reference.

## Available implementation

- Native application shell, titlebar, sidebar, session/project/model selection,
  composer, virtualized transcript, streaming, tool/thinking disclosures,
  questions, permissions, todos, subagent cards, attachments and queue actions.
- Typed in-process service, deterministic scenes, isolated persisted state,
  cancellable/coalescing events, file version conflicts and scripted terminal IO.
- Native settings pages, theme/font/palette controls, original fonts/icons and
  all30 source theme variants. Voice controls are excluded.
- File tree, editable buffers, dirty-close/conflict handling, Markdown/image
  preview, changes/diff, commit history, PR overview/checks/code/comments.
- System web engine confined to external document content, native browser
  chrome, navigation/loading/error state and overlay support.
- MyGo terminal over an in-memory Conn, not a shell. Native library packaged
  for each architecture.
-210 math/diagram SVG resources produced by the original renderer code.
- Source compact model panel, effort slider, model search and persisted favorites.
- Native file selection, external file drop and clipboard image/file ingestion,
  with backend-managed resources and draft/session isolation.
- Source background cropping, pan/zoom adjustment, five source-space image
  effects, and native GPU source-alpha feathering. Raster samples exported
  independently from Rust match the Go effects byte for byte.
- Five reviewed patches supply per-window motion/scroll preference,
  hover/drop callbacks, exact Markdown line heights and independent package
  installation names. All 1,830 framework files and five patches are hash-locked.

Implementation does not imply complete reference parity. See gaps below.

## Granular UI review after the reported screenshots

The model picker, panel/file workspace, transcript renderer and transcript
backend were inspected in parallel against source 0.2.128 and official MyGo
examples. A further sidebar/action review follows the same source/component
approach. The individual checklists are `audit-model-picker.md`,
`audit-workbench.md`, `audit-transcript.md`, `audit-agent-data.md` and
`audit-sidebar.md`. They contain explicit remaining items.

Corrected implementation issues include popup geometry and keyboard selection,
provider/model memory, a zero-width model button in a 300-DIP conversation,
file/right-pane fitting, system-theme mismatch, overflowing workspace tabs,
repeated generic assistant transcripts and oversized tool/thought cards.
Typed activity records now drive chronological work groups, truthful counters,
failure recovery and distinct per-session responses. These are implementation
repairs with regression evidence, not a claim that every acceptance case passed.

Earlier running debug executables and `build/coexist-qa-r2` predate these UI
repairs. Current review packages are built separately under `build/ui-audit`;
`scripts/review-ui.ps1` launches a deterministic fresh scenario without resetting
the ordinary profile or the installed original app. File hashes identify the
actual binary; the unchanged in-window Zeron name does not identify its build.

## Automated evidence

The full Go test suite, Go vet and MyGo vet have passed during integration.
The final commands and release hashes must be refreshed after every final
source change; release verification records the actual toolchain versions.

Tests exercise settings pages and30 palettes, control persistence/search,
IME composition not sending, new-session send, question/permission answers,
draft preservation/stale snapshots, viewport bounds, file save/conflict/close,
terminal transport, URL policy, cancellation/queues/persistence/ownership,
source assets and all210 vector references. The image comparison tool rejects
dimension mismatches and never creates reference goldens.

Race detection passed for contracts, mock, desktop and workbench with Go1.27.3
windows/amd64 and portable w64devkit2.10.1 / GCC16.2.0. The exact command was
`go test -race -count=1 -p 1 ./internal/contracts ./internal/mock ./internal/desktop ./internal/workbench`.
All four packages passed without race reports; source hashes before/after were
unchanged. Logs, official compiler asset metadata and SHA-256 verification are
in output/tools/race-check, with the result/log retained in reference/validation.
CGO/CC/PATH were changed only for that process and
restored afterward; normal builds retain CGO_ENABLED=0.

## Native Windows observations

The Go implementation opened a real1320x880 Windows window successfully.
`Window.CapturePage` produced native-render snapshots under output/captures.
This API's transparent capture cannot prove desktop backdrop composition.
Computer Use additionally observed the actual OS window and its accessibility
tree: Message input, source native controls, session controls and browser/terminal
entries were present. That observation exposed the OS light Acrylic being used
with dark app text; native appearance synchronization and the source glass
contrast guard were then implemented.

The user clarified the physical Escape was accidental and authorized restarting.
After reinitialization, the Computer Use tool still retained a stop flag and
refused actions in this turn. This is a tool-state blocker, not pending user
approval. Native compositor observation, real OS send/clipboard/IME, browser
overlay/pointer behavior and window dragging remain unverified.

Independent application self-captures subsequently initialized a real MyGo
terminal and displayed its mock ANSI output (`native-terminal-latest.png`).
The real browser engine returned the go.dev page title. Its separate native
page layer is transparent/black in `Window.CapturePage`; that image cannot
prove webpage pixels or compositor stacking. No browser visual pass is claimed.

## Reference and visual acceptance

Original source hashes, source assets, theme export and historical reference
captures are recorded in reference/. The copied historical images are explicitly
not current Windows goldens. A disposable original reference build preserves
the source checkout. Its unchanged 0.2.128 sidebar fixture compiled successfully;
`reference/native-build.json` records the executable hash. Fresh matched-state
native reference captures are pending.

`scripts/capture-implementation.ps1 -AllThemes -BoundarySizes` generated 77
implementation-only images and a SHA-256 manifest in output/captures/matrix.
These cover representative pages, all thirty variants and boundary/DPI cases;
they do not cover every state of the 161-case reference inventory and are not
reference goldens. Review found missing Mermaid SVG text. All thirty diagrams
now convert the original text to Geist glyph paths with the reference usvg/
fontdb chain, preserving canvas dimensions. All180 label locations have native
foreground-pixel checks and the matrix was recaptured. This resolves that
observed defect, while paired visual acceptance remains pending.

No same-platform, same-data reference/implementation SSIM result is claimed.
No screen has been declared pixel-perfect. DPI100/125/150/200, all state
combinations, compositor materials and the full performance targets remain
unaccepted until the paired evidence is captured.

## Known remaining coverage

- Complete original theme-library modal/import/link/reload/mapping review.
- File CRUD/drag, source tree rows, image zoom and history lane geometry are
  implemented and tested; their full native presentation still needs comparison.
- Full PR review timeline, inline review comments, provider detail dialogs,
  original model-popup transitions and remaining conditional account flows.
- System wallpaper color matching and complete source animation choreography.
- The complete source shortcut catalog, remaining Markdown/preview style
  permutations and imported SVG text presentation need further coverage.
- Background adjustment/effects and motion preference now work; their complete
  native scene/DPI matrix and operating-system material combinations remain open.
  Background animation pausing currently resolves motion to its final state;
  exact source hold/resume behavior is not yet accepted.
- Unused staged attachment resources and crash-created orphan files currently
  require cleanup; no automatic resource garbage collection is claimed.
- Full source layout policy, per-surface exact materials, glyph metrics and
  platform-specific titlebar details require paired visual iteration.
- Every state in the reference manifest requires runtime/visual evidence;
  a source inventory is not acceptance.

## macOS

Architecture/package verification is independent from runtime validation.
The release scripts build Intel, Apple Silicon and Universal packages and
verify executable/native-library architectures and file checksums.
Native macOS execution, VoiceOver, Retina, IME, menus, vibrancy and browser/
terminal behavior are pending on their respective Macs. Signing/notarization
and DMG creation require the macOS distribution environment.

## Completion gates

Windows completion requires no unresolved actionable visual/interaction
differences, the full manifest covered, matched reference evidence and a tested
release package. macOS runtime results are recorded separately per architecture.
Neither gate is currently marked complete.

## Previous integrated review package (superseded for current source)

The previous package is `build/ui-audit`, rebuilt after the sidebar fixes.
The 158-file application/build source snapshot remained unchanged throughout
the final build and race run (`output/audit-validation/sources-final.json`).

- Full Go tests, Go vet, MyGo vet and golangci-lint 2.15.0 passed; lint: 0 issues.
- Race passed for contracts, mock, desktop and workbench with portable GCC.
- Windows, macOS Intel/ARM and Universal packages passed architecture, native
  terminal library, pinned framework, resource, ZIP and checksum verification.
- Installed Windows package passed start/self-capture/uninstall checks for
  chat, terminal and browser. The replica registry/shortcut were removed and
  the original Zeron shortcut hash stayed unchanged.
- 79 fresh implementation matrix PNGs cover representative routes, data states,
  all 30 themes and viewport/scale boundaries. Separate focused audits retain
  model, transcript, sidebar and combined workspace screenshots.

Results and current package/source hashes are in
`reference/validation/ui-audit-result.json`. Installer self-captures do not
prove operating-system composition or website-layer pixels. No fresh matched
GPUI comparison, native IME/clipboard/DPI interaction matrix, performance gate
or Mac execution has been marked passed.

## Machine handoff, 2026-10-28

The source has since changed: central/side composer controls are shared, the
command/project workflows and typed integration settings have expanded, and
the main-chat working trailer and status strip follow the frozen source.
The package and implementation captures above describe their recorded source
snapshots; they do not validate the latest source. Rebuild in a fresh directory.

Current source checks and the exact source manifest are recorded in
`reference/validation/handoff-result.json`. The handoff includes curated audit
images and logs, not compiler caches, credentials or personal profiles. Start
with `HANDOFF.md` and `docs/machine-handoff.md` on the next machine.

The official Windows compact fixture compiled, but its `render_to_image` API
is unsupported on Windows. A separate capture-transport adapter also compiled;
its ready metadata is preserved without any reference PNG or visual pass claim.
Paired native reference capture, exact geometry and interaction acceptance,
the full Windows matrix and each Mac architecture remain pending.

The user requested that the active parity goal be paused after this branch is
pushed and verified, then explicitly resumed on the new machine.
