# Native compact-picker fixture

> 2026-10-30：产品已取消紧凑选择器，当前生产夹具也统一为供应商/模型两列菜单。
> 旧的 panel/list 场景和 `--page` 参数已删除，导出类型改为 `ModelPickerFixture`。
> 当前矩阵为深浅主题 × 840/360 宽度，共四场景；回归使用
> `go test -tags uifixture ./internal/desktop -run '^TestModelPickerFixture'`。
> 下面的八图、命令和视觉对照结论属于冻结的历史审查，不代表当前菜单验收。

This independent implementation fixture reuses the production model picker. It
is available only with the `uifixture` build tag and adds no product entry point.
It does not establish reference/implementation visual acceptance.

Latest sealed implementation: `reference/validation/popup-r8-result.json`.
Its native actual2x eight-scene capture uses MyGo `.5`, real Window binding and
`oneXOptionText=false`; complete original pairs reach the numerical screen in
3/8 cases, with zero visual acceptances. Historical runs below keep their own
binary/source/DPI identities. R8 footer and packaged-app interaction evidence
are separate from this fixture.

The frozen reference is Zeron v0.2.128,
`ab189159f10cf8e63c0502be75a59bbc487e0eaf`. Its original fixture and production
picker sources were inspected read-only:

| Source | SHA-256 |
| --- | --- |
| `crates/ui/examples/compact-picker-fixture.rs` | `1819ef4c2cce728769ff26480a97b045a4c1083053e8d099baa09c5fedf510a3` |
| `crates/ui/src/pickers.rs` | `fb88f1cd966f3e089af6d52363edfe06fbbe998aabb73c250ab5b496b898d560` |
| `crates/ui/src/pickers/compact.rs` | `dab709e36041c0d4799bc1e83c7cde8e991f2534801cafe222f937083983d31e` |

## Reuse and controlled state

`internal/desktop/model_picker_fixture.go` calls the existing
`centralControls().modelTrigger(c)`, which calls the production `modelPopover`.
There is no copied compact panel, list, option row, keyboard handler or slider
renderer. `cmd/model-picker-fixture` mounts that view in a separate MyGo window
with its default native frame; it draws no product titlebar and has no 900px
product-window minimum.

The fixture matches the example's client scaffold: 24px outer padding,
bottom-aligned header and 48px trigger row, 16px gap, header inset 8px,
20px/26px/weight500 title with baseline alignment. The reference Pickers root
is a min-width-zero flex row with 4px gap; this catalog has no access-mode chip,
so its only child is the production model trigger. It selects opaque surfaces
and reduced motion.
The matrix is dark/light × 840/360 logical client width × panel/list, all 560px
high. Pages are selected as controlled fixture state; the example's later
incorrect keyboard sequence is not replayed.

Business state comes from a deterministic in-process `contracts.Service`.
It has no account/session/user-profile data, executes only typed settings
updates, and returns owned snapshots. It contains exactly one installed/enabled
Codex provider and the four source catalog models:

- `gpt-5.4`: GPT-5.4; “Best for complex coding and reasoning”.
- `gpt-5.3-codex`: GPT-5.3 Codex; “Optimized for agentic coding”.
- `gpt-5.2`: GPT-5.2; “General purpose reasoning”.
- `gpt-5.1-codex-mini`: GPT-5.1 Codex Mini; “Fast, efficient coding”.

GPT-5.4 has the source `serviceTier` choices `standard`/`fast` and `contextWindow`
choices `standard`/`1m`, both defaulting to `standard`. Optional `ChoiceLabels`
preserve the distinct visible labels Standard/Fast/1M tokens while mutations
retain original IDs. Source reasoning enum values map to this application's
existing display-oriented DTO strings Low/Medium/High/X-High. Effective High
matches the source default; the fixture does not test explicit-versus-implicit
reasoning serialization.

## Commands and checks

Run from this checkout with a fresh capture directory:

```powershell
$taskStamp = go run ./scripts/check-framework.go -ldflags
go build -tags uifixture -ldflags $taskStamp -o output/fixtures/model-picker-fixture-new.exe ./cmd/model-picker-fixture
./output/fixtures/model-picker-fixture-new.exe --out-dir output/captures/compact-picker-new-run
```

