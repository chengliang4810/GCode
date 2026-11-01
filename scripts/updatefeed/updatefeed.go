// Package updatefeed verifies the signed files published for desktop updates.
package updatefeed

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Version string `json:"version"`
	Updates struct {
		PublicKey string `json:"publicKey"`
		GitHub    string `json:"github"`
	} `json:"updates"`
}

func Load(path string) (Config, error) {
	var c Config
	data, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, err
	}
	if c.Version == "" || strings.ContainsAny(c.Version, "/\\") {
		return c, fmt.Errorf("invalid update version")
	}
	parts := strings.Split(c.Updates.GitHub, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(c.Updates.GitHub, "?#\\ \r\n") {
		return c, fmt.Errorf("updates.github must be owner/repository")
	}
	_, err = c.publicKey()
	return c, err
}

func (c Config) publicKey() (ed25519.PublicKey, error) {
	key, err := base64.StdEncoding.DecodeString(c.Updates.PublicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid updates.publicKey")
	}
	return ed25519.PublicKey(key), nil
}

// CheckSigningKey fails before a release build can silently omit its updates.
func (c Config) CheckSigningKey(text string) error {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil || len(key) != ed25519.PrivateKeySize {
		return fmt.Errorf("a valid MYGO_UPDATER_PRIVATE_KEY is required for release builds")
	}
	derived := ed25519.NewKeyFromSeed(key[:ed25519.SeedSize])
	if base64.StdEncoding.EncodeToString(derived) != base64.StdEncoding.EncodeToString(key) ||
		base64.StdEncoding.EncodeToString(derived.Public().(ed25519.PublicKey)) != c.Updates.PublicKey {
		return fmt.Errorf("update signing key does not match updates.publicKey")
	}
	return nil
}

type archive struct {
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	Signature string `json:"signature"`
}

type manifest struct {
	Version string `json:"version"`
	Notes   string `json:"notes"`
	Date    string `json:"date"`
	archive
	Deltas []archive `json:"deltas"`
}

// Verify checks each platform's own manifest and every newly signed download.
func (c Config) Verify(root string, targets []string) error {
	key, err := c.publicKey()
	if err != nil {
		return err
	}
	for _, target := range targets {
		platform := strings.ReplaceAll(target, "/", "-")
		if strings.ContainsAny(platform, "\\.") || strings.Count(target, "/") != 1 {
			return fmt.Errorf("invalid update target %q", target)
		}
		dir := filepath.Join(root, platform)
		data, err := os.ReadFile(filepath.Join(dir, "update-"+platform+".json"))
		if err != nil {
			return err
		}
		var m manifest
		if err := json.Unmarshal(data, &m); err != nil {
			return err
		}
		if m.Version != c.Version || strings.TrimSpace(m.Notes) == "" {
			return fmt.Errorf("%s: update version or release notes are missing or incorrect", platform)
		}
		if _, err := time.Parse(time.RFC3339, m.Date); err != nil {
			return fmt.Errorf("%s: invalid update date", platform)
		}
		for _, file := range append([]archive{m.archive}, m.Deltas...) {
			if err := c.verifyArchive(dir, key, file); err != nil {
				return fmt.Errorf("%s: %w", platform, err)
			}
		}
	}
	return nil
}

func (c Config) verifyArchive(dir string, key ed25519.PublicKey, file archive) error {
	u, err := url.Parse(file.URL)
	if err != nil {
		return fmt.Errorf("invalid update archive URL")
	}
	name := filepath.Base(u.Path)
	expected := "https://github.com/" + c.Updates.GitHub + "/releases/download/v" + c.Version + "/" + url.PathEscape(name)
	if name == "." || name == "" || strings.ContainsAny(name, "\\") || file.URL != expected {
		return fmt.Errorf("update archive URL does not belong to this release")
	}
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	defer f.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, f)
	if err != nil {
		return err
	}
	if size <= 0 || size != file.Size {
		return fmt.Errorf("%s: update archive size mismatch", name)
	}
	sig, err := base64.StdEncoding.DecodeString(file.Signature)
	if err != nil || !ed25519.Verify(key, hash.Sum(nil), sig) {
		return fmt.Errorf("%s: update signature verification failed", name)
	}
	return nil
}
