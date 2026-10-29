# Frozen production footer fixture

`footer-build.ps1` adds a fixture-only `footer_usage_fixture` module to the
existing ignored reference export. Its only change to `src/lib.rs` is a public
module entry guarded by the already-existing `compact-picker-fixture` feature.
The module uses the production `account_usage::AccountUsage::new` and `track`,
shared `AccountsSnapshotCache`, and public `AppState.context_usage`. Context
chips/cards are rendered by the original production component. No engine is
bootstrapped and no external service or account is contacted.

```powershell
./reference/fixtures/footer-build.ps1
./reference/fixtures/footer-build.ps1 -Run
```

Coordinate the native window slot before using `-Run`. The builder uses the
existing ignored Cargo cache to avoid rebuilding dependencies, then copies the
new example to a fresh `output/footer-reference-build-*` directory. It does not
change any manifest, dependency, lock, compact fixture, production component,
renderer or read-only source checkout. Every build saves source snapshots,
protected production hashes, adapter/transport hashes, binary hash, logs and
`provenance.json`. The original lossless transport module is extracted unchanged
from `native-capture-lossless.rs`; `windows-capture` remains an example-only dev
dependency.

The client is 840×560, with a 24px root inset and a bottom 24px row aligned to
the end. It has no heading or native title. The application view changes between
dark/light while native window appearance retains the system policy. Each
production view receives harness Codex and target `None` (metadata's normalized
target is `local`). The active account has three source windows: Session 20%,
Week 40%, Month 90%, so its binding ring is 90% while the card displays only the
first two windows. Its second account uses the public `displayName` fallback.

The 28 filenames follow this matrix for each of `dark` and `light`:

| Context | Base | Context card | Accounts card |
| --- | --- | --- | --- |
| known = 42000/200000 | `footer-known-THEME.png` | `footer-known-context-THEME.png` | `footer-known-accounts-THEME.png` |
| capacity = unknown/200000 | `footer-capacity-THEME.png` | `footer-capacity-context-THEME.png` | — |
| zero = 0/200000 | `footer-zero-THEME.png` | `footer-zero-context-THEME.png` | — |
| over = 240000/200000 | `footer-over-THEME.png` | `footer-over-context-THEME.png` | — |
| warning = 150000/200000 | `footer-warning-THEME.png` | `footer-warning-context-THEME.png` | — |
| danger = 180000/200000 | `footer-danger-THEME.png` | `footer-danger-context-THEME.png` | — |
| unknown = no host snapshot | `footer-unknown-THEME.png` | — | — |

The harness does not set or expose private popup fields. A nonpainting absolute
canvas around the production entity observes its actual cluster bounds without
adding a hitbox or occupying layout space. A one-pixel inset from the first or
last chip's outer edge, at its vertical center, lies inside the production 6px
padding. Visibility and a measured 24px cluster height are checked before the
public GPUI `dispatch_event` path delivers synthetic mouse down/up to the real
production handler. This is not an OS mouse/focus test or an ElementId bounds
query. Captured cards must still be inspected as evidence; no private popup
state is invented to assert success. After every scene reset and trigger click,
the public GPUI synthetic mouse move places the pointer at root blank `[10,10]`.
Metadata records that last synthetic point. This clears residual hover on a
closed baseline while an open card retains its production open-state wash;
the OS pointer is not moved.

Each `*.png.capture.json` retains the existing WGC capture schema: own PID/HWND,
complete window and client geometry, scale, original PNG dimensions and SHA-256.
Its `fixtureState` adds the actual public context/account DTOs, production
fraction readings, measured root/footer/cluster bounds, synthetic click point,
appearance, active-window observation, title and scaffold constants. Full WGC
frames include native/nonclient pixels. Nothing is cropped, resized, masked or
converted from JPEG. No capture permission override or system-setting change is
used; unsupported/denied capture stays pending.

Compilation and capture produce instrumented reference evidence. They do not
establish pixel parity, OS input, timing, accessibility or platform acceptance.

The final 1x 2026-10-28 run is `output/footer-reference-capture-20261028-r2`.
Its 28 original PNGs passed signature, original IHDR/frame dimensions and SHA
checks; all have client 840×560, complete WGC frame 842×592 and scale 1. The
public DTO optional counts, fractions, account list, 24px footer geometry and
last synthetic pointer `[10,10]` also passed independent checks. Native window
observations were light/active/blank-title for all 28, without changing the
system appearance policy. Production account/context cards were inspected from
the saved original PNGs.

The final executable SHA-256 is
`9b97d945c47b66f0d722264f74dcd838ae05bb2c6322a202e83ce24ba5be3ae9`.
Source snapshots, build/run logs, protected manifest/lock/compact-file hashes,
artifact hashes and typed validation are in
`output/footer-reference-build-20261028-110355-698`.

R1 remains in `output/footer-reference-capture-20261028-r1` as a diagnostic:
the old generic transport's environment fallback incorrectly labeled the
surface `theme` while the harness used `Opaque`, and its synthetic pointer was
not reset between cases. R2 reads actual public settings and resets the pointer;
R1 files were not edited. `footer-path-tests.ps1` separately exercises eight
normal/rejection cases using only dummy `output/footer-*` directories, including
physical SourceRoot aliases and nested Cargo junctions. It does not access the
real frozen checkout or start a UI.

The later actual display inventory contains one scale2 display (4096×2560
physical, 2048×1280 logical); no harness changed OS display settings. The same
reference executable was recaptured at its actual scale in
`output/footer-reference-capture-20261028-r3-2x`, with provenance and typed
validation in `output/footer-reference-build-20261028-111556-297`.
All28 client frames are1680×1120 and complete WGC PNGs1684×1180; the
window-internal client offset is[13,58]. This is an independent2x run, not
resized or relabeled1x evidence. PID25044 exited0.

The Go2x production component fixtures pair with these originals in
`output/footer-comparison-r3-2x-reviewed/comparison.json`. All28 pass the
0.995 numerical screen; full footer, OS input and platform acceptance remain
pending. `footer-comparison-path-tests.ps1` exercises33 preflight cases,
including31 invalid states/paths and valid1x/2x controls. It intercepts the
parity build after successful preflight and does not run native UI or parity.
