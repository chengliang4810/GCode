# Third-party notices

GCode includes source artwork, fonts, palette adaptations,
native UI framework code, Go libraries and a terminal native library from the
projects identified below. Their original licenses and attribution are included
offline in `licenses/`. The original texts take precedence over this index.

- Zeron source material, artwork and rendering adapters: MIT, copyright 2026 Wing.
- MyGo v0.7.3 with the reviewed `rcode-native-ui.20` patches: MIT, MyGo contributors.
- Geist and Geist Mono v1.7.2: SIL Open Font License 1.1, The Geist Project Authors.
- **Solar Icons by 480 Design**: Creative Commons Attribution 4.0 International.
  Imported SVGs retain their source bytes; native controls may resize or tint them.
- Symbols filename/folder icons: MIT, copyright 2020–22 Miguel Solorio. Dark-mode
  tint adaptations follow the Zeron source implementation.
- Theme adaptations: upstream project names, audited revisions, copyright notices,
  permission terms and the additional Shades of Purple condition are retained in
  `licenses/themes/Zeron-theme-adaptations-NOTICES.md`.
- Go 1.27.3 runtime, purego, go-text/typesetting and the Go x/image, x/net, x/sys and x/text
  modules: their original license texts and HarfBuzz attribution are in `licenses/go/`.
- Plugin frontmatter: `go.yaml.in/yaml/v3` v3.0.5, mixed MIT and Apache-2.0;
  original copyright/terms and Canonical notice are in `licenses/go/yaml-v3-LICENSE.txt`
  and `licenses/go/yaml-v3-NOTICE.txt`. The complete Apache-2.0 text is also included
  in `licenses/go/purego-Apache-2.0.txt`.
- Hook ECMAScript matcher expressions: `github.com/dlclark/regexp2` v1.12.0,
  MIT, copyright Doug Clark; original license and the complete Microsoft/Go/Mono
  attribution are in `licenses/go/regexp2-MIT.txt` and `licenses/go/regexp2-ATTRIB.txt`.
- Ghostty-VT native library: MIT at the exact source revision recorded by MyGo;
  the library manifest, known component notices and source details are in
  `licenses/native/`.
- Formula and Mermaid vector assets: renderer provenance and the six original
  math-font licenses are in `licenses/renderers/` and `licenses/fonts/`.
- Native runtime formula layout: go-latex/latex at
  `v0.0.0-20250304174226-2790903426af`, BSD-3-Clause; original license is in
  `licenses/go/go-latex-BSD-3-Clause.txt`. Six unmodified mathematical OTF
  faces are embedded at runtime, under the original OFL/GUST terms above.

- Models.dev metadata: bundled JSON from `https://models.dev/api.json`, retrieved
  2026-10-30; MIT, copyright 2025 models.dev. Original license is in
  `licenses/models-dev/LICENSE.txt`. It is used only to suggest custom model settings.

See `licenses/README.md` for versions and scope, `licenses/OPEN-ITEMS.md` for
unresolved attribution or distribution items, and `licenses/manifest.json` for
the SHA-256 inventory. This notice does not imply endorsement by the named projects.
