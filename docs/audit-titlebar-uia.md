# Titlebar reachability and R7 UIA audit

## Evidence and conclusion

R7 Windows package `output/resume-validation/popup-r7-packages/windows-amd64/Zeron.exe`
has SHA256 `15203765a1d01983c4b55cbe349af02ce7ba5599619bec4d5669695562a8a22f`.
The raw native snapshot is
`output/resume-validation/popup-r7-native-input/workspace-takeover-obscured-uia.json`
and its associated JPEGs. It retains Session information and Toggle right pane in
the UIA tree while the workspace strip covers their painted positions. The JPEG
and UIA dump are native evidence; they are not lossless reference comparisons.

The primary cause is an application layout error, independently reproduced with
the real `App.View` in `ui.Tester` before changing production code. Relevant app,
layout, workbench and session-info sources matched the R7 source snapshot at that
time (`output/native-uia-r7-review/baseline-source-hashes.json`). At a 1320×880
client with caption clearance 108, takeover retained Session information at
`(1108,7,28,28)` and placed New surface at `(1110,8.5,24,24)`. A pointer click at the
retained info control's center `(1122,21)` opened the real New surface menu
`Files / Browser / Terminal / Diffs / History`. The reproducible output-only Go
overlay, log and plain geometry are in `output/native-uia-r7-review/`.

The old `mainView` built `titlebar` unconditionally, then placed an absolute
workspace strip over it. Expanded allocation makes Chat width zero, so that
strip covered the existing titlebar. Those controls were still live elements,
not removed elements that necessarily survived a stale UIA commit.

The other confirmed issue was `.OnClick(togglePanel).Menu(...)`: pinned MyGo's
`Menu` owns primary pointer-down and keyboard activation. `ui/input.go:372–376`
and `ui/access.go:489–493` open the menu before the button click action. A primary
click therefore opened a menu instead of toggling the pane. `ContextMenu` is the
secondary-button/Shift+F10 API and preserves normal primary activation.

## Fixed source and implementation

Reference: read-only Zeron `ab189159f10cf8e63c0502be75a59bbc487e0eaf`.

- `crates/ui/src/shell/tabs.rs:60–79`: Files and pane toggles have independent
  `28 + 4 + 28 = 60` slots; caption clearance is subtracted once across the right
  surface and Files columns.
- `tabs.rs:315–399`: trailing geometry follows actual right/Files allocation;
  takeover clears the left window-control cluster.
- `tabs.rs:479–538`: tabs/expand occupy a separate clipped reveal, created when
  that reveal is at least 44.
- `tabs.rs:540–598`: fixed global controls remain outside the surface reveal,
  ordered Files then pane toggle.
- `tabs.rs:604–679`: session information belongs to the transcript's trailing
  edge; takeover does not construct either it or the session title.
- `theme.rs:819–822`: titlebar height 38 and top padding 4.
- `shell.rs:13443–13455`: Windows caption clearance is 108 plus edge inset 6;
  the Go code reads the current `c.TitleBar().Right`, rather than hardcoding 108.

`internal/desktop/titlebar.go` now builds one header with flow slots for session
identity/info, the workspace strip, and the fixed Files/pane controls. The old
absolute strip in `app.go` is removed. The title can yield width; interactive
controls have fixed widths and do not overlap. In takeover, title and session
information are not constructed. Files and pane controls remain reachable.
The pane toggle uses primary `OnClick` and secondary `ContextMenu`; Expand and
Terminal menu actions remain available. Existing element state uses stable keys;
no Element or Context is retained in app state or workers.

MyGo Split owns a one-DIP seam before its second pane (`ui/split.go:65–69`). Thus
the header seam is one DIP before the second pane's content rectangle. Tests
assert that explicit relationship; they do not shift the header to the wrong
side of the divider. The initial incorrect equality assertion is retained in
`titlebar-tests-initial.log`.

## Accessibility and helper boundaries

