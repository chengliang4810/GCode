# Offline license index

Reference material was imported from Zeron v0.2.128, commit
`ab189159f10cf8e63c0502be75a59bbc487e0eaf`. Original license documents are copied
without rewriting their terms. Extracted notice sections and font metadata are
identified explicitly below. `manifest.json` records the delivered bytes.

| Component | Version / revision | License documents |
| --- | --- | --- |
| Models.dev catalog | JSON snapshot retrieved 2026-10-30 | `models-dev/LICENSE.txt` — original MIT, copyright 2025 models.dev |
| Zeron source, artwork and rendering adapters | v0.2.128 / `ab189159f10cf8e63c0502be75a59bbc487e0eaf` | `zeron/LICENSE.txt` — original MIT text, copyright 2026 Wing |
| MyGo | v0.7.4 + `rcode-native-ui.23` | `mygo/LICENSE.txt` — original MIT; patch source is fixed by `third_party/mygo.lock.json` in the source distribution |
| Geist / Geist Mono | v1.7.2 | `fonts/Geist-OFL.txt`, `fonts/Geist-NOTICE.md` — original OFL 1.1 and reference font notice |
| Solar Icons | imported reference Linear SVG set | `icons/Solar-ATTRIBUTION.md`, `icons/CC-BY-4.0.txt` |
| Symbols file icons | `296ef1b62287fb2315cb5651e552e09e8c8e1de8` | `icons/Symbols-LICENSE.txt` — original MIT, Miguel Solorio |
| Zeron palette adaptations | 30 resolved variants, original audited revisions retained | `themes/Zeron-theme-adaptations-NOTICES.md` — verbatim theme section from Zeron's notices |
| iTerm2-Color-Schemes | MyGo's generated Ghostty theme table | `themes/iTerm2-Color-Schemes-MIT.txt` — notice extracted from the generated source; this app selects its own Zeron theme palettes |
| Go runtime | go1.27.3 | `go/Go-runtime-BSD-3-Clause.txt` |
| purego | v0.11.1 | `go/purego-Apache-2.0.txt` |
| go-text/typesetting | v0.3.5 | `go/go-text-typesetting-LICENSE.txt` — upstream Unlicense OR BSD-3-Clause; HarfBuzz portions: `go/go-text-harfbuzz-MIT.txt` |
| golang.org/x/image | v0.46.0 | `go/x-image-BSD-3-Clause.txt` |
| golang.org/x/net | v0.59.0 | `go/x-net-BSD-3-Clause.txt` |
| golang.org/x/text | v0.43.0 | `go/x-text-BSD-3-Clause.txt` |
| golang.org/x/sys | v0.48.0 | `go/x-sys-BSD-3-Clause.txt`, `go/x-sys-PATENTS.txt` |
| go-latex/latex | `v0.0.0-20250304174226-2790903426af` | `go/go-latex-BSD-3-Clause.txt` |
| go.yaml.in/yaml/v3 | v3.0.5 | `go/yaml-v3-LICENSE.txt`, `go/yaml-v3-NOTICE.txt` — original mixed MIT/Apache-2.0 terms and Canonical notice; the complete Apache-2.0 text is also delivered in `go/purego-Apache-2.0.txt` |
| dlclark/regexp2 | v1.12.0 | `go/regexp2-MIT.txt`, `go/regexp2-ATTRIB.txt` — original Doug Clark MIT license plus verbatim Microsoft MIT, Go BSD and Mono test-data attribution |
| go-text/typesetting-utils | `v0.0.0-20260419141703-4ffe8874dabc` | `go/go-text-typesetting-utils-test-only-LICENSE.txt`; test dependency, not application fixture-font redistribution |
| Ghostty-VT | `befcdfd2c3a1cb24d9ec886e93c95b2b5daa7028` | `native/Ghostty-MIT.txt`, `native/PROVENANCE.md`, native manifest and component licenses |
| Fira Math | 0.3.4 | `fonts/OFL-FiraMath.txt` |
| STIX Two Math | 2.13 | `fonts/OFL-STIXTwoMath.txt` |
| Libertinus Math | 7.051 | `fonts/OFL-LibertinusMath.txt` |
| Euler Math | 0.75 | `fonts/OFL-EulerMath.txt` |
| New Computer Modern Math Book | 8.1.1 | `fonts/GUST-FONT-LICENSE.txt`, `fonts/LPPL-1.3c.txt`, original copyright from font name table in `fonts/GUST-font-metadata.json` |
| TeX Gyre Pagella Math | 1.632 | same GUST / LPPL documents and original font metadata |
| zatex | original Zeron crate | `zeron/LICENSE.txt`, `renderers/PROVENANCE.md` |
| mermaid-rs-renderer | 0.3.1 | `renderers/mermaid-rs-renderer-0.3.1-MIT.txt` |
| usvg | 0.46.0 | `renderers/usvg-0.46.0-MIT.txt`, `renderers/usvg-0.46.0-Apache-2.0.txt` |
| fontdb | 0.23.0 | `renderers/fontdb-0.23.0-MIT.txt` |
| ttf-parser | 0.25.1 | `renderers/ttf-parser-0.25.1-MIT.txt`, `renderers/ttf-parser-0.25.1-Apache-2.0.txt` |
| rustybuzz | 0.20.1 | `renderers/rustybuzz-0.20.1-MIT.txt` |
| Image thumbnail source artwork | imported reference asset | `artwork/image-thumbnail-MIT.txt` — original supplied text |

The application embeds all six unmodified mathematical OTF faces for runtime
Go formula layout, in addition to the Geist fonts and the historical vector
fixtures. Their frozen source paths and SHA-256 hashes are recorded in
`internal/assets/manifest.json` under `runtimeMathFonts`. The runtime uses
go-latex's AST/symbol backend with the application's native box compositor;
it does not load the Rust renderers. The original Rust and vector-generation
terms remain recorded for the historical generated assets.

## Primary sources

- Zeron: `https://github.com/zeronsh/zeron` (the local frozen reference is authoritative).
- MyGo: `https://github.com/egoist/mygo/tree/v0.7.4`.
- Geist: `https://github.com/vercel/geist-font/tree/v1.7.2`.
- Symbols: `https://github.com/miguelsolorio/vscode-symbols/tree/296ef1b62287fb2315cb5651e552e09e8c8e1de8`.
- CC BY 4.0 legal code: `https://creativecommons.org/licenses/by/4.0/legalcode.txt`.
- LPPL 1.3c: `https://www.latex-project.org/lppl/lppl-1-3c.txt`.
- Go library license files were copied from the local Go module cache at the exact
  versions listed above; the Go runtime license comes from the selected go1.27.3 toolchain.
  The Go runtime and all four Go x/ modules also retain their supplied `PATENTS`
  additional IP-rights grant documents in `go/`.
- Plugin frontmatter parsing uses `go.yaml.in/yaml/v3`; ECMAScript hook matcher
  expressions use `github.com/dlclark/regexp2`. Their `LICENSE`, `NOTICE` and `ATTRIB`
  files above are retained byte for byte from the pinned module versions.
- Renderer licenses were copied from the corresponding exact local Cargo registry sources.

See `OPEN-ITEMS.md` for the remaining scope limits. Websites shown in the live
browser are external content and are not archived in this resource package.