The default produces the eight-scene matrix. A single scene can be requested:

```powershell
./output/fixtures/model-picker-fixture-new.exe --matrix=false --appearance light --page list --width 360 --height 560 --out-dir output/captures/compact-picker-one-scene
```

`--capture-mode=client` remains the default and uses MyGo's client capture API.
The optional Windows `--capture-mode=wgc --capture-bridge=<absolute DLL path>`
uses the isolated in-process bridge to capture this process's own native HWND.
It retains the default native frame, explicitly clears the title after window
creation (an empty `WindowOptions.Title` alone defaults to the application
name), and leaves OS appearance at `ThemeSystem`. The view still selects its
controlled dark/light palette independently. It does not crop or scale the WGC
frame. This path adds no production runtime entry point or Go dependency.

The native HWND is obtained on the UI thread and passed to the capture worker
as a plain integer. The bridge receives no UI object, saves the PNG itself and
writes a `.png.capture.json` transport sidecar. The common command decodes and
hashes the returned original PNG bytes without overwriting a WGC PNG; its final
manifest identifies the capture mode, bridge SHA-256 and sidecar filename.
Bridge sources/build instructions are in
`reference/fixtures/native-wgc-bridge/README.md`. The successful integrated WGC
run is recorded below; a capture transport pass is not visual acceptance.

The command refuses a nonempty output directory. It keeps native runtime profile
state under that output directory and reads no user configuration. Each ready
record describes the last view actually built, not a newly requested logical
state. Current records additionally pair that scene with its committed geometry
as described below. Capture waits for the requested size/appearance/page and
nonempty measured geometry with a 10s bound and
cancels on window closure. Only actual PNG bytes produce capture hashes and the
final evidence manifest. Native window creation is followed by explicit Show;
this also covers launching the console helper with a hidden startup window.

Verified without desktop automation:

- `go test -tags uifixture ./internal/desktop -run '^TestCompactPickerFixture' -count=1`.
  All eight states mount the production popover at 256px inside their client
  bounds. The product Fast/model click callbacks mutate the typed backend;
  returned snapshot edits do not mutate backend-owned state.
- `go test -tags uifixture ./cmd/model-picker-fixture -count=1` rejects stale
  appearance/page/size/open state or any missing geometry in capture readiness.
- `go vet -tags uifixture ./cmd/model-picker-fixture ./internal/desktop`.
- `GOFLAGS=-tags=uifixture go tool mygo vet .`.
- Tagged native command builds with the verified framework stamp.

The first native launch observed 0×0 before a view was built and failed rather
than recording a successful scene. The command was corrected to wait for actual
built-scene readiness. Its stderr is retained separately from successful
capture evidence.

## Committed geometry metadata

The current tagged fixture binds four persistent `ui.Handle` values to the
header, 48px row, actual production model trigger and actual production popup.
The narrow production API adjustment only returns the two elements already
built by `modelTrigger`/`modelPopover`; ordinary product callers ignore those
return values. It adds no alternate control, renderer or product behavior.

At the start of a view build, `Handle.Bounds(c)` reads the committed frame.
Those plain boxes are paired with that frame's plain scene state. A requested
new scene cannot publish the new size/appearance/page alongside old bounds.
Readiness requires two consecutive equal committed geometry samples for the
same scene, because production popup budgeting reads the preceding anchor.
At most eight extra 1ms frames are scheduled; missing geometry produces an
explicit `layoutError` and stops scheduling. Elements and Context are not
retained, drawing callbacks have no fixture side effects, and background
capture work receives only copied state through `Window.Update`.

Evidence schema 2 adds `state.geometry.{header,row,anchor,popup}`, with plain
`x`, `y`, `width`, `height` fields and the explicit space “committed layout boxes
in window client DIPs”, plus `geometryStable` and an optional `layoutError`.
These bounds describe native layout, not painted glyph
ink or an OS-frame rectangle. All eight tester scenes compare these metadata
boxes to the independently labeled rendered controls.
Regressions also resize a list through 560/280/360/560px heights, verify popup
metadata against the final measured panel, reject ready records without stable
geometry, and simulate a missing binding to verify bounded retry termination.
The same committed state records actual `fast` and `contextWindow` option IDs,
read through the production controller's option-resolution functions. Tests
toggle Fast through its real click callback and change the context-window
option through the production mutation path, then verify both the metadata
and typed backend use `fast`/`1m` rather than fixed or display-only values.

