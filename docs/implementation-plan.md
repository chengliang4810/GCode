# Implementation contract

Accepted 2026-10-28. Reference: `D:/zeron` commit
`ab189159f10cf8e63c0502be75a59bbc487e0eaf`. Source is read-only.

## Scope

Go 1.27.3 native desktop (acceptance at that time used MyGo v0.7.3; current pin is v0.8.3). Windows 11 22H2+ amd64 first;
macOS 13+ Intel/Apple Silicon implemented together, with runtime acceptance
recorded per architecture. Current product branding is GCode and the interface
defaults to Simplified Chinese, with English available in settings. Use `rcode`
only for internal command, package and theme identifiers. Preserve upstream
names in provenance and frozen historical evidence.
Remove voice, dictation and their settings/shortcuts. Keep app identity and
storage independent. In-process typed backend, manual constructor injection.
Only external browser document bodies may use system web views.

## Gates

P0 freeze source, inventory routes/states/assets, create reproducible reference
fixtures and capture the same state on each platform. Historical repository
images are useful references, not current Windows acceptance goldens.

P1 verify native titlebar/fonts/IME/frost/SVG/streaming/math/long lists/terminal
and browser overlays before claiming capability parity. P2 implement typed
contracts, deterministic services/events and isolated persistence. P3 design
system/shell/panes. P4 complete chat/composer interactions. P5 files/editor/Git/
terminal/browser. P6 every settings/global entry. P7 full Windows acceptance
and release package. P8 native macOS acceptance and universal packaging.

## Acceptance

Reference and implementation must use the same platform, client size, DPI,
font, theme, data, time and interaction state. Default 1320x880; also 900x600
and 1600x1000. Windows scales 1/1.25/1.5/2; Mac 1/2. Compare every route/state
in light/dark, all 30 theme variants across representative component scenes.
Geometry error <=1 DIP with no accumulated row drift; exact design tokens,
font metrics, wrapping, icons and copy. SSIM >=.995 whole, >=.99 regional are
screening gates, never substitutes for visible-difference review. Capture
native compositor output for frost. Never replace reference goldens with app
captures or use broad masks to pass. Keep original and voice-excluded reference
captures and scope changes separately.

Every business control must produce a state transition. Exercise send/stream/
stop/retry/queue/questions/permissions, session archive/pin/reorder/search,
virtual file editing/conflicts, Git diff/history, terminal input/ANSI/search/resize,
real public website navigation and native overlay hit testing, settings
persistence, keyboard/IME/selection/undo/focus. Run Go tests/vet/MyGo vet/lint,
race checks on state/services, then packaged native application acceptance.

Stress: 1000 sessions, 5000 messages, long code/diffs/files/terminal history.
Record hardware. Targets: interaction P95 <=50ms; frame P95 <=16.7ms during
steady scrolling/dragging; interactive startup <=2s excluding websites; idle
does not redraw without cause; 30 pane lifecycle cycles do not leak resources.

The deliverable is complete only when the scope manifest and all applicable
platform acceptance gates pass. Compilation, unit tests, screenshots and user
acceptance are independent evidence categories. Pending native evidence must
remain pending.
