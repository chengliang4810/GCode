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
	UpstreamVersion = "v0.8.3"
	Revision        = "rcode-native-ui.24"
	Replacement     = "./third_party/mygo"
	UpstreamSum     = "h1:aTLEhS221YhsX+gKGNLQ5xEKMLCcYYr3pH9AkTwIRs4="

	ExpectedTreeSHA256 = "e32652f3fe5699ea3c1e40a8fe4b832e96f1b9e6824f70bf7768bd0cc8760926"
)

// patchChain lists the reviewed local patches in application order over
// upstream v0.8.3, ending with the pinned framework identity.
var patchChain = []Patch{
	{Path: "patches/mygo-v0.8.3-01-motion.patch", SHA256: "1b660b425c2c9c81861fdbd8566eaa6147a3d1702dc402172fa88a28abdc0c6f"},
	{Path: "patches/mygo-v0.8.3-02-markdown-line-height.patch", SHA256: "d9b86e5da49137a92beeeed104bffed7de1990ffe2cc6c8cf1dcde91624c8b6a"},
	{Path: "patches/mygo-v0.8.3-03-snap-to-window.patch", SHA256: "fef1b3beaca20591c44fcf5fb0b175411746701b0d50eac83a78468017b42797"},
	{Path: "patches/mygo-v0.8.3-04-input-display-text.patch", SHA256: "cb2463648088d31a154ca28e20e60e445a0813be392d5960fd5ff2899738768e"},
	{Path: "patches/mygo-v0.8.3-05-windows-uia-provider.patch", SHA256: "79912ce654148454156d4096943164fa2b90c6fa467fdd22242e9e2f3a5d434a"},
	{Path: "patches/mygo-v0.8.3-06-richtext-composer.patch", SHA256: "4d2e8d33adb348eccfc748959e9feef789e7881556341fee5b6699f47713c0c7"},
	{Path: "patches/mygo-v0.8.3-08-inline-link-selection.patch", SHA256: "a9bebe568bbe18dd1a6714901711cb54cbd8e976f6ed476085eef200b66e15f7"},
	{Path: "patches/mygo-v0.8.3-09-collapsible-animation.patch", SHA256: "abe31d972b3be24b21fd3c6b23192fca79dee1d9966ca8270658040ff412ec4e"},
	{Path: "patches/mygo-v0.8.3-10-webview-data-directory.patch", SHA256: "cd468a33731157c7d322d06ee592c7d0ecf964da181c58bcbd12cb21bc0c9228"},
	{Path: "patches/mygo-v0.8.3-11-windows-glyph-placement.patch", SHA256: "ac75b277499d0e8818caea72c075713999d65bb7a3cab665ae94d166a0379fc7"},
	{Path: "patches/mygo-v0.8.3-12-rendering-parity.patch", SHA256: "c08be236ac8e0664b2fec31a3b089dbcafc790f7ad4fdbcaa565e590d0ca6933"},
	{Path: "patches/mygo-v0.8.3-13-tooltip-primitives.patch", SHA256: "0a0ef73ecb35446417a5ef87dc9a63397dddcc6f3f56c2f3c230cd3e8b9cc7bd"},
	{Path: "patches/mygo-v0.8.3-15-selection-spans.patch", SHA256: "5dc26170f679afd3488e414a3375bfe97a6f4ee79b8eed4508759348153674bf"},
	{Path: "patches/mygo-v0.8.3-17-api-gen.patch", SHA256: "6feac1fc63a4a7e1a37c34623646c992f873a916e49f231da2d2a9270ab6d21a"},
	{Path: "patches/mygo-v0.8.3-18-split-drag.patch", SHA256: "c21b2b2e181e513bf7acc9221067ba3fb487f2a7e00871f35b51c2158110a2c6"},
	{Path: "patches/mygo-v0.8.3-19-patch-identity.patch", SHA256: "fc348d7bbda69047b3b77a9d3d0f16b85d059f75bb90dafc35031710199926f2"},
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
