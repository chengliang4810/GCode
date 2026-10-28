# Reference vector exports

This is a build-time evidence/asset tool, not an application dependency.
It reads the fixed `D:/zeron` reference, validates its Git commit, uses its
zatex crate and its unmodified Mermaid rendering adapter (with a minimal
RGB/theme bridge), and exports the same formula and diagram for all 30
resolved theme variants and all six math faces.

Run from the repository root with the Rust toolchain installed:

```powershell
./scripts/reference-export-vectors.ps1
```

Outputs are vector SVGs under `internal/assets/data/artifacts` and typed
resource metadata in `internal/mock/artifacts.json`. The script defaults to the
30 diagrams; pass `-All` to regenerate math as well. Fonts are outlined in both
math and diagram SVGs. Mermaid's original `<text>` output is preserved verbatim
under `reference/mermaid/raw`. Native diagram resources follow the original
`image_media.rs` preparation: usvg 0.46.0 / fontdb 0.23.0, system-font discovery,
all 16 original Geist/Geist Mono faces in their original order, the original
three generic-family mappings and disabled external/data image resolvers.
The tool rejects missing glyph outlines or a different system Geist face.
The serialized path-only vectors retain the original canvas dimensions; they
contain no text, HTML or raster images. Labels remain selectable/copyable from
the separate Mermaid source text in the Go data contract.

`internal/assets/data/artifacts/diagram-manifest.json` records both SVG hashes,
each label's outline path count and bounds, font hashes and the original font
loader source hash. Geist's OFL1.1 remains bundled at
`internal/assets/data/fonts/licenses/Geist-OFL.txt`. Tests verify all 30 vectors,
their raw text-bearing sources, exact dimensions and each of the six labels'
foreground pixels through MyGo's native SVG parser/renderer. These checks are
offscreen rendering evidence and do not establish native window visual parity.
The theme
bridge reads the imported resolved 8-bit theme definitions; native comparison
must still assess rounding, text rasterization, and material composition.

No graphical screenshot is used as a substitute for controls or an entire
application page. Rust and the reference checkout are not required to run or
build the shipped Go application after these resources are generated.
