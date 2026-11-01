package plugins

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func namespaceManager(t *testing.T) *Manager {
	t.Helper()
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, market := range []string{"first", "second"} {
		root := t.TempDir()
		writeTestFile(t, root, ".claude-plugin/marketplace.json", fmt.Sprintf(`{"name":%q,"owner":{"name":"Tests"},"plugins":[{"name":"demo","source":"./plugin"}]}`, market))
		writeTestFile(t, root, "plugin/.claude-plugin/plugin.json", `{"name":"demo"}`)
		writeTestFile(t, root, "plugin/skills/run/SKILL.md", "---\nname: run\n---\nRun fixture")
		if err := manager.PutSource(context.Background(), MarketplaceSource{ID: market, Name: market, Enabled: true, Source: Source{Source: "directory", Path: root}}); err != nil {
			t.Fatal(err)
		}
		if _, err := manager.Refresh(context.Background(), market); err != nil {
			t.Fatal(err)
		}
	}
	return manager
}

func TestManagerRejectsVisibleCrossMarketplaceNamespaceCollision(t *testing.T) {
	t.Parallel()
	manager := namespaceManager(t)
	ctx := context.Background()
	first, err := manager.Install(ctx, "first", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Install(ctx, "second", "demo"); err == nil || !strings.Contains(err.Error(), "namespace") {
		t.Fatal("visible plugin namespace collision was not rejected")
	}
	if got := manager.Installed(); len(got) != 1 || got[0].ID != first.ID {
		t.Fatal("collision changed the published installation registry")
	}
	if err := manager.SetEnabled(ctx, first.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Install(ctx, "second", "demo"); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled(ctx, first.ID, true); err == nil {
		t.Fatal("enabling a colliding namespace succeeded")
	}
}

func TestManagerAllowsSameNamespaceInDisjointProjects(t *testing.T) {
	t.Parallel()
	manager := namespaceManager(t)
	root := t.TempDir()
	first, second := filepath.Join(root, "first"), filepath.Join(root, "second")
	for i, market := range []string{"first", "second"} {
		path := []string{first, second}[i]
		if _, err := manager.Install(context.Background(), market, "demo", InstallOptions{Scope: "project", ProjectPath: path}); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{first, second} {
		bundles, err := manager.BundlesFor(path)
		if err != nil || len(bundles) != 1 {
			t.Fatalf("disjoint namespace failed: %d %v", len(bundles), err)
		}
	}
	// Existing invalid state must fail explicitly rather than silently overriding
	// a skill or creating duplicate MCP tool definitions.
	manager.mu.Lock()
	for i := range manager.data.Installations {
		manager.data.Installations[i].Scope, manager.data.Installations[i].ProjectPath = "user", ""
	}
	manager.mu.Unlock()
	if _, err := manager.Bundles(); err == nil {
		t.Fatal("existing namespace conflict was silently loaded")
	}
}

func TestMarketplaceInstallIdentityAndManifestNamespaceAreDistinct(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeTestFile(t, root, ".claude-plugin/marketplace.json", `{"name":"identity","owner":{"name":"Tests"},"plugins":[{"name":"catalog-name","source":"./plugin"}]}`)
	writeTestFile(t, root, "plugin/.claude-plugin/plugin.json", `{"name":"skill-namespace"}`)
	writeTestFile(t, root, "plugin/skills/run/SKILL.md", "---\nname: run\n---\nRun")
	if err := manager.PutSource(context.Background(), MarketplaceSource{ID: "identity", Name: "identity", Enabled: true, Source: Source{Source: "directory", Path: root}}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Refresh(context.Background(), "identity"); err != nil {
		t.Fatal(err)
	}
	installation, err := manager.Install(context.Background(), "identity", "catalog-name")
	if err != nil {
		t.Fatal(err)
	}
	bundles, err := manager.Bundles()
	if err != nil || len(bundles) != 1 || installation.ID != "catalog-name@identity" || bundles[0].Skills[0].Name != "skill-namespace:run" {
		t.Fatalf("catalog identity overwrote manifest namespace: %+v %+v %v", installation, bundles, err)
	}
}
