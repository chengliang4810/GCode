package redact

import (
	"strings"
	"testing"
)

func TestTextStreamRestoresLowercaseBarePlaceholderAcrossFragments(t *testing.T) {
	const raw = "13987654321"
	masked, count := Mask(raw, Options{Personal: true})
	if count != 1 || !strings.HasPrefix(masked, "{{PHONE_") {
		t.Fatalf("phone was not masked: count=%d masked=%q", count, masked)
	}

	bare := strings.TrimSuffix(strings.TrimPrefix(masked, "{{"), "}}")
	mixed := []byte(strings.ToLower(bare))
	for i, c := range mixed {
		if i%2 == 0 && c >= 'a' && c <= 'z' {
			mixed[i] = c - ('a' - 'A')
		}
	}

	cases := []struct {
		name string
		text string
		want string
	}{
		{name: "lowercase bare placeholder", text: strings.ToLower(bare), want: raw},
		{name: "mixed case bare placeholder", text: string(mixed), want: raw},
		// "phone" is a normal word that also prefixes the built-in PHONE kind.
		// It must be emitted intact after the stream decides it is not a placeholder.
		{name: "lowercase kind prefix word", text: "phone", want: "phone"},
		{name: "ordinary lowercase word", text: "hello", want: "hello"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for cut := 0; cut <= len(tc.text); cut++ {
				var stream TextStream
				got := stream.Write(tc.text[:cut])
				got += stream.Write(tc.text[cut:])
				got += stream.Finish()
				if got != tc.want {
					t.Errorf("split at %d/%d restored %q, want %q from %q", cut, len(tc.text), got, tc.want, tc.text)
				}
			}
		})
	}
}
