# Native agent transcript audit

2026-11-04 左侧对齐修正：按用户确认，以工具分组的展开箭头对齐正文，
分组标题保留缩进；回复复制按钮改为起始侧对齐图标。
分组整体使用 10 DIP 的负左外边距抵消原有 disclosure 画布留白，
标题前的 22 DIP 槽位、分组内部 48+8 DIP 层级和连接线相对几何保持不变。
下文源码参考的几何数值均相对分组根节点，不再代表其相对正文的绝对位置。
`TestTranscriptContentLeftAlignment` 覆盖主对话/侧聊内边距、窄窗、深浅主题与紧凑模式；
transcript、统计、共享控件专项与桌面包 MyGo vet 通过。
调整前原生截图位于 `output/transcript-left-before/20261104-032719-806348220/`，
最终原生对齐与悬停专项深浅主题通过并检查截图，位于
`output/transcript-left-final/20261104-033340-1304840333/`。
完整活动原生场景仍在思考预览断言失败，证据位于
`output/transcript-left-aligned/20261104-033108-529947149/`。
使用 Go overlay 仅撤去本次分组左偏移、保持工作区不变后，浅色同一断言仍失败，
见 `output/transcript-left-baseline/20261104-033316-3376082655/`；
未将完整活动场景记作通过，未修改范围外的思考预览或其测试。

2026-10-30 tool-detail update: the user authorized the later RCode thought
preview and question deduplication behavior, superseding the frozen-reference
rules for those interactions. Thoughts and folded notes retain every formatted
line. Tool invocation/output retain all received text, including wide lines;
their shared virtualized viewport grows to 24 display lines and scrolls inside.
Diff invocation and the native diff share a 24-line scrolling viewport.
The limit applies to visible height, not source retention; existing tool
execution/output budgets remain core policy. The final reply remains native
Markdown outside these detail viewports.

Thought details open only on user input. Folded live thoughts show the latest
nonblank source line without waiting for a newline; settled thoughts show the
first. Opening a detail pins its parent group open, so completion cannot hide a
reader's explicit choice. `AskUserQuestion` keeps its real core history but is omitted
from ordinary work rows and tool counts. As of 2026-11-04 its interactive form
replaces the bottom composer, with paged questions, Back, text overrides and
draft restoration. The transcript keeps a passive one-line record instead of
the full form. Failed question calls retain an independent bounded error detail even
when argument validation prevented creation of a question card.
Readable command/search fields are decoded by the core at `tool.start`, with
the original arguments retained for inspection.

2026-11-01 activity-summary tip: the header keeps its native
`Activity: <summary>` accessible name, but the visible summary tooltip is
attached only when the header shows the worked duration instead (compact
mode), and it anchors to that visible label rather than the full-width header
row. Normal mode already shows the counts, so its tip only repeated the row;
the frozen reference carries no tooltip there. `TestTranscriptActivitySummaryTipOnlyWhereTheHeaderHidesCounts`.

2026-10-30 当前状态：权限入口、PR 入口和配置、项目动作及本地账户菜单已移除。
正文保留当时的源码审计与验收证据；当前实施范围见
[UI 残留清理](ui-residual-cleanup.md)，不再把这些旧入口列为待实现功能。

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
only by a Wrote/narration row. That was the original implementation baseline.
The authorized 2026-10-30 update above now implements live previews after
checking RCode commits `f86269f` and `3d7f39a` from 2026-10-26. The fixed reference
and historical screenshots remain unchanged.

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
| TR-21 | Thought details require explicit input in both modes; folded headers preview live/settled text | `TestTranscriptReasoningPreviewUpdatesAndManualExpansionPersists`, `TestTranscriptCompactLiveNeverAutoOpensThought` | Tested |
| TR-22 | Explicit fold choice survives deltas, completion and virtual row recreation | Same native regression | Tested |
| TR-23 | Enter and Space operate the focused native disclosure | `TestTranscriptNativeDisclosureAndFixedTreeGeometry` | Tested |
| TR-24 | Expanded reasoning contains styled text, not Markdown source markers | `TestThoughtFlattenUsesSourceRecordLayout`, native detail test | Tested |
| TR-25 | Thought headings, inline styles, code, bullets, quotes and tables use a fixed record layout | Official parser + native RichText; flatten regression | Tested |
| TR-26 | Thought wraps at 96 characters; retains all lines within a viewport of at most 24 lines | Flatten regression; `TestTranscriptDetailsScrollCompleteContentWithinTwentyFourLines` | Tested |
| TR-27 | Tool invocation/output wrap wide text without dropping characters; at most 24 rows visible, internally scrollable | `TestTranscriptDetailPreservesWideOutputWithoutEllipsis`, scroll regression and theme/width/font geometry tests | Tested |
| TR-28 | Expanded output uses the independent code font size with 1.5 line height, 6px top/bottom padding and faint mono text | Theme/width/font geometry tests; native detail PNGs | Tested and inspected |
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
| TR-39 | Question tools do not duplicate the passive transcript record or tool count; interaction lives in the bottom composer | `TestTranscriptQuestionAppearsOnlyAsQuestionCard`; `TestQuestionComposerReplacesMessageAndRestoresDraft` | Tested |
| TR-40 | Diff and invocation share a bounded scroll viewport without moving the conversation | `TestTranscriptDiffUsesOneBoundedScrollingViewport` | Tested in implementation |

