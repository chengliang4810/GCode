package acceptance

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractAcceptsGitCommitPAXMetadataWithoutCreatingAFile(t *testing.T) {
	t.Parallel()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	if err := writer.WriteHeader(&tar.Header{Name: "pax_global_header", Typeflag: tar.TypeXGlobalHeader, PAXRecords: map[string]string{"comment": strings.Repeat("a", 40)}}); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteHeader(&tar.Header{Name: "file.txt", Typeflag: tar.TypeReg, Mode: 0600, Size: 1}); err != nil {
		t.Fatal(err)
	}
	_, _ = writer.Write([]byte("x"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := extract(context.Background(), &buffer, root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "pax_global_header")); !os.IsNotExist(err) {
		t.Fatal("Git PAX metadata was extracted as a file")
	}
}

func TestManifestTextHashNormalizesOnlyInvestigatorLineEndings(t *testing.T) {
	if manifestTextHash([]byte("{\r\n\"name\":\"demo\"\r\n}")) != manifestTextHash([]byte("{\n\"name\":\"demo\"\n}")) {
		t.Fatal("investigator LF normalization caused a false source mismatch")
	}
	if manifestTextHash([]byte(`{"name":"demo"}`)) == manifestTextHash([]byte(`{"name":"changed"}`)) {
		t.Fatal("different manifest content shared the same evidence digest")
	}
}

func TestExtractRejectsLinksAndPathEscapes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, path string
		kind       byte
	}{
		{"parent", "../escaped", tar.TypeReg}, {"absolute", "/escaped", tar.TypeReg},
		{"windows", "C:/escaped", tar.TypeReg}, {"backslash", `..\escaped`, tar.TypeReg},
		{"symbolic", "link", tar.TypeSymlink}, {"hardlink", "link", tar.TypeLink},
		{"git metadata", ".git/config", tar.TypeReg},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buffer bytes.Buffer
			writer := tar.NewWriter(&buffer)
			header := &tar.Header{Name: tc.path, Mode: 0600, Typeflag: tc.kind}
			if tc.kind == tar.TypeReg {
				header.Size = 1
			}
			if err := writer.WriteHeader(header); err != nil {
				t.Fatal(err)
			}
			if header.Size == 1 {
				_, _ = writer.Write([]byte("x"))
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err := extract(context.Background(), &buffer, t.TempDir()); err == nil {
				t.Fatal("unsafe archive entry accepted")
			}
		})
	}
}

func TestArtifactDigestBindsFileBytes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	if err := os.WriteFile(path, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := treeDigest(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("after"), 0600); err != nil {
		t.Fatal(err)
	}
	after, err := treeDigest(root)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("changed installed bytes kept the same artifact digest")
	}
}

func TestRepositoryExtractionNeverWritesThroughAnArchiveSymlink(t *testing.T) {
	t.Parallel()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	if err := writer.WriteHeader(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../outside"}); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteHeader(&tar.Header{Name: "link/secret.txt", Typeflag: tar.TypeReg, Mode: 0600, Size: 1}); err != nil {
		t.Fatal(err)
	}
	_, _ = writer.Write([]byte("x"))
	_ = writer.Close()
	parent := t.TempDir()
	root := filepath.Join(parent, "repository")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := extractRepository(context.Background(), &buffer, root, map[string]string{}); err == nil {
		t.Fatal("link and regular directory collision accepted")
	}
	if _, err := os.Stat(filepath.Join(parent, "outside", "secret.txt")); !os.IsNotExist(err) {
		t.Fatal("archive wrote through a symbolic link")
	}
}

func TestRepositoryExtractionKeepsLinksAsTypedMetadata(t *testing.T) {
	t.Parallel()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	if err := writer.WriteHeader(&tar.Header{Name: ".cursor/skills", Typeflag: tar.TypeSymlink, Linkname: "../skills"}); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	root := t.TempDir()
	links := map[string]string{}
	if err := extractRepository(context.Background(), &buffer, root, links); err != nil {
		t.Fatal(err)
	}
	if links[".cursor/skills"] != "../skills" {
		t.Fatal("frozen link metadata was lost")
	}
	if _, err := os.Lstat(filepath.Join(root, ".cursor", "skills")); !os.IsNotExist(err) {
		t.Fatal("source-cache extraction created an operating-system link")
	}
}

func TestArtifactDigestBindsContainedSymlinksAndRejectsEscapes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "actual.txt"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked.txt")
	if err := os.Symlink("actual.txt", link); err != nil {
		t.Skip("host cannot create symbolic links:", err)
	}
	if _, err := treeDigest(root); err != nil {
		t.Fatal("contained plugin link could not be hashed:", err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	if _, err := treeDigest(root); err == nil {
		t.Fatal("artifact digest traversed an external symbolic link")
	}
}

func TestArtifactDigestResolvesParentAliasesWithoutAllowingEscapes(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	root := filepath.Join(parent, "bundle")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "actual.txt"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked.txt")
	if err := os.Symlink("actual.txt", link); err != nil {
		t.Skip("host cannot create symbolic links:", err)
	}
	alias := filepath.Join(t.TempDir(), "parent")
	if err := os.Symlink(parent, alias); err != nil {
		t.Fatal(err)
	}
	direct, err := treeDigest(root)
	if err != nil {
		t.Fatal(err)
	}
	rootAlias := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(root, rootAlias); err != nil {
		t.Fatal(err)
	}
	if _, err := treeDigest(rootAlias); err == nil {
		t.Fatal("artifact root itself may not be a symbolic link")
	}
	aliasedRoot := filepath.Join(alias, "bundle")
	throughAlias, err := treeDigest(aliasedRoot)
	if err != nil || throughAlias != direct {
		t.Fatalf("parent alias changed the contained artifact: direct=%s alias=%s err=%v", direct, throughAlias, err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	if _, err := treeDigest(aliasedRoot); err == nil {
		t.Fatal("parent normalization allowed an escaping artifact link")
	}
}
