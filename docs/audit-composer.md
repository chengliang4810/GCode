# Composer implementation and handoff

> 本文保留原审计阶段的框架版本与验收范围；当前框架及补丁以 [mygo-patches.md](mygo-patches.md) 和源码锁为准。

2026-10-30 当前状态：权限入口、PR 入口和配置、项目动作、本地账户菜单及模型收藏已移除。
模型列表顶部及模型行星标已替换为模型设置入口。
正文保留当时的源码审计与验收证据；当前实施范围见
[UI 残留清理](ui-residual-cleanup.md)，不再把这些旧入口列为待实现功能。

Reference: Zeron v0.2.128, commit
`ab189159f10cf8e63c0502be75a59bbc487e0eaf`, read-only `D:/zeron`.
The resumed-machine source is at `output/reference-sources/zeron`; `D:/zeron`
was the original location. This file records shared controls and the current
settled-layout stage, with earlier handoff evidence retained separately below.
Detailed current source hashes, implementation checks and pending acceptance
are in [audit-composer-layout.md](audit-composer-layout.md). No compilation,
headless render or self-test establishes pixel parity or overall completion.

## Stable implementation

`composer_surface.go` is the one native input/action surface used by the main
conversation and side tabs. It binds `ui.TextAreaBase` and persistent handles to
the owner's state. `composerControls` contains only model/reasoning and
popup/list/widget state; the old access-mode and Plan state has been removed.
`composerController` references the root App and a
captured target session; it does not copy an App, create a service, start a
worker, own a settings transaction, or subscribe to events.

Main and side composers use the same model picker, compact effort panel,
full model list, provider filtering and model option rows.
Side selections update and send to that side session. Model favorites were
removed on 2026-10-30; provider memory and model options remain shared
preferences, written through the root's existing `saveSettings` path. Side session configuration completion is
correlated through the root `sideCommandFinished` hook; failures retain side
draft/error ownership and restore the authoritative session configuration.

On 2026-10-29 the access-mode button to the left of the model picker was
removed from both composers at the user's request, together with its popover,
Plan toggle and dedicated widget state. Session access/Plan state still flows
through the existing configuration and send paths. Unsupported access modes
still resolve using the lower-or-equal offered mode rule.

`workbench.State.OnCloseSession` calls `dismissSideComposer` only when closing a
session tab. It closes that composer's model popover and pending focus
request, while preserving its draft. The hook is installed in both App.New and
Attach. During development an earlier workbench hook placement in `selectTab`
incorrectly dismissed pickers on tab selection; the workbench owner corrected it
and added its own lifecycle regression.

After machine resume, the shared surface also gained compact/expanded layout,
route-dependent settled geometry, native auto-grow, resize settling and
preedit-aware capacity measurement. The same keyed `TextAreaBase` and persistent
handle survive layout changes. A small read-only MyGo `InputDisplayText()` API
supplies preedit snapshots without committing the draft; native input owns
wrapping, selection, undo and scrolling. The current source lock is MyGo v0.7.4
with `rcode-native-ui.23` (2,050 files, 19 patches, tree
`c50af74375c6b2cbb2b8a50a09c407469dd419b29788d5774bed1e8c99deda50`); see
[mygo-patches.md](mygo-patches.md).

## Historical shared-controls checks at handoff

These results describe the shared-controls stage before the resumed layout
changes. Current layout/input checks and their final runs are recorded in
[audit-composer-layout.md](audit-composer-layout.md).

| Check | Evidence | Result |
|---|---|---|
| Side model/reasoning/access/Plan stay independent from the central composer | `TestSharedComposerSideModelAccessAndPlanStayIndependent` | Passed |
| The side typed send carries its selected configuration | Same test checks captured SendRequest and backend session | Passed |
| Failed side configuration update restores its own snapshot and retains local error ownership | `TestSharedComposerSideConfigFailureRestoresItsSnapshot` | Passed |
| A running side session locks its own provider without locking an idle main session | `TestSharedComposerRunningSideLocksItsProviderOnly` | Passed |
| Existing model picker search, keyboard, favorites, provider memory and bounds behavior | Existing model_picker tests | Passed in desktop suite |
| Side IME, shortcut modes, send/stop/retry, queue and draft ownership | transcript_side tests | Passed |
| Main file attachment remove and side file attachment remove | Both focused tests repeated 5 times | Passed |
| Entire desktop package | `go test ./internal/desktop -count=1` | Passed, handoff run 4.982s |
| Go vet | `go vet ./internal/desktop` | Passed |
| MyGo lifetime/thread vet | `go tool mygo vet .` | Passed |
| Desktop lint | `output/tools/golangci-lint.exe run ./internal/desktop/...` | 0 issues on final local run |

The historical access test clicked the native access-popover row instead of
using `Tester.ChooseMenuItem`: the source surface is an application popover,
not the default context-menu presentation. Plan stays open, so tests dismiss it
with Escape before opening another control. That menu and its UI tests were
removed on 2026-10-29; model-selection tests now check preserved run state and
configuration failure restoration through the model picker.

The handoff freeze interrupted the next layout stage. The unfinished layout
experiment was removed and the known working shared native surface restored;
that historical baseline used an always-expanded 116px minimum, 22px leading
and fixed 22px radius. The resumed implementation below supersedes those values
with a connected native layout; the earlier captures remain evidence of their
own stage.

## Current source alignment and remaining work

Stable geometry and decision behavior are now connected and covered by native
widget geometry/input tests. Full motion/material integration and independent
paired native/reference visual acceptance remain pending. The reference has
different expanded floors: hero 120px total/76px textarea, settled dock 104px
total/60px textarea; both cap at 304px total/260px textarea.

