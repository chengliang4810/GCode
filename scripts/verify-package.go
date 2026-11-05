//go:build ignore

// Verify-package checks release binary architectures, pinned Go/MyGo metadata,
// terminal native libraries, and records SHA-256 for every delivered file.
package main

import (
	"archive/zip"
	"crypto/sha256"
	"debug/buildinfo"
	"debug/macho"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"gcode/scripts/distribution"
	"gcode/scripts/frameworkpin"
	"gcode/scripts/licenseresources"
)

type fileRecord struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}
type packageRecord struct {
	Target                string       `json:"target"`
	Executable            string       `json:"executable"`
	NativeLibrary         string       `json:"nativeLibrary"`
	Architectures         []string     `json:"architectures"`
	GoVersion             string       `json:"goVersion"`
	MyGoVersion           string       `json:"mygoVersion"`
	MyGoPatchRevision     string       `json:"mygoPatchRevision"`
	MyGoSourceSHA256      string       `json:"mygoSourceSha256"`
	NativeAcceptance      string       `json:"nativeAcceptance"`
	Files                 []fileRecord `json:"files"`
	Archive               string       `json:"archive"`
	ArchiveSHA256         string       `json:"archiveSha256"`
	ArchiveBytes          int64        `json:"archiveBytes"`
	LicenseFiles          int          `json:"licenseFiles"`
	CoexistenceAcceptance string       `json:"coexistenceAcceptance"`
}
type manifest struct {
	SchemaVersion   int                 `json:"schemaVersion"`
	ReferenceCommit string              `json:"referenceCommit"`
	BuiltAt         string              `json:"builtAt"`
	GitCommit       string              `json:"gitCommit"`
	GitDirty        bool                `json:"gitDirty"`
	Packages        []packageRecord     `json:"packages"`
	Framework       frameworkpin.Lock   `json:"framework"`
	Distribution    distribution.Config `json:"distribution"`
}

