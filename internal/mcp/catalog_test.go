package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestToolDisplayTitlesKeepProtocolIdentity(t *testing.T) {
	for _, test := range []struct {
		name, title, annotation, want string
	}{
		{"title", "读取文件", "annotation title", "文件服务 · 读取文件"},
		{"annotation", "", "读取文本", "文件服务 · 读取文本"},
		{"name", "", "", "文件服务 · read_text"},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := ""
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				request := fixtureRequest(t, r)
				switch request.Method {
				case "initialize":
					fixtureResult(w, request, map[string]any{"protocolVersion": LatestHandshakeVersion, "capabilities": map[string]any{}})
				case "notifications/initialized":
					w.WriteHeader(http.StatusAccepted)
				case "tools/list":
					fixtureResult(w, request, map[string]any{"tools": []any{map[string]any{
						"name": "read_text", "title": test.title, "annotations": map[string]any{"title": test.annotation},
						"inputSchema": map[string]any{"type": "object"},
					}}})
				case "tools/call":
					called, _ = request.Params.(map[string]any)["name"].(string)
					fixtureResult(w, request, map[string]any{"content": []any{map[string]any{"type": "text", "text": "file contents"}}})
				default:
					t.Errorf("unexpected method %s", request.Method)
				}
			}))
			defer server.Close()
			client, err := Connect(t.Context(), Config{Name: "文件服务", Transport: "http", URL: server.URL}, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			tools, err := client.Tools(t.Context())
			if err != nil || len(tools) != 1 {
				t.Fatalf("tool discovery failed: %v", err)
			}
			definition := tools[0].Definition()
			if definition.DisplayName != test.want || !strings.HasPrefix(definition.Name, "mcp__") {
				t.Fatalf("incorrect display and protocol identities: %+v", definition)
			}
			wire, _ := json.Marshal(definition)
			if strings.Contains(string(wire), "DisplayName") || strings.Contains(string(wire), test.want) {
				t.Fatal("host display name leaked into model schema")
			}
			if _, err := tools[0].Execute(t.Context(), "", json.RawMessage(`{}`)); err != nil || called != "read_text" {
				t.Fatalf("display title changed the server call: name=%q error=%v", called, err)
			}
		})
	}
}