The independent framework review is
`output/native-uia-r7-review/framework-review.md`. Ordinary absolute siblings are
both visited by `ui/access.go:287–334`; only invisible/inert nodes are excluded,
and actual modal layers replace the underlying accessible subtree
(`access.go:223–238`). This framework does not infer arbitrary sibling occlusion.
The fix removes the obsolete application nodes rather than merely marking
covered, still-interactive controls as hidden.

Windows UIA replaces same-ID node data and children on update, and disconnects
removed nodes (`internal/windows/surface_access.go:416–480`). A stale client cache
remains a separate possible observation: that implementation does not raise
Bounds/Offscreen property-change events in `notifyChanges`. No fresh Current
versus Cached native property experiment was performed in this subtask.

The numbers 44/45 in the supplied automation dump must not be treated as proof of
a stable MyGo element ID. Native RuntimeId is based on the element's uint64 ID;
the supplied dump contains no RuntimeId. A coordinate click and UIA Invoke are
also different: pointer hit testing selects the topmost current hit rectangle,
whereas Invoke sends an AccessAction for the node ID. The former explains the
observed New surface action without requiring an index-remapping defect.

For an independent native check, use
`output/native-uia-r7-review/capture-current-uia.ps1` against the chosen process.
It reads fresh Current properties, RuntimeId, screen rectangles and FromPoint at
each tracked control center without clicking or invoking anything. Capture once
before and once after takeover. Compare name/RuntimeId/rectangle/IsOffscreen,
then compare that fresh tree with the helper's tree. Do not reuse numeric helper
indices across snapshots. If Current excludes the old nodes but the helper
retains them, investigate helper caching; if Current retains them after this
application fix, investigate the provider/update path. OS pointer and Invoke
checks should be recorded separately during authorized native acceptance.

## Executed validation

`CGO_ENABLED=0` on Windows/amd64, Go 1.27.3:

```powershell
go test ./internal/desktop -run '^(TestTitlebar|TestWorkspacePaneDragAndTakeoverLifecycle|TestWorkspaceRenderedColumnOrderAndResizeRestoration|TestWorkspaceTerminalDrawerDoesNotShortenOtherColumns)' -count=1 -v
```

PASS, package time 6.381s. Seven new top-level titlebar tests include a 48-case
matrix of widths 900/1320/1600, sidebar open/collapsed, Files open/closed,
split/takeover and native caption clearance 0/108. Tests use actual rendered
rectangles and pointer clicks, including the real New surface menu. Additional
cases cover primary/secondary activation, title/info removal and restoration,
large-font long titles, Settings entered with expanded state retained, new chat,
Files-only layout and left caption clearance. Three existing workspace
allocation/drag/takeover/terminal tests also pass.

`go vet ./internal/desktop`, `go tool mygo vet ./internal/desktop` and the scoped
diff whitespace check pass. The read-only UIA script passes syntax parsing and
assembly/type loading (including explicit WindowsBase for Point); its actual
UIA capture was not run.

Plain geometry is in `output/native-uia-r7-review/fixed-geometry/`; these are
implementation-side Tester measurements, not reference/native pixel acceptance.
No native windows, package builds or full repository test run were performed by
this subtask. Mainline R8 owns full gates, native primary/secondary clicks,
takeover/recovery, fresh UIA and system window dragging.

## Remaining differences and documentation corrections

- The source titlebar comment at `tabs.rs:407–410` describes a single Git-limited
  toggle, but the actual branch constructs the Files/pane pair for an existing
  session without a Git guard. The implementation follows the branch.
- `internal/platform/access.go:27` describes Bounds as the visible box, while
  `ui/transform.go:342–344` publishes the transformed, unclipped border box. The
  framework review recorded this comment/code mismatch. This titlebar subtask
  did not edit the vendor; the later R8 tenth framework patch corrects that
  comment to the actual border-box semantics without changing clipping behavior.