| Source fact | Source location | Current implementation or remaining difference |
|---|---|---|
| Compact border-box height 49px | composer.rs:79 | Connected; rendered geometry regression passes |
| Hero expanded height 120–304px; settled dock 104–304px | composer.rs:58–76,178,11559–11572; composer_dock.rs:105 | Both route-specific floors and shared cap are connected; side chats explicitly use settled dock |
| Input font 14px, line height 22.75px | composer.rs:108–110 | Authored metrics connected with the selected interface font scale |
| Expanded textarea hero 76–260px / settled dock 60–260px, including 20px vertical inset | composer.rs:56–66,11559–11572 | Native auto-grow uses Lines(1,100), box min/max and exact leading; font12/13/14/15 cap regressions pass |
| Expanded action row 42px: 2px top + 32px chips + 8px bottom | composer.rs:63–70 | Connected, with a fixed actions slot |
| Hero radius 26px; settled dock radius 22px | composer.rs:74,11679 | Settled route-dependent radii connected |
| Hero outer cap 768px; dock outer cap min(panel, transcriptWidth+32) | composer.rs:84; shell.rs:300; shell/side_chats.rs:406–431 | Main/side caps and 16px horizontal container insets connected |
| Compact input capacity below 200px always expands | `composer_flip`:138 | Connected and boundary-tested |
| Newline or text wider than capacity expands; collapse has 32px hysteresis | composer.rs:138–158 | Connected, including uncommitted preedit; actual compact viewport thresholds tested |
| Resize settling delays collapse 150ms, expansion stays immediate | composer.rs:11336–11395 | Connected with a frame timer; 149/150ms boundary regression passes |
| Input should remain the same native widget through mode switches | Source retained ComposerInput entity | Keyed native input retained; focus/IME/commit/cancel/undo regressions pass |
| Attach target 28px, paperclip 18px; Send target 28px, arrow 14px; Stop square 11px/radius3 | composer.rs:11725+,11138+ | Connected; native target bounds/alignment tested, paired visual comparison pending |
| Compact inset 8px; expanded inset 12px | `morph_cluster_inset`:417 | Settled insets connected; interpolation motion remains pending |
| Project/device selectors are chip-only at top-right | pickers.rs:4230 | Top-right 20px row and source insets connected; exact chip/popover visual comparison pending |
| New-thread ref/checkout selectors use chip-only popovers | pickers.rs:4321 | Existing footer still uses default Select controls |
| Existing-session checkout/ref labels are read-only, leading-aligned and Git-only | pickers.rs:4396 | Existing footer still editable/default Select |
| Composer owns a 24px footer slot; a side chat can conditionally show context usage | composer.rs:6930,12170+; account_usage.rs:430–470 | Main/side 24px footprint connected; side slot is currently empty and its conditional context ring is not implemented |
| An additional independent shell status strip is also 24px | shell.rs:11826,12239 | Keep it; do not remove it as a presumed duplicate |
| Account usage rings/PR badge belong to the composer footer | composer.rs:12205+ | Controls/rings remain pending; actual shell keeps a 24px hero slot even without Git, with conditional content. See audit-composer-footer.md |
| Provider installation/enablement is target/device scoped | Snapshot.Integrations.Providers | Resolved after machine resume: target Installed/Enabled filtering and dedicated regressions; see audit-composer-device.md |
| Full compact↔expanded/dock movement and material treatment | composer.rs:11400–12170 | Settled geometry is connected; exact motion, route choreography, frost and overflow fades remain pending |

The placeholder was corrected to the source Unicode ellipsis: `Do anything…`.
The parent moved working output to a source-derived transcript trailer and kept
the separate shell status strip. The side trailer and its pending-send bridge
were subsequently connected on the resumed machine; see `machine-resume.md`.
Their paired visual acceptance remains pending.

## Migration materials and reproduction

Source dependencies are Go 1.27.3 and MyGo v0.7.4 with the existing local
`replace ./third_party/mygo`. The current lock is `rcode-native-ui.23`
(2,050 files, 19 patches). The historical shared-controls handoff stage did not
change the then-current lock; the resumed layout stage introduced the read-only
input-snapshot patch, now patch 04 in the v0.7.4 chain. The native Collapsible
Panel and fixed Rust reference are unchanged by this layout work.

Move the complete repository, including the existing `.agents`, embedded assets,
reference provenance and third-party source/patch metadata. Keep the current
`docs/audit-transcript.md`, this file and the other owner audit documents with it.

At handoff, `output/captures/transcript-audit/` was re-exported with the stable
shared surface and full side controls: 16 **implementation-only headless MyGo**
PNGs. `desktop-side-input-light.png` and its dark counterpart show that earlier
stage, not the final resumed layout. Preserve their SHA-256 `evidence.json` with
the images when moving ignored output. Keep original GPUI reference images
separate; implementation renders are not goldens and prove no OS material or
same-platform visual parity.

Reproduce current headless evidence without desktop interaction in a fresh
directory, keeping the handoff captures separate:

```powershell
$env:RCODE_TRANSCRIPT_EVIDENCE = Join-Path (Get-Location) 'output/captures/transcript-layout-resume'
go test ./internal/desktop -run '^TestTranscriptRenderEvidence$' -count=1
Remove-Item Env:RCODE_TRANSCRIPT_EVIDENCE
```

No application window was launched and no Computer Use action was performed
during the migration freeze or by the layout owner. Root owns the resumed
machine's native captures and final build; those captures are still being
collected and do not yet establish paired visual acceptance. This owner did not
commit or push; root owns the combined review, staging and transfer.
