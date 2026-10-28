# Composer implementation and handoff

Reference: Zeron v0.2.128, commit
`ab189159f10cf8e63c0502be75a59bbc487e0eaf`, read-only `D:/zeron`.
This file records the stable shared-controls stage. It does **not** establish
visual parity or completion of the overall replica.

## Stable implementation

`composer_surface.go` is the one native input/action surface used by the main
conversation and side tabs. It binds `ui.TextAreaBase` and persistent handles to
the owner's state. `composerControls` contains only model/reasoning/access/Plan,
popup and list/widget state. `composerController` references the root App and a
captured target session; it does not copy an App, create a service, start a
worker, own a settings transaction, or subscribe to events.

Main and side composers now use the same model picker, compact effort panel,
full model list, provider/favorite filtering, model option rows and access/Plan
popover. Side selections update and send to that side session. Favorites,
provider memory and model options remain shared preferences, written through the
root's existing `saveSettings` path. Side session configuration completion is
correlated through the root `sideCommandFinished` hook; failures retain side
draft/error ownership and restore the authoritative session configuration.

The access popover follows `pickers.rs::render_access_popover`: 288px panel,
mode descriptions, selected check, the one-mode explanation and a Plan toggle
that remains open after activation. Selecting an access mode closes it. The
passive switch uses the source 44.8×28.8 geometry while native `ToggleBase` owns
activation and accessibility. Unsupported access modes now resolve using the
source's lower-or-equal offered mode rule, rather than always choosing the last
offered mode.

`workbench.State.OnCloseSession` calls `dismissSideComposer` only when closing a
session tab. It closes that composer's model/access popovers and pending focus
request, while preserving its draft. The hook is installed in both App.New and
Attach. During development an earlier workbench hook placement in `selectTab`
incorrectly dismissed pickers on tab selection; the workbench owner corrected it
and added its own lifecycle regression.

## Verified checks at this stage

| Check | Evidence | Result |
|---|---|---|
| Side model/reasoning/access/Plan stay independent from the central composer | `TestSharedComposerSideModelAccessAndPlanStayIndependent` | Passed |
| The side typed send carries its selected configuration | Same test checks captured SendRequest and backend session | Passed |
| Failed side configuration update restores its own snapshot and retains local error ownership | `TestSharedComposerSideConfigFailureRestoresItsSnapshot` | Passed |
| A running side session locks its own provider without locking an idle main session | `TestSharedComposerRunningSideLocksItsProviderOnly` | Passed |
| Existing model picker search, keyboard, favorites, provider memory and bounds behavior | Existing model_picker tests | Passed in desktop suite |
| Side IME, shortcut modes, send/stop/retry, queue and draft ownership | transcript_side tests | Passed |
| Main file attachment remove and side file attachment remove | Both focused tests repeated 5 times | Passed |
| Entire desktop package | `go test ./internal/desktop -count=1` | Passed, latest run 4.982s |
| Go vet | `go vet ./internal/desktop` | Passed |
| MyGo lifetime/thread vet | `go tool mygo vet .` | Passed |
| Desktop lint | `output/tools/golangci-lint.exe run ./internal/desktop/...` | 0 issues on final local run |

The existing access test now clicks the native access-popover row instead of
using `Tester.ChooseMenuItem`: the source surface is an application popover,
not the default context-menu presentation. Plan stays open, so tests dismiss it
with Escape before opening another control.

The source freeze interrupted the next layout stage. The unfinished layout
experiment was removed and the known working shared native surface restored;
there is no half-connected width/height calculator in the migration source.

## Exact source findings for the next machine

The current shared surface still uses an expanded 116px minimum, 22px text line
height and 22px radius. The following source geometry and behavior remain to be
integrated and visually checked:

