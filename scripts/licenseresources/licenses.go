// Package licenseresources checks the offline license bundle shipped by MyGo.
package licenseresources

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type File struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
	Source string `json:"source"`
}

type Manifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	NoticeSHA256  string `json:"noticeSha256"`
	Files         []File `json:"files"`
}

var required = []string{
	"README.md", "OPEN-ITEMS.md", "zeron/LICENSE.txt", "mygo/LICENSE.txt",
	"fonts/Geist-OFL.txt", "fonts/Geist-NOTICE.md", "fonts/OFL-FiraMath.txt",
	"fonts/OFL-STIXTwoMath.txt", "fonts/OFL-LibertinusMath.txt", "fonts/OFL-EulerMath.txt",
	"fonts/GUST-FONT-LICENSE.txt", "fonts/LPPL-1.3c.txt", "fonts/GUST-font-metadata.json",
	"icons/CC-BY-4.0.txt", "icons/Solar-ATTRIBUTION.md", "icons/Symbols-LICENSE.txt",
	"themes/Zeron-theme-adaptations-NOTICES.md", "themes/iTerm2-Color-Schemes-MIT.txt",
	"go/Go-runtime-BSD-3-Clause.txt", "go/purego-Apache-2.0.txt", "go/go-text-typesetting-LICENSE.txt",
	"go/go-text-harfbuzz-MIT.txt", "go/x-image-BSD-3-Clause.txt", "go/x-net-BSD-3-Clause.txt", "go/x-text-BSD-3-Clause.txt",
	"go/Go-runtime-PATENTS.txt", "go/x-image-PATENTS.txt", "go/x-net-PATENTS.txt", "go/x-text-PATENTS.txt",
	"native/Ghostty-MIT.txt", "native/ghostty-vt-manifest.json", "native/PROVENANCE.md",
	"native/uucode-LICENSE.md", "native/uucode-Bjoern-Hoehrmann-MIT.txt", "native/uucode-Unicode.txt",
	"native/Highway-Apache-2.0.txt", "native/simdutf-9.0.0-MIT.txt", "native/simdutf-9.0.0-Apache-2.0.txt",
	"native/Wuffs-MIT.txt", "native/Wuffs-Apache-2.0.txt",
	"renderers/PROVENANCE.md", "renderers/mermaid-rs-renderer-0.3.1-MIT.txt",
	"renderers/usvg-0.46.0-MIT.txt", "renderers/usvg-0.46.0-Apache-2.0.txt", "renderers/fontdb-0.23.0-MIT.txt",
	"renderers/ttf-parser-0.25.1-MIT.txt", "renderers/ttf-parser-0.25.1-Apache-2.0.txt", "renderers/rustybuzz-0.20.1-MIT.txt",
	"artwork/image-thumbnail-MIT.txt",
}

// VerifyTree validates license files plus THIRD_PARTY_NOTICES.md in a resource
// directory. It verifies packaging integrity, not resolution of OPEN-ITEMS.md.
func VerifyTree(resources string) (Manifest, error) {
	var manifest Manifest
	root := filepath.Join(resources, "licenses")
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return manifest, fmt.Errorf("license manifest: %w", err)
	}
	if err = json.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}
	if manifest.SchemaVersion != 1 {
		return manifest, fmt.Errorf("unsupported license manifest schema")
	}
	seen := make(map[string]bool, len(manifest.Files))
	for _, record := range manifest.Files {
		if !fs.ValidPath(record.Path) || strings.ContainsAny(record.Path, "\\:") || record.Path == "manifest.json" || seen[record.Path] || record.Source == "" || record.Bytes < 1 {
			return manifest, fmt.Errorf("invalid license manifest entry: %q", record.Path)
		}
		seen[record.Path] = true
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(record.Path)))
		if err != nil {
			return manifest, fmt.Errorf("required license %s: %w", record.Path, err)
		}
		digest := sha256.Sum256(data)
		if int64(len(data)) != record.Bytes || hex.EncodeToString(digest[:]) != record.SHA256 {
			return manifest, fmt.Errorf("license checksum mismatch: %s", record.Path)
		}
	}
	for _, name := range required {
		if !seen[name] {
			return manifest, fmt.Errorf("required license missing from inventory: %s", name)
		}
	}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("license symlink is not permitted: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel != "manifest.json" && !seen[rel] {
			return fmt.Errorf("uninventoried license resource: %s", rel)
		}
		return nil
	})
	if err != nil {
		return manifest, err
	}
	notice, err := os.ReadFile(filepath.Join(resources, "THIRD_PARTY_NOTICES.md"))
	if err != nil {
		return manifest, fmt.Errorf("release notices missing: %w", err)
	}
	digest := sha256.Sum256(notice)
	if len(notice) == 0 || hex.EncodeToString(digest[:]) != manifest.NoticeSHA256 {
		return manifest, fmt.Errorf("release notices checksum mismatch")
	}
	return manifest, nil
}

// VerifyCopy confirms that the delivered resource tree contains the complete
// checked source bundle, including an identical inventory and release notice.
func VerifyCopy(sourceResources, deliveredResources string) (int, error) {
	manifest, err := VerifyTree(sourceResources)
	if err != nil {
		return 0, err
	}
	if _, err := VerifyTree(deliveredResources); err != nil {
		return 0, err
	}
	name := filepath.Join("licenses", "manifest.json")
	source, err := os.ReadFile(filepath.Join(sourceResources, name))
	if err != nil {
		return 0, err
	}
	delivered, err := os.ReadFile(filepath.Join(deliveredResources, name))
	if err != nil {
		return 0, err
	}
	if !bytes.Equal(source, delivered) {
		return 0, fmt.Errorf("release license inventory differs from checked source")
	}
	return len(manifest.Files), nil
}
