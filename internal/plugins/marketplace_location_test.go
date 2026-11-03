package plugins

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestMarketplaceLocationDetection(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "marketplace.json", `{"name":"file-market","owner":{"name":"Tests"},"plugins":[]}`)
	for _, test := range []struct {
		location, kind, address string
	}{
		{"owner/repository", "github", "owner/repository"},
		{" owner/repository.git ", "github", "owner/repository"},
		{"https://github.com/owner/repository", "git", "https://github.com/owner/repository"},
		{"https://git.example.invalid/team/market.git", "git", "https://git.example.invalid/team/market.git"},
		{"git@example.invalid:team/market.git", "git", "git@example.invalid:team/market.git"},
		{"ssh://git@example.invalid/team/market.git", "git", "ssh://git@example.invalid/team/market.git"},
		{"https://example.invalid/catalog/marketplace.json?version=1", "url", "https://example.invalid/catalog/marketplace.json?version=1"},
		{root, "directory", root},
		{filepath.Join(root, "marketplace.json"), "file", filepath.Join(root, "marketplace.json")},
	} {
		t.Run(test.kind+"-"+test.location, func(t *testing.T) {
			source, err := parseMarketplaceLocation(test.location)
			if err != nil {
				t.Fatal(err)
			}
			address := source.URL + source.Repo + source.Path
			if source.Source != test.kind || address != test.address {
				t.Fatalf("unexpected source: %+v", source)
			}
		})
	}
	for _, invalid := range []string{"", "owner", "-option", "owner/repo\ncommand", "ftp://example.invalid/market", filepath.Join(root, "missing.json")} {
		if _, err := parseMarketplaceLocation(invalid); err == nil {
			t.Fatalf("invalid location accepted: %q", invalid)
		}
	}
}

func TestManagerAddSourceLoadsNameAndCatalogAtomically(t *testing.T) {
	ctx := context.Background()
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	market := t.TempDir()
	writeTestFile(t, market, ".claude-plugin/marketplace.json", `{"name":"actual-market","owner":{"name":"Tests"},"plugins":[{"name":"demo","source":"./plugin"}]}`)
	writeTestFile(t, market, "plugin/.claude-plugin/plugin.json", `{"name":"demo"}`)
	source, err := manager.AddSource(ctx, market)
	if err != nil {
		t.Fatal(err)
	}
	if source.ID != "actual-market" || source.Name != "actual-market" || source.Source.Source != "directory" {
		t.Fatalf("name was not read from manifest: %+v", source)
	}
	if catalogs := manager.Catalogs(); len(catalogs) != 1 || catalogs[0].SourceID != source.ID || len(catalogs[0].Marketplace.Plugins) != 1 {
		t.Fatalf("add did not load catalog: %+v", catalogs)
	}
	if _, err := manager.Install(ctx, source.ID, "demo"); err != nil {
		t.Fatal("new source was not immediately installable:", err)
	}
	before := len(manager.Sources())
	if _, err := manager.AddSource(ctx, market); err == nil || len(manager.Sources()) != before {
		t.Fatal("duplicate marketplace changed registry")
	}
	broken := t.TempDir()
	writeTestFile(t, broken, ".claude-plugin/marketplace.json", `{"name":"broken-market","plugins":[]}`)
	if _, err := manager.AddSource(ctx, broken); err == nil || len(manager.Sources()) != before || len(manager.Catalogs()) != 1 {
		t.Fatal("invalid marketplace published a partial registration")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := manager.AddSource(cancelled, broken); err == nil {
		t.Fatal("cancelled addition succeeded")
	}
}

func TestManagerAddSourceManifestFileAndURL(t *testing.T) {
	for _, kind := range []string{"file", "url"} {
		t.Run(kind, func(t *testing.T) {
			manifest := fmt.Sprintf(`{"name":"%s-market","owner":{"name":"Tests"},"plugins":[]}`, kind)
			root := t.TempDir()
			writeTestFile(t, root, "marketplace.json", manifest)
			location := filepath.Join(root, "marketplace.json")
			if kind == "url" {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = w.Write([]byte(manifest))
				}))
				t.Cleanup(server.Close)
				location = server.URL + "/marketplace.json"
			}
			manager, err := NewManager(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			source, err := manager.AddSource(context.Background(), location)
			if err != nil || source.ID != kind+"-market" || source.Source.Source != kind || len(manager.Catalogs()) != 1 {
				t.Fatalf("manifest addition failed: %+v %v", source, err)
			}
		})
	}
}
