package plugins

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func requirePluginSymlinks(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, root, "target", "test")
	if err := os.Symlink("target", filepath.Join(root, "link")); err != nil {
		t.Skipf("host cannot create real symbolic links; Linux symlink tests remain required: %v", err)
	}
}

func pluginLink(t *testing.T, root, name, target string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.FromSlash(target), path); err != nil {
		t.Fatal(err)
	}
}

func symlinkMarketplace(t *testing.T) (market, plugin string) {
	t.Helper()
	requirePluginSymlinks(t)
	market = t.TempDir()
	plugin = filepath.Join(market, "plugins", "demo")
	writeTestFile(t, market, ".claude-plugin/marketplace.json", `{"name":"links","owner":{"name":"Tests"},"plugins":[{"name":"demo","source":"./plugins/demo"}]}`)
	writeTestFile(t, plugin, ".claude-plugin/plugin.json", `{"name":"demo"}`)
	writeTestFile(t, plugin, "skills/local/SKILL.md", "---\ndescription: Local skill\n---\nLOCAL")
	writeTestFile(t, plugin, "docs/command.md", "INTERNAL COMMAND")
	writeTestFile(t, market, "shared/skill/SKILL.md", "---\ndescription: Shared skill\n---\nSHARED")
	writeTestFile(t, market, "shared/skill/helper.sh", "#!/bin/sh\nprintf SHARED")
	writeTestFile(t, market, "shared/file.txt", "SHARED FILE")
	pluginLink(t, plugin, "skills/alias", "local")
	pluginLink(t, plugin, "commands/linked.md", "../docs/command.md")
	pluginLink(t, plugin, "skills/shared", "../../../shared/skill")
	pluginLink(t, plugin, "shared.txt", "../../shared/file.txt")
	outside := t.TempDir()
	writeTestFile(t, outside, "private.txt", "DO NOT COPY")
	pluginLink(t, plugin, "outside.txt", filepath.Join(outside, "private.txt"))
	pluginLink(t, plugin, "outside-dir", outside)
	return market, plugin
}

func TestMarketplacePluginSymlinksPreserveInternalDereferenceSharedAndSkipOutside(t *testing.T) {
	market, plugin := symlinkMarketplace(t)
	target := filepath.Join(t.TempDir(), "cached")
	if err := MaterializePluginSource(context.Background(), SourceTree{Root: market}, "./plugins/demo", target); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"skills/alias", "commands/linked.md"} {
		link, err := os.Readlink(filepath.Join(target, filepath.FromSlash(name)))
		if err != nil || filepath.IsAbs(link) {
			t.Fatalf("internal link was not retained as a relative link: %s -> %q: %v", name, link, err)
		}
	}
	for _, tc := range []struct{ name, want string }{
		{name: "skills/shared/SKILL.md", want: "SHARED"},
		{name: "skills/shared/helper.sh", want: "printf SHARED"},
		{name: "shared.txt", want: "SHARED FILE"},
	} {
		data, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(tc.name)))
		if err != nil || !strings.Contains(string(data), tc.want) {
			t.Fatalf("shared target not copied: %s: %q %v", tc.name, data, err)
		}
	}
	info, err := os.Lstat(filepath.Join(target, "skills", "shared"))
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("shared directory was not dereferenced: %v %v", info, err)
	}
	for _, name := range []string{"outside.txt", "outside-dir"} {
		if _, err := os.Lstat(filepath.Join(target, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("outside target was copied: %s: %v", name, err)
		}
	}
	bundle, err := Load(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Skills) != 3 || len(bundle.Commands) != 1 || bundle.Commands[0].Name != "demo:linked" {
		t.Fatalf("linked skill/command discovery failed: skills=%+v commands=%+v", bundle.Skills, bundle.Commands)
	}
	if _, err := ParseMarkdown(filepath.Join(target, "commands", "linked.md")); err == nil {
		t.Fatal("ordinary Markdown loading unexpectedly accepted symbolic links")
	}
	if data, err := os.ReadFile(filepath.Join(plugin, "outside.txt")); err != nil || string(data) != "DO NOT COPY" {
		t.Fatalf("copy modified the source or external target: %q %v", data, err)
	}
}

func TestLocalPluginCopyRetainsOnlyPluginInternalSymlinks(t *testing.T) {
	_, plugin := symlinkMarketplace(t)
	target := filepath.Join(t.TempDir(), "local")
	if err := copyTree(context.Background(), plugin, target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Readlink(filepath.Join(target, "skills", "alias")); err != nil {
		t.Fatalf("local internal link was lost: %v", err)
	}
	for _, name := range []string{"skills/shared", "shared.txt", "outside.txt", "outside-dir"} {
		if _, err := os.Lstat(filepath.Join(target, filepath.FromSlash(name))); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("local copy followed a target outside its plugin: %s: %v", name, err)
		}
	}
	if _, err := Load(target); err != nil {
		t.Fatal(err)
	}
}

