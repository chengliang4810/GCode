package plugins

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func committedVirtualLinkSource(t *testing.T, links map[string]string) (string, string) {
	t.Helper()
	repo := t.TempDir()
	writeTestFile(t, repo, ".claude-plugin/marketplace.json", `{"name":"git-links","owner":{"name":"Tests"},"plugins":[{"name":"demo","source":"./plugins/demo"}]}`)
	writeTestFile(t, repo, "plugins/demo/.claude-plugin/plugin.json", `{"name":"demo"}`)
	writeTestFile(t, repo, "plugins/demo/skills/local/SKILL.md", "LOCAL")
	writeTestFile(t, repo, "shared/skill/SKILL.md", "SHARED")
	writeTestFile(t, repo, "shared/file.txt", "SHARED FILE")
	ctx := context.Background()
	if _, err := git(ctx, "", "init", "--", repo); err != nil {
		t.Fatal(err)
	}
	if _, err := git(ctx, repo, "add", "--all"); err != nil {
		t.Fatal(err)
	}
	for path, target := range links {
		command := exec.Command("git", "-C", repo, "hash-object", "-w", "--stdin")
		command.Stdin = strings.NewReader(target)
		data, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("create symbolic-link Git blob: %s %v", data, err)
		}
		sha := strings.TrimSpace(string(data))
		if _, err := git(ctx, repo, "update-index", "--add", "--cacheinfo", "120000,"+sha+","+path); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := git(
		ctx, repo, "-c", "user.name=Plugin fixture", "-c", "user.email=fixture@example.invalid",
		"-c", "commit.gpgsign=false", "commit", "-m", "virtual symbolic links",
	); err != nil {
		t.Fatal(err)
	}
	sha, err := git(ctx, repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return repo, sha
}

func TestGitMarketplaceRefreshStoresVirtualLinksWithoutHostLinkPermission(t *testing.T) {
	links := map[string]string{
		"CLAUDE.md":      "AGENTS.md",
		"unused/a":       "b",
		"unused/b":       "a",
		"unused/private": "/nonexistent-private-file",
	}
	repo, sha := committedVirtualLinkSource(t, links)
	managerRoot := t.TempDir()
	manager, err := NewManager(managerRoot)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := manager.PutSource(ctx, MarketplaceSource{
		ID: "git-links", Name: "git-links", Enabled: true, Source: Source{Source: "git", URL: repo, SHA: sha},
	}); err != nil {
		t.Fatal(err)
	}
	catalog, err := manager.Refresh(ctx, "git-links")
	if err != nil {
		t.Fatalf("unselected repository links blocked production marketplace refresh: %v", err)
	}
	if catalog.Commit != sha || len(catalog.Links) != len(links) {
		t.Fatalf("frozen source/link metadata lost: commit=%q links=%v", catalog.Commit, catalog.Links)
	}
	for name, target := range links {
		if catalog.Links[name] != target {
			t.Fatalf("source link target changed: %s -> %q", name, catalog.Links[name])
		}
		if _, err := os.Lstat(filepath.Join(catalog.Root, filepath.FromSlash(name))); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("source cache created a repository OS link: %s: %v", name, err)
		}
	}
	catalog.Links["CLAUDE.md"] = "caller mutation"
	if manager.Catalogs()[0].Links["CLAUDE.md"] != "AGENTS.md" {
		t.Fatal("catalog link metadata aliases caller state")
	}
	reopened, err := NewManager(managerRoot)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := reopened.Install(ctx, "git-links", "demo")
	if err != nil {
		t.Fatalf("persisted source links blocked unrelated plain plugin installation: %v", err)
	}
	bundle, err := LoadInstallation(installation)
	if err != nil || len(bundle.Skills) != 1 {
		t.Fatalf("installed plain plugin failed: %+v %v", bundle.Skills, err)
	}
}

