package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfiguredCredentialsAreRedactedAtEveryMCPBoundary(t *testing.T) {
	for _, phase := range []string{"initialize", "tools/list", "tools/call"} {
		t.Run(phase, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request envelope
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				if request.ID == 0 {
					w.WriteHeader(http.StatusAccepted)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if request.Method == phase {
					_ = json.NewEncoder(w).Encode(map[string]any{
						"jsonrpc": "2.0", "id": request.ID,
						"error": map[string]any{"code": -32000, "message": "env-secret / header-token / query-secret"},
					})
					return
				}
				var result any
				switch request.Method {
				case "initialize":
					result = map[string]any{"protocolVersion": "2025-06-18"}
				case "tools/list":
					result = map[string]any{"tools": []any{map[string]any{"name": "echo", "inputSchema": map[string]any{"type": "object"}}}}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
			}))
			defer server.Close()
			config := Config{Name: "redaction", URL: server.URL + "?token=query-secret",
				Headers: map[string]string{"Authorization": "Bearer header-token"}, Env: map[string]string{"SECRET": "env-secret"}}
			client, err := Connect(context.Background(), config, t.TempDir())
			if phase != "initialize" {
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close()
				tools, listErr := client.Tools(context.Background())
				err = listErr
				if phase == "tools/call" {
					if err != nil || len(tools) != 1 {
						t.Fatalf("tools: %+v %v", tools, err)
					}
					_, err = tools[0].Execute(context.Background(), "", json.RawMessage(`{}`))
				}
			}
			if err == nil {
				t.Fatal("expected remote error")
			}
			for _, secret := range []string{"env-secret", "header-token", "query-secret"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("MCP %s leaked %q: %v", phase, secret, err)
				}
			}
			if !strings.Contains(err.Error(), "[redacted]") {
				t.Fatalf("sanitized error lost actionable diagnostic: %v", err)
			}
		})
	}
}

func TestMCPSanitizedCancellationStillMatchesContextError(t *testing.T) {
	err := sanitizeError(context.Canceled, Config{Env: map[string]string{"SECRET": "secret"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatal("sanitization removed cancellation identity")
	}
}

func TestSuccessfulToolMetadataCannotExposeConfiguredCredentials(t *testing.T) {
	tests := []struct {
		name        string
		toolName    string
		description string
		schema      map[string]any
		rejected    bool
	}{
		{
			name: "description", toolName: "safe",
			description: "Uses header-token, env-secret and query-secret.",
			schema:      map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}},
		},
		{name: "tool name", toolName: "env-secret", schema: map[string]any{"type": "object"}, rejected: true},
		{name: "schema description", toolName: "safe", schema: map[string]any{"type": "object", "description": "header-token"}, rejected: true},
		{name: "schema property name", toolName: "safe", schema: map[string]any{"type": "object", "properties": map[string]any{"env-secret": map[string]any{"type": "string"}}}, rejected: true},
		{name: "schema required", toolName: "safe", schema: map[string]any{"type": "object", "required": []any{"header-token"}}, rejected: true},
		{name: "schema enum", toolName: "safe", schema: map[string]any{"type": "string", "enum": []any{"query-secret"}}, rejected: true},
		{name: "schema ref", toolName: "safe", schema: map[string]any{"$ref": "#/$defs/env-secret"}, rejected: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request envelope
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				if request.ID == 0 {
					w.WriteHeader(http.StatusAccepted)
					return
				}
				var result any
				switch request.Method {
				case "initialize":
					result = map[string]any{"protocolVersion": "2025-06-18"}
				case "tools/list":
					result = map[string]any{"tools": []any{map[string]any{
						"name": test.toolName, "description": test.description, "inputSchema": test.schema,
					}}}
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
			}))
			defer server.Close()
			client, err := Connect(context.Background(), Config{
				Name: "metadata", URL: server.URL + "?token=query-secret",
				Env: map[string]string{"SECRET": "env-secret"}, Headers: map[string]string{"Authorization": "Bearer header-token"},
			}, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			tools, err := client.Tools(context.Background())
			if test.rejected != (err != nil) {
				t.Fatalf("metadata rejected = %v, want %v: %v", err != nil, test.rejected, err)
			}
			encoded, _ := json.Marshal(tools)
			if err != nil {
				encoded = append(encoded, err.Error()...)
			} else {
				if len(tools) != 1 {
					t.Fatal("sanitized description lost its tool")
				}
				definition := tools[0].Definition()
				encoded, _ = json.Marshal(definition)
				if !strings.Contains(definition.Description, "[redacted]") {
					t.Fatalf("description did not redact credentials: %q", definition.Description)
				}
				expectedSchema, _ := json.Marshal(test.schema)
				actualSchema, _ := json.Marshal(definition.Parameters)
				if string(actualSchema) != string(expectedSchema) {
					t.Fatal("metadata scrubbing changed the valid schema")
				}
			}
			for _, secret := range []string{"env-secret", "header-token", "query-secret"} {
				if strings.Contains(string(encoded), secret) {
					t.Fatalf("successful tools/list metadata leaked %q: %s", secret, encoded)
				}
			}
		})
	}
}

