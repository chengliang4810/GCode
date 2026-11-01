package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHTTPAuthorizationRequiredAndInjectedTokensAreTypedAndRedacted(t *testing.T) {
	const token = "oauth-injected-sensitive-token"
	var issued atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="https://auth.example.test/.well-known/oauth-protected-resource", scope="tools:read", error_description="do not expose this"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		request := fixtureRequest(t, r)
		switch request.Method {
		case "initialize":
			fixtureResult(w, request, map[string]any{"protocolVersion": LatestHandshakeVersion})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			fixtureResult(w, request, map[string]any{"tools": []any{fixtureTool()}})
		case "tools/call":
			fixtureResult(w, request, map[string]any{"content": []any{map[string]any{"type": "text", "text": "credential " + token}}, "structuredContent": map[string]any{"credential": token}})
		}
	}))
	defer server.Close()
	cfg := Config{Name: "auth", URL: server.URL + "?key=configured-query-secret"}
	client, err := Connect(context.Background(), cfg, t.TempDir())
	var auth *AuthRequiredError
	if client != nil || !errors.As(err, &auth) || auth.Challenge.StatusCode != 401 || auth.Challenge.Scope != "tools:read" {
		t.Fatalf("authorization not surfaced: %v %#v", err, auth)
	}
	if strings.Contains(auth.Challenge.ResourceURL, "configured-query-secret") || strings.Contains(err.Error(), "do not expose") {
		t.Fatal("authorization diagnostic leaked credentials or server error description")
	}
	client, err = ConnectWithOptions(context.Background(), cfg, t.TempDir(), Options{TokenProvider: func(_ context.Context, challenge AuthChallenge) (string, error) {
		issued.Add(1)
		if challenge.Scope != "tools:read" {
			t.Error(challenge)
		}
		return token, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	tools, err := client.Tools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out, err := tools[0].Execute(context.Background(), "", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if issued.Load() != 1 || strings.Contains(string(raw), token) || !strings.Contains(string(raw), "[redacted]") {
		t.Fatalf("token lifetime/redaction incorrect: issued=%d output=%s", issued.Load(), raw)
	}
}

func TestOAuthDiscoveryPKCEIssuerStateExchangeAndRefresh(t *testing.T) {
	var tokenCalls atomic.Int64
	var expectedChallenge string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("metadata/token endpoint received MCP bearer credentials")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource/mcp":
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": server.URL + "/mcp", "authorization_servers": []string{server.URL + "/issuer"}, "scopes_supported": []string{"tools:read"}})
		case "/.well-known/oauth-authorization-server/issuer":
			w.WriteHeader(http.StatusNotFound)
		case "/issuer/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": server.URL + "/issuer", "authorization_endpoint": server.URL + "/authorize", "token_endpoint": server.URL + "/token", "code_challenge_methods_supported": []string{"S256"}})
		case "/token":
			tokenCalls.Add(1)
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.Form.Get("resource") != server.URL+"/mcp" || r.Form.Get("client_id") != "registered-fixture-client" {
				t.Error("token request lacks resource/client binding")
			}
			switch r.Form.Get("grant_type") {
			case "authorization_code":
				sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
				if base64.RawURLEncoding.EncodeToString(sum[:]) != expectedChallenge || r.Form.Get("code") != "one-use-code" || r.Form.Get("redirect_uri") != "http://127.0.0.1:45678/callback" {
					t.Error("PKCE code exchange did not bind verifier and redirect")
				}
			case "refresh_token":
				if r.Form.Get("refresh_token") != "fixture-refresh-secret" {
					t.Error("refresh token not supplied")
				}
			default:
				t.Error("unknown grant")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fixture-access-secret", "refresh_token": "fixture-refresh-secret", "token_type": "Bearer", "expires_in": 3600, "scope": "tools:read"})
		default:
			t.Errorf("unexpected OAuth path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	meta, err := DiscoverAuthorization(context.Background(), nil, AuthChallenge{ResourceURL: server.URL + "/mcp", Scope: "tools:read"})
	if err != nil {
		t.Fatal(err)
	}
	flow, err := NewOAuthFlow(meta, "registered-fixture-client", "http://127.0.0.1:45678/callback", meta.ScopesSupported, nil)
	if err != nil {
		t.Fatal(err)
	}
	authorization, _ := url.Parse(flow.AuthorizationURL)
	query := authorization.Query()
	expectedChallenge = query.Get("code_challenge")
	if query.Get("code_challenge_method") != "S256" || len(query.Get("state")) < 32 || query.Get("scope") != "tools:read" || query.Get("resource") != server.URL+"/mcp" {
		t.Fatalf("authorization request not bound: %v", query)
	}
	callback := url.Values{"state": {query.Get("state")}, "code": {"one-use-code"}, "iss": {meta.Issuer}}
	for _, test := range []struct {
		name  string
		key   string
		value string
	}{{"wrong state", "state", "wrong-state"}, {"wrong issuer", "iss", "https://other.example.test"}, {"missing issuer", "iss", ""}} {
		t.Run(test.name, func(t *testing.T) {
			values := url.Values{}
			for k, v := range callback {
				values[k] = append([]string(nil), v...)
			}
			values.Set(test.key, test.value)
			if _, err := flow.Complete(context.Background(), "http://127.0.0.1:45678/callback?"+values.Encode()); err == nil {
				t.Fatal("invalid callback accepted")
			}
			if tokenCalls.Load() != 0 {
				t.Fatal("invalid callback reached token endpoint")
			}
		})
	}
	token, err := flow.Complete(context.Background(), "http://127.0.0.1:45678/callback?"+callback.Encode())
	if err != nil || token.AccessToken != "fixture-access-secret" || token.ExpiresAt.IsZero() {
		t.Fatalf("token result missing: %v %v", token, err)
	}
	raw, _ := json.Marshal(token)
	for _, diagnostic := range []string{string(raw), fmt.Sprintf("%+v", token), fmt.Sprintf("%#v", token)} {
		if strings.Contains(diagnostic, "fixture-access-secret") || strings.Contains(diagnostic, "fixture-refresh-secret") {
			t.Fatal("token diagnostics leaked credentials")
		}
	}
	if _, err = flow.Complete(context.Background(), "http://127.0.0.1:45678/callback?"+callback.Encode()); err == nil || tokenCalls.Load() != 1 {
		t.Fatal("authorization code replay was accepted")
	}
	refreshed, err := flow.Refresh(context.Background(), token.RefreshToken)
	if err != nil || refreshed.AccessToken == "" || tokenCalls.Load() != 2 {
		t.Fatalf("refresh failed: %v %v", refreshed, err)
	}
}

func TestOAuthRejectsIssuerAndResourceConfusionAndInsecureEndpoints(t *testing.T) {
	for _, test := range []struct {
		name             string
		resourceMismatch bool
		issuerMismatch   bool
	}{{name: "resource mismatch", resourceMismatch: true}, {name: "issuer mismatch", issuerMismatch: true}} {
		t.Run(test.name, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.Contains(r.URL.Path, "oauth-protected-resource") {
					resource := server.URL + "/mcp"
					if test.resourceMismatch {
						resource = "https://other.example.test/mcp"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"resource": resource, "authorization_servers": []string{server.URL}})
					return
				}
				issuer := server.URL
				if test.issuerMismatch {
					issuer = "https://other.example.test"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": server.URL + "/authorize", "token_endpoint": server.URL + "/token", "code_challenge_methods_supported": []string{"S256"}})
			}))
			defer server.Close()
			if _, err := DiscoverAuthorization(context.Background(), nil, AuthChallenge{ResourceURL: server.URL + "/mcp"}); err == nil {
				t.Fatal("confused OAuth metadata accepted")
			}
		})
	}
	if _, err := secureOAuthURL("http://non-loopback.example.test/token"); err == nil {
		t.Fatal("insecure remote OAuth endpoint accepted")
	}
	if _, err := secureOAuthURL("https://username:password@example.test/token"); err == nil {
		t.Fatal("OAuth endpoint URL credentials accepted")
	}
}