func TestProductionGitSourcesHonorOptionalPluginPath(t *testing.T) {
	repo, sha := committedVirtualLinkSource(t, nil)
	for _, sourceType := range []string{"url", "git", "git-subdir"} {
		t.Run(sourceType, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "cached")
			commit, err := materialize(
				context.Background(), Source{Source: sourceType, URL: repo, Path: "plugins/demo", SHA: sha}, target, nil,
			)
			if err != nil || commit != sha {
				t.Fatalf("plugin subdirectory materialization failed: commit=%q err=%v", commit, err)
			}
			bundle, err := Load(target)
			if err != nil || bundle.Manifest.Name != "demo" || len(bundle.Skills) != 1 {
				t.Fatalf("source path did not select the plugin: bundle=%+v err=%v", bundle, err)
			}
			for _, unrelated := range []string{"shared", ".claude-plugin/marketplace.json", "plugins"} {
				if _, err := os.Lstat(filepath.Join(target, filepath.FromSlash(unrelated))); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("repository content outside plugin path was installed: %s: %v", unrelated, err)
				}
			}
		})
	}
}

func TestProductionGitWindowsLongPathsExportAndRefresh(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows Git long-path regression")
	}
	repo, sha := committedVirtualLinkSource(t, nil)
	deep := t.TempDir()
	for len(deep) < 290 {
		deep = filepath.Join(deep, "plugin-marketplace-long-directory")
	}
	t.Logf("production source directory length: %d", len(deep))
	ctx := context.Background()
	t.Run("fetch packed objects", func(t *testing.T) {
		packed := t.TempDir()
		for len(packed) < 205 {
			packed = filepath.Join(packed, "packed-object-directory")
		}
		if err := os.MkdirAll(packed, 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := git(ctx, packed, "init"); err != nil {
			t.Fatal(err)
		}
		if _, err := git(ctx, packed, "fetch", "--depth=1", "--filter=blob:none", "--no-tags", repo, sha); err != nil {
			t.Fatal(err)
		}
		promisors, err := filepath.Glob(filepath.Join(packed, ".git", "objects", "pack", "*.promisor"))
		if err != nil || len(promisors) == 0 {
			t.Fatalf("real filtered fetch did not produce promisor files: %v %v", promisors, err)
		}
		for _, name := range promisors {
			if len(name) <= 260 {
				t.Fatalf("regression did not exercise a long pack path: %s", name)
			}
			t.Logf("filtered fetch promisor path length: %d", len(name))
		}
	})
	t.Run("export", func(t *testing.T) {
		tree, commit, err := ExportGitSource(ctx, Source{Source: "git", URL: repo, SHA: sha}, filepath.Join(deep, "export"))
		if err != nil {
			t.Fatal(err)
		}
		data, err := readSourceDefinition(tree, ".claude-plugin/marketplace.json")
		if err != nil || commit != sha || !strings.Contains(string(data), `"git-links"`) {
			t.Fatalf("long-path Git export failed: commit=%q data=%q err=%v", commit, data, err)
		}
	})
	t.Run("refresh and install", func(t *testing.T) {
		manager, err := NewManager(filepath.Join(deep, "manager"))
		if err != nil {
			t.Fatal(err)
		}
		if err := manager.PutSource(ctx, MarketplaceSource{
			ID: "git-links", Name: "git-links", Enabled: true, Source: Source{Source: "git", URL: repo, SHA: sha},
		}); err != nil {
			t.Fatal(err)
		}
		catalog, err := manager.Refresh(ctx, "git-links")
		if err != nil || catalog.Commit != sha {
			t.Fatalf("long-path marketplace refresh failed: commit=%q err=%v", catalog.Commit, err)
		}
		installation, err := manager.Install(ctx, "git-links", "demo")
		if err != nil {
			t.Fatal(err)
		}
		bundle, err := LoadInstallation(installation)
		if err != nil || bundle.Manifest.Name != "demo" || len(bundle.Skills) != 1 {
			t.Fatalf("long-path installed plugin failed to load: bundle=%+v err=%v", bundle, err)
		}
	})
}

func TestProductionGitSubdirectoryDereferencesVirtualSharedLinks(t *testing.T) {
	repo, sha := committedVirtualLinkSource(t, map[string]string{
		"CLAUDE.md":                  "AGENTS.md",
		"plugins/demo/skills/shared": "../../../shared/skill",
		"plugins/demo/shared.txt":    "../../shared/file.txt",
		"plugins/demo/outside":       "/nonexistent-private-file",
	})
	target := filepath.Join(t.TempDir(), "cached")
	got, err := materialize(
		context.Background(), Source{Source: "git-subdir", URL: repo, Path: "plugins/demo", SHA: sha}, target, nil,
	)
	if err != nil || got != sha {
		t.Fatalf("production git-subdir source was blocked by repository links: commit=%q err=%v", got, err)
	}
	if data, err := os.ReadFile(filepath.Join(target, "shared.txt")); err != nil || string(data) != "SHARED FILE" {
		t.Fatalf("shared target did not materialize: %q %v", data, err)
	}
	if info, err := os.Lstat(filepath.Join(target, "skills", "shared")); err != nil || !info.IsDir() {
		t.Fatalf("shared link was not dereferenced: %v %v", info, err)
	}
	if _, err := os.Lstat(filepath.Join(target, "outside")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("external target was materialized: %v", err)
	}
	bundle, err := Load(target)
	if err != nil || len(bundle.Skills) != 2 {
		t.Fatalf("git-subdir shared skill discovery failed: %+v %v", bundle.Skills, err)
	}
}

