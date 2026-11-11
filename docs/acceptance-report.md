# Acceptance report — 2026-10-28

> 本文保留原审计阶段的框架版本与验收范围；当前框架及补丁以 [mygo-patches.md](mygo-patches.md) 和源码锁为准。

Overall result: **in progress / not visually accepted**.

Latest sealed phase: `reference/validation/tabstrip-r16-result.json` (R16).
R16 corrects inactive file images and detail eligibility. Five gates and four
release package builds passed. Timed original PNGs retain tooltip differences;
the GPU control reproduces light rounding without yet binding actual UI adapters.
Full visual/native platform acceptance remains pending.
Earlier sections retain their original source/executable and acceptance scope.
The R17 phase recorded here used the adopted, unreleased `.9` tree: 1,864 files
and 14 patches. The current framework is `rcode-native-ui.23` (MyGo v0.7.4):
2,050 files, 19 patches and tree
`c50af74375c6b2cbb2b8a50a09c407469dd419b29788d5774bed1e8c99deda50`; see
`mygo-patches.md`. Its pre-Cloud adoption had 2,931 formal audit inputs. The later Cloud
four-file adoption below has a separate source identity; that old input manifest
and canonical packages are not relabeled to cover it. See
`audit-tabstrip-tooltip-primitives.md` for exact identities and separate V5 evidence.
The pre-Cloud `.9` pin and five gates passed via retained attempts01–03. Its canonical four-target
package build/verification passed in separate attempt02; earlier `.8` results
retain their identities.

Later P0 evidence is separate from formal `.9`: the private V4AX Go baseline,
trace-off and trace-on runs passed all six whole-PNG byte comparisons to old15,
with the first failed baseline retained. Normal Release produced 12 uploads,
12 Globals, 346 instances and 67 issued draws; 20 custom PS identities remain
notCaptured. Native ledger R7 is25 trials/214 PNGs/20 passed/5 failed. This is
host-submission evidence, not GPU completion, Present, exact WGC producer or
reference parity. GPU04's288 controlled cases retain a1-level BGRA8 difference
and nonzero CPU/GPU ULP differences. See the primitive audit for frozen records.
The default reference metadata retains its historical requested-feature gap. A
later feature+Windows-filter metadata exited0 and its helper stopped at2, with
959 packages/nodes and30 workspace members. Lock5c7b128d…caf34 and all4323 raw
inputs stayed unchanged. Completion a1e9cf48…73f1e and independent review
1e4fe0d2…2a3b close that originalP1; workspace feature union is still not an
actual package/example compilation-unit list. These metadata and first failed
Job-smoke records (8aa94b81…8178) contain no reference project compilation;
SDK/first build/ABI/native require separate evidence. The new planningP3 is
limited to prepared plan.graphScope's overbroad “all workspace dependencies”.
README's “workspace-wide” remains valid; no Cargo-documentation/tool bug is
claimed. Old19 discrepancies and phase as-of boundaries remain intact. See
`audit-tabstrip-tooltip-primitives.md` for exact source/evidence paths.

The later Job02 own-Python normal and timed parent/child smoke cases passed
(055e6710…3e68b). The first reference Rust project build then actually ran and
Cargo exited101: blake3's custom build failed with LNK1181 for an input .o.
The driver exited1 without retry; outer execution f76336b9…6fbf3 binds the raw
logs. Its original/raw and then-current2931 inputs/7docs/current guards stayed
unchanged. actualWorkInputDrift is null, not an empty array or a no-drift proof.
The later environment-only repair is now compiled: global CC=cl.exe retains
the absolute target-CC binding and selects actual ml64/MSVC.asm. Cached Cargo
exited0 in11m23s; source/raw, actual work/C5 raw subset, lock and prior receipts
stayed unchanged (new result97f00838…68d21, trace EXE f678bc89…c70ba). Bounded
own-Job helper cleanup is recorded separately from the zero compiler exit;
natural descendant teardown is not claimed. Reference baseline, runtime ABI
and baseline/off/on native whole-PNG gates remain pending.
Cloud navigation's four-file02 repair is adopted
(0b3a17e9…b090a): four failing red scenarios became green, with nine top-level
and whole-repository overlay tests passing. See [Cloud audit](audit-account-settings.md).
A separate normal native-worker path is now recorded (a5981b8b…63b1c): the
2933-input development EXE69a43719…f0090 on an independent offline profile moved
from off rev2 through provisioning rev4 to ready/cloud-1 rev5; repeated same-page
entry stayed rev5 and settings-search re-entry reached rev6. PID50440 exited0
with empty stderr. The first wrong-window image was rejected and target activation
recovered the subsequent images. Live pin/tagged/Go-vet/MyGo-vet passed; the
GOPROXYoff lint startup failure remains, followed only by a local-file-proxy lint
rerun with exit0/0 issues (8d0df8f4…3b6b1). Native failure/cancel/rapid-route cases,
race/thread tracing, exact capture producer, failed_action and the typed one-shot
Enable-to-Off difference remain open. This is no new canonical package, complete
Cloud visual acceptance, or closure of P0/focus native gaps.