The 2026-10-30 detail update passed the full Go suite, Go vet and MyGo vet.
Final Windows native evidence is
`output/transcript-activity/20261030-162651-3743064682/evidence.json`.
Its light/dark runs used a real local core, real file reads and persistent
history; only model responses were supplied by a local HTTP fixture.
Screenshots and trees verify the native interaction implementation, not reference
pixel parity, remote hosts, real external providers or OS input methods.
The reference-link investigation is separate: see
[RCode link rules and current gaps](rcode-links-investigation.md).
| TR-39 | No persistent Element/Context, view-launched worker, or alternate fixture reader | MyGo vet and source inspection | Tested by vet and inspected |
| TR-40 | Source 140ms folds, 360ms row arrivals, 480ms connectors and 3400ms active-summary shimmer | The scoped comfort follow-up uses MyGo `.16` `CollapsibleBaseWithAnimation` for 140ms CSS ease-out tool folds. Other native Collapsibles retain 200ms; arrival/connector/shimmer remain pending | Tool-fold timing implemented; remaining differences retained |
| TR-41 | Inline diff matches original checkout-diff body, dual line gutters and syntax runs | Uses official native `editor.DiffView`; a fresh GPUI inline-diff comparison has not been made | Pending visual comparison |
| TR-42 | Side transcript provides the full original side-conversation input/navigation surface | Actual child messages, independent input, send/stop, attachments and queue editing work in a native right tab. Full model/access/plan controls remain absent rather than replaced by alternate navigation | Functional input loop tested; full composer controls pending |
| TR-43 | Original GPUI and replica pixels agree for all row/hover/selection states | No fresh matching reference screenshots exist for this change | Pending |
| TR-44 | Long history, long output and selection remain within performance gates | Official outer virtualized List retained; this audit does not supply P95/native stress measurements | Pending performance acceptance |
| TR-45 | Conversation rows follow the reference `rows_for_entry` block split; the copy/timestamp footer sits on the entry's last row; a selection keeps rows the List stopped building and copies their text | `TestConversationRowsSplitAMessageIntoBlockRows`, `TestTranscriptFooterFollowsTheMessageRows`, `TestTranscriptSelectionCopiesAfterItsRowsScrollOut`; framework `TestListTextSelectionSurvivesRowsLeavingTheBuild`; native e2e `output/desktop-e2e/20261101-173322-3584585105` (links, fold motion, Markdown tables, side chat) | Component and native-window interaction pass; frame timing and visual comparison pending |
| TR-46 | Fenced code blocks show tab indentation at the editor's tab stops of their language (Go 4, JS/TS/JSON 2, unknown 4); the source and Copy code keep tabs, and a selection copies the source offset | `TestCodeBlockTabsShowAndCopy`, `TestCodeBlockTabsUseTheLanguageSize`, `TestReviewCopyMarkdownCodeTabs`, `TestTranscriptCodeBlockShowsTabs` | Component-level pass; native window visual check pending |

## Captured implementation evidence

Regenerate without opening a native application window:

```powershell
$env:RCODE_TRANSCRIPT_EVIDENCE = Join-Path (Get-Location) 'output/captures/transcript-audit'
go test ./internal/desktop -run '^TestTranscriptRenderEvidence$' -count=1
Remove-Item Env:RCODE_TRANSCRIPT_EVIDENCE
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

## 2026-10-30 transcript link integration

The ordinary Markdown, mixed math, user-reference and table link entrances now
share strict source-session routing and native link interaction. Files open in
the right workspace through the owning host's typed linked-file service; URLs
default to its Browser. A new Browser tab has an empty native address page and
creates no WebView until navigation. Markdown files default to Preview, with
case-insensitive extensions, relative document links and character-column
conversion for source locations. See [transcript-links.md](transcript-links.md)
for the implemented selection behavior, remaining presentation differences and
the actual verification scope.


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
state to be extracted. At that stage, source 140ms folds versus the official
200ms implementation, 360ms row arrivals, 480ms connector reveals and 3400ms
title shimmer remained explicit differences. The later scoped comfort follow-up
supersedes only the fold-timing boundary through the pinned `.16` native API;
arrival, connector and shimmer remain separate differences.


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


## 2026-10-30 scoped fold comfort

The scoped fold-comfort trial uses 140ms native folds and temporarily removes
the owning pane's follow spring during a manual fold. Long details retain a
24-row internal viewport while measuring visible rows and caching parsed data.
Real local-core Windows tests passed in both themes, including rapid reversal,
reduced motion and sampled stationary tail geometry. This does not resume the
paused visual-parity goal. See [tool-fold-motion.md](tool-fold-motion.md) for
before/after evidence, source-pin updates and test exclusions/failures.

## 2026-10-30 side-chat context and launcher

The current side chat inherits an independently persisted model-context snapshot,
while keeping an empty visible transcript. Creation now lives in the right
workspace's empty launcher and `+` menu; the session-info creation action below
was removed. Existing-child reopening still preserves the main selection/draft.
See [side-chats.md](side-chats.md) for the current behavior and verification scope.

## 2026-10-29 side-chat creation entry correction (historical)

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