At 560px client height the frozen scaffold produces:

| Box | Client x | Client y | Width | Height |
| --- | --- | --- | --- | --- |
| Header | 24 | 446 | client width − 48 | 26 |
| Header title | 32 | 446 | measured text | 26 |
| Trigger row | 24 | 488 | client width − 48 | 48 |
| Production model trigger | 24 | 496 | measured intrinsic width | 32 |

The production trigger's 32px hit area is measured in all eight scenes, and
matches `pickers.rs::trigger_chip`'s `.h(px(32.0))`. An earlier visual estimate of
24px confused visible content with the control's box; the fixture does not
change the trigger's height to match that estimate. Header baseline alignment
and the Pickers root wrapper now explicitly follow the frozen scaffold.

The initial popup's 24px left edge has a separate production positioning
cause. The reference `popover.rs::anchored_menu_above_end` keeps BottomRight
alignment and calls `snap_to_window_with_margin(8px)`. MyGo's `alongTarget` first
tries the opposite alignment to reduce overflow; an end-aligned 256px panel
near the left edge can therefore flip to start alignment at the trigger's
24px x coordinate. This is not a reason to change the source fixture's 24px
padding. The evidence was passed to the production picker owner; that owner's
positioning fix is now reflected in native metadata: panel
`(8,356,256,134)`, model list `(8,264,256,226)` in all eight scenes.

## R3 integrated geometry and WGC evidence

The frozen integrated fixture binary is
`output/fixtures/model-picker-fixture-geometry.exe`. Default desktop tests passed
in 3.833s, tagged command/desktop tests in 0.767s/3.853s. Default/tagged Go vet,
tagged MyGo vet, the framework stamp gate and `git diff --check` passed. The WGC
bridge's real DLL ABI smoke passed before native launch.

On the coordinated native slot, the fixture completed eight WGC scenes and
exited 0 (PID 43396), then completed eight client scenes and exited 0
(PID 38192). No further application was launched. Fresh outputs are:

- `output/captures/model-picker-fixture-wgc-20261028-r3/`: complete native
  window frames, 842/362×592, original PNG bytes with eight transport sidecars.
- `output/captures/model-picker-fixture-client-20261028-r3/`: client content,
  840/360×560, using the default capture path.

All 16 PNG signatures, disk SHA-256 values and physical dimensions were
independently rechecked against their manifests. WGC sidecar byte counts,
hashes, scale 1 and DPI 96 also matched; all sidecars report no crop, resize or
image transformation. The verification is
`output/fixtures/model-picker-fixture-r3-disk-verification.json`. The WGC dark
wide panel and light narrow list were also inspected as actual images.

Every native state records `fast=false`, `contextWindow="standard"`, stable
committed geometry and no layout error. Native header/row boxes match the
table above. The actual native trigger is `(24,496,110.656006,32)`, panel
`(8,356,256,134)`, list `(8,264,256,226)`. These measurements correct the earlier
visual trigger-height estimate and distinguish fixture scaffold geometry from
the production popup placement fix. They establish implementation state and
capture provenance; paired visual acceptance remains pending.

| Integrated phase evidence | SHA-256 |
| --- | --- |
| Fixture binary | `2b4f486585600980c78d6b5787bb38c668c3b40f3229a02b58d9e4cc7aec8ffc` |
| R3 WGC bridge DLL, preserved as `output/native-wgc-bridge/zeron_fixture_wgc-r3.dll` | `f4aa0cf654bb61b9ea2f9b5dfe71ffbe7697ebe3321fdc570d523cdae7b1c460` |
| WGC `evidence.json` | `07fb485395569afeacc1336713fab87c4e505577eabcae96dadc18db6741d90f` |
| Client `evidence.json` | `ff7236991a063bee7f5b7a9e47babd1fff55a4d10f129c5e9ec29340d9a22919` |
| WGC dark wide panel | `f74bb94c094e56477108397875c80438567d02368e0015679c62becb0139d425` |
| WGC light narrow list | `33a4fb21e45fe74cf915e917d74b24e969401b6f2444cf83051364f41f6d9d8d` |
| R3 fixture scaffold | `d599091ab7209a2520c1523f62fb9a857674c8f0dba53365352f4242b4b53548` |
| R3 command main | `5db4a348531b07c62c70b934f68661b08ae43dcc661867894f8da3da78141939` |

