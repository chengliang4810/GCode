package mcp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// AuthChallenge contains only transport-level authorization information. It
// never carries a bearer token or copies a WWW-Authenticate error description.
type AuthChallenge struct {
	Server              string
	ResourceURL         string
	ResourceMetadataURL string
	Scope               string
	StatusCode          int
}
type AuthRequiredError struct{ Challenge AuthChallenge }

func (e *AuthRequiredError) Error() string {
	return fmt.Sprintf("MCP server %q requires authorization (HTTP %d)", e.Challenge.Server, e.Challenge.StatusCode)
}

var authParameter = regexp.MustCompile(`(?i)([a-z_]+)\s*=\s*(?:"([^"\\]*(?:\\.[^"\\]*)*)"|([^,\s]+))`)

func challengeFromResponse(server, resource string, response *http.Response) AuthChallenge {
	c := AuthChallenge{Server: server, ResourceURL: resource, StatusCode: response.StatusCode}
	for _, header := range response.Header.Values("WWW-Authenticate") {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(header)), "bearer ") {
			continue
		}
		for _, m := range authParameter.FindAllStringSubmatch(header, -1) {
			value := m[2]
			if value == "" {
				value = m[3]
			}
			switch strings.ToLower(m[1]) {
			case "resource_metadata":
				c.ResourceMetadataURL = value
			case "scope":
				c.Scope = value
			}
		}
	}
	return c
}

// AuthorizationMetadata binds validated protected-resource and issuer metadata.
// It is an input to an explicit user-initiated flow; discovery never opens a
// browser, registers a client, requests a token, or grants any scopes.
type AuthorizationMetadata struct {
	Resource                          string   `json:"resource"`
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint,omitempty"`
	ScopesSupported                   []string `json:"scopes_supported,omitempty"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported,omitempty"`
	ClientIDMetadataDocumentSupported bool     `json:"client_id_metadata_document_supported,omitempty"`
}

func oauthHTTP(client *http.Client) *http.Client {
	c := &http.Client{Timeout: 20 * time.Second}
	if client != nil {
		*c = *client
		if c.Timeout == 0 {
			c.Timeout = 20 * time.Second
		}
	}
	c.Jar = nil
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c
}
func secureOAuthURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, errors.New("OAuth endpoint must be an absolute URL without user information or fragment")
	}
	host := parsed.Hostname()
	ip := net.ParseIP(host)
	local := strings.EqualFold(host, "localhost") || ip != nil && ip.IsLoopback()
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && local) {
		return nil, errors.New("OAuth endpoint requires HTTPS (loopback HTTP is permitted)")
	}
	return parsed, nil
}
func oauthJSON(ctx context.Context, client *http.Client, target string, result any) error {
	if _, err := secureOAuthURL(target); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return errors.New("invalid OAuth metadata URL")
	}
	req.Header.Set("Accept", "application/json")
	response, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("OAuth metadata request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return &HTTPStatusError{StatusCode: response.StatusCode}
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return errors.New("OAuth metadata response exceeds limit or could not be read")
	}
	if err = json.Unmarshal(raw, result); err != nil {
		return errors.New("OAuth metadata response contains invalid JSON")
	}
	return nil
}

