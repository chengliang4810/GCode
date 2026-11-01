package main

import (
	"testing"

	"github.com/egoist/mygo"
)

func TestEnforceSingleInstance(t *testing.T) {
	for _, tc := range []struct {
		name           string
		scenario       string
		headless       bool
		allow, enforce string
		want           bool
	}{
		// Plain `go test` binaries carry no package metadata.
		{"unpackaged default", "", false, "", "", false},
		{"scenario never claims", "default", false, "", "", false},
		{"headless never claims", "", true, "", "", false},
		{"allow wins over enforce", "", false, "1", "1", false},
		{"environ forces dev build", "", false, "", "1", true},
		{"environ ignored with allow", "", false, "1", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RCODE_ALLOW_MULTIPLE_INSTANCE", tc.allow)
			t.Setenv("RCODE_ENFORCE_SINGLE_INSTANCE", tc.enforce)
			if mygo.App.IsPackaged() {
				t.Skip("packaged test binary: the unpackaged default does not apply")
			}
			if got := enforceSingleInstance(tc.scenario, tc.headless); got != tc.want {
				t.Fatalf("enforceSingleInstance(%q, %v) = %v, want %v", tc.scenario, tc.headless, got, tc.want)
			}
		})
	}
}