| Source fact | Source location | Current difference |
|---|---|---|
| Compact border-box height 49px | composer.rs:79 | Current input always expanded |
| Expanded height 120–304px | composer.rs:58–76, `composer_total_height`:178 | Current minimum 116px |
| Input font 14px, line height 22.75px | composer.rs:108–110 | Current line height 22px |
| Expanded textarea box 76–260px including 20px vertical inset | composer.rs:56–66 | Current native Lines(2,8) auto-size uses different bounds |
| Expanded action row 42px: 2px top + 32px chips + 8px bottom | composer.rs:63–70 | Current shared row still uses prior spacing |
| Hero radius 26px; settled dock radius 22px | composer.rs:74,11679 | Radius is not yet route-dependent |
| Hero outer cap 768px; dock outer cap min(panel, transcriptWidth+32) | composer.rs:84; shell.rs:300 | Existing wrappers retain earlier padding/cap logic |
| Compact input capacity below 200px always expands | `composer_flip`:138 | Not implemented |
| Newline or text wider than capacity expands; collapse has 32px hysteresis | composer.rs:138–158 | Not implemented |
| Resize settling delays collapse 150ms, expansion stays immediate | composer.rs:11336–11395 | Not implemented |
| Input should remain the same native widget through mode switches | Source retained ComposerInput entity | Preserve input handle/widget identity when implementing modes |
| Attach target 28px, paperclip 18px; Send target 28px, arrow 14px; Stop square 11px/radius3 | composer.rs:11725+,11138+ | Current shared utility drawing needs final source sizing pass |
| Compact inset 8px; expanded inset 12px | `morph_cluster_inset`:417 | Not route/mode dependent yet |
| Project/device selectors are chip-only at top-right | pickers.rs:4230 | Existing wrappers retain earlier placement |
| New-thread ref/checkout selectors use chip-only popovers | pickers.rs:4321 | Existing footer still uses default Select controls |
| Existing-session checkout/ref labels are read-only, leading-aligned and Git-only | pickers.rs:4396 | Existing footer still editable/default Select |
| Composer owns a 24px footer slot, including an empty slot for side chats | composer.rs:12170+ | Needs integration |
| An additional independent shell status strip is also 24px | shell.rs:11826,12239 | Keep it; do not remove it as a presumed duplicate |
| Account usage rings/PR badge belong to the composer footer | composer.rs:12205+ | Not implemented in this stage |
| Provider installation/enablement is target/device scoped | Snapshot.Integrations.Providers | Current picker still primarily consumes global Providers; remote filtering needs a dedicated regression |
| Full compact↔expanded/dock movement and material treatment | composer.rs:11400–12170 | No exact motion/material parity is claimed |

The placeholder was corrected to the source Unicode ellipsis: `Do anything…`.
The parent moved working output to a source-derived transcript trailer and kept
the separate shell status strip. `workingTrailer(c,sessionID)` still needs to be
connected to the side transcript after the last message; no side sending bridge
was added in this stage.

## Migration materials and reproduction

Source dependencies remain Go 1.27.3 and MyGo v0.6.4 with the existing local
`replace ./third_party/mygo`. This stage did not change the dependency lock,
third-party patch tree, native Collapsible Panel or Rust reference.

Move the complete repository, including the existing `.agents`, embedded assets,
reference provenance and third-party source/patch metadata. Keep the current
`docs/audit-transcript.md`, this file and the other owner audit documents with it.

`output/captures/transcript-audit/` was re-exported with the stable shared surface
and full side controls. It contains 16 **implementation-only headless MyGo**
PNGs. In particular `desktop-side-input-light.png` and its dark counterpart now
show the shared side controls. Refresh/copy its SHA-256 `evidence.json` with the
images if moving ignored output. The original GPUI reference images must remain
separate; these implementation renders are not goldens and prove no OS material
or same-platform visual parity.

Reproduce headless evidence without desktop interaction:

```powershell
$env:ZERON_TRANSCRIPT_EVIDENCE = Join-Path (Get-Location) 'output/captures/transcript-audit'
go test ./internal/desktop -run '^TestTranscriptRenderEvidence$' -count=1
Remove-Item Env:ZERON_TRANSCRIPT_EVIDENCE
```

No application window was launched and no Computer Use action was performed
during the migration freeze. This owner did not commit or push; root owns the
combined review, staging and transfer.
