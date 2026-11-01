//go:build integration

package plugins

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"
)

func TestManagerGitPinnedPluginAndCrossSource(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	ctx := context.Background()
	repo := t.TempDir()
	writeTestFile(t, repo, "sub/.claude-plugin/plugin.json", `{"name":"remote","version":"1.0.0"}`)
	writeTestFile(t, repo, "sub/commands/run.md", "---\ndescription: Git plugin\n---\nHello")
	for _, args := range [][]string{{"init"}, {"add", "."}, {"-c", "user.name=Tests", "-c", "user.email=tests@example.invalid", "commit", "-m", "fixture"}} {
		if _, err := git(ctx, repo, args...); err != nil {
			t.Fatal(err)
		}
	}
	sha, err := git(ctx, repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	market := t.TempDir()
	writeTestFile(t, market, ".claude-plugin/marketplace.json", `{"name":"git-market","owner":{"name":"Tests"},"plugins":[{"name":"remote","source":{"source":"git-subdir","url":`+jsonString(repo)+`,"path":"sub","sha":"`+sha+`"}}]}`)
	for _, args := range [][]string{{"init"}, {"add", "."}, {"-c", "user.name=Tests", "-c", "user.email=tests@example.invalid", "commit", "-m", "market"}} {
		if _, err := git(ctx, market, args...); err != nil {
			t.Fatal(err)
		}
	}
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.PutSource(ctx, MarketplaceSource{ID: "git-test", Name: "git-market", Enabled: true, Source: Source{Source: "git", URL: market}}); err != nil {
		t.Fatal(err)
	}
	catalog, err := m.Refresh(ctx, "git-test")
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Commit == "" {
		t.Fatal("marketplace commit missing")
	}
	install, err := m.Install(ctx, "git-market", "remote")
	if err != nil {
		t.Fatal(err)
	}
	if install.Commit != sha {
		t.Fatalf("wanted %s, got %s", sha, install.Commit)
	}
	bundles, err := m.Bundles()
	if err != nil || len(bundles) != 1 || len(bundles[0].Commands) != 1 {
		t.Fatalf("remote plugin not loaded: %v", err)
	}
}

func jsonString(s string) string {
	// Windows paths in temporary Git fixtures must be valid JSON strings.
	data, _ := json.Marshal(s)
	return string(data)
}

func TestManagerDependencyChoosesHighestSatisfyingGitTag(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	ctx := context.Background()
	repo := t.TempDir()
	if _, err := git(ctx, repo, "init"); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"1.0.0", "2.1.0", "2.4.0", "3.0.0"} {
		writeTestFile(t, repo, "sub/.claude-plugin/plugin.json", `{"name":"library","version":"`+version+`"}`)
		writeTestFile(t, repo, "sub/commands/run.md", "Version "+version)
		for _, args := range [][]string{
			{"add", "."}, {"-c", "user.name=Tests", "-c", "user.email=tests@example.invalid", "commit", "-m", version},
			{"tag", "library--v" + version},
		} {
			if _, err := git(ctx, repo, args...); err != nil {
				t.Fatal(err)
			}
		}
	}
	market := t.TempDir()
	writeTestFile(t, market, ".claude-plugin/marketplace.json", `{"name":"tag-market","owner":{"name":"Tests"},"plugins":[
		{"name":"app","source":"./plugins/app"},
		{"name":"library","source":{"source":"git-subdir","url":`+jsonString(repo)+`,"path":"sub"}}
	]}`)
	writeTestFile(t, market, "plugins/app/.claude-plugin/plugin.json", `{"name":"app","dependencies":[{"name":"library","version":"^2.0"}]}`)
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.PutSource(ctx, MarketplaceSource{ID: "tags", Name: "tag-market", Enabled: true, Source: Source{Source: "directory", Path: market}}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Refresh(ctx, "tags"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Install(ctx, "tag-market", "app"); err != nil {
		t.Fatal(err)
	}
	if len(m.Installed()) != 2 {
		t.Fatal("dependency graph was not installed")
	}
	for _, installed := range m.Installed() {
		if installed.Name == "library" && installed.Version != "2.4.0" {
			t.Fatalf("wanted highest ^2 tag, got %s", installed.Version)
		}
	}
}
