# Frozen reference and provenance

The design source is `D:/zeron`, commit
`ab189159f10cf8e63c0502be75a59bbc487e0eaf`. It is read-only. The application
implementation never restores or imports the prior GCode application.

`reference/baseline.json` hashes the relevant shell, theme, typography, layout,
browser and icon sources. `reference/scenes.json` enumerates 161 acceptance cases
across thirteen areas. It includes platform visibility, light/dark appearances,
logical viewport dimensions, boundary sizes and density factors. Its statuses
are evidence statuses: they must not be changed to passed merely because a
corresponding view compiles.

`reference/settings-index.json` preserves all 58 authored settings search rows:
48 shared desktop rows, five macOS Appshots rows and five explicitly excluded
voice rows. `scripts/reference-extract-settings.ps1` regenerates the index from
the frozen source. Provider detail controls and feature-owned shortcuts remain
separate acceptance obligations; the index does not claim they are implemented.

## Artwork and palettes

507 original files are embedded by `internal/assets`: sixteen Geist/Geist Mono
font faces, source SVG controls, Symbols file artwork, the application icon,
associations and notices. `internal/assets/manifest.json` records size, SHA-256
and original path for every imported file. `scripts/reference-verify.ps1` checks
the frozen source, embedded copies and historical reference images.

All thirty theme variants across nineteen families are exported by executing
the unmodified original `zeron-theme` domain code in the isolated
`reference/theme-exporter` crate. The export retains resolved colors, semantic
accent roles, syntax tokens, terminal ANSI colors and upstream provenance.
`scripts/reference-export-themes.ps1` verifies the copied Rust source before
regenerating the JSON. Rust is reference tooling only; the desktop executable
uses the embedded JSON and has no Rust runtime dependency.

The Go background framing and effects port is in `internal/design/background*`.
It retains normalized focal coordinates, covered source geometry, anchored
zoom/pan, the original proxy thumbnail sampling, five raster treatments, and
the two-pass composer cutout/bottom-fade equations. The small console tool in
`reference/background-exporter` executes unmodified original effect methods and
the original image crate to produce independent algorithm references. All ten
appearance/effect buffers and five resize buffers match byte for byte in Go.
`reference/background-pixels.json` records their hashes and expressly classifies
them as algorithm tests, not UI screenshot goldens. Image color profiles beyond
the verified sRGB fixtures still need native comparison.

## Native Mermaid label preparation

A capture review found that MyGo's SVG parser skipped Mermaid's original
`<text>` nodes. The 30 native diagram resources now pass through the original
Zeron SVG preparation chain: usvg 0.46.0 / fontdb 0.23.0 and all 16 bundled Geist
faces, with identical generic-family mapping and resource restrictions.
Text is converted to vector paths, with the canvas size preserved exactly.
The original text-bearing SVGs remain under `reference/mermaid/raw`.

`scripts/reference-export-vectors.ps1` regenerates them. The embedded diagram
manifest retains raw/prepared hashes, label outline bounds/counts, selected
font hashes, parser source and locked exporter provenance. Tests check every
theme has zero text/raster elements and verify foreground pixels for all 180
node/edge labels using MyGo's SVG parser and offscreen renderer. Light/dark
inspection images are in `output/captures/mermaid-textpaths`; they confirm
label visibility, not native window parity. Existing generation history and
Geist's OFL notice are retained. Paired GPUI/native window acceptance is pending.

## Historical screenshots

Twelve existing repository screenshots are copied into `reference/historical`.
Their source path and SHA-256 are in the baseline manifest. These are historical
repository evidence, including Linux/macOS fixture exports, and are **not**
fresh Windows or macOS reference captures. Their owning source README files
describe platform and fixture limitations; some omit the compositor backdrop.
They cannot establish native Acrylic/vibrancy fidelity.

## Native reference build

The first offline metadata attempt failed because the pinned GPUI git checkout
was not cached. Network access to the pinned repository was available; a clean
archive of the approved commit was extracted to ignored
`output/reference-native` and a native sidebar fixture build was started for
the initial 0.2.126 baseline. The user subsequently selected the installed
0.2.128 version, and the baseline moved to the commit above. The old manifest
is retained in `reference/baseline-0.2.126.json` for chronology only.

The 0.2.128 archive is isolated in `output/reference-native-0.2.128`, with
the prior Cargo target cache reused. Its log is
`output/reference-native-0.2.128-build.log`. Both new cursor SVGs are imported;
the theme domain and original font/palette bytes are unchanged between these
two versions. GPUI's pinned revision changed, so that dependency rebuilds.

The first native build reached compilation of `zeron-ui` but was interrupted
to free the shared host for native Go integration and capture, and subsequently
stopped when the user changed the baseline. Cached artifacts are retained.
The repeatable build script limits Cargo to two jobs. Building the new frozen
reference does not authorize launching or automating its native window.

The unmodified `sidebar-fixture` deliberately uses a 1100×800 window and a
310px sidebar, rather than the application's default 1320×880 and 256px. Its
temporary projects, sessions and wall-clock values also differ from the Go
backend's scenarios. A later fixture capture must therefore be labelled with
those exact dimensions and data; it cannot be compared to a different Go scene
and presented as parity evidence. The source `settings-search-fixture` supports
an explicit `ZERON_FIXTURE_SIZE` and opens the production settings shell, but
its offscreen captures still cannot prove Windows compositor material effects.

The frozen 0.2.128 native sidebar fixture successfully compiled in 20m36s.
`reference/native-build.json` records its executable SHA-256, size, source
commit and build-only status. It has not been launched or automated. Until
native launch and capture are recorded, the fresh Windows reference status
is **pending**. macOS Intel and ARM reference capture
remain **pending on their native hosts**. No implementation image is a valid
substitute for a GPUI reference image.

## Approved differences

Voice and dictation controls, their settings page and shortcuts are excluded.
Only their removal and natural flow re-layout are approved. The raw original
capture and a separately identified voice-free reference capture must both be
retained before declaring visual acceptance. Independent bundle identity and
data directories are also intentional. Fonts, colors, icons and dimensions
are not permitted to drift under these exceptions.

## Measurement conventions

Compare the same platform, scene data, clock, appearance, viewport, density,
scroll position and interaction state. Full-frame and legible regional
comparisons are required. Logical geometry must stay within one DIP; text
wrapping and row heights must not accumulate drift. Full-frame SSIM 0.995 and
regional SSIM 0.99 are screening thresholds, not an automatic visual pass.
Native desktop captures against a fixed wallpaper are required for material
effects; offscreen captures prove only the content renderer.