`output/fixtures/model-picker-fixture-geometry-source-hashes.json` records the
fixture, command/platform/test files and the narrow production-return change.
It is a scoped source snapshot, not a replacement for the main integration
snapshot or the framework tree lock. The binary retains the verified MyGo
v0.6.4 + `zeron-native-ui.3` stamp described below. Native stdout/stderr are
`output/fixtures/model-picker-fixture-{wgc,client}-r3.*.log`.

After this capture, the bridge owner's independent review identified a
fresh-output edge case: Rust `Path::exists` does not detect an existing dangling
symlink. The owner replaced that check with `symlink_metadata` for R4. R3 used
fresh ordinary output paths and remains an accurate record of its recorded
bridge hash; it must not be relabeled as a capture made by the corrected DLL.

The main integration's complete-frame R3 comparison is
`output/compact-comparison-20261028-r3-all/comparison.json` (SHA-256
`9c83e7dc87ab1b4b374642970bb58313685664b255b2190111893d58ab1653ce`).
All eight scenes were below the 0.995 threshold: SSIM ranged from
0.9716950468 to 0.9913877327. These are unmodified whole-frame image metrics,
not a visual pass. The production owner is correcting the identified open
trigger background and inherited popup font weight; a subsequent source/bridge
phase must have its own native evidence.

## Historical R4 native evidence

After the production owner's open-trigger background, inherited 500 font
weight and star-slot fixes were frozen, the command was rebuilt as
`output/fixtures/model-picker-fixture-geometry-r4.exe`. The fixture scaffold and
main/platform adapter runtime sources are unchanged from R3. The corrected
bridge was independently rebuilt in `output/native-wgc-bridge-r4/` and its real
DLL ABI/zero-HWND/reparse rejection/buffer-canary tests passed before launch.
The framework stamp gate, tagged fixture/command tests, tagged Go vet and
`git diff --check` passed again after the production source change.

One fresh native run captured all eight WGC scenes in
`output/captures/model-picker-fixture-wgc-20261028-r4/` and exited 0 (PID 16080).
The native slot was then released; no further app was launched. All eight
original PNG signatures, disk hashes, dimensions and sidecar hashes/byte counts
were checked again. They are 842/362×592 full-window frames at DPI 96, scale 1,
with no crop or resize. Every state has Fast false, context-window ID standard,
stable geometry and no layout error. The dark wide panel and light narrow list
were inspected as actual WGC images.

R4 native geometry is header/row as above, trigger
`(24,496,111.16,32)`, compact panel `(8,356,256,134)`, model list
`(8,264,256,226)`. The trigger's intrinsic width changed with the corrected
font weight; its position and 32px hit height did not change.

| R4 evidence | SHA-256 |
| --- | --- |
| `output/fixtures/model-picker-fixture-geometry-r4.exe` | `9d8855f93a1a2a8b9f1d040a10279cfe3c9b5d0f32e384fcef8e06180b031563` |
| Corrected bridge DLL | `6c10000695b6e54163bd3c89e311a98461d2c5c08300c63c0b9b979e477805b1` |
| R4 capture `evidence.json` | `0b6d58c9e38e5fa64a93e4baff2600f348eb5f083de3afb393f709259106923a` |
| `output/fixtures/model-picker-fixture-r4-build.json` | `549b2539334901e7fab758d484f65e5141155be5baefc8c5ce1332f12faeec7b` |
| `output/fixtures/model-picker-fixture-r4-disk-verification.json` | `212e2020555aaca51a82c45372348496746b8b4c807bf6ca1d1073cef9ee09f7` |
| R4 WGC dark wide panel | `e5ca0a002d2f6ee51ee5ea611f83abce01a7fe6bd8857c3fc8b0fc3fdb3fa58f` |
| R4 WGC light narrow list | `4271ca12c2b4cee4ced17a838f3fd68f3d29f9d5ee687e3f05222e52967c98ea` |