The later exact9c58 focus attempt launched once and exited normally0, with main
marker/undo and side mounting observed. Its raw PowerShell probe was rejected
by Execution Policy (exit1) before COM. Combining Sky and direct PS UIA in one
turn also violates Windows Computer Use guidance:237; that path stopped with
no policy bypass or alternate-runtime workaround. Source triage04174c1d…36c3
and native record822ccedc…16f61 leave the four-phase public-focus and model-popup
matrices pending, without a new product defect or workaround. Historical R9
raw MTA/fresh STA established the tool-field mismatch only for its originalR8
package sequence; it is not relabeled as this `.9` result. Skill/diagnostic-path
restrictions are not product-code defects.

Reference: Zeron `ab189159f10cf8e63c0502be75a59bbc487e0eaf`.
The R16 sealed build target was Go1.27.3 / MyGo0.6.4 + pinned `zeron-native-ui.6`,
Windows11 amd64; macOS13+ amd64/arm64. The current framework is MyGo v0.7.4 +
`rcode-native-ui.23` (2,050 files, 19 patches; see `mygo-patches.md`). The
reference is Zeron 0.2.128; the independent replica package version is 0.1.0.
This report distinguishes implemented behavior, automated checks, native
observations and comparison with the reference.

## Available implementation

- Native application shell, titlebar, sidebar, session/project/model selection,
  composer, virtualized transcript, streaming, tool/thinking disclosures,
  questions, PermissionRequest hook prompts, todos, subagent cards, attachments
  and queue actions. The separate multi-permission mode was removed on 2026-10-30.
  Todo lists initially covered display fixtures only; successful builtin
  `TodoWrite` records now feed a single latest-list tray attached above the
  composer, stacked above the compact goal header. Goal editing opens a
  conversation-owned workspace tab. See [Todo and goal panels](todos.md) for
  behavior, reference sources and verification boundaries.
- Typed in-process service, deterministic scenes, isolated persisted state,
  cancellable/coalescing events, file version conflicts and scripted terminal IO.
- Native settings pages, theme/font/palette controls, original fonts/icons and
  all30 source theme variants. Voice controls are excluded.
- File tree, editable buffers, dirty-close/conflict handling, Markdown/image
  preview, changes/diff, commit history. The old PR workbench was removed; the
  sidebar keeps only the repository PR web link. See `ui-residual-cleanup.md`.
- System web engine confined to external document content, native browser
  chrome, navigation/loading/error state and overlay support.
- MyGo terminal over an in-memory Conn, not a shell. Native library packaged
  for each architecture.
-210 math/diagram SVG resources produced by the original renderer code.
- Source compact model panel, effort slider and model search. Model favorites were
  removed on 2026-10-30; see `ui-residual-cleanup.md`.
- Native file selection, external file drop and clipboard image/file ingestion,
  with backend-managed resources and draft/session isolation.
- Source background cropping, pan/zoom adjustment, five source-space image
  effects, and native GPU source-alpha feathering. Raster samples exported
  independently from Rust match the Go effects byte for byte.