func DiscoverAuthorization(ctx context.Context, client *http.Client, challenge AuthChallenge) (AuthorizationMetadata, error) {
	resource, err := secureOAuthURL(challenge.ResourceURL)
	if err != nil {
		return AuthorizationMetadata{}, err
	}
	httpClient := oauthHTTP(client)
	candidates := []string{challenge.ResourceMetadataURL}
	if candidates[0] == "" {
		base := resource.Scheme + "://" + resource.Host
		path := strings.TrimRight(resource.EscapedPath(), "/")
		candidates = []string{base + "/.well-known/oauth-protected-resource" + path}
		if path != "" {
			candidates = append(candidates, base+"/.well-known/oauth-protected-resource")
		}
	}
	var protected struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
		ScopesSupported      []string `json:"scopes_supported"`
	}
	for i, target := range candidates {
		err = oauthJSON(ctx, httpClient, target, &protected)
		if err == nil {
			break
		}
		var status *HTTPStatusError
		if !errors.As(err, &status) || status.StatusCode != http.StatusNotFound || i == len(candidates)-1 {
			return AuthorizationMetadata{}, err
		}
	}
	audience, err := secureOAuthURL(protected.Resource)
	if err != nil {
		return AuthorizationMetadata{}, errors.New("OAuth protected-resource metadata omitted a valid resource identifier")
	}
	if audience.Scheme != resource.Scheme || !strings.EqualFold(audience.Host, resource.Host) || !resourcePathContains(audience.Path, resource.Path) {
		return AuthorizationMetadata{}, errors.New("OAuth protected-resource metadata does not describe the configured MCP resource")
	}
	if len(protected.AuthorizationServers) == 0 {
		return AuthorizationMetadata{}, errors.New("OAuth protected-resource metadata has no authorization servers")
	}
	// Select an advertised issuer, never infer a token endpoint from the MCP URL.
	issuer := protected.AuthorizationServers[0]
	issuerURL, err := secureOAuthURL(issuer)
	if err != nil {
		return AuthorizationMetadata{}, err
	}
	if issuerURL.RawQuery != "" {
		return AuthorizationMetadata{}, errors.New("OAuth issuer must not contain a query")
	}
	base := issuerURL.Scheme + "://" + issuerURL.Host
	path := strings.TrimRight(issuerURL.EscapedPath(), "/")
	asCandidates := []string{base + "/.well-known/oauth-authorization-server" + path, strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"}
	var metadata AuthorizationMetadata
	for i, target := range asCandidates {
		err = oauthJSON(ctx, httpClient, target, &metadata)
		if err == nil {
			break
		}
		var status *HTTPStatusError
		if !errors.As(err, &status) || status.StatusCode != http.StatusNotFound || i == len(asCandidates)-1 {
			return AuthorizationMetadata{}, err
		}
	}
	if metadata.Issuer != issuer {
		return AuthorizationMetadata{}, errors.New("OAuth authorization-server metadata issuer does not match the advertised issuer")
	}
	for _, target := range []string{metadata.AuthorizationEndpoint, metadata.TokenEndpoint} {
		if _, err := secureOAuthURL(target); err != nil {
			return AuthorizationMetadata{}, err
		}
	}
	if !contains(metadata.CodeChallengeMethodsSupported, "S256") {
		return AuthorizationMetadata{}, errors.New("OAuth authorization server does not advertise required PKCE S256 support")
	}
	metadata.Resource = protected.Resource
	if challenge.Scope != "" {
		metadata.ScopesSupported = strings.Fields(challenge.Scope)
	} else {
		metadata.ScopesSupported = append([]string(nil), protected.ScopesSupported...)
	}
	return metadata, nil
}
func resourcePathContains(audience, request string) bool {
	audience = strings.TrimRight(audience, "/")
	return audience == "" || request == audience || strings.HasPrefix(request, audience+"/")
}

// OAuthFlow implements an explicit OAuth 2.1 public-client authorization-code
// flow with PKCE, resource binding, state and issuer validation. The owner must
// persist tokens through its own protected credential store; this package does
// not store tokens or start an implicit browser/callback listener.
type OAuthFlow struct {
	AuthorizationURL string
	metadata         AuthorizationMetadata
	clientID         string
	redirectURI      string
	state            string
	verifier         string
	http             *http.Client
	mu               sync.Mutex
	consumed         bool
}

