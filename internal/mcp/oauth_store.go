package mcp

import (
	"fmt"
	"strings"
	"time"

	"gcode/internal/oauth"
)

const mcpServerIdentityLength = 32
const maxOAuthTokenKeySegmentLength = 128
const maxMCPServerNameForIdentityToken = maxOAuthTokenKeySegmentLength - 1 - mcpServerIdentityLength

// StoredToken holds the credentials issued by an OAuth 2.0 authorization server
// for a single MCP server. The token fields are sensitive: they are tagged so
// the repo's redaction layer masks them, and they must never be written to logs
// or stream output.
type StoredToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	Scopes       []string  `json:"scopes,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
}

// TokenStatus is a redaction-safe summary of a stored token. It deliberately
// omits the access and refresh token material so it can be printed by the CLI.
type TokenStatus struct {
	ServerName      string    `json:"serverName"`
	ServerIdentity  string    `json:"serverIdentity,omitempty"`
	HasToken        bool      `json:"hasToken"`
	HasRefreshToken bool      `json:"hasRefreshToken"`
	TokenType       string    `json:"tokenType,omitempty"`
	Scopes          []string  `json:"scopes,omitempty"`
	ExpiresAt       time.Time `json:"expiresAt,omitempty"`
	Expired         bool      `json:"expired"`
}

// TokenStoreOptions configures the MCP OAuth token store. FilePath overrides
// the application-scoped store path; when empty, the GCode MCP OAuth default is
// used. Env and Now are primarily useful for hermetic tests.
type TokenStoreOptions struct {
	FilePath string
	Env      map[string]string
	Now      func() time.Time
}

// TokenStore persists MCP OAuth tokens in the MCP-only oauth store under the
// "mcp:" namespace. It never reads or migrates the retired GCode OAuth paths.
type TokenStore struct {
	store *oauth.Store
}

// ResolveTokenStorePath determines the default GCode MCP OAuth token path.
// The application runtime passes an explicit OAuthTokenPath so each dataDir
// remains isolated; this helper exists for standalone callers and tests.
func ResolveTokenStorePath(env map[string]string) (string, error) {
	return oauth.ResolveStorePath(env)
}

// NewTokenStore builds the MCP-only token store.
func NewTokenStore(options TokenStoreOptions) (*TokenStore, error) {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	unified, err := oauth.NewStore(oauth.StoreOptions{
		FilePath: options.FilePath,
		Env:      options.Env,
		Now:      now,
	})
	if err != nil {
		return nil, err
	}
	return &TokenStore{store: unified}, nil
}

// FilePath returns the resolved unified store path.
func (store *TokenStore) FilePath() string {
	return store.store.FilePath()
}

// SaveForServer persists a token under the server's target identity. Identity
// binding prevents a project config with the same server name from reusing a
// token issued for a different MCP endpoint.
func (store *TokenStore) SaveForServer(server Server, token StoredToken) error {
	key, err := mcpIdentityKey(server)
	if err != nil {
		return err
	}
	return store.store.Save(key, storedToOAuth(token))
}

// LoadForServer returns the token matching the server's target identity.
func (store *TokenStore) LoadForServer(server Server) (StoredToken, bool, error) {
	key, err := mcpIdentityKey(server)
	if err != nil {
		return StoredToken{}, false, err
	}
	token, ok, err := store.store.Load(key)
	return tokenToStored(token), ok, err
}

// DeleteForServer removes the stored token for the server's target identity. It
// reports whether an entry was present before deletion.
func (store *TokenStore) DeleteForServer(server Server) (bool, error) {
	key, err := mcpIdentityKey(server)
	if err != nil {
		return false, err
	}
	return store.store.Delete(key)
}

// Status returns a redaction-safe summary of every identity-bound MCP token,
// sorted by server name. Retired name-only entries are ignored and never
// included in the active credential view.
func (store *TokenStore) Status() ([]TokenStatus, error) {
	statuses, err := store.store.Status(oauth.KeyPrefixMCP)
	if err != nil {
		return nil, err
	}
	out := make([]TokenStatus, 0, len(statuses))
	for _, s := range statuses {
		serverName, serverIdentity, ok := splitMCPIdentityKeySegment(strings.TrimPrefix(s.Key, oauth.KeyPrefixMCP))
		if !ok {
			// Name-only entries belong to the retired storage format. They remain
			// untouched on disk, but are neither readable nor presented as active
			// credentials by the identity-bound store.
			continue
		}
		out = append(out, TokenStatus{
			ServerName:      serverName,
			ServerIdentity:  serverIdentity,
			HasToken:        s.HasToken,
			HasRefreshToken: s.HasRefreshToken,
			TokenType:       s.TokenType,
			Scopes:          s.Scopes,
			ExpiresAt:       s.ExpiresAt,
			Expired:         s.Expired,
		})
	}
	return out, nil
}

func mcpIdentityKey(server Server) (string, error) {
	name := strings.TrimSpace(server.Name)
	if err := ValidateServerName(name); err != nil {
		return "", err
	}
	if hasMCPIdentitySuffix(name) {
		return "", fmt.Errorf("MCP OAuth server name %q cannot end with .<32 hex chars> because it collides with identity-bound token storage", name)
	}
	identity := strings.TrimSpace(server.Identity)
	if len(name) > maxMCPServerNameForIdentityToken {
		return "", fmt.Errorf("MCP OAuth server name %q is too long for identity-bound token storage", name)
	}
	if len(identity) != mcpServerIdentityLength || !isMCPServerIdentity(identity) {
		return "", fmt.Errorf("MCP OAuth server %s has invalid identity %q", name, identity)
	}
	key := oauth.KeyPrefixMCP + name + "." + identity
	if err := oauth.ValidateKey(key); err != nil {
		return "", err
	}
	return key, nil
}

func splitMCPIdentityKeySegment(segment string) (string, string, bool) {
	index := strings.LastIndex(segment, ".")
	if index <= 0 || index == len(segment)-1 {
		return segment, "", false
	}
	identity := segment[index+1:]
	if len(identity) != mcpServerIdentityLength || !isMCPServerIdentity(identity) {
		return segment, "", false
	}
	return segment[:index], identity, true
}

func hasMCPIdentitySuffix(value string) bool {
	_, _, ok := splitMCPIdentityKeySegment(value)
	return ok
}

func isMCPServerIdentity(value string) bool {
	for _, ch := range value {
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F') {
			continue
		}
		return false
	}
	return value != ""
}

// storedToOAuth converts an MCP StoredToken to the OAuth protocol token (the
// inverse of tokenToStored).
func storedToOAuth(s StoredToken) oauth.Token {
	return oauth.Token{
		AccessToken:  s.AccessToken,
		RefreshToken: s.RefreshToken,
		TokenType:    s.TokenType,
		Scopes:       s.Scopes,
		ExpiresAt:    s.ExpiresAt,
	}
}

// FormatTokenStatuses renders a human-readable status table without leaking any
// token material.
func FormatTokenStatuses(statuses []TokenStatus) string {
	if len(statuses) == 0 {
		return "No MCP OAuth tokens are stored."
	}
	var builder strings.Builder
	for index, status := range statuses {
		if index > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(status.ServerName)
		if status.ServerIdentity != "" {
			builder.WriteString(" [")
			builder.WriteString(status.ServerIdentity)
			builder.WriteString("]")
		}
		builder.WriteString(": ")
		if !status.HasToken {
			builder.WriteString("no token")
			continue
		}
		builder.WriteString("token present")
		if status.HasRefreshToken {
			builder.WriteString(" (refreshable)")
		}
		if !status.ExpiresAt.IsZero() {
			if status.Expired {
				builder.WriteString(", expired at ")
			} else {
				builder.WriteString(", expires ")
			}
			builder.WriteString(status.ExpiresAt.UTC().Format(time.RFC3339))
		}
	}
	return builder.String()
}
