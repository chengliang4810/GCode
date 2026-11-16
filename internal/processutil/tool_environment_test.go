package processutil

import (
	"strings"
	"testing"
)

func TestToolEnvironmentDoesNotInheritAmbientCredentials(t *testing.T) {
	parent := []string{"PATH=/bin", "HOME=/home/user", "LANG=zh_CN.UTF-8", "LC_ALL=C", "AWS_SECRET_ACCESS_KEY=secret",
		"ANTHROPIC_API_KEY=secret", "GITHUB_TOKEN=secret", "BASH_ENV=/malicious", "CUSTOM_APP_SECRET=secret"}
	got := strings.Join(ToolEnvironment(parent), "\n")
	for _, required := range []string{"PATH=/bin", "HOME=/home/user", "LANG=zh_CN.UTF-8", "LC_ALL=C"} {
		if !strings.Contains(got, required) {
			t.Fatalf("missing runtime setting %s", required)
		}
	}
	if strings.Contains(got, "secret") || strings.Contains(got, "BASH_ENV") {
		t.Fatalf("ambient credential or startup code leaked: %s", got)
	}
	explicit := strings.Join(Environment(ToolEnvironment(parent), map[string]string{"CUSTOM_APP_SECRET": "intentional"}), "\n")
	if !strings.Contains(explicit, "CUSTOM_APP_SECRET=intentional") {
		t.Fatal("explicit child environment lost")
	}
}