func TestHTTPRedirectCannotForwardInjectedBearer(t *testing.T) {
	var contacted atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { contacted.Add(1) }))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	follow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return nil }}
	client, err := ConnectWithOptions(context.Background(), Config{Name: "redirect", URL: origin.URL, Headers: map[string]string{"Authorization": "Bearer never-forward"}}, t.TempDir(), Options{HTTPClient: follow})
	if err == nil || client != nil || contacted.Load() != 0 {
		t.Fatalf("redirect forwarded credentials: client=%v error=%v calls=%d", client, err, contacted.Load())
	}
}

func TestOAuthExplicitPublicClientRegistrationIsBoundToIssuer(t *testing.T) {
	var registered atomic.Int64
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/register" || r.Method != http.MethodPost {
			t.Error("unexpected registration request")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["application_type"] != "native" || body["token_endpoint_auth_method"] != "none" {
			t.Error("registration did not request native public-client semantics")
		}
		registered.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "registered-client", "token_endpoint_auth_method": "none"})
	}))
	defer server.Close()
	metadata := AuthorizationMetadata{Resource: server.URL + "/mcp", Issuer: server.URL, AuthorizationEndpoint: server.URL + "/authorize", TokenEndpoint: server.URL + "/token", RegistrationEndpoint: server.URL + "/register", CodeChallengeMethodsSupported: []string{"S256"}}
	registration, err := RegisterOAuthClient(context.Background(), nil, metadata, "http://127.0.0.1:45678/callback", "GCode fixture")
	if err != nil || registered.Load() != 1 {
		t.Fatalf("%+v %v", registration, err)
	}
	if _, err = NewRegisteredOAuthFlow(metadata, registration, "http://127.0.0.1:45678/callback", nil, nil); err != nil {
		t.Fatal(err)
	}
	registration.Issuer = "https://different.example.test"
	if _, err = NewRegisteredOAuthFlow(metadata, registration, "http://127.0.0.1:45678/callback", nil, nil); err == nil {
		t.Fatal("client ID reused across issuers")
	}
}
