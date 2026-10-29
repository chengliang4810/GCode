// Package frameworkpin verifies the only permitted local framework replacement.
package frameworkpin

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
)

const (
	ModulePath                    = "github.com/egoist/mygo"
	UpstreamVersion               = "v0.6.4"
	Revision                      = "zeron-native-ui.9"
	Replacement                   = "./third_party/mygo"
	UpstreamSum                   = "h1:VzCqZKgvXcym5cSYKoC6IkTgcVeGZquPiui/a3oE36o="
	ExpectedTreeSHA256            = "27e5b9c724175bd11d537765210727fe25722a3407989e7fdcae71fa4357e8d9"
	MotionPatchSHA256             = "8fa72a733547c893bfd7c24e015523ef08a19dd5262de4bc7403a38eba03606f"
	HoverPatchSHA256              = "093dc4b5bb0a78e7adfb7ba1d6332eaa1c6c3e5ab0b86e3b80d94e153e96fd9b"
	FilesDropPatchSHA256          = "216f0edada22d085830160f7af7cd2a222f4617e8fcaa2a5a73b104c88160a24"
	MarkdownPatchSHA256           = "8416b7be5bb94e5751068a33b78f9fc07351902945e81cb3fae891f7931c213a"
	CoexistencePatchSHA256        = "54e8631331fd2389eda4b7bc37eced6a54b5f8323be54d9176398c8c81556aea"
	InputDisplayPatchSHA256       = "10e6003b954b9d6038bf98a6e4e8ca731b785bb8d64c817578687821e37d4a46"
	SnapToWindowPatchSHA256       = "b34a00a524ae6ef04782f08cad9f023320b44ed689875f6df3c3c6b13949c6ca"
	PlaceholderPatchSHA256        = "d1b7ddaa6604b3b61c16b487d8cdccf4d165672961ab10dea07b0b69299ab60d"
	CaretWidthPatchSHA256         = "147cb61beb2fde6bdd3c7b0be01de7044535947609658cf9a6f96469486a1f98"
	UIAProviderPatchSHA256        = "5372a13aae75d6bb103bf7489cd9e5bfe28d4a099cd9eef9e550080ea323523e"
	ScrollbarsPatchSHA256         = "76473041ef62dc3e68681594d4840b364064c3403bd62fe4c60eb1891bc22b05"
	TooltipPrimitivesPatchSHA256  = "c0aacba0bf423852051fa61fa290214c72852d2dc655f7e0bd8c4f36603ff1bb"
	PackageStageRenamePatchSHA256 = "c73c539c94ccf9a2945d450fe12b3cf7a1d079c56ca3b51e14e17b52b9c37af0"
	GlyphPlacementPatchSHA256     = "8a463bc728654abeb1ae3461687a5d8512a69e77f45284640aec5fc0b097a893"
)

type Patch struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type Lock struct {
	SchemaVersion   int     `json:"schemaVersion"`
	Module          string  `json:"module"`
	UpstreamVersion string  `json:"upstreamVersion"`
	UpstreamSum     string  `json:"upstreamSum"`
	Revision        string  `json:"revision"`
	SourceDir       string  `json:"sourceDir"`
	TreeSHA256      string  `json:"treeSha256"`
	FileCount       int     `json:"fileCount"`
	Patches         []Patch `json:"patches"`
}

// TreeHash hashes sorted relative UTF-8 paths and their content digests. Raw
// bytes are intentional: .gitattributes prevents source newline conversion.
func TreeHash(root string) (string, int, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("framework symlink is not permitted: %s", path)
		}
		if !entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("non-regular framework file: %s", path)
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			paths = append(paths, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return "", 0, err
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return "", 0, err
		}
		digest := sha256.Sum256(data)
		fmt.Fprintf(h, "%s\x00%x\n", path, digest)
	}
	return hex.EncodeToString(h.Sum(nil)), len(paths), nil
}