func main() {
	root := flag.String("root", "build", "Release directory")
	targets := flag.String("targets", "windows/amd64", "Comma-separated GOOS/GOARCH targets")
	flag.Parse()
	if err := run(*root, strings.Split(*targets, ",")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root string, targets []string) error {
	config, err := distribution.Load("mygo.json")
	if err != nil {
		return err
	}
	lock, err := frameworkpin.VerifySource(".")
	if err != nil {
		return err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}
	m := manifest{SchemaVersion: 1, ReferenceCommit: "ab189159f10cf8e63c0502be75a59bbc487e0eaf", BuiltAt: time.Now().UTC().Format(time.RFC3339), GitCommit: git("rev-parse", "HEAD"), GitDirty: git("status", "--porcelain") != ""}
	m.Framework = lock
	m.Distribution = config
	for _, target := range targets {
		r, err := verify(root, target, lock, config)
		if err != nil {
			return fmt.Errorf("%s: %w", target, err)
		}
		archive, archiveSize, archiveHash, err := archivePackage(root, r)
		if err != nil {
			return err
		}
		r.Archive = archive
		r.ArchiveBytes = archiveSize
		r.ArchiveSHA256 = archiveHash
		m.Packages = append(m.Packages, r)
		fmt.Printf("Verified %s: %s; native library %s; %d files\n", target, strings.Join(r.Architectures, ","), r.NativeLibrary, len(r.Files))
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	manifestName := "release-manifest-" + strings.ReplaceAll(strings.Join(targets, "_"), "/", "-") + ".json"
	return os.WriteFile(filepath.Join(root, manifestName), append(data, '\n'), 0644)
}

// Zip Unix mode bits are authored explicitly even on Windows. A .app copied from
// an ordinary Windows zip otherwise loses its executable bits on macOS.
func archivePackage(root string, p packageRecord) (string, int64, string, error) {
	var config struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	data, err := os.ReadFile("mygo.json")
	if err != nil {
		return "", 0, "", err
	}
	if err = json.Unmarshal(data, &config); err != nil {
		return "", 0, "", err
	}
	if config.Version == "" || strings.ContainsAny(config.Version, "/\\") {
		return "", 0, "", fmt.Errorf("invalid package version")
	}
	name := config.Name + "-" + config.Version + "-" + strings.ReplaceAll(p.Target, "/", "-") + ".zip"
	file, err := os.Create(filepath.Join(root, name))
	if err != nil {
		return "", 0, "", err
	}
	writer := zip.NewWriter(file)
	platformDir := filepath.Join(root, strings.ReplaceAll(p.Target, "/", "-"))
	for _, entry := range p.Files {
		if strings.Contains(entry.Path, " Setup ") {
			continue
		}
		if !strings.Contains(entry.Path, "/") &&
			(strings.HasSuffix(entry.Path, ".tar.gz") || strings.HasSuffix(entry.Path, ".delta") ||
				(strings.HasPrefix(entry.Path, "update-") && strings.HasSuffix(entry.Path, ".json"))) {
			continue
		}
		if err := func() error {
			source, err := os.Open(filepath.Join(platformDir, filepath.FromSlash(entry.Path)))
			if err != nil {
				return err
			}
			defer source.Close()
			header := &zip.FileHeader{Name: entry.Path, Method: zip.Deflate}
			mode := fs.FileMode(0644)
			if entry.Path == p.Executable || entry.Path == p.NativeLibrary {
				mode = 0755
			}
			header.SetMode(mode)
			header.SetModTime(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
			dest, err := writer.CreateHeader(header)
			if err != nil {
				return err
			}
			_, err = io.Copy(dest, source)
			return err
		}(); err != nil {
			_ = writer.Close()
			_ = file.Close()
			return "", 0, "", err
		}
	}
	if err = writer.Close(); err != nil {
		_ = file.Close()
		return "", 0, "", err
	}
	if err = file.Close(); err != nil {
		return "", 0, "", err
	}
	archive, err := os.Open(filepath.Join(root, name))
	if err != nil {
		return "", 0, "", err
	}
	defer archive.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, archive)
	if err != nil {
		return "", 0, "", err
	}
	return name, size, hex.EncodeToString(hash.Sum(nil)), nil
}

func git(args ...string) string {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "unavailable"
	}
	return strings.TrimSpace(string(out))
}

func verify(root, target string, lock frameworkpin.Lock, config distribution.Config) (packageRecord, error) {
	r := packageRecord{Target: target, NativeAcceptance: "pending"}
	r.MyGoPatchRevision = lock.Revision
	r.MyGoSourceSHA256 = lock.TreeSHA256
	r.CoexistenceAcceptance = "build/script checks only; native installation pending"
	parts := strings.Split(target, "/")
	if len(parts) != 2 {
		return r, fmt.Errorf("invalid target")
	}
	if target != "windows/amd64" && target != "darwin/amd64" && target != "darwin/arm64" && target != "darwin/universal" {
		return r, fmt.Errorf("unsupported target")
	}
	dir := filepath.Join(root, parts[0]+"-"+parts[1])
	exe, library := config.Executable(parts[0]), "ghostty-vt.dll"
	if parts[0] == "darwin" {
		if err := config.VerifyInfoPlist(filepath.Join(dir, config.BundleFile(), "Contents", "Info.plist")); err != nil {
			return r, err
		}
		library = "libghostty-vt.dylib"
	}
	exePath := filepath.Join(dir, exe)
	arch, err := architectures(exePath)
	if err != nil {
		return r, err
	}
	want := []string{parts[1]}
	if parts[1] == "universal" {
		want = []string{"amd64", "arm64"}
	}
	if strings.Join(arch, ",") != strings.Join(want, ",") {
		return r, fmt.Errorf("executable architectures %v, expected %v", arch, want)
	}
	r.Architectures = arch
	r.Executable = filepath.ToSlash(exe)
	var nativePath string
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if d.Name() == library {
			if nativePath != "" {
				return fmt.Errorf("duplicate terminal native library")
			}
			nativePath = p
			r.NativeLibrary = filepath.ToSlash(rel)
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		h := sha256.New()
		n, err := io.Copy(h, f)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		r.Files = append(r.Files, fileRecord{Path: filepath.ToSlash(rel), Bytes: n, SHA256: hex.EncodeToString(h.Sum(nil))})
		return nil
	})
	if err != nil {
		return r, err
	}
	if nativePath == "" {
		return r, fmt.Errorf("missing bundled %s; packaged terminals must not download a native library", library)
	}
	nativeArch, err := architectures(nativePath)
	if err != nil {
		return r, err
	}
	if strings.Join(nativeArch, ",") != strings.Join(want, ",") {
		return r, fmt.Errorf("terminal library architectures %v, expected %v", nativeArch, want)
	}
	infos, err := binaryBuildInfo(exePath)
	if err != nil {
		return r, err
	}
	if err := verifyFrameworkStamp(exePath, lock); err != nil {
		return r, err
	}
	for _, info := range infos {
		if err := frameworkpin.VerifyBuildInfo(info, lock); err != nil {
			return r, err
		}
		r.GoVersion = info.GoVersion
		for _, dep := range info.Deps {
			if dep.Path == "github.com/egoist/mygo" {
				r.MyGoVersion = dep.Version
			}
		}
		if r.GoVersion != "go1.27.3" || r.MyGoVersion != lock.UpstreamVersion {
			return r, fmt.Errorf("unexpected toolchain/framework: %s / %s", r.GoVersion, r.MyGoVersion)
		}
	}
	sort.Slice(r.Files, func(i, j int) bool { return r.Files[i].Path < r.Files[j].Path })
	resourcesDir := config.Resources(dir, parts[0])
	r.LicenseFiles, err = licenseresources.VerifyCopy("resources", resourcesDir)
	if err != nil {
		return r, fmt.Errorf("license resource bundle: %w", err)
	}
	return r, nil
}