- The sealed R16 baseline's eleven reviewed patches supply per-window motion/scroll preference,
  hover/drop callbacks, exact Markdown line heights and independent package
  installation names, read-only preedit-aware input snapshots, opt-in popup
  fitting, placeholder color, caret width, Windows UIA provider declaration
  and per-container scrollbar indicator/hitbox policy.
  Its1,836 framework files and eleven patches remain a historical hash-locked snapshot.
  Historical R17 C4 source has1,847 files and thirteen locked patches, including
  tooltip/input/device-scale primitives and bounded Windows package-commit
  retries. The R17 `.9` phase added the fourteenth Windows glyph-placement patch
  and the checked tooltip leaf while preserving AX content names; it has1,864
  framework files. Evidence scopes are in `audit-tabstrip-tooltip-primitives.md`.

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
repairs. Review packages from that phase were built separately under `build/ui-audit`;
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

## Earlier-machine native Windows observations

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

The earlier-machine stage did not establish a matched SSIM result. The resumed
stage below now records matched complete-window comparisons; none has passed.
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

## Resumed machine, 2026-10-28

The user has explicitly resumed the goal on the new machine. It is active;
complete parity remains unaccepted. See `machine-resume.md` and
`reference/validation/resume-result.json` for the current phase rather than
interpreting the historical pause above as the current state.

Device-specific composer provider filtering and side transcript activity
trailers have new regression evidence. Native Windows observations include
Chinese draft restoration and mock sends, side-only activity and completion,
and actual side keyboard/undo routing. Computer Use returned original JPEG
images; these are observation evidence, not lossless PNG goldens or a paired
SSIM pass. UIA focus reporting still disagrees with the observed side input
route. The shared composer now has tested compact/expanded and route-specific
settled geometry, preedit-aware layout and native undo; fresh native observations
exercise multiline expansion, collapse after undo and maximum-height scrolling.
That historical phase used `.3`, with a replayable read-only input snapshot API.

Eight independent original GPUI Windows panel/list observations are now recorded
at dark/light and wide/narrow sizes. They are original JPEGs, not lossless PNGs.
The frozen fixture subsequently fails a keyboard assertion: it assumes obsolete
focus traversal while production compact-panel keys have changed. No keyboard
PASS or paired visual pass is claimed. The full Windows acceptance matrix and
both native Mac architectures remain pending.

The resumed stage now also has eight original lossless WGC PNGs and eight
implementation native client PNGs from a build-tagged fixture that reuses the
production model popover. Their capture boundaries differ (842×592 versus
840×560 for the wide case), so strict comparison rejects them without producing
SSIM. Popup surface/rail material and option-row differences are visible and
remain actionable; see `reference/validation/compact-fixture-comparison.json`.
No crop, resize or mask was used.

The prior integrated Windows, macOS Intel/ARM and Universal packages passed build,
architecture, resource, native-library and pinned-framework validation. The
Windows package executable additionally passed actual End/Down/Enter/Escape
model-panel operations. Installation and the full Windows native matrix are
still pending; cross-builds do not replace either Mac's runtime acceptance.

## Matched lossless Windows comparison

The original frame configuration was retained after two fixture-only frame
experiments failed to produce equivalent client boundaries. The implementation
now also captures its complete native HWND through an isolated in-process WGC
bridge. Both sides preserve original PNG bytes, use scale 1, client 840/360×560,
full frames 842/362×592 and the same client offset within the native window.
The bridge validates caller ownership and does not enumerate other windows.

`scripts/compare-compact-fixtures.ps1` validates PNG hashes/dimensions, typed
scene state and transport metadata before comparing all eight original pairs.
The R3 report is `output/compact-comparison-20261028-r3-reviewed/comparison.json`.
SSIM ranges from 0.971695047 to 0.991387733; all eight are below 0.995. No input
image was cropped, resized, registered or masked. Popup geometry/material fixes
are implementation progress; remaining text, trigger and list differences are
being corrected against the frozen source. These scores do not establish full
keyboard, motion, accessibility or platform acceptance.

R3 source/binary and bridge identities remain historical evidence. Subsequent
source changes and the bridge's fresh-path correction require new captures and
their own hashes. The earlier package/source manifests above likewise do not
validate later popup styling changes.

