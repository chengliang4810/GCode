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
	UpstreamVersion = "v0.10.0"
	Revision        = "rcode-native-ui.29"
	Replacement     = "./third_party/mygo"
	UpstreamSum     = "h1:HpLUrlh6WES+4VYbz5K0TvUSw/v8PSdjT/ttiUYrBFM="

	ExpectedTreeSHA256 = "73c54a5b1c3621a17ab2628b7586ca3bd91b5a4ddfcfbf4aaea22cd984e7e39c"
)

// patchChain lists the reviewed local patches in application order over
// upstream v0.10.0, ending with the pinned framework identity.
var patchChain = []Patch{
	{Path: "patches/mygo-v0.10.0-01-motion.patch", SHA256: "fdfa101eaac352e6e49d07091f28b7cec5aca1e7464407ce81c3e4b12433214e"},
	{Path: "patches/mygo-v0.10.0-02-markdown-line-height.patch", SHA256: "5d587e187d156619cc635d9a47577a47e27c9d2b1334edd48556e192ccb6b216"},
	{Path: "patches/mygo-v0.10.0-03-snap-to-window.patch", SHA256: "79e72334b5e910a03e096b2028c150770fe19c7fd3715b242237ae55e0075e80"},
	{Path: "patches/mygo-v0.10.0-04-input-display-text.patch", SHA256: "430214914aff140385145ead2f2a0e868365b21b34c9615812d3c3e15b7e1793"},
	{Path: "patches/mygo-v0.10.0-05-windows-uia-provider.patch", SHA256: "dcddc35494c430485f44f1619dc5a604dcb8d09ab1a5fa008a8c9bf97eaf2c03"},
	{Path: "patches/mygo-v0.10.0-06-richtext-composer.patch", SHA256: "0160c002c481e823266aea08b3b001ec55012fc39161761ba4f2d4bea2672796"},
	{Path: "patches/mygo-v0.10.0-08-inline-link-selection.patch", SHA256: "9aca7d6cce6da17630773e08bf6c440786ce4d1453ec298ec1953cd53865ef5e"},
	{Path: "patches/mygo-v0.10.0-09-collapsible-animation.patch", SHA256: "abe31d972b3be24b21fd3c6b23192fca79dee1d9966ca8270658040ff412ec4e"},
	{Path: "patches/mygo-v0.10.0-10-webview-data-directory.patch", SHA256: "989e5056465f9b8912bb4d37a8f391af94367778caac5cadb7eb9fd46cd95425"},
	{Path: "patches/mygo-v0.10.0-11-windows-glyph-placement.patch", SHA256: "1ed6e629a23804b73e273b5210265a1a560bde29187bcb437203aeae30da2c8f"},
	{Path: "patches/mygo-v0.10.0-12-rendering-parity.patch", SHA256: "81726a4eefb9c7219c07497992db391af4f966d2c9dc96ccd04eefc9ec9a324f"},
	{Path: "patches/mygo-v0.10.0-13-tooltip-primitives.patch", SHA256: "a9acb5b134ef80b47fe9ac33032e68c0e816c4774beb3f29ccba21e3b0623007"},
	{Path: "patches/mygo-v0.10.0-15-selection-spans.patch", SHA256: "50758406f32973315eb28ebed0356f01bd8098f3607609293e1352ba5a0818c5"},
	{Path: "patches/mygo-v0.10.0-18-split-drag.patch", SHA256: "f4a263c0540a0fbf14c1a66de22bd47e93e62a932e1d7e1117af5d6e35a55fbb"},
	{Path: "patches/mygo-v0.10.0-20-richtext-clip.patch", SHA256: "afe6898e728edf8d0723616b0b8ae36582269ff556893ef2ec477efdf4921caf"},
	{Path: "patches/mygo-v0.10.0-21-inline-code-size.patch", SHA256: "ae04a8c29c47bd8185eb43b6e3fae2beb16ac876d304b37582f970ee85212d91"},
	{Path: "patches/mygo-v0.10.0-22-diff-no-separators.patch", SHA256: "0592793453f3bca868b01f17796bd0fb86ebc3b34de99b666905b279763011c0"},
	{Path: "patches/mygo-v0.10.0-23-macos-dmg-name.patch", SHA256: "f85712a1e8f8f796d15db363cc46cac123982c73470e361c66cfebf217ac82ff"},
	{Path: "patches/mygo-v0.10.0-17-api-gen.patch", SHA256: "ecb27cd67811adb327a0e21fce6c1dcf62f9f2ef77e72887ca32d1057a1e754e"},
	{Path: "patches/mygo-v0.10.0-19-patch-identity.patch", SHA256: "a9de6ab17518774c6b4cbccc2ab72d2ef2e608c7ddf78e4275027663b4ed83e9"},
	{Path: "patches/mygo-v0.10.0-24-list-state-a11y-guard.patch", SHA256: "374af148dc024bf82fe0b916f412684963747b0fa4cbee7d64678d17b1dbbe29"},
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
