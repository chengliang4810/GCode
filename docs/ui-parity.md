# Desktop UI parity inventory

Source baseline: `ab189159f10cf8e63c0502be75a59bbc487e0eaf` in D:/zeron.
Scope: Windows amd64 first, macOS 13+ amd64 and arm64; no Linux, mobile, voice or
dictation. Public webpage bodies alone may use the system web engine.

The machine-readable acceptance inventory is `reference/scenes.json`: **161
cases**, thirteen areas, both appearances, three platform targets where
applicable. Each case tracks reference capture and implementation verification
separately. The inventory is a coverage obligation, not a completion claim.

The latest granular source/UI audits are `audit-model-picker.md`,
`audit-workbench.md`, `audit-transcript.md`, `audit-agent-data.md` and
`audit-sidebar.md`. Their rows distinguish implemented behavior, native Tester
regressions, inspected implementation PNGs and pending GPUI/OS acceptance.
`implementationStatus=automated-regression-only` in the scene inventory means
only those named regressions passed; referenceStatus and platform acceptance
remain independent.

| Area | Original owner | Acceptance coverage |
|---|---|---|
| Window shell | shell.rs, shell/panel_layout.rs, lib.rs | titlebar, history, pane resize, takeover, narrow layout, persistence |
| Sidebar/projects | shell/sidebar_sections.rs, sidebar_pins.rs, spaces.rs | detailed/compact, icons, hover, pin/reorder, groups, search/filter, archive, remote |
| New session | shell.rs, composer_dock.rs | project/device/ref/worktree selectors, create project, background crop/effects |
| Composer | composer.rs, pickers.rs | IME, multiline, attachment wrap, models/reasoning, queue/edit, send/stop/retry |
| Transcript | transcript.rs, markdown/, tool_images.rs | rich text, code/table/math/diagram, reasoning/tools/questions/permissions/todo/subagents, images, selection, streaming and anchored scroll |
| Files | files/ | tree/search/context menus, edit/rename/save/readonly/conflict, tabs, code/Markdown/image preview |
| Git | changes.rs, history.rs | changes/diff/comment/history/refs/commit, empty/error |
| Terminal | terminal/ | mock Conn, ANSI output, input, tabs, selection, search, resize, long scrollback |
| Browser | browser/ | fixed page and public URL, navigation/tabs/loading/error/zoom, overlays and focus |
| Settings | settings.rs, settings/ | General, Appearance, Notifications, Shortcuts, Providers, Devices, Cloud, Billing, Files, Archived; search/theme/font/shortcut subflows |
| Appshots | appshots.rs, appshots/ | macOS only: settings, cards, queue, preview, transferring/unavailable |
| Global | shell/command_palette.rs, onboarding.rs | command palette, account states, update states, tooltip/toast/dialog/keyboard focus |

For every applicable control check idle, hover, pressed, selected, focused,
disabled, loading, empty, success, error and offline presentation. A visible
control must have the original interaction result backed by typed services;
an inert button does not satisfy coverage.

The source settings search inventory is also exported as
`reference/settings-index.json`: 58 authored rows, comprising 48 shared desktop
rows, five macOS-only Appshots rows and five excluded voice rows. This preserves
the original row titles, groups and search vocabulary, including conditional
rows and provider details, independently of implementation coverage.

## Fixed logical metrics

| Metric | Source value |
|---|---|
| Default/minimum window | 1320×880 / 900×600 |
| Titlebar/top padding/status strip | 38 / 4 / 24 |
| Sidebar default/bounds | 256 / 242–400 |
| File column default/bounds | 286 / 220–440 |
| Right pane default/minimum/chat floor | 520 / 360 / 300 |
| Transcript default/bounds/step | 736 / 560–1200 / 16 |
| Terminal default/minimum/viewport cap | 280 / 160 / 55% |
| Bubble/panel/control radii | 16 / 10 / 6 |
| Spacing | 4 / 8 / 12 / 16 |
| Original rem base/code/terminal | 16 / 12.5 / 13 |

Most shell control text is authored at 13px or 11px relative to the 16px rem
base; the MyGo default control theme is therefore 13px. Font settings scale
authored UI sizes by the original base, while code and terminal retain their
independent settings.

## Evidence and current verification

Asset provenance and exact theme export are implemented. The original source
registry and asset manifests are available for repeatable validation.
Fresh native platform reference captures and one-to-one scene comparisons
remain pending. Do not infer completed parity from this inventory or from
implementation-generated snapshots. The latest actual build, interaction and
native inspection results belong in `docs/acceptance-report.md`.

Focused native widget tests currently cover settings section bounds, all thirty
palette variants, theme import analysis/import/removal, font search and
selection, settings search ranking/typo/reveal, native shortcut recording and
persisted bindings, notification parent/child availability, and original
autosave delay choices. These are behavioral and offscreen layout checks;
settings dialogs, provider detail states, full background adjustment and macOS
Appshots still require complete scene-by-scene reference comparison. They are
not visual acceptance evidence.

Background settings now use the native image picker and source adjustment
dialog, including pan, anchored wheel/pinch zoom, Reset, Apply/Cancel and stale
image rejection. The persistent application controller prepares resources from
construction, attachment, state/settings changes, route actions and system
appearance notifications. Views only consume prepared images. Focused tests
cover the 900×600 modal bounds, draft persistence/cancellation/replacement, and
absence of resource I/O during repeated view frames. Exact source raster and
thumbnail reference bytes separately verify the Go effect port. Native file
picker/IME/compositor validation and paired UI screenshots remain pending.