The source-confirmed open trigger wash, inherited medium typography and
keyboard-active favorite star were then corrected and recaptured as R4, using
the final corrected bridge and a separately hashed fixture executable. All
eight PNG/sidecar hashes, physical sizes and typed states were checked again.
`output/compact-comparison-20261028-r4-reviewed/comparison.json` records SSIM
0.976711056–0.993399479; all eight remain below 0.995. Option text metrics,
glyph/baseline and material residuals, plus unmeasured native activation/DWM
state, remain open. `reference/validation/compact-fixture-comparison.json`
preserves each phase and points to this latest comparison.

The next R5 stage traced a width residual to Windows DirectWrite's default
Typography behavior. The application and native fixture now use the existing
`FontFeatures("kern=0")` API on Windows to match the frozen renderer's default
Typography; font files, sizes and pinned framework are unchanged. Independent
measurements cover 312 complete glyph comparisons. Nonempty feature sets and
other platforms remain outside that mapping; see `audit-windows-typography.md`.

R5's eight complete-window comparisons are recorded in
`output/compact-comparison-20261028-r5/comparison.json`. Dark/light wide panels
score 0.995681626/0.995021923 and meet the numerical screen. The other six
score 0.984548485–0.993596317 and do not. No scene is visually accepted: small
text-position/material residuals and native frame state remain unresolved.

The final R5 source snapshot has 208 application/build/fixture entries and
remained unchanged across full/tagged tests, vet, package checks, lint and native
startup captures. Windows, Mac Intel/ARM and Universal packages were rebuilt
and verified. The Windows executable self-captured default/dark and
transcript/light native client views and exited 0; this tests startup/rendering,
not a new interaction sequence, installer or Mac runtime. Current phase evidence
is `reference/validation/popup-r5-result.json`; all earlier source/package records
remain phase-specific. Full footer behavior also remains pending; the exact
source/data/action boundaries are in `audit-composer-footer.md`.

## Later option and footer evidence

R6 independently measured original option text boxes and applied a scoped
Windows1x integer-boundary correction. Three of eight complete compact pairs
meet0.995; five remain below, with no visual acceptance. See
`reference/validation/text-bounds-implementation-r6.json` for the measured
domain, original bounds, tests, hashes and remaining glyph/material findings.

The footer implementation now includes typed checkout/ref/PR state, absolute
host context reports and the shared device-account state. Draft new-worktree
allocation, settled read-only footer, exact PR source matching, context decrease,
ownership/late results and account polling/switch rollback have state and UI
regressions. Cloud is exercised through the normal mocked settings/connect/
repository/send path. These tests do not connect to a real host or account.

Independent original production footer components and implementation components
were captured in28 dark/light known/unknown/capacity/zero/over/warning/danger
and open-card cases. The final pair is actual Windows2x/DPI192 with complete
1684×1180 WGC frames, logical840×560 and matching internal client offsets.
All28 scores exceed0.995 (0.998987004–0.999950231), without transformations.
The wide blank scaffold makes this a component numerical screen. Actual source
Inspector bounds separately establish account-card121px and the context/chip
intrinsic widths. Neither evidence accepts the complete footer or OS input.
The strict comparator's33-case review includes31 invalid-input rejections
before output/parity plus valid1x/2x controls; it is not33 parity executions.

Windows packaged-app observations include main context/account cards and
separate main/side context readings. The next click was interrupted by user
input detection and a screenshot inconsistent with the selected window; input
automation stopped and only the verified owned test process was cleaned up.
Side-card clicking, checkout/ref sends, PR destination, animation, installer,
Windows input/DPI/browser/performance and both Mac native platforms remain
pending. The observed package is recorded separately from the later lint-fixed
build; no fresh native acceptance is inferred from rebuilding.

Current phase/source/check/package evidence is
`reference/validation/footer-result.json`; native observation limits are in
`output/footer-native-ui/evidence.json`. The goal remains active and incomplete.

## R7 source popup and input alignment at actual Windows2x

MyGo `.4` adds opt-in overflow-only SnapToWindow, PlaceholderColor and
CaretWidth APIs. Nine ordered patches reproduce the current1,834-file tree;
reversing the three additions recovers `.3` exactly. Application popup cold
geometry, search14px/Faint,2DIP native caret and absolute22.75DIP ComposerInput
leading follow the frozen renderer rather than wrapper styles. The corrections
and remaining fractional/IME/tooltip boundaries are in
`audit-native-popup-style.md` and `audit-model-list-residual.md`.