func NewOAuthFlow(metadata AuthorizationMetadata, clientID, redirectURI string, scopes []string, client *http.Client) (*OAuthFlow, error) {
	if strings.TrimSpace(clientID) == "" {
		return nil, errors.New("OAuth requires a preregistered client ID or client metadata document URL")
	}
	if _, err := secureOAuthURL(metadata.Issuer); err != nil {
		return nil, err
	}
	endpoint, err := secureOAuthURL(metadata.AuthorizationEndpoint)
	if err != nil {
		return nil, err
	}
	for _, target := range []string{metadata.TokenEndpoint, metadata.Resource, redirectURI} {
		if _, err := secureOAuthURL(target); err != nil {
			return nil, err
		}
	}
	if !contains(metadata.CodeChallengeMethodsSupported, "S256") {
		return nil, errors.New("OAuth requires PKCE S256")
	}
	verifierBytes := make([]byte, 32)
	stateBytes := make([]byte, 32)
	if _, err = rand.Read(verifierBytes); err != nil {
		return nil, err
	}
	if _, err = rand.Read(stateBytes); err != nil {
		return nil, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	state := base64.RawURLEncoding.EncodeToString(stateBytes)
	sum := sha256.Sum256([]byte(verifier))
	query := endpoint.Query()
	query.Set("response_type", "code")
	query.Set("client_id", clientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("state", state)
	query.Set("code_challenge", base64.RawURLEncoding.EncodeToString(sum[:]))
	query.Set("code_challenge_method", "S256")
	query.Set("resource", metadata.Resource)
	if len(scopes) > 0 {
		query.Set("scope", strings.Join(scopes, " "))
	}
	endpoint.RawQuery = query.Encode()
	return &OAuthFlow{AuthorizationURL: endpoint.String(), metadata: metadata, clientID: clientID, redirectURI: redirectURI, state: state, verifier: verifier, http: oauthHTTP(client)}, nil
}

// OAuthToken deliberately omits credentials from JSON and fmt diagnostics. An
// owner persists AccessToken/RefreshToken explicitly in its credential store.
type OAuthToken struct {
	AccessToken  string    `json:"-"`
	RefreshToken string    `json:"-"`
	TokenType    string    `json:"tokenType"`
	ExpiresAt    time.Time `json:"expiresAt,omitempty"`
	Scope        string    `json:"scope,omitempty"`
}

func (OAuthToken) String() string   { return "OAuthToken{credentials:[redacted]}" }
func (OAuthToken) GoString() string { return "OAuthToken{credentials:[redacted]}" }
func (f *OAuthFlow) Complete(ctx context.Context, callbackURL string) (OAuthToken, error) {
	callback, err := url.Parse(callbackURL)
	if err != nil {
		return OAuthToken{}, errors.New("invalid OAuth callback")
	}
	redirect, _ := url.Parse(f.redirectURI)
	if callback.Scheme != redirect.Scheme || callback.Host != redirect.Host || callback.Path != redirect.Path || callback.Fragment != "" || callback.User != nil {
		return OAuthToken{}, errors.New("OAuth callback does not match the registered redirect URI")
	}
	query := callback.Query()
	for _, key := range []string{"state", "code", "iss", "error"} {
		if len(query[key]) > 1 {
			return OAuthToken{}, errors.New("OAuth callback contains duplicate response parameters")
		}
	}
	for key, values := range redirect.Query() {
		if len(query[key]) != len(values) {
			return OAuthToken{}, errors.New("OAuth callback changed a registered redirect parameter")
		}
		for i, value := range values {
			if query[key][i] != value {
				return OAuthToken{}, errors.New("OAuth callback changed a registered redirect parameter")
			}
		}
	}
	state := query.Get("state")
	if subtle.ConstantTimeCompare([]byte(state), []byte(f.state)) != 1 {
		return OAuthToken{}, errors.New("OAuth callback state mismatch")
	}
	if query.Get("iss") != f.metadata.Issuer {
		return OAuthToken{}, errors.New("OAuth callback issuer mismatch or missing issuer")
	}
	if query.Get("error") != "" {
		return OAuthToken{}, errors.New("OAuth authorization was declined or failed")
	}
	code := query.Get("code")
	if code == "" {
		return OAuthToken{}, errors.New("OAuth callback omitted its authorization code")
	}
	f.mu.Lock()
	if f.consumed {
		f.mu.Unlock()
		return OAuthToken{}, errors.New("OAuth authorization code has already been consumed")
	}
	f.consumed = true
	f.mu.Unlock()
	values := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {f.redirectURI}, "client_id": {f.clientID}, "code_verifier": {f.verifier}, "resource": {f.metadata.Resource}}
	return f.token(ctx, values)
}
func (f *OAuthFlow) Refresh(ctx context.Context, refreshToken string) (OAuthToken, error) {
	if refreshToken == "" {
		return OAuthToken{}, errors.New("OAuth refresh token is missing")
	}
	return f.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refreshToken}, "client_id": {f.clientID}, "resource": {f.metadata.Resource}})
}
func (f *OAuthFlow) token(ctx context.Context, values url.Values) (OAuthToken, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.metadata.TokenEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return OAuthToken{}, errors.New("invalid OAuth token endpoint")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	response, err := f.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return OAuthToken{}, ctx.Err()
		}
		return OAuthToken{}, errors.New("OAuth token request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return OAuthToken{}, fmt.Errorf("OAuth token request rejected (HTTP %d)", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return OAuthToken{}, errors.New("OAuth token response exceeds limit or could not be read")
	}
	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if json.Unmarshal(raw, &result) != nil || result.AccessToken == "" || !strings.EqualFold(result.TokenType, "bearer") {
		return OAuthToken{}, errors.New("OAuth token response omitted a valid bearer token")
	}
	token := OAuthToken{AccessToken: result.AccessToken, RefreshToken: result.RefreshToken, TokenType: "Bearer", Scope: result.Scope}
	if result.ExpiresIn > 0 && result.ExpiresIn < 31536000 {
		token.ExpiresAt = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	}
	return token, nil
}