func TestCredentialArgumentsAreRedactedWithoutChangingOrdinaryArguments(t *testing.T) {
	config := Config{Args: []string{"--api-key", "cli-api-secret", "--access-token=cli-token-secret", "--stdio", "ordinary-package-name"}}
	text := SanitizeText("cli-api-secret cli-token-secret ordinary-package-name --stdio", config)
	if strings.Contains(text, "cli-api-secret") || strings.Contains(text, "cli-token-secret") || !strings.Contains(text, "ordinary-package-name --stdio") {
		t.Fatalf("argument redaction failed: %s", text)
	}
}

func TestShortCredentialCannotEscapeInsideBinaryContent(t *testing.T) {
	client := &Client{config: Config{Headers: map[string]string{"Authorization": "Bearer tiny"}}}
	if !client.binaryHasSecrets([]byte("binary-tiny-payload")) {
		t.Fatal("short bearer credential escaped binary check")
	}
	client = &Client{config: Config{Env: map[string]string{"FIXTURE_ENABLED": "1"}}}
	if client.binaryHasSecrets([]byte("image-data-with-digit-1")) {
		t.Fatal("ordinary one-character mode was treated as a binary credential")
	}
}

func TestExplicitSensitiveListMembersArePrivateClonedAndRedactedAtEveryMCPBoundary(t *testing.T) {
	for _, phase := range []string{"initialize", "tools/list description", "tools/list schema", "tools/call"} {
		t.Run(phase, func(t *testing.T) {
			members := []string{"sensitive-list-member-A", "sensitive-list-member-B"}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				request := fixtureRequest(t, r)
				if request.ID == 0 {
					w.WriteHeader(http.StatusAccepted)
					return
				}
				if phase == "initialize" && request.Method == "initialize" {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "error": map[string]any{"code": -32000, "message": "sensitive-list-member-A"}})
					return
				}
				switch request.Method {
				case "initialize":
					fixtureResult(w, request, map[string]any{"protocolVersion": LatestHandshakeVersion})
				case "tools/list":
					tool := fixtureTool()
					if phase == "tools/list description" {
						tool["description"] = "sensitive-list-member-A"
					}
					if phase == "tools/list schema" {
						tool["inputSchema"] = map[string]any{"type": "object", "description": "sensitive-list-member-A"}
					}
					fixtureResult(w, request, map[string]any{"tools": []any{tool}})
				case "tools/call":
					fixtureResult(w, request, map[string]any{"content": []any{map[string]any{"type": "text", "text": "sensitive-list-member-A"}}, "structuredContent": map[string]any{"nested": []any{"sensitive-list-member-B"}}})
				}
			}))
			defer server.Close()
			cfg := Config{Name: "members", URL: server.URL, Env: map[string]string{"KEY": `["sensitive-list-member-A","sensitive-list-member-B"]`}, SensitiveValues: members}
			wire, _ := json.Marshal(cfg)
			if strings.Contains(string(wire), "SensitiveValues") || strings.Contains(string(wire), "sensitiveValues") {
				t.Fatal("private sensitivity metadata was serialized")
			}
			client, err := Connect(context.Background(), cfg, t.TempDir())
			if phase == "initialize" {
				if err == nil || strings.Contains(err.Error(), "sensitive-list-member-A") {
					t.Fatalf("connection diagnostic leaked member: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			members[0] = "caller-mutated-member"
			tools, err := client.Tools(context.Background())
			if phase == "tools/list schema" {
				if err == nil || strings.Contains(err.Error(), "sensitive-list-member-A") {
					t.Fatalf("schema member leaked: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			definition, _ := json.Marshal(tools[0].Definition())
			if strings.Contains(string(definition), "sensitive-list-member-A") {
				t.Fatal("tool metadata leaked member")
			}
			if phase == "tools/call" {
				out, err := tools[0].Execute(context.Background(), "", json.RawMessage(`{}`))
				if err != nil {
					t.Fatal(err)
				}
				raw, _ := json.Marshal(out)
				if strings.Contains(string(raw), "sensitive-list-member-") {
					t.Fatalf("tool Text/Data leaked member: %s", raw)
				}
			}
		})
	}
}
