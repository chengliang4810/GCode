package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCollectResultsRequiresActualDesktopPassEvents(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  []string
	}{
		{"package pass alone", `{"Action":"pass","Package":"gcode/internal/e2e"}`, []string{}},
		{"skipped test", `{"Action":"skip","Test":"TestDesktopNoDisplay"}`, []string{}},
		{"unrelated pass", `{"Action":"pass","Test":"TestHelper"}`, []string{}},
		{"subtest alone", `{"Action":"pass","Test":"TestDesktopSettings/subtest"}`, []string{}},
		{"desktop pass", `{"Action":"pass","Test":"TestDesktopSettings"}`, []string{"TestDesktopSettings"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collectResults(strings.NewReader(tc.input), &bytes.Buffer{})
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("passed=%v, err=%v; want %v", got, err, tc.want)
			}
		})
	}
}

func TestCollectResultsPreservesOutputAndRejectsCorruptStream(t *testing.T) {
	var output bytes.Buffer
	passed, err := collectResults(strings.NewReader(
		"{\"Action\":\"output\",\"Output\":\"native test output\\n\"}\n"+
			"{\"Action\":\"pass\",\"Test\":\"TestDesktopSettings\"}\n"+
			"{\"Action\":\"pass\",\"Test\":\"TestDesktopSettings\"}\n"), &output)
	if err != nil || output.String() != "native test output\n" || len(passed) != 1 {
		t.Fatalf("passed=%v, output=%q, err=%v", passed, output.String(), err)
	}
	if _, err := collectResults(strings.NewReader(`{"Action":`), &output); err == nil {
		t.Fatal("truncated event stream accepted")
	}
}

func TestEvidenceDirectoryStaysInCheckout(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, err := evidenceDirectory(root, filepath.Join("output", "native tests"))
	if err != nil || got != filepath.Join(root, "output", "native tests") {
		t.Fatalf("directory=%q err=%v", got, err)
	}
	for _, outside := range []string{"../outside", root + "-other"} {
		if _, err := evidenceDirectory(root, outside); err == nil {
			t.Fatalf("outside directory %q accepted", outside)
		}
	}
}

func TestEvidenceDirectoryRejectsEscapingSymlink(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked-output")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Skipf("directory symlink unavailable: %v", err)
	}
	if _, err := evidenceDirectory(root, filepath.Join(link, "evidence")); err == nil {
		t.Fatal("output through an external symlink accepted")
	}
}

func TestPlatformBuildArguments(t *testing.T) {
	for _, goos := range []string{"windows", "linux", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			flags := linkerFlags(goos, "-X mygo.stamp=value")
			windows := goos == "windows"
			if strings.Contains(flags, "-H=windowsgui") != windows || strings.HasSuffix(binaryName(goos), ".exe") != windows {
				t.Fatalf("%s binary=%q flags=%q", goos, binaryName(goos), flags)
			}
			if !strings.Contains(flags, "-X mygo.stamp=value") {
				t.Fatal("framework stamp lost")
			}
		})
	}
}

func TestTestEnvironmentOverridesOnlyChildConfiguration(t *testing.T) {
	original := []string{"PATH=tools", "RCODE_E2E_BINARY=old", "rcode_e2e_artifacts=old"}
	got := testEnvironment(original, "new binary", "new artifacts")
	want := []string{"PATH=tools", "RCODE_E2E_BINARY=new binary", "RCODE_E2E_ARTIFACTS=new artifacts"}
	if !reflect.DeepEqual(got, want) || original[1] != "RCODE_E2E_BINARY=old" {
		t.Fatalf("environment=%v original=%v", got, original)
	}
}
