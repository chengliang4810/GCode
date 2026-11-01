# Vector artifact rendering provenance

The application embeds SVG paths prepared by `tools/reference-artifacts` from
the original Zeron zatex crate and Mermaid adapter. Reference commit:
`ab189159f10cf8e63c0502be75a59bbc487e0eaf`. The artifact metadata records initial
generation against `bf6d71aaaf00cc1e6f97d8daf4ccb2bbd0b4414f` and verification that
the relevant renderer paths were unchanged at the updated reference.

- Formula layout: `crates/zatex`, MIT under Zeron's original license, using
  the six original math faces documented in `../README.md`.
- Mermaid rendering: mermaid-rs-renderer 0.3.1, MIT; original Zeron adapter.
- Text-to-path preparation: usvg 0.46.0, fontdb 0.23.0 and rustybuzz 0.20.1;
  ttf-parser 0.25.1 reads font outlines. Original license files are included here.
- Diagram labels are outlined using the actual imported Geist/Geist Mono faces,
  not substituted system fonts. Their OFL notice is in `../fonts/`.
- Math glyph paths come from Fira, STIX Two, New Computer Modern, Libertinus,
  TeX Gyre Pagella and Euler Math. Their supplied OFL/GUST licenses and the
  GUST fonts' original name-table copyright text are in `../fonts/`.

The executable contains native SVG views and the generated vector assets.
The Rust generator and its executable are not application runtime dependencies.
Current conversation formulas are laid out at runtime by the application's
pure Go box compositor with the pinned go-latex AST/symbol backend and the same
six original, unmodified OTF faces. They do not reuse the frozen fixture SVGs.
The go-latex license is retained in `../go/go-latex-BSD-3-Clause.txt`;
runtime font hashes are under `runtimeMathFonts` in `internal/assets/manifest.json`.
Source evidence: `internal/assets/data/artifacts/provenance.json`,
`diagram-manifest.json`, `tools/reference-artifacts/Cargo.lock` and the original
Zeron font notices. No license text in this package has been manufactured from
a license name alone.