The build record pairs the actual executable and DLL hashes with seven direct
runtime source hashes, including the production trigger/popover, typed data and
fixture command. It supplements the main task's full integration snapshot.
Bridge source/ABI evidence is in `reference/validation/native-wgc-bridge.json`
and `native-wgc-bridge-loader.json`. R3 images, binaries, logs and metrics remain
historical evidence; R4 must be compared independently. Successful R4 transport
and geometry checks are not a visual acceptance claim.

## Historical R5 native evidence

The main integration added `design.ApplyReferenceTypography` to App.View and
this fixture's View. On Windows it uses the existing
`c.Root().FontFeatures("kern=0")` API to reproduce the frozen GPUI typography
defaults. It changes neither font files nor the vendor tree; MyGo remains
v0.6.4 + `zeron-native-ui.3`. The other fixture/command/platform runtime sources
remain frozen. This phase is separate from the R4 pixel captures.

After the main task's final product/tagged tests and vet gates passed, the
framework stamp gate and fixture build were run again using the final helper
source. One fresh run captured eight WGC scenes in
`output/captures/model-picker-fixture-wgc-20261028-r5/` and exited 0 (PID 36116).
The native slot was released immediately. All eight disk PNG signatures,
hashes, sizes and sidecar hash/byte counts were rechecked: complete
842/362×592 frames, DPI 96, scale 1, no crop/resize, Fast false, context-window
ID standard, stable geometry and no layout error. The WGC dark wide panel and
light narrow list were inspected as actual images.

The R5 native trigger is `(24,496,112.108,32)`; header/row positions, compact
panel `(8,356,256,134)` and model list `(8,264,256,226)` remain unchanged.
Intrinsic text width changed with the reference typography configuration;
fixture scaffold dimensions were not adjusted to fit images.

| Historical R5 evidence | SHA-256 |
| --- | --- |
| `output/fixtures/model-picker-fixture-geometry-r5.exe` | `3f4673e8b563b95c0f788428482c6262dba96b0a2771cc6d8a1664f90095462d` |
| R4 bridge, reused unchanged | `6c10000695b6e54163bd3c89e311a98461d2c5c08300c63c0b9b979e477805b1` |
| R5 capture `evidence.json` | `68f2863146102e7de4ccf08efae62390adb43a1b5bcfc8d6a666dca38334c5bf` |
| `output/fixtures/model-picker-fixture-r5-build.json` | `713b54b07803262c53088a10c35eb0a33b68930ccca5420e75bf8f480f743233` |
| `output/fixtures/model-picker-fixture-r5-disk-verification.json` | `bfa6a3dd881dcb74fbfc82e9d76962d082175e4438e82b29811ec5afd26b73cc` |
| R5 WGC dark wide panel | `bd98a1a5b9506ddfbee4ec3053ff14bdbea07fc983a6e1ccbb27d129c2c56f31` |
| R5 WGC light narrow list | `be0c9980d523fdf06a432ae20824137bc9076be1b32f4f749cf8c4513762007b` |
| Final `internal/design/typography.go` | `72dea249f1964e922576166e404925a640f98bd1b4250eb39baabc017db5fa96` |

The build record now includes eight direct runtime sources, adding the helper
to R4's seven-file scope. A candidate built before the helper's comment was
corrected is preserved as `model-picker-fixture-geometry-r5-pre-comment.exe`
and `model-picker-fixture-r5-pre-comment-build.json`. The final explicit rebuild
produced the same executable bytes because the correction affected comments;
the final build record has the new helper source hash and its own new hash.
No candidate capture was relabeled as final evidence. Paired R5 comparison is
owned by the main integration; successful transport and geometry checks alone
do not establish visual acceptance.

## R6 option text bounds