// RegisteredOAuthClient binds public-client registration to its issuer/resource.
// Registration is an explicit operation; Connect never registers automatically.
type RegisteredOAuthClient struct {
	ClientID string
	Issuer   string
	Resource string
}

func RegisterOAuthClient(ctx context.Context, client *http.Client, metadata AuthorizationMetadata, redirectURI, clientName string) (RegisteredOAuthClient, error) {
	if metadata.RegistrationEndpoint == "" {
		return RegisteredOAuthClient{}, errors.New("OAuth server has no dynamic registration endpoint; configure a client ID or metadata document")
	}
	if _, err := secureOAuthURL(metadata.RegistrationEndpoint); err != nil {
		return RegisteredOAuthClient{}, err
	}
	if _, err := secureOAuthURL(redirectURI); err != nil {
		return RegisteredOAuthClient{}, err
	}
	if _, err := secureOAuthURL(metadata.Issuer); err != nil {
		return RegisteredOAuthClient{}, err
	}
	body := map[string]any{"client_name": clientName, "application_type": "native", "redirect_uris": []string{redirectURI}, "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}, "token_endpoint_auth_method": "none"}
	raw, err := json.Marshal(body)
	if err != nil {
		return RegisteredOAuthClient{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, metadata.RegistrationEndpoint, strings.NewReader(string(raw)))
	if err != nil {
		return RegisteredOAuthClient{}, errors.New("invalid OAuth registration endpoint")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	response, err := oauthHTTP(client).Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return RegisteredOAuthClient{}, ctx.Err()
		}
		return RegisteredOAuthClient{}, errors.New("OAuth client registration request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		return RegisteredOAuthClient{}, fmt.Errorf("OAuth client registration rejected (HTTP %d)", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return RegisteredOAuthClient{}, errors.New("OAuth registration response could not be read or exceeds limit")
	}
	var result struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		AuthMethod   string `json:"token_endpoint_auth_method"`
	}
	if json.Unmarshal(data, &result) != nil || result.ClientID == "" {
		return RegisteredOAuthClient{}, errors.New("OAuth registration omitted a client ID")
	}
	if result.ClientSecret != "" || result.AuthMethod != "" && result.AuthMethod != "none" {
		return RegisteredOAuthClient{}, errors.New("OAuth registration did not grant public-client authentication")
	}
	return RegisteredOAuthClient{ClientID: result.ClientID, Issuer: metadata.Issuer, Resource: metadata.Resource}, nil
}
func NewRegisteredOAuthFlow(metadata AuthorizationMetadata, registration RegisteredOAuthClient, redirectURI string, scopes []string, client *http.Client) (*OAuthFlow, error) {
	if registration.Issuer != metadata.Issuer || registration.Resource != metadata.Resource {
		return nil, errors.New("OAuth client registration belongs to a different issuer or resource")
	}
	return NewOAuthFlow(metadata, registration.ClientID, redirectURI, scopes, client)
}
