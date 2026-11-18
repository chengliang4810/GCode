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
	UpstreamVersion = "v0.9.1"
	Revision        = "rcode-native-ui.28"
	Replacement     = "./third_party/mygo"
	UpstreamSum     = "h1:vzMjSyahUjSk62+n+qmNXt8Bamyx1UNz359OyEDo/kg="

	ExpectedTreeSHA256 = "72a46929fae6eb7fbc8fc5ac6cb80fc850605f2930748fab309936e366f7f1d4"
)

// patchChain lists the reviewed local patches in application order over
// upstream v0.9.1, ending with the pinned framework identity.
var patchChain = []Patch{
	{Path: "patches/mygo-v0.9.1-01-motion.patch", SHA256: "1f9c2963b066917fb5953b5b13f5b471f62b1b6bed60e29964a14bb81b5ca414"},
	{Path: "patches/mygo-v0.9.1-02-markdown-line-height.patch", SHA256: "b699084b03da05e5d964527c6737b75166a9e6bec0bd0c8a511722ed48872d51"},
	{Path: "patches/mygo-v0.9.1-03-snap-to-window.patch", SHA256: "92846ab187996d26464cb54e87cf75519259afe8b467220f76ea10ef466cc00f"},
	{Path: "patches/mygo-v0.9.1-04-input-display-text.patch", SHA256: "002eabf55601dcd72eace90358887f4c2b069bd116c87301df0f80a36b399046"},
	{Path: "patches/mygo-v0.9.1-05-windows-uia-provider.patch", SHA256: "49a62d232e4689b6ffea50e8380b5177a7283d55e3700dee2a6a340f6c3fc35e"},
	{Path: "patches/mygo-v0.9.1-06-richtext-composer.patch", SHA256: "4409cffee820c56ef2141aab3b4731a48f0551d3485bb1b3f2a106753a1643de"},
	{Path: "patches/mygo-v0.9.1-08-inline-link-selection.patch", SHA256: "2ee3d2a34c27c3af79eb09023f3b3b6c023a9d3b791d257d6cc43d483a6927e1"},
	{Path: "patches/mygo-v0.9.1-09-collapsible-animation.patch", SHA256: "1445ade8f3e8455789f409bfe92ea29b8ef9eb377e1ca8a984f389f3be6f779e"},
	{Path: "patches/mygo-v0.9.1-10-webview-data-directory.patch", SHA256: "68e7e5e5340120018bc9cff946fd323cc2f5c70472d1aec8061f43a7a1220007"},
	{Path: "patches/mygo-v0.9.1-11-windows-glyph-placement.patch", SHA256: "38b1ac0e50e25a162ebc42220351a36e32e8393ed3956e9f6a63b4499005fe43"},
	{Path: "patches/mygo-v0.9.1-12-rendering-parity.patch", SHA256: "abef284990cefee5421004dd86826c9729e49184651b9246de6c1dedd6b9cc43"},
	{Path: "patches/mygo-v0.9.1-13-tooltip-primitives.patch", SHA256: "4e04cc9752589aea4986978dc247e45eec09b304f8f773a24dad341dadde3b3a"},
	{Path: "patches/mygo-v0.9.1-15-selection-spans.patch", SHA256: "7c4b408e03ac91131e01fc9c38bc709b9fbca2dafd8815de262e7f4d6d3e1a86"},
	{Path: "patches/mygo-v0.9.1-18-split-drag.patch", SHA256: "7df10a3892e503a2ffe1615cd760ab86adf26be8ffc37afbac7466008d7068dd"},
	{Path: "patches/mygo-v0.9.1-20-richtext-clip.patch", SHA256: "ae1ecd5ef1e8264a5ab0a36ece2e409e882bd04b7dd1ef7a5fbc0d416e2ac40b"},
	{Path: "patches/mygo-v0.9.1-21-inline-code-size.patch", SHA256: "471651d5f68beb08e256739ef45f55e9138d0153d716054f7ab3e476bd7ee3fc"},
	{Path: "patches/mygo-v0.9.1-22-diff-no-separators.patch", SHA256: "185b9346da60e25da45286e12a8113fbb58a0718f2c6399a1aa045968b5c4660"},
	{Path: "patches/mygo-v0.9.1-17-api-gen.patch", SHA256: "e35b0c35d5bcffaa97cc0c7a97530bf7c742a0642f48516f9979578c194ea1e9"},
	{Path: "patches/mygo-v0.9.1-19-patch-identity.patch", SHA256: "33f79bc8a799adb0256414932390292c93957a725e5076671395179e283cc9f2"},
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
