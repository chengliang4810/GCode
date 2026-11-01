package processutil

import (
	"runtime"
	"slices"
	"testing"
)

func TestEnvironmentLayersDoNotMutateParentOrExplicitConfiguration(t *testing.T) {
	parent := []string{"TEMP=parent", "KEEP=yes", "=C:=C:\\work"}
	explicit := map[string]string{"TEMP": "user"}
	got := Environment(parent, map[string]string{"TEMP": "root", "TMP": "root"}, explicit)
	for _, want := range []string{"TEMP=user", "TMP=root", "KEEP=yes", "=C:=C:\\work"} {
		if !slices.Contains(got, want) {
			t.Fatalf("missing %q: %v", want, got)
		}
	}
	if parent[0] != "TEMP=parent" || len(explicit) != 1 {
		t.Fatal("environment merge mutated inputs")
	}
	cloned := EnvironmentMap(map[string]string{"TMP": "root"}, explicit)
	cloned["TEMP"] = "changed"
	if explicit["TEMP"] != "user" {
		t.Fatal("merged map aliases user configuration")
	}
	if runtime.GOOS == "windows" {
		got = Environment([]string{"Temp=parent"}, map[string]string{"TEMP": "root"}, map[string]string{"temp": "user"})
		if len(got) != 1 || got[0] != "temp=user" {
			t.Fatalf("Windows environment has duplicate case-insensitive keys: %v", got)
		}
	}
}
