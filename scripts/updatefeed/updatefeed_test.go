package updatefeed

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func signedFixture(t *testing.T) (Config, ed25519.PrivateKey, string, manifest) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var c Config
	c.Version = "0.1.0"
	c.Updates.PublicKey = base64.StdEncoding.EncodeToString(pub)
	c.Updates.GitHub = "example/releases"
	root := t.TempDir()
	dir := filepath.Join(root, "windows-amd64")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	body := []byte("signed update archive")
	sum := sha256.Sum256(body)
	if err := os.WriteFile(filepath.Join(dir, "rcode-0.1.0-windows-amd64.tar.gz"), body, 0644); err != nil {
		t.Fatal(err)
	}
	m := manifest{Version: c.Version, Notes: "Release notes", Date: time.Now().UTC().Format(time.RFC3339),
		archive: archive{
			URL:       "https://github.com/example/releases/releases/download/v0.1.0/rcode-0.1.0-windows-amd64.tar.gz",
			Size:      int64(len(body)),
			Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(key, sum[:])),
		}}
	return c, key, root, m
}

func writeManifest(t *testing.T, root string, m manifest) {
	t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "windows-amd64", "update-windows-amd64.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestSignedReleaseVerification(t *testing.T) {
	c, key, root, m := signedFixture(t)
	writeManifest(t, root, m)
	if err := c.CheckSigningKey(base64.StdEncoding.EncodeToString(key)); err != nil {
		t.Fatal(err)
	}
	if err := c.Verify(root, []string{"windows/amd64"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Verify(root, []string{"darwin/arm64"}); err == nil {
		t.Fatal("missing platform manifest accepted")
	}
}

func TestRejectUnsafeRelease(t *testing.T) {
	for _, kind := range []string{"tampered archive", "wrong size", "wrong signature", "wrong version", "wrong repository", "missing notes", "unsigned delta"} {
		t.Run(kind, func(t *testing.T) {
			c, _, root, m := signedFixture(t)
			switch kind {
			case "tampered archive":
				if err := os.WriteFile(filepath.Join(root, "windows-amd64", "rcode-0.1.0-windows-amd64.tar.gz"),
					[]byte("tamper update archive"), 0644); err != nil {
					t.Fatal(err)
				}
			case "wrong size":
				m.Size++
			case "wrong signature":
				_, other, _ := ed25519.GenerateKey(rand.Reader)
				sum := sha256.Sum256([]byte("signed update archive"))
				m.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(other, sum[:]))
			case "wrong version":
				m.Version = "0.2.0"
			case "wrong repository":
				m.URL = "https://github.com/other/releases/releases/download/v0.1.0/rcode-0.1.0-windows-amd64.tar.gz"
			case "missing notes":
				m.Notes = ""
			case "unsigned delta":
				m.Deltas = []archive{m.archive}
				m.Deltas[0].Signature = ""
			}
			writeManifest(t, root, m)
			if err := c.Verify(root, []string{"windows/amd64"}); err == nil {
				t.Fatal("invalid release accepted")
			}
		})
	}
}

func TestRejectIncorrectSigningKey(t *testing.T) {
	c, key, _, _ := signedFixture(t)
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	corrupt := append([]byte(nil), key...)
	corrupt[len(corrupt)-1] ^= 1
	for _, text := range []string{"", "invalid", base64.StdEncoding.EncodeToString(other), base64.StdEncoding.EncodeToString(corrupt)} {
		if err := c.CheckSigningKey(text); err == nil {
			t.Fatal("invalid or mismatched signing key accepted")
		}
	}
}
