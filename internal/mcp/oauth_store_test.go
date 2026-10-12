package mcp

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestTokenStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp-oauth-tokens.json")
	store, err := NewTokenStore(TokenStoreOptions{FilePath: path})
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}
	server := testOAuthServer("demo", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false)

	expiry := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	saved := StoredToken{
		AccessToken:  "access-abc",
		RefreshToken: "refresh-abc",
		TokenType:    "Bearer",
		Scopes:       []string{"read", "write"},
		ExpiresAt:    expiry,
	}
	if err := store.SaveForServer(server, saved); err != nil {
		t.Fatalf("SaveForServer() error = %v", err)
	}

	loaded, ok, err := store.LoadForServer(server)
	if err != nil {
		t.Fatalf("LoadForServer() error = %v", err)
	}
	if !ok {
		t.Fatal("LoadForServer() ok = false, want stored token")
	}
	if loaded.AccessToken != saved.AccessToken || loaded.RefreshToken != saved.RefreshToken {
		t.Fatalf("loaded = %#v", loaded)
	}
	if !loaded.ExpiresAt.Equal(expiry) {
		t.Fatalf("expiry = %v, want %v", loaded.ExpiresAt, expiry)
	}
}

func TestTokenStoreIdentityBoundRoundTrip(t *testing.T) {
	store, err := NewTokenStore(TokenStoreOptions{FilePath: filepath.Join(t.TempDir(), "oauth-tokens.json")})
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}
	server := testOAuthServer("demo", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false)
	otherTarget := testOAuthServer("demo", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", false)
	if err := store.SaveForServer(server, StoredToken{AccessToken: "identity-token"}); err != nil {
		t.Fatalf("SaveForServer() error = %v", err)
	}

	loaded, ok, err := store.LoadForServer(server)
	if err != nil || !ok {
		t.Fatalf("LoadForServer() ok=%v err=%v", ok, err)
	}
	if loaded.AccessToken != "identity-token" {
		t.Fatalf("access token = %q", loaded.AccessToken)
	}
	if _, ok, err := store.LoadForServer(otherTarget); err != nil || ok {
		t.Fatalf("LoadForServer(other target) ok=%v err=%v, want no token", ok, err)
	}
}

func TestStoreTokenSourceRequiresMatchingIdentity(t *testing.T) {
	store, err := NewTokenStore(TokenStoreOptions{FilePath: filepath.Join(t.TempDir(), "oauth-tokens.json")})
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}
	server := testOAuthServer("demo", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false)
	otherTarget := testOAuthServer("demo", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", false)
	if err := store.SaveForServer(server, StoredToken{AccessToken: "access-a"}); err != nil {
		t.Fatalf("SaveForServer() error = %v", err)
	}

	source := &storeTokenSource{server: server, store: store}
	token, err := source.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("AccessToken() error = %v", err)
	}
	if token != "access-a" {
		t.Fatalf("token = %q, want access-a", token)
	}

	source.server = otherTarget
	if _, err := source.AccessToken(context.Background()); err == nil {
		t.Fatal("AccessToken() error = nil for mismatched identity")
	}
}

func TestTokenStoreRejectsLongIdentityBoundServerName(t *testing.T) {
	store, err := NewTokenStore(TokenStoreOptions{FilePath: filepath.Join(t.TempDir(), "oauth-tokens.json")})
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}
	name := strings.Repeat("a", maxMCPServerNameForIdentityToken+1)
	err = store.SaveForServer(testOAuthServer(name, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false), StoredToken{AccessToken: "x"})
	if err == nil {
		t.Fatal("SaveForServer() error = nil for long server name")
	}
	if !strings.Contains(err.Error(), `MCP OAuth server name "`+name+`" is too long for identity-bound token storage`) {
		t.Fatalf("error = %q, want long-name message", err.Error())
	}
}

