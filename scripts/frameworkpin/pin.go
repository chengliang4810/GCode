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
	UpstreamVersion = "v0.7.3"
	Revision        = "rcode-native-ui.21"
	Replacement     = "./third_party/mygo"
	UpstreamSum     = "h1:Jch8vV8ZzlHHr6hABg+aOx9nFbdjZ2oD/abY5Cf5/0I="

	ExpectedTreeSHA256 = "ed9afb70ef97e138936f67058db29be585c36de836a7b3529545716f462cf22f"
)

// patchChain lists the reviewed local patches in application order over
// upstream v0.7.3, ending with the pinned framework identity.
var patchChain = []Patch{
	{Path: "patches/mygo-v0.7.3-01-motion.patch", SHA256: "8614bce7424754d9192680dfb97bdcfef998ba206403bf76cce5164598e78965"},
	{Path: "patches/mygo-v0.7.3-02-hover.patch", SHA256: "bd3426b2c94a8a8fd81e71c751206199e19a604349e4b29ec41c29baa13c3d9e"},
	{Path: "patches/mygo-v0.7.3-03-files-drop.patch", SHA256: "660630908cf5f046fb2fe63c4972294dd154ebdc2180617b37a83bdf28eb868a"},
	{Path: "patches/mygo-v0.7.3-04-markdown-line-height.patch", SHA256: "39b2ceb9501f671e8fb952484462d0d48ccbcc4b071a24e0bdb86505db319c3b"},
	{Path: "patches/mygo-v0.7.3-05-snap-to-window.patch", SHA256: "b40f645a52a1abd9732c0099135f90cd16a643057aca1a20bff9bc806200f03f"},
	{Path: "patches/mygo-v0.7.3-06-placeholder-color.patch", SHA256: "a112991b5e77d0c9f0d447b6f636c55fc095d3bd0c204423c4630a024aad74cc"},
	{Path: "patches/mygo-v0.7.3-07-input-display-text.patch", SHA256: "0c6af413ebc7c707d303c40b211e129e0ac2fa0cddd53d268463642d5bd36159"},
	{Path: "patches/mygo-v0.7.3-08-caret-width.patch", SHA256: "ab8bf37b9fb3d4b85fa37d1335f4619d1171b3a7b0ff28142009fdf15353b9e4"},
	{Path: "patches/mygo-v0.7.3-09-windows-uia-provider.patch", SHA256: "f5100475ebe4bae91d39f1206e18867627c52762f769b6d22c2aca620f317dea"},
	{Path: "patches/mygo-v0.7.3-10-scrollbars.patch", SHA256: "bc86a5709a2a5add9e4f8a7b7fd67cee34b266c1f20b4c340635ddf37f8d0bcf"},
	{Path: "patches/mygo-v0.7.3-11-notification-context.patch", SHA256: "6fd30d98ba8fde4a3f7b7b5600170aa0244be75f8b072f56e925bfb3d472a474"},
	{Path: "patches/mygo-v0.7.3-12-richtext-composer.patch", SHA256: "5ec64eb8dc52b10e731123e15d52effe2af1dcf0b59a228df1e16c7e673e647c"},
	{Path: "patches/mygo-v0.7.3-13-richtext-usejustify.patch", SHA256: "95841a7f9e7f1dd5825c155612e08f1bf152372ab2de68ddfe8143287de91179"},
	{Path: "patches/mygo-v0.7.3-14-updater-availability.patch", SHA256: "f17caf1a42bef853e282f2ba083962e150f16493f9e3f27a6bb69dca699c43de"},
	{Path: "patches/mygo-v0.7.3-15-markdown-anchor.patch", SHA256: "d54a1fdf93fbd5ce76d1b3ff367e891354989ccb4e3a0147929faabcfef382aa"},
	{Path: "patches/mygo-v0.7.3-16-inline-link-selection.patch", SHA256: "a84c36cafff5fec9db621c1326901293dbe99b1d02dec3d3f9f7fbf6e92c4472"},
	{Path: "patches/mygo-v0.7.3-17-collapsible-animation.patch", SHA256: "14196fa6d33b1981909f227ab7c7ac6eb951345c907fdc805bb8e01becdb6ab0"},
	{Path: "patches/mygo-v0.7.3-18-webview-data-directory.patch", SHA256: "c6c093db0a3b07ca48bf46db951a8dd6125d18ccde5456a95b5795cecfaa24ae"},
	{Path: "patches/mygo-v0.7.3-19-package-stage-rename.patch", SHA256: "2b5006e196c36c17f8c69aad4a401f02ef04c93c52d969f3a907365388288bff"},
	{Path: "patches/mygo-v0.7.3-20-windows-glyph-placement.patch", SHA256: "c411b2b4f737fedcd4b86283b02774fd86053c5c30b6d031d229b5e8d8f53373"},
	{Path: "patches/mygo-v0.7.3-21-rendering-parity.patch", SHA256: "e06fbf864b3f560be74d0d95c930da75f2c8daee474ab733eec2fb60cee53163"},
	{Path: "patches/mygo-v0.7.3-22-tooltip-primitives.patch", SHA256: "687230a7bc5d16f1b6db1e52cef9210f6d7104f95652de0ffa76e95bba7d71d7"},
	{Path: "patches/mygo-v0.7.3-23-layoutcache-union-spacing.patch", SHA256: "71d079d2d8ba9f764985af0dcf466b60eb5d333083bb1e54a38be43b25d8bb6c"},
	{Path: "patches/mygo-v0.7.3-24-textclient-a11y-guard.patch", SHA256: "bd676acb9d3dd8ebbcc893ce6e28254a3fe9beba83c1f673c77cd2f25ed0eb49"},
	{Path: "patches/mygo-v0.7.3-25-uia-benchmark-close.patch", SHA256: "74c556efa2a7eb6ee521b7972ed4a3ec2e4941939278a45d6ffd84c530fe0bac"},
	{Path: "patches/mygo-v0.7.3-26-patch-identity-final.patch", SHA256: "fc5105bf8bed56393e31c58b272c549033da64b2c93f9c527018320e0a4cfd7e"},
	{Path: "patches/mygo-v0.7.3-27-selection-spans.patch", SHA256: "28672a79803648eb19ea0aa90793c9bd1e3a66d26db30df0d9b89c679d3cf33c"},
	{Path: "patches/mygo-v0.7.3-28-code-block-tabs.patch", SHA256: "cbc6de5f43c04f13cefb61ef08bf34d3c966da9461b163d454fb68f5c1694435"},
	{Path: "patches/mygo-v0.7.3-29-split-drag.patch", SHA256: "744f61dc6ebb599bfe0686c2d42a98084cd68475be9fc64e8240781463910c9c"},
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
