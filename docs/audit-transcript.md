# Native agent transcript audit

Baseline: Zeron **v0.2.128**, commit
`ab189159f10cf8e63c0502be75a59bbc487e0eaf`.
Scope: the assistant output area, activity grouping, individual work rows,
reasoning, final Markdown and agent transcript links. Composer and model-picker
changes belong to their separate audits.

“Tested” below means the stated implementation check passed. It does **not** mean
the implementation passed a same-platform GPUI visual comparison. No new GPUI
transcript golden was captured for this change.

## Sources inspected before implementation

- `D:/zeron/crates/ui/src/transcript.rs`: `rows_for_entry` (1363),
  `tool_group_summary` (2166), `format_elapsed` (2477),
  `activity_branch_points` (2881), `render_tool_group` (8683),
  `tool_icon_path` (10275), `detail_body` (10313),
  `chip_header_row` (10567), `activity_rail` (10952),
  `activity_ribbon` (11030) and `subagent_chip` (11117).
- `D:/zeron/crates/proto/src/view.rs`: `tool_chip_content` and
  `tool_group_summary` (770). Counter rules are implemented once in
  `internal/contracts/activity.go`, shared with the backend.
- Official `D:/projects/mygo/examples/markdown-chat/main.go`: stable keyed
  `ui.List`, following the end, settled `markdown.View` and live
  `markdown.Stream`.
- Official `D:/projects/mygo/examples/gallery/main.go`: List, Tree and
  Collapsible examples. Its Timeline example is an animation playhead; it is
  unrelated to this agent activity view and was not reused.
- The pinned `third_party/mygo/ui/collapsible.go`, `textselection.go`,
  `markdown/parser`, `richtext.go` and `editor/diffview.go` APIs.

`ui.CollapsibleBase` owns toggling, keyboard focus and expanded accessibility
state. List and Markdown use the official implementations. Reasoning is flattened
from the official Markdown parser into official `ui.RichText` lines. The only
application-specific drawing is the source connector ribbon and small chevrons.
`ui.TreeItem` was considered: its file-tree padding, selection and disclosure
semantics do not match work rows, which can open their own output independently.

The user's third screenshot is supplemental visual evidence at
`C:/Users/chengliang/AppData/Local/Temp/codex-clipboard-75315edb-07aa-4e76-b0a7-fc8cbc4c1842.png`.
It shows a compact interleaved work tree. It also shows a Thought preview that
the pinned source does not produce: `chip_header_row` explicitly returns
`("Thought process", String::new())` for a Thought; `note_chip_detail` is used
only by a Wrote/narration row. This implementation follows the pinned source:
reasoning remains available in the expandable body. A screenshot alone cannot
establish which conditional/version generated that different preview.

## Small-function checklist