- In the historical R8 phase, new-chat canvas still exposed global Files/pane
  actions although frozen source omitted its trailing group. R10 subsequently
  corrects both header and effective body visibility, preserving selected-session
  actions and stored panel intent; see the R10 section below.
- In the historical R8 phase, the reused workbench TabStrip had 24px expand/plus
  controls and source expand-button 28px geometry remained pending. R11 corrects
  expand to 28px while preserving the source's 24px plus, and corrects their ordering,
  local treatment and gutters; see the R11 section below. Full titlebar styling,
  exact left cluster composition and animated reveal remain separate work.
- The frozen source enforces a 900×600 minimum window (`lib.rs:440`). The tests
  do not claim every arbitrarily narrow width can fit all fixed controls.

## R9 independent focus clarification

`reference/validation/native-focus-r9-result.json` supersedes the earlier
ambiguous tool focus report for the sampled sealed R8 main/side sequence.
External raw COM and fresh managed UIA both follow actual side→main→side
focus; the original tool still displays main. Stable complete RuntimeIds and
Current.HasKeyboardFocus distinguish the two editors. No product focus code
was changed for this diagnosis. Event delivery, screen readers, full input
coverage and the exact tool-report mechanism remain unproved. Different
diagnostic DPI contexts prohibit using their rectangles as geometry acceptance.

## R10 canvas and fresh-session behavior

Frozen `tabs.rs:417–428` derives canvas from `selected_chat_row()==None`, not
merely an empty ID. Its trailing group is absent on canvas; `panel_layout.rs`
also excludes right/Files from effective fit, `files_panel.rs` returns no
canvas width/body, and `set_surfaces_open` cannot open a canvas right pane.
The application now uses the same actual-row lookup for effective fit and
header chrome. Stored right/Files/expanded intent is retained. The current
guard does not replace backend snapshot reconciliation of a dangling selection.

Terminal has a separate source path. A canvas can open its bottom terminal,
but `open_new_session` closes it each time. `newConversation` now explicitly
resets terminal after navigation, including the same-empty-ID early return;
it preserves the source session's state saved during navigation.

Empty/removed-ID canvas cases retain the draft editor without panel takeover.
The App.View round trip opens the actual native editor, makes its buffer dirty,
renders canvas, restores the original session, verifies active tab/Unsaved and
full buffer through Tester's clipboard, then clicks expand/pane controls.
This covers dirty data and control reachability, not nondefault scroll/selection,
multiple-tab ordering or automatic focus restoration. Independent reviews are
in `output/titlebar-canvas-review/result.json` and `final-test-review.json`.

All final application/tagged tests, Go/MyGo vet and pinned lint pass; four
target packages are validated with unchanged260-source snapshot. The new Windows
EXE was also exercised with real OS input: canvas terminal open/reset/reopen;
selected session Files/explorer/layout.go; a controlled unsaved file comment
and main draft; expand → new canvas → return; collapse and primary pane toggle.
The dirty file, tab, expanded/Files state and main draft survived. Native exit0,
37 hashed raw observation files and all identities are sealed in
`reference/validation/canvas-r10-result.json`. There was no file Save or message
send. Old R8 fixture PNG scores remain separate; no new pixel acceptance is
inferred. Mac remains build-only and the broad goal remains active.

## R11 TabStrip source correction

[The R11 audit](../output/tabstrip-r11-audit/result.json) records the fixed
`ab189159f10cf8e63c0502be75a59bbc487e0eaf` source, implementation hashes, old/new
Tester geometry and executed checks. This section updates TabStrip facts only;
the R8 and R10 sections above retain their phase-specific evidence.

Frozen `shell.rs:13586–13633` defines expand as 28×28 DIP with a 16px muted glyph
and radius 6. Plus is 24×24 with a 13px muted glyph and radius 6
(`shell.rs:12944–13076`), so changing both controls to 28px would be incorrect.
Chips and plus share the horizontal scrolling content, with plus present only
when at least one chip exists; expand occupies the fixed slot to their right.
`shell/tabs.rs:36–41,497–535,634–641,684–687` supplies the collapse glyph when
expanded, one 4px top inset, 8px left/4px right padding, a 4px gap and a transparent
band without its own bottom border. R11 implements those settled conditions, plus the 150ms
wash(.11) hover and immediate effective reduced-motion state change.