func TestManagerInstallsAndReopensPluginWithMarketplaceSymlinks(t *testing.T) {
	market, _ := symlinkMarketplace(t)
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := manager.PutSource(ctx, MarketplaceSource{
		ID: "links", Name: "links", Enabled: true, Source: Source{Source: "directory", Path: market},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Refresh(ctx, "links"); err != nil {
		t.Fatal(err)
	}
	install, err := manager.Install(ctx, "links", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(root); err != nil {
		t.Fatalf("safe cached links prevented reopening the manager: %v", err)
	}
	bundle, err := LoadInstallation(install)
	if err != nil || len(bundle.Skills) != 3 {
		t.Fatalf("installed linked components failed: skills=%v err=%v", bundle.Skills, err)
	}
	if info, err := os.Lstat(filepath.Join(install.Path, "skills", "shared")); err != nil || !info.IsDir() {
		t.Fatalf("marketplace sibling target did not become cached data: %v %v", info, err)
	}
}

func TestGitSubdirectoryMaterializesSiblingLinkedResources(t *testing.T) {
	market, _ := symlinkMarketplace(t)
	ctx := context.Background()
	for _, args := range [][]string{
		{"init", "--", market},
		{"-C", market, "add", "--all"},
		{
			"-C", market, "-c", "user.name=Plugin fixture", "-c", "user.email=fixture@example.invalid",
			"-c", "commit.gpgsign=false", "commit", "-m", "linked marketplace",
		},
	} {
		if _, err := git(ctx, "", args...); err != nil {
			t.Fatal(err)
		}
	}
	sha, err := git(ctx, market, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "cached")
	got, err := materialize(ctx, Source{Source: "git-subdir", URL: market, Path: "plugins/demo", SHA: sha}, target, nil)
	if err != nil || got != sha {
		t.Fatalf("fixed git source could not materialize linked siblings: commit=%q err=%v", got, err)
	}
	bundle, err := Load(target)
	if err != nil || len(bundle.Skills) != 3 {
		t.Fatalf("git-subdir lost linked skills: %+v %v", bundle.Skills, err)
	}
	info, err := os.Lstat(filepath.Join(target, "skills", "shared"))
	if err != nil || !info.IsDir() {
		t.Fatalf("git-subdir did not dereference a source-repository sibling: %v %v", info, err)
	}
}

func TestPluginSymlinksRejectCyclesAndUnconfinedLoads(t *testing.T) {
	requirePluginSymlinks(t)
	for _, kind := range []string{"chain cycle", "directory cycle", "outside", "git directory", "dangling"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			writeTestFile(t, root, ".claude-plugin/plugin.json", `{"name":"unsafe"}`)
			switch kind {
			case "chain cycle":
				pluginLink(t, root, "a", "b")
				pluginLink(t, root, "b", "a")
			case "directory cycle":
				writeTestFile(t, root, "dir/file", "test")
				pluginLink(t, root, "dir/ancestor", "..")
			case "outside":
				pluginLink(t, root, "external", t.TempDir())
			case "git directory":
				writeTestFile(t, root, ".git/config", "PRIVATE")
				pluginLink(t, root, "expose", ".git")
			case "dangling":
				pluginLink(t, root, "missing", "not-there")
			}
			if _, err := Load(root); err == nil {
				t.Fatal("unsafe or cyclic linked plugin was accepted")
			}
		})
	}
}

func TestPluginCopyBudgetsAndCancellationRemainEnforced(t *testing.T) {
	t.Run("size", func(t *testing.T) {
		root := t.TempDir()
		file, err := os.Create(filepath.Join(root, "oversized"))
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Truncate(maxPluginCopyBytes + 1); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		_ = file.Close()
		if err := copyTree(context.Background(), root, filepath.Join(t.TempDir(), "copy")); err == nil {
			t.Fatal("copy size budget was bypassed")
		}
	})
	t.Run("cancel", func(t *testing.T) {
		root := t.TempDir()
		writeTestFile(t, root, "file", "test")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := copyTree(ctx, root, filepath.Join(t.TempDir(), "copy")); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation was ignored: %v", err)
		}
	})
}

func TestPluginCopyRejectsDestinationLinkedBackIntoSource(t *testing.T) {
	requirePluginSymlinks(t)
	source, destination := t.TempDir(), t.TempDir()
	writeTestFile(t, source, "original", "PRESERVED")
	pluginLink(t, destination, "source-link", source)
	if err := copyTree(context.Background(), source, filepath.Join(destination, "source-link", "nested")); err == nil {
		t.Fatal("copy accepted a destination whose parent links back into its source")
	}
	if _, err := os.Stat(filepath.Join(source, "nested")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected copy changed its source directory: %v", err)
	}
}
