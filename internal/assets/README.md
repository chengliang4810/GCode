# Application artwork

`branding/app-icon-small.svg` is GCode's GC monogram adjusted for native icon
sizes: enlarged counters, a clear C shoulder, white strokes on a dark rounded
square, no wordmark. `go run ./scripts/app-icon` regenerates the SVG and PNGs
directly from the same paths at 16/20/24/32/40/48/64/96/128/256/1024 pixels.
There is no bitmap-to-bitmap downscale in this generator.

Windows windows load the dedicated size matching the title bar and taskbar
at their DPI using `AppIconForSize`; the owned native handles are refreshed on
DPI resize and released when the window closes. `mygo.json` uses the 1024px
`branding/app-icon.png` for executable, installer and bundle artwork.
The original approved concept and prompts remain in
`output/gcode-logo-c-20261030/`. These branding files are separate from frozen
upstream imports.

## Imported source artwork

The 507 original files listed in `manifest.json` are copied byte-for-byte from
D:/zeron at commit ab189159f10cf8e63c0502be75a59bbc487e0eaf. The manifest records
their SHA-256, size and original repository path. Original icons and typefaces
are not redrawn.

`data/app-icon.png` and `data/icons/rcode-logo.svg` remain unmodified upstream
artwork for provenance; they are not the GCode desktop application icon.

`data/artifacts/` additionally contains generated vector resources from the
original math and Mermaid renderers. Mermaid's SVG text was converted to glyph
outlines using the original Geist fonts and original usvg/fontdb versions,
because the native icon SVG parser does not paint text nodes. Raw SVG inputs
are retained under `reference/mermaid/raw`; `diagram-manifest.json` records
both hashes, font provenance, dimensions and all label outlines. No diagram
is replaced by a bitmap.

- Zeron source, brand artwork and adaptations: MIT; see data/LICENSE.zeron.
- Geist and Geist Mono: SIL OFL 1.1; see data/fonts/licenses/Geist-OFL.txt.
- Solar Icons by 480 Design: CC BY 4.0; attribution retained in the original
  source's crates/ui/src/icons.rs and data/THIRD_PARTY_NOTICES.zeron.md.
- Symbols file icons: MIT; see data/file-icons/LICENSE.symbols.
- Curated themes retain their own source URL, revision and license in
  internal/design/themes.json and the original notices.

The Rust reference theme exporter is offline build tooling, not an application
runtime dependency. It exports original resolved palettes instead of guessing
colors from screenshots. The Go executable embeds the resulting JSON.