Non-Git Diffs/History entries are omitted rather than disabled. The menu and
launcher now use only the selected project's typed `GitDetected`: stale branch
labels cannot create Git actions, an unloaded branch list does not remove them
from a detected repository, and no-project/missing-project scopes cannot borrow
another project's detection. The older fixed-plus and disabled-entry statements
in the workbench audit are historical and corrected in its R11 section.

All 45 workbench top-level tests pass (1.038s); scoped Go vet and MyGo vet exit 0.
The regressions cover geometry in two widths and both themes, hover/motion
overrides, changing glyphs, overflow center clicks and keyboard access, plus
focus identity after preceding-tab removal. These headless implementation checks
do not establish native or pixel parity. The main agent's R11 full-package and
native validation is still in progress at this section's write time.

The default MyGo overlay scrollbar remains a known difference: its visible
track at y26..38 overlaps the chip/plus bottom y26..33 by about 7 DIP, although
the full-height scroll region now keeps the tested control centers reachable.
Full-rectangle hit parity is not claimed. The source's 36px edge fade, 150ms sibling
drag slide and 168px custom add menu with a 10px lower gap remain unported; the
existing native menu's focus/repeated-press semantics and native visual/DPI
acceptance also remain pending.

## R12 scrollbar and opaque fade follow-up

The preceding scrollbar/fade status is historical R11 evidence. R12 scopes
`Scrollbars(false)` to the tab scroller and implements the source36DIP opaque
shell ramps. Its single-render idle tests reproduce six stale-fade cases and
fix all eight cases without extra host frames. The exact Windows release EXE
was exercised at DPI192 with horizontal wheel, lower-edge coordinate clicks,
five-item plus menu and expand/collapse; it exited0 with empty stderr.

Native inspection also found an active History tab can remain clipped after
insertion or takeover collapse, requiring manual wheel. This remains pending
and is preserved in the R12 raw evidence. Glass masks, drag/reveal transitions,
custom-menu parity, independent matched reference images and the complete
native DPI/platform matrix are not accepted. See
[R12 audit](audit-tabstrip-scrollbars.md) and
[phase index](../reference/validation/scrollbars-r12-result.json).

## R13 menu reveal follow-up

The retained R12 menu failure is now fixed: consume the entry request before
building, preserving requests raised by synchronous Chosen for the next pass.
Eight menu cases fail before and pass after; direct/End/manual-scroll controls
remain valid. The exact R13 Windows release shows the full History chip after
native menu selection without repair input, and preserves a later manual wheel.
It exits0 with empty stderr at DPI192;34 raw files are retained. Resize/takeover
expectation is separate and unestablished in the reference, so it remains
unchanged. See [R13 audit](audit-tabstrip-reveal.md).


## R14 independent component baseline

The real original TabStrip and Go production TabStrip now have36 native pairs at
actual2x DPI. Before/after actual Win32 foreground/owner identity replaces the
unproved assumption that GPUI cached active means foreground. The declared test
host includes the source-proven outer envelope and expand helper; it is not the
full production titlebar.69 comparison guards and five gates pass, but colors,
glyphs and default native caption pixels still differ. Synthetic setup is not
OS input/UIA acceptance. See [R14 audit](audit-tabstrip-fixture.md).

## R15 follow-up

R15 changes application tab drawing through the existing shader API; the `.6`
framework tree and eleven patches are unchanged. Four package targets and a
separate exact Windows EXE tab interaction smoke pass.36 original/Go baseline
pairs plus four supplemental states do not establish full visual acceptance;
see `audit-tabstrip-style.md`. Earlier evidence above retains its phase identity.
