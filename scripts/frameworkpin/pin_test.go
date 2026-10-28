package frameworkpin

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

func TestBuildInfoAcceptsOnlyFixedPatchedRelease(t *testing.T) {
	lock := Lock{Revision: Revision, TreeSHA256: ExpectedTreeSHA256}
	for _, name := range []string{"pinned", "unpatched", "other path", "other version", "unstamped", "wrong hash", "duplicate stamp", "hash suffix"} {
		t.Run(name, func(t *testing.T) {
			dep := &debug.Module{Path: ModulePath, Version: UpstreamVersion, Replace: &debug.Module{Path: Replacement, Version: "(devel)"}}
			flags := LinkerFlags(lock)
			switch name {
			case "unpatched":
				dep.Replace = nil
			case "other path":
				dep.Replace.Path = "../mygo"
			case "other version":
				dep.Version = "v0.6.5"
			case "unstamped":
				flags = "-s -w"
			case "wrong hash":
				flags = strings.ReplaceAll(flags, ExpectedTreeSHA256, strings.Repeat("0", 64))
			case "duplicate stamp":
				flags += " -X " + ModulePath + ".sourcePatchRevision=other"
			case "hash suffix":
				flags += "suffix"
			}
			info := &debug.BuildInfo{GoVersion: "go1.27.3", Deps: []*debug.Module{dep}, Settings: []debug.BuildSetting{{Key: "-ldflags", Value: flags}}}
			err := VerifyBuildInfo(info, lock)
			if (err == nil) != (name == "pinned") {
				t.Fatalf("verification = %v", err)
			}
		})
	}
}

func TestBinaryFingerprintRequiresOneExactStamp(t *testing.T) {
	lock := Lock{Revision: Revision, TreeSHA256: ExpectedTreeSHA256}
	for _, name := range []string{"valid", "missing", "wrong", "duplicate"} {
		t.Run(name, func(t *testing.T) {
			data := "binary header\x00" + BinaryStamp(lock) + "\x00binary tail"
			switch name {
			case "missing":
				data = "binary"
			case "wrong":
				data = strings.ReplaceAll(data, ExpectedTreeSHA256, strings.Repeat("0", 64))
			case "duplicate":
				data += BinaryStamp(lock)
			}
			if err := VerifyBinaryStamp([]byte(data), lock); (err == nil) != (name == "valid") {
				t.Fatalf("binary stamp validation: %v", err)
			}
		})
	}
}

func TestTreeHashDetectsContentPathsAndExtraFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("source\n"), 0644); err != nil {
		t.Fatal(err)
	}
	first, count, err := TreeHash(root)
	if err != nil || count != 1 {
		t.Fatalf("initial hash: %d, %v", count, err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("source\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	changed, _, err := TreeHash(root)
	if err != nil || changed == first {
		t.Fatalf("changed bytes not detected: %v", err)
	}
	if err := os.Rename(filepath.Join(root, "a.go"), filepath.Join(root, "b.go")); err != nil {
		t.Fatal(err)
	}
	renamed, _, err := TreeHash(root)
	if err != nil || renamed == changed {
		t.Fatalf("changed path not detected: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "extra.go"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	extra, count, err := TreeHash(root)
	if err != nil || count != 2 || extra == renamed {
		t.Fatalf("extra file not detected: %v", err)
	}
}
