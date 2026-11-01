package plugins

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func virtualSourceFixture(t *testing.T) SourceTree {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, root, "plugins/demo/.claude-plugin/plugin.json", `{"name":"demo"}`)
	writeTestFile(t, root, "plugins/demo/skills/local/SKILL.md", "LOCAL")
	writeTestFile(t, root, "shared/skill/SKILL.md", "SHARED")
	writeTestFile(t, root, "shared/file.txt", "SHARED FILE")
	return SourceTree{Root: root, Links: map[string]string{}}
}

func TestVirtualSourceIgnoresUnselectedLinksWithoutCreatingOSLinks(t *testing.T) {
	tree := virtualSourceFixture(t)
	tree.Links = map[string]string{
		"CLAUDE.md":      "AGENTS.md",
		"other/ancestor": "..",
		"other/outside":  "/nonexistent-private-file",
		"other/a":        "b",
		"other/b":        "a",
	}
	target := filepath.Join(t.TempDir(), "cached")
	if err := MaterializePluginSource(context.Background(), tree, "plugins/demo", target); err != nil {
		t.Fatalf("unrelated repository link blocked a plain plugin: %v", err)
	}
	bundle, err := Load(target)
	if err != nil || len(bundle.Skills) != 1 {
		t.Fatalf("selected regular plugin did not load: %+v %v", bundle.Skills, err)
	}
	for _, name := range []string{"CLAUDE.md", "other"} {
		if _, err := os.Lstat(filepath.Join(tree.Root, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("virtual metadata was materialized in the source cache: %s: %v", name, err)
		}
		if _, err := os.Lstat(filepath.Join(target, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unselected source link reached the plugin cache: %s: %v", name, err)
		}
	}
}

func TestVirtualSourceDereferencesSelectedMarketplaceLinksWithoutOSLinks(t *testing.T) {
	tree := virtualSourceFixture(t)
	tree.Links = map[string]string{
		"plugins/demo/skills/shared": "../../../shared/linked-skill",
		"plugins/demo/shared.txt":    "../../shared/file.txt",
		"shared/linked-skill":        "skill",
		"shared/skill/helper.txt":    "../file.txt",
		"plugins/demo/outside":       "/nonexistent-private-file",
	}
	target := filepath.Join(t.TempDir(), "cached")
	if err := MaterializePluginSource(context.Background(), tree, "plugins/demo", target); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, content string }{
		{path: "skills/shared/SKILL.md", content: "SHARED"},
		{path: "skills/shared/helper.txt", content: "SHARED FILE"},
		{path: "shared.txt", content: "SHARED FILE"},
	} {
		path := filepath.Join(target, filepath.FromSlash(tc.path))
		data, err := os.ReadFile(path)
		if err != nil || string(data) != tc.content {
			t.Fatalf("virtual shared target failed: %s: %q %v", tc.path, data, err)
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("shared target did not become an ordinary file: %v %v", info, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(target, "outside")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("virtual outside target was copied: %v", err)
	}
	if bundle, err := Load(target); err != nil || len(bundle.Skills) != 2 {
		t.Fatalf("shared virtual skill did not load: %+v %v", bundle.Skills, err)
	}
}

func TestVirtualSourcePreservesSelectedInternalLinksAsRealRelativeLinks(t *testing.T) {
	requirePluginSymlinks(t)
	tree := virtualSourceFixture(t)
	tree.Links["plugins/demo/skills/alias"] = "local"
	target := filepath.Join(t.TempDir(), "cached")
	if err := MaterializePluginSource(context.Background(), tree, "plugins/demo", target); err != nil {
		t.Fatal(err)
	}
	link, err := os.Readlink(filepath.Join(target, "skills", "alias"))
	if err != nil || filepath.IsAbs(link) || filepath.ToSlash(link) != "local" {
		t.Fatalf("selected virtual link did not become a real relative link: %q %v", link, err)
	}
	if _, err := os.Lstat(filepath.Join(tree.Root, "plugins", "demo", "skills", "alias")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source cache was modified: %v", err)
	}
	bundle, err := Load(target)
	if err != nil || len(bundle.Skills) != 2 {
		t.Fatalf("preserved virtual skill link did not load: %+v %v", bundle.Skills, err)
	}
}

func TestVirtualSourceRejectsMalformedMetadataAndSelectedCycles(t *testing.T) {
	for _, tc := range []struct {
		name  string
		links map[string]string
	}{
		{name: "escape key", links: map[string]string{"../outside": "file"}},
		{name: "absolute key", links: map[string]string{"/outside": "file"}},
		{name: "noncanonical key", links: map[string]string{"plugins/./demo/link": "file"}},
		{name: "duplicate real file", links: map[string]string{"plugins/demo/.claude-plugin/plugin.json": "other.json"}},
		{name: "symbolic parent", links: map[string]string{"a": "b", "a/child": "file"}},
		{name: "selected chain cycle", links: map[string]string{"plugins/demo/a": "b", "plugins/demo/b": "a"}},
		{name: "selected directory cycle", links: map[string]string{"plugins/demo/skills/ancestor": ".."}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree := virtualSourceFixture(t)
			tree.Links = tc.links
			if err := MaterializePluginSource(context.Background(), tree, "plugins/demo", filepath.Join(t.TempDir(), "cached")); err == nil {
				t.Fatal("invalid source metadata or a selected cycle was accepted")
			}
		})
	}
}

func TestVirtualSourceOnlyRequiredInternalLinksNeedHostPermission(t *testing.T) {
	tree := virtualSourceFixture(t)
	tree.Links["plugins/demo/skills/alias"] = "local"
	target := filepath.Join(t.TempDir(), "cached")
	err := MaterializePluginSource(context.Background(), tree, "plugins/demo", target)
	if err == nil {
		if _, err := os.Readlink(filepath.Join(target, "skills", "alias")); err != nil {
			t.Fatal("installation silently replaced a required internal link with a copied file")
		}
		return
	}
	if !strings.Contains(err.Error(), "preserve plugin symbolic link") {
		t.Fatalf("required link failed before the explicit OS-link creation boundary: %v", err)
	}
}