func TestTokenStoreRejectsAmbiguousIdentitySuffixServerName(t *testing.T) {
	store, err := NewTokenStore(TokenStoreOptions{FilePath: filepath.Join(t.TempDir(), "oauth-tokens.json")})
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}
	name := "demo.aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	if err := store.SaveForServer(testOAuthServer(name, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", false), StoredToken{AccessToken: "x"}); err == nil {
		t.Fatal("SaveForServer() error = nil for ambiguous server name")
	} else if !strings.Contains(err.Error(), `MCP OAuth server name "`+name+`" cannot end with .<32 hex chars>`) {
		t.Fatalf("SaveForServer() error = %q, want ambiguous-name rejection", err.Error())
	}
}

func TestTokenStoreDeleteForServerRemovesOnlyMatchingIdentityToken(t *testing.T) {
	store, err := NewTokenStore(TokenStoreOptions{FilePath: filepath.Join(t.TempDir(), "oauth-tokens.json")})
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}
	serverA := testOAuthServer("demo", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false)
	serverB := testOAuthServer("demo", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", false)
	other := testOAuthServer("other", "cccccccccccccccccccccccccccccccc", false)
	if err := store.SaveForServer(serverA, StoredToken{AccessToken: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveForServer(serverB, StoredToken{AccessToken: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveForServer(other, StoredToken{AccessToken: "c"}); err != nil {
		t.Fatal(err)
	}

	removed, err := store.DeleteForServer(serverA)
	if err != nil {
		t.Fatalf("DeleteForServer() error = %v", err)
	}
	if !removed {
		t.Fatal("DeleteForServer() removed = false")
	}
	if _, ok, err := store.LoadForServer(serverA); err != nil || ok {
		t.Fatalf("LoadForServer(serverA) ok=%v err=%v, want missing", ok, err)
	}
	if token, ok, err := store.LoadForServer(serverB); err != nil || !ok || token.AccessToken != "b" {
		t.Fatalf("LoadForServer(serverB) token=%#v ok=%v err=%v, want token b", token, ok, err)
	}
}

func TestTokenStoreStatusParsesIdentityAndRedacts(t *testing.T) {
	store, err := NewTokenStore(TokenStoreOptions{FilePath: filepath.Join(t.TempDir(), "oauth-tokens.json")})
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}
	identity := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := store.SaveForServer(testOAuthServer("demo", identity, false), StoredToken{AccessToken: "secret-access", RefreshToken: "secret-refresh"}); err != nil {
		t.Fatalf("SaveForServer() error = %v", err)
	}
	statuses, err := store.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if len(statuses) != 1 || statuses[0].ServerName != "demo" || statuses[0].ServerIdentity != identity {
		t.Fatalf("statuses = %#v, want parsed identity", statuses)
	}
	output := FormatTokenStatuses(statuses)
	if !strings.Contains(output, "demo ["+identity+"]") {
		t.Fatalf("status output = %q, want identity label", output)
	}
	if contains(output, "secret-access") || contains(output, "secret-refresh") {
		t.Fatalf("status output leaked token: %s", output)
	}
}

func TestTokenStoreFileIs0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file permissions are not enforced on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp-oauth-tokens.json")
	store, err := NewTokenStore(TokenStoreOptions{FilePath: path})
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}
	server := testOAuthServer("demo", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false)
	if err := store.SaveForServer(server, StoredToken{AccessToken: "a", RefreshToken: "r"}); err != nil {
		t.Fatalf("SaveForServer() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat token file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file mode = %o, want 600", perm)
	}
}

func TestTokenStoreDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp-oauth-tokens.json")
	store, err := NewTokenStore(TokenStoreOptions{FilePath: path})
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}
	server := testOAuthServer("demo", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false)
	if err := store.SaveForServer(server, StoredToken{AccessToken: "a", RefreshToken: "r"}); err != nil {
		t.Fatalf("SaveForServer() error = %v", err)
	}
	removed, err := store.DeleteForServer(server)
	if err != nil {
		t.Fatalf("DeleteForServer() error = %v", err)
	}
	if !removed {
		t.Fatal("DeleteForServer() removed = false, want true")
	}
	_, ok, err := store.LoadForServer(server)
	if err != nil {
		t.Fatalf("LoadForServer() after delete error = %v", err)
	}
	if ok {
		t.Fatal("LoadForServer() ok = true after delete")
	}

	// Deleting a missing entry reports false without error.
	removed, err = store.DeleteForServer(testOAuthServer("missing", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", false))
	if err != nil {
		t.Fatalf("DeleteForServer(missing) error = %v", err)
	}
	if removed {
		t.Fatal("DeleteForServer(missing) removed = true, want false")
	}
}

func TestTokenStoreLoadMissingIsNotError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp-oauth-tokens.json")
	store, err := NewTokenStore(TokenStoreOptions{FilePath: path})
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}
	_, ok, err := store.LoadForServer(testOAuthServer("demo", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false))
	if err != nil {
		t.Fatalf("LoadForServer() error = %v", err)
	}
	if ok {
		t.Fatal("LoadForServer() ok = true for empty store")
	}
}