The newly bound native fixture records the actual2x production branch. Eight
independent original/full-window implementation pairs score
0.986806768–0.996676105, with2/8 reaching0.995 and zero visual acceptances.
Earlier R6 was actual1x; the counts are not a controlled same-DPI comparison.
The new28 footer pairs score0.998796753–0.999755061 and all pass the numerical
screen; blank-scaffold dominance, native frame state and full-footer/input
limitations remain. No images were transformed. Comparator guards55/36 cases
pass, including actual1x/2x preflight controls and invalid-input rejections;
the guard controls intercept build and do not run parity/native code.

All256 frozen sources remain unchanged after tests, vet, lint, four-target
package validation and36 implementation native fixture captures. Source lock,
hashes and exact evidence identities are in
`reference/validation/popup-r7-result.json`. These fixture processes exited0
without OS input. The R7 Windows package was then run in default/dark and
transcript/light scenes: native client self-captures, exit0 and empty stderr,
recorded in `output/resume-validation/popup-r7-native-boot/evidence.json`.
That establishes startup/route/render only, separately from complete-window
fixture WGC and from reference or keyboard acceptance. Installer, Windows
input/DPI/IME/browser/motion/performance and both Mac native platforms remain
pending. The goal is active and complete parity is false.

The later R7 packaged-app input supplement is
`reference/validation/native-input-r7-result.json`. Opening the actual Agent
card mounted its workspace side transcript. Side and main usage cards showed
8,400/200,000 and42,000/200,000 respectively; native side draft/send reached
only the in-process mock, completed with13,200/200,000 on the side and unchanged
21% on main. Side caret/text routing worked while UIA still reported main
editor focus, so accessibility consistency remains unaccepted. These are raw
Computer Use observations tied to the R7 package SHA, not PNG/reference parity.
Visible test windows were closed through Alt+F4 and absence confirmed; their
exit codes were not recorded. Newly found header overlap and primary-menu
interception were subsequently fixed in R8, preserving R7 evidence identity.

## R8 material/header/provider checks

`reference/validation/popup-r8-result.json` seals260 unchanged sources and
MyGo `.5` (1,835 files, ten ordered patches). Patch10 corrects the Windows
provider contract and the misleading Bounds comment. Exact replay/reverse,
Windows/UI suites, application tests, vet, lint and four-target package
resource/architecture/checksum/stamp gates passed. Initial failures from Go
scanning ignored diagnostic backups are retained; moving raw backups under
`_source` allowed clean full reruns. Mac remains build-only.

The local accent overlay now follows the frozen border-box geometry. Eight
new actual2x compact pairs score0.988087192–0.997084323:3/8 meet0.995, with
unchanged list scores and zero visual acceptances. All28 new footer pairs
meet the numerical screen, with the same blank-scaffold/native-frame limits.
Original PNGs, sidecars, IHDR, source and binary hashes were rechecked. No
image transformations were used. Prior55/36 guard suites remain unchanged.

The new Windows package also passed observed primary pane toggle, secondary
menu, takeover/collapse, restored information-card target and titlebar drag.
Side text and native undo reached only the side editor. Alt+F4 produced a
recorded exit0 and empty stderr; this phase did not send a side message.
UIA still reported main editor focus after fresh settled observations, so
the provider flag correction does not establish focus consistency. Raw
observations are in `output/resume-validation/popup-r8-native-input`.

Original UIFont13/16/20 bounds were independently measured in the R9c
adapter with three exit0 runs. This does not change Go rendering or directly
measure glyph baselines/fractional ancestors. Typography, complete footer,
canvas controls, input/IME/browser/motion/performance, otherDPI and both Mac
runtime matrices remain pending. Complete UI parity remains false.

## R9 public-focus diagnostic supplement

Using the unchanged sealed R8 executable, independent external raw COM and
fresh managed UIA queries followed side→main→side correctly. Nine stable raw
rounds include18 successful GetFocusedElement calls and36 bracketed editor
Current focus reads with stable complete RuntimeIds. The original tool still
reported main during side input. This sampled public-focus consistency does
not establish focus-event delivery or the complete accessibility matrix, nor
prove the tool's specific cache/mapping mechanism. Diagnostic DPI contexts
differed; their rectangles were not geometry acceptance. Native input/undo and
exit0 are separately recorded in `reference/validation/native-focus-r9-result.json`.
R7/R8 historical observations remain unchanged. Subsequent R10 canvas source
changes require their own checks and native binary evidence.