// VerifySource checks the lock, all framework bytes, patch files and the actual
// module selected by Go. It does not accept an arbitrary replace or go.work.
func VerifySource(root string) (Lock, error) {
	var lock Lock
	data, err := os.ReadFile(filepath.Join(root, "third_party", "mygo.lock.json"))
	if err != nil {
		return lock, err
	}
	if err = json.Unmarshal(data, &lock); err != nil {
		return lock, err
	}
	if lock.SchemaVersion != 1 || lock.Module != ModulePath || lock.UpstreamVersion != UpstreamVersion || lock.UpstreamSum != UpstreamSum || lock.Revision != Revision || lock.SourceDir != "third_party/mygo" || lock.TreeSHA256 != ExpectedTreeSHA256 {
		return lock, fmt.Errorf("unexpected framework lock identity")
	}
	wantPatches := []Patch{
		{"patches/mygo-v0.6.4-motion.patch", MotionPatchSHA256},
		{"patches/mygo-v0.6.4-hover.patch", HoverPatchSHA256},
		{"patches/mygo-v0.6.4-files-drop.patch", FilesDropPatchSHA256},
		{"patches/mygo-v0.6.4-markdown-line-height.patch", MarkdownPatchSHA256},
		{"patches/mygo-v0.6.4-coexistence.patch", CoexistencePatchSHA256},
		{"patches/mygo-v0.6.4-input-display-text.patch", InputDisplayPatchSHA256},
		{"patches/mygo-v0.6.4-snap-to-window.patch", SnapToWindowPatchSHA256},
		{"patches/mygo-v0.6.4-placeholder-color.patch", PlaceholderPatchSHA256},
		{"patches/mygo-v0.6.4-caret-width.patch", CaretWidthPatchSHA256},
		{"patches/mygo-v0.6.4-windows-uia-provider.patch", UIAProviderPatchSHA256},
		{"patches/mygo-v0.6.4-scrollbars.patch", ScrollbarsPatchSHA256},
		{"patches/mygo-v0.6.4-tooltip-primitives.patch", TooltipPrimitivesPatchSHA256},
		{"patches/mygo-v0.6.4-package-stage-rename.patch", PackageStageRenamePatchSHA256},
		{"patches/mygo-v0.6.4-windows-glyph-placement.patch", GlyphPlacementPatchSHA256},
	}
	if len(lock.Patches) != len(wantPatches) {
		return lock, fmt.Errorf("unexpected framework patch list")
	}
	for i, patch := range lock.Patches {
		if patch != wantPatches[i] {
			return lock, fmt.Errorf("unexpected framework patch identity: %s", patch.Path)
		}
	}
	for _, patch := range lock.Patches {
		bytes, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(patch.Path)))
		if err != nil {
			return lock, err
		}
		digest := sha256.Sum256(bytes)
		if hex.EncodeToString(digest[:]) != patch.SHA256 {
			return lock, fmt.Errorf("framework patch checksum mismatch: %s", patch.Path)
		}
	}
	digest, count, err := TreeHash(filepath.Join(root, lock.SourceDir))
	if err != nil {
		return lock, err
	}
	if digest != lock.TreeSHA256 || count != lock.FileCount {
		return lock, fmt.Errorf("framework source fingerprint mismatch: %s (%d files)", digest, count)
	}
	var module struct {
		Path    string
		Version string
		Replace *struct{ Path, Version, Dir string }
	}
	cmd := exec.Command("go", "list", "-m", "-json", ModulePath)
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return lock, fmt.Errorf("resolve framework module: %w", err)
	}
	if err = json.Unmarshal(output, &module); err != nil {
		return lock, err
	}
	if module.Path != ModulePath || module.Version != UpstreamVersion || module.Replace == nil || filepath.ToSlash(module.Replace.Path) != Replacement || module.Replace.Version != "" {
		return lock, fmt.Errorf("framework must use %s %s => %s", ModulePath, UpstreamVersion, Replacement)
	}
	want, err := filepath.Abs(filepath.Join(root, lock.SourceDir))
	if err != nil {
		return lock, err
	}
	if filepath.Clean(module.Replace.Dir) != filepath.Clean(want) {
		return lock, fmt.Errorf("unexpected framework replacement directory: %s", module.Replace.Dir)
	}
	return lock, nil
}

func LinkerFlags(lock Lock) string {
	return "-X " + ModulePath + ".sourcePatchRevision=" + lock.Revision + " -X " + ModulePath + ".sourcePatchSHA256=" + lock.TreeSHA256 + " -X " + ModulePath + ".sourcePatchIdentity=" + BinaryStamp(lock)
}

func BinaryStamp(lock Lock) string {
	return "ZERON_MYGO_PATCH[" + UpstreamVersion + "|" + lock.Revision + "|" + lock.TreeSHA256 + "]"
}

// VerifyBinaryStamp works with -trimpath builds, which omit -ldflags from Go
// build settings. The stamped variable is retained by MyGo's init validation.
// Pass one PE or Mach-O slice at a time, including for a Universal application.
func VerifyBinaryStamp(binary []byte, lock Lock) error {
	if bytes.Count(binary, []byte(BinaryStamp(lock))) != 1 {
		return fmt.Errorf("missing or duplicated framework binary fingerprint")
	}
	return nil
}

// VerifyBuildInfo requires the exact replacement and validates linker settings
// when Go retains them. Also call VerifyBinaryStamp: -trimpath omits those
// settings, while the fingerprint remains in the executable's framework data.
func VerifyBuildInfo(info *debug.BuildInfo, lock Lock) error {
	if info.GoVersion != "go1.27.3" {
		return fmt.Errorf("unexpected Go version: %s", info.GoVersion)
	}
	found := false
	for _, dep := range info.Deps {
		if dep.Path != ModulePath {
			continue
		}
		found = true
		if dep.Version != UpstreamVersion || dep.Replace == nil || filepath.ToSlash(dep.Replace.Path) != Replacement || dep.Replace.Version != "(devel)" {
			return fmt.Errorf("binary does not use the pinned patched MyGo replacement")
		}
	}
	if !found {
		return fmt.Errorf("binary has no MyGo build metadata")
	}
	var ldflags string
	for _, setting := range info.Settings {
		if setting.Key == "-ldflags" {
			ldflags = setting.Value
		}
	}
	if ldflags == "" {
		return nil
	}
	for _, value := range []string{ModulePath + ".sourcePatchRevision=" + lock.Revision, ModulePath + ".sourcePatchSHA256=" + lock.TreeSHA256, ModulePath + ".sourcePatchIdentity=" + BinaryStamp(lock)} {
		key := strings.SplitN(value, "=", 2)[0] + "="
		found := false
		fields := strings.Fields(ldflags)
		for i := 0; i+1 < len(fields); i++ {
			if fields[i] == "-X" && fields[i+1] == value {
				found = true
			}
		}
		if !found || strings.Count(ldflags, key) != 1 {
			return fmt.Errorf("missing or ambiguous framework release stamp: %s", key)
		}
	}
	return nil
}
