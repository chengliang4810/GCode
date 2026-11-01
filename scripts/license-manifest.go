//go:build ignore

// License-manifest inventories reviewed local texts; it never downloads or
// invents a license. New files require an explicit source mapping below.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gcode/scripts/licenseresources"
)

const reference = "Zeron@ab189159f10cf8e63c0502be75a59bbc487e0eaf/"
const ghostty = "https://raw.githubusercontent.com/ghostty-org/ghostty/befcdfd2c3a1cb24d9ec886e93c95b2b5daa7028/"
const uucode = "https://raw.githubusercontent.com/jacobsandlund/uucode/9d55524551411b493cca41ca06363625d90aff1e/"
const wuffs = "https://raw.githubusercontent.com/google/wuffs-mirror-release-c/7411f488fe2e2c205c3d3b3d28638b7356522930/"

var origins = map[string]string{
	"go/yaml-v3-LICENSE.txt":                             "go.yaml.in/yaml/v3@v3.0.5/LICENSE (local module cache)",
	"go/yaml-v3-NOTICE.txt":                              "go.yaml.in/yaml/v3@v3.0.5/NOTICE (local module cache)",
	"go/regexp2-MIT.txt":                                 "github.com/dlclark/regexp2@v1.12.0/LICENSE (local module cache)",
	"go/regexp2-ATTRIB.txt":                              "github.com/dlclark/regexp2@v1.12.0/ATTRIB (local module cache; includes upstream Microsoft/Go/Mono attribution)",
	"models-dev/LICENSE.txt":                             "https://raw.githubusercontent.com/anomalyco/models.dev/dev/LICENSE (retrieved 2026-10-30)",
	"go/Go-runtime-PATENTS.txt":                          "go1.27.3 selected toolchain/PATENTS",
	"go/x-image-PATENTS.txt":                             "golang.org/x/image@v0.46.0/PATENTS (local module cache)",
	"go/x-net-PATENTS.txt":                               "golang.org/x/net@v0.59.0/PATENTS (local module cache)",
	"go/x-text-PATENTS.txt":                              "golang.org/x/text@v0.43.0/PATENTS (local module cache)",
	"go/x-sys-PATENTS.txt":                               "golang.org/x/sys@v0.48.0/PATENTS (local module cache)",
	"go/x-sys-BSD-3-Clause.txt":                          "golang.org/x/sys@v0.48.0/LICENSE (local module cache)",
	"go/go-latex-BSD-3-Clause.txt":                       "github.com/go-latex/latex@v0.0.0-20250304174226-2790903426af/LICENSE (local module cache)",
	"README.md":                                          "project-maintained component/source index; no substituted license terms",
	"OPEN-ITEMS.md":                                      "project-maintained unresolved provenance inventory; no license grant",
	"zeron/LICENSE.txt":                                  reference + "LICENSE",
	"mygo/LICENSE.txt":                                   "github.com/egoist/mygo@v0.7.3/LICENSE (fixed local source copy)",
	"fonts/Geist-OFL.txt":                                reference + "crates/ui/assets/fonts/licenses/Geist-OFL.txt",
	"fonts/Geist-NOTICE.md":                              reference + "crates/ui/assets/fonts/licenses/THIRD_PARTY_NOTICES.md",
	"fonts/GUST-font-metadata.json":                      "verbatim sfnt name-table fields from the two reference GUST OTFs; paths and font hashes recorded inside",
	"fonts/LPPL-1.3c.txt":                                "https://www.latex-project.org/lppl/lppl-1-3c.txt",
	"icons/CC-BY-4.0.txt":                                "https://creativecommons.org/licenses/by/4.0/legalcode.txt",
	"icons/Solar-ATTRIBUTION.md":                         reference + "crates/ui/src/icons.rs attribution, with replica drawing changes stated",
	"icons/Symbols-LICENSE.txt":                          reference + "crates/ui/assets/file-icons/LICENSE.symbols",
	"themes/Zeron-theme-adaptations-NOTICES.md":          reference + "THIRD_PARTY_NOTICES.md, complete Bundled theme palette adaptations section",
	"themes/iTerm2-Color-Schemes-MIT.txt":                "github.com/egoist/mygo@v0.7.3/plugins/terminal/ghostty_themes.go, original complete license comment with comment markers removed",
	"go/Go-runtime-BSD-3-Clause.txt":                     "go1.27.3 selected toolchain/LICENSE",
	"go/purego-Apache-2.0.txt":                           "github.com/ebitengine/purego@v0.11.1/LICENSE (local module cache)",
	"go/go-text-typesetting-LICENSE.txt":                 "github.com/go-text/typesetting@v0.3.5/LICENSE (local module cache)",
	"go/go-text-harfbuzz-MIT.txt":                        "github.com/go-text/typesetting@v0.3.5/harfbuzz/LICENSE (local module cache)",
	"go/go-text-typesetting-utils-test-only-LICENSE.txt": "github.com/go-text/typesetting-utils@v0.0.0-20260419141703-4ffe8874dabc/LICENSE (local module cache; test only)",
	"go/x-image-BSD-3-Clause.txt":                        "golang.org/x/image@v0.46.0/LICENSE (local module cache)",
	"go/x-net-BSD-3-Clause.txt":                          "golang.org/x/net@v0.59.0/LICENSE (local module cache)",
	"go/x-text-BSD-3-Clause.txt":                         "golang.org/x/text@v0.43.0/LICENSE (local module cache)",
	"native/Ghostty-MIT.txt":                             ghostty + "LICENSE",
	"native/ghostty-vt-manifest.json":                    "github.com/egoist/mygo@v0.7.3/plugins/terminal/mygo-plugin.json (verbatim local source)",
	"native/PROVENANCE.md":                               "project-maintained source inspection record; pinned Ghostty build source paths recorded inside",
	"native/uucode-LICENSE.md":                           uucode + "LICENSE.md",
	"native/uucode-Bjoern-Hoehrmann-MIT.txt":             uucode + "licenses/LICENSE_Bjoern_Hoehrmann",
	"native/uucode-Unicode.txt":                          uucode + "licenses/LICENSE_unicode",
	"native/Highway-Apache-2.0.txt":                      "https://raw.githubusercontent.com/google/highway/66486a10623fa0d72fe91260f96c892e41aceb06/LICENSE",
	"native/simdutf-9.0.0-MIT.txt":                       "https://raw.githubusercontent.com/simdutf/simdutf/v9.0.0/LICENSE-MIT",
	"native/simdutf-9.0.0-Apache-2.0.txt":                "https://raw.githubusercontent.com/simdutf/simdutf/v9.0.0/LICENSE-APACHE",
	"native/Wuffs-MIT.txt":                               wuffs + "LICENSE-MIT",
	"native/Wuffs-Apache-2.0.txt":                        wuffs + "LICENSE-APACHE",
	"renderers/PROVENANCE.md":                            "project-maintained vector generation record, based on frozen reference source and artifact manifests",
	"renderers/mermaid-rs-renderer-0.3.1-MIT.txt":        "mermaid-rs-renderer-0.3.1/LICENSE (local Cargo registry)",
	"renderers/usvg-0.46.0-MIT.txt":                      "usvg-0.46.0/LICENSE-MIT (local Cargo registry)",
	"renderers/usvg-0.46.0-Apache-2.0.txt":               "usvg-0.46.0/LICENSE-APACHE (local Cargo registry)",
	"renderers/fontdb-0.23.0-MIT.txt":                    "fontdb-0.23.0/LICENSE (local Cargo registry)",
	"renderers/ttf-parser-0.25.1-MIT.txt":                "ttf-parser-0.25.1/LICENSE-MIT (local Cargo registry)",
	"renderers/ttf-parser-0.25.1-Apache-2.0.txt":         "ttf-parser-0.25.1/LICENSE-APACHE (local Cargo registry)",
	"renderers/rustybuzz-0.20.1-MIT.txt":                 "rustybuzz-0.20.1/LICENSE (local Cargo registry)",
	"artwork/image-thumbnail-MIT.txt":                    "internal/assets/data/artifacts/notices/image-thumbnail.MIT (verbatim supplied source text)",
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	root := filepath.Join("resources", "licenses")
	manifest := licenseresources.Manifest{SchemaVersion: 1}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("license symlink: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "manifest.json" {
			return nil
		}
		origin := origins[rel]
		if strings.HasPrefix(rel, "fonts/OFL-") || rel == "fonts/GUST-FONT-LICENSE.txt" {
			origin = reference + "crates/zatex/fonts/" + filepath.Base(rel)
		}
		if origin == "" {
			return fmt.Errorf("license resource has no reviewed source mapping: %s", rel)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(data)
		manifest.Files = append(manifest.Files, licenseresources.File{Path: rel, Bytes: int64(len(data)), SHA256: hex.EncodeToString(digest[:]), Source: origin})
		return nil
	})
	if err != nil {
		return err
	}
	notice, err := os.ReadFile(filepath.Join("resources", "THIRD_PARTY_NOTICES.md"))
	if err != nil {
		return err
	}
	digest := sha256.Sum256(notice)
	manifest.NoticeSHA256 = hex.EncodeToString(digest[:])
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), append(data, '\n'), 0644); err != nil {
		return err
	}
	_, err = licenseresources.VerifyTree("resources")
	if err == nil {
		fmt.Printf("Inventoried %d reviewed license resources.\n", len(manifest.Files))
	}
	return err
}
