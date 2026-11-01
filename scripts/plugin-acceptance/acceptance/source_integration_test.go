//go:build integration

package acceptance

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflineGitSourceNativeInstallAndDiscoveryNeverBecomeRuntimePass(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	files := map[string]string{
		".claude-plugin/marketplace.json":         `{"name":"fixture","owner":{"name":"Tests"},"plugins":[{"name":"demo","source":"./plugins/demo"},{"name":"alias","source":"./plugins/demo"}]}`,
		"plugins/demo/.claude-plugin/plugin.json": `{"name":"demo","version":"1.0.0"}`,
		"plugins/demo/skills/run/SKILL.md":        "---\nname: run\ndescription: Use this fixture\n---\nRead the fixture.",
		"plugins/demo/hooks/hooks.json":           `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"DO_NOT_EXECUTE_UNKNOWN_PLUGIN_SCRIPT"}]}]}}`,
	}
	for name, text := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init"}, {"add", "."}, {"-c", "user.name=Tests", "-c", "user.email=tests@example.invalid", "commit", "-m", "fixture"}} {
		if err := localGit(context.Background(), root, &bytes.Buffer{}, args...); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	if err := localGit(context.Background(), root, &output, "rev-parse", "HEAD"); err != nil {
		t.Fatal(err)
	}
	commit := strings.TrimSpace(output.String())
	report, err := Execute(context.Background(), Config{MarketplaceFile: filepath.Join(root, ".claude-plugin", "marketplace.json"), MarketplaceCommit: commit, MarketplaceRepository: "fixture/repo",
		SourceRoot: root, OutputDir: t.TempDir(), InstallAndDiscover: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	row := report.Rows[0]
	if len(report.Rows) != 2 || report.Rows[1].Discover.Status != "passed" {
		t.Fatal("independent candidates interfered through their shared component namespace")
	}
	if row.Source.Status != "verified" || row.Install.Status != "passed" || row.Discover.Status != "passed" {
		t.Fatalf("offline native stages failed: %+v", row)
	}
	if len(row.Components) != 2 {
		t.Fatalf("did not inventory skill and hook: %+v", row.Components)
	}
	if row.Runtime.Status != "not_tested" || row.Passed || report.Passed != 0 || report.TargetMet {
		t.Fatal("source/install/discovery stages were counted as runtime acceptance")
	}
	if row.ArtifactDigest == "" || row.ArtifactRoot == "" {
		t.Fatal("installed source bytes were not bound to evidence")
	}
}
