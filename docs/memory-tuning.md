# Memory tuning experiments

Measured on Windows 11, Intel iGPU, Sogou IME installed. Metric is private
memory of one mock-scenario instance (`--scenario default|stress`, 1320x880),
sampled three times after a 15 s settle; median reported. Reproduce with the
probe pattern: start the binary, sample
`\Process(<name>)\Working Set - Private` plus `PrivateMemorySize64`.

## Baseline (before tuning)

| Scenario | Private working set | Private commit |
|---|---|---|
| default | 75.5 MB | 163.4 MB |
| stress | 91.8 MB | 177.2 MB |

## What does not matter (measured, do not revisit)

- **Go heap.** `GODEBUG=gctrace=1` shows 3-7 MB live at steady state. `GOGC=10`
  saves ~10 MB commit on stress; `GOGC=50` saves ~2.5 MB. Not worth the GC
  frequency; the default GOGC stays.
- **Glyph atlases.** Instrumented mygo's text system: mask atlas reaches
  256x256 (1 bpp), color atlas 128x128 (4 bpp) — 0.2 MB total. Bounded and
  repacked by `MakeRoom`.
- **Swapchain oversizing.** mygo allocates 25%+64 px oversized buffers only
  during interactive resizes and rebuilds exact-size 2-buffer swap chains once
  resizing settles (`d3d11.go` `resize`/`buffers`).
- **UI font stack.** Removing all 16 bundled Geist faces was A/B tested
  against the same binary: private commit 125.8 vs 126.6 MB — no measurable
  difference. DirectWrite parses the in-memory collection lazily. The system
  font stack change was therefore reverted; the bundled Geist/Geist Mono faces
  remain for a consistent cross-platform look.
- **Window size.** 900x600 vs 1320x880 differs by ~40 MB commit (GPU buffers
  and swap chain scale with client area). Expected, not actionable in code.

## What was implemented

1. **One-shot `debug.FreeOSMemory()` after startup settles**
   (`cmd/rcode/main.go`, 20 s after `WhenReady`). Startup allocates window
   buffers, model catalogs and the first transcript in bursts; the scavenger
   returns those free spans only gradually, so one forced GC+scavenge hands
   them back early. Measured: default 163.4 -> 128.1 MB commit, stress
   177.2 -> 132.6 MB. Live heap is tiny, so the STW cost is milliseconds.
2. **Attachment media cache bounded** (`internal/desktop/media.go`). The
   cache of decoded attachment bitmaps/SVGs was process-lifetime and untyped
   (`sync.Map`, never evicted): every image preview ever opened stayed
   resident. Now a 16-entry LRU; evicted images decode again when shown.
3. **Wallpaper retention fixed** (`internal/desktop/settings_background.go`).
   The settings row preview showed at 36 px but kept a full 2048 px bitmap
   (~16.7 MB RGBA + GPU texture), and the `Pixels` field was written but never
   read (another ~16.7 MB). The preview now builds a 128 px thumbnail and the
   dead field is gone. Only visible with a wallpaper set, which is why mock
   probes never showed it.

## Known remaining costs (environment or bounded by design)

- IME injection (~24 MB private working set on this machine) loads into any
  process with a focused text input; same for Notepad, RCode, ZCode.
- DirectWrite layout cache is capped at 64 MB live / 8 MB per layout in mygo
  and grows with real transcript volume; eviction and repacking are tested.
- Driver/OS sections (Intel user-mode driver, DWM shared sections) dominate
  the image/mapped commit and are not app-controllable.
