package plugins

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManagerLocalMarketInstallUpdateRollback(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Sources()) != 2 || len(m.Catalogs()) != 0 {
		t.Fatal("constructor must register defaults without fetching")
	}
	sourceRoot := t.TempDir()
	writeTestFile(t, sourceRoot, ".claude-plugin/marketplace.json", `{"name":"test-market","owner":{"name":"Tests"},"plugins":[{"name":"one","source":"./plugins/one"}]}`)
	writeTestFile(t, sourceRoot, "plugins/one/.claude-plugin/plugin.json", `{"name":"one","version":"1.0.0"}`)
	writeTestFile(t, sourceRoot, "plugins/one/skills/task/SKILL.md", "---\ndescription: Task\n---\nOld content")
	if err := m.PutSource(ctx, MarketplaceSource{ID: "test", Name: "test-market", Enabled: true, Source: Source{Source: "directory", Path: sourceRoot}}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Refresh(ctx, "test"); err != nil {
		t.Fatal(err)
	}
	installed, err := m.Install(ctx, "test-market", "one")
	if err != nil {
		t.Fatal(err)
	}
	oldBundles, err := m.Bundles()
	if err != nil || len(oldBundles) != 1 {
		t.Fatalf("bundles: %v %v", oldBundles, err)
	}
	writeTestFile(t, sourceRoot, "plugins/one/.claude-plugin/plugin.json", `{"name":"one","version":"2.0.0"}`)
	writeTestFile(t, sourceRoot, "plugins/one/skills/task/SKILL.md", "---\ndescription: Task\n---\nNew content")
	if _, err := m.Refresh(ctx, "test"); err != nil {
		t.Fatal(err)
	}
	updated, err := m.Update(ctx, installed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Path == installed.Path || updated.Version != "2.0.0" {
		t.Fatal("update did not install immutable version")
	}
	if _, err := os.Stat(oldBundles[0].Skills[0].Path); err != nil {
		t.Fatal("update removed a running bundle path")
	}
	writeTestFile(t, sourceRoot, "plugins/one/.claude-plugin/plugin.json", `{"name":"one","skills":"./missing"}`)
	if _, err := m.Refresh(ctx, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Update(ctx, installed.ID); err == nil {
		t.Fatal("broken update accepted")
	}
	if got := m.Installed(); len(got) != 1 || got[0].Path != updated.Path {
		t.Fatal("failed update corrupted active installation")
	}
	if err := m.SetEnabled(ctx, installed.ID, false); err != nil {
		t.Fatal(err)
	}
	if bundles, err := m.Bundles(); err != nil || len(bundles) != 0 {
		t.Fatalf("disabled bundle leaked: %v", err)
	}
	reopened, err := NewManager(m.root)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Installed()[0].Enabled {
		t.Fatal("disabled state was not persisted")
	}
	if err := reopened.Uninstall(ctx, installed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(updated.Path); err != nil {
		t.Fatal("uninstall removed retained version")
	}
}

func TestManagerScopeAndSnapshotIsolation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	market := t.TempDir()
	writeTestFile(t, market, ".claude-plugin/marketplace.json", `{"name":"scope-market","owner":{"name":"Tests"},"plugins":[{"name":"demo","source":"./plugin","commands":{"run":{"content":"hello"}}}]}`)
	writeTestFile(t, market, "plugin/readme.txt", "plugin")
	if err := m.PutSource(ctx, MarketplaceSource{ID: "scope", Name: "scope-market", Enabled: true, Source: Source{Source: "directory", Path: market}}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Refresh(ctx, "scope"); err != nil {
		t.Fatal(err)
	}
	install, err := m.Install(ctx, "scope", "demo")
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	options := InstallOptions{Scope: "project", ProjectPath: project}
	if _, err := m.Install(ctx, "scope", "demo", options); err != nil {
		t.Fatal(err)
	}
	if err := m.SetEnabled(ctx, install.ID, false, options); err != nil {
		t.Fatal(err)
	}
	if bundles, err := m.BundlesFor(filepath.Join(project, "nested")); err != nil || len(bundles) != 0 {
		t.Fatalf("disabled project must mask user: %v", err)
	}
	if bundles, err := m.BundlesFor(t.TempDir()); err != nil || len(bundles) != 1 {
		t.Fatalf("user install outside project: %v", err)
	}
	sources := m.Sources()
	sources[0].Source.Repo = "mutated"
	if m.Sources()[0].Source.Repo == "mutated" {
		t.Fatal("sources snapshot aliases internal state")
	}
}

func TestManagerPathEscapeAndCancellation(t *testing.T) {
	t.Parallel()
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	market := t.TempDir()
	writeTestFile(t, market, ".claude-plugin/marketplace.json", `{"name":"evil","owner":{"name":"Tests"},"plugins":[{"name":"demo","source":"./../../outside"}]}`)
	ctx := context.Background()
	if err := m.PutSource(ctx, MarketplaceSource{ID: "evil", Name: "evil", Enabled: true, Source: Source{Source: "directory", Path: market}}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Refresh(ctx, "evil"); err == nil {
		t.Fatal("marketplace path traversal accepted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := m.Refresh(cancelled, "evil"); err == nil {
		t.Fatal("cancelled refresh succeeded")
	}
	if len(m.Catalogs()) != 0 {
		t.Fatal("failed refresh published a catalog")
	}
}

func TestManagerHTTPRefreshKeepsProjectionResponsive(t *testing.T) {
	t.Parallel()
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = w.Write([]byte(`{"name":"http-market","owner":{"name":"Tests"},"plugins":[]}`))
	}))
	defer server.Close()
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.PutSource(context.Background(), MarketplaceSource{ID: "http", Name: "http-market", Enabled: true, Source: Source{Source: "url", URL: server.URL + "/catalog"}}); err != nil {
		t.Fatal(err)
	}
	refreshed := make(chan error, 1)
	go func() { _, err := m.Refresh(context.Background(), "http"); refreshed <- err }()
	<-started
	read := make(chan struct{})
	go func() { _ = m.Sources(); _ = m.Catalogs(); _ = m.Installed(); close(read) }()
	select {
	case <-read:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("catalog network refresh blocked in-memory projections")
	}
	close(release)
	if err := <-refreshed; err != nil {
		t.Fatal(err)
	}
}

func TestManagerInlineMarketplace(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestFile(t, root, ".claude-plugin/plugin.json", `{"name":"inline-plugin"}`)
	writeTestFile(t, root, "commands/run.md", "A command")
	source := Source{
		Source: "settings", Name: "inline-market", Owner: &Author{Name: "Tests"},
		Plugins: []MarketplaceEntry{{Manifest: Manifest{Name: "inline-plugin"}, Source: Source{Source: "directory", Path: root}}},
	}
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := m.PutSource(ctx, MarketplaceSource{ID: "inline", Name: "inline-market", Source: source, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Refresh(ctx, "inline"); err != nil {
		t.Fatal(err)
	}
	installation, err := m.Install(ctx, "inline", "inline-plugin")
	if err != nil {
		t.Fatal(err)
	}
	if installation.ID != "inline-plugin@inline-market" {
		t.Fatal(installation.ID)
	}
	serialized, err := json.Marshal(m.Sources())
	if err != nil || !strings.Contains(string(serialized), `"plugins"`) {
		t.Fatal("inline marketplace definition was lost")
	}
}

func TestManagerPersistsMarketplaceRootSourceSemantics(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, ".claude-plugin/marketplace.json", `{"name":"root-skills","owner":{"name":"Tests"},"plugins":[{"name":"subset","source":"./","strict":false,"skills":["./skills/first"]}]}`)
	writeTestFile(t, root, "skills/first/SKILL.md", "first")
	writeTestFile(t, root, "skills/second/SKILL.md", "second")
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.PutSource(ctx, MarketplaceSource{ID: "root", Name: "root-skills", Enabled: true, Source: Source{Source: "directory", Path: root}}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Refresh(ctx, "root"); err != nil {
		t.Fatal(err)
	}
	installed, err := manager.Install(ctx, "root", "subset")
	if err != nil {
		t.Fatal(err)
	}
	if installed.Source.Relative != "./" {
		t.Fatal("original root source provenance was not persisted")
	}
	reopened, err := NewManager(manager.root)
	if err != nil {
		t.Fatal(err)
	}
	bundles, err := reopened.Bundles()
	if err != nil {
		t.Fatal(err)
	}
	if len(bundles) != 1 || len(bundles[0].Skills) != 1 {
		t.Fatal("reopened root plugin loaded unrelated skills")
	}
}
