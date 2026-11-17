package redact

import "testing"

func TestBuiltInKindsIncludeModelOutputKinds(t *testing.T) {
	got := map[string]bool{}
	for _, kind := range BuiltInKinds() {
		got[kind] = true
	}
	for _, kind := range []string{"PERSON", "ADDRESS", "PHONE", "EMAIL", "ACCOUNT", "SECRET"} {
		if !got[kind] {
			t.Errorf("built-in kind %q is missing", kind)
		}
	}
	for _, label := range []string{"private_person", "private_address", "private_phone", "private_email", "account_number", "secret"} {
		if got[label] {
			t.Errorf("model label %q leaked into built-in kinds", label)
		}
	}
}
