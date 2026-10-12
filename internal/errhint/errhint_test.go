package errhint

import "testing"

func TestHasStatusCodeRequiresDigitBoundaries(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "standalone", text: "upstream returned 503", want: true},
		{name: "start of text", text: "429 too many requests", want: true},
		{name: "embedded suffix", text: "completed in 4290ms", want: false},
		{name: "embedded prefix", text: "1429 from proxy", want: false},
		{name: "missing", text: "provider request failed", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := HasStatusCode(test.text, "429", "503"); got != test.want {
				t.Fatalf("HasStatusCode(%q) = %t, want %t", test.text, got, test.want)
			}
		})
	}
}
