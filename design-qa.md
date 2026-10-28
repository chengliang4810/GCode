# Native design QA

final result: blocked

## Source visual truth

Frozen original source D:/zeron at
ab189159f10cf8e63c0502be75a59bbc487e0eaf. Historical captures and their hashes
are recorded in reference/baseline.json. Fresh matched-state Windows captures
of the original are pending.

## Implementation evidence

output/captures/home-dark.png, chat-dark.png, settings-dark.png are1320x880,
scale1, deterministic default data, MyGo Tester. Native Window.CapturePage:
output/captures/native-chat-dark.png. These are separate evidence categories.
Actual OS window was observed with Computer Use; capture alpha alone cannot
validate backdrop composition. No screenshots have been resized or normalized
for a misleading match.

## Comparison status

A normalized combined original/implementation comparison is not yet available.
The parity tool can write side-by-side, overlay, difference and SSIM files only
when supplied independently captured reference and implementation images.
Full-view and focused-region reference comparison therefore remain blocked.

## Required fidelity surfaces

- Fonts/typography: original fonts embedded; code font scale bug found in first
  capture and fixed; exact native glyph metrics/wrapping remain unaccepted.
- Spacing/layout: original metrics imported; sidebar/setting alignment and
  separator overflow corrected after first captures; source layout permutations
  and all DPI remain to be compared.
- Colors/tokens: all30 original variants exported; native dark/light material
  mismatch found via OS observation, source contrast guard added; compositor
  result pending recheck.
- Assets: source fonts/icons byte-verified; formula/diagram resources use
  original renderers. Image/diagram sizing and preview behavior need visual QA.
- Copy/content: Zeron branding and English retained, no mock banners; all
  source copy and conditional state combinations still require full audit.

## Iteration history

Latest granular iteration: model picker, workspace/files, activity transcript,
typed activity data and sidebar were audited separately against source and
official MyGo examples. Their five `docs/audit-*.md` checklists preserve both
fixed items and remaining gaps. Current review package and verification hashes
are recorded in `reference/validation/ui-audit-result.json`; 79 new matrix
captures are implementation-only. Current native self-capture/install results
do not remove the reference/native interaction blockers described above.

1. First native-Go captures: code text was massively enlarged because MyGo's
   CodeSize is relative, not absolute. Changed CodeSize to sourcePixels/bodySize,
   then recaptured chat. Fixed absent icon-name mappings.
2. Settings capture: default centered button contents and incorrect card role
   differed from source. Changed navigation to left justification and cards to
   original card token, removed width+margin separator overflow.
3. OS observation: light Windows backdrop under forced dark UI reduced contrast.
   Implemented native theme source sync and original window tint contrast guard.
   User stopped Computer Use before the next native iteration.
4. Source 0.2.128 interaction pass: added model effort/favorites, native clipboard
   and file drop, source background effects and exact Markdown line heights.
   Seventy-seven deterministic implementation captures were generated; no
   reference equality is inferred from their successful rendering.
5. Screenshot review exposed Mermaid's missing SVG text. The reference usvg/
   fontdb chain now converts all thirty resources to Geist glyph outlines;
   all180 label locations pass native SVG foreground-pixel checks. Light/dark
   renders were inspected and the implementation matrix recaptured. Matched
   original-window comparison is still required.

## Open gates

Fresh original captures, combined comparisons, materials/native recheck, the
remaining source interactions in docs/acceptance-report.md, fullDPI/performance
matrix and macOS native evidence are required before passing this report.