| ID | Check | Implementation evidence | Status |
|---|---|---|---|
| TR-01 | Join adjacent assistant continuations within one turn; preserve first ID | `TestTranscriptContinuationJoinsOnlyAdjacentTurn`, including three-part explicit continuation chains and DTO ownership | Tested |
| TR-02 | Preserve message/block order without sorting by timestamps | `TestTranscriptRowsPreserveActivityAndNarrationOrder` | Tested |
| TR-03 | Consecutive ordinary tools and reasoning share one group | Normal complete/live projection cases | Tested |
| TR-04 | Narration remains a separate Markdown row in normal mode | Normal projection cases | Tested |
| TR-05 | Compact mode folds narration into Wrote rows and keeps trailing reply outside | `TestTranscriptCompactNarrationAndWorkedDuration`, compact projection cases | Tested |
| TR-06 | Compact mode has no visible final reply while still streaming | Compact streaming projection case | Tested |
| TR-07 | Requests and generated images retain document placement around compact work | `TestTranscriptCompactKeepsRequestsAndFinalImagesInDocumentOrder` | Tested |
| TR-08 | Calls and later result blocks update one stable activity row | `TestTranscriptLiveTailAndCallResultIdentity` | Tested |
| TR-09 | Summary derives actual commands, paths and failure metadata | Shared `contracts.SummarizeActivity` and backend counter tests; render tests use its result | Tested through typed contract |
| TR-10 | Group header is 26 logical pixels | `TestTranscriptNativeDisclosureAndFixedTreeGeometry` | Tested |
| TR-11 | Work rows are 32 logical pixels, with no accumulating row gap | Same test asserts adjacent row coordinates | Tested |
| TR-12 | Text starts at gutter 48 + gap 8, not a filled card inset | Same test asserts measured 56px offset | Tested |
| TR-13 | Connector uses x=12.5, 6px elbow, 28px branch and 24-step source curve | Literal source port; rail seam pixel regression | Tested in implementation render |
| TR-14 | Connector intersections and row seams receive one alpha contribution | `TestTranscriptTreeRailHasNoDoubleAlphaSeams` samples header, pad, bend and row seams | Tested |
| TR-15 | Tools use original Solar icons and file badges use original Symbols file icons | `activityTitle`, `workbench.FileIcon`, inspected light/dark PNGs | Implementation inspected |
| TR-16 | Read/Write/Edit/Patch show basename, retain full path in tooltip/invocation | `TestActivityTypedLabelsAndPaths`; inspected PNGs | Tested |
| TR-17 | A resolvable file badge opens the native editor and does not toggle its row | `TestTranscriptFileBadgeOpensNativeEditorWithoutTogglingRow` | Tested |
| TR-18 | Failed tools tint their own label/detail/icon, while group summary stays quiet | All 30 palettes have danger-foreground pixel assertions; dark/light implementation PNGs | Tested and inspected |
| TR-19 | Complete groups start collapsed | `TestTranscriptNativeDisclosureAndFixedTreeGeometry` | Tested |
| TR-20 | Only a streaming tail group automatically opens | Live projection/native tests | Tested |
| TR-21 | Only a streaming tail Thought automatically opens its detail; compact mode suppresses defaults | `TestTranscriptExplicitFoldSurvivesStreamingAndVirtualRecreation`; source condition in renderer | Tested, including `TestTranscriptCompactLiveNeverAutoOpensThought` |
| TR-22 | Explicit fold choice survives deltas, completion and virtual row recreation | Same native regression | Tested |
| TR-23 | Enter and Space operate the focused native disclosure | `TestTranscriptNativeDisclosureAndFixedTreeGeometry` | Tested |
| TR-24 | Expanded reasoning contains styled text, not Markdown source markers | `TestThoughtFlattenUsesSourceRecordLayout`, native detail test | Tested |
| TR-25 | Thought headings, inline styles, code, bullets, quotes and tables use a fixed record layout | Official parser + native RichText; flatten regression | Tested |
| TR-26 | Thought wraps at 96 characters; max 24 lines; live keeps tail, settled keeps head | Flatten/model regressions | Tested in implementation |
| TR-27 | Tool command invocation wraps at 80 characters; output has 24-line cap and counted tail | `activityInvocation`, `activityLines` regression | Tested |
| TR-28 | Expanded output uses 18px line boxes, 6px top/bottom padding and faint mono text | Source metrics port; inspected detail PNG | Implementation inspected |
| TR-29 | Native drag selection copies across output lines | `TestTranscriptOutputSelectionSpansNativeLines` | Tested |
| TR-30 | Final reply renders separately as native Markdown | Normal/compact native tests and full desktop PNGs | Tested and inspected |
| TR-31 | Completed footer copies reply prose without reasoning/tool output; no unconditional Retry | `TestTranscriptCopyContainsReplyOnlyAndNoDefaultRetry` | Tested |
| TR-32 | Timestamp uses local time and source format; hover actions reserve a 16px lane | Source formatting port, full desktop PNGs | Implemented; native hover comparison pending |
| TR-33 | Subagent is a 30px native link card, rather than another output accordion | Source style port; native side-tab regression | Tested |
| TR-34 | Agent link opens its actual transcript in a right tab without losing central draft/session | `TestTranscriptSubagentOpensSideTabAndPreservesCentralDraft` | Tested |
| TR-35 | Per-pane list/fold identity stays independent | `TestTranscriptSideFoldDoesNotChangeCentralFold`; right pane prefixes stable view IDs and owns a native ListState | Tested |
| TR-36 | 100%, 125%, 150%, 200% device scale retain logical geometry and operable disclosures | `TestTranscriptLogicalGeometryAtDesktopScales` | Tested in Tester; Windows monitor/DPI transitions pending |
| TR-37 | All 30 original theme variants retain bounds and failed-row tint | `TestTranscriptAllThemesKeepGeometryAndFailureTint` | Tested |
| TR-38 | Existing question/permission operations still dispatch backend answers | Existing `TestQuestionAndPermissionAnswers` | Tested |
| TR-39 | No persistent Element/Context, view-launched worker, or alternate fixture reader | MyGo vet and source inspection | Tested by vet and inspected |
| TR-40 | Source 140ms folds, 360ms row arrivals, 480ms connectors and 3400ms active-summary shimmer | Current native Collapsible uses its official 200ms animation; exact source timings require a future framework-supported extension. Per the current scope decision, the dependency/Panel were preserved; arrival/connector/shimmer remain pending | Explicit difference retained |
| TR-41 | Inline diff matches original checkout-diff body, dual line gutters and syntax runs | Uses official native `editor.DiffView`; a fresh GPUI inline-diff comparison has not been made | Pending visual comparison |
| TR-42 | Side transcript provides the full original side-conversation input/navigation surface | Actual child messages, independent input, send/stop, attachments and queue editing work in a native right tab. Full model/access/plan controls remain absent rather than replaced by alternate navigation | Functional input loop tested; full composer controls pending |
| TR-43 | Original GPUI and replica pixels agree for all row/hover/selection states | No fresh matching reference screenshots exist for this change | Pending |
| TR-44 | Long history, long output and selection remain within performance gates | Official outer virtualized List retained; this audit does not supply P95/native stress measurements | Pending performance acceptance |

