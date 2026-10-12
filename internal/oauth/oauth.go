// Package oauth contains the OAuth 2.0 protocol and token-storage primitives
// used by GCode's MCP client. MCP owns the login orchestration and server
// identity.
//
// Security invariants (enforced throughout, never relaxed):
//   - PKCE S256 is mandatory on every authorization-code flow; "plain" is refused.
//   - A per-flow CSRF state is generated and verified on the callback.
//   - The MCP loopback callback server binds 127.0.0.1 only, on an OS-assigned
//     port, serves a single request, then closes.
//   - The token endpoint must be https (loopback exempt) before any credential is
//     sent — see validateTokenEndpoint.
//   - Tokens, codes, and verifiers are never logged; error bodies are redacted.
//   - Token files are 0600, base-confined, and file-locked.
package oauth

import (
	"errors"
	"strings"
	"time"
)

// Token holds the credentials issued by an authorization server. The token
// fields are sensitive: callers must never log them, and the store persists them
// 0600. It mirrors the on-disk shape internal/mcp uses so the two stay
// format-compatible.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	Scopes       []string  `json:"scopes,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
}

// Expired reports whether the token has an expiry that is at or before now.
// A zero ExpiresAt means "no known expiry" and is treated as not expired.
func (t Token) Expired(now time.Time) bool {
	return !t.ExpiresAt.IsZero() && !t.ExpiresAt.After(now)
}

// NeedsRefresh reports whether the token is expired or falls within buffer of
// expiry (so a proactive/on-demand refresh should run). A token with no expiry
// never needs a refresh on a timer.
func (t Token) NeedsRefresh(now time.Time, buffer time.Duration) bool {
	if t.ExpiresAt.IsZero() {
		return false
	}
	return !t.ExpiresAt.After(now.Add(buffer))
}

// Config describes how to talk to one authorization server for a flow.
// Endpoints may be discovered (RFC 8414) and overridden here.
type Config struct {
	ClientID     string
	ClientSecret string
	Scopes       []string

	AuthorizationEndpoint string
	TokenEndpoint         string
	RegistrationEndpoint  string
	// IssuerURL is the base for metadata discovery when endpoints are not set.
	IssuerURL string

	// ExtraAuthParams are appended to the authorization URL (e.g. login_hint).
	ExtraAuthParams map[string]string
}

// Errors returned by the engine. Callers can match these with errors.Is.
var (
	// ErrPKCEDowngrade is returned if a flow is asked to use anything but S256.
	ErrPKCEDowngrade = errors.New("oauth: PKCE S256 is mandatory; plain is refused")
	// ErrStateMismatch is returned when the callback state does not match (CSRF).
	ErrStateMismatch = errors.New("oauth: callback state mismatch; possible CSRF, login aborted")
	// ErrInsecureTokenEndpoint is returned when a credential would be sent over a
	// non-https, non-loopback endpoint.
	ErrInsecureTokenEndpoint = errors.New("oauth: refusing to send credential to a non-https token endpoint")
	// ErrUnsafeRedirect is returned when a credential-bearing OAuth POST (token
	// exchange/refresh) is redirected.
	// Following it would replay the form body — codes, PKCE verifiers, refresh
	// tokens, client secrets — to a target that never passed endpoint validation.
	ErrUnsafeRedirect = errors.New("oauth: refusing to follow a redirect from a credential endpoint")
	// ErrNoRefreshToken is returned when a refresh is attempted without one.
	ErrNoRefreshToken = errors.New("oauth: no refresh token available")
)

// trimmed is a tiny helper used across the package.
func trimmed(s string) string { return strings.TrimSpace(s) }
