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

	"gcode/internal/processutil"
)

const (
	ModulePath      = "github.com/egoist/mygo"
	UpstreamVersion = "v0.7.4"
	Revision        = "rcode-native-ui.22"
	Replacement     = "./third_party/mygo"
	UpstreamSum     = "h1:0qkDpDRo7C9g8PM8eu4V6FVouhLiTrUdqKD6I31Lv3Q="

	ExpectedTreeSHA256 = "c90ff414c0fea3d4e69035714777eae43bd875320ea22746894b1507075b069e"
)

// patchChain lists the reviewed local patches in application order over
// upstream v0.7.4, ending with the pinned framework identity.
var patchChain = []Patch{
	{Path: "patches/mygo-v0.7.4-01-motion.patch", SHA256: "00bc4df53a8d09fd6c3d86ec355d4e9511bdc1f826d1d7f4a2538206d69bfee9"},
	{Path: "patches/mygo-v0.7.4-02-hover.patch", SHA256: "2c97f70f4a2af100f98fd6e6a47e69f3f6f112f86559b5c5d801f2db7e9e6cc0"},
	{Path: "patches/mygo-v0.7.4-03-files-drop.patch", SHA256: "a9a67762458cf3d0a8bc583b6def17dea5cdf4c0a7f02701724f80aea9b785dc"},
	{Path: "patches/mygo-v0.7.4-04-markdown-line-height.patch", SHA256: "ed6bb258ed3c18ce0c3815e07800562394155ebb5ccea0b117a95b8f03ea2122"},
	{Path: "patches/mygo-v0.7.4-05-snap-to-window.patch", SHA256: "286b38b9c507a2c466d56abf2725ad6569ddd447e3d759d865128dce8299a0f4"},
	{Path: "patches/mygo-v0.7.4-06-input-display-text.patch", SHA256: "dcf89118115063b7b67b54bd59aa87a3a4cb7c412d71ea61c403eabc9aaef32e"},
	{Path: "patches/mygo-v0.7.4-07-windows-uia-provider.patch", SHA256: "f5100475ebe4bae91d39f1206e18867627c52762f769b6d22c2aca620f317dea"},
	{Path: "patches/mygo-v0.7.4-08-richtext-composer.patch", SHA256: "1abc0f07c276177cfbeb0352bb51abce94526ec9aed93a24324cefbbc9bbc068"},
	{Path: "patches/mygo-v0.7.4-09-richtext-usejustify.patch", SHA256: "95841a7f9e7f1dd5825c155612e08f1bf152372ab2de68ddfe8143287de91179"},
	{Path: "patches/mygo-v0.7.4-10-inline-link-selection.patch", SHA256: "4075d8d8679bd996b4de4c5ada01e1f2883210f7fce30d48362f14f633a69c8b"},
	{Path: "patches/mygo-v0.7.4-11-collapsible-animation.patch", SHA256: "1df6af319603983972eda4d5c1c17b8902b17a3e8dac6889a1f196c3413b3a85"},
	{Path: "patches/mygo-v0.7.4-12-webview-data-directory.patch", SHA256: "9b1fa00b65b9b0f10a0a9508ac7e1e47f0b07ddb6d5adab78c5308cb219120fa"},
	{Path: "patches/mygo-v0.7.4-13-windows-glyph-placement.patch", SHA256: "f9dc82c0f431af897d338a618d255317a15b13936896a48ff7cb69672419dcc3"},
	{Path: "patches/mygo-v0.7.4-14-rendering-parity.patch", SHA256: "7250a06873ee2c377a1b740353069087c0d5eff82fd2bb11995c1f0ca22e911b"},
	{Path: "patches/mygo-v0.7.4-15-tooltip-primitives.patch", SHA256: "4086a0bbfa25732eb447f542bfe9073f5fab04687dc2e1e8ed6a20688e3065f5"},
	{Path: "patches/mygo-v0.7.4-16-uia-benchmark-close.patch", SHA256: "74c556efa2a7eb6ee521b7972ed4a3ec2e4941939278a45d6ffd84c530fe0bac"},
	{Path: "patches/mygo-v0.7.4-17-selection-spans.patch", SHA256: "77ddf7555a3020a1126702429fc5b6b1dba8572b9841e58bc9d99da7d4fdd0b6"},
	{Path: "patches/mygo-v0.7.4-18-code-block-tabs.patch", SHA256: "44cf49599851e63b51ff882a7037788412c2e8f8240e9a83cee53b046a3f2791"},
	{Path: "patches/mygo-v0.7.4-19-api-gen.patch", SHA256: "13f051e92335ca6fb5813b0cf2ae89652dd2f154f232143f24ff1217bac7395d"},
	{Path: "patches/mygo-v0.7.4-20-split-drag.patch", SHA256: "062e68b390d4f73ccd885e7154542b6bc2e885909068d5f4c74427edae095f17"},
	{Path: "patches/mygo-v0.7.4-21-patch-identity.patch", SHA256: "599f68e70102dad24525f5b6876ab4deadaec614b73b566b4d88cb968ca311ee"},
}

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
	wantPatches := patchChain
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
	processutil.HideConsole(cmd)
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
	return "RCODE_MYGO_PATCH[" + UpstreamVersion + "|" + lock.Revision + "|" + lock.TreeSHA256 + "]"
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