## Captured implementation evidence

Regenerate without opening a native application window:

```powershell
$env:ZERON_TRANSCRIPT_EVIDENCE = Join-Path (Get-Location) 'output/captures/transcript-audit'
go test ./internal/desktop -run '^TestTranscriptRenderEvidence$' -count=1
Remove-Item Env:ZERON_TRANSCRIPT_EVIDENCE
```

The ignored `output/captures/transcript-audit/` directory contains:

- Ten focused 760×900 renders: collapsed, expanded, detail-open, compact and
  live-tail, each in light and dark.
- Six full 1320×880 desktop renders: collapsed/expanded and side input, each light/dark.
- `evidence.json` records per-file SHA-256 hashes, dimensions and provenance.

The expanded light tree, dark expanded details and full light desktop were
visually inspected. These captures demonstrate this implementation's native
widget output. They are **implementation evidence**, never reference images,
and are not used as self-generated parity goldens.

## Verification boundary

Focused projection and native Tester tests pass. The final local verification
completed successfully:

- `go test ./internal/desktop -count=1` — passed.
- `go vet ./internal/desktop` — passed.
- `go tool mygo vet .` — passed.
- `output/tools/golangci-lint.exe run ./internal/desktop/...` — 0 issues.
- Opt-in screenshot export — 16 implementation PNGs plus SHA-256 manifest.
The backend's own state-transition, counter and race evidence is maintained in
`docs/audit-agent-data.md`. Compilation, headless rendering, native Windows
runtime behavior and user visual acceptance remain separate result categories.


## Side-input closure and native-motion follow-up

The side input uses official `ui.TextAreaBase`, with the main composer's 14px
font, 22px line height, 16/18/8/18 input insets, 116px minimum surface and 22px
corner radius. It does not create a nested App, backend, window, subscription or
alternate navigation. `sideComposers` owns only per-session input/widget state.
All commands still go through the parent dispatcher. A matching request ID lets
`sideCommandFinished` consume side-send completion before the central draft/error
recovery path runs. The draft is retained while pending and on failure; a newer
edit is never cleared by a late successful completion.