## R10 canvas visibility and session round trip

The titlebar and right/Files effective fit now use actual selected-row
existence. Empty/dangling-ID canvas has no session trailing controls or right
takeover; stored panel intent remains intact. Fresh NewConversation also closes
the canvas terminal even when navigation keeps the same empty session ID.
Later terminal opening remains available. Backend dangling-selection
reconciliation is a separate pending behavior.

Only four of260 sealed sources changed from R8; framework remains `.5`.
Final full/tagged tests, Go/MyGo vet and lint pass, and all four package targets
pass architecture/resource/archive/checksum/stamp verification. The new Windows
package passed native canvas terminal open/reset/reopen and selected-session
Files/native file editing → takeover → canvas → original-session restoration.
An unsaved controlled comment, active file tab, Files/expanded state and main
draft survived; collapse and primary pane toggle remained reachable. Actual
exit0 and37 hashed observation files are in
`reference/validation/canvas-r10-result.json`.

These are deterministic mock/native observations, without Save/send/real service
or installer. Prior R8 fixture PNG scores were not relabeled for this executable.
Nondefault editor scroll/selection, multiple-tab order, source terminal intent
restoration and the complete visual/accessibility/input/platform matrix remain
pending. Mac remains build-only; complete parity is false and goal stays active.

## R11 selected-session reconciliation and TabStrip

The R10 visual guard did not clear the removed ID used by sends. R11 reconciles
against accepted Sessions, preserves Settings/PR navigation, restores the prior
canvas destination/draft, validates historical routes and blocks deleted-scope
late send/attachment restoration. Cloud checkout survives only the same project
and host; project fallback follows the source's actual display-order branch.
Two misleading source fallback comments are recorded in the session audit.

An optional durable `Run.requestId` preserves creation ownership after the
capacity1 event stream coalesces its transient ACK. Current draft generation
and preexisting RunID checks prevent unrelated/side/historical adoption. Tests
cover both completion orders, navigation, persistence, deletion before ACK and
new-canvas generations. The independent source review has no open findings.

TabStrip uses expand28 and plus24; plus follows scrolling chips, expand stays
fixed. Hover, collapse icon, effective reduced-motion and exact typed Git guards
are covered. NonGit entries are omitted. Bottom7DIP scrollbar interception,
36px fades, drag/reveal motion and custom168px menu parity remain pending.

All five final-r4 gates pass for262 frozen sources; four target packages pass
architecture/resources/archives/checksums/stamp verification. The exact Windows
release EXE was exercised through real OS input for mock session creation,
Changes/plus menu and expand/collapse. A separate native adapter exercised typed
mock backend deletion through the ordinary subscription, canvas draft recovery
and OS Enter creating session1001; backend user block and request/run relation
were observed. Both queried HWNDs report DPI192/per-monitor, actual exit0 and
empty stderr. Release18 and adapter26 raw files have separate manifests; the
adapter is not release-binary delete-menu acceptance. Its first hidden launch
had no targetable window and was stopped with exit-1, retained as failed evidence.

No real provider/service, installer or OS setting was used. R8 paired PNG scores
remain historical; no R11 reference pixel score or full visual pass is inferred.
The complete Windows input/accessibility/performance matrix and Mac Intel/ARM
native acceptance remain pending. Full UI parity is false and the goal is active.

## R12 scrollbars, opaque fades and idle convergence

The current `.6` framework has1,836 files and11 exact patches. Generic
`Scrollbars(false)` suppresses only the current scroll container's indicator
and hit targets; wheel, TrackScroll, keyboard, accessibility and neighboring
default behavior remain. Only TabStrip opts in. Exact ordered replay,
reverse11 to`.5`, reapply to`.6`, generation idempotence, ui/markdown tests and
vet pass. All20 before/after example builds pass; counter grows16/32 bytes
on Mac amd64/arm64 and101 bytes on wasm, with Windows/Linux unchanged;
hello is unchanged on all five targets. No all-platform zero-cost claim.

