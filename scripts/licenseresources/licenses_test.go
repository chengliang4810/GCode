package licenseresources

import (
	"os"
	"path/filepath"
	"testing"
)

func sourceResources(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "resources"))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestSourceLicenseBundleIsComplete(t *testing.T) {
	manifest, err := VerifyTree(sourceResources(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Files) < len(required) {
		t.Fatal("license coverage is incomplete")
	}
}

func TestReleaseCopyRejectsMissingCorruptAndUnlistedNotices(t *testing.T) {
	for _, name := range []string{"complete", "missing ghostty", "corrupt font", "missing notice", "unlisted file"} {
		t.Run(name, func(t *testing.T) {
			source := sourceResources(t)
			delivered := t.TempDir()
			if err := os.CopyFS(delivered, os.DirFS(source)); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "missing ghostty":
				if err := os.Remove(filepath.Join(delivered, "licenses", "native", "Ghostty-MIT.txt")); err != nil {
					t.Fatal(err)
				}
			case "corrupt font":
				if err := os.WriteFile(filepath.Join(delivered, "licenses", "fonts", "Geist-OFL.txt"), []byte("incomplete"), 0644); err != nil {
					t.Fatal(err)
				}
			case "missing notice":
				if err := os.Remove(filepath.Join(delivered, "THIRD_PARTY_NOTICES.md")); err != nil {
					t.Fatal(err)
				}
			case "unlisted file":
				if err := os.WriteFile(filepath.Join(delivered, "licenses", "unreviewed.txt"), []byte("other"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			_, err := VerifyCopy(source, delivered)
			if (err == nil) != (name == "complete") {
				t.Fatalf("release verification: %v", err)
			}
		})
	}
}
