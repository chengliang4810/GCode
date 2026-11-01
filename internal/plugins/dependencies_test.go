package plugins

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func registerTestMarket(t *testing.T, m *Manager, sourceID, name string, plugins map[string]Manifest, allowed ...string) string {
	t.Helper()
	root := t.TempDir()
	market := Marketplace{Name: name, Owner: Author{Name: "Tests"}, AllowCrossMarketplaceDependenciesOn: allowed}
	names := make([]string, 0, len(plugins))
	for plugin := range plugins {
		names = append(names, plugin)
	}
	slices.Sort(names)
	for _, name := range names {
		manifest := plugins[name]
		manifest.Name = name
		data, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, root, "plugins/"+name+"/.claude-plugin/plugin.json", string(data))
		market.Plugins = append(market.Plugins, MarketplaceEntry{Manifest: Manifest{Name: name}, Source: Source{Relative: "./plugins/" + name}})
	}
	data, err := json.Marshal(market)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, ".claude-plugin/marketplace.json", string(data))
	if err := m.PutSource(context.Background(), MarketplaceSource{ID: sourceID, Name: name, Enabled: true, Source: Source{Source: "directory", Path: root}}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Refresh(context.Background(), sourceID); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestInstallDependenciesTransactionAndProtection(t *testing.T) {
	t.Parallel()
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registerTestMarket(t, m, "d", "deps", map[string]Manifest{
		"root":   {Dependencies: []Dependency{{Name: "left"}, {Name: "right"}}},
		"left":   {Dependencies: []Dependency{{Name: "shared", Version: "^1.0"}}},
		"right":  {Dependencies: []Dependency{{Name: "shared", Version: ">=1.2 <2"}}},
		"shared": {Version: "1.3.0"},
	})
	root, err := m.Install(context.Background(), "deps", "root")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Installed()) != 4 {
		t.Fatalf("diamond dependency installed duplicates: %+v", m.Installed())
	}
	var sharedPath string
	for _, installation := range m.Installed() {
		if installation.Name == "shared" {
			sharedPath = installation.Path
		}
	}
	if _, err := m.Install(context.Background(), "deps", "root"); err != nil {
		t.Fatal(err)
	}
	for _, installation := range m.Installed() {
		if installation.Name == "shared" && installation.Path != sharedPath {
			t.Fatal("satisfied dependency reinstalled")
		}
	}
	if err := m.SetEnabled(context.Background(), "shared@deps", false); err == nil || !strings.Contains(err.Error(), "requires shared@deps") {
		t.Fatalf("disabling required dependency: %v", err)
	}
	if err := m.Uninstall(context.Background(), "shared@deps"); err == nil {
		t.Fatal("uninstall broke enabled dependents")
	}
	if err := m.SetEnabled(context.Background(), root.ID, false); err != nil {
		t.Fatal(err)
	}
	// Left/right remain explicitly enabled installations and still require shared.
	if err := m.SetEnabled(context.Background(), "left@deps", false); err != nil {
		t.Fatal(err)
	}
	if err := m.SetEnabled(context.Background(), "right@deps", false); err != nil {
		t.Fatal(err)
	}
	if err := m.SetEnabled(context.Background(), "shared@deps", false); err != nil {
		t.Fatal(err)
	}
	if err := m.SetEnabled(context.Background(), root.ID, true); err == nil {
		t.Fatal("enabled root with disabled dependencies")
	}
}

func TestInstallDependencyFailuresLeaveRegistryUnchanged(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		plugins map[string]Manifest
	}{
		{"cycle", map[string]Manifest{"root": {Dependencies: []Dependency{{Name: "child"}}}, "child": {Dependencies: []Dependency{{Name: "root"}}}}},
		{"missing", map[string]Manifest{"root": {Dependencies: []Dependency{{Name: "child"}, {Name: "absent"}}}, "child": {}}},
		{"version", map[string]Manifest{"root": {Dependencies: []Dependency{{Name: "child", Version: "^2.0"}}}, "child": {Version: "1.0.0"}}},
		{"unsupported range", map[string]Manifest{"root": {Dependencies: []Dependency{{Name: "child", Version: "latest"}}}, "child": {Version: "1.0.0"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := NewManager(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			registerTestMarket(t, m, "fail", "failure", tc.plugins)
			if _, err := m.Install(context.Background(), "failure", "root"); err == nil {
				t.Fatal("bad dependency graph accepted")
			}
			if len(m.Installed()) != 0 {
				t.Fatal("part of failed dependency transaction was published")
			}
			reopened, err := NewManager(m.root)
			if err != nil {
				t.Fatal(err)
			}
			if len(reopened.Installed()) != 0 {
				t.Fatal("failed dependency transaction leaked to disk")
			}
		})
	}
}

func TestCrossMarketplaceDependencyRequiresRootAllowlist(t *testing.T) {
	t.Parallel()
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registerTestMarket(t, m, "shared-source", "shared-market", map[string]Manifest{"library": {}})
	registerTestMarket(t, m, "root-source", "root-market", map[string]Manifest{"app": {Dependencies: []Dependency{{Name: "library", Marketplace: "shared-market"}}}})
	if _, err := m.Install(context.Background(), "root-market", "app"); err == nil || !strings.Contains(err.Error(), "allowCrossMarketplaceDependenciesOn") {
		t.Fatalf("unexpected cross-marketplace result: %v", err)
	}
	if len(m.Installed()) != 0 {
		t.Fatal("blocked cross-marketplace transaction mutated registry")
	}
	if _, err := m.Install(context.Background(), "shared-market", "library"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Install(context.Background(), "root-market", "app"); err != nil {
		t.Fatal("explicitly enabled same-scope dependency must be accepted:", err)
	}
	m2, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registerTestMarket(t, m2, "shared-source", "shared-market", map[string]Manifest{"library": {}})
	registerTestMarket(t, m2, "root-source", "root-market", map[string]Manifest{"app": {Dependencies: []Dependency{{Name: "library@shared-market"}}}}, "shared-market")
	if _, err := m2.Install(context.Background(), "root-market", "app"); err != nil {
		t.Fatal("root allowlist should allow dependency install:", err)
	}
}

func TestVersionConstraints(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		version, constraint string
		valid               bool
	}{
		{"1.3.0", "^1.2", true}, {"2.0.0", "^1.2", false}, {"0.3.0", "^0", true},
		{"0.0.5", "^0.0", true}, {"0.1.0", "^0.0.4", false}, {"0.0.5", "^0.0.4", false},
		{"2.1.8", "~2.1.0", true}, {"2.2.0", "~2.1.0", false},
		{"1.5.0", ">=1.2 <2", true}, {"2.1.0", "^1 || ^2", true},
		{"2.1.0-beta.1", "^2", false}, {"2.0.0-beta.1", "^2.0.0-0", true},
		{"1.2.3", "1.x", true}, {"2.2.3", "1.x", false}, {"1.2.3", "=1.2.3", true},
		{"1.2.3", "latest", false}, {"", "^1", false},
	} {
		t.Run(tc.version+"/"+tc.constraint, func(t *testing.T) {
			if got := checkVersion(tc.version, tc.constraint) == nil; got != tc.valid {
				t.Fatalf("version %s constraint %s: got %v", tc.version, tc.constraint, got)
			}
		})
	}
}