Source36DIP opaque shell ramps are implemented. Six confirmed stale-fade
cases occurred when MaxX changed without a requested host frame. Paint-time
pure ScrollState reads fix all eight idle cases without timers or extra
frames. The final51 workbench tests and all five repository gates pass for
263 frozen source entries (eight changes fromR11).

First packaging failed at a fresh macOS amd64 staging rename with Windows
AccessDenied; retained partial artifacts are not accepted. A second fresh
four-target build passes architecture/resources/ZIP/checksum/framework
identity checks. Its exact Windows EXE was exercised at DPI192: wheel and
lower-edge coordinate clicks switch diff tabs, plus opens its five native
menu entries, History opens, and expand/collapse works. It exits0 with empty
stderr;48 raw evidence files retain their identities. Exact last-DIP and
1x/2x coverage are Tester checks, not a complete native boundary matrix.

Native testing found an additional failure: an active History tab can stay
clipped after insertion or takeover collapse until manual wheel. This is
preserved forR13 diagnosis, not counted as passed automatic reveal. True glass
alpha masking, transitions/custom menu, matched original captures, complete
Windows native coverage and Mac Intel/ARM execution remain pending.
See `audit-tabstrip-scrollbars.md` and theR12 phase index. Full parity is false.

## R13 menu-selection reveal

R12's menu-created partial chip is fixed by consuming only the reveal request
present on TabStrip entry. A synchronous menu choice after the old chips are
built retains its new request for the next pass. All8 menu cases fail before
and pass after;6 direct/End controls, manual-wheel retention and all53 package
tests pass. The framework stays`.6`. Only3 of263 source entries change, including
a306-CRLF-to-LF metadata normalization with equivalent JSON and50 other license
files unchanged. All5 gates and default diff-check pass;4 target packages verify.

The exact new Windows EXE was exercised at DPI192 through the same native
plus-menu path. History is fully visible immediately and after idle without
repair input. Deliberate manual wheel then stays put while History content
remains active. Actual exit0, empty stderr and34 raw files are retained.
Resize/takeover clipping is a distinct observation with unestablished original
auto-reveal expectation; it was not changed or labeled a confirmed parity bug.
Reference matched visuals, glass/transitions/menu and the full native matrix
remain pending. See `audit-tabstrip-reveal.md`. Overall parity remains false.

## R14 production TabStrip component baseline

36 independent native pairs use actual2x DPI and the same declared host, typed
idle SideChat data, first active tab and requested fraction of each real maxX.
The final source/Go captures bracket WGC with actual own foreground identity;
source also proves querying on its owner UI thread. GPUI cached active alone did
not prove this in baseline01, which is retained under its original identity.

69 guards pass, including strict boolean/owner types and byte snapshots isolated
from later input drift;3 positive controls intercept build without parity,66
negative cases reject before output mutation. Five repository gates pass. R13
production drawing is retained; only observer bindings/empty normal implementation
were added. R14 creates no new release packages; earlier release-native evidence
remains tied to R13.

Final full-frame SSIM0.9998630230588416..0.9999965707212943 is screening only.
Host pixels still differ, including active fill and inactive generic glyph;
dirty-file dots are outside these idle side-chat scenes. Caption buttons now
have the active appearance, while20,772 caption pixels still differ by at most
one channel level. Actual window screen origins differ; DWM/position/raster
explanations are not established. No image is cropped, resized, registered or
masked. Diagnostic regional counts read original full PNG coordinates and have
no pass threshold. See `audit-tabstrip-fixture.md`; overall parity remains false.

## R15 styles and exact release smoke

See `audit-tabstrip-style.md` and `reference/validation/tabstrip-r15-result.json`.
Float wash, inactive generic glyph opacity and6-DIP dirty dot are implemented.
Five gates and four package targets pass. The exact Windows release opened,
switched and context-closed file tabs and exited0 at DPI192.36 baseline pairs
and four supplemental observations remain visually unaccepted; light blue
channel differs by1, hover dwell/tooltip is unmatched, glass and Mac native pending.

## R17 V5 source adoption and limited native regression

