package contracts

import "testing"

func TestValidateRemoteURLRequiresTLSOutsideLoopback(t *testing.T) {
	for _, test := range []struct {
		name, address string
		want          string
		rejected      bool
	}{
		{"https host", "https://host.example:27655", "https://host.example:27655", false},
		{"https host trailing slash", "https://host.example/", "https://host.example", false},
		{"plaintext host", "http://host.example:27655", "", true},
		{"plaintext public IP", "http://203.0.113.9:27655", "", true},
		{"plaintext LAN IP", "http://192.168.1.2:27655", "", true},
		{"plaintext IPv4 loopback", "http://127.0.0.1:27655", "http://127.0.0.1:27655", false},
		{"plaintext IPv4 loopback without port", "http://127.0.0.1", "http://127.0.0.1", false},
		{"plaintext IPv6 loopback", "http://[::1]:27655", "http://[::1]:27655", false},
		{"plaintext localhost", "http://localhost:27655", "http://localhost:27655", false},
		{"credentials in URL", "https://user:secret@host.example", "", true},
		{"query string", "https://host.example?token=1", "", true},
		{"fragment", "https://host.example#x", "", true},
		{"missing host", "https://", "", true},
		{"other scheme", "ftp://host.example", "", true},
		{"garbage", "host.example", "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := ValidateRemoteURL(test.address)
			if (err != nil) != test.rejected {
				t.Fatalf("ValidateRemoteURL(%q) error = %v, wanted rejection %v", test.address, err, test.rejected)
			}
			if got != test.want {
				t.Fatalf("ValidateRemoteURL(%q) = %q, wanted %q", test.address, got, test.want)
			}
		})
	}
}