func verifyFrameworkStamp(name string, lock frameworkpin.Lock) error {
	if fat, err := macho.OpenFat(name); err == nil {
		defer fat.Close()
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		defer file.Close()
		for _, arch := range fat.Arches {
			data, err := io.ReadAll(io.NewSectionReader(file, int64(arch.Offset), int64(arch.Size)))
			if err != nil {
				return err
			}
			if err := frameworkpin.VerifyBinaryStamp(data, lock); err != nil {
				return fmt.Errorf("%s slice: %w", arch.Cpu, err)
			}
		}
		return nil
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	return frameworkpin.VerifyBinaryStamp(data, lock)
}

func binaryBuildInfo(name string) ([]*debug.BuildInfo, error) {
	if info, err := buildinfo.ReadFile(name); err == nil {
		return []*debug.BuildInfo{info}, nil
	}
	fat, err := macho.OpenFat(name)
	if err != nil {
		return nil, err
	}
	defer fat.Close()
	raw, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer raw.Close()
	var infos []*debug.BuildInfo
	for _, arch := range fat.Arches {
		info, err := buildinfo.Read(io.NewSectionReader(raw, int64(arch.Offset), int64(arch.Size)))
		if err != nil {
			return nil, fmt.Errorf("read %v build metadata: %w", arch.Cpu, err)
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func architectures(name string) ([]string, error) {
	if p, err := pe.Open(name); err == nil {
		defer p.Close()
		if p.Machine == pe.IMAGE_FILE_MACHINE_AMD64 {
			return []string{"amd64"}, nil
		}
		return nil, fmt.Errorf("unsupported PE machine %#x", p.Machine)
	}
	if f, err := macho.OpenFat(name); err == nil {
		defer f.Close()
		var result []string
		for _, a := range f.Arches {
			n, err := cpu(a.Cpu)
			if err != nil {
				return nil, err
			}
			result = append(result, n)
		}
		sort.Strings(result)
		return result, nil
	}
	f, err := macho.Open(name)
	if err != nil {
		return nil, fmt.Errorf("invalid PE/Mach-O file %s: %w", name, err)
	}
	defer f.Close()
	n, err := cpu(f.Cpu)
	if err != nil {
		return nil, err
	}
	return []string{n}, nil
}
func cpu(c macho.Cpu) (string, error) {
	switch c {
	case macho.CpuAmd64:
		return "amd64", nil
	case macho.CpuArm64:
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported Mach-O CPU %v", c)
	}
}