The reviewed27-file delta was adopted as `.9`, tree
`27e5b9c724175bd11d537765210727fe25722a3407989e7fdcae71fa4357e8d9`; the first13 patch bytes and order remain.
The actual app uses the currentAX961 input set and one checked glyph-placement
leaf wrapper, without restoring generic labels or the old959 app. This does not
adopt the separate wrapping candidate.

The preceding V5 diagnostic tree c18a…1165 measured ten raw file non-growth
comparisons relative to C4: nine zero deltas and Wasm counter−425B. Runtime and
metadata zero cost are not established; historical C4 counter+2048B versus `.6`
remains open. The isolated diagnostic EXE produced40 whole PNGs equal to the
separate V4AX frames, with both-theme `Tab02.txt` AX names completed by run19
after run17 missed light AX. Full-frame reference residuals remain22410/39185
pixels for dark/light and maximum text channel difference1. Native review has
zero new findings and retains one planningP3. None establishes reference parity.

The source adoption, `.9` fresh checks/packages, diagnostic native fixture,
R16 release index and Mac acceptance have separate identities. The fresh `.9` gates have
now passed with their own exact-source records, followed by canonical package
build/verification. These establish neither native nor release acceptance. See
`output/tabstrip-r17-quarter-v5-adopted/attempt-01/` and
`output/tabstrip-r17-quarter-v5-ax-native-independent-review/attempt-01/`.

The fresh `.9` gate attempt01 retained a real root `go test ./...` exit1 from
unisolated diagnostic copies under output, despite product-package test success.
The added `output/go.mod` shared module boundary preserves existing evidence
paths/bytes and2931 production inputs; the root command is unchanged, without
package filtering. Attempt02 passed pin/full/tagged/Go vet/MyGo vet, but lint
launch failed on offline fixed-module deprecation metadata. Attempt03 reran only
lint against the existing local file proxy cache, exit0, with no network, tool
version or product-source change. Its final record reuses the five exact prior
raw results; it does not claim a complete rerun. All six final commands exit0
and all2931 inputs remain unchanged. See
`output/tabstrip-r17-quarter-v5-formal-gates/attempt-03/result.json`. The original
failure and correction are in `output/tabstrip-r17-quarter-v5-formal-gates/attempt-01/` and
`output/tabstrip-r17-root/v5-gate-discovery-correction.json`; no first-pass-green
claim is made.

Canonical attempt01 stopped at a runner-only GOENV expectation error after two
successful Go preflight commands. With the actual `GOENV=off`, Go1.27.3 legally
returns an empty `go env GOENV` value; this was not a user-environment defect.
It produced zero canonical builds/packages and no2931-source drift. Attempt02
fixed only that acceptance check, retaining the same environment, then one
canonical build and four-target verification passed with all nine commands exit0.
The original result is preserved at
`output/tabstrip-r17-quarter-v5-canonical-delivery-attempt01/result.json`.

Fresh `.9` canonical delivery is sealed in
`output/tabstrip-r17-quarter-v5-canonical-delivery-attempt02/final-result.json`
(8043c0a3…768d): Windows/amd64, Darwin/amd64, Darwin/arm64 and Universal,
2931 source inputs unchanged,231 output files. Actual Windows EXE SHA256 is
`9c58004289a9da967b3e1b6e1baae03c031374b2f0c3aa6fc272677e0b8ff795`.
This packet invoked no GUI, installer or Mac-native run and repeated no unused
size gate. R16 remains the sealed release index; visual/release acceptance is open.


### Exact `.9` main-package Windows smoke

A separate native record now runs the same canonical Windows EXE `9c580042…f795`:
Files, README, layout, README reselect, actual `README.md` detail and `Close tab`
tooltip names, then the native screenshot menu closes layout while the README
editor/filetree/session remain. Alt+F4 exits0 with empty stderr. The seal retains
14 states,15 original Sky JPEGs and the initial client PNG;2931 current source
inputs and backups match. See
`output/resume-validation/tabstrip-r17-quarter-v5-native/result.json`
(0d850ab4…767c). The earlier build-only packet remains build-only.

Observed3200×2000 client pixels and requested1600×1000 are separate facts; they
do not establish a fresh DPI measurement. This bounded Windows smoke does not
accept visual parity, exact tooltip timing, installer, Mac or screen-reader
events, nor reuse the inherited`.8` diagnostic40-frame source identity.
