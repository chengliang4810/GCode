package contracts

import "testing"

func TestSameProjectPathIgnoresHostSeparators(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		same bool
	}{
		{"host backslashes for browsed slashes", `C:/Users/idvhghol/notes`, `C:\Users\idvhghol\notes`, true},
		{"trailing separator", `C:/Users/idvhghol/notes/`, `C:\Users\idvhghol\notes`, true},
		{"volume root", `C:/`, `C:\`, true},
		{"unix path", "/home/idvhghol/notes", "/home/idvhghol/notes", true},
		{"different folders", `C:/Users/idvhghol/notes`, `C:\Users\idvhghol\other`, false},
		{"different drives", `C:/notes`, `D:/notes`, false},
		{"parent folder", `C:/Users/idvhghol`, `C:/Users/idvhghol/notes`, false},
		{"empty", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SameProjectPath(tc.a, tc.b); got != tc.same {
				t.Fatalf("SameProjectPath(%q, %q) = %t, want %t", tc.a, tc.b, got, tc.same)
			}
		})
	}
}