The later R6 fixture reuses production option text with source-confirmed
intrinsic ceil and snapped phi line height, scoped to the measured Windows1x
integer-origin domain. Actual original Inspector bounds and tests are recorded
in `reference/validation/text-bounds-implementation-r6.json`.
The eight complete PNG pairs in
`output/compact-comparison-20261028-r6/comparison.json` have three scores above
0.995 and five below. No scene is visually accepted. The four model-list
residuals remain; this option change does not cover arbitrary fractional scroll,
overhang,2x or macOS. Original images and prior phase reports were retained.

## R7 actual display binding and Windows2x

The native fixture now binds its real Window without starting App.Attach
subscriptions. The previous nil-Window fixture used the deterministic1x
fallback; that did not establish the production2x text branch. R7 records
nativeWindowAttached=true and oneXOptionText=false at actual2x/DPI192.
The separately captured frozen original matrix is
`output/reference-lossless/capture-native-state-20261028-2x-01`; implementation
matrix is `output/captures/model-picker-fixture-wgc-20261028-r7-2x`.
Complete WGC sizes are1684×1180 wide and724×1180 narrow. All manifests,
sidecar hashes, real dimensions and separately observed native frame profiles
were checked. No crop, resize, registration, mask or OS keyboard/mouse input.

The strict R7 report is
`output/compact-comparison-20261028-r7-2x/comparison.json`:2/8 scores reach0.995,
range0.986806768–0.996676105, zero visually accepted scenes. The numerical count
cannot be compared directly against the earlier1x matrix as a controlled
improvement. Search/list source style and native popup geometry changes are
described in `audit-native-popup-style.md`; source/package/fixture identities
are in `reference/validation/popup-r7-result.json`. The fixture executable is
distinct from the newly rebuilt packages and from all older observed binaries.

## Historical native capture evidence before geometry metadata

The earlier corrected command completed all eight native scenes and exited with
code 0. These files precede the geometry metadata and subsequent production
picker visual changes; their source and binary hashes are phase-specific.
The successful output is
`output/captures/model-picker-fixture-native-20261028-r2/`. Its `evidence.json`
contains each PNG's hash, physical dimensions, built scene state and framework
stamp. All eight recorded PNG hashes were independently rechecked from disk.
Wide images are 840×560 and narrow images 360×560, matching their logical client
dimensions on this run. The dark wide panel and light narrow model list were
also inspected as images to confirm the production controls were present.

| Evidence | SHA-256 |
| --- | --- |
| `output/fixtures/model-picker-fixture.exe` | `580cb67aa93e5266bcdb928f3568770173b6423dbf1c2f72ffb45c3aed132778` |
| Native capture `evidence.json` | `78dac4f827d9c358317eb029bd1fcb4ac8725a7eef8c3b5ec35192706af8e83e` |
| `compact-panel-dark-wide.png` | `46097e4418617bcd10a6d92bbdcffceb62376c509b545719b7b5a925dc402bda` |
| `compact-model-list-light-narrow.png` | `cf9e2513aac8062f7bba57a5ae15472585b78edf8a9f6f0348fb7ec2d5ff525a` |
| `internal/desktop/model_picker_fixture.go` | `2d86db1b24e2078de9f9040ecb5d2a0020fd0f15b6dcc66e71dae423937e76a5` |
| `cmd/model-picker-fixture/main.go` | `a95f012868525ba2f784b9bdc428e5f4e7ded2a88a3ab1a5954dc629a27354c7` |

The binary reports MyGo v0.6.4 + `zeron-native-ui.3`, source tree
`c6a86278ae261c75b6b195632d97d4d64218ed69189ec45554b85c5826b4d2a6`.
Successful stdout/stderr are `output/fixtures/model-picker-fixture-native-r2.*.log`;
the first unsuccessful launch remains in the similarly named logs without `r2`.
No keyboard/mouse automation or external OS screenshot helper was used.

## Capture boundary

The client path uses the fixture window's existing `Window.CapturePage()` API;
it renders native MyGo client content and excludes the OS frame. The WGC path
captures the complete actual native window, through an isolated bridge that
targets only the supplied caller-owned HWND. Neither path's successful capture
establishes pixel equivalence to GPUI, keyboard acceptance or cross-platform
acceptance. Reference and implementation evidence remain separate for the
paired comparison.