func TestProductionGitSelectedInternalLinkStillRequiresRealOSLink(t *testing.T) {
	repo, sha := committedVirtualLinkSource(t, map[string]string{
		"CLAUDE.md":                 "AGENTS.md",
		"plugins/demo/skills/alias": "local",
	})
	probe := t.TempDir()
	writeTestFile(t, probe, "target", "probe")
	linkPermissionErr := os.Symlink("target", filepath.Join(probe, "link"))
	target := filepath.Join(t.TempDir(), "cached")
	_, err := materialize(
		context.Background(), Source{Source: "git-subdir", URL: repo, Path: "plugins/demo", SHA: sha}, target, nil,
	)
	if linkPermissionErr != nil {
		if err == nil || !strings.Contains(err.Error(), "preserve plugin symbolic link") {
			t.Fatalf("required internal link silently bypassed host permissions: %v", err)
		}
		t.Logf("source export succeeded; final internal link explicitly requires unavailable host permission: %v", err)
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	link, err := os.Readlink(filepath.Join(target, "skills", "alias"))
	if err != nil || filepath.IsAbs(link) || filepath.ToSlash(link) != "local" {
		t.Fatalf("required internal Git link did not become a real relative link: %q %v", link, err)
	}
}

func TestGitSourceArchiveRejectsUnsafeEntriesWithoutCreatingLinks(t *testing.T) {
	for _, tc := range []struct {
		name    string
		headers []*tar.Header
	}{
		{name: "escape", headers: []*tar.Header{{Name: "../outside", Typeflag: tar.TypeReg}}},
		{name: "absolute", headers: []*tar.Header{{Name: "/outside", Typeflag: tar.TypeReg}}},
		{name: "git metadata", headers: []*tar.Header{{Name: ".git/config", Typeflag: tar.TypeReg}}},
		{name: "hard link", headers: []*tar.Header{{Name: "hard", Typeflag: tar.TypeLink, Linkname: "/outside"}}},
		{name: "fifo", headers: []*tar.Header{{Name: "pipe", Typeflag: tar.TypeFifo}}},
		{name: "duplicate", headers: []*tar.Header{{Name: "file", Typeflag: tar.TypeReg}, {Name: "file", Typeflag: tar.TypeReg}}},
		{
			name: "link parent before file",
			headers: []*tar.Header{
				{Name: "parent", Typeflag: tar.TypeSymlink, Linkname: "/outside"},
				{Name: "parent/file", Typeflag: tar.TypeReg},
			},
		},
		{
			name: "link parent after file",
			headers: []*tar.Header{
				{Name: "parent/file", Typeflag: tar.TypeReg},
				{Name: "parent", Typeflag: tar.TypeSymlink, Linkname: "/outside"},
			},
		},
		{name: "size budget", headers: []*tar.Header{{Name: "huge", Typeflag: tar.TypeReg, Size: maxPluginCopyBytes + 1}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buffer bytes.Buffer
			writer := tar.NewWriter(&buffer)
			for _, header := range tc.headers {
				if err := writer.WriteHeader(header); err != nil {
					t.Fatal(err)
				}
			}
			target := filepath.Join(t.TempDir(), "export")
			if _, err := extractGitSource(context.Background(), bytes.NewReader(buffer.Bytes()), target); err == nil {
				t.Fatal("unsafe or over-budget Git source archive was accepted")
			}
			_ = filepath.WalkDir(target, func(path string, entry os.DirEntry, err error) error {
				if err == nil && entry.Type()&os.ModeSymlink != 0 {
					t.Errorf("archive extraction created an OS symbolic link: %s", path)
				}
				return err
			})
		})
	}
}