func TestTokenStoreStatusReportsPresenceWithoutToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp-oauth-tokens.json")
	store, err := NewTokenStore(TokenStoreOptions{FilePath: path})
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}
	expiry := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	if err := store.SaveForServer(testOAuthServer("demo", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false), StoredToken{
		AccessToken:  "secret-access",
		RefreshToken: "secret-refresh",
		ExpiresAt:    expiry,
	}); err != nil {
		t.Fatalf("SaveForServer() error = %v", err)
	}

	statuses, err := store.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("statuses = %#v, want one entry", statuses)
	}
	status := statuses[0]
	if status.ServerName != "demo" {
		t.Fatalf("server name = %q", status.ServerName)
	}
	if !status.HasToken {
		t.Fatal("HasToken = false, want true")
	}
	if !status.HasRefreshToken {
		t.Fatal("HasRefreshToken = false, want true")
	}
	if !status.ExpiresAt.Equal(expiry) {
		t.Fatalf("expiry = %v, want %v", status.ExpiresAt, expiry)
	}
	// The status struct must not carry the secret material at all.
	if got := FormatTokenStatuses(statuses); contains(got, "secret-access") || contains(got, "secret-refresh") {
		t.Fatalf("status output leaked token: %s", got)
	}
}

func TestResolveTokenStorePathUsesXDG(t *testing.T) {
	// Use a real temp dir so the base is absolute on every OS (a literal
	// "/tmp/..." isn't absolute on Windows, where ResolveTokenStorePath would
	// then prepend the drive letter and diverge from a hard-coded want).
	configHome := t.TempDir()
	path, err := ResolveTokenStorePath(map[string]string{"XDG_CONFIG_HOME": configHome})
	if err != nil {
		t.Fatalf("ResolveTokenStorePath() error = %v", err)
	}
	want := filepath.Join(configHome, "gcode", "oauth-tokens.json")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestTokenStoreDefaultIgnoresRetiredZeroPath(t *testing.T) {
	configHome := t.TempDir()
	legacyPath := filepath.Join(configHome, "zero", "oauth-tokens.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte(`{"schemaVersion":1,"tokens":{"demo":{"access_token":"legacy"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		"HOME":                       filepath.Join(configHome, "home"),
		"XDG_CONFIG_HOME":            configHome,
		"ZERO_MCP_OAUTH_TOKENS_PATH": legacyPath,
	}
	store, err := NewTokenStore(TokenStoreOptions{Env: env})
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}
	want := filepath.Join(configHome, "gcode", "oauth-tokens.json")
	if store.FilePath() != want {
		t.Fatalf("FilePath() = %q, want GCode path %q", store.FilePath(), want)
	}
	if _, ok, err := store.LoadForServer(testOAuthServer("demo", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false)); err != nil || ok {
		t.Fatalf("LoadForServer() ok=%v err=%v, retired Zero token must not be read", ok, err)
	}
	if _, err := os.Stat(legacyPath + ".migrated"); !os.IsNotExist(err) {
		t.Fatalf("retired Zero token was migrated, stat err=%v", err)
	}
}

func testOAuthServer(name string, identity string, projectConfigured bool) Server {
	return Server{
		Name:              name,
		Type:              ServerTypeHTTP,
		URL:               "https://example.com/mcp",
		Auth:              ServerAuthOAuth,
		Identity:          identity,
		ProjectConfigured: projectConfigured,
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
