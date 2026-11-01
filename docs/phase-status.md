# Implementation phase status

Reference 0.2.128 / ab189159f10cf8e63c0502be75a59bbc487e0eaf.
Recorded 2026-10-28. These are implementation milestones, not acceptance results.

| Phase | Current evidence | Remaining exit requirement |
|---|---|---|
| P0 | Frozen source/assets, 161-case inventory, settings source index, reference fixture compiled | Matching data/state reference harness and fresh native goldens |
| P1 | Native Windows window/fonts/SVG/terminal, browser page title, background shader, Mac builds | Compositor/IME/clipboard/browser layering and Mac native checks |
| P2 | Typed mutable/persisted mock services, ordered cancellable events, fixed scenes; tests/vet/race pass | Full scene-driven UI coverage and unused resource cleanup |
| P3 | Source themes/tokens, shell dimensions, native controls and layout fitting | All controls, surfaces, transitions and geometry compared with reference |
| P4 | Send/stream/stop/retry/queue, questions/permissions, drafts, Markdown/math/diagram, annotations/attachments | Full source selection/scroll/inline rendering and all reference states |
| P5 | File CRUD/edit/preview, diff/history, browser/mock terminal | Combined native panel/focus acceptance |
| P6 | Settings/search/fonts/shortcuts/themes/background; Mac Appshots UI/mock permissions | Remaining provider/account dialogs, complete shortcut catalog, wallpaper tint and exact pause/resume |
| P7 | Updated Windows package and install/start/uninstall pass; test/vet/lint/race pass; 79 fresh implementation captures | Native scenario/DPI/performance matrix, paired images and every visible difference resolved |
| P8 | Intel/ARM/Universal packages and architecture verification | Real Intel/ARM execution/feedback fixes; signing/notarization/DMG distribution |

Windows and macOS completion are both **not accepted**. Packages in
`build/ui-audit` predate the final handoff source changes and must be rebuilt.
The agreed plan remains in progress. Read `machine-handoff.md` for the latest
implementation details, remaining items and new-machine commands.
See acceptance-report.md for actual observations, known gaps and this turn's
native UI tool-state limitation.