| ID | Follow-up check | Evidence | Status |
|---|---|---|---|
| SC-01 | Send targets the child ID and inherits that child's project/device/model/reasoning/access/plan/checkout fields | `TestSideComposerSendsToChildAndHandlesIME` checks the captured typed request and actual child messages | Tested |
| SC-02 | Sending does not mutate central draft or message list | Same native test | Tested |
| SC-03 | IME composition Enter does not submit | Same native test uses `Tester.Compose` | Tested in Tester; Windows IME runtime acceptance remains separate |
| SC-04 | Shift+Enter and Mod+Enter settings work with multiline plain Enter | `TestSideComposerRespectsSendShortcutModes` | Tested |
| SC-05 | Successful completion clears only the submitted draft and keeps input focus | Native send test | Tested |
| SC-06 | Send failure retains draft, settles pending state and retries through the same composer | `TestSideComposerFailureKeepsDraftAndRetry` | Tested |
| SC-07 | An older completion cannot erase newer text/attachments; stale request ID cannot take ownership | `TestSideCompletionDoesNotEraseNewerDraftOrAttachment` | Tested |
| SC-08 | Stop button and scoped Escape stop only that side session | `TestSideComposerStopAndEscapeOnlyTargetChild` records both commands | Tested |
| SC-09 | Switching/closing removes hidden input focus; reopening restores its native editor text | `TestSideTabsKeepDraftAndReleaseHiddenFocus`, including select-all/copy from the reopened editor | Tested |
| SC-10 | Per-tab drafts stay independent and survive tab close/reopen | Same native test | Tested |
| SC-11 | Native file picker and attachment remove stage only the child composer | `TestSideNativePickerStagesOnlyChildAttachments`; uses real typed importer and selected temp file | Tested |
| SC-12 | File drop, clipboard file list/PNG and workspace mentions use the child stage | Uses the same native transfer/importer interfaces as the central input | Implemented; additional native clipboard/drop checks pending |
| SC-13 | Sending while child is working queues there; editing queue restores only side draft/attachments | `TestSideRunningSessionQueuesAndEditsItsOwnMessage`, frozen controlled live backend | Tested |
| SC-14 | No side subscription or worker is started while building the view | Source inspection and MyGo vet; only explicit attachment actions start importer work | Inspected and vet passed |
| SC-15 | Official fold honors reduced motion and settles without persistent idle painting | `TestTranscriptOfficialFoldHonorsReducedMotionAndParks` counts builds/paints across three stationary seconds | Tested |
| SC-16 | Side-chat changes do not replace the primary composer or model picker | Side-specific code lives in `transcript_side*.go`; primary composer changes are covered by the separate model-picker audit | Inspected |

`desktop-side-input-light.png` was visually inspected after the new export. The
16-image manifest was refreshed. The side controller is a functional input/send
loop, not the full shared-composer refactor: side model/reasoning/access/plan
selection controls and their matching popovers still require the common composer
state to be extracted. Source 140ms folds versus the official 200ms implementation,
360ms row arrivals, 480ms connector reveals and 3400ms title shimmer remain
explicit differences. No MyGo dependency lock, patch tree or original Panel
implementation was changed for those timings.


## Shared-composer migration stage

The next stage replaced the separate side input skin with the single native
`composer_surface.go` and extracted model/reasoning/access/Plan widget state into
`composerControls`. Side and central controls now share the model picker and
source-shaped access popover, with independent session configuration and popup
state. This supersedes SC-16's earlier unchanged-central-surface boundary and the
earlier missing-side-controls statement. See `docs/audit-composer.md` for the
verified independent-control tests and exact remaining compact/dock/footer
differences. The source freeze removed an unfinished compact-layout experiment;
no compact/dock visual parity is claimed.


## 2026-10-29 side-chat creation entry correction

The previous input checks above exercised an already existing child and its
composer. They did not cover the **New side chat** action: that action created
an empty child but reused the central new-message pending flag, so no matching
run appeared to open it. The child entry also navigated centrally.

The entry now correlates creation by request ID, opens the right-pane composer,
and leaves the parent selection and draft intact. Existing child entries reopen
in that pane. Fork completion opens the copied conversation. Matching does not
infer ownership from list order, title, or the most recently created child.

`session_creation_test.go` covers both languages through the actual button and
send/reply path, ACK/event ordering, an unrelated concurrent child, navigation
while pending, error/retry, snapshot failure, existing-child reopening and fork.
The mock persistence check covers correlated request retries and identity reuse.
These remain deterministic in-process backend checks; they do not connect an
external model provider. Native Windows observations for this correction are
recorded separately under `output/side-chat-entry-fix/evidence/`.

The Windows build was exercised at 1600 x 900 in the Chinese showcase scenario:
**New side chat** opened an independent right pane; Enter sent a child message
and a completed mock reply appeared. Hiding the pane and reopening the child
from session info restored that transcript. The parent stayed selected and its
unsent draft remained visible. Screenshots, accessibility trees and persisted
identity/message checks are listed in `evidence/validation.json` there. The full
Go suite, MyGo vet, golangci-lint and Windows build passed. This verifies native
entry wiring and deterministic mock replies, not external-provider behavior.
